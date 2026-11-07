package upstream

import (
	"fmt"
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
//     (terlarang, termasuk bentuk ber-spasi sejak B-574);
//     pengurasan `.jalankanPenilaianSesudahKomit(
//     <ekspresi apa pun>, <ekspresi apa pun>)` (argumen pertama
//     bebas sejak B-566 — tidak lagi terikat literal `ctx` —
//     argumen kedua ekspresi apa pun termasuk akses field
//     `r.tertunda`), pengurasan lewat NILAI-METODE atas drain
//     (`f := r.jalankanPenilaianSesudahKomit`, `f := t.jalankan` —
//     TERLARANG sejak B-576), dan panggilan internal
//     `tertunda.jalankan(` di pemilik registry.
//     Seluruh hitungan (3) memindai teks yang SUDAH dikupas dari
//     komentar dan literal string (B-577): penyebutan nama di
//     prosa komentar/string bukan situs dan tidak terhitung; nama
//     juga dijangkar `\b` di kedua sisi, jadi identifier yang
//     lebih panjang yang mengandung nama persis
//     (`xpenilaianPergeseranTertundaBaru`) tidak terhitung.
//  4. TERLARANG (hitungan wajib NOL): deklarasi alias tipe atas
//     kedua repo (`type X = CredentialRepo` / `type X =
//     upstream.OAuthRepo`, dsb.) DAN atas tipe registry
//     (`type X = penilaianPergeseranTertunda`, sejak B-575) —
//     alias memberi nama kedua pada tipe yang sama dan akan
//     meloloskan konstruksi dari pola literal di (2)/(3b).
//     Dideteksi lewat AST (B-561): bentuk tunggal maupun
//     berkelompok `type (...)`, berkualifikasi, berkomentar,
//     dan berpenunjuk `*` semuanya tertangkap.
//
// Yang TIDAK dicakup gerbang ini, dan tidak boleh dibaca sebagai
// diklaimnya: komposisi graf panggilan atas situs terdaftar
// (situs terdaftar yang dibungkus pembantu baru tetap terhitung
// satu situs — apakah bungkusannya setia adalah urusan tes
// perilaku), nilai metode atas pola (1) (`f := repo.UpdateSecret`
// — tanpa kurung panggil ia tak berbentuk pola (1); pengecualian
// ini TIDAK berlaku untuk drain — nilai-metode atas drain DICAKUP
// sejak B-576, lihat (3)), refleksi, dan konstruksi repo di
// berkas _test.go (di luar biner produksi, memang dikecualikan
// dari pindaian). Juga tidak dicakup:
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
// pohon dan tidak wajar untuk argumen ctx/registry). Terakhir:
// pola (1), (2), pengurasan, dan panggilan internal memindai
// teks MENTAH — penyebutan di komentar/string ikut terhitung
// untuk keempatnya (kelas positif-palsu yang sama seperti
// sebelum B-577; B-577 hanya mengupas (3), (3b), dan
// nilai-metode drain, yang polanya menyebut nama telanjang
// sehingga jauh lebih mungkin muncul dalam prosa komentar).
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
	akar := akarRepo(t)
	sumber := muatSumber(t, akar)
	for _, pelanggaran := range periksaGerbang(sumber) {
		t.Error(pelanggaran)
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

// akarRepo mengembalikan akar repo (empat tingkat di atas direktori
// paket ini) sesudah memastikan penanda yang diharapkan ada —
// gerbang tidak boleh lolos diam-diam bila dijalankan dari tempat
// yang salah.
func akarRepo(t *testing.T) string {
	t.Helper()
	akar := filepath.Join("..", "..", "..", "..")
	if _, err := os.Stat(filepath.Join(akar, "go.mod")); err != nil {
		t.Fatalf("akar repo tidak ditemukan dari direktori paket (%v) — gerbang tidak boleh lolos diam-diam", err)
	}
	if _, err := os.Stat(filepath.Join(akar, "internal", "database", "repo", "upstream", "oauth.go")); err != nil {
		t.Fatalf("oauth.go tidak ditemukan di akar %q (%v)", akar, err)
	}
	return akar
}

// muatSumber memindai seluruh pohon repo dari akar dan mengembalikan
// isi berkas .go non-tes (path relatif slash → isi).
//
// B-557: pindaian kini atas SELURUH pohon repo (bukan hanya
// internal/ + cmd/) — berkas .go non-tes di direktori baru mana
// pun ikut terhitung. B-567 (Fase 25): pengecualian direktori
// hanya berlaku pada JALUR AKAR — `web/`, `.git/`, dan
// `node_modules/` di akar repo saja (web/ akar tidak memuat Go,
// isinya aset frontend; .git/node_modules akar bukan sumber repo).
// Direktori bernama sama pada kedalaman lebih dalam TETAP dipindai:
// literal di `internal/gateway/web/` dan alias di `node_modules`
// bersarang terhitung.
func muatSumber(t *testing.T, akar string) map[string]string {
	t.Helper()
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
	return sumber
}

// hitungPola menghitung kemunculan pola per berkas, dikunci pada
// grup tangkap pertama pola.
func hitungPola(sumber map[string]string, re *regexp.Regexp) map[string]int {
	hasil := map[string]int{}
	for rel, isi := range sumber {
		for _, m := range re.FindAllStringSubmatch(isi, -1) {
			hasil[rel+"|"+m[1]] += 1
		}
	}
	return hasil
}

// hitungKemunculan menghitung kemunculan pola per berkas dengan
// label tetap (untuk pola tanpa grup tangkap yang bermakna).
func hitungKemunculan(sumber map[string]string, re *regexp.Regexp, label string) map[string]int {
	hasil := map[string]int{}
	for rel, isi := range sumber {
		if n := len(re.FindAllString(isi, -1)); n > 0 {
			hasil[rel+"|"+label] += n
		}
	}
	return hasil
}

// hitungNilaiFungsiKonstruktor menghitung penyebutan nama
// konstruktor yang TIDAK langsung diikuti `(` — yakni nama yang
// dipakai sebagai nilai (disimpan ke variabel, diteruskan sebagai
// argumen), bukan dipanggil dan bukan dideklarasikan. Bentuk
// `f (x)` (spasi sebelum kurung) bukan bentuk gofmt sehingga tidak
// ada di pohon. Peta yang diterima adalah teks BERSIH (komentar
// dan string sudah dikupas, B-577) dan nama dijangkar `\b` di
// kedua sisi — identifier lebih panjang yang mengandung nama
// persis bukan konstruktor dan tidak terhitung.
func hitungNilaiFungsiKonstruktor(bersih map[string]string) map[string]int {
	hasil := map[string]int{}
	re := regexp.MustCompile(`\bpenilaianPergeseranTertundaBaru\b`)
	for rel, isi := range bersih {
		for _, lok := range re.FindAllStringIndex(isi, -1) {
			if lok[1] >= len(isi) || isi[lok[1]] != '(' {
				hasil[rel+"|nilai-fungsi"]++
			}
		}
	}
	return hasil
}

// teksBersih mengembalikan isi berkas dengan komentar dan literal
// string dikupas (diganti spasi, baris baru dipertahankan) —
// penyebutan nama di komentar/prosa dan di dalam string tidak lagi
// terhitung (B-577). Kupasan memakai AST (go/parser): rentang
// komentar dan literal string diambil dari pohon urai, jadi
// penanda komentar di dalam string (dan sebaliknya) tidak salah
// kupas. Berkas yang gagal diurai dikembalikan apa adanya
// (fail-closed: sapuan alias sudah menggagalkan tes untuk berkas
// rusak, dan pola teks tetap memindai teks mentahnya).
func teksBersih(isi string) string {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "sumber.go", isi, parser.ParseComments)
	if err != nil {
		return isi
	}
	kupas := make([]bool, len(isi))
	tandai := func(pos, end token.Pos) {
		awal := fset.Position(pos).Offset
		akhir := fset.Position(end).Offset
		if awal < 0 || akhir > len(isi) || awal > akhir {
			return
		}
		for i := awal; i < akhir; i++ {
			if isi[i] != '\n' {
				kupas[i] = true
			}
		}
	}
	for _, cg := range f.Comments {
		for _, c := range cg.List {
			tandai(c.Slash, c.End())
		}
	}
	ast.Inspect(f, func(n ast.Node) bool {
		if lit, ok := n.(*ast.BasicLit); ok && lit.Kind == token.STRING {
			tandai(lit.Pos(), lit.End())
		}
		return true
	})
	b := []byte(isi)
	for i := range b {
		if kupas[i] {
			b[i] = ' '
		}
	}
	return string(b)
}

// sumberBersih mengembalikan salinan peta sumber dengan tiap
// berkas sudah dikupas komentar/string-nya (B-577). Panjang tiap
// isi dipertahankan sehingga posisi hitungan tidak bergeser.
func sumberBersih(sumber map[string]string) map[string]string {
	bersih := make(map[string]string, len(sumber))
	for rel, isi := range sumber {
		bersih[rel] = teksBersih(isi)
	}
	return bersih
}

// hitungNilaiMetodeDrain menghitung penyebutan
// `.jalankanPenilaianSesudahKomit` dan `.jalankan` yang TIDAK
// langsung diikuti `(` — yakni metode drain yang dipakai sebagai
// nilai (disimpan ke variabel, diteruskan sebagai callback),
// bukan dipanggil (B-576, paralel dengan penangkap nilai-fungsi
// B-565). Peta yang diterima adalah teks BERSIH (B-577) dan nama
// dijangkar `\b` — metode lain yang namanya berawalan sama
// (`jalankanRestore`, `jalankanSesuatu`) tidak terhitung, dan
// panggilan biasa (diikuti `(`) juga tidak. TERLARANG (wajib
// nol): tidak ada pemakaian nilai-metode atas drain yang
// terdaftar.
func hitungNilaiMetodeDrain(bersih map[string]string) map[string]int {
	hasil := map[string]int{}
	for _, re := range []*regexp.Regexp{
		regexp.MustCompile(`\.jalankanPenilaianSesudahKomit\b`),
		regexp.MustCompile(`\.jalankan\b`),
	} {
		for rel, isi := range bersih {
			for _, lok := range re.FindAllStringIndex(isi, -1) {
				if lok[1] >= len(isi) || isi[lok[1]] != '(' {
					hasil[rel+"|nilai-metode-drain"]++
				}
			}
		}
	}
	return hasil
}

// selisihBandingan mengembalikan daftar ketidaksesuaian antara
// hitungan terukur dan pendaftaran, tanpa menggagalkan tes —
// pemanggil yang memutuskan: test penjaga menggagalkan tiap
// selisih, probe pengunci memeriksa isinya.
func selisihBandingan(nama string, got, mau map[string]int) []string {
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
	var selisih []string
	for _, k := range daftar {
		if got[k] != mau[k] {
			selisih = append(selisih, fmt.Sprintf("%s: %s terhitung %d, terdaftar %d — situs permukaan B-517 berubah; nilai apakah ia autocommit bercallback atau mewarisi/menguras registry penilaian tertunda, lalu perbarui pendaftaran di tes ini beserta alasannya",
				nama, k, got[k], mau[k]))
		}
	}
	return selisih
}

// periksaGerbang menjalankan SEMUA hitungan gerbang B-537/B-557 atas
// peta sumber yang diberikan dan mengembalikan daftar pelanggaran
// (kosong = lolos). Dipakai test penjaga atas pohon nyata DAN probe
// pengunci atas salinan pohon yang disuntik bypass — satu jalur
// kode, tanpa proksi: probe yang lolos membuktikan gigi gerbang
// itu sendiri yang menggigit, bukan salinan polanya.
func periksaGerbang(sumber map[string]string) []string {
	var pelanggaran []string

	// (1) Panggilan mutasi pelapor. Terdaftar:
	//   - oauth.go UpdateSecret ×2: SimpanHasilExchange dan
	//     SimpanHasilRefresh, keduanya atas `creds` yang literalnya
	//     memuat tertunda (diperiksa di invarian isi) — di cabang
	//     pool registry dibuat & dikuras pemiliknya, di cabang
	//     querier registry diwarisi dari repo transaksional.
	//   - factory.go MarkAuthFailure ×1: repo pabrik autocommit,
	//     callback terpasang dari cmd/ai-gateway/main.go.
	pelanggaran = append(pelanggaran, selisihBandingan("panggilan pelapor",
		hitungPola(sumber, regexp.MustCompile(`\.(UpdateSecret|MarkAuthFailure)\(`)),
		map[string]int{
			"internal/database/repo/upstream/oauth.go|UpdateSecret": 2,
			"internal/gateway/factory.go|MarkAuthFailure":           1,
		})...)

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
	pelanggaran = append(pelanggaran, selisihBandingan("literal repo",
		hitungPola(sumber, regexp.MustCompile(`&(CredentialRepo|OAuthRepo)\{`)),
		map[string]int{
			"internal/database/repo/upstream/credentials.go|CredentialRepo": 2,
			"internal/database/repo/upstream/oauth.go|OAuthRepo":            2,
			"internal/database/repo/upstream/oauth.go|CredentialRepo":       3,
		})...)

	// (3) Registry: dibuat ×2 (cabang pool kedua SimpanHasil*) dan
	// dikuras ×2 oleh pemilik yang sama; panggilan internal
	// tertunda.jalankan tepat ×1 di jalankanPenilaianSesudahKomit.
	// B-557: pola pembuatan dilonggarkan dari `:= ...(` menjadi
	// penyebutan panggilan dalam bentuk penugasan APA PUN — pola
	// lama buta terhadap pembuatan yang ditulis ulang sebagai `=`
	// atau argumen bersarang. Konsekuensinya deklarasi fungsi itu
	// sendiri (credentials.go) ikut terhitung dan terdaftar
	// terpisah sebagai deklarasi, bukan situs pembuatan.
	// B-577 (Fase 26): hitungan ini memindai teks BERSIH
	// (komentar/string dikupas) dan nama dijangkar `\b` —
	// panggilan di komentar dan identifier lebih panjang bukan
	// situs.
	bersih := sumberBersih(sumber)
	pelanggaran = append(pelanggaran, selisihBandingan("pembuatan registri",
		hitungPola(bersih, regexp.MustCompile(`\b(penilaianPergeseranTertundaBaru)\(`)),
		map[string]int{
			"internal/database/repo/upstream/credentials.go|penilaianPergeseranTertundaBaru": 1,
			"internal/database/repo/upstream/oauth.go|penilaianPergeseranTertundaBaru":       2,
		})...)
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
	//     yang mengisi map perProvider. B-574 (Fase 26): pola
	//     dilonggarkan ke `new\s*\(\s*...` — bentuk Go sah
	//     ber-spasi `new( penilaianPergeseranTertunda )` (bukan
	//     gofmt) ikut tertangkap.
	// B-577 (Fase 26): ketiga hitungan di atas memindai teks
	// BERSIH (komentar/string dikupas) dan nama dijangkar `\b` —
	// penyebutan di komentar/string dan identifier lebih panjang
	// bukan situs dan tidak terhitung.
	// Sisa bentuk yang TIDAK dihitung: deklarasi `var` nilai-nol
	// (lihat komentar kepala — gagal keras, bukan jalan pintas).
	tanpaPanggilan := map[string]int{}
	for k, v := range hitungNilaiFungsiKonstruktor(bersih) {
		tanpaPanggilan[k] += v
	}
	for k, v := range hitungKemunculan(bersih, regexp.MustCompile(`&?\bpenilaianPergeseranTertunda\{`), "literal-struct") {
		tanpaPanggilan[k] += v
	}
	for k, v := range hitungKemunculan(bersih, regexp.MustCompile(`new\s*\(\s*\bpenilaianPergeseranTertunda\b\s*\)`), "new") {
		tanpaPanggilan[k] += v
	}
	pelanggaran = append(pelanggaran, selisihBandingan("pembuatan registri tanpa panggilan",
		tanpaPanggilan,
		map[string]int{
			"internal/database/repo/upstream/credentials.go|literal-struct": 1,
		})...)
	// B-566 (Fase 25): pola pengurasan dilonggarkan dari
	// `(ctx, <nama>)` menjadi `(<ekspresi apa pun>, <ekspresi apa
	// pun>)` — argumen pertama bebas (bukan literal `ctx`) dan
	// argumen kedua ekspresi apa pun termasuk akses field
	// (`r.tertunda`) atau panggilan (`ambilRegistry()`); tiap
	// argumen membuka paling satu tingkat tanda kurung bersarang
	// (lihat batas di komentar kepala). (Deklarasi metodenya
	// sendiri tidak berbentuk pola ini: ia tidak diawali `.`.)
	pelanggaran = append(pelanggaran, selisihBandingan("pengurasan registri",
		hitungPola(sumber, regexp.MustCompile(`\.(jalankanPenilaianSesudahKomit)\(\s*(?:[^,()]|\([^()]*\))+,\s*(?:[^,()]|\([^()]*\))+\s*\)`)),
		map[string]int{
			"internal/database/repo/upstream/oauth.go|jalankanPenilaianSesudahKomit": 2,
		})...)
	pelanggaran = append(pelanggaran, selisihBandingan("panggilan jalankan internal",
		hitungPola(sumber, regexp.MustCompile(`tertunda\.(jalankan)\(`)),
		map[string]int{
			"internal/database/repo/upstream/oauth.go|jalankan": 1,
		})...)
	// B-576 (Fase 26): pengurasan lewat NILAI-METODE — `f :=
	// r.jalankanPenilaianSesudahKomit; f(ctx, reg)` dan `f :=
	// t.jalankan; f(ctx, nil, nil)` meneruskan drain sebagai
	// callback tanpa pernah berbentuk pola pengurasan di atas.
	// Dihitung paralel dengan penangkap nilai-fungsi B-565:
	// penyebutan `.jalankanPenilaianSesudahKomit` / `.jalankan`
	// yang TIDAK diikuti `(` — TERLARANG (wajib nol). Panggilan
	// biasa (diikuti `(`) tidak terhitung di sini; metode lain
	// bernama `jalankan*` (mis. `jalankanRestore`) tidak
	// terhitung karena jangkar `\b`.
	pelanggaran = append(pelanggaran, selisihBandingan("nilai-metode drain",
		hitungNilaiMetodeDrain(bersih),
		map[string]int{})...)

	// (4) B-557: alias tipe atas kedua repo TERLARANG (wajib nol
	// di seluruh pohon) — alias adalah nama kedua untuk tipe yang
	// sama, jadi konstruksi lewat nama alias akan lolos dari pola
	// literal (2) sambil tetap repo yang sama persis. Pendaftaran
	// ini TIDAK PERNAH diperbarui menjadi mengizinkan: temuan
	// berarti jalan pintas sedang dibuka dan harus ditutup, bukan
	// didaftarkan.
	// B-575 (Fase 26): larangan diperluas ke alias atas tipe
	// registry (`type X = penilaianPergeseranTertunda`) — alias
	// itu meloloskan literal `&penilaianPergeseranTertunda{}`
	// dari pola (3b) dengan cara yang sama.
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
	pelanggaran = append(pelanggaran, sapuAliasTerlarang(sumber)...)

	return pelanggaran
}

// sapuAliasTerlarang memeriksa seluruh sumber lewat AST dan
// mengembalikan pelanggaran alias atas repo/registry (B-561,
// B-575). Berkas yang gagal diurai dilaporkan sebagai pelanggaran
// — pohon yang rusak tidak boleh lolos diam-diam.
func sapuAliasTerlarang(sumber map[string]string) []string {
	var pelanggaran []string
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
			pelanggaran = append(pelanggaran, fmt.Sprintf("gagal mengurai %s untuk sapuan alias B-561: %v", rel, err))
			continue
		}
		ast.Inspect(f, func(n ast.Node) bool {
			ts, ok := n.(*ast.TypeSpec)
			if !ok || !ts.Assign.IsValid() {
				return true
			}
			// B-575: alias atas registry dilarang sama seperti
			// alias atas repo — pesannya menyebut bentuk yang
			// wajib dipakai untuk tiap jenis.
			wajib := ""
			switch {
			case aliasAtasRepo(ts.Type):
				wajib = "konstruksi repo wajib berbentuk literal &CredentialRepo{}/&OAuthRepo{} supaya terhitung gerbang B-537/B-557"
			case aliasAtasRegistry(ts.Type):
				wajib = "konstruksi registry wajib lewat penilaianPergeseranTertundaBaru(...) supaya terhitung gerbang B-537/B-557"
			default:
				return true
			}
			awal := fset.Position(ts.Type.Pos()).Offset
			akhir := fset.Position(ts.Type.End()).Offset
			rhs := ""
			if 0 <= awal && awal <= akhir && akhir <= len(isi) {
				rhs = isi[awal:akhir]
			}
			pelanggaran = append(pelanggaran, fmt.Sprintf("alias repo/registry terlarang di %s (%s): type %s = %s — %s; alias memberi nama kedua pada tipe yang sama dan meloloskan konstruksi dari inventaris itu",
				rel, fset.Position(ts.Pos()), ts.Name.Name, rhs, wajib))
			return true
		})
	}
	return pelanggaran
}

