package policy

import (
	"errors"
	"testing"
	"time"

	"github.com/NexGen-X/Route-X/internal/database/repo"
)

// TestCreateBanSesudahKedaluarsaB587 mengunci skenario dinamis persis entri
// B-587 lewat CreateBan sungguhan (bukan proksi):
//
//   - buat ban IP 203.0.113.77 dengan expires_at +3 dtk → berhasil;
//   - daftar bans = 1 item; duplikat selagi aktif ditolak ErrConflict;
//   - tidur 4 dtk → daftar bans = 0 item, BanFor = ErrNotFound;
//   - buat ban ulang subjek sama → HARUS berhasil (di base: ErrConflict
//     karena baris kedaluwarsa masih menempati indeks unik parsial
//     bans_active_subject_key);
//   - cabut ban baru lalu buat ulang → berhasil (slot bebas sesudah lift).
//
// Baris kedaluwarsa yang dicabut manual tetap bisa di-lift (jejaknya
// dipertahankan sampai PurgeExpired): dibuat dengan expires_at lampau
// (repo mengizinkannya; permukaan admin tidak) lalu Lift → berhasil.
func TestCreateBanSesudahKedaluarsaB587(t *testing.T) {
	ctx, r := newRepo(t)

	subjek := "203.0.113.77"
	kedaluwarsa := time.Now().Add(3 * time.Second)
	dibuat, err := r.CreateBan(ctx, CreateBanParams{
		SubjectKind: BanSubjectIP, Subject: subjek, Reason: "uji B-587", ExpiresAt: &kedaluwarsa,
	})
	if err != nil {
		t.Fatalf("CreateBan pertama: %v", err)
	}

	daftar, err := r.ActiveBans(ctx, repo.Page{Limit: 100})
	if err != nil {
		t.Fatalf("ActiveBans: %v", err)
	}
	if len(daftar) != 1 {
		t.Fatalf("ActiveBans = %d item, mau 1", len(daftar))
	}

	// Duplikat selagi ban masih aktif TETAP ditolak — penjaga keunikan
	// tidak boleh mati bersama perbaikannya.
	if _, err := r.CreateBan(ctx, CreateBanParams{
		SubjectKind: BanSubjectIP, Subject: subjek, Reason: "duplikat aktif",
	}); !errors.Is(err, repo.ErrConflict) {
		t.Fatalf("CreateBan duplikat aktif = %v, mau ErrConflict", err)
	}

	time.Sleep(4 * time.Second)

	daftar, err = r.ActiveBans(ctx, repo.Page{Limit: 100})
	if err != nil {
		t.Fatalf("ActiveBans sesudah kedaluwarsa: %v", err)
	}
	if len(daftar) != 0 {
		t.Fatalf("ActiveBans sesudah kedaluwarsa = %d item, mau 0", len(daftar))
	}
	if _, err := r.BanFor(ctx, BanSubjectIP, subjek); !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("BanFor sesudah kedaluwarsa = %v, mau ErrNotFound", err)
	}

	// Inti B-587: ban ulang subjek yang baris lamanya sudah kedaluwarsa.
	dibuatLagi, err := r.CreateBan(ctx, CreateBanParams{
		SubjectKind: BanSubjectIP, Subject: subjek, Reason: "blokir ulang B-587",
	})
	if err != nil {
		t.Fatalf("CreateBan ulang sesudah kedaluwarsa: %v", err)
	}
	if dibuatLagi.ID == dibuat.ID {
		t.Fatalf("ban ulang memakai ID lama %s — baris lama tidak diganti", dibuat.ID)
	}

	daftar, err = r.ActiveBans(ctx, repo.Page{Limit: 100})
	if err != nil {
		t.Fatalf("ActiveBans sesudah buat ulang: %v", err)
	}
	if len(daftar) != 1 {
		t.Fatalf("ActiveBans sesudah buat ulang = %d item, mau 1", len(daftar))
	}

	// Lift membebaskan slot seperti sebelumnya.
	if _, err := r.Lift(ctx, dibuatLagi.ID, ""); err != nil {
		t.Fatalf("Lift ban baru: %v", err)
	}
	if _, err := r.CreateBan(ctx, CreateBanParams{
		SubjectKind: BanSubjectIP, Subject: subjek, Reason: "blokir sesudah lift",
	}); err != nil {
		t.Fatalf("CreateBan sesudah lift: %v", err)
	}

	// Baris kedaluwarsa tetap bisa di-lift (bukan hilang diam-diam).
	lampau := time.Now().Add(-1 * time.Hour)
	mati, err := r.CreateBan(ctx, CreateBanParams{
		SubjectKind: BanSubjectIP, Subject: "203.0.113.78", Reason: "sudah lampau", ExpiresAt: &lampau,
	})
	if err != nil {
		t.Fatalf("CreateBan expires_at lampau (repo mengizinkan): %v", err)
	}
	if _, err := r.Lift(ctx, mati.ID, ""); err != nil {
		t.Fatalf("Lift baris kedaluwarsa: %v", err)
	}
}
