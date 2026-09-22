package gateway

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
	"github.com/NexGen-X/Route-X/internal/httpx"
	"github.com/NexGen-X/Route-X/internal/providers"
)

type anthropicBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	Thinking  string          `json:"thinking,omitempty"`
	Signature string          `json:"signature,omitempty"`
	Source    *anthropicImage `json:"source,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   json.RawMessage `json:"content,omitempty"`
}

type anthropicImage struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type,omitempty"`
	Data      string `json:"data,omitempty"`
	URL       string `json:"url,omitempty"`
}

type anthropicMsg struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type anthropicTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema"`
}

type anthropicRequest struct {
	Model         string          `json:"model"`
	Messages      []anthropicMsg  `json:"messages"`
	System        json.RawMessage `json:"system,omitempty"`
	MaxTokens     int             `json:"max_tokens"`
	Stream        bool            `json:"stream,omitempty"`
	Temperature   *float64        `json:"temperature,omitempty"`
	TopP          *float64        `json:"top_p,omitempty"`
	StopSequences []string        `json:"stop_sequences,omitempty"`
	Tools         []anthropicTool `json:"tools,omitempty"`
}

type anthropicErrorBody struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

type anthropicErrorEnvelope struct {
	Type  string             `json:"type"`
	Error anthropicErrorBody `json:"error"`
}

func writeAnthropicError(w http.ResponseWriter, status int, errType string, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("anthropic-version", "2023-06-01")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(anthropicErrorEnvelope{
		Type: "error",
		Error: anthropicErrorBody{
			Type:    errType,
			Message: message,
		},
	})
}

func randomMsgID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return "msg_" + hex.EncodeToString(b)
}

func extractToolResultContent(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var blocks []anthropicBlock
	if err := json.Unmarshal(raw, &blocks); err == nil {
		var parts []string
		for _, b := range blocks {
			if b.Type == "text" && b.Text != "" {
				parts = append(parts, b.Text)
			}
		}
		return strings.Join(parts, "\n")
	}
	return string(raw)
}

func decodeAnthropicContent(raw json.RawMessage, role string) (string, []providers.ContentPart, []providers.ToolCall, []providers.Message, error) {
	if len(raw) == 0 {
		return "", nil, nil, nil, nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s, nil, nil, nil, nil
	}
	var blocks []anthropicBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return "", nil, nil, nil, fmt.Errorf("gagal mengurai pesan Anthropic: %w", err)
	}

	var textParts []string
	var contentParts []providers.ContentPart
	var toolCalls []providers.ToolCall
	var extraToolMessages []providers.Message
	var isMultiModal bool

	for _, b := range blocks {
		switch b.Type {
		case "text":
			textParts = append(textParts, b.Text)
			contentParts = append(contentParts, providers.ContentPart{
				Type: providers.PartTypeText,
				Text: b.Text,
			})
		case "image":
			isMultiModal = true
			if b.Source != nil {
				url := b.Source.URL
				if url == "" && b.Source.Data != "" {
					mediaType := b.Source.MediaType
					if mediaType == "" {
						mediaType = "image/jpeg"
					}
					url = "data:" + mediaType + ";base64," + b.Source.Data
				}
				if url != "" {
					contentParts = append(contentParts, providers.ContentPart{
						Type:     providers.PartTypeImageURL,
						ImageURL: &providers.ImageURL{URL: url},
					})
				}
			}
		case "tool_use":
			toolCalls = append(toolCalls, providers.ToolCall{
				ID:   b.ID,
				Type: "function",
				Function: providers.FunctionCall{
					Name:      b.Name,
					Arguments: string(b.Input),
				},
			})
		case "tool_result":
			resText := extractToolResultContent(b.Content)
			extraToolMessages = append(extraToolMessages, providers.Message{
				Role:       providers.RoleTool,
				ToolCallID: b.ToolUseID,
				Content:    resText,
			})
		}
	}

	fullText := strings.Join(textParts, "\n")
	if isMultiModal {
		return fullText, contentParts, toolCalls, extraToolMessages, nil
	}
	return fullText, nil, toolCalls, extraToolMessages, nil
}

