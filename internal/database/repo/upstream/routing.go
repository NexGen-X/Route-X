package upstream

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/NexGen-X/Route-X/internal/database/repo"
)

// Nilai bawaan aturan routing, mencerminkan default kolom routing_rules di migrasi 0008.
//
// Alasan menyebutnya ulang di Go sama seperti nilai bawaan provider: setiap INSERT
// mengirim nilai konkret, sehingga satu aturan berarti sama entah dibuat lewat paket ini
// atau lewat SQL langsung. Priority tidak diberi konstanta sendiri karena
// routing_rules.priority memang berdefault 100 seperti DefaultPriority, dan artinya pun
// sama: urutan pemeriksaan, makin kecil makin dulu.
const (
	DefaultMaxAttempts      = 3
	DefaultBackoffMS        = 250
	DefaultFailureThreshold = 5
	DefaultOpenDurationMS   = 30000
	DefaultHalfOpenProbes   = 1
)

// RoutingRule adalah satu aturan pemilihan provider beserta kandidatnya.
//
// Aturan dievaluasi berurut priority naik dan ATURAN PERTAMA YANG COCOK MENANG, jadi
// aturan yang lebih spesifik diberi priority lebih kecil. Kondisi yang kosong berarti
// "tidak membatasi", sehingga aturan tanpa kondisi apa pun adalah aturan bawaan yang
// cocok untuk semua lalu lintas.
type RoutingRule struct {
	ID          string
	Name        string
	Description *string
	Priority    int

	// Kondisi pencocokan. nil / kosong berarti tidak membatasi.
	MatchModelID      *string
	MatchAPIKeyID     *string
	MatchCapabilities []string

	Strategy string

	MaxAttempts int
	BackoffMS   int

	FailureThreshold int
	OpenDurationMS   int
	HalfOpenProbes   int

	Enabled   bool
	CreatedAt time.Time
	UpdatedAt time.Time
	CreatedBy *string

	// Pipeline adalah resep failover multi-model (combo) dalam bentuk jsonb mentah.
	// nil berarti aturan memakai jalur lama: satu model + daftar kandidat provider.
	// Isinya diurai menjadi router.ComboPipeline oleh router.RuleFromRow; paket ini
	// sengaja tidak menafsirkkan agar perubahan bentuk resep hanya punya satu tempat.
	Pipeline []byte

	// VirtualAlias, bila tidak nil, membuat aturan bisa dipanggil klien seolah ia model
	// tersendiri. Ini menggantikan tag [combo:alias=...] di description.
	VirtualAlias *string

	// Providers terurut position naik. Kosong berarti aturan ini berlaku atas SEMUA
	// provider yang bisa melayani model yang diminta.
	//
	// Kosong BUKAN keadaan setengah jadi yang boleh ditafsirkan pemanggil sebagai "aturan
	// ini belum siap, lewati saja": skema sengaja tidak memaksa aturan punya kandidat,
	// karena daftar kosong adalah cara menuliskan "pakai strategi ini atas seluruh
	// provider yang melayani model itu" mengikuti provider_models. Aturan yang dilewati
	// diam-diam berarti aturan berikutnya yang menang, dan kekeliruan seperti itu tidak
	// memunculkan pesan apa pun — ia hanya mengirim lalu lintas ke provider yang salah.
	Providers []RuleProvider
}

// RuleProvider adalah satu kandidat provider di dalam sebuah aturan.
type RuleProvider struct {
	ProviderID string
	// Position menentukan urutan fallback: 1 dicoba lebih dulu, dan nilainya unik per
	// aturan supaya urutan itu tidak bergantung pada baris mana yang kebetulan terbaca.
	Position int
	// Weight nil berarti memakai providers.weight.
	Weight *int
}

// RoutingRepo adalah akses data aturan routing beserta kandidat providernya.
type RoutingRepo struct {
	q repo.Querier
}

