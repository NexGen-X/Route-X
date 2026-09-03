package contentfilter

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/NexGen-X/Route-X/internal/database/repo/policy"
)

// sumber adalah Source tiruan.
type sumber struct {
	mu       sync.Mutex
	rows     []*policy.ContentFilter
	err      error
	dibaca   int
	timeouts []string
}

func (s *sumber) ActiveFilters(context.Context) ([]*policy.ContentFilter, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dibaca++
	if s.err != nil {
		return nil, s.err
	}
	return s.rows, nil
}

func (s *sumber) RecordEvalTimeout(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.timeouts = append(s.timeouts, id)
	return nil
}

func (s *sumber) jumlahDibaca() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.dibaca
}

func (s *sumber) jumlahTimeout() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.timeouts)
}

func pointer[T any](v T) *T { return &v }

// row menyusun satu baris content_filters dengan bawaan yang sama seperti database.
func row(id, name, kind string, ubah func(*policy.ContentFilter)) *policy.ContentFilter {
	f := &policy.ContentFilter{
		ID: id, Name: name, Kind: kind,
		Priority: 100, AppliesTo: policy.AppliesToRequest, Action: policy.ActionBlock,
		MaxEvalMS: 50, Enabled: true,
	}
	if ubah != nil {
		ubah(f)
	}
	return f
}

// --- Batas ukuran ------------------------------------------------------------

func TestRequestSize(t *testing.T) {
	src := &sumber{rows: []*policy.ContentFilter{
		row("f1", "batas 1 KiB", policy.FilterRequestSize, func(f *policy.ContentFilter) {
			f.MaxRequestBytes = pointer(int64(1024))
		}),
	}}
	e := NewEngine(src, nil)

	if v := e.Check(context.Background(), Subject{Bytes: 1025, Text: "apa pun"}); !v.Blocked {
		t.Error("body 1025 byte lolos batas 1024")
	}
	if v := e.Check(context.Background(), Subject{Bytes: 1024, Text: "apa pun"}); v.Blocked {
		t.Error("body tepat 1024 byte diblokir batas 1024")
	}
	// Pemanggil yang belum tahu ukurannya melewatkan aturan ini, bukan diblokir olehnya.
	if v := e.Check(context.Background(), Subject{Text: "apa pun"}); v.Blocked {
		t.Error("Subject tanpa Bytes diblokir aturan ukuran")
	}
}

// --- Pola --------------------------------------------------------------------

func TestPolaTerlarang(t *testing.T) {
	src := &sumber{rows: []*policy.ContentFilter{
		row("f1", "tanpa rahasia", policy.FilterBlockedPattern, func(f *policy.ContentFilter) {
			f.Pattern = "kartu kredit"
			f.PatternType = policy.PatternSubstring
		}),
	}}
	e := NewEngine(src, nil)

	v := e.Check(context.Background(), Subject{Text: "tolong simpan Kartu Kredit saya"})
	if !v.Blocked {
		t.Fatal("pola terlarang tidak memblokir")
	}
	// Bawaan case_sensitive false, jadi beda besar-kecil tetap tertangkap.
	if v.Filter.ID != "f1" {
		t.Errorf("aturan pemicu = %s", v.Filter.ID)
	}
	// Pesan penolakan menyebut NAMA kebijakan, tidak pernah polanya maupun isi permintaan.
	if !strings.Contains(v.Reason, "tanpa rahasia") {
		t.Errorf("alasan tidak menyebut nama kebijakan: %q", v.Reason)
	}
	if strings.Contains(v.Reason, "kartu kredit") || strings.Contains(v.Reason, "simpan") {
		t.Errorf("alasan membocorkan pola atau isi permintaan: %q", v.Reason)
	}

	if v := e.Check(context.Background(), Subject{Text: "cuaca hari ini"}); v.Blocked {
		t.Error("teks tanpa pola terlarang ikut diblokir")
	}
}