// DecodeAnthropicMessagesRequest menerjemahkan permintaan Anthropic Messages API ke bentuk kanonik providers.ChatRequest.
func DecodeAnthropicMessagesRequest(body []byte) (*providers.ChatRequest, error) {
	var aReq anthropicRequest
	if err := json.Unmarshal(body, &aReq); err != nil {
		return nil, fmt.Errorf("body JSON Anthropic tidak sah: %w", err)
	}
	if strings.TrimSpace(aReq.Model) == "" {
		return nil, errors.New("field 'model' wajib diisi")
	}

	out := &providers.ChatRequest{
		Model:       aReq.Model,
		Stream:      aReq.Stream,
		Temperature: aReq.Temperature,
		TopP:        aReq.TopP,
		Stop:        aReq.StopSequences,
	}
	if aReq.MaxTokens > 0 {
		mt := aReq.MaxTokens
		out.MaxTokens = &mt
	}

	// Parsing system prompt
	if len(aReq.System) > 0 {
		var sysStr string
		if err := json.Unmarshal(aReq.System, &sysStr); err == nil {
			if sysStr != "" {
				out.Messages = append(out.Messages, providers.Message{
					Role:    providers.RoleSystem,
					Content: sysStr,
				})
			}
		} else {
			var sysBlocks []anthropicBlock
			if err := json.Unmarshal(aReq.System, &sysBlocks); err == nil {
				var parts []string
				for _, b := range sysBlocks {
					if b.Type == "text" && b.Text != "" {
						parts = append(parts, b.Text)
					}
				}
				if len(parts) > 0 {
					out.Messages = append(out.Messages, providers.Message{
						Role:    providers.RoleSystem,
						Content: strings.Join(parts, "\n\n"),
					})
				}
			}
		}
	}

	// Parsing messages
	for _, m := range aReq.Messages {
		role := providers.Role(m.Role)
		text, parts, toolCalls, extraTools, err := decodeAnthropicContent(m.Content, m.Role)
		if err != nil {
			return nil, err
		}
		if text != "" || len(parts) > 0 || len(toolCalls) > 0 {
			out.Messages = append(out.Messages, providers.Message{
				Role:      role,
				Content:   text,
				Parts:     parts,
				ToolCalls: toolCalls,
			})
		}
		if len(extraTools) > 0 {
			out.Messages = append(out.Messages, extraTools...)
		}
	}

	// Parsing tools
	for _, t := range aReq.Tools {
		out.Tools = append(out.Tools, providers.Tool{
			Type: "function",
			Function: providers.FunctionDef{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.InputSchema,
			},
		})
	}

	return out, nil
}

type anthropicResponseContentBlock struct {
	Type  string          `json:"type"`
	Text  string          `json:"text,omitempty"`
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
}

type anthropicResponse struct {
	ID           string                          `json:"id"`
	Type         string                          `json:"type"`
	Role         string                          `json:"role"`
	Model        string                          `json:"model"`
	Content      []anthropicResponseContentBlock `json:"content"`
	StopReason   *string                         `json:"stop_reason"`
	StopSequence *string                         `json:"stop_sequence"`
	Usage        anthropicUsage                  `json:"usage"`
}

type anthropicUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
}

// AnthropicMessages melayani POST /v1/messages untuk Anthropic Messages API (Claude Code, dll).
func (h *Handlers) AnthropicMessages(w http.ResponseWriter, r *http.Request) {
	j, w := h.mulaiJejak(w, r)
	defer j.selesai(r.Context())

	body, ok := h.bacaBody(w, r)
	if !ok {
		return
	}

	req, err := DecodeAnthropicMessagesRequest(body)
	if err != nil {
		writeAnthropicError(w, http.StatusBadRequest, httpx.ErrTypeInvalidRequest, err.Error())
		return
	}

	pr, ok := h.siapkanChatParsed(w, r, req, body, j)
	if !ok {
		return
	}

	if pr.req.Stream {
		h.anthropicChatMengalir(w, r, pr, j)
		return
	}
	h.anthropicChatSekali(w, r, pr, j)
}

