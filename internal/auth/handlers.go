package auth

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/NexGen-X/Route-X/internal/database/repo"
	"github.com/NexGen-X/Route-X/internal/database/repo/identity"
	"github.com/NexGen-X/Route-X/internal/httpx"
	"github.com/NexGen-X/Route-X/internal/observability"
	"github.com/NexGen-X/Route-X/internal/security"
)

// maxAuthBodyBytes membatasi body yang dibaca handler di paket ini.
//
// Body terbesar di sini adalah dua password dan satu email, jadi batasnya bisa jauh lebih
// ketat daripada batas global MAX_REQUEST_MIB yang harus menampung payload completion.
// Batas ketat di jalur yang belum terautentikasi ada gunanya: ia menghapus satu cara
// termurah membuat server bekerja tanpa memiliki kredensial apa pun.
const maxAuthBodyBytes = 8 << 10

// Handlers adalah handler HTTP untuk autentikasi dashboard.
//
// Semua bergantung pada Service dan tidak memegang state sendiri, jadi satu instance cukup
// dan aman dipakai bersamaan.
type Handlers struct {
	svc     *Service
	cookies *Cookies
	csrf    *CSRF
	logger  *slog.Logger
}

// NewHandlers membuat handler di atas layanan sesi.
func NewHandlers(svc *Service) *Handlers {
	return &Handlers{
		svc:     svc,
		cookies: svc.Cookies(),
		csrf:    svc.CSRF(),
		logger:  svc.logger,
	}
}

// Routes mengembalikan router siap pasang berisi keempat rute autentikasi.
//
// Disediakan sebagai satu handler, bukan sebagai pendaftaran ke router global, supaya
// pemanggil yang memutuskan prefiks-nya:
//
//	r.Mount("/api/auth", handlers.Routes())
//
// Susunan middleware-nya bagian dari kontrak dan sengaja tidak bisa diubah dari luar:
//
//   - POST /login tanpa RequireSession — belum ada sesi — dan tanpa CSRF, karena tidak ada
//     kewenangan yang bisa ditumpangi permintaan lintas situs di sini. Yang menjaganya
//     adalah pembatas laju dan penguncian akun.
//   - GET /me hanya membaca, jadi cukup RequireSession.
//   - POST /logout dan POST /change-password memakai CSRF: keduanya mengubah state atas
//     kewenangan yang dibawa cookie.
//
// RequirePasswordChanged sengaja TIDAK dipasang di sini: ketiga rute terautentikasi di atas
// justru yang harus tetap bisa dipakai saat pengguna wajib mengganti password. Pasang
// middleware itu pada grup rute dashboard yang lain.
func (h *Handlers) Routes() http.Handler {
	r := chi.NewRouter()

	r.Post("/login", h.Login)

	r.Group(func(authed chi.Router) {
		authed.Use(h.svc.RequireSession())
		authed.Get("/me", h.Me)

		authed.Group(func(guarded chi.Router) {
			guarded.Use(h.csrf.Protect())
			guarded.Post("/logout", h.Logout)
			guarded.Post("/change-password", h.ChangePassword)
		})
	})
	return r
}

// --- Bentuk request dan respons ----------------------------------------------

// LoginRequest adalah body POST /login.
//
// Password bertipe security.Secret sejak titik penguraian JSON, bukan string yang nanti
// dibungkus. Bedanya nyata: sebuah struct request yang kebetulan ikut di-log atau
// dikembalikan di pesan error tidak akan pernah memuat password, karena tipe itu selalu
// terserialisasi sebagai "[REDACTED]".
type LoginRequest struct {
	Email    string          `json:"email"`
	Password security.Secret `json:"password"`
}

// ChangePasswordRequest adalah body POST /change-password.
type ChangePasswordRequest struct {
	CurrentPassword security.Secret `json:"current_password"`
	NewPassword     security.Secret `json:"new_password"`
}

