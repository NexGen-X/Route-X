package gateway

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
	"github.com/NexGen-X/Route-X/internal/providers"
)

// kandidat membuat RouteCandidate untuk test.
func kandidat(nama string, ubah ...func(*upstream.RouteCandidate)) *upstream.RouteCandidate {
	c := &upstream.RouteCandidate{
		ProviderModelID:   "pm-" + nama,
		UpstreamModelName: "upstream-" + nama,
		SupportsStreaming: true,
		SupportsTools:     true,
		ProviderID:        "prov-" + nama,
		ProviderName:      nama,
		Kind:              upstream.KindOpenAI,
		TimeoutMS:         5_000,
	}
	for _, f := range ubah {
		f(c)
	}
	return c
}

// penjagaUji adalah CircuitGuard untuk test.
type penjagaUji struct {
	mu sync.Mutex
	// tolak berisi providerID yang izinnya ditolak.
	tolak map[string]bool
	// errAllow membuat Allow gagal, untuk menguji perilaku gagal-terbuka.
	errAllow error
	// dicatat merekam setiap panggilan Record dalam urutan terjadinya.
	dicatat []string
}

func penjaga() *penjagaUji { return &penjagaUji{tolak: map[string]bool{}} }

func (p *penjagaUji) Allow(_ context.Context, providerID, _ string) (bool, State, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.errAllow != nil {
		return false, "", p.errAllow
	}
	if p.tolak[providerID] {
		return false, StateOpen, nil
	}
	return true, StateClosed, nil
}

func (p *penjagaUji) Record(_ context.Context, providerID, _ string, sukses bool) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.dicatat = append(p.dicatat, fmt.Sprintf("%s:%t", providerID, sukses))
	return nil
}

func (p *penjagaUji) catatan() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.dicatat...)
}

// executorUji membuat Executor yang tidak pernah benar-benar tidur, supaya test tidak
// membayar jeda backoff. Jeda yang diminta tetap direkam sehingga bisa diperiksa.
func executorUji(g CircuitGuard) (*Executor, *[]time.Duration) {
	var jeda []time.Duration
	var mu sync.Mutex
	e := NewExecutor(g, nil,
		// Jitter dibuat deterministik: selalu ambil batas atas undian.
		WithJitterFunc(func(n int64) int64 { return n }),
		WithSleepFunc(func(ctx context.Context, d time.Duration) error {
			mu.Lock()
			jeda = append(jeda, d)
			mu.Unlock()
			return ctx.Err()
		}),
	)
	return e, &jeda
}

func rencana(model string, maks int, base time.Duration, cands ...*upstream.RouteCandidate) Plan {
	return Plan{Candidates: cands, Model: model, MaxAttempts: maks, BackoffBase: base}
}

func TestExecuteBerhasilPadaKandidatPertama(t *testing.T) {
	g := penjaga()
	e, jeda := executorUji(g)

	var dipanggil int
	out := Execute(context.Background(), e, rencana("gpt-5", 3, 0, kandidat("a"), kandidat("b")),
		func(context.Context, *upstream.RouteCandidate) (string, error) {
			dipanggil++
			return "jawaban", nil
		})

	if out.Err != nil {
		t.Fatalf("Err = %v, mau nil", out.Err)
	}
	if out.Value != "jawaban" {
		t.Errorf("Value = %q", out.Value)
	}
	if dipanggil != 1 {
		t.Errorf("op dipanggil %d kali, mau 1 — kandidat kedua tidak boleh disentuh", dipanggil)
	}
	if out.Candidate == nil || out.Candidate.ProviderName != "a" {
		t.Errorf("Candidate = %v, mau a", out.Candidate)
	}
	if len(out.Attempts) != 1 {
		t.Errorf("jejak = %d percobaan, mau 1", len(out.Attempts))
	}
	if len(*jeda) != 0 {
		t.Errorf("ada backoff pada permintaan yang berhasil: %v", *jeda)
	}
	if got := g.catatan(); len(got) != 1 || got[0] != "prov-a:true" {
		t.Errorf("catatan pemutus arus = %v, mau [prov-a:true]", got)
	}
}

