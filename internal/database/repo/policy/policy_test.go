package policy

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/NexGen-X/Route-X/internal/database"
	"github.com/NexGen-X/Route-X/internal/database/repo"
	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
)

// schemaPrefix menandai schema sekali pakai milik paket ini, sehingga sisa schema yang
// tertinggal bisa dikenali dan dilaporkan tanpa mengganggu paket lain yang memakai
// database yang sama.
const schemaPrefix = "test_policy_"

// TestMain menjalankan seluruh test lalu memastikan tidak ada schema sekali pakai yang
// tertinggal.
//
// Pemeriksaan ini ada karena test integrasi di sini membuat schema di database
// development: satu pembersihan yang terlewat akan menumpuk tanpa terlihat sampai
// database penuh dengan puluhan schema mati.
//
// Yang ditemukan hanya dilaporkan, tidak dihapus. Kalau paket ini kebetulan dijalankan
// dua kali bersamaan pada database yang sama, schema milik proses lain akan ikut terlihat
// di sini — dan menghapusnya berarti mematikan test yang sedang berjalan.
func TestMain(m *testing.M) {
	code := m.Run()

	if !testing.Short() {
		if left := leftoverSchemas(); len(left) > 0 {
			fmt.Fprintf(os.Stderr, "schema test tertinggal: %s\n", strings.Join(left, ", "))
			if code == 0 {
				code = 1
			}
		}
	}
	os.Exit(code)
}

// leftoverSchemas mengembalikan schema milik paket ini yang masih ada di database.
func leftoverSchemas() []string {
	dsn := ""
	for _, key := range []string{"TEST_DATABASE_URL", "DATABASE_URL"} {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			dsn = v
			break
		}
	}
	if dsn == "" {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil
	}
	defer pool.Close()

	rows, err := pool.Query(ctx,
		`select schema_name from information_schema.schemata where schema_name like $1`,
		schemaPrefix+"%")
	if err != nil {
		return nil
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return out
		}
		out = append(out, name)
	}
	return out
}

// --- Perkakas test integrasi -------------------------------------------------

// discardLogger membuang seluruh log supaya keluaran test tetap bersih.
func discardLogger() *slog.Logger { return slog.New(slog.DiscardHandler) }

// testDSN mengembalikan DSN untuk test integrasi, atau melewati test kalau tidak ada.
//
// TEST_DATABASE_URL diutamakan supaya CI bisa diarahkan ke database sekali pakai tanpa
// menyentuh database development.
func testDSN(t *testing.T) string {
	t.Helper()
	for _, key := range []string{"TEST_DATABASE_URL", "DATABASE_URL"} {
		dsn := strings.TrimSpace(os.Getenv(key))
		if dsn == "" {
			continue
		}
		if !strings.HasPrefix(dsn, "postgres://") && !strings.HasPrefix(dsn, "postgresql://") {
			t.Skipf("%s bukan URL postgres:// sehingga search_path test tidak bisa dipasang", key)
		}
		return dsn
	}
	t.Skip("TEST_DATABASE_URL maupun DATABASE_URL tidak diset — test integrasi dilewati")
	return ""
}

