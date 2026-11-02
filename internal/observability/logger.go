// Package observability menyediakan logging terstruktur, korelasi request, dan metrik.
//
// Logger di sini adalah satu-satunya cara paket lain menulis log, supaya redaksi
// rahasia berlaku seragam di seluruh aplikasi.
package observability

import (
	"context"
	"io"
	"log/slog"
	"net/url"
	"regexp"
	"strings"
)

// RedactedValue adalah pengganti yang muncul di log untuk atribut sensitif.
const RedactedValue = "[REDACTED]"

// sensitiveKeys adalah daftar nama atribut yang nilainya selalu disamarkan,
// dicocokkan persis (case-insensitive).
//
// Sengaja memakai pencocokan persis, BUKAN substring: produk ini melaporkan
// "input_tokens", "output_tokens", dan "total_tokens" sebagai metrik inti, jadi
// mencocokkan substring "token" akan menyamarkan angka pemakaian yang justru harus
// terlihat. Lapisan ini melengkapi security.Secret — Secret menutup nilai yang
// bertipe benar, daftar ini menangkap string mentah yang kebetulan di-log dengan
// nama kunci sensitif.
var sensitiveKeys = map[string]struct{}{
	"authorization":       {},
	"proxy-authorization": {},
	"cookie":              {},
	"set-cookie":          {},
	"x-api-key":           {},
	"x-goog-api-key":      {},
	"api-key":             {},
	"api_key":             {},
	"apikey":              {},
	"password":            {},
	"passwd":              {},
	"secret":              {},
	"session_secret":      {},
	"session_token":       {},
	"encryption_key":      {},
	"api_key_pepper":      {},
	"credential":          {},
	"credentials":         {},
	"private_key":         {},
	"access_token":        {},
	"refresh_token":       {},
	"auth_token":          {},
	"csrf_token":          {},
	"bearer":              {},
	"database_url":        {},
	"redis_url":           {},
	"dsn":                 {},
	"connection_string":   {},
	// Nama majemuk spesifik yang dulu lolos (audit3 V-EM-32). Nama POLOS
	// "token"/"key" sengaja TIDAK ditambahkan: kebijakan lama paket ini
	// (dijaga TestIsSensitiveKey) adalah atribut bernama "token" bisa
	// berarti hitungan token yang harus terlihat. Penugasan token= di
	// dalam TEKS bebas tetap disamarkan ScrubLogText di bawah, yang
	// merupakan vektor bocornya error/DSN (audit3 V-EM-33).
	"client_secret":  {},
	"webhook_secret": {},
	"accesstoken":    {},
	"refreshtoken":   {},
	"secret_key":     {},
	"signing_secret": {},
	"bot_token":      {},
	"metrics_token":  {},
}

// IsSensitiveKey melaporkan apakah atribut dengan nama ini harus disamarkan.
func IsSensitiveKey(key string) bool {
	_, ok := sensitiveKeys[strings.ToLower(key)]
	return ok
}

// LoggerOptions mengatur pembuatan logger.
type LoggerOptions struct {
	Level slog.Level
	// Pretty menghasilkan keluaran teks yang enak dibaca manusia untuk development.
	// Di produksi biarkan false agar formatnya JSON dan bisa dicerna log shipper.
	Pretty bool
	// AddSource menyertakan file:line pemanggil. Berguna saat debug, ada biayanya.
	AddSource bool
}

// NewLogger membuat logger dengan redaksi otomatis aktif.
func NewLogger(w io.Writer, opts LoggerOptions) *slog.Logger {
	handlerOpts := &slog.HandlerOptions{
		Level:       opts.Level,
		AddSource:   opts.AddSource,
		ReplaceAttr: redactAttr,
	}

	var h slog.Handler
	if opts.Pretty {
		h = slog.NewTextHandler(w, handlerOpts)
	} else {
		h = slog.NewJSONHandler(w, handlerOpts)
	}
	return slog.New(h)
}

// redactAttr menyamarkan atribut sensitif di semua level kedalaman, termasuk di dalam
// grup, sebelum nilainya sampai ke handler.
func redactAttr(groups []string, a slog.Attr) slog.Attr {
	if IsSensitiveKey(a.Key) {
		return slog.String(a.Key, RedactedValue)
	}
	// Turunkan ke anggota grup agar atribut sensitif di dalam slog.Group ikut tersamar.
	if a.Value.Kind() == slog.KindGroup {
		members := a.Value.Group()
		out := make([]slog.Attr, 0, len(members))
		for _, m := range members {
			out = append(out, redactAttr(append(groups, a.Key), m))
		}
		return slog.Attr{Key: a.Key, Value: slog.GroupValue(out...)}
	}
	// Nilai bebas (pesan error, nilai panic) bisa MEMUAT rahasia di dalam
	// teksnya — DSN berkredensial di dalam error berantai, URL proxy lengkap
	// di *url.Error, token yang dipantulkan upstream (audit3 V-EM-32/33).
	// Redaksi berbasis nama kunci tidak menangkapnya, jadi teks string dan
	// error dilewatkan penyaring pola kredensial di bawah ini. Polanya sempit
	// (hanya bentuk yang memang kredensial), sehingga pesan biasa lolos utuh.
	if a.Value.Kind() == slog.KindString {
		if bersih := ScrubLogText(a.Value.String()); bersih != a.Value.String() {
			return slog.String(a.Key, bersih)
		}
		return a
	}
	if a.Value.Kind() == slog.KindAny {
		if err, ok := a.Value.Any().(error); ok && err != nil {
			if bersih := ScrubLogText(err.Error()); bersih != err.Error() {
				return slog.String(a.Key, bersih)
			}
		}
	}
	return a
}

