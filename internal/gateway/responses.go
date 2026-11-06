package gateway

// Permukaan OpenAI Responses API (POST /v1/responses) — B-105.
//
// Sebelum berkas ini, rute /v1/responses adalah alias ChatCompletions: ia
// menuntut field `messages` dan membalas bentuk `chat.completion`. Itu
// kebohongan kontrak — klien Responses API (SDK resmi OpenAI dengan
// client.responses.create, Codex dalam mode Responses, dan sejenisnya)
// mengirim `input`/`instructions` dan mengharap objek `response`.
//
// Subset yang JUJUR dan teruji di sini adalah NON-STREAMING:
//
//   - input berupa string, atau daftar item: message (content string atau
//     part input_text/output_text/input_image), function_call,
//     function_call_output; item reasoning diterima dan dilewati (opaque —
//     isinya jejak internal model yang memang tidak diteruskan ke panggilan
//     berikut oleh klien mana pun selain OpenAI sendiri).
//   - instructions menjadi pesan system terdepan; tools bentuk datar
//     Responses diterjemahkan ke bentuk kanonik (termasuk strict);
//     tool_choice string/objek fungsi; max_output_tokens; reasoning.effort;
//     text.format (text/json_object/json_schema).
//   - Respons adalah objek `response` yang benar: id berawalan resp_,
//     object "response", created_at, status, output[] berisi item
//     message/output_text, function_call, dan reasoning (ringkasan kosong:
//     jejak penalaran mentah tidak disamarkan sebagai ringkasan), serta
//     usage {input_tokens, output_tokens, total_tokens}.
//
// Di luar subset — DITOLAK EKSPLISIT, tidak diterjemahkan diam-diam:
// stream:true (mesin event Responses terpisah; lihat peta cakupan B-105),
// previous_response_id & item_reference (gateway tidak menyimpan respons),
// part input_file (file_id tidak bisa diunduh gateway), tool_choice
// allowed_tools, dan tipe item tak dikenal.
//
// Eksekusi menumpang perancah yang sama seperti permukaan lain: persiapan
// bersama (resolusi model, kewenangan key, anggaran, kandidat, rute),
// Execute + cascade tier lama, lantai pemakaian, pencatatan token, penyaring
// jawaban, dan response cache dengan cakupan permukaan "responses" sendiri.

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
	"github.com/NexGen-X/Route-X/internal/httpx"
	"github.com/NexGen-X/Route-X/internal/providers"
	"github.com/NexGen-X/Route-X/internal/responsecache"
)

const permukaanResponses = "responses"

// kekeliruanResponses membawa kode + pesan untuk penolakan decode permukaan
// ini, supaya handler menulisnya apa adanya (mengikuti pola penolakan
// eksplisit permukaan Anthropic).
type kekeliruanResponses struct {
	kode  string
	pesan string
}

func (e *kekeliruanResponses) Error() string { return e.pesan }

func tolakResponses(pesan string) *kekeliruanResponses {
	return &kekeliruanResponses{kode: httpx.CodeInvalidRequest, pesan: pesan}
}

// Responses melayani POST /v1/responses.
func (h *Handlers) Responses(w http.ResponseWriter, r *http.Request) {
	h.pasangMemoBatas(r)
	j, w := h.mulaiJejak(w, r)
	defer j.selesai(r.Context())

	// Penjaga kuota konkurensi yang sama seperti permukaan chat: slot dipegang
	// sampai handler selesai supaya burst klien ini tidak menenggelamkan
	// burst yang sudah berjalan (audit3 V-GW-16).
	jaga := h.jagaKuotaPraUpstream(r)
	defer jaga.tutup()

	body, ok := h.bacaBody(w, r)
	if !ok {
		return
	}
	req, err := uraiPermintaanResponses(body)
	if err != nil {
		if ke, ok := err.(*kekeliruanResponses); ok {
			httpx.BadRequest(w, r, ke.kode, ke.pesan)
			return
		}
		httpx.BadRequest(w, r, httpx.CodeInvalidRequest, err.Error())
		return
	}

	pr, ok := h.siapkanChatParsed(w, r, req, body, j)
	if !ok {
		return
	}
	// Persiapan berhasil: eksekusi upstream pasti dicoba, jadi kuota yang
	// terbakar memang sah. Tanpa baris ini penjaga mengembalikannya lewat
	// tutup() dan penghitung RPM/RPD permukaan ini tidak pernah
	// terakumulasi (B-166) — permukaan lain memanggil lolos() di titik
	// yang sama persis sesudah persiapannya berhasil.
	jaga.lolos()
	h.responsesSekali(w, r, pr, j)
}

