package gateway

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/NexGen-X/Route-X/internal/database/repo"
	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
	"github.com/NexGen-X/Route-X/internal/providers"
	"github.com/NexGen-X/Route-X/internal/security"
)

// --- Tiruan ------------------------------------------------------------------

// sumberKredensial adalah CredentialSource tiruan yang isinya bisa diganti di tengah jalan,
// seperti rotasi kredensial yang sungguhan.
type sumberKredensial struct {
	mu      sync.Mutex
	cred    *upstream.ActiveCredential
	err     error
	panggil int
}

func (s *sumberKredensial) Active(_ context.Context, _ string) (*upstream.ActiveCredential, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.panggil++
	if s.err != nil {
		return nil, s.err
	}
	// Salinan, bukan pointer yang sama: pemanggil tidak boleh bisa mengubah isi sumber.
	salinan := *s.cred
	return &salinan, nil
}

func (s *sumberKredensial) ganti(id string, rahasia security.Secret) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cred = &upstream.ActiveCredential{ID: id, Label: "uji", Secret: rahasia}
	s.err = nil
}

func (s *sumberKredensial) jumlahPanggilan() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.panggil
}

// sumberEgress adalah EgressSource tiruan.
type sumberEgress struct {
	mu     sync.Mutex
	url    security.Secret
	err    error
	dipita []string
}

func (s *sumberEgress) ProxyURL(_ context.Context, id string) (security.Secret, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dipita = append(s.dipita, id)
	return s.url, s.err
}

func (s *sumberEgress) diminta() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.dipita...)
}

// --- Perkakas ----------------------------------------------------------------

const rahasiaUji = "sk-rahasia-yang-tidak-boleh-tercetak"

func kredensialUji() *sumberKredensial {
	s := &sumberKredensial{}
	s.ganti("cred-1", security.Secret(rahasiaUji))
	return s
}

func loggerSenyap() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// pabrikUji membuat Factory dengan kebijakan SSRF yang hanya mengecualikan loopback.
func pabrikUji(t *testing.T, creds CredentialSource, egress EgressSource, opts ...FactoryOption) *Factory {
	t.Helper()
	policy := security.SSRFPolicy{AllowHTTP: true, AllowedPrivateAddrs: loopbackSaja(t)}
	return NewFactory(creds, egress, policy, loggerSenyap(), opts...)
}

// serverUji menjalankan upstream tiruan yang menjawab satu chat completion yang sah.
func serverUji(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"c1","model":"m","created":1,
			"choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func kandidatUji(kind, baseURL string) *upstream.RouteCandidate {
	return &upstream.RouteCandidate{
		ProviderModelID:   "8f1f0b6e-0000-4000-8000-000000000001",
		UpstreamModelName: "m",
		SupportsStreaming: true,
		ProviderID:        "8f1f0b6e-0000-4000-8000-0000000000ff",
		ProviderName:      "uji",
		Kind:              kind,
		BaseURL:           baseURL,
		TimeoutMS:         3000,
	}
}

// --- Pemilihan adapter -------------------------------------------------------

func TestFactoryAdapterSesuaiKind(t *testing.T) {
	srv := serverUji(t)
	for _, kind := range []string{
		providers.KindOpenAI,
		providers.KindOpenAICompatible,
		providers.KindCustom,
		providers.KindAnthropic,
		providers.KindGoogle,
	} {
		t.Run(kind, func(t *testing.T) {
			f := pabrikUji(t, kredensialUji(), nil)
			p, err := f.Provider(context.Background(), kandidatUji(kind, srv.URL))
			if err != nil {
				t.Fatalf("Provider: %v", err)
			}
			if p.Kind() != kind {
				t.Errorf("Kind() = %q, mau %q", p.Kind(), kind)
			}
			if p.Name() != "uji" {
				t.Errorf("Name() = %q, mau %q", p.Name(), "uji")
			}
		})
	}
}

