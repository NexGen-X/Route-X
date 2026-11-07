package cliconfig

// fase24_prb_test.go — pengunci Fase 24 PR-B (cliconfig: penanda
// akhir dokumen & penyunting YAML):
//
//	B-543 isi non-kosong sesudah "..." tanpa "---" di config
//	      Hermes ditolak jujur (dokumen tambahan — penghitung
//	      dokumen lama GALAT atas semua bentuk ini sehingga
//	      penjaganya diam; PyYAML safe_load menolaknya juga,
//	      terbukti dengan pustaka sungguhan).
//	B-544 penanda "...\t" (pemisah tab ke komentar) DINORMALISASI
//	      ke bentuk kanonik "... # komentar" saat menulis —
//	      PyYAML menolak bentuk tab mentah (ScannerError,
//	      terbukti); toleransi serde_yaml Goose tidak diklaim,
//	      normalisasi berlaku seragam dari penyunting bersama.
//	B-550 penyunting baris melacak skalar terkutip multiline:
//	      baris isi skalar yang menyerupai kunci terkelola/header
//	      blok model diteruskan utuh, tidak dibaca sebagai
//	      struktur (keputusan per tool: Goose & Aider lewat
//	      pelacak di perbaruiYAMLTopLevel; Hermes lewat peta
//	      skalar di susunBlokModelHermes + saringIsiBlokHermes;
//	      skalar blok |/> tidak dilacak — isinya terikat
//	      indentasi).
//	B-551 jangkar pada baris kunci terkelola Goose dipertahankan
//	      saat baris itu diganti — termasuk jangkar yang hanya
//	      ada di duplikat yang dibuang — supaya alias *nama tetap
//	      terurai (preseden Hermes B-481/B-506).
//	B-552 sisipan blok Hermes pada berkas berpenanda
//	      byte-idempoten: lari kedua tidak menambah baris kosong
//	      pemisah di atas penanda.
//
// Semua test memanggil applyToolSpecificConfig — jalur Configure
// sungguhan — lewat pembantu terapkanPRD.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestB543HermesIsiSesudahPenandaDitolak mengunci B-543 atas
// bentuk-bentuk yang terbukti lolos sebelum perbaikan (Configure
// sukses + berkas tertulis + hasil tidak terbaca PyYAML): blok
// model lama, kunci lain, flow map, dan varian CRLF sesudah
// penanda. Penolakan terjadi sebelum berkas apa pun ditulis.
func TestB543HermesIsiSesudahPenandaDitolak(t *testing.T) {
	for _, tc := range []struct {
		nama string
		isi  string
	}{
		{"blok model lama sesudah penanda", "lainnya: 1\n...\nmodel:\n  default: palsu\n"},
		{"kunci lain sesudah penanda", "lainnya: 1\n...\nlainnya2: 2\n"},
		{"flow map sesudah penanda", "lainnya: 1\n...\n{a: b}\n"},
		{"CRLF blok sesudah penanda", "lainnya: 1\r\n...\r\nmodel:\r\n  default: palsu\r\n"},
	} {
		t.Run(tc.nama, func(t *testing.T) {
			home := t.TempDir()
			jalur := filepath.Join(home, ".hermes", "config.yaml")
			tulisBerkasPRD(t, jalur, []byte(tc.isi))

			err := terapkanPRD(t, home, "hermes", "")
			if err == nil {
				t.Fatalf("applyToolSpecificConfig hermes melaporkan sukses atas config ber-isi sesudah penanda (%s)", tc.nama)
			}
			if !strings.Contains(err.Error(), "sesudah penanda akhir dokumen") {
				t.Errorf("pesan galat tidak menamai isi sesudah penanda: %v", err)
			}
			if got := bacaBerkasUji(t, jalur); got != tc.isi {
				t.Errorf("config berubah sesudah penolakan jujur:\n%q", got)
			}
		})
	}
}

