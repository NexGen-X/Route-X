package rotation_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/NexGen-X/Route-X/internal/database"
	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
	"github.com/NexGen-X/Route-X/internal/security"
	"github.com/NexGen-X/Route-X/internal/security/rotation"
	"github.com/NexGen-X/Route-X/internal/webhooks"
)

func TestParseKey(t *testing.T) {
	t.Run("panjang kunci tidak valid", func(t *testing.T) {
		pendek := base64.StdEncoding.EncodeToString([]byte("terlalu-pendek"))
		if _, err := rotation.ParseKey(pendek); !errors.Is(err, rotation.ErrInvalidKeyFormat) {
			t.Fatalf("ParseKey(%q) = %v, ingin ErrInvalidKeyFormat", pendek, err)
		}
	})

	t.Run("string bukan base64", func(t *testing.T) {
		if _, err := rotation.ParseKey("bukan-base64!!!"); !errors.Is(err, rotation.ErrInvalidKeyFormat) {
			t.Fatalf("ParseKey bukan base64 ingin ErrInvalidKeyFormat, dapat: %v", err)
		}
	})

	t.Run("berhasil decode 32 byte", func(t *testing.T) {
		kunciAsli := bytes.Repeat([]byte{0x42}, 32)
		b64 := base64.StdEncoding.EncodeToString(kunciAsli)
		hasil, err := rotation.ParseKey(b64)
		if err != nil {
			t.Fatalf("ParseKey gagal: %v", err)
		}
		if !bytes.Equal(hasil, kunciAsli) {
			t.Fatalf("hasil decode tidak cocok dengan kunci asli")
		}
	})
}

func TestRotatorValidation(t *testing.T) {
	kunciA := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x01}, 32))
	kunciB := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x02}, 32))

	t.Run("koneksi db nil", func(t *testing.T) {
		_, err := rotation.NewRotator(rotation.Config{
			DB:     nil,
			OldKey: kunciA,
			NewKey: kunciB,
		})
		if err == nil {
			t.Fatal("NewRotator dengan DB nil seharusnya mengembalikan error")
		}
	})

	t.Run("kunci lama dan baru bernilai sama", func(t *testing.T) {
		dummyPool := &pgxpool.Pool{}
		_, err := rotation.NewRotator(rotation.Config{
			DB:     dummyPool,
			OldKey: kunciA,
			NewKey: kunciA,
		})
		if !errors.Is(err, rotation.ErrSameKeys) {
			t.Fatalf("NewRotator dengan kunci sama = %v, ingin ErrSameKeys", err)
		}
	})
}

func TestRotationIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("memerlukan koneksi PostgreSQL nyata")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	pool := newTestPool(ctx, t)

	kunciLamaBytes := bytes.Repeat([]byte{0x33}, 32)
	kunciBaruBytes := bytes.Repeat([]byte{0x77}, 32)
	kunciLamaB64 := base64.StdEncoding.EncodeToString(kunciLamaBytes)
	kunciBaruB64 := base64.StdEncoding.EncodeToString(kunciBaruBytes)

	cipherLama, err := security.NewCipher(kunciLamaBytes)
	if err != nil {
		t.Fatalf("security.NewCipher(lama): %v", err)
	}
	cipherBaru, err := security.NewCipher(kunciBaruBytes)
	if err != nil {
		t.Fatalf("security.NewCipher(baru): %v", err)
	}

	// 1. Siapkan data baris uji melalui repository SUNGGUHAN aplikasi.
	// Ini menjamin baris ditulis persis seperti aplikasi nyata menulisnya (termasuk AAD dan format kolom).

	// a. provider_credentials lewat upstream.ProviderRepo dan upstream.CredentialRepo
	provRepo := upstream.NewProviderRepo(pool)
	prov, err := provRepo.Create(ctx, upstream.CreateProviderParams{
		Name:        "test-prov-rot",
		DisplayName: "Test Provider Rotasi",
		Kind:        "openai",
		BaseURL:     "https://api.openai.com/v1",
	})
	if err != nil {
		t.Fatalf("provRepo.Create: %v", err)
	}

	credRepoLama, err := upstream.NewCredentialRepo(pool, cipherLama)
	if err != nil {
		t.Fatalf("upstream.NewCredentialRepo(lama): %v", err)
	}

	const plainCred = "sk-kredensial-uji-rotasi-1234567890"
	credMeta, err := credRepoLama.Create(ctx, upstream.CreateCredentialParams{
		ProviderID: prov.ID,
		Label:      "primary",
		Secret:     security.Secret(plainCred),
	})
	if err != nil {
		t.Fatalf("credRepoLama.Create: %v", err)
	}

	// b. webhooks lewat webhooks.Repo
	whRepo := webhooks.NewRepo(pool)
	const plainWebhookSecret = "whsec_rahasia_webhook_untuk_dirotasi_998877"
	wh, err := whRepo.Create(ctx, webhooks.CreateWebhookParams{
		Name:      "test-hook",
		URL:       "https://example.com/webhook",
		Events:    []string{"*"},
		RawSecret: plainWebhookSecret,
		Cipher:    cipherLama,
	})
	if err != nil {
		t.Fatalf("whRepo.Create: %v", err)
	}

	// c. egress_pool lewat upstream.EgressRepo
	egressRepoLama, err := upstream.NewEgressRepo(pool, cipherLama)
	if err != nil {
		t.Fatalf("upstream.NewEgressRepo(lama): %v", err)
	}

	const plainProxyURL = "http://user123:secretpass@proxy.internal:3128"
	egress, err := egressRepoLama.Create(ctx, upstream.CreateEgressParams{
		Name:     "default-proxy",
		Kind:     "http",
		ProxyURL: security.Secret(plainProxyURL),
	})
	if err != nil {
		t.Fatalf("egressRepoLama.Create: %v", err)
	}

	// 2. Jalankan Mode Dry-Run (Simulasi)
	dryRotator, err := rotation.NewRotator(rotation.Config{
		DB:     pool,
		OldKey: kunciLamaB64,
		NewKey: kunciBaruB64,
		DryRun: true,
	})
	if err != nil {
		t.Fatalf("NewRotator dry-run: %v", err)
	}

	dryRes, err := dryRotator.Execute(ctx)
	if err != nil {
		t.Fatalf("dryRotator.Execute: %v", err)
	}
	if !dryRes.DryRun {
		t.Errorf("dryRes.DryRun = false, ingin true")
	}
	if dryRes.TotalRotated != 3 {
		t.Errorf("dryRes.TotalRotated = %d, ingin 3", dryRes.TotalRotated)
	}

	// Verifikasi bahwa basis data TIDAK berubah selama dry run: repository lama masih bisa membaca plaintext
	revealedBefore, err := credRepoLama.Reveal(ctx, credMeta.ID)
	if err != nil {
		t.Fatalf("credRepoLama.Reveal pasca dry-run: %v", err)
	}
	if revealedBefore.Reveal() != plainCred {
		t.Errorf("plaintext sebelum rotasi = %q, ingin %q", revealedBefore.Reveal(), plainCred)
	}

	// 3. Jalankan Rotasi Nyata (SATU Transaksi Tunggal untuk Ketiga Tabel)
	realRotator, err := rotation.NewRotator(rotation.Config{
		DB:     pool,
		OldKey: kunciLamaB64,
		NewKey: kunciBaruB64,
		DryRun: false,
	})
	if err != nil {
		t.Fatalf("NewRotator nyata: %v", err)
	}

	realRes, err := realRotator.Execute(ctx)
	if err != nil {
		t.Fatalf("realRotator.Execute: %v", err)
	}
	if realRes.TotalRotated != 3 {
		t.Errorf("realRes.TotalRotated = %d, ingin 3", realRes.TotalRotated)
	}

	// 4. Verifikasi bahwa data berhasil dibaca kembali oleh repository SUNGGUHAN yang memakai cipherBaru

	// a. provider_credentials dibaca lewat upstream.CredentialRepo(cipherBaru).Reveal
	credRepoBaru, err := upstream.NewCredentialRepo(pool, cipherBaru)
	if err != nil {
		t.Fatalf("upstream.NewCredentialRepo(baru): %v", err)
	}

	revealedCred, err := credRepoBaru.Reveal(ctx, credMeta.ID)
	if err != nil {
		t.Fatalf("credRepoBaru.Reveal gagal mendekripsi pasca rotasi: %v", err)
	}
	if revealedCred.Reveal() != plainCred {
		t.Errorf("revealedCred = %q, ingin %q", revealedCred.Reveal(), plainCred)
	}
	// Kredensial aktif juga harus bisa diambil oleh executor gateway
	activeCred, err := credRepoBaru.Active(ctx, prov.ID)
	if err != nil {
		t.Fatalf("credRepoBaru.Active gagal: %v", err)
	}
	if activeCred.Secret.Reveal() != plainCred {
		t.Errorf("activeCred secret = %q, ingin %q", activeCred.Secret.Reveal(), plainCred)
	}
	// Repository kunci lama wajib gagal membaca
	if _, err := credRepoLama.Reveal(ctx, credMeta.ID); err == nil {
		t.Errorf("credRepoLama.Reveal seharusnya gagal dengan ErrKeyMismatch pasca rotasi")
	}

	// b. egress_pool dibaca lewat upstream.EgressRepo(cipherBaru).ProxyURL
	egressRepoBaru, err := upstream.NewEgressRepo(pool, cipherBaru)
	if err != nil {
		t.Fatalf("upstream.NewEgressRepo(baru): %v", err)
	}

	revealedProxy, err := egressRepoBaru.ProxyURL(ctx, egress.ID)
	if err != nil {
		t.Fatalf("egressRepoBaru.ProxyURL gagal mendekripsi pasca rotasi: %v", err)
	}
	if revealedProxy.Reveal() != plainProxyURL {
		t.Errorf("revealedProxy = %q, ingin %q", revealedProxy.Reveal(), plainProxyURL)
	}
	// Repository kunci lama wajib gagal membaca proxy URL
	if _, err := egressRepoLama.ProxyURL(ctx, egress.ID); err == nil {
		t.Errorf("egressRepoLama.ProxyURL seharusnya gagal dengan ErrKeyMismatch pasca rotasi")
	}

	// c. webhooks dibaca lewat webhooks.Repo dan didekripsi dengan cipherBaru
	whUpdated, err := whRepo.Get(ctx, wh.ID)
	if err != nil {
		t.Fatalf("whRepo.Get pasca rotasi: %v", err)
	}
	if whUpdated.EncryptionKeyID != cipherBaru.KeyID() {
		t.Errorf("webhook key_id = %s, ingin %s", whUpdated.EncryptionKeyID, cipherBaru.KeyID())
	}
	decryptedSecret, err := cipherBaru.Decrypt(whUpdated.SecretCiphertext, security.WebhookAAD(wh.ID))
	if err != nil {
		t.Fatalf("cipherBaru.Decrypt webhook gagal pasca rotasi: %v", err)
	}
	if string(decryptedSecret) != plainWebhookSecret {
		t.Errorf("decrypted webhook = %q, ingin %q", string(decryptedSecret), plainWebhookSecret)
	}
	// Kunci lama wajib gagal mendekripsi secret webhook
	if _, err := cipherLama.Decrypt(whUpdated.SecretCiphertext, security.WebhookAAD(wh.ID)); err == nil {
		t.Errorf("cipherLama.Decrypt webhook seharusnya gagal pasca rotasi")
	}

	// 5. Uji Idempotensi: Jalankan rotasi kedua kali dengan parameter yang sama
	secondRunRes, err := realRotator.Execute(ctx)
	if err != nil {
		t.Fatalf("realRotator.Execute pemanggilan kedua gagal: %v", err)
	}
	if secondRunRes.TotalRotated != 0 {
		t.Errorf("pemanggilan kedua seharusnya memproses 0 baris, tetapi memproses %d baris", secondRunRes.TotalRotated)
	}

	// Pastikan data tetap dapat dibaca tanpa cacat setelah eksekusi kedua
	revealedProxySecond, err := egressRepoBaru.ProxyURL(ctx, egress.ID)
	if err != nil || revealedProxySecond.Reveal() != plainProxyURL {
		t.Fatalf("ProxyURL pasca eksekusi rotasi kedua rusak: %v", err)
	}
}

