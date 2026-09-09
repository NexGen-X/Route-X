package providers

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/NexGen-X/Route-X/internal/security"
)

// Batas dan tenggat bawaan klien upstream.
const (
	// DefaultTimeout adalah batas seluruh permintaan non-streaming.
	DefaultTimeout = 120 * time.Second
	// DefaultConnectTimeout membatasi tahap dial saja.
	DefaultConnectTimeout = 10 * time.Second
	// MaxErrorBodyBytes membatasi body error yang dibaca untuk klasifikasi. Body error
	// tidak perlu utuh, dan provider yang bermasalah bisa mengirim muatan raksasa.
	MaxErrorBodyBytes = 64 * 1024
	// MaxSSELineBytes membatasi satu baris SSE. Tanpa batas, satu baris tanpa akhir
	// akan menghabiskan memori proses.
	MaxSSELineBytes = 1 << 20
)

// ClientConfig mengatur klien HTTP satu provider.
type ClientConfig struct {
	// BaseURL sudah divalidasi lewat security.ValidateBaseURL sebelum sampai ke sini.
	BaseURL string
	// Timeout untuk permintaan non-streaming. Nol memakai DefaultTimeout.
	//
	// Permintaan streaming TIDAK memakai batas ini: satu completion panjang bisa
	// berjalan lebih lama dari batas mana pun yang masuk akal untuk request biasa,
	// dan memotongnya di tengah berarti membuang jawaban yang sudah separuh jadi.
	// Pembatasannya lewat context yang dibawa pemanggil.
	Timeout time.Duration
	// SSRFPolicy menjaga alamat yang boleh dihubungi.
	SSRFPolicy security.SSRFPolicy
	// ProxyURL adalah egress proxy opsional. Bertipe Secret karena biasanya memuat
	// kredensial.
	ProxyURL security.Secret
	// MaxIdleConnsPerHost menahan koneksi tetap terbuka antar permintaan; upstream AI
	// hampir selalu host yang sama berulang kali, jadi handshake TLS layak dihindari.
	MaxIdleConnsPerHost int
}

// NewHTTPClient membuat klien HTTP berpenjaga SSRF untuk satu provider.
//
// Klien untuk streaming dan non-streaming dibedakan hanya oleh field Timeout: pada
// streaming ia harus nol, karena http.Client.Timeout membatasi SELURUH umur respons
// termasuk pembacaan body — memasangnya berarti setiap stream mati pada detik yang sama
// tanpa peduli sedang mengalir atau tidak.
func NewHTTPClient(cfg ClientConfig, forStreaming bool) (*http.Client, error) {
	dialer := &net.Dialer{
		Timeout:   DefaultConnectTimeout,
		KeepAlive: 30 * time.Second,
	}

	idle := cfg.MaxIdleConnsPerHost
	if idle <= 0 {
		idle = 32
	}

	tr := &http.Transport{
		DialContext:           security.GuardedDialContext(cfg.SSRFPolicy, dialer),
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   idle,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ForceAttemptHTTP2:     true,
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
	}

	if !cfg.ProxyURL.IsZero() {
		proxy, err := parseProxy(cfg.ProxyURL)
		if err != nil {
			return nil, err
		}
		tr.Proxy = http.ProxyURL(proxy)
		// Penjaga SSRF pada dialer memeriksa alamat PROXY, bukan tujuan akhir — begitu
		// lalu lintas lewat proxy, tujuan sebenarnya tidak lagi terlihat di lapisan
		// dial. Karena itu base URL tetap wajib lolos ValidateBaseURL, dan egress pool
		// hanya boleh diisi operator.
	}

	client := &http.Client{Transport: tr}
	if !forStreaming {
		client.Timeout = cfg.Timeout
		if client.Timeout <= 0 {
			client.Timeout = DefaultTimeout
		}
	}
	// Pengalihan tidak diikuti: upstream AI tidak memakainya, dan mengikutinya membuka
	// jalur ke alamat yang tidak pernah divalidasi.
	client.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return client, nil
}

