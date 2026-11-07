// Test Fase 22 PR-B (B-519, B-520, B-521) — keluarga nilai-sama
// gelombang kedua, kali ini dengan sapuan mekanis atas ruang nama
// masing-masing permukaan:
//
//   - B-519 + B-520 (satu sapuan setPricing): SEMUA field yang
//     ditulis PricingRepo.Set disapu terhadap yang dibandingkan
//     gerbang no-op handler — Currency dan Source lolos di Fase 21
//     (tepi enumerasi keempat), dan gerbang yang sama menelan
//     konflik harga terjadwal masa depan (B-520). Jalur nyata:
//     POST /api/admin/upstreams/models/mappings/{id}/pricing.
//   - B-521: updateDomainConfig diputuskan sebagai keluarga
//     nilai-sama (handler tanpa efek live selain pre-flight DNS
//     baca-saja): POST identik kedua tanpa tulis ulang setting,
//     tanpa audit palsu, last_checked tersimpan tidak disegarkan.
//     Jalur nyata: POST /api/admin/system/domain.
package admin

import (
	"context"
	"net/http"
	"testing"

	upstreamrepo "github.com/NexGen-X/Route-X/internal/database/repo/upstream"
)

// TestB519B520SetPricingSapuanField membuktikan kedua residu
// setPricing lewat POST handler sungguhan: penyimpangan currency
// lalu source pada baris berlaku membuat POST identik MENULIS
// baris koreksi (bukan no-op), dan baris terbuka ber-effective_from
// masa depan membuat POST identik — maupun berbeda — menjawab 409
// karena penilaian konflik mendahului jalan pintas nilai-sama.
func TestB519B520SetPricingSapuanField(t *testing.T) {
	env := setupTestEnv(t)
	c := env.newClient(t)
	c.login("superadmin@routex.internal", testPassword)
	ctx := context.Background()
	providerID, modelID, _, _, _ := buatEntitasB469(t, env)

	modelRepo := upstreamrepo.NewModelRepo(env.pool)
	pm, err := modelRepo.AttachProvider(ctx, upstreamrepo.AttachProviderParams{
		ModelID: modelID, ProviderID: providerID,
	})
	if err != nil {
		t.Fatalf("AttachProvider: %v", err)
	}
	url := "/api/admin/upstreams/models/mappings/" + pm.ID + "/pricing"
	jumlahBaris := func() int {
		t.Helper()
		var n int
		if err := env.pool.QueryRow(ctx,
			`select count(*) from model_pricing where provider_model_id = $1`, pm.ID).Scan(&n); err != nil {
			t.Fatalf("hitung baris harga: %v", err)
		}
		return n
	}
	kolomTerbuka := func(kolom string) string {
		t.Helper()
		var v string
		if err := env.pool.QueryRow(ctx,
			`select `+kolom+` from model_pricing where provider_model_id = $1 and effective_to is null`, pm.ID).Scan(&v); err != nil {
			t.Fatalf("baca kolom %s baris terbuka: %v", kolom, err)
		}
		return v
	}
	posIdentik := func() (*http.Response, []byte) {
		t.Helper()
		return c.do(http.MethodPost, url,
			map[string]any{"input_per_1m_usd": "1.5", "output_per_1m_usd": "2.5"}, true)
	}

	dasar := hitungAudit(t, env, "set_pricing", "provider_model")
	res, body := posIdentik()
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("POST harga pertama: status = %d (body: %s)", res.StatusCode, body)
	}
	if n := jumlahBaris(); n != 1 {
		t.Fatalf("baris harga sesudah POST pertama = %d, mau 1", n)
	}

	// --- B-519, anggota sapuan Currency: baris berlaku menyimpang
	// dari bawaan yang akan ditulis Set → POST identik BUKAN no-op.
	if _, err := env.pool.Exec(ctx,
		`update model_pricing set currency = 'EUR'
		 where provider_model_id = $1 and effective_to is null`, pm.ID); err != nil {
		t.Fatalf("suntik penyimpangan currency: %v", err)
	}
	res, body = posIdentik()
	if res.StatusCode != http.StatusCreated {
		t.Errorf("POST identik dgn currency menyimpang: status = %d, mau 201 baris koreksi (body: %s)", res.StatusCode, body)
	}
	if n := jumlahBaris(); n != 2 {
		t.Errorf("baris harga sesudah koreksi currency = %d, mau 2", n)
	}
	if v := kolomTerbuka("currency"); v != "USD" {
		t.Errorf("currency baris terbuka = %q, mau %q (kembali ke bawaan Set)", v, "USD")
	}

	// --- B-519, anggota sapuan Source: pola yang sama.
	if _, err := env.pool.Exec(ctx,
		`update model_pricing set source = 'impor-massal'
		 where provider_model_id = $1 and effective_to is null`, pm.ID); err != nil {
		t.Fatalf("suntik penyimpangan source: %v", err)
	}
	res, body = posIdentik()
	if res.StatusCode != http.StatusCreated {
		t.Errorf("POST identik dgn source menyimpang: status = %d, mau 201 baris koreksi (body: %s)", res.StatusCode, body)
	}
	if n := jumlahBaris(); n != 3 {
		t.Errorf("baris harga sesudah koreksi source = %d, mau 3", n)
	}
	if v := kolomTerbuka("source"); v != "manual" {
		t.Errorf("source baris terbuka = %q, mau %q (kembali ke bawaan Set)", v, "manual")
	}
	if n := hitungAudit(t, env, "set_pricing", "provider_model"); n != dasar+3 {
		t.Fatalf("audit set_pricing = %d, mau %d (tiga penulisan nyata)", n, dasar+3)
	}

	// Kontrol: keadaan kini persis seperti yang akan ditulis Set →
	// no-op 200 diam pulih sepenuhnya.
	res, body = posIdentik()
	if res.StatusCode != http.StatusOK {
		t.Errorf("kontrol POST identik bersih: status = %d, mau 200 diam (body: %s)", res.StatusCode, body)
	}
	if n := jumlahBaris(); n != 3 {
		t.Errorf("baris harga sesudah kontrol = %d, mau 3", n)
	}

	// --- B-520: jadwalkan harga masa depan (baris berlaku ditutup
	// pada waktu masa depan; baris terbuka mulai dari waktu itu),
	// persis keadaan yang dihasilkan Set terjadwal.
	if _, err := env.pool.Exec(ctx, `
		update model_pricing
		set effective_to = now() + interval '2 hours'
		where provider_model_id = $1 and effective_to is null`, pm.ID); err != nil {
		t.Fatalf("tutup baris berlaku ke masa depan: %v", err)
	}
	if _, err := env.pool.Exec(ctx, `
		insert into model_pricing (provider_model_id, input_usd_per_mtok,
			output_usd_per_mtok, cached_input_usd_per_mtok,
			reasoning_usd_per_mtok, currency, effective_from, effective_to,
			source, created_by)
		values ($1, 3.0, 4.0, null, null, 'USD', now() + interval '2 hours', null, 'manual', null)`,
		pm.ID); err != nil {
		t.Fatalf("sisip baris harga masa depan: %v", err)
	}
	if n := jumlahBaris(); n != 4 {
		t.Fatalf("baris harga sesudah penjadwalan = %d, mau 4", n)
	}
	res, body = posIdentik()
	if res.StatusCode != http.StatusConflict {
		t.Errorf("POST identik dgn harga masa depan terjadwal: status = %d, mau 409 — konflik tidak boleh ditelan jalan pintas (body: %s)", res.StatusCode, body)
	}
	res, body = c.do(http.MethodPost, url,
		map[string]any{"input_per_1m_usd": "9.9", "output_per_1m_usd": "2.5"}, true)
	if res.StatusCode != http.StatusConflict {
		t.Errorf("POST berbeda dgn harga masa depan terjadwal: status = %d, mau 409 (body: %s)", res.StatusCode, body)
	}
	if n := jumlahBaris(); n != 4 {
		t.Errorf("baris harga sesudah dua POST konflik = %d, mau 4 (tidak ada yang tertulis)", n)
	}
	if n := hitungAudit(t, env, "set_pricing", "provider_model"); n != dasar+3 {
		t.Errorf("audit set_pricing = %d, mau %d — konflik tidak menulis audit", n, dasar+3)
	}

	// Pemulihan: jadwal masa depan dibatalkan (barisnya dihapus,
	// baris berlaku dibuka lagi) → no-op identik kembali 200.
	if _, err := env.pool.Exec(ctx,
		`delete from model_pricing where provider_model_id = $1 and effective_from > now()`, pm.ID); err != nil {
		t.Fatalf("hapus jadwal masa depan: %v", err)
	}
	if _, err := env.pool.Exec(ctx, `
		update model_pricing set effective_to = null
		where id = (select id from model_pricing
			where provider_model_id = $1 and effective_from <= now()
			order by effective_from desc limit 1)`, pm.ID); err != nil {
		t.Fatalf("buka lagi baris berlaku: %v", err)
	}
	res, body = posIdentik()
	if res.StatusCode != http.StatusOK {
		t.Errorf("POST identik sesudah jadwal dibatalkan: status = %d, mau 200 diam (body: %s)", res.StatusCode, body)
	}
}

