package keys

import (
	"net/netip"
	"testing"
)

// TestAuthenticateMenolakKeyMilikPemilikNonaktif membuktikan perbaikan audit3
// V-AU-01 (B-039): status pemilik menegakkan diri ke API key pada jalur
// Authenticate. Sebelumnya sesi pemilik mati saat akun dinonaktifkan/di-reset
// paksa, tetapi key miliknya tetap bisa dipakai tanpa batas.
func TestAuthenticateMenolakKeyMilikPemilikNonaktif(t *testing.T) {
	anyIP := netip.MustParseAddr("203.0.113.9")

	t.Run("pemilik dinonaktifkan", func(t *testing.T) {
		ctx, pool, r := testEnv(t)
		owner := newUser(t, ctx, pool)
		created := mustCreate(t, ctx, r, CreateParams{Name: "k", OwnerUserID: owner, Live: true})

		if _, err := r.Authenticate(ctx, created.Raw.Reveal(), anyIP); err != nil {
			t.Fatalf("Authenticate sebelum penonaktifan: %v", err)
		}

		exec(t, ctx, pool, `update users set status = 'disabled' where id = $1`, owner)
		_, err := r.Authenticate(ctx, created.Raw.Reveal(), anyIP)
		assertRejection(t, err, ReasonOwnerInactive)

		// Mengaktifkan kembali pemilik menghidupkan lagi key yang sama: penolakan
		// ini penegakan status, bukan pencabutan permanen.
		exec(t, ctx, pool, `update users set status = 'active' where id = $1`, owner)
		if _, err := r.Authenticate(ctx, created.Raw.Reveal(), anyIP); err != nil {
			t.Fatalf("Authenticate sesudah pemilik aktif lagi: %v", err)
		}
	})

	t.Run("pemilik wajib ganti password karena reset paksa", func(t *testing.T) {
		ctx, pool, r := testEnv(t)
		owner := newUser(t, ctx, pool)
		created := mustCreate(t, ctx, r, CreateParams{Name: "k", OwnerUserID: owner, Live: true})

		exec(t, ctx, pool, `update users set must_change_password = true where id = $1`, owner)
		_, err := r.Authenticate(ctx, created.Raw.Reveal(), anyIP)
		assertRejection(t, err, ReasonOwnerInactive)

		exec(t, ctx, pool, `update users set must_change_password = false where id = $1`, owner)
		if _, err := r.Authenticate(ctx, created.Raw.Reveal(), anyIP); err != nil {
			t.Fatalf("Authenticate sesudah password diganti: %v", err)
		}
	})

}

// TestRotateKeyDisabledMenghasilkanPenggantiDisabled membuktikan perbaikan
// audit3 V-AU-09 (B-039): rotasi key 'disabled' tidak lagi menghidupkan
// penggantinya secara diam-diam.
func TestRotateKeyDisabledMenghasilkanPenggantiDisabled(t *testing.T) {
	ctx, pool, r := testEnv(t)
	owner := newUser(t, ctx, pool)
	anyIP := netip.MustParseAddr("203.0.113.9")

	old := mustCreate(t, ctx, r, CreateParams{Name: "k", OwnerUserID: owner, Live: true})
	if _, err := r.SetStatus(ctx, old.Key.ID, StatusDisabled); err != nil {
		t.Fatalf("SetStatus disabled: %v", err)
	}

	rotated, err := RotateKey(ctx, pool, testPepper, old.Key.ID, nil)
	if err != nil {
		t.Fatalf("RotateKey: %v", err)
	}
	if rotated.Key.Status != StatusDisabled {
		t.Errorf("status pengganti = %q, mau %q (status key lama wajib diwarisi)", rotated.Key.Status, StatusDisabled)
	}
	// Pengganti yang disabled benar-benar tidak bisa dipakai.
	_, err = r.Authenticate(ctx, rotated.Raw.Reveal(), anyIP)
	assertRejection(t, err, ReasonDisabled)

	// Mengaktifkan pengganti adalah langkah sadar yang terpisah, dan sesudahnya
	// key bisa dipakai — membuktikan statusnya memang 'disabled', bukan rusak.
	if _, err := r.SetStatus(ctx, rotated.Key.ID, StatusActive); err != nil {
		t.Fatalf("SetStatus active pada pengganti: %v", err)
	}
	if _, err := r.Authenticate(ctx, rotated.Raw.Reveal(), anyIP); err != nil {
		t.Fatalf("Authenticate pengganti sesudah diaktifkan: %v", err)
	}
}

// TestRotateKeyAktifTetapMenghasilkanPenggantiAktif menjaga arah sebaliknya:
// perbaikan status warisan tidak boleh mengubah perilaku rotasi key aktif.
func TestRotateKeyAktifTetapMenghasilkanPenggantiAktif(t *testing.T) {
	ctx, pool, r := testEnv(t)
	owner := newUser(t, ctx, pool)

	old := mustCreate(t, ctx, r, CreateParams{Name: "k", OwnerUserID: owner, Live: true})
	rotated, err := RotateKey(ctx, pool, testPepper, old.Key.ID, nil)
	if err != nil {
		t.Fatalf("RotateKey: %v", err)
	}
	if rotated.Key.Status != StatusActive {
		t.Errorf("status pengganti = %q, mau %q", rotated.Key.Status, StatusActive)
	}
}
