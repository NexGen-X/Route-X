// Package gateway memuat perkakas jalur request. Berkas ini adalah pabrik adapter: mengubah
// satu upstream.RouteCandidate menjadi providers.Provider yang siap dipakai.
//
// # Kenapa ada cache di sini
//
// Membuat adapter berarti membuat http.Client beserta http.Transport-nya. Transport ADALAH
// tempat connection pool dan cache sesi TLS tinggal, jadi adapter baru per permintaan
// berarti kolam koneksi kosong per permintaan: setiap permintaan inference membayar TCP
// handshake ditambah TLS handshake penuh ke upstream, dan koneksi yang baru dibuat itu
// langsung dibuang bersama adapternya. Untuk gateway yang menghubungi host yang sama
// berulang-ulang, itu latensi yang bisa dihilangkan seluruhnya.
//
// # Kenapa cache-nya tidak boleh sekadar per provider
//
// Adapter yang sudah jadi MEMBEKUKAN kredensial, base URL, timeout, dan egress proxy ke
// dalam dirinya — semuanya dipakai saat menyusun klien dan header, bukan dibaca ulang per
// permintaan. Karena itu kuncinya harus memuat semua itu, bukan hanya ID provider. Yang
// terjadi kalau salah satu terlewat, dan semuanya senyap:
//
//   - kredensial terlewat: setelah rotasi, gateway terus mengirim kunci yang sudah dicabut
//     sampai proses direstart, dan setiap permintaan gagal 401 dari provider yang sehat;
//   - base URL terlewat: permintaan tetap pergi ke endpoint lama setelah operator
//     memindahkannya;
//   - timeout terlewat: perubahan konfigurasi tampak tersimpan tetapi tidak pernah berlaku;
//   - egress pool terlewat: lalu lintas tetap keluar lewat jalur lama, yang untuk sebagian
//     penyewa adalah alasan pool itu dibuat.
//
// Kunci memuat SIDIK JARI, bukan nilainya: kredensial dan URL proxy masuk sebagai bagian
// dari satu SHA-256, karena kunci cache ikut tercetak di log dan metrik saat ada yang
// menyelidiki perilaku cache.
package gateway

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/NexGen-X/Route-X/internal/database/repo"
	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
	"github.com/NexGen-X/Route-X/internal/providers"
	"github.com/NexGen-X/Route-X/internal/providers/anthropic"
	"github.com/NexGen-X/Route-X/internal/providers/google"
	"github.com/NexGen-X/Route-X/internal/providers/openai"
	"github.com/NexGen-X/Route-X/internal/security"
)

// defaultAdapterCacheSize membatasi jumlah adapter yang ditahan.
//
// Batasnya ada karena kunci memuat kredensial: provider dengan beberapa kredensial yang
// dipakai bergiliran menghasilkan satu entri per kredensial, dan kredensial yang dihapus
// meninggalkan entri yang tidak akan pernah dipakai lagi. Tanpa batas, proses yang berjalan
// berbulan-bulan menyimpan seluruh riwayat konfigurasi beserta koneksi menganggurnya.
//
// 128 dipilih longgar: satu instalasi besar pun jarang punya lebih dari beberapa puluh
// pemetaan provider aktif, jadi batas ini hanya menyentuh entri yang memang sudah usang.
const defaultAdapterCacheSize = 128

// CredentialSource memasok kredensial aktif satu provider. Dipenuhi *upstream.CredentialRepo.
type CredentialSource interface {
	Active(ctx context.Context, providerID string) (*upstream.ActiveCredential, error)
}

// EgressSource memasok URL proxy satu egress pool. Dipenuhi *upstream.EgressRepo.
type EgressSource interface {
	ProxyURL(ctx context.Context, id string) (security.Secret, error)
}

