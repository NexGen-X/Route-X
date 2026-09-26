package ratelimit

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/NexGen-X/Route-X/internal/apikey"
	"github.com/NexGen-X/Route-X/internal/database/repo/policy"
)

// sumber adalah Source tiruan yang menghitung pembacaan.
type sumber struct {
	mu     sync.Mutex
	rows   []*policy.RateLimit
	err    error
	dibaca int
}

func (s *sumber) ActiveRateLimits(context.Context) ([]*policy.RateLimit, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dibaca++
	if s.err != nil {
		return nil, s.err
	}
	return s.rows, nil
}

func (s *sumber) ganti(rows []*policy.RateLimit, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rows, s.err = rows, err
}

func (s *sumber) jumlahDibaca() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.dibaca
}

func pointer[T any](v T) *T { return &v }

// baris menyusun satu baris rate_limits secukupnya untuk test.
func baris(scope, id string, ubah func(*policy.RateLimit)) *policy.RateLimit {
	r := &policy.RateLimit{Scope: scope, ScopeID: id, Enabled: true}
	if ubah != nil {
		ubah(r)
	}
	return r
}

// --- Cakupan sebagai ember terpisah ------------------------------------------

// TestSetiapCakupanEmberSendiri menjaga sifat yang menentukan seluruh gunanya tabel ini:
// batas per key dan batas per pengguna ditegakkan BERSAMAAN, bukan digabungkan. Satu
// pengguna bisa punya sepuluh key masing-masing 60 rpm sementara totalnya tetap 100 rpm.
func TestSetiapCakupanEmberSendiri(t *testing.T) {
	kunci, pengguna := "key-1", "user-1"
	src := &sumber{rows: []*policy.RateLimit{
		baris(policy.ScopeGlobal, "", func(r *policy.RateLimit) { r.RequestsPerSecond = pointer(500) }),
		baris(policy.ScopeAPIKey, kunci, func(r *policy.RateLimit) { r.RequestsPerMinute = pointer(60) }),
		baris(policy.ScopeUser, pengguna, func(r *policy.RateLimit) { r.RequestsPerMinute = pointer(100) }),
	}}
	e := NewEngine(src, nil)

	got := e.Requests(context.Background(), []policy.Target{
		{Scope: policy.ScopeAPIKey, ID: kunci},
		{Scope: policy.ScopeUser, ID: pengguna},
		{Scope: policy.ScopeGlobal},
	}, apikey.Limits{})

	if len(got) != 3 {
		t.Fatalf("jumlah cakupan = %d, mau 3: %+v", len(got), got)
	}
	// Urutan hasil mengikuti urutan target, karena itu yang menentukan batas mana yang
	// dilaporkan lewat header X-RateLimit-*.
	if got[0].Scope != apikey.ScopeAPIKey || got[0].ID != kunci || got[0].Limits.RPM != 60 {
		t.Errorf("cakupan[0] = %+v", got[0])
	}
	if got[1].Scope != apikey.ScopeUser || got[1].ID != pengguna || got[1].Limits.RPM != 100 {
		t.Errorf("cakupan[1] = %+v", got[1])
	}
	// Cakupan global memakai pengenal tetap: kunci Redis butuh bagian pengenal, dan bagian
	// kosong menghasilkan dua pemisah berurutan yang menyulitkan pembacaan.
	if got[2].Scope != apikey.ScopeGlobal || got[2].ID != apikey.GlobalID || got[2].Limits.RPS != 500 {
		t.Errorf("cakupan[2] = %+v", got[2])
	}
	// Batas per key TIDAK boleh tercampur batas pengguna: 60 dan 100 harus tetap terpisah.
	if got[0].Limits.RPM == got[1].Limits.RPM {
		t.Error("batas key dan batas pengguna tercampur")
	}
}

func TestCakupanTanpaBatasDilewati(t *testing.T) {
	src := &sumber{rows: []*policy.RateLimit{
		baris(policy.ScopeAPIKey, "key-1", func(r *policy.RateLimit) { r.RequestsPerMinute = pointer(60) }),
	}}
	e := NewEngine(src, nil)

	got := e.Requests(context.Background(), []policy.Target{
		{Scope: policy.ScopeAPIKey, ID: "key-1"},
		// Tidak ada baris untuk pengguna ini, dan key-nya tanpa kolom batas: tidak ada yang
		// perlu diperiksa, jadi cakupannya tidak boleh ikut ke Redis sama sekali.
		{Scope: policy.ScopeUser, ID: "user-tanpa-batas"},
		{Scope: policy.ScopeModel, ID: "model-tanpa-batas"},
	}, apikey.Limits{})

	if len(got) != 1 {
		t.Fatalf("jumlah cakupan = %d, mau 1: %+v", len(got), got)
	}
}