// namaTipeDasar mengembalikan nama tipe sesudah membuka penunjuk
// `*` dan tanda kurung — CredentialRepo, OAuthRepo,
// penilaianPergeseranTertunda, atau nama lain — atau "" bila
// ekspresi bukan penamaan tipe sederhana (irisan, map, struct,
// ...). Kualifikasi paket diabaikan (B-561).
func namaTipeDasar(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.StarExpr:
		return namaTipeDasar(t.X)
	case *ast.ParenExpr:
		return namaTipeDasar(t.X)
	case *ast.Ident:
		return t.Name
	case *ast.SelectorExpr:
		return t.Sel.Name
	default:
		return ""
	}
}

// aliasAtasRepo melaporkan apakah ekspresi tipe — sesudah membuka
// penunjuk `*` dan tanda kurung — bernama CredentialRepo atau
// OAuthRepo, berkualifikasi paket atau tidak (B-561). Bentuk lain
// (irisan, map, struct, ...) bukan alias atas repo itu dan tidak
// terhitung.
func aliasAtasRepo(e ast.Expr) bool {
	nama := namaTipeDasar(e)
	return nama == "CredentialRepo" || nama == "OAuthRepo"
}

// aliasAtasRegistry melaporkan apakah ekspresi tipe adalah
// penilaianPergeseranTertunda (B-575, paralel B-561). Alias atas
// tipe registry memberi nama kedua pada tipe yang sama dan
// meloloskan literal `&penilaianPergeseranTertunda{}` /
// `new(...)` dari pola (3b) — kelas yang sama dengan B-561
// untuk kedua repo.
func aliasAtasRegistry(e ast.Expr) bool {
	return namaTipeDasar(e) == "penilaianPergeseranTertunda"
}