// Kegagalan yang aman diulang dicoba ulang pada kandidat yang SAMA sampai MaxAttempts,
// baru kemudian pindah kandidat.
func TestExecuteMengulangKandidatYangSama(t *testing.T) {
	g := penjaga()
	e, jeda := executorUji(g)

	var urutan []string
	out := Execute(context.Background(), e, rencana("m", 3, 100*time.Millisecond, kandidat("a"), kandidat("b")),
		func(_ context.Context, c *upstream.RouteCandidate) (string, error) {
			urutan = append(urutan, c.ProviderName)
			if c.ProviderName == "a" {
				return "", providers.Newf(providers.ErrKindOverloaded, c.ProviderName, "penuh")
			}
			return "ok", nil
		})

	if out.Err != nil {
		t.Fatalf("Err = %v", out.Err)
	}
	// Tiga percobaan pada a, lalu satu pada b.
	mau := []string{"a", "a", "a", "b"}
	if fmt.Sprint(urutan) != fmt.Sprint(mau) {
		t.Errorf("urutan percobaan = %v, mau %v", urutan, mau)
	}
	// Dua jeda: sebelum percobaan kedua dan ketiga pada a. Tidak ada jeda saat berpindah
	// kandidat — provider lain tidak sedang meminta kita menunggu.
	if len(*jeda) != 2 {
		t.Errorf("jumlah jeda = %d (%v), mau 2", len(*jeda), *jeda)
	}
	if len(out.Attempts) != 4 {
		t.Errorf("jejak = %d percobaan, mau 4", len(out.Attempts))
	}
	// Jejak kegagalan tetap ada meski permintaannya akhirnya berhasil: tanpa itu, "upstream
	// mana yang sedang bermasalah" tidak bisa dijawab dari log.
	if out.Attempts[0].Err == nil || out.Attempts[0].Err.Kind != providers.ErrKindOverloaded {
		t.Errorf("percobaan pertama = %+v, mau tercatat gagal", out.Attempts[0])
	}
}

// Kegagalan yang TIDAK aman diulang tapi aman dialihkan harus langsung pindah kandidat,
// tanpa membuang percobaan di provider yang jelas tidak akan berubah jawabannya.
func TestExecuteAuthLangsungFailoverTanpaRetry(t *testing.T) {
	g := penjaga()
	e, jeda := executorUji(g)

	var urutan []string
	out := Execute(context.Background(), e, rencana("m", 3, 100*time.Millisecond, kandidat("a"), kandidat("b")),
		func(_ context.Context, c *upstream.RouteCandidate) (string, error) {
			urutan = append(urutan, c.ProviderName)
			if c.ProviderName == "a" {
				return "", providers.Newf(providers.ErrKindAuth, c.ProviderName, "kredensial ditolak")
			}
			return "ok", nil
		})

	if out.Err != nil {
		t.Fatalf("Err = %v", out.Err)
	}
	if mau := []string{"a", "b"}; fmt.Sprint(urutan) != fmt.Sprint(mau) {
		t.Errorf("urutan = %v, mau %v — kredensial yang ditolak diulang", urutan, mau)
	}
	if len(*jeda) != 0 {
		t.Errorf("ada backoff untuk kegagalan yang tidak diulang: %v", *jeda)
	}
}

// Kegagalan yang tidak boleh dialihkan menghentikan segalanya: mengirim permintaan cacat
// ke provider lain hanya menghasilkan kegagalan kedua sambil membebani kuota.
func TestExecuteInvalidRequestTidakDialihkan(t *testing.T) {
	g := penjaga()
	e, _ := executorUji(g)

	var dipanggil int
	out := Execute(context.Background(), e, rencana("m", 3, 0, kandidat("a"), kandidat("b")),
		func(_ context.Context, c *upstream.RouteCandidate) (string, error) {
			dipanggil++
			return "", providers.Newf(providers.ErrKindInvalidRequest, c.ProviderName, "terlalu panjang")
		})

	if out.Err == nil || out.Err.Kind != providers.ErrKindInvalidRequest {
		t.Fatalf("Err = %v, mau invalid_request", out.Err)
	}
	if dipanggil != 1 {
		t.Errorf("op dipanggil %d kali, mau 1", dipanggil)
	}
}