// Factory membuat dan menyimpan adapter provider.
//
// Aman dipakai bersamaan oleh banyak goroutine.
type Factory struct {
	creds  CredentialSource
	egress EgressSource
	policy security.SSRFPolicy
	logger *slog.Logger
	maks   int

	// jumlah adalah ukuran cache yang bisa dibaca tanpa lock. Ada supaya LogValue tidak
	// perlu mengambil mu: LogValue dipanggil dari dalam pemanggilan slog, dan slog bisa
	// dipanggil dari mana pun — termasuk, suatu hari, dari jalur yang sudah memegang mu.
	jumlah atomic.Int64

	mu    sync.Mutex
	cache map[string]*entriAdapter
	// tik adalah penghitung pemakaian, dipakai membuang entri yang paling lama tidak
	// dipakai. Penghitung, bukan waktu: dua pemakaian dalam milidetik yang sama harus tetap
	// bisa diurutkan, dan cache ini memang dipakai beberapa kali per milidetik.
	tik uint64
}

// entriAdapter adalah satu adapter yang disimpan beserta penanda pemakaian terakhirnya.
type entriAdapter struct {
	provider providers.Provider
	dipakai  uint64
}

// FactoryOption menyetel Factory saat dibuat.
type FactoryOption func(*Factory)

// WithAdapterCacheSize mengubah jumlah adapter yang ditahan. Nilai <= 0 diabaikan.
func WithAdapterCacheSize(n int) FactoryOption {
	return func(f *Factory) {
		if n > 0 {
			f.maks = n
		}
	}
}

// NewFactory membuat pabrik adapter.
//
// creds nil berarti tidak ada sumber kredensial sama sekali — sah hanya untuk kind yang
// boleh tanpa autentikasi. egress nil berarti kandidat yang menunjuk egress pool akan
// gagal, bukan diam-diam keluar lewat jalur bawaan: jalur keluar yang berbeda dari yang
// diminta operator adalah kegagalan kebijakan, bukan penyesuaian.
func NewFactory(
	creds CredentialSource,
	egress EgressSource,
	policy security.SSRFPolicy,
	logger *slog.Logger,
	opts ...FactoryOption,
) *Factory {
	if logger == nil {
		logger = slog.Default()
	}
	f := &Factory{
		creds:  creds,
		egress: egress,
		policy: policy,
		logger: logger,
		maks:   defaultAdapterCacheSize,
		cache:  make(map[string]*entriAdapter),
	}
	for _, o := range opts {
		o(f)
	}
	return f
}

// Provider mengembalikan adapter siap pakai untuk kandidat ini.
//
// Error dari sini sampai ke pemanggil sebagai "kandidat ini tidak bisa dipakai" dan boleh
// dialihkan ke kandidat berikutnya — lihat Handlers.adapter, yang juga menahan sebab
// aslinya di log karena sebab itu bisa memuat nilai yang tidak boleh dilihat klien.
func (f *Factory) Provider(ctx context.Context, c *upstream.RouteCandidate) (providers.Provider, error) {
	if c == nil {
		return nil, errors.New("kandidat rute kosong")
	}
	if c.ProviderID == "" {
		return nil, errors.New("kandidat rute tanpa ID provider")
	}
	if !dilayani(c.Kind) {
		// Kind tak dikenal adalah error, bukan jatuh ke openai: mengirim body bergaya
		// OpenAI ke upstream yang bicara dialek lain menghasilkan 400 yang tampak seperti
		// masalah permintaan pengguna, padahal ini salah konfigurasi provider.
		return nil, fmt.Errorf("kind provider %q tidak dilayani gateway ini", c.Kind)
	}

	cred, err := f.kredensial(ctx, c)
	if err != nil {
		return nil, err
	}
	proxy, err := f.proxy(ctx, c)
	if err != nil {
		return nil, err
	}

	kunci := kunciAdapter(c, cred, proxy)
	if p, ok := f.dariCache(kunci); ok {
		return p, nil
	}

	p, err := f.buat(c, cred, proxy)
	if err != nil {
		return nil, err
	}
	return f.keCache(kunci, p), nil
}