// ---------------------------------------------------------------------------
// Probe pengunci Fase 26 (B-574–B-577)
//
// Tiap probe memuat pohon sumber NYATA (helper yang sama dengan test
// penjaga), menyuntikkan SATU bentuk bypass ke isi satu berkas nyata
// — dalam memori, pohon di disk tidak disentuh — lalu menjalankan
// pemeriksa gerbang yang SAMA (periksaGerbang). Suntikan harus
// berupa Go yang terurai: sapuan alias menggagalkan tes untuk
// berkas yang rusak, jadi suntikan yang tidak terurai akan
// menggagalkan probe dengan alasan yang salah.
//
// Arah tuntutan tiap probe dicatat di komentarnya: probe "bentuk
// ditangkap" menuntut pelanggaran menyebut bentuk itu (GAGAL di
// base karena gigi belum ada, LULUS sesudah perbaikan); probe
// "bentuk benign" menuntut NOL pelanggaran (GAGAL di base karena
// penjaga salah menghitungnya, LULUS sesudah perbaikan); probe
// anti-pelemahan menuntut bypass nyata TETAP tertangkap (LULUS di
// base DAN sesudah perbaikan — ia menjaga perbaikan B-577 agar
// tidak melemahkan penjaga).
// ---------------------------------------------------------------------------

