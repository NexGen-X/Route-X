package gateway

import (
	"context"
	"fmt"
	"testing"

	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
	"github.com/NexGen-X/Route-X/internal/providers"
)

// rencanaAnggaran adalah rencana combo pipeline: MaxAttempts tetap per kandidat, tapi
// Budget membatasi TOTAL percobaan lintas seluruh kandidat.
func rencanaAnggaran(model string, maks, anggaran int, cands ...*upstream.RouteCandidate) Plan {
	return Plan{Candidates: cands, Model: model, MaxAttempts: maks, Budget: anggaran, BackoffBase: 0}
}

// Anggaran combo pipeline adalah TOTAL, bukan per kandidat. attempts:3 pada tiga kandidat
// harus menghasilkan tepat 3 percobaan upstream — bukan 9. Tanpa pembatasan ini, satu
// permintaan klien menjadi perkalian biaya yang tidak pernah diminta resepnya.
func TestExecuteAnggaranTotalMembatasiPercobaanLintasKandidat(t *testing.T) {
	g := penjaga()
	e, _ := executorUji(g)

	var urutan []string
	out := Execute(context.Background(), e,
		rencanaAnggaran("m", 3, 3, kandidat("a"), kandidat("b"), kandidat("c")),
		func(_ context.Context, c *upstream.RouteCandidate) (string, error) {
			urutan = append(urutan, c.ProviderName)
			return "", providers.Newf(providers.ErrKindOverloaded, c.ProviderName, "penuh")
		})

	if out.Err == nil {
		t.Fatal("Err nil, mau kegagalan setelah seluruh kandidat habis anggaran")
	}
	mau := []string{"a", "a", "a"}
	if fmt.Sprint(urutan) != fmt.Sprint(mau) {
		t.Errorf("urutan percobaan = %v, mau %v — anggaran 3 harus dihabiskan kandidat pertama", urutan, mau)
	}
	if len(out.Attempts) != 3 {
		t.Errorf("jejak = %d percobaan, mau 3 (sama dengan anggaran), bukan %d", len(out.Attempts), len(out.Attempts))
	}
	// Kandidat b dan c tidak boleh direkam pemutus arusnya: mereka tidak pernah dihubungi.
	if len(g.catatan()) != 3 {
		t.Errorf("catatan pemutus arus = %v, mau 3 catatan (semuanya kandidat a)", g.catatan())
	}
}

// Anggaran dipakai BERSAMA: kandidat pertama yang memakai 2 dari 3 hanya menyisakan 1
// untuk kandidat kedua. Kegagalan tidak boleh membuat kandidat kedua mendapat jatah penuh
// lagi — anggaran tidak direset saat berpindah kandidat.
func TestExecuteAnggaranDibagiAntarKandidat(t *testing.T) {
	g := penjaga()
	e, _ := executorUji(g)

	var urutan []string
	out := Execute(context.Background(), e,
		rencanaAnggaran("m", 3, 3, kandidat("a"), kandidat("b")),
		func(_ context.Context, c *upstream.RouteCandidate) (string, error) {
			urutan = append(urutan, c.ProviderName)
			// Kandidat a: percobaan pertama aman diulang, kedua tidak aman diulang
			// (berhenti mencoba a tepat setelah 2 percobaan).
			if c.ProviderName == "a" && len(urutan) == 2 {
				return "", providers.Newf(providers.ErrKindAuth, c.ProviderName, "ditolak")
			}
			return "", providers.Newf(providers.ErrKindOverloaded, c.ProviderName, "penuh")
		})

	if out.Err == nil {
		t.Fatal("Err nil, mau kegagalan")
	}
	mau := []string{"a", "a", "b"}
	if fmt.Sprint(urutan) != fmt.Sprint(mau) {
		t.Errorf("urutan percobaan = %v, mau %v", urutan, mau)
	}
	// a memakai 2, menyisakan 1 untuk b. b hanya boleh 1 kali, bukan MaxAttempts=3.
	if len(out.Attempts) != 3 {
		t.Errorf("jejak = %d percobaan, mau 3 (2 untuk a, 1 untuk b)", len(out.Attempts))
	}
	if out.Attempts[2].ProviderName != "b" {
		t.Errorf("percobaan ketiga = %v, mau kandidat b", out.Attempts[2].ProviderName)
	}
}

