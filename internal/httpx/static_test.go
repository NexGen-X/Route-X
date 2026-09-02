package httpx

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

const (
	isiIndex  = "<!doctype html><div id=\"root\"></div>"
	isiScript = "console.log('route-x')"
)

func webDist() fstest.MapFS {
	return fstest.MapFS{
		"index.html":              &fstest.MapFile{Data: []byte(isiIndex), ModTime: time.Unix(1700000000, 0)},
		"assets/index-a1b2c3.js":  &fstest.MapFile{Data: []byte(isiScript)},
		"assets/index-a1b2c3.css": &fstest.MapFile{Data: []byte("body{margin:0}")},
		"assets/logo.svg":         &fstest.MapFile{Data: []byte("<svg/>")},
		"favicon.ico":             &fstest.MapFile{Data: []byte("ico")},
		"robots.txt":              &fstest.MapFile{Data: []byte("User-agent: *")},
	}
}

func getStatic(t *testing.T, h http.Handler, target string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	return rec
}

func TestSPAMenyajikanBerkasYangAda(t *testing.T) {
	h := SPAHandler(webDist(), SPAOptions{})

	rec := getStatic(t, h, "/assets/index-a1b2c3.js")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want 200", rec.Code)
	}
	if rec.Body.String() != isiScript {
		t.Fatalf("body %q, want %q", rec.Body.String(), isiScript)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "javascript") {
		t.Errorf("Content-Type %q tidak menyebut javascript", ct)
	}
}

func TestSPARuteKlienMengembalikanIndex(t *testing.T) {
	h := SPAHandler(webDist(), SPAOptions{})

	for _, target := range []string{"/", "/dashboard", "/settings/keys", "/settings/", "/index.html"} {
		rec := getStatic(t, h, target)
		if rec.Code != http.StatusOK {
			t.Errorf("%s: status %d, want 200", target, rec.Code)
			continue
		}
		if rec.Body.String() != isiIndex {
			t.Errorf("%s: body %q, want index.html", target, rec.Body.String())
		}
		if cc := rec.Header().Get("Cache-Control"); cc != "no-cache" {
			t.Errorf("%s: Cache-Control %q, want no-cache", target, cc)
		}
	}
}

func TestSPAAsetTidakAdaTetap404(t *testing.T) {
	h := SPAHandler(webDist(), SPAOptions{})

	for _, target := range []string{
		"/assets/index-deadbeef.js",
		"/assets/hilang.css",
		"/tidak-ada.png",
		"/assets/index-a1b2c3.js.map",
	} {
		rec := getStatic(t, h, target)
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s: status %d, want 404", target, rec.Code)
			continue
		}
		if strings.Contains(rec.Body.String(), "doctype") {
			t.Errorf("%s: index.html dikembalikan untuk aset yang tidak ada", target)
		}
		var resp ErrorResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Errorf("%s: body bukan envelope JSON: %v", target, err)
			continue
		}
		if resp.Error.Code == nil || *resp.Error.Code != CodeNotFound {
			t.Errorf("%s: code %v, want %q", target, resp.Error.Code, CodeNotFound)
		}
	}
}

// TestSPAMenolakPathTraversal memastikan tidak ada bentuk ".." yang bisa keluar dari
// fs.FS, termasuk yang sudah didekode net/http dari bentuk persen.
func TestSPAMenolakPathTraversal(t *testing.T) {
	fsys := webDist()
	// Berkas di luar direktori yang "boleh" dilihat: kalau traversal berhasil,
	// isinya akan muncul di respons.
	fsys["rahasia.txt"] = &fstest.MapFile{Data: []byte("KUNCI-RAHASIA")}
	h := SPAHandler(fsys, SPAOptions{})

	jahat := []string{
		"/../index.html",
		"/../../etc/passwd",
		"/assets/../../rahasia.txt",
		"/assets/%2e%2e/%2e%2e/rahasia.txt",
		"/..%2findex.html",
		"/%2e%2e%2frahasia.txt",
		"/./assets/../rahasia.txt",
		"/assets//index-a1b2c3.js",
		"//etc/passwd",
		"/index.html%00.js",
	}

	for _, target := range jahat {
		rec := getStatic(t, h, target)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400", target, rec.Code)
		}
		if strings.Contains(rec.Body.String(), "KUNCI-RAHASIA") {
			t.Fatalf("%s: path traversal berhasil membaca berkas di luar jalur", target)
		}
		if strings.Contains(rec.Body.String(), "doctype") {
			t.Errorf("%s: path bermasalah tidak boleh dijawab index.html", target)
		}
	}
}

