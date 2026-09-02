package google

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/NexGen-X/Route-X/internal/providers"
)

// wire* adalah bentuk kawat Gemini yang ditulis ulang di test, bukan struct produksi:
// dengan begitu nama field yang salah ikut tertangkap.
type wireRequest struct {
	Contents          []wireContent   `json:"contents"`
	SystemInstruction *wireContent    `json:"systemInstruction"`
	GenerationConfig  map[string]any  `json:"generationConfig"`
	Tools             []wireTool      `json:"tools"`
	ToolConfig        map[string]any  `json:"toolConfig"`
	Messages          json.RawMessage `json:"messages"`
	Model             json.RawMessage `json:"model"`
	MaxTokens         json.RawMessage `json:"max_tokens"`
	System            json.RawMessage `json:"system"`
}

type wireContent struct {
	Role  string     `json:"role"`
	Parts []wirePart `json:"parts"`
}

type wirePart struct {
	Text       string `json:"text"`
	Thought    bool   `json:"thought"`
	InlineData *struct {
		MimeType string `json:"mimeType"`
		Data     string `json:"data"`
	} `json:"inlineData"`
	FileData *struct {
		MimeType string `json:"mimeType"`
		FileURI  string `json:"fileUri"`
	} `json:"fileData"`
	FunctionCall *struct {
		Name string          `json:"name"`
		Args json.RawMessage `json:"args"`
	} `json:"functionCall"`
	FunctionResponse *struct {
		Name     string          `json:"name"`
		Response json.RawMessage `json:"response"`
	} `json:"functionResponse"`
}

type wireTool struct {
	FunctionDeclarations []struct {
		Name        string         `json:"name"`
		Description string         `json:"description"`
		Parameters  map[string]any `json:"parameters"`
	} `json:"functionDeclarations"`
}

func decodeRequest(t *testing.T, raw []byte) wireRequest {
	t.Helper()
	var out wireRequest
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("body permintaan tidak bisa diurai: %v\nbody: %s", err, raw)
	}
	return out
}

func TestPermintaanKanonikDiterjemahkanFieldPerField(t *testing.T) {
	p, log := newScriptedServer(t, textResponse)

	req := &providers.ChatRequest{
		Model: "gemini-2.5-flash",
		Messages: []providers.Message{
			{Role: providers.RoleSystem, Content: "jadilah singkat"},
			{Role: providers.RoleUser, Content: "berapa 2+2?"},
			{Role: providers.RoleAssistant, Content: "4"},
			{Role: providers.RoleUser, Content: "yakin?"},
		},
		MaxTokens:   ptrInt(256),
		Temperature: ptrFloat(0.25),
		TopP:        ptrFloat(0.9),
		Stop:        []string{"STOP"},
		Seed:        ptrInt(7),
		// Field tanpa padanan tidak boleh diteruskan: Gemini menolak field asing.
		User:  "pengguna-42",
		Extra: map[string]json.RawMessage{"frequency_penalty": json.RawMessage(`0.5`)},
	}
	if _, err := p.ChatCompletion(context.Background(), req); err != nil {
		t.Fatalf("ChatCompletion() error: %v", err)
	}

	body := decodeRequest(t, log.raw(t, 0))
	if len(body.Messages) != 0 || len(body.Model) != 0 || len(body.MaxTokens) != 0 || len(body.System) != 0 {
		t.Errorf("field bergaya OpenAI ikut terkirim: %s", log.raw(t, 0))
	}
	if body.SystemInstruction == nil || len(body.SystemInstruction.Parts) != 1 ||
		body.SystemInstruction.Parts[0].Text != "jadilah singkat" {
		t.Errorf("systemInstruction = %+v", body.SystemInstruction)
	}
	if len(body.Contents) != 3 {
		t.Fatalf("jumlah contents = %d, mau 3: %+v", len(body.Contents), body.Contents)
	}
	// Gemini hanya mengenal user dan model; tidak ada assistant.
	mau := []string{roleUser, roleModel, roleUser}
	for i, c := range body.Contents {
		if c.Role != mau[i] {
			t.Errorf("contents[%d].role = %q, mau %q", i, c.Role, mau[i])
		}
		if len(c.Parts) != 1 {
			t.Errorf("contents[%d].parts = %+v, mau satu part", i, c.Parts)
		}
	}
	if body.Contents[0].Parts[0].Text != "berapa 2+2?" {
		t.Errorf("parts[0].text = %q", body.Contents[0].Parts[0].Text)
	}

	cfg := body.GenerationConfig
	if cfg == nil {
		t.Fatal("generationConfig tidak terkirim")
	}
	if cfg["maxOutputTokens"] != float64(256) {
		t.Errorf("maxOutputTokens = %v, mau 256 (bukan max_tokens)", cfg["maxOutputTokens"])
	}
	if cfg["temperature"] != 0.25 || cfg["topP"] != 0.9 {
		t.Errorf("temperature/topP = %v/%v", cfg["temperature"], cfg["topP"])
	}
	if cfg["seed"] != float64(7) {
		t.Errorf("seed = %v", cfg["seed"])
	}
	stops, ok := cfg["stopSequences"].([]any)
	if !ok || len(stops) != 1 || stops[0] != "STOP" {
		t.Errorf("stopSequences = %v", cfg["stopSequences"])
	}
	if _, ada := cfg["stop"]; ada {
		t.Error("generationConfig memakai nama field bergaya OpenAI")
	}
}

func TestTanpaParameterGenerasiTidakMengirimGenerationConfig(t *testing.T) {
	p, log := newScriptedServer(t, textResponse)
	if _, err := p.ChatCompletion(context.Background(), simpleRequest()); err != nil {
		t.Fatalf("ChatCompletion() error: %v", err)
	}
	if got := decodeRequest(t, log.raw(t, 0)).GenerationConfig; got != nil {
		t.Errorf("generationConfig = %v, mau tidak terkirim", got)
	}
}

