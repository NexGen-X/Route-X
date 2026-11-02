package admin

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
)

// Berkas ini memuat pemindai keamanan dump SQL untuk jalur restore
// (audit3 B-045 / V-AU-18). Latar belakangnya: dump yang diunggah operator
// disalurkan ke psql, dan psql mengeksekusi baris yang diawali backslash
// sebagai META-COMMAND klien — termasuk `\ ! <perintah>` yang menjalankan
// perintah OS sebagai user proses gateway. Berkas cadangan adalah artefak
// yang bisa berasal dari mesin lain, jadi isinya TIDAK boleh dipercaya
// mentah-mentah hanya karena yang mengunggah adalah admin terautentikasi.
//
// pindaiDumpSQL menolak dump yang memuat:
//   - meta-command psql apa pun (karakter '\' pada keadaan SQL normal),
//     kecuali dua baris utuh yang diterbitkan pg_dump modern sendiri:
//     `\restrict <kunci>` dan `\unrestrict <kunci>` (keduanya perintah
//     pembatas bawaan psql, tidak mengeksekusi apa pun);
//   - pernyataan COPY ... FROM/TO PROGRAM (eksekusi program OS oleh user
//     server PostgreSQL);
//   - dump terpotong (string/komentar/dollar-quote/COPY tak berujung).
//
// Pemindai memahami keadaan leksikal SQL yang diterbitkan pg_dump: string
// '...', identifier "...", komentar -- dan /* */, string dollar-quote
// ($tag$...$tag$), serta blok data COPY ... FROM stdin yang diakhiri baris
// `\.` — di dalam blok data, backslash adalah data biasa dan dilewatkan.

var (
	// errDumpMetaCommand: dump memuat meta-command psql atau COPY PROGRAM.
	errDumpMetaCommand = errors.New("dump mengandung perintah yang dilarang")
	// errDumpTerpotong: dump berakhir di tengah konstruksi leksikal.
	errDumpTerpotong = errors.New("dump terpotong atau tidak lengkap")
)

var (
	// polaBarisRestrict mencocokkan dua baris meta yang diterbitkan pg_dump
	// versi berpatches keamanan (2025+): `\restrict <kunci>` di awal dump dan
	// `\unrestrict <kunci>` di akhir. Kunci adalah token acak tanpa spasi.
	polaBarisRestrict = regexp.MustCompile(`^\\(?:restrict|unrestrict)\s+\S+$`)
	// polaCopyStdin mencocokkan kepala pernyataan COPY yang datanya mengalir
	// lewat stdin — bentuk satu-satunya yang diterbitkan pg_dump teks.
	polaCopyStdin = regexp.MustCompile(`(?is)^\s*COPY\s+.+\s+FROM\s+stdin\s*$`)
	// polaCopyProgram mencocokkan pernyataan COPY yang mengeksekusi program.
	polaCopyProgram = regexp.MustCompile(`(?is)\bCOPY\b.+\bPROGRAM\b`)
)

// batasStmtPindai membatasi buffer pernyataan yang ditahan untuk deteksi
// COPY; kepala COPY dari pg_dump selalu pendek, jadi pernyataan yang lebih
// panjang dari ini pasti bukan kepala COPY dan buffer boleh dibuang.
const batasStmtPindai = 8192

type keadaanPindai int

const (
	pindaiNormal keadaanPindai = iota
	pindaiStringTunggal
	pindaiIdentifier
	pindaiKomentarBaris
	pindaiKomentarBlok
	pindaiDollar
)

// pindaiDumpSQL memindai seluruh aliran dump dan mengembalikan error pada
// temuan pertama yang dilarang. Lihat komentar berkas untuk kontraknya.
func pindaiDumpSQL(r io.Reader) error {
	p := &pemindaiDump{br: bufio.NewReaderSize(r, 64*1024)}
	return p.jalankan()
}

