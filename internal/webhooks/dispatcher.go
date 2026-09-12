package webhooks

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/NexGen-X/Route-X/internal/security"
)

const (
	// DefaultBaseBackoff adalah jeda awal untuk backoff webhook.
	DefaultBaseBackoff = 2 * time.Second
	// MaxWebhookBackoff membatasi jeda pengulangan maksimum ke 1 jam.
	MaxWebhookBackoff = 1 * time.Hour
	// DefaultDeliveryTimeout adalah batas waktu pengiriman jika webhook tidak menyetelnya.
	DefaultDeliveryTimeout = 10 * time.Second
)

// Dispatcher bertugas mengirim satu webhook delivery melalui HTTP, menandatangani payload
// dengan HMAC-SHA256, dan mencatat hasilnya kembali ke repositori.
type Dispatcher struct {
	repo       *Repo
	cipher     *security.Cipher
	client     *http.Client
	ssrfPolicy security.SSRFPolicy
	logger     *slog.Logger
	randJitter func(int64) int64
}

// NewDispatcher membuat instance baru Dispatcher webhook.
//
// Menggunakan transport berpenjaga dial (security.GuardedDialContext) untuk menutup celah DNS rebinding
// serta memasang CheckRedirect untuk mencegah SSRF lewat celah pengalihan (redirect) HTTP hop.
func NewDispatcher(
	repo *Repo,
	cipher *security.Cipher,
	client *http.Client,
	ssrfPolicy security.SSRFPolicy,
	logger *slog.Logger,
) *Dispatcher {
	dialer := &net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 30 * time.Second,
	}
	defaultTr := &http.Transport{
		DialContext:           security.GuardedDialContext(ssrfPolicy, dialer),
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   32,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ForceAttemptHTTP2:     true,
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
	}

	checkRedirect := func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return errors.New("terlalu banyak pengalihan (redirect limit 10 tercapai)")
		}
		// Validasi setiap hop redirect terhadap SSRF policy (skema, kredensial userinfo, IP privat)
		if err := security.ValidateBaseURL(req.URL.String(), ssrfPolicy); err != nil {
			return fmt.Errorf("pengalihan URL melanggar kebijakan SSRF: %w", err)
		}
		return nil
	}

	if client == nil {
		// Client.Timeout SENGAJA dibiarkan nol (tidak diset): batas waktu per-webhook
		// timeout_ms (maksimal 120 detik) sudah ditegakkan lewat context WithTimeout
		// pada setiap request (reqCtx). Mengisi Client.Timeout akan memotong paksa
		// seluruh pengiriman pada DefaultDeliveryTimeout (10 detik) termasuk redirect
		// dan baca body, sehingga webhook dengan timeout_ms besar tidak pernah berlaku.
		client = &http.Client{
			Transport:     defaultTr,
			CheckRedirect: checkRedirect,
		}
	} else {
		if client.Transport == nil {
			client.Transport = defaultTr
		}
		if client.CheckRedirect == nil {
			client.CheckRedirect = checkRedirect
		}
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Dispatcher{
		repo:       repo,
		cipher:     cipher,
		client:     client,
		ssrfPolicy: ssrfPolicy,
		logger:     logger,
		randJitter: defaultCryptoRandJitter,
	}
}

// SetRandJitter mengganti fungsi pengacak jitter (berguna untuk test deterministik).
func (d *Dispatcher) SetRandJitter(fn func(int64) int64) {
	d.randJitter = fn
}