func TestPesanSystemTidakPernahTertinggalDiContents(t *testing.T) {
	tests := []struct {
		nama      string
		messages  []providers.Message
		mauSystem string
	}{
		{
			nama:      "tanpa system",
			messages:  []providers.Message{{Role: providers.RoleUser, Content: "halo"}},
			mauSystem: "",
		},
		{
			nama: "satu system",
			messages: []providers.Message{
				{Role: providers.RoleSystem, Content: "kamu penerjemah"},
				{Role: providers.RoleUser, Content: "halo"},
			},
			mauSystem: "kamu penerjemah",
		},
		{
			nama: "beberapa system dan developer",
			messages: []providers.Message{
				{Role: providers.RoleSystem, Content: "aturan satu"},
				{Role: providers.RoleDeveloper, Content: "aturan dua"},
				{Role: providers.RoleUser, Content: "halo"},
				{Role: providers.RoleSystem, Content: "aturan tiga"},
			},
			mauSystem: "aturan satu\n\naturan dua\n\naturan tiga",
		},
		{
			nama: "system kosong dibuang",
			messages: []providers.Message{
				{Role: providers.RoleSystem, Content: ""},
				{Role: providers.RoleUser, Content: "halo"},
			},
			mauSystem: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.nama, func(t *testing.T) {
			p, log := newScriptedServer(t, textResponse)
			req := &providers.ChatRequest{Model: "gemini-2.5-flash", Messages: tc.messages}
			if _, err := p.ChatCompletion(context.Background(), req); err != nil {
				t.Fatalf("ChatCompletion() error: %v", err)
			}

			body := decodeRequest(t, log.raw(t, 0))
			got := ""
			if body.SystemInstruction != nil && len(body.SystemInstruction.Parts) > 0 {
				got = body.SystemInstruction.Parts[0].Text
			}
			if got != tc.mauSystem {
				t.Errorf("systemInstruction = %q, mau %q", got, tc.mauSystem)
			}
			if tc.mauSystem == "" && body.SystemInstruction != nil {
				t.Error("systemInstruction terkirim padahal tidak ada instruksi")
			}
			for i, c := range body.Contents {
				if c.Role != roleUser && c.Role != roleModel {
					t.Errorf("contents[%d].role = %q; Gemini hanya mengenal user dan model", i, c.Role)
				}
			}
		})
	}
}

func TestPeranBerurutanDigabungMenjadiSatuContent(t *testing.T) {
	p, log := newScriptedServer(t, textResponse)
	req := &providers.ChatRequest{
		Model: "gemini-2.5-flash",
		Messages: []providers.Message{
			{Role: providers.RoleUser, Content: "bagian satu"},
			{Role: providers.RoleUser, Content: "bagian dua"},
			{Role: providers.RoleAssistant, Content: "jawab"},
			{Role: providers.RoleUser, Content: ""},
			{Role: providers.RoleUser, Content: "bagian tiga"},
		},
	}
	if _, err := p.ChatCompletion(context.Background(), req); err != nil {
		t.Fatalf("ChatCompletion() error: %v", err)
	}

	body := decodeRequest(t, log.raw(t, 0))
	if len(body.Contents) != 3 {
		t.Fatalf("jumlah contents = %d, mau 3: %+v", len(body.Contents), body.Contents)
	}
	if len(body.Contents[0].Parts) != 2 {
		t.Errorf("contents pertama memuat %d part, mau 2", len(body.Contents[0].Parts))
	}
	if body.Contents[0].Parts[1].Text != "bagian dua" {
		t.Errorf("urutan part berubah: %+v", body.Contents[0].Parts)
	}
}

func TestPercakapanTidakSahDitolakSebelumDikirim(t *testing.T) {
	tests := []struct {
		nama     string
		req      *providers.ChatRequest
		mauPesan string
	}{
		{"tanpa pesan", &providers.ChatRequest{Model: "gemini-2.5-flash"}, "percakapan"},
		{
			"hanya system",
			&providers.ChatRequest{Model: "gemini-2.5-flash", Messages: []providers.Message{
				{Role: providers.RoleSystem, Content: "aturan"},
			}},
			"percakapan",
		},
		{
			"peran tidak dikenal",
			&providers.ChatRequest{Model: "gemini-2.5-flash", Messages: []providers.Message{
				{Role: providers.Role("penonton"), Content: "halo"},
			}},
			"peran",
		},
		{
			"hasil tool tanpa panggilan yang bisa dikaitkan",
			&providers.ChatRequest{Model: "gemini-2.5-flash", Messages: []providers.Message{
				{Role: providers.RoleUser, Content: "cuaca?"},
				{Role: providers.RoleTool, ToolCallID: "id-asing", Content: "cerah"},
			}},
			"tidak bisa dikaitkan",
		},
		{
			"argumen tool bukan objek",
			&providers.ChatRequest{Model: "gemini-2.5-flash", Messages: []providers.Message{
				{Role: providers.RoleUser, Content: "cuaca?"},
				{Role: providers.RoleAssistant, ToolCalls: []providers.ToolCall{{
					ID: "call_0_cuaca", Function: providers.FunctionCall{Name: "cuaca", Arguments: "[1,2]"},
				}}},
			}},
			"argumen tool",
		},
		{
			"tool tanpa nama",
			&providers.ChatRequest{Model: "gemini-2.5-flash",
				Messages: []providers.Message{{Role: providers.RoleUser, Content: "halo"}},
				Tools:    []providers.Tool{{Type: "function"}},
			},
			"nama",
		},
	}

	for _, tc := range tests {
		t.Run(tc.nama, func(t *testing.T) {
			p, log := newScriptedServer(t, textResponse)
			_, err := p.ChatCompletion(context.Background(), tc.req)
			e := providers.AsError(err)
			if e == nil {
				t.Fatalf("error = %v, mau *providers.Error", err)
			}
			if e.Kind != providers.ErrKindInvalidRequest {
				t.Errorf("Kind = %q, mau %q", e.Kind, providers.ErrKindInvalidRequest)
			}
			if !strings.Contains(strings.ToLower(e.Message), strings.ToLower(tc.mauPesan)) {
				t.Errorf("pesan %q tidak menyebut %q", e.Message, tc.mauPesan)
			}
			if log.count() != 0 {
				t.Error("permintaan cacat tetap dikirim ke upstream")
			}
		})
	}
}