type pemindaiDump struct {
	br        *bufio.Reader
	keadaan   keadaanPindai
	dollar    string // penanda dollar-quote yang sedang terbuka (mis. "$func$")
	stmt      []byte // akumulasi pernyataan berjalan pada keadaan normal
	stmtPenuh bool

	awalBaris   bool // hanya spasi/tab sejak newline terakhir
	pendingCopy bool // kepala COPY ... FROM stdin selesai; data mulai baris berikut
}

func (p *pemindaiDump) jalankan() error {
	p.awalBaris = true
	for {
		if p.pendingCopy {
			if err := p.konsumsiDataCopy(); err != nil {
				return err
			}
			p.pendingCopy = false
			continue
		}
		b, err := p.br.ReadByte()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("membaca dump: %w", err)
		}
		if err := p.langkah(b); err != nil {
			return err
		}
	}
	switch p.keadaan {
	case pindaiNormal, pindaiKomentarBaris:
		return nil
	default:
		return fmt.Errorf("%w: aliran berakhir di tengah konstruksi SQL", errDumpTerpotong)
	}
}

// langkah memproses satu byte pada keadaan leksikal saat ini (di luar blok
// data COPY, yang ditangani konsumsiDataCopy).
func (p *pemindaiDump) langkah(b byte) error {
	switch p.keadaan {
	case pindaiStringTunggal:
		if b == '\'' {
			if nb, ok := p.intip(); ok && nb == '\'' {
				_, _ = p.br.ReadByte() // '' adalah kutip ter-escape, tetap di string
				return nil
			}
			p.keadaan = pindaiNormal
		}
		return nil
	case pindaiIdentifier:
		if b == '"' {
			if nb, ok := p.intip(); ok && nb == '"' {
				_, _ = p.br.ReadByte()
				return nil
			}
			p.keadaan = pindaiNormal
		}
		return nil
	case pindaiKomentarBaris:
		if b == '\n' {
			p.keadaan = pindaiNormal
			p.awalBaris = true
		}
		return nil
	case pindaiKomentarBlok:
		if b == '*' {
			if nb, ok := p.intip(); ok && nb == '/' {
				_, _ = p.br.ReadByte()
				p.keadaan = pindaiNormal
			}
		}
		return nil
	case pindaiDollar:
		if b == '$' && p.cocokPenanda(p.dollar) {
			p.keadaan = pindaiNormal
			p.dollar = ""
		}
		return nil
	}

	// Keadaan normal.
	if b == '\n' {
		p.awalBaris = true
		p.tambahStmt(b)
		return nil
	}
	if b != ' ' && b != '\t' && b != '\r' {
		if b == '\\' {
			return p.tanganiBackslash()
		}
		if p.awalBaris {
			p.awalBaris = false
		}
	}
	switch b {
	case '\'':
		p.keadaan = pindaiStringTunggal
	case '"':
		p.keadaan = pindaiIdentifier
	case '-':
		if nb, ok := p.intip(); ok && nb == '-' {
			_, _ = p.br.ReadByte()
			p.keadaan = pindaiKomentarBaris
			// Penanda & isi komentar TIDAK masuk buffer pernyataan: blok
			// komentar pg_dump selalu mendahului kepala COPY, dan satu
			// karakter '-' yang bocor akan menggagalkan pola COPY.
			return nil
		}
	case '/':
		if nb, ok := p.intip(); ok && nb == '*' {
			_, _ = p.br.ReadByte()
			p.keadaan = pindaiKomentarBlok
			return nil
		}
	case '$':
		if tag, ok := p.bacaPenandaDollar(); ok {
			p.keadaan = pindaiDollar
			p.dollar = tag
			p.tambahStmt(b)
			return nil
		}
	case ';':
		if err := p.akhirPernyataan(); err != nil {
			return err
		}
		return nil
	}
	p.tambahStmt(b)
	return nil
}