func TestCakupanTakDikenalDiabaikan(t *testing.T) {
	e := NewEngine(&sumber{}, nil)
	got := e.Requests(context.Background(), []policy.Target{{Scope: "galaksi", ID: "x"}}, apikey.Limits{RPM: 10})
	if len(got) != 0 {
		t.Errorf("cakupan tak dikenal menghasilkan %+v, mau kosong", got)
	}
}

// --- Penggabungan ------------------------------------------------------------

// TestKolomKeyMenangPerField: kolom di api_keys adalah angka yang ditampilkan dashboard
// pada key itu, jadi operator yang mengisinya berharap itulah yang berlaku. Baris
// rate_limits mengisi yang dibiarkan kosong, tidak menimpanya.
func TestKolomKeyMenangPerField(t *testing.T) {
	src := &sumber{rows: []*policy.RateLimit{
		baris(policy.ScopeAPIKey, "key-1", func(r *policy.RateLimit) {
			r.RequestsPerSecond = pointer(5)
			r.RequestsPerMinute = pointer(60)
			r.DailyRequestLimit = pointer(int64(1000))
		}),
	}}
	e := NewEngine(src, nil)

	got := e.Requests(context.Background(),
		[]policy.Target{{Scope: policy.ScopeAPIKey, ID: "key-1"}},
		apikey.Limits{RPM: 999})

	if len(got) != 1 {
		t.Fatalf("jumlah cakupan = %d, mau 1", len(got))
	}
	l := got[0].Limits
	if l.RPM != 999 {
		t.Errorf("RPM = %d, mau 999 dari kolom api_keys", l.RPM)
	}
	if l.RPS != 5 {
		t.Errorf("RPS = %d, mau 5 dari tabel (kolom api_keys kosong)", l.RPS)
	}
	if l.Daily != 1000 {
		t.Errorf("Daily = %d, mau 1000 dari tabel", l.Daily)
	}
}

// TestBarisGandaDigabungDenganUrutanTetap: skema tidak melarang dua baris untuk cakupan dan
// entitas yang sama, dan tanpa urutan tetap batas mana yang menang akan berpindah-pindah
// antar instance.
func TestBarisGandaDigabungDenganUrutanTetap(t *testing.T) {
	src := &sumber{rows: []*policy.RateLimit{
		baris(policy.ScopeModel, "gpt", func(r *policy.RateLimit) { r.RequestsPerMinute = pointer(10) }),
		baris(policy.ScopeModel, "gpt", func(r *policy.RateLimit) {
			r.RequestsPerMinute = pointer(999)
			r.RequestsPerSecond = pointer(3)
		}),
	}}
	e := NewEngine(src, nil)

	got := e.Requests(context.Background(), []policy.Target{{Scope: policy.ScopeModel, ID: "gpt"}}, apikey.Limits{})
	if len(got) != 1 {
		t.Fatalf("jumlah cakupan = %d, mau 1", len(got))
	}
	if got[0].Limits.RPM != 10 {
		t.Errorf("RPM = %d, mau 10 (baris pertama menang)", got[0].Limits.RPM)
	}
	if got[0].Limits.RPS != 3 {
		t.Errorf("RPS = %d, mau 3 (diisi baris kedua)", got[0].Limits.RPS)
	}
}

// --- Batas token -------------------------------------------------------------

func TestTokensMemakaiKolomTokenBaris(t *testing.T) {
	src := &sumber{rows: []*policy.RateLimit{
		baris(policy.ScopeGlobal, "", func(r *policy.RateLimit) {
			r.TokensPerMinute = pointer(100_000)
			r.MonthlyTokenLimit = pointer(int64(5_000_000))
		}),
		baris(policy.ScopeAPIKey, "key-1", func(r *policy.RateLimit) {
			r.DailyTokenLimit = pointer(int64(50_000))
		}),
	}}
	e := NewEngine(src, nil)

	got := e.Tokens(context.Background(), []policy.Target{
		{Scope: policy.ScopeAPIKey, ID: "key-1"},
		{Scope: policy.ScopeGlobal},
	}, apikey.TokenLimits{TPM: 7})

	if len(got) != 2 {
		t.Fatalf("jumlah cakupan token = %d, mau 2: %+v", len(got), got)
	}
	if got[0].Limits.TPM != 7 {
		t.Errorf("TPM cakupan key = %d, mau 7 dari kolom api_keys", got[0].Limits.TPM)
	}
	if got[0].Limits.Daily != 50_000 {
		t.Errorf("Daily cakupan key = %d, mau 50000", got[0].Limits.Daily)
	}
	if got[1].Limits.TPM != 100_000 || got[1].Limits.Monthly != 5_000_000 {
		t.Errorf("cakupan global = %+v", got[1].Limits)
	}
}

