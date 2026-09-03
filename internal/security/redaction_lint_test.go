package security_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// Test ini adalah lint, bukan uji perilaku: ia membaca SELURUH sumber repo dan
// menggagalkan build test bila ada tipe yang melanggar kewajiban redaksi.
//
// Alasannya sebuah cacat sungguhan. security.Secret menutup semua jalur keluaran umum
// lewat metodenya sendiri, tetapi fmt TIDAK BOLEH memanggil metode pada nilai yang
// diperoleh dari field tak diekspor — reflect menandai nilai seperti itu read-only dan
// CanInterface() bernilai false. Akibatnya:
//
//	type Provider struct { credential security.Secret }  // tak diekspor
//	fmt.Sprintf("%v", p)  // => {sk-RAHASIA-ASLI}, BUKAN {[REDACTED]}
//
// Bandingkan dengan field diekspor, yang aman karena fmt boleh memanggil String():
//
//	type Provider struct { Credential security.Secret }
//	fmt.Sprintf("%v", p)  // => {[REDACTED]}
//
// Celah ini tidak tertangkap uji Secret berdiri sendiri, dan adapter Anthropic serta
// Google sungguh-sungguh membocorkan API key lewat "%v" sebelum test ini ada.
//
// Aturan yang ditegakkan: setiap tipe struct yang punya field TAK DIEKSPOR yang
// menjangkau security.Secret — langsung maupun lewat struct lain di repo ini — wajib
// menyediakan String, GoString, dan LogValue dengan RECEIVER NILAI.
//
// Receiver nilai, bukan pointer, karena dengan receiver pointer "%v" pada T (bukan *T)
// tidak melewati metode dan tetap bocor.

const (
	modulePath  = "github.com/NexGen-X/Route-X"
	securityPkg = modulePath + "/internal/security"
	secretType  = "Secret"
)

// metodeWajib adalah jalur keluaran yang harus ditutup sendiri oleh tipe pemuat rahasia.
var metodeWajib = []string{"String", "GoString", "LogValue"}

// tipeStruct adalah satu deklarasi struct yang ditemukan di sumber.
type tipeStruct struct {
	pkg    string // import path paket
	nama   string
	posisi string   // file:baris, untuk pesan kegagalan yang bisa diklik
	field  []fieldS //
}

// fieldS adalah satu field struct beserta tipe yang sudah diselesaikan.
type fieldS struct {
	nama      string
	diekspor  bool
	tipePkg   string // import path tipe field; kosong bila tipe dasar
	tipeNama  string
	adaTipeNm bool
	// pointer menandai field yang tipenya di balik * (atau slice/map berisi pointer).
	// Bedanya nyata di dua tempat: fmt tidak menelusuri pointer bersarang pada %v, dan
	// lock di balik pointer tidak membuat struct pemuatnya haram disalin.
	pointer bool
}

// kunci mengidentifikasi satu tipe lintas paket.
type kunci struct{ pkg, nama string }