// newTestPool menyiapkan schema sekali pakai bermigrasi lengkap dan mengembalikan pool
// yang search_path-nya sudah mengarah ke sana.
//
// Seluruh tabel dibuat di dalam schema itu dan schema-nya dihapus di t.Cleanup, jadi test
// tidak pernah menyentuh schema public maupun data development. search_path dikirim
// sebagai parameter DSN karena pgx meneruskan parameter tak dikenal sebagai runtime
// parameter, sehingga setiap koneksi yang dibuka pool ikut terarahkan.
func newTestPool(ctx context.Context, t *testing.T) *pgxpool.Pool {
	t.Helper()
	baseDSN := testDSN(t)

	admin, err := pgxpool.New(ctx, baseDSN)
	if err != nil {
		t.Fatalf("membuka koneksi admin: %v", err)
	}
	t.Cleanup(admin.Close)

	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		t.Fatalf("membuat nama schema acak: %v", err)
	}
	schema := schemaPrefix + hex.EncodeToString(buf)
	ident := pgx.Identifier{schema}.Sanitize()

	if _, err := admin.Exec(ctx, "create schema "+ident); err != nil {
		t.Fatalf("membuat schema %s: %v", schema, err)
	}
	t.Cleanup(func() {
		// Context baru: ctx test bisa sudah kedaluwarsa saat pembersihan jalan, dan schema
		// yang tertinggal akan mengotori database development.
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if _, err := admin.Exec(cleanupCtx, "drop schema "+ident+" cascade"); err != nil {
			t.Errorf("membersihkan schema %s: %v", schema, err)
		}
	})

	u, err := url.Parse(baseDSN)
	if err != nil {
		t.Fatalf("DSN test tidak bisa diparse: %v", err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()

	pool, err := pgxpool.New(ctx, u.String())
	if err != nil {
		t.Fatalf("membuka pool test: %v", err)
	}
	t.Cleanup(pool.Close)

	if _, err := database.Migrate(ctx, &database.DB{Pool: pool}, discardLogger()); err != nil {
		t.Fatalf("menjalankan migrasi ke schema %s: %v", schema, err)
	}
	return pool
}

// --- Perkakas khusus paket ini -----------------------------------------------

// newRepo menyiapkan schema bermigrasi lengkap dan mengembalikan repository di atasnya.
func newRepo(t *testing.T) (context.Context, *Repo) {
	t.Helper()
	ctx := context.Background()
	return ctx, New(newTestPool(ctx, t))
}

// pointer mengembalikan pointer ke nilai apa pun, untuk mengisi kolom opsional.
func pointer[T any](v T) *T { return &v }

// idAcak membuat UUID acak untuk dipakai sebagai scope_id yang tidak menunjuk ke mana pun.
//
// Kolom scope_id sengaja bukan foreign key di skema — cakupan 'ip' menyimpan alamat, bukan
// UUID — jadi nilai yang tidak menunjuk baris mana pun tetap sah dan tidak perlu tabel lain.
func idAcak(t *testing.T) string {
	t.Helper()
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		t.Fatalf("membuat UUID acak: %v", err)
	}
	h := hex.EncodeToString(b)
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32]
}

// --- Batas laju --------------------------------------------------------------

func TestActiveRateLimitsHanyaYangAktifDanGlobalDiDepan(t *testing.T) {
	ctx, r := newRepo(t)
	kunci := idAcak(t)

	if _, err := r.CreateRateLimit(ctx, CreateRateLimitParams{
		Scope: ScopeAPIKey, ScopeID: kunci, RequestsPerMinute: pointer(60),
	}); err != nil {
		t.Fatalf("CreateRateLimit cakupan key: %v", err)
	}
	global, err := r.CreateRateLimit(ctx, CreateRateLimitParams{
		Scope: ScopeGlobal, RequestsPerSecond: pointer(100),
	})
	if err != nil {
		t.Fatalf("CreateRateLimit cakupan global: %v", err)
	}
	mati, err := r.CreateRateLimit(ctx, CreateRateLimitParams{
		Scope: ScopeUser, ScopeID: idAcak(t), DailyRequestLimit: pointer(int64(10)),
	})
	if err != nil {
		t.Fatalf("CreateRateLimit cakupan user: %v", err)
	}
	if _, err := r.SetRateLimitEnabled(ctx, mati.ID, false); err != nil {
		t.Fatalf("SetRateLimitEnabled: %v", err)
	}

	aktif, err := r.ActiveRateLimits(ctx)
	if err != nil {
		t.Fatalf("ActiveRateLimits: %v", err)
	}
	if len(aktif) != 2 {
		t.Fatalf("jumlah batas aktif = %d, mau 2 (yang dimatikan harus tersaring)", len(aktif))
	}
	// Global lebih dulu supaya penggabungan batas selalu menghasilkan hasil yang sama.
	if aktif[0].ID != global.ID || aktif[0].Scope != ScopeGlobal {
		t.Errorf("baris pertama = %s/%s, mau cakupan global", aktif[0].Scope, aktif[0].ID)
	}
	if aktif[1].Scope != ScopeAPIKey || aktif[1].ScopeID != kunci {
		t.Errorf("baris kedua = %s/%s", aktif[1].Scope, aktif[1].ScopeID)
	}
	// NULL harus tetap NULL sampai ke pemakainya: nol pada kolom batas berarti "tidak boleh
	// satu pun permintaan", yang sangat berbeda dari "tidak dibatasi".
	if aktif[1].RequestsPerMinute == nil || *aktif[1].RequestsPerMinute != 60 {
		t.Errorf("RequestsPerMinute = %v, mau 60", aktif[1].RequestsPerMinute)
	}
	if aktif[1].RequestsPerSecond != nil {
		t.Errorf("RequestsPerSecond = %v, mau nil (kolomnya NULL)", *aktif[1].RequestsPerSecond)
	}
}