// Anggaran nol adalah rule biasa: tiap kandidat memakai MaxAttempts penuhnya sendiri.
// Tanpa jalur ini, rule non-combo akan kehilangan kesabaran per-provider yang menjadi
// sifat dasarnya — ini menguji bahwa anggaran benar-benar opt-in, bukan selalu aktif.
func TestExecuteTanpaAnggaranPerilakuLama(t *testing.T) {
	g := penjaga()
	e, _ := executorUji(g)

	var urutan []string
	out := Execute(context.Background(), e,
		rencana("m", 3, 0, kandidat("a"), kandidat("b"), kandidat("c")),
		func(_ context.Context, c *upstream.RouteCandidate) (string, error) {
			urutan = append(urutan, c.ProviderName)
			return "", providers.Newf(providers.ErrKindOverloaded, c.ProviderName, "penuh")
		})

	if out.Err == nil {
		t.Fatal("Err nil, mau kegagalan")
	}
	mau := []string{"a", "a", "a", "b", "b", "b", "c", "c", "c"}
	if fmt.Sprint(urutan) != fmt.Sprint(mau) {
		t.Errorf("urutan percobaan = %v, mau %v (MaxAttempts penuh per kandidat)", urutan, mau)
	}
	if len(out.Attempts) != 9 {
		t.Errorf("jejak = %d percobaan, mau 9 (3 kandidat × 3 MaxAttempts)", len(out.Attempts))
	}
}

// Invarian no-retry-setelah-aliran-terbuka bertemu anggaran: begitu byte pertama akan
// terkirim, sisa anggaran tidak boleh dipakai untuk failover. Memutus aliran di tengah
// adalah kegagalan yang SUDAH terlihat klien — tidak ada jawaban lain yang jujur.
func TestExecuteStreamAnggaranBerhentiSaatAliranTerbuka(t *testing.T) {
	g := penjaga()
	e, _ := executorUji(g)

	var dipanggil int
	out := ExecuteStream(context.Background(), e,
		// Anggaran 2, tapi kandidat pertama langsung berhasil membuka aliran.
		rencanaAnggaran("m", 2, 2, kandidat("a"), kandidat("b")),
		func(cctx context.Context, c *upstream.RouteCandidate) (providers.Stream, error) {
			dipanggil++
			return &aliranUji{ctx: cctx}, nil
		})

	if out.Err != nil {
		t.Fatalf("Err = %v, mau nil", out.Err)
	}
	if out.Value == nil {
		t.Fatal("Value nil, mau aliran")
	}
	if dipanggil != 1 {
		t.Errorf("op dipanggil %d kali, mau 1 — kandidat b tidak boleh disentuh", dipanggil)
	}
	if len(out.Attempts) != 1 {
		t.Errorf("jejak = %d percobaan, mau 1", len(out.Attempts))
	}

	// Membaca dan menutup aliran TIDAK boleh menambah percobaan: titik tanpa-return
	// sudah terlewati saat aliran terbuka, dan sisa anggaran 1 tidak dipakai failover.
	if _, err := out.Value.Recv(); err != nil {
		t.Fatalf("Recv = %v, mau nil", err)
	}
	if err := out.Value.Close(); err != nil {
		t.Fatalf("Close = %v", err)
	}
	if len(out.Attempts) != 1 {
		t.Errorf("jejak = %d percobaan setelah aliran dibaca, mau tetap 1", len(out.Attempts))
	}
	if g.catatan()[0] != "prov-a:true" {
		t.Errorf("catatan pemutus arus = %v, mau [prov-a:true]", g.catatan())
	}
}

// Anggaran tidak berkurang karena kandidat yang dilewati pemutus arus. Mereka tidak
// menagih upstream apa pun, dan mengurangi anggaran untuk mereka akan menghukum
// kandidat sehat yang berada setelah kandidat yang sedang sirkuit-terbuka.
func TestExecuteAnggaranTidakHabisOlehKandidatSkip(t *testing.T) {
	g := penjaga()
	g.tolak["prov-a"] = true // kandidat a: pemutus arus terbuka, dilewati tanpa menghubungi.
	g.tolak["prov-b"] = true
	e, _ := executorUji(g)

	var urutan []string
	out := Execute(context.Background(), e,
		rencanaAnggaran("m", 1, 1, kandidat("a"), kandidat("b"), kandidat("c")),
		func(_ context.Context, c *upstream.RouteCandidate) (string, error) {
			urutan = append(urutan, c.ProviderName)
			return "ok", nil
		})

	if out.Err != nil {
		t.Fatalf("Err = %v, mau sukses pada kandidat c", out.Err)
	}
	mau := []string{"c"}
	if fmt.Sprint(urutan) != fmt.Sprint(mau) {
		t.Errorf("urutan percobaan = %v, mau %v", urutan, mau)
	}
	if out.Candidate == nil || out.Candidate.ProviderName != "c" {
		t.Errorf("Candidate = %v, mau c", out.Candidate)
	}
}
