package gateway

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
)

// --- B-143: semua provider pemetaan nonaktif bukan "tidak dikonfigurasi" ---

// sumberKandidatNonaktif meniru keadaan live B-143: pemetaan provider
// ADA, tetapi query kandidat biasa (dan query IncludeUnhealthy) kosong
// karena setiap provider/pemetaannya dinonaktifkan — kandidat hanya
// muncul pada query diagnosa IncludeDisabled.
type sumberKandidatNonaktif struct {
	semua []*upstream.RouteCandidate
}

func (s *sumberKandidatNonaktif) RouteCandidates(_ context.Context, q upstream.RouteQuery) ([]*upstream.RouteCandidate, error) {
	if q.IncludeDisabled {
		return s.semua, nil
	}
	return nil, nil
}

func TestSemuaProviderNonaktifJadi503ProviderDisabled(t *testing.T) {
	s := susun(t, providerSukses(), func(_ *susunan, d *HandlersDeps) {
		d.Candidates = &sumberKandidatNonaktif{
			semua: []*upstream.RouteCandidate{kandidatRute("utama", "gpt-4o-mini-2024")},
		}
	})
	w := s.panggil(t, http.MethodPost, "/chat/completions", bodyChatMinimal, nil)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, mau 503 (body: %s)", w.Code, w.Body.String())
	}
	if got := kodeGalat(galat(t, w)); got != "provider_disabled" {
		t.Errorf("code = %q, mau provider_disabled — bukan no_provider_for_model (B-143)", got)
	}
	if pesan := galat(t, w).Message; !strings.Contains(pesan, "dinonaktifkan") {
		t.Errorf("pesan harus menyebut penonaktifan, dapat: %q", pesan)
	}
}

// Arah sebaliknya: pemetaan benar-benar tidak ada (query diagnosa
// IncludeDisabled pun kosong) — kode lama tetap keluar apa adanya.
func TestTanpaPemetaanSamaSekaliTetapNoProviderForModel(t *testing.T) {
	s := susun(t, providerSukses(), func(_ *susunan, d *HandlersDeps) {
		d.Candidates = &sumberKandidatNonaktif{semua: nil}
	})
	w := s.panggil(t, http.MethodPost, "/chat/completions", bodyChatMinimal, nil)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, mau 503 (body: %s)", w.Code, w.Body.String())
	}
	if got := kodeGalat(galat(t, w)); got != "no_provider_for_model" {
		t.Errorf("code = %q, mau no_provider_for_model (B-143 arah sebaliknya)", got)
	}
}

// sumberKandidatSebagianNonaktif: satu provider nonaktif, satu aktif —
// query biasa menghasilkan kandidat aktif dan diagnosa tidak berjalan.
type sumberKandidatSebagianNonaktif struct {
	aktif []*upstream.RouteCandidate
	semua []*upstream.RouteCandidate
}

func (s *sumberKandidatSebagianNonaktif) RouteCandidates(_ context.Context, q upstream.RouteQuery) ([]*upstream.RouteCandidate, error) {
	if q.IncludeDisabled {
		return s.semua, nil
	}
	return s.aktif, nil
}

func TestSebagianNonaktifKandidatAktifTetapMelayani(t *testing.T) {
	aktif := kandidatRute("utama", "gpt-4o-mini-2024")
	s := susun(t, providerSukses(), func(_ *susunan, d *HandlersDeps) {
		d.Candidates = &sumberKandidatSebagianNonaktif{
			aktif: []*upstream.RouteCandidate{aktif},
			semua: []*upstream.RouteCandidate{aktif, kandidatRute("cadangan", "gpt-4o-mini-2024")},
		}
	})
	w := s.panggil(t, http.MethodPost, "/chat/completions", bodyChatMinimal, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, mau 200 — kandidat aktif tetap melayani (body: %s)", w.Code, w.Body.String())
	}
}
