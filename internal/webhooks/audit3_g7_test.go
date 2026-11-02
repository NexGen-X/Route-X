package webhooks

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strconv"
	"strings"
	"testing"

	"github.com/NexGen-X/Route-X/internal/security"
)

// TestSanitizeErrorMessageMenyamarkanTokenDiPath membuktikan V-WH-10 tertutup:
// token bot yang hidup di PATH URL (Telegram/Discord) tidak lagi lolos utuh
// ke pesan error yang disimpan ke DB dan log.
func TestSanitizeErrorMessageMenyamarkanTokenDiPath(t *testing.T) {
	const tokenTG = "123456789:AAEfakeTokenTelegramUntukUji1234567"
	raw := `Post "https://api.telegram.org/bot` + tokenTG + `/sendMessage": dial tcp: connect: connection refused`
	clean := SanitizeErrorMessage(raw)
	if strings.Contains(clean, tokenTG) || strings.Contains(clean, "AAEfakeTokenTelegram") {
		t.Errorf("token Telegram masih bocor di path: %s", clean)
	}
	if !strings.Contains(clean, "api.telegram.org") || !strings.Contains(clean, "sendMessage") {
		t.Errorf("konteks URL yang sah ikut terbuang: %s", clean)
	}

	const tokenDC = "discordTokenPalsuUntukUji_abc123XYZ789"
	rawDC := `Post "https://discord.com/api/webhooks/123456789012345678/` + tokenDC + `": dial tcp: timeout`
	cleanDC := SanitizeErrorMessage(rawDC)
	if strings.Contains(cleanDC, tokenDC) {
		t.Errorf("token Discord masih bocor di path: %s", cleanDC)
	}
	if !strings.Contains(cleanDC, "discord.com/api/webhooks/123456789012345678") {
		t.Errorf("ID webhook Discord yang bukan rahasia ikut terbuang: %s", cleanDC)
	}
}

// TestKnownEventsKonsistenDenganProduser memastikan katalog langganan hanya
// memuat event yang dikenal (penjaga daftar; produsernya dipasang di gateway,
// guard, admin, billing, worker health, dan listener breaker — audit3 V-WH-08).
func TestKnownEventsKonsistenDenganProduser(t *testing.T) {
	for _, ev := range []string{
		"request.completed", "request.failed",
		"provider.unhealthy", "provider.recovered",
		"circuit.opened", "circuit.closed",
		"budget.threshold", "budget.exceeded",
		"ratelimit.exceeded", "content.blocked",
		"api_key.created", "api_key.revoked", "ban.created",
	} {
		if !ValidEvent(ev) {
			t.Errorf("event %q hilang dari katalog", ev)
		}
	}
	if ValidEvent("event.ngawur") {
		t.Error("event tak dikenal lolos ValidEvent")
	}
}

// tangkapServer adalah server penerima yang merekam request terakhir.
type tangkapServer struct {
	srv       *httptest.Server
	body      []byte
	header    http.Header
	hitCount  int
	respondFn func(w http.ResponseWriter, r *http.Request)
}