// PrincipalResponse adalah balasan POST /login dan GET /me.
//
// Bentuknya sama untuk keduanya supaya dashboard punya satu jalur pemrosesan, baik saat
// pengguna baru masuk maupun saat halaman dimuat ulang dengan sesi yang sudah ada.
type PrincipalResponse struct {
	User        identity.User    `json:"user"`
	Roles       []string         `json:"roles"`
	Permissions []string         `json:"permissions"`
	Session     identity.Session `json:"session"`
	// CSRFToken adalah salinan nilai di cookie CSRF. Dikirim di body juga supaya dashboard
	// tidak harus menguraikan document.cookie untuk memulai; keduanya nilai yang sama, dan
	// keterbacaannya oleh skrip halaman kita memang inti pola double-submit.
	CSRFToken string `json:"csrf_token"`
	// MustChangePassword menyalin nilai di User supaya dashboard tidak perlu menggali ke
	// dalam objek pengguna hanya untuk memutuskan pengalihan halaman.
	MustChangePassword bool `json:"must_change_password"`
}

// StatusResponse adalah balasan ringkas untuk aksi yang tidak mengembalikan data.
type StatusResponse struct {
	Status string `json:"status"`
	// RevokedSessions adalah jumlah sesi lain yang ikut dicabut, bila ada.
	RevokedSessions int `json:"revoked_sessions,omitempty"`
}

// Nilai field Status pada StatusResponse.
const (
	StatusLoggedOut       = "logged_out"
	StatusPasswordChanged = "password_changed"
)

// --- Handler -----------------------------------------------------------------

// Login menangani POST /login: memverifikasi kredensial, memasang cookie sesi dan cookie
// CSRF, lalu membalas principal lengkap.
//
// Seluruh kegagalan kredensial dibalas 401 dengan satu pesan yang sama. Yang dibedakan
// hanyalah batas laju (429, dengan Retry-After supaya dashboard tahu harus menunggu, bukan
// menampilkan "password salah" yang menyesatkan) dan kegagalan internal (500).
func (h *Handlers) Login(w http.ResponseWriter, r *http.Request) {
	var in LoginRequest
	if !decodeJSON(w, r, &in) {
		return
	}
	if strings.TrimSpace(in.Email) == "" || in.Password.IsZero() {
		httpx.BadRequest(w, r, httpx.CodeInvalidRequest, "email dan password wajib diisi")
		return
	}

	res, err := h.svc.Login(r.Context(), in.Email, in.Password, clientIP(r), r.UserAgent())
	if err != nil {
		var failure *LoginError
		if errors.As(err, &failure) {
			if failure.Reason == ReasonRateLimited {
				httpx.TooManyRequests(w, r, MessageLoginRateLimited, failure.RetryAfter)
				return
			}
			// Pesan seragam untuk email tak terdaftar, password salah, akun terkunci, dan
			// akun dimatikan. Sebabnya sudah tercatat di log dan audit oleh Service.
			httpx.WriteError(w, r, http.StatusUnauthorized, httpx.ErrTypeAuthentication,
				CodeLoginFailed, MessageLoginFailed)
			return
		}
		h.fail(w, r, "gagal memproses login", err)
		return
	}

	h.cookies.SetSession(w, res.Token)
	h.writeprincipal(w, r, res.Principal, http.StatusOK)
}

// Me menangani GET /me: mengembalikan identitas dan kewenangan sesi yang sedang dipakai.
//
// Cookie CSRF ikut disegarkan di sini. Inilah rute yang dipanggil dashboard saat halaman
// dimuat, termasuk setelah refresh dengan sesi yang masih hidup, jadi di sinilah tempat
// paling alami memastikan token CSRF yang dipegang skrip halaman selalu sesuai dengan sesi
// yang sedang berjalan.
func (h *Handlers) Me(w http.ResponseWriter, r *http.Request) {
	principal, ok := PrincipalFrom(r.Context())
	if !ok {
		unauthorized(w, r)
		return
	}
	h.writeprincipal(w, r, principal, http.StatusOK)
}

