package apikey

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/NexGen-X/Route-X/internal/httpx"
)

// Grup 12 audit3 (B-021 / V-GW-46): penolakan middleware 401 & 429 harus
// dilaporkan ke RejectionSink agar menjadi baris log request.

// sinkPalsu menangkap RejectionInfo untuk assertions test.
type sinkPalsu struct {
	mu    sync.Mutex
	infos []RejectionInfo
}

func (s *sinkPalsu) RecordRejection(_ context.Context, info RejectionInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.infos = append(s.infos, info)
}

func (s *sinkPalsu) semua() []RejectionInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]RejectionInfo(nil), s.infos...)
}

func TestPenolakanAutentikasiDicatatGrup12(t *testing.T) {
	fake := newFakeRepo(testKey())
	fake.reason = "revoked"
	sink := &sinkPalsu{}
	a := NewAuthenticator(fake, nil, discardLogger(), WithRejectionSink(sink))

	rec := serve(a, okHandler(nil), bearerRequest())
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, mau 401", rec.Code)
	}

	infos := sink.semua()
	if len(infos) != 1 {
		t.Fatalf("sink menerima %d penolakan, mau 1", len(infos))
	}
	info := infos[0]
	if info.StatusCode != http.StatusUnauthorized {
		t.Errorf("StatusCode = %d, mau 401", info.StatusCode)
	}
	if info.ErrorType != "auth" || info.ErrorCode != httpx.CodeInvalidAPIKey {
		t.Errorf("kategori/kode = %q/%q, mau auth/%q", info.ErrorType, info.ErrorCode, httpx.CodeInvalidAPIKey)
	}
	if info.APIKeyID != "" {
		t.Errorf("APIKeyID = %q, mau kosong: key yang gagal verifikasi tidak boleh ditautkan", info.APIKeyID)
	}
	if info.Request == nil || info.Request.URL.Path != "/v1/chat/completions" {
		t.Errorf("Request tidak terbawa dengan benar: %+v", info.Request)
	}
}

func TestPenolakanLajuDicatatGrup12(t *testing.T) {
	sink := &sinkPalsu{}
	env := newLimiterEnv(t, WithLimiterRejectionSink(sink))
	rps := 1
	key := testKey()
	key.RateLimitRPS = &rps

	handler := httpx.RealIP(nil)(env.limiter.Limit()(okHandler(nil)))
	request := func() *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "/v1/chat/completions", nil)
		r = r.WithContext(WithPrincipal(r.Context(), &Principal{Key: key}))
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, r)
		return rec
	}

	if rec := request(); rec.Code != http.StatusOK {
		t.Fatalf("request pertama status = %d, mau 200", rec.Code)
	}
	if rec := request(); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("request kedua status = %d, mau 429", rec.Code)
	}

	infos := sink.semua()
	if len(infos) != 1 {
		t.Fatalf("sink menerima %d penolakan, mau 1", len(infos))
	}
	info := infos[0]
	if info.StatusCode != http.StatusTooManyRequests {
		t.Errorf("StatusCode = %d, mau 429", info.StatusCode)
	}
	if info.ErrorType != "rate_limit" || info.ErrorCode != httpx.CodeRateLimitExceeded {
		t.Errorf("kategori/kode = %q/%q, mau rate_limit/%q", info.ErrorType, info.ErrorCode, httpx.CodeRateLimitExceeded)
	}
	// Principal SUDAH terverifikasi pada penolakan laju: key pemiliknya sah
	// dicantumkan agar badai 429 per key terlihat.
	if info.APIKeyID != key.ID || info.APIKeyName != key.Name {
		t.Errorf("key = %q/%q, mau %q/%q", info.APIKeyID, info.APIKeyName, key.ID, key.Name)
	}
}

func TestTanpaSinkPenolakanTetapBerjalanGrup12(t *testing.T) {
	// Kompatibilitas: tanpa sink, perilaku penolakan tidak berubah.
	fake := newFakeRepo(testKey())
	fake.reason = "not_found"
	a := NewAuthenticator(fake, nil, discardLogger())

	rec := serve(a, okHandler(nil), bearerRequest())
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, mau 401", rec.Code)
	}
}