// Sifat yang dipegang paling ketat di paket ini: aliran yang sudah mulai terkirim tidak
// pernah diulang maupun dialihkan. Percobaan kedua akan menyambung aliran baru di tengah
// aliran pertama, dan klien menerima jawaban yang tidak koheren — lebih buruk daripada
// kegagalan yang jujur.
func TestExecuteAliranTerkirimSebagianTidakDiulangMaupunDialihkan(t *testing.T) {
	g := penjaga()
	e, jeda := executorUji(g)

	var dipanggil int
	out := Execute(context.Background(), e, rencana("m", 3, 100*time.Millisecond, kandidat("a"), kandidat("b")),
		func(_ context.Context, c *upstream.RouteCandidate) (string, error) {
			dipanggil++
			// Timeout: kegagalan yang PALING aman diulang maupun dialihkan — kecuali
			// setelah byte terkirim.
			return "", &providers.Error{
				Kind: providers.ErrKindTimeout, Provider: c.ProviderName,
				Message: "putus di tengah aliran", StreamedBytes: 4096,
			}
		})

	if dipanggil != 1 {
		t.Errorf("op dipanggil %d kali, mau 1 — aliran yang sudah terkirim sebagian dicoba lagi", dipanggil)
	}
	if len(*jeda) != 0 {
		t.Errorf("ada backoff setelah aliran terkirim sebagian: %v", *jeda)
	}
	if out.Err == nil || out.Err.StreamedBytes == 0 {
		t.Errorf("Err = %+v, mau kegagalan yang membawa StreamedBytes", out.Err)
	}
	if out.Candidate != nil {
		t.Errorf("Candidate = %v, mau nil", out.Candidate)
	}
}

// Pemutus arus yang terbuka membuat kandidat DILEWATI, bukan dicoba lalu gagal — dan
// pelewatan itu tetap tercatat, supaya "kenapa provider ini tidak dipakai" bisa dijawab
// dari log tanpa menebak.
func TestExecuteMelewatiKandidatSaatPemutusArusTerbuka(t *testing.T) {
	g := penjaga()
	g.tolak["prov-a"] = true
	e, _ := executorUji(g)

	var urutan []string
	out := Execute(context.Background(), e, rencana("m", 3, 0, kandidat("a"), kandidat("b")),
		func(_ context.Context, c *upstream.RouteCandidate) (string, error) {
			urutan = append(urutan, c.ProviderName)
			return "ok", nil
		})

	if out.Err != nil {
		t.Fatalf("Err = %v", out.Err)
	}
	if mau := []string{"b"}; fmt.Sprint(urutan) != fmt.Sprint(mau) {
		t.Errorf("urutan = %v, mau %v", urutan, mau)
	}
	if len(out.Attempts) != 2 {
		t.Fatalf("jejak = %d, mau 2 (satu dilewati, satu berhasil)", len(out.Attempts))
	}
	lewat := out.Attempts[0]
	if lewat.SkipReason == "" {
		t.Error("kandidat yang dilewati tidak membawa alasan")
	}
	if lewat.BreakerState != StateOpen {
		t.Errorf("BreakerState = %q, mau %q", lewat.BreakerState, StateOpen)
	}
	if lewat.Berhasil() {
		t.Error("percobaan yang dilewati dilaporkan berhasil")
	}
	// Kandidat yang dilewati tidak pernah dihubungi, jadi tidak ada hasil untuk dicatat.
	if got := g.catatan(); len(got) != 1 || got[0] != "prov-b:true" {
		t.Errorf("catatan = %v, mau hanya prov-b:true", got)
	}
}

// Penjaga yang error meloloskan permintaan, tidak memblokirnya. Gagal-tertutup berarti
// Redis mati mematikan seluruh gateway.
func TestExecutePenjagaErrorMeloloskanPermintaan(t *testing.T) {
	g := penjaga()
	g.errAllow = errors.New("redis mati")
	e, _ := executorUji(g)

	out := Execute(context.Background(), e, rencana("m", 1, 0, kandidat("a")),
		func(context.Context, *upstream.RouteCandidate) (string, error) { return "ok", nil })

	if out.Err != nil {
		t.Fatalf("Err = %v, mau nil — penjaga yang error memblokir permintaan", out.Err)
	}
	if out.Value != "ok" {
		t.Errorf("Value = %q", out.Value)
	}
}

// Tanpa penjaga sama sekali, retry dan failover tetap berjalan.
func TestExecuteTanpaPenjaga(t *testing.T) {
	e, _ := executorUji(nil)
	out := Execute(context.Background(), e, rencana("m", 2, 0, kandidat("a"), kandidat("b")),
		func(_ context.Context, c *upstream.RouteCandidate) (string, error) {
			if c.ProviderName == "a" {
				return "", providers.Newf(providers.ErrKindServer, c.ProviderName, "500")
			}
			return "ok", nil
		})
	if out.Err != nil || out.Value != "ok" {
		t.Errorf("Err = %v, Value = %q", out.Err, out.Value)
	}
}