// salinanDenganSuntikan memuat pohon sumber nyata lalu menambahkan
// cuplikan ke akhir isi satu berkas nyata (dalam memori).
func salinanDenganSuntikan(t *testing.T, berkas, suntikan string) map[string]string {
	t.Helper()
	sumber := muatSumber(t, akarRepo(t))
	isi, ok := sumber[berkas]
	if !ok {
		t.Fatalf("berkas suntikan %s tidak ada di pohon sumber", berkas)
	}
	sumber[berkas] = isi + "\n" + suntikan + "\n"
	return sumber
}

// gabungPelanggaran menggabungkan daftar pelanggaran menjadi satu
// teks untuk pemeriksaan strings.Contains.
func gabungPelanggaran(pelanggaran []string) string {
	return strings.Join(pelanggaran, "\n")
}

// TestProbePenjagaB574NewBerSpasi — B-574: `new(
// penilaianPergeseranTertunda )` dan `new
// (penilaianPergeseranTertunda)` (spasi sah Go, bukan gofmt) wajib
// tertangkap gigi `new` (3b) — keduanya. `new(
// penilaianPergeseranTertundaX )` (nama tipe lebih panjang) BUKAN
// situs dan wajib tidak terhitung. GAGAL di base: pola
// `new\(penilaianPergeseranTertunda\)` menuntut `(` menempel.
func TestProbePenjagaB574NewBerSpasi(t *testing.T) {
	sumber := salinanDenganSuntikan(t,
		"internal/database/repo/upstream/credentials.go",
		"var _ = new( penilaianPergeseranTertunda )\n\nvar _ = new (penilaianPergeseranTertunda)\n\nvar _ = new( penilaianPergeseranTertundaX )")
	pelanggaran := periksaGerbang(sumber)
	if !strings.Contains(gabungPelanggaran(pelanggaran), "credentials.go|new terhitung 2, terdaftar 0") {
		t.Errorf("probe B-574: `new(...)` ber-spasi tidak tertangkap penjaga (harap 2, nama lebih panjang wajib lolos) — pelanggaran: %v", pelanggaran)
	}
}