// parseProxy mengurai URL proxy tanpa membocorkan isinya ke pesan error.
func parseProxy(raw security.Secret) (*url.URL, error) {
	u, err := url.Parse(raw.Reveal())
	if err != nil {
		// Pesan dari url.Parse memuat URL lengkap beserta kredensialnya.
		return nil, errors.New("URL egress proxy tidak sah")
	}
	switch u.Scheme {
	case "http", "https", "socks5", "socks5h":
		return u, nil
	default:
		return nil, fmt.Errorf("skema egress proxy %q tidak didukung", u.Scheme)
	}
}

// JoinURL menggabungkan base URL dengan path endpoint.
//
// Menangani base URL yang sudah maupun belum memuat "/v1", karena operator menuliskan
// keduanya dan menolak salah satunya hanya menghasilkan kegagalan yang membingungkan.
func JoinURL(base, path string) string {
	base = strings.TrimRight(base, "/")
	path = "/" + strings.TrimLeft(path, "/")

	// Bila base sudah berakhir dengan segmen yang sama dengan awal path, jangan
	// digandakan: "https://x/v1" + "/v1/chat" tidak boleh menjadi "https://x/v1/v1/chat".
	if seg := firstSegment(path); seg != "" && strings.HasSuffix(base, "/"+seg) {
		path = strings.TrimPrefix(path, "/"+seg)
		if path == "" {
			return base
		}
	}
	return base + path
}

func firstSegment(path string) string {
	p := strings.TrimLeft(path, "/")
	if i := strings.IndexByte(p, '/'); i >= 0 {
		return p[:i]
	}
	return p
}

// ReadErrorBody membaca body respons gagal, dibatasi ukurannya, lalu mengembalikan kode
// error milik provider bila bisa ditemukan.
//
// Yang dikembalikan hanya kode dan pesan singkat — body mentah TIDAK diteruskan ke
// pemanggil. Pesan error upstream sering memuat kembali potongan permintaan, dan pada
// gateway itu berarti isi permintaan satu pengguna muncul di log operator.
func ReadErrorBody(body io.Reader) (code, message string) {
	limited := io.LimitReader(body, MaxErrorBodyBytes)
	raw, err := io.ReadAll(limited)
	if err != nil || len(raw) == 0 {
		return "", ""
	}

	// Satu struct untuk semua dialek yang kami hadapi, bukan beberapa percobaan
	// berurutan. Percobaan berurutan tidak bekerja di sini: body Google juga memuat
	// "error.message", sehingga percobaan bentuk OpenAI berhasil lebih dulu dan
	// "error.status" milik Google tidak pernah terbaca.
	var shape struct {
		Error struct {
			Message string          `json:"message"`
			Code    json.RawMessage `json:"code"`
			Type    string          `json:"type"`
			Status  string          `json:"status"` // dipakai Google
		} `json:"error"`
	}
	if json.Unmarshal(raw, &shape) != nil || shape.Error.Message == "" {
		return "", ""
	}
	// Urutan pemilihan kode: "code" milik OpenAI paling spesifik, lalu "type", lalu
	// "status" milik Google.
	return firstNonEmpty(unquote(shape.Error.Code), shape.Error.Type, shape.Error.Status),
		truncate(shape.Error.Message, 200)
}