// TestFactoryKindTakDikenalDitolak menjaga agar kind yang tidak dikenal TIDAK jatuh ke
// adapter openai. Kalau jatuh, body bergaya OpenAI dikirim ke upstream yang bicara dialek
// lain, dan 400 yang kembali tampak seperti masalah permintaan pengguna.
func TestFactoryKindTakDikenalDitolak(t *testing.T) {
	f := pabrikUji(t, kredensialUji(), nil)
	for _, kind := range []string{"bedrock", "vertex", "", "OPENAI"} {
		if _, err := f.Provider(context.Background(), kandidatUji(kind, "http://127.0.0.1:1")); err == nil {
			t.Errorf("kind %q diterima, mau ditolak", kind)
		}
	}
}

func TestFactoryKandidatKosong(t *testing.T) {
	f := pabrikUji(t, kredensialUji(), nil)
	if _, err := f.Provider(context.Background(), nil); err == nil {
		t.Error("kandidat nil diterima, mau ditolak")
	}
	c := kandidatUji(providers.KindOpenAI, "http://127.0.0.1:1")
	c.ProviderID = ""
	if _, err := f.Provider(context.Background(), c); err == nil {
		t.Error("kandidat tanpa ID provider diterima, mau ditolak")
	}
}

// --- Cache -------------------------------------------------------------------

func TestFactoryCacheMengembalikanInstanceSama(t *testing.T) {
	srv := serverUji(t)
	creds := kredensialUji()
	f := pabrikUji(t, creds, nil)
	c := kandidatUji(providers.KindOpenAI, srv.URL)

	pertama, err := f.Provider(context.Background(), c)
	if err != nil {
		t.Fatalf("Provider pertama: %v", err)
	}
	for i := 0; i < 5; i++ {
		lagi, err := f.Provider(context.Background(), c)
		if err != nil {
			t.Fatalf("Provider ke-%d: %v", i+2, err)
		}
		// Pembandingan antarmuka membandingkan tipe dinamis DAN pointernya, jadi ini benar
		// benar memeriksa instance yang sama — yaitu http.Transport yang sama, yaitu
		// connection pool dan sesi TLS yang sama.
		if lagi != pertama {
			t.Fatalf("permintaan ke-%d mendapat adapter baru; cache tidak bekerja", i+2)
		}
	}
	if n := len(f.cache); n != 1 {
		t.Errorf("jumlah entri cache = %d, mau 1", n)
	}
}

func TestFactoryCacheDibatalkanSaatBentukAdapterBerubah(t *testing.T) {
	srv := serverUji(t)
	srvLain := serverUji(t)

	for _, tc := range []struct {
		nama   string
		ubah   func(c *upstream.RouteCandidate, creds *sumberKredensial)
		alasan string
	}{
		{
			"nilai kredensial berganti",
			func(_ *upstream.RouteCandidate, creds *sumberKredensial) {
				// Baris yang sama, isi yang berbeda: operator mengganti kunci tanpa
				// mengganti barisnya. Kunci cache yang hanya memuat ID akan melewatkan ini.
				creds.ganti("cred-1", security.Secret("sk-kunci-yang-baru"))
			},
			"gateway akan terus memakai kunci lama sampai proses direstart",
		},
		{
			"baris kredensial berganti",
			func(_ *upstream.RouteCandidate, creds *sumberKredensial) {
				creds.ganti("cred-2", security.Secret("sk-kunci-lain"))
			},
			"kredensial yang sedang ditolak upstream akan terus dipakai",
		},
		{
			"base URL berganti",
			func(c *upstream.RouteCandidate, _ *sumberKredensial) { c.BaseURL = srvLain.URL },
			"permintaan tetap pergi ke endpoint lama",
		},
		{
			"timeout berganti",
			func(c *upstream.RouteCandidate, _ *sumberKredensial) { c.TimeoutMS = 9000 },
			"perubahan konfigurasi tampak tersimpan tetapi tidak pernah berlaku",
		},
	} {
		t.Run(tc.nama, func(t *testing.T) {
			creds := kredensialUji()
			f := pabrikUji(t, creds, nil)
			c := kandidatUji(providers.KindOpenAI, srv.URL)

			sebelum, err := f.Provider(context.Background(), c)
			if err != nil {
				t.Fatalf("Provider sebelum: %v", err)
			}
			tc.ubah(c, creds)
			sesudah, err := f.Provider(context.Background(), c)
			if err != nil {
				t.Fatalf("Provider sesudah: %v", err)
			}
			if sesudah == sebelum {
				t.Errorf("adapter tidak dibuat ulang setelah %s; akibatnya: %s", tc.nama, tc.alasan)
			}
		})
	}
}

