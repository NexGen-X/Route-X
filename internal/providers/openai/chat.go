package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/NexGen-X/Route-X/internal/providers"
)

// wireMessage adalah satu pesan dalam body permintaan.
//
// Content bertipe any karena dialek OpenAI menerima dua bentuk: string untuk pesan teks
// biasa dan array bagian untuk multimodal. Bentuknya dipilih dari Message.IsMultimodal(),
// bukan ditebak dari isi string.
type wireMessage struct {
	Role       string         `json:"role"`
	Content    any            `json:"content,omitempty"`
	Name       string         `json:"name,omitempty"`
	ToolCalls  []wireToolCall `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
}

// wireToolCall dipakai dua arah: menguraikan tool call dari respons dan potongannya dari
// aliran, sekaligus menyusunnya kembali saat riwayat percakapan dikirim ulang.
//
// Index berupa pointer supaya bisa dibedakan "tidak dikirim" dari "0": pada aliran, index
// itu satu-satunya cara menyusun ulang potongan argumen ke tool call yang benar, sedangkan
// pada permintaan keluar field itu tidak boleh ikut sama sekali.
type wireToolCall struct {
	Index    *int   `json:"index,omitempty"`
	ID       string `json:"id,omitempty"`
	Type     string `json:"type,omitempty"`
	Function struct {
		Name string `json:"name,omitempty"`
		// arguments selalu ditulis, termasuk saat kosong: nilainya string JSON dan
		// sebagian implementasi menolak tool call tanpa field ini.
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// toCanonical menerjemahkan tool call ke bentuk kanonik. fallbackIndex dipakai bila
// upstream tidak mengirim index, yaitu pada respons non-streaming.
func (w wireToolCall) toCanonical(fallbackIndex int) providers.ToolCall {
	idx := fallbackIndex
	if w.Index != nil {
		idx = *w.Index
	}
	return providers.ToolCall{
		Index: idx,
		ID:    w.ID,
		Type:  w.Type,
		Function: providers.FunctionCall{
			Name:      w.Function.Name,
			Arguments: w.Function.Arguments,
		},
	}
}

// toolCallToWire menyusun tool call untuk dikirim kembali ke upstream.
func toolCallToWire(tc providers.ToolCall) wireToolCall {
	out := wireToolCall{ID: tc.ID, Type: tc.Type}
	if out.Type == "" {
		// Satu-satunya jenis yang ada di dialek OpenAI. Mengirim type kosong hanya
		// menghasilkan 400 yang membingungkan pengguna.
		out.Type = "function"
	}
	out.Function.Name = tc.Function.Name
	out.Function.Arguments = tc.Function.Arguments
	return out
}

// chatResponse adalah respons completion non-streaming.
//
// Hanya field yang dipakai gateway yang diurai; sisanya tetap utuh di ChatResponse.Raw.
type chatResponse struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Created int64  `json:"created"`
	Choices []struct {
		Index   int `json:"index"`
		Message struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
			// Dua nama untuk hal yang sama: DeepSeek memakai reasoning_content,
			// beberapa agregator memakai reasoning.
			ReasoningContent string          `json:"reasoning_content"`
			Reasoning        json.RawMessage `json:"reasoning"`
			ToolCalls        []wireToolCall  `json:"tool_calls"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage *wireUsage `json:"usage"`
}

