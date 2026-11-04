package cliconfig

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestConfigureOpenCommitAnggaranKoheren membuktikan B-147 (regresi
// B-144): ketiga variabel token OpenCommit yang ditulis Configure
// harus membentuk anggaran yang KOHEREN menurut rumus tool 3.3.11,
// bukan sekadar hadir. Rumusnya (generateCommitMessageByDiff):
//
//	anggaran request = OCO_TOKENS_MAX_INPUT − 20 − prompt awal − maxOutput
//
// Dengan input bawaan tool 4096 dan keluaran 4000 hasilnya negatif →
// oco gagal lokal pada SETIAP commit ("Token limit exceeded…" dari
// splitDiff, tampil ke pengguna sebagai "Insufficient Credits").
// Test ini menguraikan angka dari EnvVars DAN dari berkas
// ~/.opencommit (keduanya harus sama persis), lalu memeriksa untuk
// KEDUA plafon (keluaran & reasoning): input − 20 − margin prompt
// awal − plafon menyisakan ruang diff yang positif dan wajar.
func TestConfigureOpenCommitAnggaranKoheren(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	mgr := NewManager(newMockSettingsStore(), nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	res, err := mgr.Configure(context.Background(), ConfigureParams{
		ToolID: "opencommit", Mode: ModeModelOnly, Target: "intern-s2", APIKey: "kunci-oco-123",
	}, "http://localhost:8080/v1")
	if err != nil {
		t.Fatalf("Configure opencommit: %v", err)
	}

	urai := func(sumber, nama string) int {
		t.Helper()
		v, err := strconv.Atoi(sumber)
		if err != nil || v <= 0 {
			t.Fatalf("%s = %q, mau bilangan positif", nama, sumber)
		}
		return v
	}
	maxInput := urai(res.EnvVars["OCO_TOKENS_MAX_INPUT"], "EnvVars OCO_TOKENS_MAX_INPUT")
	maxOutput := urai(res.EnvVars["OCO_TOKENS_MAX_OUTPUT"], "EnvVars OCO_TOKENS_MAX_OUTPUT")
	maxReasoning := urai(res.EnvVars["OCO_REASONING_MAX_TOKENS"], "EnvVars OCO_REASONING_MAX_TOKENS")

	// Nilai persis yang diputuskan (lihat konstanta oco* di manager.go):
	// input 16384 = kontrafaktual terverifikasi live Putaran 2;
	// keluaran/reasoning 4000 = dasar B-144 yang tidak berubah.
	if maxInput != 16384 || maxOutput != 4000 || maxReasoning != 4000 {
		t.Errorf("anggaran EnvVars = input %d / output %d / reasoning %d, mau 16384/4000/4000",
			maxInput, maxOutput, maxReasoning)
	}

	// Ketiga variabel juga tertulis di berkas ~/.opencommit dengan
	// nilai yang SAMA dengan EnvVars (dua permukaan, satu anggaran).
	data, err := os.ReadFile(filepath.Join(home, ".opencommit"))
	if err != nil {
		t.Fatalf(".opencommit tidak ditulis: %v", err)
	}
	berkas := make(map[string]string)
	for _, baris := range strings.Split(string(data), "\n") {
		if k, v, ok := strings.Cut(baris, "="); ok {
			berkas[k] = v
		}
	}
	for _, nama := range []string{"OCO_TOKENS_MAX_INPUT", "OCO_TOKENS_MAX_OUTPUT", "OCO_REASONING_MAX_TOKENS"} {
		if berkas[nama] == "" {
			t.Errorf(".opencommit tidak memuat %s:\n%s", nama, data)
		} else if berkas[nama] != res.EnvVars[nama] {
			t.Errorf("%s berkas = %q, EnvVars = %q — harus sama", nama, berkas[nama], res.EnvVars[nama])
		}
	}

	// Koherensi: margin prompt awal 1024 token adalah taksiran
	// konservatif (request oco teramati live: 508 token input total;
	// prompt awal tool orde ratusan token) — jauh di atas kebutuhan
	// nyata, supaya test ini gagal bila ada yang menurunkan input
	// sampai anggaran kembali sempit, bukan hanya bila negatif.
	const overheadRumus = 20 // konstanta rumus tool: MAX_INPUT − 20 − …
	const marginPromptAwal = 1024
	for nama, plafon := range map[string]int{"keluaran": maxOutput, "reasoning": maxReasoning} {
		sisa := maxInput - overheadRumus - marginPromptAwal - plafon
		if sisa <= 0 {
			t.Errorf("anggaran %s tidak koheren: %d − %d − %d − %d = %d (harus positif; pada rumus tool nilai <= 0 = oco gagal lokal, B-147)",
				nama, maxInput, overheadRumus, marginPromptAwal, plafon, sisa)
		}
		// Ruang diff wajar: sisa harus sekurang-kurangnya sebesar
		// plafon keluaran itu sendiri — diff commit yang wajar jauh
		// lebih besar dari pesan commit yang dihasilkan.
		if sisa < plafon {
			t.Errorf("ruang diff jalur %s sempit: sisa %d < plafon %d", nama, sisa, plafon)
		}
	}
}

// TestDeteksiVersiBinerLambatTerbaca membuktikan B-148: biner yang
// menjawab versi SESUDAH batas lama 1500 ms (di sini 2 detik, meniru
// startup sgpt terukur 1,66–1,68 detik) kini terbaca versinya.
func TestDeteksiVersiBinerLambatTerbaca(t *testing.T) {
	def, ok := cariToolDef("llm")
	if !ok {
		t.Fatal("llm tidak ditemukan")
	}
	bin := tulisBinerPalsu(t, "llm", "#!/bin/sh\nsleep 2\necho 'llm, version 0.37.1'\n")
	mgr := NewManager(newMockSettingsStore(), nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	mulai := time.Now()
	got := mgr.detectVersionUntukTool(context.Background(), def, bin)
	lama := time.Since(mulai)
	if got != "llm, version 0.37.1" {
		t.Errorf("versi biner lambat (2 dtk) = %q, mau %q (batas lama 1500 ms akan mengosongkannya, B-148)", got, "llm, version 0.37.1")
	}
	if lama < 2*time.Second {
		t.Errorf("deteksi selesai dalam %v — biner lambat tidak benar-benar ditunggu", lama)
	}
}

// TestDeteksiVersiBinerMenggantungTerpotong membuktikan sisi aman
// B-148: biner yang menggantung tetap terpotong pada batas baru
// (timeoutDeteksiVersi = 5 detik), bukan menggantung tanpa batas.
// Biner memakai `exec sleep` supaya proses yang dibunuh adalah
// proses pemegang stdout itu sendiri (tanpa exec, cucu sleep
// mewarisi pipa stdout dan cmd.Run menunggu pipanya).
func TestDeteksiVersiBinerMenggantungTerpotong(t *testing.T) {
	if timeoutDeteksiVersi != 5*time.Second {
		t.Fatalf("timeoutDeteksiVersi = %v, mau 5 detik (B-148)", timeoutDeteksiVersi)
	}
	def, _ := cariToolDef("llm")
	bin := tulisBinerPalsu(t, "llm", "#!/bin/sh\nexec sleep 30\n")
	mgr := NewManager(newMockSettingsStore(), nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	mulai := time.Now()
	got := mgr.detectVersionUntukTool(context.Background(), def, bin)
	lama := time.Since(mulai)
	if got != "" {
		t.Errorf("versi biner menggantung = %q, mau kosong", got)
	}
	if lama < timeoutDeteksiVersi-time.Second {
		t.Errorf("deteksi terpotong terlalu dini dalam %v (batas %v) — batas baru tidak benar-benar berlaku", lama, timeoutDeteksiVersi)
	}
	if lama > timeoutDeteksiVersi+10*time.Second {
		t.Errorf("deteksi biner menggantung memakan %v — batas %v tidak memotongnya", lama, timeoutDeteksiVersi)
	}
}

// TestChatbladeDeteksiVersiKembali membuktikan B-146: VersionArg
// chatblade KEMBALI ke "--version" (membalik pencabutan B-145(b)
// untuk entri ini saja) berdasarkan bukti Uji Live Putaran 2 — build
// resmi chatblade 0.7.0 (pip, venv Python 3.13) menjalankan
// `chatblade --version` dengan exit 0 dan stdout persis
// "chatblade 0.7.0"; build Putaran 1 yang menolak flag itu terbukti
// bukan rilis resmi. Biner palsu di sini mencetak persis keluaran
// build resmi itu dan harus terbaca oleh parser versi produk, dan
// binernya harus benar-benar dieksekusi (kebalikan asersi B-145(b)).
func TestChatbladeDeteksiVersiKembali(t *testing.T) {
	def, ok := cariToolDef("chatblade")
	if !ok {
		t.Fatal("chatblade tidak ditemukan")
	}
	if def.VersionArg != "--version" {
		t.Fatalf("VersionArg chatblade = %q, mau --version (B-146)", def.VersionArg)
	}
	penanda := filepath.Join(t.TempDir(), "pernah-dijalankan")
	bin := tulisBinerPalsu(t, "chatblade", "#!/bin/sh\necho ran > "+penanda+"\necho 'chatblade 0.7.0'\n")
	mgr := NewManager(newMockSettingsStore(), nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if got := mgr.detectVersionUntukTool(context.Background(), def, bin); got != "chatblade 0.7.0" {
		t.Errorf("versi chatblade = %q, mau %q (keluaran persis build resmi 0.7.0, B-146)", got, "chatblade 0.7.0")
	}
	if _, err := os.Stat(penanda); err != nil {
		t.Errorf("biner chatblade harus dieksekusi untuk deteksi versi (B-146): %v", err)
	}
}
