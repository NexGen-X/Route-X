package google

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/NexGen-X/Route-X/internal/providers"
)

// ChatCompletionStream membuka aliran SSE ke :streamGenerateContent.
//
// Parameter alt=sse WAJIB: tanpanya Gemini mengalirkan satu array JSON raksasa yang hanya
// bisa dibaca setelah utuh, bukan peristiwa Server-Sent Events.
func (p *Provider) ChatCompletionStream(ctx context.Context, req *providers.ChatRequest) (providers.Stream, error) {
	if req == nil {
		return nil, providers.Newf(providers.ErrKindInvalidRequest, p.name, "permintaan kosong")
	}
	body, err := p.buildGenerateRequest(req)
	if err != nil {
		return nil, err
	}
	endpoint, err := p.methodURL(req.Model, "streamGenerateContent", "alt=sse")
	if err != nil {
		return nil, err
	}
	resp, err := p.do(ctx, p.streamClient, http.MethodPost, endpoint, body, "text/event-stream")
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

// stream membaca aliran Gemini dan menerjemahkannya ke bentuk kanonik.
//
// Berbeda dari aliran bergaya OpenAI, setiap peristiwa alt=sse memuat objek respons UTUH:
// bentuknya sama dengan respons non-streaming, dan yang baru hanyalah isi parts-nya. Jadi
// satu peristiwa upstream diterjemahkan menjadi satu StreamEvent — bukan diakumulasi.
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

	// usage bersifat kumulatif di setiap peristiwa Gemini, jadi digabung dengan aturan
	// "yang terbaru menang" dan hanya dikirim sekali di peristiwa penutup. Menjumlahkannya
	// antar peristiwa akan menggandakan tagihan; mengirimkannya di setiap peristiwa akan
	// menggandakannya pada klien yang menjumlahkan sendiri.
	usage     usageMetadata
	usageSeen bool
	usageSent bool

	// finished menandai finishReason sudah diterima, yaitu satu-satunya tanda akhir normal
	// yang dimiliki aliran Gemini — tidak ada penanda [DONE] maupun peristiwa penutup.
	finished bool
	roleSent bool
	nextTool int
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
func (s *stream) String() string { return "google.stream{[REDACTED]}" }

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
				return s.finish()
			}
			return nil, s.failure(err, "aliran Gemini gagal dibaca")
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
		// Peristiwa tanpa isi untuk klien (keep-alive, kandidat kosong): lanjut membaca.
	}
}

// finish menutup aliran yang berakhir di EOF.
func (s *stream) finish() (*providers.StreamEvent, error) {
	if !s.finished {
		// Aliran Gemini yang selesai normal selalu memuat finishReason di peristiwa
		// terakhir. Berakhir tanpa itu berarti koneksi terputus di tengah.
		return nil, s.failure(nil, "aliran Gemini terputus sebelum finishReason diterima")
	}
	if s.usageSeen && !s.usageSent {
		// Usage yang belum terkirim tetap harus sampai ke pencatat pemakaian. Potongan
		// tanpa choice adalah bentuk yang dipakai OpenAI untuk itu.
		u := toCanonicalUsage(&s.usage)
		s.usageSent = true
		out := s.event(nil, "", &u)
		s.streamed += int64(len(out.Raw))
		return out, nil
	}
	return nil, io.EOF
}

