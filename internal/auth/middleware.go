package auth

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/NexGen-X/Route-X/internal/httpx"
	"github.com/NexGen-X/Route-X/internal/observability"
)

// principalCtxKey adalah kunci context untuk principal.
//
// Tipe kosong yang tidak diekspor, bukan string: dengan begitu paket lain tidak bisa
// menuliskan nilai ke kunci yang sama — baik karena kebetulan memakai string yang sama,
// maupun dengan sengaja. Satu-satunya jalan menaruh principal di context adalah
// WithPrincipal di paket ini.
type principalCtxKey struct{}

// WithPrincipal menautkan principal ke context.
//
// Diekspor untuk dua pemakai: middleware di paket ini, dan test atau jalur admin di fase
// berikutnya yang perlu memanggil handler dengan identitas tertentu tanpa melewati
// seluruh jalur login.
func WithPrincipal(ctx context.Context, p *Principal) context.Context {
	return context.WithValue(ctx, principalCtxKey{}, p)
}

// PrincipalFrom mengambil principal dari context. Nilai kedua false berarti request ini
// belum melewati RequireSession.
func PrincipalFrom(ctx context.Context) (*Principal, bool) {
	p, ok := ctx.Value(principalCtxKey{}).(*Principal)
	return p, ok && p != nil
}

// RequireSession mewajibkan cookie sesi yang berlaku.
//
// Alurnya: ambil token dari cookie, tukar menjadi principal lewat Authenticate, taruh
// principal di context, lalu segarkan jejak last_seen_at bila sudah cukup tua.
//
// Pembaruan last_seen_at TIDAK dilakukan pada setiap request — lihat touchIfStale untuk
// alasan lengkapnya. Ringkasnya: satu UPDATE per request dashboard berarti satu versi baris
// mati dan satu tambahan WAL per request, untuk data yang hanya dipakai menampilkan
// "terakhir terlihat"; ambang lima menit membuang biaya itu tanpa mengubah apa pun soal
// keamanan, karena masa berlaku sesi ditentukan expires_at, bukan jejak ini.
//
// Kegagalan dibagi dua, dan hanya dua. Segala hal yang berarti "tidak ada sesi yang
// berlaku" menjadi 401 dengan pesan yang sama — tidak ada perbedaan antara cookie yang
// tidak ada, token yang tidak dikenal, sesi yang kedaluwarsa, dan sesi yang dicabut, karena
// perbedaan itu memberi tahu pemegang token curian apakah tebakannya pernah bernilai.
// Kegagalan infrastruktur menjadi 500, karena membalasnya 401 akan menyuruh pengguna masuk
// ulang berkali-kali untuk masalah yang tidak ada hubungannya dengan kredensialnya.
//
// Pengguna yang wajib mengganti password TETAP lolos di sini; lihat RequirePasswordChanged.
func (s *Service) RequireSession() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			principal, err := s.Authenticate(ctx, s.cookies.SessionToken(r))
			if err != nil {
				if !errors.Is(err, ErrNoSession) {
					observability.LoggerFrom(ctx).LogAttrs(ctx, slog.LevelError,
						"gagal memeriksa sesi", slog.String("error", err.Error()))
					httpx.InternalError(w, r)
					return
				}
				// Cookie yang sudah tidak mewakili sesi apa pun dihapus, supaya browser
				// berhenti mengirim token mati pada setiap request berikutnya dan dashboard
				// tidak terjebak memuat ulang sesi yang tidak ada.
				s.cookies.ClearSession(w)
				s.cookies.ClearCSRF(w)
				unauthorized(w, r)
				return
			}

			s.touchIfStale(ctx, principal)
			next.ServeHTTP(w, r.WithContext(WithPrincipal(ctx, principal)))
		})
	}
}

// RequireSessionOrBearer mewajibkan sesi cookie yang valid, ATAU Bearer token tertentu,
// ATAU mengizinkan permintaan jika koneksi berasal dari loopback lokal.
// Ini dirancang khusus untuk endpoint telemetri seperti /metrics agar dapat diakses oleh
// scraper Prometheus internal tanpa mengorbankan keamanan data operasional dari publik.
func (s *Service) RequireSessionOrBearer(validToken string, allowLoopback bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			// 1. Periksa header Authorization Bearer jika validToken disediakan
			if validToken != "" {
				authHeader := r.Header.Get("Authorization")
				if strings.HasPrefix(authHeader, "Bearer ") {
					bearer := strings.TrimPrefix(authHeader, "Bearer ")
					if bearer == validToken {
						next.ServeHTTP(w, r)
						return
					}
				}
			}

			// 2. Periksa apakah request berasal dari loopback lokal jika diizinkan
			if allowLoopback {
				if ip, ok := httpx.ClientIPFrom(ctx); ok && ip.IsLoopback() {
					next.ServeHTTP(w, r)
					return
				}
			}

			// 3. Periksa sesi cookie admin
			if s != nil && s.cookies != nil {
				token := s.cookies.SessionToken(r)
				if !token.IsZero() {
					principal, err := s.Authenticate(ctx, token)
					if err == nil && principal != nil {
						s.touchIfStale(ctx, principal)
						next.ServeHTTP(w, r.WithContext(WithPrincipal(ctx, principal)))
						return
					}
				}
			}

			unauthorized(w, r)
		})
	}
}