func TestCreateRateLimitDitolakDatabaseSaatBentuknyaSalah(t *testing.T) {
	ctx, r := newRepo(t)

	for nama, p := range map[string]CreateRateLimitParams{
		"cakupan tak dikenal":       {Scope: "galaksi", ScopeID: idAcak(t), RequestsPerMinute: pointer(1)},
		"non-global tanpa scope_id": {Scope: ScopeAPIKey, RequestsPerMinute: pointer(1)},
		"global dengan scope_id":    {Scope: ScopeGlobal, ScopeID: idAcak(t), RequestsPerMinute: pointer(1)},
		"tanpa satu pun batas":      {Scope: ScopeGlobal},
	} {
		t.Run(nama, func(t *testing.T) {
			if _, err := r.CreateRateLimit(ctx, p); err == nil {
				t.Error("baris tidak sah diterima, mau ditolak constraint tabel")
			}
		})
	}
}

func TestDeleteRateLimit(t *testing.T) {
	ctx, r := newRepo(t)
	l, err := r.CreateRateLimit(ctx, CreateRateLimitParams{Scope: ScopeGlobal, RequestsPerMinute: pointer(5)})
	if err != nil {
		t.Fatalf("CreateRateLimit: %v", err)
	}
	if err := r.DeleteRateLimit(ctx, l.ID); err != nil {
		t.Fatalf("DeleteRateLimit: %v", err)
	}
	if err := r.DeleteRateLimit(ctx, l.ID); err == nil {
		t.Error("penghapusan kedua berhasil, mau ErrNotFound")
	}
	if err := r.DeleteRateLimit(ctx, "bukan-uuid"); err == nil {
		t.Error("ID salah bentuk diterima, mau ErrNotFound tanpa menyentuh database")
	}
}

// --- Anggaran ----------------------------------------------------------------

func TestBudgetNilaiUangBolakBalikTanpaKehilangan(t *testing.T) {
	ctx, r := newRepo(t)

	// Skala 6 adalah yang bisa disimpan kolomnya tepat; nilai ini memakai seluruhnya.
	batas := upstream.MustParseUSD("1234.567891")
	b, err := r.CreateBudget(ctx, CreateBudgetParams{
		Name: "anggaran uji", Scope: ScopeGlobal, Period: PeriodMonthly, LimitUSD: batas,
	})
	if err != nil {
		t.Fatalf("CreateBudget: %v", err)
	}
	if b.LimitUSD != batas {
		t.Errorf("LimitUSD = %s, mau %s", b.LimitUSD, batas)
	}
	if b.SpentUSD != 0 {
		t.Errorf("SpentUSD = %s, mau 0", b.SpentUSD)
	}
	if b.ActionOnExceed != ActionBlock || b.AlertThresholdPct != 80 {
		t.Errorf("bawaan action/threshold = %s/%d", b.ActionOnExceed, b.AlertThresholdPct)
	}
	if b.PeriodStart.IsZero() {
		t.Error("PeriodStart kosong, mau diisi now() oleh database")
	}
}

