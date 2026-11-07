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
//     apa pun; sejak B-598 pola dilonggarkan ke
//     `\.(UpdateSecret|MarkAuthFailure)\s*\(` (spasi/komentar di
//     antara nama dan tanda buka ikut tertangkap) dan memindai
//     teks bersih (B-577), jadi penyebutan di komentar/string
//     bukan situs.
//  2. Literal TAK BERKUALIFIKASI `&CredentialRepo{` / `&OAuthRepo{`
//     (konstruksi repo kredensial di luar konstruktor pun
//     tertangkap — selama ditulis tanpa kualifikasi paket; lihat
//     pengecualian literal berkualifikasi di bawah). Sejak B-598
//     pola dilonggarkan ke `&\s*(CredentialRepo|OAuthRepo)\s*\{`
//     (spasi/komentar di antara `&`, nama, dan `{` ikut
//     tertangkap) dan memindai teks bersih.
//  3. Setiap penyebutan panggilan `penilaianPergeseranTertundaBaru(`
//     — bentuk penugasan apa pun (`:=`, `=`, argumen bersarang),
//     DITAMBAH deklarasi fungsinya sendiri yang terdaftar terpisah
//     per berkas; pembuatan registri TANPA panggilan konstruktor —
//     LIMA bentuk: nilai fungsi
//     (`f := penilaianPergeseranTertundaBaru`), literal struct
//     (`&penilaianPergeseranTertunda{`, hanya badan konstruktor
//     sendiri yang terdaftar — dihitung lewat AST sejak B-598),
//     `new(...)` (terlarang, termasuk bentuk ber-spasi sejak
//     B-574), konversi eksplisit dari tipe terdefinisi
//     (`penilaianPergeseranTertunda(R{...})`,
//     `CredentialRepo(R{...})`, `OAuthRepo(R{...})` — terlarang
//     sejak B-595), dan `var` yang DIISI field
//     (`r.perProvider = ...`, `r.qKomit = ...` — terlarang sejak
//     B-596; `var` TAK TERISI tetap bukan situs, lihat bawah);
//     pengurasan `.jalankanPenilaianSesudahKomit(
//     <ekspresi apa pun>, <ekspresi apa pun>)` (argumen pertama
//     bebas sejak B-566 — tidak lagi terikat literal `ctx` —
//     argumen kedua ekspresi apa pun termasuk akses field
//     `r.tertunda`), pengurasan lewat NILAI-METODE atas drain
//     (`f := r.jalankanPenilaianSesudahKomit`, `f := t.jalankan` —
//     TERLARANG sejak B-576), dan panggilan internal
//     `.jalankan(` atas receiver NAMA APA PUN di pemilik registry
//     (bebas nama sejak B-597 — dulu terikat `tertunda`; spasi/
//     komentar sebelum `(` ikut tertangkap sejak B-598).
//     Seluruh hitungan (3) memindai teks yang SUDAH dikupas dari
//     komentar dan literal string (B-577): penyebutan nama di
//     prosa komentar/string bukan situs dan tidak terhitung; nama
//     juga dijangkar `\b` di kedua sisi, jadi identifier yang
//     lebih panjang yang mengandung nama persis
//     (`xpenilaianPergeseranTertundaBaru`) tidak terhitung.
//     `\b` di sini adalah batas kata ASCII (Go regexp/RE2):
//     huruf non-ASCII yang menempel di depan nama tetap memberi
//     batas sehingga terhitung (fail-closed), huruf ASCII tidak.
//  4. TERLARANG (hitungan wajib NOL): deklarasi alias tipe atas
//     kedua repo (`type X = CredentialRepo` / `type X =
//     upstream.OAuthRepo`, dsb.) DAN atas tipe registry
//     (`type X = penilaianPergeseranTertunda`, sejak B-575) —
//     alias memberi nama kedua pada tipe yang sama dan akan
//     meloloskan konstruksi dari pola literal di (2)/(3b);
//     sejak B-595, deklarasi tipe TERDEFINISI atas ketiga nama
//     (`type X T`, `type X *T`, `type X (T)` — tanpa `=`) juga
//     TERLARANG: konversi eksplisit `T(X{...})` antara tipe
//     terdefinisi dan tipe asalnya adalah Go yang sah dan
//     membangun registry/repo kerja yang lolos pola literal.
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
// nil, jadi ia gagal keras SELAMA TAK TERISI, bukan jalan pintas
// diam-diam (koreksi B-596 atas B-565: var yang DIISI field
// adalah registry kerja dan TERHITUNG sejak B-596); panggilan
// `.catat(` langsung — satu-satunya pemanggil produksinya adalah
// `selesaikanPergeseran`, dan rantai yang memakainya tetap
// tertangkap di mata konstruksi + drain (B-595/B-596/B-597,
// dibuktikan probe rantai Fase 27); dan argumen pengurasan yang
// bersarang lebih dari satu tingkat tanda kurung (pola (3)
// membuka satu tingkat — bentuk itu tidak ada di pohon dan tidak
// wajar untuk argumen ctx/registry). Terakhir: pola pengurasan
// (drain) MASIH memindai teks MENTAH — penyebutan di
// komentar/string ikut terhitung untuknya; (1), (2), panggilan
// internal, dan seluruh hitungan (3)/(3b) memindai teks BERSIH
// (B-577; (1)/(2)/internal sejak B-598).
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

	// B-577 (Fase 26): hitungan (1), (2), (3), (3b), dan panggilan
	// internal memindai teks BERSIH (komentar/string dikupas) —
	// penyebutan di komentar/string bukan situs. Hanya pola
	// pengurasan (drain) yang masih memindai teks mentah (lihat
	// komentar kepala).
	bersih := sumberBersih(sumber)

	// (1) Panggilan mutasi pelapor. Terdaftar:
	//   - oauth.go UpdateSecret ×2: SimpanHasilExchange dan
	//     SimpanHasilRefresh, keduanya atas `creds` yang literalnya
	//     memuat tertunda (diperiksa di invarian isi) — di cabang
	//     pool registry dibuat & dikuras pemiliknya, di cabang
	//     querier registry diwarisi dari repo transaksional.
	//   - factory.go MarkAuthFailure ×1: repo pabrik autocommit,
	//     callback terpasang dari cmd/ai-gateway/main.go.
	// B-598 (Fase 27): pola dilonggarkan ke
	// `\.(UpdateSecret|MarkAuthFailure)\s*\(` — spasi/komentar di
	// antara nama dan tanda buka ikut tertangkap; (1) kini
	// memindai teks bersih, bukan mentah.
	pelanggaran = append(pelanggaran, selisihBandingan("panggilan pelapor",
		hitungPola(bersih, regexp.MustCompile(`\.(UpdateSecret|MarkAuthFailure)\s*\(`)),
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
	// B-598 (Fase 27): pola dilonggarkan ke
	// `&\s*(CredentialRepo|OAuthRepo)\s*\{` — spasi/komentar di
	// antara `&`, nama, dan `{` ikut tertangkap; (2) kini
	// memindai teks bersih, bukan mentah.
	pelanggaran = append(pelanggaran, selisihBandingan("literal repo",
		hitungPola(bersih, regexp.MustCompile(`&\s*(CredentialRepo|OAuthRepo)\s*\{`)),
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
	pelanggaran = append(pelanggaran, selisihBandingan("pembuatan registri",
		hitungPola(bersih, regexp.MustCompile(`\b(penilaianPergeseranTertundaBaru)\(`)),
		map[string]int{
			"internal/database/repo/upstream/credentials.go|penilaianPergeseranTertundaBaru": 1,
			"internal/database/repo/upstream/oauth.go|penilaianPergeseranTertundaBaru":       2,
		})...)
	// (3b) B-565 (Fase 25): pembuatan registri TANPA memanggil
	// konstruktor — LIMA bentuk yang lolos dari hitungan panggilan
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
	//     B-598 (Fase 27): hitungan literal PINDAH ke AST
	//     (CompositeLit atas tipe itu, hitungLiteralStructRegistry)
	//     — regex naif `&?\s*\bpenilaianPergeseranTertunda\s*\{`
	//     SALAH karena ikut menghitung baris deklarasi
	//     konstruktornya sendiri
	//     (`func ... *penilaianPergeseranTertunda {`) sebagai
	//     literal; AST hanya menghitung literal komposit
	//     sungguhan, dan spasi/komentar di antara nama dan `{`
	//     tidak relevan bagi AST.
	//   - `new(penilaianPergeseranTertunda)` — TERLARANG (wajib
	//     nol); tidak ada pemakaian sahnya karena konstruktorlah
	//     yang mengisi map perProvider. B-574 (Fase 26): pola
	//     dilonggarkan ke `new\s*\(\s*...` — bentuk Go sah
	//     ber-spasi `new( penilaianPergeseranTertunda )` (bukan
	//     gofmt) ikut tertangkap.
	//   - konversi eksplisit dari tipe terdefinisi (B-595, Fase
	//     27): `penilaianPergeseranTertunda(R{...})`,
	//     `CredentialRepo(R{...})`, `OAuthRepo(R{...})` —
	//     TERLARANG (wajib nol); dihitung terpisah di bawah
	//     ("konversi tipe terdefinisi") karena satu-satunya
	//     pemakaian sah ketiga nama diikuti `(` tidak ada —
	//     deklarasi/field/parameter tidak diikuti `(` dan nama
	//     konstruktor (`...Baru`) tidak cocok karena jangkar `\b`.
	//   - `var` yang DIISI field (B-596, Fase 27): penugasan
	//     `.perProvider =` / `.qKomit =` — TERLARANG (wajib nol);
	//     dihitung terpisah di bawah ("penugasan field registry").
	//     Penugasan INDEKS (`t.perProvider[k] = ...`, dipakai
	//     `catat`) dan perbandingan (`== nil`) BUKAN penugasan
	//     field dan tidak terhitung.
	// B-577 (Fase 26): hitungan-hitungan di atas memindai teks
	// BERSIH (komentar/string dikupas) dan nama dijangkar `\b` —
	// penyebutan di komentar/string dan identifier lebih panjang
	// bukan situs dan tidak terhitung.
	// Sisa bentuk yang TIDAK dihitung: deklarasi `var` nilai-nol
	// TAK TERISI (lihat komentar kepala — gagal keras SELAMA tak
	// terisi, bukan jalan pintas; koreksi B-596 atas B-565).
	tanpaPanggilan := map[string]int{}
	for k, v := range hitungNilaiFungsiKonstruktor(bersih) {
		tanpaPanggilan[k] += v
	}
	for k, v := range hitungLiteralStructRegistry(sumber) {
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
	// B-595 (Fase 27): konversi eksplisit `T(...)` atas ketiga
	// nama — pasangan dari larangan tipe terdefinisi di (4):
	// deklarasi tipenya TERLARANG, dan konversi yang memakainya
	// TERHITUNG di sini. TERLARANG (wajib nol).
	konversi := map[string]int{}
	for k, v := range hitungKemunculan(bersih, regexp.MustCompile(`\bpenilaianPergeseranTertunda\s*\(`), "konversi-registry") {
		konversi[k] += v
	}
	for k, v := range hitungPola(bersih, regexp.MustCompile(`\b(CredentialRepo|OAuthRepo)\s*\(`)) {
		konversi[k] += v
	}
	pelanggaran = append(pelanggaran, selisihBandingan("konversi tipe terdefinisi",
		konversi,
		map[string]int{})...)
	// B-596 (Fase 27): penugasan field atas registry — `var`
	// nilai-nol yang diisi field menjadi registry kerja penuh.
	// TERLARANG (wajib nol). Pola menuntut `=` tunggal (bukan
	// `==`): `[^=]` sesudah `=` menolak perbandingan, dan `[`
	// sesudah nama field menolak penugasan indeks.
	pelanggaran = append(pelanggaran, selisihBandingan("penugasan field registry",
		hitungPola(bersih, regexp.MustCompile(`\.(perProvider|qKomit)\s*=(?:[^=]|$)`)),
		map[string]int{})...)
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
	// B-597 (Fase 27): pola panggilan internal dilonggarkan dari
	// `tertunda\.(jalankan)\(` menjadi `\.(jalankan)\s*\(` —
	// bebas nama receiver (paralel B-566) dan spasi/komentar
	// sebelum `(` ikut tertangkap (B-598); hitungan kini
	// memindai teks bersih. Terdaftar:
	//   - oauth.go ×1: `tertunda.jalankan(...)` di
	//     jalankanPenilaianSesudahKomit (pemilik registry).
	//   - backup_sql.go ×1: `p.jalankan()` atas `*pemindaiDump`
	//     (pemindai dump SQL admin — metode tak terkait registry,
	//     nol argumen; ikut terhitung karena namanya sama).
	//     KOREKSI atas klaim B-597 "nol situs lain di pohon
	//     nyata": klaim itu salah — situs ini ada dan kini
	//     TERDAFTAR dengan alasannya, bukan nol.
	// Panggilan `.jalankanPenilaianSesudahKomit(` BUKAN
	// `.jalankan(` (nama berlanjut) dan tidak terhitung di sini.
	pelanggaran = append(pelanggaran, selisihBandingan("panggilan jalankan internal",
		hitungPola(bersih, regexp.MustCompile(`\.(jalankan)\s*\(`)),
		map[string]int{
			"internal/database/repo/upstream/oauth.go|jalankan": 1,
			"internal/admin/backup_sql.go|jalankan":             1,
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
	// B-595 (Fase 27): larangan diperluas ke tipe TERDEFINISI
	// atas ketiga nama (`type X T`, tanpa `=`) — klaim benign
	// probe B-575 ("tipe terdefinisi wajib tidak terhitung")
	// GUGUR: konversi `T(X{...})` membangun registry/repo kerja
	// (dibuktikan dinamis Putaran #14). Deklarasi terdefinisi
	// TERLARANG seperti alias, dan konversinya dihitung di (3b).
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
	pelanggaran = append(pelanggaran, sapuAliasDanDefinisiTerlarang(sumber)...)

	return pelanggaran
}

// sapuAliasDanDefinisiTerlarang memeriksa seluruh sumber lewat
// AST dan mengembalikan pelanggaran alias atas repo/registry
// (B-561, B-575) DAN tipe terdefinisi atas ketiganya (B-595).
// Berkas yang gagal diurai dilaporkan sebagai pelanggaran — pohon
// yang rusak tidak boleh lolos diam-diam.
func sapuAliasDanDefinisiTerlarang(sumber map[string]string) []string {
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
			if !ok {
				return true
			}
			if !ts.Assign.IsValid() {
				// B-595: tipe terdefinisi atas repo/registry
				// (`type X T`, tanpa `=`) TERLARANG seperti
				// alias — konversi `T(X{...})` membangun
				// registry/repo kerja dari nama kedua itu.
				// Deklarasi struct/map/dsb. (RHS bukan penamaan
				// tipe sederhana) bukan tipe terdefinisi atas
				// ketiganya dan tidak terhitung.
				if !aliasAtasRepo(ts.Type) && !aliasAtasRegistry(ts.Type) {
					return true
				}
				awal := fset.Position(ts.Type.Pos()).Offset
				akhir := fset.Position(ts.Type.End()).Offset
				rhs := ""
				if 0 <= awal && awal <= akhir && akhir <= len(isi) {
					rhs = isi[awal:akhir]
				}
				pelanggaran = append(pelanggaran, fmt.Sprintf("tipe terdefinisi atas repo/registry terlarang di %s (%s): type %s %s — konstruksi registry/repo wajib lewat penilaianPergeseranTertundaBaru(...) / literal &CredentialRepo{}/&OAuthRepo{} supaya terhitung gerbang B-537/B-557; konversi dari tipe terdefinisi membangun registry/repo kerja yang lolos dari inventaris itu",
					rel, fset.Position(ts.Pos()), ts.Name.Name, rhs))
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

// hitungLiteralStructRegistry menghitung literal struct atas tipe
// registry lewat AST (B-598, Fase 27): setiap CompositeLit yang
// tipenya — sesudah membuka `*`/tanda kurung — bernama
// penilaianPergeseranTertunda. AST adalah obat yang benar untuk
// pola (3b) ber-spasi/berkomentar: spasi dan komentar di antara
// nama dan `{` tidak relevan bagi pengurai, dan — yang
// menentukan — baris deklarasi konstruktornya sendiri
// (`func ... *penilaianPergeseranTertunda {`) BUKAN CompositeLit
// sehingga tidak terhitung, padahal regex naif
// `&?\s*\bpenilaianPergeseranTertunda\s*\{` menghitungnya
// (terbukti: pola itu cocok pada credentials.go:222). Berkas
// yang gagal diurai dilewati di sini — sapuan alias sudah
// melaporkan berkas itu sebagai pelanggaran, jadi penjaga tetap
// merah.
func hitungLiteralStructRegistry(sumber map[string]string) map[string]int {
	hasil := map[string]int{}
	fset := token.NewFileSet()
	for rel, isi := range sumber {
		f, err := parser.ParseFile(fset, rel, isi, 0)
		if err != nil {
			continue
		}
		ast.Inspect(f, func(n ast.Node) bool {
			lit, ok := n.(*ast.CompositeLit)
			if !ok {
				return true
			}
			if namaTipeDasar(lit.Type) == "penilaianPergeseranTertunda" {
				hasil[rel+"|literal-struct"]++
			}
			return true
		})
	}
	return hasil
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
// berpenunjuk (paralel B-561). Alias atas tipe lain BUKAN alias
// atas registry dan wajib tidak terhitung. KOREKSI B-595 (Fase
// 27): kalimat lama probe ini — "tipe terdefinisi tanpa `=`
// wajib tidak terhitung" — GUGUR; tipe terdefinisi kini
// TERLARANG dan diuji di probe B-595, bukan di sini. GAGAL di
// base: sapuan alias hanya memeriksa
// CredentialRepo/OAuthRepo.
func TestProbePenjagaB575AliasRegistry(t *testing.T) {
	sumber := salinanDenganSuntikan(t,
		"internal/database/repo/upstream/credentials.go",
		"type R1 = penilaianPergeseranTertunda\n\nvar _ = &R1{}\n\ntype (\n\tR2 = penilaianPergeseranTertunda\n)\n\ntype R3 = *penilaianPergeseranTertunda\n\ntype S = map[string]int")
	pelanggaran := periksaGerbang(sumber)
	gabung := gabungPelanggaran(pelanggaran)
	if n := strings.Count(gabung, "alias repo/registry terlarang"); n != 3 {
		t.Errorf("probe B-575: alias atas penilaianPergeseranTertunda terhitung %d, harap 3 (tunggal+berkelompok+penunjuk; alias atas tipe lain wajib lolos) — pelanggaran: %v", n, pelanggaran)
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

// ---------------------------------------------------------------------------
// Probe pengunci Fase 27 (B-595–B-598)
//
// Aturan yang sama dengan probe Fase 26: satu bentuk bypass per
// probe (rantai komposisi sebagai satu unit), suntikan berupa Go
// yang terurai, pemeriksa gerbang yang sama. Setiap probe di sini
// GAGAL di base Fase 27 (gigi belum ada) dan LULUS sesudah
// perbaikan — dibuktikan dengan lari merah-sebelum / hijau-sesudah
// (lihat badan PR).
// ---------------------------------------------------------------------------

// TestProbePenjagaB595TipeTerdefinisiDanKonversi — B-595: tipe
// terdefinisi atas registry/repo (`type R T`, tanpa `=`) lalu
// konversi eksplisit (`T(R{...})`) membangun registry/repo kerja
// yang lolos semua pola. Deklarasi tipe terdefinisi TERLARANG
// (wajib nol, seperti alias B-561/B-575) DAN konversi `T(...)`
// atas ketiga nama terhitung terpisah. Tipe terdefinisi
// berpenunjuk (`type R *OAuthRepo`) dan bertanda kurung
// (`type R (penilaianPergeseranTertunda)`) ikut terlarang —
// sapuan AST membuka `*`/tanda kurung seperti pada alias.
// Kontrol benign: `var` telanjang (tak terisi) atas registry
// bukan situs dan wajib tidak terhitung. GAGAL di base: sapuan
// alias hanya memeriksa `type X = T`, dan tidak ada hitungan
// konversi.
func TestProbePenjagaB595TipeTerdefinisiDanKonversi(t *testing.T) {
	sumber := salinanDenganSuntikan(t,
		"internal/database/repo/upstream/credentials.go",
		"type b595RDef penilaianPergeseranTertunda\n\nvar _ = penilaianPergeseranTertunda(b595RDef{perProvider: map[string]*penilaianPergeseran{}, qKomit: nil})\n\ntype b595RDefRepo CredentialRepo\n\nvar _ = CredentialRepo(b595RDefRepo{q: nil, cipher: nil, sesudahPemenangBergeser: nil, tertunda: nil})\n\ntype b595RDefParen (penilaianPergeseranTertunda)\n\nvar b595Benign *penilaianPergeseranTertunda")
	pelanggaran := periksaGerbang(sumber)
	gabung := gabungPelanggaran(pelanggaran)
	if n := strings.Count(gabung, "tipe terdefinisi atas repo/registry terlarang"); n != 3 {
		t.Errorf("probe B-595: tipe terdefinisi atas repo/registry terhitung %d, harap 3 (tunggal+repo+bertanda-kurung; var telanjang wajib lolos) — pelanggaran: %v", n, pelanggaran)
	}
	for _, harap := range []string{
		"konversi-registry terhitung 1, terdaftar 0",
		"credentials.go|CredentialRepo terhitung 1, terdaftar 0",
	} {
		if !strings.Contains(gabung, harap) {
			t.Errorf("probe B-595: konversi %q tidak tertangkap penjaga — pelanggaran: %v", harap, pelanggaran)
		}
	}
}

// TestProbePenjagaB596VarTerisiPenugasanField — B-596: `var`
// nilai-nol yang DIISI field (`r.perProvider = ...`,
// `r.qKomit = ...`) adalah registry kerja penuh — premis B-565
// ("var gagal keras") hanya benar selama var TAK TERISI.
// Penugasan field atas kedua nama field itu TERLARANG (wajib
// nol). Kontrol benign: penugasan INDEKS (`t.perProvider[k] =
// ...`, yang dipakai `catat` di pohon nyata) dan perbandingan
// (`== nil`) BUKAN penugasan field dan wajib tidak terhitung;
// `var` telanjang tanpa init juga bukan situs. GAGAL di base:
// deklarasi `var` dan penugasan field tidak dihitung pola apa pun.
func TestProbePenjagaB596VarTerisiPenugasanField(t *testing.T) {
	sumber := salinanDenganSuntikan(t,
		"internal/database/repo/upstream/credentials.go",
		"var b596VarReg penilaianPergeseranTertunda\n\nvar b596VarKosong penilaianPergeseranTertunda\n\nfunc init() {\n\tb596VarReg.perProvider = map[string]*penilaianPergeseran{}\n\tb596VarReg.qKomit = nil\n\tb596VarReg.perProvider[\"x\"] = &penilaianPergeseran{sebelum: \"a\", dalamTx: \"b\"}\n\tif b596VarReg.perProvider == nil {\n\t}\n}")
	pelanggaran := periksaGerbang(sumber)
	gabung := gabungPelanggaran(pelanggaran)
	for _, harap := range []string{
		"credentials.go|perProvider terhitung 1, terdaftar 0",
		"credentials.go|qKomit terhitung 1, terdaftar 0",
	} {
		if !strings.Contains(gabung, harap) {
			t.Errorf("probe B-596: penugasan field %q tidak tertangkap penjaga (penugasan indeks/perbandingan/var kosong wajib lolos) — pelanggaran: %v", harap, pelanggaran)
		}
	}
}

// TestProbePenjagaB597DrainReceiverBebasNama — B-597: panggilan
// `.jalankan(` langsung atas receiver BUKAN `tertunda` lolos
// pola internal yang terikat nama (`tertunda\.(jalankan)\(`).
// Pola dilonggarkan menjadi `\.(jalankan)\s*\(` bebas nama
// (paralel B-566). Panggilan `.jalankanPenilaianSesudahKomit(`
// BUKAN `.jalankan(` (nama berlanjut) dan wajib tidak
// menambah hitungan ini — dibuktikan dengan tidak adanya
// pelanggaran `oauth.go|jalankan` (kedua situs drain + satu
// situs internal terdaftar di sana tetap seimbang). KOREKSI
// atas klaim B-597: "nol situs lain di pohon nyata" SALAH —
// `p.jalankan()` (pemindaiDump, internal/admin/backup_sql.go)
// adalah situs `.jalankan(` yang tidak terkait registry dan
// kini TERDAFTAR, bukan nol. GAGAL di base: pola menuntut
// receiver bernama `tertunda`.
func TestProbePenjagaB597DrainReceiverBebasNama(t *testing.T) {
	sumber := salinanDenganSuntikan(t,
		"internal/database/repo/upstream/credentials.go",
		"var b597Reg penilaianPergeseranTertunda\n\nfunc init() {\n\tb597Reg.jalankan(nil, nil, nil)\n}")
	pelanggaran := periksaGerbang(sumber)
	gabung := gabungPelanggaran(pelanggaran)
	if !strings.Contains(gabung, "credentials.go|jalankan terhitung 1, terdaftar 0") {
		t.Errorf("probe B-597: drain atas receiver bukan `tertunda` tidak tertangkap penjaga — pelanggaran: %v", pelanggaran)
	}
	if strings.Contains(gabung, "oauth.go|jalankan") {
		t.Errorf("probe B-597: situs drain/pemilik di oauth.go ikut berubah hitungannya (jalankanPenilaianSesudahKomit bukan .jalankan) — pelanggaran: %v", pelanggaran)
	}
}

// TestProbePenjagaB598PanggilanPelaporBerSpasi — B-598 (1):
// panggilan mutasi pelapor ber-spasi/berkomentar
// (`r.UpdateSecret (...)`, `r.UpdateSecret /*k*/ (...)`,
// `r.MarkAuthFailure (...)`) lolos pola (1) yang menuntut `(` menempel.
// Obat B-574 diterapkan: pola menjadi
// `\.(UpdateSecret|MarkAuthFailure)\s*\(` DAN memindai teks
// bersih (komentar/string dikupas, paralel B-577) sehingga
// komentar di antara nama dan tanda buka ikut tertutup.
// Kontrol: penyebutan di komentar (`// r.UpdateSecret (x)`)
// BUKAN situs dan wajib tidak terhitung. GAGAL di base: pola
// (1) menuntut `(` menempel dan memindai teks mentah.
func TestProbePenjagaB598PanggilanPelaporBerSpasi(t *testing.T) {
	sumber := salinanDenganSuntikan(t,
		"internal/database/repo/upstream/credentials.go",
		"var b598Secret security.Secret\n\nfunc init() {\n\tvar b598r *CredentialRepo\n\t_ = b598r.UpdateSecret (nil, \"\", b598Secret, nil)\n\t_ = b598r.UpdateSecret /*k*/ (nil, \"\", b598Secret, nil)\n\t_, _ = b598r.MarkAuthFailure (nil, \"\")\n}\n\n// r.UpdateSecret (x) di komentar bukan situs")
	pelanggaran := periksaGerbang(sumber)
	gabung := gabungPelanggaran(pelanggaran)
	for _, harap := range []string{
		"credentials.go|UpdateSecret terhitung 2, terdaftar 0",
		"credentials.go|MarkAuthFailure terhitung 1, terdaftar 0",
	} {
		if !strings.Contains(gabung, harap) {
			t.Errorf("probe B-598 (1): panggilan pelapor ber-spasi/berkomentar %q tidak tertangkap penjaga (komentar wajib lolos) — pelanggaran: %v", harap, pelanggaran)
		}
	}
}

// TestProbePenjagaB598LiteralRepoBerSpasi — B-598 (2): literal
// repo ber-spasi/berkomentar (`&CredentialRepo {}`,
// `&CredentialRepo /*k*/ {}`, `& CredentialRepo{}`) lolos pola
// (2) yang menuntut `{` menempel pada nama. Obat B-574
// diterapkan: pola menjadi
// `&\s*(CredentialRepo|OAuthRepo)\s*\{` DAN memindai teks bersih
// sehingga komentar di antara nama dan tanda buka ikut tertutup.
// Literal BERKUALIFIKASI (`&upstream.CredentialRepo{}`) tetap
// tidak dicakup (lihat komentar kepala) dan bukan bagian probe
// ini. Kontrol: penyebutan di komentar BUKAN situs. GAGAL di
// base: pola (2) menuntut `{` menempel dan memindai teks mentah.
func TestProbePenjagaB598LiteralRepoBerSpasi(t *testing.T) {
	sumber := salinanDenganSuntikan(t,
		"internal/database/repo/upstream/credentials.go",
		"var (\n\t_ = &CredentialRepo {}\n\t_ = &CredentialRepo /*k*/ {}\n\t_ = & CredentialRepo{}\n)\n\n// &CredentialRepo {} di komentar bukan situs")
	pelanggaran := periksaGerbang(sumber)
	gabung := gabungPelanggaran(pelanggaran)
	if !strings.Contains(gabung, "credentials.go|CredentialRepo terhitung 5, terdaftar 2") {
		t.Errorf("probe B-598 (2): literal repo ber-spasi/berkomentar tidak tertangkap penjaga (harap 5 = 2 terdaftar + 3 suntikan; komentar wajib lolos) — pelanggaran: %v", pelanggaran)
	}
}

// TestProbePenjagaB598LiteralStructBerSpasi — B-598 (3b):
// literal struct registry ber-spasi/berkomentar
// (`penilaianPergeseranTertunda {…}`,
// `&penilaianPergeseranTertunda {}`, dan
// `penilaianPergeseranTertunda /*k*/ {…}`) lolos pola (3b)
// yang menuntut `{` menempel. Obat B-574 diterapkan dengan
// cara yang BENAR untuk pola ini: hitungan literal pindah ke
// AST (CompositeLit atas tipe itu) — regex naif
// `&?\s*\bpenilaianPergeseranTertunda\s*\{` SALAH karena ikut
// menghitung baris deklarasi konstruktornya sendiri
// (`func ... *penilaianPergeseranTertunda {`, credentials.go:222)
// sebagai literal; AST hanya menghitung literal komposit
// sungguhan, dan spasi/komentar tidak relevan bagi AST.
// Kontrol: penyebutan di komentar BUKAN situs, dan deklarasi
// konstruktor BUKAN literal. GAGAL di base: pola (3b) menuntut
// `{` menempel.
func TestProbePenjagaB598LiteralStructBerSpasi(t *testing.T) {
	sumber := salinanDenganSuntikan(t,
		"internal/database/repo/upstream/credentials.go",
		"var (\n\t_ = penilaianPergeseranTertunda {qKomit: nil, perProvider: nil}\n\t_ = &penilaianPergeseranTertunda {}\n\t_ = penilaianPergeseranTertunda /*k*/ {}\n)\n\n// penilaianPergeseranTertunda {} di komentar bukan situs")
	pelanggaran := periksaGerbang(sumber)
	gabung := gabungPelanggaran(pelanggaran)
	if !strings.Contains(gabung, "credentials.go|literal-struct terhitung 4, terdaftar 1") {
		t.Errorf("probe B-598 (3b): literal struct ber-spasi/berkomentar tidak tertangkap penjaga (harap 4 = 1 terdaftar + 3 suntikan; komentar/deklarasi konstruktor wajib lolos) — pelanggaran: %v", pelanggaran)
	}
}

// TestProbePenjagaB598DrainInternalBerSpasi — interaksi
// B-597/B-598: drain internal ber-spasi/berkomentar
// (`r.jalankan (...)`, `r.jalankan /*k*/ (...)`) tertangkap
// pola internal yang dilonggarkan (`\.(jalankan)\s*\(` atas
// teks bersih) — DAN terhitung pula sebagai nilai-metode drain
// (penangkap B-576 memeriksa karakter tepat sesudah nama;
// spasi/komentar bukan `(`). Hitungan ganda ini adalah perilaku
// fail-closed yang SUDAH didokumentasikan sebagai kontrol di
// entri B-598 ("panggilan drain ber-spasi terhitung
// nilai-metode — penjaga merah, bukan lolos"), bukan cacat:
// satu situs, dua tuduhan, keduanya merah. Probe ini mengunci
// interaksi itu agar tidak hilang diam-diam. GAGAL di base:
// pola internal menuntut receiver `tertunda` dan `(` menempel.
func TestProbePenjagaB598DrainInternalBerSpasi(t *testing.T) {
	sumber := salinanDenganSuntikan(t,
		"internal/database/repo/upstream/credentials.go",
		"var b598RegDrain penilaianPergeseranTertunda\n\nfunc init() {\n\tb598RegDrain.jalankan (nil, nil, nil)\n\tb598RegDrain.jalankan /*k*/ (nil, nil, nil)\n}")
	pelanggaran := periksaGerbang(sumber)
	gabung := gabungPelanggaran(pelanggaran)
	for _, harap := range []string{
		"credentials.go|jalankan terhitung 2, terdaftar 0",
		"credentials.go|nilai-metode-drain terhitung 2, terdaftar 0",
	} {
		if !strings.Contains(gabung, harap) {
			t.Errorf("probe B-598 (drain ber-spasi): %q tidak tertangkap penjaga — pelanggaran: %v", harap, pelanggaran)
		}
	}
}

// TestProbePenjagaB595B596B597RantaiSenyap — komposisi rantai
// senyap (B-595 + B-596 + `.catat(` tak terhitung + B-597)
// sebagai SATU unit: tipe terdefinisi + konversi, var yang
// diisi field, catat langsung, dan drain atas receiver bukan
// `tertunda`. Unit ini terbukti NOL pelanggaran di base
// (Putaran #14, PB14-20) dan HARUS tertangkap sesudah
// perbaikan. KEJUJURAN CAKUPAN: `.catat(` sendiri TETAP tidak
// dihitung pola apa pun — satu-satunya pemanggil produksinya
// adalah `selesaikanPergeseran` (credentials.go:183), dan
// `catat` atas var TAK TERISI panik (map nil) sehingga bukan
// jalan pintas diam-diam. Rantai tertangkap karena SETIAP
// registry kerja harus dibangun lewat salah satu bentuk
// konstruksi yang terhitung (B-595/B-596 di sini) DAN dikuras
// lewat drain yang terhitung (B-597 di sini): tidak ada mata
// rantai yang lolos tanpa meninggalkan jejak terhitung.
func TestProbePenjagaB595B596B597RantaiSenyap(t *testing.T) {
	sumber := salinanDenganSuntikan(t,
		"internal/database/repo/upstream/credentials.go",
		"type b595Rantai penilaianPergeseranTertunda\n\nvar b596Rantai penilaianPergeseranTertunda\n\nfunc init() {\n\tb596Rantai.perProvider = map[string]*penilaianPergeseran{}\n\tb596Rantai.qKomit = nil\n\tb596Rantai.catat(\"prov\", \"sebelum\", \"dalamTx\")\n\tb596Rantai.jalankan(nil, nil, nil)\n\tvar _ = penilaianPergeseranTertunda(b595Rantai{perProvider: map[string]*penilaianPergeseran{}, qKomit: nil})\n}")
	pelanggaran := periksaGerbang(sumber)
	gabung := gabungPelanggaran(pelanggaran)
	for _, harap := range []string{
		"tipe terdefinisi atas repo/registry terlarang",
		"konversi-registry terhitung 1, terdaftar 0",
		"credentials.go|perProvider terhitung 1, terdaftar 0",
		"credentials.go|qKomit terhitung 1, terdaftar 0",
		"credentials.go|jalankan terhitung 1, terdaftar 0",
	} {
		if !strings.Contains(gabung, harap) {
			t.Errorf("probe rantai senyap: mata rantai %q tidak tertangkap penjaga (`.catat(` sendiri memang tetap tak terhitung — lihat komentar probe) — pelanggaran: %v", harap, pelanggaran)
		}
	}
}
