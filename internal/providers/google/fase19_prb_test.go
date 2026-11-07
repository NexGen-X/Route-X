package google

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/NexGen-X/Route-X/internal/providers"
	"github.com/NexGen-X/Route-X/internal/security"
)

// TestB427ConstNotSkema adalah pengunci B-427: normalizeSchema
// dulu meloloskan const dan not utuh ke upstream, di luar subset
// Gemini. Kini const diterjemahkan setia menjadi enum:[nilai]
// dan not dibuang — rekursif, termasuk di dalam anyOf, items,
// dan properties bersarang.
func TestB427ConstNotSkema(t *testing.T) {
	schema := json.RawMessage(`{
		"type": "object",
		"properties": {
			"mode": {"type": "string", "const": "cepat"},
			"lain": {"type": "string", "not": {"pattern": "^x"}},
			"dalam": {"anyOf": [{"type": "integer", "const": 7}, {"type": "null"}]},
			"daftar": {"type": "array", "items": {"type": "string", "const": "tetap"}}
		}
	}`)
	out := string(sanitizeSchema(schema))
	if strings.Contains(out, `"const"`) {
		t.Errorf("const masih lolos ke skema hasil: %s", out)
	}
	if strings.Contains(out, `"not"`) {
		t.Errorf("not masih lolos ke skema hasil: %s", out)
	}
	for _, mau := range []string{`"enum":["cepat"]`, `"enum":[7]`, `"enum":["tetap"]`} {
		if !strings.Contains(out, mau) {
			t.Errorf("skema hasil tidak memuat %s: %s", mau, out)
		}
	}
}

// TestB428AmplopErrorAntigravity adalah pengunci B-428: error di
// dalam amplop {"response":{"error":…}} dulu tak pernah dibuka —
// 429 RESOURCE_EXHAUSTED jatuh ke kind server generik tanpa kode
// dan tanpa RetryInfo. Kini amplop dibuka dan klasifikasinya
// terbaca, di jalur non-streaming maupun streaming.
func TestB428AmplopErrorAntigravity(t *testing.T) {
	bodyAmplop := `{"response":{"error":{"code":429,"message":"kuota habis","status":"RESOURCE_EXHAUSTED","details":[{"@type":"type.googleapis.com/google.rpc.RetryInfo","retryDelay":"7s"}]}}}`

	t.Run("non-streaming", func(t *testing.T) {
		p, _ := newServer(t, jsonHandler(200, bodyAmplop))
		p.name = "antigravity-uji"
		_, err := p.ChatCompletion(context.Background(), simpleRequest())
		pe := providers.AsError(err)
		if pe == nil {
			t.Fatalf("err = %v, mau *providers.Error", err)
		}
		if pe.Kind != providers.ErrKindRateLimit {
			t.Errorf("kind = %q, mau rate_limit", pe.Kind)
		}
		if pe.UpstreamCode != "RESOURCE_EXHAUSTED" {
			t.Errorf("UpstreamCode = %q, mau RESOURCE_EXHAUSTED", pe.UpstreamCode)
		}
		if pe.RetryAfter != 7*time.Second {
			t.Errorf("RetryAfter = %v, mau 7s dari RetryInfo B-266", pe.RetryAfter)
		}
	})

	t.Run("streaming", func(t *testing.T) {
		p, _ := newServer(t, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(200)
			_, _ = io.WriteString(w, "data: "+bodyAmplop+"\n\n")
		})
		p.name = "antigravity-uji"
		st, err := p.ChatCompletionStream(context.Background(), simpleRequest())
		if err != nil {
			t.Fatalf("ChatCompletionStream: %v", err)
		}
		defer st.Close()
		_, err = st.Recv()
		pe := providers.AsError(err)
		if pe == nil {
			t.Fatalf("Recv err = %v, mau *providers.Error rate_limit dari amplop", err)
		}
		if pe.Kind != providers.ErrKindRateLimit {
			t.Errorf("kind streaming = %q, mau rate_limit", pe.Kind)
		}
		if pe.UpstreamCode != "RESOURCE_EXHAUSTED" {
			t.Errorf("UpstreamCode streaming = %q, mau RESOURCE_EXHAUSTED", pe.UpstreamCode)
		}
	})
}