// TestFactoryCacheDibatalkanSaatEgressBerubah dipisah karena butuh sumber egress.
func TestFactoryCacheDibatalkanSaatEgressBerubah(t *testing.T) {
	srv := serverUji(t)
	creds := kredensialUji()
	eg := &sumberEgress{url: security.Secret("http://proxy-satu.internal:3128")}
	f := pabrikUji(t, creds, eg)

	pool := "8f1f0b6e-0000-4000-8000-00000000e001"
	c := kandidatUji(providers.KindOpenAI, srv.URL)
	c.EgressPoolID = &pool

	sebelum, err := f.Provider(context.Background(), c)
	if err != nil {
		t.Fatalf("Provider sebelum: %v", err)
	}
	if got := eg.diminta(); len(got) != 1 || got[0] != pool {
		t.Errorf("egress pool yang diminta = %v, mau [%s]", got, pool)
	}

	eg.mu.Lock()
	eg.url = security.Secret("http://proxy-dua.internal:3128")
	eg.mu.Unlock()

	sesudah, err := f.Provider(context.Background(), c)
	if err != nil {
		t.Fatalf("Provider sesudah: %v", err)
	}
	if sesudah == sebelum {
		t.Error("adapter tidak dibuat ulang setelah URL proxy berganti; lalu lintas tetap keluar lewat jalur lama")
	}
}

// TestFactoryEgressBenarBenarSampaiKeTransport membuktikan URL proxy tidak hanya masuk kunci
// cache, tetapi benar-benar dipakai membentuk transport: skema proxy yang tidak didukung
// ditolak oleh pembentuk transport, bukan diabaikan.
func TestFactoryEgressBenarBenarSampaiKeTransport(t *testing.T) {
	srv := serverUji(t)
	eg := &sumberEgress{url: security.Secret("gopher://proxy.internal:70")}
	f := pabrikUji(t, kredensialUji(), eg)

	pool := "8f1f0b6e-0000-4000-8000-00000000e002"
	c := kandidatUji(providers.KindOpenAI, srv.URL)
	c.EgressPoolID = &pool

	if _, err := f.Provider(context.Background(), c); err == nil {
		t.Fatal("URL proxy berskema tak didukung diterima; berarti ProxyURL tidak sampai ke transport")
	}
}

func TestFactoryEgressTanpaSumberDitolak(t *testing.T) {
	srv := serverUji(t)
	f := pabrikUji(t, kredensialUji(), nil)

	pool := "8f1f0b6e-0000-4000-8000-00000000e003"
	c := kandidatUji(providers.KindOpenAI, srv.URL)
	c.EgressPoolID = &pool

	// Jalur keluar yang berbeda dari yang diminta operator adalah kegagalan kebijakan,
	// bukan penyesuaian yang boleh dilakukan diam-diam.
	if _, err := f.Provider(context.Background(), c); err == nil {
		t.Error("kandidat beregress pool diterima tanpa sumber egress")
	}
}

