package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/NexGen-X/Route-X/internal/config"
	"github.com/NexGen-X/Route-X/internal/httpx"
	"github.com/NexGen-X/Route-X/internal/observability"
)

// HeaderCSRFToken adalah header tempat dashboard menyalin token dari cookie.
//
// Bahwa perlindungannya berdiri di atas sebuah HEADER, bukan sebuah cookie, adalah inti
// pola ini: browser mengirim cookie ke origin kita pada setiap permintaan, termasuk
// permintaan yang dipicu situs lain, tetapi situs lain tidak bisa MEMBACA cookie kita
// untuk menyalinnya ke header — dan header custom pada permintaan lintas origin baru
// terkirim setelah preflight CORS yang tidak akan kita izinkan.
const HeaderCSRFToken = "X-CSRF-Token"

// csrfTokenPurpose memisahkan domain pemakaian HMAC.
//
// SESSION_SECRET adalah satu kunci untuk seluruh aplikasi, dan bisa dipakai lagi nanti
// untuk keperluan lain (tanda tangan cursor, tautan sekali pakai). Membubuhkan label
// pemakaian ke dalam HMAC memastikan token yang sah untuk satu keperluan tidak pernah
// bisa dipakai sebagai token keperluan lain, walaupun keduanya memakai kunci yang sama.
const csrfTokenPurpose = "routex-csrf-v1"

// CSRF menerbitkan dan memeriksa token CSRF untuk rute yang diautentikasi cookie.
//
// # Mengapa rute API dengan Bearer API key tidak butuh ini
//
// CSRF hanya mungkin terjadi ketika browser MELAMPIRKAN kredensial secara otomatis. Itu
// yang dilakukan browser dengan cookie: setiap permintaan ke origin kita membawa cookie
// sesi, tidak peduli halaman mana yang memicunya, jadi form atau fetch di situs penyerang
// bisa menumpang kewenangan pengguna yang sedang masuk.
//
// Header Authorization tidak pernah dilampirkan otomatis. Ia harus ditulis kode yang
// membuat permintaan, dan kode itu harus lebih dulu MEMILIKI nilai API key-nya. Penyerang
// yang sudah memiliki API key korban tidak perlu browser korban sama sekali — ia bisa
// memanggil API langsung dari servernya. Jadi token CSRF pada rute Bearer tidak menutup
// serangan apa pun; ia hanya menambah satu langkah yang harus dilakukan setiap klien SDK,
// dan langkah yang tidak berguna akhirnya dimatikan orang.
//
// Karena itu Protect() dipasang HANYA pada grup rute dashboard yang berada di belakang
// RequireSession, bukan pada /v1/* milik gateway.
type CSRF struct {
	secret  []byte
	cookies *Cookies
	// origin adalah origin yang diizinkan, hasil dari cfg.PublicURL. Kosong berarti
	// dibandingkan dengan host request itu sendiri.
	origin string
	logger *slog.Logger
}

// NewCSRF membuat penjaga CSRF.
//
// Kuncinya cfg.SessionSecret. Bila kosong — hanya mungkin di test atau konfigurasi yang
// belum lengkap, karena config mewajibkan SESSION_SECRET minimal 32 karakter — dipakai
// kunci acak sekali pakai dan itu dicatat sebagai kesalahan: token yang diterbitkan satu
// instance tidak akan berlaku di instance lain, sehingga dashboard akan tampak menolak
// permintaannya sendiri secara acak di belakang load balancer.
func NewCSRF(cfg *config.Config, cookies *Cookies, logger *slog.Logger) *CSRF {
	if logger == nil {
		logger = slog.Default()
	}
	c := &CSRF{cookies: cookies, logger: logger}

	if cfg != nil && !cfg.SessionSecret.IsZero() {
		c.secret = []byte(cfg.SessionSecret.Reveal())
	} else {
		c.secret = make([]byte, 32)
		// Sejak Go 1.24 crypto/rand.Read tidak pernah mengembalikan error.
		_, _ = rand.Read(c.secret)
		logger.Error("SESSION_SECRET kosong; kunci token CSRF dibuat acak per proses",
			"akibat", "token CSRF tidak berlaku lintas instance dan hangus setiap restart",
			"perbaikan", "isi SESSION_SECRET di environment")
	}
	if cfg != nil {
		c.origin = originOf(cfg.PublicURL)
	}
	return c
}