func TestToolDiterjemahkanKeFunctionDeclarations(t *testing.T) {
	p, log := newScriptedServer(t, textResponse)

	req := simpleRequest()
	req.Tools = []providers.Tool{
		{Type: "function", Function: providers.FunctionDef{
			Name:        "cuaca",
			Description: "cari cuaca kota",
			// Skema bergaya OpenAI: memuat kata kunci yang ditolak Gemini.
			Parameters: json.RawMessage(`{
			  "$schema": "https://json-schema.org/draft/2020-12/schema",
			  "type": "object",
			  "additionalProperties": false,
			  "properties": {
			    "kota": {"type": "string", "additionalProperties": false},
			    "hari": {"type": "array", "items": {"type": "string", "$schema": "x"}}
			  },
			  "required": ["kota"]
			}`),
		}},
		{Type: "code_interpreter", Function: providers.FunctionDef{Name: "abaikan-aku"}},
	}
	if _, err := p.ChatCompletion(context.Background(), req); err != nil {
		t.Fatalf("ChatCompletion() error: %v", err)
	}

	body := decodeRequest(t, log.raw(t, 0))
	// Seluruh fungsi berkumpul di SATU entri tools.
	if len(body.Tools) != 1 {
		t.Fatalf("jumlah entri tools = %d, mau 1: %+v", len(body.Tools), body.Tools)
	}
	decls := body.Tools[0].FunctionDeclarations
	if len(decls) != 1 {
		t.Fatalf("jumlah functionDeclarations = %d, mau 1", len(decls))
	}
	if decls[0].Name != "cuaca" || decls[0].Description != "cari cuaca kota" {
		t.Errorf("deklarasi = %+v", decls[0])
	}
	params := decls[0].Parameters
	if params["type"] != "object" {
		t.Errorf("parameters kehilangan type: %v", params)
	}
	if _, ada := params["$schema"]; ada {
		t.Error("$schema tidak dibuang; Gemini menolaknya")
	}
	if _, ada := params["additionalProperties"]; ada {
		t.Error("additionalProperties tidak dibuang; Gemini menolaknya")
	}
	props, _ := params["properties"].(map[string]any)
	kota, _ := props["kota"].(map[string]any)
	if _, ada := kota["additionalProperties"]; ada {
		t.Error("pembersihan skema tidak rekursif pada properties")
	}
	hari, _ := props["hari"].(map[string]any)
	items, _ := hari["items"].(map[string]any)
	if _, ada := items["$schema"]; ada {
		t.Error("pembersihan skema tidak rekursif pada items")
	}
	if req, _ := params["required"].([]any); len(req) != 1 || req[0] != "kota" {
		t.Errorf("required hilang: %v", params["required"])
	}
}

func TestToolTanpaParameterTetapDikirim(t *testing.T) {
	p, log := newScriptedServer(t, textResponse)
	req := simpleRequest()
	req.Tools = []providers.Tool{{Type: "function", Function: providers.FunctionDef{Name: "waktu"}}}
	if _, err := p.ChatCompletion(context.Background(), req); err != nil {
		t.Fatalf("ChatCompletion() error: %v", err)
	}
	decls := decodeRequest(t, log.raw(t, 0)).Tools[0].FunctionDeclarations
	if len(decls) != 1 || decls[0].Name != "waktu" {
		t.Errorf("deklarasi = %+v", decls)
	}
	if decls[0].Parameters != nil {
		t.Errorf("parameters = %v, mau tidak terkirim untuk tool tanpa argumen", decls[0].Parameters)
	}
}

func TestSeluruhToolDilewatiTidakMengirimTools(t *testing.T) {
	p, log := newScriptedServer(t, textResponse)
	req := simpleRequest()
	req.Tools = []providers.Tool{{Type: "code_interpreter", Function: providers.FunctionDef{Name: "x"}}}
	if _, err := p.ChatCompletion(context.Background(), req); err != nil {
		t.Fatalf("ChatCompletion() error: %v", err)
	}
	if got := decodeRequest(t, log.raw(t, 0)).Tools; got != nil {
		t.Errorf("tools = %+v, mau tidak terkirim", got)
	}
}

func TestToolConfigDiterjemahkan(t *testing.T) {
	tests := []struct {
		nama       string
		raw        string
		mauMode    string
		mauAllowed []string
		mauKosong  bool
	}{
		{nama: "auto", raw: `"auto"`, mauMode: "AUTO"},
		{nama: "none", raw: `"none"`, mauMode: "NONE"},
		{nama: "required menjadi ANY", raw: `"required"`, mauMode: "ANY"},
		{nama: "fungsi tertentu", raw: `{"type":"function","function":{"name":"cuaca"}}`,
			mauMode: "ANY", mauAllowed: []string{"cuaca"}},
		{nama: "tidak dikenal diabaikan", raw: `"entah"`, mauKosong: true},
		{nama: "objek tanpa nama diabaikan", raw: `{"type":"function"}`, mauKosong: true},
		{nama: "objek jenis lain diabaikan", raw: `{"type":"entah"}`, mauKosong: true},
		{nama: "bukan objek diabaikan", raw: `[1,2]`, mauKosong: true},
	}

	for _, tc := range tests {
		t.Run(tc.nama, func(t *testing.T) {
			p, log := newScriptedServer(t, textResponse)
			req := simpleRequest()
			req.ToolChoice = json.RawMessage(tc.raw)
			if _, err := p.ChatCompletion(context.Background(), req); err != nil {
				t.Fatalf("ChatCompletion() error: %v", err)
			}

			got := decodeRequest(t, log.raw(t, 0)).ToolConfig
			if tc.mauKosong {
				if got != nil {
					t.Fatalf("toolConfig = %v, mau tidak terkirim", got)
				}
				return
			}
			cfg, _ := got["functionCallingConfig"].(map[string]any)
			if cfg == nil {
				t.Fatalf("functionCallingConfig hilang: %v", got)
			}
			if cfg["mode"] != tc.mauMode {
				t.Errorf("mode = %v, mau %q", cfg["mode"], tc.mauMode)
			}
			allowed, _ := cfg["allowedFunctionNames"].([]any)
			if len(allowed) != len(tc.mauAllowed) {
				t.Fatalf("allowedFunctionNames = %v, mau %v", allowed, tc.mauAllowed)
			}
			for i, name := range tc.mauAllowed {
				if allowed[i] != name {
					t.Errorf("allowedFunctionNames[%d] = %v, mau %q", i, allowed[i], name)
				}
			}
		})
	}
}