func TestPolaPekaBesarKecil(t *testing.T) {
	src := &sumber{rows: []*policy.ContentFilter{
		row("f1", "peka", policy.FilterBlockedPattern, func(f *policy.ContentFilter) {
			f.Pattern = "RAHASIA"
			f.PatternType = policy.PatternSubstring
			f.CaseSensitive = true
		}),
	}}
	e := NewEngine(src, nil)

	if v := e.Check(context.Background(), Subject{Text: "ini RAHASIA"}); !v.Blocked {
		t.Error("pola peka besar-kecil tidak cocok pada bentuk yang sama")
	}
	if v := e.Check(context.Background(), Subject{Text: "ini rahasia"}); v.Blocked {
		t.Error("pola peka besar-kecil cocok pada bentuk huruf yang berbeda")
	}
}

func TestPolaRegex(t *testing.T) {
	src := &sumber{rows: []*policy.ContentFilter{
		row("f1", "nomor kartu", policy.FilterBlockedPattern, func(f *policy.ContentFilter) {
			f.Pattern = `\b\d{16}\b`
			f.PatternType = policy.PatternRegex
		}),
	}}
	e := NewEngine(src, nil)

	if v := e.Check(context.Background(), Subject{Text: "bayar dengan 4111111111111111 ya"}); !v.Blocked {
		t.Error("regex tidak cocok pada 16 digit")
	}
	if v := e.Check(context.Background(), Subject{Text: "bayar 1234 saja"}); v.Blocked {
		t.Error("regex cocok pada teks yang seharusnya tidak")
	}
}

// TestPolaDiwajibkan menjaga semantik daftar putih: terpicu justru ketika TIDAK cocok.
func TestPolaDiwajibkan(t *testing.T) {
	src := &sumber{rows: []*policy.ContentFilter{
		row("f1", "hanya bahasa Indonesia", policy.FilterAllowedPattern, func(f *policy.ContentFilter) {
			f.Pattern = "^[\\p{Latin}\\s\\d[:punct:]]+$"
			f.PatternType = policy.PatternRegex
		}),
	}}
	e := NewEngine(src, nil)

	if v := e.Check(context.Background(), Subject{Text: "halo dunia"}); v.Blocked {
		t.Errorf("teks yang memenuhi daftar putih diblokir: %s", v.Reason)
	}
	if v := e.Check(context.Background(), Subject{Text: "こんにちは"}); !v.Blocked {
		t.Error("teks yang tidak memenuhi daftar putih diloloskan")
	}
}

// TestRegexRusakArahKesalahanBerbeda adalah inti keputusan paket ini: "tidak diketahui" pada
// daftar hitam berarti belum terbukti buruk, pada daftar putih berarti belum terbukti boleh.
func TestRegexRusakArahKesalahanBerbeda(t *testing.T) {
	src := &sumber{rows: []*policy.ContentFilter{
		row("hitam", "daftar hitam rusak", policy.FilterBlockedPattern, func(f *policy.ContentFilter) {
			f.Pattern = "([a-z"
			f.PatternType = policy.PatternRegex
		}),
	}}
	e := NewEngine(src, nil)
	if v := e.Check(context.Background(), Subject{Text: "apa pun"}); v.Blocked {
		t.Error("daftar hitam dengan regex rusak memblokir; satu pola salah tulis akan mematikan seluruh lalu lintas")
	}
	if inert := e.Inert(); len(inert) != 1 || inert[0] != "daftar hitam rusak" {
		t.Errorf("Inert() = %v, mau memuat aturan yang tidak menegakkan apa pun", inert)
	}

	src2 := &sumber{rows: []*policy.ContentFilter{
		row("putih", "daftar putih rusak", policy.FilterAllowedPattern, func(f *policy.ContentFilter) {
			f.Pattern = "([a-z"
			f.PatternType = policy.PatternRegex
		}),
	}}
	e2 := NewEngine(src2, nil)
	if v := e2.Check(context.Background(), Subject{Text: "apa pun"}); !v.Blocked {
		t.Error("daftar putih dengan regex rusak meloloskan; tidak ada bukti isi ini termasuk yang diizinkan")
	}
}