// Dispatch memproses satu antrean delivery: mendekripsi secret, menandatangani payload,
// mengirim HTTP POST, dan memperbarui status baris.
func (d *Dispatcher) Dispatch(ctx context.Context, delivery *Delivery) error {
	if delivery == nil {
		return errors.New("delivery webhook kosong")
	}

	wh, err := d.repo.Get(ctx, delivery.WebhookID)
	if err != nil {
		d.logger.WarnContext(ctx, "webhook tidak ditemukan untuk antrean delivery",
			"delivery_id", delivery.ID, "webhook_id", delivery.WebhookID, "error", err)
		// Bila webhook induk sudah dihapus, delivery ini tidak bisa dikirim lagi.
		return d.repo.RecordFailure(ctx, delivery.ID, delivery.WebhookID, delivery.AttemptCount+1, 0,
			time.Now(), nil, "webhook induk tidak ditemukan")
	}

	// Jika endpoint dinonaktifkan manual oleh operator, batalkan pengiriman.
	if !wh.Enabled {
		d.logger.InfoContext(ctx, "webhook dinonaktifkan oleh operator, pengiriman ditinggalkan",
			"delivery_id", delivery.ID, "webhook_id", wh.ID)
		return d.repo.RecordFailure(ctx, delivery.ID, wh.ID, wh.MaxRetries, wh.MaxRetries,
			time.Now(), nil, "webhook dinonaktifkan operator")
	}

	// 1. Dekripsi secret webhook memakai AAD terikat ke ID webhook.
	aad := security.WebhookAAD(wh.ID)
	secretBytes, err := d.cipher.Decrypt(wh.SecretCiphertext, aad)
	if err != nil {
		d.logger.ErrorContext(ctx, "gagal mendekripsi secret webhook",
			"delivery_id", delivery.ID, "webhook_id", wh.ID, "error", err)
		return d.repo.RecordFailure(ctx, delivery.ID, wh.ID, wh.MaxRetries, wh.MaxRetries,
			time.Now(), nil, "gagal mendekripsi secret webhook")
	}

	// 2. Hitung tanda tangan HMAC-SHA256 dari payload mentah.
	mac := hmac.New(sha256.New, secretBytes)
	mac.Write(delivery.Payload)
	sigHex := hex.EncodeToString(mac.Sum(nil))

	// 3. Siapkan HTTP Request dengan batas waktu timeout_ms.
	timeout := time.Duration(wh.TimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = DefaultDeliveryTimeout
	}
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Validasi URL terhadap SSRF guard sebelum dial.
	if err := security.ValidateBaseURL(wh.URL, d.ssrfPolicy); err != nil {
		sanitizedErr := SanitizeErrorMessage(err.Error())
		d.logger.WarnContext(ctx, "URL webhook melanggar kebijakan SSRF",
			"delivery_id", delivery.ID, "webhook_id", wh.ID, "error", sanitizedErr)
		return d.repo.RecordFailure(ctx, delivery.ID, wh.ID, wh.MaxRetries, wh.MaxRetries,
			time.Now(), nil, "SSRF: "+sanitizedErr)
	}

	bodyPayload := delivery.Payload
		if strings.Contains(wh.URL, "discord.com/api/webhooks") {
			discordBody := map[string]string{"content": fmt.Sprintf("🔔 **Route-X Alert**\n```json\n%s\n```", string(delivery.Payload))}
			bodyPayload, _ = json.Marshal(discordBody)
		} else if strings.Contains(wh.URL, "api.telegram.org/bot") {
			tgBody := map[string]string{"text": fmt.Sprintf("🔔 *Route-X Alert*\n```json\n%s\n```", string(delivery.Payload))}
			bodyPayload, _ = json.Marshal(tgBody)
		}
		req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, wh.URL, bytes.NewReader(bodyPayload))
	if err != nil {
		sanitizedErr := SanitizeErrorMessage(err.Error())
		return d.repo.RecordFailure(ctx, delivery.ID, wh.ID, delivery.AttemptCount+1, wh.MaxRetries,
			time.Now().Add(d.backoff(delivery.AttemptCount+1)), nil, sanitizedErr)
	}

	nowUnix := strconv.FormatInt(time.Now().Unix(), 10)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Route-X-Webhook/1.0")
	req.Header.Set("X-RouteX-Delivery", strconv.FormatInt(delivery.ID, 10))
	req.Header.Set("X-RouteX-Event", delivery.Event)
	req.Header.Set("X-RouteX-Timestamp", nowUnix)
	req.Header.Set("X-RouteX-Signature", "sha256="+sigHex)
	req.Header.Set("X-Hub-Signature-256", "sha256="+sigHex)

	// 4. Kirim request HTTP.
	resp, err := d.client.Do(req)
	now := time.Now()
	if err != nil {
		sanitizedErr := SanitizeErrorMessage(err.Error())
		newAttempt := delivery.AttemptCount + 1
		nextAt := now.Add(d.backoff(newAttempt))
		d.logger.WarnContext(ctx, "pengiriman webhook gagal menghubungi endpoint",
			"delivery_id", delivery.ID, "webhook_id", wh.ID, "attempt", newAttempt, "error", sanitizedErr)
		return d.repo.RecordFailure(ctx, delivery.ID, wh.ID, newAttempt, wh.MaxRetries, nextAt, nil, sanitizedErr)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))
		_ = resp.Body.Close()
	}()

	// 5. Evaluasi kode status respons HTTP.
	// 2xx dianggap sukses; di luar 2xx dianggap gagal dan dijadwalkan ulang bila retry masih ada.
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return d.repo.RecordSuccess(ctx, delivery.ID, wh.ID, resp.StatusCode)
	}

	newAttempt := delivery.AttemptCount + 1
	nextAt := now.Add(d.backoff(newAttempt))
	errMsg := fmt.Sprintf("HTTP %d", resp.StatusCode)
	d.logger.WarnContext(ctx, "endpoint webhook mengembalikan status non-2xx",
		"delivery_id", delivery.ID, "webhook_id", wh.ID, "status_code", resp.StatusCode, "attempt", newAttempt)
	return d.repo.RecordFailure(ctx, delivery.ID, wh.ID, newAttempt, wh.MaxRetries, nextAt, &resp.StatusCode, errMsg)
}

