package providers

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

// Klasifikasi error menentukan perilaku retry dan failover, jadi tabelnya diuji
// eksplisit — salah satu nilai di sini berubah berarti perilaku routing berubah.
func TestErrorRetryAndFailoverPolicy(t *testing.T) {
	tests := []struct {
		kind         ErrorKind
		retryable    bool
		failoverable bool
	}{
		{ErrKindTimeout, true, true},
		{ErrKindNetwork, true, true},
		{ErrKindRateLimit, true, true},
		{ErrKindOverloaded, true, true},
		{ErrKindServer, true, true},
		// Kuota habis: menunggu tidak menolong, provider lain menolong.
		{ErrKindQuota, false, true},
		// Kredensial: tidak berubah dalam milidetik, tapi provider lain punya kredensial sendiri.
		{ErrKindAuth, false, true},
		{ErrKindPermission, false, true},
		{ErrKindModelNotFound, false, true},
		// Permintaan cacat: mengalihkannya hanya menghasilkan kegagalan kedua.
		{ErrKindInvalidRequest, false, false},
		// Kebijakan konten: provider lain kemungkinan menolak dengan alasan sama.
		{ErrKindContentFilter, false, false},
		{ErrKindCanceled, false, false},
		{ErrKindStreamAborted, false, false},
		{ErrKindUnknown, false, false},
	}

	for _, tc := range tests {
		t.Run(string(tc.kind), func(t *testing.T) {
			e := &Error{Kind: tc.kind}
			if e.Retryable() != tc.retryable {
				t.Errorf("Retryable() = %v, mau %v", e.Retryable(), tc.retryable)
			}
			if e.Failoverable() != tc.failoverable {
				t.Errorf("Failoverable() = %v, mau %v", e.Failoverable(), tc.failoverable)
			}
		})
	}
}

// Setelah sebagian aliran terkirim ke klien, tidak ada kegagalan yang boleh diulang:
// aliran kedua akan tersambung di tengah aliran pertama dan klien menerima jawaban
// yang tidak koheren.
func TestStreamedResponseNeverRetried(t *testing.T) {
	for kind := range retryable {
		e := &Error{Kind: kind, StreamedBytes: 1}
		if e.Retryable() {
			t.Errorf("%s: Retryable() = true padahal sudah ada byte terkirim", kind)
		}
		if e.Failoverable() {
			t.Errorf("%s: Failoverable() = true padahal sudah ada byte terkirim", kind)
		}
	}
}

func TestClassify(t *testing.T) {
	tests := []struct {
		name   string
		status int
		code   string
		want   ErrorKind
	}{
		{"401", 401, "", ErrKindAuth},
		{"403", 403, "", ErrKindPermission},
		{"404", 404, "", ErrKindModelNotFound},
		{"408", 408, "", ErrKindTimeout},
		{"413", 413, "", ErrKindInvalidRequest},
		{"429 tanpa kode", 429, "", ErrKindRateLimit},
		{"400", 400, "", ErrKindInvalidRequest},
		{"500", 500, "", ErrKindServer},
		{"502", 502, "", ErrKindServer},
		{"503", 503, "", ErrKindOverloaded},
		{"529 ala Anthropic", 529, "", ErrKindOverloaded},
		{"200 bukan error", 200, "", ErrKindUnknown},

		// Kode upstream mengalahkan status: 429 dipakai untuk dua hal yang menuntut
		// keputusan routing berbeda.
		{"429 kuota habis", 429, "insufficient_quota", ErrKindQuota},
		{"429 saldo kurang", 429, "credit_balance_too_low", ErrKindQuota},
		{"200 model tak ada", 200, "model_not_found", ErrKindModelNotFound},
		{"400 kebijakan konten", 400, "content_policy_violation", ErrKindContentFilter},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Classify(tc.status, tc.code); got != tc.want {
				t.Errorf("Classify(%d, %q) = %s, mau %s", tc.status, tc.code, got, tc.want)
			}
		})
	}
}

