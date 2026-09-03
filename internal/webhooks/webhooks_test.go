package webhooks

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/NexGen-X/Route-X/internal/database/repo"
	"github.com/NexGen-X/Route-X/internal/security"
)

func testUUID(t *testing.T) string {
	t.Helper()
	id, err := newID()
	if err != nil {
		t.Fatalf("membuat UUID pengujian: %v", err)
	}
	return id
}

func TestMaskSecret(t *testing.T) {
	if got := MaskSecret("whsec_12345678"); got != "whsec_****5678" {
		t.Errorf("MaskSecret salah: dapat %q, ingin whsec_****5678", got)
	}
	if got := MaskSecret("abc"); got != "whsec_****abc" {
		t.Errorf("MaskSecret pendek salah: dapat %q, ingin whsec_****abc", got)
	}
	if got := MaskSecret(""); got != "whsec_****" {
		t.Errorf("MaskSecret kosong salah: dapat %q, ingin whsec_****", got)
	}
}

func TestSanitizeErrorMessage(t *testing.T) {
	raw := "panggilan ke http://endpoint.com/hook?token=sangat_rahasia&user=1 gagal koneksi"
	clean := SanitizeErrorMessage(raw)
	if strings.Contains(clean, "sangat_rahasia") {
		t.Errorf("SanitizeErrorMessage membocorkan token: %s", clean)
	}
	if !strings.Contains(clean, "endpoint.com/hook") {
		t.Errorf("SanitizeErrorMessage membuang host URL yang sah: %s", clean)
	}
}

func TestCalculateBackoffEqualJitter(t *testing.T) {
	base := 2 * time.Second
	maxB := 30 * time.Second

	// Percobaan 1: base = 2s, separuh dijamin = 1s, separuh acak = 0..1s => range [1s, 2s]
	d1 := CalculateBackoff(1, base, maxB, func(n int64) int64 { return 0 })
	if d1 != 1*time.Second {
		d1Max := CalculateBackoff(1, base, maxB, func(n int64) int64 { return n })
		if d1Max < 1*time.Second {
			t.Errorf("CalculateBackoff attempt 1 di bawah batas dijamin: %v", d1)
		}
	}

	// Percobaan besar tidak boleh melebihi maxBackoff
	dBesar := CalculateBackoff(25, base, maxB, func(n int64) int64 { return n })
	if dBesar > maxB {
		t.Errorf("CalculateBackoff melebihi maxBackoff: %v > %v", dBesar, maxB)
	}

	// Percobaan 0 atau base 0 mengembalikan 0
	if CalculateBackoff(0, base, maxB, nil) != 0 {
		t.Errorf("CalculateBackoff attempt 0 harus 0")
	}
}

func TestVerifySignature(t *testing.T) {
	secret := []byte("kunci-rahasia-hmac-1234")
	payload := []byte(`{"event":"provider.unhealthy","status":"down"}`)

	// Hitung tanda tangan yang benar
	h := hmac.New(sha256.New, secret)
	h.Write(payload)
	mac := hex.EncodeToString(h.Sum(nil))
	sigValid := "sha256=" + mac

	if !VerifySignature(payload, secret, sigValid) {
		t.Error("VerifySignature gagal memvalidasi tanda tangan sah")
	}

	// Tanda tangan tanpa prefiks sha256= tetap diterima
	if !VerifySignature(payload, secret, mac) {
		t.Error("VerifySignature gagal memvalidasi tanda tangan tanpa prefiks sha256=")
	}

	// Payload diubah sedikit (tampered) wajib ditolak
	payloadPalsu := []byte(`{"event":"provider.unhealthy","status":"up"}`)
	if VerifySignature(payloadPalsu, secret, sigValid) {
		t.Error("VerifySignature meloloskan payload palsu")
	}

	// Secret salah wajib ditolak
	if VerifySignature(payload, []byte("kunci-salah"), sigValid) {
		t.Error("VerifySignature meloloskan kunci salah")
	}
}

// --- Integrasi PostgreSQL ---------------------------------------------------

func testPool(t *testing.T) (*pgxpool.Pool, *security.Cipher) {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL tidak disetel, melewati pengujian integrasi database")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("koneksi database gagal: %v", err)
	}
	t.Cleanup(pool.Close)

	key := make([]byte, 32)
	_, _ = rand.Read(key)
	cipher, err := security.NewCipher(key)
	if err != nil {
		t.Fatalf("NewCipher: %v", err)
	}

	return pool, cipher
}

