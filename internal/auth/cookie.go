package auth

import (
	"net/http"
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
// login tidak akan pernah berhasil. Jadi nama yang dipakai ditentukan environment, dan
// bukan Secure-nya saja yang berubah — namanya ikut berubah, karena "__Host-" tanpa
// Secure adalah kombinasi yang tidak sah.
const (
	// SessionCookieName dipakai di luar produksi.
	SessionCookieName = "routex_session"
	// SessionCookieNameHost dipakai di produksi.
	SessionCookieNameHost = hostPrefix + SessionCookieName

	// CSRFCookieName dipakai di luar produksi.
	CSRFCookieName = "routex_csrf"
	// CSRFCookieNameHost dipakai di produksi.
	CSRFCookieNameHost = hostPrefix + CSRFCookieName
)

// hostPrefix adalah prefiks cookie yang mengikat cookie ke satu host.
const hostPrefix = "__Host-"

// Cookies memasang dan menghapus cookie sesi serta cookie CSRF.
//
// Dibuat sekali saat start lalu dibagikan: seluruh isinya hanya dibaca, jadi aman dipakai
// bersamaan dari banyak goroutine.
type Cookies struct {
	// production menentukan nama cookie dan atribut Secure sekaligus, karena keduanya
	// tidak boleh berbeda pendapat.
	production bool
	// ttl menentukan Max-Age. Sama dengan masa berlaku sesi di database supaya cookie
	// tidak hidup lebih lama daripada sesi yang diwakilinya.
	ttl time.Duration
}

// NewCookies membuat penyusun cookie. cfg boleh nil dan diperlakukan sebagai
// non-produksi; ttl ≤ 0 memakai masa berlaku sesi bawaan.
func NewCookies(cfg *config.Config, ttl time.Duration) *Cookies {
	if ttl <= 0 {
		ttl = DefaultSessionTTL
	}
	return &Cookies{production: cfg != nil && cfg.AppEnv.IsProduction(), ttl: ttl}
}

// SessionName mengembalikan nama cookie sesi untuk environment ini.
func (c *Cookies) SessionName() string {
	if c.production {
		return SessionCookieNameHost
	}
	return SessionCookieName
}

// CSRFName mengembalikan nama cookie CSRF untuk environment ini.
func (c *Cookies) CSRFName() string {
	if c.production {
		return CSRFCookieNameHost
	}
	return CSRFCookieName
}

// TTL mengembalikan masa berlaku cookie sesi.
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
	http.SetCookie(w, c.cookie(c.SessionName(), token.Reveal(), true, int(c.ttl.Seconds())))
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

// SessionToken membaca token sesi dari request. Secret kosong berarti tidak ada cookie.
//
// Hanya nama yang sesuai environment yang dibaca. Menerima kedua nama sekaligus akan
// membuat cookie non-Secure yang tertinggal dari masa development tetap diterima di
// produksi, dan itu persis kelonggaran yang prefiks "__Host-" ada untuk menutup.
func (c *Cookies) SessionToken(r *http.Request) security.Secret {
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
		Secure:   c.production,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   maxAge,
	}
}