func unquote(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	return strings.Trim(string(raw), `"`)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func truncate(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}

// ParseRetryAfter membaca header Retry-After, yang bisa berupa jumlah detik atau
// timestamp HTTP.
func ParseRetryAfter(h http.Header) time.Duration {
	v := strings.TrimSpace(h.Get("Retry-After"))
	if v == "" {
		return 0
	}
	if secs, err := strconv.Atoi(v); err == nil {
		if secs < 0 {
			return 0
		}
		return time.Duration(secs) * time.Second
	}
	if t, err := http.ParseTime(v); err == nil {
		if d := time.Until(t); d > 0 {
			return d
		}
	}
	return 0
}

// SSEEvent adalah satu peristiwa Server-Sent Events.
type SSEEvent struct {
	Event string
	Data  []byte
	// Raw adalah blok mentah peristiwa ini, termasuk baris kosong penutup.
	Raw []byte
}

// SSEReader membaca aliran Server-Sent Events.
//
// Ditulis sendiri alih-alih memakai pustaka karena aliran dari upstream AI perlu
// diteruskan ke klien apa adanya sekaligus diintip untuk mengambil token usage — dua
// kebutuhan yang tidak dilayani sekaligus oleh pembaca SSE umum.
type SSEReader struct {
	r   *bufio.Reader
	buf bytes.Buffer
}

// NewSSEReader membuat pembaca SSE.
func NewSSEReader(r io.Reader) *SSEReader {
	return &SSEReader{r: bufio.NewReaderSize(r, 16*1024)}
}

// Next mengembalikan peristiwa berikutnya, atau io.EOF saat aliran berakhir.
func (s *SSEReader) Next() (*SSEEvent, error) {
	ev := &SSEEvent{}
	s.buf.Reset()
	var raw bytes.Buffer
	seen := false

	for {
		line, err := s.readLine()
		if err != nil {
			if errors.Is(err, io.EOF) && seen {
				// Aliran berakhir tanpa baris kosong penutup. Peristiwa yang sudah
				// terkumpul tetap dikembalikan — membuangnya berarti kehilangan chunk
				// terakhir, yang justru chunk pembawa token usage pada banyak provider.
				break
			}
			return nil, err
		}
		raw.Write(line)
		raw.WriteByte('\n')

		trimmed := strings.TrimRight(string(line), "\r")
		if trimmed == "" {
			if !seen {
				// Baris kosong sebelum ada isi: pemisah antar peristiwa atau keep-alive.
				raw.Reset()
				continue
			}
			break
		}
		// Baris berawalan ":" adalah komentar, biasanya keep-alive. Diperiksa SEBELUM
		// menandai peristiwa punya isi: kalau tidak, satu keep-alive di awal aliran
		// membuat baris kosong sesudahnya menutup peristiwa kosong, dan seluruh aliran
		// bergeser satu peristiwa.
		if strings.HasPrefix(trimmed, ":") {
			continue
		}
		seen = true

		field, value, found := strings.Cut(trimmed, ":")
		if !found {
			field, value = trimmed, ""
		}
		value = strings.TrimPrefix(value, " ")

		switch field {
		case "event":
			ev.Event = value
		case "data":
			if s.buf.Len() > 0 {
				s.buf.WriteByte('\n')
			}
			s.buf.WriteString(value)
		}
	}

	ev.Data = append([]byte(nil), s.buf.Bytes()...)
	ev.Raw = raw.Bytes()
	return ev, nil
}

// readLine membaca satu baris dengan batas ukuran.
//
// Batasnya diperiksa setiap kali potongan bertambah, bukan hanya di dalam cabang
// ErrBufferFull: baris raksasa yang berakhir dengan EOF alih-alih memenuhi buffer akan
// lolos dari pemeriksaan yang hanya ada di cabang itu.
func (s *SSEReader) readLine() ([]byte, error) {
	var acc []byte
	for {
		frag, err := s.r.ReadSlice('\n')
		acc = append(acc, frag...)

		if len(acc) > MaxSSELineBytes {
			return nil, fmt.Errorf("baris SSE melebihi %d byte", MaxSSELineBytes)
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		if err != nil {
			// EOF dengan isi terkumpul berarti baris terakhir tanpa newline penutup —
			// itu isi yang sah dan harus diserahkan, bukan dibuang.
			if errors.Is(err, io.EOF) && len(acc) > 0 {
				return bytes.TrimSuffix(acc, []byte("\n")), nil
			}
			return nil, err
		}
		return bytes.TrimSuffix(acc, []byte("\n")), nil
	}
}

// IsDoneMarker melaporkan apakah data peristiwa adalah penanda akhir aliran ala OpenAI.
func IsDoneMarker(data []byte) bool {
	return bytes.Equal(bytes.TrimSpace(data), []byte("[DONE]"))
}
