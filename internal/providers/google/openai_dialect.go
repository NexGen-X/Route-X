package google

import (
	"encoding/json"

	"github.com/NexGen-X/Route-X/internal/providers"
)

// Berkas ini menyusun bentuk DIALEK OPENAI dari hasil terjemahan.
//
// Field Raw pada ChatResponse dan StreamEvent bukan body Anthropic apa adanya: permukaan
// API gateway berdialek OpenAI, dan Raw-lah yang diteruskan ke klien. Menyusun ulang
// respons dari struct kanonik di lapisan HTTP tidak mungkin dilakukan tanpa kehilangan
// bentuk, jadi terjemahannya dibuat sekali di sini, di tempat konteks Anthropic-nya masih
// lengkap.
//
// Struktur yang sama juga ada di paket anthropic. Duplikasi itu disengaja: keduanya
// dimiliki adapter masing-masing, dan kontrak bersama di paket providers tidak boleh tumbuh
// hanya untuk melayani dua penerjemah.

type oaCompletion struct {
	ID      string     `json:"id"`
	Object  string     `json:"object"`
	Created int64      `json:"created"`
	Model   string     `json:"model"`
	Choices []oaChoice `json:"choices"`
	Usage   *oaUsage   `json:"usage,omitempty"`
}

type oaChoice struct {
	Index   int       `json:"index"`
	Message oaMessage `json:"message"`
	// FinishReason bertipe penunjuk agar bisa keluar sebagai null, sama seperti OpenAI
	// pada potongan yang belum final.
	FinishReason *string `json:"finish_reason"`
}

type oaMessage struct {
	Role string `json:"role"`
	// Content sengaja bertipe penunjuk: OpenAI mengirim null (bukan string kosong) ketika
	// jawaban hanya berisi pemanggilan tool, dan sebagian klien membedakan keduanya.
	Content          *string      `json:"content"`
	ReasoningContent string       `json:"reasoning_content,omitempty"`
	ToolCalls        []oaToolCall `json:"tool_calls,omitempty"`
}

type oaDelta struct {
	Role             string       `json:"role,omitempty"`
	Content          *string      `json:"content,omitempty"`
	ReasoningContent string       `json:"reasoning_content,omitempty"`
	ToolCalls        []oaToolCall `json:"tool_calls,omitempty"`
}

type oaToolCall struct {
	// Index hanya muncul pada aliran, tempat klien memakainya untuk menyusun ulang
	// potongan argumen.
	Index    *int       `json:"index,omitempty"`
	ID       string     `json:"id,omitempty"`
	Type     string     `json:"type,omitempty"`
	Function oaFunction `json:"function"`
}

type oaFunction struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments"`
}

type oaUsage struct {
	PromptTokens            int             `json:"prompt_tokens"`
	CompletionTokens        int             `json:"completion_tokens"`
	TotalTokens             int             `json:"total_tokens"`
	PromptTokensDetails     *oaPromptDetail `json:"prompt_tokens_details,omitempty"`
	CompletionTokensDetails *oaOutputDetail `json:"completion_tokens_details,omitempty"`
}

type oaPromptDetail struct {
	CachedTokens int `json:"cached_tokens"`
}

type oaOutputDetail struct {
	ReasoningTokens int `json:"reasoning_tokens"`
}

type oaChunk struct {
	ID      string          `json:"id"`
	Object  string          `json:"object"`
	Created int64           `json:"created"`
	Model   string          `json:"model"`
	Choices []oaChunkChoice `json:"choices"`
	Usage   *oaUsage        `json:"usage,omitempty"`
}

type oaChunkChoice struct {
	Index        int     `json:"index"`
	Delta        oaDelta `json:"delta"`
	FinishReason *string `json:"finish_reason"`
}

// encode menyerialisasi bentuk dialek OpenAI.
//
// Kegagalan marshal tidak mungkin terjadi: seluruh field bertipe dasar, dan satu-satunya
// json.RawMessage yang ikut (argumen tool) sudah menjadi string di sini. Kalaupun terjadi,
// objek kosong lebih baik daripada menggagalkan respons yang isinya sudah lengkap.
func encode(v any) json.RawMessage {
	raw, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return raw
}

