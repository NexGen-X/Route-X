package upstream

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/NexGen-X/Route-X/internal/database/repo"
	"github.com/NexGen-X/Route-X/internal/security"
)

// proxyTest memuat kredensial supaya kebocoran bisa dicari dengan pasti.
const testProxyURL = "http://keluar:sandi-proxy-xyzzy@proxy.contoh.test:3128"

// Jalur keluar menyimpan URL proxy terenkripsi dengan aturan yang sama seperti kredensial
// provider.
func TestEgressPoolIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("butuh PostgreSQL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	pool := newTestPool(ctx, t)
	cipher := testCipher(t, 0xD4)
	egress, err := NewEgressRepo(pool, cipher)
	if err != nil {
		t.Fatalf("NewEgressRepo: %v", err)
	}
	admin := seedUser(ctx, t, pool)

	created, err := egress.Create(ctx, CreateEgressParams{
		Name:      "keluar-singapura",
		Kind:      EgressHTTP,
		ProxyURL:  security.Secret(testProxyURL),
		Region:    ptr("ap-southeast-1"),
		Weight:    ptr(3),
		CreatedBy: &admin,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	switch {
	case created.MaskedHint != "http://keluar:****@proxy.contoh.test:3128":
		t.Errorf("masked_hint = %q", created.MaskedHint)
	case created.EncryptionKeyID != cipher.KeyID():
		t.Errorf("encryption_key_id = %q, ingin %q", created.EncryptionKeyID, cipher.KeyID())
	case created.Weight != 3 || !created.Enabled:
		t.Errorf("jalur keluar = %+v", created)
	}

	t.Run("URL proxy tidak ada sebagai teks biasa di database", func(t *testing.T) {
		for _, row := range rowsAsText(ctx, t, pool, "egress_pool") {
			if strings.Contains(row, "sandi-proxy-xyzzy") {
				t.Fatal("baris egress_pool memuat sandi proxy")
			}
		}
	})

	t.Run("URL proxy bisa dibuka kembali", func(t *testing.T) {
		got, err := egress.ProxyURL(ctx, created.ID)
		if err != nil {
			t.Fatalf("ProxyURL: %v", err)
		}
		if got.Reveal() != testProxyURL {
			t.Error("URL proxy hasil dekripsi tidak sama dengan yang disimpan")
		}
	})

	t.Run("ciphertext terikat ke barisnya", func(t *testing.T) {
		second, err := egress.Create(ctx, CreateEgressParams{
			Name: "keluar-jakarta", Kind: EgressSOCKS5,
			ProxyURL: security.Secret("socks5://lain:lain@jakarta.contoh.test:1080"),
		})
		if err != nil {
			t.Fatalf("Create kedua: %v", err)
		}
		if _, err := pool.Exec(ctx, `
			update egress_pool
			set proxy_url_encrypted = (select proxy_url_encrypted from egress_pool where id = $1)
			where id = $2`, created.ID, second.ID); err != nil {
			t.Fatalf("memindahkan ciphertext: %v", err)
		}
		if _, err := egress.ProxyURL(ctx, second.ID); !errors.Is(err, security.ErrDecryptionFailed) {
			t.Errorf("ProxyURL setelah ciphertext dipindahkan = %v, ingin ErrDecryptionFailed", err)
		}

		if err := egress.Delete(ctx, second.ID); err != nil {
			t.Fatalf("Delete: %v", err)
		}
	})

	t.Run("ganti URL proxy", func(t *testing.T) {
		replacement := security.Secret("https://keluar2:sandi2@proxy2.contoh.test:8443")
		updated, err := egress.SetProxyURL(ctx, created.ID, replacement)
		if err != nil {
			t.Fatalf("SetProxyURL: %v", err)
		}
		if updated.MaskedHint != "https://keluar2:****@proxy2.contoh.test:8443" {
			t.Errorf("masked_hint = %q", updated.MaskedHint)
		}
		got, err := egress.ProxyURL(ctx, created.ID)
		if err != nil {
			t.Fatalf("ProxyURL: %v", err)
		}
		if got.Reveal() != replacement.Reveal() {
			t.Error("URL proxy tidak berganti")
		}
	})

	t.Run("update sebagian dan daftar", func(t *testing.T) {
		updated, err := egress.Update(ctx, created.ID, UpdateEgressParams{
			Region:  Clear[string](),
			Enabled: ptr(false),
		})
		if err != nil {
			t.Fatalf("Update: %v", err)
		}
		if updated.Region != nil || updated.Enabled {
			t.Errorf("jalur keluar = %+v", updated)
		}

		all, err := egress.List(ctx, false)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		enabled, err := egress.List(ctx, true)
		if err != nil {
			t.Fatalf("List aktif: %v", err)
		}
		if len(all) != 1 || len(enabled) != 0 {
			t.Errorf("jumlah semua/aktif = %d/%d, ingin 1/0", len(all), len(enabled))
		}
	})

	t.Run("nama ganda menjadi ErrConflict dan jenis asing menjadi ErrConstraint", func(t *testing.T) {
		_, err := egress.Create(ctx, CreateEgressParams{
			Name: "keluar-singapura", Kind: EgressHTTP, ProxyURL: security.Secret("http://a:b@c:1"),
		})
		if !errors.Is(err, repo.ErrConflict) {
			t.Errorf("Create nama ganda = %v, ingin ErrConflict", err)
		}

		_, err = egress.Create(ctx, CreateEgressParams{
			Name: "keluar-aneh", Kind: "carrier-pigeon", ProxyURL: security.Secret("http://a:b@c:1"),
		})
		if !errors.Is(err, repo.ErrConstraint) {
			t.Errorf("Create jenis asing = %v, ingin ErrConstraint", err)
		}
	})

	t.Run("cuplikan kesehatan", func(t *testing.T) {
		if err := egress.RecordHealth(ctx, created.ID, HealthDegraded, ptr(850)); err != nil {
			t.Fatalf("RecordHealth: %v", err)
		}
		got, err := egress.Get(ctx, created.ID)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if got.LastHealthStatus == nil || *got.LastHealthStatus != HealthDegraded {
			t.Errorf("last_health_status = %v", got.LastHealthStatus)
		}
		if got.LastLatencyMS == nil || *got.LastLatencyMS != 850 || got.LastHealthAt == nil {
			t.Errorf("cuplikan kesehatan tidak lengkap: %+v", got)
		}
	})

	t.Run("provider kehilangan jalur keluarnya saat jalur itu dihapus", func(t *testing.T) {
		providers := NewProviderRepo(pool)
		provider, err := providers.Create(ctx, CreateProviderParams{
			Name: "provider-berproxy", DisplayName: "Berproxy", Kind: KindCustom,
			BaseURL: "https://berproxy.contoh.test", EgressPoolID: &created.ID,
		})
		if err != nil {
			t.Fatalf("Create provider: %v", err)
		}
		if provider.EgressPoolID == nil || *provider.EgressPoolID != created.ID {
			t.Fatalf("egress_pool_id = %v, ingin %s", provider.EgressPoolID, created.ID)
		}

		if err := egress.Delete(ctx, created.ID); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		after, err := providers.Get(ctx, provider.ID)
		if err != nil {
			t.Fatalf("Get provider: %v", err)
		}
		if after.EgressPoolID != nil {
			t.Errorf("egress_pool_id = %v, ingin NULL setelah jalur keluarnya dihapus", after.EgressPoolID)
		}
	})
}

// Rotasi kunci wajib mencakup URL proxy juga; kalau tidak, kunci lama harus disimpan
// selamanya.
func TestEgressKeyRotationIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("butuh PostgreSQL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	pool := newTestPool(ctx, t)
	oldCipher := testCipher(t, 0xD4)
	newCipher := testCipher(t, 0xE5)

	oldRepo, err := NewEgressRepo(pool, oldCipher)
	if err != nil {
		t.Fatalf("NewEgressRepo: %v", err)
	}
	newRepo, err := NewEgressRepo(pool, newCipher)
	if err != nil {
		t.Fatalf("NewEgressRepo: %v", err)
	}

	created, err := oldRepo.Create(ctx, CreateEgressParams{
		Name: "keluar-rotasi", Kind: EgressHTTP, ProxyURL: security.Secret(testProxyURL),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if _, err := newRepo.ProxyURL(ctx, created.ID); !errors.Is(err, security.ErrKeyMismatch) {
		t.Errorf("ProxyURL dengan kunci baru = %v, ingin ErrKeyMismatch", err)
	}

	rotated, err := newRepo.Reencrypt(ctx, oldCipher, 10)
	if err != nil {
		t.Fatalf("Reencrypt: %v", err)
	}
	if rotated != 1 {
		t.Fatalf("baris berpindah = %d, ingin 1", rotated)
	}

	got, err := newRepo.ProxyURL(ctx, created.ID)
	if err != nil {
		t.Fatalf("ProxyURL setelah rotasi: %v", err)
	}
	if got.Reveal() != testProxyURL {
		t.Error("URL proxy berubah setelah rotasi")
	}

	after, err := newRepo.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if after.EncryptionKeyID != newCipher.KeyID() {
		t.Errorf("encryption_key_id = %q, ingin %q", after.EncryptionKeyID, newCipher.KeyID())
	}

	if rotated, err := newRepo.Reencrypt(ctx, oldCipher, 10); err != nil || rotated != 0 {
		t.Errorf("Reencrypt kedua = %d, %v; ingin 0, nil", rotated, err)
	}
}

// Cipher wajib ada: tanpa itu tidak ada jalur yang boleh menyimpan rahasia.
func TestRepoRequiresCipher(t *testing.T) {
	if _, err := NewCredentialRepo(nil, nil); err == nil {
		t.Error("NewCredentialRepo tanpa cipher seharusnya gagal")
	}
	if _, err := NewEgressRepo(nil, nil); err == nil {
		t.Error("NewEgressRepo tanpa cipher seharusnya gagal")
	}
}