// chatChunk adalah satu chunk pada respons streaming.
type chatChunk struct {
	Choices []struct {
		Index int `json:"index"`
		Delta struct {
			Role             string          `json:"role"`
			Content          string          `json:"content"`
			ReasoningContent string          `json:"reasoning_content"`
			Reasoning        json.RawMessage `json:"reasoning"`
			ToolCalls        []wireToolCall  `json:"tool_calls"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage *wireUsage `json:"usage"`
}

// ChatCompletion menjalankan satu completion non-streaming.
func (p *Provider) ChatCompletion(ctx context.Context, req *providers.ChatRequest) (*providers.ChatResponse, error) {
	payload, err := p.chatPayload(req, false)
	if err != nil {
		return nil, err
	}

	resp, doErr := p.do(ctx, http.MethodPost, pathChatCompletions, payload, false)
	if doErr != nil {
		return nil, doErr
	}
	defer resp.Body.Close()

	raw, readErr := readBody(p.name, resp.Body)
	if readErr != nil {
		return nil, readErr
	}

	var wire chatResponse
	if uerr := json.Unmarshal(raw, &wire); uerr != nil {
		// Pesan json memuat potongan body upstream, jadi tidak diteruskan.
		return nil, providers.Newf(providers.ErrKindServer, p.name,
			"respons upstream bukan JSON chat completion yang dikenali")
	}
	if len(wire.Choices) == 0 {
		return nil, p.errorFromSuccessBody(raw, resp.StatusCode,
			"respons upstream tidak memuat satu pun choice")
	}

	out := &providers.ChatResponse{
		ID:      wire.ID,
		Model:   wire.Model,
		Created: wire.Created,
		// Raw adalah body upstream apa adanya — inilah yang diteruskan ke klien. Menyusun
		// ulang JSON dari struct di atas akan menghapus setiap field yang belum dikenal
		// gateway, padahal klien mungkin sudah memakainya.
		Raw:     raw,
		Choices: make([]providers.Choice, 0, len(wire.Choices)),
	}
	if out.Model == "" {
		// Sebagian provider compatible tidak mengembalikan nama model. Pencatatan
		// pemakaian butuh nilai itu, dan model yang diminta adalah jawaban yang benar.
		out.Model = req.Model
	}
	if wire.Usage != nil {
		out.Usage = wire.Usage.toCanonical()
	}

	for _, c := range wire.Choices {
		msg := providers.Message{
			Role:             providers.Role(firstNonEmpty(c.Message.Role, string(providers.RoleAssistant))),
			ReasoningContent: firstNonEmpty(c.Message.ReasoningContent, jsonString(c.Message.Reasoning)),
		}
		decodeContent(&msg, c.Message.Content)
		for i, tc := range c.Message.ToolCalls {
			msg.ToolCalls = append(msg.ToolCalls, tc.toCanonical(i))
		}
		out.Choices = append(out.Choices, providers.Choice{
			Index:        c.Index,
			Message:      msg,
			FinishReason: c.FinishReason,
		})
	}
	return out, nil
}

// decodeContent mengisi Content atau Parts dari nilai "content" mentah.
//
// Bentuk yang dikirim upstream hampir selalu string atau null, tetapi sebagian provider
// compatible menjawab dengan array bagian. Keduanya diterima: menolak yang kedua berarti
// seluruh respons gagal diurai padahal isinya lengkap.
func decodeContent(msg *providers.Message, raw json.RawMessage) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return
	}
	var text string
	if json.Unmarshal(trimmed, &text) == nil {
		msg.Content = text
		return
	}
	var parts []providers.ContentPart
	if json.Unmarshal(trimmed, &parts) == nil {
		msg.Parts = parts
	}
}

// jsonString membaca satu nilai JSON sebagai string, kosong bila bentuknya lain.
func jsonString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) != nil {
		return ""
	}
	return s
}

// chatPayload menyusun body permintaan chat completion.
func (p *Provider) chatPayload(req *providers.ChatRequest, streaming bool) ([]byte, *providers.Error) {
	if req == nil || len(req.Messages) == 0 {
		return nil, providers.Newf(providers.ErrKindInvalidRequest, p.name, "permintaan chat tanpa pesan")
	}
	if strings.TrimSpace(req.Model) == "" {
		return nil, providers.Newf(providers.ErrKindInvalidRequest, p.name, "nama model wajib diisi")
	}

	body := map[string]any{
		"model":    req.Model,
		"messages": messagesToWire(req.Messages),
		// stream selalu ditulis eksplisit, termasuk ketika false. Itu menjadikannya field
		// milik gateway sehingga Extra tidak bisa membalik mode transport — tanpa ini
		// sebuah "stream":true dari klien pada jalur non-streaming membuat adapter mencoba
		// menguraikan aliran SSE sebagai satu objek JSON.
		"stream": streaming,
	}
	if streaming && p.wantsStreamUsage() {
		body["stream_options"] = map[string]any{"include_usage": true}
	}

	if req.MaxTokens != nil {
		body["max_tokens"] = *req.MaxTokens
	}
	if req.Temperature != nil {
		body["temperature"] = *req.Temperature
	}
	if req.TopP != nil {
		body["top_p"] = *req.TopP
	}
	if len(req.Stop) > 0 {
		body["stop"] = req.Stop
	}
	if len(req.Tools) > 0 {
		body["tools"] = toolsToWire(req.Tools)
	}
	if len(req.ToolChoice) > 0 {
		body["tool_choice"] = req.ToolChoice
	}
	if req.ParallelToolCalls != nil {
		body["parallel_tool_calls"] = *req.ParallelToolCalls
	}
	if len(req.ResponseFormat) > 0 {
		body["response_format"] = req.ResponseFormat
	}
	if req.Seed != nil {
		body["seed"] = *req.Seed
	}
	if req.ReasoningEffort != "" {
		body["reasoning_effort"] = req.ReasoningEffort
	}
	if req.User != "" {
		body["user"] = req.User
	}

	applyExtra(body, req.Extra)

	payload, err := json.Marshal(body)
	if err != nil {
		// Satu-satunya sumber kegagalan di sini adalah json.RawMessage tidak sah dari
		// Extra atau ToolChoice/ResponseFormat. Pesan json memuat potongan body klien,
		// jadi tidak diteruskan.
		return nil, providers.Newf(providers.ErrKindInvalidRequest, p.name,
			"body permintaan tidak bisa diserialisasi ke JSON")
	}
	return payload, nil
}

// applyExtra menambahkan field mentah dari klien ke body.
//
// Aturannya satu arah: field yang SUDAH diisi gateway tidak bisa ditimpa, dan pada
// tabrakan nilai dari Extra diabaikan tanpa membuat permintaan gagal. Extra berasal dari
// body klien, dan membiarkannya menang berarti klien bisa menukar model yang sudah
// dipilih mesin routing, mengganti messages yang sudah lewat content filter, atau
// membalik "stream" sehingga adapter membaca bentuk respons yang salah.
//
// Field yang tidak diisi gateway tetap diteruskan apa adanya — itulah gunanya Extra:
// parameter baru dari provider bisa dipakai tanpa menunggu gateway diperbarui.
func applyExtra(body map[string]any, extra map[string]json.RawMessage) {
	for k, v := range extra {
		if k == "" || len(v) == 0 {
			continue
		}
		if _, taken := body[k]; taken {
			continue
		}
		body[k] = v
	}
}

// messagesToWire menerjemahkan pesan kanonik ke bentuk body OpenAI.
func messagesToWire(msgs []providers.Message) []wireMessage {
	out := make([]wireMessage, 0, len(msgs))
	for i := range msgs {
		m := &msgs[i]
		w := wireMessage{
			Role:       string(m.Role),
			Name:       m.Name,
			ToolCallID: m.ToolCallID,
		}

		switch {
		case m.IsMultimodal():
			// Bentuk array dipilih dari Parts, bukan ditebak dari panjang Content.
			w.Content = m.Parts
		case m.Content == "" && len(m.ToolCalls) > 0:
			// Pesan asisten yang isinya hanya tool call: content dibiarkan tidak terkirim
			// (setara null), karena string kosong ditolak sebagian implementasi.
			w.Content = nil
		default:
			w.Content = m.Content
		}

		for _, tc := range m.ToolCalls {
			w.ToolCalls = append(w.ToolCalls, toolCallToWire(tc))
		}
		// ReasoningContent sengaja TIDAK dikirim balik. Itu field keluaran: DeepSeek
		// menolak permintaan yang menyertakannya dan OpenAI tidak mengenalnya, sehingga
		// meneruskannya justru mematikan percakapan lanjutan dengan model penalaran.
		out = append(out, w)
	}
	return out
}

// toolsToWire menyalin daftar tool dengan satu penyesuaian: type kosong diisi "function".
func toolsToWire(tools []providers.Tool) []providers.Tool {
	out := make([]providers.Tool, len(tools))
	copy(out, tools)
	for i := range out {
		if out[i].Type == "" {
			out[i].Type = "function"
		}
	}
	return out
}

// wantsStreamUsage memutuskan apakah stream_options.include_usage ikut dikirim.
//
// Tanpa usage di aliran, biaya request streaming tidak bisa dihitung: chunk penutup OpenAI
// hanya memuat usage bila diminta lewat field ini. Karena itu untuk KindOpenAI field itu
// SELALU dikirim — di sana kami tahu ia didukung.
//
// Untuk KindOpenAICompatible dan KindCustom field itu TIDAK dikirim. Banyak implementasi
// pihak ketiga memvalidasi body secara ketat dan menolak field yang tidak dikenalnya
// dengan 400, jadi memaksakannya akan mematikan seluruh streaming pada provider yang
// sebenarnya berfungsi — kegagalan total demi menghitung biaya. Yang dilakukan sebagai
// gantinya: usage tetap dibaca kalau upstream mengirimkannya sendiri, dan pemanggil yang
// tahu upstream-nya mendukung bisa menyalakannya lewat Extra["stream_options"], yang
// diteruskan apa adanya karena gateway tidak mengisi kunci itu untuk kedua Kind ini.
func (p *Provider) wantsStreamUsage() bool { return p.kind == providers.KindOpenAI }

// ChatCompletionStream menjalankan satu completion streaming.
//
// Tenggat aliran berasal dari context pemanggil, bukan dari Config.Timeout: klien HTTP
// streaming sengaja dibuat tanpa Timeout karena http.Client.Timeout membatasi seluruh umur
// respons termasuk pembacaan body, sehingga setiap aliran akan mati pada detik yang sama
// tanpa peduli masih mengalir atau tidak.
func (p *Provider) ChatCompletionStream(ctx context.Context, req *providers.ChatRequest) (providers.Stream, error) {
	payload, err := p.chatPayload(req, true)
	if err != nil {
		return nil, err
	}

	// Context diturunkan supaya Close punya cara membatalkannya. Tanpa ini, satu-satunya
	// pegangan untuk melepas koneksi adalah menutup body.
	streamCtx, cancel := context.WithCancel(ctx)
	resp, doErr := p.do(streamCtx, http.MethodPost, pathChatCompletions, payload, true)
	if doErr != nil {
		cancel()
		return nil, doErr
	}

	// Sebagian provider compatible mengabaikan "stream":true dan menjawab satu objek JSON.
	// Kalau dibiarkan lewat, pembaca SSE tidak menemukan satu pun baris data dan pemanggil
	// menerima aliran kosong tanpa penjelasan. Belum ada byte yang terkirim ke klien di
	// titik ini, jadi kegagalannya masih boleh diulang maupun dialihkan.
	if ct := strings.ToLower(resp.Header.Get("Content-Type")); strings.Contains(ct, "application/json") {
		resp.Body.Close()
		cancel()
		return nil, providers.Newf(providers.ErrKindServer, p.name,
			"upstream menjawab JSON biasa, bukan aliran SSE")
	}

	return &chatStream{
		provider: p.name,
		body:     resp.Body,
		reader:   providers.NewSSEReader(resp.Body),
		cancel:   cancel,
	}, nil
}

// chatStream adalah providers.Stream di atas aliran SSE berdialek OpenAI.
type chatStream struct {
	provider string
	body     io.ReadCloser
	reader   *providers.SSEReader
	cancel   context.CancelFunc

	// readMu menjaga pembaca SSE beserta pencatatan aliran. Close TIDAK mengambil lock
	// ini: Recv memegangnya selama menggantung menunggu byte dari upstream, jadi Close
	// yang menunggu lock yang sama tidak akan pernah bisa menghentikan aliran yang macet —
	// padahal justru itu keadaan ketika Close paling dibutuhkan.
	readMu   sync.Mutex
	ended    error
	streamed int64

	closed    atomic.Bool
	closeOnce sync.Once
	closeErr  error
}

// Recv mengembalikan peristiwa berikutnya, atau io.EOF saat aliran selesai normal.
func (s *chatStream) Recv() (*providers.StreamEvent, error) {
	s.readMu.Lock()
	defer s.readMu.Unlock()

	if s.ended != nil {
		return nil, s.ended
	}
	if s.closed.Load() {
		return nil, s.end(providers.Newf(providers.ErrKindCanceled, s.provider, "aliran sudah ditutup"))
	}

	for {
		ev, err := s.reader.Next()
		if err != nil {
			return nil, s.end(s.readFailure(err))
		}

		data := bytes.TrimSpace(ev.Data)
		switch {
		case len(data) == 0:
			// Peristiwa tanpa data: keep-alive atau pemisah, bukan chunk.
			continue
		case providers.IsDoneMarker(data):
			// Penanda akhir adalah bagian framing, bukan chunk. Pemanggil mengenali akhir
			// aliran dari io.EOF lalu menulis penandanya sendiri ke klien.
			return nil, s.end(io.EOF)
		}

		out, perr := parseChunk(data)
		if perr != nil {
			return nil, s.end(s.chunkFailure())
		}
		// Yang dihitung adalah byte yang benar-benar diserahkan ke pemanggil: begitu satu
		// chunk keluar dari Recv, ia dianggap sudah sampai ke klien.
		s.streamed += int64(len(out.Raw))
		return out, nil
	}
}

// end mencatat akhir aliran supaya setiap Recv sesudahnya mengembalikan hal yang sama.
func (s *chatStream) end(err error) error {
	s.ended = err
	return err
}

// readFailure mengubah kegagalan pembacaan menjadi hasil yang tepat untuk mesin routing.
func (s *chatStream) readFailure(err error) error {
	// EOF bersih berarti aliran habis. Tidak semua provider berdialek OpenAI mengirim
	// [DONE] sebelum menutup, dan menganggap ini kegagalan akan menghasilkan error palsu
	// pada setiap request ke provider yang sebenarnya menjawab lengkap.
	if errors.Is(err, io.EOF) {
		return io.EOF
	}

	kind := providers.ClassifyTransport(err)
	msg := "aliran upstream gagal dibaca"
	switch {
	case s.closed.Load():
		// Pembacaan gagal karena Close menutup body, bukan karena upstream.
		kind = providers.ErrKindCanceled
		msg = "aliran dihentikan sebelum selesai"
	case kind == providers.ErrKindCanceled:
		msg = "aliran dihentikan sebelum selesai"
	case s.streamed > 0:
		// Inilah yang dimaksud ErrKindStreamAborted: sebagian jawaban sudah sampai ke
		// klien, jadi aliran kedua dari mana pun akan tersambung di tengah aliran pertama.
		kind = providers.ErrKindStreamAborted
		msg = "aliran upstream terputus setelah sebagian jawaban terkirim"
	}
	return &providers.Error{
		Kind:     kind,
		Provider: s.provider,
		Message:  msg,
		// StreamedBytes yang lebih dari nol menahan retry maupun failover. Nilainya diisi
		// pada semua kegagalan, bukan hanya stream_aborted, karena yang menentukan bukan
		// kategorinya melainkan apakah klien sudah menerima sesuatu.
		StreamedBytes: s.streamed,
	}
}

// chunkFailure dipakai ketika muatan chunk bukan JSON yang bisa diurai.
//
// Chunk seperti itu tidak boleh diteruskan ke klien: Raw adalah muatan yang dikirim apa
// adanya, jadi meneruskannya berarti menyisipkan sampah ke tengah aliran yang sedang
// dibaca pustaka klien.
func (s *chatStream) chunkFailure() error {
	kind := providers.ErrKindServer
	if s.streamed > 0 {
		kind = providers.ErrKindStreamAborted
	}
	return &providers.Error{
		Kind:          kind,
		Provider:      s.provider,
		Message:       "chunk aliran upstream tidak bisa diurai",
		StreamedBytes: s.streamed,
	}
}

// Close menutup aliran. Aman dipanggil berulang dan dari goroutine mana pun.
func (s *chatStream) Close() error {
	s.closeOnce.Do(func() {
		// Ditandai lebih dulu supaya Recv yang sedang menggantung mengenali bahwa
		// kegagalan bacanya berasal dari sini, bukan dari upstream.
		s.closed.Store(true)
		// Body ditutup lebih dulu: itu yang menghentikan pembacaan yang menggantung dan
		// melepas koneksi. Context dibatalkan sesudahnya sebagai jaring pengaman — pada
		// respons yang masih ditulis upstream, menutup body saja tidak selalu langsung
		// menghentikan transport, dan koneksi yang tidak dilepas akan menggantung sampai
		// timeout upstream, satu untuk setiap aliran yang dibatalkan.
		s.closeErr = s.body.Close()
		s.cancel()
	})
	return s.closeErr
}

// parseChunk menerjemahkan satu muatan data SSE menjadi StreamEvent.
func parseChunk(data []byte) (*providers.StreamEvent, error) {
	var wire chatChunk
	if err := json.Unmarshal(data, &wire); err != nil {
		return nil, err
	}

	out := &providers.StreamEvent{
		// Raw adalah MUATAN data SSE, bukan blok SSE lengkap: framing ("data: ", baris
		// kosong, penanda [DONE]) dibuat ulang oleh lapisan HTTP gateway. Itu satu-satunya
		// pilihan yang konsisten antar adapter — provider berdialek lain hanya bisa
		// menghasilkan muatan bergaya OpenAI, bukan byte SSE asli.
		//
		// Disalin karena SSEReader memang menyerahkan slice baru setiap peristiwa, tetapi
		// muatan ini hidup lebih lama daripada satu putaran Recv dan tidak boleh bergantung
		// pada detail itu.
		Raw: bytes.Clone(data),
	}
	if wire.Usage != nil {
		usage := wire.Usage.toCanonical()
		out.Usage = &usage
	}
	if len(wire.Choices) > 0 {
		// Hanya choice pertama yang diurai: StreamEvent tidak punya tempat untuk lebih
		// dari satu. Chunk aslinya tetap utuh di Raw, jadi klien yang meminta n>1 tetap
		// menerima seluruh choice — yang tidak tersedia hanyalah bentuk teruraiannya untuk
		// content filter dan pencatatan gateway.
		c := wire.Choices[0]
		out.Delta = c.Delta.Content
		out.ReasoningDelta = firstNonEmpty(c.Delta.ReasoningContent, jsonString(c.Delta.Reasoning))
		out.FinishReason = c.FinishReason
		for i, tc := range c.Delta.ToolCalls {
			out.ToolCalls = append(out.ToolCalls, tc.toCanonical(i))
		}
	}
	return out, nil
}
