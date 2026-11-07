// Test Fase 23 PR-C (B-532, B-539) — residu gerbang nilai-sama
// konfigurasi domain:
//
//   - B-532: gerbang B-521 membandingkan SEMUA kunci yang ditulis
//     transaksi updateDomainConfig — bukan hanya
//     system:domain:config, tetapi juga system:public_url.
//     public_url yang menyimpang atau hilang membuat POST identik
//     MENULIS (menyembuhkan), bukan no-op. Jalur nyata:
//     POST /api/admin/system/domain.
//   - B-539: DTO jalur no-op memuat field message yang sama
//     persis dengan jalur tulis.
package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

// TestB532B539DomainGerbangDuaKunci membuktikan lewat POST
// handler sungguhan: (1) POST identik atas keadaan sehat adalah
// no-op yang DTO-nya memuat message jalur tulis (B-539);
// (2) public_url disuntik menyimpang → POST identik menulis +
// mengaudit dan nilai pulih (B-532); (3) baris public_url
// dihapus → pola yang sama; (4) sesudah sembuh, no-op pulih.
func TestB532B539DomainGerbangDuaKunci(t *testing.T) {
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
	bacaSetting := func(kunci string) (string, bool) {
		t.Helper()
		var v string
		err := env.pool.QueryRow(ctx,
			`select value from settings where key = $1`, kunci).Scan(&v)
		if err != nil {
			return "", false
		}
		return v, true
	}
	pesanRespons := func(body []byte) string {
		t.Helper()
		var dto struct {
			Message string `json:"message"`
		}
		if err := json.Unmarshal(body, &dto); err != nil {
			t.Fatalf("respons bukan JSON sah: %v (body: %s)", err, body)
		}
		return dto.Message
	}
	posIdentik := func() []byte {
		t.Helper()
		res, body := c.do(http.MethodPost, "/api/admin/system/domain",
			map[string]any{"domain": "example.com", "mode": "cloudflare"}, true)
		if res.StatusCode != http.StatusOK {
			t.Fatalf("POST domain: status = %d (body: %s)", res.StatusCode, body)
		}
		return body
	}

	dasar := hitungAudit(t, env, "configure", "system_domain")
	body := posIdentik()
	if n := hitungAudit(t, env, "configure", "system_domain"); n != dasar+1 {
		t.Fatalf("audit configure domain = %d, mau %d", n, dasar+1)
	}
	pesanTulis := pesanRespons(body)
	if pesanTulis == "" {
		t.Fatal("DTO jalur tulis tidak memuat message — premis B-539 gugur")
	}
	if v, ada := bacaSetting(SettingKeyPublicURL); !ada || v != `"https://example.com"` {
		t.Fatalf("public_url sesudah POST pertama = %q (ada=%v), mau %q", v, ada, `"https://example.com"`)
	}

	// (1) B-539 + kontrol no-op sehat: POST identik diam, dan
	// message-nya sama persis dengan jalur tulis.
	body = posIdentik()
	if n := hitungAudit(t, env, "configure", "system_domain"); n != dasar+1 {
		t.Errorf("audit sesudah POST identik sehat = %d, mau %d — no-op bocor menulis", n, dasar+1)
	}
	if pesan := pesanRespons(body); pesan != pesanTulis {
		t.Errorf("message jalur no-op = %q, mau sama persis dengan jalur tulis %q (B-539)", pesan, pesanTulis)
	}

	// (2) B-532 arah menyimpang: public_url disuntik nilai salah
	// → POST identik BUKAN no-op; ia menulis dan memulihkan.
	if _, err := env.pool.Exec(ctx,
		`update settings set value = '"https://salah.example.net"' where key = $1`,
		SettingKeyPublicURL); err != nil {
		t.Fatalf("suntik public_url menyimpang: %v", err)
	}
	posIdentik()
	if n := hitungAudit(t, env, "configure", "system_domain"); n != dasar+2 {
		t.Errorf("audit sesudah POST identik dgn public_url menyimpang = %d, mau %d — gerbang menelan penyimpangan (B-532)", n, dasar+2)
	}
	if v, ada := bacaSetting(SettingKeyPublicURL); !ada || v != `"https://example.com"` {
		t.Errorf("public_url sesudah POST penyembuh = %q (ada=%v), mau pulih ke %q", v, ada, `"https://example.com"`)
	}

	// (3) B-532 arah hilang: baris public_url dihapus → pola sama.
	if _, err := env.pool.Exec(ctx,
		`delete from settings where key = $1`, SettingKeyPublicURL); err != nil {
		t.Fatalf("hapus baris public_url: %v", err)
	}
	posIdentik()
	if n := hitungAudit(t, env, "configure", "system_domain"); n != dasar+3 {
		t.Errorf("audit sesudah POST identik dgn public_url hilang = %d, mau %d — gerbang menelan kehilangan (B-532)", n, dasar+3)
	}
	if v, ada := bacaSetting(SettingKeyPublicURL); !ada || v != `"https://example.com"` {
		t.Errorf("public_url sesudah POST pemulih = %q (ada=%v), mau pulih ke %q", v, ada, `"https://example.com"`)
	}

	// (4) Kontrol akhir: keadaan sembuh → no-op pulih sepenuhnya.
	posIdentik()
	if n := hitungAudit(t, env, "configure", "system_domain"); n != dasar+3 {
		t.Errorf("audit akhir = %d, mau %d — no-op tidak pulih sesudah keadaan sembuh", n, dasar+3)
	}
}
