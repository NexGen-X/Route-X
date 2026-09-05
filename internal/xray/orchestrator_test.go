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
	if !ok || len(inbounds) < 4 {
		t.Fatalf("jumlah inbound tidak sesuai: %v", len(inbounds))
	}
}

func TestGenerateShareLinks(t *testing.T) {
	s := DefaultState("ai.test.example.com")
	links := GenerateShareLinks(s)

	if !strings.HasPrefix(links.VlessWS, "vless://") {
		t.Errorf("VlessWS tidak diawali 'vless://': %s", links.VlessWS)
	}
	if !strings.Contains(links.VlessWS, "ai.test.example.com:443") {
		t.Errorf("VlessWS tidak memuat host & port: %s", links.VlessWS)
	}
	if !strings.Contains(links.VlessWS, "type=ws") {
		t.Errorf("VlessWS tidak memuat type=ws: %s", links.VlessWS)
	}

	if !strings.HasPrefix(links.VlessGRPC, "vless://") {
		t.Errorf("VlessGRPC tidak diawali 'vless://': %s", links.VlessGRPC)
	}
	if !strings.Contains(links.VlessGRPC, "type=grpc") {
		t.Errorf("VlessGRPC tidak memuat type=grpc: %s", links.VlessGRPC)
	}

	if !strings.HasPrefix(links.TrojanWS, "trojan://") {
		t.Errorf("TrojanWS tidak diawali 'trojan://': %s", links.TrojanWS)
	}
	if !strings.Contains(links.TrojanWS, s.TrojanPassword) {
		t.Errorf("TrojanWS tidak memuat password: %s", links.TrojanWS)
	}

	// Pastikan parsing URL tidak panic
	parsed, err := url.Parse(links.VlessWS)
	if err != nil {
		t.Fatalf("gagal mem-parsing URL VlessWS: %v", err)
	}
	if parsed.User.Username() != s.UUID {
		t.Errorf("UUID di URL salah: dapat %s, ingin %s", parsed.User.Username(), s.UUID)
	}
}

func TestSyncToFileCandidates(t *testing.T) {
	tmpDir := t.TempDir()
	p1 := filepath.Join(tmpDir, "sub", "config.json")

	content := []byte(`{"test": true}`)
	// /dev/null adalah file perangkat karakter, tidak bisa dibuat subdirektori di bawahnya
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
}