func TestBudgetPenilaianTerlampauiDanBlokir(t *testing.T) {
	for _, tc := range []struct {
		nama       string
		limit      string
		spent      string
		action     string
		enabled    bool
		mauLampaui bool
		mauBlokir  bool
	}{
		// Tepat di batas sudah terlampaui: meloloskan satu permintaan lagi di titik ini
		// membuat setiap anggaran berlaku "batas ditambah satu permintaan".
		{"tepat di batas", "10", "10", ActionBlock, true, true, true},
		{"di bawah batas", "10", "9.999999", ActionBlock, true, false, false},
		{"di atas batas", "10", "10.000001", ActionBlock, true, true, true},
		// warn memang dimaksudkan memberitahu tanpa menghentikan lalu lintas.
		{"terlampaui tapi warn", "10", "20", ActionWarn, true, true, false},
		{"terlampaui tapi mati", "10", "20", ActionBlock, false, true, false},
	} {
		t.Run(tc.nama, func(t *testing.T) {
			b := &Budget{
				LimitUSD:       upstream.MustParseUSD(tc.limit),
				SpentUSD:       upstream.MustParseUSD(tc.spent),
				ActionOnExceed: tc.action,
				Enabled:        tc.enabled,
			}
			if got := b.Exceeded(); got != tc.mauLampaui {
				t.Errorf("Exceeded() = %v, mau %v", got, tc.mauLampaui)
			}
			if got := b.Blocks(); got != tc.mauBlokir {
				t.Errorf("Blocks() = %v, mau %v", got, tc.mauBlokir)
			}
		})
	}
}

func TestBudgetShouldAlert(t *testing.T) {
	buat := func(limit, spent string, pct int, alerted bool) *Budget {
		b := &Budget{
			LimitUSD:          upstream.MustParseUSD(limit),
			SpentUSD:          upstream.MustParseUSD(spent),
			AlertThresholdPct: pct,
			Enabled:           true,
		}
		if alerted {
			b.AlertedAt = pointer(time.Now())
		}
		return b
	}
	for _, tc := range []struct {
		nama string
		b    *Budget
		mau  bool
	}{
		{"di bawah ambang", buat("100", "79.999999", 80, false), false},
		{"tepat di ambang", buat("100", "80", 80, false), true},
		{"di atas ambang", buat("100", "95", 80, false), true},
		{"sudah diberitahu", buat("100", "95", 80, true), false},
		{"ambang 1 persen", buat("100", "1", 1, false), true},
	} {
		t.Run(tc.nama, func(t *testing.T) {
			if got := tc.b.ShouldAlert(); got != tc.mau {
				t.Errorf("ShouldAlert() = %v, mau %v", got, tc.mau)
			}
		})
	}
}

