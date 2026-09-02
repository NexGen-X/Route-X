package openai

import (
	"context"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/NexGen-X/Route-X/internal/providers"
)

func TestModels(t *testing.T) {
	const body = `{
  "object": "list",
  "data": [
    {"id": "gpt-4o-mini", "object": "model", "created": 1712345678, "owned_by": "openai"},
    {"id": "text-embedding-3-small", "object": "model", "created": 1700000000, "owned_by": "system"},
    {"object": "model", "created": 1, "owned_by": "tanpa-id"}
  ]
}`
	u := newUpstream(t, jsonHandler(200, body))
	p := testProvider(t, u.url(), nil)

	models, err := p.Models(context.Background())
	if err != nil {
		t.Fatalf("Models: %v", err)
	}

	got := u.last()
	if got.method != http.MethodGet || got.path != "/v1/models" {
		t.Errorf("permintaan = %s %s", got.method, got.path)
	}
	if len(got.body) != 0 {
		t.Errorf("permintaan GET membawa body: %s", got.body)
	}
	// Permintaan tanpa body tidak boleh memasang Content-Type.
	if ct := got.header.Get("Content-Type"); ct != "" {
		t.Errorf("Content-Type = %q, mau kosong", ct)
	}

	// Entri tanpa id dibuang: tidak bisa dipakai untuk apa pun.
	if len(models) != 2 {
		t.Fatalf("jumlah model = %d, mau 2", len(models))
	}
	if models[0].ID != "gpt-4o-mini" || models[0].OwnedBy != "openai" || models[0].Created != 1712345678 {
		t.Errorf("model 0 = %+v", models[0])
	}
	if models[1].ID != "text-embedding-3-small" {
		t.Errorf("model 1 = %+v", models[1])
	}
}

// Daftar kosong bukan kegagalan: provider yang belum mengaktifkan model memang menjawab
// begitu, dan pemanggil membedakannya lewat error nil.
func TestModelsEmptyListIsNotAnError(t *testing.T) {
	u := newUpstream(t, jsonHandler(200, `{"object":"list","data":[]}`))
	p := testProvider(t, u.url(), nil)

	models, err := p.Models(context.Background())
	if err != nil {
		t.Fatalf("Models: %v", err)
	}
	if len(models) != 0 {
		t.Errorf("jumlah model = %d, mau 0", len(models))
	}
}

func TestModelsRejectsNonJSON(t *testing.T) {
	u := newUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte("halaman login proxy perusahaan"))
	})
	p := testProvider(t, u.url(), nil)

	_, err := p.Models(context.Background())
	e := providers.AsError(err)
	if e == nil {
		t.Fatalf("error bukan *providers.Error: %v", err)
	}
	if e.Kind != providers.ErrKindServer {
		t.Errorf("Kind = %s, mau %s", e.Kind, providers.ErrKindServer)
	}
	if strings.Contains(e.Message, "login") {
		t.Errorf("pesan meneruskan body upstream: %q", e.Message)
	}
}

// Health check harus murah dan tidak menagih: daftar model, bukan completion.
func TestHealthCheckUsesModelsEndpoint(t *testing.T) {
	u := newUpstream(t, jsonHandler(200, `{"object":"list","data":[{"id":"gpt-4o-mini"}]}`))
	p := testProvider(t, u.url(), nil)

	result := p.HealthCheck(context.Background())
	if !result.Healthy {
		t.Fatalf("HealthCheck = %+v, mau sehat", result)
	}
	if result.StatusCode != 200 {
		t.Errorf("StatusCode = %d", result.StatusCode)
	}
	if result.Latency <= 0 {
		t.Errorf("Latency = %v, mau lebih dari 0", result.Latency)
	}
	if result.ErrorKind != "" || result.ErrorMessage != "" {
		t.Errorf("hasil sehat memuat galat: %+v", result)
	}

	got := u.last()
	if got.method != http.MethodGet || got.path != "/v1/models" {
		t.Errorf("health check memanggil %s %s, mau GET /v1/models", got.method, got.path)
	}
}