func TestSPAHeaderCache(t *testing.T) {
	h := SPAHandler(webDist(), SPAOptions{})

	tests := map[string]string{
		"/assets/index-a1b2c3.js":  "public, max-age=31536000, immutable",
		"/assets/index-a1b2c3.css": "public, max-age=31536000, immutable",
		"/assets/logo.svg":         "public, max-age=3600",
		"/favicon.ico":             "public, max-age=3600",
		"/robots.txt":              "public, max-age=3600",
		"/index.html":              "no-cache",
		"/":                        "no-cache",
	}
	for target, want := range tests {
		rec := getStatic(t, h, target)
		if rec.Code != http.StatusOK {
			t.Errorf("%s: status %d, want 200", target, rec.Code)
			continue
		}
		if got := rec.Header().Get("Cache-Control"); got != want {
			t.Errorf("%s: Cache-Control %q, want %q", target, got, want)
		}
	}
}

func TestSPATidakMenanganiRuteAPI(t *testing.T) {
	h := SPAHandler(webDist(), SPAOptions{})

	for _, target := range []string{"/v1/chat/completions", "/v1/models", "/api/admin/keys"} {
		rec := getStatic(t, h, target)
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s: status %d, want 404", target, rec.Code)
			continue
		}
		if strings.Contains(rec.Body.String(), "doctype") {
			t.Errorf("%s: klien API tidak boleh menerima index.html", target)
		}
		if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "json") {
			t.Errorf("%s: Content-Type %q, want JSON", target, ct)
		}
	}
}

func TestSPAMetodeSelainGetDanHead(t *testing.T) {
	h := SPAHandler(webDist(), SPAOptions{})

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/index.html", strings.NewReader("x")))

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status %d, want 405", rec.Code)
	}
	if allow := rec.Header().Get("Allow"); allow != "GET, HEAD" {
		t.Errorf("Allow %q, want \"GET, HEAD\"", allow)
	}
}

func TestSPAHead(t *testing.T) {
	h := SPAHandler(webDist(), SPAOptions{})

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodHead, "/assets/index-a1b2c3.js", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Content-Length"); got != "22" {
		t.Errorf("Content-Length %q, want 22", got)
	}
}

func TestSPATanpaIndex(t *testing.T) {
	h := SPAHandler(fstest.MapFS{}, SPAOptions{})

	for _, target := range []string{"/", "/dashboard"} {
		rec := getStatic(t, h, target)
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s: status %d, want 404 saat frontend belum dibuild", target, rec.Code)
		}
	}
}

func TestSPADirektoriTidakDisajikan(t *testing.T) {
	h := SPAHandler(webDist(), SPAOptions{})

	// "/assets" adalah direktori: harus jatuh ke index.html, bukan daftar isi.
	rec := getStatic(t, h, "/assets")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want 200", rec.Code)
	}
	if rec.Body.String() != isiIndex {
		t.Fatalf("body %q; direktori tidak boleh menghasilkan daftar isi", rec.Body.String())
	}
}

func TestSPAOptionsDihormati(t *testing.T) {
	fsys := fstest.MapFS{
		"shell.html":       &fstest.MapFile{Data: []byte("shell")},
		"app-9f8e7d6c.js":  &fstest.MapFile{Data: []byte("app")},
		"vendor-legacy.js": &fstest.MapFile{Data: []byte("vendor")},
	}
	h := SPAHandler(fsys, SPAOptions{Index: "shell.html", ImmutableMaxAge: 10 * time.Minute})

	rec := getStatic(t, h, "/dashboard")
	if rec.Body.String() != "shell" {
		t.Fatalf("Index kustom tidak dipakai: %q", rec.Body.String())
	}
	rec = getStatic(t, h, "/app-9f8e7d6c.js")
	if got := rec.Header().Get("Cache-Control"); got != "public, max-age=600, immutable" {
		t.Fatalf("ImmutableMaxAge tidak dipakai: %q", got)
	}
}

func TestLooksHashed(t *testing.T) {
	hashed := []string{
		"index-a1b2c3.js",
		"index-a1b2c3d4.js",
		"vendor.4f8c1e2d.css",
		"app.min.9f8e7d6c.js",
		"logo-1a2b3c4d.svg",
	}
	for _, name := range hashed {
		if !looksHashed(name) {
			t.Errorf("looksHashed(%q) = false, want true", name)
		}
	}

	biasa := []string{
		"index.js",
		"favicon.ico",
		"chart-legend.js",
		"logo-2x.png",
		"vendor-abcdefgh.js",
		"style-.css",
		"a1b2c3d4.js",
	}
	for _, name := range biasa {
		if looksHashed(name) {
			t.Errorf("looksHashed(%q) = true, want false", name)
		}
	}
}
