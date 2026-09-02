package observability

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

// logJSON menjalankan satu panggilan log dan mengembalikan hasilnya sebagai map.
func logJSON(t *testing.T, level slog.Level, fn func(*slog.Logger)) map[string]any {
	t.Helper()
	var buf bytes.Buffer
	fn(NewLogger(&buf, LoggerOptions{Level: level}))

	if buf.Len() == 0 {
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("keluaran bukan JSON yang sah: %v\n%s", err, buf.String())
	}
	return out
}

func TestRedactsSensitiveKeys(t *testing.T) {
	keys := []string{
		"authorization", "Authorization", "AUTHORIZATION",
		"proxy-authorization", "cookie", "set-cookie",
		"x-api-key", "x-goog-api-key", "api-key", "api_key", "apikey",
		"password", "passwd", "secret", "session_secret", "session_token",
		"encryption_key", "api_key_pepper", "credential", "credentials",
		"private_key", "access_token", "refresh_token", "auth_token",
		"csrf_token", "bearer", "database_url", "redis_url", "dsn",
		"connection_string",
	}
	for _, key := range keys {
		t.Run(key, func(t *testing.T) {
			out := logJSON(t, slog.LevelInfo, func(l *slog.Logger) {
				l.Info("uji", key, "nilai-sangat-rahasia")
			})
			if got := out[key]; got != RedactedValue {
				t.Errorf("kunci %q = %v, mau %q", key, got, RedactedValue)
			}
		})
	}
}

// Ini perilaku khas produk ini: hitungan token adalah metrik inti dan HARUS terlihat.
// Pencocokan substring pada "token" akan merusaknya, jadi dijaga dengan test.
func TestDoesNotRedactTokenCounts(t *testing.T) {
	visible := map[string]any{
		"input_tokens":  float64(1200),
		"output_tokens": float64(340),
		"total_tokens":  float64(1540),
		"tokens":        float64(1540),
		"token_count":   float64(1540),
		"model":         "gpt-5",
		"provider":      "openai",
		"cost_usd":      0.0123,
		"latency_ms":    float64(842),
		"ttft_ms":       float64(210),
		"status":        float64(200),
	}

	out := logJSON(t, slog.LevelInfo, func(l *slog.Logger) {
		attrs := make([]any, 0, len(visible)*2)
		for k, v := range visible {
			attrs = append(attrs, k, v)
		}
		l.Info("request selesai", attrs...)
	})

	for k, want := range visible {
		if got := out[k]; got != want {
			t.Errorf("kunci %q = %v (%T), mau %v (%T) — metrik ini tidak boleh disamarkan", k, got, got, want, want)
		}
	}
}

func TestRedactsInsideGroup(t *testing.T) {
	out := logJSON(t, slog.LevelInfo, func(l *slog.Logger) {
		l.Info("upstream",
			slog.Group("provider",
				slog.String("name", "openai"),
				slog.String("api_key", "sk-rahasia-sekali"),
				slog.Group("nested",
					slog.String("password", "juga-rahasia"),
					slog.Int("total_tokens", 42),
				),
			),
		)
	})

	raw, _ := json.Marshal(out)
	if bytes.Contains(raw, []byte("rahasia")) {
		t.Fatalf("rahasia bocor dari dalam grup: %s", raw)
	}

	provider, ok := out["provider"].(map[string]any)
	if !ok {
		t.Fatalf("grup provider hilang: %v", out)
	}
	if provider["api_key"] != RedactedValue {
		t.Errorf("api_key dalam grup tidak disamarkan: %v", provider["api_key"])
	}
	if provider["name"] != "openai" {
		t.Errorf("nilai non-sensitif dalam grup ikut hilang: %v", provider["name"])
	}

	nested, ok := provider["nested"].(map[string]any)
	if !ok {
		t.Fatalf("grup bersarang hilang: %v", provider)
	}
	if nested["password"] != RedactedValue {
		t.Errorf("password bersarang tidak disamarkan: %v", nested["password"])
	}
	if nested["total_tokens"] != float64(42) {
		t.Errorf("total_tokens bersarang ikut disamarkan: %v", nested["total_tokens"])
	}
}