// TestAddSpendMengenaiSeluruhCakupanSekaligus menjaga bentuk yang menentukan biaya jalur
// request: satu permintaan dikenai beberapa anggaran, dan satu UPDATE harus mengenai
// semuanya.
func TestAddSpendMengenaiSeluruhCakupanSekaligus(t *testing.T) {
	ctx, r := newRepo(t)
	kunci, pengguna, lain := idAcak(t), idAcak(t), idAcak(t)

	global, err := r.CreateBudget(ctx, CreateBudgetParams{
		Name: "global", Scope: ScopeGlobal, Period: PeriodMonthly, LimitUSD: upstream.MustParseUSD("100"),
	})
	if err != nil {
		t.Fatalf("CreateBudget global: %v", err)
	}
	perKunci, err := r.CreateBudget(ctx, CreateBudgetParams{
		Name: "per key", Scope: ScopeAPIKey, ScopeID: kunci, Period: PeriodDaily,
		LimitUSD: upstream.MustParseUSD("10"),
	})
	if err != nil {
		t.Fatalf("CreateBudget per key: %v", err)
	}
	perPenggunaLain, err := r.CreateBudget(ctx, CreateBudgetParams{
		Name: "pengguna lain", Scope: ScopeUser, ScopeID: lain, Period: PeriodDaily,
		LimitUSD: upstream.MustParseUSD("10"),
	})
	if err != nil {
		t.Fatalf("CreateBudget pengguna lain: %v", err)
	}

	n, err := r.AddSpend(ctx, []Target{
		{Scope: ScopeAPIKey, ID: kunci},
		{Scope: ScopeUser, ID: pengguna},
	}, upstream.MustParseUSD("2.5"))
	if err != nil {
		t.Fatalf("AddSpend: %v", err)
	}
	if n != 2 {
		t.Errorf("baris tersentuh = %d, mau 2 (global + per key)", n)
	}

	sesudah, err := r.ActiveBudgets(ctx)
	if err != nil {
		t.Fatalf("ActiveBudgets: %v", err)
	}
	spent := map[string]upstream.USD{}
	for _, b := range sesudah {
		spent[b.ID] = b.SpentUSD
	}
	if got := spent[global.ID]; got != upstream.MustParseUSD("2.5") {
		t.Errorf("anggaran global terpakai %s, mau 2.5", got)
	}
	if got := spent[perKunci.ID]; got != upstream.MustParseUSD("2.5") {
		t.Errorf("anggaran per key terpakai %s, mau 2.5", got)
	}
	// Anggaran milik pengguna LAIN tidak boleh ikut bertambah — kalau ikut, satu penyewa
	// menghabiskan anggaran penyewa lain tanpa pernah memakai kuotanya.
	if got := spent[perPenggunaLain.ID]; got != 0 {
		t.Errorf("anggaran pengguna lain terpakai %s, mau 0", got)
	}
}

func TestAddSpendNolTidakMenyentuhApaPun(t *testing.T) {
	ctx, r := newRepo(t)
	if _, err := r.CreateBudget(ctx, CreateBudgetParams{
		Name: "global", Scope: ScopeGlobal, Period: PeriodTotal, LimitUSD: upstream.MustParseUSD("1"),
	}); err != nil {
		t.Fatalf("CreateBudget: %v", err)
	}
	n, err := r.AddSpend(ctx, []Target{{Scope: ScopeGlobal}}, 0)
	if err != nil {
		t.Fatalf("AddSpend: %v", err)
	}
	if n != 0 {
		t.Errorf("baris tersentuh = %d, mau 0", n)
	}
}

// TestActiveBudgetsMengabaikanPeriodeYangSudahBerakhir: menegakkan anggaran dengan
// period_end yang sudah lewat berarti memblokir lalu lintas atas pemakaian periode lalu.
func TestActiveBudgetsMengabaikanPeriodeYangSudahBerakhir(t *testing.T) {
	ctx, r := newRepo(t)
	lewat := time.Now().Add(-time.Hour)
	if _, err := r.CreateBudget(ctx, CreateBudgetParams{
		Name: "sudah lewat", Scope: ScopeGlobal, Period: PeriodDaily,
		LimitUSD: upstream.MustParseUSD("1"), PeriodEnd: &lewat,
	}); err != nil {
		t.Fatalf("CreateBudget: %v", err)
	}
	aktif, err := r.ActiveBudgets(ctx)
	if err != nil {
		t.Fatalf("ActiveBudgets: %v", err)
	}
	if len(aktif) != 0 {
		t.Errorf("jumlah anggaran aktif = %d, mau 0", len(aktif))
	}
}

