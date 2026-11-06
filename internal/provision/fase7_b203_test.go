package provision

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// Test B-203 (klien): nama proyek bawaan saat ProjectName kosong adalah
// nama deterministik NamaProyekRelayBawaan — bukan lagi sufiks acak
// berbasis waktu yang membuat setiap percobaan mencetak proyek yatim
// baru dan retry tidak pernah menabrak proyek yang sama.
func TestFase7B203NamaProyekBawaanDeterministik(t *testing.T) {
	var namaDiminta string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/projects" && r.Method == http.MethodPost {
			var req struct {
				Name string `json:"name"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			namaDiminta = req.Name
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(denoProjectResp{ID: "proj-b203", Name: req.Name})
			return
		}
		if r.URL.Path == "/projects/proj-b203/deployments" && r.Method == http.MethodPost {
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(denoDeploymentResp{
				ID:      "dep-b203",
				Domains: []string{"routex-relay.deno.dev"},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client := NewDenoClient(server.URL, 5*time.Second)
	res, err := client.ProvisionRelay(context.Background(), DenoProvisionParams{
		AccessToken: "GANTI_token_deno_uji",
		ProjectName: "",
	})
	if err != nil {
		t.Fatalf("ProvisionRelay: %v", err)
	}
	// Literal (bukan konstanta NamaProyekRelayBawaan) supaya test ini juga
	// terkompilasi pada kode pra-fix dan gagal di sana pada nilainya.
	if namaDiminta != "routex-relay" {
		t.Errorf("nama proyek yang diminta = %q, mau deterministik %q", namaDiminta, "routex-relay")
	}
	if res.ProjectName != "routex-relay" {
		t.Errorf("ProjectName hasil = %q, mau %q", res.ProjectName, "routex-relay")
	}
	if res.RelayURL != "https://routex-relay.deno.dev" {
		t.Errorf("RelayURL = %q, mau https://routex-relay.deno.dev", res.RelayURL)
	}
}