// TestProbePenjagaB575AliasRegistry — B-575: `type R =
// penilaianPergeseranTertunda` lalu `&R{}` wajib tertangkap
// larangan alias (4) — begitu pula bentuk berkelompok dan
// berpenunjuk (paralel B-561). Tipe terdefinisi tanpa `=` dan
// alias atas tipe lain BUKAN alias atas registry dan wajib tidak
// terhitung. GAGAL di base: sapuan alias hanya memeriksa
// CredentialRepo/OAuthRepo.
func TestProbePenjagaB575AliasRegistry(t *testing.T) {
	sumber := salinanDenganSuntikan(t,
		"internal/database/repo/upstream/credentials.go",
		"type R1 = penilaianPergeseranTertunda\n\nvar _ = &R1{}\n\ntype (\n\tR2 = penilaianPergeseranTertunda\n)\n\ntype R3 = *penilaianPergeseranTertunda\n\ntype RDef penilaianPergeseranTertunda\n\ntype S = map[string]int")
	pelanggaran := periksaGerbang(sumber)
	gabung := gabungPelanggaran(pelanggaran)
	if n := strings.Count(gabung, "alias repo/registry terlarang"); n != 3 {
		t.Errorf("probe B-575: alias atas penilaianPergeseranTertunda terhitung %d, harap 3 (tunggal+berkelompok+penunjuk; tipe terdefinisi/alias lain wajib lolos) — pelanggaran: %v", n, pelanggaran)
	}
	if !strings.Contains(gabung, "type R1 = penilaianPergeseranTertunda") {
		t.Errorf("probe B-575: pesan alias tidak menyebut bentuk yang disuntik — pelanggaran: %v", pelanggaran)
	}
}