// TestB521DomainConfigNilaiSama membuktikan keputusan keluarga
// nilai-sama untuk konfigurasi domain lewat POST handler
// sungguhan: POST pertama menulis + mengaudit; POST identik kedua
// menjawab 200 dengan nilai setting BYTE-PER-BYTE tidak berubah
// (last_checked tersimpan tidak disegarkan) dan tanpa audit baru;
// POST yang berubah menulis dan mengaudit lagi seperti biasa.
func TestB521DomainConfigNilaiSama(t *testing.T) {
	env := setupTestEnv(t)
	c := env.newClient(t)
	c.login("superadmin@routex.internal", testPassword)
	ctx := context.Background()

	bersihkan := func() {
		_, _ = env.pool.Exec(ctx, `delete from settings where key in ($1, $2)`,
			SettingKeyDomainConfig, SettingKeyPublicURL)
	}
	bersihkan()
	defer bersihkan()
	bacaSetting := func(kunci string) string {
		t.Helper()
		var v string
		if err := env.pool.QueryRow(ctx,
			`select value from settings where key = $1`, kunci).Scan(&v); err != nil {
			t.Fatalf("baca setting %s: %v", kunci, err)
		}
		return v
	}

	dasar := hitungAudit(t, env, "configure", "system_domain")
	res, body := c.do(http.MethodPost, "/api/admin/system/domain",
		map[string]any{"domain": "example.com", "mode": "cloudflare"}, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("POST domain pertama: status = %d (body: %s)", res.StatusCode, body)
	}
	if n := hitungAudit(t, env, "configure", "system_domain"); n != dasar+1 {
		t.Fatalf("audit configure domain = %d, mau %d", n, dasar+1)
	}
	konfigAwal := bacaSetting(SettingKeyDomainConfig)
	urlAwal := bacaSetting(SettingKeyPublicURL)

	res, body = c.do(http.MethodPost, "/api/admin/system/domain",
		map[string]any{"domain": "example.com", "mode": "cloudflare"}, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("POST domain identik: status = %d (body: %s)", res.StatusCode, body)
	}
	if n := hitungAudit(t, env, "configure", "system_domain"); n != dasar+1 {
		t.Errorf("audit configure domain sesudah POST identik = %d, mau %d — audit palsu no-op", n, dasar+1)
	}
	if v := bacaSetting(SettingKeyDomainConfig); v != konfigAwal {
		t.Errorf("nilai %s berubah pada POST identik — setting ditulis ulang (last_checked ikut disegarkan):\nawal: %s\nbaru: %s",
			SettingKeyDomainConfig, konfigAwal, v)
	}
	if v := bacaSetting(SettingKeyPublicURL); v != urlAwal {
		t.Errorf("nilai %s berubah pada POST identik — setting ditulis ulang", SettingKeyPublicURL)
	}

	// Kontrol perubahan nyata: domain berbeda tetap menulis +
	// mengaudit, dan pengulangan atas keadaan BARU itu kembali
	// diam.
	res, body = c.do(http.MethodPost, "/api/admin/system/domain",
		map[string]any{"domain": "example.org", "mode": "cloudflare"}, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("POST domain berubah: status = %d (body: %s)", res.StatusCode, body)
	}
	if n := hitungAudit(t, env, "configure", "system_domain"); n != dasar+2 {
		t.Errorf("audit configure domain sesudah perubahan = %d, mau %d", n, dasar+2)
	}
	if v := bacaSetting(SettingKeyDomainConfig); v == konfigAwal {
		t.Errorf("nilai %s tidak berubah sesudah POST domain baru", SettingKeyDomainConfig)
	}
	res, body = c.do(http.MethodPost, "/api/admin/system/domain",
		map[string]any{"domain": "example.org", "mode": "cloudflare"}, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("POST domain baru identik: status = %d (body: %s)", res.StatusCode, body)
	}
	if n := hitungAudit(t, env, "configure", "system_domain"); n != dasar+2 {
		t.Errorf("audit configure domain akhir = %d, mau %d", n, dasar+2)
	}
}