func TestFactoryBatasCacheDihormati(t *testing.T) {
	srv := serverUji(t)
	f := pabrikUji(t, kredensialUji(), nil, WithAdapterCacheSize(2))

	for i, ms := range []int{1000, 2000, 3000, 4000} {
		c := kandidatUji(providers.KindOpenAI, srv.URL)
		c.TimeoutMS = ms
		if _, err := f.Provider(context.Background(), c); err != nil {
			t.Fatalf("Provider ke-%d: %v", i+1, err)
		}
	}
	if n := len(f.cache); n > 2 {
		t.Errorf("jumlah entri cache = %d, mau paling banyak 2", n)
	}
	if got := f.jumlah.Load(); got != int64(len(f.cache)) {
		t.Errorf("penghitung atomik = %d, isi peta = %d", got, len(f.cache))
	}
}

// --- Kredensial --------------------------------------------------------------

func TestFactoryKredensialOpsionalPerKind(t *testing.T) {
	srv := serverUji(t)

	for _, tc := range []struct {
		kind     string
		mauGagal bool
	}{
		// Server yang dijalankan sendiri umumnya tanpa autentikasi. Menuntut kredensial di
		// sana hanya membuat operator mengisi nilai palsu supaya lolos validasi.
		{providers.KindOpenAICompatible, false},
		{providers.KindCustom, false},
		// Ketiga ini selalu menuntut kredensial; yang hilang berarti salah konfigurasi yang
		// harus terlihat sekarang, bukan 401 dari upstream nanti.
		{providers.KindOpenAI, true},
		{providers.KindAnthropic, true},
		{providers.KindGoogle, true},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			// Dua bentuk yang sama-sama harus dikenali: sentinel apa adanya dan sentinel
			// yang sudah dibungkus, karena repo membungkus errornya dengan konteks operasi.
			for _, err := range []error{repo.ErrNotFound, fmt.Errorf("mengambil kredensial: %w", repo.ErrNotFound)} {
				creds := &sumberKredensial{err: err}
				f := pabrikUji(t, creds, nil)
				p, gagal := f.Provider(context.Background(), kandidatUji(tc.kind, srv.URL))
				switch {
				case tc.mauGagal && gagal == nil:
					t.Errorf("kind %s berhasil tanpa kredensial, mau gagal", tc.kind)
				case !tc.mauGagal && gagal != nil:
					t.Errorf("kind %s gagal tanpa kredensial: %v", tc.kind, gagal)
				case !tc.mauGagal && p == nil:
					t.Errorf("kind %s mengembalikan provider nil tanpa error", tc.kind)
				}
			}
		})
	}
}

func TestFactoryTanpaSumberKredensial(t *testing.T) {
	srv := serverUji(t)
	f := pabrikUji(t, nil, nil)

	if _, err := f.Provider(context.Background(), kandidatUji(providers.KindOpenAICompatible, srv.URL)); err != nil {
		t.Errorf("kind openai_compatible gagal tanpa sumber kredensial: %v", err)
	}
	if _, err := f.Provider(context.Background(), kandidatUji(providers.KindOpenAI, srv.URL)); err == nil {
		t.Error("kind openai berhasil tanpa sumber kredensial, mau gagal")
	}
}

// TestFactoryKegagalanSumberKredensialDiteruskan memastikan kegagalan yang BUKAN
// "tidak ditemukan" tidak diperlakukan sebagai provider tanpa autentikasi. Kunci yang gagal
// didekripsi karena rotasi yang belum selesai tidak boleh berubah menjadi permintaan tanpa
// kredensial ke upstream.
func TestFactoryKegagalanSumberKredensialDiteruskan(t *testing.T) {
	srv := serverUji(t)
	creds := &sumberKredensial{err: errors.New("kunci enkripsi tidak cocok")}
	f := pabrikUji(t, creds, nil)

	for _, kind := range []string{providers.KindOpenAI, providers.KindOpenAICompatible, providers.KindCustom} {
		if _, err := f.Provider(context.Background(), kandidatUji(kind, srv.URL)); err == nil {
			t.Errorf("kind %s: kegagalan dekripsi diperlakukan sebagai tanpa kredensial", kind)
		}
	}
}

