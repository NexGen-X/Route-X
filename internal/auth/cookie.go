package auth

import (
	"net/http"
	"strings"
	"time"

	"github.com/NexGen-X/Route-X/internal/config"
	"github.com/NexGen-X/Route-X/internal/security"
)

// Nama cookie yang dipakai paket ini.
//
// Ada dua pasang nama, dan bedanya bukan kosmetik. Prefiks "__Host-" adalah kontrak yang
// ditegakkan browser, bukan sekadar konvensi penamaan: cookie berprefiks itu hanya
// diterima bila ia dikirim lewat HTTPS, memakai Secure, memakai Path=/, dan TIDAK memakai
// atribut Domain. Efeknya cookie terikat ke satu host persis — subdomain mana pun, termasuk
// subdomain yang dikuasai penyerang lewat DNS takeover atau lewat aplikasi lain di domain
// yang sama, tidak bisa menuliskan ulang cookie sesi kita (session fixation) maupun
// membacanya.
//
// Justru karena syaratnya HTTPS, prefiks itu tidak bisa dipakai di development yang
// berjalan di http://localhost: browser akan menolak cookie-nya secara diam-diam dan
// login tidak akan pernah berhasil. Jadi nama yang dipakai ditentukan environment dan
// skema URL, dan bukan Secure-nya saja yang berubah — namanya ikut berubah, karena "__Host-"
// tanpa Secure adalah kombinasi yang tidak sah.
const (
	// SessionCookieName dipakai di luar produksi dan non-HTTPS.
	SessionCookieName = "routex_session"
	// SessionCookieNameHost dipakai di produksi atau saat PublicURL berskema HTTPS.
	SessionCookieNameHost = hostPrefix + SessionCookieName

	// CSRFCookieName dipakai di luar produksi dan non-HTTPS.
	CSRFCookieName = "routex_csrf"
	// CSRFCookieNameHost dipakai di produksi atau saat PublicURL berskema HTTPS.
	CSRFCookieNameHost = hostPrefix + CSRFCookieName
)

// hostPrefix adalah prefiks cookie yang mengikat cookie ke satu host.
const hostPrefix = "__Host-"

// Cookies memasang dan menghapus cookie sesi serta cookie CSRF.
//
// Dibuat sekali saat start lalu dibagikan: seluruh isinya hanya dibaca, jadi aman dipakai
// bersamaan dari banyak goroutine.
type Cookies struct {
	// secure menentukan nama cookie (prefiks "__Host-") dan atribut Secure sekaligus,
	// karena keduanya tidak boleh berbeda pendapat. Aktif di lingkungan produksi
	// atau bila PublicURL berskema HTTPS (mis. staging HTTPS).
	secure bool
	// ttl menentukan Max-Age. Sama dengan masa berlaku sesi di database supaya cookie
	// tidak hidup lebih lama daripada sesi yang diwakilinya.
	ttl time.Duration
}

// NewCookies membuat penyusun cookie. cfg boleh nil dan diperlakukan sebagai
// non-produksi / non-HTTPS; ttl ≤ 0 memakai masa berlaku sesi bawaan.
// Bila cfg.PublicURL berskema HTTPS atau cfg di lingkungan produksi, prefiks
// "__Host-" dan atribut Secure: true diaktifkan.
func NewCookies(cfg *config.Config, ttl time.Duration) *Cookies {
	if ttl <= 0 {
		ttl = DefaultSessionTTL
	}
	secure := false
	if cfg != nil {
		secure = cfg.AppEnv.IsProduction() || isHTTPSPublicURL(cfg.PublicURL)
	}
	return &Cookies{secure: secure, ttl: ttl}
}

