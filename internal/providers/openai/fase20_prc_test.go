package openai

// fase20_prc_test.go — pengunci Fase 20 PR-C (sisi OpenAI).
//
//	B-460 error peristiwa-pertama streaming membawa Retry-After
//	      dari header respons pembuka aliran (200), bukan 0.

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/NexGen-X/Route-X/internal/providers"
)

// TestB460RetryAfterPembukaAliranOpenAI membuktikan aliran yang
// dibuka 200 dengan header Retry-After lalu gagal pada peristiwa
// pertama menghasilkan error ber-RetryAfter sesuai header — jalur
// yang dulu selalu 0 karena chatStream tidak menyimpan header.
func TestB460RetryAfterPembukaAliranOpenAI(t *testing.T) {
	u := newUpstream(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Retry-After", "17")
		w.WriteHeader(http.StatusOK)
		rc := http.NewResponseController(w)
		_, _ = w.Write([]byte(`data: {"error":{"message":"kuota habis","type":"rate_limit_error","code":"rate_limit_exceeded"}}` + "\n\n"))
		_ = rc.Flush()
	}))
	p := testProvider(t, u.url(), nil)

	st, err := p.ChatCompletionStream(context.Background(), chatRequest())
	if err != nil {
		t.Fatalf("ChatCompletionStream: %v", err)
	}
	defer st.Close()
	_, err = st.Recv()
	pe := providers.AsError(err)
	if pe == nil {
		t.Fatalf("Recv: err = %v, mau *providers.Error", err)
	}
	if pe.Kind != providers.ErrKindRateLimit {
		t.Errorf("Kind = %q, mau rate_limit", pe.Kind)
	}
	if pe.RetryAfter != 17*time.Second {
		t.Errorf("RetryAfter = %v, mau 17s dari header pembuka aliran", pe.RetryAfter)
	}
}
