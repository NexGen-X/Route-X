package httpx

import (
	"bufio"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"runtime/debug"
	"slices"
	"strings"
	"time"

	"github.com/NexGen-X/Route-X/internal/config"
	"github.com/NexGen-X/Route-X/internal/observability"
)

// Chain menggabungkan beberapa middleware menjadi satu. Yang pertama di daftar
// berada paling luar, jadi urutannya sama dengan urutan pemanggilan chi.Use.
// Elemen nil dilewati supaya middleware opsional bisa ditulis tanpa percabangan.
func Chain(middlewares ...func(http.Handler) http.Handler) func(http.Handler) http.Handler {
	// Salin agar perubahan pada slice pemanggil tidak mengubah rantai yang sudah jadi.
	stack := slices.Clone(middlewares)
	return func(next http.Handler) http.Handler {
		for i := len(stack) - 1; i >= 0; i-- {
			if stack[i] == nil {
				continue
			}
			next = stack[i](next)
		}
		return next
	}
}

// --- Wrapper ResponseWriter -------------------------------------------------

// responseWriter membungkus http.ResponseWriter untuk mencatat status code dan
// jumlah byte terkirim, tanpa mengorbankan satu pun kemampuan streaming:
//
//   - Flush dan FlushError diteruskan, jadi setiap event SSE langsung sampai ke
//     klien alih-alih menumpuk sampai handler selesai.
//   - Unwrap membuat http.ResponseController menelusuri rantai sampai writer asli
//     net/http, sehingga SetReadDeadline, SetWriteDeadline, EnableFullDuplex, dan
//     Hijack tetap tersedia untuk handler.
//   - ReadFrom diteruskan supaya penyajian file statis tetap bisa memakai jalur
//     cepat sendfile milik net/http.
type responseWriter struct {
	http.ResponseWriter
	status      int
	bytes       int64
	wroteHeader bool
}

// wrapResponseWriter memakai kembali wrapper yang sudah ada di rantai. Dengan
// begitu AccessLog, Recover, dan MaxBytes berbagi satu pencatat status: tidak ada
// byte yang terhitung dua kali, dan semuanya melihat "respons sudah dimulai" yang
// sama.
func wrapResponseWriter(w http.ResponseWriter) *responseWriter {
	if rw, ok := w.(*responseWriter); ok {
		return rw
	}
	// Default 200 karena itulah yang dikirim net/http bila handler menulis body
	// tanpa memanggil WriteHeader, atau tidak menulis apa pun.
	return &responseWriter{ResponseWriter: w, status: http.StatusOK}
}

func (w *responseWriter) WriteHeader(status int) {
	// Respons informasional (1xx, mis. 103 Early Hints) boleh dikirim berulang dan
	// bukan penutup header, jadi jangan tandai respons sebagai sudah dimulai.
	if status >= 100 && status < 200 {
		w.ResponseWriter.WriteHeader(status)
		return
	}
	if w.wroteHeader {
		// Menghindari peringatan "superfluous WriteHeader call" dari net/http saat
		// handler dan middleware sama-sama mencoba menulis status.
		return
	}
	w.status = status
	w.wroteHeader = true
	w.ResponseWriter.WriteHeader(status)
}

func (w *responseWriter) Write(b []byte) (int, error) {
	w.markStarted()
	n, err := w.ResponseWriter.Write(b)
	w.bytes += int64(n)
	return n, err
}

// markStarted mencatat bahwa respons sudah berjalan tanpa mengirim header kedua:
// Write dan Flush pada net/http sendiri yang mengirim 200 bila belum ada status.
func (w *responseWriter) markStarted() {
	if !w.wroteHeader {
		w.status = http.StatusOK
		w.wroteHeader = true
	}
}

// FlushError meneruskan flush ke writer di bawah. Ini varian yang dicari
// http.ResponseController lebih dulu, sehingga galat tulis benar-benar sampai ke
// handler streaming alih-alih hilang.
func (w *responseWriter) FlushError() error {
	w.markStarted()
	switch f := w.ResponseWriter.(type) {
	case interface{ FlushError() error }:
		return f.FlushError()
	case http.Flusher:
		f.Flush()
		return nil
	default:
		// Writer di bawah mungkin masih menyembunyikan Flusher di balik Unwrap-nya
		// sendiri; biarkan ResponseController yang menelusuri.
		return http.NewResponseController(w.ResponseWriter).Flush()
	}
}