// Gemini tidak punya id panggilan tool, sedangkan bentuk kanonik memakainya. Alur penuh ini
// membuktikan jembatannya bekerja dua arah: id disintesis saat menerjemahkan respons, dan
// dipulihkan menjadi nama fungsi saat hasilnya dikirim balik.
func TestAlurToolBolakBalikPenuh(t *testing.T) {
	toolResponse := `{
	  "candidates": [{
	    "content": {"role": "model", "parts": [
	      {"text": "sebentar"},
	      {"functionCall": {"name": "cuaca", "args": {"kota": "Bandung"}}},
	      {"functionCall": {"name": "cuaca", "args": {"kota": "Solo"}}}
	    ]},
	    "finishReason": "STOP",
	    "index": 0
	  }],
	  "usageMetadata": {"promptTokenCount": 40, "candidatesTokenCount": 12, "totalTokenCount": 52}
	}`
	finalResponse := `{
	  "candidates": [{
	    "content": {"role": "model", "parts": [{"text": "Bandung 24, Solo 26"}]},
	    "finishReason": "STOP"
	  }],
	  "usageMetadata": {"promptTokenCount": 60, "candidatesTokenCount": 20, "totalTokenCount": 80}
	}`
	p, log := newScriptedServer(t, toolResponse, finalResponse)

	tools := []providers.Tool{{Type: "function", Function: providers.FunctionDef{
		Name:       "cuaca",
		Parameters: json.RawMessage(`{"type":"object","properties":{"kota":{"type":"string"}}}`),
	}}}

	first, err := p.ChatCompletion(context.Background(), &providers.ChatRequest{
		Model:    "gemini-2.5-flash",
		Messages: []providers.Message{{Role: providers.RoleUser, Content: "cuaca Bandung dan Solo?"}},
		Tools:    tools,
	})
	if err != nil {
		t.Fatalf("putaran 1 error: %v", err)
	}

	choice := first.Choices[0]
	// Gemini menjawab STOP walau menghasilkan panggilan tool; klien OpenAI butuh tool_calls.
	if choice.FinishReason != providers.FinishToolCalls {
		t.Errorf("finish reason = %q, mau %q", choice.FinishReason, providers.FinishToolCalls)
	}
	if choice.Message.Content != "sebentar" {
		t.Errorf("teks = %q", choice.Message.Content)
	}
	if len(choice.Message.ToolCalls) != 2 {
		t.Fatalf("jumlah tool call = %d, mau 2", len(choice.Message.ToolCalls))
	}
	// Dua panggilan ke fungsi YANG SAMA harus tetap bisa dibedakan lewat id buatan.
	if choice.Message.ToolCalls[0].ID == choice.Message.ToolCalls[1].ID {
		t.Fatalf("id panggilan tool tidak unik: %q", choice.Message.ToolCalls[0].ID)
	}
	if choice.Message.ToolCalls[0].ID != "call_0_cuaca" || choice.Message.ToolCalls[1].ID != "call_1_cuaca" {
		t.Errorf("id panggilan = %q dan %q", choice.Message.ToolCalls[0].ID, choice.Message.ToolCalls[1].ID)
	}
	// Argumen diteruskan sebagai byte upstream apa adanya, tanpa diurai ulang: model kadang
	// menghasilkan JSON yang tidak sah, dan klien lebih siap menanganinya daripada gateway.
	if choice.Message.ToolCalls[0].Function.Arguments != `{"kota": "Bandung"}` {
		t.Errorf("argumen = %q", choice.Message.ToolCalls[0].Function.Arguments)
	}

	// Putaran dua: hasil dikirim balik seperti yang dilakukan klien OpenAI.
	second, err := p.ChatCompletion(context.Background(), &providers.ChatRequest{
		Model: "gemini-2.5-flash",
		Messages: []providers.Message{
			{Role: providers.RoleUser, Content: "cuaca Bandung dan Solo?"},
			{Role: providers.RoleAssistant, Content: "sebentar", ToolCalls: choice.Message.ToolCalls},
			{Role: providers.RoleTool, ToolCallID: "call_0_cuaca", Content: `{"suhu":24}`},
			{Role: providers.RoleTool, ToolCallID: "call_1_cuaca", Content: "26 derajat"},
		},
		Tools: tools,
	})
	if err != nil {
		t.Fatalf("putaran 2 error: %v", err)
	}
	if second.Choices[0].Message.Content != "Bandung 24, Solo 26" {
		t.Errorf("jawaban akhir = %q", second.Choices[0].Message.Content)
	}

	body := decodeRequest(t, log.raw(t, 1))
	if len(body.Contents) != 3 {
		t.Fatalf("jumlah contents = %d, mau 3: %+v", len(body.Contents), body.Contents)
	}

	model := body.Contents[1]
	if model.Role != roleModel {
		t.Errorf("contents[1].role = %q, mau model", model.Role)
	}
	if len(model.Parts) != 3 {
		t.Fatalf("contents[1] memuat %d part, mau teks + dua functionCall", len(model.Parts))
	}
	if model.Parts[1].FunctionCall == nil || model.Parts[1].FunctionCall.Name != "cuaca" {
		t.Errorf("functionCall = %+v", model.Parts[1])
	}
	// Di arah keluar, encoding/json memadatkan kembali muatan json.RawMessage.
	if string(model.Parts[1].FunctionCall.Args) != `{"kota":"Bandung"}` {
		t.Errorf("args = %s", model.Parts[1].FunctionCall.Args)
	}

	results := body.Contents[2]
	// Hasil tool dikirim sebagai part functionResponse di peran user, bukan peran tool.
	if results.Role != roleUser {
		t.Errorf("hasil tool berperan %q, mau user", results.Role)
	}
	if len(results.Parts) != 2 {
		t.Fatalf("hasil tool memuat %d part, mau 2 dalam SATU content", len(results.Parts))
	}
	for i, part := range results.Parts {
		if part.FunctionResponse == nil {
			t.Fatalf("part[%d] bukan functionResponse: %+v", i, part)
		}
		// Gemini mengaitkan hasil lewat NAMA, jadi id kanonik harus dipulihkan menjadi nama.
		if part.FunctionResponse.Name != "cuaca" {
			t.Errorf("functionResponse[%d].name = %q, mau cuaca", i, part.FunctionResponse.Name)
		}
	}
	if string(results.Parts[0].FunctionResponse.Response) != `{"suhu":24}` {
		t.Errorf("response[0] = %s, mau objek JSON apa adanya", results.Parts[0].FunctionResponse.Response)
	}
	if string(results.Parts[1].FunctionResponse.Response) != `{"output":"26 derajat"}` {
		t.Errorf("response[1] = %s, mau dibungkus objek", results.Parts[1].FunctionResponse.Response)
	}
}

