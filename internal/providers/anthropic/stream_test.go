package anthropic

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/NexGen-X/Route-X/internal/providers"
)

// sseHandler mengirim potongan SSE satu per satu, masing-masing langsung di-flush supaya
// klien benar-benar membacanya sebagai aliran.
func sseHandler(chunks ...string) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		for _, c := range chunks {
			_, _ = io.WriteString(w, c)
			if flusher != nil {
				flusher.Flush()
			}
		}
	}
}

// drained adalah hasil pembacaan satu aliran sampai selesai.
type drained struct {
	text      string
	reasoning string
	calls     []providers.ToolCall
	finish    string
	usage     *providers.Usage
	raws      [][]byte
	err       error
}

func drain(st providers.Stream) drained {
	var out drained
	for {
		ev, err := st.Recv()
		if err != nil {
			out.err = err
			return out
		}
		out.text += ev.Delta
		out.reasoning += ev.ReasoningDelta
		out.calls = append(out.calls, ev.ToolCalls...)
		if ev.FinishReason != "" {
			out.finish = ev.FinishReason
		}
		if ev.Usage != nil {
			out.usage = ev.Usage
		}
		out.raws = append(out.raws, ev.Raw)
	}
}

func TestAliranMenanganiSeluruhJenisPeristiwa(t *testing.T) {
	p, log := newServer(t, sseHandler(
		": keep-alive sebelum peristiwa pertama\n\n",
		"event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_S\",\"model\":\"claude-sonnet-4-5-20250929\",\"usage\":{\"input_tokens\":100,\"cache_read_input_tokens\":20,\"cache_creation_input_tokens\":5,\"output_tokens\":1}}}\n\n",
		"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n",
		"event: ping\ndata: {\"type\":\"ping\"}\n\n",
		"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"Hal\"}}\n\n",
		"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"o!\"}}\n\n",
		"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"thinking_delta\",\"thinking\":\"pikir dulu\"}}\n\n",
		"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"signature_delta\",\"signature\":\"abc\"}}\n\n",
		"event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n",
		"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":1,\"content_block\":{\"type\":\"tool_use\",\"id\":\"toolu_S\",\"name\":\"cuaca\",\"input\":{}}}\n\n",
		"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":1,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"{\\\"kota\\\":\"}}\n\n",
		"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":1,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"\\\"Solo\\\"}\"}}\n\n",
		"event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":1}\n\n",
		"event: peristiwa_baru\ndata: {\"type\":\"peristiwa_baru\",\"catatan\":\"harus dilewati\"}\n\n",
		"event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"tool_use\"},\"usage\":{\"output_tokens\":50}}\n\n",
		"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n",
	))

	st, err := p.ChatCompletionStream(context.Background(), simpleRequest())
	if err != nil {
		t.Fatalf("ChatCompletionStream() error: %v", err)
	}
	defer st.Close()

	got := drain(st)
	if !errors.Is(got.err, io.EOF) {
		t.Fatalf("error akhir = %v, mau io.EOF", got.err)
	}
	if log.Header("Accept") != "text/event-stream" {
		t.Errorf("Accept = %q, mau text/event-stream", log.Header("Accept"))
	}
	if !decodeRequest(t, log.Body()).Stream {
		t.Error("body permintaan tidak menyalakan stream")
	}

	if got.text != "Halo!" {
		t.Errorf("teks tergabung = %q, mau %q", got.text, "Halo!")
	}
	if got.reasoning != "pikir dulu" {
		t.Errorf("penalaran = %q", got.reasoning)
	}
	if got.finish != providers.FinishToolCalls {
		t.Errorf("finish reason = %q, mau %q", got.finish, providers.FinishToolCalls)
	}

	if len(got.calls) != 3 {
		t.Fatalf("jumlah potongan tool call = %d, mau 3 (pembuka + dua potongan argumen): %+v", len(got.calls), got.calls)
	}
	if got.calls[0].ID != "toolu_S" || got.calls[0].Function.Name != "cuaca" {
		t.Errorf("potongan pembuka tool = %+v", got.calls[0])
	}
	// Block teks menempati index 0 di Anthropic, jadi tool_use di block index 1 tetap harus
	// menjadi tool_calls index 0 di dialek OpenAI.
	for i, c := range got.calls {
		if c.Index != 0 {
			t.Errorf("potongan tool[%d].Index = %d, mau 0", i, c.Index)
		}
	}
	if args := got.calls[1].Function.Arguments + got.calls[2].Function.Arguments; args != `{"kota":"Solo"}` {
		t.Errorf("argumen tergabung = %q", args)
	}

	// Usage datang terpisah: input di message_start, output di message_delta.
	if got.usage == nil {
		t.Fatal("usage tidak pernah dilaporkan")
	}
	if got.usage.InputTokens != 125 {
		t.Errorf("InputTokens = %d, mau 125 (100 + 20 cache read + 5 cache creation)", got.usage.InputTokens)
	}
	if got.usage.CachedInputTokens != 20 {
		t.Errorf("CachedInputTokens = %d, mau 20", got.usage.CachedInputTokens)
	}
	if got.usage.OutputTokens != 50 {
		t.Errorf("OutputTokens = %d, mau 50 (nilai kumulatif terakhir, bukan 1+50)", got.usage.OutputTokens)
	}
	if got.usage.TotalTokens != 175 {
		t.Errorf("TotalTokens = %d, mau 175", got.usage.TotalTokens)
	}

	// Peristiwa pembuka harus membawa peran, seperti aliran OpenAI.
	var first struct {
		Object  string `json:"object"`
		ID      string `json:"id"`
		Model   string `json:"model"`
		Choices []struct {
			Delta struct {
				Role    string  `json:"role"`
				Content *string `json:"content"`
			} `json:"delta"`
			FinishReason *string `json:"finish_reason"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(got.raws[0], &first); err != nil {
		t.Fatalf("potongan pertama bukan JSON: %v", err)
	}
	if first.Object != "chat.completion.chunk" {
		t.Errorf("object = %q, mau chat.completion.chunk", first.Object)
	}
	if first.ID != "msg_S" || first.Model != "claude-sonnet-4-5-20250929" {
		t.Errorf("identitas potongan = %s/%s, mau diambil dari message_start", first.ID, first.Model)
	}
	if len(first.Choices) != 1 || first.Choices[0].Delta.Role != "assistant" {
		t.Errorf("potongan pertama = %s", got.raws[0])
	}
	if first.Choices[0].FinishReason != nil {
		t.Error("finish_reason terisi di potongan pertama")
	}

	// Potongan penutup memuat usage dalam bentuk OpenAI.
	last := got.raws[len(got.raws)-1]
	if !strings.Contains(string(last), `"prompt_tokens":125`) ||
		!strings.Contains(string(last), `"total_tokens":175`) ||
		!strings.Contains(string(last), `"cached_tokens":20`) {
		t.Errorf("potongan penutup tidak memuat usage dialek OpenAI: %s", last)
	}
	if !strings.Contains(string(last), `"finish_reason":"tool_calls"`) {
		t.Errorf("potongan penutup tidak memuat finish_reason: %s", last)
	}

	// Tidak ada penanda [DONE] pada dialek Anthropic; itu tugas lapisan HTTP gateway.
	for _, raw := range got.raws {
		if providers.IsDoneMarker(raw) {
			t.Error("penanda [DONE] muncul di aliran hasil terjemahan")
		}
	}

	// Close wajib aman dipanggil berulang.
	if err := st.Close(); err != nil {
		t.Errorf("Close() pertama = %v", err)
	}
	if err := st.Close(); err != nil {
		t.Errorf("Close() kedua = %v", err)
	}
	if _, err := st.Recv(); !errors.Is(err, io.EOF) {
		t.Errorf("Recv() setelah selesai = %v, mau io.EOF", err)
	}
}

func TestAliranMengirimUsageSaatMessageStopTanpaMessageDelta(t *testing.T) {
	p, _ := newServer(t, sseHandler(
		"event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_U\",\"usage\":{\"input_tokens\":8,\"output_tokens\":2}}}\n\n",
		"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"hai\"}}\n\n",
		"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n",
	))
	st, err := p.ChatCompletionStream(context.Background(), simpleRequest())
	if err != nil {
		t.Fatalf("ChatCompletionStream() error: %v", err)
	}
	defer st.Close()

	got := drain(st)
	if !errors.Is(got.err, io.EOF) {
		t.Fatalf("error akhir = %v, mau io.EOF", got.err)
	}
	if got.usage == nil {
		t.Fatal("usage hilang saat message_delta tidak dikirim")
	}
	if got.usage.InputTokens != 8 || got.usage.OutputTokens != 2 || got.usage.TotalTokens != 10 {
		t.Errorf("usage = %+v", *got.usage)
	}
	// Potongan penutup tanpa choice adalah bentuk yang dipakai OpenAI untuk usage.
	last := string(got.raws[len(got.raws)-1])
	if !strings.Contains(last, `"choices":[]`) {
		t.Errorf("potongan usage seharusnya tanpa choice: %s", last)
	}
}

func TestAliranTanpaUsageBerakhirBersih(t *testing.T) {
	p, _ := newServer(t, sseHandler(
		"event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_N\"}}\n\n",
		"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n",
	))
	st, err := p.ChatCompletionStream(context.Background(), simpleRequest())
	if err != nil {
		t.Fatalf("ChatCompletionStream() error: %v", err)
	}
	defer st.Close()

	got := drain(st)
	if !errors.Is(got.err, io.EOF) {
		t.Fatalf("error akhir = %v, mau io.EOF", got.err)
	}
	if got.usage != nil {
		t.Errorf("usage = %+v, mau nil", *got.usage)
	}
}

// Aliran yang terputus setelah sebagian data terkirim tidak boleh diulang: aliran kedua
// akan tersambung di tengah aliran pertama.
func TestAliranTerputusDiTengah(t *testing.T) {
	p, _ := newServer(t, sseHandler(
		"event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_P\",\"usage\":{\"input_tokens\":5}}}\n\n",
		"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"separuh\"}}\n\n",
		// Tidak ada message_stop: koneksi berakhir di tengah.
	))
	st, err := p.ChatCompletionStream(context.Background(), simpleRequest())
	if err != nil {
		t.Fatalf("ChatCompletionStream() error: %v", err)
	}
	defer st.Close()

	got := drain(st)
	e := providers.AsError(got.err)
	if e == nil {
		t.Fatalf("error = %v, mau *providers.Error", got.err)
	}
	if e.Kind != providers.ErrKindStreamAborted {
		t.Errorf("Kind = %q, mau %q", e.Kind, providers.ErrKindStreamAborted)
	}
	if e.StreamedBytes <= 0 {
		t.Error("StreamedBytes = 0; sebagian aliran sudah terkirim ke klien")
	}
	if e.Retryable() {
		t.Error("Retryable() = true; aliran yang separuh terkirim tidak boleh diulang")
	}
	if e.Failoverable() {
		t.Error("Failoverable() = true; aliran yang separuh terkirim tidak boleh dialihkan")
	}
	if got.text != "separuh" {
		t.Errorf("teks sebelum terputus = %q", got.text)
	}
}

func TestAliranKosongYangLangsungTerputusMasihBolehDiulang(t *testing.T) {
	p, _ := newServer(t, sseHandler())
	st, err := p.ChatCompletionStream(context.Background(), simpleRequest())
	if err != nil {
		t.Fatalf("ChatCompletionStream() error: %v", err)
	}
	defer st.Close()

	got := drain(st)
	e := providers.AsError(got.err)
	if e == nil {
		t.Fatalf("error = %v, mau *providers.Error", got.err)
	}
	if e.StreamedBytes != 0 {
		t.Errorf("StreamedBytes = %d, mau 0", e.StreamedBytes)
	}
	if !e.Retryable() {
		t.Error("Retryable() = false; belum ada byte yang terkirim ke klien")
	}
}

func TestAliranPeristiwaError(t *testing.T) {
	p, _ := newServer(t, sseHandler(
		"event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_E\",\"usage\":{\"input_tokens\":5}}}\n\n",
		"event: error\ndata: {\"type\":\"error\",\"error\":{\"type\":\"overloaded_error\",\"message\":\"sedang penuh\"}}\n\n",
	))
	st, err := p.ChatCompletionStream(context.Background(), simpleRequest())
	if err != nil {
		t.Fatalf("ChatCompletionStream() error: %v", err)
	}
	defer st.Close()

	got := drain(st)
	e := providers.AsError(got.err)
	if e == nil {
		t.Fatalf("error = %v, mau *providers.Error", got.err)
	}
	if e.Kind != providers.ErrKindOverloaded {
		t.Errorf("Kind = %q, mau %q", e.Kind, providers.ErrKindOverloaded)
	}
	if e.UpstreamCode != "overloaded_error" {
		t.Errorf("UpstreamCode = %q", e.UpstreamCode)
	}
	if e.Message != "sedang penuh" {
		t.Errorf("Message = %q", e.Message)
	}
	if e.Retryable() {
		t.Error("Retryable() = true padahal sebagian aliran sudah terkirim")
	}
}

func TestAliranPeristiwaErrorTanpaDetail(t *testing.T) {
	p, _ := newServer(t, sseHandler("event: error\ndata: {\"type\":\"error\"}\n\n"))
	st, err := p.ChatCompletionStream(context.Background(), simpleRequest())
	if err != nil {
		t.Fatalf("ChatCompletionStream() error: %v", err)
	}
	defer st.Close()

	e := providers.AsError(drain(st).err)
	if e == nil {
		t.Fatal("mau *providers.Error")
	}
	if e.Kind != providers.ErrKindServer || e.Message == "" {
		t.Errorf("error = %+v", e)
	}
}

func TestPemetaanJenisErrorAnthropic(t *testing.T) {
	tests := map[string]providers.ErrorKind{
		"overloaded_error":      providers.ErrKindOverloaded,
		"rate_limit_error":      providers.ErrKindRateLimit,
		"authentication_error":  providers.ErrKindAuth,
		"permission_error":      providers.ErrKindPermission,
		"not_found_error":       providers.ErrKindModelNotFound,
		"invalid_request_error": providers.ErrKindInvalidRequest,
		"timeout_error":         providers.ErrKindTimeout,
		"api_error":             providers.ErrKindServer,
		"":                      providers.ErrKindServer,
	}
	for upstream, mau := range tests {
		if got := kindForUpstreamType(upstream); got != mau {
			t.Errorf("kindForUpstreamType(%q) = %q, mau %q", upstream, got, mau)
		}
	}
}

func TestAliranPeristiwaRusak(t *testing.T) {
	p, _ := newServer(t, sseHandler("event: message_start\ndata: {bukan json}\n\n"))
	st, err := p.ChatCompletionStream(context.Background(), simpleRequest())
	if err != nil {
		t.Fatalf("ChatCompletionStream() error: %v", err)
	}
	defer st.Close()

	e := providers.AsError(drain(st).err)
	if e == nil {
		t.Fatal("mau *providers.Error")
	}
	if e.Kind != providers.ErrKindServer {
		t.Errorf("Kind = %q, mau %q (belum ada byte yang terkirim)", e.Kind, providers.ErrKindServer)
	}
}

func TestAliranGagalSebelumDimulai(t *testing.T) {
	p, _ := newServer(t, jsonHandler(http.StatusTooManyRequests,
		`{"error":{"type":"rate_limit_error","message":"tunggu dulu"}}`))

	st, err := p.ChatCompletionStream(context.Background(), simpleRequest())
	if st != nil {
		t.Error("aliran tidak nil padahal permintaan gagal")
	}
	e := providers.AsError(err)
	if e == nil {
		t.Fatalf("error = %v, mau *providers.Error", err)
	}
	if e.Kind != providers.ErrKindRateLimit {
		t.Errorf("Kind = %q, mau %q", e.Kind, providers.ErrKindRateLimit)
	}
	// Belum ada apa pun yang terkirim ke klien, jadi permintaan ini masih boleh diulang.
	if e.StreamedBytes != 0 || !e.Retryable() {
		t.Errorf("StreamedBytes = %d, Retryable = %v", e.StreamedBytes, e.Retryable())
	}
}

func TestAliranPermintaanCacatTidakMenyentuhJaringan(t *testing.T) {
	p, log := newServer(t, sseHandler())
	_, err := p.ChatCompletionStream(context.Background(), &providers.ChatRequest{Model: ""})
	if providers.AsError(err) == nil {
		t.Fatalf("error = %v, mau *providers.Error", err)
	}
	if log.Path() != "" {
		t.Errorf("permintaan cacat tetap dikirim ke %q", log.Path())
	}
}

func TestAliranDibatalkanDiTengah(t *testing.T) {
	mulai := make(chan struct{})
	p, _ := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		_, _ = io.WriteString(w, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_C\"}}\n\n")
		if flusher != nil {
			flusher.Flush()
		}
		close(mulai)
		// Ditahan sampai klien membatalkan: itulah yang menghasilkan pemutusan di tengah.
		<-r.Context().Done()
	})

	ctx, cancel := context.WithCancel(context.Background())
	st, err := p.ChatCompletionStream(ctx, simpleRequest())
	if err != nil {
		cancel()
		t.Fatalf("ChatCompletionStream() error: %v", err)
	}
	defer st.Close()

	if _, err := st.Recv(); err != nil {
		cancel()
		t.Fatalf("Recv() pertama = %v", err)
	}
	<-mulai
	cancel()

	_, err = st.Recv()
	e := providers.AsError(err)
	if e == nil {
		t.Fatalf("error = %v, mau *providers.Error", err)
	}
	if e.Kind != providers.ErrKindCanceled {
		t.Errorf("Kind = %q, mau %q", e.Kind, providers.ErrKindCanceled)
	}
	if e.Retryable() || e.Failoverable() {
		t.Error("permintaan yang dibatalkan klien tidak boleh diulang atau dialihkan")
	}
}

// Peristiwa yang bentuknya sah tetapi tidak membawa apa pun untuk klien harus dilewati,
// bukan diteruskan sebagai potongan kosong — klien yang menerima potongan kosong akan
// menampilkan kursor berkedip tanpa teks.
func TestAliranMelewatiPeristiwaTanpaIsi(t *testing.T) {
	p, _ := newServer(t, sseHandler(
		"event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_K\"}}\n\n",
		// content_block_start tanpa content_block, dan dengan jenis block yang tidak dikenal.
		"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0}\n\n",
		"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"jenis_baru\"}}\n\n",
		// content_block_start berisi teks awal: sah, dan harus diteruskan.
		"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"awal \"}}\n\n",
		// Delta kosong pada setiap jenis: tidak ada yang perlu diteruskan.
		"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0}\n\n",
		"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"\"}}\n\n",
		"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"thinking_delta\",\"thinking\":\"\"}}\n\n",
		"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"\"}}\n\n",
		"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"isi\"}}\n\n",
		// Baris data tanpa isi sama sekali.
		"data: \n\n",
		// message_delta tanpa delta: hanya membawa usage.
		"event: message_delta\ndata: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":4}}\n\n",
		"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n",
	))
	st, err := p.ChatCompletionStream(context.Background(), simpleRequest())
	if err != nil {
		t.Fatalf("ChatCompletionStream() error: %v", err)
	}
	defer st.Close()

	got := drain(st)
	if !errors.Is(got.err, io.EOF) {
		t.Fatalf("error akhir = %v, mau io.EOF", got.err)
	}
	if got.text != "awal isi" {
		t.Errorf("teks = %q, mau %q", got.text, "awal isi")
	}
	if got.finish != "" {
		t.Errorf("finish reason = %q, mau kosong", got.finish)
	}
	if got.usage == nil || got.usage.OutputTokens != 4 {
		t.Errorf("usage = %+v", got.usage)
	}
	// Tiga potongan: peran, "awal ", "isi", lalu penutup usage.
	if len(got.raws) != 4 {
		t.Errorf("jumlah potongan = %d, mau 4: %s", len(got.raws), got.raws)
	}
}

func TestAliranMelewatiBatasWaktuContext(t *testing.T) {
	p, _ := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		_, _ = io.WriteString(w, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_T\"}}\n\n")
		if flusher != nil {
			flusher.Flush()
		}
		<-r.Context().Done()
	})

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	st, err := p.ChatCompletionStream(ctx, simpleRequest())
	if err != nil {
		t.Fatalf("ChatCompletionStream() error: %v", err)
	}
	defer st.Close()

	e := providers.AsError(drain(st).err)
	if e == nil {
		t.Fatal("mau *providers.Error")
	}
	if e.Kind != providers.ErrKindTimeout {
		t.Errorf("Kind = %q, mau %q", e.Kind, providers.ErrKindTimeout)
	}
	if e.Retryable() {
		t.Error("Retryable() = true padahal sebagian aliran sudah terkirim")
	}
}