func TestRedaksiFieldTakDiekspor(t *testing.T) {
	akar := akarModul(t)

	structs, metode := bacaRepo(t, akar)

	// Indeks struct per (paket, nama) supaya penjangkauan bisa menyeberang paket.
	indeks := make(map[kunci]*tipeStruct, len(structs))
	for i := range structs {
		indeks[kunci{structs[i].pkg, structs[i].nama}] = &structs[i]
	}

	menjangkau := pembacaJangkauan(indeks)
	punyaLock := pembacaLock(indeks)

	var pelanggaran []string
	for i := range structs {
		s := &structs[i]

		// Cari field tak diekspor yang menjangkau Secret.
		var lewat []string
		for _, f := range s.field {
			if f.diekspor || !f.adaTipeNm {
				continue
			}
			if f.tipePkg == securityPkg && f.tipeNama == secretType {
				lewat = append(lewat, f.nama+" (security.Secret)")
				continue
			}
			if menjangkau(kunci{f.tipePkg, f.tipeNama}) {
				lewat = append(lewat, f.nama+" ("+f.tipeNama+" memuat security.Secret)")
			}
		}
		if len(lewat) == 0 {
			continue
		}

		// Tipe itu wajib menutup sendiri seluruh jalur keluaran.
		//
		// Bentuk receiver-nya ditentukan apakah tipe ini boleh disalin. Struct yang memuat
		// lock WAJIB memakai receiver pointer, karena vet menolak yang sebaliknya — dan
		// itu aman justru karena vet juga melarang siapa pun menyalinnya, sehingga "%v"
		// pada bentuk nilainya tidak bisa ditulis tanpa gate ikut merah.
		wajibPointer := punyaLock(kunci{s.pkg, s.nama})
		bentukWajib, sebut := "nilai", "receiver pointer, harus nilai"
		if wajibPointer {
			bentukWajib, sebut = "pointer", "receiver nilai, harus pointer (struct memuat lock)"
		}

		var kurang []string
		for _, m := range metodeWajib {
			penerima, punya := metode[kunci{s.pkg, s.nama + "." + m}]
			switch {
			case !punya:
				kurang = append(kurang, m+" (tidak ada)")
			case penerima != bentukWajib:
				kurang = append(kurang, m+" ("+sebut+")")
			}
		}
		if len(kurang) > 0 {
			pelanggaran = append(pelanggaran, s.posisi+": "+s.pkg+"."+s.nama+
				"\n    field tak diekspor: "+strings.Join(lewat, ", ")+
				"\n    metode kurang     : "+strings.Join(kurang, ", "))
		}
	}

	if len(pelanggaran) > 0 {
		t.Fatalf("%d tipe memuat rahasia di field tak diekspor tanpa redaksi tingkat struct.\n"+
			"fmt tidak memanggil metode pada field tak diekspor, jadi \"%%v\" pada tipe ini\n"+
			"mencetak rahasianya apa adanya. Tambahkan String, GoString, dan LogValue dengan\n"+
			"receiver NILAI (lihat internal/providers/openai.Provider sebagai contoh).\n\n%s",
			len(pelanggaran), strings.Join(pelanggaran, "\n\n"))
	}

	// Jaring pengaman: kalau lint tidak menemukan satu pun tipe pemuat rahasia, ada yang
	// salah dengan pembacaannya sendiri — dan lint yang tidak memeriksa apa pun selalu
	// hijau. Repo ini punya beberapa, jadi nol berarti lint-nya rusak, bukan repo bersih.
	var jumlahDijaga int
	for i := range structs {
		s := &structs[i]
		for _, f := range s.field {
			if !f.diekspor && f.adaTipeNm && f.tipePkg == securityPkg && f.tipeNama == secretType {
				jumlahDijaga++
				break
			}
		}
	}
	if jumlahDijaga == 0 {
		t.Fatal("lint tidak menemukan satu pun field tak diekspor bertipe security.Secret; " +
			"pembacaan sumbernya kemungkinan rusak, bukan repo yang bersih")
	}
	t.Logf("lint memeriksa %d struct; %d di antaranya memuat security.Secret di field tak diekspor",
		len(structs), jumlahDijaga)
}

// akarModul mencari direktori yang memuat go.mod, mulai dari direktori test.
func akarModul(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		induk := filepath.Dir(dir)
		if induk == dir {
			t.Fatal("go.mod tidak ditemukan di atas direktori test")
		}
		dir = induk
	}
}

