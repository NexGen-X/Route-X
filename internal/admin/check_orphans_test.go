package admin

import (
	"fmt"
	"github.com/go-chi/chi/v5"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// normalisasiEndpoint menyamakan penulisan parameter dinamis antara
// pola rute chi (`{id}`), literal template klien (`${id}`), dan query
// string yang menempel, agar kunci perbandingan selalu berbentuk
// "METODE /pola/seragam".
func normalisasiEndpoint(raw string) string {
	endpoint := regexp.MustCompile(`\$\{q[^}]*\}`).ReplaceAllString(raw, "")
	endpoint = regexp.MustCompile(`\$\{[^}]+\}`).ReplaceAllString(endpoint, "{param}")
	endpoint = regexp.MustCompile(`\{[^}]+\}`).ReplaceAllString(endpoint, "{param}")
	return strings.TrimSuffix(endpoint, "/")
}

// hapusKomentarTS mengosongkan komentar // dan /* */ dari sumber
// TypeScript sambil menghormati string ('…', "…") dan template
// literal (`…`, termasuk ekspresi ${…} bersarang) — URL di dalam
// string adalah data panggilan yang sah, sedangkan teks komentar
// tidak boleh terbaca sebagai panggilan. Isi komentar diganti spasi
// agar posisi teks lain tidak bergeser.
func hapusKomentarTS(src string) string {
	out := []byte(src)
	var tumpukan []int // kedalaman {} ekspresi ${} template yang terbuka
	mode := byte(0)    // 0=kode, selain itu karakter kutip yang terbuka
	escape := false
	for i := 0; i < len(src); {
		c := src[i]
		if mode == 0 {
			if c == '/' && i+1 < len(src) && src[i+1] == '/' {
				for i < len(src) && src[i] != '\n' {
					out[i] = ' '
					i++
				}
				continue
			}
			if c == '/' && i+1 < len(src) && src[i+1] == '*' {
				out[i], out[i+1] = ' ', ' '
				i += 2
				for i < len(src) {
					if src[i] == '*' && i+1 < len(src) && src[i+1] == '/' {
						out[i], out[i+1] = ' ', ' '
						i += 2
						break
					}
					if src[i] != '\n' {
						out[i] = ' '
					}
					i++
				}
				continue
			}
			if c == '\'' || c == '"' || c == '`' {
				mode = c
				i++
				continue
			}
			if c == '{' && len(tumpukan) > 0 {
				tumpukan[len(tumpukan)-1]++
			}
			if c == '}' && len(tumpukan) > 0 {
				tumpukan[len(tumpukan)-1]--
				if tumpukan[len(tumpukan)-1] == 0 {
					tumpukan = tumpukan[:len(tumpukan)-1]
					mode = '`'
				}
			}
			i++
			continue
		}
		if mode == '`' {
			if escape {
				escape = false
				i++
				continue
			}
			if c == '\\' {
				escape = true
				i++
				continue
			}
			if c == '`' {
				mode = 0
				i++
				continue
			}
			if c == '$' && i+1 < len(src) && src[i+1] == '{' {
				tumpukan = append(tumpukan, 1)
				mode = 0
				i += 2
				continue
			}
			i++
			continue
		}
		// String '…' atau "…".
		if escape {
			escape = false
			i++
			continue
		}
		if c == '\\' {
			escape = true
			i++
			continue
		}
		if c == mode {
			mode = 0
		}
		i++
	}
	return string(out)
}

// ekstrakEndpointKlien membaca operasi (METODE + path) yang benar-benar
// dipanggil web/src/api/client.ts (B-314). Versi lama gerbang ini
// hanya mengumpulkan PATH: operasi yang metodenya yatim tetapi
// path-nya disebut klien untuk metode lain lolos tanpa terlihat.
//
// Metode diambil dari opsi `method:` pada panggilan request() /
// fetchBertenggat() yang memuat literal URL itu; tanpa opsi, request
// klien ini bawaan GET (lihat request() di client.ts). Segmen yang
// diperiksa untuk satu panggilan dibatasi sampai panggilan berikutnya
// agar opsi milik panggilan lain tidak terbaca sebagai miliknya.
// Literal URL yang hanya muncul di komentar/dokumentasi tidak dihitung
// karena tidak berada di dalam segmen panggilan mana pun.
func ekstrakEndpointKlien(content string) map[string]bool {
	content = hapusKomentarTS(content)
	reURL := regexp.MustCompile(`['"\x60](/api/(?:admin|auth)/[^'"\x60?]+)`)
	reMetode := regexp.MustCompile(`method\s*:\s*['"]([A-Z]+)['"]`)
	rePanggil := regexp.MustCompile(`\b(?:request|fetchBertenggat)\s*(?:<[^()]*>)?\s*\(`)

	// Variabel path lokal (mis. `const path = '/api/...'` yang dipakai
	// sebagai `${path}` di dalam panggilan) tetap operasi klien.
	reVarPath := regexp.MustCompile(`(?:const|let|var)\s+(\w+)\s*=\s*['"\x60](/api/(?:admin|auth)/[^'"\x60?]+)['"\x60]`)
	varPath := make(map[string]string)
	for _, m := range reVarPath.FindAllStringSubmatch(content, -1) {
		varPath[m[1]] = normalisasiEndpoint(m[2])
	}

	endpoints := make(map[string]bool)
	locs := rePanggil.FindAllStringIndex(content, -1)
	for i, loc := range locs {
		akhir := len(content)
		if i+1 < len(locs) {
			akhir = locs[i+1][0]
		}
		segmen := content[loc[0]:akhir]
		metode := "GET"
		if m := reMetode.FindStringSubmatch(segmen); m != nil {
			metode = m[1]
		}
		for _, m := range reURL.FindAllStringSubmatch(segmen, -1) {
			endpoints[metode+" "+normalisasiEndpoint(m[1])] = true
		}
		for nama, path := range varPath {
			if strings.Contains(segmen, "${"+nama+"}") || strings.Contains(segmen, nama+")") {
				endpoints[metode+" "+path] = true
			}
		}
	}
	return endpoints
}

// TestEkstrakEndpointKlienSadarMetodeB314 mengunci perilaku ekstraktor
// B-314 pada cuplikan sintetis: metode yang sama pada path yang sama
// dibedakan, bawaan tanpa opsi adalah GET, dan URL di komentar bukan
// panggilan.
func TestEkstrakEndpointKlienSadarMetodeB314(t *testing.T) {
	src := "" +
		"// request('/api/admin/contoh/komentar')\n" +
		"const a = () => request('/api/admin/contoh');\n" +
		"const b = () => request('/api/admin/contoh', { method: 'PUT' });\n" +
		"const c = () => request(`/api/admin/contoh/${id}/toggle`, { method: 'POST' });\n"
	got := ekstrakEndpointKlien(src)
	for _, kunci := range []string{
		"GET /api/admin/contoh",
		"PUT /api/admin/contoh",
		"POST /api/admin/contoh/{param}/toggle",
	} {
		if !got[kunci] {
			t.Errorf("endpoint %q tidak terekstrak: %v", kunci, got)
		}
	}
	for _, kunci := range []string{
		"GET /api/admin/contoh/komentar",
		"GET /api/admin/contoh/{param}/toggle",
	} {
		if got[kunci] {
			t.Errorf("endpoint %q seharusnya tidak terekstrak: %v", kunci, got)
		}
	}
}

func TestCheckOrphans(t *testing.T) {
	clientTSPath := filepath.Join("..", "..", "web", "src", "api", "client.ts")
	content, _ := os.ReadFile(clientTSPath)

	clientEndpoints := ekstrakEndpointKlien(string(content))

	h := NewHandlers(Config{})
	serverRouter := chi.NewRouter()
	serverRouter.Mount("/api/admin", h.Routes())

	registeredRoutes := make(map[string]bool)
	_ = chi.Walk(serverRouter, func(method, route string, handler http.Handler, middlewares ...func(http.Handler) http.Handler) error {
		if method == http.MethodHead {
			// HEAD adalah turunan otomatis chi dari GET, bukan operasi
			// yang ditulis terpisah — tidak dihitung sebagai yatim.
			return nil
		}
		registeredRoutes[method+" "+normalisasiEndpoint(route)] = true
		return nil
	})

	// Daftar putih rute yatim yang DISENGAJA, dua kategori:
	//
	// (1) Tiga alias ringkasan baca (/summary, /overview, /diagnostics)
	// yang dipertahankan sesudah pemangkasan alias Fase 2 R5 — 23 alias
	// CRUD datar dihapus dari routes.go setelah analisis membuktikan
	// nol konsumen di dalam repo.
	//
	// (2) Rute-rute B-286 (Fase 14): wrapper klien web untuk permukaan
	// ini dihapus dari client.ts karena nol pemanggil UI (grup
	// filters/bans/sessions utuh + metode sebaran), TETAPI rutenya
	// SENGAJA dipertahankan hidup sebagai permukaan API langsung —
	// keputusan entri B-286 membatasi penghapusan pada wrapper klien
	// dan menyatakan rute backend tidak disentuh. Pendaftaran di sini
	// mendokumentasikan kesengajaan itu; gerbang tetap menggagalkan
	// tes untuk yatim BARU apa pun di luar daftar.
	//
	// Di luar daftar ini, rute backend TANPA pemanggil frontend adalah
	// temuan nyata dan MENGGAGALKAN tes ini — sebelum perbaikan F11 tes
	// ini hanya mencetak tanpa assertion sehingga tidak bisa gagal.
	orphanTerdaftar := map[string]bool{
		// Kategori (1) — tiga alias baca tingkat-atas:
		"GET /api/admin/diagnostics": true,
		"GET /api/admin/overview":    true,
		"GET /api/admin/summary":     true,
		// Kategori (2) — B-286, kini dihitung per OPERASI (B-314):
		// sesi global (inventaris + cabut satu + cabut semua untuk
		// respons insiden runbook):
		"GET /api/admin/access/sessions":             true,
		"DELETE /api/admin/access/sessions/{param}":  true,
		"POST /api/admin/access/sessions/revoke-all": true,
		// ban manual (satu-satunya produser/pengelola ban):
		"GET /api/admin/gateway/bans":               true,
		"POST /api/admin/gateway/bans":              true,
		"POST /api/admin/gateway/bans/{param}/lift": true,
		// filter konten (satu-satunya permukaan pengelolaan mesin
		// filter yang hidup di jalur gateway):
		"GET /api/admin/gateway/content-filters":                 true,
		"POST /api/admin/gateway/content-filters":                true,
		"GET /api/admin/gateway/content-filters/{param}":         true,
		"PUT /api/admin/gateway/content-filters/{param}":         true,
		"DELETE /api/admin/gateway/content-filters/{param}":      true,
		"POST /api/admin/gateway/content-filters/{param}/toggle": true,
		// Kandidat hapus B-286 yang ditunda ke fase fitur (didaftarkan
		// apa adanya, tidak dihapus di sini):
		"POST /api/admin/gateway/rate-limits/{param}/toggle": true,
		"GET /api/admin/observability/metrics/live":          true,
		"POST /api/admin/upstreams/models/{param}/mappings":  true,
		// pelengkap baca/kelola per-ID yang tidak punya jalur UI lain:
		"GET /api/admin/requests/{param}":                            true,
		"GET /api/admin/upstreams/models/{param}":                    true,
		"PUT /api/admin/upstreams/models/{param}":                    true,
		"DELETE /api/admin/upstreams/models/{param}":                 true,
		"DELETE /api/admin/upstreams/models/{param}/aliases/{param}": true,
		"DELETE /api/admin/upstreams/models/mappings/{param}":        true,
		"GET /api/admin/upstreams/providers/{param}/health-checks":   true,
		// Kategori (3) — yatim tingkat-METODE yang baru terlihat
		// sesudah gerbang sadar-metode (B-314): path-nya hidup di
		// klien untuk metode lain, metodenya sendiri tidak punya
		// wrapper. Dipertahankan sebagai permukaan API langsung per
		// pola B-286; tidak ada rute yang dihapus oleh butir ini.
		"PUT /api/admin/gateway/budgets/{param}":     true,
		"GET /api/admin/automation/webhooks/{param}": true,
		"PUT /api/admin/automation/webhooks/{param}": true,
		// GET per-ID berikut adalah pasangan baca dari operasi
		// PUT/DELETE pada path yang sama yang memang dipakai klien;
		// gerbang path-saja dulu menganggap path-nya "terpanggil",
		// padahal wrapper GET tunggalnya tidak ada. Dipertahankan
		// sebagai permukaan API langsung per pola B-286:
		"GET /api/admin/upstreams/providers/{param}":    true,
		"GET /api/admin/upstreams/egress-pools/{param}": true,
		"GET /api/admin/system/settings/{param}":        true,
		"GET /api/admin/access/api-keys/{param}":        true,
		"GET /api/admin/gateway/routing-rules/{param}":  true,
		"GET /api/admin/gateway/rate-limits/{param}":    true,
		"GET /api/admin/gateway/budgets/{param}":        true,
		// PUT per-ID yang wrapper kliennya tidak ada (klien memakai
		// operasi sekeliling path yang sama); permukaan tulis API
		// langsung, dipertahankan per pola yang sama:
		"PUT /api/admin/access/api-keys/{param}":     true,
		"PUT /api/admin/gateway/rate-limits/{param}": true,
		// POST allowed adalah alias handler yang sama persis dengan
		// PUT allowed yang dipakai klien (access.go mendaftarkan
		// keduanya ke setAllowedAPIKey); dipertahankan untuk
		// kompatibilitas pemanggil langsung:
		"POST /api/admin/access/api-keys/{param}/allowed": true,
	}

	var yatimBaru []string
	for operasi := range registeredRoutes {
		if clientEndpoints[operasi] {
			continue
		}
		if orphanTerdaftar[operasi] {
			fmt.Printf("ORPHAN BACKEND ROUTE (terdaftar/disengaja): %s\n", operasi)
			continue
		}
		yatimBaru = append(yatimBaru, operasi)
	}
	if len(yatimBaru) > 0 {
		t.Errorf("ditemukan %d operasi backend yatim BARU tanpa pemanggil frontend dan tidak terdaftar sebagai alias sengaja: %v — panggil dari frontend, hapus rutenya, atau daftarkan di orphanTerdaftar dengan alasan",
			len(yatimBaru), yatimBaru)
	}
}
