package security_test

import (
	"bufio"
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// TestSecretLeakLint adalah linter permanen yang memeriksa SELURUH berkas terlacak
// di git (git ls-files) untuk memastikan tidak ada rahasia, kunci privat, token API nyata,
// atau kata sandi literal yang tidak sengaja ter-commit ke repositori.
//
// Mengapa test ini wajib ada:
// Audit Fase 10-14 membuktikan bahwa SESSION_SECRET, ENCRYPTION_KEY, API_KEY_PEPPER,
// dan sandi awal admin ter-commit langsung ke repositori publik. Jika penjaga ini tidak ada,
// pengembang atau agen berikutnya dapat kembali secara tidak sengaja memasukkan rahasia
// literal ke dalam berkas docker-compose, script pengujian, atau contoh konfigurasi.
//
// Aturan pendeteksian:
// 1. Pola sk- dan sk_live_ diikuti minimal 20 karakter alfanumerik.
// 2. String base64 panjang (>= 32 karakter) pada baris yang memuat kata kunci rahasia (SECRET, KEY, PEPPER).
// 3. Penetapan PASSWORD= atau POSTGRES_PASSWORD= dengan nilai literal non-kosong dan bukan ekspansi env ${...}.
//
// Pengecualian (Whitelist):
// Hanya diperbolehkan untuk berkas pengujian tertentu yang secara sah membutuhkan string tiruan
// untuk memverifikasi logika enkripsi, hashing, atau redaksi. Setiap entri wajib disertai alasan teknis.
func TestSecretLeakLint(t *testing.T) {
	akar := akarModul(t)

	// Ambil seluruh berkas yang dilacak oleh git dari akar modul
	cmd := exec.Command("git", "ls-files")
	cmd.Dir = akar
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("gagal mengeksekusi git ls-files: %v", err)
	}

	scanner := bufio.NewScanner(bytes.NewReader(out))
	var files []string
	for scanner.Scan() {
		f := strings.TrimSpace(scanner.Text())
		if f != "" {
			files = append(files, f)
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("gagal membaca output git ls-files: %v", err)
	}

	// Daftar putih berkas yang dikecualikan beserta alasan teknisnya.
	// Pengecualian dibuat sespesifik mungkin agar tidak membuka celah di berkas lain.
	whitelist := map[string]string{
		"internal/apikey/apikey_test.go":                      "Memuat kunci contoh untuk menguji format hashing dan verifikasi API key klien.",
		"internal/apikey/middleware_test.go":                  "Memuat token mock sk_live_ untuk menguji otorisasi Bearer token pada middleware.",
		"internal/security/secret_test.go":                    "Memuat string rahasia tiruan untuk menguji redaksi tipe security.Secret.",
		"internal/security/crypto_test.go":                    "Memuat ciphertext dan kunci AES-256 statis untuk memvalidasi algoritma enkripsi GCM.",
		"internal/security/redaction_lint_test.go":            "Memuat dokumentasi komentar mengenai pola sk- yang dicegah peredaksiannya.",
		"internal/security/rotation/rotation_test.go":         "Memuat kredensial dan webhook secret uji untuk menguji fungsi rotasi database.",
		"internal/database/repo/upstream/credentials_test.go": "Memuat token mock untuk menguji penyimpanan kredensial upstream di database.",
		"internal/providers/anthropic/anthropic_test.go":      "Memuat mock api key Anthropic untuk memvalidasi adapter provider.",
		"internal/providers/anthropic/redaction_test.go":      "Memuat string uji untuk memvalidasi penyaringan log provider Anthropic.",
		"internal/providers/google/redaction_test.go":         "Memuat string uji untuk memvalidasi penyaringan log provider Google.",
		"internal/providers/openai/openai_test.go":            "Memuat mock api key OpenAI untuk memvalidasi adapter provider.",
		"internal/providers/openai/redaction_test.go":         "Memuat string uji untuk memvalidasi penyaringan log provider OpenAI.",
		"internal/observability/logger_test.go":               "Memuat contoh API key pada pengujian filter redaksi logger terstruktur.",
		"internal/gateway/factory_test.go":                    "Memuat dummy secret provider untuk memvalidasi factory gateway.",
		"internal/gateway/codec_test.go":                      "Memuat string credential uji untuk memvalidasi parsing codec.",
		"web/package-lock.json":                               "Berkas integritas dependensi npm yang memuat hash sha512 integritas paket.",
		"web/src/pages/ContentFilters.tsx":                    "Memuat string placeholder UI regex 'sk-proj-[a-zA-Z0-9]+'.",
		"internal/database/migrations/0003_providers.sql":     "Memuat komentar dokumentasi contoh format 'sk-****9a21'.",
		"internal/database/repo/upstream/credentials.go":      "Memuat komentar dokumentasi contoh format 'sk-****9a21'.",
		"internal/database/repo/upstream/upstream.go":         "Memuat komentar dokumentasi contoh format 'sk-****9a21'.",
		"internal/database/repo/upstream/upstream_test.go":    "Memuat data uji untuk fungsi masking hint.",
		"internal/providers/anthropic/anthropic.go":           "Memuat dokumentasi pesan error upstream Anthropic.",
		"internal/security/secret_leak_lint_test.go":          "Berkas linter itu sendiri yang memuat regex pendeteksi rahasia.",
		"README.md":                                           "Dokumentasi contoh panggilan API dengan format sk_live_ placeholder.",
		"docs/QUICKSTART.md":                                  "Panduan integrasi cepat dengan contoh panggilan API sk_live_ placeholder.",
	}

	// 1. Pola pendeteksian sk- atau sk_live_ >= 20 karakter
	reAPIKey := regexp.MustCompile(`(sk-[a-zA-Z0-9_-]{20,}|sk_live_[a-zA-Z0-9_-]{20,})`)

	// 2. Pola pendeteksian nilai base64 panjang (>= 32 char) pada penetapan variabel rahasia
	// Misal: SESSION_SECRET=..., ENCRYPTION_KEY=..., API_KEY_PEPPER=...
	reSecretAssign := regexp.MustCompile(`(?i)(SESSION_SECRET|ENCRYPTION_KEY|API_KEY_PEPPER)\s*[:=]\s*["']?([A-Za-z0-9+/=_-]{32,})["']?`)

	// 3. Pola penetapan PASSWORD= atau INITIAL_ADMIN_PASSWORD= atau POSTGRES_PASSWORD= dengan nilai literal non-kosong
	// Menolak PASSWORD=foobar tetapi mengizinkan PASSWORD= atau PASSWORD=${VAR:-...} atau ekspansi kosong
	rePasswordAssign := regexp.MustCompile(`(?i)(INITIAL_ADMIN_PASSWORD|POSTGRES_PASSWORD|ADMIN_PASS(WORD)?)\s*[:=]\s*["']?([^"'\s#]+)["']?`)

	var violations []string

	for _, filePath := range files {
		if reason, ok := whitelist[filePath]; ok {
			_ = reason // Berkas diizinkan secara sah
			continue
		}

		fullPath := filepath.Join(akar, filePath)
		content, err := os.ReadFile(fullPath)
		if err != nil {
			// Jika file tidak bisa dibaca di working copy, lewati
			continue
		}

		lines := strings.Split(string(content), "\n")
		for lineNum, line := range lines {
			trimmed := strings.TrimSpace(line)
			// Abaikan baris komentar penuh
			if strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "/*") {
				continue
			}

			// Cek pola 1: API key tiruan sk- / sk_live_
			if match := reAPIKey.FindString(line); match != "" {
				violations = append(violations, filePath+":"+strconv.Itoa(lineNum+1)+" - terdeteksi API key/token mencurigakan: "+match)
			}

			// Cek pola 2: Nilai rahasia base64 panjang pada variabel rahasia
			if match := reSecretAssign.FindStringSubmatch(line); len(match) > 2 {
				val := match[2]
				// Pastikan bukan referensi variabel ${...}
				if !strings.HasPrefix(val, "${") && !strings.HasPrefix(val, "$") {
					violations = append(violations, filePath+":"+strconv.Itoa(lineNum+1)+" - terdeteksi base64 rahasia pada "+match[1])
				}
			}

			// Cek pola 3: Password literal yang terisi langsung
			if match := rePasswordAssign.FindStringSubmatch(line); len(match) > 3 {
				val := match[3]
				// Izinkan jika nama variabel berada di dalam ekspresi variabel ${...}
				if strings.Contains(line, "${"+match[1]) {
					continue
				}
				// Izinkan jika kosong, berupa parameter default ekspansi ${...}, atau placeholder aman
				if val != "" && !strings.HasPrefix(val, "${") && !strings.HasPrefix(val, "$") && !strings.Contains(val, "GANTI_") {
					violations = append(violations, filePath+":"+strconv.Itoa(lineNum+1)+" - terdeteksi password literal pada "+match[1])
				}
			}
		}
	}

	if len(violations) > 0 {
		t.Errorf("Ditemukan %d kebocoran rahasia di berkas terlacak:\n%s", len(violations), strings.Join(violations, "\n"))
	}
}