// TestResetPeriodMelepasPenandaPeringatan: tanpa ini, anggaran yang pernah memperingatkan
// tidak akan pernah memperingatkan lagi, dan kegagalannya berupa notifikasi yang tidak
// datang — hal yang tidak terlihat sampai tagihan sudah lewat batas.
func TestResetPeriodMelepasPenandaPeringatan(t *testing.T) {
	ctx, r := newRepo(t)
	b, err := r.CreateBudget(ctx, CreateBudgetParams{
		Name: "bulanan", Scope: ScopeGlobal, Period: PeriodMonthly, LimitUSD: upstream.MustParseUSD("10"),
	})
	if err != nil {
		t.Fatalf("CreateBudget: %v", err)
	}
	if _, err := r.AddSpend(ctx, nil, upstream.MustParseUSD("9")); err != nil {
		t.Fatalf("AddSpend: %v", err)
	}
	pertama, err := r.MarkAlerted(ctx, b.ID)
	if err != nil {
		t.Fatalf("MarkAlerted: %v", err)
	}
	if !pertama {
		t.Fatal("MarkAlerted pertama = false, mau true")
	}
	// Dua instance yang memeriksa ambang bersamaan hanya boleh menghasilkan satu notifikasi.
	kedua, err := r.MarkAlerted(ctx, b.ID)
	if err != nil {
		t.Fatalf("MarkAlerted kedua: %v", err)
	}
	if kedua {
		t.Error("MarkAlerted kedua = true, mau false")
	}

	mulai := time.Now()
	setelah, err := r.ResetPeriod(ctx, b.ID, mulai, nil)
	if err != nil {
		t.Fatalf("ResetPeriod: %v", err)
	}
	if setelah.SpentUSD != 0 {
		t.Errorf("SpentUSD setelah reset = %s, mau 0", setelah.SpentUSD)
	}
	if setelah.AlertedAt != nil {
		t.Errorf("AlertedAt setelah reset = %v, mau nil", setelah.AlertedAt)
	}
}

// --- Blokir ------------------------------------------------------------------

func TestBanSiklusHidup(t *testing.T) {
	ctx, r := newRepo(t)
	subjek := idAcak(t)

	b, err := r.CreateBan(ctx, CreateBanParams{
		SubjectKind: BanSubjectAPIKey, Subject: subjek, Reason: "penyalahgunaan",
	})
	if err != nil {
		t.Fatalf("CreateBan: %v", err)
	}
	if !b.Active(time.Now()) {
		t.Error("blokir baru tidak aktif")
	}

	ditemukan, err := r.BanFor(ctx, BanSubjectAPIKey, subjek)
	if err != nil {
		t.Fatalf("BanFor: %v", err)
	}
	if ditemukan.ID != b.ID {
		t.Errorf("BanFor mengembalikan %s, mau %s", ditemukan.ID, b.ID)
	}

	dicabut, err := r.Lift(ctx, b.ID, "")
	if err != nil {
		t.Fatalf("Lift: %v", err)
	}
	if dicabut.LiftedAt == nil {
		t.Error("LiftedAt masih nil setelah Lift")
	}
	// Pencabutan kedua tidak boleh memindahkan waktu pencabutan yang sudah tercatat.
	if _, err := r.Lift(ctx, b.ID, ""); err == nil {
		t.Error("pencabutan kedua berhasil, mau ErrNotFound")
	}
	if _, err := r.BanFor(ctx, BanSubjectAPIKey, subjek); err == nil {
		t.Error("BanFor masih menemukan blokir yang sudah dicabut")
	}
}

func TestActiveBansMenyaringYangDicabutDanKedaluwarsa(t *testing.T) {
	ctx, r := newRepo(t)
	lewat := time.Now().Add(-time.Minute)
	depan := time.Now().Add(time.Hour)

	berlaku, err := r.CreateBan(ctx, CreateBanParams{
		SubjectKind: BanSubjectIP, Subject: "203.0.113.7", Reason: "pemindaian", ExpiresAt: &depan,
	})
	if err != nil {
		t.Fatalf("CreateBan berlaku: %v", err)
	}
	if _, err := r.CreateBan(ctx, CreateBanParams{
		SubjectKind: BanSubjectIP, Subject: "203.0.113.8", Reason: "kedaluwarsa", ExpiresAt: &lewat,
	}); err != nil {
		t.Fatalf("CreateBan kedaluwarsa: %v", err)
	}
	dicabut, err := r.CreateBan(ctx, CreateBanParams{
		SubjectKind: BanSubjectUser, Subject: idAcak(t), Reason: "salah blokir",
	})
	if err != nil {
		t.Fatalf("CreateBan untuk dicabut: %v", err)
	}
	if _, err := r.Lift(ctx, dicabut.ID, ""); err != nil {
		t.Fatalf("Lift: %v", err)
	}

	aktif, err := r.ActiveBans(ctx, repo.Page{Limit: 50})
	if err != nil {
		t.Fatalf("ActiveBans: %v", err)
	}
	if len(aktif) != 1 || aktif[0].ID != berlaku.ID {
		t.Fatalf("blokir aktif = %d baris, mau hanya yang masih berlaku", len(aktif))
	}
}

