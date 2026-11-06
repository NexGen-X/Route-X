package webhooks

import (
	"context"
	"errors"
	"testing"

	"github.com/NexGen-X/Route-X/internal/security"
)

// TestB354UpdateWebhookTanpaPerubahanMenolak adalah pengunci
// B-354 sisi webhooks: Update dengan params kosong dulu menjawab
// baris yang sudah ada (200 idempoten), sementara repo lain
// menjawab ErrNoChanges → 400 no_changes (B-306). Kini kontraknya
// seragam: params kosong ditolak dengan ErrNoChanges paket ini.
func TestB354UpdateWebhookTanpaPerubahanMenolak(t *testing.T) {
	pool, cipher := testPool(t)
	ctx := context.Background()
	r := NewRepo(pool)

	wh, err := r.Create(ctx, CreateWebhookParams{
		Name:      "webhook-b354",
		URL:       "https://contoh.test/hook",
		Events:    []string{"request.completed"},
		RawSecret: "GANTI_rahasia" + "_b354",
		Cipher:    cipher,
		Enabled:   true,
		SSRFPolicy: security.SSRFPolicy{
			AllowHTTP: true, AllowPrivate: true,
		},
	})
	if err != nil {
		t.Fatalf("Create webhook: %v", err)
	}
	if _, err := r.Update(ctx, wh.ID, UpdateWebhookParams{}); !errors.Is(err, ErrNoChanges) {
		t.Errorf("Update(params kosong) = %v, mau ErrNoChanges", err)
	}
	nama := "webhook-b354-baru"
	if _, err := r.Update(ctx, wh.ID, UpdateWebhookParams{Name: &nama}); err != nil {
		t.Errorf("Update(dengan nama) = %v, mau sukses (kontrol)", err)
	}
}