func TestIntegrationWebhooksSiklusHidupDanPengiriman(t *testing.T) {
	pool, cipher := testPool(t)
	ctx := context.Background()
	r := NewRepo(pool)

	rawSecret := "whsec_supersecret1234567890"
	testID := testUUID(t)

	var receivedPayload []byte
	var receivedSig string
	var callCount atomic.Int32

	// Server penerima webhook tiruan
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		count := callCount.Add(1)
		body, _ := io.ReadAll(req.Body)
		receivedPayload = body
		receivedSig = req.Header.Get("X-RouteX-Signature")

		if count == 1 {
			// Percobaan pertama sengaja gagal 500 untuk menguji backoff dan retry
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"internal_server_error"}`))
			return
		}
		// Percobaan kedua sukses 200
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer srv.Close()

	// 1. Buat Webhook baru
	wh, err := r.Create(ctx, CreateWebhookParams{
		ID:         testID,
		Name:       "webhook-uji-" + testID[:8],
		URL:        srv.URL,
		Events:     []string{"provider.unhealthy", "circuit.opened"},
		RawSecret:  rawSecret,
		Cipher:     cipher,
		MaxRetries: 3,
		TimeoutMS:  5000,
	})
	if err != nil {
		t.Fatalf("Create webhook gagal: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, "delete from webhooks where id = $1::uuid", wh.ID)
	})

	if wh.MaskedHint != "whsec_****7890" {
		t.Errorf("MaskedHint = %q, ingin whsec_****7890", wh.MaskedHint)
	}

	// 2. Enqueue event yang cocok
	payloadAsli := map[string]any{
		"event":         "provider.unhealthy",
		"provider_name": "openai-prod",
		"status":        "unhealthy",
	}
	n, err := r.Enqueue(ctx, "provider.unhealthy", payloadAsli)
	if err != nil {
		t.Fatalf("Enqueue gagal: %v", err)
	}
	if n < 1 {
		t.Fatalf("Enqueue memasukkan %d baris, mau minimal 1", n)
	}

	// Enqueue event yang TIDAK dilanggani (mis. budget.exceeded) tidak boleh masuk ke webhook ini
	nUnsubscribed, err := r.Enqueue(ctx, "budget.exceeded", map[string]any{"event": "budget.exceeded"})
	if err != nil {
		t.Fatalf("Enqueue budget.exceeded gagal: %v", err)
	}
	_ = nUnsubscribed

	// 3. Ambil antrean due dengan ClaimDue (FOR UPDATE SKIP LOCKED)
	deliveries, err := r.ClaimDue(ctx, "worker-test-1", 10)
	if err != nil {
		t.Fatalf("ClaimDue gagal: %v", err)
	}
	var targetDelivery *Delivery
	for _, d := range deliveries {
		if d.WebhookID == wh.ID {
			targetDelivery = d
			break
		}
	}
	if targetDelivery == nil {
		t.Fatalf("antrean delivery untuk webhook %s tidak ditemukan", wh.ID)
	}

	if targetDelivery.Status != StatusDelivering {
		t.Errorf("status delivery saat di-claim = %s, mau delivering", targetDelivery.Status)
	}
	if targetDelivery.LockedBy == nil || *targetDelivery.LockedBy != "worker-test-1" {
		t.Errorf("locked_by delivery = %v, mau worker-test-1", targetDelivery.LockedBy)
	}

	// Siapkan Dispatcher dengan SSRF policy yang mengizinkan loopback
	ssrfPolicy := security.SSRFPolicy{
		AllowHTTP:           true,
		AllowedPrivateAddrs: []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")},
	}
	disp := NewDispatcher(r, cipher, srv.Client(), ssrfPolicy, nil)
	disp.SetRandJitter(func(n int64) int64 { return 0 }) // Jitter deterministik untuk test

	// 4. Pengiriman percobaan pertama: menghasilkan kegagalan HTTP 500
	if err := disp.Dispatch(ctx, targetDelivery); err != nil {
		t.Fatalf("Dispatch percobaan 1 error: %v", err)
	}

	// Cek status setelah gagal
	d1, err := r.GetDelivery(ctx, targetDelivery.ID)
	if err != nil {
		t.Fatalf("GetDelivery: %v", err)
	}
	if d1.Status != StatusFailed {
		t.Errorf("status delivery setelah 500 = %s, mau failed", d1.Status)
	}
	if d1.AttemptCount != 1 {
		t.Errorf("attempt_count = %d, mau 1", d1.AttemptCount)
	}
	if d1.LockedAt != nil {
		t.Errorf("locked_at seharusnya nil setelah keluar dari delivering")
	}

	// Periksa webhook induk: consecutive_failures harus 1 dan webhook TIDAK dinonaktifkan
	wh1, err := r.Get(ctx, wh.ID)
	if err != nil {
		t.Fatalf("Get webhook: %v", err)
	}
	if wh1.ConsecutiveFailures != 1 {
		t.Errorf("consecutive_failures = %d, mau 1", wh1.ConsecutiveFailures)
	}
	if !wh1.Enabled {
		t.Errorf("worker melanggar aturan 6: webhook dinonaktifkan otomatis padahal dilarang")
	}

	// 5. Majukan next_attempt_at ke masa lalu agar bisa di-claim kembali
	_, err = pool.Exec(ctx, "update webhook_deliveries set next_attempt_at = now() - interval '1 second' where id = $1", targetDelivery.ID)
	if err != nil {
		t.Fatalf("update next_attempt_at gagal: %v", err)
	}

	claimedLagi, err := r.ClaimDue(ctx, "worker-test-2", 10)
	if err != nil {
		t.Fatalf("ClaimDue kedua: %v", err)
	}
	var targetDelivery2 *Delivery
	for _, d := range claimedLagi {
		if d.ID == targetDelivery.ID {
			targetDelivery2 = d
			break
		}
	}
	if targetDelivery2 == nil {
		t.Fatalf("delivery tidak bisa di-claim ulang setelah gagal")
	}

	// 6. Pengiriman percobaan kedua: sukses HTTP 200
	if err := disp.Dispatch(ctx, targetDelivery2); err != nil {
		t.Fatalf("Dispatch percobaan 2 error: %v", err)
	}

	// Cek status setelah berhasil
	d2, err := r.GetDelivery(ctx, targetDelivery.ID)
	if err != nil {
		t.Fatalf("GetDelivery d2: %v", err)
	}
	if d2.Status != StatusDelivered {
		t.Errorf("status delivery setelah 200 = %s, mau delivered", d2.Status)
	}
	if d2.DeliveredAt == nil {
		t.Errorf("delivered_at seharusnya tidak nil saat status delivered")
	}

	// Cek HMAC signature yang diterima server
	if !VerifySignature(receivedPayload, []byte(rawSecret), receivedSig) {
		t.Errorf("tanda tangan webhook yang diterima server tidak valid: %s", receivedSig)
	}

	// Cek webhook induk: consecutive_failures harus direset ke 0
	wh2, err := r.Get(ctx, wh.ID)
	if err != nil {
		t.Fatalf("Get webhook wh2: %v", err)
	}
	if wh2.ConsecutiveFailures != 0 {
		t.Errorf("consecutive_failures setelah sukses = %d, mau 0", wh2.ConsecutiveFailures)
	}
}

func TestIntegrationWebhooksStuckRecovery(t *testing.T) {
	pool, _ := testPool(t)
	ctx := context.Background()
	r := NewRepo(pool)

	testID := testUUID(t)
	// Sisipkan webhook dan delivery langsung dengan status 'delivering' yang tertinggal
	_, err := pool.Exec(ctx, `
		insert into webhooks (id, name, url, events, secret_ciphertext, encryption_key_id, masked_hint)
		values ($1::uuid, $2, 'http://127.0.0.1:9999', array['*'], 'v1.1234.ciphertext', '1234', 'whsec_****1234')`,
		testID, "webhook-stuck-"+testID[:8],
	)
	if err != nil {
		t.Fatalf("insert webhook stuck: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, "delete from webhooks where id = $1::uuid", testID)
	})

	var delID int64
	err = pool.QueryRow(ctx, `
		insert into webhook_deliveries (webhook_id, event, payload, status, locked_at, locked_by)
		values ($1::uuid, 'circuit.opened', '{"state":"open"}'::jsonb, 'delivering', now() - interval '10 minutes', 'worker-mati')
		returning id`,
		testID,
	).Scan(&delID)
	if err != nil {
		t.Fatalf("insert stuck delivery: %v", err)
	}

	// Pulihkan antrean yang tersangkut > 5 menit
	recovered, err := r.RecoverStuck(ctx, 5*time.Minute)
	if err != nil {
		t.Fatalf("RecoverStuck: %v", err)
	}
	if recovered < 1 {
		t.Errorf("RecoverStuck memulihkan %d baris, mau minimal 1", recovered)
	}

	// Cek status delivery yang dipulihkan
	d, err := r.GetDelivery(ctx, delID)
	if err != nil {
		t.Fatalf("GetDelivery: %v", err)
	}
	if d.Status != StatusPending {
		t.Errorf("status setelah pemulihan = %s, mau pending", d.Status)
	}
	if d.LockedAt != nil || d.LockedBy != nil {
		t.Errorf("locked_at dan locked_by harus nil setelah pemulihan sewa")
	}
}

func TestIntegrationWebhooksPembersihanRetensi(t *testing.T) {
	pool, _ := testPool(t)
	ctx := context.Background()
	r := NewRepo(pool)

	testID := testUUID(t)
	_, err := pool.Exec(ctx, `
		insert into webhooks (id, name, url, events, secret_ciphertext, encryption_key_id, masked_hint)
		values ($1::uuid, $2, 'http://127.0.0.1:9999', array['*'], 'v1.1234.ciphertext', '1234', 'whsec_****1234')`,
		testID, "webhook-retensi-"+testID[:8],
	)
	if err != nil {
		t.Fatalf("insert webhook retensi: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, "delete from webhooks where id = $1::uuid", testID)
	})

	// Sisipkan baris delivered kuno (30 hari lalu)
	_, err = pool.Exec(ctx, `
		insert into webhook_deliveries (webhook_id, event, payload, status, created_at, delivered_at)
		values ($1::uuid, 'circuit.closed', '{"state":"closed"}'::jsonb, 'delivered', now() - interval '30 days', now() - interval '30 days')`,
		testID,
	)
	if err != nil {
		t.Fatalf("insert old delivered: %v", err)
	}

	// Sisipkan baris pending kuno (TIDAK BOLEH dihapus oleh pembersihan delivered/abandoned)
	var pendingID int64
	err = pool.QueryRow(ctx, `
		insert into webhook_deliveries (webhook_id, event, payload, status, created_at)
		values ($1::uuid, 'circuit.closed', '{"state":"closed"}'::jsonb, 'pending', now() - interval '30 days')
		returning id`,
		testID,
	).Scan(&pendingID)
	if err != nil {
		t.Fatalf("insert old pending: %v", err)
	}

	// Jalankan pembersihan retensi dengan batas 7 hari lalu
	cutoff := time.Now().Add(-7 * 24 * time.Hour)
	terhapus, err := r.DeleteRetention(ctx, cutoff, 100)
	if err != nil {
		t.Fatalf("DeleteRetention: %v", err)
	}
	if terhapus < 1 {
		t.Errorf("DeleteRetention menghapus %d baris, mau minimal 1", terhapus)
	}

	// Pastikan baris pending kuno TIDAK terhapus
	dPending, err := r.GetDelivery(ctx, pendingID)
	if err != nil {
		t.Fatalf("baris pending kuno seharusnya tidak terhapus: %v", err)
	}
	if dPending.Status != StatusPending {
		t.Errorf("status baris pending kuno berubah: %s", dPending.Status)
	}
}

func TestIntegrationWebhooksCRUDDanListDeliveries(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL tidak disetel, melewati pengujian integrasi database")
	}

	ctx := context.Background()
	pool, cipher := testPool(t)

	r := NewRepo(pool)

	// 1. Buat webhook awal
	wh, err := r.Create(ctx, CreateWebhookParams{
		Name:      "webhook-crud-test",
		URL:       "https://example.com/hooks",
		Events:    []string{"provider.unhealthy"},
		RawSecret: "whsec_secret123",
		Cipher:    cipher,
	})
	if err != nil {
		t.Fatalf("Create webhook: %v", err)
	}
	t.Cleanup(func() {
		_ = r.Delete(ctx, wh.ID)
	})

	// 2. Uji Update
	newName := "webhook-crud-updated"
	newURL := "https://example.com/hooks/v2"
	updated, err := r.Update(ctx, wh.ID, UpdateWebhookParams{
		Name: &newName,
		URL:  &newURL,
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Name != newName || updated.URL != newURL {
		t.Errorf("Update hasil tidak sesuai: name=%s, url=%s", updated.Name, updated.URL)
	}

	// 3. Uji SetEnabled
	disabled, err := r.SetEnabled(ctx, wh.ID, false)
	if err != nil {
		t.Fatalf("SetEnabled false: %v", err)
	}
	if disabled.Enabled {
		t.Errorf("webhook seharusnya disabled")
	}

	// 4. Enqueue & ListDeliveries
	_, err = r.Enqueue(ctx, "provider.unhealthy", map[string]any{"status": "down"})
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	// Aktifkan kembali agar bisa claim atau list
	_, _ = r.SetEnabled(ctx, wh.ID, true)

	deliveries, _, err := r.ListDeliveries(ctx, wh.ID, repo.Page{Limit: 10})
	if err != nil {
		t.Fatalf("ListDeliveries: %v", err)
	}
	_ = deliveries

	// 5. Uji Delete
	if err := r.Delete(ctx, wh.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err = r.Get(ctx, wh.ID)
	if !errors.Is(err, repo.ErrNotFound) {
		t.Errorf("Get setelah Delete seharusnya ErrNotFound, dapat: %v", err)
	}
}
