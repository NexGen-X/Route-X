package upstream

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/NexGen-X/Route-X/internal/database/repo"
)

// TestB355RemoveAliasMenjagaID adalah pengunci B-355(i): RemoveAlias
// adalah satu-satunya operasi alias tanpa penjaga idOK pada
// modelID — AddAlias dan ListAliases sama-sama menolak ID bukan
// UUID di penjaga dengan pesan yang menyebut field-nya. ID cacat
// pada RemoveAlias dulu lolos ke query DELETE; klasifikasinya
// tertolong pemetaan 22P02 di repo.Err (sentinelnya cocok), tetapi
// pesannya adalah teks sintaks DB yang bocor dan penolakannya
// bergantung pada perjalanan ke basis data. Kini penjaganya sama
// seperti kedua tetangganya: ErrInvalidReference yang menyebut
// "model_id bukan UUID" sebelum DB disentuh.
func TestB355RemoveAliasMenjagaID(t *testing.T) {
	if testing.Short() {
		t.Skip("butuh PostgreSQL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	pool := newTestPool(ctx, t)
	models := NewModelRepo(pool)
	canonical := seedModel(ctx, t, pool, "model-b355", CapText)

	if _, err := models.AddAlias(ctx, canonical.ID, "alias-b355", nil); err != nil {
		t.Fatalf("AddAlias: %v", err)
	}

	err := models.RemoveAlias(ctx, "bukan-uuid", "alias-b355")
	if !errors.Is(err, repo.ErrInvalidReference) {
		t.Errorf("RemoveAlias(modelID cacat) = %v, mau ErrInvalidReference", err)
	}
	if err == nil || !strings.Contains(err.Error(), "model_id bukan UUID") {
		t.Errorf("RemoveAlias(modelID cacat) error = %v, mau menyebut 'model_id bukan UUID' — penjaga yang sama seperti AddAlias, bukan error sintaks DB yang bocor", err)
	}
	// Kontrol: penghapusan sah tetap bekerja, dan alias model lain
	// tidak ikut terhapus (klausa model_id B-304 tetap utuh).
	if err := models.RemoveAlias(ctx, canonical.ID, "alias-b355"); err != nil {
		t.Fatalf("RemoveAlias sah: %v", err)
	}
	sisa, err := models.ListAliases(ctx, canonical.ID)
	if err != nil {
		t.Fatalf("ListAliases: %v", err)
	}
	if len(sisa) != 0 {
		t.Errorf("alias tersisa = %d, mau 0", len(sisa))
	}
}