// handle menerjemahkan satu peristiwa. Kembalian (nil, nil) berarti peristiwa dilewati.
func (s *stream) handle(ev *providers.SSEEvent) (*providers.StreamEvent, error) {
	if len(ev.Data) == 0 {
		return nil, nil
	}
	var chunk generateResponse
	if err := json.Unmarshal(ev.Data, &chunk); err != nil {
		return nil, s.failure(nil, "peristiwa aliran Gemini tidak bisa diurai")
	}
	// blockReason maupun error yang menumpang di tengah aliran diklasifikasi sama seperti
	// pada respons non-streaming; bedanya di sini StreamedBytes ikut terisi.
	if err := s.provider.checkBlocked(&chunk); err != nil {
		if e := providers.AsError(err); e != nil {
			e.StreamedBytes = s.streamed
			return nil, e
		}
		return nil, err
	}

	if chunk.ResponseID != "" {
		s.id = chunk.ResponseID
	}
	if chunk.ModelVersion != "" {
		s.model = chunk.ModelVersion
	}
	if chunk.UsageMetadata != nil {
		s.mergeUsage(chunk.UsageMetadata)
	}
	if len(chunk.Candidates) == 0 {
		return nil, nil
	}

	// Hanya kandidat pertama yang dialirkan: StreamEvent kanonik tidak punya penanda
	// choice, jadi kandidat kedua tidak punya tempat untuk dituju tanpa merusak urutan
	// potongan kandidat pertama.
	cand := &chunk.Candidates[0]
	var text, reasoning strings.Builder
	var calls []providers.ToolCall

	for i := range cand.Content.Parts {
		part := &cand.Content.Parts[i]
		switch {
		case part.FunctionCall != nil:
			args := "{}"
			if len(strings.TrimSpace(string(part.FunctionCall.Args))) > 0 {
				args = string(part.FunctionCall.Args)
			}
			// Panggilan tool di Gemini datang utuh dalam satu peristiwa, tidak dipecah
			// seperti argumen bergaya OpenAI — jadi id, nama, dan argumen sudah lengkap
			// pada potongan pertama. Penomorannya berjalan lintas peristiwa agar id yang
			// disintesis tetap unik sepanjang aliran.
			calls = append(calls, providers.ToolCall{
				Index:    s.nextTool,
				ID:       synthesizeCallID(s.nextTool, part.FunctionCall.Name),
				Type:     "function",
				Function: providers.FunctionCall{Name: part.FunctionCall.Name, Arguments: args},
			})
			s.nextTool++
		case part.Thought:
			reasoning.WriteString(part.Text)
		case part.Text != "":
			text.WriteString(part.Text)
		}
	}

	finish := mapFinishReason(cand.FinishReason)
	// Gemini menjawab STOP walau yang dihasilkannya panggilan tool. Yang diperiksa adalah
	// SELURUH aliran, bukan peristiwa ini saja: finishReason kerap datang di peristiwa
	// terakhir yang sudah tidak memuat part functionCall lagi, sehingga memeriksa peristiwa
	// itu sendiri akan melaporkan "stop" dan klien berhenti tanpa menjalankan tool.
	if s.nextTool > 0 && (finish == providers.FinishStop || finish == "") {
		finish = providers.FinishToolCalls
	}
	if cand.FinishReason != "" {
		s.finished = true
	}

	var usage *providers.Usage
	if s.finished && s.usageSeen && !s.usageSent {
		u := toCanonicalUsage(&s.usage)
		usage = &u
		s.usageSent = true
	}

	if text.Len() == 0 && reasoning.Len() == 0 && len(calls) == 0 && finish == "" && usage == nil {
		return nil, nil
	}

	delta := &oaDelta{ReasoningContent: reasoning.String(), ToolCalls: encodeToolCalls(calls, true)}
	if text.Len() > 0 {
		content := text.String()
		delta.Content = &content
	}
	if !s.roleSent {
		// Potongan pertama membawa peran, seperti aliran OpenAI.
		delta.Role = string(providers.RoleAssistant)
		s.roleSent = true
	}

	out := s.event(delta, finish, usage)
	out.Delta = text.String()
	out.ReasoningDelta = reasoning.String()
	out.ToolCalls = calls
	return out, nil
}

// event menyusun StreamEvent kanonik beserta muatan dialek OpenAI-nya.
func (s *stream) event(delta *oaDelta, finishReason string, usage *providers.Usage) *providers.StreamEvent {
	id := s.id
	if id == "" {
		id = "gemini-" + strconv.FormatInt(s.created, 10)
	}
	return &providers.StreamEvent{
		FinishReason: finishReason,
		Usage:        usage,
		Raw:          encodeChunk(id, s.model, s.created, delta, finishReason, usage),
	}
}

// mergeUsage menggabungkan usageMetadata yang datang berulang sepanjang aliran.
func (s *stream) mergeUsage(u *usageMetadata) {
	if u == nil {
		return
	}
	if u.PromptTokenCount > 0 {
		s.usage.PromptTokenCount = u.PromptTokenCount
	}
	if u.CandidatesTokenCount > 0 {
		s.usage.CandidatesTokenCount = u.CandidatesTokenCount
	}
	if u.CachedContentTokenCount > 0 {
		s.usage.CachedContentTokenCount = u.CachedContentTokenCount
	}
	if u.ThoughtsTokenCount > 0 {
		s.usage.ThoughtsTokenCount = u.ThoughtsTokenCount
	}
	if u.TotalTokenCount > 0 {
		s.usage.TotalTokenCount = u.TotalTokenCount
	}
	if u.PromptTokenCount > 0 || u.CandidatesTokenCount > 0 || u.CachedContentTokenCount > 0 ||
		u.ThoughtsTokenCount > 0 || u.TotalTokenCount > 0 {
		s.usageSeen = true
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
		// Belum ada satu byte pun yang diteruskan ke klien, jadi permintaan ini masih aman
		// diulang maupun dialihkan.
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
