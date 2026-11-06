package gateway

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/NexGen-X/Route-X/internal/apikey"
	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
	"github.com/NexGen-X/Route-X/internal/httpx"
	"github.com/NexGen-X/Route-X/internal/providers"
	"github.com/NexGen-X/Route-X/internal/responsecache"
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
	// IsError hanya bermakna pada block tool_result.
	IsError bool `json:"is_error,omitempty"`
}

// anthropicImage adalah sumber block image maupun document. Keduanya berbagi
// bentuk: base64 (media_type + data), url, teks polos (data, hanya document),
// atau daftar block (content, hanya document).
type anthropicImage struct {
	Type      string           `json:"type"`
	MediaType string           `json:"media_type,omitempty"`
	Data      string           `json:"data,omitempty"`
	URL       string           `json:"url,omitempty"`
	Content   []anthropicBlock `json:"content,omitempty"`
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
	Model     string          `json:"model"`
	Messages  []anthropicMsg  `json:"messages"`
	System    json.RawMessage `json:"system,omitempty"`
	MaxTokens *int            `json:"max_tokens"`
	Stream    bool            `json:"stream,omitempty"`
	// Temperature, TopP, dan TopK adalah parameter sampling. Ketiganya wajib
	// terbawa ke bentuk kanonik: menguraikannya lalu membuangnya berarti
	// permintaan dijalankan dengan parameter yang BUKAN parameter pengirimnya
	// (audit3 V-GW-17).
	Temperature   *float64           `json:"temperature,omitempty"`
	TopP          *float64           `json:"top_p,omitempty"`
	TopK          *int               `json:"top_k,omitempty"`
	StopSequences []string           `json:"stop_sequences,omitempty"`
	Tools         []anthropicTool    `json:"tools,omitempty"`
	ToolChoice    json.RawMessage    `json:"tool_choice,omitempty"`
	Thinking      *anthropicThinking `json:"thinking,omitempty"`
	Metadata      *anthropicMetadata `json:"metadata,omitempty"`
}

// anthropicThinking adalah parameter extended thinking pada body /v1/messages.
//
// Sub-field selain type/budget_tokens (mis. "display" dari klien modern)
// TIDAK dibuang saat decode (B-137): UnmarshalJSON mempertahankannya mentah
// di Extra agar adapter Anthropic dapat meneruskannya setia ke upstream
// Anthropic. Untuk upstream non-Anthropic seluruh thinking memang dibuang
// secara terdokumentasi (B-096) — Extra mengikuti nasib yang sama.
type anthropicThinking struct {
	Type         string                     `json:"type"`
	BudgetTokens int                        `json:"budget_tokens,omitempty"`
	Extra        map[string]json.RawMessage `json:"-"`
}

// UnmarshalJSON mengurai type/budget_tokens dan menyimpan sub-field lain
// apa adanya di Extra.
func (t *anthropicThinking) UnmarshalJSON(data []byte) error {
	type alias anthropicThinking
	var a alias
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	var mentah map[string]json.RawMessage
	if err := json.Unmarshal(data, &mentah); err != nil {
		return err
	}
	delete(mentah, "type")
	delete(mentah, "budget_tokens")
	*t = anthropicThinking(a)
	if len(mentah) > 0 {
		t.Extra = mentah
	}
	return nil
}

// anthropicMetadata adalah block metadata pada body /v1/messages.
type anthropicMetadata struct {
	UserID string `json:"user_id,omitempty"`
}

type anthropicErrorBody struct {
	Type    string `json:"type"`
	Message string `json:"message"`
	// Code adalah kode mesin milik gateway (bukan bagian dari spec
	// Anthropic, yang hanya mengenal type + message). Ia hanya diisi untuk
	// penolakan yang pemanggilnya perlu bedakan secara programatik;
	// klien Anthropic biasa aman mengabaikannya.
	Code string `json:"code,omitempty"`
}

type anthropicErrorEnvelope struct {
	Type  string             `json:"type"`
	Error anthropicErrorBody `json:"error"`
}

func writeAnthropicError(w http.ResponseWriter, status int, errType string, message string) {
	tulisEnvelopeAnthropic(w, status, anthropicErrorBody{Type: errType, Message: message})
}

// writeAnthropicErrorBerkode menulis error ber-envelope Anthropic yang juga
// membawa kode mesin gateway, untuk penolakan yang dibedakan pemanggil
// secara programatik (mis. dokumen PDF tanpa kandidat Anthropic).
func writeAnthropicErrorBerkode(w http.ResponseWriter, status int, errType, code, message string) {
	tulisEnvelopeAnthropic(w, status, anthropicErrorBody{Type: errType, Code: code, Message: message})
}

func tulisEnvelopeAnthropic(w http.ResponseWriter, status int, body anthropicErrorBody) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("anthropic-version", "2023-06-01")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(anthropicErrorEnvelope{Type: "error", Error: body})
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

// pembangunPesan mengumpulkan isi SATU pesan sumber Anthropic menjadi pesan
// kanonik. Ia ada karena block tool_result harus menjadi pesan berperan tool
// SENDIRI di bentuk kanonik, terpisah dari teks yang mengapitnya — memisahkannya
// belakangan, sesudah seluruh teks dikumpulkan, membalik urutan percakapan
// (audit3 V-GW-25). Pemanggil menyiram pembangun ini setiap kali tool_result
// muncul, sehingga urutan block klien adalah urutan pesan kanonik, persis.
type pembangunPesan struct {
	role       providers.Role
	teks       []string
	parts      []providers.ContentPart
	multimodal bool
	toolCalls  []providers.ToolCall
	reasoning  []string
	signatures []string
}

func (p *pembangunPesan) kosong() bool {
	return len(p.teks) == 0 && len(p.parts) == 0 && len(p.toolCalls) == 0 && len(p.reasoning) == 0
}

func (p *pembangunPesan) jadi() providers.Message {
	m := providers.Message{Role: p.role, ToolCalls: p.toolCalls}
	if p.multimodal {
		m.Parts = p.parts
	} else {
		m.Content = strings.Join(p.teks, "\n")
	}
	if len(p.reasoning) > 0 {
		m.ReasoningContent = strings.Join(p.reasoning, "")
		// Tanda tangan hanya bisa dibawa bila riwayat memuat TEPAT SATU block
		// thinking: tanda tangan mengesahkan isi block-nya sendiri, jadi
		// memasangkan tanda tangan satu block ke gabungan beberapa block akan
		// ditolak upstream. Sisanya adapter membuang jejak penalarannya juga —
		// keduanya berjalan bersama atau tidak sama sekali (audit3 V-GW-23).
		if len(p.signatures) == 1 && len(p.reasoning) == 1 {
			m.ReasoningSignature = p.signatures[0]
		}
	}
	return m
}

func (p *pembangunPesan) tambahTeks(teks string) {
	p.teks = append(p.teks, teks)
	p.parts = append(p.parts, providers.ContentPart{Type: providers.PartTypeText, Text: teks})
}