// Nama fungsi masih bisa dipulihkan dari id buatan walau klien tidak menyertakan ulang
// pesan assistant yang memuat panggilannya.
func TestNamaFungsiDipulihkanDariIDTanpaPesanAssistant(t *testing.T) {
	p, log := newScriptedServer(t, textResponse)
	_, err := p.ChatCompletion(context.Background(), &providers.ChatRequest{
		Model: "gemini-2.5-flash",
		Messages: []providers.Message{
			{Role: providers.RoleUser, Content: "cuaca?"},
			{Role: providers.RoleTool, ToolCallID: "call_3_cari_cuaca_kota", Content: "cerah"},
		},
	})
	if err != nil {
		t.Fatalf("ChatCompletion() error: %v", err)
	}
	parts := decodeRequest(t, log.raw(t, 0)).Contents[0].Parts
	last := parts[len(parts)-1]
	if last.FunctionResponse == nil || last.FunctionResponse.Name != "cari_cuaca_kota" {
		t.Errorf("functionResponse = %+v, mau nama dengan garis bawah utuh", last.FunctionResponse)
	}
}

func TestIDPanggilanToolBolakBalik(t *testing.T) {
	tests := []struct {
		id      string
		mauNama string
	}{
		{synthesizeCallID(0, "cuaca"), "cuaca"},
		{synthesizeCallID(12, "cari_cuaca_kota"), "cari_cuaca_kota"},
		{"call_0_", ""},
		{"call__cuaca", ""},
		{"call_abc_cuaca", ""},
		{"cuaca", ""},
		{"", ""},
		{"call_1", ""},
	}
	for _, tc := range tests {
		if got := functionNameFromCallID(tc.id); got != tc.mauNama {
			t.Errorf("functionNameFromCallID(%q) = %q, mau %q", tc.id, got, tc.mauNama)
		}
	}
}

func TestFunctionResponsePayload(t *testing.T) {
	tests := []struct {
		in  string
		mau string
	}{
		{`{"suhu":24}`, `{"suhu":24}`},
		{"  {\"a\":1}  ", `{"a":1}`},
		{"teks biasa", `{"output":"teks biasa"}`},
		{"[1,2]", `{"output":"[1,2]"}`},
		{"", `{"output":""}`},
	}
	for _, tc := range tests {
		if got := string(functionResponsePayload(tc.in)); got != tc.mau {
			t.Errorf("functionResponsePayload(%q) = %s, mau %s", tc.in, got, tc.mau)
		}
	}
}

func TestUsageMemetakanCacheDanPenalaran(t *testing.T) {
	body := `{
	  "candidates": [{"content":{"role":"model","parts":[{"text":"ya"}]},"finishReason":"STOP"}],
	  "usageMetadata": {
	    "promptTokenCount": 100,
	    "candidatesTokenCount": 20,
	    "cachedContentTokenCount": 30,
	    "thoughtsTokenCount": 7,
	    "totalTokenCount": 127
	  }
	}`
	p, _ := newScriptedServer(t, body)

	resp, err := p.ChatCompletion(context.Background(), simpleRequest())
	if err != nil {
		t.Fatalf("ChatCompletion() error: %v", err)
	}
	// promptTokenCount Gemini SUDAH memuat token cache, jadi tidak boleh dijumlahkan lagi.
	if resp.Usage.InputTokens != 100 {
		t.Errorf("InputTokens = %d, mau 100 (cache sudah termasuk)", resp.Usage.InputTokens)
	}
	if resp.Usage.CachedInputTokens != 30 {
		t.Errorf("CachedInputTokens = %d, mau 30", resp.Usage.CachedInputTokens)
	}
	// thoughtsTokenCount ditagih sebagai keluaran dan tidak termasuk candidatesTokenCount.
	if resp.Usage.OutputTokens != 27 {
		t.Errorf("OutputTokens = %d, mau 27 (20 + 7 penalaran)", resp.Usage.OutputTokens)
	}
	if resp.Usage.ReasoningTokens != 7 {
		t.Errorf("ReasoningTokens = %d, mau 7", resp.Usage.ReasoningTokens)
	}
	if resp.Usage.TotalTokens != 127 {
		t.Errorf("TotalTokens = %d, mau 127", resp.Usage.TotalTokens)
	}
}

func TestUsageTanpaTotalDihitungSendiri(t *testing.T) {
	body := `{
	  "candidates": [{"content":{"role":"model","parts":[{"text":"ya"}]},"finishReason":"STOP"}],
	  "usageMetadata": {"promptTokenCount": 5, "candidatesTokenCount": 3}
	}`
	p, _ := newScriptedServer(t, body)
	resp, err := p.ChatCompletion(context.Background(), simpleRequest())
	if err != nil {
		t.Fatalf("ChatCompletion() error: %v", err)
	}
	if resp.Usage.TotalTokens != 8 {
		t.Errorf("TotalTokens = %d, mau 8", resp.Usage.TotalTokens)
	}

	if got := toCanonicalUsage(nil); got != (providers.Usage{}) {
		t.Errorf("toCanonicalUsage(nil) = %+v, mau nol", got)
	}
}

func TestPemetaanFinishReason(t *testing.T) {
	tests := map[string]string{
		"STOP":                    providers.FinishStop,
		"MAX_TOKENS":              providers.FinishLength,
		"SAFETY":                  providers.FinishContentFilter,
		"RECITATION":              providers.FinishContentFilter,
		"BLOCKLIST":               providers.FinishContentFilter,
		"PROHIBITED_CONTENT":      providers.FinishContentFilter,
		"SPII":                    providers.FinishContentFilter,
		"IMAGE_SAFETY":            providers.FinishContentFilter,
		"OTHER":                   providers.FinishStop,
		"MALFORMED_FUNCTION_CALL": providers.FinishStop,
		"":                        "",
		"stop":                    providers.FinishStop,
	}
	for upstream, mau := range tests {
		if got := mapFinishReason(upstream); got != mau {
			t.Errorf("mapFinishReason(%q) = %q, mau %q", upstream, got, mau)
		}
	}

	p, _ := newScriptedServer(t, `{
	  "candidates":[{"content":{"role":"model","parts":[{"text":"terpotong"}]},"finishReason":"SAFETY"}],
	  "usageMetadata":{"promptTokenCount":1,"candidatesTokenCount":1,"totalTokenCount":2}
	}`)
	resp, err := p.ChatCompletion(context.Background(), simpleRequest())
	if err != nil {
		t.Fatalf("ChatCompletion() error: %v", err)
	}
	if resp.Choices[0].FinishReason != providers.FinishContentFilter {
		t.Errorf("FinishReason = %q, mau content_filter", resp.Choices[0].FinishReason)
	}
}