// TestProbePenjagaB576NilaiMetodeDrain — B-576: pengurasan lewat
// nilai-metode (`f := r.jalankanPenilaianSesudahKomit` dan
// `f := t.jalankan`) wajib tertangkap — keduanya, bukan salah
// satu. Panggilan biasa (`q.jalankan(...)`) dan nilai-metode atas
// nama yang lebih panjang (`h.jalankanRestore`) BUKAN nilai-metode
// drain dan wajib tidak terhitung. GAGAL di base: pola drain
// menuntut `(` sesudah nama metode.
func TestProbePenjagaB576NilaiMetodeDrain(t *testing.T) {
	sumber := salinanDenganSuntikan(t,
		"internal/database/repo/upstream/oauth.go",
		"var _ = r.jalankanPenilaianSesudahKomit\n\nvar _ = t.jalankan\n\nvar _ = q.jalankan(ctx, nil, nil)\n\nvar _ = h.jalankanRestore")
	pelanggaran := periksaGerbang(sumber)
	if !strings.Contains(gabungPelanggaran(pelanggaran), "oauth.go|nilai-metode-drain terhitung 2, terdaftar 0") {
		t.Errorf("probe B-576: nilai-metode atas drain tidak tertangkap penjaga (harap 2; panggilan/nama lebih panjang wajib lolos) — pelanggaran: %v", pelanggaran)
	}
}