// bentukPermintaanResponses adalah kerangka JSON permukaan Responses.
// Field yang tidak diterjemahkan secara sadar (metadata, store, include,
// service_tier, truncation, background) tidak dibaca: mereka metadata
// penyimpanan/kontrol milik OpenAI yang tidak mengubah panggilan model.
type bentukPermintaanResponses struct {
	Model             string          `json:"model"`
	Input             json.RawMessage `json:"input"`
	Instructions      string          `json:"instructions"`
	Tools             []alatResponses `json:"tools"`
	ToolChoice        json.RawMessage `json:"tool_choice"`
	ParallelToolCalls *bool           `json:"parallel_tool_calls"`
	Temperature       *float64        `json:"temperature"`
	TopP              *float64        `json:"top_p"`
	MaxOutputTokens   *int            `json:"max_output_tokens"`
	Stream            bool            `json:"stream"`
	PreviousResponse  string          `json:"previous_response_id"`
	Reasoning         *struct {
		Effort string `json:"effort"`
	} `json:"reasoning"`
	Text *struct {
		Format json.RawMessage `json:"format"`
	} `json:"text"`
}

// alatResponses adalah tool bentuk datar permukaan Responses:
// {type:"function", name, description, parameters, strict} — berbeda dari
// bentuk bersarang chat-completions ({type, function:{...}}).
type alatResponses struct {
	Type        string          `json:"type"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
	Strict      *bool           `json:"strict"`
}

// pesanDecodeResponses membersihkan kesalahan unmarshal body Responses
// sebelum sampai ke klien (B-253): kesalahan encoding/json menyebut nama
// struct Go ("Go struct field bentukPermintaanResponses...") dan detail
// stdlib lain yang merupakan jeroan implementasi. Yang boleh tahu klien
// hanyalah nama field dalam ejaan JSON-nya dan fakta tipenya tak cocok —
// pola yang sama seperti pesanDecodeAnthropic di permukaan Messages.
func pesanDecodeResponses(err error) error {
	var ute *json.UnmarshalTypeError
	if errors.As(err, &ute) && ute.Field != "" {
		return tolakResponses("field '" + ute.Field + "' memiliki tipe yang tidak sesuai")
	}
	return tolakResponses("body bukan JSON yang sah")
}

// uraiPermintaanResponses menerjemahkan body Responses API menjadi
// ChatRequest kanonik. Penolakan eksplisit untuk semua yang di luar subset.
func uraiPermintaanResponses(body []byte) (*providers.ChatRequest, error) {
	var b bentukPermintaanResponses
	if err := json.Unmarshal(body, &b); err != nil {
		return nil, pesanDecodeResponses(err)
	}
	if b.Stream {
		return nil, &kekeliruanResponses{
			kode:  "streaming_not_supported",
			pesan: "stream:true pada /v1/responses belum didukung gateway ini; kirim tanpa stream untuk jawaban non-streaming",
		}
	}
	if b.PreviousResponse != "" {
		return nil, tolakResponses("previous_response_id tidak didukung: gateway tidak menyimpan respons; kirim riwayat lengkap lewat input")
	}
	if b.Model == "" {
		return nil, tolakResponses("field model wajib diisi")
	}
	if len(b.Input) == 0 {
		return nil, tolakResponses("field input wajib diisi (string atau daftar item)")
	}

	// Validasi rentang (B-171), pasangan blok yang sama di
	// DecodeChatRequest: nilai di luar rentang kontrak ditolak lokal,
	// bukan diteruskan untuk ditolak upstream sebagai kegagalan
	// provider.
	if b.MaxOutputTokens != nil && *b.MaxOutputTokens <= 0 {
		return nil, tolakResponses("field max_output_tokens harus lebih besar dari 0")
	}
	if b.Temperature != nil && (*b.Temperature < 0 || *b.Temperature > 2) {
		return nil, tolakResponses("field temperature harus antara 0 dan 2")
	}
	if b.TopP != nil && (*b.TopP < 0 || *b.TopP > 1) {
		return nil, tolakResponses("field top_p harus antara 0 dan 1")
	}

	req := &providers.ChatRequest{
		Model:             b.Model,
		Temperature:       b.Temperature,
		TopP:              b.TopP,
		MaxTokens:         b.MaxOutputTokens,
		ParallelToolCalls: b.ParallelToolCalls,
	}
	if b.Reasoning != nil {
		req.ReasoningEffort = b.Reasoning.Effort
	}
	if b.Instructions != "" {
		req.Messages = append(req.Messages, providers.Message{
			Role: providers.RoleSystem, Content: b.Instructions,
		})
	}

	pesan, err := uraiInputResponses(b.Input)
	if err != nil {
		return nil, err
	}
	req.Messages = append(req.Messages, pesan...)
	if len(pesan) == 0 && b.Instructions == "" {
		return nil, tolakResponses("input tidak menghasilkan pesan apa pun")
	}

	for _, t := range b.Tools {
		if t.Type != "" && t.Type != "function" {
			return nil, tolakResponses("tipe tool " + t.Type + " tidak didukung; hanya function")
		}
		if t.Name == "" {
			return nil, tolakResponses("tool function wajib bernama")
		}
		req.Tools = append(req.Tools, providers.Tool{
			Type: "function",
			Function: providers.FunctionDef{
				Name: t.Name, Description: t.Description,
				Parameters: t.Parameters, Strict: t.Strict,
			},
		})
	}
	if len(b.ToolChoice) > 0 {
		tc, err := uraiToolChoiceResponses(b.ToolChoice)
		if err != nil {
			return nil, err
		}
		req.ToolChoice = tc
	}
	if b.Text != nil && len(b.Text.Format) > 0 {
		rf, err := uraiFormatTeksResponses(b.Text.Format)
		if err != nil {
			return nil, err
		}
		req.ResponseFormat = rf
	}
	return req, nil
}

// uraiInputResponses mengurai field input: string bebas menjadi satu pesan
// user; daftar item diterjemahkan per tipe. Item function_call yang
// berurutan digabung menjadi SATU pesan assistant dengan banyak ToolCalls —
// itulah semantik satu giliran model pada Responses API.
func uraiInputResponses(raw json.RawMessage) ([]providers.Message, error) {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		if strings.TrimSpace(s) == "" {
			return nil, tolakResponses("input string kosong")
		}
		return []providers.Message{{Role: providers.RoleUser, Content: s}}, nil
	}
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, tolakResponses("input harus string atau daftar item")
	}

	var hasil []providers.Message
	var panggilan []providers.ToolCall // function_call berurutan yang belum dibuang
	buangPanggilan := func() {
		if len(panggilan) == 0 {
			return
		}
		hasil = append(hasil, providers.Message{Role: providers.RoleAssistant, ToolCalls: panggilan})
		panggilan = nil
	}

	for _, mentah := range items {
		var kepala struct {
			Type string `json:"type"`
			Role string `json:"role"`
		}
		if err := json.Unmarshal(mentah, &kepala); err != nil {
			return nil, tolakResponses("item input bukan objek JSON yang sah")
		}
		switch {
		case kepala.Type == "function_call":
			var fc struct {
				CallID    string          `json:"call_id"`
				ID        string          `json:"id"`
				Name      string          `json:"name"`
				Arguments json.RawMessage `json:"arguments"`
			}
			if err := json.Unmarshal(mentah, &fc); err != nil {
				return nil, tolakResponses("item function_call tidak sah")
			}
			id := fc.CallID
			if id == "" {
				id = fc.ID
			}
			args := ""
			if len(fc.Arguments) > 0 {
				var argStr string
				if err := json.Unmarshal(fc.Arguments, &argStr); err == nil {
					args = argStr
				} else {
					args = string(fc.Arguments) // bentuk objek: padatkan apa adanya
				}
			}
			panggilan = append(panggilan, providers.ToolCall{
				ID: id, Type: "function",
				Function: providers.FunctionCall{Name: fc.Name, Arguments: args},
			})
		case kepala.Type == "function_call_output":
			buangPanggilan()
			var fo struct {
				CallID string          `json:"call_id"`
				Output json.RawMessage `json:"output"`
			}
			if err := json.Unmarshal(mentah, &fo); err != nil {
				return nil, tolakResponses("item function_call_output tidak sah")
			}
			teks, err := teksDariKeluaranTool(fo.Output)
			if err != nil {
				return nil, err
			}
			hasil = append(hasil, providers.Message{
				Role: providers.RoleTool, ToolCallID: fo.CallID, Content: teks,
			})
		case kepala.Type == "reasoning":
			// Diterima dan dilewati dengan sadar: isinya opaque (summary/
			// encrypted_content) dan tidak ada padanan kanoniknya.
			continue
		case kepala.Type == "item_reference":
			return nil, tolakResponses("item_reference tidak didukung: gateway tidak menyimpan item; kirim item lengkap")
		case kepala.Type == "message" || kepala.Type == "" && kepala.Role != "":
			buangPanggilan()
			m, err := uraiItemPesanResponses(mentah)
			if err != nil {
				return nil, err
			}
			hasil = append(hasil, m)
		default:
			return nil, tolakResponses("tipe item input " + kepala.Type + " tidak didukung")
		}
	}
	buangPanggilan()
	return hasil, nil
}

// uraiItemPesanResponses mengurai satu item message: content string, atau
// daftar part input_text/output_text (teks), input_image (URL), refusal.
func uraiItemPesanResponses(mentah json.RawMessage) (providers.Message, error) {
	var it struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(mentah, &it); err != nil {
		return providers.Message{}, tolakResponses("item message tidak sah")
	}
	role := providers.Role(it.Role)
	if role == "" {
		role = providers.RoleUser
	}
	m := providers.Message{Role: role}

	var s string
	if err := json.Unmarshal(it.Content, &s); err == nil {
		m.Content = s
		return m, nil
	}
	var parts []json.RawMessage
	if err := json.Unmarshal(it.Content, &parts); err != nil {
		return providers.Message{}, tolakResponses("content item message harus string atau daftar part")
	}
	var teks strings.Builder
	for _, p := range parts {
		var pk struct {
			Type     string `json:"type"`
			Text     string `json:"text"`
			Refusal  string `json:"refusal"`
			ImageURL string `json:"image_url"`
		}
		if err := json.Unmarshal(p, &pk); err != nil {
			return providers.Message{}, tolakResponses("part content tidak sah")
		}
		switch pk.Type {
		case "input_text", "output_text", "text":
			if teks.Len() > 0 {
				teks.WriteString("\n")
			}
			teks.WriteString(pk.Text)
		case "refusal":
			if teks.Len() > 0 {
				teks.WriteString("\n")
			}
			teks.WriteString(pk.Refusal)
		case "input_image":
			if pk.ImageURL == "" {
				return providers.Message{}, tolakResponses("part input_image tanpa image_url tidak didukung (file_id tidak bisa diunduh gateway)")
			}
			if teks.Len() > 0 {
				m.Parts = append(m.Parts, providers.ContentPart{Type: providers.PartTypeText, Text: teks.String()})
				teks.Reset()
			}
			m.Parts = append(m.Parts, providers.ContentPart{
				Type: providers.PartTypeImageURL, ImageURL: &providers.ImageURL{URL: pk.ImageURL},
			})
		case "input_file":
			return providers.Message{}, tolakResponses("part input_file tidak didukung: gateway tidak mengunduh file_id")
		default:
			return providers.Message{}, tolakResponses("tipe part content " + pk.Type + " tidak didukung")
		}
	}
	if len(m.Parts) > 0 {
		if teks.Len() > 0 {
			m.Parts = append(m.Parts, providers.ContentPart{Type: providers.PartTypeText, Text: teks.String()})
		}
		return m, nil
	}
	m.Content = teks.String()
	return m, nil
}

// teksDariKeluaranTool membaca output function_call_output: string apa
// adanya, atau daftar part yang teksnya digabung (gambar pada keluaran tool
// tidak punya padanan kanonik pesan tool — ditolak eksplisit).
func teksDariKeluaranTool(raw json.RawMessage) (string, error) {
	if len(raw) == 0 {
		return "", nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s, nil
	}
	var parts []json.RawMessage
	if err := json.Unmarshal(raw, &parts); err != nil {
		return "", tolakResponses("output function_call_output harus string atau daftar part teks")
	}
	var teks strings.Builder
	for _, p := range parts {
		var pk struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}
		if err := json.Unmarshal(p, &pk); err != nil {
			return "", tolakResponses("part output tool tidak sah")
		}
		if pk.Type != "input_text" && pk.Type != "output_text" && pk.Type != "text" {
			return "", tolakResponses("part output tool bertipe " + pk.Type + " tidak didukung")
		}
		if teks.Len() > 0 {
			teks.WriteString("\n")
		}
		teks.WriteString(pk.Text)
	}
	return teks.String(), nil
}

// uraiToolChoiceResponses menerjemahkan tool_choice Responses ke bentuk
// chat-completions kanonik: string diteruskan; objek fungsi {type:"function",
// name} menjadi {type:"function", function:{name}}.
func uraiToolChoiceResponses(raw json.RawMessage) (json.RawMessage, error) {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return raw, nil
	}
	var o struct {
		Type string `json:"type"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(raw, &o); err != nil {
		return nil, tolakResponses("tool_choice tidak sah")
	}
	switch o.Type {
	case "function":
		out, _ := json.Marshal(map[string]any{
			"type":     "function",
			"function": map[string]any{"name": o.Name},
		})
		return out, nil
	case "allowed_tools":
		return nil, tolakResponses("tool_choice allowed_tools tidak didukung")
	default:
		return raw, nil // auto/none/required berbentuk objek: teruskan apa adanya
	}
}