func (d *Dispatcher) backoff(attempt int) time.Duration {
	return CalculateBackoff(attempt, DefaultBaseBackoff, MaxWebhookBackoff, d.randJitter)
}

// CalculateBackoff menghitung jeda eksponensial dengan equal jitter.
//
// Separuh jeda dijamin dan separuhnya diundi acak, sama seperti backoff upstream di executor.go.
// Ini mencegah efek kawanan (thundering herd) ketika banyak pengiriman mengulang serentak.
func CalculateBackoff(
	attempt int,
	base time.Duration,
	maxBackoff time.Duration,
	randFn func(int64) int64,
) time.Duration {
	if base <= 0 || attempt < 1 {
		return 0
	}
	// Batasi pergeseran bit agar tidak meluap menjadi negatif
	geser := min(attempt-1, 20)
	d := base << geser
	if d <= 0 || d > maxBackoff {
		d = maxBackoff
	}
	separuh := int64(d / 2)
	if separuh <= 0 {
		return d
	}
	if randFn == nil {
		randFn = defaultCryptoRandJitter
	}
	return time.Duration(separuh + randFn(separuh))
}

// defaultCryptoRandJitter mengundi bilangan acak dari 0 sampai n-1 menggunakan crypto/rand.
func defaultCryptoRandJitter(n int64) int64 {
	if n <= 0 {
		return 0
	}
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return 0
	}
	val := int64(binary.LittleEndian.Uint64(b[:]) & 0x7fffffffffffffff)
	return val % n
}

// urlQueryRegex menangkap query parameter pada URL dalam pesan teks.
var urlQueryRegex = regexp.MustCompile(`(\?|&)[^ \t\r\n"'<>()]+`)

// urlInTextRegex menangkap URL berprotokol http atau https dalam teks pesan error.
var urlInTextRegex = regexp.MustCompile(`https?://[^\s"'<>()]+`)

// SanitizeErrorMessage menyaring pesan error agar tidak membocorkan query parameter URL atau userinfo (kredensial).
//
// Aturan 9: URL webhook bisa memuat token atau secret di query string atau kredensial userinfo;
// jangan pernah menulis error mentah klien HTTP ke basis data tanpa disanitasi.
func SanitizeErrorMessage(raw string) string {
	if raw == "" {
		return ""
	}

	// Ganti semua kemunculan URL dengan representasi tersanitasi (tanpa query dan dengan userinfo tersamarkan lewat u.Redacted())
	clean := urlInTextRegex.ReplaceAllStringFunc(raw, func(rawURL string) string {
		suffix := ""
		trimmed := rawURL
		for len(trimmed) > 0 && (strings.HasSuffix(trimmed, ":") || strings.HasSuffix(trimmed, ",") || strings.HasSuffix(trimmed, ".")) {
			suffix = trimmed[len(trimmed)-1:] + suffix
			trimmed = trimmed[:len(trimmed)-1]
		}
		u, err := url.Parse(trimmed)
		if err != nil {
			return "[URL_TIDAK_SAH]" + suffix
		}
		u.RawQuery = ""
		return u.Redacted() + suffix
	})

	// Jaring pengaman tambahan untuk query parameter sisa
	clean = urlQueryRegex.ReplaceAllString(clean, "?[TERSEMBUNYI]")

	// Potong agar panjang pesan wajar dan tidak memenuhi kolom basis data
	const maxLen = 400
	if len(clean) > maxLen {
		clean = clean[:maxLen] + "..."
	}
	return clean
}