func (h *Handlers) anthropicChatSekali(w http.ResponseWriter, r *http.Request, pr *persiapan, j *jejak) {
	out := Execute(r.Context(), h.exec, pr.plan,
		func(ctx context.Context, c *upstream.RouteCandidate) (*providers.ChatResponse, error) {
			if perr := h.guard.PeriksaBatasProvider(ctx, c.ProviderID, c.ProviderName); perr != nil {
				return nil, perr
			}
			p, err := h.adapter(ctx, c)
			if err != nil {
				return nil, err
			}
			return p.ChatCompletion(ctx, untukKandidat(pr.req, c))
		})

	h.catatRute(r, pr, out.Attempts, out.Candidate)
	j.pasangPercobaan(out.Attempts, out.Candidate)

	// Cascade combo fallback bila Tier 1 gagal
	if out.Err != nil && !pr.decision.Rule.Combo() {
		// Cascade ini memakai tag [combo:...] di description (jalur lama). Aturan combo
		// pipeline jsonb TIDAK memakainya: kandidatnya sudah mencakup seluruh model
		// resep dalam satu loop bercadangan total, sehingga cascade di sini hanya akan
		// mencoba model-model yang sudah habis anggarannya di loop itu.
		if pipeline := ekstrakComboPipeline(pr.decision.Rule); len(pipeline) > 0 {
			for _, tier := range pipeline {
				tOut, rj := h.cobaTierFallbackSekali(r.Context(), tier, pr, j)
				switch {
				case rj != nil:
					writeAnthropicError(w, rj.Status, rj.Type, rj.Message)
					return
				case tOut != nil:
					h.catatRute(r, pr, tOut.Attempts, tOut.Candidate)
					out = tOut
				}
				if out.Err == nil {
					break
				}
			}
		}
	}

	if out.Err != nil {
		j.pasangKegagalanUpstream(out.Err)
		status, errType, _, msg := httpUntuk(providers.AsError(out.Err))
		writeAnthropicError(w, status, errType, msg)
		return
	}

	j.pasangPemakaian(out.Value.Usage)
	h.guard.CatatToken(r.Context(), pr.principal, pr.targets, int64(out.Value.Usage.TotalTokens))

	if rj := h.guard.SaringJawaban(r.Context(), teksJawaban(out.Value),
		pr.model.ID, out.Candidate.ProviderID); rj != nil {
		writeAnthropicError(w, rj.Status, rj.Type, rj.Message)
		return
	}

	msgID := out.Value.ID
	if msgID == "" {
		msgID = randomMsgID()
	} else if !strings.HasPrefix(msgID, "msg_") {
		msgID = "msg_" + strings.TrimPrefix(msgID, "chatcmpl-")
	}

	var contentBlocks []anthropicResponseContentBlock
	stopReason := "end_turn"

	if len(out.Value.Choices) > 0 {
		choice := out.Value.Choices[0]
		if choice.Message.Content != "" {
			contentBlocks = append(contentBlocks, anthropicResponseContentBlock{
				Type: "text",
				Text: choice.Message.Content,
			})
		}
		for _, tc := range choice.Message.ToolCalls {
			inputRaw := json.RawMessage(tc.Function.Arguments)
			if len(inputRaw) == 0 {
				inputRaw = json.RawMessage("{}")
			}
			contentBlocks = append(contentBlocks, anthropicResponseContentBlock{
				Type:  "tool_use",
				ID:    tc.ID,
				Name:  tc.Function.Name,
				Input: inputRaw,
			})
		}
		switch choice.FinishReason {
		case providers.FinishStop:
			stopReason = "end_turn"
		case providers.FinishToolCalls:
			stopReason = "tool_use"
		case providers.FinishLength:
			stopReason = "max_tokens"
		default:
			stopReason = "end_turn"
		}
	}

	if len(contentBlocks) == 0 {
		contentBlocks = append(contentBlocks, anthropicResponseContentBlock{
			Type: "text",
			Text: "",
		})
	}

	resp := anthropicResponse{
		ID:         msgID,
		Type:       "message",
		Role:       "assistant",
		Model:      pr.model.ModelID,
		Content:    contentBlocks,
		StopReason: &stopReason,
		Usage: anthropicUsage{
			InputTokens:          out.Value.Usage.InputTokens,
			OutputTokens:         out.Value.Usage.OutputTokens,
			CacheReadInputTokens: out.Value.Usage.CachedInputTokens,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("anthropic-version", "2023-06-01")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

func (h *Handlers) anthropicChatMengalir(w http.ResponseWriter, r *http.Request, pr *persiapan, j *jejak) {
	out := ExecuteStream(r.Context(), h.exec, pr.plan,
		func(ctx context.Context, c *upstream.RouteCandidate) (providers.Stream, error) {
			if perr := h.guard.PeriksaBatasProvider(ctx, c.ProviderID, c.ProviderName); perr != nil {
				return nil, perr
			}
			p, err := h.adapter(ctx, c)
			if err != nil {
				return nil, err
			}
			return p.ChatCompletionStream(ctx, untukKandidat(pr.req, c))
		})

	h.catatRute(r, pr, out.Attempts, out.Candidate)
	j.pasangPercobaan(out.Attempts, out.Candidate)

	if out.Err != nil && !pr.decision.Rule.Combo() {
		// Cascade ini memakai tag [combo:...] di description (jalur lama). Aturan combo
		// pipeline jsonb TIDAK memakainya: kandidatnya sudah mencakup seluruh model
		// resep dalam satu loop bercadangan total, sehingga cascade di sini hanya akan
		// mencoba model-model yang sudah habis anggarannya di loop itu.
		if pipeline := ekstrakComboPipeline(pr.decision.Rule); len(pipeline) > 0 {
			for _, tier := range pipeline {
				tOut, rj := h.cobaTierFallbackMengalir(r.Context(), tier, pr, j)
				switch {
				case rj != nil:
					writeAnthropicError(w, rj.Status, rj.Type, rj.Message)
					return
				case tOut != nil:
					h.catatRute(r, pr, tOut.Attempts, tOut.Candidate)
					out = tOut
				}
				if out.Err == nil {
					break
				}
			}
		}
	}

	if out.Err != nil {
		j.pasangKegagalanUpstream(out.Err)
		status, errType, _, msg := httpUntuk(providers.AsError(out.Err))
		writeAnthropicError(w, status, errType, msg)
		return
	}

	mulaiAliran := time.Now()
	defer func() { j.tambahHulu(time.Since(mulaiAliran)) }()
	defer func() { _ = out.Value.Close() }()

	w.Header().Set("anthropic-version", "2023-06-01")
	if err := httpx.PrepareSSE(w, r); err != nil {
		h.logger.ErrorContext(r.Context(), "penyiapan SSE gagal", "error", err)
		httpx.InternalError(w, r)
		return
	}

	rc := http.NewResponseController(w)
	var (
		terkirim   int64
		usage      providers.Usage
		msgID      = randomMsgID()
		modelName  = pr.model.ModelID
		stopReason = "end_turn"

		// Mesin blok konten Anthropic. Anthropic menyusun pesan sebagai urutan
		// content_block_start -> content_block_delta* -> content_block_stop, dan
		// tiap blok punya index sendiri di dalam pesan. Bentuk kanonik gateway
		// sebaliknya memisahkan text dari tool_calls, jadi state blok harus
		// dijaga di sini: blok dibuka secara malas saat potongan pertama benar-benar
		// tiba, bukan di awal, supaya pesan tanpa teks tidak mengirim blok hampa.
		indexBlokSekarang int    // index blok yang sedang terbuka
		tipeBlokSekarang  string // "text" atau "tool_use"; kosong berarti belum ada blok
		blokTerbuka       bool
		blokBerikutnya    int             // index untuk content_block berikutnya
		blokTool          = map[int]int{} // index tool kanonik -> index blok keluaran
	)

	// kirimEvent menulis satu peristiwa SSE beserta framingnya lalu memflush klien.
	// Menulis SSE tanpa flush segera menumpuk chunk di buffer proxy dan mematikan
	// tujuan streaming itu sendiri. Kegagalan tulisan dianggap aliran sudah putus:
	// false memberitahu pemanggil untuk berhenti memproses.
	kirimEvent := func(nama string, muat []byte) bool {
		n, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", nama, muat)
		if err != nil {
			return false
		}
		terkirim += int64(n)
		return rc.Flush() == nil
	}

	// tutupBlok menutup content_block yang masih terbuka. Hanya ada satu blok
	// terbuka dalam satu waktu, persis seperti aliran Anthropic yang sesungguhnya.
	tutupBlok := func() {
		if !blokTerbuka {
			return
		}
		muat, _ := json.Marshal(map[string]any{
			"type":  "content_block_stop",
			"index": indexBlokSekarang,
		})
		kirimEvent("content_block_stop", muat)
		blokTerbuka = false
		tipeBlokSekarang = ""
	}

	defer func() {
		if usage.TotalTokens <= 0 && terkirim > 0 {
			lantai := int(terkirim / bytePerToken)
			if lantai < 1 {
				lantai = 1
			}
			usage.OutputTokens = lantai
			usage.TotalTokens = usage.InputTokens + lantai
		}
		h.guard.CatatToken(r.Context(), pr.principal, pr.targets, int64(usage.TotalTokens))
		j.pasangPemakaian(usage)
	}()

	// 1. Kirim event message_start
	startPayload, _ := json.Marshal(map[string]any{
		"type": "message_start",
		"message": map[string]any{
			"id":            msgID,
			"type":          "message",
			"role":          "assistant",
			"model":         modelName,
			"content":       []any{},
			"stop_reason":   nil,
			"stop_sequence": nil,
			"usage": map[string]any{
				"input_tokens":  EstimateTokens(pr.req).InputTokens,
				"output_tokens": 1,
			},
		},
	})
	if _, err := fmt.Fprintf(w, "event: message_start\ndata: %s\n\n", startPayload); err != nil {
		return
	}
	_ = rc.Flush()

	// 2. content block dibuka secara malas di dalam loop baca, bukan di sini: jenis
	// blok pertama dan jumlahnya baru diketahui saat potongan tiba. Membuka blok
	// text di sini membuat pesan tool-only mengirim blok text hampa di index 0.

	// Baca stream peristiwa
	for {
		ev, err := out.Value.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			errPayload, _ := json.Marshal(map[string]any{
				"type": "error",
				"error": map[string]any{
					"type":    "api_error",
					"message": "aliran terputus dari upstream",
				},
			})
			_, _ = fmt.Fprintf(w, "event: error\ndata: %s\n\n", errPayload)
			_ = rc.Flush()
			return
		}

		if ev.Usage != nil {
			usage = *ev.Usage
		}
		if ev.FinishReason != "" {
			switch ev.FinishReason {
			case providers.FinishStop:
				stopReason = "end_turn"
			case providers.FinishToolCalls:
				stopReason = "tool_use"
			case providers.FinishLength:
				stopReason = "max_tokens"
			}
		}

		// Catatan tentang reasoning: ev.ReasoningDelta sengaja TIDAK di-stream ke
		// sini. Anthropic hanya menerima blok thinking yang disertai signature
		// kriptografis dari sisi server; bentuk kanonik gateway tidak membawa
		// signature (providers.StreamEvent tidak punya field Signature), dan
		// adapter Anthropic memang menjatuhkannya secara eksplisit. Memblokir
		// thinking tanpa signature yang sah hanya membuang-buang output, jadi
		// jejak penalaran dilewati pada permukaan ini.

		// Potongan teks: buka blok text bila belum ada blok text yang terbuka.
		// Transisi tool -> text menutup blok tool lama lebih dulu.
		if ev.Delta != "" {
			if tipeBlokSekarang != "text" {
				tutupBlok()
				indexBlokSekarang = blokBerikutnya
				blokBerikutnya++
				blockStartPayload, _ := json.Marshal(map[string]any{
					"type":  "content_block_start",
					"index": indexBlokSekarang,
					"content_block": map[string]any{
						"type": "text",
						"text": "",
					},
				})
				if !kirimEvent("content_block_start", blockStartPayload) {
					return
				}
				blokTerbuka = true
				tipeBlokSekarang = "text"
			}
			deltaPayload, _ := json.Marshal(map[string]any{
				"type":  "content_block_delta",
				"index": indexBlokSekarang,
				"delta": map[string]any{
					"type": "text_delta",
					"text": ev.Delta,
				},
			})
			if !kirimEvent("content_block_delta", deltaPayload) {
				return
			}
			j.tandaiTTFT()
			continue
		}

		// Potongan tool_calls. Adapter mengirim content_block_start tool_use sebagai
		// satu peristiwa (membawa id dan nama, tanpa argumen), lalu setiap potongan
		// argumen sebagai input_json_delta terpisah. Keduanya disusun ulang ke blok
		// tool_use di index yang sama.
		for _, tc := range ev.ToolCalls {
			idx, dikenal := blokTool[tc.Index]
			if !dikenal {
				// Tool baru: tutup blok yang sedang terbuka (text atau tool lain)
				// sebelum membuka blok tool_use di index berikutnya.
				tutupBlok()
				idx = blokBerikutnya
				blokBerikutnya++
				blokTool[tc.Index] = idx
				indexBlokSekarang = idx
				blockStartPayload, _ := json.Marshal(map[string]any{
					"type":  "content_block_start",
					"index": idx,
					"content_block": map[string]any{
						"type":  "tool_use",
						"id":    tc.ID,
						"name":  tc.Function.Name,
						"input": map[string]any{},
					},
				})
				if !kirimEvent("content_block_start", blockStartPayload) {
					return
				}
				blokTerbuka = true
				tipeBlokSekarang = "tool_use"
			}
			// Argumen dikirim sebagai delta JSON sebagaimana adanya, tanpa diurai:
			// model kadang menghasilkan JSON tidak sah yang harus tetap sampai ke
			// klien utuh (lihat dokumentasi providers.FunctionCall.Arguments).
			if tc.Function.Arguments != "" {
				deltaPayload, _ := json.Marshal(map[string]any{
					"type":  "content_block_delta",
					"index": idx,
					"delta": map[string]any{
						"type":         "input_json_delta",
						"partial_json": tc.Function.Arguments,
					},
				})
				if !kirimEvent("content_block_delta", deltaPayload) {
					return
				}
				j.tandaiTTFT()
			}
		}
	}

	// 3. Tutup content block yang masih terbuka saat aliran berakhir.
	tutupBlok()

	// 4. Kirim message_delta
	outputTokens := usage.OutputTokens
	if outputTokens <= 0 {
		outputTokens = int(terkirim / bytePerToken)
		if outputTokens < 1 {
			outputTokens = 1
		}
	}
	msgDeltaPayload, _ := json.Marshal(map[string]any{
		"type": "message_delta",
		"delta": map[string]any{
			"stop_reason":   stopReason,
			"stop_sequence": nil,
		},
		"usage": map[string]any{
			"output_tokens": outputTokens,
		},
	})
	_, _ = fmt.Fprintf(w, "event: message_delta\ndata: %s\n\n", msgDeltaPayload)
	_ = rc.Flush()

	// 5. Kirim message_stop
	_, _ = fmt.Fprintf(w, "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
	_ = rc.Flush()
}