// RequirePermission menolak request yang principal-nya tidak memiliki izin tertentu.
//
// Izin yang kurang tidak disebutkan dalam respons, hanya di log. Bagi pengguna, nama kunci
// izin bukan sesuatu yang bisa ditindaklanjuti — yang bisa menindaklanjutinya adalah
// operator, dan operator membaca log. Sebaliknya bagi penyerang yang sudah punya sesi
// berperan rendah, respons yang menyebutkan kunci izin adalah peta kewenangan sistem.
func RequirePermission(permission string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			principal, ok := PrincipalFrom(r.Context())
			if !ok {
				// Tidak ada principal berarti rute ini tidak berada di belakang
				// RequireSession. Dijawab 401, bukan 403: yang kurang adalah
				// autentikasinya, bukan kewenangannya.
				unauthorized(w, r)
				return
			}
			if !principal.Can(permission) {
				logDenied(r, principal, permission)
				forbidden(w, r)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireAnyPermission menolak request yang principal-nya tidak memiliki satu pun dari izin
// yang diberikan.
//
// Dipakai untuk rute yang melayani beberapa keperluan sekaligus — misalnya ringkasan
// dashboard yang boleh dilihat pemegang usage:read maupun requests:read. Daftar kosong
// selalu menolak: "tanpa syarat izin" harus ditulis dengan tidak memasang middleware ini,
// bukan dengan memanggilnya tanpa argumen, supaya tidak ada rute yang tampak terlindungi
// padahal tidak memeriksa apa pun.
func RequireAnyPermission(permissions ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			principal, ok := PrincipalFrom(r.Context())
			if !ok {
				unauthorized(w, r)
				return
			}
			if !principal.CanAny(permissions...) {
				logDenied(r, principal, strings.Join(permissions, "|"))
				forbidden(w, r)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequirePasswordChanged menolak request dari pengguna yang wajib mengganti passwordnya.
//
// # Mengapa dipisah dari RequireSession
//
// Pengguna dengan must_change_password TETAP terautentikasi sepenuhnya: sesinya sah dan
// principal-nya lengkap. Kalau RequireSession sendiri yang menolaknya, tidak akan ada
// jalan keluar — form ganti password juga membutuhkan sesi, jadi satu-satunya cara keluar
// dari keadaan itu adalah membuat pengecualian di dalam middleware autentikasi, yang berarti
// middleware itu harus tahu nama rute mana yang boleh dilewati. Itu menempatkan pengetahuan
// tentang rute di tempat yang paling salah, dan sekali ada pengecualian berbasis path, ada
// yang harus memelihara daftar itu selamanya.
//
// Karena itu pembagiannya: RequireSession menjawab "siapa ini", middleware ini menjawab
// "boleh ke mana", dan router yang memutuskan rute mana yang memakainya. Pasang pada
// SELURUH grup rute dashboard KECUALI GET /me, POST /logout, dan POST /change-password —
// tiga rute yang harus tetap bisa dipakai untuk keluar dari keadaan ini.
//
// Kodenya CodePasswordChangeRequired, bukan kode 403 biasa, supaya dashboard bisa
// mengarahkan ke form ganti password alih-alih menampilkan "akses ditolak" yang membuat
// pengguna terjebak tanpa tahu harus berbuat apa.
func RequirePasswordChanged() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			principal, ok := PrincipalFrom(r.Context())
			if !ok {
				unauthorized(w, r)
				return
			}
			if principal.MustChangePassword() {
				httpx.WriteError(w, r, http.StatusForbidden, httpx.ErrTypePermission,
					CodePasswordChangeRequired, MessagePasswordChangeRequired)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// --- Balasan gagal -----------------------------------------------------------

// unauthorized membalas 401 dengan envelope httpx dan pesan seragam.
func unauthorized(w http.ResponseWriter, r *http.Request) {
	httpx.WriteError(w, r, http.StatusUnauthorized, httpx.ErrTypeAuthentication,
		CodeSessionRequired, MessageSessionRequired)
}

// forbidden membalas 403 dengan envelope httpx dan pesan seragam.
func forbidden(w http.ResponseWriter, r *http.Request) {
	httpx.WriteError(w, r, http.StatusForbidden, httpx.ErrTypePermission,
		httpx.CodeInsufficientPermission, MessageForbidden)
}

// logDenied mencatat penolakan otorisasi lengkap dengan izin yang kurang.
//
// Ini satu-satunya tempat nama izin yang kurang ditulis. Levelnya Warn, bukan Info:
// deretan penolakan atas satu akun adalah salah satu tanda paling awal bahwa sebuah sesi
// dipakai untuk hal di luar peran pemiliknya.
func logDenied(r *http.Request, p *Principal, wanted string) {
	ctx := r.Context()
	observability.LoggerFrom(ctx).LogAttrs(ctx, slog.LevelWarn, "akses ditolak: izin tidak cukup",
		slog.String("user_id", p.User.ID),
		slog.String("role", p.TopRole()),
		slog.String("permission_required", wanted),
		slog.String("method", r.Method),
		slog.String("path", r.URL.Path))
}
