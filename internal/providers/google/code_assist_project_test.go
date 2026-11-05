package google

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/NexGen-X/Route-X/internal/providers"
	"github.com/NexGen-X/Route-X/internal/security"
)

// newAntigravityServer membuat provider Antigravity (nama memuat
// "antigravity" agar isAntigravity() benar pada base URL httptest) dengan
// project Code Assist yang bisa disetel, beserta log permintaannya.
func newAntigravityServer(t *testing.T, project string, handler func(w http.ResponseWriter, r *http.Request)) (*Provider, *requestLog) {
	t.Helper()
	log := &requestLog{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.record(r)
		handler(w, r)
	}))
	t.Cleanup(srv.Close)

	p, err := New(Config{
		Name:       "antigravity-uji",
		BaseURL:    srv.URL,
		Credential: security.Secret(testKey),
		SSRFPolicy: testPolicy(),
		Timeout:    5 * time.Second,
		Project:    project,
	})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	return p, log
}

const daftarModelAntigravity = `{
  "models": {
    "gemini-3-pro": {"displayName": "Gemini 3 Pro"},
    "model-argon-x": {"displayName": "Argon"}
  },
  "agentModelSorts": [{"groups": [{"modelIds": ["gemini-3-pro"]}]}]
}`

// TestModelsAntigravityMeneruskanProject membuktikan B-157 di lapisan
// adapter: body fetchAvailableModels membawa project terselesaikan, dan
// tanpa project body tetap "{}" persis perilaku lama.
func TestModelsAntigravityMeneruskanProject(t *testing.T) {
	p, log := newAntigravityServer(t, "proj-b157", jsonHandler(http.StatusOK, daftarModelAntigravity))
	models, err := p.Models(context.Background())
	if err != nil {
		t.Fatalf("Models: %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("jumlah model = %d, ingin 2: %+v", len(models), models)
	}
	var body map[string]any
	if err := json.Unmarshal(log.raw(t, 0), &body); err != nil {
		t.Fatalf("body fetchAvailableModels bukan JSON: %v", err)
	}
	if body["project"] != "proj-b157" {
		t.Errorf("body project = %v, ingin proj-b157 (body %s)", body["project"], log.raw(t, 0))
	}

	p2, log2 := newAntigravityServer(t, "", jsonHandler(http.StatusOK, daftarModelAntigravity))
	if _, err := p2.Models(context.Background()); err != nil {
		t.Fatalf("Models tanpa project: %v", err)
	}
	if got := string(log2.raw(t, 0)); got != "{}" {
		t.Errorf("body tanpa project = %s, ingin {}", got)
	}
}

// TestDiscoveryAntigravityUserAgentHub membuktikan kunci katalog
// B-157 tahap 4 di lapisan adapter: kedua panggilan fetchAvailableModels
// (Models dan HealthCheck) mengirim User-Agent hub Antigravity persis
// seperti konstanta bersama, sedangkan panggilan generate
// mempertahankan User-Agent bawaan adapter — cakupan perubahan memang
// hanya discovery, sesuai bukti kontrol tahap 4 (User-Agent adalah
// penentu tunggal; header lain tidak ditambahkan).
func TestDiscoveryAntigravityUserAgentHub(t *testing.T) {
	p, log := newAntigravityServer(t, "proj-b157", jsonHandler(http.StatusOK, daftarModelAntigravity))
	if _, err := p.Models(context.Background()); err != nil {
		t.Fatalf("Models: %v", err)
	}
	if ua := log.Header("User-Agent"); ua != providers.AntigravityHubUserAgent {
		t.Errorf("User-Agent Models = %q, ingin %q", ua, providers.AntigravityHubUserAgent)
	}

	p2, log2 := newAntigravityServer(t, "proj-b157", jsonHandler(http.StatusOK, daftarModelAntigravity))
	if res := p2.HealthCheck(context.Background()); !res.Healthy {
		t.Fatalf("health check tidak sehat: %+v", res)
	}
	if ua := log2.Header("User-Agent"); ua != providers.AntigravityHubUserAgent {
		t.Errorf("User-Agent HealthCheck = %q, ingin %q", ua, providers.AntigravityHubUserAgent)
	}

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"response": `+textResponse+`}`)
	}
	p3, log3 := newAntigravityServer(t, "proj-b157", handler)
	if _, err := p3.ChatCompletion(context.Background(), simpleRequest()); err != nil {
		t.Fatalf("ChatCompletion: %v", err)
	}
	if ua := log3.Header("User-Agent"); ua != "antigravity/1.0.0" {
		t.Errorf("User-Agent generate = %q, ingin tetap antigravity/1.0.0 (perubahan hanya discovery)", ua)
	}
}

// TestHealthCheckAntigravityMeneruskanProject membuktikan jalur health
// memakai body yang sama dengan Models.
func TestHealthCheckAntigravityMeneruskanProject(t *testing.T) {
	p, log := newAntigravityServer(t, "proj-b157", jsonHandler(http.StatusOK, daftarModelAntigravity))
	res := p.HealthCheck(context.Background())
	if !res.Healthy {
		t.Fatalf("health check tidak sehat: %+v", res)
	}
	var body map[string]any
	if err := json.Unmarshal(log.raw(t, 0), &body); err != nil {
		t.Fatalf("body health bukan JSON: %v", err)
	}
	if body["project"] != "proj-b157" {
		t.Errorf("body health project = %v, ingin proj-b157", body["project"])
	}
}

// TestChatCompletionAntigravityProjectDiAmplop membuktikan amplop
// generateContent membawa project terselesaikan, bukan lagi string kosong
// hardcoded — dan tetap kosong bila project memang tidak ada.
func TestChatCompletionAntigravityProjectDiAmplop(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"response": `+textResponse+`}`)
	}

	p, log := newAntigravityServer(t, "proj-b157", handler)
	resp, err := p.ChatCompletion(context.Background(), simpleRequest())
	if err != nil {
		t.Fatalf("ChatCompletion: %v", err)
	}
	if resp == nil || len(resp.Choices) == 0 || resp.Choices[0].Message.Content != "halo juga" {
		t.Fatalf("respons tidak terurai dari amplop: %+v", resp)
	}
	var amplop struct {
		Model   string          `json:"model"`
		Project string          `json:"project"`
		Request json.RawMessage `json:"request"`
	}
	if err := json.Unmarshal(log.raw(t, 0), &amplop); err != nil {
		t.Fatalf("amplop bukan JSON: %v", err)
	}
	if amplop.Project != "proj-b157" {
		t.Errorf("amplop project = %q, ingin proj-b157", amplop.Project)
	}

	p2, log2 := newAntigravityServer(t, "", handler)
	if _, err := p2.ChatCompletion(context.Background(), simpleRequest()); err != nil {
		t.Fatalf("ChatCompletion tanpa project: %v", err)
	}
	if err := json.Unmarshal(log2.raw(t, 0), &amplop); err != nil {
		t.Fatalf("amplop kedua bukan JSON: %v", err)
	}
	if amplop.Project != "" {
		t.Errorf("amplop tanpa project = %q, ingin kosong", amplop.Project)
	}
}
