package upstream

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/NexGen-X/Route-X/internal/database/repo"
	"github.com/NexGen-X/Route-X/internal/security"
)

// TestOAuthCodeAssistProjectRoundTrip membuktikan B-157 di lapisan repo:
// kolom code_assist_project bermigrasi, tersimpan lewat
// UpdateCodeAssistProject, terbaca konsisten dari metadata maupun sesi
// aktif, dan sesi bisa ditemukan dari ID kredensialnya — jalur yang dipakai
// factory dan sync admin yang hanya memegang kredensial aktif.
func TestOAuthCodeAssistProjectRoundTrip(t *testing.T) {
	if testing.Short() {
		t.Skip("butuh PostgreSQL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	pool := newTestPool(ctx, t)
	cipher := testCipher(t, 0xD7)
	cred := seedOAuthCredential(ctx, t, pool, cipher, "provider-oauth-project")

	oauthRepo, err := NewOAuthRepo(pool, cipher)
	if err != nil {
		t.Fatalf("NewOAuthRepo: %v", err)
	}

	meta, err := oauthRepo.UpsertSession(ctx, UpsertOAuthSessionParams{
		ProviderID:   cred.ProviderID,
		CredentialID: cred.ID,
		AccountEmail: "project@contoh.test",
		Scopes:       []string{"https://www.googleapis.com/auth/cloud-platform"},
		RefreshToken: security.Secret("rt-oauth-project-xyzzy"),
	})
	if err != nil {
		t.Fatalf("UpsertSession: %v", err)
	}
	if meta.CodeAssistProject != nil {
		t.Fatalf("sesi baru sudah punya project %q, ingin nil (belum terselesaikan)", *meta.CodeAssistProject)
	}

	// Pencarian dari kredensial: sesi yang sama, project masih kosong.
	dariKredensial, err := oauthRepo.GetSessionByCredentialID(ctx, cred.ID)
	if err != nil {
		t.Fatalf("GetSessionByCredentialID: %v", err)
	}
	if dariKredensial.ID != meta.ID {
		t.Errorf("sesi dari kredensial = %q, ingin %q", dariKredensial.ID, meta.ID)
	}
	if dariKredensial.CodeAssistProject != nil {
		t.Errorf("project dari kredensial = %q, ingin nil", *dariKredensial.CodeAssistProject)
	}

	// Kredensial acak yang sah bentuknya tetapi tanpa sesi → ErrNotFound.
	if _, err := oauthRepo.GetSessionByCredentialID(ctx, "00000000-0000-0000-0000-000000000000"); !errors.Is(err, repo.ErrNotFound) {
		t.Errorf("kredensial tanpa sesi: err = %v, ingin ErrNotFound", err)
	}

	// Project kosong ditolak — "belum terselesaikan" adalah NULL, bukan "".
	if err := oauthRepo.UpdateCodeAssistProject(ctx, meta.ID, ""); err == nil {
		t.Error("UpdateCodeAssistProject dengan project kosong harus ditolak")
	}

	if err := oauthRepo.UpdateCodeAssistProject(ctx, meta.ID, "proj-b157-roundtrip"); err != nil {
		t.Fatalf("UpdateCodeAssistProject: %v", err)
	}

	sesudah, err := oauthRepo.GetSession(ctx, meta.ID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if sesudah.CodeAssistProject == nil || *sesudah.CodeAssistProject != "proj-b157-roundtrip" {
		t.Fatalf("project tersimpan = %v, ingin proj-b157-roundtrip", sesudah.CodeAssistProject)
	}

	aktif, err := oauthRepo.GetActiveSession(ctx, meta.ID)
	if err != nil {
		t.Fatalf("GetActiveSession: %v", err)
	}
	if aktif.CodeAssistProject != "proj-b157-roundtrip" {
		t.Errorf("sesi aktif project = %q, ingin proj-b157-roundtrip", aktif.CodeAssistProject)
	}

	// Sesi yang tidak ada → ErrNotFound.
	if err := oauthRepo.UpdateCodeAssistProject(ctx, "00000000-0000-0000-0000-000000000000", "proj-x"); !errors.Is(err, repo.ErrNotFound) {
		t.Errorf("update sesi hantu: err = %v, ingin ErrNotFound", err)
	}
}
