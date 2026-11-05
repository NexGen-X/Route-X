package oauth

import (
	"context"
	"encoding/json"
	"io"
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

// TestLoadCodeAssistProject membuktikan panggilan keluarnya: endpoint dari
// seam pengujian, header Authorization dari access token, body metadata
// Code Assist, dan penguraian kedua bentuk project.
func TestLoadCodeAssistProject(t *testing.T) {
	var reqBody []byte
	var reqAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqBody, _ = io.ReadAll(r.Body)
		reqAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"cloudaicompanionProject":{"id":"proj-dari-server"}}`))
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
	var body struct {
		Metadata map[string]string `json:"metadata"`
	}
	if err := json.Unmarshal(reqBody, &body); err != nil {
		t.Fatalf("body request bukan JSON sah: %v", err)
	}
	if body.Metadata["pluginType"] != "GEMINI" {
		t.Errorf("metadata pluginType = %q, ingin GEMINI", body.Metadata["pluginType"])
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
