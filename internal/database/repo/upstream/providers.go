package upstream

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/NexGen-X/Route-X/internal/database/repo"
)

// Provider adalah satu baris tabel providers.
//
// Field bertipe pointer mewakili kolom yang boleh NULL, sehingga "tanpa batas" bisa
// dibedakan dari "batasnya nol". Metadata dibiarkan sebagai JSON mentah: lapisan data
// tidak punya alasan menafsirkan isinya, dan menerjemahkannya ke struct di sini akan
// memaksa migrasi kode setiap kali ada penyedia baru yang butuh satu field tambahan.
type Provider struct {
	ID          string
	Name        string
	DisplayName string
	Kind        string
	BaseURL     string

	Enabled  bool
	Priority int
	Weight   int

	TimeoutMS  int
	MaxRetries int

	RateLimitRPM  *int
	RateLimitTPM  *int
	MaxConcurrent *int

	EgressPoolID *string

	IsBYOK      bool
	OwnerUserID *string

	LastHealthStatus    *string
	LastHealthAt        *time.Time
	LastLatencyMS       *int
	ConsecutiveFailures int

	Metadata  []byte
	CreatedAt time.Time
	UpdatedAt time.Time
	CreatedBy *string
}

// ProviderRepo adalah akses data provider beserta kesehatannya.
type ProviderRepo struct {
	q repo.Querier
}

// NewProviderRepo membuat ProviderRepo di atas pool maupun transaksi.
func NewProviderRepo(q repo.Querier) *ProviderRepo { return &ProviderRepo{q: q} }

// WithQuerier membuat salinan ProviderRepo yang berjalan di atas Querier baru (misal transaksi pgx.Tx).
func (r *ProviderRepo) WithQuerier(q repo.Querier) *ProviderRepo {
	return &ProviderRepo{q: q}
}

// providerColumns disatukan supaya setiap query menghasilkan bentuk baris yang sama.
// Kolom uuid dibaca sebagai teks karena pengenal di API paket ini bertipe string.
const providerColumns = `
	id::text, name, display_name, kind, base_url,
	enabled, priority, weight, timeout_ms, max_retries,
	rate_limit_rpm, rate_limit_tpm, max_concurrent, egress_pool_id::text,
	is_byok, owner_user_id::text,
	last_health_status, last_health_at, last_latency_ms, consecutive_failures,
	metadata, created_at, updated_at, created_by::text`