// blockReason berarti permintaan diblokir kebijakan konten. Klasifikasinya penting:
// content_filter sengaja TIDAK di-failover.
func TestBlockReasonMenjadiContentFilter(t *testing.T) {
	p, _ := newScriptedServer(t, `{"promptFeedback":{"blockReason":"SAFETY"}}`)

	_, err := p.ChatCompletion(context.Background(), simpleRequest())
	e := providers.AsError(err)
	if e == nil {
		t.Fatalf("error = %v, mau *providers.Error", err)
	}
	if e.Kind != providers.ErrKindContentFilter {
		t.Errorf("Kind = %q, mau %q", e.Kind, providers.ErrKindContentFilter)
	}
	if !strings.Contains(e.Message, "SAFETY") {
		t.Errorf("pesan tidak menyebut alasan blokir: %q", e.Message)
	}
	if e.Failoverable() {
		t.Error("Failoverable() = true; muatan yang sama tidak boleh dikirim ke provider lain")
	}
	if e.Retryable() {
		t.Error("Retryable() = true; mengulang akan diblokir dengan alasan yang sama")
	}
}

func TestErrorDiBodyRespons(t *testing.T) {
	p, _ := newScriptedServer(t, `{"error":{"code":429,"message":"kuota habis","status":"RESOURCE_EXHAUSTED"}}`)
	_, err := p.ChatCompletion(context.Background(), simpleRequest())
	e := providers.AsError(err)
	if e == nil {
		t.Fatalf("error = %v", err)
	}
	if e.Kind != providers.ErrKindRateLimit {
		t.Errorf("Kind = %q, mau %q", e.Kind, providers.ErrKindRateLimit)
	}
	if e.UpstreamCode != "RESOURCE_EXHAUSTED" {
		t.Errorf("UpstreamCode = %q", e.UpstreamCode)
	}

	p2, _ := newScriptedServer(t, `{"error":{"code":500}}`)
	_, err = p2.ChatCompletion(context.Background(), simpleRequest())
	if e := providers.AsError(err); e == nil || e.Message == "" {
		t.Errorf("error tanpa pesan seharusnya tetap punya keterangan: %v", err)
	}
}

func TestResponseFormatDiterjemahkan(t *testing.T) {
	tests := []struct {
		nama       string
		raw        string
		mauMime    string
		mauSkemaOK bool
	}{
		{"json_object", `{"type":"json_object"}`, "application/json", false},
		{"json_schema", `{"type":"json_schema","json_schema":{"name":"x","schema":{"type":"object","additionalProperties":false}}}`,
			"application/json", true},
		{"json_schema tanpa skema", `{"type":"json_schema","json_schema":{"name":"x"}}`, "application/json", false},
		{"text dibiarkan", `{"type":"text"}`, "", false},
		{"bukan objek diabaikan", `"json"`, "", false},
	}

	for _, tc := range tests {
		t.Run(tc.nama, func(t *testing.T) {
			p, log := newScriptedServer(t, textResponse)
			req := simpleRequest()
			req.ResponseFormat = json.RawMessage(tc.raw)
			if _, err := p.ChatCompletion(context.Background(), req); err != nil {
				t.Fatalf("ChatCompletion() error: %v", err)
			}

			cfg := decodeRequest(t, log.raw(t, 0)).GenerationConfig
			mime, _ := cfg["responseMimeType"].(string)
			if mime != tc.mauMime {
				t.Errorf("responseMimeType = %q, mau %q", mime, tc.mauMime)
			}
			schema, ada := cfg["responseSchema"]
			if ada != tc.mauSkemaOK {
				t.Errorf("responseSchema ada = %v, mau %v", ada, tc.mauSkemaOK)
			}
			if tc.mauSkemaOK {
				obj, _ := schema.(map[string]any)
				if _, buruk := obj["additionalProperties"]; buruk {
					t.Error("responseSchema masih memuat additionalProperties")
				}
			}
		})
	}
}

func TestSanitizeSchemaMenoleransiMasukanBuruk(t *testing.T) {
	if got := sanitizeSchema(nil); got != nil {
		t.Errorf("sanitizeSchema(nil) = %s, mau nil", got)
	}
	raw := json.RawMessage(`{bukan json`)
	if got := string(sanitizeSchema(raw)); got != string(raw) {
		t.Errorf("sanitizeSchema(rusak) = %s, mau diteruskan apa adanya", got)
	}
	if got := string(sanitizeSchema(json.RawMessage(`[{"$schema":"x","type":"string"}]`))); strings.Contains(got, "$schema") {
		t.Errorf("array skema tidak dibersihkan: %s", got)
	}
}

func TestReasoningEffortMenjadiThinkingConfig(t *testing.T) {
	tests := []struct {
		effort     string
		mauBudget  float64
		mauThought bool
		mauKosong  bool
	}{
		{effort: "minimal", mauBudget: 0, mauThought: false},
		{effort: "low", mauBudget: 1024, mauThought: true},
		{effort: "medium", mauBudget: 8192, mauThought: true},
		{effort: "high", mauBudget: 24576, mauThought: true},
		{effort: "", mauKosong: true},
		{effort: "entah", mauKosong: true},
	}

	for _, tc := range tests {
		t.Run("effort="+tc.effort, func(t *testing.T) {
			p, log := newScriptedServer(t, textResponse)
			req := simpleRequest()
			req.ReasoningEffort = tc.effort
			if _, err := p.ChatCompletion(context.Background(), req); err != nil {
				t.Fatalf("ChatCompletion() error: %v", err)
			}

			cfg := decodeRequest(t, log.raw(t, 0)).GenerationConfig
			thinking, ada := cfg["thinkingConfig"].(map[string]any)
			if tc.mauKosong {
				if ada {
					t.Fatalf("thinkingConfig = %v, mau tidak terkirim", thinking)
				}
				return
			}
			if !ada {
				t.Fatalf("thinkingConfig tidak terkirim: %v", cfg)
			}
			if thinking["thinkingBudget"] != tc.mauBudget {
				t.Errorf("thinkingBudget = %v, mau %v", thinking["thinkingBudget"], tc.mauBudget)
			}
			gotThought, _ := thinking["includeThoughts"].(bool)
			if gotThought != tc.mauThought {
				t.Errorf("includeThoughts = %v, mau %v", gotThought, tc.mauThought)
			}
		})
	}
}

