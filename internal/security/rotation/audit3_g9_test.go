package rotation_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
	"github.com/NexGen-X/Route-X/internal/security"
	"github.com/NexGen-X/Route-X/internal/security/rotation"
)

// TestRotasiDibatalkanOlehKeyIDAsing membuktikan audit3 V-LN-29 (B-044):
// satu baris dengan encryption_key_id milik kunci KETIGA membatalkan seluruh
// rotasi dengan error yang jelas, alih-alih rotasi melaporkan sukses sambil
// meninggalkan baris itu tak terdekripsi oleh kunci mana pun. Skenario
// nyatanya persis ini: rotasi silang dua arah, atau sebagian baris ditulis
// konfigurasi kunci lain.
func TestRotasiDibatalkanOlehKeyIDAsing(t *testing.T) {
	if testing.Short() {
		t.Skip("memerlukan koneksi PostgreSQL nyata")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	pool := newTestPool(ctx, t)

	kunciLama := bytes.Repeat([]byte{0x61}, 32)
	kunciBaru := bytes.Repeat([]byte{0x62}, 32)
	kunciAsing := bytes.Repeat([]byte{0x63}, 32)
	cipherLama, err := security.NewCipher(kunciLama)
	if err != nil {
		t.Fatalf("NewCipher(lama): %v", err)
	}
	cipherAsing, err := security.NewCipher(kunciAsing)
	if err != nil {
		t.Fatalf("NewCipher(asing): %v", err)
	}

	provRepo := upstream.NewProviderRepo(pool)
	prov, err := provRepo.Create(ctx, upstream.CreateProviderParams{
		Name:        "test-rot-asing",
		DisplayName: "Test Rotasi Key Asing",
		Kind:        "openai",
		BaseURL:     "https://api.openai.com/v1",
	})
	if err != nil {
		t.Fatalf("provRepo.Create: %v", err)
	}

	// Satu kredensial berkunci lama (sasaran rotasi yang sah)…
	credLama, err := upstream.NewCredentialRepo(pool, cipherLama)
	if err != nil {
		t.Fatalf("NewCredentialRepo(lama): %v", err)
	}
	metaLama, err := credLama.Create(ctx, upstream.CreateCredentialParams{
		ProviderID: prov.ID, Label: "lama", Secret: security.Secret("uji-lama-untuk-rotasi-0000000001"),
	})
	if err != nil {
		t.Fatalf("Create(lama): %v", err)
	}
	// …dan satu berkunci asing yang tidak dikenal pasangan rotasi ini.
	credAsing, err := upstream.NewCredentialRepo(pool, cipherAsing)
	if err != nil {
		t.Fatalf("NewCredentialRepo(asing): %v", err)
	}
	asing, err := credAsing.Create(ctx, upstream.CreateCredentialParams{
		ProviderID: prov.ID, Label: "asing", Secret: security.Secret("uji-asing-untuk-rotasi-0000000002"),
	})
	if err != nil {
		t.Fatalf("Create(asing): %v", err)
	}

	rotator, err := rotation.NewRotator(rotation.Config{
		DB:     pool,
		OldKey: base64.StdEncoding.EncodeToString(kunciLama),
		NewKey: base64.StdEncoding.EncodeToString(kunciBaru),
		DryRun: false,
	})
	if err != nil {
		t.Fatalf("NewRotator: %v", err)
	}
	_, err = rotator.Execute(ctx)
	if err == nil {
		t.Fatal("Execute harus gagal: ada baris berkunci asing")
	}
	if !strings.Contains(err.Error(), "dibatalkan") || !strings.Contains(err.Error(), "provider_credentials") {
		t.Errorf("error = %v, mau menyebut tabel dan pembatalan", err)
	}

	// Transaksi utuh dibatalkan: baris kunci lama MASIH terbaca kunci lama
	// (tidak ada rotasi parsial), dan baris asing tidak tersentuh sama sekali.
	sisa, err := credLama.Reveal(ctx, metaLama.ID)
	if err != nil {
		t.Fatalf("Reveal(lama) sesudah rotasi batal: %v — rotasi parsial terjadi?", err)
	}
	if sisa.Reveal() != "uji-lama-untuk-rotasi-0000000001" {
		t.Errorf("kredensial lama berubah: %q", sisa.Reveal())
	}
	if _, err := credLama.Reveal(ctx, asing.ID); err == nil {
		t.Error("kredensial asing terbaca oleh cipher lama — tidak seharusnya")
	}
	terbaca, err := credAsing.Reveal(ctx, asing.ID)
	if err != nil {
		t.Fatalf("Reveal(asing) sesudah rotasi batal: %v", err)
	}
	if terbaca.Reveal() != "uji-asing-untuk-rotasi-0000000002" {
		t.Errorf("kredensial asing berubah: %q", terbaca.Reveal())
	}
}

// TestRotasiMelaporkanSudahBaru: rotasi kedua atas data yang sudah memakai
// kunci baru melaporkan already_current, bukan scanned=0 yang ambigu.
func TestRotasiMelaporkanSudahBaru(t *testing.T) {
	if testing.Short() {
		t.Skip("memerlukan koneksi PostgreSQL nyata")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	pool := newTestPool(ctx, t)

	kunciLama := bytes.Repeat([]byte{0x71}, 32)
	kunciBaru := bytes.Repeat([]byte{0x72}, 32)
	cipherBaru, err := security.NewCipher(kunciBaru)
	if err != nil {
		t.Fatalf("NewCipher(baru): %v", err)
	}

	provRepo := upstream.NewProviderRepo(pool)
	prov, err := provRepo.Create(ctx, upstream.CreateProviderParams{
		Name:        "test-rot-baru",
		DisplayName: "Test Rotasi Sudah Baru",
		Kind:        "openai",
		BaseURL:     "https://api.openai.com/v1",
	})
	if err != nil {
		t.Fatalf("provRepo.Create: %v", err)
	}
	credRepo, err := upstream.NewCredentialRepo(pool, cipherBaru)
	if err != nil {
		t.Fatalf("NewCredentialRepo(baru): %v", err)
	}
	if _, err := credRepo.Create(ctx, upstream.CreateCredentialParams{
		ProviderID: prov.ID, Label: "baru", Secret: security.Secret("uji-sudah-baru-00000000000003"),
	}); err != nil {
		t.Fatalf("Create(baru): %v", err)
	}

	rotator, err := rotation.NewRotator(rotation.Config{
		DB:     pool,
		OldKey: base64.StdEncoding.EncodeToString(kunciLama),
		NewKey: base64.StdEncoding.EncodeToString(kunciBaru),
		DryRun: true,
	})
	if err != nil {
		t.Fatalf("NewRotator: %v", err)
	}
	res, err := rotator.Execute(ctx)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	for _, tr := range res.Tables {
		if tr.TableName == "provider_credentials" {
			if tr.SudahBaru != 1 {
				t.Errorf("SudahBaru = %d, mau 1", tr.SudahBaru)
			}
			if tr.Scanned != 0 {
				t.Errorf("Scanned = %d, mau 0", tr.Scanned)
			}
		}
	}
}