// TestProbePenjagaB577KomentarDanStringTidakTerhitung — B-577:
// penyebutan nama konstruktor di komentar, di literal string, dan
// sebagai bagian identifier yang lebih panjang BUKAN situs
// pembuatan dan wajib TIDAK terhitung. GAGAL di base: penangkap
// nilai-fungsi memindai teks mentah tanpa jangkar `\b` awal.
func TestProbePenjagaB577KomentarDanStringTidakTerhitung(t *testing.T) {
	sumber := salinanDenganSuntikan(t,
		"internal/database/repo/upstream/credentials.go",
		"// lihat penilaianPergeseranTertundaBaru\n\nvar _ = \"penilaianPergeseranTertundaBaru\"\n\nvar _ = xpenilaianPergeseranTertundaBaru\n\nvar _ = xpenilaianPergeseranTertundaBaru()")
	if pelanggaran := periksaGerbang(sumber); len(pelanggaran) != 0 {
		t.Errorf("probe B-577: penyebutan benign (komentar/string/nama lebih panjang) ikut terhitung — pelanggaran: %v", pelanggaran)
	}
}

// TestProbePenjagaB577BypassNyataTetapTertangkap — B-577
// (anti-pelemahan): perbaikan B-577 TIDAK BOLEH melemahkan
// penjaga. Ketiga bentuk bypass nyata — nilai-fungsi, literal
// struct, dan `new(...)` — wajib TETAP menggagalkan penjaga
// sesudah perbaikan. LULUS di base DAN sesudah perbaikan.
func TestProbePenjagaB577BypassNyataTetapTertangkap(t *testing.T) {
	sumber := salinanDenganSuntikan(t,
		"internal/database/repo/upstream/credentials.go",
		"var _ = penilaianPergeseranTertundaBaru\n\nvar _ = &penilaianPergeseranTertunda{}\n\nvar _ = new(penilaianPergeseranTertunda)")
	gabung := gabungPelanggaran(periksaGerbang(sumber))
	for _, harap := range []string{
		"credentials.go|nilai-fungsi terhitung 1, terdaftar 0",
		"credentials.go|literal-struct terhitung 2, terdaftar 1",
		"credentials.go|new terhitung 1, terdaftar 0",
	} {
		if !strings.Contains(gabung, harap) {
			t.Errorf("probe anti-pelemahan B-577: bentuk bypass nyata %q tidak lagi tertangkap — pelanggaran: %v", harap, gabung)
		}
	}
}
