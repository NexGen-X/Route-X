package router

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
)

// Rule adalah satu aturan routing dalam bentuk yang dipakai mesin, bukan bentuk barisnya
// di database.
//
// Bedanya bukan gaya. Baris database menyimpan tenggat sebagai milidetik bertipe int
// karena itulah tipe kolomnya, dan strategi sebagai text karena itulah yang bisa dijaga
// check constraint. Di jalur request keduanya salah bentuk: durasi bertipe int menuntut
// setiap pemakai mengalikan sendiri dengan time.Millisecond — dan pemakai yang lupa tidak
// mendapat error, hanya tenggat sejuta kali lebih panjang. Konversinya dilakukan sekali di
// RuleFromRow, bukan di setiap pemakaian.
type Rule struct {
	ID          string
	Name        string
	Description string
	Priority    int

	// Kondisi pencocokan. Kosong berarti tidak membatasi.
	MatchModelID      string
	MatchAPIKeyID     string
	MatchCapabilities []string

	Strategy Strategy

	// MaxAttempts adalah jumlah percobaan di dalam SATU kandidat, termasuk yang pertama.
	MaxAttempts int
	// BackoffBase adalah jeda dasar backoff eksponensial; jitter ditambahkan saat dipakai.
	BackoffBase time.Duration

	FailureThreshold int
	OpenDuration     time.Duration
	HalfOpenProbes   int

	// Pipeline adalah resep failover multi-model (combo). Nilai nil berarti aturan
	// memakai jalur lama: satu model, satu daftar provider, tanpa cascade antar model.
	Pipeline *ComboPipeline

	// VirtualAlias, bila terisi, membuat aturan ini bisa dipanggil klien seolah ia
	// model tersendiri. Ini menggantikan tag [combo:alias=...] di description.
	VirtualAlias string

	// Providers terurut position naik. Kosong berarti aturan ini berlaku atas SEMUA
	// provider yang bisa melayani model yang diminta.
	Providers []RuleProvider
}

// RuleProvider adalah satu kandidat provider di dalam sebuah aturan.
type RuleProvider struct {
	ProviderID string
	Position   int
	// Weight nil berarti memakai bobot provider.
	Weight *int
}

// ComboPipeline adalah resep failover multi-model: kandidat diurutkan per Strategi,
// lalu dicoba satu per satu sampai anggaran Attempts habis atau ada yang berhasil.
type ComboPipeline struct {
	// Strategy mengurutkan model. Level Go menerima 6 nilai yang dikenal Strategy,
	// meski constraint database pipeline hanya menerima 4 yang disuguhkan UI.
	Strategy Strategy `json:"strategy"`
	// Attempts adalah ANGGARAN TOTAL percobaan lintas seluruh model, bukan per model.
	Attempts int `json:"attempts"`
	// Models adalah daftar model_id terurut. Urutannya adalah urutan fallback
	// ketika Strategi tidak menggesernya: misalnya strategi priority mengurutkan
	// kandidat gabungan per prioritas provider, sehingga provider model kedua
	// yang berprioritas lebih kecil bisa didahulukan melebihi model pertama.
	// Anggap urutan array sebagai preferensi, bukan jaminan absolut.
	Models []string `json:"models"`
}

// Combo melaporkan apakah aturan ini memakai combo pipeline (jalur baru).
//
// Aman dipanggil pada aturan nil, sama seperti Matches dan String: pemanggil di paket
// gateway memanggilnya pada Decision.Rule yang boleh nil, dan nil di sana berarti
// "tidak ada aturan yang cocok" — keadaan biasa, bukan kesalahan yang pantas jadi panic.
func (r *Rule) Combo() bool {
	if r == nil {
		return false
	}
	return r.Pipeline != nil && len(r.Pipeline.Models) > 1
}

