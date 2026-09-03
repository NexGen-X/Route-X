package anthropic

import (
	"fmt"
	"strings"
	"testing"
)

// kredensialUji adalah nilai yang tidak boleh muncul di keluaran mana pun.
const kredensialUji = "sk-jangan-sampai-tercetak-9f21"

// TestKredensialTidakPernahTercetak menguji perilakunya, bukan keberadaan metodenya.
//
// Ini pelengkap TestRedaksiFieldTakDiekspor di internal/security: lint itu memeriksa bahwa
// metode redaksinya ADA dengan bentuk receiver yang benar, sedangkan test ini memeriksa
// bahwa hasilnya memang tidak memuat kredensial — metode yang ada tetapi salah isi akan
// lolos lint dan gagal di sini.
//
// Keempat verb diuji karena jalurnya di fmt berbeda: %v dan %+v lewat String, %#v lewat
// GoString, dan %s pada struct TANPA String menempuh jalur verb-salah yang membongkar
// seluruh isi struct. Jalur terakhir itulah yang dulu membocorkan kredensial dari stream
// lewat field pointer yang tampak aman di %v.
//
// Batas yang tidak bisa ditutup dari sini didokumentasikan di test serupa milik paket
// openai: struct ANONIM yang menyimpan *Provider di field tak diekspor lalu dicetak %s.
func TestKredensialTidakPernahTercetak(t *testing.T) {
	p, err := New(Config{Name: "uji", Credential: kredensialUji})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	kasus := map[string]any{
		"Provider nilai": *p,
		"Provider ptr":   p,
		"Provider slice": []*Provider{p},
		"Provider map":   map[string]*Provider{"a": p},
		"Provider error": fmt.Errorf("gagal pada %v", p),
		// stream menyimpan *Provider di field tak diekspor; inilah kasus yang dulu bocor.
		"stream ptr": &stream{provider: p},
	}

	for nama, v := range kasus {
		for _, verb := range []string{"%v", "%+v", "%#v", "%s"} {
			if out := fmt.Sprintf(verb, v); strings.Contains(out, kredensialUji) {
				t.Errorf("%s dengan %s membocorkan kredensial: %s", nama, verb, out)
			}
		}
	}
}
