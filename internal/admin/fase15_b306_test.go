package admin

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

// TestB306PutEgressHanyaProxyURL adalah pengunci B-306: PUT egress pool
// yang HANYA membawa proxy_url (rotasi kredensial murni, pola klien API
// langsung) sebelumnya selalu 500 — Update repo menolak params kosong
// dengan ErrNoChanges yang tidak dipetakan mapRepoError, dan rotasinya
// tidak pernah terjadi. UI tidak pernah terlihat karena selalu
// mengirim body penuh. Kini: proxy_url saja → 200 + URL terotasi +
// audit tertulis; body kosong {} → 400 no_changes, bukan 500.
func TestB306PutEgressHanyaProxyURL(t *testing.T) {
	env := setupTestEnv(t)
	client := env.newClient(t)
	client.login("superadmin@routex.internal", testPassword)
	ctx := context.Background()
	ep := buatPoolUjiB270(t, env)

	var hintAwal string
	if err := env.pool.QueryRow(ctx, `select masked_hint from egress_pool where id = $1`, ep.ID).Scan(&hintAwal); err != nil {
		t.Fatalf("baca hint awal: %v", err)
	}

	res, body := client.do(http.MethodPut, "/api/admin/upstreams/egress-pools/"+ep.ID, map[string]any{
		"proxy_url": "http://pengguna:GANTI_baru@proxy-lain.contoh.test:18081",
	}, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("PUT hanya proxy_url: status = %d, mau 200 (body: %s)", res.StatusCode, body)
	}

	var hintBaru string
	if err := env.pool.QueryRow(ctx, `select masked_hint from egress_pool where id = $1`, ep.ID).Scan(&hintBaru); err != nil {
		t.Fatalf("baca hint baru: %v", err)
	}
	if hintBaru == hintAwal || !strings.Contains(hintBaru, "proxy-lain.contoh.test") {
		t.Errorf("masked_hint = %q (awal %q) — rotasi proxy_url tidak terjadi", hintBaru, hintAwal)
	}

	var nAudit int
	if err := env.pool.QueryRow(ctx,
		`select count(*) from audit_logs where action = 'update' and resource_type = 'egress_pool' and resource_id = $1`,
		ep.ID).Scan(&nAudit); err != nil {
		t.Fatalf("hitung audit: %v", err)
	}
	if nAudit != 1 {
		t.Errorf("audit update = %d, mau 1 untuk rotasi yang berhasil", nAudit)
	}

	// Body kosong: tidak ada perubahan yang diminta → 400, bukan 500.
	res, body = client.do(http.MethodPut, "/api/admin/upstreams/egress-pools/"+ep.ID, map[string]any{}, true)
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("PUT body kosong: status = %d, mau 400 (body: %s)", res.StatusCode, body)
	}
	if !strings.Contains(string(body), "no_changes") {
		t.Errorf("PUT body kosong: body = %s, mau memuat kode no_changes", body)
	}
}