// uraiFormatTeksResponses menerjemahkan text.format Responses ke
// response_format chat-completions: json_schema Responses berbentuk datar
// {type, name, schema, strict}; chat memintanya bersarang di bawah
// "json_schema".
func uraiFormatTeksResponses(raw json.RawMessage) (json.RawMessage, error) {
	var o struct {
		Type   string          `json:"type"`
		Name   string          `json:"name"`
		Schema json.RawMessage `json:"schema"`
		Strict *bool           `json:"strict"`
	}
	if err := json.Unmarshal(raw, &o); err != nil {
		return nil, tolakResponses("text.format tidak sah")
	}
	switch o.Type {
	case "text", "":
		return nil, nil // bawaan: tanpa response_format
	case "json_object":
		return json.RawMessage(`{"type":"json_object"}`), nil
	case "json_schema":
		dalam := map[string]any{"name": o.Name, "schema": o.Schema}
		if o.Strict != nil {
			dalam["strict"] = *o.Strict
		}
		out, _ := json.Marshal(map[string]any{"type": "json_schema", "json_schema": dalam})
		return out, nil
	default:
		return nil, tolakResponses("text.format bertipe " + o.Type + " tidak didukung")
	}
}

// --- Pengodean respons ---

// itemKeluaranResponses adalah satu item pada output[] objek response.
type itemKeluaranResponses struct {
	Type string `json:"type"`
	ID   string `json:"id,omitempty"`
	Role string `json:"role,omitempty"`
	// Status adalah status item: "completed" untuk item yang tuntas.
	Status string `json:"status,omitempty"`
	// Content bertipe []any karena part-nya tidak sejenis: output_text
	// membawa text+annotations, refusal membawa field refusal (B-213) —
	// memaksakan satu struct untuk keduanya mengarang field kosong pada
	// salah satunya.
	Content []any            `json:"content,omitempty"`
	Summary []map[string]any `json:"summary,omitempty"`
	CallID  string           `json:"call_id,omitempty"`
	Name    string           `json:"name,omitempty"`
	// Arguments adalah argumen function_call berbentuk string JSON.
	Arguments string `json:"arguments,omitempty"`
}

