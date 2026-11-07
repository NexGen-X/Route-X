package upstream

import (
	"go/ast"
	"go/parser"
	"go/token"
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
// CAKUPAN PERSIS gerbang ini (dipersempit B-557 — klaim versi
// pertama menyiratkan penjagaan atas graf panggilan, padahal yang
// dihitung adalah SITUS TEKS):
//
// Yang dipindai adalah TEKS seluruh berkas .go non-tes di SELURUH
// pohon repo (internal/, cmd/, docs/, dan direktori mana pun di
// masa depan; web/ dikecualikan karena tidak memuat Go), untuk:
//
//  1. Panggilan mutasi pelapor `.UpdateSecret(` / `.MarkAuthFailure(`
//     — bentuk panggilan metode langsung pada ekspresi penerima
//     apa pun.
//  2. Literal TAK BERKUALIFIKASI `&CredentialRepo{` / `&OAuthRepo{`
//     (konstruksi repo kredensial di luar konstruktor pun
//     tertangkap — selama ditulis tanpa kualifikasi paket; lihat
//     pengecualian literal berkualifikasi di bawah).
//  3. Setiap penyebutan panggilan `penilaianPergeseranTertundaBaru(`
//     — bentuk penugasan apa pun (`:=`, `=`, argumen bersarang),
//     DITAMBAH deklarasi fungsinya sendiri yang terdaftar terpisah
//     per berkas; pembuatan registri TANPA panggilan konstruktor —
//     nilai fungsi (`f := penilaianPergeseranTertundaBaru`),
//     literal struct (`&penilaianPergeseranTertunda{`, hanya badan
//     konstruktor sendiri yang terdaftar), dan `new(...)`
//     (terlarang); pengurasan `.jalankanPenilaianSesudahKomit(
//     <ekspresi apa pun>, <ekspresi apa pun>)` (argumen pertama
//     bebas sejak B-566 — tidak lagi terikat literal `ctx` —
//     argumen kedua ekspresi apa pun termasuk akses field
//     `r.tertunda`); dan panggilan internal `tertunda.jalankan(`
//     di pemilik registry.
//  4. TERLARANG (hitungan wajib NOL): deklarasi alias tipe atas
//     kedua repo (`type X = CredentialRepo` / `type X =
//     upstream.OAuthRepo`, dsb.) — alias memberi nama kedua pada
//     tipe yang sama dan akan meloloskan konstruksi dari pola
//     literal di (2). Dideteksi lewat AST (B-561): bentuk tunggal
//     maupun berkelompok `type (...)`, berkualifikasi, berkomentar,
//     dan berpenunjuk `*` semuanya tertangkap.
//
// Yang TIDAK dicakup gerbang ini, dan tidak boleh dibaca sebagai
// diklaimnya: komposisi graf panggilan atas situs terdaftar
// (situs terdaftar yang dibungkus pembantu baru tetap terhitung
// satu situs — apakah bungkusannya setia adalah urusan tes
// perilaku), nilai metode (`f := repo.UpdateSecret` — tanpa
// kurung panggil ia tak berbentuk pola (1)), refleksi, dan
// konstruksi repo di berkas _test.go (di luar biner produksi,
// memang dikecualikan dari pindaian). Juga tidak dicakup:
// literal repo BERKUALIFIKASI (`&upstream.CredentialRepo{}` dari
// paket lain) — semua field kedua repo tak-terekspor, jadi
// literal itu tidak bisa dipasangi querier/cipher/registry dari
// luar paket dan repo nilai-nolnya tidak dapat dipakai untuk
// mutasi pelapor; registry NILAI-NOL via deklarasi `var`
// (`var r penilaianPergeseranTertunda`) — map `perProvider`-nya
// nil sehingga `catat` panik pada mutasi pertama dan `qKomit`-nya
// nil, jadi ia gagal keras bukan jalan pintas diam-diam; dan
// argumen pengurasan yang bersarang lebih dari satu tingkat tanda
// kurung (pola (3) membuka satu tingkat — bentuk itu tidak ada di
// pohon dan tidak wajar untuk argumen ctx/registry).
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

	// B-557: pindaian kini atas SELURUH pohon repo (bukan hanya
	// internal/ + cmd/) — berkas .go non-tes di direktori baru
	// mana pun ikut terhitung. B-567 (Fase 25): pengecualian
	// direktori hanya berlaku pada JALUR AKAR — `web/`, `.git/`,
	// dan `node_modules/` di akar repo saja (web/ akar tidak
	// memuat Go, isinya aset frontend; .git/node_modules akar
	// bukan sumber repo). Direktori bernama sama pada kedalaman
	// lebih dalam TETAP dipindai: literal di `internal/gateway/
	// web/` dan alias di `node_modules` bersarang terhitung.
	sumber := map[string]string{} // path relatif (slash) → isi
	err := filepath.Walk(akar, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			rel, err := filepath.Rel(akar, p)
			if err != nil {
				return err
			}
			switch filepath.ToSlash(rel) {
			case ".git", "web", "node_modules":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
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
		t.Fatalf("pindai pohon repo: %v", err)
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
	// hitungKemunculan menghitung kemunculan pola per berkas
	// dengan label tetap (untuk pola tanpa grup tangkap yang
	// bermakna — bandingkan dengan hitungPola yang mengunci pada
	// grup pertama).
	hitungKemunculan := func(re *regexp.Regexp, label string) map[string]int {
		hasil := map[string]int{}
		for rel, isi := range sumber {
			if n := len(re.FindAllString(isi, -1)); n > 0 {
				hasil[rel+"|"+label] += n
			}
		}
		return hasil
	}
	// hitungNilaiFungsiKonstruktor menghitung penyebutan nama
	// konstruktor yang TIDAK langsung diikuti `(` — yakni nama
	// yang dipakai sebagai nilai (disimpan ke variabel,
	// diteruskan sebagai argumen), bukan dipanggil dan bukan
	// dideklarasikan. Bentuk `f (x)` (spasi sebelum kurung)
	// bukan bentuk gofmt sehingga tidak ada di pohon.
	hitungNilaiFungsiKonstruktor := func() map[string]int {
		hasil := map[string]int{}
		re := regexp.MustCompile(`penilaianPergeseranTertundaBaru\b`)
		for rel, isi := range sumber {
			for _, lok := range re.FindAllStringIndex(isi, -1) {
				if lok[1] >= len(isi) || isi[lok[1]] != '(' {
					hasil[rel+"|nilai-fungsi"]++
				}
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
	// B-557: pola pembuatan dilonggarkan dari `:= ...(` menjadi
	// penyebutan panggilan dalam bentuk penugasan APA PUN — pola
	// lama buta terhadap pembuatan yang ditulis ulang sebagai `=`
	// atau argumen bersarang. Konsekuensinya deklarasi fungsi itu
	// sendiri (credentials.go) ikut terhitung dan terdaftar
	// terpisah sebagai deklarasi, bukan situs pembuatan.
	bandingkan("pembuatan registri",
		hitungPola(regexp.MustCompile(`(penilaianPergeseranTertundaBaru)\(`)),
		map[string]int{
			"internal/database/repo/upstream/credentials.go|penilaianPergeseranTertundaBaru": 1,
			"internal/database/repo/upstream/oauth.go|penilaianPergeseranTertundaBaru":       2,
		})
	// (3b) B-565 (Fase 25): pembuatan registri TANPA memanggil
	// konstruktor — tiga bentuk yang lolos dari hitungan panggilan
	// di atas:
	//   - nilai fungsi: `f := penilaianPergeseranTertundaBaru`
	//     lalu `f(nil)` memanggil konstruktor lewat variabel —
	//     TERLARANG (wajib nol); yang dihitung adalah penyebutan
	//     nama yang TIDAK langsung diikuti `(` (deklarasi dan
	//     panggilan biasa diikuti `(` dan tidak terhitung di sini).
	//   - literal struct: `&penilaianPergeseranTertunda{...}`
	//     (dengan/tanpa `&`) membangun registry tanpa konstruktor
	//     — di dalam paket ini literal itu BISA mengisi qKomit dan
	//     perProvider, jadi satu-satunya yang diizinkan adalah
	//     badan konstruktor sendiri (credentials.go ×1).
	//   - `new(penilaianPergeseranTertunda)` — TERLARANG (wajib
	//     nol); tidak ada pemakaian sahnya karena konstruktorlah
	//     yang mengisi map perProvider.
	// Sisa bentuk yang TIDAK dihitung: deklarasi `var` nilai-nol
	// (lihat komentar kepala — gagal keras, bukan jalan pintas).
	tanpaPanggilan := map[string]int{}
	for k, v := range hitungNilaiFungsiKonstruktor() {
		tanpaPanggilan[k] += v
	}
	for k, v := range hitungKemunculan(regexp.MustCompile(`&?penilaianPergeseranTertunda\{`), "literal-struct") {
		tanpaPanggilan[k] += v
	}
	for k, v := range hitungKemunculan(regexp.MustCompile(`new\(penilaianPergeseranTertunda\)`), "new") {
		tanpaPanggilan[k] += v
	}
	bandingkan("pembuatan registri tanpa panggilan",
		tanpaPanggilan,
		map[string]int{
			"internal/database/repo/upstream/credentials.go|literal-struct": 1,
		})
	// B-566 (Fase 25): pola pengurasan dilonggarkan dari
	// `(ctx, <nama>)` menjadi `(<ekspresi apa pun>, <ekspresi apa
	// pun>)` — argumen pertama bebas (bukan literal `ctx`) dan
	// argumen kedua ekspresi apa pun termasuk akses field
	// (`r.tertunda`) atau panggilan (`ambilRegistry()`); tiap
	// argumen membuka paling satu tingkat tanda kurung bersarang
	// (lihat batas di komentar kepala). (Deklarasi metodenya
	// sendiri tidak berbentuk pola ini: ia tidak diawali `.`.)
	bandingkan("pengurasan registri",
		hitungPola(regexp.MustCompile(`\.(jalankanPenilaianSesudahKomit)\(\s*(?:[^,()]|\([^()]*\))+,\s*(?:[^,()]|\([^()]*\))+\s*\)`)),
		map[string]int{
			"internal/database/repo/upstream/oauth.go|jalankanPenilaianSesudahKomit": 2,
		})
	bandingkan("panggilan jalankan internal",
		hitungPola(regexp.MustCompile(`tertunda\.(jalankan)\(`)),
		map[string]int{
			"internal/database/repo/upstream/oauth.go|jalankan": 1,
		})

	// (4) B-557: alias tipe atas kedua repo TERLARANG (wajib nol
	// di seluruh pohon) — alias adalah nama kedua untuk tipe yang
	// sama, jadi konstruksi lewat nama alias akan lolos dari pola
	// literal (2) sambil tetap repo yang sama persis. Pendaftaran
	// ini TIDAK PERNAH diperbarui menjadi mengizinkan: temuan
	// berarti jalan pintas sedang dibuka dan harus ditutup, bukan
	// didaftarkan.
	//
	// B-561 (Fase 25): deteksi lewat AST, bukan pola teks — pola
	// `type\s+\w+\s*=` buta terhadap deklarasi berkelompok
	// `type ( A = CredentialRepo )`, alias berkomentar
	// `type A = /*x*/ CredentialRepo`, dan alias penunjuk
	// `type A = *CredentialRepo`. AST menangkap semuanya (RHS
	// dibuka dari `*`/tanda kurung; kualifikasi paket diabaikan),
	// dan komentar yang sekadar menyebut alias tidak lagi memicu
	// positif palsu. Berkas yang gagal diurai dilaporkan sebagai
	// galat — pohon yang rusak tidak boleh lolos diam-diam.
	urutRel := make([]string, 0, len(sumber))
	for rel := range sumber {
		urutRel = append(urutRel, rel)
	}
	sort.Strings(urutRel)
	fset := token.NewFileSet()
	for _, rel := range urutRel {
		isi := sumber[rel]
		f, err := parser.ParseFile(fset, rel, isi, parser.ParseComments)
		if err != nil {
			t.Errorf("gagal mengurai %s untuk sapuan alias B-561: %v", rel, err)
			continue
		}
		ast.Inspect(f, func(n ast.Node) bool {
			ts, ok := n.(*ast.TypeSpec)
			if !ok || !ts.Assign.IsValid() {
				return true
			}
			if !aliasAtasRepo(ts.Type) {
				return true
			}
			awal := fset.Position(ts.Type.Pos()).Offset
			akhir := fset.Position(ts.Type.End()).Offset
			rhs := ""
			if 0 <= awal && awal <= akhir && akhir <= len(isi) {
				rhs = isi[awal:akhir]
			}
			t.Errorf("alias repo terlarang di %s (%s): type %s = %s — konstruksi repo wajib berbentuk literal &CredentialRepo{}/&OAuthRepo{} supaya terhitung gerbang B-537/B-557; alias memberi jalan pintas keluar dari inventaris itu",
				rel, fset.Position(ts.Pos()), ts.Name.Name, rhs)
			return true
		})
	}

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

// aliasAtasRepo melaporkan apakah ekspresi tipe — sesudah membuka
// penunjuk `*` dan tanda kurung — bernama CredentialRepo atau
// OAuthRepo, berkualifikasi paket atau tidak (B-561). Bentuk lain
// (irisan, map, struct, ...) bukan alias atas repo itu dan tidak
// terhitung.
func aliasAtasRepo(e ast.Expr) bool {
	switch t := e.(type) {
	case *ast.StarExpr:
		return aliasAtasRepo(t.X)
	case *ast.ParenExpr:
		return aliasAtasRepo(t.X)
	case *ast.Ident:
		return t.Name == "CredentialRepo" || t.Name == "OAuthRepo"
	case *ast.SelectorExpr:
		return t.Sel.Name == "CredentialRepo" || t.Sel.Name == "OAuthRepo"
	default:
		return false
	}
}