// RuleFromRow mengubah baris aturan menjadi bentuk yang dipakai mesin.
//
// Strategi yang tidak dikenal ditolak, bukan diperlakukan sebagai priority: nilai seperti
// itu hanya bisa masuk kalau check constraint dan kode ini tidak lagi sepakat, dan diam
// pada keadaan itu berarti seluruh lalu lintas yang cocok aturan tersebut dirutekan dengan
// cara yang bukan diminta operator — tanpa satu pun tanda di log.
func RuleFromRow(row *upstream.RoutingRule) (*Rule, error) {
	if row == nil {
		return nil, fmt.Errorf("aturan routing kosong")
	}
	strategy := Strategy(row.Strategy)
	if !strategy.Valid() {
		return nil, fmt.Errorf("aturan %q memakai strategi %q yang tidak dikenal", row.Name, row.Strategy)
	}

	r := &Rule{
		ID:                row.ID,
		Name:              row.Name,
		Priority:          row.Priority,
		MatchCapabilities: slices.Clone(row.MatchCapabilities),
		Strategy:          strategy,
		MaxAttempts:       row.MaxAttempts,
		BackoffBase:       time.Duration(row.BackoffMS) * time.Millisecond,
		FailureThreshold:  row.FailureThreshold,
		OpenDuration:      time.Duration(row.OpenDurationMS) * time.Millisecond,
		HalfOpenProbes:    row.HalfOpenProbes,
	}
	if row.Description != nil {
		r.Description = *row.Description
	}
	if row.MatchModelID != nil {
		r.MatchModelID = *row.MatchModelID
	}
	if row.MatchAPIKeyID != nil {
		r.MatchAPIKeyID = *row.MatchAPIKeyID
	}
	for _, rp := range row.Providers {
		r.Providers = append(r.Providers, RuleProvider{
			ProviderID: rp.ProviderID,
			Position:   rp.Position,
			Weight:     rp.Weight,
		})
	}
	slices.SortStableFunc(r.Providers, func(a, b RuleProvider) int {
		return a.Position - b.Position
	})

	// Pipeline diurai di sini, bukan di setiap pemakai, sama seperti konversi milidetik:
	// jsonb mentah hanya boleh disentuh satu tempat. Yang diurai adalah seluruh isi
	// resep; penerapan strategi dan pemotongan anggaran adalah urusan gateway (PR #2).
	//
	// Pipeline yang rusak TIDAK menggagalkan aturan. Data seperti itu hanya bisa muncul
	// kalau constraint database dan pengurai tidak lagi sepakat; memadamkan seluruh
	// aturan karena satu field opsional tak terbaca akan diam-diam merutekan lalu
	// lintasnya ke aturan berikutnya. Aturan tetap dipakai tanpa cascade (jalur lama),
	// dan keadaannya dicatat sebagai peringatan.
	if len(row.Pipeline) > 0 {
		if p, err := parseComboPipeline(row.Pipeline); err != nil {
			slog.Warn("pipeline combo aturan tidak bisa dibaca, aturan dipakai tanpa cascade",
				"aturan", row.Name, "error", err)
		} else {
			r.Pipeline = p
		}
	}
	if row.VirtualAlias != nil {
		r.VirtualAlias = *row.VirtualAlias
	}
	return r, nil
}

