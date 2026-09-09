// Package contentfilter menegakkan aturan penyaring konten atas isi permintaan dan jawaban.
//
// Enam jenis aturan dilayani: batas ukuran permintaan, pola terlarang, pola yang diwajibkan,
// pembatasan model, pembatasan provider, dan moderasi eksternal. Yang terakhir BELUM
// dikerjakan — lihat catatan Inert di bawah, dan baca itu sebelum menyalakan aturan moderasi
// di produksi.
//
// # Arah kesalahan tidak simetris, dan kodenya mengikuti itu
//
// Pola TERLARANG yang tidak bisa dievaluasi diloloskan; pola yang DIWAJIBKAN yang tidak bisa
// dievaluasi memblokir. Itu bukan ketidakkonsistenan: "tidak diketahui" pada daftar hitam
// berarti belum terbukti buruk, sedangkan pada daftar putih berarti belum terbukti boleh.
// Membalik salah satunya menghasilkan kegagalan yang mahal — daftar hitam yang gagal-tertutup
// mematikan seluruh lalu lintas karena satu pola yang salah tulis, dan daftar putih yang
// gagal-terbuka meloloskan tepat apa yang seharusnya dijaga.
//
// # Anggaran waktu, dan mengapa bukan soal ReDoS
//
// regexp di Go memakai RE2, yang berjalan linear terhadap panjang masukan dan tidak bisa
// meledak karena backtracking. Jadi kolom max_eval_ms BUKAN penangkal ReDoS klasik. Yang
// dijaganya adalah biaya nyata yang tetap ada: puluhan aturan dikalikan permintaan sepanjang
// megabyte, di jalur yang sedang ditunggu pengguna. Aturan yang melewati anggarannya dicatat
// ke content_filters.eval_timeout_count, dan penghitung itu satu-satunya cara operator tahu
// aturannya terlalu mahal — tanpa itu, aturan yang selalu kehabisan waktu tampak persis sama
// dengan aturan yang tidak pernah cocok.
package contentfilter

import (
	"context"
	"log/slog"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/NexGen-X/Route-X/internal/database/repo/policy"
)

const (
	// defaultTTL adalah umur salinan aturan di memori.
	defaultTTL = 5 * time.Second

	// batasTeksTerjaga adalah panjang teks yang mulai dievaluasi dengan penjaga waktu.
	//
	// Di bawahnya, pencocokan RE2 atas beberapa kilobyte selesai dalam mikrodetik dan biaya
	// membuat goroutine penjaga justru lebih besar daripada yang dijaganya. Di atasnya,
	// anggaran waktu mulai bermakna.
	batasTeksTerjaga = 4 << 10
)

// Source memasok aturan penyaring aktif dan mencatat pelanggaran anggaran waktu.
// Dipenuhi *policy.Repo.
type Source interface {
	ActiveFilters(ctx context.Context) ([]*policy.ContentFilter, error)
	RecordEvalTimeout(ctx context.Context, id string) error
}

// Subject adalah bahan yang diperiksa.
//
// Field yang kosong berarti "belum diketahui" dan aturan yang bergantung padanya dilewati.
// Itu yang membuat pemeriksaan bisa dijalankan bertahap: ukuran dan pola begitu body terbaca,
// pembatasan model setelah alias diselesaikan, pembatasan provider setelah kandidat dipilih.
type Subject struct {
	// Text adalah teks yang dicocokkan dengan pola.
	Text string
	// Bytes adalah ukuran body mentah, untuk aturan request_size. Nol melewatkannya.
	Bytes int64
	// ModelID adalah models.id kanonik, untuk aturan model_restriction.
	ModelID string
	// ProviderID adalah providers.id, untuk aturan provider_restriction.
	ProviderID string
	// Response true berarti yang diperiksa adalah JAWABAN, sehingga hanya aturan
	// ber-applies_to 'response' atau 'both' yang berlaku.
	Response bool
}

