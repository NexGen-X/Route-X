package gateway

import (
	"net/http"
	"testing"

	"github.com/NexGen-X/Route-X/internal/usage"
)

// TestCacheHITMenandaiEventPemakaianB188 adalah pengunci B-188 di sisi
// gateway: event pemakaian untuk jawaban cache HIT wajib membawa penanda
// CacheHit=true — itulah yang membuat Recorder memaksa biayanya nol walau
// kandidat pemenang (dan ProviderModelID berharga) terpasang demi atribusi
// (audit3 V-GW-09). Event untuk MISS tidak boleh bertanda.
func TestCacheHITMenandaiEventPemakaianB188(t *testing.T) {
	prov := providerDenganUsage("utama", 40)
	sink := &pencatatTiruan{}
	mesin := mesinCacheAktif(t)
	s := susun(t, prov, func(_ *susunan, d *HandlersDeps) {
		d.Usage = sink
		d.ResponseCache = mesin
	})

	w := s.panggil(t, http.MethodPost, "/chat/completions", bodyChat, nil)
	if w.Code != http.StatusOK || w.Header().Get("X-RouteX-Cache") != "MISS" {
		t.Fatalf("pertama: status = %d, cache = %q, mau 200/MISS (%s)",
			w.Code, w.Header().Get("X-RouteX-Cache"), w.Body.String())
	}
	w = s.panggil(t, http.MethodPost, "/chat/completions", bodyChat, nil)
	if w.Code != http.StatusOK || w.Header().Get("X-RouteX-Cache") != "HIT" {
		t.Fatalf("kedua: status = %d, cache = %q, mau 200/HIT (%s)",
			w.Code, w.Header().Get("X-RouteX-Cache"), w.Body.String())
	}

	sink.mu.Lock()
	evs := make([]usage.Event, len(sink.ev))
	copy(evs, sink.ev)
	sink.mu.Unlock()

	if len(evs) != 2 {
		t.Fatalf("catatan pemakaian = %d, mau 2", len(evs))
	}
	if evs[0].CacheHit {
		t.Error("event MISS bertanda CacheHit, mau false")
	}
	if !evs[1].CacheHit {
		t.Error("event HIT tidak bertanda CacheHit — Recorder akan menagih harga penuh (B-188)")
	}
	if evs[1].ProviderModelID == "" {
		t.Error("event HIT tanpa ProviderModelID; kombinasi regresi B-188 tidak terwakili (V-GW-09 memasangnya)")
	}
}