func TestExecuteTanpaKandidat(t *testing.T) {
	e, _ := executorUji(penjaga())
	out := Execute(context.Background(), e, rencana("gpt-5", 3, 0),
		func(context.Context, *upstream.RouteCandidate) (string, error) {
			t.Fatal("op dipanggil padahal tidak ada kandidat")
			return "", nil
		})

	if out.Err == nil || out.Err.Kind != providers.ErrKindModelNotFound {
		t.Fatalf("Err = %v, mau model_not_found", out.Err)
	}
	if !out.TanpaKandidat() {
		t.Error("TanpaKandidat() = false")
	}
	// Nama model yang diminta harus muncul: tanpa itu operator tidak tahu model mana yang
	// tidak punya rute.
	if got := out.Err.Error(); !strings.Contains(got, "gpt-5") {
		t.Errorf("pesan = %q, tidak menyebut model yang diminta", got)
	}
}

// Pembatalan oleh klien bukan kegagalan provider dan tidak boleh dihitung ke pemutus arus:
// satu klien yang menutup koneksi berkali-kali bisa memutus arus ke provider yang sehat.
func TestExecutePembatalanKlienTidakDicatatKePemutusArus(t *testing.T) {
	g := penjaga()
	e, _ := executorUji(g)

	ctx, batal := context.WithCancel(context.Background())
	batal()

	out := Execute(ctx, e, rencana("m", 3, 0, kandidat("a"), kandidat("b")),
		func(context.Context, *upstream.RouteCandidate) (string, error) {
			t.Fatal("op dipanggil padahal context sudah dibatalkan")
			return "", nil
		})

	if out.Err == nil || out.Err.Kind != providers.ErrKindCanceled {
		t.Fatalf("Err = %v, mau canceled", out.Err)
	}
	if got := g.catatan(); len(got) != 0 {
		t.Errorf("catatan pemutus arus = %v, mau kosong", got)
	}
}

// Pembatalan yang terjadi DI TENGAH percobaan juga tidak boleh dicatat, dan tidak boleh
// membuat gateway menghubungi seluruh sisa daftar provider satu per satu.
func TestExecutePembatalanSaatPercobaanBerjalan(t *testing.T) {
	g := penjaga()
	e, _ := executorUji(g)

	ctx, batal := context.WithCancel(context.Background())
	var dipanggil int
	out := Execute(ctx, e, rencana("m", 3, 0, kandidat("a"), kandidat("b"), kandidat("c")),
		func(cctx context.Context, c *upstream.RouteCandidate) (string, error) {
			dipanggil++
			batal()
			return "", providers.FromTransport(c.ProviderName, cctx.Err())
		})

	if dipanggil != 1 {
		t.Errorf("op dipanggil %d kali, mau 1 — klien yang pergi tetap membuat sisa provider dihubungi", dipanggil)
	}
	if out.Err == nil || out.Err.Kind != providers.ErrKindCanceled {
		t.Fatalf("Err = %v, mau canceled", out.Err)
	}
	if got := g.catatan(); len(got) != 0 {
		t.Errorf("catatan pemutus arus = %v, mau kosong", got)
	}
}

// Tenggat PERCOBAAN yang habis harus terklasifikasi sebagai timeout, bukan canceled.
// Keduanya sampai ke pemanggil sebagai context yang selesai, dan akibatnya berlawanan:
// timeout aman dialihkan, pembatalan klien tidak. Tanpa koreksi ini, failover tidak pernah
// berjalan untuk kegagalan paling umum yang dimiliki gateway.
func TestExecuteTenggatPercobaanDiklasifikasiTimeoutBukanCanceled(t *testing.T) {
	g := penjaga()
	e, _ := executorUji(g)

	// Timeout percobaan sangat pendek; op menunggu lebih lama.
	c := kandidat("lambat", func(c *upstream.RouteCandidate) { c.TimeoutMS = 20 })

	out := Execute(context.Background(), e, rencana("m", 1, 0, c, kandidat("cepat")),
		func(cctx context.Context, cand *upstream.RouteCandidate) (string, error) {
			if cand.ProviderName == "lambat" {
				<-cctx.Done()
				return "", providers.FromTransport(cand.ProviderName, cctx.Err())
			}
			return "ok", nil
		})

	if out.Err != nil {
		t.Fatalf("Err = %v, mau nil — failover tidak berjalan untuk tenggat percobaan", out.Err)
	}
	if out.Candidate == nil || out.Candidate.ProviderName != "cepat" {
		t.Fatalf("Candidate = %v, mau cepat", out.Candidate)
	}
	pertama := out.Attempts[0]
	if pertama.Err == nil || pertama.Err.Kind != providers.ErrKindTimeout {
		t.Errorf("percobaan pertama = %+v, mau terklasifikasi timeout", pertama.Err)
	}
	// Kegagalan itu tetap dicatat ke pemutus arus: provider yang tidak menjawab sebelum
	// tenggat memang sedang bermasalah.
	if got := g.catatan(); len(got) != 2 || got[0] != "prov-lambat:false" {
		t.Errorf("catatan = %v, mau [prov-lambat:false prov-cepat:true]", got)
	}
}

