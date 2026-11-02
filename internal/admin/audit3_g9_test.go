package admin

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
)

// --- B-043 / V-AU-12: permukaan admin gagal-tertutup -------------------------

// Handlers tanpa layanan auth (salah-pasang konstruksi) harus menolak SEMUA
// permintaan admin. Versi lama memasang penjaga secara kondisional sehingga
// keadaan ini melayani seluruh permukaan — termasuk mutasi — tanpa
// autentikasi apa pun.
func TestRoutesTanpaAuthMenolakSemua(t *testing.T) {
	h := NewHandlers(Config{})
	router := h.Routes()

	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/overview"},
		{http.MethodGet, "/access/users/"},
		{http.MethodPost, "/access/users/"},
		{http.MethodDelete, "/access/sessions/11111111-1111-4111-8111-111111111111"},
	} {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(tc.method, tc.path, nil))
		if rec.Code != http.StatusInternalServerError {
			t.Errorf("%s %s: status = %d, mau 500 (fail-closed)", tc.method, tc.path, rec.Code)
		}
	}
}

// --- B-046 / V-AU-19: gateway_url dari konfigurasi tepercaya -----------------

// Muatan persis bukti live audit3: Host dan X-Forwarded-Proto dari klien
// langsung (bukan proxy tepercaya) tidak boleh membentuk URL yang ditulis ke
// konfigurasi CLI.
func TestResolveGatewayURLTidakDiracuniHeader(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://internal:8080/api/admin/cli/detected", nil)
	req.Host = "evil.example"
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Host", "evil.example")
	// RemoteAddr bawaan httptest adalah 192.0.2.1 — bukan proxy tepercaya.

	h := &Handlers{}
	if got := h.resolveGatewayURL(req); got != "http://evil.example/v1" {
		// Host dari koneksi memang dipakai apa adanya pada jenjang terakhir
		// (ia yang benar-benar dihubungi klien), tetapi skemanya TIDAK boleh
		// naik menjadi https hanya karena header memintanya.
		t.Errorf("tanpa konfigurasi: got = %q, mau skema http dari koneksi nyata", got)
	}

	// Konfigurasi env operator selalu menang atas turunan request.
	h = &Handlers{publicURL: "https://gateway.contoh.test"}
	if got := h.resolveGatewayURL(req); got != "https://gateway.contoh.test/v1" {
		t.Errorf("dengan PUBLIC_URL: got = %q, mau URL konfigurasi", got)
	}
}

// Di belakang proxy tepercaya, X-Forwarded-Proto dan X-Forwarded-Host adalah
// fakta yang ditulis proxy kita sendiri dan boleh dipakai.
func TestResolveGatewayURLDiBelakangProxyTepercaya(t *testing.T) {
	h := &Handlers{trustedProxies: []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}}

	req := httptest.NewRequest(http.MethodGet, "http://internal:8080/api/admin/cli/detected", nil)
	req.RemoteAddr = "10.1.2.3:5678"
	req.Host = "internal:8080"
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Host", "gateway.publik.test")
	if got := h.resolveGatewayURL(req); got != "https://gateway.publik.test/v1" {
		t.Errorf("got = %q, mau https://gateway.publik.test/v1", got)
	}

	// Peer yang sama dari alamat BUKAN proxy: header diabaikan lagi.
	req.RemoteAddr = "203.0.113.9:5678"
	if got := h.resolveGatewayURL(req); got != "http://internal:8080/v1" {
		t.Errorf("peer tak tepercaya: got = %q, mau http://internal:8080/v1", got)
	}
}
