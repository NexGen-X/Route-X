package cliconfig

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Penyunting JSONC sadar-teks (B-280). OpenCode dimuat dengan
// pengurai toleran JSONC (B-217) tetapi dulu ditulis ulang dari peta
// via json.MarshalIndent — seluruh komentar // dan /* */ milik
// pengguna hilang pada Configure pertama walau datanya selamat.
// Fungsi-fungsi di berkas ini menyunting TEKS asli secara bedah:
// hanya rentang nilai kunci yang dikelola yang diganti/disisipkan/
// dihapus, sehingga komentar, urutan kunci, dan format di luar
// kunci terkelola bertahan apa adanya.
//
// Penyunting hanya dipakai bila hasilnya LOLOS verifikasi semantik
// terhadap jalur peta (lihat penulis opencode): hasil sunting
// diurai ulang dan wajib sama persis dengan peta hasil mutasi yang
// sama. Bila tidak — bentuk JSONC apa pun yang tidak ditangani
// pemindai ini — penulis jatuh ke jalur MarshalIndent lama (data
// tetap benar; komentar hilang seperti perilaku sebelum B-280).

// entriJSONC adalah satu pasangan kunci:nilai di dalam objek JSONC,
// dinyatakan sebagai rentang byte pada teks sumber.
type entriJSONC struct {
	kunci      string
	kunciAwal  int // offset tanda kutip pembuka kunci
	nilaiAwal  int // offset karakter pertama nilai
	nilaiAkhir int // offset sesudah karakter terakhir nilai
}

// lewatiRuangKomentarJSONC memajukan i melewati spasi dan komentar
// (// … dan /* … */) — keduanya bukan token JSON.
func lewatiRuangKomentarJSONC(s string, i int) int {
	for i < len(s) {
		c := s[i]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			i++
			continue
		}
		if c == '/' && i+1 < len(s) && s[i+1] == '/' {
			for i < len(s) && s[i] != '\n' {
				i++
			}
			continue
		}
		if c == '/' && i+1 < len(s) && s[i+1] == '*' {
			akhir := strings.Index(s[i+2:], "*/")
			if akhir < 0 {
				return len(s)
			}
			i += 2 + akhir + 2
			continue
		}
		break
	}
	return i
}

// rentangStringJSONC mengembalikan offset sesudah string JSON yang
// dimulai pada i (s[i] wajib '"'), menghormati escape backslash.
func rentangStringJSONC(s string, i int) (int, error) {
	for j := i + 1; j < len(s); j++ {
		switch s[j] {
		case '\\':
			j++
		case '"':
			return j + 1, nil
		}
	}
	return 0, fmt.Errorf("string JSONC tidak tertutup pada offset %d", i)
}

// rentangNilaiJSONC mengembalikan offset sesudah nilai JSON(C) yang
// dimulai pada i: string, objek/array (dipindai berimbang dengan
// kesadaran string & komentar), atau literal (angka/bool/null).
func rentangNilaiJSONC(s string, i int) (int, error) {
	switch {
	case i >= len(s):
		return 0, fmt.Errorf("nilai JSONC hilang pada offset %d", i)
	case s[i] == '"':
		return rentangStringJSONC(s, i)
	case s[i] == '{' || s[i] == '[':
		buka, tutup := s[i], byte('}')
		if s[i] == '[' {
			tutup = ']'
		}
		kedalaman := 0
		for j := i; j < len(s); {
			c := s[j]
			switch {
			case c == '"':
				akhir, err := rentangStringJSONC(s, j)
				if err != nil {
					return 0, err
				}
				j = akhir
			case c == '/' && j+1 < len(s) && (s[j+1] == '/' || s[j+1] == '*'):
				j = lewatiRuangKomentarJSONC(s, j)
			case c == buka:
				kedalaman++
				j++
			case c == tutup:
				kedalaman--
				j++
				if kedalaman == 0 {
					return j, nil
				}
			default:
				j++
			}
		}
		return 0, fmt.Errorf("objek/array JSONC tidak tertutup pada offset %d", i)
	default:
		// Literal: baca sampai pemisah struktural, spasi, atau
		// awal komentar.
		j := i
		for j < len(s) {
			c := s[j]
			if c == ',' || c == '}' || c == ']' || c == ' ' || c == '\t' || c == '\n' || c == '\r' ||
				(c == '/' && j+1 < len(s) && (s[j+1] == '/' || s[j+1] == '*')) {
				break
			}
			j++
		}
		if j == i {
			return 0, fmt.Errorf("nilai JSONC tidak terbaca pada offset %d", i)
		}
		return j, nil
	}
}