// tambahDokumen melampirkan satu bagian dokumen biner (PDF) pada posisi
// block-nya di dalam pesan. Berbeda dari tambahTeks, dokumen TIDAK masuk
// daftar teks: ia tidak punya padanan teks dan harus tiba di adapter
// Anthropic sebagai bagiannya sendiri (Batch C2).
func (p *pembangunPesan) tambahDokumen(dok *providers.Document) {
	p.multimodal = true
	p.parts = append(p.parts, providers.ContentPart{Type: providers.PartTypeDocument, Document: dok})
}

// maksByteDokumen adalah batas ukuran SATU dokumen sesudah base64 didekode.
//
// Batasnya dinyatakan di sini, bukan diwarisi diam-diam dari batas body:
// body gateway menampung 32 MiB untuk SELURUH permintaan (dan base64
// mengembang ±4/3), jadi dokumen tunggal 10 MiB menyisakan ruang bagi
// beberapa lampiran plus percakapannya, sementara dokumen di atas itu
// hampir pasti berkas yang salah tempat di badan chat.
const maksByteDokumen = 10 << 20

// hasilDokumen adalah keluaran terjemahan satu block document: tepat satu
// dari teks / dok yang terisi.
//
//   - teks: dokumen yang setia sebagai teks di semua dialek (sumber teks,
//     sumber content, dan base64 text/plain yang didekode). Ia diinjeksi ke
//     pesan kanonik sebagai teks biasa, pada posisi block-nya.
//   - dok: dokumen biner (PDF base64) yang berjalan sebagai bagian kanonik
//     sendiri sampai adapter Anthropic — satu-satunya adapter yang bisa
//     meneruskannya utuh (Batch C2).
type hasilDokumen struct {
	teks string
	dok  *providers.Document
}

// terjemahkanDokumen menerjemahkan sumber satu block document.
//
// Yang tidak didukung ditolak eksplisit, tidak pernah diabaikan diam-diam —
// model yang menjawab tanpa lampirannya lebih berbahaya daripada penolakan
// yang jelas (audit3 V-GW-22):
//
//   - sumber "url": gateway TIDAK mengambil berkas dari URL pilihan klien
//     (itu SSRF yang dijaga di seluruh repositori ini), jadi ditolak walau
//     API Anthropic sendiri menerimanya.
//   - sumber lain (file, dsb.) dan media_type di luar yang didukung: ditolak
//     dengan menyebut alasannya.
func terjemahkanDokumen(b anthropicBlock) (hasilDokumen, error) {
	if b.Source == nil {
		return hasilDokumen{}, errors.New("block 'document' tanpa sumber tidak bisa diterjemahkan")
	}
	switch b.Source.Type {
	case "text":
		if mt := b.Source.MediaType; mt != "" && mt != "text/plain" {
			return hasilDokumen{}, fmt.Errorf("block 'document' bersumber teks dengan media_type %q tidak didukung; sumber teks hanya menerima text/plain", mt)
		}
		return hasilDokumen{teks: b.Source.Data}, nil
	case "content":
		var parts []string
		for _, sub := range b.Source.Content {
			if sub.Type == "text" && sub.Text != "" {
				parts = append(parts, sub.Text)
			}
		}
		return hasilDokumen{teks: strings.Join(parts, "\n")}, nil
	case "base64":
		return dokumenBase64(b.Source)
	case "url":
		return hasilDokumen{}, errors.New("block 'document' dengan sumber \"url\" tidak didukung gateway ini; kirim isi dokumen sebagai teks atau base64")
	default:
		return hasilDokumen{}, fmt.Errorf("block 'document' dengan sumber %q tidak didukung gateway ini; kirim isi dokumen sebagai teks atau base64", b.Source.Type)
	}
}

// dokumenBase64 mengurai sumber base64 satu block document.
func dokumenBase64(src *anthropicImage) (hasilDokumen, error) {
	// Spasi dan baris baru di dalam base64 ditoleransi (sebagian klien
	// membungkus base64 per baris); selain itu penguraiannya ketat.
	bersih := strings.Map(func(r rune) rune {
		if r == ' ' || r == '\n' || r == '\r' || r == '\t' {
			return -1
		}
		return r
	}, src.Data)
	// Perkiraan ukuran terdékode diperiksa SEBELUM mendekode sungguhan,
	// supaya lampiran raksasa ditolak tanpa mengalokasikan isinya.
	if len(bersih)/4*3 > maksByteDokumen {
		return hasilDokumen{}, fmt.Errorf("dokumen melebihi batas ukuran %d byte sesudah didekode", maksByteDokumen)
	}
	mentah, err := base64.StdEncoding.DecodeString(bersih)
	if err != nil {
		return hasilDokumen{}, errors.New("block 'document' memuat data base64 yang tidak sah")
	}
	if len(mentah) > maksByteDokumen {
		return hasilDokumen{}, fmt.Errorf("dokumen melebihi batas ukuran %d byte sesudah didekode", maksByteDokumen)
	}

	switch src.MediaType {
	case "application/pdf":
		if len(mentah) == 0 {
			return hasilDokumen{}, errors.New("block 'document' PDF tidak memuat data")
		}
		// Data yang disimpan adalah base64 BERSIH (tanpa jeda baris), supaya
		// bentuk kanoniknya satu-satunya dan sidiknya stabil.
		return hasilDokumen{dok: &providers.Document{MediaType: "application/pdf", Data: bersih}}, nil
	case "text/plain":
		if !utf8.Valid(mentah) {
			return hasilDokumen{}, errors.New("block 'document' base64 text/plain tidak didekode menjadi teks UTF-8 yang sah")
		}
		return hasilDokumen{teks: string(mentah)}, nil
	default:
		return hasilDokumen{}, fmt.Errorf("block 'document' base64 dengan media_type %q tidak didukung; yang didukung application/pdf dan text/plain", src.MediaType)
	}
}