// Flush memenuhi http.Flusher untuk kode yang masih memakai type assertion langsung.
func (w *responseWriter) Flush() { _ = w.FlushError() }

// Unwrap dipakai http.ResponseController untuk menemukan writer asli.
func (w *responseWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// Hijack diteruskan supaya protokol yang mengambil alih koneksi (mis. WebSocket)
// tetap bisa dipakai di belakang middleware ini.
func (w *responseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hj, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("httpx: writer di bawah tidak mendukung Hijack: %w", http.ErrNotSupported)
	}
	w.markStarted()
	return hj.Hijack()
}

// ReadFrom menjaga jalur cepat io.Copy pada writer di bawah sambil tetap menghitung
// byte yang terkirim.
func (w *responseWriter) ReadFrom(src io.Reader) (int64, error) {
	w.markStarted()
	n, err := io.Copy(w.ResponseWriter, src)
	w.bytes += n
	return n, err
}

// Kontrak yang diandalkan seluruh paket ini.
var (
	_ http.Flusher  = (*responseWriter)(nil)
	_ http.Hijacker = (*responseWriter)(nil)
	_ io.ReaderFrom = (*responseWriter)(nil)
	_ interface {
		Unwrap() http.ResponseWriter
	} = (*responseWriter)(nil)
)

// --- RequestID --------------------------------------------------------------

// HeaderRequestID adalah nama header korelasi yang dibaca dari klien dan selalu
// dikirim balik di respons.
const HeaderRequestID = "X-Request-Id"

// maxRequestIDLen membatasi panjang ID yang diterima dari klien. Nilai ini masuk ke
// setiap baris log dan ke header respons, jadi panjangnya harus dibatasi agar tidak
// bisa dipakai membanjiri sistem log.
const maxRequestIDLen = 128

// RequestID memastikan setiap request punya satu ID korelasi.
//
// ID dari klien dipakai bila lolos validasi, supaya trace milik pemanggil tetap
// tersambung dengan log kita. Bila tidak ada atau tidak layak, dibuat UUID v4 baru.
// ID lalu ditaruh di context, ditulis ke header respons, dan ditautkan ke logger
// context sehingga seluruh log dalam satu request otomatis membawa request_id.
func RequestID() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := sanitizeRequestID(r.Header.Get(HeaderRequestID))
			if id == "" {
				id = newRequestID()
			}

			ctx := observability.WithRequestID(r.Context(), id)
			ctx = observability.WithLogger(ctx, observability.LoggerFrom(ctx).With(slog.String("request_id", id)))

			// Header dipasang sebelum handler jalan supaya ID tetap terkirim walau
			// handler langsung menulis body atau memutus stream di tengah jalan.
			w.Header().Set(HeaderRequestID, id)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// sanitizeRequestID mengembalikan ID klien bila layak dipakai, atau "" bila harus
// diganti dengan ID baru.
//
// Nilai ini dipantulkan ke header respons dan ditulis ke log, jadi input sembarangan
// tidak boleh lolos: CR/LF membuka pemisahan header (header injection), karakter
// kontrol lain bisa mengacaukan parser log dan terminal operator, dan nilai panjang
// membebani setiap baris log. net/http juga menolak CR/LF di header, tapi lapisan
// ini menolak nilainya secara utuh alih-alih diam-diam mengganti karakter.
func sanitizeRequestID(v string) string {
	if v == "" || len(v) > maxRequestIDLen {
		return ""
	}
	for i := 0; i < len(v); i++ {
		if !isRequestIDByte(v[i]) {
			return ""
		}
	}
	return v
}

// isRequestIDByte membatasi charset ke bentuk yang benar-benar dipakai pengenal
// request: UUID, ULID, dan trace ID W3C, plus beberapa pemisah yang umum.
func isRequestIDByte(c byte) bool {
	switch {
	case c >= '0' && c <= '9', c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z':
		return true
	case c == '-', c == '_', c == '.', c == ':':
		return true
	}
	return false
}

const hexDigits = "0123456789abcdef"

