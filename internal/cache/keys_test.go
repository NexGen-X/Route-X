package cache

import (
	"strings"
	"testing"
)

func TestKey(t *testing.T) {
	tests := []struct {
		name  string
		parts []string
		want  string
	}{
		{"tanpa bagian hanya menghasilkan prefix", nil, "routex"},
		{"satu bagian", []string{"health"}, "routex:health"},
		{"beberapa bagian", []string{"rl", "apikey", "01H8Z"}, "routex:rl:apikey:01H8Z"},

		// Bagian opsional yang kosong tidak boleh meninggalkan segmen hampa.
		{"bagian kosong dilewati", []string{"rl", "", "01H8Z"}, "routex:rl:01H8Z"},
		{"bagian kosong di ujung dilewati", []string{"rl", "01H8Z", ""}, "routex:rl:01H8Z"},
		{"bagian berisi hanya spasi dilewati", []string{"rl", "   ", "01H8Z"}, "routex:rl:01H8Z"},
		{"semua bagian kosong", []string{"", "  ", ""}, "routex"},
		{"spasi di pinggir dipangkas", []string{" rl ", "\t01H8Z\n"}, "routex:rl:01H8Z"},

		// Karakter berbahaya di-escape, bukan dibuang.
		{"titik dua di-escape", []string{"rl", "apikey:admin"}, "routex:rl:apikey%3Aadmin"},
		{"spasi di dalam di-escape", []string{"model", "gpt 4 turbo"}, "routex:model:gpt%204%20turbo"},
		{"persen di-escape lebih dulu", []string{"model", "100%"}, "routex:model:100%25"},
		{"glob SCAN di-escape", []string{"model", "a*b?c[d]"}, "routex:model:a%2Ab%3Fc%5Bd%5D"},
		{"hash tag cluster di-escape", []string{"model", "{shard}"}, "routex:model:%7Bshard%7D"},
		{"baris baru di-escape", []string{"model", "a\nb"}, "routex:model:a%0Ab"},
		{"non-ascii jadi deretan byte", []string{"model", "café"}, "routex:model:caf%C3%A9"},

		// Karakter yang lazim ada di pengenal nyata tetap terbaca apa adanya.
		{"karakter aman dibiarkan", []string{"openai/gpt-4o", "v1.5_beta", "admin@example.com"},
			"routex:openai/gpt-4o:v1.5_beta:admin@example.com"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Key(tc.parts...)
			if got != tc.want {
				t.Fatalf("Key(%q) = %q, mau %q", tc.parts, got, tc.want)
			}
			if !strings.HasPrefix(got, KeyPrefix) {
				t.Fatalf("kunci %q tidak berada di namespace %q", got, KeyPrefix)
			}
			if strings.Contains(got, "::") {
				t.Fatalf("kunci %q memuat segmen hampa", got)
			}
		})
	}
}

// TestKeyTidakBisaMenabrakNamespace menjaga alasan utama escaping ada: nilai yang datang
// dari data (ID API key, nama model) tidak boleh bisa mengarang kunci milik segmen lain.
func TestKeyTidakBisaMenabrakNamespace(t *testing.T) {
	// Kalau ":" dibiarkan lewat, dua panggilan ini akan menghasilkan kunci yang sama dan
	// pemilik "apikey:admin" bisa membaca atau menimpa kuota milik admin.
	crafted := Key(NamespaceRateLimit, "apikey:admin")
	honest := Key(NamespaceRateLimit, "apikey", "admin")
	if crafted == honest {
		t.Fatalf("bagian bermuatan pemisah menabrak kunci sah: keduanya %q", crafted)
	}

	// Nilai yang menyamar sebagai prefix aplikasi lain juga harus tetap terkurung.
	escaped := Key(NamespaceHealth, "other-app:secrets")
	if strings.Count(escaped, ":") != 2 {
		t.Fatalf("kunci %q punya jumlah segmen yang tidak terduga", escaped)
	}
}

// TestEscapeKeyPartInjektif memastikan encoding-nya tidak pernah memetakan dua nilai
// berbeda ke satu kunci — beda dengan pendekatan "ganti karakter terlarang dengan _",
// di mana "a:b" dan "a_b" bertabrakan.
func TestEscapeKeyPartInjektif(t *testing.T) {
	parts := []string{
		"a:b", "a_b", "a b", "a%b", "a%3Ab", "a-b", "a.b", "a/b",
		"a*b", "a?b", "a{b", "ab", "A:B", "a\tb", "a\nb", "café", "caf%C3%A9",
	}
	seen := make(map[string]string, len(parts))
	for _, p := range parts {
		key := Key("x", p)
		if before, dup := seen[key]; dup {
			t.Fatalf("%q dan %q menghasilkan kunci yang sama: %q", before, p, key)
		}
		seen[key] = p
	}
}

func TestKonstruktorKunci(t *testing.T) {
	tests := []struct {
		name string
		got  string
		want string
	}{
		{"rate limit per API key", RateLimitKey("apikey", "01H8Z", "60"), "routex:rl:apikey:01H8Z:60"},
		{"rate limit per IP", RateLimitKey("ip", "203.0.113.7", "3600"), "routex:rl:ip:203.0.113.7:3600"},
		{"rate limit dengan bagian kotor", RateLimitKey("ip", "203.0.113.7:9000", " 60 "), "routex:rl:ip:203.0.113.7%3A9000:60"},
		{"breaker provider dan model", CircuitBreakerKey("openai-prod", "gpt-4o-mini"), "routex:cb:openai-prod:gpt-4o-mini"},
		{"breaker model bergaya vendor", CircuitBreakerKey("openrouter", "openai/gpt-4o"), "routex:cb:openrouter:openai/gpt-4o"},
		{"breaker tanpa model", CircuitBreakerKey("openai-prod", ""), "routex:cb:openai-prod"},
		{"health provider", HealthKey("anthropic-prod"), "routex:health:anthropic-prod"},
		{"health provider bermuatan pemisah", HealthKey("anthropic:prod"), "routex:health:anthropic%3Aprod"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Fatalf("dapat %q, mau %q", tc.got, tc.want)
			}
		})
	}
}

// TestKonstruktorKunciTidakSalingTumpang menjaga agar subsistem tidak bisa saling menimpa
// kunci walau memakai pengenal yang sama.
func TestKonstruktorKunciTidakSalingTumpang(t *testing.T) {
	id := "sama"
	keys := []string{
		RateLimitKey(id, id, id),
		CircuitBreakerKey(id, id),
		HealthKey(id),
	}
	seen := make(map[string]struct{}, len(keys))
	for _, k := range keys {
		if _, dup := seen[k]; dup {
			t.Fatalf("kunci %q dihasilkan oleh lebih dari satu subsistem", k)
		}
		seen[k] = struct{}{}
	}
}