func newTangkapServer(t *testing.T, respondFn func(w http.ResponseWriter, r *http.Request)) *tangkapServer {
	t.Helper()
	ts := &tangkapServer{respondFn: respondFn}
	ts.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ts.hitCount++
		b, _ := io.ReadAll(r.Body)
		ts.body = b
		ts.header = r.Header.Clone()
		if ts.respondFn != nil {
			ts.respondFn(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(ts.srv.Close)
	return ts
}

func policyLoopbackUji() security.SSRFPolicy {
	return security.SSRFPolicy{
		AllowHTTP:           true,
		AllowedPrivateAddrs: []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")},
	}
}

// TestIntegrationWebhookEnabledFalseDihormati membuktikan V-WH-02 tertutup:
// webhook yang dibuat nonaktif tersimpan nonaktif dan tidak menerima antrean.
func TestIntegrationWebhookEnabledFalseDihormati(t *testing.T) {
	pool, cipher := testPool(t)
	ctx := context.Background()
	r := NewRepo(pool)

	wh, err := r.Create(ctx, CreateWebhookParams{
		Name:       "webhook-nonaktif-uji",
		URL:        "https://example.com/hooks",
		Events:     []string{"provider.unhealthy"},
		RawSecret:  "whsec_rahasia_uji_enabled",
		Cipher:     cipher,
		Enabled:    false,
		MaxRetries: 3,
		TimeoutMS:  5000,
	})
	if err != nil {
		t.Fatalf("Create webhook: %v", err)
	}
	t.Cleanup(func() { _ = r.Delete(ctx, wh.ID) })

	if wh.Enabled {
		t.Fatal("webhook yang dibuat dengan Enabled=false tersimpan aktif")
	}
	lagi, err := r.Get(ctx, wh.ID)
	if err != nil {
		t.Fatalf("Get webhook: %v", err)
	}
	if lagi.Enabled {
		t.Fatal("webhook nonaktif terbaca aktif dari DB")
	}

	n, err := r.Enqueue(ctx, "provider.unhealthy", map[string]any{"event": "provider.unhealthy"})
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if n != 0 {
		t.Fatalf("Enqueue memasukkan %d delivery ke webhook nonaktif, mau 0", n)
	}
}

// TestIntegrationDispatcherNonaktifLangsungAbandoned membuktikan V-WH-01
// tertutup: delivery pending milik webhook yang dinonaktifkan sesudah antre
// ditinggalkan permanen pada dispatch PERTAMA — tidak berputar ±1×/detik.
func TestIntegrationDispatcherNonaktifLangsungAbandoned(t *testing.T) {
	pool, cipher := testPool(t)
	ctx := context.Background()
	r := NewRepo(pool)

	wh, err := r.Create(ctx, CreateWebhookParams{
		Name:       "webhook-dimati-uji",
		URL:        "https://example.com/hooks",
		Events:     []string{"provider.unhealthy"},
		RawSecret:  "whsec_rahasia_uji_abandon",
		Cipher:     cipher,
		Enabled:    true,
		MaxRetries: 2,
		TimeoutMS:  5000,
	})
	if err != nil {
		t.Fatalf("Create webhook: %v", err)
	}
	t.Cleanup(func() { _ = r.Delete(ctx, wh.ID) })

	// Sisipkan delivery pending langsung (meniru antrean yang terlanjur masuk),
	// lalu nonaktifkan webhook-nya — persis skenario verifikasi live V-WH-01.
	var deliveryID int64
	err = pool.QueryRow(ctx, `
		insert into webhook_deliveries (webhook_id, event, payload, status, next_attempt_at)
		values ($1::uuid, 'provider.unhealthy', '{"event":"provider.unhealthy"}'::jsonb, 'pending', now())
		returning id`, wh.ID).Scan(&deliveryID)
	if err != nil {
		t.Fatalf("sisip delivery: %v", err)
	}
	if _, err := r.SetEnabled(ctx, wh.ID, false); err != nil {
		t.Fatalf("SetEnabled: %v", err)
	}

	claimed, err := r.ClaimDue(ctx, "worker-uji-g7", 10)
	if err != nil {
		t.Fatalf("ClaimDue: %v", err)
	}
	var target *Delivery
	for _, d := range claimed {
		if d.ID == deliveryID {
			target = d
		}
	}
	if target == nil {
		t.Fatalf("delivery %d tidak ter-claim", deliveryID)
	}

	disp := NewDispatcher(r, cipher, nil, policyLoopbackUji(), nil)
	if err := disp.Dispatch(ctx, target); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}

	d, err := r.GetDelivery(ctx, deliveryID)
	if err != nil {
		t.Fatalf("GetDelivery: %v", err)
	}
	if d.Status != StatusAbandoned {
		t.Fatalf("status delivery = %s, mau abandoned (bukan failed yang dijadwalkan ulang)", d.Status)
	}

	// Klaim berikutnya tidak boleh mengambilnya lagi — putaran abadi tertutup.
	claimed2, err := r.ClaimDue(ctx, "worker-uji-g7", 10)
	if err != nil {
		t.Fatalf("ClaimDue kedua: %v", err)
	}
	for _, d2 := range claimed2 {
		if d2.ID == deliveryID {
			t.Fatal("delivery abandoned ter-claim lagi — putaran retry abadi masih hidup")
		}
	}
}

// TestIntegrationDispatcherTandaTanganMengikatDanAtasBodyFinal membuktikan
// V-WH-05 + V-WH-06 tertutup: tanda tangan cocok dengan perhitungan ulang atas
// (timestamp, ID, body yang BENAR-BENAR diterima) — termasuk untuk URL Discord
// yang bodynya dibungkus — dan TIDAK cocok dengan skema lama HMAC(body) saja.
func TestIntegrationDispatcherTandaTanganMengikatDanAtasBodyFinal(t *testing.T) {
	pool, cipher := testPool(t)
	ctx := context.Background()
	r := NewRepo(pool)
	const rawSecret = "whsec_rahasia_uji_tandatangan"

	jalankan := func(t *testing.T, nama, urlSuffix string, discord bool) {
		ts := newTangkapServer(t, nil)
		wh, err := r.Create(ctx, CreateWebhookParams{
			Name:       nama,
			URL:        ts.srv.URL + urlSuffix,
			Events:     []string{"provider.unhealthy"},
			RawSecret:  rawSecret,
			Cipher:     cipher,
			Enabled:    true,
			MaxRetries: 0,
			TimeoutMS:  5000,
			SSRFPolicy: policyLoopbackUji(),
		})
		if err != nil {
			t.Fatalf("Create webhook: %v", err)
		}
		t.Cleanup(func() { _ = r.Delete(ctx, wh.ID) })

		if _, err := r.Enqueue(ctx, "provider.unhealthy", map[string]any{"event": "provider.unhealthy", "status": "down"}); err != nil {
			t.Fatalf("Enqueue: %v", err)
		}
		claimed, err := r.ClaimDue(ctx, "worker-uji-g7", 10)
		if err != nil {
			t.Fatalf("ClaimDue: %v", err)
		}
		var target *Delivery
		for _, d := range claimed {
			if d.WebhookID == wh.ID {
				target = d
			}
		}
		if target == nil {
			t.Fatal("delivery tidak ter-claim")
		}

		disp := NewDispatcher(r, cipher, ts.srv.Client(), policyLoopbackUji(), nil)
		if err := disp.Dispatch(ctx, target); err != nil {
			t.Fatalf("Dispatch: %v", err)
		}
		if ts.hitCount != 1 {
			t.Fatalf("server menerima %d request, mau 1", ts.hitCount)
		}

		sig := ts.header.Get("X-RouteX-Signature")
		tsHeader := ts.header.Get("X-RouteX-Timestamp")
		idHeader := ts.header.Get("X-RouteX-Delivery")
		if sig == "" || tsHeader == "" || idHeader == "" {
			t.Fatalf("header tanda tangan tidak lengkap: sig=%q ts=%q id=%q", sig, tsHeader, idHeader)
		}
		idParsed, err := strconv.ParseInt(idHeader, 10, 64)
		if err != nil {
			t.Fatalf("header delivery tidak bisa diparse: %v", err)
		}

		// Penerima memverifikasi atas body yang DITERIMA + header yang menyertai.
		if !VerifySignature(ts.body, []byte(rawSecret), tsHeader, idParsed, sig) {
			t.Errorf("verifikasi penerima gagal atas body yang diterima (discord=%v)", discord)
		}

		// Skema lama (HMAC atas body saja) tidak boleh cocok.
		mac := hmac.New(sha256.New, []byte(rawSecret))
		mac.Write(ts.body)
		if sig == "sha256="+hex.EncodeToString(mac.Sum(nil)) {
			t.Error("tanda tangan masih skema lama HMAC(body) — timestamp/ID tidak terikat")
		}
	}

	t.Run("url biasa", func(t *testing.T) { jalankan(t, "webhook-ttd-biasa", "/hook", false) })
	t.Run("url discord dibungkus", func(t *testing.T) {
		jalankan(t, "webhook-ttd-discord", "/discord.com/api/webhooks/123456/tokenpalsu", true)
	})
}

// TestIntegrationDispatcherRedirectHostLainDitolak membuktikan V-WH-09
// tertutup: redirect 307 ke host lain tidak diikuti, sehingga server kedua
// tidak menerima payload maupun tanda tangan.
func TestIntegrationDispatcherRedirectHostLainDitolak(t *testing.T) {
	pool, cipher := testPool(t)
	ctx := context.Background()
	r := NewRepo(pool)

	kedua := newTangkapServer(t, nil)
	pertama := newTangkapServer(t, func(w http.ResponseWriter, req *http.Request) {
		http.Redirect(w, req, kedua.srv.URL+"/target", http.StatusTemporaryRedirect)
	})

	wh, err := r.Create(ctx, CreateWebhookParams{
		Name:       "webhook-redirect-uji",
		URL:        pertama.srv.URL + "/awal",
		Events:     []string{"provider.unhealthy"},
		RawSecret:  "whsec_rahasia_uji_redirect",
		Cipher:     cipher,
		Enabled:    true,
		MaxRetries: 0,
		TimeoutMS:  5000,
		SSRFPolicy: policyLoopbackUji(),
	})
	if err != nil {
		t.Fatalf("Create webhook: %v", err)
	}
	t.Cleanup(func() { _ = r.Delete(ctx, wh.ID) })

	if _, err := r.Enqueue(ctx, "provider.unhealthy", map[string]any{"event": "provider.unhealthy"}); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	claimed, err := r.ClaimDue(ctx, "worker-uji-g7", 10)
	if err != nil {
		t.Fatalf("ClaimDue: %v", err)
	}
	var target *Delivery
	for _, d := range claimed {
		if d.WebhookID == wh.ID {
			target = d
		}
	}
	if target == nil {
		t.Fatal("delivery tidak ter-claim")
	}

	disp := NewDispatcher(r, cipher, pertama.srv.Client(), policyLoopbackUji(), nil)
	_ = disp.Dispatch(ctx, target)

	if kedua.hitCount != 0 {
		t.Fatalf("server kedua menerima %d request — redirect lintas host masih diikuti", kedua.hitCount)
	}
	d, err := r.GetDelivery(ctx, target.ID)
	if err != nil {
		t.Fatalf("GetDelivery: %v", err)
	}
	if d.Status == StatusDelivered {
		t.Fatal("delivery tercatat delivered padahal redirect ke host lain ditolak")
	}
}

// TestIntegrationCreateMenolakSecretKosongDanEventNgawur membuktikan validasi
// repo sebagai lapis kedua V-WH-03/V-WH-04.
func TestIntegrationCreateMenolakMasukanTidakSah(t *testing.T) {
	pool, cipher := testPool(t)
	ctx := context.Background()
	r := NewRepo(pool)

	kasus := []struct {
		nama   string
		ubah   func(p *CreateWebhookParams)
		muatDi string
	}{
		{"secret kosong", func(p *CreateWebhookParams) { p.RawSecret = "  " }, "secret"},
		{"event tidak dikenal", func(p *CreateWebhookParams) { p.Events = []string{"event.ngawur"} }, "tidak dikenal"},
		{"max_retries di luar rentang", func(p *CreateWebhookParams) { p.MaxRetries = 99 }, "max_retries"},
		{"timeout di luar rentang", func(p *CreateWebhookParams) { p.TimeoutMS = 50 }, "timeout_ms"},
	}
	for _, k := range kasus {
		t.Run(k.nama, func(t *testing.T) {
			p := CreateWebhookParams{
				Name:       "webhook-tidak-sah-" + k.nama,
				URL:        "https://example.com/hooks",
				Events:     []string{"provider.unhealthy"},
				RawSecret:  "whsec_rahasia_uji_validasi",
				Cipher:     cipher,
				Enabled:    true,
				MaxRetries: 3,
				TimeoutMS:  5000,
			}
			k.ubah(&p)
			_, err := r.Create(ctx, p)
			if err == nil {
				t.Fatalf("Create menerima masukan tidak sah (%s)", k.nama)
			}
			if !strings.Contains(err.Error(), k.muatDi) {
				t.Errorf("pesan error tidak menyebut field/masalah: %v", err)
			}
		})
	}
}