// Error mentah dari adapter yang tidak terklasifikasi TIDAK diulang dan TIDAK dialihkan:
// ia bisa berarti permintaannya sebenarnya sampai dan sudah menimbulkan efek di sisi
// provider, dan mengulangnya berarti menagih pengguna dua kali.
func TestExecuteErrorTakTerklasifikasiTidakDiulang(t *testing.T) {
	g := penjaga()
	e, _ := executorUji(g)

	var dipanggil int
	out := Execute(context.Background(), e, rencana("m", 3, 0, kandidat("a"), kandidat("b")),
		func(context.Context, *upstream.RouteCandidate) (string, error) {
			dipanggil++
			return "", errors.New("bug adapter yang tidak dibungkus providers.Error")
		})

	if dipanggil != 1 {
		t.Errorf("op dipanggil %d kali, mau 1", dipanggil)
	}
	if out.Err == nil || out.Err.Kind != providers.ErrKindUnknown {
		t.Fatalf("Err = %v, mau unknown", out.Err)
	}
	// Pesan aslinya tidak boleh diteruskan: error tak terklasifikasi dari adapter bisa
	// memuat potongan permintaan atau URL berkredensial.
	if strings.Contains(out.Err.Message, "bug adapter") {
		t.Errorf("pesan asli adapter diteruskan: %q", out.Err.Message)
	}
	if out.Err.Provider != "a" {
		t.Errorf("Provider = %q, mau a", out.Err.Provider)
	}
}

// Kegagalan yang dilaporkan adalah yang TERAKHIR, bukan yang pertama: yang pertama sudah
// tidak menggambarkan keadaan setelah gateway mencoba tempat lain.
func TestExecuteMelaporkanKegagalanTerakhir(t *testing.T) {
	g := penjaga()
	e, _ := executorUji(g)

	out := Execute(context.Background(), e, rencana("m", 1, 0, kandidat("a"), kandidat("b")),
		func(_ context.Context, c *upstream.RouteCandidate) (string, error) {
			if c.ProviderName == "a" {
				return "", providers.Newf(providers.ErrKindOverloaded, c.ProviderName, "penuh")
			}
			return "", providers.Newf(providers.ErrKindQuota, c.ProviderName, "kuota habis")
		})

	if out.Err == nil || out.Err.Kind != providers.ErrKindQuota {
		t.Fatalf("Err = %v, mau quota (kegagalan kandidat terakhir)", out.Err)
	}
	if out.Percobaan() != 2 {
		t.Errorf("Percobaan() = %d, mau 2", out.Percobaan())
	}
	if out.TanpaKandidat() {
		t.Error("TanpaKandidat() = true padahal dua provider dihubungi")
	}
}

func TestBackoffTumbuhEksponensialDanTerbatas(t *testing.T) {
	// Jitter dibuat mengambil batas atas undian, jadi hasilnya jeda penuh: base<<geser.
	e := NewExecutor(nil, nil, WithJitterFunc(func(n int64) int64 { return n }))
	base := 100 * time.Millisecond

	for _, tc := range []struct {
		n   int
		mau time.Duration
	}{
		{1, 0}, // percobaan pertama tidak pernah menunggu
		{2, 100 * time.Millisecond},
		{3, 200 * time.Millisecond},
		{4, 400 * time.Millisecond},
		{5, 800 * time.Millisecond},
	} {
		if got := e.backoff(base, tc.n); got != tc.mau {
			t.Errorf("backoff(%v, %d) = %v, mau %v", base, tc.n, got, tc.mau)
		}
	}

	// Batas atas dipegang: base besar dengan n besar tidak boleh meluap menjadi durasi
	// negatif, karena durasi negatif membuat timer memicu seketika — backoff yang justru
	// hilang tepat ketika paling dibutuhkan.
	for _, n := range []int{20, 40, 100} {
		got := e.backoff(30*time.Second, n)
		if got <= 0 || got > maxBackoff {
			t.Errorf("backoff(30s, %d) = %v, mau di antara 0 dan %v", n, got, maxBackoff)
		}
	}
}