type partKeluaranResponses struct {
	Type        string   `json:"type"`
	Text        string   `json:"text"`
	Annotations []string `json:"annotations"`
}

// partRefusalResponses adalah part penolakan pada item message Responses:
// bentuknya persis dialeknya, {"type":"refusal","refusal":"..."}.
type partRefusalResponses struct {
	Type    string `json:"type"`
	Refusal string `json:"refusal"`
}

// partGambarResponses adalah part gambar pada item message Responses:
// {"type":"output_image","image_url":"..."} — simetris dengan
// input_image yang dibaca dekoder permukaan ini (B-332).
type partGambarResponses struct {
	Type     string `json:"type"`
	ImageURL string `json:"image_url"`
}

// objekResponses adalah bentuk jawaban /v1/responses non-streaming.
type objekResponses struct {
	ID                string                  `json:"id"`
	Object            string                  `json:"object"`
	CreatedAt         int64                   `json:"created_at"`
	Status            string                  `json:"status"`
	Model             string                  `json:"model"`
	Output            []itemKeluaranResponses `json:"output"`
	Error             any                     `json:"error"`
	IncompleteDetails any                     `json:"incomplete_details"`
	Usage             *pemakaianResponses     `json:"usage,omitempty"`
	ParallelToolCalls bool                    `json:"parallel_tool_calls"`
}

