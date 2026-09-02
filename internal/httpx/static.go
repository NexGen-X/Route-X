package httpx

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"time"
)

const (
	defaultIndexFile = "index.html"

	// immutableMaxAge satu tahun, batas praktis yang dianjurkan RFC 9111 untuk aset
	// yang namanya memuat hash isi.
	immutableMaxAge = 365 * 24 * time.Hour

	// mutableMaxAge untuk aset tanpa hash di nama (favicon.ico, robots.txt): cukup
	// pendek supaya penggantian terlihat dalam waktu wajar, cukup panjang supaya
	// tidak diminta ulang setiap navigasi.
	mutableMaxAge = time.Hour

	// Batas panjang token hash yang dianggap wajar. Vite memakai 8 karakter, bundler
	// lain ada yang memakai 6 sampai 32.
	minHashLen = 6
	maxHashLen = 64
)

// apiPathPrefixes adalah rute yang selalu milik router API.
//
// SPAHandler dipasang sebagai fallback di akar, sesudah semua rute API terdaftar.
// Kalau permintaan ke prefix ini sampai ke sini, artinya endpoint-nya memang tidak
// ada — dan klien API harus menerima JSON 404, bukan index.html yang tidak bisa
// diurai dan menghasilkan pesan galat menyesatkan di sisi klien.
var apiPathPrefixes = []string{"/v1/", "/api/"}

// SPAOptions mengatur perilaku SPAHandler.
type SPAOptions struct {
	// Index adalah dokumen fallback untuk rute sisi klien. Kosong berarti "index.html".
	Index string
	// ImmutableMaxAge adalah umur cache aset ber-hash. Nol memakai default satu tahun.
	ImmutableMaxAge time.Duration
}

// SPAHandler menyajikan aset frontend dari fsys.
//
// fsys diberikan dari luar dan bukan hasil go:embed di paket ini, dengan dua alasan:
// paket ini bisa diuji dengan fstest.MapFS tanpa build frontend, dan keputusan
// menyematkan web/dist tetap milik main.go.
//
// Perilakunya:
//
//   - File yang ada disajikan langsung, lengkap dengan penanganan Range dan
//     If-Modified-Since dari http.ServeContent.
//   - Path yang tidak ada dan tidak berekstensi dianggap rute sisi klien dan dijawab
//     dengan Index berstatus 200, sehingga router di browser bisa mengambil alih.
//   - Path berekstensi yang tidak ada tetap 404. Mengembalikan index.html untuk
//     <script src> yang salah hanya menghasilkan galat parse yang membingungkan.
//   - Direktori tidak pernah dilayani sebagai daftar isi.
//
// Handler ini dipasang sebagai fallback (mis. router.NotFound), bukan di depan
// router API.
func SPAHandler(fsys fs.FS, opts SPAOptions) http.Handler {
	index := opts.Index
	if index == "" {
		index = defaultIndexFile
	}
	immutable := opts.ImmutableMaxAge
	if immutable <= 0 {
		immutable = immutableMaxAge
	}
	return &spaHandler{
		fsys:            fsys,
		index:           index,
		immutableHeader: fmt.Sprintf("public, max-age=%d, immutable", int64(immutable.Seconds())),
		mutableHeader:   fmt.Sprintf("public, max-age=%d", int64(mutableMaxAge.Seconds())),
	}
}

type spaHandler struct {
	fsys            fs.FS
	index           string
	immutableHeader string
	mutableHeader   string
}

func (h *spaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		WriteError(w, r, http.StatusMethodNotAllowed, ErrTypeInvalidRequest, CodeMethodNotAllowed,
			"metode request ini tidak didukung untuk aset statis")
		return
	}

	name, ok := assetName(r.URL.Path)
	if !ok {
		BadRequest(w, r, "invalid_path", "path yang diminta tidak sah")
		return
	}
	if name == "" {
		name = h.index
	}

	if h.serveFile(w, r, name, name == h.index) {
		return
	}

	// Berkas tidak ada. Hanya path tanpa ekstensi yang boleh jatuh ke index.html.
	if path.Ext(name) != "" || isAPIPath(r.URL.Path) {
		NotFound(w, r, "")
		return
	}
	if !h.serveFile(w, r, h.index, true) {
		// Bahkan index.html tidak ada: frontend belum dibuild atau fsys salah.
		NotFound(w, r, "")
	}
}

