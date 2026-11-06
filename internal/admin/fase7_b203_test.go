package admin

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
	"github.com/NexGen-X/Route-X/internal/security"
	"github.com/NexGen-X/Route-X/internal/worker"
)

// Test B-203: provisioning Deno atas proyek yang sudah punya pool
// ditolak 409 SEBELUM panggilan Deno apa pun — provisioning ulang
// selalu merotasi token relay dan akan memutus pool lama diam-diam,
// dan pool kedua bahkan gagal dibuat (nama unik) sesudah efek samping
// deploy terjadi. Pra-fix handler langsung men-deploy: proyek bernama
// acak tidak pernah tertangkap, dan retry bernama sama menabrak unik
// nama pool di akhir.
func TestFase7B203ProvisioningDenoProyekSudahAda(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()
	c := env.newClient(t)
	c.login("superadmin@routex.internal", testPassword)

	egressRepo, err := upstream.NewEgressRepo(env.pool, env.cipher)
	if err != nil {
		t.Fatalf("NewEgressRepo: %v", err)
	}
	buatPool := func(nama, proxyURL string) {
		t.Helper()
		enabled := true
		weight := 1
		if _, err := egressRepo.Create(ctx, upstream.CreateEgressParams{
			Name:     nama,
			Kind:     "https",
			ProxyURL: security.Secret(proxyURL),
			Enabled:  &enabled,
			Weight:   &weight,
		}); err != nil {
			t.Fatalf("buat pool %s: %v", nama, err)
		}
	}
	// Pool dari provisioning sebelumnya: nama bawaan untuk proyek proj-x,
	// dan pool bernama bebas yang URL-nya menunjuk proyek proj-y.
	buatPool("🦕 Deno Relay (proj-x)", "http://relay:token@proj-x.deno.dev")
	buatPool("pool-relay-kedua", "http://relay:token@proj-y.deno.dev")

	provision := func(proyek string) (int, string) {
		t.Helper()
		res, body := c.do(http.MethodPost, "/api/admin/upstreams/egress-pools/provision/deno",
			map[string]any{"project_name": proyek, "access_token": "GANTI_token_deno_uji"}, true)
		return res.StatusCode, string(body)
	}

	t.Run("nama pool bawaan yang sama ditolak", func(t *testing.T) {
		code, body := provision("proj-x")
		if code != http.StatusConflict || !strings.Contains(body, "deno_project_already_provisioned") {
			t.Fatalf("status %d, body %s; mau 409 deno_project_already_provisioned", code, body)
		}
		if !strings.Contains(body, "Deno Relay (proj-x)") {
			t.Errorf("pesan 409 tidak menyebut pool yang ada: %s", body)
		}
	})

	t.Run("host masked hint yang sama ditolak walau nama pool bebas", func(t *testing.T) {
		code, body := provision("proj-y")
		if code != http.StatusConflict || !strings.Contains(body, "deno_project_already_provisioned") {
			t.Fatalf("status %d, body %s; mau 409 deno_project_already_provisioned", code, body)
		}
		if !strings.Contains(body, "pool-relay-kedua") {
			t.Errorf("pesan 409 tidak menyebut pool yang ada: %s", body)
		}
	})

	t.Run("kunci provisioning yang sedang dipegang ditolak", func(t *testing.T) {
		// Nilai literal worker.LockProvisionDeno (prefix pekerja | 0x08)
		// supaya test terkompilasi juga pada kode pra-fix.
		unlock, terkunci, err := worker.TryAdvisoryLock(ctx, env.pool, 0x5258574F00000008)
		if err != nil || !terkunci {
			t.Fatalf("mengambil kunci uji: terkunci=%v err=%v", terkunci, err)
		}
		defer unlock()
		code, body := provision("proj-z")
		if code != http.StatusConflict || !strings.Contains(body, "provision_in_progress") {
			t.Fatalf("status %d, body %s; mau 409 provision_in_progress", code, body)
		}
	})
}
