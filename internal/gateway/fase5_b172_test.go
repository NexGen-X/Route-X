package gateway

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/NexGen-X/Route-X/internal/providers"
)

// Test pengunci B-172 (Fase 5 kampanye tuntas-logika): putusnya
// penulisan ke klien di tengah stream tercatat sebagai sukses penuh.
//
// Kedua permukaan streaming sebelumnya hanya me-log kegagalan tulis /
// flush lalu kembali: jejak melihat status 200 + kind kosong, sehingga
// error_type kosong dan stream yang tidak pernah diterima utuh klien
// tak terbedakan dari sukses di dashboard maupun rollup. Fix-nya
// menandai ErrKindCanceled pada jalur gagal-tulis/flush — status
// tetap 200, error_type "canceled" yang memisahkannya.

// penulisPutus adalah http.ResponseWriter yang gagal menulis sesudah
// `batas` penulisan berhasil — simulasi klien yang menutup koneksi di
// tengah aliran.
type penulisPutus struct {
	header http.Header
	status int
	tulis  int
	batas  int
}

func (p *penulisPutus) Header() http.Header {
	if p.header == nil {
		p.header = http.Header{}
	}
	return p.header
}

func (p *penulisPutus) WriteHeader(status int) { p.status = status }

func (p *penulisPutus) Write(b []byte) (int, error) {
	p.tulis++
	if p.tulis > p.batas {
		return 0, errors.New("koneksi klien terputus")
	}
	return len(b), nil
}

func (p *penulisPutus) Flush() {}

func TestB172ChatMengalirGagalTulisTercatatCanceled(t *testing.T) {
	prov := &providerTiruan{
		nama: "utama", kind: providers.KindOpenAI,
		alir: func(_ context.Context, _ *providers.ChatRequest) (providers.Stream, error) {
			return &aliranTiruan{ev: []*providers.StreamEvent{
				peristiwa("satu"), peristiwa("dua"),
				{FinishReason: providers.FinishStop},
			}}, nil
		},
	}
	s, sink := susunDenganPencatat(t, prov, nil)

	r := httptest.NewRequest(http.MethodPost, "/chat/completions",
		strings.NewReader(`{"model":"gpt-4o-mini","stream":true,"messages":[{"role":"user","content":"halo"}]}`))
	w := &penulisPutus{batas: 0} // penulisan pertama langsung gagal
	s.h.Routes().ServeHTTP(w, r)

	ev := sink.satu(t)
	if ev.StatusCode != http.StatusOK {
		t.Errorf("status tercatat = %d, mau 200 (status yang terkirim memang 200)", ev.StatusCode)
	}
	if ev.ErrorType != string(providers.ErrKindCanceled) {
		t.Errorf("B-172: error_type = %q, mau %q — aliran yang putus saat ditulis tidak boleh tercatat sukses penuh",
			ev.ErrorType, providers.ErrKindCanceled)
	}
}

func TestB172AnthropicGagalTulisTercatatCanceled(t *testing.T) {
	prov := &providerTiruan{
		nama: "utama", kind: providers.KindAnthropic,
		alir: func(_ context.Context, _ *providers.ChatRequest) (providers.Stream, error) {
			return &aliranTiruan{ev: []*providers.StreamEvent{
				{Delta: "satu"}, {Delta: "dua"},
				{FinishReason: providers.FinishStop},
			}}, nil
		},
	}
	s, sink := susunDenganPencatat(t, prov, nil)

	r := httptest.NewRequest(http.MethodPost, "/v1/messages",
		strings.NewReader(`{"model":"claude-sonnet-4-5","max_tokens":64,"stream":true,"messages":[{"role":"user","content":"halo"}]}`))
	w := &penulisPutus{batas: 0} // message_start pun tidak sampai
	s.h.Routes().ServeHTTP(w, r)

	ev := sink.satu(t)
	if ev.StatusCode != http.StatusOK {
		t.Errorf("status tercatat = %d, mau 200", ev.StatusCode)
	}
	if ev.ErrorType != string(providers.ErrKindCanceled) {
		t.Errorf("B-172: error_type = %q, mau %q — permukaan Anthropic wajib menandai putus tulis yang sama",
			ev.ErrorType, providers.ErrKindCanceled)
	}
}

// Kontrol: aliran yang diterima utuh tetap tercatat sukses bersih —
// penanda canceled tidak boleh bocor ke jalur normal.
func TestB172AliranUtuhTetapSuksesBersih(t *testing.T) {
	prov := &providerTiruan{
		nama: "utama", kind: providers.KindOpenAI,
		alir: func(_ context.Context, _ *providers.ChatRequest) (providers.Stream, error) {
			return &aliranTiruan{ev: []*providers.StreamEvent{
				peristiwa("satu"),
				{FinishReason: providers.FinishStop},
			}}, nil
		},
	}
	s, sink := susunDenganPencatat(t, prov, nil)

	w := s.panggil(t, http.MethodPost, "/chat/completions",
		`{"model":"gpt-4o-mini","stream":true,"messages":[{"role":"user","content":"halo"}]}`, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, mau 200", w.Code)
	}
	ev := sink.satu(t)
	if ev.ErrorType != "" {
		t.Errorf("kontrol: error_type = %q, mau kosong untuk aliran utuh", ev.ErrorType)
	}
}