// Pembatalan harus dikenali sebelum diperlakukan sebagai galat jaringan; kalau tidak,
// gateway mengulang permintaan yang kliennya sudah pergi.
func TestClassifyTransport(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want ErrorKind
	}{
		{"nil", nil, ErrKindUnknown},
		{"context dibatalkan", context.Canceled, ErrKindCanceled},
		{"deadline terlampaui", context.DeadlineExceeded, ErrKindTimeout},
		{"pembatalan terbungkus", errors.New("dial: " + context.Canceled.Error()), ErrKindNetwork},
		{"pembatalan terbungkus %w", wrap(context.Canceled), ErrKindCanceled},
		{"timeout jaringan", &net.DNSError{IsTimeout: true}, ErrKindTimeout},
		{"galat jaringan biasa", &net.DNSError{}, ErrKindNetwork},
		{"galat lain", errors.New("apa saja"), ErrKindNetwork},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ClassifyTransport(tc.err); got != tc.want {
				t.Errorf("ClassifyTransport = %s, mau %s", got, tc.want)
			}
		})
	}
}

func wrap(err error) error { return errWrapper{err} }

type errWrapper struct{ err error }

func (w errWrapper) Error() string { return "terbungkus: " + w.err.Error() }
func (w errWrapper) Unwrap() error { return w.err }

// Pesan dari paket net memuat host dan port upstream; pada BYOK itu alamat milik
// pengguna lain, jadi tidak boleh diteruskan.
func TestFromTransportHidesUpstreamAddress(t *testing.T) {
	err := &net.OpError{
		Op:   "dial",
		Net:  "tcp",
		Addr: &net.TCPAddr{IP: net.ParseIP("203.0.113.55"), Port: 8443},
		Err:  errors.New("connection refused"),
	}
	got := FromTransport("provider-byok", err)

	if strings.Contains(got.Error(), "203.0.113.55") || strings.Contains(got.Error(), "8443") {
		t.Errorf("pesan membocorkan alamat upstream: %v", got)
	}
	if !strings.Contains(got.Error(), "provider-byok") {
		t.Errorf("pesan tidak menyebut provider: %v", got)
	}
	if got.Kind != ErrKindNetwork {
		t.Errorf("Kind = %s, mau %s", got.Kind, ErrKindNetwork)
	}
}

func TestAsError(t *testing.T) {
	e := &Error{Kind: ErrKindTimeout, Provider: "x"}
	if AsError(e) != e {
		t.Error("AsError tidak mengembalikan error yang sama")
	}
	if AsError(wrap(e)) != e {
		t.Error("AsError gagal membuka error terbungkus")
	}
	if AsError(errors.New("bukan")) != nil {
		t.Error("AsError mengembalikan nilai untuk error lain")
	}
	if AsError(nil) != nil {
		t.Error("AsError(nil) tidak nil")
	}
}

func TestParseRetryAfter(t *testing.T) {
	tests := []struct {
		value string
		want  time.Duration
	}{
		{"", 0},
		{"5", 5 * time.Second},
		{"0", 0},
		{"-3", 0},
		{"bukan angka", 0},
		{" 12 ", 12 * time.Second},
	}
	for _, tc := range tests {
		h := http.Header{}
		if tc.value != "" {
			h.Set("Retry-After", tc.value)
		}
		if got := ParseRetryAfter(h); got != tc.want {
			t.Errorf("ParseRetryAfter(%q) = %v, mau %v", tc.value, got, tc.want)
		}
	}

	// Bentuk timestamp HTTP.
	h := http.Header{}
	h.Set("Retry-After", time.Now().Add(30*time.Second).UTC().Format(http.TimeFormat))
	if got := ParseRetryAfter(h); got < 25*time.Second || got > 31*time.Second {
		t.Errorf("timestamp Retry-After = %v, mau sekitar 30s", got)
	}
	// Timestamp yang sudah lewat berarti tidak perlu menunggu.
	h.Set("Retry-After", time.Now().Add(-time.Hour).UTC().Format(http.TimeFormat))
	if got := ParseRetryAfter(h); got != 0 {
		t.Errorf("timestamp lampau = %v, mau 0", got)
	}
}

