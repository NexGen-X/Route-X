package upstream

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/NexGen-X/Route-X/internal/database/repo"
)

// Price adalah satu baris riwayat harga untuk sepasang model-provider.
//
// Semua harga adalah harga per SATU JUTA token, mengikuti konvensi yang dipakai seluruh
// penyedia, dan bertipe USD (bilangan bulat satuan 10^-8 dolar) — bukan float64.
type Price struct {
	ID              string
	ProviderModelID string

	Input  USD
	Output USD
	// CachedInput nil berarti penyedia tidak memberi harga khusus untuk token yang
	// terlayani prompt cache; perhitungan biaya lalu memakai harga input biasa.
	CachedInput *USD
	// Reasoning nil berarti token penalaran ditagih seperti token output, yang merupakan
	// perilaku semua penyedia yang tidak menagihnya terpisah.
	Reasoning *USD

	Currency string

	EffectiveFrom time.Time
	// EffectiveTo nil berarti inilah harga yang sedang berlaku.
	EffectiveTo *time.Time

	Source    string
	CreatedAt time.Time
	CreatedBy *string
}

// IsCurrent melaporkan apakah harga ini yang sedang berlaku.
func (p *Price) IsCurrent() bool { return p.EffectiveTo == nil }

// PricingRepo adalah akses data riwayat harga.
type PricingRepo struct {
	q repo.Querier
}

// NewPricingRepo membuat PricingRepo di atas pool maupun transaksi.
func NewPricingRepo(q repo.Querier) *PricingRepo { return &PricingRepo{q: q} }

// priceColumns membaca kolom numeric sebagai bilangan bulat satuan 10^-8 USD.
//
// Perkalian dengan 10^8 dilakukan PostgreSQL pada tipe numeric, jadi hasilnya eksak:
// kolomnya numeric(14,6) sehingga setiap nilai adalah bilangan bulat setelah dikali
// 10^8, dan cast ke bigint tidak membulatkan apa pun. Tidak ada nilai uang yang pernah
// melewati float64 di jalur ini.
const priceColumns = `
	id::text, provider_model_id::text,
	(input_usd_per_mtok        * 100000000)::bigint,
	(output_usd_per_mtok       * 100000000)::bigint,
	(cached_input_usd_per_mtok * 100000000)::bigint,
	(reasoning_usd_per_mtok    * 100000000)::bigint,
	currency, effective_from, effective_to, source, created_at, created_by::text`

// scanPrice membaca satu baris sesuai priceColumns.
func scanPrice(row pgxRow) (*Price, error) {
	var (
		p                      Price
		input, output          int64
		cachedInput, reasoning *int64
	)
	err := row.Scan(
		&p.ID, &p.ProviderModelID,
		&input, &output, &cachedInput, &reasoning,
		&p.Currency, &p.EffectiveFrom, &p.EffectiveTo, &p.Source, &p.CreatedAt, &p.CreatedBy,
	)
	if err != nil {
		return nil, err
	}
	p.Input = USD(input)
	p.Output = USD(output)
	if cachedInput != nil {
		v := USD(*cachedInput)
		p.CachedInput = &v
	}
	if reasoning != nil {
		v := USD(*reasoning)
		p.Reasoning = &v
	}
	return &p, nil
}

// SetPriceParams adalah masukan pemasangan harga baru.
type SetPriceParams struct {
	ProviderModelID string

	Input  USD
	Output USD
	// CachedInput dan Reasoning boleh nil; artinya lihat dokumentasi Price.
	CachedInput *USD
	Reasoning   *USD

	// Currency kosong menjadi "USD".
	Currency string
	// EffectiveFrom kosong berarti sekarang menurut jam database. Diisi hanya untuk
	// memasukkan harga yang berlaku mulai suatu saat di masa depan, atau saat mengimpor
	// riwayat harga lama.
	EffectiveFrom time.Time
	// Source kosong menjadi "manual"; dipakai membedakan angka resmi dari angka yang
	// diketik operator.
	Source    string
	CreatedBy *string
}