// TestModerasiDilaporkanTidakMenegakkan: aturan moderasi yang tampak enabled di dashboard
// tetapi tidak memeriksa apa pun adalah kegagalan paling mahal yang bisa dimiliki tabel ini.
func TestModerasiDilaporkanTidakMenegakkan(t *testing.T) {
	src := &sumber{rows: []*policy.ContentFilter{
		row("mod", "moderasi OpenAI", policy.FilterModeration, func(f *policy.ContentFilter) {
			f.ModerationIntegrationID = "integrasi-1"
		}),
	}}
	e := NewEngine(src, nil)

	if v := e.Check(context.Background(), Subject{Text: "apa pun"}); v.Blocked {
		t.Error("aturan moderasi memblokir padahal belum didukung")
	}
	if inert := e.Inert(); len(inert) != 1 || inert[0] != "moderasi OpenAI" {
		t.Errorf("Inert() = %v, mau memuat aturan moderasi", inert)
	}
}

// --- Pembatasan entitas ------------------------------------------------------

func TestPembatasanModelDanProvider(t *testing.T) {
	src := &sumber{rows: []*policy.ContentFilter{
		row("m", "model dilarang", policy.FilterModelRestriction, func(f *policy.ContentFilter) {
			f.ModelID = "model-terlarang"
		}),
		row("p", "provider dilarang", policy.FilterProviderRestriction, func(f *policy.ContentFilter) {
			f.ProviderID = "provider-terlarang"
			f.Priority = 200
		}),
	}}
	e := NewEngine(src, nil)

	if v := e.Check(context.Background(), Subject{ModelID: "model-terlarang"}); !v.Blocked {
		t.Error("model yang dibatasi lolos")
	}
	if v := e.Check(context.Background(), Subject{ModelID: "model-lain"}); v.Blocked {
		t.Error("model lain ikut terblokir")
	}
	if v := e.Check(context.Background(), Subject{ProviderID: "provider-terlarang"}); !v.Blocked {
		t.Error("provider yang dibatasi lolos")
	}
	// Pemeriksaan bertahap: field yang belum diketahui melewatkan aturannya.
	if v := e.Check(context.Background(), Subject{Text: "halo"}); v.Blocked {
		t.Error("aturan pembatasan entitas memblokir padahal entitasnya belum diketahui")
	}
}

// --- applies_to --------------------------------------------------------------

func TestAppliesTo(t *testing.T) {
	src := &sumber{rows: []*policy.ContentFilter{
		row("req", "hanya permintaan", policy.FilterBlockedPattern, func(f *policy.ContentFilter) {
			f.Pattern = "x"
			f.PatternType = policy.PatternSubstring
			f.AppliesTo = policy.AppliesToRequest
		}),
		row("res", "hanya jawaban", policy.FilterBlockedPattern, func(f *policy.ContentFilter) {
			f.Pattern = "y"
			f.PatternType = policy.PatternSubstring
			f.AppliesTo = policy.AppliesToResponse
			f.Priority = 200
		}),
		row("both", "keduanya", policy.FilterBlockedPattern, func(f *policy.ContentFilter) {
			f.Pattern = "z"
			f.PatternType = policy.PatternSubstring
			f.AppliesTo = policy.AppliesToBoth
			f.Priority = 300
		}),
	}}
	e := NewEngine(src, nil)

	for _, tc := range []struct {
		nama      string
		s         Subject
		mauBlok   bool
		mauAturan string
	}{
		{"permintaan memuat x", Subject{Text: "x"}, true, "req"},
		{"permintaan memuat y", Subject{Text: "y"}, false, ""},
		{"jawaban memuat y", Subject{Text: "y", Response: true}, true, "res"},
		{"jawaban memuat x", Subject{Text: "x", Response: true}, false, ""},
		{"permintaan memuat z", Subject{Text: "z"}, true, "both"},
		{"jawaban memuat z", Subject{Text: "z", Response: true}, true, "both"},
	} {
		t.Run(tc.nama, func(t *testing.T) {
			v := e.Check(context.Background(), tc.s)
			if v.Blocked != tc.mauBlok {
				t.Fatalf("Blocked = %v, mau %v", v.Blocked, tc.mauBlok)
			}
			if tc.mauBlok && v.Filter.ID != tc.mauAturan {
				t.Errorf("aturan pemicu = %s, mau %s", v.Filter.ID, tc.mauAturan)
			}
		})
	}
}