// TestB543HermesKomentarSesudahPenandaLolos adalah kontrol arah
// sebaliknya: baris kosong dan komentar sesudah penanda SAH
// (PyYAML menerimanya, terbukti) — Configure berhasil dan blok
// model efektif di dalam dokumen.
func TestB543HermesKomentarSesudahPenandaLolos(t *testing.T) {
	home := t.TempDir()
	jalur := filepath.Join(home, ".hermes", "config.yaml")
	tulisBerkasPRD(t, jalur, []byte("lainnya: 1\n...\n# catatan penutup\n\n"))

	if err := terapkanPRD(t, home, "hermes", ""); err != nil {
		t.Fatalf("applyToolSpecificConfig hermes menolak komentar sesudah penanda: %v", err)
	}
	konten := bacaBerkasUji(t, jalur)
	doc, err := uraiYAMLUtuh(konten)
	if err != nil {
		t.Fatalf("hasil sunting tidak terurai utuh: %v\n%s", err, konten)
	}
	blok, ok := doc["model"].(map[string]any)
	if !ok || blok["default"] != "model-baru" {
		t.Errorf("blok model tidak efektif di dalam dokumen: %v\n%s", doc["model"], konten)
	}
	if !strings.Contains(konten, "# catatan penutup") {
		t.Errorf("komentar sesudah penanda hilang:\n%s", konten)
	}
}

// TestB544PenandaTabDinormalisasi mengunci B-544 untuk
// Hermes DAN penyunting bersama (Goose): penanda berpenanda-tab
// "...\t# selesai" berhasil di-Configure dan baris penanda pada
// hasil tertulis dalam bentuk kanonik "... # selesai" — bukan
// bentuk tab yang ditolak PyYAML mentah-mentah.
func TestB544PenandaTabDinormalisasi(t *testing.T) {
	t.Run("hermes", func(t *testing.T) {
		home := t.TempDir()
		jalur := filepath.Join(home, ".hermes", "config.yaml")
		tulisBerkasPRD(t, jalur, []byte("lainnya: 1\n...\t# selesai\n"))

		if err := terapkanPRD(t, home, "hermes", ""); err != nil {
			t.Fatalf("applyToolSpecificConfig hermes atas penanda tab: %v", err)
		}
		konten := bacaBerkasUji(t, jalur)
		if !strings.Contains(konten, "... # selesai") {
			t.Errorf("penanda tidak ternormalisasi ke bentuk kanonik:\n%q", konten)
		}
		if strings.Contains(konten, "...\t") {
			t.Errorf("bentuk penanda tab masih tertinggal:\n%q", konten)
		}
		// Penanda berkomentar tidak lolos pemeriksaan baris
		// terakhir periksaHermesPenandaB536 (ia mengunci bentuk
		// polos B-536) — di sini keefektifan dokumen diperiksa
		// langsung: satu dokumen, blok model di dalamnya.
		doc, err := uraiYAMLUtuh(konten)
		if err != nil {
			t.Fatalf("hasil sunting tidak terurai utuh: %v\n%s", err, konten)
		}
		blok, ok := doc["model"].(map[string]any)
		if !ok || blok["default"] != "model-baru" || doc["lainnya"] != 1 {
			t.Errorf("dokumen hasil tidak efektif: %v\n%s", doc, konten)
		}
	})

	t.Run("goose", func(t *testing.T) {
		home := t.TempDir()
		jalur := filepath.Join(home, ".config", "goose", "config.yaml")
		tulisBerkasPRD(t, jalur, []byte("lainnya: 1\n...\t# selesai\n"))

		if err := terapkanPRD(t, home, "goose", ""); err != nil {
			t.Fatalf("applyToolSpecificConfig goose atas penanda tab: %v", err)
		}
		konten := bacaBerkasUji(t, jalur)
		if !strings.Contains(konten, "... # selesai") {
			t.Errorf("penanda tidak ternormalisasi ke bentuk kanonik:\n%q", konten)
		}
		doc, err := uraiYAMLUtuh(konten)
		if err != nil {
			t.Fatalf("hasil sunting tidak terurai utuh: %v\n%s", err, konten)
		}
		if doc["GOOSE_MODEL"] != "model-baru" || doc["lainnya"] != 1 {
			t.Errorf("dokumen hasil tidak efektif: %v\n%s", doc, konten)
		}
	})

	t.Run("hermes dengan blok model", func(t *testing.T) {
		home := t.TempDir()
		jalur := filepath.Join(home, ".hermes", "config.yaml")
		tulisBerkasPRD(t, jalur, []byte("model:\n  default: lama\nlainnya: 1\n...\t# selesai\n"))

		if err := terapkanPRD(t, home, "hermes", ""); err != nil {
			t.Fatalf("applyToolSpecificConfig hermes atas penanda tab dengan blok: %v", err)
		}
		konten := bacaBerkasUji(t, jalur)
		if !strings.Contains(konten, "... # selesai") || strings.Contains(konten, "...\t") {
			t.Errorf("penanda tidak ternormalisasi pada jalur gabung blok:\n%q", konten)
		}
		doc, err := uraiYAMLUtuh(konten)
		if err != nil {
			t.Fatalf("hasil sunting tidak terurai utuh: %v\n%s", err, konten)
		}
		blok, ok := doc["model"].(map[string]any)
		if !ok || blok["default"] != "model-baru" {
			t.Errorf("blok model tidak diperbarui di dalam dokumen: %v\n%s", doc["model"], konten)
		}
	})
}

