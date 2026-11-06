package worker

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/NexGen-X/Route-X/internal/oauth"
)

// TestB365PenonaktifanSesiMemanggilCallback adalah pengunci
// B-365 sisi rantai refresh: penonaktifan sesi saat refresh token
// ditolak permanen mengubah himpunan kredensial, tetapi tidak ada
// siraman cache respons di jalur itu. RefreshSesiOAuth kini
// memanggil deps.SesudahNonaktif sesudah SetEnabled(false)
// berhasil — pemilik proses (main, handler admin) memasang
// siraman cache-nya di sana. Callback TIDAK dipanggil untuk
// kegagalan transien maupun refresh yang berhasil.
func TestB365PenonaktifanSesiMemanggilCallback(t *testing.T) {
	tanpaJeda(t)
	ctx := context.Background()

	t.Run("ditolak permanen memanggil callback dan menonaktifkan", func(t *testing.T) {
		env := siapkanRefreshG8(t, false)
		klien := &klienOAuthPalsu{perilaku: func(int) (*oauth.TokenResult, error) {
			return nil, fmt.Errorf("%w: invalid_grant", oauth.ErrTokenDitolakPermanen)
		}}
		deps := env.deps(klien)
		dipanggil := 0
		deps.SesudahNonaktif = func(context.Context) { dipanggil++ }

		if err := RefreshSesiOAuth(ctx, deps, env.sesi); err == nil {
			t.Fatal("refresh yang ditolak permanen harus mengembalikan error")
		}
		if dipanggil != 1 {
			t.Errorf("callback SesudahNonaktif dipanggil %d kali, mau 1", dipanggil)
		}
		var aktif bool
		if err := env.pool.QueryRow(ctx,
			`select enabled from provider_oauth_sessions where id = $1`, env.meta.ID).Scan(&aktif); err != nil {
			t.Fatalf("baca status sesi: %v", err)
		}
		if aktif {
			t.Error("sesi masih aktif sesudah penolakan permanen")
		}
	})

	t.Run("kegagalan transien tidak memanggil callback", func(t *testing.T) {
		env := siapkanRefreshG8(t, false)
		klien := &klienOAuthPalsu{perilaku: func(int) (*oauth.TokenResult, error) {
			return nil, errors.New("jaringan putus sesaat")
		}}
		deps := env.deps(klien)
		dipanggil := 0
		deps.SesudahNonaktif = func(context.Context) { dipanggil++ }

		if err := RefreshSesiOAuth(ctx, deps, env.sesi); err == nil {
			t.Fatal("refresh yang gagal transien harus mengembalikan error")
		}
		if dipanggil != 0 {
			t.Errorf("callback dipanggil %d kali untuk kegagalan transien, mau 0", dipanggil)
		}
		var aktif bool
		if err := env.pool.QueryRow(ctx,
			`select enabled from provider_oauth_sessions where id = $1`, env.meta.ID).Scan(&aktif); err != nil {
			t.Fatalf("baca status sesi: %v", err)
		}
		if !aktif {
			t.Error("sesi ikut nonaktif oleh kegagalan transien")
		}
	})
}