// Separuh jeda dijamin, separuhnya diundi. Bagian yang dijamin ada karena jitter penuh
// boleh menghasilkan nol, dan mengulang seketika ke provider yang baru mengirim 429 hanya
// menghasilkan 429 kedua.
func TestBackoffJitterMenjaminSeparuhJeda(t *testing.T) {
	base := 200 * time.Millisecond

	// Undian selalu nol: hasilnya batas bawah, yaitu separuh jeda.
	bawah := NewExecutor(nil, nil, WithJitterFunc(func(int64) int64 { return 0 }))
	if got := bawah.backoff(base, 2); got != base/2 {
		t.Errorf("batas bawah = %v, mau %v", got, base/2)
	}

	// Undian sungguhan tetap harus berada di antara separuh dan penuh.
	nyata := NewExecutor(nil, nil)
	for i := 0; i < 200; i++ {
		got := nyata.backoff(base, 3)
		penuh := 2 * base
		if got < penuh/2 || got > penuh {
			t.Fatalf("backoff = %v, mau di antara %v dan %v", got, penuh/2, penuh)
		}
	}
}

func TestBackoffTanpaBase(t *testing.T) {
	e := NewExecutor(nil, nil)
	if got := e.backoff(0, 5); got != 0 {
		t.Errorf("backoff tanpa base = %v, mau 0", got)
	}
}

// Retry-After dari upstream menang atas backoff hitungan sendiri: upstream tahu kapan
// kuotanya kembali, kita hanya menduga.
func TestJedaSebelumMematuhiRetryAfter(t *testing.T) {
	e := NewExecutor(nil, nil, WithJitterFunc(func(n int64) int64 { return n }))
	perr := &providers.Error{Kind: providers.ErrKindRateLimit, RetryAfter: 1200 * time.Millisecond}

	got, boleh := e.jedaSebelum(context.Background(), 100*time.Millisecond, 2, perr)
	if !boleh {
		t.Fatal("boleh = false untuk Retry-After yang masih pantas ditunggu")
	}
	if got != 1200*time.Millisecond {
		t.Errorf("jeda = %v, mau 1,2s dari Retry-After", got)
	}
}

// Retry-After yang terlalu panjang membuat kandidatnya DILEWATI, bukan ditunggu. Provider
// yang membatasi laju biasa mengirim jeda dalam hitungan menit, dan satu permintaan klien
// tidak boleh menggantung selama itu.
func TestJedaSebelumMelewatiKandidatSaatRetryAfterTerlaluPanjang(t *testing.T) {
	e := NewExecutor(nil, nil)
	perr := &providers.Error{Kind: providers.ErrKindRateLimit, RetryAfter: time.Hour}

	if _, boleh := e.jedaSebelum(context.Background(), 100*time.Millisecond, 2, perr); boleh {
		t.Error("boleh = true untuk Retry-After satu jam")
	}
}

// Menunggu melewati tenggat permintaan berarti menunggu untuk percobaan yang tidak akan
// pernah sempat berjalan.
func TestJedaSebelumMenolakSaatTenggatPermintaanHampirHabis(t *testing.T) {
	e := NewExecutor(nil, nil, WithJitterFunc(func(n int64) int64 { return n }))

	ctx, batal := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer batal()

	if _, boleh := e.jedaSebelum(ctx, time.Second, 2, nil); boleh {
		t.Error("boleh = true padahal jedanya melewati tenggat permintaan")
	}
	// Tenggat yang masih lapang tetap mengizinkan.
	lapang, batal2 := context.WithTimeout(context.Background(), time.Minute)
	defer batal2()
	if _, boleh := e.jedaSebelum(lapang, 100*time.Millisecond, 2, nil); !boleh {
		t.Error("boleh = false padahal tenggat masih lapang")
	}
}