// --- action warn -------------------------------------------------------------

func TestActionWarnTidakMemblokirTetapiDilaporkan(t *testing.T) {
	src := &sumber{rows: []*policy.ContentFilter{
		row("w", "pantau saja", policy.FilterBlockedPattern, func(f *policy.ContentFilter) {
			f.Pattern = "pantau"
			f.PatternType = policy.PatternSubstring
			f.Action = policy.ActionWarn
		}),
		row("b", "blokir", policy.FilterBlockedPattern, func(f *policy.ContentFilter) {
			f.Pattern = "blokir"
			f.PatternType = policy.PatternSubstring
			f.Priority = 200
		}),
	}}
	e := NewEngine(src, nil)

	v := e.Check(context.Background(), Subject{Text: "pantau ini"})
	if v.Blocked {
		t.Error("aturan berjenis warn memblokir")
	}
	if len(v.Warnings) != 1 || v.Warnings[0].ID != "w" {
		t.Errorf("Warnings = %v", v.Warnings)
	}

	// Aturan warn yang terpicu tidak boleh menghentikan evaluasi aturan pemblokir sesudahnya.
	v = e.Check(context.Background(), Subject{Text: "pantau lalu blokir"})
	if !v.Blocked || v.Filter.ID != "b" {
		t.Errorf("aturan warn menghentikan evaluasi: %+v", v)
	}
	if len(v.Warnings) != 1 {
		t.Errorf("Warnings = %v, mau tetap terkumpul", v.Warnings)
	}
}

// --- Urutan ------------------------------------------------------------------

// TestUrutanMengikutiSumber: yang murah didahulukan justru supaya permintaan buruk berhenti
// sebelum pola yang mahal dijalankan, dan urutan itu ditentukan priority di database.
func TestUrutanMengikutiSumber(t *testing.T) {
	src := &sumber{rows: []*policy.ContentFilter{
		row("murah", "ukuran", policy.FilterRequestSize, func(f *policy.ContentFilter) {
			f.MaxRequestBytes = pointer(int64(10))
			f.Priority = 10
		}),
		row("mahal", "pola", policy.FilterBlockedPattern, func(f *policy.ContentFilter) {
			f.Pattern = "apa"
			f.PatternType = policy.PatternSubstring
			f.Priority = 20
		}),
	}}
	e := NewEngine(src, nil)

	v := e.Check(context.Background(), Subject{Bytes: 999, Text: "apa pun"})
	if !v.Blocked || v.Filter.ID != "murah" {
		t.Errorf("aturan pemicu = %+v, mau aturan berpriority terkecil", v.Filter)
	}
}

// --- Anggaran waktu ---------------------------------------------------------