// Kegagalan ADALAH hasilnya: HealthCheck tidak pernah mengembalikan error, dan hasilnya
// tetap terisi lengkap supaya bisa diagregasi.
func TestHealthCheckUnhealthyResults(t *testing.T) {
	tests := []struct {
		name     string
		status   int
		body     string
		wantKind providers.ErrorKind
	}{
		{"kredensial ditolak", 401, `{"error":{"message":"kunci salah","code":"invalid_api_key"}}`, providers.ErrKindAuth},
		{"tidak berhak", 403, `{"error":{"message":"tidak berhak"}}`, providers.ErrKindPermission},
		{"provider penuh", 503, `{"error":{"message":"penuh"}}`, providers.ErrKindOverloaded},
		{"galat internal", 500, `{"error":{"message":"galat"}}`, providers.ErrKindServer},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			u := newUpstream(t, jsonHandler(tc.status, tc.body))
			p := testProvider(t, u.url(), nil)

			result := p.HealthCheck(context.Background())
			if result.Healthy {
				t.Error("Healthy = true padahal status gagal")
			}
			if result.StatusCode != tc.status {
				t.Errorf("StatusCode = %d, mau %d", result.StatusCode, tc.status)
			}
			if result.ErrorKind != tc.wantKind {
				t.Errorf("ErrorKind = %s, mau %s", result.ErrorKind, tc.wantKind)
			}
			if result.ErrorMessage == "" {
				t.Error("ErrorMessage kosong")
			}
			if strings.Contains(result.ErrorMessage, testKey) {
				t.Errorf("ErrorMessage memuat kredensial: %q", result.ErrorMessage)
			}
			if result.Latency <= 0 {
				t.Errorf("Latency = %v", result.Latency)
			}
		})
	}
}

// Upstream yang tidak bisa dihubungi: StatusCode nol karena tidak pernah ada respons, dan
// alamatnya tidak boleh muncul di pesan.
func TestHealthCheckTransportFailure(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	if err := ln.Close(); err != nil {
		t.Fatalf("close listener: %v", err)
	}
	p := testProvider(t, "http://"+addr, nil)

	result := p.HealthCheck(context.Background())
	if result.Healthy {
		t.Error("Healthy = true padahal upstream tidak bisa dihubungi")
	}
	if result.StatusCode != 0 {
		t.Errorf("StatusCode = %d, mau 0", result.StatusCode)
	}
	if result.ErrorKind != providers.ErrKindNetwork {
		t.Errorf("ErrorKind = %s, mau %s", result.ErrorKind, providers.ErrKindNetwork)
	}
	assertNoAddress(t, result.ErrorMessage, "http://"+addr)
}

// Probe berjalan terjadwal untuk setiap provider, jadi batas waktunya tidak boleh mengikuti
// timeout request yang panjang.
func TestHealthCheckHonoursShorterTimeout(t *testing.T) {
	u := newUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	})
	p := testProvider(t, u.url(), func(c *Config) { c.Timeout = 100 * time.Millisecond })

	start := time.Now()
	result := p.HealthCheck(context.Background())
	if result.Healthy {
		t.Error("Healthy = true padahal upstream tidak menjawab")
	}
	if result.ErrorKind != providers.ErrKindTimeout {
		t.Errorf("ErrorKind = %s, mau %s", result.ErrorKind, providers.ErrKindTimeout)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("health check butuh %v, seharusnya berhenti pada batas 100ms", elapsed)
	}
}

// Timeout bawaan health check dipakai bila timeout request lebih panjang.
func TestHealthCheckTimeoutCappedByDefault(t *testing.T) {
	p := testProvider(t, "https://api.openai.com/v1", func(c *Config) { c.Timeout = 5 * time.Minute })
	if p.healthTimeout != healthCheckTimeout {
		t.Errorf("healthTimeout = %v, mau %v", p.healthTimeout, healthCheckTimeout)
	}

	q := testProvider(t, "https://api.openai.com/v1", func(c *Config) { c.Timeout = 2 * time.Second })
	if q.healthTimeout != 2*time.Second {
		t.Errorf("healthTimeout = %v, mau 2s", q.healthTimeout)
	}
}
