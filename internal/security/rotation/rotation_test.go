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
	"github.com/NexGen-X/Route-X/internal/security"
	"github.com/NexGen-X/Route-X/internal/security/rotation"
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

	// 1. Siapkan data baris uji pada ketiga tabel
	// a. provider_credentials
	var providerID string
	err = pool.QueryRow(ctx, `
		INSERT INTO providers (name, display_name, kind, base_url)
		VALUES ('test-prov-rot', 'Test Provider Rotasi', 'openai', 'https://api.openai.com/v1')
		RETURNING id::text`).Scan(&providerID)
	if err != nil {
		t.Fatalf("membuat provider uji: %v", err)
	}

	const plainCred = "sk-kredensial-uji-rotasi-1234567890"
	var credID string
	err = pool.QueryRow(ctx, `SELECT gen_random_uuid()::text`).Scan(&credID)
	if err != nil {
		t.Fatalf("gen_random_uuid: %v", err)
	}

	credCiphertext, err := cipherLama.Encrypt([]byte(plainCred), security.CredentialAAD(credID))
	if err != nil {
		t.Fatalf("enkripsi cred uji: %v", err)
	}

	_, err = pool.Exec(ctx, `
		INSERT INTO provider_credentials (id, provider_id, label, ciphertext, encryption_key_id, masked_hint)
		VALUES ($1::uuid, $2::uuid, 'primary', $3, $4, 'sk-****7890')`,
		credID, providerID, credCiphertext, cipherLama.KeyID())
	if err != nil {
		t.Fatalf("insert provider_credentials uji: %v", err)
	}

	// b. webhooks
	const plainWebhookSecret = "whsec_rahasia_webhook_untuk_dirotasi_998877"
	var webhookID string
	err = pool.QueryRow(ctx, `SELECT gen_random_uuid()::text`).Scan(&webhookID)
	if err != nil {
		t.Fatalf("gen_random_uuid webhook: %v", err)
	}

	whCiphertext, err := cipherLama.Encrypt([]byte(plainWebhookSecret), security.WebhookAAD(webhookID))
	if err != nil {
		t.Fatalf("enkripsi webhook uji: %v", err)
	}

	_, err = pool.Exec(ctx, `
		INSERT INTO webhooks (id, name, url, events, secret_ciphertext, encryption_key_id, masked_hint)
		VALUES ($1::uuid, 'test-hook', 'https://example.com/webhook', array['*'], $2, $3, 'whsec_****8877')`,
		webhookID, whCiphertext, cipherLama.KeyID())
	if err != nil {
		t.Fatalf("insert webhooks uji: %v", err)
	}

	// c. egress_pool
	const plainProxyURL = "http://user123:secretpass@proxy.internal:3128"
	var egressID string
	err = pool.QueryRow(ctx, `SELECT gen_random_uuid()::text`).Scan(&egressID)
	if err != nil {
		t.Fatalf("gen_random_uuid egress: %v", err)
	}

	egressCiphertext, err := cipherLama.Encrypt([]byte(plainProxyURL), security.EgressAAD(egressID))
	if err != nil {
		t.Fatalf("enkripsi egress uji: %v", err)
	}

	_, err = pool.Exec(ctx, `
		INSERT INTO egress_pool (id, name, kind, proxy_url_encrypted, encryption_key_id, masked_hint)
		VALUES ($1::uuid, 'default-proxy', 'http', $2, $3, 'http://user123:****@proxy.internal:3128')`,
		egressID, egressCiphertext, cipherLama.KeyID())
	if err != nil {
		t.Fatalf("insert egress_pool uji: %v", err)
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

	// Verifikasi bahwa basis data TIDAK berubah selama dry run
	var dryKeyID string
	err = pool.QueryRow(ctx, `SELECT encryption_key_id FROM provider_credentials WHERE id = $1::uuid`, credID).Scan(&dryKeyID)
	if err != nil {
		t.Fatalf("cek encryption_key_id pasca dry-run: %v", err)
	}
	if dryKeyID != cipherLama.KeyID() {
		t.Errorf("dry-run memodifikasi baris! key_id = %s, ingin tetap %s", dryKeyID, cipherLama.KeyID())
	}

	// 3. Jalankan Rotasi Nyata
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

	// 4. Verifikasi bahwa data berhasil dirotasi dan dekripsi kunci baru menghasilkan plaintext awal
	// a. provider_credentials
	var nextCredCipher, nextCredKeyID string
	err = pool.QueryRow(ctx, `SELECT ciphertext, encryption_key_id FROM provider_credentials WHERE id = $1::uuid`, credID).
		Scan(&nextCredCipher, &nextCredKeyID)
	if err != nil {
		t.Fatalf("membaca cred pasca rotasi: %v", err)
	}
	if nextCredKeyID != cipherBaru.KeyID() {
		t.Errorf("cred key_id = %s, ingin %s", nextCredKeyID, cipherBaru.KeyID())
	}
	decryptedCred, err := cipherBaru.Decrypt(nextCredCipher, security.CredentialAAD(credID))
	if err != nil {
		t.Fatalf("cipherBaru.Decrypt cred: %v", err)
	}
	if string(decryptedCred) != plainCred {
		t.Errorf("decrypted cred = %q, ingin %q", string(decryptedCred), plainCred)
	}
	// Kunci lama wajib gagal mendekripsi
	if _, err := cipherLama.Decrypt(nextCredCipher, security.CredentialAAD(credID)); !errors.Is(err, security.ErrKeyMismatch) {
		t.Errorf("cipherLama.Decrypt ingin ErrKeyMismatch, dapat %v", err)
	}

	// b. webhooks
	var nextWhCipher, nextWhKeyID string
	err = pool.QueryRow(ctx, `SELECT secret_ciphertext, encryption_key_id FROM webhooks WHERE id = $1::uuid`, webhookID).
		Scan(&nextWhCipher, &nextWhKeyID)
	if err != nil {
		t.Fatalf("membaca webhook pasca rotasi: %v", err)
	}
	if nextWhKeyID != cipherBaru.KeyID() {
		t.Errorf("webhook key_id = %s, ingin %s", nextWhKeyID, cipherBaru.KeyID())
	}
	decryptedWh, err := cipherBaru.Decrypt(nextWhCipher, security.WebhookAAD(webhookID))
	if err != nil {
		t.Fatalf("cipherBaru.Decrypt webhook: %v", err)
	}
	if string(decryptedWh) != plainWebhookSecret {
		t.Errorf("decrypted webhook = %q, ingin %q", string(decryptedWh), plainWebhookSecret)
	}

	// c. egress_pool
	var nextEgressCipher, nextEgressKeyID string
	err = pool.QueryRow(ctx, `SELECT proxy_url_encrypted, encryption_key_id FROM egress_pool WHERE id = $1::uuid`, egressID).
		Scan(&nextEgressCipher, &nextEgressKeyID)
	if err != nil {
		t.Fatalf("membaca egress pasca rotasi: %v", err)
	}
	if nextEgressKeyID != cipherBaru.KeyID() {
		t.Errorf("egress key_id = %s, ingin %s", nextEgressKeyID, cipherBaru.KeyID())
	}
	decryptedEgress, err := cipherBaru.Decrypt(nextEgressCipher, security.EgressAAD(egressID))
	if err != nil {
		t.Fatalf("cipherBaru.Decrypt egress: %v", err)
	}
	if string(decryptedEgress) != plainProxyURL {
		t.Errorf("decrypted egress = %q, ingin %q", string(decryptedEgress), plainProxyURL)
	}

	// 5. Uji Idempotensi: Jalankan rotasi kedua kali dengan parameter yang sama
	secondRunRes, err := realRotator.Execute(ctx)
	if err != nil {
		t.Fatalf("realRotator.Execute pemanggilan kedua gagal: %v", err)
	}
	if secondRunRes.TotalRotated != 0 {
		t.Errorf("pemanggilan kedua seharusnya memproses 0 baris, tetapi memproses %d baris", secondRunRes.TotalRotated)
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