// Verdict adalah hasil pemeriksaan.
type Verdict struct {
	// Blocked true berarti permintaan atau jawaban harus ditolak.
	Blocked bool
	// Filter adalah aturan yang memblokir, nil bila tidak ada.
	Filter *policy.ContentFilter
	// Reason adalah keterangan yang AMAN dikirim ke klien: ia menyebut nama dan jenis
	// aturan, tidak pernah isi permintaan maupun polanya. Pola adalah setelan operator dan
	// membocorkannya memberi tahu penyerang bentuk penghindaran yang tepat.
	Reason string
	// Warnings memuat aturan berjenis warn yang terpicu. Permintaan tetap dilayani.
	Warnings []*policy.ContentFilter
}

// aturan adalah satu penyaring yang sudah siap dievaluasi.
type aturan struct {
	row *policy.ContentFilter
	// re terisi untuk pola berbentuk regex yang berhasil dikompilasi.
	re *regexp.Regexp
	// pola adalah bentuk yang dipakai pencocokan substring, sudah dilipat huruf bila aturan
	// tidak peka besar-kecil.
	pola string
	// rusak true berarti aturan tidak bisa dievaluasi (regex tidak sah, atau jenis yang
	// belum didukung). Diperlakukan sesuai arah kesalahan jenisnya.
	rusak bool
}

// Engine menyimpan salinan aturan beserta regexp yang sudah dikompilasi.
//
// Aman dipakai bersamaan oleh banyak goroutine.
type Engine struct {
	source Source
	logger *slog.Logger
	ttl    time.Duration
	now    func() time.Time

	mu     sync.RWMutex
	dimuat time.Time
	aturan []aturan
	// dilaporkan menandai aturan bermasalah yang sudah dikeluhkan, supaya keluhannya tidak
	// diulang setiap kali salinan dimuat ulang — yaitu setiap lima detik, selamanya.
	dilaporkan map[string]struct{}
	// pemuatan menyatukan pemuatan ulang yang bersamaan menjadi satu query,
	// seperti di internal/billing dan internal/router.
	pemuatan sync.Mutex
}

// Option menyetel Engine saat konstruksi.
type Option func(*Engine)

// WithTTL mengubah umur salinan aturan. Nilai <= 0 diabaikan.
func WithTTL(d time.Duration) Option {
	return func(e *Engine) {
		if d > 0 {
			e.ttl = d
		}
	}
}

// WithClock mengganti sumber waktu, untuk test kedaluwarsa salinan.
func WithClock(now func() time.Time) Option {
	return func(e *Engine) {
		if now != nil {
			e.now = now
		}
	}
}

// NewEngine membuat mesin penyaring konten.
//
// source nil sah dan berarti tidak ada penyaring yang ditegakkan.
func NewEngine(source Source, logger *slog.Logger, opts ...Option) *Engine {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	e := &Engine{
		source: source, logger: logger, ttl: defaultTTL, now: time.Now,
		dilaporkan: make(map[string]struct{}),
	}
	for _, o := range opts {
		if o != nil {
			o(e)
		}
	}
	return e
}

// Invalidate memaksa pembacaan ulang pada pemakaian berikutnya.
func (e *Engine) Invalidate() {
	e.mu.Lock()
	e.dimuat = time.Time{}
	e.mu.Unlock()
}