// --- Keamanan dipakai bersamaan ----------------------------------------------

func TestFactoryAmanDipakaiBersamaan(t *testing.T) {
	srv := serverUji(t)
	creds := kredensialUji()
	f := pabrikUji(t, creds, nil)

	const goroutine = 16
	const putaran = 25

	berhenti := make(chan struct{})
	var pengganti sync.WaitGroup
	pengganti.Add(1)
	go func() {
		// Kredensial berganti terus-menerus selagi adapter diminta: inilah keadaan yang
		// membuat cache dan pembuatan adapter berjalan bersamaan.
		defer pengganti.Done()
		for i := 0; ; i++ {
			select {
			case <-berhenti:
				return
			default:
			}
			creds.ganti(fmt.Sprintf("cred-%d", i%3), security.Secret(fmt.Sprintf("sk-%d", i%3)))
		}
	}()

	var wg sync.WaitGroup
	gagal := make(chan error, goroutine*putaran)
	for i := 0; i < goroutine; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < putaran; j++ {
				p, err := f.Provider(context.Background(), kandidatUji(providers.KindOpenAI, srv.URL))
				if err != nil {
					gagal <- err
					return
				}
				if p == nil {
					gagal <- errors.New("provider nil tanpa error")
					return
				}
			}
		}()
	}
	wg.Wait()
	close(berhenti)
	pengganti.Wait()
	close(gagal)

	for err := range gagal {
		t.Errorf("Provider gagal saat dipakai bersamaan: %v", err)
	}
	if n := len(f.cache); n > f.maks {
		t.Errorf("jumlah entri cache = %d, melewati batas %d", n, f.maks)
	}
}

// --- Redaksi -----------------------------------------------------------------

// TestFactoryTidakMembocorkanKredensial memeriksa keempat jalur fmt sekaligus.
//
// Empat verb diperiksa, bukan satu, karena keduanya menempuh jalur berbeda di fmt: %v dan
// %s lewat Stringer, %#v lewat GoStringer, dan %+v pada struct tanpa Stringer akan
// mencetak seluruh field beserta isinya. Satu jalur yang lupa ditutup cukup untuk
// menempatkan kredensial upstream di log.
func TestFactoryTidakMembocorkanKredensial(t *testing.T) {
	srv := serverUji(t)
	creds := kredensialUji()
	eg := &sumberEgress{url: security.Secret("http://pengguna:sandiproxy@proxy.internal:3128")}
	f := pabrikUji(t, creds, eg)

	pool := "8f1f0b6e-0000-4000-8000-00000000e004"
	c := kandidatUji(providers.KindOpenAI, srv.URL)
	c.EgressPoolID = &pool
	p, err := f.Provider(context.Background(), c)
	if err != nil {
		t.Fatalf("Provider: %v", err)
	}

	dilarang := []string{rahasiaUji, "sandiproxy"}
	cetakan := map[string]string{
		"%v pabrik":  fmt.Sprintf("%v", f),
		"%+v pabrik": fmt.Sprintf("%+v", f),
		"%#v pabrik": fmt.Sprintf("%#v", f),
		//lint:ignore S1025 pengujian sengaja memverifikasi format verb %s untuk redaksi rahasia
		"%s pabrik":       fmt.Sprintf("%s", f),
		"%v isi cache":    fmt.Sprintf("%v", f.cache),
		"%+v isi cache":   fmt.Sprintf("%+v", f.cache),
		"%#v isi cache":   fmt.Sprintf("%#v", f.cache),
		"%v adapter":      fmt.Sprintf("%v", p),
		"%+v adapter":     fmt.Sprintf("%+v", p),
		"%#v adapter":     fmt.Sprintf("%#v", p),
		"%s adapter":      fmt.Sprintf("%s", p),
		"kunci cache":     strings.Join(kunciCache(f), " "),
		"LogValue pabrik": fmt.Sprintf("%v", f.LogValue()),
	}
	for nama, isi := range cetakan {
		for _, r := range dilarang {
			if strings.Contains(isi, r) {
				t.Errorf("%s membocorkan rahasia: %s", nama, isi)
			}
		}
	}
}

