// Package docs menyematkan spesifikasi OpenAPI 3.1 ke dalam biner aplikasi dan
// menyajikan antarmuka dokumentasi interaktif pada endpoint /docs.
//
// Keputusan teknis menggunakan direktif go:embed langsung pada direktori docs/
// diambil karena spesifikasi Go melarang traverse ke direktori induk (..).
// Dengan meletakkan berkas ini di dalam docs/, openapi.yaml dapat langsung
// disematkan ke biner produksi tanpa membebani runtime dengan pembacaan berkas fisik.
package docs

import (
	_ "embed"
	"net/http"
	"strings"
)

// OpenAPIYAML memuat seluruh isi berkas openapi.yaml yang disematkan saat kompilasi.
//
//go:embed openapi.yaml
var OpenAPIYAML []byte

// Handler mengembalikan http.Handler untuk menyajikan dokumentasi interaktif pada /docs.
//
// Keputusan menggunakan antarmuka interaktif Scalar berbasis tema gelap (dark theme)
// diambil agar selaras dengan bahasa visual Route-X (#0A0A0A, #101010, aksen lime #BEF264),
// sekaligus mendukung penjelajahan skema OpenAPI 3.1 secara lengkap dan pengujian langsung (try-it-out).
func Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/docs")
		path = strings.TrimPrefix(path, "/")

		switch path {
		case "openapi.yaml", "openapi.yml":
			w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
			w.Header().Set("Cache-Control", "public, max-age=3600")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(OpenAPIYAML)
			return

		case "", "index.html":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-cache")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(scalarHTML))
			return

		default:
			http.NotFound(w, r)
		}
	})
}

// scalarHTML menyajikan antarmuka visual dokumentasi interaktif Scalar.
// Konfigurasi tema diselaraskan dengan design tokens Route-X (dark mode, warna aksen #BEF264).
const scalarHTML = `<!doctype html>
<html lang="id">
  <head>
    <title>Route-X — Dokumentasi API Gateway</title>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <link rel="preconnect" href="https://fonts.googleapis.com" />
    <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin />
    <link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700&family=JetBrains+Mono:wght@400;500;600&display=swap" rel="stylesheet" />
    <link rel="icon" type="image/svg+xml" href="data:image/svg+xml,<svg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 24 24' fill='%23BEF264'><circle cx='12' cy='12' r='10'/><path d='M8 12h8M12 8l4 4-4 4' stroke='%230A0A0A' stroke-width='2' stroke-linecap='round' stroke-linejoin='round'/></svg>" />
    <style>
      body {
        margin: 0;
        background-color: #0A0A0A;
        font-family: 'Inter', system-ui, sans-serif;
      }
      /* Custom styling untuk penyesuaian tema Route-X */
      :root {
        --scalar-color-1: #F5F5F5;
        --scalar-color-2: #A1A1AA;
        --scalar-color-3: #6B7280;
        --scalar-color-accent: #BEF264;
        --scalar-background-1: #0A0A0A;
        --scalar-background-2: #101010;
        --scalar-background-3: #141414;
        --scalar-border-color: #1F1F1F;
        --scalar-font-code: 'JetBrains Mono', monospace;
      }
    </style>
  </head>
  <body>
    <script
      id="api-reference"
      data-url="/docs/openapi.yaml"
      data-configuration='{"theme":"none","darkMode":true,"layout":"modern","hideModels":false,"showSidebar":true}'>
    </script>
    <script src="https://cdn.jsdelivr.net/npm/@scalar/api-reference@1.25.75/dist/browser/standalone.min.js"></script>
    <noscript>
      <div style="padding: 2rem; color: #F5F5F5; background: #101010; margin: 2rem; border-radius: 8px;">
        <h2>Route-X API Reference</h2>
        <p>JavaScript diperlukan untuk menampilkan dokumentasi interaktif.</p>
        <p>Anda dapat mengunduh spesifikasi mentah di: <a href="/docs/openapi.yaml" style="color: #BEF264;">/docs/openapi.yaml</a></p>
      </div>
    </noscript>
  </body>
</html>`