// Check menjalankan seluruh aturan yang berlaku dan berhenti pada pemblokir pertama.
//
// Berhenti pada yang pertama, bukan mengumpulkan semuanya: aturan diurutkan operator lewat
// priority, dan yang lebih murah didahulukan justru supaya permintaan buruk berhenti sebelum
// pola yang mahal dijalankan. Mengumpulkan seluruh pelanggaran berarti membayar seluruh
// biaya evaluasi untuk permintaan yang sudah pasti ditolak.
func (e *Engine) Check(ctx context.Context, s Subject) Verdict {
	e.muat(ctx)

	e.mu.RLock()
	daftar := e.aturan
	e.mu.RUnlock()

	var v Verdict
	for _, a := range daftar {
		if !berlaku(a.row, s) {
			continue
		}
		if !e.terpicu(ctx, a, s) {
			continue
		}
		if a.row.Action == policy.ActionWarn {
			v.Warnings = append(v.Warnings, a.row)
			continue
		}
		v.Blocked, v.Filter, v.Reason = true, a.row, alasan(a.row)
		return v
	}
	return v
}

// berlaku melaporkan apakah aturan ini punya bahan yang dibutuhkannya pada Subject ini.
func berlaku(f *policy.ContentFilter, s Subject) bool {
	switch f.AppliesTo {
	case policy.AppliesToRequest:
		if s.Response {
			return false
		}
	case policy.AppliesToResponse:
		if !s.Response {
			return false
		}
	}

	switch f.Kind {
	case policy.FilterRequestSize:
		// Ukuran hanya bermakna untuk permintaan, dan hanya bila pemanggil mengisinya.
		return !s.Response && s.Bytes > 0 && f.MaxRequestBytes != nil
	case policy.FilterBlockedPattern, policy.FilterAllowedPattern:
		return s.Text != ""
	case policy.FilterModelRestriction:
		return s.ModelID != ""
	case policy.FilterProviderRestriction:
		return s.ProviderID != ""
	case policy.FilterModeration:
		// Belum dikerjakan; keluhannya sudah dicatat saat pemuatan.
		return false
	}
	return false
}

// terpicu mengevaluasi satu aturan.
func (e *Engine) terpicu(ctx context.Context, a aturan, s Subject) bool {
	switch a.row.Kind {
	case policy.FilterRequestSize:
		return s.Bytes > *a.row.MaxRequestBytes

	case policy.FilterModelRestriction:
		return s.ModelID == a.row.ModelID

	case policy.FilterProviderRestriction:
		return s.ProviderID == a.row.ProviderID

	case policy.FilterBlockedPattern:
		if a.rusak {
			// Daftar hitam yang tidak bisa dievaluasi meloloskan: satu pola salah tulis
			// tidak boleh mematikan seluruh lalu lintas inference.
			return false
		}
		cocok, habis := e.cocok(ctx, a, s.Text)
		return cocok && !habis

	case policy.FilterAllowedPattern:
		if a.rusak {
			// Daftar putih yang tidak bisa dievaluasi memblokir: tidak ada bukti bahwa isi
			// ini termasuk yang diizinkan.
			return true
		}
		cocok, habis := e.cocok(ctx, a, s.Text)
		if habis {
			return true
		}
		// Terpicu justru ketika TIDAK cocok: aturan ini menyatakan bentuk yang diizinkan.
		return !cocok
	}
	return false
}

// cocok menjalankan pencocokan pola dengan anggaran waktu.
//
// Nilai balik kedua true berarti anggaran waktunya habis dan hasil pencocokan tidak
// diketahui. Pemanggil yang memutuskan artinya, karena artinya berbeda per jenis aturan.
func (e *Engine) cocok(ctx context.Context, a aturan, teks string) (cocok, habis bool) {
	jalankan := func() bool {
		if a.re != nil {
			return a.re.MatchString(teks)
		}
		if a.row.CaseSensitive {
			return strings.Contains(teks, a.pola)
		}
		return strings.Contains(strings.ToLower(teks), a.pola)
	}

	// Teks pendek dievaluasi langsung: RE2 linear, dan goroutine penjaga untuk beberapa
	// kilobyte lebih mahal daripada pencocokannya sendiri.
	if len(teks) < batasTeksTerjaga {
		return jalankan(), false
	}

	anggaran := time.Duration(a.row.MaxEvalMS) * time.Millisecond
	if anggaran <= 0 {
		return jalankan(), false
	}

	// Goroutine ini TIDAK dibatalkan saat anggaran habis: regexp tidak punya cara
	// menghentikan pencocokan yang sedang berjalan. Ia dibiarkan selesai sendiri, dan itu
	// aman justru karena RE2 linear — pencocokannya pasti berakhir, dan buffer berkapasitas
	// satu membuat pengirimannya tidak pernah menggantung setelah pembacanya pergi.
	hasil := make(chan bool, 1)
	go func() { hasil <- jalankan() }()

	timer := time.NewTimer(anggaran)
	defer timer.Stop()
	select {
	case c := <-hasil:
		return c, false
	case <-timer.C:
		e.catatWaktuHabis(ctx, a.row)
		return false, true
	}
}

