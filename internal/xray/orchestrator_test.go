package xray

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewUUID(t *testing.T) {
	u1 := NewUUID()
	u2 := NewUUID()
	if len(u1) != 36 {
		t.Fatalf("panjang UUID salah: dapat %d, ingin 36 (%s)", len(u1), u1)
	}
	if u1 == u2 {
		t.Fatal("NewUUID menghasilkan nilai duplikat berturut-turut")
	}
}

func TestNewX25519KeyPair(t *testing.T) {
	priv, pub, err := NewX25519KeyPair()
	if err != nil {
		t.Fatalf("NewX25519KeyPair gagal: %v", err)
	}
	if len(priv) != 43 {
		t.Errorf("panjang kunci privat X25519 salah: dapat %d, ingin 43", len(priv))
	}
	if len(pub) != 43 {
		t.Errorf("panjang kunci publik X25519 salah: dapat %d, ingin 43", len(pub))
	}
	if priv == pub {
		t.Error("kunci privat dan publik identik")
	}
}

func TestNewShortID(t *testing.T) {
	s1 := NewShortID()
	s2 := NewShortID()
	if len(s1) != 8 {
		t.Fatalf("panjang short ID salah: dapat %d, ingin 8", len(s1))
	}
	if s1 == s2 {
		t.Error("short ID menghasilkan nilai duplikat")
	}
}

// TestNewBridgePassword memastikan kata sandi bridge selalu terisi, panjangnya
// sesuai, dan tidak memuat karakter yang membahayakan framing JSON/URL.
func TestNewBridgePassword(t *testing.T) {
	p1 := NewBridgePassword()
	p2 := NewBridgePassword()
	if len(p1) != bridgePasswordLength {
		t.Fatalf("panjang kata sandi bridge salah: dapat %d, ingin %d", len(p1), bridgePasswordLength)
	}
	if p1 == "" {
		t.Fatal("kata sandi bridge kosong")
	}
	if p1 == p2 {
		t.Error("kata sandi bridge menghasilkan nilai duplikat berturut-turut")
	}
	for _, c := range p1 {
		if !strings.ContainsRune(bridgePasswordAlphabet, c) {
			t.Errorf("kata sandi bridge memuat karakter di luar alfabet: %q", string(c))
		}
	}
}

// TestEnsureDefaultsFillsBridgeCredentials memastikan state lama yang tidak
// memiliki kredensial bridge (pra-perbaikan celah open proxy) mendapatkannya saat
// sinkronisasi, sehingga instalasi yang sudah berjalan langsung diamankan.
func TestEnsureDefaultsFillsBridgeCredentials(t *testing.T) {
	old := State{
		UUID:              "uuid-yang-sudah-ada",
		RealityPrivateKey: "kunci-lama",
		RealityPublicKey:  "kunci-publik-lama",
		RealityShortID:    "abcd1234",
		RealityDest:       "www.apple.com:443",
		RealityServerName: "www.apple.com",
		Domain:            "ai.test.example.com",
		SocksPort:         10808,
		HTTPPort:          10809,
		RealityPort:       8443,
	}
	old.EnsureDefaults("ai.test.example.com")

	if old.SocksUser != BridgeUser {
		t.Errorf("SocksUser state lama = %q, ingin %q", old.SocksUser, BridgeUser)
	}
	if old.SocksPassword == "" {
		t.Error("SocksPassword state lama tidak diisi oleh EnsureDefaults")
	}
	if old.UUID != "uuid-yang-sudah-ada" {
		t.Error("EnsureDefaults tidak boleh menimpa nilai yang sudah ada")
	}
}

