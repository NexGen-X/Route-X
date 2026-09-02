// Package observability menyediakan logging terstruktur, korelasi request, dan metrik.
//
// Logger di sini adalah satu-satunya cara paket lain menulis log, supaya redaksi
// rahasia berlaku seragam di seluruh aplikasi.
package observability

import (
	"context"
	"io"
	"log/slog"
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
	return a
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