// catatWaktuHabis menaikkan penghitung anggaran waktu di database.
//
// Dijalankan di latar dengan context sendiri: pemanggilnya berada di jalur request yang
// sedang ditunggu pengguna, dan context request bisa dibatalkan tepat setelah ini — yang
// akan membuat penghitungnya justru tidak pernah naik ketika aturannya paling sering
// kehabisan waktu.
func (e *Engine) catatWaktuHabis(ctx context.Context, f *policy.ContentFilter) {
	if e.source == nil {
		return
	}
	e.logger.WarnContext(ctx, "penyaring konten melewati anggaran waktunya",
		"penyaring", f.Name, "kind", f.Kind, "anggaran_ms", f.MaxEvalMS)

	go func() {
		bg, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := e.source.RecordEvalTimeout(bg, f.ID); err != nil {
			e.logger.Warn("penghitung waktu habis penyaring gagal dinaikkan",
				"penyaring", f.Name, "error", err)
		}
	}()
}

// alasan menyusun keterangan penolakan yang aman dikirim ke klien.
//
// Nama aturan disebut karena itu setelan operator dan justru itu yang membuat pengguna bisa
// menghubungi orang yang tepat. Pola TIDAK disebut: ia memberi tahu penyerang bentuk
// penghindaran yang tepat, dan isi permintaan juga tidak, karena pesan ini ikut ke log
// bersama.
func alasan(f *policy.ContentFilter) string {
	switch f.Kind {
	case policy.FilterRequestSize:
		return "permintaan melebihi batas ukuran yang ditetapkan kebijakan " + kutip(f.Name)
	case policy.FilterBlockedPattern:
		return "isi permintaan tertahan kebijakan konten " + kutip(f.Name)
	case policy.FilterAllowedPattern:
		return "isi permintaan tidak memenuhi kebijakan konten " + kutip(f.Name)
	case policy.FilterModelRestriction:
		return "model ini dibatasi kebijakan " + kutip(f.Name)
	case policy.FilterProviderRestriction:
		return "provider untuk model ini dibatasi kebijakan " + kutip(f.Name)
	}
	return "permintaan tertahan kebijakan konten " + kutip(f.Name)
}

// kutip membungkus nama kebijakan dengan tanda kutip, dengan panjang dibatasi.
func kutip(v string) string {
	const maks = 80
	if len(v) > maks {
		v = v[:maks] + "…"
	}
	return "\"" + v + "\""
}

// Inert mengembalikan nama aturan yang AKTIF di database tetapi tidak menegakkan apa pun.
//
// Wajib ada, dan bukan untuk kenyamanan. Aturan moderasi belum dikerjakan, dan regex yang
// tidak sah tidak ditolak oleh constraint tabel mana pun. Keduanya menghasilkan kegagalan
// yang paling mahal yang bisa dimiliki penyaring konten: operator melihat aturannya
// "enabled" di dashboard dan menyangka perlindungannya berjalan, sementara tidak satu pun
// permintaan pernah diperiksa olehnya. Dashboard Fase 12 menampilkan daftar ini.
func (e *Engine) Inert() []string {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var out []string
	for _, a := range e.aturan {
		if a.rusak {
			out = append(out, a.row.Name)
		}
	}
	return out
}

