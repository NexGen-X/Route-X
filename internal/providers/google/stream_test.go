package google

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
	text       string
	reasoning  string
	calls      []providers.ToolCall
	finish     string
	usage      *providers.Usage
	usageCount int
	raws       [][]byte
	err        error
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
			out.usageCount++
		}
		out.raws = append(out.raws, ev.Raw)
	}
}

// Setiap peristiwa alt=sse memuat objek respons UTUH, bukan delta bergaya OpenAI.
func TestAliranMenerjemahkanObjekResponsUtuhPerPeristiwa(t *testing.T) {
	p, log := newServer(t, sseHandler(
		`data: {"candidates":[{"content":{"role":"model","parts":[{"text":"pikir dulu","thought":true}]},"index":0}],"modelVersion":"gemini-2.5-flash-001","responseId":"resp-S","usageMetadata":{"promptTokenCount":100,"cachedContentTokenCount":20}}`+"\n\n",
		`data: {"candidates":[{"content":{"role":"model","parts":[{"text":"Hal"}]},"index":0}]}`+"\n\n",
		": keep-alive\n\n",
		`data: {"candidates":[{"content":{"role":"model","parts":[{"text":"o!"}]},"index":0}]}`+"\n\n",
		`data: {"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"name":"cuaca","args":{"kota":"Solo"}}}]},"index":0}]}`+"\n\n",
		`data: {"candidates":[{"content":{"role":"model","parts":[]},"finishReason":"STOP","index":0}],"usageMetadata":{"promptTokenCount":100,"candidatesTokenCount":50,"cachedContentTokenCount":20,"thoughtsTokenCount":8,"totalTokenCount":158}}`+"\n\n",
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

	if log.Path() != "/v1beta/models/gemini-2.5-flash:streamGenerateContent" {
		t.Errorf("path = %q", log.Path())
	}
	// Tanpa alt=sse, Gemini mengirim array JSON raksasa, bukan SSE.
	if log.Query() != "alt=sse" {
		t.Errorf("query = %q, mau alt=sse", log.Query())
	}
	if log.Header("Accept") != "text/event-stream" {
		t.Errorf("Accept = %q", log.Header("Accept"))
	}
	if strings.Contains(log.URI(), testKey) {
		t.Errorf("kredensial muncul di URL aliran: %q", log.URI())
	}

	if got.text != "Halo!" {
		t.Errorf("teks tergabung = %q", got.text)
	}
	if got.reasoning != "pikir dulu" {
		t.Errorf("penalaran = %q", got.reasoning)
	}
	if got.finish != providers.FinishToolCalls {
		t.Errorf("finish reason = %q, mau tool_calls (Gemini menjawab STOP)", got.finish)
	}
	if len(got.calls) != 1 {
		t.Fatalf("jumlah tool call = %d, mau 1: %+v", len(got.calls), got.calls)
	}
	// Panggilan tool Gemini datang utuh dalam satu peristiwa, tidak dipecah.
	if got.calls[0].ID != "call_0_cuaca" || got.calls[0].Index != 0 ||
		got.calls[0].Function.Arguments != `{"kota":"Solo"}` {
		t.Errorf("tool call = %+v", got.calls[0])
	}

	if got.usage == nil {
		t.Fatal("usage tidak pernah dilaporkan")
	}
	if got.usage.InputTokens != 100 || got.usage.CachedInputTokens != 20 {
		t.Errorf("usage input = %+v", *got.usage)
	}
	if got.usage.OutputTokens != 58 {
		t.Errorf("OutputTokens = %d, mau 58 (50 + 8 penalaran)", got.usage.OutputTokens)
	}
	if got.usage.ReasoningTokens != 8 || got.usage.TotalTokens != 158 {
		t.Errorf("usage = %+v", *got.usage)
	}
	// Usage kumulatif hanya boleh dilaporkan SEKALI; klien yang menjumlahkan sendiri akan
	// menggandakan tagihan bila setiap peristiwa membawanya.
	if got.usageCount != 1 {
		t.Errorf("usage dilaporkan %d kali, mau 1", got.usageCount)
	}

	var first struct {
		Object  string `json:"object"`
		ID      string `json:"id"`
		Model   string `json:"model"`
		Choices []struct {
			Delta struct {
				Role             string `json:"role"`
				ReasoningContent string `json:"reasoning_content"`
			} `json:"delta"`
			FinishReason *string `json:"finish_reason"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(got.raws[0], &first); err != nil {
		t.Fatalf("potongan pertama bukan JSON: %v", err)
	}
	if first.Object != "chat.completion.chunk" {
		t.Errorf("object = %q", first.Object)
	}
	if first.ID != "resp-S" || first.Model != "gemini-2.5-flash-001" {
		t.Errorf("identitas potongan = %s/%s", first.ID, first.Model)
	}
	if first.Choices[0].Delta.Role != "assistant" {
		t.Errorf("potongan pertama tanpa peran: %s", got.raws[0])
	}
	if first.Choices[0].Delta.ReasoningContent != "pikir dulu" {
		t.Errorf("reasoning_content = %q", first.Choices[0].Delta.ReasoningContent)
	}
	if first.Choices[0].FinishReason != nil {
		t.Error("finish_reason terisi di potongan pertama")
	}

	last := string(got.raws[len(got.raws)-1])
	if !strings.Contains(last, `"finish_reason":"tool_calls"`) ||
		!strings.Contains(last, `"total_tokens":158`) ||
		!strings.Contains(last, `"reasoning_tokens":8`) {
		t.Errorf("potongan penutup = %s", last)
	}

	for _, raw := range got.raws {
		if providers.IsDoneMarker(raw) {
			t.Error("penanda [DONE] muncul di aliran hasil terjemahan")
		}
	}

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

// Usage yang baru datang setelah peristiwa berisi finishReason tetap harus sampai.
func TestAliranMengirimUsageYangTertinggalDiAkhir(t *testing.T) {
	p, _ := newServer(t, sseHandler(
		`data: {"candidates":[{"content":{"role":"model","parts":[{"text":"hai"}]},"index":0}]}`+"\n\n",
		`data: {"candidates":[{"content":{"role":"model","parts":[]},"finishReason":"STOP","index":0}]}`+"\n\n",
		`data: {"usageMetadata":{"promptTokenCount":4,"candidatesTokenCount":2,"totalTokenCount":6}}`+"\n\n",
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
		t.Fatal("usage hilang")
	}
	if got.usage.InputTokens != 4 || got.usage.TotalTokens != 6 {
		t.Errorf("usage = %+v", *got.usage)
	}
	last := string(got.raws[len(got.raws)-1])
	if !strings.Contains(last, `"choices":[]`) {
		t.Errorf("potongan usage seharusnya tanpa choice: %s", last)
	}
}

func TestAliranTanpaUsageBerakhirBersih(t *testing.T) {
	p, _ := newServer(t, sseHandler(
		`data: {"candidates":[{"content":{"role":"model","parts":[{"text":"hai"}]},"finishReason":"STOP","index":0}]}`+"\n\n",
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
	if got.text != "hai" || got.finish != providers.FinishStop {
		t.Errorf("hasil = %q/%q", got.text, got.finish)
	}
}

// Aliran Gemini tidak punya penanda penutup; berakhir tanpa finishReason berarti terputus.
func TestAliranTerputusDiTengah(t *testing.T) {
	p, _ := newServer(t, sseHandler(
		`data: {"candidates":[{"content":{"role":"model","parts":[{"text":"separuh"}]},"index":0}],"usageMetadata":{"promptTokenCount":5}}`+"\n\n",
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
	if e.Retryable() || e.Failoverable() {
		t.Error("aliran yang separuh terkirim tidak boleh diulang atau dialihkan")
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

	e := providers.AsError(drain(st).err)
	if e == nil {
		t.Fatalf("mau *providers.Error")
	}
	if e.StreamedBytes != 0 || !e.Retryable() {
		t.Errorf("StreamedBytes = %d, Retryable = %v", e.StreamedBytes, e.Retryable())
	}
}

func TestAliranErrorDiTengah(t *testing.T) {
	p, _ := newServer(t, sseHandler(
		`data: {"candidates":[{"content":{"role":"model","parts":[{"text":"mulai"}]},"index":0}]}`+"\n\n",
		`data: {"error":{"code":503,"message":"model overloaded","status":"UNAVAILABLE"}}`+"\n\n",
	))
	st, err := p.ChatCompletionStream(context.Background(), simpleRequest())
	if err != nil {
		t.Fatalf("ChatCompletionStream() error: %v", err)
	}
	defer st.Close()

	e := providers.AsError(drain(st).err)
	if e == nil {
		t.Fatal("mau *providers.Error")
	}
	if e.Kind != providers.ErrKindOverloaded {
		t.Errorf("Kind = %q, mau %q", e.Kind, providers.ErrKindOverloaded)
	}
	if e.StreamedBytes <= 0 {
		t.Error("StreamedBytes = 0 padahal sebagian aliran sudah terkirim")
	}
	if e.Retryable() {
		t.Error("Retryable() = true padahal sebagian aliran sudah terkirim")
	}
}

func TestAliranBlockReasonDiTengah(t *testing.T) {
	p, _ := newServer(t, sseHandler(
		`data: {"promptFeedback":{"blockReason":"PROHIBITED_CONTENT"}}`+"\n\n",
	))
	st, err := p.ChatCompletionStream(context.Background(), simpleRequest())
	if err != nil {
		t.Fatalf("ChatCompletionStream() error: %v", err)
	}
	defer st.Close()

	e := providers.AsError(drain(st).err)
	if e == nil {
		t.Fatal("mau *providers.Error")
	}
	if e.Kind != providers.ErrKindContentFilter {
		t.Errorf("Kind = %q, mau %q", e.Kind, providers.ErrKindContentFilter)
	}
	if e.Failoverable() {
		t.Error("Failoverable() = true; content_filter sengaja tidak dialihkan")
	}
}

func TestAliranPeristiwaRusak(t *testing.T) {
	p, _ := newServer(t, sseHandler("data: {bukan json}\n\n"))
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

func TestAliranMelewatiPeristiwaTanpaIsi(t *testing.T) {
	p, _ := newServer(t, sseHandler(
		"data: \n\n",
		`data: {"candidates":[]}`+"\n\n",
		`data: {"candidates":[{"content":{"role":"model","parts":[]},"index":0}]}`+"\n\n",
		`data: {"candidates":[{"content":{"role":"model","parts":[{"text":""}]},"index":0}]}`+"\n\n",
		`data: {"candidates":[{"content":{"role":"model","parts":[{"text":"isi"}]},"finishReason":"STOP","index":0}]}`+"\n\n",
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
	if got.text != "isi" {
		t.Errorf("teks = %q", got.text)
	}
	if len(got.raws) != 1 {
		t.Errorf("jumlah potongan = %d, mau 1: %s", len(got.raws), got.raws)
	}
}

func TestAliranGagalSebelumDimulai(t *testing.T) {
	p, _ := newServer(t, jsonHandler(http.StatusTooManyRequests,
		`{"error":{"code":429,"message":"tunggu dulu","status":"RESOURCE_EXHAUSTED"}}`))

	st, err := p.ChatCompletionStream(context.Background(), simpleRequest())
	if st != nil {
		t.Error("aliran tidak nil padahal permintaan gagal")
	}
	e := providers.AsError(err)
	if e == nil {
		t.Fatalf("error = %v, mau *providers.Error", err)
	}
	if e.Kind != providers.ErrKindRateLimit {
		t.Errorf("Kind = %q", e.Kind)
	}
	if e.StreamedBytes != 0 || !e.Retryable() {
		t.Errorf("StreamedBytes = %d, Retryable = %v", e.StreamedBytes, e.Retryable())
	}
}

func TestAliranPermintaanCacatTidakMenyentuhJaringan(t *testing.T) {
	p, log := newServer(t, sseHandler())

	// Percakapan kosong: gagal saat menyusun body.
	if _, err := p.ChatCompletionStream(context.Background(), &providers.ChatRequest{Model: "gemini-2.5-flash"}); providers.AsError(err) == nil {
		t.Fatal("mau *providers.Error")
	}
	// Nama model cacat: gagal saat menyusun URL.
	if _, err := p.ChatCompletionStream(context.Background(), &providers.ChatRequest{
		Model:    "gemini:x",
		Messages: []providers.Message{{Role: providers.RoleUser, Content: "halo"}},
	}); providers.AsError(err) == nil {
		t.Fatal("mau *providers.Error")
	}
	if log.count() != 0 {
		t.Error("permintaan cacat tetap dikirim ke upstream")
	}
}

func TestAliranDibatalkanDiTengah(t *testing.T) {
	mulai := make(chan struct{})
	p, _ := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		_, _ = io.WriteString(w, `data: {"candidates":[{"content":{"role":"model","parts":[{"text":"mulai"}]},"index":0}]}`+"\n\n")
		if flusher != nil {
			flusher.Flush()
		}
		close(mulai)
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

	e := providers.AsError(drain(st).err)
	if e == nil {
		t.Fatal("mau *providers.Error")
	}
	if e.Kind != providers.ErrKindCanceled {
		t.Errorf("Kind = %q, mau %q", e.Kind, providers.ErrKindCanceled)
	}
	if e.Retryable() || e.Failoverable() {
		t.Error("permintaan yang dibatalkan klien tidak boleh diulang atau dialihkan")
	}
}

func TestAliranMelewatiBatasWaktuContext(t *testing.T) {
	p, _ := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		_, _ = io.WriteString(w, `data: {"candidates":[{"content":{"role":"model","parts":[{"text":"mulai"}]},"index":0}]}`+"\n\n")
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
