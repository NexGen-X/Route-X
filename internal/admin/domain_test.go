package admin

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
	"github.com/NexGen-X/Route-X/internal/security"
	"github.com/NexGen-X/Route-X/internal/xray"
)

func TestDomainRegex(t *testing.T) {
	valid := []string{
		"example.com",
		"ai.example.com",
		"sub.sub.domain.co.id",
		"my-gateway.org",
		"route-x.dev",
	}

	for _, d := range valid {
		if !domainRegex.MatchString(d) {
			t.Errorf("domain %q harusnya valid, tapi ditolak regex", d)
		}
	}

	invalid := []string{
		"localhost",
		"127.0.0.1",
		"http://example.com",
		"example",
		"example..com",
		"-bad.com",
		"bad-.com",
	}

	for _, d := range invalid {
		if domainRegex.MatchString(d) {
			t.Errorf("domain %q harusnya tidak valid, tapi diterima regex", d)
		}
	}
}

func TestCaddyTLSCheck_Localhost(t *testing.T) {
	h := &Handlers{}

	loopback := func(target string) *http.Request {
		// httptest default RemoteAddr 192.0.2.1 (bukan loopback); guard
		// CaddyTLSCheck hanya melayani peer loopback, jadi test memakai
		// RemoteAddr loopback seperti Caddy same-host yang asli.
		req := httptest.NewRequest(http.MethodGet, target, nil)
		req.RemoteAddr = "127.0.0.1:1234"
		return req
	}

	// Localhost harus selalu diizinkan
	req := loopback("/api/internal/tls-check?domain=localhost")
	rr := httptest.NewRecorder()
	h.CaddyTLSCheck(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status localhost = %d, diharapkan %d", rr.Code, http.StatusOK)
	}

	// 127.0.0.1 harus selalu diizinkan
	req = loopback("/api/internal/tls-check?domain=127.0.0.1")
	rr = httptest.NewRecorder()
	h.CaddyTLSCheck(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status 127.0.0.1 = %d, diharapkan %d", rr.Code, http.StatusOK)
	}

	// Domain kosong harus 400
	req = loopback("/api/internal/tls-check")
	rr = httptest.NewRecorder()
	h.CaddyTLSCheck(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status domain kosong = %d, diharapkan %d", rr.Code, http.StatusBadRequest)
	}

	// Domain yang tidak terdaftar harus 403
	req = loopback("/api/internal/tls-check?domain=random.unauthorized.com")
	rr = httptest.NewRecorder()
	h.CaddyTLSCheck(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("status domain liar = %d, diharapkan %d", rr.Code, http.StatusForbidden)
	}
}

func TestCaddyTLSCheck_NonLoopbackDitolak(t *testing.T) {
	// Peer non-loopback DITOLAK sebelum membaca parameter: endpoint ini bukan
	// oracle enumerasi domain untuk jaringan luar.
	h := &Handlers{}
	req := httptest.NewRequest(http.MethodGet, "/api/internal/tls-check?domain=localhost", nil)
	req.RemoteAddr = "203.0.113.10:1234"
	rr := httptest.NewRecorder()
	h.CaddyTLSCheck(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("status non-loopback = %d, diharapkan %d", rr.Code, http.StatusForbidden)
	}
}

func TestGetXrayLinks(t *testing.T) {
	h := &Handlers{}
	state, links := h.getXrayLinks(t.Context(), "ai.gateway.test")
	if state.UUID == "" {
		t.Error("UUID Xray kosong")
	}
	if !strings.HasPrefix(links.VlessReality, "vless://") {
		t.Errorf("VlessReality tidak valid: %s", links.VlessReality)
	}
	if !strings.Contains(links.VlessReality, "security=reality") {
		t.Errorf("VlessReality tidak memuat security=reality: %s", links.VlessReality)
	}
	if !strings.HasPrefix(links.SocksInternal, "socks5://") {
		t.Errorf("SocksInternal tidak valid: %s", links.SocksInternal)
	}
	if !strings.HasPrefix(links.HTTPInternal, "http://") {
		t.Errorf("HTTPInternal tidak valid: %s", links.HTTPInternal)
	}
	if len(links.Protocols) != 3 {
		t.Errorf("daftar Protocols salah: %d, diharapkan 3", len(links.Protocols))
	}
}

// TestEnsureXrayEgressPool memverifikasi penutupan celah di jalur runtime:
// pool egress bawaan harus memuat kredensial bridge (bukan socks5://xray:10808
// tanpa autentikasi), dan pool yang sudah admin arahkan ke host lain tidak
// boleh ditimpa.
func TestEnsureXrayEgressPool(t *testing.T) {
	env := setupTestEnv(t)

	egressRepo, err := upstream.NewEgressRepo(env.pool, env.cipher)
	if err != nil {
		t.Fatalf("buat egress repo: %v", err)
	}
	h := &Handlers{egressRepo: egressRepo, logger: slog.New(slog.DiscardHandler)}

	state := xray.DefaultState("ai.test.example.com")
	want := security.Secret(xray.BridgeEgressURL(state))
	ctx := t.Context()

	// 1. Pool pertama kali dibuat sudah memuat kredensial.
	h.ensureXrayEgressPool(ctx, state)

	pools, err := egressRepo.List(ctx, false)
	if err != nil {
		t.Fatalf("daftar egress pool: %v", err)
	}
	var poolID string
	for _, p := range pools {
		if strings.Contains(p.Name, "Xray") {
			poolID = p.ID
			break
		}
	}
	if poolID == "" {
		t.Fatal("pool egress Xray tidak dibuat")
	}
	got, err := egressRepo.ProxyURL(ctx, poolID)
	if err != nil {
		t.Fatalf("baca proxy URL pool: %v", err)
	}
	if got != want {
		t.Errorf("proxy URL pool baru = %s, ingin memuat kredensial bridge", security.Mask(got.Reveal(), "socks5://", 0))
	}
	if !strings.Contains(got.Reveal(), state.SocksPassword) {
		t.Errorf("proxy URL pool baru tidak memuat kata sandi bridge")
	}

	// 2. Pool lama tanpa autentikasi (pra-perbaikan) harus diperbarui.
	if _, err := egressRepo.SetProxyURL(ctx, poolID, security.Secret("socks5://xray:10808")); err != nil {
		t.Fatalf("simulasikan pool lama: %v", err)
	}
	h.ensureXrayEgressPool(ctx, state)
	got, err = egressRepo.ProxyURL(ctx, poolID)
	if err != nil {
		t.Fatalf("baca proxy URL pool setelah pembaruan: %v", err)
	}
	if got != want {
		t.Errorf("pool lama tidak diperbarui: dapat %s", security.Mask(got.Reveal(), "socks5://", 0))
	}

	// 3. Custom edit admin ke host lain tidak boleh ditimpa.
	custom := security.Secret("socks5://custom-proxy.example.test:1080")
	if _, err := egressRepo.SetProxyURL(ctx, poolID, custom); err != nil {
		t.Fatalf("simulasikan custom edit: %v", err)
	}
	h.ensureXrayEgressPool(ctx, state)
	got, err = egressRepo.ProxyURL(ctx, poolID)
	if err != nil {
		t.Fatalf("baca proxy URL pool setelah sinkronisasi ulang: %v", err)
	}
	if got != custom {
		t.Errorf("custom edit pool ditimpa: dapat %s", security.Mask(got.Reveal(), "socks5://", 0))
	}
}