// NewRoutingRepo membuat RoutingRepo di atas pool maupun transaksi.
func NewRoutingRepo(q repo.Querier) *RoutingRepo { return &RoutingRepo{q: q} }

// routingRuleColumns memakai awalan tabel "r" supaya bentuk baris yang sama juga bisa
// dipakai pada query yang sumber barisnya bukan tabel routing_rules langsung, melainkan
// CTE yang dialiaskan sebagai r — lihat Update dan List.
const routingRuleColumns = `
	r.id::text, r.name, r.description, r.priority,
	r.match_model_id::text, r.match_api_key_id::text, r.match_capabilities,
	r.strategy, r.max_attempts, r.backoff_ms,
	r.failure_threshold, r.open_duration_ms, r.half_open_probes,
	r.enabled, r.created_at, r.updated_at, r.created_by::text,
	r.pipeline, r.virtual_alias`

// ruleProviderColumns adalah kolom kandidat yang ikut terbaca pada baris aturan.
const ruleProviderColumns = `rp.provider_id::text, rp.position, rp.weight`

// ruleProviderJoin menempelkan kandidat pada baris aturannya.
//
// LEFT join, bukan join biasa: aturan tanpa kandidat wajib tetap terbaca karena kosongnya
// daftar itu punya arti tersendiri. Dengan join biasa, justru aturan yang berlaku atas
// semua provider yang hilang dari hasil — tanpa satu pun error.
const ruleProviderJoin = ` left join routing_rule_providers rp on rp.rule_id = r.id`

// ruleDest mengembalikan tujuan scan untuk routingRuleColumns.
//
// Dipisah sebagai fungsi karena dua pembaca memakainya: satu untuk baris aturan saja,
// satu untuk baris aturan yang dilengkapi kandidat. Kalau daftarnya ditulis dua kali,
// menambah kolom di satu tempat dan lupa di tempat lain akan menggeser seluruh sisa
// kolom — dan pergeseran seperti itu baru terlihat sebagai kesalahan tipe, bukan sebagai
// nilai yang salah tempat.
func ruleDest(rule *RoutingRule) []any {
	return []any{
		&rule.ID, &rule.Name, &rule.Description, &rule.Priority,
		&rule.MatchModelID, &rule.MatchAPIKeyID, &rule.MatchCapabilities,
		&rule.Strategy, &rule.MaxAttempts, &rule.BackoffMS,
		&rule.FailureThreshold, &rule.OpenDurationMS, &rule.HalfOpenProbes,
		&rule.Enabled, &rule.CreatedAt, &rule.UpdatedAt, &rule.CreatedBy,
		&rule.Pipeline, &rule.VirtualAlias,
	}
}

// scanRule membaca satu baris sesuai routingRuleColumns, tanpa kandidat.
func scanRule(row pgxRow) (*RoutingRule, error) {
	var rule RoutingRule
	if err := row.Scan(ruleDest(&rule)...); err != nil {
		return nil, err
	}
	return &rule, nil
}

// scanRuleRow membaca satu baris hasil ruleProviderJoin: kolom aturan ditambah paling
// banyak satu kandidat.
//
// Kandidat nil berarti barisnya datang dari aturan tanpa kandidat, bukan dari data yang
// kurang lengkap. Ketiga kolom kandidat dibaca sebagai pointer karena left join
// menghasilkan NULL untuk aturan seperti itu, meskipun position dan provider_id tidak
// pernah NULL di tabelnya.
func scanRuleRow(row pgxRow) (*RoutingRule, *RuleProvider, error) {
	var (
		rule       RoutingRule
		providerID *string
		position   *int
		weight     *int
	)
	if err := row.Scan(append(ruleDest(&rule), &providerID, &position, &weight)...); err != nil {
		return nil, nil, err
	}
	if providerID == nil {
		return &rule, nil, nil
	}
	return &rule, &RuleProvider{ProviderID: *providerID, Position: *position, Weight: weight}, nil
}