type pemakaianResponses struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

func idAcakResponses(awalan string) string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return awalan + hex.EncodeToString(b)
}

// kodekanResponsResponses menyusun objek response dari jawaban kanonik.
// model yang dilaporkan adalah nama model upstream yang benar-benar
// melayani (dari jawaban upstream), sejajar perilaku permukaan lain.
func kodekanResponsResponses(kanonik *providers.ChatResponse, modelDiminta string, paralel bool) []byte {
	obj := objekResponses{
		Object:    "response",
		Status:    "completed",
		Model:     kanonik.Model,
		Output:    []itemKeluaranResponses{},
		CreatedAt: int64(kanonik.Created),
	}
	if obj.Model == "" {
		obj.Model = modelDiminta
	}
	if kanonik.ID != "" {
		obj.ID = "resp_" + strings.TrimPrefix(kanonik.ID, "chatcmpl-")
	} else {
		obj.ID = idAcakResponses("resp_")
	}
	if obj.CreatedAt == 0 {
		obj.CreatedAt = time.Now().Unix()
	}
	if paralel {
		obj.ParallelToolCalls = true
	}

	if len(kanonik.Choices) > 0 {
		pilihan := kanonik.Choices[0]
		msg := pilihan.Message
		if msg.ReasoningContent != "" {
			// Ringkasan dibiarkan kosong dengan sadar: isi ReasoningContent
			// adalah jejak mentah, dan melabelnya summary_text akan
			// mengarang. Kehadiran itemnya saja yang jujur dilaporkan.
			obj.Output = append(obj.Output, itemKeluaranResponses{
				Type: "reasoning", ID: idAcakResponses("rs_"), Summary: []map[string]any{},
			})
		}
		teks := msg.Content
		if teks == "" {
			var sb strings.Builder
			for _, p := range msg.Parts {
				if p.Type == providers.PartTypeText {
					sb.WriteString(p.Text)
				}
			}
			teks = sb.String()
		}
		var isiPesan []any
		if teks != "" {
			isiPesan = append(isiPesan, partKeluaranResponses{
				Type: "output_text", Text: teks, Annotations: []string{},
			})
		}
		if msg.Refusal != "" {
			// Penolakan punya bentuk part-nya sendiri di dialek ini
			// (B-213) — sejajar decoder permukaan ini yang memang sudah
			// membaca part refusal pada riwayat masuk.
			isiPesan = append(isiPesan, partRefusalResponses{
				Type: "refusal", Refusal: msg.Refusal,
			})
		}
		if msg.AudioTranscript != "" {
			// Transkrip audio adalah teks keluaran model (B-213).
			isiPesan = append(isiPesan, partKeluaranResponses{
				Type: "output_text", Text: msg.AudioTranscript, Annotations: []string{},
			})
		}
		// B-332: part gambar pada pesan kanonik kini ikut diterbitkan
		// sebagai output_image. Encoder ini dulu hanya membaca
		// PartTypeText (untuk teks cadangan) — respons model
		// penghasil gambar tiba di klien Responses sebagai jawaban
		// teks saja, atau pesan kosong bila isinya murni gambar.
		// URL/data-URI diteruskan apa adanya, sejajar perlakuan
		// permukaan lain terhadap part yang sama.
		for _, p := range msg.Parts {
			if p.Type != providers.PartTypeImageURL || p.ImageURL == nil {
				continue
			}
			isiPesan = append(isiPesan, partGambarResponses{
				Type: "output_image", ImageURL: p.ImageURL.URL,
			})
		}
		if len(isiPesan) > 0 {
			obj.Output = append(obj.Output, itemKeluaranResponses{
				Type: "message", ID: idAcakResponses("msg_"),
				Role: "assistant", Status: "completed",
				Content: isiPesan,
			})
		}
		for _, tc := range msg.ToolCalls {
			obj.Output = append(obj.Output, itemKeluaranResponses{
				Type: "function_call", ID: idAcakResponses("fc_"),
				CallID: tc.ID, Name: tc.Function.Name,
				Arguments: tc.Function.Arguments, Status: "completed",
			})
		}
		switch pilihan.FinishReason {
		case providers.FinishLength:
			obj.Status = "incomplete"
			obj.IncompleteDetails = map[string]any{"reason": "max_output_tokens"}
		case providers.FinishPauseTurn:
			// Model berhenti di tengah giliran dan mengharapkan
			// kelanjutan (kosakata Anthropic): bukan jawaban tuntas,
			// jadi "completed" akan mengarang penyelesaian.
			obj.Status = "incomplete"
			obj.IncompleteDetails = map[string]any{"reason": "pause_turn"}
		case providers.FinishContentFilter:
			// Jawaban yang dihentikan filter konten BUKAN jawaban
			// tuntas (B-173): melaporkannya "completed" — apalagi
			// dengan keluaran kosong — adalah janji palsu ke klien.
			// Permukaan Anthropic memetakan keadaan yang sama ke
			// stop_reason "refusal"; di kosakata Responses bentuk
			// jujurnya incomplete dengan alasan content_filter.
			obj.Status = "incomplete"
			obj.IncompleteDetails = map[string]any{"reason": "content_filter"}
		case providers.FinishStop, providers.FinishToolCalls, providers.FinishStopSequence, "":
			// Kosakata kanonik yang memang berarti tuntas di permukaan
			// ini: berhenti normal, panggilan tool selesai, berhenti
			// pada stop sequence yang diminta klien, atau upstream
			// tidak menyebut sebab sama sekali.
		default:
			// Finish reason asing di luar kosakata kanonik (B-248):
			// permukaan chat meneruskannya apa adanya, jadi di sini
			// pun ia tidak dilipat menjadi "completed" — bentuk
			// jujurnya incomplete dengan nilai aslinya sebagai alasan,
			// supaya klien bisa membedakannya dari selesai normal.
			obj.Status = "incomplete"
			obj.IncompleteDetails = map[string]any{"reason": pilihan.FinishReason}
		}
	}
	if kanonik.Usage.TotalTokens > 0 || kanonik.Usage.InputTokens > 0 || kanonik.Usage.OutputTokens > 0 {
		obj.Usage = &pemakaianResponses{
			InputTokens:  kanonik.Usage.InputTokens,
			OutputTokens: kanonik.Usage.OutputTokens,
			TotalTokens:  kanonik.Usage.TotalTokens,
		}
	}
	buf, _ := json.Marshal(obj)
	return buf
}