// MaskSecret menyamarkan secret webhook untuk tampilan dashboard, mis. "whsec_****1b7e".
func MaskSecret(secret string) string {
	s := strings.TrimSpace(secret)
	if len(s) == 0 {
		return "whsec_****"
	}
	if len(s) <= 4 {
		return "whsec_****" + s
	}
	last4 := s[len(s)-4:]
	return "whsec_****" + last4
}

// VerifySignature memverifikasi tanda tangan payload webhook menggunakan secret HMAC-SHA256.
//
// Menggunakan hmac.Equal agar kebal terhadap serangan timing attack.
func VerifySignature(payload []byte, secret []byte, sigHeader string) bool {
	sig := strings.TrimSpace(sigHeader)
	sig = strings.TrimPrefix(sig, "sha256=")
	expectedHex, err := hex.DecodeString(sig)
	if err != nil {
		return false
	}

	mac := hmac.New(sha256.New, secret)
	mac.Write(payload)
	computed := mac.Sum(nil)

	return hmac.Equal(computed, expectedHex)
}

// Ping mengirimkan HTTP POST uji koneksi langsung ke endpoint webhook tanpa melalui antrean database.
// Ini mencegah tes ping menyebar ke pelanggan event wildcard (*) dan mematuhi konvensi event berformat valid.
func (d *Dispatcher) Ping(ctx context.Context, wh *Webhook) (int, time.Duration, error) {
	if wh == nil {
		return 0, 0, errors.New("webhook tidak boleh nil")
	}

	// 1. Dekripsi secret webhook memakai AAD terikat ke ID webhook
	aad := security.WebhookAAD(wh.ID)
	secretBytes, err := d.cipher.Decrypt(wh.SecretCiphertext, aad)
	if err != nil {
		return 0, 0, fmt.Errorf("gagal mendekripsi secret webhook: %w", err)
	}

	// 2. Siapkan payload uji ping dengan event berformat domain.aksi valid
	nowStr := time.Now().UTC().Format(time.RFC3339)
	payload := []byte(fmt.Sprintf(`{"event":"ping.test","webhook_id":%q,"timestamp":%q,"message":"Uji koneksi webhook dari Route-X Admin"}`, wh.ID, nowStr))

	// 3. Hitung HMAC-SHA256
	mac := hmac.New(sha256.New, secretBytes)
	mac.Write(payload)
	sigHex := hex.EncodeToString(mac.Sum(nil))

	// 4. Validasi SSRF
	if err := security.ValidateBaseURL(wh.URL, d.ssrfPolicy); err != nil {
		return 0, 0, fmt.Errorf("SSRF: %s", SanitizeErrorMessage(err.Error()))
	}

	timeout := time.Duration(wh.TimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = DefaultDeliveryTimeout
	}
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	bodyPayload := payload
		if strings.Contains(wh.URL, "discord.com/api/webhooks") {
			discordBody := map[string]string{"content": fmt.Sprintf("🔔 **Route-X Ping**\n```json\n%s\n```", string(payload))}
			bodyPayload, _ = json.Marshal(discordBody)
		} else if strings.Contains(wh.URL, "api.telegram.org/bot") {
			tgBody := map[string]string{"text": fmt.Sprintf("🔔 *Route-X Ping*\n```json\n%s\n```", string(payload))}
			bodyPayload, _ = json.Marshal(tgBody)
		}
		req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, wh.URL, bytes.NewReader(bodyPayload))
	if err != nil {
		return 0, 0, fmt.Errorf("membuat request ping: %w", err)
	}

	nowUnix := strconv.FormatInt(time.Now().Unix(), 10)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Route-X-Webhook/1.0")
	req.Header.Set("X-RouteX-Delivery", "0")
	req.Header.Set("X-RouteX-Event", "ping.test")
	req.Header.Set("X-RouteX-Timestamp", nowUnix)
	req.Header.Set("X-RouteX-Signature", "sha256="+sigHex)
	req.Header.Set("X-Hub-Signature-256", "sha256="+sigHex)

	start := time.Now()
	resp, err := d.client.Do(req)
	dur := time.Since(start)
	if err != nil {
		return 0, dur, fmt.Errorf("gagal menghubungi endpoint: %s", SanitizeErrorMessage(err.Error()))
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))
		_ = resp.Body.Close()
	}()

	return resp.StatusCode, dur, nil
}