// tanganiBackslash dipanggil saat '\' muncul pada keadaan SQL normal. Satu-
// satunya bentuk yang sah dalam dump pg_dump adalah baris utuh
// \restrict/\unrestrict, dan hanya di awal baris logis.
func (p *pemindaiDump) tanganiBackslash() error {
	if !p.awalBaris {
		return fmt.Errorf("%w: meta-command psql di tengah baris", errDumpMetaCommand)
	}
	// Baca sisa baris untuk memeriksa daftar-izinkan.
	sisa, _ := p.br.ReadString('\n')
	baris := "\\" + strings.TrimRight(sisa, "\r\n")
	if polaBarisRestrict.MatchString(strings.TrimSpace(baris)) {
		p.awalBaris = true
		return nil
	}
	return fmt.Errorf("%w: meta-command psql %q", errDumpMetaCommand, potongBaris(baris))
}

// akhirPernyataan mengevaluasi pernyataan yang baru ditutup ';'.
func (p *pemindaiDump) akhirPernyataan() error {
	defer func() {
		p.stmt = p.stmt[:0]
		p.stmtPenuh = false
	}()
	if p.stmtPenuh {
		return nil // terlalu panjang untuk kepala COPY; bukan urusan pemindai
	}
	s := string(p.stmt)
	if polaCopyProgram.MatchString(s) {
		return fmt.Errorf("%w: pernyataan COPY ... PROGRAM", errDumpMetaCommand)
	}
	if polaCopyStdin.MatchString(s) {
		p.pendingCopy = true
	}
	return nil
}

// konsumsiDataCopy menelan baris-baris data COPY sampai baris penutup `\.`.
// Di dalam blok data, seluruh isi adalah data (backslash termasuk data).
func (p *pemindaiDump) konsumsiDataCopy() error {
	for {
		baris, err := p.br.ReadString('\n')
		if strings.TrimRight(baris, "\r\n") == `\.` {
			p.awalBaris = true
			return nil
		}
		if err == io.EOF {
			return fmt.Errorf("%w: blok data COPY tidak diakhiri \\.", errDumpTerpotong)
		}
		if err != nil {
			return fmt.Errorf("membaca dump: %w", err)
		}
	}
}

func (p *pemindaiDump) tambahStmt(b byte) {
	if len(p.stmt) >= batasStmtPindai {
		p.stmtPenuh = true
		return
	}
	p.stmt = append(p.stmt, b)
}

func (p *pemindaiDump) intip() (byte, bool) {
	bs, err := p.br.Peek(1)
	if err != nil || len(bs) == 0 {
		return 0, false
	}
	return bs[0], true
}

// cocokPenanda melaporkan apakah aliran tepat sesudah byte '$' yang baru
// dibaca memuat sisa penanda dollar-quote (tag tanpa '$' pembuka), dan bila
// ya mengonsumsinya. p.dollar berbentuk "$tag$".
func (p *pemindaiDump) cocokPenanda(tag string) bool {
	sisa := tag[1:] // "tag$" — '$' pembuka sudah terbaca
	bs, err := p.br.Peek(len(sisa))
	if err != nil || string(bs) != sisa {
		return false
	}
	_, _ = p.br.Discard(len(sisa))
	return true
}

// bacaPenandaDollar mencoba membaca penanda dollar-quote sesudah '$' yang
// baru dibaca: $$ atau $tag$ dengan tag beraturan identifier. Mengembalikan
// penanda lengkap ("$tag$") dan mengonsumsi sisanya dari aliran bila cocok.
// Parameter posisi ($1) BUKAN dollar-quote dan tidak dicocokkan.
func (p *pemindaiDump) bacaPenandaDollar() (string, bool) {
	bs, err := p.br.Peek(64)
	if err != nil && len(bs) == 0 {
		return "", false
	}
	i := 0
	if i < len(bs) && bs[i] == '$' {
		_, _ = p.br.Discard(1)
		return "$$", true
	}
	for i < len(bs) && i < 62 {
		c := bs[i]
		if c == '$' {
			if i == 0 {
				return "", false
			}
			tag := "$" + string(bs[:i]) + "$"
			_, _ = p.br.Discard(i + 1)
			return tag, true
		}
		if !(c == '_' || ('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z') || (i > 0 && '0' <= c && c <= '9')) {
			return "", false
		}
		i++
	}
	return "", false
}