func TestMultimodalDiterjemahkan(t *testing.T) {
	p, log := newScriptedServer(t, textResponse)

	req := &providers.ChatRequest{
		Model: "gemini-2.5-flash",
		Messages: []providers.Message{{Role: providers.RoleUser, Parts: []providers.ContentPart{
			{Type: providers.PartTypeText, Text: "apa ini?"},
			{Type: providers.PartTypeText, Text: ""},
			{Type: providers.PartTypeImageURL, ImageURL: &providers.ImageURL{URL: "data:image/png;base64,AAAB"}},
			{Type: providers.PartTypeImageURL, ImageURL: &providers.ImageURL{URL: "https://contoh.test/a.png"}},
			{Type: providers.PartTypeImageURL, ImageURL: &providers.ImageURL{URL: ""}},
			{Type: providers.PartTypeAudio, Audio: &providers.Audio{Data: "QUJD", Format: "mp3"}},
			{Type: providers.PartTypeAudio, Audio: &providers.Audio{Data: "QUJD", Format: "audio/wav"}},
			{Type: providers.PartTypeAudio},
			{Type: "jenis_baru", Text: "diabaikan"},
		}}},
	}
	if _, err := p.ChatCompletion(context.Background(), req); err != nil {
		t.Fatalf("ChatCompletion() error: %v", err)
	}

	parts := decodeRequest(t, log.raw(t, 0)).Contents[0].Parts
	if len(parts) != 5 {
		t.Fatalf("jumlah part = %d, mau 5: %+v", len(parts), parts)
	}
	if parts[1].InlineData == nil || parts[1].InlineData.MimeType != "image/png" || parts[1].InlineData.Data != "AAAB" {
		t.Errorf("data URI tidak menjadi inlineData: %+v", parts[1])
	}
	// URL biasa menjadi fileData; gateway TIDAK mengunduhnya sendiri (itu akan menjadi SSRF).
	if parts[2].FileData == nil || parts[2].FileData.FileURI != "https://contoh.test/a.png" {
		t.Errorf("URL gambar tidak menjadi fileData: %+v", parts[2])
	}
	if parts[3].InlineData == nil || parts[3].InlineData.MimeType != "audio/mp3" {
		t.Errorf("audio tidak menjadi inlineData: %+v", parts[3])
	}
	if parts[4].InlineData == nil || parts[4].InlineData.MimeType != "audio/wav" {
		t.Errorf("format audio berawalan audio/ digandakan: %+v", parts[4])
	}
}

func TestParseDataURI(t *testing.T) {
	tests := []struct {
		raw         string
		mauType     string
		mauData     string
		mauBerhasil bool
	}{
		{"data:image/jpeg;base64,QUJD", "image/jpeg", "QUJD", true},
		{"data:image/png;charset=utf-8;base64,QQ", "image/png", "QQ", true},
		{"data:;base64,QUJD", "application/octet-stream", "QUJD", true},
		{"data:image/png,QUJD", "", "", false},
		{"data:image/png;base64,", "", "", false},
		{"https://contoh.test/a.png", "", "", false},
		{"data:tanpa-koma", "", "", false},
	}
	for _, tc := range tests {
		gotType, gotData, ok := parseDataURI(tc.raw)
		if ok != tc.mauBerhasil || gotType != tc.mauType || gotData != tc.mauData {
			t.Errorf("parseDataURI(%q) = (%q, %q, %v), mau (%q, %q, %v)",
				tc.raw, gotType, gotData, ok, tc.mauType, tc.mauData, tc.mauBerhasil)
		}
	}
}

func TestPenalaranDipisahDariJawaban(t *testing.T) {
	body := `{
	  "candidates": [{"content":{"role":"model","parts":[
	    {"text":"pikir dulu","thought":true},
	    {"text":"jawaban"}
	  ]},"finishReason":"STOP"}],
	  "usageMetadata":{"promptTokenCount":1,"candidatesTokenCount":1,"thoughtsTokenCount":2,"totalTokenCount":4}
	}`
	p, log := newScriptedServer(t, body, textResponse)

	resp, err := p.ChatCompletion(context.Background(), simpleRequest())
	if err != nil {
		t.Fatalf("ChatCompletion() error: %v", err)
	}
	msg := resp.Choices[0].Message
	if msg.Content != "jawaban" {
		t.Errorf("Content = %q", msg.Content)
	}
	if msg.ReasoningContent != "pikir dulu" {
		t.Errorf("ReasoningContent = %q", msg.ReasoningContent)
	}

	// Penalaran tidak dikirim ulang sebagai riwayat.
	if _, err := p.ChatCompletion(context.Background(), &providers.ChatRequest{
		Model: "gemini-2.5-flash",
		Messages: []providers.Message{
			{Role: providers.RoleUser, Content: "halo"},
			{Role: providers.RoleAssistant, Content: "jawaban", ReasoningContent: "pikir dulu"},
			{Role: providers.RoleUser, Content: "lanjut"},
		},
	}); err != nil {
		t.Fatalf("ChatCompletion() error: %v", err)
	}
	for _, c := range decodeRequest(t, log.raw(t, 1)).Contents {
		for _, part := range c.Parts {
			if part.Thought {
				t.Errorf("part ber-thought terkirim ulang: %+v", part)
			}
		}
	}
}