// kunciCache mengembalikan seluruh kunci cache untuk diperiksa.
func kunciCache(f *Factory) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, 0, len(f.cache))
	for k := range f.cache {
		out = append(out, k)
	}
	return out
}

// --- Kebijakan SSRF ----------------------------------------------------------

// TestFactoryMemakaiKebijakanSSRFMiliknya menutup lubang yang paling penting di berkas ini:
// adapter yang dibuat dengan kebijakan kosong berarti base URL BYOK bisa menunjuk ke alamat
// internal.
//
// Dua arah diperiksa. Dengan kebijakan bawaan yang ketat, base URL loopback harus DITOLAK —
// membuktikan kebijakan pabrik dipakai saat memvalidasi. Dengan kebijakan yang mengecualikan
// loopback, permintaan sungguhan ke server tiruan harus BERHASIL — membuktikan pengecualian
// itu ikut sampai ke dialer adapter, bukan hanya ke validasinya.
func TestFactoryMemakaiKebijakanSSRFMiliknya(t *testing.T) {
	srv := serverUji(t)

	t.Run("kebijakan ketat menolak loopback", func(t *testing.T) {
		f := NewFactory(kredensialUji(), nil, security.DefaultSSRFPolicy(), loggerSenyap())
		if _, err := f.Provider(context.Background(), kandidatUji(providers.KindOpenAI, srv.URL)); err == nil {
			t.Fatal("base URL loopback diterima dengan kebijakan bawaan; kebijakan pabrik tidak dipakai")
		}
	})

	t.Run("pengecualian sampai ke dialer", func(t *testing.T) {
		f := pabrikUji(t, kredensialUji(), nil)
		p, err := f.Provider(context.Background(), kandidatUji(providers.KindOpenAI, srv.URL))
		if err != nil {
			t.Fatalf("Provider: %v", err)
		}
		// Permintaan sungguhan: kalau adapter dibuat dengan kebijakan kosong, dial ke
		// 127.0.0.1 diblokir dan panggilan ini gagal.
		resp, err := p.ChatCompletion(context.Background(), &providers.ChatRequest{
			Model:    "m",
			Messages: []providers.Message{{Role: providers.RoleUser, Content: "hai"}},
		})
		if err != nil {
			t.Fatalf("ChatCompletion lewat adapter dari pabrik: %v", err)
		}
		if len(resp.Choices) != 1 {
			t.Errorf("jumlah choice = %d, mau 1", len(resp.Choices))
		}
	})
}

// TestFactoryKredensialDiambilSetiapPermintaan mencatat sifat yang dipilih sengaja: cache
// menyimpan ADAPTER, bukan kredensial. Satu query per permintaan adalah harga yang dibayar
// supaya rotasi kredensial berlaku pada permintaan berikutnya, bukan setelah restart.
func TestFactoryKredensialDiambilSetiapPermintaan(t *testing.T) {
	srv := serverUji(t)
	creds := kredensialUji()
	f := pabrikUji(t, creds, nil)
	c := kandidatUji(providers.KindOpenAI, srv.URL)

	for i := 0; i < 3; i++ {
		if _, err := f.Provider(context.Background(), c); err != nil {
			t.Fatalf("Provider ke-%d: %v", i+1, err)
		}
	}
	if got := creds.jumlahPanggilan(); got != 3 {
		t.Errorf("jumlah panggilan Active = %d, mau 3", got)
	}
}

