package upstream

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// Gerbang arsitektur B-537 — pola gerbang mekanis yang sama dengan
// TestCheckOrphans (internal/admin): inventaris permukaan berbahaya
// dipindai MEKANIS dari sumber, dan setiap situs harus TERDAFTAR di
// sini beserta alasannya. Situs baru yang belum terdaftar — atau
// pendaftaran yang sudah tidak punya situs — MENGGAGALKAN tes.
//
// Permukaan yang dijaga adalah permukaan B-517: mutasi pelapor
// pergeseran pemenang (UpdateSecret, MarkAuthFailure) yang berjalan
// di dalam transaksi TANPA registry penilaian tertunda akan menilai
// pergeseran dari keadaan yang belum tentu commit dan bisa selesai
// tanpa satu pun laporan — persis celah yang ditutup B-517 untuk
// jalur OAuthRepo.SimpanHasil*. Tanpa gerbang ini, konstruksi
// transaksional baru di masa depan bisa membuka kembali celah itu
// tanpa satu pun tes merah: tes perilaku yang ada hanya menguji
// jalur yang sudah dikenal.
//
// Yang dipindai (seluruh pohon internal/ dan cmd/, berkas .go
// non-tes):
//
//  1. Panggilan mutasi pelapor `.UpdateSecret(` / `.MarkAuthFailure(`.
//  2. Literal `&CredentialRepo{` / `&OAuthRepo{` (konstruksi repo
//     kredensial di luar konstruktor pun tertangkap).
//  3. Pembuatan registry (`:= penilaianPergeseranTertundaBaru(`) dan
//     pengurasannya (`jalankanPenilaianSesudahKomit(ctx, tertunda)`,
//     plus satu panggilan internal `.jalankan(` di pemiliknya).
//
// Invarian isi yang diperiksa terpisah dari hitungan:
//   - Setiap literal `creds := &CredentialRepo{...}` di oauth.go
//     MEMUAT field tertunda (registry diwarisi/dipasang — bukan
//     literal telanjang yang menilai dari dalam transaksi).
//   - Literal yang dikembalikan WithQuerier kedua repo MEMUAT
//     field tertunda (salinan transaksional mewarisi registry).
//   - cmd/ai-gateway/main.go memasang callback ke KEDUA repo
//     produksi (klaim "terpasang di semua konstruksi produksi"
//     diverifikasi mekanis, bukan dari ingatan).
func TestGerbangRegistriPergeseranB537(t *testing.T) {
	akar := filepath.Join("..", "..", "..", "..")
	if _, err := os.Stat(filepath.Join(akar, "go.mod")); err != nil {
		t.Fatalf("akar repo tidak ditemukan dari direktori paket (%v) — gerbang tidak boleh lolos diam-diam", err)
	}
	if _, err := os.Stat(filepath.Join(akar, "internal", "database", "repo", "upstream", "oauth.go")); err != nil {
		t.Fatalf("oauth.go tidak ditemukan di akar %q (%v)", akar, err)
	}

	sumber := map[string]string{} // path relatif (slash) → isi
	for _, dir := range []string{"internal", "cmd"} {
		err := filepath.Walk(filepath.Join(akar, dir), func(p string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
				return nil
			}
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(akar, p)
			if err != nil {
				return err
			}
			sumber[filepath.ToSlash(rel)] = string(b)
			return nil
		})
		if err != nil {
			t.Fatalf("pindai %s: %v", dir, err)
		}
	}
	if len(sumber) == 0 {
		t.Fatal("pindai kosong — gerbang tidak boleh lolos diam-diam")
	}

	hitungPola := func(re *regexp.Regexp) map[string]int {
		hasil := map[string]int{}
		for rel, isi := range sumber {
			for _, m := range re.FindAllStringSubmatch(isi, -1) {
				hasil[rel+"|"+m[1]] += 1
			}
		}
		return hasil
	}
	bandingkan := func(nama string, got, mau map[string]int) {
		t.Helper()
		kunci := map[string]bool{}
		for k := range got {
			kunci[k] = true
		}
		for k := range mau {
			kunci[k] = true
		}
		daftar := make([]string, 0, len(kunci))
		for k := range kunci {
			daftar = append(daftar, k)
		}
		sort.Strings(daftar)
		for _, k := range daftar {
			if got[k] != mau[k] {
				t.Errorf("%s: %s terhitung %d, terdaftar %d — situs permukaan B-517 berubah; nilai apakah ia autocommit bercallback atau mewarisi/menguras registry penilaian tertunda, lalu perbarui pendaftaran di tes ini beserta alasannya",
					nama, k, got[k], mau[k])
			}
		}
	}

	// (1) Panggilan mutasi pelapor. Terdaftar:
	//   - oauth.go UpdateSecret ×2: SimpanHasilExchange dan
	//     SimpanHasilRefresh, keduanya atas `creds` yang literalnya
	//     memuat tertunda (diperiksa di invarian isi) — di cabang
	//     pool registry dibuat & dikuras pemiliknya, di cabang
	//     querier registry diwarisi dari repo transaksional.
	//   - factory.go MarkAuthFailure ×1: repo pabrik autocommit,
	//     callback terpasang dari cmd/ai-gateway/main.go.
	bandingkan("panggilan pelapor",
		hitungPola(regexp.MustCompile(`\.(UpdateSecret|MarkAuthFailure)\(`)),
		map[string]int{
			"internal/database/repo/upstream/oauth.go|UpdateSecret": 2,
			"internal/gateway/factory.go|MarkAuthFailure":           1,
		})

	// (2) Literal konstruksi repo. Terdaftar:
	//   - credentials.go CredentialRepo ×2: NewCredentialRepo
	//     (autocommit; tertunda nil memang rancangannya) dan
	//     WithQuerier (memuat tertunda — invarian isi).
	//   - oauth.go OAuthRepo ×2: NewOAuthRepo dan WithQuerier
	//     (alasan sama).
	//   - oauth.go CredentialRepo ×3: pembaca baca-saja di
	//     jalankanPenilaianSesudahKomit (oauth.go:123 pada basis
	//     Fase 23 — TIDAK PERNAH bermutasi, callback diteruskan
	//     sebagai argumen ke tertunda.jalankan), dan `creds` di
	//     kedua SimpanHasil* (memuat tertunda — invarian isi).
	bandingkan("literal repo",
		hitungPola(regexp.MustCompile(`&(CredentialRepo|OAuthRepo)\{`)),
		map[string]int{
			"internal/database/repo/upstream/credentials.go|CredentialRepo": 2,
			"internal/database/repo/upstream/oauth.go|OAuthRepo":            2,
			"internal/database/repo/upstream/oauth.go|CredentialRepo":       3,
		})

	// (3) Registry: dibuat ×2 (cabang pool kedua SimpanHasil*) dan
	// dikuras ×2 oleh pemilik yang sama; panggilan internal
	// tertunda.jalankan tepat ×1 di jalankanPenilaianSesudahKomit.
	bandingkan("pembuatan registri",
		hitungPola(regexp.MustCompile(`:= (penilaianPergeseranTertundaBaru)\(`)),
		map[string]int{
			"internal/database/repo/upstream/oauth.go|penilaianPergeseranTertundaBaru": 2,
		})
	bandingkan("pengurasan registri",
		hitungPola(regexp.MustCompile(`\.(jalankanPenilaianSesudahKomit)\(ctx, tertunda\)`)),
		map[string]int{
			"internal/database/repo/upstream/oauth.go|jalankanPenilaianSesudahKomit": 2,
		})
	bandingkan("panggilan jalankan internal",
		hitungPola(regexp.MustCompile(`tertunda\.(jalankan)\(`)),
		map[string]int{
			"internal/database/repo/upstream/oauth.go|jalankan": 1,
		})

	// Invarian isi: literal blok dari awal pola sampai '}' pertama.
	blokLiteral := func(isi, pola string) []string {
		var hasil []string
		for {
			i := strings.Index(isi, pola)
			if i < 0 {
				return hasil
			}
			sisa := isi[i:]
			j := strings.Index(sisa, "}")
			if j < 0 {
				return hasil
			}
			hasil = append(hasil, sisa[:j])
			isi = sisa[j:]
		}
	}
	oauth := sumber["internal/database/repo/upstream/oauth.go"]
	creds := blokLiteral(oauth, "creds := &CredentialRepo{")
	if len(creds) != 2 {
		t.Fatalf("literal creds di oauth.go terhitung %d, harap 2", len(creds))
	}
	for i, b := range creds {
		if !strings.Contains(b, "tertunda:") {
			t.Errorf("literal creds #%d di oauth.go tidak memuat field tertunda — mutasi pelapornya akan menilai dari dalam transaksi (celah B-517): %q", i+1, b)
		}
	}
	for _, cek := range []struct{ berkas, pola string }{
		{"internal/database/repo/upstream/credentials.go", "func (r *CredentialRepo) WithQuerier"},
		{"internal/database/repo/upstream/oauth.go", "func (r *OAuthRepo) WithQuerier"},
	} {
		isi := sumber[cek.berkas]
		i := strings.Index(isi, cek.pola)
		if i < 0 {
			t.Fatalf("%s tidak ditemukan di %s", cek.pola, cek.berkas)
		}
		segmen := isi[i:]
		if j := strings.Index(segmen, "\n}\n"); j >= 0 {
			segmen = segmen[:j]
		}
		if !strings.Contains(segmen, "tertunda:") {
			t.Errorf("%s di %s tidak mewariskan field tertunda ke salinannya — salinan transaksional kehilangan registry B-517", cek.pola, cek.berkas)
		}
	}

	// Pemasangan callback produksi di cmd.
	main := sumber["cmd/ai-gateway/main.go"]
	for _, pola := range []string{
		"creds.SetSesudahPemenangBergeser(",
		"oauthRepo.SetSesudahPemenangBergeser(",
	} {
		if !strings.Contains(main, pola) {
			t.Errorf("cmd/ai-gateway/main.go tidak lagi memuat %q — callback produksi tidak terpasang", pola)
		}
	}
}
