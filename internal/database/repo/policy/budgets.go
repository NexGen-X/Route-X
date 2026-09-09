package policy

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/NexGen-X/Route-X/internal/database/repo"
	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
)

// Budget adalah satu baris budgets: anggaran biaya untuk satu cakupan dan satu periode.
//
// Nilai uang bertipe upstream.USD, bukan float64. Alasannya ada di doc tipe itu, dan di
// sini akibatnya paling langsung: anggaran yang dibandingkan dengan nilai floating point
// akan memblokir atau meloloskan permintaan di sekitar batasnya secara tidak konsisten.
type Budget struct {
	ID      string
	Name    string
	Scope   string
	ScopeID string
	Period  string

	LimitUSD upstream.USD
	SpentUSD upstream.USD

	PeriodStart time.Time
	PeriodEnd   *time.Time

	ActionOnExceed    string
	AlertThresholdPct int
	AlertedAt         *time.Time
	ExceededAlertedAt *time.Time

	Enabled   bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Exceeded melaporkan apakah pemakaian sudah mencapai batas.
//
// Perbandingannya >=, bukan >: anggaran 10 USD yang sudah terpakai tepat 10 USD sudah
// habis, dan meloloskan satu permintaan lagi di titik itu berarti setiap anggaran
// sebenarnya berlaku "batas ditambah satu permintaan".
func (b *Budget) Exceeded() bool { return b.SpentUSD >= b.LimitUSD }

// Blocks melaporkan apakah anggaran ini menolak permintaan baru.
//
// Terlampaui saja tidak cukup: action_on_exceed 'warn' memang dimaksudkan untuk
// memberitahu tanpa menghentikan lalu lintas, dan memperlakukannya sebagai blokir akan
// mematikan produksi milik operator yang justru memilih untuk tidak diblokir.
func (b *Budget) Blocks() bool { return b.Enabled && b.ActionOnExceed == ActionBlock && b.Exceeded() }

// ShouldAlertThreshold melaporkan apakah ambang peringatan sudah terlewati dan belum diberitahukan.
// Hanya berlaku sebelum batas limit terlampaui (b.SpentUSD < b.LimitUSD) dan AlertThresholdPct > 0.
// Ambang nol berarti peringatan ambang MATI (ShouldAlertThreshold selalu false).
// CreateBudget menerjemahkan 0 menjadi 80 sebagai default operator; baca dan tulis
// sengaja asimetris: API tidak bisa menyimpan 0 eksplisit lewat Create.
func (b *Budget) ShouldAlertThreshold() bool {
	if !b.Enabled || b.AlertedAt != nil || b.LimitUSD <= 0 || b.AlertThresholdPct <= 0 {
		return false
	}
	if b.Exceeded() {
		return false
	}
	// Perbandingan dilakukan tanpa floating point, tetapi spent * 100 bisa
	// meluap int64 pada spent ~9.2e16 unit (92 juta USD — mustahil hari ini,
	// tetapi aritmetika uang tidak boleh punya batas runyam). Bentuk yang
	// setara dan kebal luap: spent/100 >= limit*pct/100/100 tidak presisi,
	// jadi gunakan cross-multiply yang menjaga presisi: bandingkan
	// spent*100 >= limit*pct dengan deteksi luap — bila meluap, spent sudah
	// pasti di atas ambang (karena MaxInt64/100 ~ 9.2e16, pct maks 100).
	spent, limit, pct := int64(b.SpentUSD), int64(b.LimitUSD), int64(b.AlertThresholdPct)
	if spent > (1<<63-1)/100 {
		return true
	}
	if limit > (1<<63-1)/pct {
		// limit*pct meluap: limit sudah astronomis, spent di bawah meluap
		// berarti tidak mungkin melampaui ambang yang juga meluap — tetap
		// hitung dengan pembagian aman.
		return spent >= limit/100*pct
	}
	return spent*100 >= limit*pct
}

// ShouldAlertExceeded melaporkan apakah batas anggaran sudah terlampaui dan belum diberitahukan.
func (b *Budget) ShouldAlertExceeded() bool {
	if !b.Enabled || b.ExceededAlertedAt != nil || b.LimitUSD <= 0 {
		return false
	}
	return b.Exceeded()
}

// ShouldAlert melaporkan apakah ambang peringatan sudah terlewati dan belum diberitahukan.
// Dipertahankan demi kompatibilitas balik pemanggil yang memeriksa ambang batas.
func (b *Budget) ShouldAlert() bool {
	return b.ShouldAlertThreshold()
}

// Remaining mengembalikan sisa anggaran, nol bila sudah terlampaui.
func (b *Budget) Remaining() upstream.USD {
	if b.SpentUSD >= b.LimitUSD {
		return 0
	}
	return b.LimitUSD - b.SpentUSD
}

// Kolom uang dibaca sebagai teks lalu diurai ParseUSD, bukan didekode sebagai numeric oleh
// driver: pgx menyerahkan numeric sebagai tipe desimalnya sendiri atau float64, dan kedua
// jalur itu menambah satu konversi yang bisa membulatkan. Teks adalah bentuk yang tepat
// dan satu-satunya yang bisa diurai tanpa kehilangan.
const budgetColumns = `id::text, name, scope, coalesce(scope_id, ''), period,
	limit_usd::text, spent_usd::text,
	period_start, period_end, action_on_exceed, alert_threshold_pct, alerted_at, exceeded_alerted_at, enabled,
	created_at, updated_at`

// scanBudget membaca satu baris budgets sesuai budgetColumns.
func scanBudget(s interface{ Scan(...any) error }) (*Budget, error) {
	var (
		b                    Budget
		limitText, spentText string
	)
	err := s.Scan(&b.ID, &b.Name, &b.Scope, &b.ScopeID, &b.Period,
		&limitText, &spentText,
		&b.PeriodStart, &b.PeriodEnd, &b.ActionOnExceed, &b.AlertThresholdPct, &b.AlertedAt, &b.ExceededAlertedAt, &b.Enabled,
		&b.CreatedAt, &b.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if b.LimitUSD, err = upstream.ParseUSD(limitText); err != nil {
		return nil, fmt.Errorf("limit_usd anggaran %s: %w", b.ID, err)
	}
	if b.SpentUSD, err = upstream.ParseUSD(spentText); err != nil {
		return nil, fmt.Errorf("spent_usd anggaran %s: %w", b.ID, err)
	}
	return &b, nil
}

// ActiveBudgets mengembalikan seluruh anggaran yang aktif dan periodenya belum berakhir.
//
// period_end yang sudah lewat berarti periode itu selesai dan angka spent_usd-nya milik
// masa lalu; menegakkannya berarti memblokir lalu lintas atas pemakaian bulan lalu. Baris
// seperti itu menunggu worker pemelihara periode, dan sampai itu terjadi ia tidak boleh
// menolak apa pun.
func (r *Repo) ActiveBudgets(ctx context.Context) ([]*Budget, error) {
	const op = "mengambil anggaran aktif"

	rows, err := r.q.Query(ctx, `
		select `+budgetColumns+`
		from budgets
		where enabled and (period_end is null or period_end > now())
		order by (scope <> 'global'), scope, created_at, id`)
	if err != nil {
		return nil, repo.Err(op, err)
	}
	defer rows.Close()

	var out []*Budget
	for rows.Next() {
		b, err := scanBudget(rows)
		if err != nil {
			return nil, repo.Err(op, err)
		}
		out = append(out, b)
	}
	if err := rows.Err(); err != nil {
		return nil, repo.Err(op, err)
	}
	return out, nil
}

// ExpiredBudgets mengembalikan anggaran aktif yang batas period_end-nya sudah lewat.
//
// ActiveBudgets sengaja mengecualikan baris-baris ini agar lalu lintas baru tidak diblokir
// oleh pemakaian periode lampau. Fungsi ini adalah pasangannya untuk background worker:
// menemukan anggaran yang periodenya sudah selesai agar bisa dimajukan ke periode berikutnya.
func (r *Repo) ExpiredBudgets(ctx context.Context, asOf time.Time) ([]*Budget, error) {
	const op = "mengambil anggaran kedaluwarsa"

	rows, err := r.q.Query(ctx, `
		select `+budgetColumns+`
		from budgets
		where enabled and period_end is not null and period_end <= $1
		order by created_at, id`, asOf)
	if err != nil {
		return nil, repo.Err(op, err)
	}
	defer rows.Close()

	var out []*Budget
	for rows.Next() {
		b, err := scanBudget(rows)
		if err != nil {
			return nil, repo.Err(op, err)
		}
		out = append(out, b)
	}
	if err := rows.Err(); err != nil {
		return nil, repo.Err(op, err)
	}
	return out, nil
}

// CreateBudgetParams adalah masukan pembuatan anggaran.
type CreateBudgetParams struct {
	Name    string
	Scope   string
	ScopeID string
	Period  string

	LimitUSD upstream.USD

	PeriodStart time.Time
	PeriodEnd   *time.Time

	ActionOnExceed    string
	AlertThresholdPct int

	CreatedBy string
}

// CreateBudget menyimpan satu anggaran baru.
func (r *Repo) CreateBudget(ctx context.Context, p CreateBudgetParams) (*Budget, error) {
	const op = "membuat anggaran"

	var createdBy any
	if p.CreatedBy != "" {
		if !idOK(p.CreatedBy) {
			return nil, fmt.Errorf("%s: %w: created_by bukan UUID", op, repo.ErrInvalidReference)
		}
		createdBy = p.CreatedBy
	}

	action := p.ActionOnExceed
	if action == "" {
		action = ActionBlock
	}
	threshold := p.AlertThresholdPct
	if threshold == 0 {
		threshold = 80
	}
	var start any
	if !p.PeriodStart.IsZero() {
		start = p.PeriodStart
	}

	row := r.q.QueryRow(ctx, `
		insert into budgets (name, scope, scope_id, period, limit_usd,
			period_start, period_end, action_on_exceed, alert_threshold_pct, created_by)
		values ($1, $2, $3, $4, $5::numeric, coalesce($6, now()), $7, $8, $9, $10)
		returning `+budgetColumns,
		p.Name, p.Scope, nullIfEmpty(p.ScopeID), p.Period, p.LimitUSD.Decimal(),
		start, p.PeriodEnd, action, threshold, createdBy)

	b, err := scanBudget(row)
	if err != nil {
		return nil, repo.Err(op, err)
	}
	return b, nil
}

// AddSpend menambahkan biaya ke SETIAP anggaran aktif yang berlaku bagi target-target ini.
//
// Satu UPDATE untuk semua cakupan sekaligus, bukan satu per cakupan: satu permintaan
// biasanya dikenai anggaran global, anggaran API key, anggaran pengguna, dan anggaran
// modelnya, dan empat UPDATE berurutan berarti empat kali biaya perjalanan ditambah
// kemungkinan sebagian berhasil sebagian tidak.
//
// Presisi: kolomnya numeric(14,6) sementara USD berskala 8, jadi PostgreSQL MEMBULATKAN
// setiap penambahan ke enam angka desimal. Selisih maksimumnya 5e-7 USD per permintaan.
// Angka yang berwenang untuk penagihan adalah requests.cost_usd (skala 8); spent_usd di
// sini adalah penghitung untuk PENEGAKAN, dan worker pemelihara periode bisa menghitungnya
// ulang dari sumber berskala penuh itu.
//
// Mengembalikan jumlah baris anggaran yang tersentuh.
func (r *Repo) AddSpend(ctx context.Context, targets []Target, amount upstream.USD) (int64, error) {
	const op = "menambah pemakaian anggaran"
	if amount == 0 {
		return 0, nil
	}
	// Jumlah negatif DITOLAK: lolos ke UPDATE berarti spent_usd BERKURANG —
	// pemakaian anggaran yang bisa dikurangi lewat pemanggilan repo.
	// Satu-satunya jalan mengurangi yang sah adalah reset periode oleh worker.
	if amount < 0 {
		return 0, fmt.Errorf("%s: %w: jumlah negatif %s tidak diizinkan", op, repo.ErrConstraint, amount.String())
	}

	scopes, ids := scopeArrays(targets)
	tag, err := r.q.Exec(ctx, `
		update budgets
		set spent_usd = spent_usd + $1::numeric, updated_at = now()
		where enabled
		  and (period_end is null or period_end > now())
		  and (scope = 'global'
		       or exists (select 1 from unnest($2::text[], $3::text[]) as t(s, i)
		                  where t.s = budgets.scope and t.i = budgets.scope_id))`,
		amount.Decimal(), scopes, ids)
	if err != nil {
		return 0, repo.Err(op, err)
	}
	return tag.RowsAffected(), nil
}

// ResetPeriod memulai periode baru: pemakaian kembali nol dan penanda peringatan dilepas.
//
// Dipakai worker pemeliharaan periode. Penanda peringatan WAJIB ikut dilepas — kalau tidak,
// anggaran yang pernah memicu peringatan tidak akan pernah memperingatkan lagi di
// periode-periode berikutnya, dan kegagalannya berupa notifikasi yang tidak datang, yaitu
// hal yang tidak terlihat sampai seseorang menyadari tagihannya sudah lewat batas.
func (r *Repo) ResetPeriod(ctx context.Context, id string, start time.Time, end *time.Time) (*Budget, error) {
	const op = "mereset periode anggaran"
	if !idOK(id) {
		return nil, fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}

	row := r.q.QueryRow(ctx, `
		update budgets
		set spent_usd = 0, period_start = $2, period_end = $3, alerted_at = null, exceeded_alerted_at = null, updated_at = now()
		where id = $1
		returning `+budgetColumns, id, start, end)

	b, err := scanBudget(row)
	if err != nil {
		return nil, repo.Err(op, err)
	}
	return b, nil
}

// ResetPeriodWithSpend memulai periode baru dengan nilai spent_usd terhitung ulang dan
// penanda peringatan dilepas.
//
// Dipakai worker reset periode. spent_usd diisi dari hasil hitung ulang requests.cost_usd
// (skala 8) agar tidak ada pembulatan yang tertimbun dan memperhitungkan request yang mungkin
// sudah mendarat di periode baru sebelum worker sempat mereset.
func (r *Repo) ResetPeriodWithSpend(ctx context.Context, id string, start time.Time, end *time.Time, spent upstream.USD) (*Budget, error) {
	const op = "mereset periode anggaran dengan pemakaian terhitung"
	if !idOK(id) {
		return nil, fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}

	row := r.q.QueryRow(ctx, `
		update budgets
		set spent_usd = $2::numeric, period_start = $3, period_end = $4, alerted_at = null, exceeded_alerted_at = null, updated_at = now()
		where id = $1
		returning `+budgetColumns, id, spent.Decimal(), start, end)

	b, err := scanBudget(row)
	if err != nil {
		return nil, repo.Err(op, err)
	}
	return b, nil
}

// CalculateSpend menghitung ulang akumulasi biaya nyata dari requests.cost_usd (skala 8).
//
// Dipakai saat reset periode anggaran: kolom budgets.spent_usd bertipe numeric(14,6) sehingga
// setiap AddSpend dibulatkan PostgreSQL (selisih hingga 5e-7 USD per permintaan). Menghitung
// ulang dari tabel requests memulihkan presisi penuh 8 desimal sebelum periode baru dimulai.
func (r *Repo) CalculateSpend(ctx context.Context, scope, scopeID string, from time.Time, to *time.Time) (upstream.USD, error) {
	const op = "menghitung pemakaian riil dari log request"

	row := r.q.QueryRow(ctx, `
		select coalesce(sum(cost_usd), 0)::text
		from requests
		where created_at >= $1 and ($2::timestamptz is null or created_at < $2)
		  and (
		    ($3 = 'global')
		    or ($3 = 'api_key' and api_key_id = nullif($4, '')::uuid)
		    or ($3 = 'user' and user_id = nullif($4, '')::uuid)
		    or ($3 = 'provider' and provider_id = nullif($4, '')::uuid)
		    or ($3 = 'model' and model_id = nullif($4, '')::uuid)
		  )`, from, to, scope, scopeID)

	var valText string
	if err := row.Scan(&valText); err != nil {
		return 0, repo.Err(op, err)
	}
	usd, err := upstream.ParseUSD(valText)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", op, err)
	}
	return usd, nil
}

// MarkThresholdAlerted menandai bahwa peringatan ambang batas (budget.threshold) sudah dikirim.
//
// Bersyarat pada alerted_at yang masih NULL, sehingga dua instance yang memeriksa ambang
// pada saat yang sama hanya menghasilkan satu notifikasi. false berarti instance lain sudah
// mengirimkannya.
func (r *Repo) MarkThresholdAlerted(ctx context.Context, id string) (bool, error) {
	const op = "menandai peringatan ambang anggaran"
	if !idOK(id) {
		return false, fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}

	tag, err := r.q.Exec(ctx,
		`update budgets set alerted_at = now(), updated_at = now() where id = $1 and alerted_at is null`, id)
	if err != nil {
		return false, repo.Err(op, err)
	}
	return tag.RowsAffected() == 1, nil
}

// MarkExceededAlerted menandai bahwa peringatan batas terlampaui (budget.exceeded) sudah dikirim.
//
// Bersyarat pada exceeded_alerted_at yang masih NULL, sehingga notifikasi budget.exceeded
// terkirim tepat satu kali walaupun peringatan ambang (budget.threshold) telah dikirim sebelumnya.
func (r *Repo) MarkExceededAlerted(ctx context.Context, id string) (bool, error) {
	const op = "menandai peringatan batas terlampaui anggaran"
	if !idOK(id) {
		return false, fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}

	tag, err := r.q.Exec(ctx,
		`update budgets set exceeded_alerted_at = now(), updated_at = now() where id = $1 and exceeded_alerted_at is null`, id)
	if err != nil {
		return false, repo.Err(op, err)
	}
	return tag.RowsAffected() == 1, nil
}

// MarkAlerted menandai bahwa peringatan ambang sudah dikirim (kompatibilitas balik).
func (r *Repo) MarkAlerted(ctx context.Context, id string) (bool, error) {
	return r.MarkThresholdAlerted(ctx, id)
}

// SetBudgetEnabled menyalakan atau mematikan satu anggaran.
func (r *Repo) SetBudgetEnabled(ctx context.Context, id string, enabled bool) (*Budget, error) {
	const op = "mengubah status anggaran"
	if !idOK(id) {
		return nil, fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}

	row := r.q.QueryRow(ctx, `
		update budgets set enabled = $2, updated_at = now()
		where id = $1
		returning `+budgetColumns, id, enabled)

	b, err := scanBudget(row)
	if err != nil {
		return nil, repo.Err(op, err)
	}
	return b, nil
}

// DeleteBudget menghapus satu anggaran.
func (r *Repo) DeleteBudget(ctx context.Context, id string) error {
	const op = "menghapus anggaran"
	if !idOK(id) {
		return fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}

	tag, err := r.q.Exec(ctx, `delete from budgets where id = $1`, id)
	if err != nil {
		return repo.Err(op, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}
	return nil
}

// GetBudget mengambil satu baris budgets berdasarkan id.
func (r *Repo) GetBudget(ctx context.Context, id string) (*Budget, error) {
	const op = "mengambil anggaran"
	if !idOK(id) {
		return nil, fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}

	row := r.q.QueryRow(ctx, `select `+budgetColumns+` from budgets where id = $1`, id)
	b, err := scanBudget(row)
	if err != nil {
		return nil, repo.Err(op, err)
	}
	return b, nil
}

// UpdateBudgetParams berisi opsi pembaruan konfigurasi anggaran.
type UpdateBudgetParams struct {
	Name              *string
	LimitUSD          *upstream.USD
	ActionOnExceed    *string
	AlertThresholdPct *int
	Enabled           *bool
}

// UpdateBudget memperbarui konfigurasi anggaran.
func (r *Repo) UpdateBudget(ctx context.Context, id string, p UpdateBudgetParams) (*Budget, error) {
	const op = "memperbarui anggaran"
	if !idOK(id) {
		return nil, fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}

	var (
		setClauses []string
		args       []any
	)
	args = append(args, id)

	if p.Name != nil {
		args = append(args, *p.Name)
		setClauses = append(setClauses, fmt.Sprintf("name = $%d", len(args)))
	}
	if p.LimitUSD != nil {
		args = append(args, p.LimitUSD.String())
		setClauses = append(setClauses, fmt.Sprintf("limit_usd = $%d::numeric", len(args)))
	}
	if p.ActionOnExceed != nil {
		args = append(args, *p.ActionOnExceed)
		setClauses = append(setClauses, fmt.Sprintf("action_on_exceed = $%d", len(args)))
	}
	if p.AlertThresholdPct != nil {
		args = append(args, *p.AlertThresholdPct)
		setClauses = append(setClauses, fmt.Sprintf("alert_threshold_pct = $%d", len(args)))
	}
	if p.Enabled != nil {
		args = append(args, *p.Enabled)
		setClauses = append(setClauses, fmt.Sprintf("enabled = $%d", len(args)))
	}

	if len(setClauses) == 0 {
		return r.GetBudget(ctx, id)
	}

	setClauses = append(setClauses, "updated_at = now()")
	query := fmt.Sprintf(`update budgets set %s where id = $1 returning %s`,
		strings.Join(setClauses, ", "), budgetColumns)

	row := r.q.QueryRow(ctx, query, args...)
	b, err := scanBudget(row)
	if err != nil {
		return nil, repo.Err(op, err)
	}
	return b, nil
}

// ListBudgets mengambil seluruh anggaran dengan keyset pagination.
func (r *Repo) ListBudgets(ctx context.Context, page repo.Page) ([]*Budget, string, error) {
	const op = "mendaftar anggaran"
	limit := page.Normalize()

	var (
		query strings.Builder
		args  []any
	)
	query.WriteString(`select ` + budgetColumns + ` from budgets `)

	if page.Cursor != "" {
		ts, id, err := decodeCursor(op, page.Cursor)
		if err != nil {
			return nil, "", err
		}
		args = append(args, ts, id)
		query.WriteString(`where (created_at, id) < ($1, $2) `)
	}

	args = append(args, limit+1)
	query.WriteString(fmt.Sprintf(`order by created_at desc, id desc limit $%d`, len(args)))

	rows, err := r.q.Query(ctx, query.String(), args...)
	if err != nil {
		return nil, "", repo.Err(op, err)
	}
	defer rows.Close()

	var items []*Budget
	for rows.Next() {
		b, err := scanBudget(rows)
		if err != nil {
			return nil, "", repo.Err(op, err)
		}
		items = append(items, b)
	}
	if err := rows.Err(); err != nil {
		return nil, "", repo.Err(op, err)
	}

	var next string
	if len(items) > limit {
		items = items[:limit]
		last := items[limit-1]
		next = encodeCursor(last.CreatedAt, last.ID)
	}

	return items, next, nil
}