// TestPurgeExpiredMenyisakanYangDicabutManual: pencabutan manual adalah keputusan orang dan
// jejaknya bernilai; blokir sementara yang habis sendiri adalah kebisingan yang menumpuk.
func TestPurgeExpiredMenyisakanYangDicabutManual(t *testing.T) {
	ctx, r := newRepo(t)
	lama := time.Now().Add(-48 * time.Hour)

	if _, err := r.CreateBan(ctx, CreateBanParams{
		SubjectKind: BanSubjectIP, Subject: "203.0.113.9", Reason: "lama", ExpiresAt: &lama,
	}); err != nil {
		t.Fatalf("CreateBan kedaluwarsa lama: %v", err)
	}
	dicabut, err := r.CreateBan(ctx, CreateBanParams{
		SubjectKind: BanSubjectUser, Subject: idAcak(t), Reason: "dicabut",
	})
	if err != nil {
		t.Fatalf("CreateBan: %v", err)
	}
	if _, err := r.Lift(ctx, dicabut.ID, ""); err != nil {
		t.Fatalf("Lift: %v", err)
	}

	n, err := r.PurgeExpired(ctx, 24*time.Hour)
	if err != nil {
		t.Fatalf("PurgeExpired: %v", err)
	}
	if n != 1 {
		t.Errorf("baris terhapus = %d, mau 1", n)
	}
	if _, err := r.Lift(ctx, dicabut.ID, ""); err == nil {
		t.Error("blokir yang sudah dicabut ikut terhapus, mau tetap ada sebagai jejak")
	}
}

// --- Penyaring konten --------------------------------------------------------

func TestActiveFiltersUrutanEvaluasi(t *testing.T) {
	ctx, r := newRepo(t)

	// Priority sengaja dibuat tidak berurutan saat dimasukkan, dan dua di antaranya sama
	// supaya pemutus serinya ikut teruji.
	for _, p := range []CreateFilterParams{
		{Name: "z-terakhir", Kind: FilterBlockedPattern, Pattern: "z", PatternType: PatternSubstring, Priority: 300},
		{Name: "b-sama", Kind: FilterBlockedPattern, Pattern: "b", PatternType: PatternSubstring, Priority: 100},
		{Name: "a-sama", Kind: FilterBlockedPattern, Pattern: "a", PatternType: PatternSubstring, Priority: 100},
		{Name: "paling-dulu", Kind: FilterRequestSize, MaxRequestBytes: pointer(int64(1024)), Priority: 10},
	} {
		if _, err := r.CreateFilter(ctx, p); err != nil {
			t.Fatalf("CreateFilter %s: %v", p.Name, err)
		}
	}

	aktif, err := r.ActiveFilters(ctx)
	if err != nil {
		t.Fatalf("ActiveFilters: %v", err)
	}
	mau := []string{"paling-dulu", "a-sama", "b-sama", "z-terakhir"}
	if len(aktif) != len(mau) {
		t.Fatalf("jumlah penyaring = %d, mau %d", len(aktif), len(mau))
	}
	for i := range mau {
		if aktif[i].Name != mau[i] {
			t.Errorf("urutan[%d] = %q, mau %q", i, aktif[i].Name, mau[i])
		}
	}
	// Bawaan kolom harus ikut terbaca, bukan nol Go.
	if aktif[1].MaxEvalMS != 50 {
		t.Errorf("MaxEvalMS = %d, mau bawaan 50", aktif[1].MaxEvalMS)
	}
	if aktif[1].AppliesTo != AppliesToRequest || aktif[1].Action != ActionBlock {
		t.Errorf("bawaan applies_to/action = %s/%s", aktif[1].AppliesTo, aktif[1].Action)
	}
}