// --- Penandaan pemakaian kredensial ------------------------------------------

// penandaTiruan mencatat kredensial yang ditandai terpakai.
type penandaTiruan struct {
	mu sync.Mutex
	id []string
	// selesai ditutup setiap kali satu penandaan selesai, supaya test tidak perlu tidur.
	selesai chan struct{}
	err     error
}

func (p *penandaTiruan) MarkUsed(_ context.Context, id string) error {
	p.mu.Lock()
	p.id = append(p.id, id)
	p.mu.Unlock()
	if p.selesai != nil {
		p.selesai <- struct{}{}
	}
	return p.err
}

func (p *penandaTiruan) semua() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.id...)
}

// tungguPenandaan menunggu satu penandaan selesai, atau menggagalkan test.
func tungguPenandaan(t *testing.T, p *penandaTiruan) {
	t.Helper()
	select {
	case <-p.selesai:
	case <-time.After(3 * time.Second):
		t.Fatal("penandaan pemakaian kredensial tidak pernah berjalan")
	}
}

func TestFactoryMenandaiPemakaianKredensialDenganJeda(t *testing.T) {
	srv := serverUji(t)
	tanda := &penandaTiruan{selesai: make(chan struct{}, 8)}
	f := pabrikUji(t, kredensialUji(), nil, WithCredentialUseMarker(tanda))

	for range 5 {
		if _, err := f.Provider(context.Background(), kandidatUji(providers.KindOpenAI, srv.URL)); err != nil {
			t.Fatalf("Provider: %v", err)
		}
	}
	tungguPenandaan(t, tanda)

	// Lima permintaan, satu penandaan: jedanya yang menahan sisanya. Tanpa jeda, setiap
	// permintaan inference menghasilkan satu UPDATE ke baris yang sama.
	if got := tanda.semua(); len(got) != 1 {
		t.Fatalf("penandaan = %v, mau tepat satu di dalam satu jeda", got)
	}

	// Setelah jedanya lewat, penandaan berikutnya boleh jalan — itulah yang membuat beberapa
	// kredensial pada satu provider benar-benar bergiliran.
	f.tandai.mu.Lock()
	f.tandai.akhir["cred-1"] = time.Now().Add(-2 * jedaTandaiPemakaian)
	f.tandai.mu.Unlock()

	if _, err := f.Provider(context.Background(), kandidatUji(providers.KindOpenAI, srv.URL)); err != nil {
		t.Fatalf("Provider setelah jeda: %v", err)
	}
	tungguPenandaan(t, tanda)
	if got := tanda.semua(); len(got) != 2 {
		t.Fatalf("penandaan setelah jeda = %v, mau dua", got)
	}
}

func TestFactoryTanpaPenandaTidakMenulisApaPun(t *testing.T) {
	srv := serverUji(t)
	f := pabrikUji(t, kredensialUji(), nil)

	if _, err := f.Provider(context.Background(), kandidatUji(providers.KindOpenAI, srv.URL)); err != nil {
		t.Fatalf("Provider: %v", err)
	}
	// Tanpa penanda, last_used_at tidak pernah ditulis dan Active terus memilih kredensial
	// yang sama. Itu keadaan yang sah, dan yang penting ia tidak nil-pointer.
	if f.tandai != nil {
		t.Error("penanda terpasang tanpa diminta")
	}
}

// Kegagalan penandaan tidak boleh berakibat apa pun pada permintaan: yang hilang hanya
// ketepatan urutan giliran kredensial.
func TestFactoryKegagalanPenandaanTidakMenggagalkanPermintaan(t *testing.T) {
	srv := serverUji(t)
	tanda := &penandaTiruan{selesai: make(chan struct{}, 4), err: errors.New("baris kredensial sudah dihapus")}
	f := pabrikUji(t, kredensialUji(), nil, WithCredentialUseMarker(tanda))

	if _, err := f.Provider(context.Background(), kandidatUji(providers.KindOpenAI, srv.URL)); err != nil {
		t.Fatalf("Provider: %v", err)
	}
	tungguPenandaan(t, tanda)
}