// responsesSekali menjalankan satu permintaan Responses non-streaming:
// meniru jalur eksekusi permukaan Anthropic (Execute + cascade tier lama +
// lantai pemakaian + penyaring + cache bercakupan permukaan sendiri),
// dengan pengodean objek response.
func (h *Handlers) responsesSekali(w http.ResponseWriter, r *http.Request, pr *persiapan, j *jejak) {
	// Response cache bercakupan permukaan "responses": entri HIT wajib
	// membawa teks (syarat ketat permukaan Anthropic) supaya penyaring bisa
	// dinilai ulang dengan kebijakan saat HIT.
	var cacheKey string
	if h.respCache != nil && h.respCache.IsEnabled() && layakCacheRespons(pr.req) {
		cacheKey = h.respCache.ComputeScopedKey(pr.req, cakupanCache(pr, permukaanResponses))
		if entry, hit, err := h.respCache.Get(r.Context(), cacheKey); err == nil && hit && entry != nil && len(entry.Raw) > 0 && entry.Teks != "" {
			pemakaian := pemakaianCacheHIT(pr.req, entry)
			j.pasangPemakaian(pemakaian)
			if menang := kandidatCacheHIT(pr, entry); menang != nil {
				j.pasangPercobaan(nil, menang)
			}
			h.guard.CatatToken(r.Context(), pr.principal, pr.targets, int64(pemakaian.TotalTokens))
			if rj := h.guard.SaringJawaban(r.Context(), entry.Teks,
				pr.model.ID, entry.ProviderID); rj != nil {
				rj.Tulis(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/json")
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

	// Cascade combo fallback jalur lama — sama seperti permukaan lain.
	if out.Err != nil && !pr.decision.Rule.Combo() {
		if pipeline := ekstrakComboPipeline(pr.decision.Rule); len(pipeline) > 0 {
			for _, tier := range pipeline {
				tOut, rj := h.cobaTierFallbackSekali(r.Context(), tier, pr, j)
				switch {
				case rj != nil:
					rj.Tulis(w, r)
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
		tulisKegagalan(w, r, out.Err)
		return
	}

	if out.Value == nil || (len(out.Value.Raw) == 0 && responsKanonikKosong(out.Value)) {
		// Adapter yang "berhasil" tanpa body mentah dan tanpa satu pun
		// konten kanonik melanggar kontrak providers.Provider (B-247,
		// diperluas B-293): permukaan chat dan embeddings menjawab
		// keadaan yang sama dengan 500. Varian choice-ada-tetapi-kosong
		// total dulu lolos karena kriteria lama hanya menghitung
		// choice; mengodekannya menjadi objek completed ber-output
		// kosong berarti mengarang sukses yang tidak pernah terjadi.
		h.logger.ErrorContext(r.Context(), "adapter mengembalikan respons kosong",
			"provider", namaProvider(out.Candidate), "model", pr.model.ModelID)
		httpx.InternalError(w, r)
		return
	}

	teks := teksJawaban(out.Value)
	pemakaian := terapkanLantaiPemakaian(pr.req, out.Value.Usage, int64(len(teks)))
	j.pasangPemakaian(pemakaian)
	h.guard.CatatToken(r.Context(), pr.principal, pr.targets, int64(pemakaian.TotalTokens))
	h.guard.CatatTokenKolomProvider(r.Context(), out.Candidate, int64(pemakaian.TotalTokens))

	if rj := h.guard.SaringJawaban(r.Context(), teks,
		pr.model.ID, out.Candidate.ProviderID); rj != nil {
		rj.Tulis(w, r)
		return
	}

	// Usage pada objek yang dikodekan memakai pemakaian FINAL (sesudah
	// lantai), bukan angka mentah upstream — klien membaca objek ini.
	disajikan := *out.Value
	disajikan.Usage = pemakaian
	paralel := pr.req.ParallelToolCalls != nil && *pr.req.ParallelToolCalls
	buf := kodekanResponsResponses(&disajikan, pr.req.Model, paralel)

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-RouteX-Cache", "MISS")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buf)

	if cacheKey != "" && h.respCache != nil && h.respCache.IsEnabled() {
		_ = h.respCache.Set(r.Context(), cacheKey, &responsecache.Entry{
			Model:      out.Candidate.UpstreamModelName,
			ProviderID: out.Candidate.ProviderID,
			Raw:        buf,
			Usage:      pemakaian,
			Teks:       teks,
			CachedAt:   time.Now(),
		})
	}
}
