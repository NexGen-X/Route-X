package oauth

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/NexGen-X/Route-X/internal/security"
)

// TestParseCodeAssistProject mengunci bentuk respons loadCodeAssist yang
// dikenal (B-157): cloudaicompanionProject sebagai string langsung dan
// sebagai objek ber-field id, beserta keadaan tanpa project yang sah
// (bukan error).
func TestParseCodeAssistProject(t *testing.T) {
	kasus := []struct {
		nama  string
		body  string
		ingin string
	}{
		{"bentuk string", `{"cloudaicompanionProject":"proj-string-1"}`, "proj-string-1"},
		{"bentuk objek", `{"cloudaicompanionProject":{"id":"proj-objek-2","name":"projects/proj-objek-2"}}`, "proj-objek-2"},
		{"field absen", `{"currentTier":{"id":"free-tier"}}`, ""},
		{"field null", `{"cloudaicompanionProject":null}`, ""},
		{"string kosong", `{"cloudaicompanionProject":""}`, ""},
		{"string spasi", `{"cloudaicompanionProject":"  proj-spasi  "}`, "proj-spasi"},
		{"objek tanpa id", `{"cloudaicompanionProject":{"name":"tanpa-id"}}`, ""},
		{"bentuk tak dikenal", `{"cloudaicompanionProject":42}`, ""},
	}
	for _, k := range kasus {
		t.Run(k.nama, func(t *testing.T) {
			got, err := ParseCodeAssistProject([]byte(k.body))
			if err != nil {
				t.Fatalf("ParseCodeAssistProject: %v", err)
			}
			if got != k.ingin {
				t.Errorf("project = %q, ingin %q", got, k.ingin)
			}
		})
	}

	if _, err := ParseCodeAssistProject([]byte(`{"cloudaicompanionProject":`)); err == nil {
		t.Error("JSON rusak harus menghasilkan error, bukan project kosong senyap")
	}
}

// TestParseCodeAssistTierSummary mengunci ringkasan tier aman-log dari
// bentuk respons yang tertangkap pada pembuktian B-157: konteks Antigravity
// yang berhasil membawa tier akun dan project, sedangkan konteks generik
// lama hanya membawa tier yang diizinkan/ditolak tanpa currentTier.
func TestParseCodeAssistTierSummary(t *testing.T) {
	body := []byte(`{
		"currentTier":{"id":"free-tier"},
		"paidTier":{"id":"g1-pro-tier"},
		"allowedTiers":[{"id":"free-tier"},{"id":"standard-tier"}],
		"cloudaicompanionProject":"project-uji",
		"gcpManaged":false,
		"upgradeSubscriptionUri":"https://example.invalid/?Email=rahasia%40example.invalid"
	}`)
	summary, err := parseCodeAssistTierSummary(body, "project-uji")
	if err != nil {
		t.Fatalf("parseCodeAssistTierSummary: %v", err)
	}
	if summary.CurrentTier != "free-tier" || summary.PaidTier != "g1-pro-tier" {
		t.Errorf("tier saat ini/berbayar = %q/%q, ingin free-tier/g1-pro-tier", summary.CurrentTier, summary.PaidTier)
	}
	if strings.Join(summary.AllowedTiers, ",") != "free-tier,standard-tier" {
		t.Errorf("allowedTiers = %v, ingin [free-tier standard-tier]", summary.AllowedTiers)
	}
	if summary.GCPManaged || !summary.ProjectAda || summary.IneligibleAda {
		t.Errorf("ringkasan = %+v, ingin gcpManaged=false, projectAda=true, ineligibleAda=false", summary)
	}

	generik, err := parseCodeAssistTierSummary([]byte(`{
		"allowedTiers":[{"id":"standard-tier"}],
		"ineligibleTiers":[{"id":"free-tier","reasonCode":"UNSUPPORTED_CLIENT"}]
	}`), "")
	if err != nil {
		t.Fatalf("parseCodeAssistTierSummary generik: %v", err)
	}
	if generik.CurrentTier != "" || generik.ProjectAda || !generik.IneligibleAda {
		t.Errorf("ringkasan generik = %+v, ingin currentTier kosong, projectAda=false, ineligibleAda=true", generik)
	}
	if _, err := parseCodeAssistTierSummary([]byte(`{"currentTier":`), ""); err == nil {
		t.Error("JSON rusak harus menghasilkan error ringkasan tier")
	}
}