// kredensial mengambil kredensial aktif provider, atau nil bila kind ini boleh tanpanya.
func (f *Factory) kredensial(ctx context.Context, c *upstream.RouteCandidate) (*upstream.ActiveCredential, error) {
	if f.creds == nil {
		if kredensialOpsional(c.Kind) {
			return nil, nil
		}
		return nil, fmt.Errorf("provider %q butuh kredensial tetapi pabrik dibuat tanpa sumber kredensial", c.ProviderName)
	}

	// Active, bukan Reveal: Active-lah yang memilih kredensial mana yang aktif — yang paling
	// sedikit gagal autentikasi lebih dulu — sehingga kunci yang sedang ditolak upstream
	// tidak dicoba terus-menerus selama masih ada yang lain.
	cred, err := f.creds.Active(ctx, c.ProviderID)
	if err == nil {
		return cred, nil
	}
	if errors.Is(err, repo.ErrNotFound) && kredensialOpsional(c.Kind) {
		// Server berdialek OpenAI yang dijalankan sendiri (Ollama, vLLM, LM Studio) umumnya
		// berjalan tanpa autentikasi. Menuntut kredensial di sana hanya membuat operator
		// mengisi nilai palsu supaya lolos.
		return nil, nil
	}
	if errors.Is(err, repo.ErrNotFound) {
		// Pesan sendiri, bukan errornya: "data tidak ditemukan" tidak memberi tahu operator
		// data apa yang mana.
		return nil, fmt.Errorf("provider %q tidak punya satu pun kredensial aktif", c.ProviderName)
	}
	return nil, fmt.Errorf("kredensial provider %q tidak bisa diambil: %w", c.ProviderName, err)
}

// proxy mengambil URL egress proxy kandidat ini, kosong bila tidak memakai egress pool.
func (f *Factory) proxy(ctx context.Context, c *upstream.RouteCandidate) (security.Secret, error) {
	if c.EgressPoolID == nil || *c.EgressPoolID == "" {
		return "", nil
	}
	if f.egress == nil {
		return "", fmt.Errorf("provider %q memakai egress pool tetapi pabrik dibuat tanpa sumbernya", c.ProviderName)
	}
	url, err := f.egress.ProxyURL(ctx, *c.EgressPoolID)
	if err != nil {
		return "", fmt.Errorf("URL egress provider %q tidak bisa diambil: %w", c.ProviderName, err)
	}
	return url, nil
}

// kunciAdapter menyusun kunci cache satu adapter.
//
// Seluruh bahan yang dibekukan ke dalam adapter masuk ke SHA-256, lalu ID provider
// ditempelkan di depan sebagai bagian yang boleh dibaca manusia. Nilai kredensial dan URL
// proxy karena itu tidak pernah muncul apa adanya, sementara perubahan sekecil apa pun pada
// keduanya tetap menghasilkan kunci yang berbeda.
//
// Kredensial diwakili ID barisnya DAN sidik jari nilainya. ID saja tidak cukup: operator
// bisa mengganti isi kredensial tanpa mengganti barisnya, dan gateway akan terus memakai
// nilai lama sampai proses direstart.
func kunciAdapter(c *upstream.RouteCandidate, cred *upstream.ActiveCredential, proxy security.Secret) string {
	h := sha256.New()
	bagian := func(s string) {
		// Panjang ikut ditulis supaya dua rangkaian bagian yang berbeda tidak bisa
		// menghasilkan byte yang sama. Base URL boleh memuat karakter apa pun, termasuk
		// pemisah apa pun yang bisa kami pilih.
		_, _ = h.Write([]byte(strconv.Itoa(len(s))))
		_, _ = h.Write([]byte{0})
		_, _ = h.Write([]byte(s))
	}

	bagian(c.Kind)
	bagian(c.BaseURL)
	bagian(strconv.Itoa(c.TimeoutMS))
	if cred != nil {
		bagian(cred.ID)
		bagian(cred.Secret.Reveal())
	} else {
		bagian("")
		bagian("")
	}
	if c.EgressPoolID != nil {
		bagian(*c.EgressPoolID)
	} else {
		bagian("")
	}
	bagian(proxy.Reveal())

	return c.ProviderID + ":" + hex.EncodeToString(h.Sum(nil))
}

