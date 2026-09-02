package openai

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

// Chunk aliran yang dipakai beberapa test di bawah.
const (
	chunkRole   = `{"id":"chatcmpl-1","object":"chat.completion.chunk","created":1,"model":"gpt-4o-mini","choices":[{"index":0,"delta":{"role":"assistant","content":""},"finish_reason":null}]}`
	chunkHa     = `{"id":"chatcmpl-1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":"Ha"},"finish_reason":null}],"field_masa_depan":true}`
	chunkLo     = `{"id":"chatcmpl-1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":"lo","reasoning_content":"berpikir"},"finish_reason":null}]}`
	chunkFinish = `{"id":"chatcmpl-1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`
	chunkUsage  = `{"id":"chatcmpl-1","object":"chat.completion.chunk","choices":[],"usage":{"prompt_tokens":11,"completion_tokens":5,"total_tokens":16,"prompt_tokens_details":{"cached_tokens":8},"completion_tokens_details":{"reasoning_tokens":2}}}`
)

// sseHandler menulis setiap muatan sebagai satu peristiwa SSE dan langsung mem-flush-nya,
// supaya klien benar-benar membacanya bertahap seperti pada aliran sungguhan.
func sseHandler(payloads ...string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		rc := http.NewResponseController(w)
		for _, payload := range payloads {
			_, _ = io.WriteString(w, "data: "+payload+"\n\n")
			_ = rc.Flush()
		}
	}
}

// recvAll membaca aliran sampai selesai normal.
func recvAll(t *testing.T, s providers.Stream) []*providers.StreamEvent {
	t.Helper()
	var out []*providers.StreamEvent
	for i := 0; i < 100; i++ {
		ev, err := s.Recv()
		if errors.Is(err, io.EOF) {
			return out
		}
		if err != nil {
			t.Fatalf("Recv: %v", err)
		}
		out = append(out, ev)
	}
	t.Fatal("aliran tidak berakhir setelah 100 peristiwa")
	return nil
}