// -----------------------------------------------------------------------------
// Helper isolasi schema basis data pengujian
// -----------------------------------------------------------------------------

const schemaPrefix = "test_rot_"

func testDSN(t *testing.T) string {
	t.Helper()
	for _, key := range []string{"TEST_DATABASE_URL", "DATABASE_URL"} {
		dsn := strings.TrimSpace(os.Getenv(key))
		if dsn == "" {
			continue
		}
		if !strings.HasPrefix(dsn, "postgres://") && !strings.HasPrefix(dsn, "postgresql://") {
			t.Skipf("%s bukan URL postgres:// yang sah", key)
		}
		return dsn
	}
	t.Skip("TEST_DATABASE_URL maupun DATABASE_URL tidak diset")
	return ""
}

func newTestPool(ctx context.Context, t *testing.T) *pgxpool.Pool {
	t.Helper()
	baseDSN := testDSN(t)

	admin, err := pgxpool.New(ctx, baseDSN)
	if err != nil {
		t.Fatalf("membuka koneksi admin basis data: %v", err)
	}
	t.Cleanup(admin.Close)

	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		t.Fatalf("membuat nama schema acak: %v", err)
	}
	schema := schemaPrefix + hex.EncodeToString(buf)
	ident := pgx.Identifier{schema}.Sanitize()

	if _, err := admin.Exec(ctx, "create schema "+ident); err != nil {
		t.Fatalf("membuat schema %s: %v", schema, err)
	}
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if _, err := admin.Exec(cleanupCtx, "drop schema "+ident+" cascade"); err != nil {
			t.Errorf("membersihkan schema %s: %v", schema, err)
		}
	})

	u, err := url.Parse(baseDSN)
	if err != nil {
		t.Fatalf("DSN test tidak dapat diurai: %v", err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()

	pool, err := pgxpool.New(ctx, u.String())
	if err != nil {
		t.Fatalf("membuka pool test: %v", err)
	}
	t.Cleanup(pool.Close)

	discardLogger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if _, err := database.Migrate(ctx, &database.DB{Pool: pool}, discardLogger); err != nil {
		t.Fatalf("migrasi schema pengujian %s gagal: %v", schema, err)
	}
	return pool
}