// Retry-After yang panjang harus menghentikan retry TAPI tetap membiarkan failover: itu
// justru guna failover.
func TestExecuteRetryAfterPanjangTetapFailover(t *testing.T) {
	g := penjaga()
	e, jeda := executorUji(g)

	out := Execute(context.Background(), e, rencana("m", 3, 100*time.Millisecond, kandidat("a"), kandidat("b")),
		func(_ context.Context, c *upstream.RouteCandidate) (string, error) {
			if c.ProviderName == "a" {
				return "", &providers.Error{
					Kind: providers.ErrKindRateLimit, Provider: c.ProviderName,
					Message: "dibatasi", RetryAfter: time.Hour,
				}
			}
			return "ok", nil
		})

	if out.Err != nil {
		t.Fatalf("Err = %v, mau nil", out.Err)
	}
	if out.Candidate == nil || out.Candidate.ProviderName != "b" {
		t.Fatalf("Candidate = %v, mau b", out.Candidate)
	}
	if len(*jeda) != 0 {
		t.Errorf("gateway menunggu padahal Retry-After terlalu panjang: %v", *jeda)
	}
	// Hanya satu percobaan pada a: sisanya tidak dipakai karena menunggu bukan pilihan.
	if n := out.Percobaan(); n != 2 {
		t.Errorf("Percobaan() = %d, mau 2", n)
	}
}

// aliranUji adalah providers.Stream untuk test.
//
// Ia MEMBACA context yang diberikan saat dibuka, dan itu inti pengujiannya: aliran yang
// context-nya sudah mati akan gagal pada Recv pertama.
type aliranUji struct {
	ctx      context.Context
	ditutup  atomicBool
	terkirim int
}

// atomicBool dipisah supaya Close aman dipanggil dari goroutine mana pun, seperti kontrak
// providers.Stream mensyaratkan.
type atomicBool struct {
	mu sync.Mutex
	v  bool
}

func (b *atomicBool) set() {
	b.mu.Lock()
	b.v = true
	b.mu.Unlock()
}

func (b *atomicBool) get() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.v
}

func (s *aliranUji) Recv() (*providers.StreamEvent, error) {
	if err := s.ctx.Err(); err != nil {
		return nil, err
	}
	s.terkirim++
	return &providers.StreamEvent{Delta: "potongan", Raw: []byte(`{"ok":true}`)}, nil
}

func (s *aliranUji) Close() error {
	s.ditutup.set()
	return nil
}

// Kekeliruan yang paling mudah dibuat di jalur streaming: mematikan context percobaan saat
// pembukaan selesai. Akibatnya aliran selalu terputus tepat sebelum byte pertama, dan itu
// tampak seperti masalah upstream.
func TestExecuteStreamContextTetapHidupSetelahAliranTerbuka(t *testing.T) {
	g := penjaga()
	e, _ := executorUji(g)

	var dibuat *aliranUji
	out := ExecuteStream(context.Background(), e, rencana("m", 1, 0, kandidat("a")),
		func(cctx context.Context, _ *upstream.RouteCandidate) (providers.Stream, error) {
			dibuat = &aliranUji{ctx: cctx}
			return dibuat, nil
		})

	if out.Err != nil {
		t.Fatalf("Err = %v", out.Err)
	}
	if out.Value == nil {
		t.Fatal("Value nil")
	}

	// Pembacaan setelah Execute kembali harus tetap berhasil.
	for i := 0; i < 3; i++ {
		ev, err := out.Value.Recv()
		if err != nil {
			t.Fatalf("Recv ke-%d = %v, mau nil — context percobaan dimatikan terlalu awal", i+1, err)
		}
		if ev.Delta == "" {
			t.Error("peristiwa tanpa delta")
		}
	}

	// Close pada aliran yang dikembalikan harus menutup aliran di bawahnya DAN melepas
	// context-nya.
	if err := out.Value.Close(); err != nil {
		t.Errorf("Close = %v", err)
	}
	if !dibuat.ditutup.get() {
		t.Error("Close tidak diteruskan ke aliran di bawahnya")
	}
	if dibuat.ctx.Err() == nil {
		t.Error("context tidak dilepas setelah Close — ia akan bocor untuk setiap aliran yang ditutup lebih awal")
	}
	// Close aman dipanggil berulang, seperti kontrak providers.Stream mensyaratkan.
	if err := out.Value.Close(); err != nil {
		t.Errorf("Close kedua = %v", err)
	}
}

