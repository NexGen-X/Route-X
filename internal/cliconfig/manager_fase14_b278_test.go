package cliconfig

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/NexGen-X/Route-X/internal/database/repo/identity"
)

// Berkas ini memuat test pengunci B-278 (Fase 14 kampanye
// tuntas-logika): rollback B-218 memulihkan berkas, tetapi efek
// Configure Cline yang sesungguhnya — store internal cline yang
// ditulis `cline auth` — berada di luar jangkauan jepretan. Put DB
// yang gagal sesudah auth sukses meninggalkan store cline pada
// konfigurasi BARU sementara DB tetap lama, dan Configure
// melaporkan gagal. Sesudah fix, jalur gagal-Put menjalankan
// kompensasi best-effort: auth ulang dengan konfigurasi tersimpan
// sebelumnya, sehingga store cline kembali sepasang dengan DB.

// TestB278ClinePutGagalKompensasiKonfigurasiLama mereproduksi
// probe Putaran Bersih W5-BARU-2 dengan biner cline palsu yang
// mencatat setiap pemanggilan auth: sesudah Configure gagal di
// Put, pemanggilan auth TERAKHIR harus membawa nilai konfigurasi
// LAMA (kompensasi), bukan berhenti pada nilai baru.
func TestB278ClinePutGagalKompensasiKonfigurasiLama(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	// Biner cline palsu: pisahkan tiap pemanggilan dengan penanda
	// ==== lalu catat argv per baris.
	binDir := t.TempDir()
	skrip := "#!/bin/sh\n" +
		"printf '====\\n' >> \"$HOME/cline-argv.txt\"\n" +
		"printf '%s\\n' \"$@\" >> \"$HOME/cline-argv.txt\"\n"
	if err := os.WriteFile(filepath.Join(binDir, "cline"), []byte(skrip), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	// Tanam konfigurasi lama (kunci pada field plaintext warisan —
	// jalur baca kunciTersimpan yang sama seperti konfigurasi hasil
	// migrasi), lalu bungkus store dengan Put yang selalu gagal.
	dasar := newMockSettingsStore()
	lama := StoredConfig{
		Mode:       ModeModelOnly,
		Target:     "model-lama",
		GatewayURL: "http://lama.invalid/v1",
		APIKey:     "GANTI_kunci-lama",
	}
	rawLama, err := json.Marshal(lama)
	if err != nil {
		t.Fatal(err)
	}
	dasar.data["cli:config:cline"] = identity.Setting{
		Key:   "cli:config:cline",
		Value: rawLama,
	}
	mgr := manajerUjiToolBaru(&mockSettingsStorePutGagal{dasar: dasar})

	_, err = mgr.Configure(context.Background(), ConfigureParams{
		ToolID: "cline", Mode: ModeModelOnly, Target: "model-baru",
		GatewayURL: "http://localhost:8080/v1", APIKey: "GANTI_kunci-baru",
	}, "http://localhost:8080/v1")
	if err == nil {
		t.Fatal("Configure harus gagal karena Put gagal")
	}

	argv := bacaBerkasUji(t, filepath.Join(home, "cline-argv.txt"))
	panggilan := strings.Split(argv, "====\n")
	// Split menghasilkan elemen kosong di kepala; panggilan auth
	// sungguhan adalah elemen-elemen berikutnya.
	var bersih []string
	for _, p := range panggilan {
		if strings.TrimSpace(p) != "" {
			bersih = append(bersih, p)
		}
	}
	if len(bersih) != 2 {
		t.Fatalf("jumlah pemanggilan cline auth = %d, mau 2 (terapan + kompensasi):\n%s", len(bersih), argv)
	}
	harusMemuat(t, bersih[0], "model-baru")
	harusMemuat(t, bersih[0], "GANTI_kunci-baru")
	// Kompensasi: auth terakhir membawa konfigurasi LAMA seutuhnya.
	harusMemuat(t, bersih[1], "model-lama")
	harusMemuat(t, bersih[1], "http://lama.invalid/v1")
	harusMemuat(t, bersih[1], "GANTI_kunci-lama")
	if strings.Contains(bersih[1], "model-baru") {
		t.Fatalf("kompensasi masih membawa nilai baru:\n%s", bersih[1])
	}

	// DB tetap pada konfigurasi lama (Put gagal) — pasangan yang
	// konsisten dengan store cline hasil kompensasi.
	tersimpan, err := dasar.Get(context.Background(), "cli:config:cline")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(tersimpan.Value), "model-lama") {
		t.Fatalf("DB tidak lagi memuat konfigurasi lama: %s", tersimpan.Value)
	}
}