// --- Cache -------------------------------------------------------------------

func TestSalinanDipakaiUlangSelamaTTL(t *testing.T) {
	src := &sumber{rows: []*policy.RateLimit{
		baris(policy.ScopeGlobal, "", func(r *policy.RateLimit) { r.RequestsPerMinute = pointer(10) }),
	}}
	sekarang := time.Now()
	e := NewEngine(src, nil, WithTTL(5*time.Second), WithClock(func() time.Time { return sekarang }))

	target := []policy.Target{{Scope: policy.ScopeGlobal}}
	for i := 0; i < 5; i++ {
		e.Requests(context.Background(), target, apikey.Limits{})
	}
	if n := src.jumlahDibaca(); n != 1 {
		t.Errorf("pembacaan tabel = %d, mau 1 selama TTL belum habis", n)
	}

	sekarang = sekarang.Add(6 * time.Second)
	e.Requests(context.Background(), target, apikey.Limits{})
	if n := src.jumlahDibaca(); n != 2 {
		t.Errorf("pembacaan tabel = %d setelah TTL habis, mau 2", n)
	}
}

func TestInvalidateMemaksaPembacaanUlang(t *testing.T) {
	src := &sumber{rows: []*policy.RateLimit{
		baris(policy.ScopeGlobal, "", func(r *policy.RateLimit) { r.RequestsPerMinute = pointer(10) }),
	}}
	e := NewEngine(src, nil)
	target := []policy.Target{{Scope: policy.ScopeGlobal}}

	e.Requests(context.Background(), target, apikey.Limits{})
	e.Invalidate()
	e.Requests(context.Background(), target, apikey.Limits{})
	if n := src.jumlahDibaca(); n != 2 {
		t.Errorf("pembacaan tabel = %d, mau 2 setelah Invalidate", n)
	}
}

// TestPembacaanGagalMempertahankanSalinanLama menjaga dua hal sekaligus: satu tabel
// konfigurasi yang tidak terbaca tidak boleh mematikan penegakan yang sudah berjalan, dan
// kegagalan tidak boleh memperpanjang masa tanpa penegakan sampai satu TTL penuh.
func TestPembacaanGagalMempertahankanSalinanLama(t *testing.T) {
	src := &sumber{rows: []*policy.RateLimit{
		baris(policy.ScopeGlobal, "", func(r *policy.RateLimit) { r.RequestsPerMinute = pointer(10) }),
	}}
	sekarang := time.Now()
	e := NewEngine(src, nil, WithTTL(5*time.Second), WithClock(func() time.Time { return sekarang }))
	target := []policy.Target{{Scope: policy.ScopeGlobal}}

	if got := e.Requests(context.Background(), target, apikey.Limits{}); len(got) != 1 {
		t.Fatalf("pembacaan pertama gagal: %+v", got)
	}

	src.ganti(nil, errors.New("database sedang tidak bisa dibaca"))
	sekarang = sekarang.Add(6 * time.Second)

	got := e.Requests(context.Background(), target, apikey.Limits{})
	if len(got) != 1 || got[0].Limits.RPM != 10 {
		t.Errorf("setelah pembacaan gagal = %+v, mau salinan lama tetap dipakai", got)
	}
	// Percobaan berikutnya tidak menunggu TTL penuh: waktu muat tidak diperbarui saat gagal.
	e.Requests(context.Background(), target, apikey.Limits{})
	if n := src.jumlahDibaca(); n != 3 {
		t.Errorf("pembacaan tabel = %d, mau 3 (dua percobaan setelah yang berhasil)", n)
	}
}

// TestTanpaSumberHanyaKolomKey: pemasangan yang belum menyentuh tabel rate_limits harus
// tetap menegakkan batas yang tertulis di baris api_keys.
func TestTanpaSumberHanyaKolomKey(t *testing.T) {
	e := NewEngine(nil, nil)
	got := e.Requests(context.Background(),
		[]policy.Target{{Scope: policy.ScopeAPIKey, ID: "key-1"}, {Scope: policy.ScopeGlobal}},
		apikey.Limits{RPM: 42})

	if len(got) != 1 {
		t.Fatalf("jumlah cakupan = %d, mau 1: %+v", len(got), got)
	}
	if got[0].Scope != apikey.ScopeAPIKey || got[0].Limits.RPM != 42 {
		t.Errorf("cakupan = %+v", got[0])
	}
}