// encodeUsage menerjemahkan usage kanonik ke bentuk OpenAI.
func encodeUsage(u *providers.Usage) *oaUsage {
	if u == nil {
		return nil
	}
	out := &oaUsage{
		PromptTokens:     u.InputTokens,
		CompletionTokens: u.OutputTokens,
		TotalTokens:      u.TotalTokens,
	}
	if u.CachedInputTokens > 0 {
		out.PromptTokensDetails = &oaPromptDetail{CachedTokens: u.CachedInputTokens}
	}
	if u.ReasoningTokens > 0 {
		out.CompletionTokensDetails = &oaOutputDetail{ReasoningTokens: u.ReasoningTokens}
	}
	return out
}

// encodeToolCalls menerjemahkan pemanggilan tool kanonik, dengan atau tanpa index.
func encodeToolCalls(calls []providers.ToolCall, withIndex bool) []oaToolCall {
	if len(calls) == 0 {
		return nil
	}
	out := make([]oaToolCall, 0, len(calls))
	for i := range calls {
		tc := &calls[i]
		item := oaToolCall{
			ID:       tc.ID,
			Type:     tc.Type,
			Function: oaFunction{Name: tc.Function.Name, Arguments: tc.Function.Arguments},
		}
		if withIndex {
			idx := tc.Index
			item.Index = &idx
		}
		out = append(out, item)
	}
	return out
}

// encodeCompletion menyusun objek chat.completion dari respons kanonik.
func encodeCompletion(resp *providers.ChatResponse) json.RawMessage {
	out := oaCompletion{
		ID:      resp.ID,
		Object:  "chat.completion",
		Created: resp.Created,
		Model:   resp.Model,
		Choices: make([]oaChoice, 0, len(resp.Choices)),
		Usage:   encodeUsage(&resp.Usage),
	}
	for i := range resp.Choices {
		c := &resp.Choices[i]
		msg := oaMessage{
			Role:             string(c.Message.Role),
			ReasoningContent: c.Message.ReasoningContent,
			ToolCalls:        encodeToolCalls(c.Message.ToolCalls, false),
		}
		if c.Message.Content != "" || len(c.Message.ToolCalls) == 0 {
			content := c.Message.Content
			msg.Content = &content
		}
		choice := oaChoice{Index: c.Index, Message: msg}
		if c.FinishReason != "" {
			reason := c.FinishReason
			choice.FinishReason = &reason
		}
		out.Choices = append(out.Choices, choice)
	}
	return encode(out)
}

// encodeChunk menyusun satu objek chat.completion.chunk.
//
// delta nil berarti potongan tanpa choice — bentuk yang dipakai OpenAI untuk potongan
// penutup yang hanya membawa usage.
func encodeChunk(id, model string, created int64, delta *oaDelta, finishReason string, usage *providers.Usage) []byte {
	chunk := oaChunk{
		ID:      id,
		Object:  "chat.completion.chunk",
		Created: created,
		Model:   model,
		Choices: []oaChunkChoice{},
		Usage:   encodeUsage(usage),
	}
	if delta != nil {
		choice := oaChunkChoice{Index: 0, Delta: *delta}
		if finishReason != "" {
			reason := finishReason
			choice.FinishReason = &reason
		}
		chunk.Choices = append(chunk.Choices, choice)
	}
	return encode(chunk)
}

// oaEmbeddingList adalah respons embeddings bergaya OpenAI.
type oaEmbeddingList struct {
	Object string        `json:"object"`
	Data   []oaEmbedding `json:"data"`
	Model  string        `json:"model"`
	Usage  *oaUsage      `json:"usage,omitempty"`
}

type oaEmbedding struct {
	Object    string    `json:"object"`
	Index     int       `json:"index"`
	Embedding []float32 `json:"embedding"`
}

// encodeEmbeddings menyusun objek daftar embedding dari respons kanonik.
func encodeEmbeddings(resp *providers.EmbeddingsResponse) json.RawMessage {
	out := oaEmbeddingList{
		Object: "list",
		Data:   make([]oaEmbedding, 0, len(resp.Data)),
		Model:  resp.Model,
	}
	for i := range resp.Data {
		out.Data = append(out.Data, oaEmbedding{
			Object:    "embedding",
			Index:     resp.Data[i].Index,
			Embedding: resp.Data[i].Vector,
		})
	}
	// Gemini tidak melaporkan token pada endpoint embeddings; usage nol tetap disertakan
	// agar bentuknya tidak berbeda dari respons provider lain.
	out.Usage = encodeUsage(&resp.Usage)
	return encode(out)
}
