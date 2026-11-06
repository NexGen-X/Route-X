package gateway

import (
	"net/http"
	"testing"
)

// Test pengunci B-170 (Fase 5 kampanye tuntas-logika): count_tokens
// membakar kuota limiter tanpa pengembalian, tak tercatat, dan
// melewati izin model.
//
// Dua koreksi yang dikunci di sini:
//   - Rute count_tokens tinggal di LUAR grup pembatas laju inferensi —
//     ia bacaan lokal yang tidak menyentuh upstream, sama seperti
//     katalog model. Di dalam grup, setiap panggilan menagih kuota
//     laju key tanpa satu pun jalur pengembalian.
//   - Handler memeriksa izin model pada key, persis seperti jalur
//     chat: key yang dibatasi model tidak lagi bisa memastikan model
//     terlarangnya ada beserta taksiran tokennya dari endpoint ini.

const bodyCountTokensUji = `{"model":"gpt-4o-mini","messages":[{"role":"user","content":"halo dunia ini prompt uji"}]}`

func TestB170CountTokensDiLuarPembatasLaju(t *testing.T) {
	s := susun(t, providerSukses(), func(_ *susunan, d *HandlersDeps) {
		// Pembatas laju yang menolak SEMUA yang melewatinya: membuktikan
		// rute mana yang benar-benar berada di dalam grupnya.
		d.PembatasLaju = func(http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusTooManyRequests)
			})
		}
	})

	// Kontrol: rute inferensi memang tertahan pembatas.
	if w := s.panggil(t, http.MethodPost, "/chat/completions", bodyChatMinimal, nil); w.Code != http.StatusTooManyRequests {
		t.Fatalf("kontrol: chat/completions status = %d, mau 429 dari pembatas uji", w.Code)
	}
	// count_tokens (kedua bentuk path) tetap melayani: ia tidak boleh
	// membakar kuota laju untuk taksiran lokal.
	for _, jalur := range []string{"/messages/count_tokens", "/v1/messages/count_tokens"} {
		w := s.panggil(t, http.MethodPost, jalur, bodyCountTokensUji, nil)
		if w.Code != http.StatusOK {
			t.Errorf("B-170: %s status = %d, mau 200 — count_tokens tidak boleh tertahan pembatas laju inferensi (body: %s)",
				jalur, w.Code, w.Body.String())
		}
	}
}

func TestB170CountTokensMemeriksaIzinModel(t *testing.T) {
	s := susun(t, providerSukses(), func(s *susunan, d *HandlersDeps) {
		s.batas = &pembatasKey{model: false, provider: true}
		d.Restrict = s.batas
	})
	w := s.panggil(t, http.MethodPost, "/messages/count_tokens", bodyCountTokensUji, ctxDenganKey("k-1", "u-1"))
	if w.Code != http.StatusForbidden {
		t.Fatalf("B-170: status = %d, mau 403 — key yang dilarang model ini tidak boleh menaksir tokennya (body: %s)",
			w.Code, w.Body.String())
	}

	// Kontrol: key yang diizinkan tetap dilayani.
	s2 := susun(t, providerSukses(), func(s *susunan, d *HandlersDeps) {
		s.batas = &pembatasKey{model: true, provider: true}
		d.Restrict = s.batas
	})
	w = s2.panggil(t, http.MethodPost, "/messages/count_tokens", bodyCountTokensUji, ctxDenganKey("k-1", "u-1"))
	if w.Code != http.StatusOK {
		t.Fatalf("kontrol: status = %d, mau 200 untuk key yang diizinkan (body: %s)", w.Code, w.Body.String())
	}
}