// queryRules menjalankan query berbentuk routingRuleColumns + ruleProviderColumns lalu
// menggabungkan baris berulang menjadi satu RoutingRule per aturan.
//
// Query yang dilayaninya WAJIB mengurutkan kolom aturan lebih dulu sehingga baris satu
// aturan selalu berdampingan; penggabungan di sini hanya membandingkan baris dengan
// aturan terakhir, bukan mencari ke seluruh hasil. Itu disengaja: peta pengenal→aturan
// akan menyembunyikan query yang urutannya salah, dan hasilnya baru terasa sebagai daftar
// kandidat yang tercampur antar aturan.
func (r *RoutingRepo) queryRules(ctx context.Context, op, sql string, args ...any) ([]*RoutingRule, error) {
	rows, err := r.q.Query(ctx, sql, args...)
	if err != nil {
		return nil, repo.Err(op, err)
	}
	defer rows.Close()

	var out []*RoutingRule
	for rows.Next() {
		rule, candidate, err := scanRuleRow(rows)
		if err != nil {
			return nil, repo.Err(op, err)
		}
		last := len(out) - 1
		if last < 0 || out[last].ID != rule.ID {
			out = append(out, rule)
			last++
		}
		if candidate != nil {
			out[last].Providers = append(out[last].Providers, *candidate)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, repo.Err(op, err)
	}
	return out, nil
}

// activeRulesSQL adalah query jalur evaluasi. Dipisah sebagai konstanta supaya test bisa
// menjalankan EXPLAIN pada teks yang persis sama dengan yang dipakai produksi — bukti
// pemakaian indeks jadi tidak bisa basi karena querynya diubah tanpa test ikut diubah.
//
// Penengah terakhir adalah r.id, di belakang (priority, created_at) yang dijanjikan skema.
// Alasannya: created_at berasal dari now(), yaitu waktu TRANSAKSI, jadi aturan yang dibuat
// dalam satu transaksi punya created_at yang identik — dan untuk baris yang sama menurut
// seluruh kunci pengurutan, PostgreSQL tidak menjanjikan urutan apa pun. Tanpa penengah
// ini, "aturan pertama yang cocok menang" bisa berarti aturan yang berbeda antar query.
const activeRulesSQL = `
	select ` + routingRuleColumns + `, ` + ruleProviderColumns + `
	from routing_rules r` + ruleProviderJoin + `
	where r.enabled
	order by r.priority asc, r.created_at asc, r.id asc, rp.position asc`

// ActiveRules mengembalikan SEMUA aturan aktif berurut (priority, created_at) beserta
// kandidat providernya, siap dievaluasi mesin routing.
//
// Ini dipanggil di jalur request (di belakang cache), jadi WAJIB tidak N+1: aturan dan
// kandidatnya diambil satu query, bukan satu query per aturan. Kalau tidak, jumlah
// perjalanan ke database tumbuh mengikuti banyaknya aturan yang dipasang operator, dan
// biayanya muncul pada setiap pengisian ulang cache.
//
// Satu pernyataan juga berarti satu snapshot, dan itu alasan kedua yang tidak kalah
// penting. Bila aturan dan kandidatnya dibaca dua query terpisah, kandidat yang terhapus
// di antara keduanya membuat aturan terbaca dengan daftar KOSONG — dan daftar kosong bukan
// sekadar data yang hilang, artinya berubah menjadi "berlaku atas semua provider". Tidak
// ada error yang muncul dari keadaan itu; yang muncul hanya lalu lintas di provider yang
// tidak dipilih siapa pun.
//
// Aturan dengan enabled=false tidak ikut, dan penyaringannya persis bentuk yang dilayani
// indeks partial routing_rules_eval_idx (priority, created_at) where enabled.
func (r *RoutingRepo) ActiveRules(ctx context.Context) ([]*RoutingRule, error) {
	return r.queryRules(ctx, "mengambil aturan routing aktif", activeRulesSQL)
}

// Get mengambil satu aturan beserta kandidatnya, aktif maupun tidak.
//
// Aturan yang dimatikan tetap dikembalikan: dashboard berhak menyuntingnya, dan "aturan
// ini dimatikan" adalah jawaban yang berbeda dari "aturan tidak ada".
func (r *RoutingRepo) Get(ctx context.Context, id string) (*RoutingRule, error) {
	const op = "mengambil aturan routing"
	if !idOK(id) {
		return nil, fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}

	rules, err := r.queryRules(ctx, op, `
		select `+routingRuleColumns+`, `+ruleProviderColumns+`
		from routing_rules r`+ruleProviderJoin+`
		where r.id = $1
		order by rp.position asc`, id)
	if err != nil {
		return nil, err
	}
	if len(rules) == 0 {
		return nil, fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}
	return rules[0], nil
}

// CreateRoutingRuleParams adalah masukan pembuatan aturan routing.
//
// Field pointer yang dibiarkan nil memakai nilai bawaan kolomnya (DefaultMaxAttempts dan
// kawan-kawan), bukan nol: nol ditolak constraint tabel untuk hampir semua field ini, dan
// priority nol berarti "paling dulu dievaluasi" — arti yang terlalu jauh dari "tidak
// diisi" untuk ditebak.
//
// Kandidat provider tidak diisi di sini melainkan lewat SetProviders, yang butuh
// transaksi. Aturan baru karena itu lahir tanpa kandidat, dan itu bukan keadaan setengah
// jadi: aturan tanpa kandidat sudah bermakna "pakai strategi ini atas semua provider yang
// bisa melayani modelnya".
type CreateRoutingRuleParams struct {
	Name string
	// Description nil atau berisi spasi saja disimpan sebagai NULL: kolomnya bermakna
	// "tidak ada deskripsi", bukan "ada tapi kosong".
	Description *string
	// Priority nil memakai DefaultPriority. Makin kecil makin dulu dievaluasi.
	Priority *int

	// Kondisi pencocokan. nil / kosong berarti tidak membatasi, jadi aturan tanpa satu pun
	// kondisi adalah aturan bawaan yang cocok untuk semua lalu lintas.
	MatchModelID  *string
	MatchAPIKeyID *string
	// MatchCapabilities dibatasi ke CapText dan kawan-kawan oleh constraint tabel.
	MatchCapabilities []string

	// Strategy kosong memakai StrategyPriority, seperti default kolomnya.
	Strategy string

	MaxAttempts *int
	BackoffMS   *int

	FailureThreshold *int
	OpenDurationMS   *int
	HalfOpenProbes   *int

	Enabled   *bool
	CreatedBy *string
}

// Create menyimpan aturan routing baru, tanpa kandidat provider.
func (r *RoutingRepo) Create(ctx context.Context, p CreateRoutingRuleParams) (*RoutingRule, error) {
	const op = "membuat aturan routing"

	for _, ref := range []struct {
		field string
		id    *string
	}{
		{"match_model_id", p.MatchModelID},
		{"match_api_key_id", p.MatchAPIKeyID},
		{"created_by", p.CreatedBy},
	} {
		if err := refID(op, ref.field, ref.id); err != nil {
			return nil, err
		}
	}

	// Deskripsi yang hanya berisi spasi disamakan dengan tidak ada: kolomnya bermakna
	// "tidak ada deskripsi", bukan "ada tapi kosong".
	var description any
	if p.Description != nil {
		description = nullIfEmpty(*p.Description)
	}

	// Tabel diberi alias "r" supaya klausa returning bisa memakai routingRuleColumns yang
	// sama dengan query lain, tanpa menuliskan daftar kolomnya dua kali.
	row := r.q.QueryRow(ctx, `
		insert into routing_rules as r (
			name, description, priority,
			match_model_id, match_api_key_id, match_capabilities,
			strategy, max_attempts, backoff_ms,
			failure_threshold, open_duration_ms, half_open_probes,
			enabled, created_by
		) values ($1, $2, $3, $4, $5, coalesce($6::text[], '{}'::text[]),
			$7, $8, $9, $10, $11, $12, $13, $14)
		returning `+routingRuleColumns,
		p.Name, description, intOr(p.Priority, DefaultPriority),
		p.MatchModelID, p.MatchAPIKeyID, p.MatchCapabilities,
		strOr(p.Strategy, StrategyPriority),
		intOr(p.MaxAttempts, DefaultMaxAttempts), intOr(p.BackoffMS, DefaultBackoffMS),
		intOr(p.FailureThreshold, DefaultFailureThreshold),
		intOr(p.OpenDurationMS, DefaultOpenDurationMS),
		intOr(p.HalfOpenProbes, DefaultHalfOpenProbes),
		boolOr(p.Enabled, true), p.CreatedBy,
	)

	rule, err := scanRule(row)
	if err != nil {
		return nil, repo.Err(op, err)
	}
	return rule, nil
}

// UpdateRoutingRuleParams adalah pembaruan sebagian sebuah aturan.
//
// Field pointer: nil berarti biarkan. Field Opt melayani kolom yang boleh NULL, sehingga
// "biarkan" bisa dibedakan dari "kosongkan" — pakai Set dan Clear. Mengosongkan kondisi
// pencocokan berarti melebarkan aturan sampai cocok untuk semua lalu lintas, jadi kedua
// permintaan itu memang tidak boleh tertukar.
//
// created_by sengaja tidak bisa diubah: itu jejak siapa yang memasang aturan, bukan
// atribut yang bisa dipindahkan.
type UpdateRoutingRuleParams struct {
	Name        *string
	Description Opt[string]
	Priority    *int

	MatchModelID  Opt[string]
	MatchAPIKeyID Opt[string]
	// MatchCapabilities nil berarti biarkan; daftar kosong berarti kosongkan, yaitu
	// berhenti membatasi kemampuan.
	MatchCapabilities []string

	Strategy *string

	MaxAttempts *int
	BackoffMS   *int

	FailureThreshold *int
	OpenDurationMS   *int
	HalfOpenProbes   *int

	Enabled *bool
}

// Update mengubah field yang diminta saja dan mengembalikan aturan hasilnya beserta
// kandidatnya. Kandidat tidak pernah ikut berubah di sini — itu urusan SetProviders.
func (r *RoutingRepo) Update(ctx context.Context, id string, p UpdateRoutingRuleParams) (*RoutingRule, error) {
	const op = "memperbarui aturan routing"
	if !idOK(id) {
		return nil, fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}
	if err := refID(op, "match_model_id", p.MatchModelID.Value); err != nil {
		return nil, err
	}
	if err := refID(op, "match_api_key_id", p.MatchAPIKeyID.Value); err != nil {
		return nil, err
	}

	// Urutan kolom di klausa SET dijaga tetap: teks SQL yang stabil berarti satu entri di
	// cache prepared statement pgx per bentuk pembaruan, bukan satu per permutasi.
	set := newUpdateSet(id)
	if p.Name != nil {
		set.add("name", *p.Name)
	}
	addOpt(set, "description", p.Description)
	if p.Priority != nil {
		set.add("priority", *p.Priority)
	}
	addOpt(set, "match_model_id", p.MatchModelID)
	addOpt(set, "match_api_key_id", p.MatchAPIKeyID)
	if p.MatchCapabilities != nil {
		set.add("match_capabilities", p.MatchCapabilities)
	}
	if p.Strategy != nil {
		set.add("strategy", *p.Strategy)
	}
	for _, f := range []struct {
		column string
		value  *int
	}{
		{"max_attempts", p.MaxAttempts},
		{"backoff_ms", p.BackoffMS},
		{"failure_threshold", p.FailureThreshold},
		{"open_duration_ms", p.OpenDurationMS},
		{"half_open_probes", p.HalfOpenProbes},
	} {
		if f.value != nil {
			set.add(f.column, *f.value)
		}
	}
	if p.Enabled != nil {
		set.add("enabled", *p.Enabled)
	}

	if set.empty() {
		return nil, fmt.Errorf("%s: %w", op, ErrNoChanges)
	}

	// Baris hasil pembaruan dan kandidatnya dibaca dalam SATU pernyataan lewat CTE, bukan
	// dengan Get menyusul: di antara dua pernyataan, kandidat yang terhapus akan terbaca
	// sebagai daftar kosong, dan daftar kosong berarti "berlaku atas semua provider" —
	// bukan "belum terbaca". Pemanggil yang menyimpan hasil Update ke cache akan
	// menyebarkan arti yang salah itu.
	rules, err := r.queryRules(ctx, op, `
		with updated as (
			update routing_rules set `+set.clause()+` where id = $1 returning *
		)
		select `+routingRuleColumns+`, `+ruleProviderColumns+`
		from updated r`+ruleProviderJoin+`
		order by rp.position asc`, set.args...)
	if err != nil {
		return nil, err
	}
	if len(rules) == 0 {
		return nil, fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}
	return rules[0], nil
}

// SetEnabled menyalakan atau mematikan aturan. Aturan yang mati tidak pernah ikut
// dievaluasi, tetapi tetap tersimpan beserta kandidatnya.
func (r *RoutingRepo) SetEnabled(ctx context.Context, id string, enabled bool) (*RoutingRule, error) {
	return r.Update(ctx, id, UpdateRoutingRuleParams{Enabled: &enabled})
}

// Delete menghapus aturan beserta seluruh kandidatnya (on delete cascade).
func (r *RoutingRepo) Delete(ctx context.Context, id string) error {
	const op = "menghapus aturan routing"
	if !idOK(id) {
		return fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}

	tag, err := r.q.Exec(ctx, `delete from routing_rules where id = $1`, id)
	if err != nil {
		return repo.Err(op, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}
	return nil
}

// SetProviders mengganti SELURUH daftar kandidat satu aturan dalam satu transaksi.
//
// position diambil dari urutan providerIDs (1-based), bukan dari pemanggil: position hanya
// bermakna sebagai urutan fallback, dan membiarkan pemanggil menomori sendiri membuka
// satu-satunya cara merusaknya — dua kandidat pada position yang sama, yang membuat urutan
// fallback bergantung pada baris mana yang kebetulan terbaca lebih dulu. Unique index
// routing_rule_providers_order_key memang menolak keadaan itu, tetapi menolaknya sebagai
// error di dashboard jauh lebih buruk daripada tidak pernah bisa terjadi.
//
// weights menimpa providers.weight khusus dalam aturan ini dan hanya bermakna untuk
// strategi weighted; pengenal yang tidak disebut di map memakai bobot providernya.
//
// providerIDs kosong SAH dan berarti mengembalikan aturan ke "berlaku atas semua provider".
// Karena itu jumlah baris yang terhapus maupun tersisip tidak bisa dipakai menyimpulkan
// aturannya ada, dan keberadaan aturan diperiksa terpisah.
//
// WAJIB dijalankan di dalam transaksi, dan itu ditegakkan, bukan sekadar didokumentasikan:
// penggantian ini adalah hapus lalu sisip. Kalau penyisipan gagal setelah penghapusan
// berhasil, aturan tertinggal tanpa kandidat — dan itu bukan kerusakan yang kelihatan,
// melainkan aturan yang diam-diam berlaku atas SEMUA provider.
//
// Pakai SetRuleProviders bila pemanggil hanya punya pool.
func (r *RoutingRepo) SetProviders(ctx context.Context, ruleID string, providerIDs []string, weights map[string]int) error {
	const op = "memasang kandidat provider aturan routing"

	if _, inTx := r.q.(pgx.Tx); !inTx {
		return fmt.Errorf("%s: %w", op, ErrNeedsTx)
	}
	if !idOK(ruleID) {
		return fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}

	// Bobot dirakit sebagai array sejajar providerIDs supaya seluruh daftar masuk lewat
	// satu INSERT. Pengenal ganda ditolak di sini, bukan dibiarkan menabrak primary key:
	// pesan "data sudah ada" tidak memberi tahu operator provider mana yang disebut dua
	// kali, padahal itulah satu-satunya yang perlu ia perbaiki.
	seen := make(map[string]bool, len(providerIDs))
	weightArgs := make([]*int, len(providerIDs))
	for i, id := range providerIDs {
		if !idOK(id) {
			return fmt.Errorf("%s: %w: provider_id ke-%d bukan UUID", op, repo.ErrInvalidReference, i+1)
		}
		if seen[id] {
			return fmt.Errorf("%s: %w: provider %s disebut dua kali, padahal satu provider hanya bisa menempati satu position",
				op, repo.ErrConflict, id)
		}
		seen[id] = true
		if w, ok := weights[id]; ok {
			weightArgs[i] = &w
		}
	}

	// Aturannya dikunci lebih dulu, dengan dua alasan. Pertama, daftar kandidat kosong juga
	// sah, jadi tanpa pemeriksaan ini SetProviders atas aturan yang tidak ada akan
	// dilaporkan berhasil. Kedua, dua operator yang mengubah daftar aturan yang sama
	// berjalan berurutan, bukan saling menyelipkan hapus dan sisip sehingga hasil akhirnya
	// campuran dua daftar.
	var locked string
	if err := r.q.QueryRow(ctx,
		`select id::text from routing_rules where id = $1 for update`, ruleID).Scan(&locked); err != nil {
		return repo.Err(op, err)
	}

	if _, err := r.q.Exec(ctx,
		`delete from routing_rule_providers where rule_id = $1`, ruleID); err != nil {
		return repo.Err(op, err)
	}
	if len(providerIDs) == 0 {
		return nil
	}

	// Satu INSERT untuk seluruh daftar, dengan position diambil dari ordinality — yaitu
	// urutan elemen di dalam array. Dengan begitu penomorannya tidak bisa berbeda dari
	// urutan yang diminta pemanggil, bahkan kalau daftarnya panjang.
	_, err := r.q.Exec(ctx, `
		insert into routing_rule_providers (rule_id, provider_id, position, weight)
		select $1, t.provider_id, t.position, t.weight
		from unnest($2::uuid[], $3::int[]) with ordinality as t(provider_id, weight, position)`,
		ruleID, providerIDs, weightArgs)
	if err != nil {
		return repo.Err(op, err)
	}
	return nil
}

// SetRuleProviders menjalankan RoutingRepo.SetProviders di dalam transaksinya sendiri.
//
// Disediakan mengikuti SetPrice: pemanggil yang hanya memegang pool tidak perlu merakit
// transaksi sendiri — dan tidak ada yang tergoda memanggil SetProviders di luar transaksi.
func SetRuleProviders(ctx context.Context, pool *pgxpool.Pool, ruleID string, providerIDs []string, weights map[string]int) error {
	return repo.InTx(ctx, pool, func(q repo.Querier) error {
		return NewRoutingRepo(q).SetProviders(ctx, ruleID, providerIDs, weights)
	})
}

// RoutingRuleFilter menyaring daftar aturan untuk dashboard.
type RoutingRuleFilter struct {
	// Enabled nil berarti aktif dan tidak aktif keduanya.
	Enabled *bool
	// Strategy kosong berarti semua strategi.
	Strategy string
	// ModelID menjawab "aturan mana yang MENYEBUT model ini", yaitu yang match_model_id-nya
	// sama — bukan "aturan mana yang akan cocok untuk model ini". Aturan tanpa kondisi model
	// sengaja tidak ikut meskipun lalu lintas model itu akan dilayaninya juga: cocok atau
	// tidak bergantung pada kondisi lain yang baru diketahui saat request, jadi pertanyaan
	// kedua itu dijawab mesin routing atas hasil ActiveRules, bukan oleh penyaring daftar.
	// Bentuk ini pula yang dilayani indeks routing_rules_model_idx.
	ModelID string
	// Search mencocokkan nama dan deskripsi.
	Search string
}

// List mengembalikan aturan berpaginasi keyset, terbaru dulu, beserta kandidatnya dan
// kursor halaman berikutnya ("" bila sudah habis).
//
// Urutan di sini adalah urutan tampilan dashboard, BUKAN urutan evaluasi: urutan evaluasi
// dilayani ActiveRules, dan mencampur keduanya akan membuat orang membaca daftar ini
// sebagai "aturan mana yang menang lebih dulu".
//
// LIMIT dikenakan di dalam CTE, bukan di query luar. Kalau dikenakan di luar, batasnya
// memotong baris HASIL JOIN: satu aturan berkandidat lima menghabiskan lima baris,
// sehingga jumlah aturan per halaman menjadi tak tentu dan daftar kandidat aturan terakhir
// terpotong di tengah — persis bentuk kerusakan yang paling sulit dikenali, karena daftar
// kandidat yang terpotong tetap terlihat seperti daftar yang sah.
func (r *RoutingRepo) List(ctx context.Context, f RoutingRuleFilter, page repo.Page) ([]*RoutingRule, string, error) {
	const op = "mendaftar aturan routing"
	limit := page.Normalize()

	var (
		conds []string
		args  []any
	)
	add := func(cond string, arg any) {
		args = append(args, arg)
		conds = append(conds, fmt.Sprintf(cond, len(args)))
	}

	if f.Enabled != nil {
		add("r.enabled = $%d", *f.Enabled)
	}
	if f.Strategy != "" {
		add("r.strategy = $%d", f.Strategy)
	}
	if f.ModelID != "" {
		if !idOK(f.ModelID) {
			return nil, "", fmt.Errorf("%s: %w: model_id bukan UUID", op, repo.ErrInvalidReference)
		}
		add("r.match_model_id = $%d", f.ModelID)
	}
	if f.Search != "" {
		// Satu argumen dipakai dua kali, jadi kondisinya dirakit langsung.
		args = append(args, f.Search)
		n := len(args)
		conds = append(conds, fmt.Sprintf(
			"(r.name ilike '%%' || $%d || '%%' or r.description ilike '%%' || $%d || '%%')", n, n))
	}
	if page.Cursor != "" {
		ts, id, err := decodeCursor(op, page.Cursor)
		if err != nil {
			return nil, "", err
		}
		if !idOK(id) {
			return nil, "", fmt.Errorf("%s: %w: bagian pengenal pada kursor bukan UUID", op, repo.ErrConstraint)
		}
		args = append(args, ts, id)
		conds = append(conds, fmt.Sprintf("(r.created_at, r.id) < ($%d::timestamptz, $%d::uuid)", len(args)-1, len(args)))
	}

	where := ""
	if len(conds) > 0 {
		where = " where " + strings.Join(conds, " and ")
	}
	args = append(args, limit+1) // satu aturan lebih untuk mengetahui ada halaman lagi

	// CTE dialiaskan sebagai r di query luar supaya routingRuleColumns yang sama bisa
	// dipakai apa adanya, tanpa menuliskan daftar kolomnya untuk kedua tingkat query.
	rules, err := r.queryRules(ctx, op, `
		with page as (
			select r.* from routing_rules r`+where+
		fmt.Sprintf(` order by r.created_at desc, r.id desc limit $%d`, len(args))+`
		)
		select `+routingRuleColumns+`, `+ruleProviderColumns+`
		from page r`+ruleProviderJoin+`
		order by r.created_at desc, r.id desc, rp.position asc`, args...)
	if err != nil {
		return nil, "", err
	}

	next := ""
	if len(rules) > limit {
		rules = rules[:limit]
		last := rules[limit-1]
		next = encodeCursor(last.CreatedAt, last.ID)
	}
	return rules, next, nil
}