// TestFactoryProviderForHealthCheckBerbagiCacheDenganKandidat memastikan pemanggilan ProviderFor
// langsung dari baris tabel providers (tanpa pemetaan model) menghasilkan adapter yang valid
// dan memanfaatkan cache yang sama dengan pemanggilan kandidat rute.
func TestFactoryProviderForHealthCheckBerbagiCacheDenganKandidat(t *testing.T) {
	srv := serverUji(t)
	f := pabrikUji(t, kredensialUji(), nil)

	p := &upstream.Provider{
		ID:        "p-1",
		Name:      "test-prov",
		Kind:      providers.KindOpenAI,
		BaseURL:   srv.URL,
		TimeoutMS: 5000,
	}

	// 1. Ambil adapter lewat ProviderFor (misalnya dari health checker).
	adapter1, err := f.ProviderFor(context.Background(), p)
	if err != nil {
		t.Fatalf("ProviderFor gagal: %v", err)
	}
	if adapter1.Name() != "test-prov" {
		t.Errorf("adapter Name() = %q, mau test-prov", adapter1.Name())
	}

	// 2. Ambil adapter untuk provider yang sama lewat kandidat rute.
	cand := &upstream.RouteCandidate{
		ProviderID:   "p-1",
		ProviderName: "test-prov",
		Kind:         providers.KindOpenAI,
		BaseURL:      srv.URL,
		TimeoutMS:    5000,
	}
	adapter2, err := f.Provider(context.Background(), cand)
	if err != nil {
		t.Fatalf("Provider gagal: %v", err)
	}

	// Harus instance adapter yang identik dari cache.
	if adapter1 != adapter2 {
		t.Errorf("adapter dari ProviderFor dan Provider seharusnya berbagi entri cache yang sama")
	}

	// Provider kosong harus ditolak secara bersih.
	if _, err := f.ProviderFor(context.Background(), nil); err == nil {
		t.Error("ProviderFor dengan provider nil seharusnya gagal")
	}
}

// TestFactoryBYOKSelaluKebijakanKetat memastikan provider BYOK tidak mewarisi
// pelonggaran SSRF operator: pabrikUji memakai kebijakan yang mengecualikan
// loopback (untuk Ollama lokal operator), tetapi kandidat BYOK ke loopback
// harus tetap DITOLAK karena BYOK selalu memakai DefaultSSRFPolicy.
func TestFactoryBYOKSelaluKebijakanKetat(t *testing.T) {
	srv := serverUji(t)
	f := pabrikUji(t, kredensialUji(), nil)

	// Non-BYOK ke loopback: lolos, karena operator mengecualikannya.
	biasa := kandidatUji(providers.KindOpenAI, srv.URL)
	if _, err := f.Provider(context.Background(), biasa); err != nil {
		t.Fatalf("provider operator ke loopback ditolak, padahal dikecualikan: %v", err)
	}

	// BYOK ke loopback yang sama: DITOLAK.
	byok := kandidatUji(providers.KindOpenAI, srv.URL)
	byok.IsBYOK = true
	if _, err := f.Provider(context.Background(), byok); err == nil {
		t.Error("provider BYOK ke loopback diterima; harus ditolak dengan kebijakan ketat")
	}

	// BYOK tidak boleh memakai egress pool operator.
	pool := "8f1f0b6e-0000-4000-8000-00000000e099"
	byokEgress := kandidatUji(providers.KindOpenAI, "https://api.example.com/v1")
	byokEgress.IsBYOK = true
	byokEgress.EgressPoolID = &pool
	if _, err := f.Provider(context.Background(), byokEgress); err == nil {
		t.Error("provider BYOK beregress pool diterima; harus ditolak")
	}
}