// parseComboPipeline mengurai jsonb resep combo menjadi ComboPipeline.
//
// Strategi tidak divalidasi di sini: constraint pipeline di database menerima 4 nilai
// UI, tetapi enum Strategy di Go mengenal 6 — memvalidasi di sini berarti menolak
// keadaan yang sah menurut salah satunya. Yang tidak dikenal akan ditolak oleh
// pemakai pipeline saat strategi dipakai, bukan saat resepnya dibaca.
func parseComboPipeline(raw []byte) (*ComboPipeline, error) {
	var p ComboPipeline
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// ComboPipelineDTO adalah resep combo dalam bentuk kontrak API admin (JSON).
//
// Sengaja tipe terpisah dari ComboPipeline mesin: strategi di sini string biasa dan hanya
// menerima 4 nilai yang diterima constraint pipeline (migrasi 0014), sedangkan ComboPipeline
// memakai enum Strategy yang mengenal 6. Bentuk mesin dihasilkan RuleFromRow dari jsonb;
// bentuk API inilah satu-satunya yang dipakai admin handler, supaya perubahan salah satunya
// tidak otomatis mengubah kontrak publik admin API.
type ComboPipelineDTO struct {
	Strategy string   `json:"strategy"`
	Attempts int      `json:"attempts"`
	Models   []string `json:"models"`
}

// pipelineStrategies adalah strategi yang diterima constraint
// routing_rules_pipeline_strategy_valid (migrasi 0014). Hanya empat: resep combo hanya
// bisa diisi dari UI yang menyuguhkan empat tombol ini, dan menerima lebih banyak di sini
// berarti mengizinkan keadaan yang tidak bisa dibuat operator mana pun — lalu menolaknya
// di database sebagai pelanggaran constraint, padahal penolakannya bisa lebih dini dan
// lebih jelas dilakukan di sini.
var pipelineStrategies = []string{
	string(StrategyPriority),
	string(StrategyRoundRobin),
	string(StrategyLowestLatency),
	string(StrategyLowestCost),
}

// Valid melaporkan kesalahan yang membuat resep tidak bisa disimpan ke kolom pipeline.
//
// Aturannya mengikuti constraint pipeline persis: strategi salah satu dari empat nilai UI,
// attempts 1-20, models 1-8, tanpa model ganda maupun kosong. Memeriksa di sini, bukan
// membiarkan database menolak: pelanggaran constraint baru terlihat sebagai error generik
// setelah aturan dibuat, dan operator kehilangan seluruh isian formnya padahal sebabnya
// satu field.
func (p *ComboPipelineDTO) Valid() error {
	if p == nil {
		return fmt.Errorf("pipeline kosong")
	}
	if !slices.Contains(pipelineStrategies, p.Strategy) {
		return fmt.Errorf("strategi pipeline %q tidak dikenal; yang diterima: %s",
			p.Strategy, strings.Join(pipelineStrategies, ", "))
	}
	if p.Attempts < 1 || p.Attempts > 20 {
		return fmt.Errorf("attempts pipeline %d di luar jangkauan 1-20", p.Attempts)
	}
	if n := len(p.Models); n < 1 || n > 8 {
		return fmt.Errorf("jumlah model pipeline %d di luar jangkauan 1-8", n)
	}
	seen := make(map[string]bool, len(p.Models))
	for i, m := range p.Models {
		if strings.TrimSpace(m) == "" {
			return fmt.Errorf("model ke-%d pipeline kosong", i+1)
		}
		if seen[m] {
			return fmt.Errorf("model %q disebut dua kali dalam pipeline", m)
		}
		seen[m] = true
	}
	return nil
}

// ParseComboPipelineDTO mengurai jsonb pipeline menjadi bentuk API dan memvalidasinya.
//
// Dipakai admin handler untuk membaca isi body permintaan sekaligus untuk mengubah kolom
// pipeline mentah menjadi DTO saat aturan dibaca kembali.
func ParseComboPipelineDTO(raw []byte) (*ComboPipelineDTO, error) {
	var p ComboPipelineDTO
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, fmt.Errorf("pipeline bukan JSON yang sah: %w", err)
	}
	if err := p.Valid(); err != nil {
		return nil, err
	}
	return &p, nil
}

// Alias mengembalikan virtual model aturan ini: kolom virtual_alias bila terisi, jika
// tidak, tag [combo:alias=...] di description.
//
// Urutan ini sengaja, dan membaliknya akan memutus lalu lintas produksi: aturan yang
// dibuat sebelum kolom virtual_alias ada (migrasi 0014) masih memakai tag di description,
// dan kompatibilitas itu wajib tetap berjalan sampai seluruh aturan dikonversi. Kolom
// adalah sumber kebenaran untuk aturan baru; tag adalah sumber kebenaran untuk aturan lama
// yang belum dikonversi.
func (r *Rule) Alias() string {
	if r == nil {
		return ""
	}
	if r.VirtualAlias != "" {
		return r.VirtualAlias
	}
	return ExtractComboAlias(r.Description)
}