// TestCreateFilterBentukDijagaDatabase memastikan aturan yang kolom pentingnya kosong tidak
// bisa masuk. Aturan seperti itu tidak melakukan apa pun sementara operator menyangka
// perlindungannya aktif — kegagalan paling mahal yang bisa dimiliki tabel ini.
func TestCreateFilterBentukDijagaDatabase(t *testing.T) {
	ctx, r := newRepo(t)

	for nama, p := range map[string]CreateFilterParams{
		"pola tanpa pattern":         {Name: "a", Kind: FilterBlockedPattern, PatternType: PatternSubstring},
		"pola tanpa pattern_type":    {Name: "b", Kind: FilterBlockedPattern, Pattern: "x"},
		"request_size tanpa batas":   {Name: "c", Kind: FilterRequestSize},
		"model_restriction tanpa id": {Name: "d", Kind: FilterModelRestriction},
		"kind tak dikenal":           {Name: "e", Kind: "telepati", Pattern: "x", PatternType: PatternRegex},
	} {
		t.Run(nama, func(t *testing.T) {
			if _, err := r.CreateFilter(ctx, p); err == nil {
				t.Error("penyaring tidak sah diterima, mau ditolak constraint tabel")
			}
		})
	}
}

// TestRecordEvalTimeout menjaga satu-satunya umpan balik yang dimiliki operator tentang pola
// yang terlalu mahal. Tanpa penghitung ini, aturan yang selalu kehabisan waktu tampak sama
// dengan aturan yang tidak pernah cocok.
func TestRecordEvalTimeout(t *testing.T) {
	ctx, r := newRepo(t)
	f, err := r.CreateFilter(ctx, CreateFilterParams{
		Name: "pola mahal", Kind: FilterBlockedPattern,
		Pattern: "(a+)+b", PatternType: PatternRegex, MaxEvalMS: 5,
	})
	if err != nil {
		t.Fatalf("CreateFilter: %v", err)
	}

	for i := 0; i < 3; i++ {
		if err := r.RecordEvalTimeout(ctx, f.ID); err != nil {
			t.Fatalf("RecordEvalTimeout ke-%d: %v", i+1, err)
		}
	}

	stats, err := r.TimingOutFilters(ctx, repo.Page{Limit: 10})
	if err != nil {
		t.Fatalf("TimingOutFilters: %v", err)
	}
	if len(stats) != 1 {
		t.Fatalf("jumlah penyaring bermasalah = %d, mau 1", len(stats))
	}
	if stats[0].EvalTimeoutCount != 3 {
		t.Errorf("EvalTimeoutCount = %d, mau 3", stats[0].EvalTimeoutCount)
	}
	if stats[0].LastTimeoutAt == nil {
		t.Error("LastTimeoutAt masih nil")
	}
}

func TestFilterDimatikanTidakIkutAktif(t *testing.T) {
	ctx, r := newRepo(t)
	f, err := r.CreateFilter(ctx, CreateFilterParams{
		Name: "sementara", Kind: FilterBlockedPattern, Pattern: "x", PatternType: PatternSubstring,
	})
	if err != nil {
		t.Fatalf("CreateFilter: %v", err)
	}
	if _, err := r.SetFilterEnabled(ctx, f.ID, false); err != nil {
		t.Fatalf("SetFilterEnabled: %v", err)
	}
	aktif, err := r.ActiveFilters(ctx)
	if err != nil {
		t.Fatalf("ActiveFilters: %v", err)
	}
	if len(aktif) != 0 {
		t.Errorf("jumlah penyaring aktif = %d, mau 0", len(aktif))
	}
	if err := r.DeleteFilter(ctx, f.ID); err != nil {
		t.Fatalf("DeleteFilter: %v", err)
	}
}
