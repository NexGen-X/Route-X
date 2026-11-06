package gateway

import (
	"context"
	"testing"

	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
	"github.com/NexGen-X/Route-X/internal/providers"
)

// Test regresi B-178: invalid_request pada pipeline multi-model bersifat
// per-model. Sebelum perbaikan, gerbang failover menghentikan seluruh
// rencana pada invalid_request anggota pertama — anggota sehat dari
// model lain tidak pernah dicoba, persis skenario yang menjadi alasan
// combo ada (validator satu model menolak, penerjemah model lain
// menerima; pola B-161).

func kandidatModel(nama, model string) *upstream.RouteCandidate {
	return kandidat(nama, func(c *upstream.RouteCandidate) { c.ModelID = model })
}

func TestExecuteInvalidRequestLanjutAntarModelB178(t *testing.T) {
	g := penjaga()
	e, _ := executorUji(g)

	a1 := kandidatModel("a1", "m-1")
	a2 := kandidatModel("a2", "m-1")
	b1 := kandidatModel("b1", "m-2")

	var dipanggil []string
	out := Execute(context.Background(), e, rencana("m-1", 3, 0, a1, a2, b1),
		func(_ context.Context, c *upstream.RouteCandidate) (string, error) {
			dipanggil = append(dipanggil, c.ProviderName)
			if c == b1 {
				return "ok", nil
			}
			return "", providers.Newf(providers.ErrKindInvalidRequest, c.ProviderName, "skema ditolak")
		})

	if out.Err != nil {
		t.Fatalf("Err = %v, mau sukses lewat model lain", out.Err)
	}
	if out.Candidate != b1 {
		t.Errorf("Candidate = %v, mau b1 (model m-2)", out.Candidate)
	}
	if len(dipanggil) != 2 || dipanggil[0] != "a1" || dipanggil[1] != "b1" {
		t.Errorf("op dipanggil untuk %v, mau [a1 b1] — a2 semodel dengan a1 wajib dilewati, bukan dicoba", dipanggil)
	}
	// Lewatan a2 tetap tercatat dengan alasan, bukan hilang diam-diam.
	var lewatanA2 bool
	for _, att := range out.Attempts {
		if att.ProviderName == "a2" && att.SkipReason != "" {
			lewatanA2 = true
		}
	}
	if !lewatanA2 {
		t.Errorf("tidak ada percobaan lewatan ber-SkipReason untuk a2 pada Attempts: %+v", out.Attempts)
	}
}

func TestExecuteInvalidRequestBerhentiBilaHanyaModelSamaB178(t *testing.T) {
	g := penjaga()
	e, _ := executorUji(g)

	var dipanggil int
	out := Execute(context.Background(), e, rencana("m-1", 3, 0,
		kandidatModel("a1", "m-1"), kandidatModel("a2", "m-1")),
		func(_ context.Context, c *upstream.RouteCandidate) (string, error) {
			dipanggil++
			return "", providers.Newf(providers.ErrKindInvalidRequest, c.ProviderName, "skema ditolak")
		})

	if out.Err == nil || out.Err.Kind != providers.ErrKindInvalidRequest {
		t.Fatalf("Err = %v, mau invalid_request", out.Err)
	}
	if dipanggil != 1 {
		t.Errorf("op dipanggil %d kali, mau 1 — invalid_request tidak failover antar-provider model yang sama", dipanggil)
	}
}

func TestExecuteInvalidRequestModelTakDikenalTetapBerhentiB178(t *testing.T) {
	g := penjaga()
	e, _ := executorUji(g)

	// Kandidat kedua TANPA ModelID: identitas modelnya tidak diketahui,
	// jadi ia tidak boleh dihitung sebagai "model lain" (gagal-tertutup
	// ke perilaku lama).
	var dipanggil int
	out := Execute(context.Background(), e, rencana("m-1", 3, 0,
		kandidatModel("a1", "m-1"), kandidat("b1")),
		func(_ context.Context, c *upstream.RouteCandidate) (string, error) {
			dipanggil++
			return "", providers.Newf(providers.ErrKindInvalidRequest, c.ProviderName, "skema ditolak")
		})

	if out.Err == nil || out.Err.Kind != providers.ErrKindInvalidRequest {
		t.Fatalf("Err = %v, mau invalid_request", out.Err)
	}
	if dipanggil != 1 {
		t.Errorf("op dipanggil %d kali, mau 1 — kandidat tanpa ModelID bukan bukti model lain", dipanggil)
	}
}

func TestExecuteInvalidRequestDuaModelDitolakKetigaBerhasilB178(t *testing.T) {
	g := penjaga()
	e, _ := executorUji(g)

	c1 := kandidatModel("c1", "m-3")
	var dipanggil []string
	out := Execute(context.Background(), e, rencana("m-1", 3, 0,
		kandidatModel("a1", "m-1"), kandidatModel("b1", "m-2"), c1),
		func(_ context.Context, c *upstream.RouteCandidate) (string, error) {
			dipanggil = append(dipanggil, c.ProviderName)
			if c == c1 {
				return "ok", nil
			}
			return "", providers.Newf(providers.ErrKindInvalidRequest, c.ProviderName, "skema ditolak")
		})

	if out.Err != nil || out.Candidate != c1 {
		t.Fatalf("out = (Err %v, Candidate %v), mau sukses lewat c1", out.Err, out.Candidate)
	}
	if len(dipanggil) != 3 {
		t.Errorf("op dipanggil %v, mau 3 kali (a1, b1, c1) — penolakan satu model tidak menular ke model lain", dipanggil)
	}
}