// Issue menghasilkan token CSRF untuk satu sesi.
//
// Token adalah HMAC-SHA256 atas ID sesi, jadi ia terikat ke sesi itu: token milik sesi
// lain — termasuk sesi lain milik pengguna yang sama — tidak akan pernah cocok, sehingga
// penyerang yang berhasil menanam cookie CSRF pilihannya sendiri tidak mendapatkan apa pun.
//
// Karena bentuknya deterministik, tidak ada yang perlu disimpan di server: token bisa
// dihitung ulang kapan saja dari ID sesi, dan ia hangus bersama sesinya. Nilainya bukan
// rahasia dalam arti kerahasiaan — ia memang harus bisa dibaca skrip halaman kita — tetapi
// ia tidak bisa dikarang tanpa SESSION_SECRET.
func (c *CSRF) Issue(sessionID string) string {
	if sessionID == "" {
		return ""
	}
	mac := hmac.New(sha256.New, c.secret)
	mac.Write([]byte(csrfTokenPurpose))
	mac.Write([]byte(sessionID))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// Verify melaporkan apakah token sah untuk sesi ini.
//
// Perbandingannya waktu-konstan lewat crypto/subtle. Ini bukan kehati-hatian yang
// berlebihan: perbandingan string biasa berhenti pada byte pertama yang berbeda, sehingga
// lamanya jawaban ikut menunjukkan berapa byte awal yang sudah benar, dan seorang penyerang
// yang bisa mengukurnya bisa menyusun token yang sah satu byte demi satu byte alih-alih
// menebak seluruh 256 bit sekaligus.
func (c *CSRF) Verify(sessionID, token string) bool {
	if sessionID == "" || token == "" {
		return false
	}
	return constantTimeEqual(c.Issue(sessionID), token)
}

// constantTimeEqual membandingkan dua nilai tanpa membocorkan posisi byte pertama yang
// berbeda melalui lamanya perbandingan.
//
// Dipisah menjadi fungsi sendiri, bukan ditulis dua kali di tempat pemakaian, supaya hanya
// ada satu tempat yang menentukan bagaimana token dibandingkan — dan supaya sifat waktu
// tetapnya bisa diuji langsung.
func constantTimeEqual(want, got string) bool {
	return subtle.ConstantTimeCompare([]byte(want), []byte(got)) == 1
}

// Protect menolak permintaan pengubah state yang tidak membawa token CSRF yang sah.
//
// Wajib dipasang SETELAH Service.RequireSession: pemeriksaannya berdiri di atas ID sesi
// yang sudah terautentikasi, karena itulah yang mengikat token ke satu sesi. Bila tidak ada
// principal di context, permintaan ditolak dan pemasangan yang salah dicatat sebagai
// kesalahan — gagal tertutup, dan berisik, supaya salah pasang tidak berakhir menjadi
// middleware yang diam-diam tidak memeriksa apa pun.
//
// Metode aman (GET, HEAD, OPTIONS, TRACE) dilewati. Itu sah karena tidak ada satu pun rute
// di aplikasi ini yang mengubah state pada metode tersebut; kalau suatu saat ada, celahnya
// bukan di sini melainkan di rute itu.
func (c *CSRF) Protect() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !stateChanging(r.Method) {
				next.ServeHTTP(w, r)
				return
			}

			principal, ok := PrincipalFrom(r.Context())
			if !ok {
				observability.LoggerFrom(r.Context()).LogAttrs(r.Context(), slog.LevelError,
					"middleware CSRF dipasang tanpa RequireSession di depannya",
					slog.String("method", r.Method), slog.String("path", r.URL.Path))
				c.reject(w, r, "tidak ada sesi terautentikasi")
				return
			}

			if reason := c.checkOrigin(r); reason != "" {
				c.reject(w, r, reason)
				return
			}

			// Bagian double-submit: nilai di cookie harus sama dengan nilai di header.
			// Situs lain bisa memaksa browser mengirim cookie kita, tetapi tidak bisa
			// membacanya untuk menyalin nilainya ke header.
			header := r.Header.Get(HeaderCSRFToken)
			cookie := c.cookies.CSRFToken(r)
			if header == "" || cookie == "" {
				c.reject(w, r, "token CSRF tidak disertakan")
				return
			}
			if !constantTimeEqual(cookie, header) {
				c.reject(w, r, "token CSRF di cookie dan header tidak sama")
				return
			}

			// Bagian pengikatan sesi: nilai yang sama di dua tempat belum cukup, karena
			// penyerang yang bisa menanam cookie (mis. lewat subdomain di deployment tanpa
			// prefiks __Host-) bisa menaruh nilai pilihannya sendiri di keduanya. Yang
			// menutup celah itu adalah tanda tangan atas ID sesi.
			if !c.Verify(principal.SessionID, header) {
				c.reject(w, r, "token CSRF tidak cocok dengan sesi")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// reject menolak permintaan dengan pesan seragam, mencatat sebab sebenarnya ke log.
//
// Klien tidak diberi tahu bagian mana yang gagal: itu tidak bisa ditindaklanjuti (jawaban
// dashboard selalu sama — ambil token baru lewat GET /me lalu ulangi) dan bagi penyerang ia
// menjadi petunjuk bagian mana yang sudah berhasil dilewati.
func (c *CSRF) reject(w http.ResponseWriter, r *http.Request, reason string) {
	observability.LoggerFrom(r.Context()).LogAttrs(r.Context(), slog.LevelWarn,
		"permintaan ditolak pemeriksaan CSRF",
		slog.String("reason", reason),
		slog.String("method", r.Method),
		slog.String("path", r.URL.Path))
	httpx.WriteError(w, r, http.StatusForbidden, httpx.ErrTypePermission,
		CodeCSRFInvalid, MessageCSRFInvalid)
}

// checkOrigin memastikan permintaan berasal dari origin kita sendiri. Mengembalikan alasan
// penolakan, atau "" bila lolos.
//
// Ini lapisan kedua yang berdiri sendiri, bukan pelengkap token. Token bisa gugur oleh hal
// yang tidak berhubungan dengan serangan — cookie terhapus, dashboard dibuka di dua tab —
// sementara Origin adalah keterangan yang ditulis BROWSER dan tidak bisa disetel kode
// halaman: fetch tidak boleh mengubahnya, dan form HTML tidak punya cara menyentuhnya.
// Sebuah permintaan dari evil.example.com akan selalu membawa Origin miliknya sendiri.
//
// Referer dipakai sebagai cadangan untuk permintaan yang tidak membawa Origin. Bila
// keduanya tidak ada, permintaan ditolak: browser modern selalu mengirim Origin pada
// permintaan pengubah state, jadi ketiadaan keduanya berarti permintaan itu bukan dari
// browser — dan yang bukan dari browser seharusnya memakai API key, bukan cookie sesi.
func (c *CSRF) checkOrigin(r *http.Request) string {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" || origin == "null" {
		// Origin "null" dikirim untuk konteks yang tidak punya origin sebenarnya
		// (iframe sandbox, dokumen dari data: URL). Tidak ada yang bisa dicocokkan.
		origin = originOf(r.Header.Get("Referer"))
	}
	if origin == "" {
		return "permintaan tanpa header Origin maupun Referer"
	}

	if c.origin != "" {
		if !strings.EqualFold(origin, c.origin) {
			return "origin di luar PUBLIC_URL"
		}
		return ""
	}

	// PUBLIC_URL tidak diisi — keadaan yang hanya sah di luar produksi, karena config
	// mewajibkannya saat APP_ENV=production. Yang dibandingkan hanya host-nya, bukan
	// skemanya: menebak skema request sendiri menuntut mempercayai X-Forwarded-Proto,
	// yang bisa dipalsukan siapa pun dan karena itu tidak layak menjadi dasar keputusan
	// keamanan.
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return "origin tidak bisa diurai"
	}
	if !strings.EqualFold(u.Host, r.Host) {
		return "origin berbeda dengan host request"
	}
	return ""
}

// originOf mengambil bagian "skema://host" dari sebuah URL, "" bila tidak lengkap.
func originOf(rawURL string) string {
	raw := strings.TrimSpace(rawURL)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

// stateChanging melaporkan apakah metode ini boleh mengubah state.
//
// Daftarnya positif (metode yang diperiksa), bukan negatif (metode yang dilewati): metode
// yang tidak dikenal — termasuk yang baru dan yang dikarang klien — jatuh ke sisi yang
// diperiksa, bukan ke sisi yang dilewatkan.
func stateChanging(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace:
		return false
	default:
		return true
	}
}