// isHTTPSPublicURL melaporkan apakah URL mentah memakai skema https, tanpa
// memedulikan huruf besar/kecil.
//
// Pemeriksaan awalan peka huruf yang lama membuat PUBLIC_URL "HTTPS://…" di
// lingkungan non-produksi menghasilkan cookie TANPA Secure dan tanpa prefiks
// __Host- (audit3 V-AU-07): satu huruf kapital di berkas env cukup untuk
// menurunkan perlindungan cookie secara diam-diam.
func isHTTPSPublicURL(raw string) bool {
	i := strings.Index(raw, "://")
	return i > 0 && strings.EqualFold(raw[:i], "https")
}

// SessionName mengembalikan nama cookie sesi untuk environment ini.
func (c *Cookies) SessionName() string {
	if c.secure {
		return SessionCookieNameHost
	}
	return SessionCookieName
}

// CSRFName mengembalikan nama cookie CSRF untuk environment ini.
func (c *Cookies) CSRFName() string {
	if c.secure {
		return CSRFCookieNameHost
	}
	return CSRFCookieName
}

// TTL mengembalikan masa berlaku cookie sesi.
// Akses baca (triage B-005): jalur produksi memakai field internal langsung; method ini dipertahankan untuk test dan paket lain.
func (c *Cookies) TTL() time.Duration { return c.ttl }

// SetSession memasang cookie sesi berisi token mentah.
//
// HttpOnly menutup satu-satunya jalan skrip halaman membaca token: dengan itu, XSS masih
// bisa melakukan permintaan atas nama pengguna, tetapi tidak bisa mengeluarkan token sesi
// dari browser untuk dipakai penyerang di tempat lain nanti.
//
// SameSite=Lax dipilih, bukan Strict. Strict tidak mengirim cookie bahkan pada navigasi
// tingkat atas dari situs lain, sehingga tautan ke dashboard dari email atau chat selalu
// mendarat di halaman login walau pengguna sedang masuk — dan itu mendorong orang membuat
// pengecualian yang lebih buruk. Lax sudah menahan seluruh permintaan lintas situs yang
// mengubah state (POST/PUT/PATCH/DELETE dari origin lain tidak membawa cookie ini), dan
// sisa celahnya — permintaan GET lintas situs — ditutup dengan tidak pernah mengubah state
// pada GET, ditambah pemeriksaan CSRF di csrf.go.
func (c *Cookies) SetSession(w http.ResponseWriter, token security.Secret) {
	c.SetSessionDenganSisa(w, token, c.ttl)
}

// SetSessionDenganSisa memasang cookie sesi dengan Max-Age sebesar sisa umur
// yang diberikan pemanggil, bukan TTL awal.
//
// Dipakai saat touch menggeser kedaluwarsa sesi di database (B-198): tanpa
// penerbitan ulang, browser membuang cookie tepat pada TTL awal walau baris
// sesinya sudah diperpanjang, dan janji "TTL geser" hanya berlaku untuk
// jalur header-token. Sisa umur dihitung pemanggil dari keadaan sesi
// sesudah penggeseran — termasuk plafon mutlaknya — supaya cookie tidak
// pernah hidup lebih lama dari sesi yang diwakilinya.
func (c *Cookies) SetSessionDenganSisa(w http.ResponseWriter, token security.Secret, sisa time.Duration) {
	maxAge := int(sisa.Seconds())
	if maxAge < 0 {
		maxAge = 0
	}
	http.SetCookie(w, c.cookie(c.SessionName(), token.Reveal(), true, maxAge))
}

// ClearSession menghapus cookie sesi.
//
// Nilai dikosongkan sekaligus Max-Age negatif: sebagian browser lama hanya menghormati
// salah satunya, dan cookie sesi yang tertinggal berarti browser terus mengirim token
// yang sudah dicabut pada setiap request.
func (c *Cookies) ClearSession(w http.ResponseWriter) {
	http.SetCookie(w, c.cookie(c.SessionName(), "", true, -1))
}