// newRequestID menghasilkan UUID versi 4 dari crypto/rand. Dibuat di sini alih-alih
// menambah dependensi: UUID v4 hanyalah 16 byte acak dengan dua nibble penanda.
func newRequestID() string {
	var b [16]byte
	// Sejak Go 1.24 crypto/rand.Read didokumentasikan tidak pernah gagal; ia panic
	// bila sumber acak sistem tidak tersedia.
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40 // versi 4
	b[8] = (b[8] & 0x3f) | 0x80 // varian RFC 4122

	var out [36]byte
	for i, j := 0, 0; i < len(b); i++ {
		if i == 4 || i == 6 || i == 8 || i == 10 {
			out[j] = '-'
			j++
		}
		out[j] = hexDigits[b[i]>>4]
		out[j+1] = hexDigits[b[i]&0x0f]
		j += 2
	}
	return string(out[:])
}

// --- Recover ----------------------------------------------------------------

// Recover mengubah panic menjadi respons 500 yang rapi dan satu baris log berisi
// stack trace.
//
// Klien tidak pernah melihat pesan panic maupun stack: keduanya sering memuat nama
// tabel, potongan query, atau nilai variabel. Yang sampai ke klien hanya envelope
// generik plus request_id, yang cukup untuk mencari baris log yang bersangkutan.
func Recover(logger *slog.Logger) func(http.Handler) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rw := wrapResponseWriter(w)

			defer func() {
				recovered := recover()
				if recovered == nil {
					return
				}
				// http.ErrAbortHandler adalah cara net/http meminta koneksi diputus
				// tanpa dianggap kegagalan; teruskan agar semantiknya utuh.
				if err, ok := recovered.(error); ok && errors.Is(err, http.ErrAbortHandler) {
					panic(recovered)
				}

				attrs := []slog.Attr{
					slog.Any("panic", recovered),
					slog.String("method", r.Method),
					slog.String("path", r.URL.Path),
					slog.String("stack", string(debug.Stack())),
				}
				if id := observability.RequestIDFrom(r.Context()); id != "" {
					attrs = append(attrs, slog.String("request_id", id))
				}
				logger.LogAttrs(r.Context(), slog.LevelError, "panic saat menangani request", attrs...)

				if rw.wroteHeader {
					// Respons sudah mulai terkirim — misalnya panic di tengah stream
					// SSE. Header tidak bisa ditarik kembali dan menempelkan JSON di
					// belakang event akan merusak parser klien, jadi cukup log dan
					// biarkan koneksi ditutup.
					return
				}
				InternalError(rw, r)
			}()

			next.ServeHTTP(rw, r)
		})
	}
}

// --- SecurityHeaders --------------------------------------------------------

const (
	// cspSPA cocok untuk frontend hasil build bundler: seluruh script dan style
	// berasal dari origin sendiri, tidak ada plugin, dan halaman tidak boleh
	// disematkan di frame mana pun.
	//
	// 'unsafe-inline' hanya diberikan pada style-src karena bundler dan komponen UI
	// menyuntikkan <style> inline saat runtime; script-src tetap ketat, dan di
	// situlah risiko XSS sebenarnya berada.
	cspSPA = "default-src 'self'; " +
		"base-uri 'self'; " +
		"form-action 'self'; " +
		"frame-ancestors 'none'; " +
		"object-src 'none'; " +
		"script-src 'self'; " +
		"style-src 'self' 'unsafe-inline'; " +
		"img-src 'self' data: blob:; " +
		"font-src 'self' data:; " +
		"connect-src 'self'; " +
		"worker-src 'self' blob:; " +
		"manifest-src 'self'"

	// permissionsPolicy mematikan seluruh API perangkat. Dashboard gateway tidak
	// butuh kamera, mikrofon, atau lokasi, jadi tidak ada alasan membiarkannya
	// tersedia untuk skrip yang lolos.
	permissionsPolicy = "accelerometer=(), autoplay=(), camera=(), display-capture=(), " +
		"encrypted-media=(), geolocation=(), gyroscope=(), magnetometer=(), " +
		"microphone=(), midi=(), payment=(), usb=(), xr-spatial-tracking=()"

	// hstsValue tanpa "preload": preload adalah komitmen yang praktis permanen dan
	// harus jadi keputusan operator, bukan default aplikasi.
	hstsValue = "max-age=31536000; includeSubDomains"
)

