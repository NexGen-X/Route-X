package admin

import (
	"context"
	"net/http"
	"testing"
)

// TestB300UpdateEgressAtomikLangkahKedua adalah penguat DISKRIMINATIF
// untuk test B-270 (B-300): pemicu test B-270 lama (kind yang melanggar
// CHECK) gagal di dalam pernyataan Update ITU SENDIRI — langkah pertama
// — sehingga test itu lulus juga pada kode pra-perbaikan yang belum
// bertransaksi. Di sini langkah pertama DIBUAT LOLOS (nama sah) dan
// langkah kedua (SetProxyURL) dibuat gagal secara deterministik lewat
// trigger DB pada kolom proxy_url_encrypted di schema uji. Pada
// perilaku dua-langkah-tanpa-transaksi, nama baru ter-commit dan test
// ini GAGAL pada asersi nama; dengan transaksi B-270, seluruhnya
// tergulung balik: nama & hint tidak berubah, audit 0.
func TestB300UpdateEgressAtomikLangkahKedua(t *testing.T) {
	env := setupTestEnv(t)
	client := env.newClient(t)
	client.login("superadmin@routex.internal", testPassword)
	ctx := context.Background()
	ep := buatPoolUjiB270(t, env)

	if _, err := env.pool.Exec(ctx, `
		create or replace function larang_ubah_proxy_b300() returns trigger as $$
		begin
			raise exception 'kegagalan langkah kedua yang disimulasikan (B-300)';
		end $$ language plpgsql`); err != nil {
		t.Fatalf("buat fungsi pemicu: %v", err)
	}
	if _, err := env.pool.Exec(ctx, `
		create trigger trg_b300_gagal_proxy before update of proxy_url_encrypted on egress_pool
		for each row execute function larang_ubah_proxy_b300()`); err != nil {
		t.Fatalf("buat trigger pemicu: %v", err)
	}

	var hintAwal string
	if err := env.pool.QueryRow(ctx, `select masked_hint from egress_pool where id = $1`, ep.ID).Scan(&hintAwal); err != nil {
		t.Fatalf("baca hint awal: %v", err)
	}

	res, body := client.do(http.MethodPut, "/api/admin/upstreams/egress-pools/"+ep.ID, map[string]any{
		"name":      "nama-langkah-pertama-lolos",
		"proxy_url": "http://pengguna:GANTI_baru@proxy-lain.contoh.test:18081",
	}, true)
	if res.StatusCode == http.StatusOK {
		t.Fatalf("PUT harus gagal di langkah kedua, dapat 200: %s", body)
	}

	var nama, hint string
	if err := env.pool.QueryRow(ctx, `select name, masked_hint from egress_pool where id = $1`, ep.ID).Scan(&nama, &hint); err != nil {
		t.Fatalf("baca pool: %v", err)
	}
	if nama != "pool-uji-b270" {
		t.Fatalf("nama pool = %q — mutasi langkah pertama ter-commit padahal langkah kedua gagal (perilaku pra-B-270)", nama)
	}
	if hint != hintAwal {
		t.Errorf("masked_hint berubah = %q, mau tetap %q", hint, hintAwal)
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
}
