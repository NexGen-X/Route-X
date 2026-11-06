package admin

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
	"github.com/NexGen-X/Route-X/internal/gateway"
	"github.com/NexGen-X/Route-X/internal/security"
)

// TestSyncModelsGagalUpstreamTidakDitutupiKatalog membuktikan B-204:
// kegagalan discovery (upstream menolak, tak terjangkau, atau menjawab
// tanpa model) adalah kegagalan sync yang dilaporkan apa adanya.
// Versi lama menutupi ketiga keadaan itu dengan katalog terkurasi —
// sync menjawab 200 "Berhasil" dan memasang model karangan (termasuk
// model pensiun) sebagai model aktif provider. Nama provider pada
// kasus-kasus gagal di bawah sengaja memuat "openai" supaya katalog
// lama PASTI cocok: test ini gagal pada kode pra-perbaikan.
func TestSyncModelsGagalUpstreamTidakDitutupiKatalog(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()
	c := env.newClient(t)
	c.login("superadmin@routex.internal", testPassword)

	// Factory ber-kebijakan SSRF loopback agar discovery ke 127.0.0.1
	// sah (pola yang sama dengan test B-157).
	prefiks, err := security.ParsePrivateAddrs([]string{"127.0.0.1", "::1"})
	if err != nil {
		t.Fatalf("ParsePrivateAddrs: %v", err)
	}
	env.handlers.factory = gateway.NewFactory(nil, nil,
		security.SSRFPolicy{AllowHTTP: true, AllowedPrivateAddrs: prefiks},
		slog.New(slog.DiscardHandler))

	provRepo := upstream.NewProviderRepo(env.pool)
	modelRepo := upstream.NewModelRepo(env.pool)

	buatProvider := func(nama, kind, base string) *upstream.Provider {
		prov, err := provRepo.Create(ctx, upstream.CreateProviderParams{
			Name: nama, DisplayName: nama, Kind: kind, BaseURL: base,
		})
		if err != nil {
			t.Fatalf("buat provider %s: %v", nama, err)
		}
		return prov
	}
	jumlahPemetaan := func(providerID string) int {
		peta, err := modelRepo.ListProviderModels(ctx, upstream.ProviderModelFilter{ProviderID: providerID})
		if err != nil {
			t.Fatalf("ListProviderModels: %v", err)
		}
		return len(peta)
	}
	sync := func(providerID string) (int, []byte) {
		res, body := c.do(http.MethodPost, "/api/admin/upstreams/providers/"+providerID+"/sync-models", map[string]any{}, true)
		return res.StatusCode, body
	}

	// 1) Upstream menolak (401): sync wajib 502 upstream_error dan TIDAK
	// memasang satu pun model.
	srvTolak := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"kunci tidak sah"}`, http.StatusUnauthorized)
	}))
	defer srvTolak.Close()
	provTolak := buatProvider("openai-b204-ditolak", "openai", srvTolak.URL)
	st, body := sync(provTolak.ID)
	if st != http.StatusBadGateway || !strings.Contains(string(body), "upstream_error") {
		t.Errorf("sync upstream 401: status %d, ingin 502 upstream_error (body %s)", st, body)
	}
	if n := jumlahPemetaan(provTolak.ID); n != 0 {
		t.Errorf("sync upstream 401 memasang %d pemetaan model, ingin 0", n)
	}

	// 2) Upstream menjawab 200 tanpa model: sync wajib 400
	// no_models_found, bukan sukses berkatalog.
	srvKosong := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer srvKosong.Close()
	provKosong := buatProvider("openai-b204-kosong", "openai", srvKosong.URL)
	st, body = sync(provKosong.ID)
	if st != http.StatusBadRequest || !strings.Contains(string(body), "no_models_found") {
		t.Errorf("sync upstream kosong: status %d, ingin 400 no_models_found (body %s)", st, body)
	}
	if n := jumlahPemetaan(provKosong.ID); n != 0 {
		t.Errorf("sync upstream kosong memasang %d pemetaan model, ingin 0", n)
	}

	// 3) Upstream tak terjangkau: sync wajib 502 upstream_unreachable.
	srvMati := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	urlMati := srvMati.URL
	srvMati.Close()
	provMati := buatProvider("openai-b204-mati", "openai", urlMati)
	st, body = sync(provMati.ID)
	if st != http.StatusBadGateway || !strings.Contains(string(body), "upstream_unreachable") {
		t.Errorf("sync upstream mati: status %d, ingin 502 upstream_unreachable (body %s)", st, body)
	}
	if n := jumlahPemetaan(provMati.ID); n != 0 {
		t.Errorf("sync upstream mati memasang %d pemetaan model, ingin 0", n)
	}

	// 4) Kontrol sukses: upstream menjawab daftar nyata — sync tetap
	// 200 dan memasang persis model itu (jalur jujur tidak rusak).
	srvOK := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"model-b204-nyata"}]}`))
	}))
	defer srvOK.Close()
	provOK := buatProvider("b204-sukses", "openai_compatible", srvOK.URL)
	st, body = sync(provOK.ID)
	if st != http.StatusOK {
		t.Fatalf("sync sukses: status %d, ingin 200 (body %s)", st, body)
	}
	var dto SyncModelsResponseDTO
	if err := json.Unmarshal(body, &dto); err != nil {
		t.Fatalf("urai respons sync sukses: %v", err)
	}
	if dto.Count != 1 || len(dto.Models) != 1 || dto.Models[0] != "model-b204-nyata" {
		t.Errorf("dto sync sukses = %+v, ingin tepat 1 model-b204-nyata", dto)
	}
	if n := jumlahPemetaan(provOK.ID); n != 1 {
		t.Errorf("sync sukses memasang %d pemetaan, ingin 1", n)
	}
}
