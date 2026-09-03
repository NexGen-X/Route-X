package openai

import (
	"fmt"
	"strings"
	"testing"

	"github.com/NexGen-X/Route-X/internal/providers"
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
// seluruh isi struct — jalur terakhir itulah yang membocorkan kredensial lewat field
// pointer yang tampak aman di %v.
//
// # Batas yang tidak bisa ditutup tipe ini sendiri
//
// Satu jalur tetap terbuka dan memang tidak bisa ditutup dari sini: struct ANONIM yang
// menyimpan *Provider di field tak diekspor, lalu dicetak dengan %s.
//
//	fmt.Sprintf("%s", struct{ p *Provider }{p})  // => %!s(*openai.Provider=&{... kredensial ...})
//
// Sebabnya sama seperti seluruh persoalan ini: reflect menandai nilai dari field tak
// diekspor read-only, jadi String() milik Provider tidak dipanggil — dan pemilik field itu
// bukan Provider, sehingga tidak ada tempat di paket ini untuk memasang redaksinya.
//
// Yang menutupnya adalah TestRedaksiFieldTakDiekspor di internal/security, yang mewajibkan
// SETIAP tipe bernama di repo ini memasang redaksinya sendiri begitu ia menyimpan pemuat
// rahasia di field tak diekspor. Yang tersisa di luar jangkauan hanyalah struct anonim di
// dalam badan fungsi, dan hanya dengan %s pada struct — kekeliruan yang sudah ditandai fmt
// sendiri lewat awalan %!s(.
func TestKredensialTidakPernahTercetak(t *testing.T) {
	p, err := New(Config{
		Name: "uji", Kind: providers.KindOpenAI,
		BaseURL: "https://api.openai.com/v1", Credential: kredensialUji,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	kasus := map[string]any{
		"Provider nilai": *p,
		"Provider ptr":   p,
		"Provider slice": []*Provider{p},
		"Provider map":   map[string]*Provider{"a": p},
		"Provider error": fmt.Errorf("gagal pada %v", p),
		// chatStream menyimpan NAMA provider, bukan Provider-nya; kasus ini menjaga
		// pilihan itu supaya tidak diubah menjadi *Provider tanpa ikut menambah redaksi.
		"chatStream ptr": &chatStream{provider: p.Name()},
	}

	for nama, v := range kasus {
		for _, verb := range []string{"%v", "%+v", "%#v", "%s"} {
			if out := fmt.Sprintf(verb, v); strings.Contains(out, kredensialUji) {
				t.Errorf("%s dengan %s membocorkan kredensial: %s", nama, verb, out)
			}
		}
	}
}