func TestBanyakKandidatMenjadiBanyakChoice(t *testing.T) {
	body := `{
	  "candidates": [
	    {"content":{"role":"model","parts":[{"text":"satu"}]},"finishReason":"STOP","index":0},
	    {"content":{"role":"model","parts":[{"functionCall":{"name":"cuaca","args":{}}}]},"finishReason":"STOP","index":1}
	  ],
	  "usageMetadata":{"promptTokenCount":1,"candidatesTokenCount":2,"totalTokenCount":3}
	}`
	p, _ := newScriptedServer(t, body)

	resp, err := p.ChatCompletion(context.Background(), simpleRequest())
	if err != nil {
		t.Fatalf("ChatCompletion() error: %v", err)
	}
	if len(resp.Choices) != 2 {
		t.Fatalf("jumlah choice = %d, mau 2", len(resp.Choices))
	}
	if resp.Choices[1].Index != 1 {
		t.Errorf("choice[1].Index = %d", resp.Choices[1].Index)
	}
	if resp.Choices[1].FinishReason != providers.FinishToolCalls {
		t.Errorf("choice[1].FinishReason = %q", resp.Choices[1].FinishReason)
	}
	// Penomoran id panggilan berjalan lintas kandidat supaya tetap unik.
	if resp.Choices[1].Message.ToolCalls[0].ID != "call_0_cuaca" {
		t.Errorf("id panggilan = %q", resp.Choices[1].Message.ToolCalls[0].ID)
	}
	if resp.Choices[1].Message.ToolCalls[0].Function.Arguments != "{}" {
		t.Errorf("argumen kosong = %q, mau {}", resp.Choices[1].Message.ToolCalls[0].Function.Arguments)
	}
}

func TestIdentitasResponsDiisi(t *testing.T) {
	p, _ := newScriptedServer(t, textResponse)
	resp, err := p.ChatCompletion(context.Background(), simpleRequest())
	if err != nil {
		t.Fatalf("ChatCompletion() error: %v", err)
	}
	if resp.ID != "resp-1" {
		t.Errorf("ID = %q, mau responseId upstream", resp.ID)
	}
	if resp.Model != "gemini-2.5-flash-001" {
		t.Errorf("Model = %q, mau modelVersion upstream", resp.Model)
	}
	if resp.Created == 0 {
		t.Error("Created = 0; klien berdialek OpenAI mengharapkannya terisi")
	}

	// Tanpa responseId dan modelVersion, keduanya diisi dari yang tersedia.
	p2, _ := newScriptedServer(t, `{
	  "candidates":[{"content":{"role":"model","parts":[{"text":"x"}]},"finishReason":"STOP"}]
	}`)
	resp2, err := p2.ChatCompletion(context.Background(), simpleRequest())
	if err != nil {
		t.Fatalf("ChatCompletion() error: %v", err)
	}
	if resp2.ID == "" {
		t.Error("ID kosong")
	}
	if resp2.Model != "gemini-2.5-flash" {
		t.Errorf("Model = %q, mau nama model yang diminta", resp2.Model)
	}
}

// Raw wajib berisi bentuk dialek OpenAI hasil terjemahan, karena itulah yang diteruskan ke
// klien gateway — bukan body Gemini apa adanya.
func TestRawBerisiDialekOpenAI(t *testing.T) {
	body := `{
	  "candidates": [{"content":{"role":"model","parts":[
	    {"text":"pikir","thought":true},
	    {"functionCall":{"name":"cuaca","args":{"kota":"Solo"}}}
	  ]},"finishReason":"STOP","index":0}],
	  "usageMetadata":{"promptTokenCount":9,"candidatesTokenCount":4,"cachedContentTokenCount":2,"thoughtsTokenCount":3,"totalTokenCount":16},
	  "modelVersion":"gemini-2.5-flash-001",
	  "responseId":"resp-raw"
	}`
	p, _ := newScriptedServer(t, body)

	resp, err := p.ChatCompletion(context.Background(), simpleRequest())
	if err != nil {
		t.Fatalf("ChatCompletion() error: %v", err)
	}

	var raw struct {
		ID      string `json:"id"`
		Object  string `json:"object"`
		Created int64  `json:"created"`
		Model   string `json:"model"`
		Choices []struct {
			Index   int `json:"index"`
			Message struct {
				Role             string          `json:"role"`
				Content          json.RawMessage `json:"content"`
				ReasoningContent string          `json:"reasoning_content"`
				ToolCalls        []struct {
					ID       string `json:"id"`
					Type     string `json:"type"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
			FinishReason *string `json:"finish_reason"`
		} `json:"choices"`
		Usage struct {
			PromptTokens        int `json:"prompt_tokens"`
			CompletionTokens    int `json:"completion_tokens"`
			TotalTokens         int `json:"total_tokens"`
			PromptTokensDetails *struct {
				CachedTokens int `json:"cached_tokens"`
			} `json:"prompt_tokens_details"`
			CompletionTokensDetails *struct {
				ReasoningTokens int `json:"reasoning_tokens"`
			} `json:"completion_tokens_details"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(resp.Raw, &raw); err != nil {
		t.Fatalf("Raw bukan JSON dialek OpenAI: %v\nraw: %s", err, resp.Raw)
	}
	if raw.Object != "chat.completion" {
		t.Errorf("object = %q", raw.Object)
	}
	if raw.ID != "resp-raw" || raw.Model != "gemini-2.5-flash-001" || raw.Created == 0 {
		t.Errorf("identitas = %+v", raw)
	}
	choice := raw.Choices[0]
	if string(choice.Message.Content) != "null" {
		t.Errorf("content = %s, mau null saat hanya ada tool call", choice.Message.Content)
	}
	if choice.Message.ReasoningContent != "pikir" {
		t.Errorf("reasoning_content = %q", choice.Message.ReasoningContent)
	}
	if choice.FinishReason == nil || *choice.FinishReason != providers.FinishToolCalls {
		t.Errorf("finish_reason = %v", choice.FinishReason)
	}
	if len(choice.Message.ToolCalls) != 1 {
		t.Fatalf("jumlah tool call = %d", len(choice.Message.ToolCalls))
	}
	tc := choice.Message.ToolCalls[0]
	if tc.ID != "call_0_cuaca" || tc.Type != "function" || tc.Function.Name != "cuaca" {
		t.Errorf("tool call = %+v", tc)
	}
	if tc.Function.Arguments != `{"kota":"Solo"}` {
		t.Errorf("arguments = %q, mau string JSON", tc.Function.Arguments)
	}
	if raw.Usage.PromptTokens != 9 || raw.Usage.CompletionTokens != 7 || raw.Usage.TotalTokens != 16 {
		t.Errorf("usage = %+v", raw.Usage)
	}
	if raw.Usage.PromptTokensDetails == nil || raw.Usage.PromptTokensDetails.CachedTokens != 2 {
		t.Errorf("cached_tokens = %+v", raw.Usage.PromptTokensDetails)
	}
	if raw.Usage.CompletionTokensDetails == nil || raw.Usage.CompletionTokensDetails.ReasoningTokens != 3 {
		t.Errorf("reasoning_tokens = %+v", raw.Usage.CompletionTokensDetails)
	}
}