func TestReadErrorBody(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		wantCode    string
		wantMessage string
	}{
		{"bentuk OpenAI", `{"error":{"message":"terlalu banyak permintaan","code":"rate_limit_exceeded","type":"rate_limit_error"}}`,
			"rate_limit_exceeded", "terlalu banyak permintaan"},
		{"OpenAI tanpa code", `{"error":{"message":"salah","type":"invalid_request_error"}}`,
			"invalid_request_error", "salah"},
		{"code berupa angka", `{"error":{"message":"salah","code":429}}`, "429", "salah"},
		{"bentuk Google", `{"error":{"status":"RESOURCE_EXHAUSTED","message":"kuota habis"}}`,
			"RESOURCE_EXHAUSTED", "kuota habis"},
		{"body kosong", "", "", ""},
		{"bukan JSON", "gagal total", "", ""},
		{"JSON tanpa error", `{"ok":true}`, "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			code, msg := ReadErrorBody(strings.NewReader(tc.body))
			if code != tc.wantCode || msg != tc.wantMessage {
				t.Errorf("= (%q, %q), mau (%q, %q)", code, msg, tc.wantCode, tc.wantMessage)
			}
		})
	}

	// Body raksasa dipotong, bukan dibaca seluruhnya.
	huge := `{"error":{"message":"` + strings.Repeat("a", 2*MaxErrorBodyBytes) + `"}}`
	_, msg := ReadErrorBody(strings.NewReader(huge))
	if len(msg) > 256 {
		t.Errorf("pesan tidak dipotong: %d karakter", len(msg))
	}
}

func TestJoinURL(t *testing.T) {
	tests := []struct{ base, path, want string }{
		{"https://api.openai.com", "/v1/chat/completions", "https://api.openai.com/v1/chat/completions"},
		{"https://api.openai.com/", "/v1/models", "https://api.openai.com/v1/models"},
		// Base yang sudah memuat /v1 tidak boleh digandakan.
		{"https://api.openai.com/v1", "/v1/chat/completions", "https://api.openai.com/v1/chat/completions"},
		{"https://api.openai.com/v1/", "/v1/models", "https://api.openai.com/v1/models"},
		{"https://x.test/openai/v1", "/v1/models", "https://x.test/openai/v1/models"},
		{"https://x.test", "v1/models", "https://x.test/v1/models"},
		{"https://x.test/v1", "/v1", "https://x.test/v1"},
	}
	for _, tc := range tests {
		if got := JoinURL(tc.base, tc.path); got != tc.want {
			t.Errorf("JoinURL(%q, %q) = %q, mau %q", tc.base, tc.path, got, tc.want)
		}
	}
}

func TestSSEReader(t *testing.T) {
	stream := "" +
		": keep-alive\n" +
		"\n" +
		"data: {\"delta\":\"Ha\"}\n" +
		"\n" +
		"event: chunk\n" +
		"data: {\"delta\":\"lo\"}\n" +
		"\n" +
		"data: baris satu\n" +
		"data: baris dua\n" +
		"\n" +
		"data: [DONE]\n" +
		"\n"

	r := NewSSEReader(strings.NewReader(stream))

	first, err := r.Next()
	if err != nil {
		t.Fatalf("peristiwa 1: %v", err)
	}
	if string(first.Data) != `{"delta":"Ha"}` {
		t.Errorf("data 1 = %q", first.Data)
	}

	second, err := r.Next()
	if err != nil {
		t.Fatalf("peristiwa 2: %v", err)
	}
	if second.Event != "chunk" {
		t.Errorf("event 2 = %q, mau \"chunk\"", second.Event)
	}

	// Beberapa baris data pada satu peristiwa digabung dengan newline, sesuai spesifikasi SSE.
	third, err := r.Next()
	if err != nil {
		t.Fatalf("peristiwa 3: %v", err)
	}
	if string(third.Data) != "baris satu\nbaris dua" {
		t.Errorf("data 3 = %q", third.Data)
	}

	done, err := r.Next()
	if err != nil {
		t.Fatalf("peristiwa 4: %v", err)
	}
	if !IsDoneMarker(done.Data) {
		t.Errorf("penanda akhir tidak dikenali: %q", done.Data)
	}

	if _, err := r.Next(); !errors.Is(err, io.EOF) {
		t.Errorf("setelah aliran habis: err = %v, mau io.EOF", err)
	}
}