// SetCSRF memasang cookie token CSRF.
//
// Cookie ini sengaja BUKAN HttpOnly — itu bagian dari pola double-submit: skrip dashboard
// harus bisa membacanya untuk menyalinnya ke header X-CSRF-Token. Yang perlu dipahami,
// keterbacaan itu tidak melemahkan apa pun, karena token CSRF bukan kredensial: ia hanya
// bukti bahwa permintaan datang dari kode kita sendiri, dan situs lain tidak bisa membaca
// cookie milik origin kita. Yang harus tetap tidak terbaca adalah token sesi, dan itu ada
// di cookie yang berbeda.
func (c *Cookies) SetCSRF(w http.ResponseWriter, token string) {
	http.SetCookie(w, c.cookie(c.CSRFName(), token, false, int(c.ttl.Seconds())))
}

// ClearCSRF menghapus cookie token CSRF.
func (c *Cookies) ClearCSRF(w http.ResponseWriter) {
	http.SetCookie(w, c.cookie(c.CSRFName(), "", false, -1))
}

// SessionToken membaca token sesi dari request. Secret kosong berarti tidak ada cookie atau token.
//
// Mendukung header Authorization: Bearer <token> dan X-Session-Token: <token> untuk
// aplikasi native mobile companion APK & CLI, serta cookie sesi browser. Header
// didahulukan; gunakan SessionTokenFromHeader/SessionTokenFromCookie bila sumber
// tokennya penting — jalur autentikasi utama (Service.authenticateRequest) memakai
// keduanya secara terpisah supaya header yang tidak sah tidak mengalahkan cookie
// yang sah.
// Status (triage B-005): pembungkus gabungan header+cookie. Jalur produksi memakai SessionTokenFromHeader/SessionTokenFromCookie secara terpisah (penegakan CSRF berbasis sumber, grup 9 audit3); fungsi ini dipertahankan untuk pembacaan gabungan yang tidak sensitif sumber.
func (c *Cookies) SessionToken(r *http.Request) security.Secret {
	if token := c.SessionTokenFromHeader(r); !token.IsZero() {
		return token
	}
	return c.SessionTokenFromCookie(r)
}

// SessionTokenFromHeader membaca token sesi HANYA dari header: Authorization
// Bearer atau X-Session-Token. Secret kosong berarti tidak ada header kredensial.
//
// Nilai yang dikembalikan belum tentu sesi yang sah — ia hanya kandidat. Apakah
// kandidat ini yang benar-benar mengautentikasi request diputuskan Service, dan
// perbedaan itu penting: kehadiran header semata tidak boleh diperlakukan sebagai
// bukti apa pun (lihat CSRF.Protect dan audit3 V-AU-03/V-AU-04).
func (c *Cookies) SessionTokenFromHeader(r *http.Request) security.Secret {
	if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		token := strings.TrimPrefix(auth, "Bearer ")
		if token != "" {
			return security.Secret(token)
		}
	}
	if token := r.Header.Get("X-Session-Token"); token != "" {
		return security.Secret(token)
	}
	return ""
}

// SessionTokenFromCookie membaca token sesi HANYA dari cookie sesi.
func (c *Cookies) SessionTokenFromCookie(r *http.Request) security.Secret {
	ck, err := r.Cookie(c.SessionName())
	if err != nil || ck.Value == "" {
		return ""
	}
	return security.Secret(ck.Value)
}

// CSRFToken membaca token CSRF dari cookie. String kosong berarti tidak ada.
func (c *Cookies) CSRFToken(r *http.Request) string {
	ck, err := r.Cookie(c.CSRFName())
	if err != nil {
		return ""
	}
	return ck.Value
}

// cookie merakit satu cookie dengan atribut yang seragam.
//
// Path=/ bukan pilihan gaya: prefiks "__Host-" mensyaratkannya, dan tanpa itu cookie
// tidak akan terkirim untuk seluruh rute dashboard di bawah path lain. Domain sengaja
// dibiarkan kosong, juga karena syarat prefiks yang sama — cookie berlaku untuk host yang
// menerbitkannya saja.
func (c *Cookies) cookie(name, value string, httpOnly bool, maxAge int) *http.Cookie {
	return &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		HttpOnly: httpOnly,
		Secure:   c.secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   maxAge,
	}
}