// muat memuat ulang salinan aturan bila sudah kedaluwarsa.
//
// Kegagalan pembacaan mempertahankan salinan lama tanpa memperbarui waktu muat, sama seperti
// di internal/ratelimit dan internal/billing.
func (e *Engine) muat(ctx context.Context) {
	if e.source == nil {
		return
	}

	e.mu.RLock()
	segar := !e.dimuat.IsZero() && e.now().Sub(e.dimuat) < e.ttl
	e.mu.RUnlock()
	if segar {
		return
	}

	e.pemuatan.Lock()
	defer e.pemuatan.Unlock()

	e.mu.RLock()
	segar = !e.dimuat.IsZero() && e.now().Sub(e.dimuat) < e.ttl
	e.mu.RUnlock()
	if segar {
		return
	}

	rows, err := e.source.ActiveFilters(ctx)
	if err != nil {
		e.logger.WarnContext(ctx, "penyaring konten gagal dibaca, memakai salinan sebelumnya", "error", err)
		return
	}

	daftar := make([]aturan, 0, len(rows))
	var keluhan []*policy.ContentFilter
	for _, row := range rows {
		a := siapkan(row)
		if a.rusak {
			keluhan = append(keluhan, row)
		}
		daftar = append(daftar, a)
	}

	e.mu.Lock()
	e.aturan, e.dimuat = daftar, e.now()
	// Keluhan dilaporkan sekali per aturan, bukan setiap pemuatan: pemuatan terjadi setiap
	// lima detik selamanya, dan log yang mengulang hal yang sama ribuan kali sehari adalah
	// log yang berhenti dibaca.
	baru := keluhan[:0:0]
	for _, row := range keluhan {
		if _, sudah := e.dilaporkan[row.ID]; sudah {
			continue
		}
		e.dilaporkan[row.ID] = struct{}{}
		baru = append(baru, row)
	}
	e.mu.Unlock()

	for _, row := range baru {
		e.logger.WarnContext(ctx, "penyaring konten aktif tetapi TIDAK menegakkan apa pun",
			"penyaring", row.Name, "kind", row.Kind, "sebab", sebabRusak(row))
	}
}

// siapkan mengubah satu baris menjadi aturan yang siap dievaluasi.
func siapkan(row *policy.ContentFilter) aturan {
	a := aturan{row: row}

	switch row.Kind {
	case policy.FilterModeration:
		// Integrasi moderasi eksternal belum dikerjakan. Ditandai rusak supaya muncul di
		// Inert() alih-alih diam-diam tidak melakukan apa pun.
		a.rusak = true
		return a

	case policy.FilterBlockedPattern, policy.FilterAllowedPattern:
		if row.PatternType == policy.PatternRegex {
			pola := row.Pattern
			if !row.CaseSensitive {
				// Bendera di dalam pola, bukan pelipatan huruf pada masukan: pelipatan akan
				// mengubah teks yang dicocokkan sehingga pola yang memang menargetkan huruf
				// besar tidak akan pernah cocok.
				pola = "(?i)" + pola
			}
			re, err := regexp.Compile(pola)
			if err != nil {
				// Bentuk regex tidak diperiksa constraint tabel mana pun, jadi baris seperti
				// ini memang bisa ada.
				a.rusak = true
				return a
			}
			a.re = re
			return a
		}
		a.pola = row.Pattern
		if !row.CaseSensitive {
			a.pola = strings.ToLower(a.pola)
		}
		return a
	}
	return a
}

// sebabRusak menjelaskan mengapa satu aturan tidak menegakkan apa pun.
func sebabRusak(row *policy.ContentFilter) string {
	if row.Kind == policy.FilterModeration {
		return "moderasi eksternal belum didukung gateway ini"
	}
	return "pola regex tidak bisa dikompilasi"
}