// buat membuat adapter baru untuk kandidat ini.
func (f *Factory) buat(
	c *upstream.RouteCandidate,
	cred *upstream.ActiveCredential,
	proxy security.Secret,
) (providers.Provider, error) {
	var rahasia security.Secret
	if cred != nil {
		rahasia = cred.Secret
	}
	timeout := time.Duration(c.TimeoutMS) * time.Millisecond

	switch c.Kind {
	case providers.KindOpenAI, providers.KindOpenAICompatible, providers.KindCustom:
		// Base URL divalidasi DI SINI karena openai.New sengaja tidak melakukannya — kontrak
		// providers.ClientConfig menyatakan pemanggil yang memvalidasi, dan pemanggil itu
		// adalah berkas ini. Justru tiga kind inilah yang base URL-nya berasal dari operator
		// atau dari pengguna BYOK, jadi ini bukan pemeriksaan formalitas: tanpa ini satu
		// baris provider bisa menunjuk ke alamat internal atau ke endpoint metadata cloud.
		//
		// Penjaga di lapisan dial tetap ada, tetapi ia memeriksa alamat yang BENAR-BENAR
		// didial — dan ketika lalu lintas lewat egress proxy, yang didial adalah proxy-nya,
		// bukan tujuan akhir. Untuk kandidat berproxy, pemeriksaan di sini adalah satu-satunya
		// yang melihat tujuan sebenarnya. Karena itu juga egress pool hanya boleh diisi
		// operator, bukan pengguna.
		if err := security.ValidateBaseURL(c.BaseURL, f.policy); err != nil {
			return nil, fmt.Errorf("base URL provider %q tidak sah: %w", c.ProviderName, err)
		}
		return openai.New(openai.Config{
			Name:       c.ProviderName,
			Kind:       c.Kind,
			BaseURL:    c.BaseURL,
			Credential: rahasia,
			Timeout:    timeout,
			SSRFPolicy: f.policy,
			ProxyURL:   proxy,
		})

	case providers.KindAnthropic:
		// anthropic.New dan google.New memanggil ValidateBaseURL sendiri, termasuk untuk base
		// URL bawaannya ketika kolomnya kosong. Memvalidasi dua kali tidak salah, tetapi
		// menyalin pemeriksaan yang sudah ada di tempatnya membuat dua tempat yang harus
		// berubah bersamaan.
		return anthropic.New(anthropic.Config{
			Name:       c.ProviderName,
			Kind:       c.Kind,
			BaseURL:    c.BaseURL,
			Credential: rahasia,
			Timeout:    timeout,
			SSRFPolicy: f.policy,
			ProxyURL:   proxy,
		})

	case providers.KindGoogle:
		return google.New(google.Config{
			Name:       c.ProviderName,
			Kind:       c.Kind,
			BaseURL:    c.BaseURL,
			Credential: rahasia,
			Timeout:    timeout,
			SSRFPolicy: f.policy,
			ProxyURL:   proxy,
		})

	default:
		return nil, fmt.Errorf("kind provider %q tidak dilayani gateway ini", c.Kind)
	}
}

// --- Cache -------------------------------------------------------------------

// dariCache mengambil adapter yang sudah ada dan menandainya baru dipakai.
func (f *Factory) dariCache(kunci string) (providers.Provider, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()

	e, ok := f.cache[kunci]
	if !ok {
		return nil, false
	}
	f.tik++
	e.dipakai = f.tik
	return e.provider, true
}

// keCache menyimpan adapter dan mengembalikan yang benar-benar dipakai.
//
// Nilai baliknya bisa BUKAN p: dua goroutine yang meminta kandidat yang sama pada saat yang
// sama akan membuat dua adapter, dan hanya satu yang boleh menang. Yang kalah dibuang di
// sini, bukan dikembalikan ke pemanggilnya — dua adapter untuk satu konfigurasi berarti dua
// connection pool yang saling tidak tahu, yaitu tepat pemborosan yang cache ini ada untuk
// mencegahnya.
func (f *Factory) keCache(kunci string, p providers.Provider) providers.Provider {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.tik++
	if e, ok := f.cache[kunci]; ok {
		e.dipakai = f.tik
		return e.provider
	}
	f.cache[kunci] = &entriAdapter{provider: p, dipakai: f.tik}
	f.buangYangUsang()
	f.jumlah.Store(int64(len(f.cache)))
	return p
}

