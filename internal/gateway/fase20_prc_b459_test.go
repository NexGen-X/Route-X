package gateway

// fase20_prc_b459_test.go — pengunci Fase 20 PR-C (B-459).
//
// Penulisan jawaban akhir non-streaming yang GAGAL (klien putus
// tepat pada Write terakhir) dulu ditelan ketujuh permukaan:
// request tercatat sukses penuh dan webhook menerbitkan
// request.completed. Kini jejaknya berkategori canceled — status
// terkirim tetap 200, polanya sama dengan kegagalan mid-stream
// B-172 — sehingga event pemakaian ber-ErrorType dan webhook-nya
// request.failed.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/NexGen-X/Route-X/internal/providers"
)

// panggilDenganPenulis memanggil handler dengan penulisPutus dari
// fase5_b172_test.go (batas 0: penulisan pertama — yaitu penulisan
// jawaban akhir — langsung gagal).
func panggilDenganPenulis(s *susunan, penulis http.ResponseWriter, body string) {
	r := httptest.NewRequest(http.MethodPost, "/chat/completions", strings.NewReader(body))
	s.h.Routes().ServeHTTP(penulis, r)
}

// TestB459GagalTulisAkhirBukanSukses membuktikan kaki MISS dan HIT:
// keduanya harus tercatat ber-ErrorType canceled dengan webhook
// request.failed, bukan request.completed.
func TestB459GagalTulisAkhirBukanSukses(t *testing.T) {
	// --- MISS ---
	sinkW := &sinkWebhookPalsu{}
	prov := providerDenganUsage("utama", 40)
	s, sink := susunDenganPencatat(t, prov, func(_ *susunan, d *HandlersDeps) {
		d.Webhooks = sinkW
		d.ResponseCache = mesinCacheAktif(t)
	})

	panggilDenganPenulis(s, &penulisPutus{batas: 0}, bodyChat)
	sink.mu.Lock()
	jumlah := len(sink.ev)
	var tipe string
	var status int
	if jumlah > 0 {
		tipe = sink.ev[jumlah-1].ErrorType
		status = sink.ev[jumlah-1].StatusCode
	}
	sink.mu.Unlock()
	if jumlah != 1 {
		t.Fatalf("MISS: catatan pemakaian = %d, mau 1", jumlah)
	}
	if tipe != string(providers.ErrKindCanceled) {
		t.Errorf("MISS: ErrorType = %q, mau %q — gagal tulis tercatat sebagai sukses", tipe, providers.ErrKindCanceled)
	}
	if status != http.StatusOK {
		t.Errorf("MISS: status tercatat = %d, mau 200 (header memang terkirim)", status)
	}
	if p := sinkW.cari("request.completed"); p != nil {
		t.Errorf("MISS: request.completed terbit untuk pengiriman yang gagal; events=%v", sinkW.events)
	}
	if p := sinkW.cari("request.failed"); p == nil {
		t.Errorf("MISS: request.failed tidak terbit; events=%v", sinkW.events)
	}

	// --- HIT: tanam lewat panggilan sehat, putuskan panggilan kedua. ---
	sinkW2 := &sinkWebhookPalsu{}
	s2, sink2 := susunDenganPencatat(t, providerDenganUsage("utama", 40), func(_ *susunan, d *HandlersDeps) {
		d.Webhooks = sinkW2
		d.ResponseCache = mesinCacheAktif(t)
	})
	if w := s2.panggil(t, http.MethodPost, "/chat/completions", bodyChat, nil); w.Code != http.StatusOK ||
		w.Header().Get("X-RouteX-Cache") != "MISS" {
		t.Fatalf("HIT persiapan: status = %d cache = %q, mau 200/MISS", w.Code, w.Header().Get("X-RouteX-Cache"))
	}
	panggilDenganPenulis(s2, &penulisPutus{batas: 0}, bodyChat)
	sink2.mu.Lock()
	jumlah2 := len(sink2.ev)
	var tipe2 string
	var hit2 bool
	if jumlah2 > 0 {
		tipe2 = sink2.ev[jumlah2-1].ErrorType
		hit2 = sink2.ev[jumlah2-1].CacheHit
	}
	sink2.mu.Unlock()
	if jumlah2 != 2 {
		t.Fatalf("HIT: catatan pemakaian = %d, mau 2", jumlah2)
	}
	if !hit2 {
		t.Fatal("HIT: event kedua bukan cache HIT — skenario tidak terwakili")
	}
	if tipe2 != string(providers.ErrKindCanceled) {
		t.Errorf("HIT: ErrorType = %q, mau %q — gagal tulis pada HIT tercatat sukses", tipe2, providers.ErrKindCanceled)
	}
	if p := sinkW2.cari("request.failed"); p == nil {
		t.Errorf("HIT: request.failed tidak terbit; events=%v", sinkW2.events)
	}
	// Event pertama (MISS sehat) tetap completed — penanda gagal
	// tulis tidak boleh bocor ke request lain.
	if sinkW2.events[0] != "request.completed" {
		t.Errorf("HIT: event pertama = %q, mau request.completed (MISS sehat)", sinkW2.events[0])
	}
}
