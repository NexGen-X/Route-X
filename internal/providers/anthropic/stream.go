package anthropic

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/NexGen-X/Route-X/internal/providers"
)

// Nama peristiwa aliran Anthropic.
const (
	evMessageStart      = "message_start"
	evContentBlockStart = "content_block_start"
	evContentBlockDelta = "content_block_delta"
	evContentBlockStop  = "content_block_stop"
	evMessageDelta      = "message_delta"
	evMessageStop       = "message_stop"
	evPing              = "ping"
	evError             = "error"
)

// streamEnvelope memuat seluruh bentuk data peristiwa aliran Anthropic.
//
// Satu struct untuk semua jenis peristiwa: bidangnya tidak bertumpang tindih, dan
// menguraikannya dua kali (sekali untuk membaca "type", sekali untuk isinya) berarti
// setiap potongan diurai dua kali sepanjang aliran.
type streamEnvelope struct {
	Type         string            `json:"type"`
	Index        int               `json:"index"`
	Message      *messagesResponse `json:"message"`
	ContentBlock *block            `json:"content_block"`
	Delta        *streamDelta      `json:"delta"`
	Usage        *upstreamUsage    `json:"usage"`
	Error        *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

type streamDelta struct {
	Type        string `json:"type"`
	Text        string `json:"text"`
	PartialJSON string `json:"partial_json"`
	Thinking    string `json:"thinking"`
	StopReason  string `json:"stop_reason"`
}

// ChatCompletionStream membuka aliran SSE ke /v1/messages.
//
// Body selalu memakai stream:true tanpa melihat ChatRequest.Stream: pemanggil sudah
// memilih method ini, dan menghormati flag yang berlawanan hanya menghasilkan respons
// non-streaming yang tidak bisa dibaca sebagai aliran.
func (p *Provider) ChatCompletionStream(ctx context.Context, req *providers.ChatRequest) (providers.Stream, error) {
	body, err := p.buildMessagesRequest(req, true)
	if err != nil {
		return nil, err
	}
	resp, err := p.do(ctx, p.streamClient, http.MethodPost, p.endpoint("/v1/messages"), body, "text/event-stream")
	if err != nil {
		return nil, err
	}
	return &stream{
		provider: p,
		resp:     resp,
		reader:   providers.NewSSEReader(resp.Body),
		created:  time.Now().Unix(),
		model:    req.Model,
	}, nil
}

// stream membaca aliran peristiwa Anthropic dan menerjemahkannya ke bentuk kanonik.
type stream struct {
	provider *Provider
	resp     *http.Response
	reader   *providers.SSEReader

	closeOnce sync.Once
	closeErr  error

	done     bool
	streamed int64

	id      string
	model   string
	created int64

	// usage dikumpulkan lintas peristiwa: input datang di message_start, output final di
	// message_delta. Nilainya kumulatif, jadi penggabungannya "yang terbaru menang",
	// bukan penjumlahan — menjumlahkan output_tokens dari kedua peristiwa akan
	// menggandakan tagihan.
	usage     upstreamUsage
	usageSeen bool
	usageSent bool

	// toolIndex memetakan index content block Anthropic ke index tool_calls kanonik.
	// Keduanya berbeda: block teks juga menempati index, sehingga tool_use pertama bisa
	// berada di block index 1 sementara klien mengharapkan tool_calls index 0.
	toolIndex map[int]int
	nextTool  int
}

// String menyamarkan seluruh isi stream.
//
// provider adalah field TAK DIEKSPOR yang memuat kredensial upstream, dan fmt tidak boleh
// memanggil metode pada nilai yang diperoleh dari field tak diekspor. Tanpa metode di
// tingkat struct, "%s" pada *stream menempuh jalur verb-salah milik fmt yang membongkar
// isi struct beserta kredensialnya.
//
// Receiver-nya POINTER, bukan nilai: struct ini memuat sync.Once, dan go vet menolak
// metode ber-receiver nilai di atas struct berlock ("String passes lock by value").
// Itu aman justru karena larangan yang sama berlaku bagi siapa pun yang mencoba menyalin
// nilainya, sehingga "%v" pada bentuk nilai tidak bisa ditulis tanpa vet ikut merah.
// Dijaga TestRedaksiFieldTakDiekspor di internal/security.
func (s *stream) String() string { return "anthropic.stream{[REDACTED]}" }

// GoString menutup jalur "%#v".
func (s *stream) GoString() string { return s.String() }

// LogValue menutup jalur slog.
func (s *stream) LogValue() slog.Value { return slog.StringValue(s.String()) }

// Recv mengembalikan peristiwa berikutnya, atau io.EOF saat aliran selesai normal.
func (s *stream) Recv() (*providers.StreamEvent, error) {
	if s.done {
		return nil, io.EOF
	}
	for {
		ev, err := s.reader.Next()
		if err != nil {
			s.done = true
			if errors.Is(err, io.EOF) {
				// Aliran Anthropic selalu ditutup message_stop; tidak ada penanda [DONE].
				// Berakhir tanpa message_stop berarti koneksi terputus di tengah.
				return nil, s.failure(nil, "aliran Anthropic terputus sebelum peristiwa message_stop")
			}
			return nil, s.failure(err, "aliran Anthropic gagal dibaca")
		}

		out, err := s.handle(ev)
		switch {
		case err != nil:
			s.done = true
			return nil, err
		case out != nil:
			s.streamed += int64(len(out.Raw))
			return out, nil
		}
		// Peristiwa tanpa isi untuk klien (ping, komentar keep-alive, penutup block):
		// lanjut membaca alih-alih mengembalikan peristiwa kosong.
	}
}

// handle menerjemahkan satu peristiwa. Kembalian (nil, nil) berarti peristiwa dilewati.
func (s *stream) handle(ev *providers.SSEEvent) (*providers.StreamEvent, error) {
	if len(ev.Data) == 0 {
		return nil, nil
	}
	var env streamEnvelope
	if err := json.Unmarshal(ev.Data, &env); err != nil {
		return nil, s.failure(nil, "peristiwa aliran Anthropic tidak bisa diurai")
	}
	// Jenis peristiwa diambil dari body bila ada; nama pada baris "event:" hanya cadangan.
	kind := env.Type
	if kind == "" {
		kind = ev.Event
	}

	switch kind {
	case evPing, evContentBlockStop:
		return nil, nil

	case evError:
		return nil, s.upstreamError(&env)

	case evMessageStart:
		if env.Message != nil {
			s.id = env.Message.ID
			if env.Message.Model != "" {
				s.model = env.Message.Model
			}
			s.mergeUsage(&env.Message.Usage)
		}
		// Potongan pembuka bergaya OpenAI hanya membawa peran. Klien resmi OpenAI
		// mengandalkannya untuk mengetahui pengirim sebelum teks pertama datang.
		return s.event(&oaDelta{Role: string(providers.RoleAssistant)}, "", nil), nil

	case evContentBlockStart:
		return s.blockStart(&env), nil

	case evContentBlockDelta:
		return s.blockDelta(&env), nil

	case evMessageDelta:
		if env.Usage != nil {
			s.mergeUsage(env.Usage)
		}
		reason := ""
		if env.Delta != nil {
			reason = mapStopReason(env.Delta.StopReason)
		}
		var usage *providers.Usage
		if s.usageSeen {
			u := toCanonicalUsage(s.usage)
			usage = &u
			s.usageSent = true
		}
		return s.event(&oaDelta{}, reason, usage), nil

	case evMessageStop:
		s.done = true
		// Bila message_delta tidak pernah datang, usage yang sudah terkumpul masih harus
		// sampai ke pencatat pemakaian. Potongan tanpa choice adalah bentuk yang dipakai
		// OpenAI untuk itu.
		if s.usageSeen && !s.usageSent {
			u := toCanonicalUsage(s.usage)
			s.usageSent = true
			return s.event(nil, "", &u), nil
		}
		return nil, io.EOF

	default:
		// Anthropic menambah jenis peristiwa baru tanpa menaikkan versi API, jadi yang
		// tidak dikenal dilewati alih-alih menggagalkan aliran yang sedang berjalan.
		return nil, nil
	}
}

// blockStart menangani content_block_start.
func (s *stream) blockStart(env *streamEnvelope) *providers.StreamEvent {
	if env.ContentBlock == nil {
		return nil
	}
	switch env.ContentBlock.Type {
	case blockToolUse:
		idx := s.toolIndexFor(env.Index)
		call := providers.ToolCall{
			Index: idx,
			ID:    env.ContentBlock.ID,
			Type:  "function",
			// Argumen menyusul lewat input_json_delta; potongan pembuka hanya membawa
			// id dan nama, persis seperti potongan tool_calls pertama milik OpenAI.
			Function: providers.FunctionCall{Name: env.ContentBlock.Name},
		}
		out := s.event(&oaDelta{ToolCalls: encodeToolCalls([]providers.ToolCall{call}, true)}, "", nil)
		out.ToolCalls = []providers.ToolCall{call}
		return out

	case blockText:
		// Block teks lahir kosong pada kasus normal; hanya diteruskan bila sudah berisi.
		if env.ContentBlock.Text == "" {
			return nil
		}
		out := s.event(textDelta(env.ContentBlock.Text), "", nil)
		out.Delta = env.ContentBlock.Text
		return out

	default:
		return nil
	}
}

// blockDelta menangani content_block_delta.
func (s *stream) blockDelta(env *streamEnvelope) *providers.StreamEvent {
	if env.Delta == nil {
		return nil
	}
	switch env.Delta.Type {
	case "text_delta":
		if env.Delta.Text == "" {
			return nil
		}
		out := s.event(textDelta(env.Delta.Text), "", nil)
		out.Delta = env.Delta.Text
		return out

	case "thinking_delta":
		if env.Delta.Thinking == "" {
			return nil
		}
		// Dialek OpenAI tidak punya field resmi untuk jejak penalaran; "reasoning_content"
		// adalah nama yang sudah dipakai luas oleh penyedia lain, jadi klien yang
		// menampilkan penalaran sudah mengenalinya.
		out := s.event(&oaDelta{ReasoningContent: env.Delta.Thinking}, "", nil)
		out.ReasoningDelta = env.Delta.Thinking
		return out

	case "input_json_delta":
		if env.Delta.PartialJSON == "" {
			return nil
		}
		call := providers.ToolCall{
			Index:    s.toolIndexFor(env.Index),
			Function: providers.FunctionCall{Arguments: env.Delta.PartialJSON},
		}
		out := s.event(&oaDelta{ToolCalls: encodeToolCalls([]providers.ToolCall{call}, true)}, "", nil)
		out.ToolCalls = []providers.ToolCall{call}
		return out

	default:
		// signature_delta dan jenis baru lainnya tidak punya wujud di sisi klien.
		return nil
	}
}

// event menyusun StreamEvent kanonik beserta muatan dialek OpenAI-nya.
func (s *stream) event(delta *oaDelta, finishReason string, usage *providers.Usage) *providers.StreamEvent {
	return &providers.StreamEvent{
		FinishReason: finishReason,
		Usage:        usage,
		Raw:          encodeChunk(s.id, s.model, s.created, delta, finishReason, usage),
	}
}

// toolIndexFor memetakan index content block ke index tool_calls kanonik.
func (s *stream) toolIndexFor(blockIndex int) int {
	if idx, ok := s.toolIndex[blockIndex]; ok {
		return idx
	}
	if s.toolIndex == nil {
		s.toolIndex = make(map[int]int, 4)
	}
	idx := s.nextTool
	s.nextTool++
	s.toolIndex[blockIndex] = idx
	return idx
}

// mergeUsage menggabungkan laporan usage yang datang terpisah antar peristiwa.
func (s *stream) mergeUsage(u *upstreamUsage) {
	if u == nil {
		return
	}
	if u.InputTokens > 0 {
		s.usage.InputTokens = u.InputTokens
	}
	if u.OutputTokens > 0 {
		s.usage.OutputTokens = u.OutputTokens
	}
	if u.CacheReadInputTokens > 0 {
		s.usage.CacheReadInputTokens = u.CacheReadInputTokens
	}
	if u.CacheCreationInputTokens > 0 {
		s.usage.CacheCreationInputTokens = u.CacheCreationInputTokens
	}
	if u.InputTokens > 0 || u.OutputTokens > 0 || u.CacheReadInputTokens > 0 || u.CacheCreationInputTokens > 0 {
		s.usageSeen = true
	}
}

// upstreamError menerjemahkan peristiwa error di tengah aliran.
func (s *stream) upstreamError(env *streamEnvelope) *providers.Error {
	code, message := "", "Anthropic melaporkan kegagalan di tengah aliran"
	if env.Error != nil {
		code = env.Error.Type
		if env.Error.Message != "" {
			message = s.provider.safeMessage(env.Error.Message)
		}
	}
	return &providers.Error{
		Kind:          kindForUpstreamType(code),
		Provider:      s.provider.name,
		Message:       message,
		UpstreamCode:  code,
		StreamedBytes: s.streamed,
	}
}

// failure menyusun kegagalan aliran.
//
// StreamedBytes selalu diisi: begitu sebagian aliran sampai ke klien, tidak ada kegagalan
// yang boleh diulang atau dialihkan — aliran kedua akan tersambung di tengah aliran
// pertama. Pembatalan dan batas waktu tetap dibedakan karena keduanya bukan kesalahan
// provider.
func (s *stream) failure(err error, msg string) *providers.Error {
	out := &providers.Error{
		Provider:      s.provider.name,
		Message:       msg,
		StreamedBytes: s.streamed,
	}
	switch {
	case err != nil && errors.Is(err, context.Canceled):
		out.Kind = providers.ErrKindCanceled
		out.Message = "permintaan dibatalkan sebelum aliran selesai"
	case err != nil && errors.Is(err, context.DeadlineExceeded):
		out.Kind = providers.ErrKindTimeout
		out.Message = "aliran melewati batas waktu sebelum selesai"
	case s.streamed > 0:
		out.Kind = providers.ErrKindStreamAborted
	default:
		// Belum ada satu byte pun yang diteruskan ke klien, jadi permintaan ini masih
		// aman diulang maupun dialihkan.
		out.Kind = providers.ErrKindServer
	}
	return out
}

// Close menutup koneksi upstream. Aman dipanggil berulang.
func (s *stream) Close() error {
	s.closeOnce.Do(func() {
		if s.resp != nil && s.resp.Body != nil {
			s.closeErr = s.resp.Body.Close()
		}
	})
	return s.closeErr
}

// kindForUpstreamType menerjemahkan jenis error Anthropic ke klasifikasi kanonik.
func kindForUpstreamType(t string) providers.ErrorKind {
	switch t {
	case "overloaded_error":
		return providers.ErrKindOverloaded
	case "rate_limit_error":
		return providers.ErrKindRateLimit
	case "authentication_error":
		return providers.ErrKindAuth
	case "permission_error":
		return providers.ErrKindPermission
	case "not_found_error":
		return providers.ErrKindModelNotFound
	case "invalid_request_error":
		return providers.ErrKindInvalidRequest
	case "timeout_error":
		return providers.ErrKindTimeout
	default:
		return providers.ErrKindServer
	}
}