// SecurityHeaders memasang header pengerasan pada setiap respons. cfg boleh nil dan
// diperlakukan sebagai non-produksi.
func SecurityHeaders(cfg *config.Config) func(http.Handler) http.Handler {
	production := cfg != nil && cfg.AppEnv.IsProduction()

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
			h.Set("X-Frame-Options", "DENY")
			h.Set("Permissions-Policy", permissionsPolicy)
			h.Set("Cross-Origin-Opener-Policy", "same-origin")
			h.Set("Content-Security-Policy", cspSPA)

			// HSTS hanya di produksi, dan ini bukan sekadar kerapian.
			//
			// Di development gateway diakses lewat http://localhost:8080. HSTS berlaku
			// per host tanpa memandang port, jadi satu respons saja akan membuat
			// browser memaksa https untuk SELURUH proyek lain di localhost, dan
			// pinning itu bertahan sampai operator membersihkan state browser secara
			// manual. Biayanya nyata, manfaatnya nol karena tidak ada TLS di dev.
			if production {
				h.Set("Strict-Transport-Security", hstsValue)
			}

			next.ServeHTTP(w, r)
		})
	}
}

// --- MaxBytes ---------------------------------------------------------------

// MaxBytes membatasi ukuran body request menjadi n byte dan menjawab 413 dengan
// envelope error bila terlampaui. n <= 0 berarti tanpa batas.
//
// Ada dua jalur deteksi karena klien tidak selalu jujur soal ukuran:
//
//  1. Content-Length lebih besar dari batas ditolak sebelum satu byte pun dibaca.
//  2. Body chunked tanpa Content-Length baru ketahuan saat dibaca, jadi body
//     dibungkus http.MaxBytesReader dan hasilnya diperiksa setelah handler selesai.
//     Bila handler belum mengirim apa pun, middleware yang menulis 413 supaya klien
//     tetap menerima bentuk error yang sama.
func MaxBytes(n int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if n <= 0 {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.ContentLength > n {
				PayloadTooLarge(w, r, n)
				return
			}
			if r.Body == nil || r.Body == http.NoBody {
				next.ServeHTTP(w, r)
				return
			}

			body := &limitedBody{ReadCloser: http.MaxBytesReader(w, r.Body, n)}
			// Salin dangkal seperti http.MaxBytesHandler: request milik net/http tidak
			// boleh dimutasi di tempat.
			r2 := *r
			r2.Body = body

			rw := wrapResponseWriter(w)
			next.ServeHTTP(rw, &r2)

			if body.exceeded && !rw.wroteHeader {
				PayloadTooLarge(rw, &r2, n)
			}
		})
	}
}

// limitedBody mengingat apakah batas ukuran sempat terlampaui saat body dibaca.
type limitedBody struct {
	io.ReadCloser
	exceeded bool
}

func (b *limitedBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		b.exceeded = true
	}
	return n, err
}

// --- AccessLog --------------------------------------------------------------

// AccessLog menulis satu baris log per request setelah handler selesai.
//
// Header request TIDAK PERNAH di-log, satu pun. Authorization, X-Api-Key, dan
// Cookie membawa kredensial pelanggan, dan cara paling andal untuk memastikan
// kredensial tidak mendarat di sistem log adalah tidak pernah mengambilnya. Yang
// dicatat hanya metadata rute: metode, path (tanpa query mentah), status, jumlah
// byte, durasi, request_id, dan IP klien.
//
// Untuk respons SSE baris log muncul setelah stream berakhir, sehingga durasinya
// adalah umur seluruh stream.
func AccessLog(logger *slog.Logger) func(http.Handler) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rw := wrapResponseWriter(w)

			next.ServeHTTP(rw, r)

			elapsed := time.Since(start)
			attrs := []slog.Attr{
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", rw.status),
				slog.Int64("bytes", rw.bytes),
				slog.Float64("duration_ms", float64(elapsed.Microseconds())/1000),
				slog.String("remote_ip", clientIPForLog(r)),
				slog.String("proto", r.Proto),
			}
			if id := observability.RequestIDFrom(r.Context()); id != "" {
				attrs = append(attrs, slog.String("request_id", id))
			}
			// Query dicatat dengan nilai parameter sensitif disamarkan: sebagian klien
			// masih menaruh API key di query string, dan nama parameternya sendiri
			// berguna untuk menelusuri rute.
			if q := redactQuery(r.URL.RawQuery); q != "" {
				attrs = append(attrs, slog.String("query", q))
			}

			logger.LogAttrs(r.Context(), levelForStatus(rw.status), "request selesai", attrs...)
		})
	}
}

