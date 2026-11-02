package security

import (
	"context"
	"errors"
	"net/netip"
	"strings"
	"testing"
)

// TestCheckAddrPrefixBaruAudit3 membuktikan dua kelas alamat yang dulu lolos
// kini tertolak: NAT64 local-use (64:ff9b:1::/48 — padanannya adalah alamat
// privat/lab di balik penerjemah) dan IPv4-compatible usang (::/96)
// (audit3 V-EO-12).
func TestCheckAddrPrefixBaruAudit3(t *testing.T) {
	p := DefaultSSRFPolicy()
	for _, raw := range []string{
		"64:ff9b:1::c000:201",  // NAT64 local-use → 192.0.2.1 di baliknya
		"64:ff9b:1::a9fe:a9fe", // NAT64 local-use → 169.254.169.254
		"::192.0.2.1",          // IPv4-compatible
		"::169.254.169.254",    // IPv4-compatible metadata
	} {
		addr := netip.MustParseAddr(raw)
		if err := CheckAddr(addr, p); !errors.Is(err, ErrBlockedAddress) {
			t.Errorf("CheckAddr(%s) = %v, ingin ErrBlockedAddress", raw, err)
		}
	}
	// NAT64 well-known yang membenamkan alamat metadata juga tertolak
	// (kebijakan memang memblokir awalan itu secara keseluruhan).
	if err := CheckAddr(netip.MustParseAddr("64:ff9b::a9fe:a9fe"), p); err == nil {
		t.Error("CheckAddr(NAT64 well-known → metadata) lolos, ingin tertolak")
	}
}

// TestKlasifikasiRelayDariHost membuktikan klasifikasi membaca HOST hasil
// uraian URL — bukan substring di path/query/userinfo seperti bug lama yang
// mengubah proxy biasa menjadi "relay" (audit3 V-EO-08).
func TestKlasifikasiRelayDariHost(t *testing.T) {
	kasis := []struct {
		raw   string
		ingin string
	}{
		{"https://gateway.ai.cloudflare.com/v1/acc/gw", RelayCloudflare},
		{"https://user:pass@gateway.ai.cloudflare.com/v1/acc/gw", RelayCloudflare},
		{"https://proj.deno.dev/relay", RelayDeno},
		{"https://deno.dev/x", RelayDeno},
		// Substring di path/query BUKAN relay:
		{"http://proxy.internal:3128/gateway.ai.cloudflare.com", RelayTidakAda},
		{"http://proxy.internal:3128/?next=.deno.dev", RelayTidakAda},
		{"http://deno.dev@proxy.internal:3128/", RelayTidakAda}, // userinfo menipu
		{"https://evil-deno.dev/", RelayTidakAda},               // bukan subdomain deno.dev
		{"https://deno.dev.evil.com/", RelayTidakAda},           // sufiks palsu
		{"https://gateway.ai.cloudflare.com.evil.com/", RelayTidakAda},
		{"bukan url", RelayTidakAda},
		{"", RelayTidakAda},
	}
	for _, k := range kasis {
		got, _ := KlasifikasiRelay(k.raw)
		if got != k.ingin {
			t.Errorf("KlasifikasiRelay(%q) = %q, ingin %q", k.raw, got, k.ingin)
		}
	}
}

// TestPeriksaHostTujuanNamaPrivat membuktikan gerbang nama→IP di balik proxy
// (audit3 V-EO-11): nama yang beresolusi ke alamat terlarang ditolak meski
// nama itu sendiri lolos pemeriksaan nama, dan nama/IP publik lolos.
func TestPeriksaHostTujuanNamaPrivat(t *testing.T) {
	ctx := context.Background()
	p := DefaultSSRFPolicy()

	// "localhost" beresolusi ke loopback lewat /etc/hosts tanpa jaringan.
	if err := PeriksaHostTujuan(ctx, "http://localhost:19091/v1", p); err == nil ||
		!strings.Contains(err.Error(), "localhost") {
		t.Errorf("PeriksaHostTujuan(localhost) = %v, ingin penolakan menyebut host", err)
	}

	// Alamat IP literal publik lolos tanpa resolusi.
	if err := PeriksaHostTujuan(ctx, "https://8.8.8.8/v1", p); err != nil {
		t.Errorf("PeriksaHostTujuan(8.8.8.8) = %v, ingin lolos", err)
	}

	// Alamat metadata literal tertolak langsung.
	if err := PeriksaHostTujuan(ctx, "http://169.254.169.254/latest", p); err == nil {
		t.Error("PeriksaHostTujuan(169.254.169.254) lolos, ingin tertolak")
	}

	// Kebijakan yang mengizinkan loopback (operator lokal) tetap menghormati
	// whitelist: nama localhost lolos.
	pLonggar := SSRFPolicy{AllowHTTP: true, AllowedPrivateAddrs: []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8"), netip.MustParsePrefix("::1/128")}}
	if err := PeriksaHostTujuan(ctx, "http://localhost:19091/v1", pLonggar); err != nil {
		t.Errorf("PeriksaHostTujuan(localhost, whitelist) = %v, ingin lolos", err)
	}
}