// Logout menangani POST /logout: mencabut sesi lalu menghapus kedua cookie.
//
// Cookie hanya dihapus setelah pencabutan di server berhasil. Kalau urutannya dibalik,
// kegagalan pencabutan akan menghasilkan keadaan terburuk: pengguna merasa sudah keluar
// karena cookie-nya hilang, sementara token yang mungkin sudah dicuri masih berlaku sampai
// kedaluwarsa sendiri.
func (h *Handlers) Logout(w http.ResponseWriter, r *http.Request) {
	principal, ok := PrincipalFrom(r.Context())
	if !ok {
		unauthorized(w, r)
		return
	}

	err := h.svc.Logout(r.Context(), principal.SessionID, principal.Actor(),
		clientIP(r), r.UserAgent())
	if err != nil {
		h.fail(w, r, "gagal mencabut sesi", err)
		return
	}

	h.cookies.ClearSession(w)
	h.cookies.ClearCSRF(w)
	h.write(w, r, http.StatusOK, StatusResponse{Status: StatusLoggedOut})
}

// ChangePassword menangani POST /change-password.
//
// Sesi yang sedang dipakai dipertahankan dan sesi lain dicabut, jadi tidak ada cookie yang
// perlu diganti — pengguna tetap masuk di perangkat ini dan keluar di semua perangkat lain.
//
// Password lama yang salah dibalas 403, bukan 401. Ini penting untuk perilaku dashboard:
// 401 berarti "sesimu tidak berlaku" dan akan memicu pengalihan ke halaman login, padahal
// sesinya justru sah — yang salah hanya satu field di dalam form.
func (h *Handlers) ChangePassword(w http.ResponseWriter, r *http.Request) {
	principal, ok := PrincipalFrom(r.Context())
	if !ok {
		unauthorized(w, r)
		return
	}

	var in ChangePasswordRequest
	if !decodeJSON(w, r, &in) {
		return
	}
	if in.CurrentPassword.IsZero() || in.NewPassword.IsZero() {
		httpx.BadRequest(w, r, httpx.CodeInvalidRequest,
			"current_password dan new_password wajib diisi")
		return
	}

	revoked, err := h.svc.ChangePassword(r.Context(), principal.User.ID,
		in.CurrentPassword, in.NewPassword, principal.SessionID, clientIP(r), r.UserAgent())
	switch {
	case err == nil:
	case errors.Is(err, ErrWrongPassword):
		httpx.WriteError(w, r, http.StatusForbidden, httpx.ErrTypePermission,
			CodeCurrentPasswordInvalid, ErrWrongPassword.Error())
		return
	case errors.Is(err, ErrPasswordUnchanged):
		httpx.BadRequest(w, r, CodePasswordUnchanged, ErrPasswordUnchanged.Error())
		return
	case errors.Is(err, ErrWeakPassword):
		// Aturan yang dilanggar disebutkan: berbeda dari kegagalan login, di sini
		// keterangan rinci tidak membocorkan apa pun dan tanpa keterangan itu pengguna
		// hanya bisa menebak-nebak kebijakan password.
		httpx.BadRequest(w, r, CodeWeakPassword, PasswordRuleMessage(err))
		return
	case errors.Is(err, repo.ErrNotFound):
		// Sesi sah tetapi penggunanya sudah tidak ada: hanya mungkin bila akunnya dihapus
		// dalam sela-sela request ini.
		unauthorized(w, r)
		return
	default:
		h.fail(w, r, "gagal mengganti password", err)
		return
	}

	h.write(w, r, http.StatusOK, StatusResponse{
		Status:          StatusPasswordChanged,
		RevokedSessions: revoked,
	})
}

// --- Perkakas handler --------------------------------------------------------

