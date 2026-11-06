package worker

import (
	"context"
	"testing"

	"github.com/NexGen-X/Route-X/internal/oauth"
	"github.com/NexGen-X/Route-X/internal/security"
)

// TestRefreshSesiGagalSimpanTidakMengubahKredensial membuktikan B-208 dari
// jalur pemanggil sungguhan: bila penyimpanan refresh token rotasi gagal di
// tengah rangkaian (di sini baris sesi dihapus sesudah sesi dimuat), access
// token kredensial TIDAK ikut berubah — hasil refresh disimpan atomik atau
// tidak sama sekali. Di kode lama test ini gagal: UpdateSecret sudah
// terlanjur commit ketika UpdateRefreshToken menemukan sesinya hilang,
// meninggalkan access token baru berdampingan dengan refresh token lama
// yang sudah dianggap mati oleh penyedia yang merotasi.
func TestRefreshSesiGagalSimpanTidakMengubahKredensial(t *testing.T) {
	tanpaJeda(t)
	ctx := context.Background()
	env := siapkanRefreshG8(t, false)

	if _, err := env.pool.Exec(ctx,
		`delete from provider_oauth_sessions where id = $1`, env.meta.ID); err != nil {
		t.Fatalf("menghapus baris sesi: %v", err)
	}

	klien := &klienOAuthPalsu{perilaku: func(int) (*oauth.TokenResult, error) {
		return &oauth.TokenResult{
			AccessToken:  security.Secret("akses-baru-b208"),
			ExpiresIn:    3600,
			RefreshToken: security.Secret("refresh-rotasi-b208"),
		}, nil
	}}

	if err := RefreshSesiOAuth(ctx, env.deps(klien), env.sesi); err == nil {
		t.Fatal("refresh dengan sesi yang hilang di tengah jalan harus gagal")
	}

	rahasia, err := env.credRepo.Reveal(ctx, env.meta.CredentialID)
	if err != nil {
		t.Fatalf("Reveal kredensial: %v", err)
	}
	if rahasia.Reveal() != nilaiAksesWorkerG8().Reveal() {
		t.Errorf("access token kredensial berubah menjadi %q padahal penyimpanan hasil refresh gagal — penulisan tidak atomik (B-208)", rahasia.Reveal())
	}
}