// TestB550HermesSkalarMultilineBukanBlokModel mengunci B-550
// untuk Hermes: baris "model:" DI DALAM skalar terkutip multiline
// milik kunci lain bukan header blok — blok model sungguhan
// ditambahkan di dokumen, skalar pengguna utuh, dan hasil terurai
// sebagai satu dokumen dengan nilai yang benar (bentuk ini sah
// menurut PyYAML, terbukti dengan pustaka sungguhan; sebelum
// perbaikan komposer menulis di tengah skalar dan verifikasi
// menolaknya — Configure mustahil atas config yang sah).
func TestB550HermesSkalarMultilineBukanBlokModel(t *testing.T) {
	home := t.TempDir()
	jalur := filepath.Join(home, ".hermes", "config.yaml")
	tulisBerkasPRD(t, jalur, []byte("catatan: \"awal\nmodel:\n default: palsu\"\nlainnya: 1\n"))

	if err := terapkanPRD(t, home, "hermes", ""); err != nil {
		t.Fatalf("applyToolSpecificConfig hermes atas skalar multiline berisi 'model:': %v", err)
	}
	konten := bacaBerkasUji(t, jalur)
	doc, err := uraiYAMLUtuh(konten)
	if err != nil {
		t.Fatalf("hasil sunting tidak terurai utuh: %v\n%s", err, konten)
	}
	if doc["catatan"] != "awal model: default: palsu" {
		t.Errorf("skalar multiline pengguna rusak: %v\n%s", doc["catatan"], konten)
	}
	blok, ok := doc["model"].(map[string]any)
	if !ok || blok["default"] != "model-baru" {
		t.Errorf("blok model tidak efektif: %v\n%s", doc["model"], konten)
	}
}

// TestB550HermesSkalarDiDalamBlokModel mengunci sisi kedua
// B-550 Hermes: skalar multiline DI DALAM blok model yang isinya
// memuat baris kolom-0 — rentang blok tidak berhenti di baris isi
// itu, dan sub-kunci tak-terkelola pemilik skalar dipertahankan
// utuh sementara sub-kunci terkelola sesudahnya tetap diperbarui.
func TestB550HermesSkalarDiDalamBlokModel(t *testing.T) {
	home := t.TempDir()
	jalur := filepath.Join(home, ".hermes", "config.yaml")
	tulisBerkasPRD(t, jalur, []byte("model:\n  catatan: \"awal\ntengah kolom nol\"\n  default: lama\nlainnya: 1\n"))

	if err := terapkanPRD(t, home, "hermes", ""); err != nil {
		t.Fatalf("applyToolSpecificConfig hermes atas skalar di dalam blok: %v", err)
	}
	konten := bacaBerkasUji(t, jalur)
	doc, err := uraiYAMLUtuh(konten)
	if err != nil {
		t.Fatalf("hasil sunting tidak terurai utuh: %v\n%s", err, konten)
	}
	blok, ok := doc["model"].(map[string]any)
	if !ok {
		t.Fatalf("blok model hilang: %v\n%s", doc, konten)
	}
	if blok["default"] != "model-baru" {
		t.Errorf("sub-kunci terkelola sesudah skalar tidak diperbarui: %v\n%s", blok, konten)
	}
	if blok["catatan"] != "awal tengah kolom nol" {
		t.Errorf("skalar di dalam blok rusak: %v\n%s", blok["catatan"], konten)
	}
	if doc["lainnya"] != 1 {
		t.Errorf("kunci sesudah blok rusak: %v\n%s", doc, konten)
	}
}