// uraianObjekJSONC mengurai entri-entri objek yang '{' -nya berada
// pada objAwal. Mengembalikan daftar entri dan offset '}' penutup.
// Koma menggantung (trailing comma) ditoleransi seperti tool-nya.
func uraianObjekJSONC(s string, objAwal int) ([]entriJSONC, int, error) {
	if objAwal >= len(s) || s[objAwal] != '{' {
		return nil, 0, fmt.Errorf("offset %d bukan awal objek JSONC", objAwal)
	}
	var entri []entriJSONC
	i := lewatiRuangKomentarJSONC(s, objAwal+1)
	if i < len(s) && s[i] == '}' {
		return entri, i, nil
	}
	for {
		i = lewatiRuangKomentarJSONC(s, i)
		// Koma menggantung: '}' persis sesudah koma terakhir adalah
		// bentuk sah JSONC — objek berakhir di sini.
		if i < len(s) && s[i] == '}' {
			return entri, i, nil
		}
		if i >= len(s) || s[i] != '"' {
			return nil, 0, fmt.Errorf("kunci objek JSONC tidak terbaca pada offset %d", i)
		}
		kunciAwal := i
		kunciAkhir, err := rentangStringJSONC(s, i)
		if err != nil {
			return nil, 0, err
		}
		var kunci string
		if err := json.Unmarshal([]byte(s[kunciAwal:kunciAkhir]), &kunci); err != nil {
			return nil, 0, fmt.Errorf("kunci objek JSONC rusak pada offset %d: %w", kunciAwal, err)
		}
		i = lewatiRuangKomentarJSONC(s, kunciAkhir)
		if i >= len(s) || s[i] != ':' {
			return nil, 0, fmt.Errorf("titik dua objek JSONC hilang pada offset %d", i)
		}
		i = lewatiRuangKomentarJSONC(s, i+1)
		nilaiAwal := i
		nilaiAkhir, err := rentangNilaiJSONC(s, i)
		if err != nil {
			return nil, 0, err
		}
		entri = append(entri, entriJSONC{
			kunci: kunci, kunciAwal: kunciAwal,
			nilaiAwal: nilaiAwal, nilaiAkhir: nilaiAkhir,
		})
		i = lewatiRuangKomentarJSONC(s, nilaiAkhir)
		if i < len(s) && s[i] == ',' {
			i++
			continue
		}
		if i < len(s) && s[i] == '}' {
			return entri, i, nil
		}
		return nil, 0, fmt.Errorf("pemisah objek JSONC tidak terbaca pada offset %d", i)
	}
}

// indentasiBaris mengembalikan indentasi (spasi/tab di awal baris)
// dari baris yang memuat offset i.
func indentasiBaris(s string, i int) string {
	awal := strings.LastIndex(s[:i], "\n") + 1
	j := awal
	for j < len(s) && (s[j] == ' ' || s[j] == '\t') {
		j++
	}
	return s[awal:j]
}

// tetapkanKunciJSONC menetapkan kunci pada objek yang '{' -nya di
// objAwal ke nilaiJSON (teks JSON yang sudah ter-encode): nilai lama
// diganti rentangnya, kunci yang belum ada disisipkan sebagai entri
// terakhir dengan indentasi mengikuti entri yang ada. Komentar dan
// format di luar rentang itu tidak disentuh.
func tetapkanKunciJSONC(s string, objAwal int, kunci, nilaiJSON string) (string, error) {
	entri, tutupKurung, err := uraianObjekJSONC(s, objAwal)
	if err != nil {
		return "", err
	}
	kunciJSON, err := json.Marshal(kunci)
	if err != nil {
		return "", err
	}
	for _, e := range entri {
		if e.kunci == kunci {
			return s[:e.nilaiAwal] + nilaiJSON + s[e.nilaiAkhir:], nil
		}
	}
	if len(entri) > 0 {
		indentAnak := indentasiBaris(s, entri[0].kunciAwal)
		terakhir := entri[len(entri)-1]
		sisip := ",\n" + indentAnak + string(kunciJSON) + ": " + nilaiJSON
		return s[:terakhir.nilaiAkhir] + sisip + s[terakhir.nilaiAkhir:], nil
	}
	// Objek kosong: celah antara { dan } hanya boleh berisi spasi/
	// komentar — komentar dipertahankan, entri ditaruh sesudahnya.
	indentInduk := indentasiBaris(s, objAwal)
	celah := strings.TrimRight(s[objAwal+1:tutupKurung], " \t\r\n")
	sisip := "\n" + indentInduk + "  " + string(kunciJSON) + ": " + nilaiJSON + "\n" + indentInduk
	return s[:objAwal+1] + celah + sisip + s[tutupKurung:], nil
}