// TestAnggaranWaktuHabisArahKesalahanBerbeda memakai teks besar dan anggaran satu milidetik
// supaya jalur penjaga waktu benar-benar dilalui.
func TestAnggaranWaktuHabisArahKesalahanBerbeda(t *testing.T) {
	// Pola yang harus memindai seluruh masukan sebelum bisa menjawab, atas teks jauh di atas
	// batasTeksTerjaga.
	teks := strings.Repeat("a", 8<<20)

	src := &sumber{rows: []*policy.ContentFilter{
		row("hitam", "hitam lambat", policy.FilterBlockedPattern, func(f *policy.ContentFilter) {
			f.Pattern = "(a|aa)+z"
			f.PatternType = policy.PatternRegex
			f.MaxEvalMS = 1
		}),
	}}
	e := NewEngine(src, nil)
	if v := e.Check(context.Background(), Subject{Text: teks}); v.Blocked {
		t.Error("daftar hitam yang kehabisan waktu memblokir")
	}

	src2 := &sumber{rows: []*policy.ContentFilter{
		row("putih", "putih lambat", policy.FilterAllowedPattern, func(f *policy.ContentFilter) {
			f.Pattern = "(a|aa)+z"
			f.PatternType = policy.PatternRegex
			f.MaxEvalMS = 1
		}),
	}}
	e2 := NewEngine(src2, nil)
	if v := e2.Check(context.Background(), Subject{Text: teks}); !v.Blocked {
		t.Error("daftar putih yang kehabisan waktu meloloskan")
	}

	// Penghitung umpan balik dinaikkan di latar; tanpa itu operator tidak punya cara tahu
	// aturannya terlalu mahal.
	tunggu := time.Now().Add(3 * time.Second)
	for time.Now().Before(tunggu) {
		if src.jumlahTimeout()+src2.jumlahTimeout() >= 2 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Errorf("penghitung waktu habis = %d, mau 2", src.jumlahTimeout()+src2.jumlahTimeout())
}

// --- Cache -------------------------------------------------------------------

func TestSalinanDipakaiUlangSelamaTTL(t *testing.T) {
	src := &sumber{rows: []*policy.ContentFilter{
		row("f1", "a", policy.FilterBlockedPattern, func(f *policy.ContentFilter) {
			f.Pattern = "x"
			f.PatternType = policy.PatternSubstring
		}),
	}}
	sekarang := time.Now()
	e := NewEngine(src, nil, WithTTL(5*time.Second), WithClock(func() time.Time { return sekarang }))

	for i := 0; i < 5; i++ {
		e.Check(context.Background(), Subject{Text: "y"})
	}
	if n := src.jumlahDibaca(); n != 1 {
		t.Errorf("pembacaan tabel = %d, mau 1", n)
	}
	sekarang = sekarang.Add(6 * time.Second)
	e.Check(context.Background(), Subject{Text: "y"})
	if n := src.jumlahDibaca(); n != 2 {
		t.Errorf("pembacaan tabel = %d setelah TTL habis, mau 2", n)
	}
}

func TestPembacaanGagalMempertahankanSalinanLama(t *testing.T) {
	src := &sumber{rows: []*policy.ContentFilter{
		row("f1", "a", policy.FilterBlockedPattern, func(f *policy.ContentFilter) {
			f.Pattern = "x"
			f.PatternType = policy.PatternSubstring
		}),
	}}
	sekarang := time.Now()
	e := NewEngine(src, nil, WithTTL(time.Second), WithClock(func() time.Time { return sekarang }))

	if v := e.Check(context.Background(), Subject{Text: "x"}); !v.Blocked {
		t.Fatal("pembacaan pertama tidak memblokir")
	}
	src.mu.Lock()
	src.err = errors.New("database sedang tidak bisa dibaca")
	src.mu.Unlock()
	sekarang = sekarang.Add(time.Minute)

	if v := e.Check(context.Background(), Subject{Text: "x"}); !v.Blocked {
		t.Error("setelah pembacaan gagal, aturan yang sudah dimuat berhenti berlaku")
	}
}

func TestTanpaSumberTidakMemblokirApaPun(t *testing.T) {
	e := NewEngine(nil, nil)
	if v := e.Check(context.Background(), Subject{Text: "apa pun", Bytes: 1 << 30}); v.Blocked {
		t.Error("tanpa sumber malah memblokir")
	}
	if inert := e.Inert(); len(inert) != 0 {
		t.Errorf("Inert() = %v, mau kosong", inert)
	}
}

func TestAmanDipakaiBersamaan(t *testing.T) {
	src := &sumber{rows: []*policy.ContentFilter{
		row("f1", "a", policy.FilterBlockedPattern, func(f *policy.ContentFilter) {
			f.Pattern = "x"
			f.PatternType = policy.PatternSubstring
		}),
		row("f2", "b", policy.FilterAllowedPattern, func(f *policy.ContentFilter) {
			f.Pattern = "[a-z ]+"
			f.PatternType = policy.PatternRegex
			f.Priority = 200
		}),
	}}
	e := NewEngine(src, nil, WithTTL(time.Nanosecond))

	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				e.Check(context.Background(), Subject{Text: "halo dunia", Bytes: 100})
				e.Inert()
				if j%10 == 0 {
					e.Invalidate()
				}
			}
		}()
	}
	wg.Wait()
}