// decodePesanAnthropic menerjemahkan satu pesan Anthropic menjadi satu atau
// lebih pesan kanonik, menjaga urutan block.
func decodePesanAnthropic(m anthropicMsg) ([]providers.Message, error) {
	role := providers.Role(m.Role)
	if len(m.Content) == 0 {
		return nil, nil
	}
	var s string
	if err := json.Unmarshal(m.Content, &s); err == nil {
		if s == "" {
			return nil, nil
		}
		return []providers.Message{{Role: role, Content: s}}, nil
	}
	var blocks []anthropicBlock
	if err := json.Unmarshal(m.Content, &blocks); err != nil {
		return nil, fmt.Errorf("gagal mengurai pesan Anthropic: %w", err)
	}

	pb := &pembangunPesan{role: role}
	var hasil []providers.Message
	siram := func() {
		if !pb.kosong() {
			hasil = append(hasil, pb.jadi())
			pb = &pembangunPesan{role: role}
		}
	}

	for _, b := range blocks {
		switch b.Type {
		case "text":
			pb.tambahTeks(b.Text)
		case "image":
			pb.multimodal = true
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
					pb.parts = append(pb.parts, providers.ContentPart{
						Type:     providers.PartTypeImageURL,
						ImageURL: &providers.ImageURL{URL: url},
					})
				}
			}
		case "document":
			hasil, err := terjemahkanDokumen(b)
			if err != nil {
				return nil, err
			}
			if hasil.dok != nil {
				pb.tambahDokumen(hasil.dok)
			} else if hasil.teks != "" {
				pb.tambahTeks(hasil.teks)
			}
		case "thinking":
			// Jejak penalaran riwayat assistant dibawa beserta tanda tangannya
			// supaya adapter Anthropic bisa mengirimkannya kembali utuh
			// (audit3 V-GW-23).
			pb.reasoning = append(pb.reasoning, b.Thinking)
			if b.Signature != "" {
				pb.signatures = append(pb.signatures, b.Signature)
			}
		case "redacted_thinking":
			// Muatannya terenkripsi dan hanya bisa dipakai ulang oleh Anthropic
			// sebagai block yang sama persis; bentuk kanonik tidak punya tempat
			// untuknya. Berbeda dari thinking biasa, di sini tidak ada isi yang
			// hilang dari sudut pandang model — block ini memang buram.
			continue
		case "tool_use":
			pb.toolCalls = append(pb.toolCalls, providers.ToolCall{
				ID:   b.ID,
				Type: "function",
				Function: providers.FunctionCall{
					Name:      b.Name,
					Arguments: string(b.Input),
				},
			})
		case "tool_result":
			// Siram dulu isi yang mendahului, barulah hasil tool menjadi pesan
			// sendiri: [tool_result, text] harus tetap hasil-tool mendahului
			// teks, bukan sebaliknya (audit3 V-GW-25). is_error ikut dibawa —
			// menghilangkannya mengubah kegagalan menjadi keberhasilan di mata
			// model (audit3 V-GW-24).
			siram()
			hasil = append(hasil, providers.Message{
				Role:       providers.RoleTool,
				ToolCallID: b.ToolUseID,
				Content:    extractToolResultContent(b.Content),
				IsError:    b.IsError,
			})
		default:
			// Block yang tidak dikenal TIDAK diabaikan diam-diam: klien harus
			// tahu bahwa sebagian permintaannya tidak bisa diterjemahkan,
			// bukan menerima jawaban atas permintaan yang berbeda.
			return nil, fmt.Errorf("block konten bertipe %q belum didukung gateway ini", b.Type)
		}
	}
	siram()
	return hasil, nil
}

// teksSystemAnthropic mengekstrak teks dari field system (string atau daftar
// block teks) pada body Anthropic.
func teksSystemAnthropic(raw json.RawMessage) string {
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
		return strings.Join(parts, "\n\n")
	}
	return ""
}

// DecodeAnthropicMessagesRequest menerjemahkan permintaan Anthropic Messages API ke bentuk kanonik providers.ChatRequest.
func DecodeAnthropicMessagesRequest(body []byte) (*providers.ChatRequest, error) {
	return decodeAnthropicMessagesRequest(body, true)
}