// hapusKunciJSONC menghapus satu entri kunci dari objek. Koma
// sesudahnya ikut dibuang; untuk entri terakhir, koma sebelumnya
// yang dibuang — kecuali bila ada komentar di antaranya (komentar
// tidak pernah dimakan: koma menggantung yang tersisa sah di
// JSONC dan ditoleransi pengurai tool maupun uraiJSONCToleran).
func hapusKunciJSONC(s string, objAwal int, kunci string) (string, error) {
	entri, _, err := uraianObjekJSONC(s, objAwal)
	if err != nil {
		return "", err
	}
	for idx, e := range entri {
		if e.kunci != kunci {
			continue
		}
		// Koma sesudah nilai (hanya spasi/tab yang boleh menghalangi)?
		j := e.nilaiAkhir
		for j < len(s) && (s[j] == ' ' || s[j] == '\t') {
			j++
		}
		if j < len(s) && s[j] == ',' {
			return s[:e.kunciAwal] + s[j+1:], nil
		}
		if idx > 0 {
			// Koma sebelumnya, bila celahnya murni spasi + satu koma.
			sebelum := entri[idx-1]
			celah := s[sebelum.nilaiAkhir:e.kunciAwal]
			if k := strings.Index(celah, ","); k >= 0 && strings.Trim(celah, " \t\r\n,") == "" &&
				strings.Count(celah, ",") == 1 {
				return s[:sebelum.nilaiAkhir] + celah[k+1:] + s[e.nilaiAkhir:], nil
			}
		}
		return s[:e.kunciAwal] + s[e.nilaiAkhir:], nil
	}
	return s, nil
}

// cariObjekAnakJSONC mencari '{' objek anak bernama kunci pada
// objek induk. ok=false bila kunci tidak ada; bukanObjek=true bila
// ada tetapi nilainya bukan objek.
func cariObjekAnakJSONC(s string, objAwal int, kunci string) (anakAwal int, ok bool, bukanObjek bool, err error) {
	entri, _, err := uraianObjekJSONC(s, objAwal)
	if err != nil {
		return 0, false, false, err
	}
	for _, e := range entri {
		if e.kunci == kunci {
			if s[e.nilaiAwal] == '{' {
				return e.nilaiAwal, true, false, nil
			}
			return 0, true, true, nil
		}
	}
	return 0, false, false, nil
}

// pastikanObjekAnakJSONC memastikan kunci pada objek induk memuat
// sebuah objek (dibuat {} bila belum ada; nilai non-objek diganti
// {}), dan mengembalikan teks baru + offset '{' anak pada teks
// baru itu.
func pastikanObjekAnakJSONC(s string, objAwal int, kunci string) (string, int, error) {
	anakAwal, ok, bukanObjek, err := cariObjekAnakJSONC(s, objAwal, kunci)
	if err != nil {
		return "", 0, err
	}
	if ok && !bukanObjek {
		return s, anakAwal, nil
	}
	s, err = tetapkanKunciJSONC(s, objAwal, kunci, "{}")
	if err != nil {
		return "", 0, err
	}
	anakAwal, ok, bukanObjek, err = cariObjekAnakJSONC(s, objAwal, kunci)
	if err != nil || !ok || bukanObjek {
		return "", 0, fmt.Errorf("objek anak %q tidak terbentuk sesudah penetapan", kunci)
	}
	return s, anakAwal, nil
}

// jsoncMengandungKomentar melaporkan apakah teks memuat komentar
// // atau /* */ di luar string — satu-satunya keadaan di mana
// penulisan ulang jalur peta kehilangan sesuatu (B-280).
func jsoncMengandungKomentar(s string) bool {
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '"':
			akhir, err := rentangStringJSONC(s, i)
			if err != nil {
				return false
			}
			i = akhir - 1
		case '/':
			if i+1 < len(s) && (s[i+1] == '/' || s[i+1] == '*') {
				return true
			}
		}
	}
	return false
}

// akarObjekJSONC mengembalikan offset '{' objek akar dokumen.
func akarObjekJSONC(s string) (int, error) {
	i := lewatiRuangKomentarJSONC(s, 0)
	if i >= len(s) || s[i] != '{' {
		return 0, fmt.Errorf("dokumen JSONC tidak berakar objek")
	}
	return i, nil
}