// openStream membuka aliran dan memastikan Close selalu terpanggil.
func openStream(t *testing.T, p *Provider, req *providers.ChatRequest) providers.Stream {
	t.Helper()
	s, err := p.ChatCompletionStream(context.Background(), req)
	if err != nil {
		t.Fatalf("ChatCompletionStream: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestStreamAssemblesDeltas(t *testing.T) {
	u := newUpstream(t, sseHandler(chunkRole, chunkHa, chunkLo, chunkFinish, chunkUsage, "[DONE]"))
	p := testProvider(t, u.url(), nil)

	s := openStream(t, p, chatRequest())
	events := recvAll(t, s)

	if len(events) != 5 {
		t.Fatalf("jumlah peristiwa = %d, mau 5", len(events))
	}

	var text, reasoning strings.Builder
	for _, ev := range events {
		text.WriteString(ev.Delta)
		reasoning.WriteString(ev.ReasoningDelta)
	}
	if text.String() != "Halo" {
		t.Errorf("teks tersusun = %q, mau %q", text.String(), "Halo")
	}
	if reasoning.String() != "berpikir" {
		t.Errorf("jejak penalaran = %q", reasoning.String())
	}

	if got := events[3].FinishReason; got != providers.FinishStop {
		t.Errorf("FinishReason = %q, mau stop", got)
	}
	// Raw adalah muatan SSE apa adanya, termasuk field yang belum dikenal gateway.
	if string(events[1].Raw) != chunkHa {
		t.Errorf("Raw = %s", events[1].Raw)
	}

	usage := events[4].Usage
	if usage == nil {
		t.Fatal("chunk usage tidak terbaca")
	}
	want := providers.Usage{InputTokens: 11, CachedInputTokens: 8, OutputTokens: 5, ReasoningTokens: 2, TotalTokens: 16}
	if *usage != want {
		t.Errorf("Usage = %+v, mau %+v", *usage, want)
	}
	// Peristiwa selain penutup tidak boleh mengarang usage.
	if events[0].Usage != nil {
		t.Errorf("chunk biasa memuat usage: %+v", events[0].Usage)
	}

	// Setelah [DONE], setiap Recv berikutnya tetap io.EOF.
	for i := 0; i < 2; i++ {
		if _, err := s.Recv(); !errors.Is(err, io.EOF) {
			t.Errorf("Recv setelah selesai = %v, mau io.EOF", err)
		}
	}

	// Permintaan streaming memakai Accept SSE dan stream:true.
	if got := u.last().header.Get("Accept"); got != "text/event-stream" {
		t.Errorf("Accept = %q", got)
	}
	body := u.lastPayload()
	if body.Stream == nil || !*body.Stream {
		t.Errorf("stream = %v, mau true", body.Stream)
	}
}

// Tool call datang terpotong beberapa chunk, dan index adalah satu-satunya cara menyusunnya
// kembali ke tool call yang benar.
func TestStreamAssemblesToolCallFragments(t *testing.T) {
	frames := []string{
		`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_a","type":"function","function":{"name":"get_weather","arguments":""}}]}}]}`,
		`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"kota\":"}}]}}]}`,
		`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"Jakarta\"}"}}]}}]}`,
		// Tool call kedua muncul sebagai satu-satunya potongan di chunk-nya. Kalau index
		// diambil dari posisi array alih-alih dari muatan, ia salah dianggap index 0 dan
		// argumennya menempel ke tool call pertama.
		`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":1,"id":"call_b","type":"function","function":{"name":"get_time","arguments":"{}"}}]}}]}`,
		`{"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
		"[DONE]",
	}
	u := newUpstream(t, sseHandler(frames...))
	p := testProvider(t, u.url(), nil)

	events := recvAll(t, openStream(t, p, chatRequest()))

	args := map[int]*strings.Builder{}
	names := map[int]string{}
	ids := map[int]string{}
	for _, ev := range events {
		for _, tc := range ev.ToolCalls {
			if args[tc.Index] == nil {
				args[tc.Index] = &strings.Builder{}
			}
			args[tc.Index].WriteString(tc.Function.Arguments)
			if tc.Function.Name != "" {
				names[tc.Index] = tc.Function.Name
			}
			if tc.ID != "" {
				ids[tc.Index] = tc.ID
			}
		}
	}

	if len(args) != 2 {
		t.Fatalf("jumlah tool call = %d, mau 2", len(args))
	}
	if got := args[0].String(); got != `{"kota":"Jakarta"}` {
		t.Errorf("arguments index 0 = %q", got)
	}
	if got := args[1].String(); got != `{}` {
		t.Errorf("arguments index 1 = %q", got)
	}
	if names[0] != "get_weather" || names[1] != "get_time" {
		t.Errorf("nama fungsi = %v", names)
	}
	if ids[0] != "call_a" || ids[1] != "call_b" {
		t.Errorf("id tool call = %v", ids)
	}
	if events[len(events)-1].FinishReason != providers.FinishToolCalls {
		t.Errorf("FinishReason = %q", events[len(events)-1].FinishReason)
	}
}

// Keep-alive dan peristiwa tanpa data bukan chunk dan tidak boleh diteruskan ke klien.
func TestStreamSkipsKeepAliveEvents(t *testing.T) {
	u := newUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		rc := http.NewResponseController(w)
		for _, block := range []string{
			": keep-alive\n\n",
			"data: " + chunkHa + "\n\n",
			"data: \n\n",
			"event: ping\ndata:  \n\n",
			"data: [DONE]\n\n",
		} {
			_, _ = io.WriteString(w, block)
			_ = rc.Flush()
		}
	})
	p := testProvider(t, u.url(), nil)

	events := recvAll(t, openStream(t, p, chatRequest()))
	if len(events) != 1 {
		t.Fatalf("jumlah peristiwa = %d, mau 1", len(events))
	}
	if events[0].Delta != "Ha" {
		t.Errorf("Delta = %q", events[0].Delta)
	}
}

// Aliran yang berakhir tanpa [DONE] tetap akhir yang normal: sebagian provider berdialek
// OpenAI menutup begitu saja, dan menganggapnya kegagalan menghasilkan error palsu pada
// setiap request yang sebenarnya lengkap.
func TestStreamEndsCleanlyWithoutDoneMarker(t *testing.T) {
	u := newUpstream(t, sseHandler(chunkHa, chunkFinish))
	p := testProvider(t, u.url(), func(c *Config) { c.Kind = providers.KindOpenAICompatible })

	events := recvAll(t, openStream(t, p, chatRequest()))
	if len(events) != 2 {
		t.Fatalf("jumlah peristiwa = %d, mau 2", len(events))
	}
}

// stream_options adalah satu-satunya cara mendapat token usage pada aliran OpenAI, tetapi
// juga field yang paling mungkin ditolak implementasi pihak ketiga. Keputusannya bergantung
// Kind, dan tabel ini yang menjaganya.
func TestStreamOptionsDependsOnKind(t *testing.T) {
	tests := []struct {
		kind      string
		wantUsage bool
	}{
		{providers.KindOpenAI, true},
		{providers.KindOpenAICompatible, false},
		{providers.KindCustom, false},
	}

	for _, tc := range tests {
		t.Run(tc.kind, func(t *testing.T) {
			u := newUpstream(t, sseHandler(chunkHa, "[DONE]"))
			p := testProvider(t, u.url(), func(c *Config) { c.Kind = tc.kind })

			recvAll(t, openStream(t, p, chatRequest()))

			body := u.lastBody()
			opts, ada := body["stream_options"]
			if !tc.wantUsage {
				if ada {
					t.Fatalf("stream_options terkirim ke kind %s: %v", tc.kind, opts)
				}
				return
			}
			asMap, ok := opts.(map[string]any)
			if !ok || asMap["include_usage"] != true {
				t.Fatalf("stream_options = %v, mau include_usage true", opts)
			}
		})
	}
}

// Pemanggil yang tahu upstream compatible-nya mendukung usage bisa menyalakannya lewat
// Extra — kunci itu memang tidak diisi gateway untuk Kind tersebut.
func TestStreamOptionsCanBeOptedInViaExtraOnCompatibleKind(t *testing.T) {
	u := newUpstream(t, sseHandler(chunkHa, "[DONE]"))
	p := testProvider(t, u.url(), func(c *Config) { c.Kind = providers.KindOpenAICompatible })

	req := chatRequest()
	req.Extra = map[string]json.RawMessage{"stream_options": json.RawMessage(`{"include_usage":true}`)}
	recvAll(t, openStream(t, p, req))

	opts, ok := u.lastBody()["stream_options"].(map[string]any)
	if !ok || opts["include_usage"] != true {
		t.Fatalf("stream_options = %v", u.lastBody()["stream_options"])
	}
}

// Pada Kind openai kunci itu diisi gateway, jadi Extra tidak bisa mematikannya — tanpa usage
// biaya request streaming tidak bisa dihitung.
func TestExtraCannotDisableStreamUsageOnOpenAIKind(t *testing.T) {
	u := newUpstream(t, sseHandler(chunkHa, "[DONE]"))
	p := testProvider(t, u.url(), nil)

	req := chatRequest()
	req.Extra = map[string]json.RawMessage{"stream_options": json.RawMessage(`{"include_usage":false}`)}
	recvAll(t, openStream(t, p, req))

	opts, ok := u.lastBody()["stream_options"].(map[string]any)
	if !ok || opts["include_usage"] != true {
		t.Fatalf("stream_options = %v, mau include_usage true", u.lastBody()["stream_options"])
	}
}

// Aliran yang terputus setelah sebagian jawaban terkirim TIDAK boleh diulang: aliran kedua
// akan tersambung di tengah aliran pertama dan klien menerima jawaban tidak koheren.
func TestStreamAbortedMidFlightBlocksRetry(t *testing.T) {
	u := newUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		rc := http.NewResponseController(w)
		_, _ = io.WriteString(w, "data: "+chunkRole+"\n\n")
		_ = rc.Flush()
		_, _ = io.WriteString(w, "data: "+chunkHa+"\n\n")
		_ = rc.Flush()
		// Memutus koneksi tanpa menutup body chunked: bentuk kegagalan yang sama dengan
		// upstream yang mati di tengah jawaban.
		panic(http.ErrAbortHandler)
	})
	p := testProvider(t, u.url(), nil)
	s := openStream(t, p, chatRequest())

	var got int
	var err error
	for i := 0; i < 10; i++ {
		if _, err = s.Recv(); err != nil {
			break
		}
		got++
	}
	if got == 0 {
		t.Fatal("tidak satu pun chunk terkirim sebelum aliran terputus")
	}
	if errors.Is(err, io.EOF) {
		t.Fatal("aliran terputus dilaporkan sebagai akhir normal")
	}

	e := providers.AsError(err)
	if e == nil {
		t.Fatalf("error bukan *providers.Error: %v", err)
	}
	if e.StreamedBytes <= 0 {
		t.Errorf("StreamedBytes = %d, mau lebih dari 0", e.StreamedBytes)
	}
	if e.Kind != providers.ErrKindStreamAborted {
		t.Errorf("Kind = %s, mau %s", e.Kind, providers.ErrKindStreamAborted)
	}
	if e.Retryable() {
		t.Error("Retryable() = true padahal sebagian jawaban sudah terkirim")
	}
	if e.Failoverable() {
		t.Error("Failoverable() = true padahal sebagian jawaban sudah terkirim")
	}
	// Kegagalan yang sama harus terus dilaporkan, bukan berubah menjadi io.EOF.
	if _, again := s.Recv(); providers.AsError(again) == nil {
		t.Errorf("Recv kedua = %v, mau kegagalan yang sama", again)
	}
}

// Chunk yang bukan JSON tidak boleh diteruskan ke klien: Raw dikirim apa adanya, jadi
// meneruskannya berarti menyisipkan sampah ke tengah aliran yang sedang dibaca pustaka klien.
func TestStreamRejectsMalformedChunk(t *testing.T) {
	u := newUpstream(t, sseHandler(`{bukan json`, "[DONE]"))
	p := testProvider(t, u.url(), nil)

	_, err := openStream(t, p, chatRequest()).Recv()
	e := providers.AsError(err)
	if e == nil {
		t.Fatalf("error bukan *providers.Error: %v", err)
	}
	// Belum ada byte yang terkirim, jadi masih boleh diulang maupun dialihkan.
	if e.Kind != providers.ErrKindServer {
		t.Errorf("Kind = %s, mau %s", e.Kind, providers.ErrKindServer)
	}
	if e.StreamedBytes != 0 {
		t.Errorf("StreamedBytes = %d, mau 0", e.StreamedBytes)
	}
	if !e.Failoverable() {
		t.Error("kegagalan sebelum ada byte terkirim seharusnya boleh dialihkan")
	}
}

// Chunk rusak SETELAH sebagian jawaban terkirim adalah aliran yang gagal di tengah.
func TestStreamMalformedChunkAfterPartialOutputIsAborted(t *testing.T) {
	u := newUpstream(t, sseHandler(chunkHa, `{rusak`, "[DONE]"))
	p := testProvider(t, u.url(), nil)
	s := openStream(t, p, chatRequest())

	if _, err := s.Recv(); err != nil {
		t.Fatalf("Recv pertama: %v", err)
	}
	_, err := s.Recv()
	e := providers.AsError(err)
	if e == nil {
		t.Fatalf("error bukan *providers.Error: %v", err)
	}
	if e.Kind != providers.ErrKindStreamAborted || e.StreamedBytes <= 0 {
		t.Errorf("Kind = %s, StreamedBytes = %d", e.Kind, e.StreamedBytes)
	}
}

// Status gagal pada jalur streaming diklasifikasikan sama seperti jalur biasa, dan tidak
// pernah menghasilkan Stream yang harus ditutup pemanggil.
func TestStreamErrorStatusReturnsClassifiedError(t *testing.T) {
	u := newUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "3")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `{"error":{"message":"terlalu cepat","code":"rate_limit_exceeded"}}`)
	})
	p := testProvider(t, u.url(), nil)

	s, err := p.ChatCompletionStream(context.Background(), chatRequest())
	if s != nil {
		t.Error("Stream dikembalikan bersama error")
	}
	e := providers.AsError(err)
	if e == nil {
		t.Fatalf("error bukan *providers.Error: %v", err)
	}
	if e.Kind != providers.ErrKindRateLimit {
		t.Errorf("Kind = %s", e.Kind)
	}
	if e.RetryAfter != 3*time.Second {
		t.Errorf("RetryAfter = %v", e.RetryAfter)
	}
	if e.StreamedBytes != 0 {
		t.Errorf("StreamedBytes = %d, mau 0", e.StreamedBytes)
	}
}

// Provider yang mengabaikan "stream":true dan menjawab satu objek JSON harus dikenali,
// bukan menghasilkan aliran kosong tanpa penjelasan.
func TestStreamRejectsNonSSEResponse(t *testing.T) {
	u := newUpstream(t, jsonHandler(200, chatResponseBody))
	p := testProvider(t, u.url(), func(c *Config) { c.Kind = providers.KindCustom })

	_, err := p.ChatCompletionStream(context.Background(), chatRequest())
	e := providers.AsError(err)
	if e == nil {
		t.Fatalf("error bukan *providers.Error: %v", err)
	}
	if e.Kind != providers.ErrKindServer {
		t.Errorf("Kind = %s, mau %s", e.Kind, providers.ErrKindServer)
	}
	if !e.Failoverable() {
		t.Error("belum ada byte terkirim, seharusnya boleh dialihkan ke provider lain")
	}
}

// Close wajib aman dipanggil berulang: pemanggil biasanya memakai defer sekaligus menutup
// lebih awal saat gagal di tengah, dan panic di jalur itu akan mematikan request.
func TestStreamCloseIsIdempotent(t *testing.T) {
	u := newUpstream(t, sseHandler(chunkHa, "[DONE]"))
	p := testProvider(t, u.url(), nil)

	s, err := p.ChatCompletionStream(context.Background(), chatRequest())
	if err != nil {
		t.Fatalf("ChatCompletionStream: %v", err)
	}
	if _, err := s.Recv(); err != nil {
		t.Fatalf("Recv: %v", err)
	}

	for i := 0; i < 3; i++ {
		if err := s.Close(); err != nil {
			t.Errorf("Close ke-%d = %v", i+1, err)
		}
	}

	// Recv setelah Close bukan akhir normal: pemanggil harus bisa membedakannya dari aliran
	// yang selesai, karena jawabannya tidak lengkap.
	_, err = s.Recv()
	e := providers.AsError(err)
	if e == nil {
		t.Fatalf("Recv setelah Close = %v, mau *providers.Error", err)
	}
	if e.Kind != providers.ErrKindCanceled {
		t.Errorf("Kind = %s, mau %s", e.Kind, providers.ErrKindCanceled)
	}
	if e.Retryable() || e.Failoverable() {
		t.Error("aliran yang ditutup pemanggil tidak boleh diulang maupun dialihkan")
	}
}

// Close harus benar-benar melepas koneksi upstream. Tanpa itu koneksi menggantung sampai
// timeout upstream — satu untuk setiap aliran yang dibatalkan, dan pada beban nyata itu
// menghabiskan kuota koneksi provider.
//
// Buktinya diambil dari sisi server: handler menahan respons tetap terbuka sampai context
// request-nya selesai, dan context itu hanya selesai bila klien benar-benar memutus.
func TestStreamCloseReleasesUpstreamConnection(t *testing.T) {
	released := make(chan struct{})
	u := newUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		rc := http.NewResponseController(w)
		_, _ = io.WriteString(w, "data: "+chunkHa+"\n\n")
		_ = rc.Flush()
		<-r.Context().Done()
		close(released)
	})
	p := testProvider(t, u.url(), nil)

	s, err := p.ChatCompletionStream(context.Background(), chatRequest())
	if err != nil {
		t.Fatalf("ChatCompletionStream: %v", err)
	}
	if _, err := s.Recv(); err != nil {
		t.Fatalf("Recv: %v", err)
	}

	select {
	case <-released:
		t.Fatal("koneksi terlepas sebelum Close dipanggil")
	case <-time.After(50 * time.Millisecond):
	}

	if err := s.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
	select {
	case <-released:
	case <-time.After(5 * time.Second):
		t.Fatal("koneksi upstream masih terbuka setelah Close")
	}
}

// Close tidak boleh menunggu Recv yang sedang menggantung.
//
// Ini alasan Close tidak mengambil lock pembaca: Recv memegangnya selama menunggu byte dari
// upstream, jadi Close yang menunggu lock yang sama akan macet justru pada satu-satunya
// keadaan ketika ia paling dibutuhkan — aliran yang tidak lagi mengirim apa pun.
func TestStreamCloseUnblocksHangingRecv(t *testing.T) {
	u := newUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		rc := http.NewResponseController(w)
		_, _ = io.WriteString(w, "data: "+chunkHa+"\n\n")
		_ = rc.Flush()
		<-r.Context().Done()
	})
	p := testProvider(t, u.url(), nil)

	s, err := p.ChatCompletionStream(context.Background(), chatRequest())
	if err != nil {
		t.Fatalf("ChatCompletionStream: %v", err)
	}
	if _, err := s.Recv(); err != nil {
		t.Fatalf("Recv: %v", err)
	}

	hanging := make(chan error, 1)
	go func() {
		_, err := s.Recv()
		hanging <- err
	}()

	// Memberi Recv kesempatan benar-benar menggantung di pembacaan socket.
	time.Sleep(100 * time.Millisecond)

	closed := make(chan error, 1)
	go func() { closed <- s.Close() }()
	select {
	case err := <-closed:
		if err != nil {
			t.Errorf("Close: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Close menggantung menunggu Recv")
	}

	select {
	case err := <-hanging:
		e := providers.AsError(err)
		if e == nil {
			t.Fatalf("Recv yang terputus = %v, mau *providers.Error", err)
		}
		if e.Kind != providers.ErrKindCanceled {
			t.Errorf("Kind = %s, mau %s", e.Kind, providers.ErrKindCanceled)
		}
		if e.StreamedBytes <= 0 {
			t.Errorf("StreamedBytes = %d, mau lebih dari 0", e.StreamedBytes)
		}
		if e.Retryable() {
			t.Error("Retryable() = true padahal sebagian jawaban sudah terkirim")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Recv tidak terbebas setelah Close")
	}
}

// Pembatalan context oleh klien di tengah aliran: bukan kegagalan provider, dan tidak boleh
// diulang karena kliennya sudah pergi.
func TestStreamContextCancellationMidFlight(t *testing.T) {
	u := newUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		rc := http.NewResponseController(w)
		_, _ = io.WriteString(w, "data: "+chunkHa+"\n\n")
		_ = rc.Flush()
		<-r.Context().Done()
	})
	p := testProvider(t, u.url(), nil)

	ctx, cancel := context.WithCancel(context.Background())
	s, err := p.ChatCompletionStream(ctx, chatRequest())
	if err != nil {
		cancel()
		t.Fatalf("ChatCompletionStream: %v", err)
	}
	defer func() {
		_ = s.Close()
		cancel()
	}()

	if _, err := s.Recv(); err != nil {
		t.Fatalf("Recv: %v", err)
	}
	cancel()

	_, err = s.Recv()
	e := providers.AsError(err)
	if e == nil {
		t.Fatalf("error bukan *providers.Error: %v", err)
	}
	if e.Kind != providers.ErrKindCanceled {
		t.Errorf("Kind = %s, mau %s", e.Kind, providers.ErrKindCanceled)
	}
	if e.Retryable() || e.Failoverable() {
		t.Error("permintaan yang dibatalkan klien tidak boleh diulang maupun dialihkan")
	}
}

// Potongan penalaran juga datang dengan dua nama field yang berbeda.
func TestStreamReadsReasoningDeltaFromEitherField(t *testing.T) {
	u := newUpstream(t, sseHandler(
		`{"choices":[{"index":0,"delta":{"reasoning":"satu "}}]}`,
		`{"choices":[{"index":0,"delta":{"reasoning_content":"dua"}}]}`,
		`{"choices":[{"index":0,"delta":{"reasoning":{"ringkasan":"diabaikan"}}}]}`,
		"[DONE]",
	))
	p := testProvider(t, u.url(), nil)

	var b strings.Builder
	for _, ev := range recvAll(t, openStream(t, p, chatRequest())) {
		b.WriteString(ev.ReasoningDelta)
	}
	if b.String() != "satu dua" {
		t.Errorf("jejak penalaran = %q, mau %q", b.String(), "satu dua")
	}
}
