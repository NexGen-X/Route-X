package gateway

import (
	"context"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/NexGen-X/Route-X/internal/database/repo"
	"github.com/NexGen-X/Route-X/internal/providers"
	"github.com/NexGen-X/Route-X/internal/security"
)

// TestFactoryRelayDenoMenyusunTarget membuktikan protokol relay Deno yang
// benar (audit3 V-EO-02 + V-EO-03): base menjadi host relay DITAMBAH path
// asal, host tujuan asli pergi sebagai x-target-host, dan token relay dari
// userinfo pool diteruskan sebagai x-relay-token — bukan lagi seluruh URL
// proxy ditimpa menjadi base URL seperti bug lama.
func TestFactoryRelayDenoMenyusunTarget(t *testing.T) {
	relay, err := url.Parse("https://relay:tok3n-uji@proj-abc.deno.dev")
	if err != nil {
		t.Fatalf("parse relay: %v", err)
	}
	base, header, err := targetRelayDeno("https://api.anthropic.com/v1", relay)
	if err != nil {
		t.Fatalf("targetRelayDeno: %v", err)
	}
	if base != "https://proj-abc.deno.dev/v1" {
		t.Errorf("base = %q, ingin https://proj-abc.deno.dev/v1", base)
	}
	if header["x-target-host"] != "api.anthropic.com" {
		t.Errorf("x-target-host = %q, ingin api.anthropic.com", header["x-target-host"])
	}
	if header["x-relay-token"] != "tok3n-uji" {
		t.Errorf("x-relay-token = %q, ingin tok3n-uji", header["x-relay-token"])
	}

	// Tanpa token di userinfo: tidak ada header token (relay lama tanpa env
	// tetap terbuka seperti sebelumnya).
	relayPolos, _ := url.Parse("https://proj-abc.deno.dev")
	_, header2, err := targetRelayDeno("https://api.openai.com/v1", relayPolos)
	if err != nil {
		t.Fatalf("targetRelayDeno polos: %v", err)
	}
	if _, ada := header2["x-relay-token"]; ada {
		t.Error("x-relay-token terkirim padahal pool tidak menyimpan token")
	}

	// Seluruh jalur pabrik: kandidat Anthropic lewat pool Deno menghasilkan
	// adapter (konstruksi tidak menyentuh jaringan).
	eg := &sumberEgress{url: security.Secret("https://relay:tok3n-uji@proj-abc.deno.dev")}
	f := pabrikUji(t, kredensialUji(), eg)
	pool := "8f1f0b6e-0000-4000-8000-00000000d001"
	c := kandidatUji(providers.KindAnthropic, "https://api.anthropic.com")
	c.EgressPoolID = &pool
	if _, err := f.Provider(context.Background(), c); err != nil {
		t.Fatalf("Provider Anthropic via Deno: %v", err)
	}
}

// TestFactoryCloudflareGatingHost membuktikan gateway Cloudflare TIDAK lagi
// dipakai sebagai forward proxy generik (audit3 V-EO-04): host asal yang
// tidak punya padanan jalur di gateway ditolak eksplisit, sedangkan tiga
// host vendor yang dipetakan diterima.
func TestFactoryCloudflareGatingHost(t *testing.T) {
	eg := &sumberEgress{url: security.Secret("https://gateway.ai.cloudflare.com/v1/akun/gw")}
	f := pabrikUji(t, kredensialUji(), eg)
	pool := "8f1f0b6e-0000-4000-8000-00000000cf01"

	ditolak := kandidatUji(providers.KindOpenAICompatible, "https://api.deepseek.com/v1")
	ditolak.EgressPoolID = &pool
	if _, err := f.Provider(context.Background(), ditolak); err == nil ||
		!strings.Contains(err.Error(), "tidak bisa melayani") {
		t.Fatalf("host tak terpetakan via CF: err = %v, ingin penolakan eksplisit", err)
	}

	for _, k := range []struct{ kind, base string }{
		{providers.KindOpenAI, "https://api.openai.com/v1"},
		{providers.KindAnthropic, "https://api.anthropic.com"},
		{providers.KindGoogle, "https://generativelanguage.googleapis.com/v1beta"},
	} {
		c := kandidatUji(k.kind, k.base)
		c.EgressPoolID = &pool
		if _, err := f.Provider(context.Background(), c); err != nil {
			t.Errorf("host sah %s (%s) ditolak: %v", k.base, k.kind, err)
		}
	}
}

// TestFactoryNamaPrivatDiBalikProxy membuktikan audit3 V-EO-11 di jalur
// pabrik: di balik forward proxy, nama host target yang beresolusi ke alamat
// privat ditolak SEBELUM adapter dibuat — pemeriksaan nama mentah saja tidak
// cukup karena resolusi terjadi di sisi proxy.
func TestFactoryNamaPrivatDiBalikProxy(t *testing.T) {
	srv := serverUji(t)
	eg := &sumberEgress{url: security.Secret("http://127.0.0.1:19199")}
	// Kebijakan ketat tanpa whitelist loopback.
	f := NewFactory(kredensialUji(), eg, security.SSRFPolicy{AllowHTTP: true}, loggerSenyap())

	pool := "8f1f0b6e-0000-4000-8000-00000000e011"
	c := kandidatUji(providers.KindOpenAI, strings.Replace(srv.URL, "127.0.0.1", "localhost", 1))
	c.EgressPoolID = &pool
	_, err := f.Provider(context.Background(), c)
	if err == nil || !strings.Contains(err.Error(), "egress proxy") {
		t.Fatalf("target localhost di balik proxy: err = %v, ingin penolakan pemeriksaan di balik proxy", err)
	}
}

// penyegarPalsu menghitung panggilan refresh on-demand.
type penyegarPalsu struct {
	jumlah  atomic.Int32
	sesudah func()
}

func (p *penyegarPalsu) RefreshProvider(_ context.Context, _ string) error {
	p.jumlah.Add(1)
	if p.sesudah != nil {
		p.sesudah()
	}
	return nil
}

// TestFactoryRefreshOnDemand membuktikan audit3 V-EO-15: ketika tidak ada
// kredensial aktif untuk provider (kasus khas token OAuth kedaluwarsa semua),
// pabrik memicu refresh on-demand sekali lalu mencoba mengambil kredensial
// lagi — alih-alih langsung gagal sampai worker berkala berjalan.
func TestFactoryRefreshOnDemand(t *testing.T) {
	srv := serverUji(t)
	sumber := kredensialUji()
	sumber.err = repo.ErrNotFound

	penyegar := &penyegarPalsu{sesudah: func() {
		sumber.ganti("cred-2", security.Secret(rahasiaUji))
	}}
	f := pabrikUji(t, sumber, nil, WithOAuthRefresher(penyegar))

	if _, err := f.Provider(context.Background(), kandidatUji(providers.KindOpenAI, srv.URL)); err != nil {
		t.Fatalf("Provider sesudah refresh on-demand: %v", err)
	}
	if penyegar.jumlah.Load() != 1 {
		t.Errorf("refresh on-demand terpanggil %d kali, ingin 1", penyegar.jumlah.Load())
	}
	if sumber.jumlahPanggilan() != 2 {
		t.Errorf("Active terpanggil %d kali, ingin 2 (gagal lalu sesudah refresh)", sumber.jumlahPanggilan())
	}
}