// buangYangUsang menjaga ukuran cache. Pemanggil WAJIB memegang mu.
//
// Pemindaian linier, bukan struktur data terurut: batasnya puluhan sampai ratusan entri dan
// pemindaian hanya terjadi saat ada entri baru yang melewati batas. Daftar berkait yang
// dijaga di dua tempat akan lebih cepat dan jauh lebih mudah salah.
//
// Adapter yang dibuang tidak ditutup paksa karena kontrak providers.Provider tidak
// menyediakan jalan untuk itu; koneksi menganggurnya dilepas transport sendiri ketika
// IdleConnTimeout habis. Yang penting: tidak ada yang tertahan selamanya.
func (f *Factory) buangYangUsang() {
	for len(f.cache) > f.maks {
		var (
			kunciTertua string
			tertua      uint64
			pertama     = true
		)
		for k, e := range f.cache {
			if pertama || e.dipakai < tertua {
				kunciTertua, tertua, pertama = k, e.dipakai, false
			}
		}
		delete(f.cache, kunciTertua)
	}
}

// --- Redaksi -----------------------------------------------------------------

// String, GoString, dan LogValue wajib ada karena cache memuat adapter yang menyimpan
// security.Secret di field TAK DIEKSPOR. fmt tidak boleh memanggil metode pada field seperti
// itu, sehingga "%v" pada Factory akan mencetak isi rahasianya apa adanya tanpa ketiga metode
// ini. Dijaga TestRedaksiFieldTakDiekspor di internal/security.
//
// Receiver POINTER karena Factory memuat sync.Mutex: go vet menolak receiver nilai di sana,
// dan larangan yang sama itulah yang menutup jalur cetak-nilai — jadi pointer sekaligus
// wajib dan cukup.
func (f *Factory) String() string { return "gateway.Factory{…}" }

// GoString memenuhi fmt.GoStringer sehingga %#v juga tersamar.
func (f *Factory) GoString() string { return f.String() }

// LogValue melaporkan yang berguna untuk diagnosa cache, tanpa satu pun nilai rahasia.
//
// Ukuran cache dibaca dari penghitung atomik, bukan dari peta: LogValue dipanggil di dalam
// pemanggilan slog, dan mengambil mu di sini membuat setiap pencatatan log yang terjadi
// selagi mu dipegang menjadi kebuntuan.
func (f *Factory) LogValue() slog.Value {
	return slog.GroupValue(
		slog.Int64("adapter_ditahan", f.jumlah.Load()),
		slog.Int("batas_cache", f.maks),
	)
}

// String menyamarkan entri cache. Alasannya sama dengan Factory: provider di dalamnya
// menyimpan kredensial di field tak diekspor. Receiver NILAI karena tipe ini tidak memuat
// lock, dan receiver nilai juga menutup jalur cetak-nilai.
func (e entriAdapter) String() string { return "gateway.entriAdapter{…}" }

// GoString memenuhi fmt.GoStringer.
func (e entriAdapter) GoString() string { return e.String() }

// LogValue memenuhi slog.LogValuer.
func (e entriAdapter) LogValue() slog.Value { return slog.StringValue(e.String()) }

// --- Kind --------------------------------------------------------------------

// dilayani melaporkan apakah ada adapter untuk kind ini.
func dilayani(kind string) bool {
	switch kind {
	case providers.KindOpenAI, providers.KindOpenAICompatible, providers.KindCustom,
		providers.KindAnthropic, providers.KindGoogle:
		return true
	}
	return false
}

// kredensialOpsional melaporkan apakah kind ini sah dipakai tanpa kredensial.
//
// Hanya dua: openai_compatible dan custom. Keduanya menunjuk ke server yang dijalankan
// sendiri, dan server seperti itu umumnya tidak menuntut autentikasi sama sekali. Untuk
// openai, anthropic, dan google, kredensial yang hilang berarti salah konfigurasi yang
// harus terlihat sekarang, bukan 401 dari upstream nanti.
func kredensialOpsional(kind string) bool {
	return kind == providers.KindOpenAICompatible || kind == providers.KindCustom
}
