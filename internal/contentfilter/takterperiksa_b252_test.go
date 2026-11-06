package contentfilter

import (
	"context"
	"strings"
	"testing"

	"github.com/NexGen-X/Route-X/internal/database/repo/policy"
)

// B-252: muatan yang tidak bisa diperiksa sebagai teks (masukan embeddings
// berbentuk token) tidak boleh lolos aturan pola hanya karena Text-nya
// kosong — tetapi penolakannya juga hanya boleh terjadi bila memang ada
// aturan pola pemblokir yang berlaku. Test ini mengunci kedua arahnya.

func polaBlokirUji() *policy.ContentFilter {
	return row("f-token", "tanpa kata terlarang", policy.FilterBlockedPattern, func(f *policy.ContentFilter) {
		f.Pattern = "terlarang"
		f.PatternType = policy.PatternSubstring
	})
}

func TestTakTerperiksaDitolakBilaAdaAturanPolaBlokir(t *testing.T) {
	e := NewEngine(&sumber{rows: []*policy.ContentFilter{polaBlokirUji()}}, nil)

	v := e.Check(context.Background(), Subject{TakTerperiksa: true})
	if !v.Blocked {
		t.Fatal("muatan tak terperiksa lolos padahal ada aturan pola blokir aktif")
	}
	// Alasannya harus jujur: ditolak karena tidak bisa diperiksa, bukan
	// karena terbukti memuat pola.
	if !strings.Contains(v.Reason, "tidak dapat diperiksa") {
		t.Errorf("alasan = %q; ingin menyebut tidak dapat diperiksa", v.Reason)
	}
	if v.Filter == nil || v.Filter.ID != "f-token" {
		t.Errorf("filter penolak = %+v; ingin f-token", v.Filter)
	}
}

func TestTakTerperiksaTanpaAturanPolaTetapLolos(t *testing.T) {
	// Tanpa aturan pola, tak terperiksa bukan pelanggaran: aturan ukuran
	// dan sejenisnya tidak bergantung pada teks.
	e := NewEngine(&sumber{rows: []*policy.ContentFilter{
		row("f-ukuran", "batas ukuran", policy.FilterRequestSize, func(f *policy.ContentFilter) {
			f.MaxRequestBytes = pointer(int64(1 << 20))
		}),
	}}, nil)
	if v := e.Check(context.Background(), Subject{TakTerperiksa: true, Bytes: 10}); v.Blocked {
		t.Errorf("muatan tak terperiksa diblokir tanpa aturan pola: %+v", v)
	}

	kosong := NewEngine(&sumber{}, nil)
	if v := kosong.Check(context.Background(), Subject{TakTerperiksa: true}); v.Blocked {
		t.Errorf("muatan tak terperiksa diblokir tanpa aturan apa pun: %+v", v)
	}
}

func TestTakTerperiksaMenghormatiArahDanAksiWarn(t *testing.T) {
	// Aturan yang hanya berlaku untuk jawaban tidak memblokir permintaan.
	e := NewEngine(&sumber{rows: []*policy.ContentFilter{
		row("f-jawaban", "pola jawaban", policy.FilterBlockedPattern, func(f *policy.ContentFilter) {
			f.Pattern = "x"
			f.PatternType = policy.PatternSubstring
			f.AppliesTo = policy.AppliesToResponse
		}),
	}}, nil)
	if v := e.Check(context.Background(), Subject{TakTerperiksa: true}); v.Blocked {
		t.Error("aturan khusus jawaban memblokir permintaan tak terperiksa")
	}
	if v := e.Check(context.Background(), Subject{TakTerperiksa: true, Response: true}); !v.Blocked {
		t.Error("jawaban tak terperiksa lolos aturan pola jawaban")
	}

	// Aturan beraksi peringatan tidak pernah memblokir, juga tidak untuk
	// muatan tak terperiksa.
	e2 := NewEngine(&sumber{rows: []*policy.ContentFilter{
		row("f-warn", "pola peringatan", policy.FilterBlockedPattern, func(f *policy.ContentFilter) {
			f.Pattern = "x"
			f.PatternType = policy.PatternSubstring
			f.Action = policy.ActionWarn
		}),
	}}, nil)
	if v := e2.Check(context.Background(), Subject{TakTerperiksa: true}); v.Blocked {
		t.Error("aturan warn memblokir muatan tak terperiksa")
	}
}

func TestTakterperiksaPolaPutihDanRusak(t *testing.T) {
	// Daftar putih: muatan tak terperiksa tidak bisa membuktikan dirinya
	// termasuk yang diizinkan, jadi ditolak.
	e := NewEngine(&sumber{rows: []*policy.ContentFilter{
		row("f-putih", "hanya yang diizinkan", policy.FilterAllowedPattern, func(f *policy.ContentFilter) {
			f.Pattern = "diizinkan"
			f.PatternType = policy.PatternSubstring
		}),
	}}, nil)
	if v := e.Check(context.Background(), Subject{TakTerperiksa: true}); !v.Blocked {
		t.Error("muatan tak terperiksa lolos daftar putih")
	}

	// Daftar hitam berpola regex rusak meloloskan menurut desain terpicu()
	// (satu pola salah tulis tidak mematikan lalu lintas) — jalur
	// tak-terperiksa tidak boleh diam-diam lebih keras dari desain itu.
	e2 := NewEngine(&sumber{rows: []*policy.ContentFilter{
		row("f-rusak", "pola rusak", policy.FilterBlockedPattern, func(f *policy.ContentFilter) {
			f.Pattern = "(["
			f.PatternType = policy.PatternRegex
		}),
	}}, nil)
	if v := e2.Check(context.Background(), Subject{TakTerperiksa: true}); v.Blocked {
		t.Error("daftar hitam rusak memblokir muatan tak terperiksa, menyalahi desain fail-open pola rusak")
	}
}

func TestTanpaTandaTakTerperiksaPerilakuLama(t *testing.T) {
	// Subject teks kosong TANPA tanda tak terperiksa tetap dilewati
	// aturan pola seperti sebelumnya — pemeriksaan bertahap (mis. subjek
	// khusus model) bergantung pada kelonggaran ini.
	e := NewEngine(&sumber{rows: []*policy.ContentFilter{polaBlokirUji()}}, nil)
	if v := e.Check(context.Background(), Subject{}); v.Blocked {
		t.Error("Subject kosong tanpa tanda tak terperiksa diblokir; perilaku bertahap berubah")
	}
	// Dan teks yang memang memuat pola tetap tertangkap jalur biasa.
	if v := e.Check(context.Background(), Subject{Text: "ini terlarang"}); !v.Blocked {
		t.Error("teks berpola terlarang lolos")
	}
}
