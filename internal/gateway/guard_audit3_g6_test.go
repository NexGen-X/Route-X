package gateway

import (
	"context"
	"testing"
	"time"

	"github.com/NexGen-X/Route-X/internal/providers"
)

// TestMemoBatasPenolakanKedaluwarsaSetelahRetryAfter memverifikasi
// V-BR-04: penolakan batas yang dimemoisasi hanya berlaku selama
// RetryAfter-nya. Sesudah jendela limiter berganti, pemeriksaan
// berikutnya harus segar — bukan ditolak oleh keputusan basi dan tercatat
// sebagai percobaan upstream yang tidak pernah terjadi.
func TestMemoBatasPenolakanKedaluwarsaSesudahRetryAfter(t *testing.T) {
	g := &Guard{}
	ctx := g.DenganMemoBatas(context.Background())

	penolakan := &providers.Error{
		Kind:       providers.ErrKindRateLimit,
		Message:    "penuh",
		Local:      true,
		RetryAfter: 60 * time.Millisecond,
	}
	memoBatasSimpan(ctx, "kebijakan:prov-a", penolakan)

	// Selama RetryAfter belum lewat, memo menjawab tanpa pemeriksaan baru.
	tersimpan, ada := memoBatasAmbil(ctx, "kebijakan:prov-a")
	if !ada || tersimpan != penolakan {
		t.Fatalf("memo langsung = (%v, %v), mau penolakan yang sama", tersimpan, ada)
	}

	// Sesudah RetryAfter lewat, memo harus kosong: pemeriksaan segar yang
	// memutuskan, bukan penolakan basi.
	time.Sleep(90 * time.Millisecond)
	if tersimpan, ada := memoBatasAmbil(ctx, "kebijakan:prov-a"); ada {
		t.Fatalf("memo sesudah RetryAfter = %v, mau tidak ada (kedaluwarsa)", tersimpan)
	}
}

// TestMemoBatasPenolakanTanpaRetryAfterTidakDimemoisasi memverifikasi
// sisi lainnya: penolakan yang tidak menyebut masa berlakunya tidak
// dimemoisasi sama sekali, sementara keputusan lolos tetap dimemoisasi
// sepanjang permintaan (kuotanya sudah termakan satu kali).
func TestMemoBatasPenolakanTanpaRetryAfterTidakDimemoisasi(t *testing.T) {
	g := &Guard{}
	ctx := g.DenganMemoBatas(context.Background())

	memoBatasSimpan(ctx, "kebijakan:prov-a", &providers.Error{
		Kind:  providers.ErrKindRateLimit,
		Local: true,
	})
	if _, ada := memoBatasAmbil(ctx, "kebijakan:prov-a"); ada {
		t.Fatal("penolakan tanpa RetryAfter ikut dimemoisasi, mau langsung dibuang")
	}

	memoBatasSimpan(ctx, "kebijakan:prov-b", nil)
	tersimpan, ada := memoBatasAmbil(ctx, "kebijakan:prov-b")
	if !ada || tersimpan != nil {
		t.Fatalf("memo keputusan lolos = (%v, %v), mau (nil, true)", tersimpan, ada)
	}
}