// validate memeriksa hal-hal yang lebih baik ditolak sebelum menyentuh database.
func (p SetPriceParams) validate(op string) error {
	if !idOK(p.ProviderModelID) {
		return fmt.Errorf("%s: %w: provider_model_id bukan UUID", op, repo.ErrInvalidReference)
	}
	// Kolom harga bertipe numeric(14,6). Nilai yang lebih presisi akan dibulatkan diam-diam
	// saat disimpan, dan harga yang berubah sendiri saat disimpan baru terlihat setelah
	// pengguna ditagih dengan angka yang salah.
	for _, f := range []struct {
		name  string
		value *USD
	}{
		{"input", &p.Input},
		{"output", &p.Output},
		{"cached_input", p.CachedInput},
		{"reasoning", p.Reasoning},
	} {
		if f.value == nil {
			continue
		}
		if *f.value < 0 {
			return fmt.Errorf("%s: %w: harga %s negatif", op, repo.ErrConstraint, f.name)
		}
		if !f.value.IsPriceScale() {
			return fmt.Errorf("%s: %w: harga %s (%s) lebih presisi daripada %d angka desimal yang bisa disimpan",
				op, repo.ErrConstraint, f.name, f.value.Decimal(), priceScale)
		}
	}
	return nil
}

// priceArg mengubah harga menjadi argumen teks desimal, atau NULL bila nil.
//
// Nilai dikirim sebagai teks lalu di-cast di SQL, bukan sebagai angka: itu satu-satunya
// jalur yang pasti tidak melewati floating point di driver mana pun.
func priceArg(v *USD) any {
	if v == nil {
		return nil
	}
	return v.Decimal()
}

