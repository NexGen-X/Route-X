package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/NexGen-X/Route-X/internal/security"
	"github.com/NexGen-X/Route-X/internal/webhooks"
)

// TestB342Allowlist4In6Dinormalkan adalah pengunci B-342 sisi
// allowlist API key: entri 4-in-6 (::ffff:a.b.c.d) dulu tersimpan
// apa adanya sementara alamat klien selalu di-Unmap sebelum
// dibandingkan — entri itu tidak pernah cocok dan allowlist-nya
// mati senyap. Kini entri dinormalkan ke bentuk IPv4 polos di
// titik tulis, pada jalur buat MAUPUN ubah (keduanya kini memakai
// pengurai bersama yang sama, B-355(ii)); prefiks 4-in-6 yang
// lebih luas dari peta IPv4 ditolak dengan menyebut entrinya.
func TestB342Allowlist4In6Dinormalkan(t *testing.T) {
	env := setupTestEnv(t)
	client := env.newClient(t)
	client.login("superadmin@routex.internal", testPassword)

	ambilAllowlist := func(body []byte) []string {
		t.Helper()
		var dto struct {
			Key struct {
				IPAllowlist []string `json:"ip_allowlist"`
			} `json:"key"`
		}
		if err := json.Unmarshal(body, &dto); err != nil {
			t.Fatalf("urai DTO kunci: %v (body: %s)", err, body)
		}
		return dto.Key.IPAllowlist
	}

	res, body := client.do(http.MethodPost, "/api/admin/access/api-keys", map[string]any{
		"name":         "Kunci 4in6",
		"ip_allowlist": []string{"::ffff:10.0.0.9", "::ffff:10.0.0.0/120"},
	}, true)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("buat dengan allowlist 4-in-6: status = %d, mau 201 (body: %s)", res.StatusCode, body)
	}
	got := ambilAllowlist(body)
	if len(got) != 2 || got[0] != "10.0.0.9/32" || got[1] != "10.0.0.0/24" {
		t.Errorf("allowlist tersimpan = %v, mau [10.0.0.9/32 10.0.0.0/24] — bentuk 4-in-6 tersimpan mentah", got)
	}
	var dibuat struct {
		Key struct {
			ID string `json:"id"`
		} `json:"key"`
	}
	if err := json.Unmarshal(body, &dibuat); err != nil || dibuat.Key.ID == "" {
		t.Fatalf("urai id kunci: %v (body: %s)", err, body)
	}

	// Jalur ubah: normalisasi yang sama.
	res, body = client.do(http.MethodPut, "/api/admin/access/api-keys/"+dibuat.Key.ID, map[string]any{
		"ip_allowlist": []string{"::ffff:192.168.0.0/112"},
	}, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("ubah dengan allowlist 4-in-6: status = %d, mau 200 (body: %s)", res.StatusCode, body)
	}
	got = ambilAllowlist(body)
	if len(got) != 1 || got[0] != "192.168.0.0/16" {
		t.Errorf("allowlist sesudah ubah = %v, mau [192.168.0.0/16]", got)
	}

	// Prefiks 4-in-6 yang lebih luas dari peta IPv4: ditolak.
	res, body = client.do(http.MethodPut, "/api/admin/access/api-keys/"+dibuat.Key.ID, map[string]any{
		"ip_allowlist": []string{"::ffff:0.0.0.0/95"},
	}, true)
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("ubah dengan prefiks 4-in-6 /95: status = %d, mau 400 (body: %s)", res.StatusCode, body)
	}
	if !strings.Contains(string(body), "invalid_ip_allowlist") {
		t.Errorf("respons tidak memuat invalid_ip_allowlist: %s", body)
	}

	// Kontrol: IPv6 murni tetap tersimpan sebagai IPv6.
	res, body = client.do(http.MethodPut, "/api/admin/access/api-keys/"+dibuat.Key.ID, map[string]any{
		"ip_allowlist": []string{"2001:db8::/32", "::1"},
	}, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("ubah dengan allowlist v6 murni: status = %d, mau 200 (body: %s)", res.StatusCode, body)
	}
	got = ambilAllowlist(body)
	if len(got) != 2 || got[0] != "2001:db8::/32" || got[1] != "::1/128" {
		t.Errorf("allowlist v6 = %v, mau [2001:db8::/32 ::1/128]", got)
	}
}

// TestB354UpdateKosongSeragam400 adalah pengunci B-354(ii) sisi
// handler: PUT tanpa satu pun field pada budgets, rate-limits, dan
// webhooks dulu menjawab 200 dengan baris lama, sementara
// keys/upstream/egress menjawab 400 no_changes sejak B-306. Kini
// ketiganya ikut kontrak yang sama.
func TestB354UpdateKosongSeragam400(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()
	client := env.newClient(t)
	client.login("superadmin@routex.internal", testPassword)

	res, body := client.do(http.MethodPost, "/api/admin/gateway/budgets/", map[string]any{
		"name": "anggaran-b354", "scope": "global", "period": "monthly", "limit_usd": "10",
	}, true)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("buat anggaran: status = %d (body: %s)", res.StatusCode, body)
	}
	var budget BudgetDTO
	if err := json.Unmarshal(body, &budget); err != nil {
		t.Fatalf("urai anggaran: %v", err)
	}

	res, body = client.do(http.MethodPost, "/api/admin/gateway/rate-limits/", map[string]any{
		"scope": "global", "requests_per_minute": 60,
	}, true)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("buat batas laju: status = %d (body: %s)", res.StatusCode, body)
	}
	var rl RateLimitDTO
	if err := json.Unmarshal(body, &rl); err != nil {
		t.Fatalf("urai batas laju: %v", err)
	}

	repoWebhook := webhooks.NewRepo(env.pool)
	wh, err := repoWebhook.Create(ctx, webhooks.CreateWebhookParams{
		Name: "webhook-b354", URL: "https://example.com/hooks",
		Events:     []string{"request.completed"},
		RawSecret:  "GANTI_rahasia" + "_b354",
		Cipher:     env.cipher,
		Enabled:    true,
		MaxRetries: 3,
		TimeoutMS:  10000,
		SSRFPolicy: security.SSRFPolicy{AllowHTTP: true, AllowPrivate: true},
	})
	if err != nil {
		t.Fatalf("buat webhook: %v", err)
	}

	kasus := []struct {
		nama string
		url  string
	}{
		{"budgets", "/api/admin/gateway/budgets/" + budget.ID},
		{"rate-limits", "/api/admin/gateway/rate-limits/" + rl.ID},
		{"webhooks", "/api/admin/automation/webhooks/" + wh.ID},
	}
	for _, k := range kasus {
		t.Run(k.nama, func(t *testing.T) {
			res, body := client.do(http.MethodPut, k.url, map[string]any{}, true)
			if res.StatusCode != http.StatusBadRequest {
				t.Fatalf("PUT {} status = %d, mau 400 (body: %s)", res.StatusCode, body)
			}
			if !strings.Contains(string(body), "no_changes") {
				t.Errorf("PUT {} body = %s, mau memuat kode no_changes", body)
			}
		})
	}
}