// potongBaris membatasi panjang baris yang dikutip ke pesan error/log.
func potongBaris(s string) string {
	const maks = 40
	if len(s) > maks {
		return s[:maks] + "…"
	}
	return s
}

// polaSetTransactionTimeout mengenali baris tunggal "SET transaction_timeout
// = ...;" persis seperti yang diterbitkan pg_dump di pembuka dump.
var polaSetTransactionTimeout = regexp.MustCompile(`(?i)^\s*set\s+transaction_timeout\s*=`)

// siapkanBerkasRestore menyalin dump hasil spool ke berkas efektif yang
// diserahkan ke psql, sambil membuang baris pembuka yang tidak dikenal
// server tujuan.
//
// Masalahnya (terbukti di CI): pg_dump 17+ menerbitkan baris
// "SET transaction_timeout = 0;" di pembuka dump, tetapi GUC
// transaction_timeout baru ada di server PostgreSQL 17 — di server yang
// lebih lama baris itu adalah ERROR, dan dengan ON_ERROR_STOP restore gugur
// sebelum pernyataan pertama. Client pg_dump/psql di mesin gateway boleh
// lebih baru daripada servernya (runner CI, image Docker, host campuran),
// jadi jalur restore tidak boleh mengasumsikan keduanya seversi.
//
// Membuang baris itu aman di semua versi: satu-satunya nilai yang pernah
// diterbitkan pg_dump adalah 0 (tanpa batas), yang juga merupakan nilai
// bawaan server. Penyaringan HANYA berlaku di wilayah pembuka dump (baris
// kosong, komentar --, SET ..., set_config, \restrict) — begitu pernyataan
// isi pertama muncul, seluruh sisa berkas disalin apa adanya, sehingga
// baris serupa di dalam badan fungsi tidak pernah tersentuh.
func siapkanBerkasRestore(pathSpool string) (string, error) {
	src, err := os.Open(pathSpool)
	if err != nil {
		return "", err
	}
	defer src.Close()

	dst, err := os.CreateTemp("", "routex-restore-efektif-*.sql")
	if err != nil {
		return "", err
	}
	pathDst := dst.Name()
	selesai := false
	defer func() {
		_ = dst.Close()
		if !selesai {
			_ = os.Remove(pathDst)
		}
	}()

	br := bufio.NewReaderSize(src, 64*1024)
	bw := bufio.NewWriterSize(dst, 64*1024)
	pembuka := true
	for {
		baris, errBaca := br.ReadString('\n')
		if baris != "" {
			buang := false
			if pembuka {
				trim := strings.TrimSpace(baris)
				switch {
				case trim == "" || strings.HasPrefix(trim, "--"):
					// Masih pembuka.
				case polaSetTransactionTimeout.MatchString(baris):
					buang = true
				case polaPembukaDump.MatchString(baris):
					// SET/set_config/\restrict lain: masih pembuka.
				default:
					pembuka = false
				}
			}
			if !buang {
				if _, err := bw.WriteString(baris); err != nil {
					return "", err
				}
			}
		}
		if errBaca == io.EOF {
			break
		}
		if errBaca != nil {
			return "", errBaca
		}
	}
	if err := bw.Flush(); err != nil {
		return "", err
	}
	if err := dst.Close(); err != nil {
		return "", err
	}
	selesai = true
	return pathDst, nil
}

// polaPembukaDump mengenali baris-baris pembuka lain yang diterbitkan
// pg_dump sebelum pernyataan isi pertama: SET <guc> ..., pemanggilan
// set_config, serta \restrict dari pg_dump berpatches keamanan.
var polaPembukaDump = regexp.MustCompile(`(?i)^\s*(set\s+|select\s+pg_catalog\.set_config\(|\\restrict\s)`)