// Set menutup harga yang sedang berlaku lalu membuka harga baru.
//
// WAJIB dijalankan di dalam transaksi, dan itu ditegakkan, bukan sekadar
// didokumentasikan: indeks unique partial model_pricing_one_current_idx hanya
// mengizinkan satu baris terbuka per pasangan model-provider, jadi kalau penutupan
// berhasil tetapi penyisipan gagal, pasangan itu akan kehilangan harga sama sekali dan
// setiap request berikutnya tercatat tanpa biaya.
//
// Alasan kedua: bila EffectiveFrom tidak diisi, waktu penutupan dan waktu pembukaan
// harus sama persis supaya garis waktu harga tidak berlubang. now() di PostgreSQL adalah
// waktu transaksi, sehingga dua pernyataan di dalam satu transaksi memang mendapat nilai
// yang identik — di luar transaksi, keduanya akan berbeda beberapa mikrodetik dan ada
// request yang jatuh di celah itu.
//
// Pakai SetPrice bila pemanggil hanya punya pool.
func (r *PricingRepo) Set(ctx context.Context, p SetPriceParams) (*Price, error) {
	const op = "memasang harga model"

	if _, inTx := r.q.(pgx.Tx); !inTx {
		return nil, fmt.Errorf("%s: %w", op, ErrNeedsTx)
	}
	if err := p.validate(op); err != nil {
		return nil, err
	}
	if err := refID(op, "created_by", p.CreatedBy); err != nil {
		return nil, err
	}

	// Satu titik waktu dipakai untuk menutup baris lama dan membuka baris baru, sehingga
	// effective_to yang lama sama dengan effective_from yang baru: tidak ada celah, tidak
	// ada tumpang tindih. Waktunya diambil dari database, bukan dari jam proses ini, agar
	// seluruh garis waktu harga berasal dari satu sumber.
	var from time.Time
	if err := r.q.QueryRow(ctx, `select coalesce($1::timestamptz, now())`,
		nullTime(p.EffectiveFrom)).Scan(&from); err != nil {
		return nil, repo.Err(op, err)
	}

	// Baris yang sedang berlaku dikunci lebih dulu supaya dua operator yang mengubah
	// harga bersamaan berjalan berurutan, bukan saling menimpa lalu salah satunya gagal
	// pada indeks unique dengan pesan yang tidak menjelaskan apa-apa.
	var (
		currentID   string
		currentFrom time.Time
	)
	err := r.q.QueryRow(ctx, `
		select id::text, effective_from from model_pricing
		where provider_model_id = $1 and effective_to is null
		for update`, p.ProviderModelID).Scan(&currentID, &currentFrom)
	switch {
	case err == nil:
		if !from.After(currentFrom) {
			return nil, fmt.Errorf("%s: %w: harga berlaku sejak %s, harga baru tidak boleh mulai pada atau sebelum itu",
				op, repo.ErrConflict, currentFrom.UTC().Format(time.RFC3339Nano))
		}
		if _, err := r.q.Exec(ctx,
			`update model_pricing set effective_to = $2 where id = $1`, currentID, from); err != nil {
			return nil, repo.Err(op, err)
		}
	case errors.Is(err, pgx.ErrNoRows):
		// Belum ada harga untuk pasangan ini; tidak ada yang perlu ditutup.
	default:
		return nil, repo.Err(op, err)
	}

	row := r.q.QueryRow(ctx, `
		insert into model_pricing (
			provider_model_id,
			input_usd_per_mtok, output_usd_per_mtok,
			cached_input_usd_per_mtok, reasoning_usd_per_mtok,
			currency, effective_from, source, created_by
		) values (
			$1,
			$2::text::numeric(14,6), $3::text::numeric(14,6),
			$4::text::numeric(14,6), $5::text::numeric(14,6),
			$6, $7, $8, $9
		)
		returning `+priceColumns,
		p.ProviderModelID,
		p.Input.Decimal(), p.Output.Decimal(), priceArg(p.CachedInput), priceArg(p.Reasoning),
		strOr(p.Currency, DefaultCurrency), from, strOr(p.Source, DefaultPriceSource), p.CreatedBy,
	)

	price, err := scanPrice(row)
	if err != nil {
		return nil, repo.Err(op, err)
	}
	return price, nil
}