// Aliran yang berakhir tanpa baris kosong penutup tetap harus menyerahkan peristiwa
// terakhirnya — pada banyak provider justru chunk itu yang membawa token usage.
func TestSSEReaderKeepsFinalEventWithoutTrailingBlankLine(t *testing.T) {
	r := NewSSEReader(strings.NewReader("data: {\"usage\":{\"total_tokens\":42}}\n"))

	ev, err := r.Next()
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if !strings.Contains(string(ev.Data), "total_tokens") {
		t.Errorf("chunk terakhir hilang: %q", ev.Data)
	}
	if _, err := r.Next(); !errors.Is(err, io.EOF) {
		t.Errorf("err = %v, mau io.EOF", err)
	}
}

func TestSSEReaderCRLF(t *testing.T) {
	r := NewSSEReader(strings.NewReader("data: halo\r\n\r\n"))
	ev, err := r.Next()
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if string(ev.Data) != "halo" {
		t.Errorf("data = %q, mau \"halo\"", ev.Data)
	}
}

func TestSSEReaderRejectsOversizedLine(t *testing.T) {
	r := NewSSEReader(strings.NewReader("data: " + strings.Repeat("a", MaxSSELineBytes+1024)))
	if _, err := r.Next(); err == nil {
		t.Error("baris raksasa seharusnya ditolak")
	}
}

func TestMessageText(t *testing.T) {
	simple := Message{Role: RoleUser, Content: "halo"}
	if simple.IsMultimodal() {
		t.Error("pesan sederhana dianggap multimodal")
	}
	if simple.Text() != "halo" {
		t.Errorf("Text() = %q", simple.Text())
	}

	multi := Message{Role: RoleUser, Parts: []ContentPart{
		{Type: PartTypeText, Text: "lihat ini"},
		{Type: PartTypeImageURL, ImageURL: &ImageURL{URL: "https://x.test/a.png"}},
		{Type: PartTypeText, Text: "dan ini"},
	}}
	if !multi.IsMultimodal() {
		t.Error("pesan multimodal tidak dikenali")
	}
	if multi.Text() != "lihat ini\ndan ini" {
		t.Errorf("Text() = %q, mau \"lihat ini\\ndan ini\"", multi.Text())
	}
	// URL gambar tidak boleh masuk teks yang diperiksa content filter sebagai teks.
	if strings.Contains(multi.Text(), "a.png") {
		t.Error("Text() menyertakan URL gambar")
	}
}

func TestNewHTTPClientTimeoutOnlyForNonStreaming(t *testing.T) {
	cfg := ClientConfig{BaseURL: "https://api.example.com", Timeout: 5 * time.Second}

	nonStream, err := NewHTTPClient(cfg, false)
	if err != nil {
		t.Fatalf("NewHTTPClient: %v", err)
	}
	if nonStream.Timeout != 5*time.Second {
		t.Errorf("timeout non-streaming = %v, mau 5s", nonStream.Timeout)
	}

	// http.Client.Timeout membatasi SELURUH umur respons termasuk pembacaan body, jadi
	// memasangnya pada streaming akan mematikan setiap stream pada detik yang sama.
	streaming, err := NewHTTPClient(cfg, true)
	if err != nil {
		t.Fatalf("NewHTTPClient streaming: %v", err)
	}
	if streaming.Timeout != 0 {
		t.Errorf("timeout streaming = %v, mau 0", streaming.Timeout)
	}

	// Pengalihan tidak diikuti.
	if streaming.CheckRedirect == nil {
		t.Error("CheckRedirect tidak dipasang")
	}
}

func TestNewHTTPClientRejectsBadProxy(t *testing.T) {
	_, err := NewHTTPClient(ClientConfig{
		BaseURL:  "https://api.example.com",
		ProxyURL: "gopher://user:sandi@proxy.internal:70",
	}, false)
	if err == nil {
		t.Fatal("skema proxy asing seharusnya ditolak")
	}
	if strings.Contains(err.Error(), "sandi") {
		t.Errorf("pesan error membocorkan kredensial proxy: %v", err)
	}
}