// Pola-pola kredensial di dalam teks bebas untuk ScrubLogText.
var (
	// URL dengan userinfo atau query: keduanya tempat kredensial/token hidup.
	polaURLLog = regexp.MustCompile(`https?://[^\s"'<>()]+`)
	// Skema koneksi lain yang userinfo-nya memuat kata sandi (postgres, redis, ...).
	polaDSNLog = regexp.MustCompile(`\b[a-z][a-z0-9+.-]*://[^\s"'<>()]+`)
	// Header otorisasi yang terbawa ke pesan error.
	polaBearerLog = regexp.MustCompile(`(?i)\bBearer\s+[A-Za-z0-9._~+/=-]{8,}`)
	// Bentuk token yang dikenal: kunci API bergaya OpenAI, secret Google,
	// token GitHub, token Slack, JWT.
	polaTokenLog = regexp.MustCompile(`\b(?:sk-[A-Za-z0-9_-]{8,}|GOCSPX-[A-Za-z0-9_-]+|gh[pousr]_[A-Za-z0-9]{20,}|xox[baprs]-[A-Za-z0-9-]{10,}|eyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{4,})`)
	// Penugasan nama=nilai untuk nama-nama rahasia di dalam teks. Kelas nilai
	// mengecualikan & supaya penugasan di dalam query string tidak menelan
	// parameter sesudahnya (api_key=…&model=gpt-4o harus menyisakan model).
	polaTugasLog = regexp.MustCompile(`(?i)\b(password|passwd|client_secret|webhook_secret|api[_-]?key|access[_-]?token|refresh[_-]?token|secret|token)\s*[:=]\s*[^\s,;"'&]+`)
)

// ScrubLogText menyamarkan kredensial yang tertanam di dalam teks bebas
// (pesan error, nilai panic, stderr perkakas) sebelum teks itu ditulis ke log
// atau kolom basis data. Diekspor supaya jalur yang mencatat teks mentah di
// luar logger (mis. stderr backup) bisa memakai penyaring yang sama.
func ScrubLogText(s string) string {
	if s == "" {
		return s
	}
	bersih := polaURLLog.ReplaceAllStringFunc(s, samarkanURLLog)
	bersih = polaDSNLog.ReplaceAllStringFunc(bersih, samarkanURLLog)
	bersih = polaBearerLog.ReplaceAllString(bersih, "Bearer "+RedactedValue)
	bersih = polaTokenLog.ReplaceAllString(bersih, RedactedValue)
	bersih = polaTugasLog.ReplaceAllString(bersih, "$1="+RedactedValue)
	return bersih
}

// samarkanURLLog membuang userinfo dan query dari satu URL di dalam teks,
// mempertahankan skema/host/path supaya pesan tetap bisa didiagnosis.
func samarkanURLLog(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" {
		return "[URL_TIDAK_SAH]"
	}
	u.User = nil
	u.RawQuery = ""
	u.ForceQuery = false
	return u.String()
}

// --- Korelasi request -------------------------------------------------------

type loggerCtxKey struct{}
type requestIDCtxKey struct{}

// WithLogger menautkan logger ke context, biasanya logger yang sudah dibubuhi
// request_id sehingga setiap log dari satu request bisa dikorelasikan.
func WithLogger(ctx context.Context, l *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerCtxKey{}, l)
}

// LoggerFrom mengambil logger dari context. Selalu mengembalikan logger yang bisa
// dipakai — jatuh ke slog.Default() bila context belum punya.
func LoggerFrom(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(loggerCtxKey{}).(*slog.Logger); ok && l != nil {
		return l
	}
	return slog.Default()
}

// WithRequestID menautkan ID request ke context untuk tracing.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDCtxKey{}, id)
}

// RequestIDFrom mengambil ID request dari context, "" bila tidak ada.
func RequestIDFrom(ctx context.Context) string {
	if id, ok := ctx.Value(requestIDCtxKey{}).(string); ok {
		return id
	}
	return ""
}