func TestAmanDipakaiBersamaan(t *testing.T) {
	src := &sumber{rows: []*policy.RateLimit{
		baris(policy.ScopeAPIKey, "key-1", func(r *policy.RateLimit) { r.RequestsPerMinute = pointer(60) }),
	}}
	e := NewEngine(src, nil, WithTTL(time.Nanosecond))
	target := []policy.Target{{Scope: policy.ScopeAPIKey, ID: "key-1"}}

	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				e.Requests(context.Background(), target, apikey.Limits{})
				e.Tokens(context.Background(), target, apikey.TokenLimits{})
				if j%10 == 0 {
					e.Invalidate()
				}
			}
		}()
	}
	wg.Wait()
}

// TestKonkurensiFailOpenDanInvalidate menguji ketahanan konkurensi (0 data race)
// saat database/source mengalami gangguan dinamis (sukses dan error bergantian)
// sementara puluhan goroutine memanggil Requests, Tokens, dan Invalidate secara serentak.
func TestKonkurensiFailOpenDanInvalidate(t *testing.T) {
	src := &sumber{rows: []*policy.RateLimit{
		baris(policy.ScopeGlobal, "", func(r *policy.RateLimit) { r.RequestsPerSecond = pointer(200) }),
		baris(policy.ScopeAPIKey, "key-concur", func(r *policy.RateLimit) { r.RequestsPerMinute = pointer(120) }),
		baris(policy.ScopeUser, "user-concur", func(r *policy.RateLimit) { r.DailyRequestLimit = pointer(int64(5000)) }),
	}}
	// TTL sangat pendek untuk memaksa siklus reload dan double-checked locking bekerja terus menerus.
	e := NewEngine(src, nil, WithTTL(time.Millisecond))

	targets := []policy.Target{
		{Scope: policy.ScopeGlobal},
		{Scope: policy.ScopeAPIKey, ID: "key-concur"},
		{Scope: policy.ScopeUser, ID: "user-concur"},
		{Scope: policy.ScopeIP, ID: "192.168.1.1"},
	}

	stopCh := make(chan struct{})
	var wg sync.WaitGroup

	// Goroutine pembuat gejolak sumber data: berganti-ganti antara normal dan error (failover)
	wg.Add(1)
	go func() {
		defer wg.Done()
		flip := false
		for {
			select {
			case <-stopCh:
				return
			case <-time.After(5 * time.Millisecond):
				flip = !flip
				if flip {
					src.ganti(nil, errors.New("koneksi database terputus (simulasi failover)"))
				} else {
					src.ganti([]*policy.RateLimit{
						baris(policy.ScopeGlobal, "", func(r *policy.RateLimit) { r.RequestsPerSecond = pointer(300) }),
						baris(policy.ScopeAPIKey, "key-concur", func(r *policy.RateLimit) { r.RequestsPerMinute = pointer(150) }),
					}, nil)
				}
			}
		}
	}()

	// 32 goroutine pemanggil Requests dan Tokens secara simultan
	const numWorkers = 32
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		workerID := i
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				reqScopes := e.Requests(context.Background(), targets, apikey.Limits{RPM: 50})
				if len(reqScopes) == 0 {
					t.Errorf("worker %d: Requests mengembalikan slice kosong padahal ada target aktif", workerID)
				}
				tokScopes := e.Tokens(context.Background(), targets, apikey.TokenLimits{TPM: 1000})
				_ = tokScopes

				if j%15 == 0 {
					e.Invalidate()
				}
			}
		}()
	}

	// Tunggu workers selesai lalu hentikan goroutine flipper
	time.Sleep(50 * time.Millisecond)
	close(stopCh)
	wg.Wait()
}