// levelForStatus menaikkan level log sesuai keparahan supaya kegagalan tidak
// tenggelam di antara request normal.
func levelForStatus(status int) slog.Level {
	switch {
	case status >= 500:
		return slog.LevelError
	case status >= 400:
		return slog.LevelWarn
	default:
		return slog.LevelInfo
	}
}

// redactQuery menyusun ulang query string dengan nilai parameter sensitif diganti
// penanda. Query yang tidak bisa diurai disamarkan seluruhnya, bukan diteruskan
// mentah.
func redactQuery(raw string) string {
	if raw == "" {
		return ""
	}
	values, err := url.ParseQuery(raw)
	if err != nil {
		return observability.RedactedValue
	}
	for key, vals := range values {
		if !isSensitiveQueryKey(key) {
			continue
		}
		for i := range vals {
			vals[i] = observability.RedactedValue
		}
	}
	return values.Encode()
}

// isSensitiveQueryKey melengkapi daftar observability dengan nama parameter yang
// hanya muncul di query string.
func isSensitiveQueryKey(key string) bool {
	if observability.IsSensitiveKey(key) {
		return true
	}
	switch strings.ToLower(key) {
	case "key", "token", "signature", "sig", "code", "state", "assertion", "client_secret":
		return true
	}
	return false
}

func clientIPForLog(r *http.Request) string {
	if ip, ok := ClientIPFrom(r.Context()); ok {
		return ip.String()
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

// --- RealIP -----------------------------------------------------------------

const (
	headerForwardedFor = "X-Forwarded-For"
	headerRealIP       = "X-Real-Ip"
)

type clientIPCtxKey struct{}

// RealIP menentukan IP klien dan menyimpannya di context.
//
// X-Forwarded-For dan X-Real-Ip hanya dipercaya bila peer langsung koneksi berada di
// salah satu prefix trustedProxies. Daftar kosong berarti tidak ada proxy yang
// dipercaya dan RemoteAddr selalu yang dipakai.
//
// Default ketat ini penting: IP dipakai sebagai kunci rate limit dan daftar ban,
// jadi kalau header apa pun dipercaya tanpa syarat, siapa saja bisa melewati rate
// limit dengan mengganti satu header, atau memancing ban atas alamat orang lain.
//
// RemoteAddr sengaja TIDAK ditimpa. Peer asli tetap tersedia untuk menelusuri
// masalah tingkat koneksi, dan kode lain tidak jadi diam-diam mempercayai nilai yang
// berasal dari header hanya karena membaca RemoteAddr. Ambil hasilnya lewat
// ClientIPFrom.
func RealIP(trustedProxies []netip.Prefix) func(http.Handler) http.Handler {
	trusted := slices.Clone(trustedProxies)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := resolveClientIP(r, trusted)
			if !ip.IsValid() {
				// RemoteAddr tidak bisa diurai (mis. unix socket): jangan menaruh nilai
				// palsu di context, biarkan pemanggil tahu bahwa IP tidak diketahui.
				next.ServeHTTP(w, r)
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), clientIPCtxKey{}, ip)))
		})
	}
}

// ClientIPFrom mengembalikan IP klien hasil resolusi RealIP. Nilai kedua false bila
// RealIP tidak dipasang atau alamatnya tidak bisa ditentukan.
func ClientIPFrom(ctx context.Context) (netip.Addr, bool) {
	ip, ok := ctx.Value(clientIPCtxKey{}).(netip.Addr)
	return ip, ok && ip.IsValid()
}

// ParseTrustedProxies mengubah daftar teks berisi CIDR ("10.0.0.0/8") atau IP
// tunggal ("192.0.2.10") menjadi prefix untuk RealIP. Entri kosong dilewati.
func ParseTrustedProxies(values []string) ([]netip.Prefix, error) {
	out := make([]netip.Prefix, 0, len(values))
	for _, raw := range values {
		v := strings.TrimSpace(raw)
		if v == "" {
			continue
		}
		if prefix, err := netip.ParsePrefix(v); err == nil {
			out = append(out, prefix.Masked())
			continue
		}
		addr, err := netip.ParseAddr(v)
		if err != nil {
			return nil, fmt.Errorf("proxy terpercaya %q bukan IP atau CIDR yang sah", v)
		}
		addr = normalizeIP(addr)
		out = append(out, netip.PrefixFrom(addr, addr.BitLen()))
	}
	return out, nil
}