// ExtractComboAlias membaca virtual model alias dari deskripsi aturan combo routing bila ada.
func ExtractComboAlias(description string) string {
	const prefix = "[combo:alias="
	idx := strings.Index(description, prefix)
	if idx == -1 {
		return ""
	}
	sub := description[idx+len(prefix):]
	end := strings.IndexByte(sub, ']')
	if end == -1 {
		return ""
	}
	return strings.TrimSpace(sub[:end])
}

// Matches melaporkan apakah aturan ini berlaku untuk permintaan tersebut.
//
// Seluruh kondisi digabung dengan AND, dan kondisi yang kosong berarti tidak membatasi —
// sehingga aturan tanpa kondisi apa pun adalah aturan bawaan yang cocok untuk semua
// lalu lintas. Ini semantik yang dijanjikan komentar migrasi 0008, dan satu-satunya
// tempat ia ditegakkan.
func (r *Rule) Matches(req Request) bool {
	if r == nil {
		return false
	}
	alias := r.Alias()
	explicitTargetMatch := req.ModelName != "" && (r.Name == req.ModelName || (alias != "" && alias == req.ModelName))
	if !explicitTargetMatch && r.MatchModelID != "" && r.MatchModelID != req.ModelID {
		return false
	}
	if r.MatchAPIKeyID != "" && r.MatchAPIKeyID != req.APIKeyID {
		return false
	}
	// Aturan cocok bila permintaan MEMINTA seluruh kemampuan yang disyaratkan aturan.
	//
	// Arah pembandingannya mudah terbalik, dan terbaliknya tidak kelihatan sampai
	// produksi: aturan "khusus lalu lintas vision" harus cocok untuk permintaan bergambar
	// yang juga memakai tools, jadi yang diperiksa adalah apakah syarat aturan TERMUAT di
	// kemampuan yang diminta — bukan sebaliknya.
	for _, butuh := range r.MatchCapabilities {
		if !slices.Contains(req.Capabilities, butuh) {
			return false
		}
	}
	return true
}

// FirstMatch mengembalikan aturan pertama yang cocok, atau nil bila tidak ada.
//
// rules WAJIB sudah terurut (priority, created_at) seperti yang dikembalikan
// upstream.RoutingRepo.ActiveRules — "aturan pertama yang cocok menang" tidak bermakna
// kalau urutannya bisa berbeda antar pemanggilan. Fungsi ini tidak mengurutkan ulang,
// justru supaya urutan itu tetap menjadi tanggung jawab satu tempat: query di repository,
// yang indeksnya memang dibuat untuk itu.
func FirstMatch(rules []*Rule, req Request) *Rule {
	// Bila permintaan meminta nama aturan atau alias combo secara spesifik,
	// cari aturan yang cocok secara eksplisit terlebih dahulu. Alias yang dipakai adalah
	// Alias() (kolom virtual_alias, lalu tag description), sehingga kedua jalur — aturan
	// baru dan aturan produksi lama — diperlakukan sama di sini.
	if req.ModelName != "" {
		for _, r := range rules {
			alias := r.Alias()
			if (r.Name == req.ModelName || (alias != "" && alias == req.ModelName)) && r.Matches(req) {
				return r
			}
		}
	}
	for _, r := range rules {
		if r.Matches(req) {
			return r
		}
	}
	return nil
}

// String menyebut aturan dalam bentuk yang aman masuk log.
func (r *Rule) String() string {
	if r == nil {
		return "aturan bawaan (tidak ada aturan yang cocok)"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "aturan %q (prioritas %d, strategi %s", r.Name, r.Priority, r.Strategy)
	if n := len(r.Providers); n > 0 {
		fmt.Fprintf(&b, ", %d kandidat", n)
	}
	b.WriteString(")")
	return b.String()
}