// TestLoadCodeAssistProject membuktikan panggilan keluarnya: endpoint dari
// seam pengujian, header Authorization dari access token, sidik jari klien
// yang terbukti pada B-157, body metadata Code Assist, penguraian project,
// dan log tier yang tidak membocorkan body mentah respons.
func TestLoadCodeAssistProject(t *testing.T) {
	var logBuf bytes.Buffer
	loggerLama := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logBuf, nil)))
	defer slog.SetDefault(loggerLama)

	var reqBody []byte
	var reqAuth, reqUA, reqClientMetadata string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqBody, _ = io.ReadAll(r.Body)
		reqAuth = r.Header.Get("Authorization")
		reqUA = r.Header.Get("User-Agent")
		reqClientMetadata = r.Header.Get("Client-Metadata")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"cloudaicompanionProject":{"id":"proj-dari-server"},
			"currentTier":{"id":"free-tier"},
			"paidTier":{"id":"g1-pro-tier"},
			"allowedTiers":[{"id":"free-tier"},{"id":"standard-tier"}],
			"gcpManaged":false,
			"upgradeSubscriptionUri":"https://example.invalid/?Email=rahasia%40example.invalid"
		}`))
	}))
	defer srv.Close()

	klien := NewGoogleOAuthClient(nil).WithCodeAssistEndpoint(srv.URL)
	project, err := klien.LoadCodeAssistProject(context.Background(), "token-akses-uji", security.Secret(""))
	if err != nil {
		t.Fatalf("LoadCodeAssistProject: %v", err)
	}
	if project != "proj-dari-server" {
		t.Errorf("project = %q, ingin %q", project, "proj-dari-server")
	}
	if reqAuth != "Bearer token-akses-uji" {
		t.Errorf("header Authorization = %q, ingin Bearer token sesi", reqAuth)
	}
	if reqUA != codeAssistUserAgent {
		t.Errorf("header User-Agent = %q, ingin %q", reqUA, codeAssistUserAgent)
	}
	var clientMetadata map[string]string
	if err := json.Unmarshal([]byte(reqClientMetadata), &clientMetadata); err != nil {
		t.Fatalf("header Client-Metadata bukan JSON sah: %v", err)
	}
	if len(clientMetadata) != 3 || clientMetadata["ideType"] != "ANTIGRAVITY" ||
		clientMetadata["platform"] != "PLATFORM_UNSPECIFIED" || clientMetadata["pluginType"] != "GEMINI" {
		t.Errorf("header Client-Metadata = %v, ingin sidik jari Antigravity terbukti", clientMetadata)
	}
	var body struct {
		Metadata map[string]string `json:"metadata"`
	}
	if err := json.Unmarshal(reqBody, &body); err != nil {
		t.Fatalf("body request bukan JSON sah: %v", err)
	}
	if len(body.Metadata) != 4 || body.Metadata["ideType"] != "ANTIGRAVITY" ||
		body.Metadata["platform"] != "PLATFORM_UNSPECIFIED" || body.Metadata["pluginType"] != "GEMINI" ||
		body.Metadata["ideVersion"] != codeAssistClientVersion {
		t.Errorf("metadata = %v, ingin sidik jari Antigravity lengkap", body.Metadata)
	}
	logText := logBuf.String()
	for _, ingin := range []string{"respons loadCodeAssist diterima", "current_tier=free-tier", "paid_tier=g1-pro-tier", "project_tersedia=true"} {
		if !strings.Contains(logText, ingin) {
			t.Errorf("log tier tidak memuat %q: %s", ingin, logText)
		}
	}
	for _, terlarang := range []string{"proj-dari-server", "rahasia@example.invalid", "upgradeSubscriptionUri"} {
		if strings.Contains(logText, terlarang) {
			t.Errorf("log tier membocorkan %q: %s", terlarang, logText)
		}
	}
}

// TestLoadCodeAssistProjectDitolak membuktikan penolakan upstream menjadi
// error yang memuat pesan upstream (tanpa body mentah), dan token kosong
// ditolak sebelum panggilan apa pun.
func TestLoadCodeAssistProjectDitolak(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":{"code":403,"message":"akun tidak terdaftar di code assist","status":"PERMISSION_DENIED"}}`))
	}))
	defer srv.Close()

	klien := NewGoogleOAuthClient(nil).WithCodeAssistEndpoint(srv.URL)
	_, err := klien.LoadCodeAssistProject(context.Background(), "token-akses-uji", security.Secret(""))
	if err == nil {
		t.Fatal("status 403 harus menjadi error")
	}
	if !strings.Contains(err.Error(), "akun tidak terdaftar di code assist") {
		t.Errorf("error tidak memuat pesan upstream: %v", err)
	}

	if _, err := klien.LoadCodeAssistProject(context.Background(), "  ", security.Secret("")); err == nil {
		t.Error("access token kosong harus ditolak")
	}
}