// serveFile menyajikan satu berkas dan melaporkan apakah berkasnya ada. Header
// respons baru disentuh setelah dipastikan berkasnya bisa disajikan, sehingga
// pemanggil masih bebas menulis respons lain saat hasilnya false.
func (h *spaHandler) serveFile(w http.ResponseWriter, r *http.Request, name string, isIndex bool) bool {
	if h.fsys == nil {
		return false
	}
	f, err := h.fsys.Open(name)
	if err != nil {
		return false
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil || info.IsDir() {
		// Direktori tidak pernah disajikan: daftar isi membocorkan struktur build dan
		// tidak ada gunanya untuk SPA.
		return false
	}

	content, ok := f.(io.ReadSeeker)
	if !ok {
		// fs.FS tidak mewajibkan Seek. Baca ke memori agar http.ServeContent tetap
		// bisa menangani Range dan permintaan bersyarat.
		data, err := io.ReadAll(f)
		if err != nil {
			return false
		}
		content = bytes.NewReader(data)
	}

	w.Header().Set("Cache-Control", h.cacheControl(name, isIndex))
	http.ServeContent(w, r, path.Base(name), info.ModTime(), content)
	return true
}

// cacheControl memilih kebijakan cache berdasarkan nama berkas.
//
// index.html adalah yang memetakan nama aset ber-hash ke URL-nya, jadi dokumen ini
// harus selalu divalidasi ulang; kalau di-cache, browser akan terus memuat bundel
// lama setelah deploy walaupun aset barunya sudah tersedia.
func (h *spaHandler) cacheControl(name string, isIndex bool) string {
	base := path.Base(name)
	if isIndex || strings.EqualFold(path.Ext(base), ".html") {
		return "no-cache"
	}
	if looksHashed(base) {
		return h.immutableHeader
	}
	return h.mutableHeader
}

// looksHashed menebak apakah nama berkas memuat hash build, misalnya
// "index-a1b2c3.js" atau "vendor.4f8c1e2d.css". Hanya berkas seperti itu yang boleh
// di-cache selamanya, karena isi untuk satu nama tidak akan pernah berubah.
//
// Aturannya sengaja ketat — token harus panjangnya masuk akal, seluruhnya
// alfanumerik, dan memuat minimal satu digit — supaya nama biasa seperti
// "chart-legend.js" tidak salah dikenali. Kalau tebakan gagal, akibatnya hanya cache
// yang lebih pendek; tidak ada aset basi yang tersaji.
func looksHashed(base string) bool {
	stem := strings.TrimSuffix(base, path.Ext(base))
	sep := strings.LastIndexAny(stem, "-.")
	if sep < 0 || sep == len(stem)-1 {
		return false
	}

	token := stem[sep+1:]
	if len(token) < minHashLen || len(token) > maxHashLen {
		return false
	}

	hasDigit := false
	for i := 0; i < len(token); i++ {
		c := token[i]
		switch {
		case c >= '0' && c <= '9':
			hasDigit = true
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c == '_':
		default:
			return false
		}
	}
	return hasDigit
}

func isAPIPath(urlPath string) bool {
	for _, prefix := range apiPathPrefixes {
		if strings.HasPrefix(urlPath, prefix) {
			return true
		}
	}
	return false
}

// assetName mengubah path request menjadi nama berkas untuk fs.FS, atau melaporkan
// false bila path-nya harus ditolak. Nilai "" dengan ok true berarti akar situs.
//
// Pemeriksaan dilakukan pada path yang sudah didekode net/http, jadi "%2e%2e%2f"
// ikut tertangkap. Path bermasalah DITOLAK, bukan dibersihkan dengan path.Clean:
// path.Clean("/../etc/passwd") menghasilkan "/etc/passwd", sehingga percobaan
// traversal berubah menjadi permintaan yang tampak sah dan dilayani tanpa jejak.
// Menolak membuat percobaan itu terlihat sebagai 400 di log.
func assetName(urlPath string) (string, bool) {
	// Path absolut wajib: bentuk lain berarti request tidak lewat parser HTTP biasa.
	if urlPath == "" || urlPath[0] != '/' {
		return "", false
	}
	// NUL bisa memotong nama berkas di lapisan yang memakai C string.
	if strings.IndexByte(urlPath, 0) >= 0 {
		return "", false
	}

	trimmed := strings.TrimPrefix(urlPath, "/")
	if trimmed == "" {
		return "", true
	}

	segments := strings.Split(trimmed, "/")
	// Satu garis miring di akhir wajar untuk rute sisi klien seperti "/settings/".
	if segments[len(segments)-1] == "" {
		segments = segments[:len(segments)-1]
	}
	if len(segments) == 0 {
		return "", true
	}
	for _, segment := range segments {
		switch segment {
		case "", ".", "..":
			// Segmen kosong ("//a"), "." dan ".." tidak pernah dihasilkan bundler.
			return "", false
		}
	}

	name := strings.Join(segments, "/")
	// Jaring terakhir: fs.FS hanya menerima path relatif, bersih, dan berpemisah "/".
	if !fs.ValidPath(name) {
		return "", false
	}
	return name, true
}