// decodeAnthropicMessagesRequest adalah pengurai bersama permukaan messages.
//
// wajibMaxTokens membedakan dua pemanggil: /v1/messages mewajibkan max_tokens
// (spec Anthropic dan OpenAPI repo sama-sama menyatakannya required, jadi
// ketiadaannya ditolak, bukan diganti diam-diam dengan 4096 — audit3 V-GW-30),
// sedangkan /v1/messages/count_tokens tidak pernah memintanya.
func decodeAnthropicMessagesRequest(body []byte, wajibMaxTokens bool) (*providers.ChatRequest, error) {
	var aReq anthropicRequest
	if err := json.Unmarshal(body, &aReq); err != nil {
		return nil, fmt.Errorf("body JSON Anthropic tidak sah: %w", err)
	}
	if strings.TrimSpace(aReq.Model) == "" {
		return nil, errors.New("field 'model' wajib diisi")
	}
	if wajibMaxTokens {
		if aReq.MaxTokens == nil {
			return nil, errors.New("field 'max_tokens' wajib diisi pada permukaan Anthropic")
		}
		if *aReq.MaxTokens <= 0 {
			return nil, errors.New("field 'max_tokens' harus lebih besar dari 0")
		}
	}

	out := &providers.ChatRequest{
		Model:       aReq.Model,
		Stream:      aReq.Stream,
		Temperature: aReq.Temperature,
		TopP:        aReq.TopP,
		TopK:        aReq.TopK,
		Stop:        aReq.StopSequences,
	}
	if aReq.MaxTokens != nil && *aReq.MaxTokens > 0 {
		out.MaxTokens = aReq.MaxTokens
	}

	// Seluruh parameter sah klien wajib terbawa ke bentuk kanonik. Versi lama
	// pengurai ini tidak punya field untuk keempatnya, jadi tool_choice,
	// top_k, metadata.user_id, dan thinking terbuang pada saat decode — sebelum
	// adapter mana pun bisa melihatnya (audit3 V-GW-17).
	if aReq.Metadata != nil && aReq.Metadata.UserID != "" {
		out.User = aReq.Metadata.UserID
	}
	if aReq.Thinking != nil && aReq.Thinking.Type != "" {
		switch aReq.Thinking.Type {
		case "enabled", "disabled", "adaptive":
			// "adaptive" DITERIMA di sini (B-096): decode belum tahu upstream
			// mana yang akan melayani — kandidat baru dipilih routing
			// sesudahnya — jadi menolaknya di sini menghukum semua rute
			// untuk keterbatasan sebagian upstream. Nilai kanoniknya
			// dibawa apa adanya; keputusan meneruskan atau membuangnya
			// diambil di titik yang tahu targetnya: untukKandidat (per
			// kandidat) dan adapter masing-masing upstream.
			out.Thinking = &providers.ThinkingParam{
				Type:         aReq.Thinking.Type,
				BudgetTokens: aReq.Thinking.BudgetTokens,
				Extra:        aReq.Thinking.Extra,
			}
		default:
			return nil, fmt.Errorf("thinking.type %q tidak dikenal; gunakan \"enabled\", \"disabled\", atau \"adaptive\"", aReq.Thinking.Type)
		}
	}
	if len(aReq.ToolChoice) > 0 {
		pilihan, parallel, err := terjemahkanToolChoiceAnthropic(aReq.ToolChoice)
		if err != nil {
			return nil, err
		}
		out.ToolChoice = pilihan
		if parallel != nil {
			out.ParallelToolCalls = parallel
		}
	}

	// System prompt: field tingkat atas DAN pesan berperan system di tengah
	// daftar messages. Permukaan Anthropic tidak mengenal peran system di
	// dalam messages — tempatnya hanya field system — jadi pesan seperti itu
	// diangkat ke system prompt di sini, pada saat decode, untuk SEMUA rute.
	// Sebelumnya pengangkatan hanya terjadi di adapter Anthropic, sehingga
	// permintaan yang sama berarti dua hal berbeda tergantung upstream yang
	// melayaninya (audit3 V-GW-33).
	var sistem []string
	if t := teksSystemAnthropic(aReq.System); t != "" {
		sistem = append(sistem, t)
	}

	for _, m := range aReq.Messages {
		if m.Role == "system" {
			msgs, err := decodePesanAnthropic(m)
			if err != nil {
				return nil, err
			}
			for _, sm := range msgs {
				if t := sm.Text(); t != "" {
					sistem = append(sistem, t)
				}
			}
			continue
		}
		msgs, err := decodePesanAnthropic(m)
		if err != nil {
			return nil, err
		}
		out.Messages = append(out.Messages, msgs...)
	}

	if len(sistem) > 0 {
		out.Messages = append([]providers.Message{{
			Role:    providers.RoleSystem,
			Content: strings.Join(sistem, "\n\n"),
		}}, out.Messages...)
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

// terjemahkanToolChoiceAnthropic mengubah tool_choice dialek Anthropic ke
// bentuk kanonik bergaya OpenAI yang disimpan ChatRequest.ToolChoice.
//
// Peta maknanya: auto→"auto", any→"required" (OpenAI menyebut pemaksaan
// memanggil salah satu tool dengan nama itu), none→"none", dan tool dengan
// nama→objek function bernama sama. disable_parallel_tool_use adalah
// kebalikan persis parallel_tool_calls kanonik.
func terjemahkanToolChoiceAnthropic(raw json.RawMessage) (json.RawMessage, *bool, error) {
	var tc struct {
		Type                   string `json:"type"`
		Name                   string `json:"name"`
		DisableParallelToolUse bool   `json:"disable_parallel_tool_use"`
	}
	if err := json.Unmarshal(raw, &tc); err != nil {
		return nil, nil, fmt.Errorf("tool_choice Anthropic tidak sah: %w", err)
	}
	var parallel *bool
	if tc.DisableParallelToolUse {
		f := false
		parallel = &f
	}
	switch tc.Type {
	case "auto":
		return json.RawMessage(`"auto"`), parallel, nil
	case "none":
		return json.RawMessage(`"none"`), parallel, nil
	case "any":
		return json.RawMessage(`"required"`), parallel, nil
	case "tool":
		if tc.Name == "" {
			return nil, nil, errors.New("tool_choice bertipe \"tool\" wajib menyebut nama tool pada field 'name'")
		}
		b, err := json.Marshal(map[string]any{
			"type":     "function",
			"function": map[string]any{"name": tc.Name},
		})
		if err != nil {
			return nil, nil, err
		}
		return b, parallel, nil
	default:
		return nil, nil, fmt.Errorf("tool_choice.type %q tidak dikenal; gunakan \"auto\", \"any\", \"tool\", atau \"none\"", tc.Type)
	}
}

type anthropicResponseContentBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	Thinking  string          `json:"thinking,omitempty"`
	Signature string          `json:"signature,omitempty"`
	Source    *anthropicImage `json:"source,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
}

// usageAnthropicDariKanonik mengubah usage kanonik menjadi bentuk spec
// Anthropic.
//
// Bentuk kanonik bersarang gaya OpenAI: InputTokens MEMUAT token cache, dan
// CachedInputTokens/CacheCreationInputTokens adalah bagiannya. Spec Anthropic
// sebaliknya: input_tokens adalah token input segar SAJA, dan token cache
// dilaporkan pada fieldnya sendiri. Menyalin InputTokens apa adanya membuat
// klien yang patuh spec menghitung token cache dua kali, dan tidak melaporkan
// cache_creation sama sekali mengubur angka yang benar-benar ditagih
// (audit3 V-GW-26).
func usageAnthropicDariKanonik(u providers.Usage) anthropicUsage {
	masuk := u.InputTokens - u.CachedInputTokens - u.CacheCreationInputTokens
	if masuk < 0 {
		masuk = 0
	}
	out := anthropicUsage{
		InputTokens:              masuk,
		OutputTokens:             u.OutputTokens,
		CacheCreationInputTokens: u.CacheCreationInputTokens,
		CacheReadInputTokens:     u.CachedInputTokens,
	}
	// Rincian TTL tulis cache diteruskan bila kanonik membawanya, persis
	// bentuk objek cache_creation pada spec Anthropic; tanpa rincian field
	// ini hilang (omitempty), bukan objek bernol yang menyiratkan upstream
	// melaporkannya.
	if u.CacheCreation1hInputTokens > 0 || u.CacheCreation5mInputTokens > 0 {
		out.CacheCreation = &anthropicCacheCreation{
			Ephemeral1hInputTokens: u.CacheCreation1hInputTokens,
			Ephemeral5mInputTokens: u.CacheCreation5mInputTokens,
		}
	}
	return out
}

// blokGambarDariPart mengubah satu bagian gambar kanonik menjadi block image
// Anthropic, membongkar data URI kembali ke sumber base64-nya.
func blokGambarDariPart(part providers.ContentPart) (anthropicResponseContentBlock, bool) {
	if part.ImageURL == nil || part.ImageURL.URL == "" {
		return anthropicResponseContentBlock{}, false
	}
	url := part.ImageURL.URL
	if strings.HasPrefix(url, "data:") {
		meta, data, ditemukan := strings.Cut(url[len("data:"):], ",")
		if ditemukan && strings.HasSuffix(meta, ";base64") {
			mediaType := strings.TrimSuffix(meta, ";base64")
			if i := strings.IndexByte(mediaType, ';'); i >= 0 {
				mediaType = mediaType[:i]
			}
			return anthropicResponseContentBlock{
				Type:   "image",
				Source: &anthropicImage{Type: "base64", MediaType: mediaType, Data: data},
			}, true
		}
	}
	return anthropicResponseContentBlock{
		Type:   "image",
		Source: &anthropicImage{Type: "url", URL: url},
	}, true
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
	// CacheCreation adalah rincian TTL token tulis cache; hanya hadir bila
	// upstream melaporkannya (lihat usageAnthropicDariKanonik).
	CacheCreation *anthropicCacheCreation `json:"cache_creation,omitempty"`
}

// anthropicCacheCreation adalah objek cache_creation pada usage spec
// Anthropic: pecahan token tulis cache menurut TTL-nya.
type anthropicCacheCreation struct {
	Ephemeral1hInputTokens int `json:"ephemeral_1h_input_tokens"`
	Ephemeral5mInputTokens int `json:"ephemeral_5m_input_tokens"`
}

// penangkapRespons menahan seluruh keluaran HTTP selama tahap persiapan
// permukaan Anthropic.
//
// Tahap persiapan memakai jalur bersama permukaan OpenAI (bacaBody,
// siapkanChatParsed), dan jalur itu menjawab setiap penolakan ber-envelope
// OpenAI. Klien Anthropic tidak bisa membaca envelope itu — bentuk error
// adalah bagian dari kontrak permukaan ini (audit3 V-GW-29). Menulis ulang
// seluruh jalur persiapan hanya untuk bentuk error akan menggandakan logika
// yang paling rawan menyimpang, jadi keluaran persiapan DITAHAN di sini lalu
// ditulis ulang sebagai envelope Anthropic oleh teruskanSebagaiErrorAnthropic.
// Persiapan yang berhasil tidak menulis satu byte pun, jadi penangkapan ini
// tidak mengubah jalur sukses sama sekali.
type penangkapRespons struct {
	header http.Header
	status int
	isi    bytes.Buffer
}

func penangkapBaru() *penangkapRespons {
	return &penangkapRespons{header: make(http.Header)}
}

func (p *penangkapRespons) Header() http.Header { return p.header }

func (p *penangkapRespons) WriteHeader(status int) {
	if p.status == 0 {
		p.status = status
	}
}

func (p *penangkapRespons) Write(b []byte) (int, error) {
	if p.status == 0 {
		p.status = http.StatusOK
	}
	return p.isi.Write(b)
}

// tipeErrorAnthropic memetakan status HTTP (dan kode internal bila dikenal) ke
// tipe error dalam kosakata Anthropic.
func tipeErrorAnthropic(status int, kode string) string {
	switch {
	case status == http.StatusBadRequest:
		return "invalid_request_error"
	case status == http.StatusRequestEntityTooLarge:
		return "request_too_large"
	case status == http.StatusUnauthorized:
		return "authentication_error"
	case status == http.StatusPaymentRequired:
		return "billing_error"
	case status == http.StatusForbidden:
		return "permission_error"
	case status == http.StatusNotFound:
		return "not_found_error"
	case status == http.StatusTooManyRequests:
		return "rate_limit_error"
	case status == http.StatusServiceUnavailable && kode == "upstream_overloaded":
		return "overloaded_error"
	default:
		return "api_error"
	}
}

// tipeErrorAnthropicUntukUpstream memetakan kegagalan upstream ke tipe error
// Anthropic, untuk peristiwa error di tengah aliran maupun jawaban persiapan.
func tipeErrorAnthropicUntukUpstream(e *providers.Error) string {
	if e == nil {
		return "api_error"
	}
	switch e.Kind {
	case providers.ErrKindRateLimit:
		return "rate_limit_error"
	case providers.ErrKindOverloaded:
		return "overloaded_error"
	case providers.ErrKindInvalidRequest, providers.ErrKindContentFilter:
		return "invalid_request_error"
	case providers.ErrKindModelNotFound:
		return "not_found_error"
	case providers.ErrKindAuth:
		return "authentication_error"
	case providers.ErrKindPermission:
		return "permission_error"
	case providers.ErrKindQuota:
		return "billing_error"
	default:
		return "api_error"
	}
}

// teruskanSebagaiErrorAnthropic menulis ulang respons error yang tertangkap
// selama persiapan (ber-envelope OpenAI) sebagai envelope Anthropic
// {"type":"error","error":{...}}, dengan status dan pesan yang sama.
func teruskanSebagaiErrorAnthropic(w http.ResponseWriter, p *penangkapRespons) {
	status := p.status
	if status == 0 {
		status = http.StatusInternalServerError
	}
	// Header yang bermakna bagi klien diteruskan apa adanya — Retry-After dan
	// X-RateLimit-* adalah bagian dari kontrak penolakannya. Content-Type dan
	// Content-Length tidak: envelope baru menyusunnya sendiri.
	for k, vs := range p.header {
		if strings.EqualFold(k, "Content-Type") || strings.EqualFold(k, "Content-Length") {
			continue
		}
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}

	pesan, kode := "", ""
	var env struct {
		Error struct {
			Message string  `json:"message"`
			Code    *string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(p.isi.Bytes(), &env); err == nil {
		pesan = env.Error.Message
		if env.Error.Code != nil {
			kode = *env.Error.Code
		}
	}
	if pesan == "" {
		pesan = "terjadi kesalahan pada tahap persiapan permintaan"
	}
	writeAnthropicError(w, status, tipeErrorAnthropic(status, kode), pesan)
}

// mengandungDokumenPDF melaporkan apakah permintaan kanonik ini memuat
// satu pun bagian dokumen biner (PDF base64).
func mengandungDokumenPDF(req *providers.ChatRequest) bool {
	if req == nil {
		return false
	}
	for i := range req.Messages {
		for _, p := range req.Messages[i].Parts {
			if p.Type == providers.PartTypeDocument && p.Document != nil {
				return true
			}
		}
	}
	return false
}

// kandidatAnthropicSaja menyaring kandidat ke provider berkind Anthropic
// saja, mempertahankan urutan routing aslinya.
func kandidatAnthropicSaja(cands []*upstream.RouteCandidate) []*upstream.RouteCandidate {
	out := make([]*upstream.RouteCandidate, 0, len(cands))
	for _, c := range cands {
		if c.Kind == providers.KindAnthropic {
			out = append(out, c)
		}
	}
	return out
}

// AnthropicMessages melayani POST /v1/messages untuk Anthropic Messages API (Claude Code, dll).
func (h *Handlers) AnthropicMessages(w http.ResponseWriter, r *http.Request) {
	j, w := h.mulaiJejak(w, r)
	defer j.selesai(r.Context())

	// Penolakan pra-upstream mengembalikan kuota Limit() (audit3 V-AB-17).
	jaga := h.jagaKuotaPraUpstream(r)
	defer jaga.tutup()

	// Persiapan berjalan di atas penangkap: penolakannya ditulis ulang
	// ber-envelope Anthropic, bukan OpenAI (audit3 V-GW-29).
	tangkap := penangkapBaru()
	body, ok := h.bacaBody(tangkap, r)
	if !ok {
		teruskanSebagaiErrorAnthropic(w, tangkap)
		return
	}

	req, err := DecodeAnthropicMessagesRequest(body)
	if err != nil {
		writeAnthropicError(w, http.StatusBadRequest, httpx.ErrTypeInvalidRequest, err.Error())
		return
	}

	pr, ok := h.siapkanChatParsed(tangkap, r, req, body, j)
	if !ok {
		teruskanSebagaiErrorAnthropic(w, tangkap)
		return
	}

	// Dokumen PDF hanya bisa diteruskan utuh oleh upstream Anthropic; dialek
	// lain tidak punya padanannya di gateway ini. Kandidat dibatasi ke
	// Anthropic SEBELUM eksekusi — bukan sesudah pemenang diketahui — supaya
	// PDF tidak pernah terkirim ke upstream yang akan membuangnya, dan
	// penolakannya terjadi sebelum satu byte pun menyentuh jaringan
	// (Batch C2). Penyaringan yang sama berlaku di jalur fallback tier
	// (siapkanTierFallback).
	if mengandungDokumenPDF(pr.req) {
		anthropic := kandidatAnthropicSaja(pr.plan.Candidates)
		if len(anthropic) == 0 {
			writeAnthropicErrorBerkode(w, http.StatusBadRequest, httpx.ErrTypeInvalidRequest,
				"unsupported_document_upstream",
				"dokumen PDF hanya bisa dilayani provider Anthropic, dan tidak ada kandidat Anthropic yang tersedia untuk model ini")
			return
		}
		pr.plan.Candidates = anthropic
	}
	jaga.lolos()

	if pr.req.Stream {
		h.anthropicChatMengalir(w, r, pr, j)
		return
	}
	h.anthropicChatSekali(w, r, pr, j)
}

// AnthropicCountTokens melayani POST /v1/messages/count_tokens.
//
// Endpoint ini bagian dari kontrak Messages API — klien resmi (Claude Code)
// memanggilnya untuk menaksir konteks sebelum mengirim. Sebelumnya rute ini
// tidak terdaftar sama sekali dan menjawab 404 (audit3 V-GW-29). Penghitungnya
// EstimateTokens yang sama dengan yang dipakai persiapan routing: sebuah
// TAKSIRAN, sama seperti yang dijanjikan nama endpoint-nya, dan model yang
// tidak dikenal tetap ditolak — menghitung token untuk model hantu hanya
// mengarang angka.
//
// Dua koreksi B-170: rutenya tinggal di luar grup pembatas laju (lihat
// Routes — ia tidak menyentuh upstream, jadi tidak boleh membakar kuota
// laju key), dan izin model pada key diperiksa seperti jalur chat.
func (h *Handlers) AnthropicCountTokens(w http.ResponseWriter, r *http.Request) {
	tangkap := penangkapBaru()
	body, ok := h.bacaBody(tangkap, r)
	if !ok {
		teruskanSebagaiErrorAnthropic(w, tangkap)
		return
	}
	req, err := decodeAnthropicMessagesRequest(body, false)
	if err != nil {
		writeAnthropicError(w, http.StatusBadRequest, httpx.ErrTypeInvalidRequest, err.Error())
		return
	}
	model, ok := h.selesaikanModel(tangkap, r, req.Model)
	if !ok {
		teruskanSebagaiErrorAnthropic(w, tangkap)
		return
	}
	// Izin model diperiksa persis seperti jalur chat (B-170): endpoint ini
	// luput sebelumnya karena dianggap remeh, sehingga key yang dibatasi
	// model tetap bisa memastikan model terlarangnya ada beserta taksiran
	// tokennya. Penolakan ditulis ke penangkap supaya envelope-nya tetap
	// Anthropic, sama seperti penolakan resolusi di atas.
	p, _ := apikey.PrincipalFrom(r.Context())
	if !h.izinModel(tangkap, r, p, model) {
		teruskanSebagaiErrorAnthropic(w, tangkap)
		return
	}
	w.Header().Set("anthropic-version", "2023-06-01")
	_ = httpx.JSON(w, http.StatusOK, map[string]any{
		"input_tokens": EstimateTokens(req).InputTokens,
	})
}

func (h *Handlers) anthropicChatSekali(w http.ResponseWriter, r *http.Request, pr *persiapan, j *jejak) {
	// Response cache tahap awal (Batch C1) juga melayani permukaan
	// Anthropic non-streaming, dengan kunci berpermukaan "anthropic":
	// tubuh yang disimpan adalah tubuh dialek Anthropic yang disusun
	// handler ini, bukan Raw upstream dialek OpenAI. Syarat HIT lebih
	// ketat daripada permukaan OpenAI — entri wajib membawa Teks, karena
	// pengekstrak teks dari Raw hanya memahami dialek OpenAI; entri tanpa
	// teks diperlakukan sebagai MISS, bukan disajikan tanpa penyaring.
	var cacheKey string
	if h.respCache != nil && h.respCache.IsEnabled() && layakCacheRespons(pr.req) {
		cacheKey = h.respCache.ComputeScopedKey(pr.req, cakupanCache(pr, permukaanAnthropic))
		if entry, hit, err := h.respCache.Get(r.Context(), cacheKey); err == nil && hit && entry != nil && len(entry.Raw) > 0 && entry.Teks != "" {
			pemakaian := pemakaianCacheHIT(pr.req, entry)
			j.pasangPemakaian(pemakaian)
			if menang := kandidatCacheHIT(pr, entry); menang != nil {
				j.pasangPercobaan(nil, menang)
			}
			h.guard.CatatToken(r.Context(), pr.principal, pr.targets, int64(pemakaian.TotalTokens))
			if rj := h.guard.SaringJawaban(r.Context(), entry.Teks,
				pr.model.ID, entry.ProviderID); rj != nil {
				writeAnthropicError(w, rj.Status, rj.Type, rj.Message)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("anthropic-version", "2023-06-01")
			w.Header().Set("Cache-Control", "no-store")
			j.tandaiCacheHit()
			w.Header().Set("X-RouteX-Cache", "HIT")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(entry.Raw)
			return
		}
	}

	out := Execute(h.guard.DenganMemoBatas(r.Context()), h.exec, pr.plan,
		func(ctx context.Context, c *upstream.RouteCandidate) (*providers.ChatResponse, error) {
			if perr := h.guard.PeriksaBatasProvider(ctx, c.ProviderID, c.ProviderName); perr != nil {
				return nil, perr
			}
			if perr := h.guard.PeriksaKolomProvider(ctx, c); perr != nil {
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
		perr := providers.AsError(out.Err)
		status, _, _, msg := httpUntuk(perr)
		// Status + pesan dari httpUntuk, tetapi TYPE dari pemeta Anthropic
		// (B-169): errType httpUntuk adalah kosakata OpenAI —
		// invalid_request_error untuk model tak dikenal, api_error untuk
		// auth/permission/quota — yang bukan tipe kontrak Anthropic.
		// Kegagalan yang sama di tengah aliran sudah memakai pemeta ini;
		// dua titik pra-aliran ini menyusul supaya satu kegagalan punya
		// satu type di permukaan ini, fase apa pun ia gagalnya.
		writeAnthropicError(w, status, tipeErrorAnthropicUntukUpstream(perr), msg)
		return
	}

	// Lantai pemakaian yang sama seperti permukaan OpenAI (audit3 V-GW-10):
	// respons tanpa usage tetap memakan kuota dan tertagih.
	pemakaian := terapkanLantaiPemakaian(pr.req, out.Value.Usage, int64(len(teksJawaban(out.Value))))
	j.pasangPemakaian(pemakaian)
	h.guard.CatatToken(r.Context(), pr.principal, pr.targets, int64(pemakaian.TotalTokens))
	h.guard.CatatTokenKolomProvider(r.Context(), out.Candidate, int64(pemakaian.TotalTokens))

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
	var stopSequence *string

	if len(out.Value.Choices) > 0 {
		choice := out.Value.Choices[0]
		// Block thinking mendahului isi lain, sesuai susunan Anthropic. Ia
		// hanya diterbitkan bila upstream benar-benar mengembalikan jejak
		// penalaran — sebelumnya jejak itu dibuang dan klien tidak pernah
		// melihat block thinking pada jalur non-streaming (audit3 V-GW-21).
		if choice.Message.ReasoningContent != "" {
			contentBlocks = append(contentBlocks, anthropicResponseContentBlock{
				Type:      "thinking",
				Thinking:  choice.Message.ReasoningContent,
				Signature: choice.Message.ReasoningSignature,
			})
		}
		// Isi berbentuk Parts — jawaban upstream yang content-nya array bagian
		// — diterjemahkan block demi block. Membaca Message.Content saja
		// menghasilkan block teks KOSONG untuk jawaban seperti itu, karena
		// bentuk Parts memang meninggalkan Content kosong (audit3 V-GW-20).
		if len(choice.Message.Parts) > 0 {
			for _, part := range choice.Message.Parts {
				switch part.Type {
				case providers.PartTypeText:
					if part.Text != "" {
						contentBlocks = append(contentBlocks, anthropicResponseContentBlock{
							Type: "text",
							Text: part.Text,
						})
					}
				case providers.PartTypeImageURL:
					if blok, ok := blokGambarDariPart(part); ok {
						contentBlocks = append(contentBlocks, blok)
					}
				}
			}
		} else if choice.Message.Content != "" {
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
		// stop_reason dipetakan setia: stop_sequence dan pause_turn adalah
		// alasan berhenti yang berbeda dari end_turn bagi klien Anthropic, dan
		// melipatkannya mengarang sebab berhenti yang tidak terjadi. Nilai
		// stop_sequence-nya sendiri ikut dikirim (audit3 V-GW-28).
		switch choice.FinishReason {
		case providers.FinishStop:
			stopReason = "end_turn"
		case providers.FinishToolCalls:
			stopReason = "tool_use"
		case providers.FinishLength:
			stopReason = "max_tokens"
		case providers.FinishStopSequence:
			stopReason = "stop_sequence"
			if choice.StopSequence != "" {
				ss := choice.StopSequence
				stopSequence = &ss
			}
		case providers.FinishPauseTurn:
			stopReason = "pause_turn"
		case providers.FinishContentFilter:
			stopReason = "refusal"
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
		ID:           msgID,
		Type:         "message",
		Role:         "assistant",
		Model:        pr.model.ModelID,
		Content:      contentBlocks,
		StopReason:   &stopReason,
		StopSequence: stopSequence,
		// Angka yang dilaporkan ke klien adalah angka yang SAMA dengan yang
		// dicatat dan ditagih (pemakaian sesudah lantai), dalam bentuk spec
		// Anthropic — bukan usage mentah upstream (audit3 V-GW-26).
		Usage: usageAnthropicDariKanonik(pemakaian),
	}

	// Tubuh disusun ke buffer lebih dulu: byte yang sama persis yang
	// dikirim ke klien inilah yang disimpan ke cache (Batch C1), bukan
	// hasil susun ulang kedua yang bisa berbeda (ID pesan acak).
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(resp); err != nil {
		h.logger.ErrorContext(r.Context(), "gagal menyusun respons Anthropic", "error", err)
		httpx.InternalError(w, r)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("anthropic-version", "2023-06-01")
	w.Header().Set("Cache-Control", "no-store")
	if cacheKey != "" {
		// Header hanya tampil bila cache benar-benar dilibatkan pada
		// request ini — sama seperti permukaan OpenAI.
		w.Header().Set("X-RouteX-Cache", "MISS")
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buf.Bytes())

	if cacheKey != "" && h.respCache != nil && h.respCache.IsEnabled() {
		// Hanya respons sukses yang mencapai titik ini: kegagalan upstream
		// dan penolakan penyaring sudah return lebih dulu, jadi tidak ada
		// error maupun jawaban tersaring yang ter-cache. Teks ikut disimpan
		// supaya jalur HIT bisa menegakkan penyaring dengan kebijakan
		// SAAT HIT.
		_ = h.respCache.Set(r.Context(), cacheKey, &responsecache.Entry{
			Model:      out.Candidate.UpstreamModelName,
			ProviderID: out.Candidate.ProviderID,
			Raw:        buf.Bytes(),
			Usage:      pemakaian,
			Teks:       teksJawaban(out.Value),
			CachedAt:   time.Now(),
		})
	}
}

// alatAliran adalah catatan satu tool yang sudah dibuka block-nya pada aliran
// Anthropic: index kanonik yang dilaporkan adapter, ID tool-nya, dan index
// block keluaran tempat potongannya ditulis.
type alatAliran struct {
	indexKanonik int
	id           string
	indexBlok    int
}

func (h *Handlers) anthropicChatMengalir(w http.ResponseWriter, r *http.Request, pr *persiapan, j *jejak) {
	out := ExecuteStream(h.guard.DenganMemoBatas(r.Context()), h.exec, pr.plan,
		func(ctx context.Context, c *upstream.RouteCandidate) (providers.Stream, error) {
			if perr := h.guard.PeriksaBatasProvider(ctx, c.ProviderID, c.ProviderName); perr != nil {
				return nil, perr
			}
			if perr := h.guard.PeriksaKolomProvider(ctx, c); perr != nil {
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
		perr := providers.AsError(out.Err)
		status, _, _, msg := httpUntuk(perr)
		// Type dari pemeta Anthropic, bukan kosakata OpenAI httpUntuk —
		// lihat catatan B-169 di jalur non-streaming di atas.
		writeAnthropicError(w, status, tipeErrorAnthropicUntukUpstream(perr), msg)
		return
	}

	mulaiAliran := time.Now()
	defer func() { j.tambahHulu(time.Since(mulaiAliran)) }()
	defer func() { _ = out.Value.Close() }()

	w.Header().Set("anthropic-version", "2023-06-01")
	if err := httpx.PrepareSSE(w, r); err != nil {
		h.gagalSiapkanSSE(w, r, j, err)
		return
	}

	rc := http.NewResponseController(w)
	var (
		terkirim int64
		// byteKonten hanya menghitung byte KONTEN (teks delta + argumen tool),
		// BUKAN byte aliran terbingkai. Lantai taksiran output memakai angka ini:
		// menaksir dari terkirim (yang memuat baris event:, envelope JSON, dan
		// peristiwa buka/tutup blok) menggelembungkan taksiran belasan kali
		// lipat dari token konten sebenarnya (audit3 V-AB-15).
		byteKonten int64
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
		blokBerikutnya    int // index untuk content_block berikutnya
		stopSeq           string

		// Identitas tool TIDAK bisa bertumpu pada index kanonik saja. Adapter
		// yang upstreamnya tidak mengirim index menomori potongan menurut
		// posisinya di dalam chunk, jadi tool kedua yang tiba pada chunk-nya
		// sendiri juga ber-index 0 — deltanya lalu menempel ke block tool
		// pertama yang sudah ditutup dan tool itu sendiri tidak pernah menjadi
		// block (audit3 V-GW-19). Yang stabil adalah ID: potongan ber-ID baru
		// selalu tool baru, dan potongan tanpa ID adalah lanjutan tool terakhir
		// yang dibuka untuk index tersebut.
		alatDibuka []alatAliran
	)

	// cariAlat menyelesaikan satu potongan tool ke index block keluaran.
	// false berarti potongan ini membuka tool yang belum dikenal.
	cariAlat := func(tc providers.ToolCall) (int, bool) {
		if tc.ID != "" {
			for _, a := range alatDibuka {
				if a.id == tc.ID {
					return a.indexBlok, true
				}
			}
			return 0, false
		}
		for i := len(alatDibuka) - 1; i >= 0; i-- {
			if alatDibuka[i].indexKanonik == tc.Index {
				return alatDibuka[i].indexBlok, true
			}
		}
		return 0, false
	}

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

	// finalisasiPemakaian menerapkan lantai pemakaian dan mencatatnya, tepat
	// satu kali. Jalur sukses memanggilnya SEBELUM message_delta disusun —
	// supaya angka yang dikoreksi itulah yang sampai ke klien, bukan estimasi
	// message_start yang tidak pernah diperbaiki (audit3 V-GW-27) — dan defer
	// di bawahnya adalah jaring pengaman untuk semua jalan keluar lain
	// (aliran terputus, klien pergi), yang tetap harus tercatat.
	pemakaianFinal := false
	finalisasiPemakaian := func() {
		if pemakaianFinal {
			return
		}
		pemakaianFinal = true
		// Lantai dua sisi dari byte KONTEN (audit3 V-AB-14, V-AB-15): input
		// dari prompt, output dari konten yang benar-benar dihasilkan —
		// bukan dari byte aliran terbingkai.
		usage = terapkanLantaiPemakaian(pr.req, usage, byteKonten)
		h.guard.CatatToken(r.Context(), pr.principal, pr.targets, int64(usage.TotalTokens))
		h.guard.CatatTokenKolomProvider(r.Context(), out.Candidate, int64(usage.TotalTokens))
		j.pasangPemakaian(usage)
	}
	defer finalisasiPemakaian()

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
			// Kegagalan di tengah aliran DUA kali salah sebelum perbaikan ini:
			// ia tidak pernah ditandai sebagai kegagalan upstream, jadi request
			// tercatat sukses penuh (audit3 V-GW-47), dan peristiwa error-nya
			// selalu api_error generik berapa pun sebab aslinya (audit3
			// V-EM-08). Keduanya diperbaiki di sini: jejak ditandai gagal, dan
			// tipe + pesan error dipetakan dari kegagalan aslinya.
			perr := providers.AsError(err)
			j.pasangKegagalanUpstream(perr)
			_, _, _, pesanGalat := httpUntuk(perr)
			errPayload, _ := json.Marshal(map[string]any{
				"type": "error",
				"error": map[string]any{
					"type":    tipeErrorAnthropicUntukUpstream(perr),
					"message": pesanGalat,
				},
			})
			_, _ = fmt.Fprintf(w, "event: error\ndata: %s\n\n", errPayload)
			_ = rc.Flush()
			return
		}

		if ev.Usage != nil {
			usage = *ev.Usage
		}
		if ev.StopSequence != "" {
			stopSeq = ev.StopSequence
		}
		if ev.FinishReason != "" {
			switch ev.FinishReason {
			case providers.FinishStop:
				stopReason = "end_turn"
			case providers.FinishToolCalls:
				stopReason = "tool_use"
			case providers.FinishLength:
				stopReason = "max_tokens"
			case providers.FinishStopSequence:
				stopReason = "stop_sequence"
			case providers.FinishPauseTurn:
				stopReason = "pause_turn"
			case providers.FinishContentFilter:
				stopReason = "refusal"
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
		//
		// Sesudah teks, alur TIDAK melompat ke peristiwa berikutnya: satu
		// peristiwa kanonik bisa memuat teks DAN tool_calls sekaligus (chunk
		// gabungan dari upstream berdialek OpenAI), dan melompat akan membuang
		// tool_calls-nya — klien lalu menerima stop_reason "tool_use" untuk
		// tool yang tidak pernah muncul sebagai block (audit3 V-GW-18).
		if ev.Delta != "" {
			byteKonten += int64(len(ev.Delta))
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
		}

		// Potongan tool_calls. Adapter mengirim content_block_start tool_use sebagai
		// satu peristiwa (membawa id dan nama, tanpa argumen), lalu setiap potongan
		// argumen sebagai input_json_delta terpisah. Keduanya disusun ulang ke blok
		// tool_use yang sama — diselesaikan lewat cariAlat, bukan index mentah,
		// supaya tool yang index kanoniknya bertabrakan tetap menjadi block
		// sendiri-sendiri (audit3 V-GW-19).
		for _, tc := range ev.ToolCalls {
			idx, dikenal := cariAlat(tc)
			if !dikenal {
				// Tool baru: tutup blok yang sedang terbuka (text atau tool lain)
				// sebelum membuka blok tool_use di index berikutnya.
				tutupBlok()
				idx = blokBerikutnya
				blokBerikutnya++
				alatDibuka = append(alatDibuka, alatAliran{
					indexKanonik: tc.Index, id: tc.ID, indexBlok: idx,
				})
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
				byteKonten += int64(len(tc.Function.Arguments))
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

	// 4. Kirim message_delta. Pemakaian difinalisasi DULU: message_delta
	// adalah tempat koreksi usage pada protokol Anthropic, dan di sinilah
	// input_tokens asli upstream menggantikan estimasi lokal yang dibawa
	// message_start (audit3 V-GW-27) — dalam bentuk spec, tanpa hitung ganda
	// token cache (audit3 V-GW-26). Lantai di dalam finalisasi juga menaksir
	// output dari byte KONTEN, bukan byte terbingkai (audit3 V-AB-15).
	finalisasiPemakaian()
	outputTokens := usage.OutputTokens
	if outputTokens < 1 {
		outputTokens = 1
	}
	usageKlien := usageAnthropicDariKanonik(usage)
	var stopSeqNilai any
	if stopSeq != "" {
		stopSeqNilai = stopSeq
	}
	usageDelta := map[string]any{
		"input_tokens":                usageKlien.InputTokens,
		"output_tokens":               outputTokens,
		"cache_creation_input_tokens": usageKlien.CacheCreationInputTokens,
		"cache_read_input_tokens":     usageKlien.CacheReadInputTokens,
	}
	if usageKlien.CacheCreation != nil {
		usageDelta["cache_creation"] = map[string]any{
			"ephemeral_1h_input_tokens": usageKlien.CacheCreation.Ephemeral1hInputTokens,
			"ephemeral_5m_input_tokens": usageKlien.CacheCreation.Ephemeral5mInputTokens,
		}
	}
	msgDeltaPayload, _ := json.Marshal(map[string]any{
		"type": "message_delta",
		"delta": map[string]any{
			"stop_reason":   stopReason,
			"stop_sequence": stopSeqNilai,
		},
		"usage": usageDelta,
	})
	_, _ = fmt.Fprintf(w, "event: message_delta\ndata: %s\n\n", msgDeltaPayload)
	_ = rc.Flush()

	// 5. Kirim message_stop
	_, _ = fmt.Fprintf(w, "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
	_ = rc.Flush()
}