// PrivateProxyPrefixes mengembalikan rentang loopback dan jaringan privat, pilihan
// yang tepat bila gateway berada di belakang reverse proxy atau load balancer pada
// jaringan internal.
//
// Jangan dipakai bila gateway menerima koneksi langsung dari internet: setiap
// alamat di rentang ini akan diizinkan memalsukan X-Forwarded-For.
func PrivateProxyPrefixes() []netip.Prefix {
	return []netip.Prefix{
		netip.MustParsePrefix("127.0.0.0/8"),
		netip.MustParsePrefix("10.0.0.0/8"),
		netip.MustParsePrefix("172.16.0.0/12"),
		netip.MustParsePrefix("192.168.0.0/16"),
		netip.MustParsePrefix("169.254.0.0/16"),
		netip.MustParsePrefix("::1/128"),
		netip.MustParsePrefix("fc00::/7"),
		netip.MustParsePrefix("fe80::/10"),
	}
}

func resolveClientIP(r *http.Request, trusted []netip.Prefix) netip.Addr {
	peer := parseIP(r.RemoteAddr)
	if len(trusted) == 0 || !prefixesContain(trusted, peer) {
		return peer
	}
	if ip := clientFromForwardedFor(r.Header.Values(headerForwardedFor), trusted); ip.IsValid() {
		return ip
	}
	// X-Real-Ip hanya memuat satu alamat, jadi tidak ada rantai untuk ditelusuri.
	if ip := parseIP(r.Header.Get(headerRealIP)); ip.IsValid() {
		return ip
	}
	return peer
}

// clientFromForwardedFor menelusuri X-Forwarded-For dari kanan ke kiri dan
// mengembalikan hop pertama yang bukan proxy terpercaya.
//
// Arah ini yang benar: header bertambah dari kiri ke kanan sewaktu melewati proxy,
// jadi entri paling kanan ditulis oleh proxy terakhir dan tidak bisa dipalsukan
// klien, sementara entri paling kiri justru sepenuhnya di bawah kendali klien.
//
// Entri yang tidak bisa diurai menghentikan penelusuran: melewatinya sama dengan
// mempercayai entri di sebelah kiri, dan menyisipkan sampah adalah cara paling
// mudah memaksa kita melangkah terlalu jauh ke kiri. Bila semua hop terpercaya,
// entri paling kiri yang dipakai — itulah klien asli di dalam jaringan sendiri.
func clientFromForwardedFor(headers []string, trusted []netip.Prefix) netip.Addr {
	var hops []netip.Addr
	for _, header := range headers {
		for _, part := range strings.Split(header, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			hops = append(hops, parseIP(part))
		}
	}
	for i := len(hops) - 1; i >= 0; i-- {
		if !hops[i].IsValid() {
			return netip.Addr{}
		}
		if !prefixesContain(trusted, hops[i]) {
			return hops[i]
		}
	}
	if len(hops) > 0 {
		return hops[0]
	}
	return netip.Addr{}
}

// parseIP menerima bentuk yang lazim muncul di RemoteAddr dan header proxy:
// "1.2.3.4", "1.2.3.4:5678", "::1", dan "[::1]:5678".
func parseIP(v string) netip.Addr {
	v = strings.TrimSpace(v)
	if v == "" {
		return netip.Addr{}
	}
	if addrPort, err := netip.ParseAddrPort(v); err == nil {
		return normalizeIP(addrPort.Addr())
	}
	if addr, err := netip.ParseAddr(v); err == nil {
		return normalizeIP(addr)
	}
	if host, _, err := net.SplitHostPort(v); err == nil {
		if addr, err := netip.ParseAddr(host); err == nil {
			return normalizeIP(addr)
		}
	}
	return netip.Addr{}
}

// normalizeIP menyeragamkan alamat supaya bisa dibandingkan dan dipakai sebagai
// kunci rate limit: bentuk ::ffff:a.b.c.d dijadikan IPv4 dan zona IPv6 dibuang,
// sehingga satu klien tidak pernah tampil sebagai dua alamat berbeda.
func normalizeIP(addr netip.Addr) netip.Addr {
	return addr.Unmap().WithZone("")
}

func prefixesContain(prefixes []netip.Prefix, ip netip.Addr) bool {
	if !ip.IsValid() {
		return false
	}
	for _, prefix := range prefixes {
		if prefix.Contains(ip) {
			return true
		}
	}
	return false
}
