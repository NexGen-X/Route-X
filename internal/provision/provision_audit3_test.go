package provision

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// TestCloudflareKode1000BukanAlreadyExists membuktikan kode 1000 generik
// (mis. token tidak sah) TIDAK lagi ditelan sebagai "gateway sudah ada"
// (audit3 V-EO-07): pesannya tidak memuat frasa already exists.
func TestCloudflareKode1000BukanAlreadyExists(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"errors":  []map[string]any{{"code": 1000, "message": "Invalid API token"}},
		})
	}))
	defer server.Close()

	p := NewCloudflareClient(server.URL, 5*time.Second)
	res, err := p.ProvisionAIGateway(context.Background(), CloudflareProvisionParams{APIToken: "token", AccountID: "acct", GatewayID: "gw"})
	if err == nil {
		t.Fatalf("kode 1000 generik diterima sebagai sukses: %+v", res)
	}
	if !strings.Contains(err.Error(), "1000") && !strings.Contains(err.Error(), "Invalid API token") {
		t.Errorf("error tidak memuat kode/pesan asli: %v", err)
	}
}

// TestCloudflareAlreadyExistsTapiGatewayTidakAda membuktikan klaim
// "already exists" diverifikasi: bila GET gateway menjawab 404, hasilnya
// error — bukan AlreadyExist palsu.
func TestCloudflareAlreadyExistsTapiGatewayTidakAda(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]any{"success": false})
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"errors":  []map[string]any{{"code": 1000, "message": "Gateway with id already exists"}},
		})
	}))
	defer server.Close()

	p := NewCloudflareClient(server.URL, 5*time.Second)
	_, err := p.ProvisionAIGateway(context.Background(), CloudflareProvisionParams{APIToken: "token", AccountID: "acct", GatewayID: "gw"})
	if err == nil {
		t.Fatal("klaim already-exists tanpa gateway nyata diterima")
	}
}

// TestDenoTokenRelayDanKompensasiHapus membuktikan (audit3 V-EO-03 + V-EO-06):
//  1. provisioning menanam token acak sebagai env ROUTEX_RELAY_TOKEN pada
//     deployment dan mengembalikannya pada hasil;
//  2. kegagalan deploy SETELAH project baru dibuat menghapus project itu
//     (kompensasi), sedangkan kegagalan pada project yang sudah ada
//     sebelumnya TIDAK menghapus apa pun.
func TestDenoTokenRelayDanKompensasiHapus(t *testing.T) {
	var envTertangkap atomic.Value // map[string]string dari payload deploy
	var hapusTercatat atomic.Int32
	var proyekAda atomic.Bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/deployments"):
			// Jalur /projects/{id}/deployments diperiksa lebih dulu karena
			// juga mengandung "/projects".
			var payload struct {
				EnvVars map[string]string `json:"envVars"`
			}
			_ = json.NewDecoder(r.Body).Decode(&payload)
			envTertangkap.Store(payload.EnvVars)
			// Gagal sesudah project dibuat: memicu kompensasi.
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"message":"deploy gagal total"}`))
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/projects"):
			if proyekAda.Load() {
				w.WriteHeader(http.StatusConflict)
				_, _ = w.Write([]byte(`{"message":"sudah ada"}`))
				return
			}
			_, _ = w.Write([]byte(`{"id":"proj-baru"}`))
		case r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "/projects/"):
			hapusTercatat.Add(1)
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	p := NewDenoClient(server.URL, 5*time.Second)

	// Kasus 1: project baru + deploy gagal → token tertanam di percobaan
	// payload tidak tercapai (deploy gagal sebelum payload dibaca handler
	// kita di atas tetap membaca env), kompensasi DELETE terpanggil.
	_, err := p.ProvisionRelay(context.Background(), DenoProvisionParams{AccessToken: "akses", ProjectName: "relay-uji"})
	if err == nil || !strings.Contains(err.Error(), "dihapus") {
		t.Fatalf("deploy gagal pada project baru: err = %v, ingin menyebut kompensasi hapus", err)
	}
	if hapusTercatat.Load() != 1 {
		t.Fatalf("DELETE kompensasi terpanggil %d kali, ingin 1", hapusTercatat.Load())
	}

	// Kasus 2: project sudah ada (409) + deploy gagal → TIDAK ada DELETE.
	proyekAda.Store(true)
	hapusTercatat.Store(0)
	_, err = p.ProvisionRelay(context.Background(), DenoProvisionParams{AccessToken: "akses", ProjectName: "relay-uji"})
	if err == nil {
		t.Fatal("deploy gagal pada project lama tidak menghasilkan error")
	}
	if hapusTercatat.Load() != 0 {
		t.Fatalf("project lama ikut terhapus (%d DELETE), ingin 0", hapusTercatat.Load())
	}
}

// TestDenoTokenRelaySuksesMemakaiEnv membuktikan pada jalur sukses token yang
// tertanam di env deployment sama dengan token pada hasil provisioning.
func TestDenoTokenRelaySuksesMemakaiEnv(t *testing.T) {
	var envTertangkap atomic.Value

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/deployments"):
			var payload struct {
				EnvVars map[string]string `json:"envVars"`
			}
			_ = json.NewDecoder(r.Body).Decode(&payload)
			envTertangkap.Store(payload.EnvVars)
			_, _ = w.Write([]byte(`{"id":"dep-1","status":"success"}`))
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/projects"):
			_, _ = w.Write([]byte(`{"id":"proj-1"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	p := NewDenoClient(server.URL, 5*time.Second)
	res, err := p.ProvisionRelay(context.Background(), DenoProvisionParams{AccessToken: "akses", ProjectName: "relay-uji"})
	if err != nil {
		t.Fatalf("ProvisionRelay: %v", err)
	}
	env, _ := envTertangkap.Load().(map[string]string)
	if res.RelayToken == "" {
		t.Fatal("hasil provisioning tidak membawa RelayToken")
	}
	if env["ROUTEX_RELAY_TOKEN"] != res.RelayToken {
		t.Errorf("env ROUTEX_RELAY_TOKEN %q != hasil %q", env["ROUTEX_RELAY_TOKEN"], res.RelayToken)
	}
	if len(res.RelayToken) < 32 {
		t.Errorf("token relay terlalu pendek: %d char", len(res.RelayToken))
	}
}
