package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
	"github.com/NexGen-X/Route-X/internal/security"
)

// FASE 11 B-270 (PB-25) + B-272 (PB-27).
//
// B-270: updateEgressPool dulu menjalankan Update lalu SetProxyURL sebagai
// dua query terpisah dan menulis audit hanya sesudah keduanya berhasil —
// kegagalan langkah kedua meninggalkan mutasi pertama permanen tanpa audit.
// Kini satu transaksi: kind yang melanggar CHECK constraint DB dipakai
// sebagai pemicu kegagalan langkah kedua yang deterministik.
//
// B-272: testEgressPool mengubah state pool lewat RecordHealth tetapi tidak
// pernah menulis audit — satu-satunya handler uji di areanya yang begitu
// (testWebhookPing menulis). Probe ke proxy yang menolak koneksi dipakai:
// hasilnya unhealthy secara deterministik tanpa host eksternal.

func buatPoolUjiB270(t *testing.T, env *testEnv) *upstream.EgressPool {
	t.Helper()
	egressRepo, err := upstream.NewEgressRepo(env.pool, env.cipher)
	if err != nil {
		t.Fatalf("NewEgressRepo: %v", err)
	}
	aktif, bobot := true, 1
	ep, err := egressRepo.Create(context.Background(), upstream.CreateEgressParams{
		Name:     "pool-uji-b270",
		Kind:     "http",
		ProxyURL: security.Secret("http://pengguna:GANTI_rahasia@proxy.contoh.test:18080"),
		Enabled:  &aktif,
		Weight:   &bobot,
	})
	if err != nil {
		t.Fatalf("buat egress pool: %v", err)
	}
	return ep
}

func TestFase11B270UpdateEgressAtomik(t *testing.T) {
	env := setupTestEnv(t)
	client := env.newClient(t)
	client.login("superadmin@routex.internal", testPassword)
	ctx := context.Background()
	ep := buatPoolUjiB270(t, env)

	// Langkah pertama sah (nama baru), langkah kedua pasti gagal di DB
	// (kind di luar CHECK egress_pool_kind_valid). Tanpa transaksi, nama
	// baru akan ter-commit dan tidak ada audit yang menjelaskannya.
	res, body := client.do(http.MethodPut, "/api/admin/upstreams/egress-pools/"+ep.ID, map[string]any{
		"name":      "nama-tidak-boleh-tercommit",
		"kind":      "jenis-ngawur",
		"proxy_url": "http://pengguna:GANTI_baru@proxy-lain.contoh.test:18081",
	}, true)
	if res.StatusCode == http.StatusOK {
		t.Fatalf("PUT harus gagal, dapat 200: %s", body)
	}

	var nama string
	if err := env.pool.QueryRow(ctx, `select name from egress_pool where id = $1`, ep.ID).Scan(&nama); err != nil {
		t.Fatalf("baca nama pool: %v", err)
	}
	if nama != "pool-uji-b270" {
		t.Fatalf("nama pool = %q — mutasi langkah pertama ter-commit padahal langkah kedua gagal (B-270)", nama)
	}

	var nAudit int
	if err := env.pool.QueryRow(ctx,
		`select count(*) from audit_logs where action = 'update' and resource_type = 'egress_pool' and resource_id = $1`,
		ep.ID).Scan(&nAudit); err != nil {
		t.Fatalf("hitung audit: %v", err)
	}
	if nAudit != 0 {
		t.Fatalf("audit update = %d, mau 0 untuk mutasi yang digulung balik", nAudit)
	}

	// Kontrol positif: pembaruan sah tetap berhasil DAN teraudit.
	res, body = client.do(http.MethodPut, "/api/admin/upstreams/egress-pools/"+ep.ID, map[string]any{
		"name": "nama-baru-sah",
	}, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("PUT sah = %d: %s", res.StatusCode, body)
	}
	if err := env.pool.QueryRow(ctx, `select name from egress_pool where id = $1`, ep.ID).Scan(&nama); err != nil {
		t.Fatalf("baca nama pool sesudah PUT sah: %v", err)
	}
	if nama != "nama-baru-sah" {
		t.Fatalf("nama pool = %q, mau nama-baru-sah", nama)
	}
	if err := env.pool.QueryRow(ctx,
		`select count(*) from audit_logs where action = 'update' and resource_type = 'egress_pool' and resource_id = $1`,
		ep.ID).Scan(&nAudit); err != nil {
		t.Fatalf("hitung audit sesudah PUT sah: %v", err)
	}
	if nAudit != 1 {
		t.Fatalf("audit update = %d, mau 1 untuk mutasi yang berhasil", nAudit)
	}
}

func TestFase11B272TestEgressPoolTeraudit(t *testing.T) {
	env := setupTestEnv(t)
	client := env.newClient(t)
	client.login("superadmin@routex.internal", testPassword)
	ctx := context.Background()
	ep := buatPoolUjiB270(t, env)

	// proxy.contoh.test tidak terjangkau dari lingkungan uji: probe gagal
	// terhubung → RecordHealth("unhealthy") → respons 200 unhealthy.
	res, body := client.do(http.MethodPost, "/api/admin/upstreams/egress-pools/"+ep.ID+"/test", nil, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("POST test = %d: %s", res.StatusCode, body)
	}
	var dto EgressProbeResponseDTO
	if err := json.Unmarshal(body, &dto); err != nil {
		t.Fatalf("decode DTO probe: %v", err)
	}
	if dto.Status != "unhealthy" {
		t.Fatalf("status probe = %q, mau unhealthy (proxy tak terjangkau)", dto.Status)
	}

	var statusHealth string
	if err := env.pool.QueryRow(ctx, `select last_health_status from egress_pool where id = $1`, ep.ID).Scan(&statusHealth); err != nil {
		t.Fatalf("baca health pool: %v", err)
	}
	if statusHealth != "unhealthy" {
		t.Fatalf("last_health_status = %q, mau unhealthy (state memang berubah)", statusHealth)
	}

	var nAudit int
	if err := env.pool.QueryRow(ctx,
		`select count(*) from audit_logs where action = 'test' and resource_type = 'egress_pool' and resource_id = $1`,
		ep.ID).Scan(&nAudit); err != nil {
		t.Fatalf("hitung audit test: %v", err)
	}
	if nAudit != 1 {
		t.Fatalf("audit test egress = %d, mau 1 (B-272: probe pengubah state wajib teraudit)", nAudit)
	}
}
