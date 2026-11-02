package traffic

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/NexGen-X/Route-X/internal/database/repo"
)

// Grup 12 audit3 (V-EM-31): kesalahan parameter agregasi & filter harus
// membawa sentinel repo.ErrInvalidParameter agar lapisan HTTP menjawab 400,
// bukan 500 dari cabang default. Semua kasus gagal di rencanakan/penyusunan
// filter — sebelum pool disentuh — jadi Repo kosong cukup.

func TestParameterTidakSahMembawaSentinelGrup12(t *testing.T) {
	r := &Repo{}
	ctx := context.Background()
	dari := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	sampai := dari.Add(24 * time.Hour)

	t.Run("summary sumber asing", func(t *testing.T) {
		_, err := r.Summary(ctx, Query{From: dari, To: sampai, Source: Source("per_menit")})
		if !errors.Is(err, repo.ErrInvalidParameter) {
			t.Errorf("err = %v, mau ErrInvalidParameter", err)
		}
	})

	t.Run("summary granularitas asing", func(t *testing.T) {
		_, err := r.Summary(ctx, Query{From: dari, To: sampai, Granularity: Granularity("per_detik")})
		if !errors.Is(err, repo.ErrInvalidParameter) {
			t.Errorf("err = %v, mau ErrInvalidParameter", err)
		}
	})

	t.Run("summary rentang kosong", func(t *testing.T) {
		_, err := r.Summary(ctx, Query{From: sampai, To: dari})
		if !errors.Is(err, repo.ErrInvalidParameter) {
			t.Errorf("err = %v, mau ErrInvalidParameter", err)
		}
	})

	t.Run("breakdown dimensi asing", func(t *testing.T) {
		_, err := r.Breakdown(ctx, Query{From: dari, To: sampai}, Dimension("region"), 10)
		if !errors.Is(err, repo.ErrInvalidParameter) {
			t.Errorf("err = %v, mau ErrInvalidParameter", err)
		}
	})

	t.Run("list status bukan kode atau kelas", func(t *testing.T) {
		_, err := r.List(ctx, Filter{From: dari, To: sampai, Status: "sukses"}, repo.Page{Limit: 10})
		if !errors.Is(err, repo.ErrInvalidParameter) {
			t.Errorf("err = %v, mau ErrInvalidParameter", err)
		}
	})
}
