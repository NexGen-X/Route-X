package security

import (
	"net/url"
	"strings"
)

// Klasifikasi URL egress menjadi jenis relay reverse-proxy.
//
// Egress pool ada dua keluarga yang perilakunya berlawanan: forward proxy
// (HTTP CONNECT / SOCKS — gateway tetap menghubungi base URL provider lewat
// proxy) dan relay reverse proxy (Cloudflare AI Gateway & Deno Deploy — base
// URL provider DITIMPA ke URL relay). Menentukan keluarga dari string mentah
// URL adalah bug yang terbukti live (audit3 V-EO-08): proxy biasa yang URL-nya
// mengandung ".deno.dev" di path atau userinfo salah dikenali sebagai relay
// dan lalu lintasnya dibelokkan total. Karena itu klasifikasi di sini selalu
// bekerja pada HOST hasil uraian URL, tidak pernah pada substring mentah.
const (
	// RelayTidakAda berarti URL bukan relay yang dikenal (forward proxy biasa
	// atau bukan URL proxy sama sekali).
	RelayTidakAda = ""
	// RelayCloudflare berarti host adalah gateway.ai.cloudflare.com.
	RelayCloudflare = "cloudflare"
	// RelayDeno berarti host adalah subdomain *.deno.dev.
	RelayDeno = "deno"
)

// KlasifikasiRelay mengembalikan jenis relay dari URL proxy mentah beserta
// hasil uraiannya. URL yang tidak terurai atau host-nya tidak dikenal
// menghasilkan RelayTidakAda.
func KlasifikasiRelay(raw string) (string, *url.URL) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u == nil {
		return RelayTidakAda, nil
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	switch {
	case host == "gateway.ai.cloudflare.com":
		return RelayCloudflare, u
	case host == "deno.dev" || strings.HasSuffix(host, ".deno.dev"):
		return RelayDeno, u
	default:
		return RelayTidakAda, u
	}
}
