package upstream

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/NexGen-X/Route-X/internal/database/repo"
)

// pgUniqueViolation adalah kode SQLSTATE untuk pelanggaran keunikan. Dipakai untuk
// mengenali error dari trigger tabrakan alias, yang memakainya dengan sengaja.
const pgUniqueViolation = "23505"

// Model adalah satu baris tabel models: model kanonik yang boleh diminta klien.
type Model struct {
	// ID adalah kunci internal; ModelID adalah nama yang dikirim klien, mis. "gpt-5".
	ID      string
	ModelID string

	DisplayName string
	Family      *string

	ContextWindow   *int
	MaxOutputTokens *int

	Capabilities []string

	Enabled         bool
	RoutingPriority int
	RoutingStrategy *string

	DeprecatedAt *time.Time
	Metadata     []byte
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// IsDeprecated melaporkan apakah model sudah ditandai usang.
func (m *Model) IsDeprecated() bool { return m.DeprecatedAt != nil }

// HasCapability melaporkan apakah model punya kemampuan tertentu.
func (m *Model) HasCapability(capability string) bool {
	for _, c := range m.Capabilities {
		if c == capability {
			return true
		}
	}
	return false
}

// ModelRepo adalah akses data registry model: model kanonik, aliasnya, dan pemetaannya
// ke provider.
type ModelRepo struct {
	q repo.Querier
}

// NewModelRepo membuat ModelRepo di atas pool maupun transaksi.
func NewModelRepo(q repo.Querier) *ModelRepo { return &ModelRepo{q: q} }

// modelColumns memakai awalan tabel "m" supaya bentuk baris yang sama bisa dipakai pada
// query gabungan di Resolve.
const modelColumns = `
	m.id::text, m.model_id, m.display_name, m.family,
	m.context_window, m.max_output_tokens, m.capabilities,
	m.enabled, m.routing_priority, m.routing_strategy,
	m.deprecated_at, m.metadata, m.created_at, m.updated_at`

// scanModel membaca satu baris sesuai modelColumns.
func scanModel(row pgxRow) (*Model, error) {
	var m Model
	err := row.Scan(
		&m.ID, &m.ModelID, &m.DisplayName, &m.Family,
		&m.ContextWindow, &m.MaxOutputTokens, &m.Capabilities,
		&m.Enabled, &m.RoutingPriority, &m.RoutingStrategy,
		&m.DeprecatedAt, &m.Metadata, &m.CreatedAt, &m.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// aliasCollision melaporkan apakah error berasal dari trigger tabrakan alias↔model_id.
//
// Trigger itu memakai "raise exception ... using errcode = 'unique_violation'", jadi
// errornya sampai ke sini sebagai pelanggaran keunikan tanpa nama constraint — berbeda
// dari pelanggaran indeks sungguhan yang selalu menyebut namanya. Perbedaan itulah yang
// dipakai untuk mengganti pesan bawaan dengan penjelasan yang bisa dipahami pengguna.
func aliasCollision(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation && pgErr.ConstraintName == ""
}

// CreateModelParams adalah masukan pembuatan model.
type CreateModelParams struct {
	// ModelID adalah nama yang diminta klien. Tidak boleh sama dengan alias mana pun.
	ModelID     string
	DisplayName string
	Family      *string

	ContextWindow   *int
	MaxOutputTokens *int

	// Capabilities dibatasi ke CapText dan kawan-kawan oleh constraint tabel.
	Capabilities []string

	Enabled         *bool
	RoutingPriority *int
	RoutingStrategy *string

	DeprecatedAt *time.Time
	Metadata     []byte
}

// Create menyimpan model kanonik baru.
func (r *ModelRepo) Create(ctx context.Context, p CreateModelParams) (*Model, error) {
	const op = "membuat model"

	// Tabel diberi alias "m" supaya klausa returning bisa memakai modelColumns yang sama
	// dengan query lain, tanpa menuliskan daftar kolomnya dua kali.
	row := r.q.QueryRow(ctx, `
		insert into models as m (
			model_id, display_name, family, context_window, max_output_tokens,
			capabilities, enabled, routing_priority, routing_strategy,
			deprecated_at, metadata
		) values ($1, $2, $3, $4, $5, coalesce($6::text[], '{}'::text[]), $7, $8, $9, $10,
			coalesce($11::jsonb, '{}'::jsonb))
		returning `+modelColumns,
		p.ModelID, p.DisplayName, p.Family, p.ContextWindow, p.MaxOutputTokens,
		p.Capabilities, boolOr(p.Enabled, true), intOr(p.RoutingPriority, DefaultPriority),
		p.RoutingStrategy, p.DeprecatedAt, p.Metadata,
	)

	model, err := scanModel(row)
	if err != nil {
		if aliasCollision(err) {
			return nil, fmt.Errorf("%s: %w: model_id %q sudah dipakai sebagai alias model lain",
				op, repo.ErrConflict, p.ModelID)
		}
		return nil, repo.Err(op, err)
	}
	return model, nil
}

// Get mengambil model menurut pengenal internalnya.
func (r *ModelRepo) Get(ctx context.Context, id string) (*Model, error) {
	const op = "mengambil model"
	if !idOK(id) {
		return nil, fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}

	model, err := scanModel(r.q.QueryRow(ctx,
		`select `+modelColumns+` from models m where m.id = $1`, id))
	if err != nil {
		return nil, repo.Err(op, err)
	}
	return model, nil
}

// GetByModelID mengambil model menurut nama kanoniknya, tanpa melihat alias.
func (r *ModelRepo) GetByModelID(ctx context.Context, modelID string) (*Model, error) {
	const op = "mengambil model menurut model_id"

	model, err := scanModel(r.q.QueryRow(ctx,
		`select `+modelColumns+` from models m where m.model_id = $1`, modelID))
	if err != nil {
		return nil, repo.Err(op, err)
	}
	return model, nil
}

// Resolve menerjemahkan nama yang diminta klien menjadi model kanonik.
//
// Satu query untuk dua kemungkinan: nama itu bisa berupa model_id kanonik atau alias.
// Keduanya dicari sebagai dua cabang UNION ALL yang masing-masing menandai dirinya
// (0 untuk kanonik, 1 untuk alias), lalu diurutkan menurut tanda itu dan diambil satu.
// Dengan begitu model kanonik selalu menang tanpa bergantung pada urutan eksekusi cabang
// — kalau hanya memakai UNION ALL + LIMIT 1, "cabang pertama dulu" adalah kebetulan
// rencana eksekusi, bukan janji SQL.
//
// Dua perjalanan ke database dihindari karena inilah langkah pertama setiap request
// inference. Trigger di skema sudah menjamin alias tidak mungkin bertabrakan dengan
// model_id, jadi paling banyak satu cabang yang berisi.
//
// Model yang mati atau usang tetap dikembalikan: pemanggil berhak membalas "model ini
// dimatikan" alih-alih "model tidak dikenal", dan keduanya adalah pesan yang berbeda
// bagi pengguna.
func (r *ModelRepo) Resolve(ctx context.Context, requested string) (*Model, error) {
	const op = "menyelesaikan nama model"

	// Kolom penanda cabang harus ikut di daftar select supaya bisa dipakai ORDER BY pada
	// query UNION, lalu dibuang lagi oleh select luar agar bentuk barisnya tetap sama
	// dengan query model lainnya.
	model, err := scanModel(r.q.QueryRow(ctx, `
		select `+modelColumns+` from (
			select `+modelColumns+`, 0 as source
			from models m
			where m.model_id = $1 or m.id::text = $1
			union all
			select `+modelColumns+`, 1 as source
			from models m
			join model_aliases a on a.model_id = m.id
			where a.alias = $1
			order by source
			limit 1
		) m`, requested))
	if err != nil {
		return nil, repo.Err(op, err)
	}
	return model, nil
}

// UpdateModelParams adalah pembaruan sebagian sebuah model. Field pointer nil berarti
// biarkan; field Opt bisa juga dikosongkan lewat Clear.
type UpdateModelParams struct {
	// ModelID mengganti nama kanonik. Perlu diingat nama lama tidak otomatis menjadi
	// alias, jadi klien yang masih memakainya akan mendapat "model tidak dikenal" —
	// tambahkan aliasnya kalau itu bukan yang diinginkan.
	ModelID     *string
	DisplayName *string
	Family      Opt[string]

	ContextWindow   Opt[int]
	MaxOutputTokens Opt[int]

	// Capabilities nil berarti biarkan; daftar kosong berarti kosongkan.
	Capabilities []string

	Enabled         *bool
	RoutingPriority *int
	RoutingStrategy Opt[string]

	DeprecatedAt Opt[time.Time]
	Metadata     []byte
}

// Update mengubah field yang diminta saja.
func (r *ModelRepo) Update(ctx context.Context, id string, p UpdateModelParams) (*Model, error) {
	const op = "memperbarui model"
	if !idOK(id) {
		return nil, fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}

	set := newUpdateSet(id)
	if p.ModelID != nil {
		set.add("model_id", *p.ModelID)
	}
	if p.DisplayName != nil {
		set.add("display_name", *p.DisplayName)
	}
	addOpt(set, "family", p.Family)
	addOpt(set, "context_window", p.ContextWindow)
	addOpt(set, "max_output_tokens", p.MaxOutputTokens)
	if p.Capabilities != nil {
		set.add("capabilities", p.Capabilities)
	}
	if p.Enabled != nil {
		set.add("enabled", *p.Enabled)
	}
	if p.RoutingPriority != nil {
		set.add("routing_priority", *p.RoutingPriority)
	}
	addOpt(set, "routing_strategy", p.RoutingStrategy)
	addOpt(set, "deprecated_at", p.DeprecatedAt)
	if p.Metadata != nil {
		set.add("metadata", p.Metadata)
	}

	if set.empty() {
		return nil, fmt.Errorf("%s: %w", op, ErrNoChanges)
	}

	model, err := scanModel(r.q.QueryRow(ctx,
		`update models as m set `+set.clause()+` where m.id = $1 returning `+modelColumns, set.args...))
	if err != nil {
		if aliasCollision(err) {
			return nil, fmt.Errorf("%s: %w: model_id baru sudah dipakai sebagai alias model lain",
				op, repo.ErrConflict)
		}
		return nil, repo.Err(op, err)
	}
	return model, nil
}

// Delete menghapus model beserta alias dan pemetaan providernya (on delete cascade).
func (r *ModelRepo) Delete(ctx context.Context, id string) error {
	const op = "menghapus model"
	if !idOK(id) {
		return fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}

	tag, err := r.q.Exec(ctx, `delete from models where id = $1`, id)
	if err != nil {
		return repo.Err(op, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}
	return nil
}

// ModelFilter menyaring daftar model.
type ModelFilter struct {
	// Capabilities menyaring model yang punya SEMUA kemampuan yang disebut.
	Capabilities []string
	// Enabled nil berarti aktif dan tidak aktif keduanya.
	Enabled *bool
	// Family kosong berarti semua keluarga.
	Family string
	// Search mencocokkan model_id dan nama tampilan.
	Search string
	// ExcludeDeprecated membuang model yang sudah ditandai usang. Daftar model untuk
	// klien memakainya; dashboard admin biasanya tidak.
	ExcludeDeprecated bool
}

// List mengembalikan model berpaginasi keyset, terurut menurut model_id.
//
// Kursornya cukup berisi model_id karena kolom itu unik — tidak perlu penanda kedua
// seperti pada tabel lain — dan urutan abjad adalah urutan yang paling berguna untuk
// katalog yang dibaca manusia maupun dikirim sebagai /v1/models.
//
// Penyaringan kemampuan memakai operator "@>" (mengandung), bukan unnest atau ANY,
// karena hanya bentuk itu yang bisa dilayani indeks GIN models_capabilities_idx. Dengan
// unnest, setiap baris harus dibongkar lebih dulu sehingga seluruh tabel dibaca; dengan
// "@>", PostgreSQL mencari lewat indeks dan hanya menyentuh baris yang cocok.
func (r *ModelRepo) List(ctx context.Context, f ModelFilter, page repo.Page) ([]*Model, string, error) {
	const op = "mendaftar model"
	limit := page.Normalize()
	sql, args := listModelsQuery(f, page.Cursor, limit)

	rows, err := r.q.Query(ctx, sql, args...)
	if err != nil {
		return nil, "", repo.Err(op, err)
	}
	defer rows.Close()

	out := make([]*Model, 0, limit)
	for rows.Next() {
		model, err := scanModel(rows)
		if err != nil {
			return nil, "", repo.Err(op, err)
		}
		out = append(out, model)
	}
	if err := rows.Err(); err != nil {
		return nil, "", repo.Err(op, err)
	}

	next := ""
	if len(out) > limit {
		out = out[:limit]
		next = out[limit-1].ModelID
	}
	return out, next, nil
}

// listModelsQuery merakit query daftar model.
//
// Dipisah dari List supaya test bisa menjalankan EXPLAIN pada teks query yang persis sama
// dengan yang dipakai produksi: bukti bahwa penyaringan kemampuan memakai indeks GIN jadi
// tidak bisa basi karena querynya diubah tanpa test ikut diubah.
func listModelsQuery(f ModelFilter, cursor string, limit int) (string, []any) {
	var (
		conds []string
		args  []any
	)
	add := func(cond string, arg any) {
		args = append(args, arg)
		conds = append(conds, fmt.Sprintf(cond, len(args)))
	}

	if len(f.Capabilities) > 0 {
		add("m.capabilities @> $%d::text[]", f.Capabilities)
	}
	if f.Enabled != nil {
		add("m.enabled = $%d", *f.Enabled)
	}
	if f.Family != "" {
		add("m.family = $%d", f.Family)
	}
	if f.Search != "" {
		args = append(args, f.Search)
		n := len(args)
		conds = append(conds, fmt.Sprintf(
			"(m.model_id ilike '%%' || $%d || '%%' "+
				"or m.display_name ilike '%%' || $%d || '%%' "+
				"or m.family ilike '%%' || $%d || '%%' "+
				"or exists (select 1 from provider_models pm join providers p on p.id = pm.provider_id where pm.model_id = m.id and (p.name ilike '%%' || $%d || '%%' or p.display_name ilike '%%' || $%d || '%%' or pm.upstream_model_name ilike '%%' || $%d || '%%')) "+
				"or exists (select 1 from model_aliases ma where ma.model_id = m.id and ma.alias ilike '%%' || $%d || '%%'))",
			n, n, n, n, n, n, n))
	}
	if f.ExcludeDeprecated {
		conds = append(conds, "m.deprecated_at is null")
	}
	if cursor != "" {
		add("m.model_id > $%d", cursor)
	}

	where := ""
	if len(conds) > 0 {
		where = " where " + strings.Join(conds, " and ")
	}
	args = append(args, limit+1) // satu baris lebih untuk mengetahui ada halaman lagi

	return `select ` + modelColumns + ` from models m` + where +
		fmt.Sprintf(` order by m.model_id asc limit $%d`, len(args)), args
}

// Alias adalah satu baris tabel model_aliases.
type Alias struct {
	Alias     string
	ModelID   string
	CreatedAt time.Time
	CreatedBy *string
}

// AddAlias menambahkan nama alternatif untuk sebuah model.
//
// Alias tidak boleh sama dengan model_id kanonik mana pun; trigger di skema menolaknya
// sebagai pelanggaran keunikan. Pesannya diganti di sini karena error bawaan hanya
// berbunyi "data sudah ada" tanpa menyebut constraint, sehingga pengguna tidak tahu
// bahwa masalahnya adalah tabrakan dengan nama kanonik, bukan dengan alias lain.
func (r *ModelRepo) AddAlias(ctx context.Context, modelID, alias string, createdBy *string) (*Alias, error) {
	const op = "menambah alias model"
	if !idOK(modelID) {
		return nil, fmt.Errorf("%s: %w: model_id bukan UUID", op, repo.ErrInvalidReference)
	}
	if err := refID(op, "created_by", createdBy); err != nil {
		return nil, err
	}

	var a Alias
	err := r.q.QueryRow(ctx, `
		insert into model_aliases (alias, model_id, created_by) values ($1, $2, $3)
		returning alias, model_id::text, created_at, created_by::text`,
		alias, modelID, createdBy).Scan(&a.Alias, &a.ModelID, &a.CreatedAt, &a.CreatedBy)
	if err != nil {
		if aliasCollision(err) {
			return nil, fmt.Errorf(
				"%s: %w: %q sudah dipakai sebagai model_id kanonik, jadi alias itu tidak akan pernah terpakai",
				op, repo.ErrConflict, alias)
		}
		return nil, repo.Err(op, err)
	}
	return &a, nil
}

// RemoveAlias menghapus satu alias.
func (r *ModelRepo) RemoveAlias(ctx context.Context, alias string) error {
	const op = "menghapus alias model"

	tag, err := r.q.Exec(ctx, `delete from model_aliases where alias = $1`, alias)
	if err != nil {
		return repo.Err(op, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}
	return nil
}

// ListAliases mengembalikan seluruh alias sebuah model, terurut abjad.
//
// Tanpa paginasi: alias per model hanya beberapa buah, dan daftar ini selalu ditampilkan
// utuh di halaman detail model.
func (r *ModelRepo) ListAliases(ctx context.Context, modelID string) ([]*Alias, error) {
	const op = "mendaftar alias model"
	if !idOK(modelID) {
		return nil, fmt.Errorf("%s: %w: model_id bukan UUID", op, repo.ErrInvalidReference)
	}

	rows, err := r.q.Query(ctx, `
		select alias, model_id::text, created_at, created_by::text
		from model_aliases where model_id = $1 order by alias asc`, modelID)
	if err != nil {
		return nil, repo.Err(op, err)
	}
	defer rows.Close()

	var out []*Alias
	for rows.Next() {
		var a Alias
		if err := rows.Scan(&a.Alias, &a.ModelID, &a.CreatedAt, &a.CreatedBy); err != nil {
			return nil, repo.Err(op, err)
		}
		out = append(out, &a)
	}
	if err := rows.Err(); err != nil {
		return nil, repo.Err(op, err)
	}
	return out, nil
}

// ProviderModel adalah satu baris tabel provider_models: pemetaan model kanonik ke
// provider yang bisa melayaninya.
type ProviderModel struct {
	ID         string
	ModelID    string
	ProviderID string
	// UpstreamModelName adalah nama model di sisi upstream, yang sering berbeda dari
	// nama kanonik.
	UpstreamModelName string

	Enabled bool
	// Priority dan Weight nil berarti memakai nilai dari tabel providers.
	Priority *int
	Weight   *int

	MaxContextWindow  *int
	SupportsStreaming bool
	SupportsTools     bool

	CreatedAt time.Time
	UpdatedAt time.Time
}

// providerModelColumns disatukan supaya bentuk barisnya sama di semua query.
const providerModelColumns = `
	id::text, model_id::text, provider_id::text, upstream_model_name,
	enabled, priority, weight, max_context_window,
	supports_streaming, supports_tools, created_at, updated_at`

// scanProviderModel membaca satu baris sesuai providerModelColumns.
func scanProviderModel(row pgxRow) (*ProviderModel, error) {
	var pm ProviderModel
	err := row.Scan(
		&pm.ID, &pm.ModelID, &pm.ProviderID, &pm.UpstreamModelName,
		&pm.Enabled, &pm.Priority, &pm.Weight, &pm.MaxContextWindow,
		&pm.SupportsStreaming, &pm.SupportsTools, &pm.CreatedAt, &pm.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &pm, nil
}

// AttachProviderParams adalah masukan pemetaan model ke provider.
type AttachProviderParams struct {
	ModelID    string
	ProviderID string
	// UpstreamModelName kosong berarti memakai model_id kanonik apa adanya, yang benar
	// untuk provider yang memakai nama yang sama seperti katalog kami.
	UpstreamModelName string

	Enabled *bool
	// Priority dan Weight nil berarti mengikuti nilai provider.
	Priority *int
	Weight   *int

	MaxContextWindow  *int
	SupportsStreaming *bool
	SupportsTools     *bool
}

// AttachProvider mendaftarkan satu provider sebagai penyedia sebuah model.
func (r *ModelRepo) AttachProvider(ctx context.Context, p AttachProviderParams) (*ProviderModel, error) {
	const op = "memetakan model ke provider"
	if !idOK(p.ModelID) {
		return nil, fmt.Errorf("%s: %w: model_id bukan UUID", op, repo.ErrInvalidReference)
	}
	if !idOK(p.ProviderID) {
		return nil, fmt.Errorf("%s: %w: provider_id bukan UUID", op, repo.ErrInvalidReference)
	}

	// Nama upstream yang dibiarkan kosong diambil dari katalog di dalam query yang sama,
	// bukan lewat query terpisah, supaya tidak ada celah antara membaca nama kanonik dan
	// memakainya.
	row := r.q.QueryRow(ctx, `
		insert into provider_models (
			model_id, provider_id, upstream_model_name, enabled, priority, weight,
			max_context_window, supports_streaming, supports_tools
		) values (
			$1, $2,
			coalesce(nullif(btrim($3), ''), (select model_id from models where id = $1)),
			$4, $5, $6, $7, $8, $9
		)
		returning `+providerModelColumns,
		p.ModelID, p.ProviderID, p.UpstreamModelName, boolOr(p.Enabled, true),
		p.Priority, p.Weight, p.MaxContextWindow,
		boolOr(p.SupportsStreaming, true), boolOr(p.SupportsTools, true),
	)

	pm, err := scanProviderModel(row)
	if err != nil {
		return nil, repo.Err(op, err)
	}
	return pm, nil
}

// UpdateProviderModelParams adalah pembaruan sebagian sebuah pemetaan.
type UpdateProviderModelParams struct {
	UpstreamModelName *string
	Enabled           *bool
	Priority          Opt[int]
	Weight            Opt[int]
	MaxContextWindow  Opt[int]
	SupportsStreaming *bool
	SupportsTools     *bool
}

// UpdateProviderModel mengubah field yang diminta saja pada satu pemetaan.
func (r *ModelRepo) UpdateProviderModel(ctx context.Context, id string, p UpdateProviderModelParams) (*ProviderModel, error) {
	const op = "memperbarui pemetaan model-provider"
	if !idOK(id) {
		return nil, fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}

	set := newUpdateSet(id)
	if p.UpstreamModelName != nil {
		set.add("upstream_model_name", *p.UpstreamModelName)
	}
	if p.Enabled != nil {
		set.add("enabled", *p.Enabled)
	}
	addOpt(set, "priority", p.Priority)
	addOpt(set, "weight", p.Weight)
	addOpt(set, "max_context_window", p.MaxContextWindow)
	if p.SupportsStreaming != nil {
		set.add("supports_streaming", *p.SupportsStreaming)
	}
	if p.SupportsTools != nil {
		set.add("supports_tools", *p.SupportsTools)
	}

	if set.empty() {
		return nil, fmt.Errorf("%s: %w", op, ErrNoChanges)
	}

	pm, err := scanProviderModel(r.q.QueryRow(ctx,
		`update provider_models set `+set.clause()+` where id = $1 returning `+providerModelColumns, set.args...))
	if err != nil {
		return nil, repo.Err(op, err)
	}
	return pm, nil
}

// DetachProvider menghapus satu pemetaan model-provider beserta riwayat harganya
// (on delete cascade). Riwayat harga ikut hilang karena harga selalu milik pasangan
// model-provider, bukan milik model atau provider sendiri.
func (r *ModelRepo) DetachProvider(ctx context.Context, id string) error {
	const op = "menghapus pemetaan model-provider"
	if !idOK(id) {
		return fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}

	tag, err := r.q.Exec(ctx, `delete from provider_models where id = $1`, id)
	if err != nil {
		return repo.Err(op, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}
	return nil
}

// GetProviderModel mengambil satu pemetaan menurut pengenalnya.
func (r *ModelRepo) GetProviderModel(ctx context.Context, id string) (*ProviderModel, error) {
	const op = "mengambil pemetaan model-provider"
	if !idOK(id) {
		return nil, fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}

	pm, err := scanProviderModel(r.q.QueryRow(ctx,
		`select `+providerModelColumns+` from provider_models where id = $1`, id))
	if err != nil {
		return nil, repo.Err(op, err)
	}
	return pm, nil
}

// ProviderModelFilter menyaring daftar pemetaan. Kedua pengenal boleh diisi bersamaan;
// karena pasangannya unik, hasilnya paling banyak satu baris.
type ProviderModelFilter struct {
	ModelID    string
	ProviderID string
	Enabled    *bool
}

// ListProviderModels mengembalikan pemetaan model-provider, terurut prioritas efektif.
//
// Tanpa paginasi: jumlah pemetaan per model maupun per provider dibatasi oleh berapa
// banyak provider yang benar-benar dipasang operator, dan halaman detail selalu
// menampilkannya utuh.
func (r *ModelRepo) ListProviderModels(ctx context.Context, f ProviderModelFilter) ([]*ProviderModel, error) {
	const op = "mendaftar pemetaan model-provider"

	var (
		conds []string
		args  []any
	)
	if f.ModelID != "" {
		if !idOK(f.ModelID) {
			return nil, fmt.Errorf("%s: %w: model_id bukan UUID", op, repo.ErrInvalidReference)
		}
		args = append(args, f.ModelID)
		conds = append(conds, fmt.Sprintf("model_id = $%d", len(args)))
	}
	if f.ProviderID != "" {
		if !idOK(f.ProviderID) {
			return nil, fmt.Errorf("%s: %w: provider_id bukan UUID", op, repo.ErrInvalidReference)
		}
		args = append(args, f.ProviderID)
		conds = append(conds, fmt.Sprintf("provider_id = $%d", len(args)))
	}
	if f.Enabled != nil {
		args = append(args, *f.Enabled)
		conds = append(conds, fmt.Sprintf("enabled = $%d", len(args)))
	}

	where := ""
	if len(conds) > 0 {
		where = " where " + strings.Join(conds, " and ")
	}

	rows, err := r.q.Query(ctx, `select `+providerModelColumns+` from provider_models`+where+
		` order by priority asc nulls last, weight desc nulls last, created_at asc`, args...)
	if err != nil {
		return nil, repo.Err(op, err)
	}
	defer rows.Close()

	var out []*ProviderModel
	for rows.Next() {
		pm, err := scanProviderModel(rows)
		if err != nil {
			return nil, repo.Err(op, err)
		}
		out = append(out, pm)
	}
	if err := rows.Err(); err != nil {
		return nil, repo.Err(op, err)
	}
	return out, nil
}
