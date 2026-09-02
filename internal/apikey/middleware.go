package apikey

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/netip"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/NexGen-X/Route-X/internal/database/repo/keys"
	"github.com/NexGen-X/Route-X/internal/httpx"
	"github.com/NexGen-X/Route-X/internal/observability"
)

// Pesan yang boleh dilihat klien.
const (
	// MessageInvalidKey dipakai untuk SELURUH kegagalan autentikasi, apa pun sebabnya.
	// Keseragamannya bagian dari kontrak paket ini, bukan kebetulan — lihat
	// Authenticator.Authenticate.
	MessageInvalidKey = "API key tidak sah atau tidak disertakan"
	// MessageRateLimited dipakai saat batas laju terlampaui.
	MessageRateLimited = "batas laju terlampaui, coba lagi beberapa saat lagi"
)

// Alasan penolakan pada tahap pengambilan kredensial, yaitu sebelum database disentuh.
// Nilainya melengkapi keys.Reason* dan hanya dipakai sebagai label log dan metrik.
// Jumlahnya tetap, jadi kardinalitas metriknya aman.
const (
	ReasonMissingCredential     = "missing_credential"
	ReasonMalformedCredential   = "malformed_credential"
	ReasonConflictingCredential = "conflicting_credential"
)

// DefaultTouchInterval adalah selang minimum pembaruan last_used_at. Lihat
// Authenticator.touchIfStale untuk alasan angkanya.
const DefaultTouchInterval = 5 * time.Minute

// touchTimeout membatasi UPDATE last_used_at yang berjalan di latar. Nilainya pendek:
// pencatatan ini tidak berharga cukup untuk menahan koneksi database lama-lama.
const touchTimeout = 3 * time.Second

// KeyAuthenticator adalah bagian dari keys.Repo yang dipakai middleware ini.
//
// Antarmuka, bukan *keys.Repo langsung, karena dua alasan. Pertama, seluruh keputusan
// yang diuji di paket ini — bentuk respons, keseragaman body, pemilihan status —
// tidak bergantung pada PostgreSQL, jadi test yang memeriksanya tidak seharusnya
// membutuhkan database yang hidup. Kedua, ia mencatat dengan tepat apa yang dibutuhkan
// jalur autentikasi dari lapisan data: memverifikasi kredensial, dan mencatat pemakaian.
type KeyAuthenticator interface {
	// Authenticate memverifikasi key mentah. Kegagalan yang berarti "key tidak boleh
	// dipakai" mengembalikan *keys.Rejection.
	Authenticate(ctx context.Context, raw string, clientIP netip.Addr) (*keys.Key, error)
	// TouchUsage mencatat pemakaian terakhir.
	TouchUsage(ctx context.Context, id string, ip netip.Addr) error
}

// keys.Repo adalah implementasi produksinya.
var _ KeyAuthenticator = (*keys.Repo)(nil)

// Authenticator menegakkan autentikasi API key pada jalur /v1/*.
type Authenticator struct {
	repo   KeyAuthenticator
	logger *slog.Logger

	// rejected dipecah per alasan. Inilah satu-satunya tempat alasan penolakan bisa
	// dihitung, karena responsnya sengaja tidak membedakannya.
	rejected *prometheus.CounterVec

	touchInterval time.Duration
}

// Option menyetel Authenticator saat konstruksi.
type Option func(*Authenticator)

// WithTouchInterval mengubah selang minimum pembaruan last_used_at. Nilai <= 0
// mematikan pencatatan pemakaian sama sekali.
func WithTouchInterval(d time.Duration) Option {
	return func(a *Authenticator) { a.touchInterval = d }
}

// NewAuthenticator membuat middleware autentikasi.
//
// repo wajib ada. metrics boleh nil (penghitungnya tetap dibuat, hanya tidak terdaftar
// sehingga tidak terekspos di /metrics), dan logger nil jatuh ke slog.Default().
func NewAuthenticator(repo KeyAuthenticator, metrics *observability.Metrics, logger *slog.Logger, opts ...Option) *Authenticator {
	if logger == nil {
		logger = slog.Default()
	}
	a := &Authenticator{
		repo:          repo,
		logger:        logger,
		rejected:      registerAuthRejected(metrics),
		touchInterval: DefaultTouchInterval,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(a)
		}
	}
	return a
}

