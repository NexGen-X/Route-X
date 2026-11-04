package cliconfig

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/NexGen-X/Route-X/internal/database/repo/identity"
)

// idToolTercabutTahap1 adalah 12 id yang dicabut total pada
// perombakan registri Tahap 1.
var idToolTercabutTahap1 = []string{
	"agy", "cursor-agent", "cody", "plandex", "ollama", "continue",
	"sgpt", "fabric", "llm", "opencommit", "chatblade", "k8sgpt",
}

// TestRegistriTahap1TepatSebelas membuktikan sasaran final Tahap 1:
// registri tepat berisi 11 tool (lima yang dipertahankan ditambah
// enam tool baru), semuanya didukung gateway (UnsupportedReason
// kosong), dan ke-12 id tercabut tidak dikenal lagi.
func TestRegistriTahap1TepatSebelas(t *testing.T) {
	tools := SupportedTools()
	if len(tools) != 11 {
		t.Fatalf("jumlah tool registri = %d, mau 11 sesudah perombakan Tahap 1", len(tools))
	}
	mau := map[string]bool{
		"claude": true, "opencode": true, "copilot": true, "goose": true, "aider": true,
		"codex": true, "grok-build": true, "qwen": true, "crush": true, "hermes": true, "cline": true,
	}
	for _, tool := range tools {
		if !mau[tool.ID] {
			t.Errorf("tool %s tidak seharusnya ada di registri hasil perombakan Tahap 1", tool.ID)
		}
		delete(mau, tool.ID)
		if tool.UnsupportedReason != "" {
			t.Errorf("tool %s seharusnya didukung gateway, alasan terisi: %q", tool.ID, tool.UnsupportedReason)
		}
	}
	for id := range mau {
		t.Errorf("tool bertahan %s hilang dari registri", id)
	}
	for _, dicabut := range idToolTercabutTahap1 {
		if _, ok := cariToolDef(dicabut); ok {
			t.Errorf("tool %s seharusnya sudah dicabut dari registri", dicabut)
		}
	}
}

// TestConfigureToolTercabutTidakDikenal membuktikan Configure dan
// deteksi untuk ke-12 id lama kini menjawab tidak dikenal — bukan
// "tidak didukung gateway", bukan sukses — dan tanpa efek samping
// (tidak ada setelan tersimpan, tidak ada key.env).
func TestConfigureToolTercabutTidakDikenal(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	ctx := context.Background()
	store := newMockSettingsStore()
	mgr := NewManager(store, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))

	for _, id := range idToolTercabutTahap1 {
		_, err := mgr.Configure(ctx, ConfigureParams{
			ToolID: id, Mode: ModeModelOnly, Target: "intern-s2", APIKey: "kunci-uji-123",
		}, "http://localhost:8080/v1")
		if err == nil {
			t.Errorf("Configure %s seharusnya gagal (id tidak dikenal)", id)
			continue
		}
		if !strings.Contains(err.Error(), "tidak didukung") {
			t.Errorf("pesan Configure %s tidak menyatakan tidak didukung: %v", id, err)
		}
		if _, errGet := store.Get(ctx, "cli:config:"+id); errGet == nil {
			t.Errorf("Configure %s yang gagal tetap menyimpan setelan", id)
		}
		if _, ok := cariToolDef(id); ok {
			t.Errorf("deteksi %s seharusnya tidak mengenal id tercabut", id)
		}
	}
	if _, err := os.Stat(filepath.Join(home, ".routex", "key.env")); !os.IsNotExist(err) {
		t.Errorf("key.env tidak boleh ditulis oleh Configure id tercabut")
	}
}