// bacaRepo mengurai seluruh sumber non-test dan mengembalikan daftar struct beserta
// peta metode. Peta metode berkunci (paket, "Tipe.Metode") bernilai "nilai" atau
// "pointer" sesuai bentuk receiver-nya.
func bacaRepo(t *testing.T, akar string) ([]tipeStruct, map[kunci]string) {
	t.Helper()

	var structs []tipeStruct
	metode := map[kunci]string{}
	fset := token.NewFileSet()

	err := filepath.WalkDir(akar, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// Direktori yang tidak memuat sumber produk.
			switch d.Name() {
			case ".git", "web", "node_modules", "docs", "scripts":
				if path != akar {
					return fs.SkipDir
				}
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		f, perr := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if perr != nil {
			return perr
		}

		rel, _ := filepath.Rel(akar, filepath.Dir(path))
		pkgPath := modulePath
		if rel != "." {
			pkgPath = modulePath + "/" + filepath.ToSlash(rel)
		}
		impor := petaImpor(f)

		for _, dekl := range f.Decls {
			switch d := dekl.(type) {
			case *ast.GenDecl:
				kumpulkanStruct(&structs, d, fset, pkgPath, impor)
			case *ast.FuncDecl:
				kumpulkanMetode(metode, d, pkgPath)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("membaca sumber repo: %v", err)
	}
	if len(structs) == 0 {
		t.Fatal("tidak ada struct yang terbaca dari sumber repo")
	}
	return structs, metode
}

// petaImpor memetakan nama paket yang dipakai di file ke import path-nya.
func petaImpor(f *ast.File) map[string]string {
	out := make(map[string]string, len(f.Imports))
	for _, imp := range f.Imports {
		jalur, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			continue
		}
		nama := jalur[strings.LastIndex(jalur, "/")+1:]
		if imp.Name != nil {
			nama = imp.Name.Name
		}
		out[nama] = jalur
	}
	return out
}

// kumpulkanStruct mencatat setiap "type X struct { ... }" dalam satu deklarasi.
func kumpulkanStruct(out *[]tipeStruct, d *ast.GenDecl, fset *token.FileSet, pkgPath string, impor map[string]string) {
	if d.Tok != token.TYPE {
		return
	}
	for _, spec := range d.Specs {
		ts, ok := spec.(*ast.TypeSpec)
		if !ok {
			continue
		}
		st, ok := ts.Type.(*ast.StructType)
		if !ok {
			continue
		}

		s := tipeStruct{
			pkg:    pkgPath,
			nama:   ts.Name.Name,
			posisi: fset.Position(ts.Pos()).String(),
		}
		for _, f := range st.Fields.List {
			pkgTipe, namaTipe, ada, ptr := selesaikanTipe(f.Type, pkgPath, impor)
			if len(f.Names) == 0 {
				// Field tersemat: namanya adalah nama tipenya.
				s.field = append(s.field, fieldS{
					nama: namaTipe, diekspor: diekspor(namaTipe),
					tipePkg: pkgTipe, tipeNama: namaTipe, adaTipeNm: ada, pointer: ptr,
				})
				continue
			}
			for _, n := range f.Names {
				s.field = append(s.field, fieldS{
					nama: n.Name, diekspor: diekspor(n.Name),
					tipePkg: pkgTipe, tipeNama: namaTipe, adaTipeNm: ada, pointer: ptr,
				})
			}
		}
		*out = append(*out, s)
	}
}

// kumpulkanMetode mencatat metode beserta bentuk receiver-nya.
func kumpulkanMetode(out map[kunci]string, d *ast.FuncDecl, pkgPath string) {
	if d.Recv == nil || len(d.Recv.List) != 1 {
		return
	}
	bentuk := "nilai"
	tipe := d.Recv.List[0].Type
	if star, ok := tipe.(*ast.StarExpr); ok {
		bentuk = "pointer"
		tipe = star.X
	}
	// Receiver bergenerik: ambil tipe dasarnya.
	if idx, ok := tipe.(*ast.IndexExpr); ok {
		tipe = idx.X
	}
	id, ok := tipe.(*ast.Ident)
	if !ok {
		return
	}
	out[kunci{pkgPath, id.Name + "." + d.Name.Name}] = bentuk
}

// selesaikanTipe membuka pointer, slice, array, dan map untuk sampai ke tipe bernama,
// lalu menyelesaikan kualifikasi paketnya.
//
// Pembukaan itu perlu karena rahasia yang disimpan sebagai []security.Secret atau
// map[string]security.Secret sama bocornya lewat "%v" seperti field tunggal.
func selesaikanTipe(e ast.Expr, pkgSaatIni string, impor map[string]string) (pkg, nama string, ada, pointer bool) {
	for {
		switch t := e.(type) {
		case *ast.StarExpr:
			pointer = true
			e = t.X
		case *ast.ArrayType:
			e = t.Elt
		case *ast.MapType:
			e = t.Value
		case *ast.ParenExpr:
			e = t.X
		case *ast.Ident:
			// Tipe tanpa kualifikasi: milik paket file ini sendiri.
			return pkgSaatIni, t.Name, true, pointer
		case *ast.SelectorExpr:
			id, ok := t.X.(*ast.Ident)
			if !ok {
				return "", "", false, pointer
			}
			jalur, ok := impor[id.Name]
			if !ok {
				return "", "", false, pointer
			}
			return jalur, t.Sel.Name, true, pointer
		default:
			return "", "", false, pointer
		}
	}
}

// diekspor melaporkan apakah pengenal ini diekspor.
func diekspor(nama string) bool {
	return nama != "" && nama[0] >= 'A' && nama[0] <= 'Z'
}

// pembacaJangkauan mengembalikan predikat "tipe ini memuat security.Secret di suatu
// tempat", menelusuri field bersarang lintas paket.
//
// Visibilitas field TIDAK diperiksa di sini: begitu penelusuran masuk lewat satu field
// tak diekspor, reflect menandai SELURUH nilai di bawahnya read-only, jadi String() milik
// Secret maupun milik struct perantara tidak akan dipanggil sampai sedalam apa pun.
//
// Hasilnya di-memo, dan tipe yang sedang ditelusuri dianggap "belum terbukti" supaya
// struct yang menunjuk dirinya sendiri (mis. lewat pointer) tidak membuat rekursi tak
// berujung.
func pembacaJangkauan(indeks map[kunci]*tipeStruct) func(kunci) bool {
	memo := map[kunci]bool{}
	jalan := map[kunci]bool{}

	var jangkau func(k kunci) bool
	jangkau = func(k kunci) bool {
		if k.pkg == securityPkg && k.nama == secretType {
			return true
		}
		if v, ok := memo[k]; ok {
			return v
		}
		s, ok := indeks[k]
		if !ok {
			// Tipe di luar repo (stdlib, dependensi). Tidak bisa memuat security.Secret.
			return false
		}
		if jalan[k] {
			return false
		}
		jalan[k] = true
		defer delete(jalan, k)

		hasil := slices.ContainsFunc(s.field, func(f fieldS) bool {
			return f.adaTipeNm && jangkau(kunci{f.tipePkg, f.tipeNama})
		})
		memo[k] = hasil
		return hasil
	}
	return jangkau
}

// syncPkg adalah paket pemilik tipe lock bawaan.
const syncPkg = "sync"

// tipeLock adalah tipe sync yang membuat struct pemuatnya haram disalin.
var tipeLock = []string{"Mutex", "RWMutex", "Once", "WaitGroup", "Cond", "Pool", "Map"}

// pembacaLock mengembalikan predikat "menyalin tipe ini adalah pelanggaran copylocks".
//
// Ini yang menentukan bentuk receiver yang WAJIB dipakai, dan karena itu tidak boleh
// ditebak: go vet menolak metode ber-receiver nilai pada struct yang memuat lock
// ("String passes lock by value"), jadi memaksakan receiver nilai di sana membuat gate
// merah. Sebaliknya, struct seperti itu justru tidak perlu receiver nilai — vet sudah
// melarang SIAPA PUN menyalinnya, sehingga jalur "%v" pada nilai (bukan pointer) tidak
// bisa ditulis tanpa vet ikut menggagalkannya.
//
// Hanya field bertipe NILAI yang dihitung: *sync.Mutex tidak membuat pemuatnya haram
// disalin.
func pembacaLock(indeks map[kunci]*tipeStruct) func(kunci) bool {
	memo := map[kunci]bool{}
	jalan := map[kunci]bool{}

	var punya func(k kunci) bool
	punya = func(k kunci) bool {
		if k.pkg == syncPkg && slices.Contains(tipeLock, k.nama) {
			return true
		}
		if v, ok := memo[k]; ok {
			return v
		}
		s, ok := indeks[k]
		if !ok {
			return false
		}
		if jalan[k] {
			return false
		}
		jalan[k] = true
		defer delete(jalan, k)

		hasil := slices.ContainsFunc(s.field, func(f fieldS) bool {
			return f.adaTipeNm && !f.pointer && punya(kunci{f.tipePkg, f.tipeNama})
		})
		memo[k] = hasil
		return hasil
	}
	return punya
}