// scanProvider membaca satu baris sesuai providerColumns.
func scanProvider(row pgxRow) (*Provider, error) {
	var p Provider
	err := row.Scan(
		&p.ID, &p.Name, &p.DisplayName, &p.Kind, &p.BaseURL,
		&p.Enabled, &p.Priority, &p.Weight, &p.TimeoutMS, &p.MaxRetries,
		&p.RateLimitRPM, &p.RateLimitTPM, &p.MaxConcurrent, &p.EgressPoolID,
		&p.IsBYOK, &p.OwnerUserID,
		&p.LastHealthStatus, &p.LastHealthAt, &p.LastLatencyMS, &p.ConsecutiveFailures,
		&p.Metadata, &p.CreatedAt, &p.UpdatedAt, &p.CreatedBy,
	)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// CreateProviderParams adalah masukan pembuatan provider.
//
// Field pointer yang dibiarkan nil memakai nilai bawaan (DefaultPriority dan
// kawan-kawan), bukan nol: weight nol dan timeout nol ditolak constraint tabel, dan
// priority nol berarti "paling diutamakan" — arti yang terlalu jauh dari "tidak diisi"
// untuk ditebak.
type CreateProviderParams struct {
	Name        string
	DisplayName string
	Kind        string
	BaseURL     string

	Enabled    *bool
	Priority   *int
	Weight     *int
	TimeoutMS  *int
	MaxRetries *int

	RateLimitRPM  *int
	RateLimitTPM  *int
	MaxConcurrent *int

	EgressPoolID *string

	// IsBYOK dan OwnerUserID wajib sejalan: BYOK harus punya pemilik dan provider
	// bersama tidak boleh punya. Aturannya ditegakkan constraint
	// providers_byok_has_owner, sehingga tidak ada jalur — termasuk SQL langsung —
	// yang bisa membuat kredensial satu pengguna terpakai lalu lintas pengguna lain.
	IsBYOK      bool
	OwnerUserID *string

	// Metadata adalah JSON mentah; nil menjadi '{}'.
	Metadata  []byte
	CreatedBy *string
}

// Create menyimpan provider baru.
func (r *ProviderRepo) Create(ctx context.Context, p CreateProviderParams) (*Provider, error) {
	const op = "membuat provider"

	for _, ref := range []struct {
		field string
		id    *string
	}{
		{"egress_pool_id", p.EgressPoolID},
		{"owner_user_id", p.OwnerUserID},
		{"created_by", p.CreatedBy},
	} {
		if err := refID(op, ref.field, ref.id); err != nil {
			return nil, err
		}
	}

	row := r.q.QueryRow(ctx, `
		insert into providers (
			name, display_name, kind, base_url,
			enabled, priority, weight, timeout_ms, max_retries,
			rate_limit_rpm, rate_limit_tpm, max_concurrent, egress_pool_id,
			is_byok, owner_user_id, metadata, created_by
		) values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15,
			coalesce($16::jsonb, '{}'::jsonb), $17)
		returning `+providerColumns,
		p.Name, p.DisplayName, p.Kind, p.BaseURL,
		boolOr(p.Enabled, true), intOr(p.Priority, DefaultPriority), intOr(p.Weight, DefaultWeight),
		intOr(p.TimeoutMS, DefaultTimeoutMS), intOr(p.MaxRetries, DefaultMaxRetries),
		p.RateLimitRPM, p.RateLimitTPM, p.MaxConcurrent, p.EgressPoolID,
		p.IsBYOK, p.OwnerUserID, p.Metadata, p.CreatedBy,
	)

	provider, err := scanProvider(row)
	if err != nil {
		return nil, repo.Err(op, err)
	}
	return provider, nil
}

// Get mengambil provider menurut pengenalnya.
func (r *ProviderRepo) Get(ctx context.Context, id string) (*Provider, error) {
	const op = "mengambil provider"
	if !idOK(id) {
		return nil, fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}

	provider, err := scanProvider(r.q.QueryRow(ctx,
		`select `+providerColumns+` from providers where id = $1`, id))
	if err != nil {
		return nil, repo.Err(op, err)
	}
	return provider, nil
}

// GetByName mengambil provider menurut nama pendeknya. Nama itulah yang dipakai di
// konfigurasi, log, dan label metrik, jadi jalur ini dibutuhkan sama seringnya.
func (r *ProviderRepo) GetByName(ctx context.Context, name string) (*Provider, error) {
	const op = "mengambil provider menurut nama"

	provider, err := scanProvider(r.q.QueryRow(ctx,
		`select `+providerColumns+` from providers where name = $1`, name))
	if err != nil {
		return nil, repo.Err(op, err)
	}
	return provider, nil
}

// UpdateProviderParams adalah pembaruan sebagian sebuah provider.
//
// Field bertipe pointer: nil berarti biarkan. Field bertipe Opt bisa juga dikosongkan
// karena kolomnya boleh NULL — pakai Set dan Clear.
//
// is_byok dan owner_user_id sengaja tidak bisa diubah. Memindahkan kepemilikan provider
// BYOK berarti memindahkan kredensial pengguna lain ke tangan orang lain; kalau itu
// benar-benar diperlukan, provider baru dibuat dan yang lama dihapus, sehingga
// kredensialnya ikut terhapus dan harus dimasukkan ulang oleh pemilik barunya.
type UpdateProviderParams struct {
	Name        *string
	DisplayName *string
	Kind        *string
	BaseURL     *string

	Enabled    *bool
	Priority   *int
	Weight     *int
	TimeoutMS  *int
	MaxRetries *int

	RateLimitRPM  Opt[int]
	RateLimitTPM  Opt[int]
	MaxConcurrent Opt[int]
	EgressPoolID  Opt[string]

	Metadata []byte
}

// Update mengubah field yang diminta saja dan mengembalikan baris hasilnya.
func (r *ProviderRepo) Update(ctx context.Context, id string, p UpdateProviderParams) (*Provider, error) {
	const op = "memperbarui provider"
	if !idOK(id) {
		return nil, fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}
	if err := refID(op, "egress_pool_id", p.EgressPoolID.Value); err != nil {
		return nil, err
	}

	// Urutan kolom di klausa SET dijaga tetap: teks SQL yang stabil berarti satu entri
	// di cache prepared statement pgx per bentuk pembaruan, bukan satu per permutasi.
	set := newUpdateSet(id)
	for _, f := range []struct {
		column string
		value  *string
	}{
		{"name", p.Name},
		{"display_name", p.DisplayName},
		{"kind", p.Kind},
		{"base_url", p.BaseURL},
	} {
		if f.value != nil {
			set.add(f.column, *f.value)
		}
	}
	if p.Enabled != nil {
		set.add("enabled", *p.Enabled)
	}
	for _, f := range []struct {
		column string
		value  *int
	}{
		{"priority", p.Priority},
		{"weight", p.Weight},
		{"timeout_ms", p.TimeoutMS},
		{"max_retries", p.MaxRetries},
	} {
		if f.value != nil {
			set.add(f.column, *f.value)
		}
	}
	addOpt(set, "rate_limit_rpm", p.RateLimitRPM)
	addOpt(set, "rate_limit_tpm", p.RateLimitTPM)
	addOpt(set, "max_concurrent", p.MaxConcurrent)
	addOpt(set, "egress_pool_id", p.EgressPoolID)
	if p.Metadata != nil {
		set.add("metadata", p.Metadata)
	}

	if set.empty() {
		return nil, fmt.Errorf("%s: %w", op, ErrNoChanges)
	}

	provider, err := scanProvider(r.q.QueryRow(ctx,
		`update providers set `+set.clause()+` where id = $1 returning `+providerColumns, set.args...))
	if err != nil {
		return nil, repo.Err(op, err)
	}
	return provider, nil
}

// SetEnabled menyalakan atau mematikan provider. Provider yang mati tidak pernah
// menjadi kandidat rute.
func (r *ProviderRepo) SetEnabled(ctx context.Context, id string, enabled bool) (*Provider, error) {
	return r.Update(ctx, id, UpdateProviderParams{Enabled: &enabled})
}

// Delete menghapus provider beserta kredensial, pemetaan model, dan riwayat health
// check-nya (lewat on delete cascade di skema).
func (r *ProviderRepo) Delete(ctx context.Context, id string) error {
	const op = "menghapus provider"
	if !idOK(id) {
		return fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}

	tag, err := r.q.Exec(ctx, `delete from providers where id = $1`, id)
	if err != nil {
		return repo.Err(op, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}
	return nil
}

// ProviderFilter menyaring daftar provider.
type ProviderFilter struct {
	// Kind kosong berarti semua jenis.
	Kind string
	// Enabled nil berarti aktif dan tidak aktif keduanya.
	Enabled *bool
	// IsBYOK nil berarti provider bersama dan BYOK keduanya.
	IsBYOK *bool
	// OwnerUserID hanya bermakna untuk provider BYOK.
	OwnerUserID string
	// AccessibleByUserID membatasi hasil ke provider bersama (non-BYOK) atau BYOK milik pengguna ini.
	AccessibleByUserID string
	// Search mencocokkan nama dan nama tampilan.
	Search string
}

// List mengembalikan provider berpaginasi keyset, terbaru dulu, beserta kursor halaman
// berikutnya ("" bila sudah habis).
//
// Urutan di sini adalah urutan tampilan dashboard, bukan urutan routing: pemilihan rute
// punya urutannya sendiri dan dilayani RouteCandidates.
func (r *ProviderRepo) List(ctx context.Context, f ProviderFilter, page repo.Page) ([]*Provider, string, error) {
	const op = "mendaftar provider"
	limit := page.Normalize()

	var (
		conds []string
		args  []any
	)
	add := func(cond string, arg any) {
		args = append(args, arg)
		conds = append(conds, fmt.Sprintf(cond, len(args)))
	}

	if f.Kind != "" {
		add("kind = $%d", f.Kind)
	}
	if f.Enabled != nil {
		add("enabled = $%d", *f.Enabled)
	}
	if f.IsBYOK != nil {
		add("is_byok = $%d", *f.IsBYOK)
	}
	if f.OwnerUserID != "" {
		if !idOK(f.OwnerUserID) {
			return nil, "", fmt.Errorf("%s: %w: owner_user_id bukan UUID", op, repo.ErrInvalidReference)
		}
		add("owner_user_id = $%d", f.OwnerUserID)
	}
	if f.AccessibleByUserID != "" {
		if !idOK(f.AccessibleByUserID) {
			return nil, "", fmt.Errorf("%s: %w: accessible_by_user_id bukan UUID", op, repo.ErrInvalidReference)
		}
		add("(not is_byok or owner_user_id = $%d::uuid)", f.AccessibleByUserID)
	}
	if f.Search != "" {
		// Satu argumen dipakai dua kali, jadi kondisinya dirakit langsung.
		args = append(args, f.Search)
		n := len(args)
		conds = append(conds, fmt.Sprintf(
			"(name ilike '%%' || $%d || '%%' or display_name ilike '%%' || $%d || '%%')", n, n))
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
		conds = append(conds, fmt.Sprintf("(created_at, id) < ($%d::timestamptz, $%d::uuid)", len(args)-1, len(args)))
	}

	where := ""
	if len(conds) > 0 {
		where = " where " + strings.Join(conds, " and ")
	}
	args = append(args, limit+1) // satu baris lebih untuk mengetahui ada halaman lagi

	rows, err := r.q.Query(ctx, `select `+providerColumns+` from providers`+where+
		fmt.Sprintf(` order by created_at desc, id desc limit $%d`, len(args)), args...)
	if err != nil {
		return nil, "", repo.Err(op, err)
	}
	defer rows.Close()

	out := make([]*Provider, 0, limit)
	for rows.Next() {
		provider, err := scanProvider(rows)
		if err != nil {
			return nil, "", repo.Err(op, err)
		}
		out = append(out, provider)
	}
	if err := rows.Err(); err != nil {
		return nil, "", repo.Err(op, err)
	}

	next := ""
	if len(out) > limit {
		out = out[:limit]
		last := out[limit-1]
		next = encodeCursor(last.CreatedAt, last.ID)
	}
	return out, next, nil
}

// ActiveProviders mengembalikan seluruh provider aktif untuk health checker dan routing.
func (r *ProviderRepo) ActiveProviders(ctx context.Context) ([]*Provider, error) {
	const op = "mengambil provider aktif"
	rows, err := r.q.Query(ctx, `select `+providerColumns+` from providers where enabled = true order by name`)
	if err != nil {
		return nil, repo.Err(op, err)
	}
	defer rows.Close()

	var out []*Provider
	for rows.Next() {
		provider, err := scanProvider(rows)
		if err != nil {
			return nil, repo.Err(op, err)
		}
		out = append(out, provider)
	}
	if err := rows.Err(); err != nil {
		return nil, repo.Err(op, err)
	}
	return out, nil
}

// RouteCandidate adalah satu provider yang siap melayani sebuah model, berikut semua
// yang dibutuhkan mesin routing untuk memanggilnya tanpa query tambahan.
//
// Priority dan Weight sudah efektif: nilai dari provider_models dipakai bila ada, kalau
// tidak nilai dari provider. Dengan begitu pemanggil tidak perlu tahu penimpaan itu ada.
type RouteCandidate struct {
	// ProviderModelID adalah baris pemetaan; inilah kunci yang dipakai tabel harga.
	ProviderModelID   string
	UpstreamModelName string
	MaxContextWindow  *int
	SupportsStreaming bool
	SupportsTools     bool

	Priority int
	Weight   int

	ProviderID    string
	ProviderName  string
	Kind          string
	BaseURL       string
	TimeoutMS     int
	MaxRetries    int
	RateLimitRPM  *int
	RateLimitTPM  *int
	MaxConcurrent *int
	EgressPoolID  *string

	IsBYOK      bool
	OwnerUserID *string

	LastHealthStatus    *string
	LastLatencyMS       *int
	ConsecutiveFailures int
}

// RouteQuery adalah parameter pencarian kandidat rute.
type RouteQuery struct {
	// ModelID adalah models.id (UUID), bukan nama yang diminta klien. Nama diselesaikan
	// lebih dulu oleh ModelRepo.Resolve, yang hasilnya bisa di-cache.
	ModelID string

	// OwnerUserID kosong berarti hanya provider bersama. Bila diisi, provider BYOK milik
	// pengguna itu ikut menjadi kandidat — BYOK milik orang lain tidak pernah ikut.
	OwnerUserID string

	// IncludeUnhealthy menyertakan provider yang health check terakhirnya gagal. Dipakai
	// dashboard dan worker health check; jalur request membiarkannya false.
	IncludeUnhealthy bool
}

// routeCandidateColumns dipisah supaya bentuk barisnya bisa dibaca sekali pandang.
const routeCandidateColumns = `
	pm.id::text, pm.upstream_model_name, pm.max_context_window,
	pm.supports_streaming, pm.supports_tools,
	coalesce(pm.priority, p.priority) as effective_priority,
	coalesce(pm.weight, p.weight)     as effective_weight,
	p.id::text, p.name, p.kind, p.base_url, p.timeout_ms, p.max_retries,
	p.rate_limit_rpm, p.rate_limit_tpm, p.max_concurrent, p.egress_pool_id::text,
	p.is_byok, p.owner_user_id::text,
	p.last_health_status, p.last_latency_ms, p.consecutive_failures`

// routeCandidatesSQL adalah query kandidat rute. Dipisah sebagai konstanta supaya test
// bisa menjalankan EXPLAIN pada teks yang persis sama dengan yang dipakai produksi —
// bukti pemakaian indeks jadi tidak bisa basi karena querynya diubah tanpa test ikut
// diubah.
const routeCandidatesSQL = `
	select ` + routeCandidateColumns + `
	from provider_models pm
	join providers p on p.id = pm.provider_id
	where pm.model_id = $1
	  and pm.enabled
	  and p.enabled
	  -- Provider yang belum pernah diperiksa (NULL) tetap ikut: kalau tidak, tidak ada
	  -- satu pun rute yang bisa dipakai sampai worker health check selesai berjalan
	  -- pertama kali setelah deploy.
	  and ($2::boolean or p.last_health_status is null or p.last_health_status <> 'unhealthy')
	  and (case
		when $3::uuid is null then not p.is_byok
		else not p.is_byok or p.owner_user_id = $3::uuid
	  end)
	order by effective_priority asc, effective_weight desc, p.name asc`

// RouteCandidates mengembalikan provider aktif dan sehat yang bisa melayani satu model,
// terurut dari yang paling diutamakan.
//
// Inilah query terpanas di sistem: dijalankan sekali per request inference. Karena itu:
//
//   - Satu query, bukan satu query per provider. Semua yang dibutuhkan pemanggil sudah
//     ikut terbaca, jadi tidak ada N+1 di jalur request.
//   - Penyaringannya (model_id, pm.enabled) persis bentuk yang dilayani indeks partial
//     provider_models_routing_idx, sehingga hanya baris pemetaan model ini yang dibaca,
//     bukan seluruh tabel.
//   - Pengurutannya memakai prioritas efektif, yaitu coalesce dua tabel, jadi tidak bisa
//     diambil langsung dari urutan indeks dan diselesaikan dengan Sort. Itu disengaja:
//     jumlah kandidat per model hanya beberapa baris, sementara mengurutkan hanya
//     berdasarkan kolom penimpaan akan menaruh provider di urutan yang salah begitu
//     provider_models.priority dibiarkan NULL.
//
// Tabel models tidak ikut di-join: pemanggil sudah memegang hasil Resolve, termasuk
// apakah modelnya aktif, jadi join itu hanya menambah biaya pada setiap request.
func (r *ProviderRepo) RouteCandidates(ctx context.Context, q RouteQuery) ([]*RouteCandidate, error) {
	const op = "mengambil kandidat rute"
	if !idOK(q.ModelID) {
		return nil, fmt.Errorf("%s: %w: model_id bukan UUID", op, repo.ErrInvalidReference)
	}
	var owner any
	if q.OwnerUserID != "" {
		if !idOK(q.OwnerUserID) {
			return nil, fmt.Errorf("%s: %w: owner_user_id bukan UUID", op, repo.ErrInvalidReference)
		}
		owner = q.OwnerUserID
	}

	rows, err := r.q.Query(ctx, routeCandidatesSQL, q.ModelID, q.IncludeUnhealthy, owner)
	if err != nil {
		return nil, repo.Err(op, err)
	}
	defer rows.Close()

	var out []*RouteCandidate
	for rows.Next() {
		var c RouteCandidate
		err := rows.Scan(
			&c.ProviderModelID, &c.UpstreamModelName, &c.MaxContextWindow,
			&c.SupportsStreaming, &c.SupportsTools,
			&c.Priority, &c.Weight,
			&c.ProviderID, &c.ProviderName, &c.Kind, &c.BaseURL, &c.TimeoutMS, &c.MaxRetries,
			&c.RateLimitRPM, &c.RateLimitTPM, &c.MaxConcurrent, &c.EgressPoolID,
			&c.IsBYOK, &c.OwnerUserID,
			&c.LastHealthStatus, &c.LastLatencyMS, &c.ConsecutiveFailures,
		)
		if err != nil {
			return nil, repo.Err(op, err)
		}
		out = append(out, &c)
	}
	if err := rows.Err(); err != nil {
		return nil, repo.Err(op, err)
	}
	return out, nil
}

// HealthReport adalah hasil satu health check.
//
// ErrorMessage wajib SUDAH disaring oleh pemanggil. Pesan mentah dari klien HTTP bisa
// memuat URL berkredensial, dan tabel health_checks dibaca dashboard.
type HealthReport struct {
	Status       string
	LatencyMS    *int
	StatusCode   *int
	ErrorKind    string
	ErrorMessage string
}

// RecordHealth memperbarui cuplikan kesehatan di providers dan mencatat riwayatnya ke
// health_checks dalam satu pernyataan.
//
// Keduanya harus terjadi bersama: cuplikan tanpa riwayat membuat grafik dashboard
// berlubang, dan riwayat tanpa cuplikan membuat pemilihan rute memakai keadaan basi.
// Satu pernyataan berisi CTE menjamin itu tanpa perlu transaksi tersendiri, sehingga
// worker health check tetap bisa memakai pool langsung.
//
// consecutive_failures direset oleh 'healthy' dan dinaikkan oleh 'unhealthy'. Status
// 'degraded' tidak mengubahnya: provider yang lambat masih melayani, jadi tidak layak
// menaikkan penghitung yang memicu pemutus arus — tetapi juga tidak layak menghapus
// jejak kegagalan sebelumnya.
func (r *ProviderRepo) RecordHealth(ctx context.Context, providerID string, h HealthReport) error {
	const op = "mencatat health check provider"
	if !idOK(providerID) {
		return fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}

	tag, err := r.q.Exec(ctx, `
		with snapshot as (
			update providers set
				last_health_status = $2,
				last_health_at     = now(),
				last_latency_ms    = $3,
				consecutive_failures = case
					when $2 = 'healthy'   then 0
					when $2 = 'unhealthy' then consecutive_failures + 1
					else consecutive_failures
				end
			where id = $1
			returning id
		)
		insert into health_checks (provider_id, status, latency_ms, status_code, error_kind, error_message)
		select id, $2, $3, $4, $5, $6 from snapshot`,
		providerID, h.Status, h.LatencyMS, h.StatusCode,
		nullIfEmpty(h.ErrorKind), nullIfEmpty(h.ErrorMessage),
	)
	if err != nil {
		return repo.Err(op, err)
	}
	// Nol baris berarti CTE tidak menemukan providernya: tidak ada riwayat yang ditulis,
	// jadi tidak ada baris yatim yang tertinggal.
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}
	return nil
}

// HealthCheck adalah satu baris riwayat health_checks.
type HealthCheck struct {
	ID           int64
	ProviderID   string
	CheckedAt    time.Time
	Status       string
	LatencyMS    *int
	StatusCode   *int
	ErrorKind    *string
	ErrorMessage *string
}

// ListHealthChecks mengembalikan riwayat health check satu provider, terbaru dulu,
// berpaginasi keyset. Kursornya memuat waktu dan pengenal baris karena satu worker bisa
// menulis beberapa pemeriksaan dengan checked_at yang sama.
func (r *ProviderRepo) ListHealthChecks(ctx context.Context, providerID string, page repo.Page) ([]*HealthCheck, string, error) {
	const op = "mendaftar riwayat health check"
	if !idOK(providerID) {
		return nil, "", fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}
	limit := page.Normalize()

	args := []any{providerID}
	cond := ""
	if page.Cursor != "" {
		ts, id, err := decodeCursor(op, page.Cursor)
		if err != nil {
			return nil, "", err
		}
		args = append(args, ts, id)
		cond = fmt.Sprintf(" and (checked_at, id) < ($%d::timestamptz, $%d::bigint)", len(args)-1, len(args))
	}
	args = append(args, limit+1)

	rows, err := r.q.Query(ctx, `
		select id, provider_id::text, checked_at, status, latency_ms, status_code,
			error_kind, error_message
		from health_checks
		where provider_id = $1`+cond+
		fmt.Sprintf(` order by checked_at desc, id desc limit $%d`, len(args)), args...)
	if err != nil {
		return nil, "", repo.Err(op, err)
	}
	defer rows.Close()

	out := make([]*HealthCheck, 0, limit)
	for rows.Next() {
		var c HealthCheck
		if err := rows.Scan(&c.ID, &c.ProviderID, &c.CheckedAt, &c.Status, &c.LatencyMS,
			&c.StatusCode, &c.ErrorKind, &c.ErrorMessage); err != nil {
			return nil, "", repo.Err(op, err)
		}
		out = append(out, &c)
	}
	if err := rows.Err(); err != nil {
		return nil, "", repo.Err(op, err)
	}

	next := ""
	if len(out) > limit {
		out = out[:limit]
		last := out[limit-1]
		next = encodeCursor(last.CheckedAt, strconv.FormatInt(last.ID, 10))
	}
	return out, next, nil
}