// Tahap MEMBUKA aliran tetap dibatasi tenggat percobaan, dan tenggat itu harus
// terklasifikasi sebagai timeout — bukan canceled, yang tidak akan pernah dialihkan.
func TestExecuteStreamTenggatPembukaanDiklasifikasiTimeout(t *testing.T) {
	g := penjaga()
	e, _ := executorUji(g)

	lambat := kandidat("lambat", func(c *upstream.RouteCandidate) { c.TimeoutMS = 20 })

	out := ExecuteStream(context.Background(), e, rencana("m", 1, 0, lambat, kandidat("cepat")),
		func(cctx context.Context, c *upstream.RouteCandidate) (providers.Stream, error) {
			if c.ProviderName == "lambat" {
				// Menggantung sampai pengawas membatalkan, seperti upstream yang tidak
				// pernah mengirim header.
				<-cctx.Done()
				return nil, cctx.Err()
			}
			return &aliranUji{ctx: cctx}, nil
		})

	if out.Err != nil {
		t.Fatalf("Err = %v, mau nil — failover tidak berjalan untuk tenggat pembukaan", out.Err)
	}
	if out.Candidate == nil || out.Candidate.ProviderName != "cepat" {
		t.Fatalf("Candidate = %v, mau cepat", out.Candidate)
	}
	if pertama := out.Attempts[0]; pertama.Err == nil || pertama.Err.Kind != providers.ErrKindTimeout {
		t.Errorf("percobaan pertama = %+v, mau timeout", pertama.Err)
	}
	if out.Value != nil {
		_ = out.Value.Close()
	}
}

// Pembatalan oleh KLIEN saat pembukaan tetap canceled, bukan timeout: bedanya menentukan
// apakah gateway mencoba provider lain, dan mencoba provider lain untuk klien yang sudah
// pergi hanya membakar kuota.
func TestExecuteStreamPembatalanKlienTetapCanceled(t *testing.T) {
	g := penjaga()
	e, _ := executorUji(g)

	ctx, batal := context.WithCancel(context.Background())
	var dipanggil int

	out := ExecuteStream(ctx, e, rencana("m", 1, 0, kandidat("a"), kandidat("b")),
		func(cctx context.Context, c *upstream.RouteCandidate) (providers.Stream, error) {
			dipanggil++
			batal()
			<-cctx.Done()
			return nil, cctx.Err()
		})

	if dipanggil != 1 {
		t.Errorf("open dipanggil %d kali, mau 1", dipanggil)
	}
	if out.Err == nil || out.Err.Kind != providers.ErrKindCanceled {
		t.Fatalf("Err = %v, mau canceled", out.Err)
	}
}

// Adapter yang mengembalikan aliran nil tanpa error tidak boleh membuat pemanggil
// dereference nil: itu panik di jalur permintaan, bukan kegagalan yang bisa dilaporkan.
func TestExecuteStreamMenolakAliranNilTanpaError(t *testing.T) {
	g := penjaga()
	e, _ := executorUji(g)

	out := ExecuteStream(context.Background(), e, rencana("m", 1, 0, kandidat("a")),
		func(context.Context, *upstream.RouteCandidate) (providers.Stream, error) {
			return nil, nil
		})

	if out.Err == nil || out.Err.Kind != providers.ErrKindUnknown {
		t.Fatalf("Err = %v, mau unknown", out.Err)
	}
	if out.Value != nil {
		t.Error("Value tidak nil padahal adapter mengembalikan aliran kosong")
	}
}

// PlanFromRule tanpa aturan harus menghasilkan kesabaran yang sama dengan nilai bawaan
// kolom routing_rules. Kalau berbeda, operator yang membuat aturan pertamanya akan melihat
// karakter retry berubah tanpa pernah memintanya.
func TestPlanFromRuleBawaanSamaDenganMigrasi(t *testing.T) {
	p := PlanFromRule(nil, "gpt-5", []*upstream.RouteCandidate{kandidat("a")})
	if p.MaxAttempts != 3 {
		t.Errorf("MaxAttempts = %d, mau 3 (bawaan routing_rules.max_attempts)", p.MaxAttempts)
	}
	if p.BackoffBase != 250*time.Millisecond {
		t.Errorf("BackoffBase = %v, mau 250ms (bawaan routing_rules.backoff_ms)", p.BackoffBase)
	}
	if p.Model != "gpt-5" || len(p.Candidates) != 1 {
		t.Errorf("Plan = %+v", p)
	}
}