// writeprincipal menerbitkan token CSRF baru, memasangnya sebagai cookie, lalu membalas
// principal.
func (h *Handlers) writeprincipal(w http.ResponseWriter, r *http.Request, p *Principal, status int) {
	token := h.csrf.Issue(p.SessionID)
	h.cookies.SetCSRF(w, token)

	h.write(w, r, status, PrincipalResponse{
		User:               p.User,
		Roles:              p.Roles,
		Permissions:        p.Permissions,
		Session:            p.Session,
		CSRFToken:          token,
		MustChangePassword: p.MustChangePassword(),
	})
}

// write mengirim respons JSON dan mencatat kegagalan penulisannya.
//
// Kegagalan di sini praktis selalu berarti klien sudah menutup koneksi, jadi tidak ada
// balasan lain yang bisa dikirim — tapi ia tetap dicatat, karena kegagalan tulis yang
// sering muncul pada satu rute adalah petunjuk masalah lain (respons terlalu besar,
// timeout proxy).
func (h *Handlers) write(w http.ResponseWriter, r *http.Request, status int, body any) {
	if err := httpx.JSON(w, status, body); err != nil {
		observability.LoggerFrom(r.Context()).LogAttrs(r.Context(), slog.LevelWarn,
			"gagal menulis respons autentikasi", slog.String("error", err.Error()))
	}
}

// fail mencatat penyebab asli sebuah kegagalan lalu membalas 500 generik.
//
// Pemisahan ini yang menjaga agar pesan error internal — nama tabel, teks driver pgx,
// alamat upstream — tidak pernah sampai ke klien. Yang diterima klien hanya request_id,
// dan request_id itu cukup untuk menemukan baris log ini.
func (h *Handlers) fail(w http.ResponseWriter, r *http.Request, msg string, err error) {
	observability.LoggerFrom(r.Context()).LogAttrs(r.Context(), slog.LevelError, msg,
		slog.String("error", err.Error()))
	httpx.InternalError(w, r)
}

// decodeJSON menguraikan body JSON. Mengembalikan false bila balasan gagal sudah ditulis.
//
// Content-Type wajib application/json, dan itu bukan formalitas. Form HTML hanya bisa
// mengirim application/x-www-form-urlencoded, multipart/form-data, atau text/plain — jadi
// mewajibkan JSON menutup satu kelas serangan CSRF secara struktural: sebuah form di situs
// penyerang tidak bisa membentuk permintaan yang lolos syarat ini sama sekali, tanpa
// bergantung pada token maupun pada pemeriksaan Origin.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		httpx.WriteError(w, r, http.StatusUnsupportedMediaType, httpx.ErrTypeInvalidRequest,
			CodeUnsupportedMediaType, "Content-Type harus application/json")
		return false
	}

	// Body dibatasi di sini, bukan hanya oleh middleware global: handler ini bisa dipasang
	// di router mana pun, dan jalur yang belum terautentikasi tidak boleh bergantung pada
	// middleware yang mungkin lupa dipasang.
	if err := json.NewDecoder(io.LimitReader(r.Body, maxAuthBodyBytes)).Decode(dst); err != nil {
		// Pesan error json bisa memuat cuplikan body — yang di rute ini berarti password.
		// Karena itu hanya jenis kegagalannya yang disampaikan, tanpa teks aslinya.
		httpx.BadRequest(w, r, httpx.CodeInvalidRequest, "body request bukan JSON yang sah")
		return false
	}
	return true
}

// clientIP mengambil alamat klien untuk jejak sesi dan audit.
//
// Hasil resolusi httpx.RealIP diutamakan karena hanya di sana X-Forwarded-For diperiksa
// terhadap daftar proxy yang dipercaya. Tanpa middleware itu, yang dipakai adalah peer
// koneksi apa adanya — bukan header, yang bisa diisi siapa pun.
func clientIP(r *http.Request) string {
	if ip, ok := httpx.ClientIPFrom(r.Context()); ok {
		return ip.String()
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}