// TestKonkurensiTargetMultiScope memastikan pemetaan multi-scope (Global, User, APIKey,
// Provider, Model, IP) dievaluasi dengan benar dan aman dari balapan data saat diakses
// bersamaan dengan keyLimits per goroutine.
func TestKonkurensiTargetMultiScope(t *testing.T) {
	src := &sumber{rows: []*policy.RateLimit{
		baris(policy.ScopeGlobal, "", func(r *policy.RateLimit) { r.RequestsPerSecond = pointer(100) }),
		baris(policy.ScopeAPIKey, "key-a", func(r *policy.RateLimit) { r.RequestsPerMinute = pointer(60) }),
		baris(policy.ScopeUser, "user-a", func(r *policy.RateLimit) { r.RequestsPerMinute = pointer(120) }),
		baris(policy.ScopeProvider, "prov-openai", func(r *policy.RateLimit) { r.RequestsPerSecond = pointer(50) }),
		baris(policy.ScopeModel, "model-gpt4", func(r *policy.RateLimit) { r.RequestsPerMinute = pointer(30) }),
		baris(policy.ScopeIP, "10.0.0.1", func(r *policy.RateLimit) { r.RequestsPerSecond = pointer(20) }),
	}}
	e := NewEngine(src, nil, WithTTL(5*time.Millisecond))

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				targets := []policy.Target{
					{Scope: policy.ScopeGlobal},
					{Scope: policy.ScopeAPIKey, ID: "key-a"},
					{Scope: policy.ScopeUser, ID: "user-a"},
					{Scope: policy.ScopeProvider, ID: "prov-openai"},
					{Scope: policy.ScopeModel, ID: "model-gpt4"},
					{Scope: policy.ScopeIP, ID: "10.0.0.1"},
				}
				res := e.Requests(context.Background(), targets, apikey.Limits{RPM: 99})
				if len(res) < 5 {
					t.Errorf("worker %d: jumlah scope terurai = %d, mau minimal 5", id, len(res))
				}
			}
		}(i)
	}
	wg.Wait()
}

// TestFailOpenSaatContextDibatalkan memverifikasi bahwa saat konteks dibatalkan di tengah
// pembacaan, mesin menangani error tanpa panic dan mempertahankan snapshot cache yang ada.
func TestFailOpenSaatContextDibatalkan(t *testing.T) {
	src := &sumber{rows: []*policy.RateLimit{
		baris(policy.ScopeGlobal, "", func(r *policy.RateLimit) { r.RequestsPerSecond = pointer(10) }),
	}}
	e := NewEngine(src, nil, WithTTL(time.Nanosecond))

	// Pembacaan pertama berhasil
	got := e.Requests(context.Background(), []policy.Target{{Scope: policy.ScopeGlobal}}, apikey.Limits{})
	if len(got) != 1 {
		t.Fatalf("pembacaan awal gagal: %+v", got)
	}

	// Pembacaan kedua dengan context yang sudah dibatalkan
	ctxBatal, cancel := context.WithCancel(context.Background())
	cancel()

	// Harus fail-open dengan salinan lama dan tidak panic
	gotBatal := e.Requests(ctxBatal, []policy.Target{{Scope: policy.ScopeGlobal}}, apikey.Limits{})
	if len(gotBatal) != 1 || gotBatal[0].Limits.RPS != 10 {
		t.Errorf("setelah context dibatalkan = %+v, mau salinan lama tetap dipakai (fail-open)", gotBatal)
	}
}

// TestEngineNilReceiverAman memastikan seluruh public method Engine aman dipanggil
// saat pointer Engine bernilai nil tanpa memicu nil-pointer dereference panic.
func TestEngineNilReceiverAman(t *testing.T) {
	var e *Engine
	e.Invalidate()

	reqs := e.Requests(context.Background(), []policy.Target{{Scope: policy.ScopeGlobal}}, apikey.Limits{RPM: 10})
	if reqs != nil {
		t.Errorf("Requests pada nil engine = %v, mau nil", reqs)
	}

	toks := e.Tokens(context.Background(), []policy.Target{{Scope: policy.ScopeGlobal}}, apikey.TokenLimits{TPM: 100})
	if toks != nil {
		t.Errorf("Tokens pada nil engine = %v, mau nil", toks)
	}
}

// TestMemoryLimiter memvalidasi fungsionalitas token bucket: kapasitas awal,
// konsumsi token, penolakan saat habis, pengisian ulang (refill), dan keamanan nil receiver.
func TestMemoryLimiter(t *testing.T) {
	// Limiter kapasitas 3 token, laju 10 token/detik
	lim := NewMemoryLimiter(10, 3)

	// 3 token pertama harus diizinkan
	for i := 0; i < 3; i++ {
		if !lim.Allow() {
			t.Fatalf("token ke-%d seharusnya diizinkan", i+1)
		}
	}

	// Token ke-4 harus ditolak karena bucket habis
	if lim.Allow() {
		t.Fatal("token ke-4 seharusnya ditolak saat bucket kosong")
	}

	// Simulasi waktu maju 200ms -> harus terisi ~2 token
	now := time.Now().Add(200 * time.Millisecond)
	if !lim.AllowN(now, 1.0) {
		t.Fatal("seharusnya diizinkan setelah refill 200ms")
	}

	// Nil receiver aman
	var nilLim *MemoryLimiter
	if !nilLim.Allow() {
		t.Fatal("nil receiver MemoryLimiter harus mengembalikan true (fail-open)")
	}
}
