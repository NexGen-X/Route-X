package gateway

import (
	"context"
	"testing"

	"github.com/NexGen-X/Route-X/internal/providers"
)

// B-255: gerbang jenis-error pemutus arus hanya ada di jalur non-streaming
// (cobaKandidat). Jalur streaming melaporkan lewat aliranPencatat yang
// hanya tahu sukses/gagal dari bentuk error — error tengah-aliran berjenis
// InvalidRequest/ContentFilter/ModelNotFound/Local (fakta tentang
// PERMINTAAN, bukan kesehatan provider) tercatat sebagai kegagalan
// provider dan membuka pemutus arus bagi trafik lain yang sah.
func TestAliranPencatatMenghormatiPengecualianBreakerB255(t *testing.T) {
	dikecualikan := []struct {
		nama string
		err  *providers.Error
	}{
		{"invalid request", providers.Newf(providers.ErrKindInvalidRequest, "uji", "permintaan tidak sah")},
		{"content filter", providers.Newf(providers.ErrKindContentFilter, "uji", "ditolak filter konten")},
		{"model not found", providers.Newf(providers.ErrKindModelNotFound, "uji", "model tidak dikenal")},
		{"canceled berjenis", providers.Newf(providers.ErrKindCanceled, "uji", "klien pergi")},
		{"lokal", &providers.Error{Kind: providers.ErrKindServer, Provider: "uji", Message: "keputusan gateway sendiri", Local: true}},
	}
	for _, tc := range dikecualikan {
		t.Run(tc.nama, func(t *testing.T) {
			var hasil []bool
			a := &aliranPencatat{
				Stream: &aliranHasil{hasil: []error{nil, tc.err}},
				ctx:    context.Background(),
				catat:  func(s bool) { hasil = append(hasil, s) },
			}
			_, _ = a.Recv()
			_, _ = a.Recv()
			_ = a.Close()
			if len(hasil) != 0 {
				t.Errorf("catatan = %v, mau kosong — jenis %s dikecualikan dari pencatatan pemutus arus", hasil, tc.nama)
			}
		})
	}

	// Kontrol: error tengah-aliran yang BUKAN pengecualian tetap tercatat
	// sebagai kegagalan, tepat satu kali.
	t.Run("server tetap tercatat gagal", func(t *testing.T) {
		var hasil []bool
		a := &aliranPencatat{
			Stream: &aliranHasil{hasil: []error{nil, providers.Newf(providers.ErrKindServer, "uji", "upstream rusak")}},
			ctx:    context.Background(),
			catat:  func(s bool) { hasil = append(hasil, s) },
		}
		_, _ = a.Recv()
		_, _ = a.Recv()
		_ = a.Close()
		if len(hasil) != 1 || hasil[0] {
			t.Errorf("catatan = %v, mau [false]", hasil)
		}
	})
}