// TestB429GambarHasilToolGoogle adalah pengunci B-429 (sisi
// Google): gambar pada pesan hasil tool dulu hilang — cabang
// RoleTool hanya membungkus Text() ke functionResponse. Kini ia
// menjadi part inlineData pendamping pada body upstream.
func TestB429GambarHasilToolGoogle(t *testing.T) {
	p, log := newServer(t, jsonHandler(200, textResponse))
	req := &providers.ChatRequest{
		Model: "gemini-2.5-flash",
		Messages: []providers.Message{
			{Role: providers.RoleUser, Content: "pakai tool"},
			{Role: providers.RoleAssistant, ToolCalls: []providers.ToolCall{{
				ID: "call_0_tangkap", Type: "function",
				Function: providers.FunctionCall{Name: "tangkap", Arguments: "{}"},
			}}},
			{Role: providers.RoleTool, ToolCallID: "call_0_tangkap", Parts: []providers.ContentPart{
				{Type: providers.PartTypeText, Text: "hasil tangkapan:"},
				{Type: providers.PartTypeImageURL, ImageURL: &providers.ImageURL{URL: "data:image/png;base64,aVBvcg=="}},
			}},
		},
	}
	if _, err := p.ChatCompletion(context.Background(), req); err != nil {
		t.Fatalf("ChatCompletion: %v", err)
	}
	body := string(log.raw(t, 0))
	if !strings.Contains(body, "aVBvcg==") {
		t.Errorf("data gambar tidak muncul di body upstream: %s", body)
	}
	if !strings.Contains(body, "inlineData") {
		t.Errorf("part inlineData tidak ada di body upstream: %s", body)
	}
}

// TestB430UrutanModelsAntigravityDeterministik adalah pengunci
// B-430: Models() mode Antigravity mengiterasi map sehingga
// urutannya acak antar panggilan. Kini urutannya deterministik
// (kunci map terurut) dan himpunannya tetap utuh (dedup B-264).
func TestB430UrutanModelsAntigravityDeterministik(t *testing.T) {
	katalog := `{"models":{"zebra-model":{},"alpha-model":{},"mike-model":{},"beta-model":{}},"commandModelIds":["alpha-model","cmd-model"]}`
	// Server tiruan lokal; nama provider memuat "antigravity"
	// sehingga mode Antigravity aktif walau base URL-nya lokal.
	log := &requestLog{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.record(r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, katalog)
	}))
	t.Cleanup(srv.Close)
	p, err := New(Config{
		Name:       "antigravity-uji",
		BaseURL:    srv.URL,
		Credential: security.Secret(testKey),
		SSRFPolicy: testPolicy(),
		Timeout:    5 * time.Second,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	pertama, err := p.Models(context.Background())
	if err != nil {
		t.Fatalf("Models: %v", err)
	}
	if len(pertama) != 5 {
		t.Fatalf("jumlah model = %d, mau 5 (4 map + cmd-model, alpha terdedup): %+v", len(pertama), pertama)
	}
	// Blok map harus terurut menurut ID.
	mauAwalan := []string{"alpha-model", "beta-model", "mike-model", "zebra-model"}
	for i, mau := range mauAwalan {
		if pertama[i].ID != mau {
			t.Errorf("model[%d] = %q, mau %q (urutan map terurut)", i, pertama[i].ID, mau)
		}
	}
	for putaran := 0; putaran < 10; putaran++ {
		lagi, err := p.Models(context.Background())
		if err != nil {
			t.Fatalf("Models putaran %d: %v", putaran, err)
		}
		if len(lagi) != len(pertama) {
			t.Fatalf("putaran %d: jumlah %d != %d", putaran, len(lagi), len(pertama))
		}
		for i := range lagi {
			if lagi[i].ID != pertama[i].ID {
				t.Fatalf("putaran %d: urutan berubah di indeks %d: %q != %q", putaran, i, lagi[i].ID, pertama[i].ID)
			}
		}
	}
}

// TestB431RetryAfterBody200Google adalah pengunci B-431 (sisi
// Google): error yang menumpang di body respons 200 dengan
// header Retry-After: 17 dan TANPA RetryInfo dulu menghasilkan
// RetryAfter 0. Kini header terbaca (dan menang atas body).
func TestB431RetryAfterBody200Google(t *testing.T) {
	p, _ := newServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Retry-After", "17")
		w.WriteHeader(200)
		_, _ = io.WriteString(w, `{"error":{"code":429,"message":"kuota habis","status":"RESOURCE_EXHAUSTED"}}`)
	})
	_, err := p.ChatCompletion(context.Background(), simpleRequest())
	pe := providers.AsError(err)
	if pe == nil {
		t.Fatalf("err = %v, mau *providers.Error", err)
	}
	if pe.Kind != providers.ErrKindRateLimit {
		t.Errorf("kind = %q, mau rate_limit", pe.Kind)
	}
	if pe.RetryAfter != 17*time.Second {
		t.Errorf("RetryAfter = %v, mau 17s — header body-200 dibuang", pe.RetryAfter)
	}
}