// TestScanMembersihkanPreferensiToolTercabut membuktikan penanganan
// jujur data preferensi tersimpan: store yang memuat campuran id
// tercabut dan id bertahan termuat TANPA error lewat Scan, entri
// tercabut dibersihkan dari store, dan entri bertahan utuh persis
// (mode/targetnya tetap terbaca pada hasil Scan).
func TestScanMembersihkanPreferensiToolTercabut(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	ctx := context.Background()
	store := newMockSettingsStore()

	simpan := func(id, mode, target string) {
		raw, err := json.Marshal(StoredConfig{Mode: mode, Target: target, UpdatedAt: time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)})
		if err != nil {
			t.Fatal(err)
		}
		store.data["cli:config:"+id] = identity.Setting{Key: "cli:config:" + id, Value: raw}
	}
	// Campuran: dua id bertahan + tiga id tercabut.
	simpan("claude", ModeRouting, "rule-claude-utuh")
	simpan("goose", ModeModelOnly, "model-goose-utuh")
	simpan("sgpt", ModeModelOnly, "model-sgpt-lama")
	simpan("agy", ModeModelOnly, "model-agy-lama")
	simpan("k8sgpt", ModeCombo, "combo-k8s-lama")

	mgr := NewManager(store, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	statuses, err := mgr.Scan(ctx, "http://localhost:8080/v1")
	if err != nil {
		t.Fatalf("Scan dengan preferensi campuran gagal: %v", err)
	}
	if len(statuses) != len(SupportedTools()) {
		t.Fatalf("Scan mengembalikan %d status, mau %d (seluruh registri)", len(statuses), len(SupportedTools()))
	}
	for _, st := range statuses {
		switch st.ID {
		case "claude":
			if st.ActiveMode != ModeRouting || st.ActiveTarget != "rule-claude-utuh" {
				t.Errorf("preferensi claude rusak sesudah pembersihan: %+v", st)
			}
		case "goose":
			if st.ActiveTarget != "model-goose-utuh" {
				t.Errorf("preferensi goose rusak sesudah pembersihan: %+v", st)
			}
		}
	}
	// Id tercabut dibersihkan dari store; id bertahan TIDAK tersentuh.
	for _, id := range []string{"sgpt", "agy", "k8sgpt"} {
		if _, errGet := store.Get(ctx, "cli:config:"+id); errGet == nil {
			t.Errorf("preferensi tercabut %s seharusnya dibersihkan dari store", id)
		}
	}
	for _, id := range []string{"claude", "goose"} {
		setting, errGet := store.Get(ctx, "cli:config:"+id)
		if errGet != nil {
			t.Fatalf("preferensi bertahan %s hilang dari store: %v", id, errGet)
		}
		var stored StoredConfig
		if err := json.Unmarshal(setting.Value, &stored); err != nil {
			t.Fatalf("preferensi bertahan %s tidak lagi terbaca: %v", id, err)
		}
		if stored.Target == "" {
			t.Errorf("preferensi bertahan %s kehilangan targetnya", id)
		}
	}
	// Pemuatan kunci gabungan untuk key.env juga melewati id
	// tercabut tanpa error dan tanpa menyumbang kunci mereka.
	konten, err := susunKontenKeyEnv(map[string]kunciTersimpan{
		"claude": {kunci: "kunci-claude", diperbarui: time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)},
		"sgpt":   {kunci: "kunci-sgpt", diperbarui: time.Date(2026, 11, 1, 11, 0, 0, 0, time.UTC)},
	})
	if err != nil {
		t.Fatalf("susunKontenKeyEnv campuran: %v", err)
	}
	if strings.Contains(konten, "kunci-sgpt") {
		t.Errorf("kunci tool tercabut masih disumbangkan ke key.env:\n%s", konten)
	}
	if !strings.Contains(konten, "kunci-claude") {
		t.Errorf("kunci tool bertahan hilang dari key.env:\n%s", konten)
	}
}

// tulisBinerPalsu menulis skrip executable kecil sebagai pengganti
// biner tool untuk test deteksi versi.
func tulisBinerPalsu(t *testing.T, nama, isi string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), nama)
	if err := os.WriteFile(p, []byte(isi), 0755); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestDeteksiVersiToolBertahan membuktikan jalur deteksi versi tool
// registri kini tetap bekerja apa adanya.
func TestDeteksiVersiToolBertahan(t *testing.T) {
	def, _ := cariToolDef("claude")
	bin := tulisBinerPalsu(t, "claude", "#!/bin/sh\necho '2.1.0 (Claude Code)'\n")
	mgr := NewManager(newMockSettingsStore(), nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if got := mgr.detectVersionUntukTool(context.Background(), def, bin); got != "2.1.0 (Claude Code)" {
		t.Errorf("versi claude = %q, mau %q", got, "2.1.0 (Claude Code)")
	}
}

// TestDeteksiVersiBinerLambatTerbaca membuktikan B-148: biner yang
// menjawab versi SESUDAH batas lama 1500 ms (di sini 2 detik) kini
// terbaca versinya.
func TestDeteksiVersiBinerLambatTerbaca(t *testing.T) {
	def, ok := cariToolDef("claude")
	if !ok {
		t.Fatal("claude tidak ditemukan")
	}
	bin := tulisBinerPalsu(t, "claude", "#!/bin/sh\nsleep 2\necho '2.1.0 (Claude Code)'\n")
	mgr := NewManager(newMockSettingsStore(), nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	mulai := time.Now()
	got := mgr.detectVersionUntukTool(context.Background(), def, bin)
	lama := time.Since(mulai)
	if got != "2.1.0 (Claude Code)" {
		t.Errorf("versi biner lambat (2 dtk) = %q, mau terbaca (batas lama 1500 ms akan mengosongkannya, B-148)", got)
	}
	if lama < 2*time.Second {
		t.Errorf("deteksi selesai dalam %v — biner lambat tidak benar-benar ditunggu", lama)
	}
}

// TestDeteksiVersiBinerMenggantungTerpotong membuktikan sisi aman
// B-148: biner yang menggantung tetap terpotong pada batas
// timeoutDeteksiVersi = 5 detik, bukan menggantung tanpa batas.
func TestDeteksiVersiBinerMenggantungTerpotong(t *testing.T) {
	if timeoutDeteksiVersi != 5*time.Second {
		t.Fatalf("timeoutDeteksiVersi = %v, mau 5 detik (B-148)", timeoutDeteksiVersi)
	}
	def, _ := cariToolDef("claude")
	bin := tulisBinerPalsu(t, "claude", "#!/bin/sh\nexec sleep 30\n")
	mgr := NewManager(newMockSettingsStore(), nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	mulai := time.Now()
	got := mgr.detectVersionUntukTool(context.Background(), def, bin)
	lama := time.Since(mulai)
	if got != "" {
		t.Errorf("versi biner menggantung = %q, mau kosong", got)
	}
	if lama < timeoutDeteksiVersi-time.Second {
		t.Errorf("deteksi terpotong terlalu dini dalam %v (batas %v)", lama, timeoutDeteksiVersi)
	}
	if lama > timeoutDeteksiVersi+10*time.Second {
		t.Errorf("deteksi biner menggantung memakan %v — batas %v tidak memotongnya", lama, timeoutDeteksiVersi)
	}
}