// SetPrice menjalankan PricingRepo.Set di dalam transaksinya sendiri.
//
// Disediakan supaya pemanggil yang hanya memegang pool tidak perlu merakit transaksi
// sendiri — dan supaya tidak ada yang tergoda memanggil Set di luar transaksi.
func SetPrice(ctx context.Context, pool *pgxpool.Pool, p SetPriceParams) (*Price, error) {
	var out *Price
	err := repo.InTx(ctx, pool, func(q repo.Querier) error {
		price, err := NewPricingRepo(q).Set(ctx, p)
		if err != nil {
			return err
		}
		out = price
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// nullTime mengubah waktu kosong menjadi NULL.
func nullTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t
}

// Current mengambil harga yang sedang berlaku untuk sepasang model-provider.
//
// Dilayani indeks model_pricing_one_current_idx, yang sekaligus menjamin hasilnya paling
// banyak satu baris.
func (r *PricingRepo) Current(ctx context.Context, providerModelID string) (*Price, error) {
	const op = "mengambil harga berlaku"
	if !idOK(providerModelID) {
		return nil, fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}

	price, err := scanPrice(r.q.QueryRow(ctx, `select `+priceColumns+`
		from model_pricing where provider_model_id = $1 and effective_to is null`, providerModelID))
	if err != nil {
		return nil, repo.Err(op, err)
	}
	return price, nil
}

// At mengambil harga yang berlaku pada satu titik waktu.
//
// Inilah yang dipakai menghitung biaya request lampau: laporan bulan lalu harus tetap
// memakai harga bulan lalu, sehingga angkanya tidak berubah ketika penyedia menaikkan
// harga hari ini.
//
// Rentangnya setengah terbuka — effective_from <= t < effective_to — sehingga saat
// pergantian harga, satu titik waktu selalu jatuh tepat di satu baris. Urutan menurun
// dengan limit 1 adalah jaring pengaman kalau ada baris tumpang tindih yang masuk lewat
// SQL langsung: yang dipilih adalah harga terbaru yang berlaku, bukan baris yang
// kebetulan terbaca lebih dulu.
func (r *PricingRepo) At(ctx context.Context, providerModelID string, t time.Time) (*Price, error) {
	const op = "mengambil harga pada satu titik waktu"
	if !idOK(providerModelID) {
		return nil, fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}

	price, err := scanPrice(r.q.QueryRow(ctx, `select `+priceColumns+`
		from model_pricing
		where provider_model_id = $1
		  and effective_from <= $2
		  and (effective_to is null or effective_to > $2)
		order by effective_from desc
		limit 1`, providerModelID, t))
	if err != nil {
		return nil, repo.Err(op, err)
	}
	return price, nil
}

// History mengembalikan riwayat harga sepasang model-provider, terbaru dulu,
// berpaginasi keyset.
func (r *PricingRepo) History(ctx context.Context, providerModelID string, page repo.Page) ([]*Price, string, error) {
	const op = "mendaftar riwayat harga"
	if !idOK(providerModelID) {
		return nil, "", fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}
	limit := page.Normalize()

	args := []any{providerModelID}
	cond := ""
	if page.Cursor != "" {
		ts, id, err := decodeCursor(op, page.Cursor)
		if err != nil {
			return nil, "", err
		}
		if !idOK(id) {
			return nil, "", fmt.Errorf("%s: %w: bagian pengenal pada kursor bukan UUID", op, repo.ErrConstraint)
		}
		args = append(args, ts, id)
		cond = fmt.Sprintf(" and (effective_from, id) < ($%d::timestamptz, $%d::uuid)", len(args)-1, len(args))
	}
	args = append(args, limit+1)

	rows, err := r.q.Query(ctx, `select `+priceColumns+`
		from model_pricing where provider_model_id = $1`+cond+
		fmt.Sprintf(` order by effective_from desc, id desc limit $%d`, len(args)), args...)
	if err != nil {
		return nil, "", repo.Err(op, err)
	}
	defer rows.Close()

	out := make([]*Price, 0, limit)
	for rows.Next() {
		price, err := scanPrice(rows)
		if err != nil {
			return nil, "", repo.Err(op, err)
		}
		out = append(out, price)
	}
	if err := rows.Err(); err != nil {
		return nil, "", repo.Err(op, err)
	}

	next := ""
	if len(out) > limit {
		out = out[:limit]
		last := out[limit-1]
		next = encodeCursor(last.EffectiveFrom, last.ID)
	}
	return out, next, nil
}

// TokenUsage adalah jumlah token satu request, dipecah menurut cara penagihannya.
//
// Keempat angka SALING LEPAS. Token yang terlayani prompt cache dihitung di CachedInput
// dan tidak boleh ikut dihitung di Input. Ini penting karena penyedia melaporkannya
// dengan cara lain: OpenAI mengirim prompt_tokens yang SUDAH termasuk cached_tokens,
// jadi adaptornya wajib mengurangkan lebih dulu — kalau tidak, token cache tertagih dua
// kali, dengan harga penuh sekaligus dengan harga cache.
type TokenUsage struct {
	Input       int64
	CachedInput int64
	Output      int64
	Reasoning   int64
}

// Total mengembalikan jumlah seluruh token, sesuai kolom requests.total_tokens.
func (u TokenUsage) Total() int64 {
	return u.Input + u.CachedInput + u.Output + u.Reasoning
}

// CostBreakdown adalah biaya satu request beserta rinciannya.
//
// Total adalah jumlah dari keempat rincian setelah masing-masing dibulatkan, sehingga
// rincian yang ditampilkan di dashboard benar-benar berjumlah sama dengan totalnya.
// Membulatkan hanya sekali di akhir memang lebih tepat secara matematis, tetapi
// menghasilkan tabel yang tidak bisa dijumlahkan pengguna dengan tangan, dan selisihnya
// paling besar dua satuan 10^-8 dolar.
type CostBreakdown struct {
	Input       USD
	CachedInput USD
	Output      USD
	Reasoning   USD
	Total       USD
}

// Cost menghitung biaya sejumlah token dengan harga ini.
//
// Rumusnya per jenis token: jumlah_token × harga_per_juta_token ÷ 1.000.000. Perkaliannya
// dikerjakan pada bilangan bulat berpresisi besar, lalu dibagi sekali dengan pembulatan
// setengah ke atas. Tidak ada floating point di jalur ini, sehingga total sejuta request
// tidak pernah menyimpang dari penjumlahan yang sama di PostgreSQL.
//
// Harga yang tidak diisi punya arti yang jelas, bukan nol: token cache ditagih dengan
// harga input biasa (penyedia yang tidak memberi diskon cache), dan token penalaran
// ditagih dengan harga output (perilaku semua penyedia yang tidak menagihnya terpisah).
// Menganggapnya nol berarti menagih pengguna lebih murah daripada biaya kami sendiri.
func (p *Price) Cost(u TokenUsage) (CostBreakdown, error) {
	cachedPrice := p.Input
	if p.CachedInput != nil {
		cachedPrice = *p.CachedInput
	}
	reasoningPrice := p.Output
	if p.Reasoning != nil {
		reasoningPrice = *p.Reasoning
	}

	var (
		out CostBreakdown
		err error
	)
	for _, part := range []struct {
		name   string
		tokens int64
		price  USD
		into   *USD
	}{
		{"input", u.Input, p.Input, &out.Input},
		{"cached input", u.CachedInput, cachedPrice, &out.CachedInput},
		{"output", u.Output, p.Output, &out.Output},
		{"reasoning", u.Reasoning, reasoningPrice, &out.Reasoning},
	} {
		*part.into, err = costOf(part.tokens, part.price)
		if err != nil {
			return CostBreakdown{}, fmt.Errorf("menghitung biaya token %s: %w", part.name, err)
		}
	}

	out.Total = out.Input + out.CachedInput + out.Output + out.Reasoning
	return out, nil
}

// costOf menghitung tokens × perMTok ÷ 1.000.000 secara eksak.
//
// Perkaliannya bisa melewati batas int64 pada masukan yang tidak masuk akal (miliaran
// token dikali harga selangit), jadi dikerjakan dengan math/big lalu diperiksa: lebih
// baik menolak dengan jelas daripada menyimpan biaya yang meluap menjadi angka negatif.
func costOf(tokens int64, perMTok USD) (USD, error) {
	switch {
	case tokens < 0:
		return 0, fmt.Errorf("%w: jumlah token negatif (%d)", ErrInvalidAmount, tokens)
	case perMTok < 0:
		return 0, fmt.Errorf("%w: harga negatif (%s)", ErrInvalidAmount, perMTok.Decimal())
	case tokens == 0 || perMTok == 0:
		return 0, nil
	}

	// Pembulatan setengah ke atas: tambahkan separuh pembagi sebelum membagi. Kedua nilai
	// tidak negatif, jadi pembagian yang memangkas sama dengan pembulatan ke bawah.
	total := new(big.Int).Mul(big.NewInt(tokens), big.NewInt(int64(perMTok)))
	total.Add(total, big.NewInt(tokensPerMTok/2))
	total.Quo(total, big.NewInt(tokensPerMTok))

	if !total.IsInt64() {
		return 0, fmt.Errorf("%w: biaya %d token dengan harga %s di luar rentang yang bisa disimpan",
			ErrInvalidAmount, tokens, perMTok.Decimal())
	}
	return USD(total.Int64()), nil
}