// TestGenerateConfigLoopbackAuth memverifikasi penutupan celah open proxy pada
// konfigurasi runtime yang dihasilkan GenerateConfig:
// bridge SOCKS5/HTTP wajib mendengarkan loopback dan memerlukan autentikasi,
// sementara VLESS-Reality (proxy keluar sah yang berautentikasi kriptografis)
// tetap mendengarkan di semua antarmuka.
func TestGenerateConfigLoopbackAuth(t *testing.T) {
	s := DefaultState("ai.test.example.com")
	data, err := GenerateConfig(s)
	if err != nil {
		t.Fatalf("GenerateConfig error: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("GenerateConfig bukan JSON valid: %v", err)
	}

	inbounds, ok := parsed["inbounds"].([]any)
	if !ok || len(inbounds) != 3 {
		t.Fatalf("jumlah inbound tidak sesuai: %v, diharapkan 3", len(inbounds))
	}

	byTag := map[string]map[string]any{}
	for _, in := range inbounds {
		if m, isMap := in.(map[string]any); isMap {
			if tag, hasTag := m["tag"].(string); hasTag {
				byTag[tag] = m
			}
		}
	}

	// (a) bridge SOCKS5 & HTTP wajib loopback
	for _, tag := range []string{"socks-internal", "http-internal"} {
		m, ok := byTag[tag]
		if !ok {
			t.Fatalf("tag inbound wajib tidak ditemukan: %s", tag)
		}
		if listen, _ := m["listen"].(string); listen != "127.0.0.1" {
			t.Errorf("listen %s = %q, ingin 127.0.0.1 (bridge internal tidak boleh terbuka)", tag, listen)
		}
	}

	// (b) bridge SOCKS5 memerlukan autentikasi password dengan akun terisi
	socks := byTag["socks-internal"]
	socksSettings, _ := socks["settings"].(map[string]any)
	if auth, _ := socksSettings["auth"].(string); auth != "password" {
		t.Errorf("auth socks-internal = %q, ingin \"password\"", auth)
	}
	socksAccounts, _ := socksSettings["accounts"].([]any)
	if len(socksAccounts) == 0 {
		t.Fatal("accounts socks-internal kosong: bridge SOCKS5 tidak memiliki kredensial")
	}
	socksAccount, _ := socksAccounts[0].(map[string]any)
	if user, _ := socksAccount["user"].(string); user == "" {
		t.Error("user akun socks-internal kosong")
	}
	pass, _ := socksAccount["pass"].(string)
	if pass == "" {
		t.Error("pass akun socks-internal kosong: celah open proxy belum tertutup")
	}
	if pass != s.SocksPassword {
		t.Errorf("pass socks-internal tidak cocok kata sandi state")
	}

	// (c) bridge HTTP wajib punya akun (basic auth)
	httpIn := byTag["http-internal"]
	httpSettings, _ := httpIn["settings"].(map[string]any)
	httpAccounts, _ := httpSettings["accounts"].([]any)
	if len(httpAccounts) == 0 {
		t.Fatal("accounts http-internal kosong: bridge HTTP tidak memiliki kredensial")
	}
	httpAccount, _ := httpAccounts[0].(map[string]any)
	if pass, _ := httpAccount["pass"].(string); pass != s.SocksPassword {
		t.Errorf("pass http-internal tidak cocok kata sandi state")
	}

	// (d) VLESS-Reality tetap mendengarkan 0.0.0.0: proxy keluar sah dari internet
	vless := byTag["vless-reality-in"]
	if listen, _ := vless["listen"].(string); listen != "0.0.0.0" {
		t.Errorf("listen vless-reality-in = %q, ingin 0.0.0.0 (proxy keluar berautentikasi)", listen)
	}

	// (e) kata sandi state tidak pernah kosong
	if s.SocksPassword == "" {
		t.Error("SocksPassword state kosong")
	}
}

// TestBridgeEgressURL memastikan URL egress yang dipakai gateway memuat kredensial
// bridge dan mengarah ke loopback pada porta yang benar.
func TestBridgeEgressURL(t *testing.T) {
	s := DefaultState("ai.test.example.com")
	got := BridgeEgressURL(s)

	u, err := url.Parse(got)
	if err != nil {
		t.Fatalf("BridgeEgressURL tidak bisa di-parse: %v", err)
	}
	if u.Scheme != "socks5" {
		t.Errorf("skema = %q, ingin socks5", u.Scheme)
	}
	if u.Hostname() != DefaultBridgeHost {
		t.Errorf("host = %q, ingin %q", u.Hostname(), DefaultBridgeHost)
	}
	if u.Port() != fmt.Sprintf("%d", s.SocksPort) {
		t.Errorf("porta = %q, ingin %d", u.Port(), s.SocksPort)
	}
	if u.User.Username() != s.SocksUser {
		t.Errorf("user = %q, ingin %q", u.User.Username(), s.SocksUser)
	}
	pw, ok := u.User.Password()
	if !ok || pw != s.SocksPassword {
		t.Errorf("password di URL tidak cocok kata sandi state")
	}
}

// TestGenerateConfigDockerTopology memverifikasi bahwa deployment Docker Compose
// (XRAY_BRIDGE_HOST=xray) tetap aman: Xray listen 0.0.0.0 di dalam container
// terisolasi (port tidak di-publish ke host) dengan autentikasi wajib, sementara
// gateway terhubung lewat nama layanan. Tanpa turunan listen ini Xray menolak
// memulai karena tidak bisa listen pada nama DNS.
func TestGenerateConfigDockerTopology(t *testing.T) {
	s := DefaultState("ai.test.example.com")
	s.BridgeHost = "xray"
	data, err := GenerateConfig(s)
	if err != nil {
		t.Fatalf("GenerateConfig error: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("GenerateConfig bukan JSON valid: %v", err)
	}
	for _, in := range parsed["inbounds"].([]any) {
		m := in.(map[string]any)
		tag, _ := m["tag"].(string)
		listen, _ := m["listen"].(string)
		set, _ := m["settings"].(map[string]any)
		acc, _ := set["accounts"].([]any)
		auth, _ := set["auth"].(string)

		switch tag {
		case "socks-internal", "http-internal":
			if listen != "0.0.0.0" {
				t.Errorf("listen %s = %q, ingin 0.0.0.0 (Xray tidak bisa listen pada nama DNS)", tag, listen)
			}
			if len(acc) == 0 {
				t.Errorf("accounts %s kosong: bridge docker tanpa autentikasi = open proxy", tag)
			}
		case "vless-reality-in":
			if listen != "0.0.0.0" {
				t.Errorf("listen vless = %q, ingin 0.0.0.0", listen)
			}
		}
		if tag == "socks-internal" && auth != "password" {
			t.Errorf("auth socks docker = %q, ingin password", auth)
		}
	}

	// URL egress gateway harus pakai nama layanan (bukan 0.0.0.0) + kredensial
	u, err := url.Parse(BridgeEgressURL(s))
	if err != nil {
		t.Fatalf("BridgeEgressURL tidak bisa di-parse: %v", err)
	}
	if u.Hostname() != "xray" {
		t.Errorf("host egress docker = %q, ingin xray", u.Hostname())
	}
	if pw, ok := u.User.Password(); !ok || pw == "" {
		t.Error("egress docker tanpa kata sandi")
	}
}

// TestBridgeListenAddress memastikan alamat listen diturunkan dengan benar:
// host loopback dipertahankan, host lain (nama service Docker) menjadi 0.0.0.0.
func TestBridgeListenAddress(t *testing.T) {
	cases := []struct{ in, want string }{
		{"127.0.0.1", "127.0.0.1"},
		{"localhost", "localhost"},
		{"::1", "::1"},
		{"xray", "0.0.0.0"},
		{"proxy.internal", "0.0.0.0"},
	}
	for _, c := range cases {
		if got := bridgeListenAddress(c.in); got != c.want {
			t.Errorf("bridgeListenAddress(%q) = %q, ingin %q", c.in, got, c.want)
		}
	}
}

func TestGenerateConfig(t *testing.T) {
	s := DefaultState("ai.test.example.com")
	data, err := GenerateConfig(s)
	if err != nil {
		t.Fatalf("GenerateConfig error: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("GenerateConfig bukan JSON valid: %v", err)
	}

	inbounds, ok := parsed["inbounds"].([]any)
	if !ok || len(inbounds) != 3 {
		t.Fatalf("jumlah inbound tidak sesuai: %v, diharapkan 3", len(inbounds))
	}

	// Pastikan inbound mencakup semua protokol aktif
	expectedTags := map[string]bool{
		"socks-internal":   false,
		"http-internal":    false,
		"vless-reality-in": false,
	}

	for _, in := range inbounds {
		m, isMap := in.(map[string]any)
		if !isMap {
			continue
		}
		if tag, hasTag := m["tag"].(string); hasTag {
			expectedTags[tag] = true
		}
	}

	for tag, found := range expectedTags {
		if !found {
			t.Errorf("tag inbound wajib tidak ditemukan dalam config: %s", tag)
		}
	}
}

func TestGenerateShareLinks(t *testing.T) {
	s := DefaultState("ai.test.example.com")
	links := GenerateShareLinks(s)

	if !strings.HasPrefix(links.VlessReality, "vless://") {
		t.Errorf("VlessReality tidak diawali 'vless://': %s", links.VlessReality)
	}
	if !strings.Contains(links.VlessReality, "security=reality") {
		t.Errorf("VlessReality tidak memuat security=reality: %s", links.VlessReality)
	}
	if !strings.Contains(links.VlessReality, "flow=xtls-rprx-vision") {
		t.Errorf("VlessReality tidak memuat flow=xtls-rprx-vision: %s", links.VlessReality)
	}

	if !strings.HasPrefix(links.SocksInternal, "socks5://") {
		t.Errorf("SocksInternal tidak diawali 'socks5://': %s", links.SocksInternal)
	}
	if !strings.HasPrefix(links.HTTPInternal, "http://") {
		t.Errorf("HTTPInternal tidak diawali 'http://': %s", links.HTTPInternal)
	}

	if len(links.Protocols) != 3 {
		t.Fatalf("daftar Protocols salah: dapat %d, ingin 3", len(links.Protocols))
	}

	// Pastikan parsing URL tidak panic
	parsed, err := url.Parse(links.VlessReality)
	if err != nil {
		t.Fatalf("gagal mem-parsing URL VlessReality: %v", err)
	}
	if parsed.User.Username() != s.UUID {
		t.Errorf("UUID di URL salah: dapat %s, ingin %s", parsed.User.Username(), s.UUID)
	}
}

func TestSyncToFileCandidates(t *testing.T) {
	tmpDir := t.TempDir()
	p1 := filepath.Join(tmpDir, "sub", "config.json")

	content := []byte(`{"test": true}`)
	written, err := SyncToFileCandidates(content, "/dev/null/invalid_dir/config.json", p1)
	if err != nil {
		t.Fatalf("SyncToFileCandidates gagal: %v", err)
	}
	if written != p1 {
		t.Errorf("path tertulis salah: dapat %s, ingin %s", written, p1)
	}

	got, err := os.ReadFile(p1)
	if err != nil {
		t.Fatalf("gagal membaca kembali berkas: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("isi berkas berbeda: dapat %s, ingin %s", string(got), string(content))
	}

	fileInfo, err := os.Stat(p1)
	if err != nil {
		t.Fatalf("membaca metadata berkas: %v", err)
	}
	if gotMode := fileInfo.Mode().Perm(); gotMode != 0600 {
		t.Errorf("izin berkas = %04o, diharapkan 0600", gotMode)
	}
	dirInfo, err := os.Stat(filepath.Dir(p1))
	if err != nil {
		t.Fatalf("membaca metadata direktori: %v", err)
	}
	if gotMode := dirInfo.Mode().Perm(); gotMode != 0700 {
		t.Errorf("izin direktori = %04o, diharapkan 0700", gotMode)
	}
}
