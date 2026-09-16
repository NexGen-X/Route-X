package xray

import (
	"encoding/json"
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