// registerAuthRejected mendaftarkan penghitung penolakan ke registry aplikasi.
//
// Metriknya didefinisikan di sini, bukan di observability.Metrics, karena alasan
// penolakan adalah urusan paket ini sendiri: observability tidak perlu tahu daftar
// alasannya, dan menambahkannya di sana berarti setiap perubahan daftar menyentuh dua
// paket. observability.Metrics.Registry() memang disediakan untuk keperluan ini.
func registerAuthRejected(m *observability.Metrics) *prometheus.CounterVec {
	c := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "routex_apikey_auth_rejected_total",
		Help: "Jumlah request yang gagal autentikasi API key, dipecah per alasan penolakan.",
	}, []string{"reason"})
	if m == nil {
		return c
	}
	if err := m.Registry().Register(c); err != nil {
		// Dua Authenticator di atas registry yang sama (mis. beberapa test dalam satu
		// proses) memakai penghitung yang sudah ada alih-alih panik.
		var already prometheus.AlreadyRegisteredError
		if errors.As(err, &already) {
			if existing, ok := already.ExistingCollector.(*prometheus.CounterVec); ok {
				return existing
			}
		}
		// Kegagalan lain tidak boleh menghentikan gateway: metrik yang hilang jauh
		// lebih ringan akibatnya daripada jalur autentikasi yang tidak bisa dirakit.
	}
	return c
}

// Authenticate mewajibkan API key yang berlaku pada setiap request.
//
// Alurnya: ambil kredensial dari header, tukar menjadi baris api_keys lewat repo dengan
// membawa IP klien, taruh *Principal di context, lalu catat pemakaian di latar.
//
// # Satu bentuk kegagalan untuk semua sebab
//
// Setiap kegagalan autentikasi membalas 401 dengan body yang identik byte per byte
// (kecuali request_id, yang memang unik per request). Key yang tidak ada, dinonaktifkan,
// dicabut, kedaluwarsa, datang dari IP di luar daftar putih, atau sedang diblokir —
// semuanya sama di mata klien. Alasan sebenarnya hanya masuk log dan metrik.
//
// Membedakannya akan mengubah endpoint ini menjadi alat pengintaian. "Key tidak
// ditemukan" versus "key sudah dicabut" memberi tahu pemegang key curian bahwa
// tebakannya PERNAH sah, yang berarti ia memegang nilai nyata dan hanya perlu mencari
// key lain dari sumber yang sama. Bagi pelanggan yang sah, perbedaan itu tidak berguna:
// yang bisa menindaklanjutinya adalah operator, dan operator membaca log.
//
// # Kenapa banned dan ip_not_allowed juga 401, bukan 403
//
// Keduanya terasa seperti 403 — kredensialnya dikenali, hanya keadaannya yang tidak
// mengizinkan. Justru itu masalahnya: 403 di sini adalah pengakuan bahwa key-nya sah.
//
// Untuk ip_not_allowed, 403 mengubah daftar putih IP menjadi orakel. Penyerang yang
// memegang key bocor dari repositori publik akan tahu key itu masih hidup dan tinggal
// mencari jalan keluar dari alamat lain — persis satu-satunya hal yang ingin
// disembunyikan daftar putih. Untuk banned, 403 memberi tahu yang diblokir bahwa
// blokirnya menyasar dia secara spesifik, sehingga ia bisa mencoba key atau alamat lain
// sampai menemukan yang lolos; 401 yang sama dengan "key salah" tidak memberi petunjuk
// apa pun.
//
// Harganya diketahui: pelanggan yang salah mengisi daftar putih IP sendiri akan melihat
// 401 yang membingungkan, sama seperti kalau key-nya memang salah. Itu ditebus di sisi
// operator, bukan di respons — alasannya ada di log lengkap dengan IP klien dan ID key,
// jadi dukungan bisa menjawab pertanyaannya dalam satu pencarian. 403 yang lebih ramah
// untuk satu pelanggan tidak sebanding dengan orakel yang terbuka untuk semua orang.
//
// Kegagalan infrastruktur (database mati, context habis) dibalas 500, bukan 401.
// Membalasnya 401 akan menyuruh klien memutar API key-nya untuk masalah yang tidak ada
// hubungannya dengan kredensialnya.
func (a *Authenticator) Authenticate() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			raw, err := Credential(r)
			if err != nil {
				a.reject(w, r, credentialReason(err))
				return
			}

			// IP dipakai repo untuk menegakkan ip_allowlist dan memeriksa blokir.
			// Alamat tak dikenal (mis. koneksi lewat unix socket, atau RealIP tidak
			// dipasang) diteruskan sebagai netip.Addr kosong; repo memperlakukannya
			// sebagai "tidak cocok dengan daftar putih mana pun", yang merupakan arah
			// aman.
			clientIP, _ := httpx.ClientIPFrom(ctx)

			key, err := a.repo.Authenticate(ctx, raw, clientIP)
			if err != nil {
				var rejection *keys.Rejection
				if errors.As(err, &rejection) {
					a.reject(w, r, rejection.Reason)
					return
				}
				// Sengaja tidak memakai errors.Is(err, keys.ErrKeyNotUsable): sentinel
				// itu juga terbuka lewat Rejection, sedangkan yang tersisa di sini
				// betul-betul kegagalan infrastruktur.
				a.log(ctx).LogAttrs(ctx, slog.LevelError,
					"gagal memverifikasi API key", slog.String("error", err.Error()))
				httpx.InternalError(w, r)
				return
			}

			principal := &Principal{Key: key}
			a.touchIfStale(ctx, principal, clientIP)
			next.ServeHTTP(w, r.WithContext(WithPrincipal(ctx, principal)))
		})
	}
}