func TestLevelFiltering(t *testing.T) {
	if out := logJSON(t, slog.LevelWarn, func(l *slog.Logger) { l.Info("tidak boleh muncul") }); out != nil {
		t.Errorf("log Info lolos padahal level Warn: %v", out)
	}
	if out := logJSON(t, slog.LevelWarn, func(l *slog.Logger) { l.Warn("harus muncul") }); out == nil {
		t.Error("log Warn tidak muncul padahal level Warn")
	}
}

func TestPrettyOutputIsText(t *testing.T) {
	var buf bytes.Buffer
	NewLogger(&buf, LoggerOptions{Level: slog.LevelInfo, Pretty: true}).
		Info("halo", "api_key", "sk-rahasia")

	got := buf.String()
	if strings.HasPrefix(strings.TrimSpace(got), "{") {
		t.Errorf("Pretty seharusnya teks, bukan JSON: %s", got)
	}
	if strings.Contains(got, "sk-rahasia") {
		t.Errorf("mode Pretty membocorkan rahasia: %s", got)
	}
	if !strings.Contains(got, RedactedValue) {
		t.Errorf("mode Pretty tidak menyamarkan: %s", got)
	}
}

func TestAddSource(t *testing.T) {
	out := logJSON(t, slog.LevelInfo, func(l *slog.Logger) { l.Info("dengan sumber") })
	if _, ok := out["source"]; ok {
		t.Error("source muncul padahal AddSource=false")
	}

	var buf bytes.Buffer
	NewLogger(&buf, LoggerOptions{Level: slog.LevelInfo, AddSource: true}).Info("dengan sumber")
	if !strings.Contains(buf.String(), "logger_test.go") {
		t.Errorf("AddSource=true tidak menyertakan file pemanggil: %s", buf.String())
	}
}

func TestIsSensitiveKey(t *testing.T) {
	tests := []struct {
		key  string
		want bool
	}{
		{"authorization", true},
		{"AUTHORIZATION", true},
		{"Api_Key", true},
		{"total_tokens", false},
		{"input_tokens", false},
		{"model", false},
		{"", false},
		{"token", false}, // sengaja: hitungan token harus terlihat
	}
	for _, tc := range tests {
		if got := IsSensitiveKey(tc.key); got != tc.want {
			t.Errorf("IsSensitiveKey(%q) = %v, mau %v", tc.key, got, tc.want)
		}
	}
}

func TestLoggerContext(t *testing.T) {
	ctx := context.Background()

	// Tanpa logger di context, harus jatuh ke default yang tetap bisa dipakai.
	if LoggerFrom(ctx) == nil {
		t.Fatal("LoggerFrom mengembalikan nil untuk context kosong")
	}

	var buf bytes.Buffer
	logger := NewLogger(&buf, LoggerOptions{Level: slog.LevelInfo}).With("request_id", "req_123")
	ctx = WithLogger(ctx, logger)

	LoggerFrom(ctx).Info("dari context")
	if !strings.Contains(buf.String(), "req_123") {
		t.Errorf("logger dari context kehilangan atribut: %s", buf.String())
	}
}

func TestRequestIDContext(t *testing.T) {
	ctx := context.Background()
	if got := RequestIDFrom(ctx); got != "" {
		t.Errorf("RequestIDFrom context kosong = %q, mau string kosong", got)
	}

	ctx = WithRequestID(ctx, "req_abc")
	if got := RequestIDFrom(ctx); got != "req_abc" {
		t.Errorf("RequestIDFrom = %q, mau %q", got, "req_abc")
	}
}

// Logger harus aman dipakai bersamaan dari banyak goroutine, karena setiap request
// HTTP memakai turunan logger yang sama.
func TestLoggerConcurrentUse(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf, LoggerOptions{Level: slog.LevelInfo})

	done := make(chan struct{})
	for i := 0; i < 20; i++ {
		go func(n int) {
			defer func() { done <- struct{}{} }()
			logger.Info("paralel", "n", n, "api_key", "sk-rahasia")
		}(i)
	}
	for i := 0; i < 20; i++ {
		<-done
	}
	if strings.Contains(buf.String(), "sk-rahasia") {
		t.Error("rahasia bocor pada penulisan paralel")
	}
}