// TestB550GooseSkalarMultilineBukanKunci mengunci B-550 untuk
// penyunting bersama (Goose): baris yang menyerupai kunci
// terkelola DI DALAM skalar terkutip multiline bukan kunci —
// skalar utuh sesudah Configure dan kunci terkelola sungguhan
// tertulis satu kali.
func TestB550GooseSkalarMultilineBukanKunci(t *testing.T) {
	home := t.TempDir()
	jalur := filepath.Join(home, ".config", "goose", "config.yaml")
	tulisBerkasPRD(t, jalur, []byte("catatan: \"awal\nGOOSE_MODEL: palsu\"\nlainnya: 1\n"))

	if err := terapkanPRD(t, home, "goose", ""); err != nil {
		t.Fatalf("applyToolSpecificConfig goose atas skalar multiline berisi kunci semu: %v", err)
	}
	konten := bacaBerkasUji(t, jalur)
	doc, err := uraiYAMLUtuh(konten)
	if err != nil {
		t.Fatalf("hasil sunting tidak terurai utuh: %v\n%s", err, konten)
	}
	if doc["catatan"] != "awal GOOSE_MODEL: palsu" {
		t.Errorf("skalar multiline pengguna rusak: %v\n%s", doc["catatan"], konten)
	}
	if doc["GOOSE_MODEL"] != "model-baru" {
		t.Errorf("kunci terkelola tidak tertulis: %v\n%s", doc, konten)
	}
	if n := strings.Count(konten, "GOOSE_MODEL:"); n != 2 {
		t.Errorf("kemunculan 'GOOSE_MODEL:' = %d, mau 2 (satu di dalam skalar, satu terkelola):\n%s", n, konten)
	}
}

// TestB551GooseJangkarDipertahankan mengunci B-551: jangkar pada
// baris terkelola Goose bertahan saat baris diganti, sehingga
// alias *m tetap terurai — bentuk pertama menaruh jangkar di
// baris yang diganti, bentuk kedua HANYA di duplikat yang dibuang
// (jangkar dipanen dari kemunculan mana pun).
func TestB551GooseJangkarDipertahankan(t *testing.T) {
	for _, tc := range []struct {
		nama string
		isi  string
	}{
		{"jangkar di baris yang diganti",
			"GOOSE_MODEL: &m model-lama\npilihan: *m\n"},
		{"jangkar hanya di duplikat yang dibuang",
			"GOOSE_MODEL: model-lama\nGOOSE_MODEL: &m model-duplikat\npilihan: *m\n"},
	} {
		t.Run(tc.nama, func(t *testing.T) {
			home := t.TempDir()
			jalur := filepath.Join(home, ".config", "goose", "config.yaml")
			tulisBerkasPRD(t, jalur, []byte(tc.isi))

			if err := terapkanPRD(t, home, "goose", ""); err != nil {
				t.Fatalf("applyToolSpecificConfig goose atas config berjangkar (%s): %v", tc.nama, err)
			}
			konten := bacaBerkasUji(t, jalur)
			if !strings.Contains(konten, "GOOSE_MODEL: &m") {
				t.Errorf("jangkar &m hilang dari baris terkelola:\n%s", konten)
			}
			doc, err := uraiYAMLUtuh(konten)
			if err != nil {
				t.Fatalf("hasil sunting tidak terurai utuh (alias menggantung?): %v\n%s", err, konten)
			}
			if doc["GOOSE_MODEL"] != "model-baru" || doc["pilihan"] != "model-baru" {
				t.Errorf("nilai terkelola/alias tidak sesuai: %v\n%s", doc, konten)
			}
		})
	}
}

// TestB552HermesPenandaByteIdempoten mengunci B-552: Configure
// Hermes atas config berpenanda "..." yang SUDAH memuat blok
// model menghasilkan byte yang sama pada lari kedua dan ketiga
// (sebelum perbaikan lari kedua menyelipkan satu baris kosong
// pemisah di atas penanda — keluaran tidak idempoten).
func TestB552HermesPenandaByteIdempoten(t *testing.T) {
	home := t.TempDir()
	jalur := filepath.Join(home, ".hermes", "config.yaml")
	tulisBerkasPRD(t, jalur, []byte("model:\n  default: lama\nlainnya: 1\n...\n"))

	if err := terapkanPRD(t, home, "hermes", ""); err != nil {
		t.Fatalf("lari pertama: %v", err)
	}
	satu := bacaBerkasUji(t, jalur)
	if err := terapkanPRD(t, home, "hermes", ""); err != nil {
		t.Fatalf("lari kedua: %v", err)
	}
	dua := bacaBerkasUji(t, jalur)
	if err := terapkanPRD(t, home, "hermes", ""); err != nil {
		t.Fatalf("lari ketiga: %v", err)
	}
	tiga := bacaBerkasUji(t, jalur)
	if satu != dua || dua != tiga {
		t.Errorf("Configure atas berkas berpenanda tidak byte-idempoten:\nlari1:\n%q\nlari2:\n%q\nlari3:\n%q", satu, dua, tiga)
	}
	if _, err := os.Lstat(jalur); err != nil {
		t.Fatalf("config hilang: %v", err)
	}
}