// RequireScope menolak request yang key-nya tidak memiliki cakupan tertentu.
//
// Pasang di belakang Authenticate. Di sini 403 dipakai, bukan 401, dan cakupan yang
// kurang IKUT disebutkan dalam pesan — dua-duanya kebalikan dari kebijakan pada jalur
// autentikasi, dan alasannya sama: pada titik ini klien sudah membuktikan key-nya sah,
// jadi tidak ada informasi baru yang bocor. Nama cakupan pun bukan rahasia; ia bagian
// dari dokumentasi API, dan pemegang key bisa melihat cakupan key-nya sendiri di
// dashboard. Menyebutkannya membuat pengembang bisa memperbaiki sendiri — membuat key
// baru dengan cakupan yang benar — alih-alih menebak-nebak lewat dukungan.
func RequireScope(scope string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			principal, ok := PrincipalFrom(ctx)
			if !ok {
				// Tidak ada principal berarti rute ini tidak berada di belakang
				// Authenticate. Dijawab 401, bukan 403: yang kurang adalah
				// autentikasinya, bukan kewenangannya.
				observability.LoggerFrom(ctx).LogAttrs(ctx, slog.LevelError,
					"rute memerlukan cakupan tetapi tidak dipasang di belakang Authenticate",
					slog.String("scope_required", scope),
					slog.String("method", r.Method),
					slog.String("path", r.URL.Path))
				httpx.Unauthorized(w, r, MessageInvalidKey)
				return
			}
			if !principal.Can(scope) {
				// Warn, bukan Info: deretan penolakan atas satu key adalah tanda awal
				// bahwa key dipakai untuk hal di luar peruntukannya.
				observability.LoggerFrom(ctx).LogAttrs(ctx, slog.LevelWarn,
					"akses ditolak: cakupan API key tidak cukup",
					slog.String("api_key_id", principal.ID()),
					slog.String("api_key", principal.Masked()),
					slog.String("scope_required", scope),
					slog.String("method", r.Method),
					slog.String("path", r.URL.Path))
				httpx.Forbidden(w, r, "API key tidak memiliki cakupan "+scope)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// --- Penolakan ---------------------------------------------------------------

// reject membalas 401 seragam sambil mencatat alasan sebenarnya.
//
// Alasan hanya masuk log dan metrik. Yang TIDAK pernah ikut, di jalur mana pun, adalah
// nilai key mentah: satu-satunya cara memastikan kredensial tidak mendarat di sistem log
// adalah tidak pernah menuliskannya, bahkan tersamar. Pada tahap ini kami juga belum
// punya baris api_keys-nya — repo tidak mengembalikan key yang ditolak — jadi tidak ada
// ID maupun bentuk tersamar yang bisa dicatat.
func (a *Authenticator) reject(w http.ResponseWriter, r *http.Request, reason string) {
	ctx := r.Context()
	a.rejected.WithLabelValues(reason).Inc()

	attrs := []slog.Attr{
		slog.String("reason", reason),
		slog.String("method", r.Method),
		slog.String("path", r.URL.Path),
	}
	if ip, ok := httpx.ClientIPFrom(ctx); ok {
		attrs = append(attrs, slog.String("client_ip", ip.String()))
	}
	a.log(ctx).LogAttrs(ctx, slog.LevelWarn, "autentikasi API key gagal", attrs...)

	httpx.Unauthorized(w, r, MessageInvalidKey)
}

// credentialReason menerjemahkan kegagalan Credential menjadi label.
func credentialReason(err error) string {
	switch {
	case errors.Is(err, ErrConflictingCredential):
		return ReasonConflictingCredential
	case errors.Is(err, ErrMalformedCredential):
		return ReasonMalformedCredential
	default:
		return ReasonMissingCredential
	}
}

// log mengembalikan logger layanan yang sudah dibubuhi request_id bila context punya.
//
// Logger milik konstruktor yang dipakai sebagai dasar, bukan yang ada di context: yang
// dijanjikan konstruktor adalah logger itu, dan pemanggil non-HTTP tidak punya logger di
// context sama sekali. Bandingkan dengan RequireScope, yang tidak punya penerima sehingga
// harus mengambil logger dari context.
func (a *Authenticator) log(ctx context.Context) *slog.Logger {
	if id := observability.RequestIDFrom(ctx); id != "" {
		return a.logger.With(slog.String("request_id", id))
	}
	return a.logger
}

// --- Pencatatan pemakaian ----------------------------------------------------

// touchIfStale memperbarui last_used_at, tetapi tidak pada setiap request dan tidak di
// jalur kritis.
//
// # Kenapa berambang
//
// last_used_at hanya dipakai untuk menampilkan "terakhir dipakai" di dashboard dan untuk
// menemukan key yang sudah lama menganggur. Satu UPDATE per request berarti satu versi
// baris mati dan satu tambahan WAL untuk setiap panggilan yang lewat gateway — pada
// beban ribuan request per menit, itu menjadikan tabel api_keys tempat penulisan
// terpanas di database, demi informasi yang ketepatannya tidak perlu lebih baik dari
// beberapa menit. Ambang lima menit menghapus hampir seluruh biaya itu tanpa mengubah
// kegunaannya, dan tidak menyentuh keamanan sama sekali: masa berlaku key ditentukan
// expires_at dan status, bukan jejak ini.
//
// # Kenapa di latar
//
// Dijalankan di goroutine dengan context yang dilepas dari pembatalan request, jadi
// UPDATE-nya tidak pernah menambah latensi yang dirasakan klien dan tidak ikut mati saat
// respons selesai — context request dibatalkan begitu handler kembali, dan sebuah UPDATE
// yang dibatalkan di tengah jalan hanya menyisakan pekerjaan sia-sia di server.
// context.WithoutCancel mempertahankan seluruh nilai di context, sehingga log dari
// goroutine ini tetap membawa request_id yang sama.
//
// Jumlah goroutine-nya terbatas dengan sendirinya oleh ambang di atas: paling banyak
// satu per key per lima menit, bukan satu per request.
//
// Kegagalannya tidak boleh menggagalkan request. Yang hilang hanyalah satu pembaruan
// jejak; request itu sendiri sudah terautentikasi dengan sah.
//
// Dipanggil setelah autentikasi berhasil dan sebelum handler dijalankan, jadi jejaknya
// tetap tercatat walau request itu kemudian ditolak batas laju atau gagal di upstream —
// key-nya memang terlihat dipakai, dan itulah yang direkam kolom ini.
func (a *Authenticator) touchIfStale(ctx context.Context, p *Principal, ip netip.Addr) {
	if a.touchInterval <= 0 || p == nil || p.Key == nil {
		return
	}
	if last := p.Key.LastUsedAt; last != nil && time.Since(*last) < a.touchInterval {
		return
	}

	detached := context.WithoutCancel(ctx)
	id := p.Key.ID
	go func() {
		tctx, cancel := context.WithTimeout(detached, touchTimeout)
		defer cancel()

		if err := a.repo.TouchUsage(tctx, id, ip); err != nil {
			a.log(detached).LogAttrs(detached, slog.LevelWarn,
				"gagal mencatat pemakaian API key",
				slog.String("api_key_id", id),
				slog.String("error", err.Error()))
		}
	}()
}
