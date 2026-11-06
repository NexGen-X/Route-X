package gateway

import (
	"context"
	"math"
	"net/http"
	"strings"
	"testing"

	"github.com/NexGen-X/Route-X/internal/providers"
)

// TestB358AliranTypedNilDitolakTerkendali adalah pengunci B-358:
// bungkusPencatatAliran dulu hanya menyaring str == nil pada
// antarmuka — adapter yang mengembalikan (typed-nil, nil) lolos
// dan meledak sebagai panic pada intipan Recv pertama di
// cobaKandidatStream (pelanggaran kontrak yang sama yang sudah
// diubah B-335 menjadi error terkendali di jalur non-streaming).
// Kini pelanggaran itu menjadi kegagalan membuka berkind server.
func TestB358AliranTypedNilDitolakTerkendali(t *testing.T) {
	const bodyAliran = `{"model":"gpt-4o-mini","stream":true,"messages":[{"role":"user","content":"hai"}]}`

	t.Run("typed-nil", func(t *testing.T) {
		prov := &providerTiruan{
			nama: "utama", kind: providers.KindOpenAI,
			alir: func(_ context.Context, _ *providers.ChatRequest) (providers.Stream, error) {
				return (*aliranTiruan)(nil), nil
			},
		}
		s := susun(t, prov, nil)
		w := panggilTanpaPanic(t, s, "/chat/completions", bodyAliran)
		if w.Code < 500 {
			t.Errorf("status = %d, mau 5xx terkendali (body: %s)", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), `"error"`) {
			t.Errorf("body = %q, mau envelope error", w.Body.String())
		}
	})

	// Kontrol: nil polos juga terkendali (sudah benar sebelum fix),
	// dan aliran sehat tetap mengalir.
	t.Run("nil polos", func(t *testing.T) {
		prov := &providerTiruan{
			nama: "utama", kind: providers.KindOpenAI,
			alir: func(_ context.Context, _ *providers.ChatRequest) (providers.Stream, error) {
				return nil, nil
			},
		}
		s := susun(t, prov, nil)
		w := panggilTanpaPanic(t, s, "/chat/completions", bodyAliran)
		if w.Code < 500 {
			t.Errorf("status = %d, mau 5xx terkendali (body: %s)", w.Code, w.Body.String())
		}
	})

	t.Run("aliran sehat tetap mengalir", func(t *testing.T) {
		prov := &providerTiruan{
			nama: "utama", kind: providers.KindOpenAI,
			alir: func(_ context.Context, _ *providers.ChatRequest) (providers.Stream, error) {
				return &aliranTiruan{ev: []*providers.StreamEvent{
					{Delta: "halo"},
				}}, nil
			},
		}
		s := susun(t, prov, nil)
		w := s.panggil(t, http.MethodPost, "/chat/completions", bodyAliran, nil)
		if w.Code != http.StatusOK {
			t.Errorf("status = %d, mau 200 (body: %s)", w.Code, w.Body.String())
		}
	})
}

// TestB362LantaiPemakaianMenjenuh adalah pengunci B-362:
// terapkanLantaiPemakaian dulu menaikkan TotalTokens dengan +=
// polos — total bawaan upstream yang dekat MaxInt membungkus
// negatif sesudah ditambah taksiran lantai, dan total negatif
// membuat pencatatan kuota token terlewat senyap (penjaga menolak
// token <= 0). Kini penjumlahan lantai memakai penjaga jenuh
// providers.TambahToken yang sama dengan adapter (B-352).
func TestB362LantaiPemakaianMenjenuh(t *testing.T) {
	req := &providers.ChatRequest{Messages: []providers.Message{
		{Role: "user", Content: "halo dunia, ini kalimat taksiran yang cukup panjang"},
	}}
	u := providers.Usage{TotalTokens: math.MaxInt}
	hasil := terapkanLantaiPemakaian(req, u, 400)
	if hasil.TotalTokens != math.MaxInt {
		t.Errorf("TotalTokens = %d, mau jenuh di MaxInt (%d) — penjumlahan lantai membungkus", hasil.TotalTokens, math.MaxInt)
	}
	if hasil.TotalTokens <= 0 {
		t.Errorf("TotalTokens = %d tidak positif — pencatatan kuota akan terlewat", hasil.TotalTokens)
	}
	if hasil.InputTokens <= 0 || hasil.OutputTokens <= 0 {
		t.Errorf("lantai tidak terpasang: %+v, mau Input dan Output terisi taksiran", hasil)
	}

	// Kontrol nilai wajar: lantai menjumlah persis seperti biasa
	// (Input/Output awalnya 0 dan diganti taksiran; total = bawaan
	// + taksiran masuk + taksiran keluar).
	u2 := providers.Usage{TotalTokens: 10}
	hasil2 := terapkanLantaiPemakaian(req, u2, 400)
	if hasil2.TotalTokens != 10+hasil2.InputTokens+hasil2.OutputTokens {
		// InputTokens/OutputTokens pada u2 awalnya 0 dan diganti
		// taksiran; total = 10 + taksiran masuk + taksiran keluar.
		t.Errorf("TotalTokens wajar = %d, mau %d", hasil2.TotalTokens, 10+hasil2.InputTokens+hasil2.OutputTokens)
	}
}
