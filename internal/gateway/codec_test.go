package gateway

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
	"github.com/NexGen-X/Route-X/internal/providers"
	"github.com/NexGen-X/Route-X/internal/providers/openai"
	"github.com/NexGen-X/Route-X/internal/security"
)

// --- Bentuk konten -----------------------------------------------------------

func TestDecodeChatRequestKontenBentukString(t *testing.T) {
	req, err := DecodeChatRequest([]byte(`{
		"model": "gpt-4o-mini",
		"messages": [{"role": "user", "content": "halo"}]
	}`))
	if err != nil {
		t.Fatalf("DecodeChatRequest: %v", err)
	}
	if req.Model != "gpt-4o-mini" {
		t.Errorf("Model = %q, mau %q", req.Model, "gpt-4o-mini")
	}
	if len(req.Messages) != 1 {
		t.Fatalf("jumlah pesan = %d, mau 1", len(req.Messages))
	}
	m := req.Messages[0]
	if m.Role != providers.RoleUser {
		t.Errorf("Role = %q, mau %q", m.Role, providers.RoleUser)
	}
	if m.Content != "halo" {
		t.Errorf("Content = %q, mau %q", m.Content, "halo")
	}
	// Bentuk string TIDAK boleh ikut mengisi Parts: adapter memilih bentuk body dari
	// IsMultimodal(), jadi Parts yang terisi diam-diam mengubah bentuk yang dikirim.
	if m.IsMultimodal() {
		t.Errorf("pesan bentuk string dilaporkan multimodal, Parts = %#v", m.Parts)
	}
}

func TestDecodeChatRequestKontenBentukArray(t *testing.T) {
	req, err := DecodeChatRequest([]byte(`{
		"model": "m",
		"messages": [{"role": "user", "content": [
			{"type": "text", "text": "apa ini"},
			{"type": "text", "text": "dan ini"}
		]}]
	}`))
	if err != nil {
		t.Fatalf("DecodeChatRequest: %v", err)
	}
	m := req.Messages[0]
	if !m.IsMultimodal() {
		t.Fatalf("pesan bentuk array tidak dilaporkan multimodal")
	}
	if len(m.Parts) != 2 {
		t.Fatalf("jumlah bagian = %d, mau 2", len(m.Parts))
	}
	if m.Content != "" {
		t.Errorf("Content = %q, mau kosong pada bentuk array", m.Content)
	}
	if got := m.Text(); got != "apa ini\ndan ini" {
		t.Errorf("Text() = %q", got)
	}
}

func TestDecodeChatRequestMultimodalGambarDanAudio(t *testing.T) {
	req, err := DecodeChatRequest([]byte(`{
		"model": "m",
		"messages": [{"role": "user", "content": [
			{"type": "text", "text": "lihat"},
			{"type": "image_url", "image_url": {"url": "https://contoh/x.png", "detail": "low"}},
			{"type": "input_audio", "input_audio": {"data": "AAAA", "format": "wav"}}
		]}]
	}`))
	if err != nil {
		t.Fatalf("DecodeChatRequest: %v", err)
	}
	parts := req.Messages[0].Parts
	if len(parts) != 3 {
		t.Fatalf("jumlah bagian = %d, mau 3", len(parts))
	}
	if parts[1].ImageURL == nil || parts[1].ImageURL.URL != "https://contoh/x.png" {
		t.Errorf("bagian gambar = %#v", parts[1])
	}
	if parts[1].ImageURL.Detail != "low" {
		t.Errorf("detail gambar = %q, mau %q", parts[1].ImageURL.Detail, "low")
	}
	if parts[2].Audio == nil || parts[2].Audio.Data != "AAAA" || parts[2].Audio.Format != "wav" {
		t.Errorf("bagian audio = %#v", parts[2])
	}
}

func TestDecodeChatRequestToolCallDanPeranTool(t *testing.T) {
	req, err := DecodeChatRequest([]byte(`{
		"model": "m",
		"messages": [
			{"role": "user", "content": "cuaca di mana?"},
			{"role": "assistant", "content": null, "tool_calls": [
				{"id": "call_1", "type": "function", "function": {"name": "cuaca", "arguments": "{\"kota\":\"Jakarta\"}"}}
			]},
			{"role": "tool", "tool_call_id": "call_1", "content": "31 derajat"}
		],
		"tools": [{"type": "function", "function": {"name": "cuaca", "parameters": {"type": "object"}}}]
	}`))
	if err != nil {
		t.Fatalf("DecodeChatRequest: %v", err)
	}
	if len(req.Messages) != 3 {
		t.Fatalf("jumlah pesan = %d, mau 3", len(req.Messages))
	}

	asisten := req.Messages[1]
	if len(asisten.ToolCalls) != 1 {
		t.Fatalf("jumlah tool call = %d, mau 1", len(asisten.ToolCalls))
	}
	tc := asisten.ToolCalls[0]
	if tc.ID != "call_1" || tc.Type != "function" || tc.Function.Name != "cuaca" {
		t.Errorf("tool call = %#v", tc)
	}
	// Arguments tetap string JSON apa adanya, tidak diurai. Model kadang menghasilkan JSON
	// tidak sah, dan menguraikannya di sini berarti gateway menolak permintaan yang
	// sebenarnya harus diteruskan.
	if tc.Function.Arguments != `{"kota":"Jakarta"}` {
		t.Errorf("Arguments = %q", tc.Function.Arguments)
	}
	// Pesan asisten dengan content null tetap sah karena ia membawa tool call.
	if asisten.Content != "" {
		t.Errorf("Content asisten = %q, mau kosong", asisten.Content)
	}

	alat := req.Messages[2]
	if alat.Role != providers.RoleTool || alat.ToolCallID != "call_1" {
		t.Errorf("pesan tool = %#v", alat)
	}
	if len(req.Tools) != 1 || req.Tools[0].Function.Name != "cuaca" {
		t.Errorf("Tools = %#v", req.Tools)
	}
}

func TestDecodeChatRequestArgumentsBerupaObjekDitolakDenganJalurnya(t *testing.T) {
	_, err := DecodeChatRequest([]byte(`{
		"model": "m",
		"messages": [{"role": "assistant", "content": null, "tool_calls": [
			{"id": "c", "function": {"name": "f", "arguments": {"bukan": "string"}}}
		]}]
	}`))
	if err == nil {
		t.Fatal("arguments berbentuk objek diterima, mau ditolak")
	}
	if !strings.Contains(err.Error(), "messages[0].tool_calls[0].function.arguments") {
		t.Errorf("pesan error tidak menyebut jalur field: %v", err)
	}
}

// --- Extra -------------------------------------------------------------------

func TestDecodeChatRequestExtraHanyaFieldTakDikenal(t *testing.T) {
	req, err := DecodeChatRequest([]byte(`{
		"model": "m",
		"messages": [{"role": "user", "content": "hai"}],
		"max_tokens": 100,
		"temperature": 0.2,
		"top_p": 0.9,
		"stop": ["X"],
		"stream": true,
		"tools": [{"type": "function", "function": {"name": "f"}}],
		"tool_choice": "auto",
		"parallel_tool_calls": false,
		"response_format": {"type": "json_object"},
		"seed": 7,
		"reasoning_effort": "high",
		"user": "u-1",
		"n": 2,
		"stream_options": {"include_usage": true},
		"frequency_penalty": 0.5,
		"logit_bias": {"50256": -100},
		"field_masa_depan": {"apa pun": [1, 2, 3]}
	}`))
	if err != nil {
		t.Fatalf("DecodeChatRequest: %v", err)
	}

	// Setiap field yang punya tempat kanonik harus TIDAK ada di Extra. Kalau ada, adapter
	// openai akan mengirimnya dua kali — dan pada "stream" itu berarti mode transport
	// terbalik di tengah jalan.
	kanonik := []string{
		"model", "messages", "max_tokens", "temperature", "top_p", "stop", "stream",
		"tools", "tool_choice", "parallel_tool_calls", "response_format", "seed",
		"reasoning_effort", "user",
	}
	for _, k := range kanonik {
		if _, ada := req.Extra[k]; ada {
			t.Errorf("field kanonik %q ikut masuk ke Extra", k)
		}
	}

	mau := []string{"n", "stream_options", "frequency_penalty", "logit_bias", "field_masa_depan"}
	for _, k := range mau {
		if _, ada := req.Extra[k]; !ada {
			t.Errorf("field tak dikenal %q tidak masuk ke Extra", k)
		}
	}
	if len(req.Extra) != len(mau) {
		t.Errorf("jumlah Extra = %d, mau %d; isi = %v", len(req.Extra), len(mau), kunci(req.Extra))
	}

	// Nilai Extra harus utuh apa adanya, bukan hasil penyusunan ulang.
	if got := string(req.Extra["field_masa_depan"]); got != `{"apa pun": [1, 2, 3]}` {
		t.Errorf("nilai Extra berubah: %s", got)
	}

	// Sekalian memeriksa field kanonik benar-benar terisi, bukan hanya tidak di Extra.
	if req.MaxTokens == nil || *req.MaxTokens != 100 {
		t.Errorf("MaxTokens = %v", req.MaxTokens)
	}
	if req.Temperature == nil || *req.Temperature != 0.2 {
		t.Errorf("Temperature = %v", req.Temperature)
	}
	if !req.Stream {
		t.Error("Stream = false, mau true")
	}
	if req.ParallelToolCalls == nil || *req.ParallelToolCalls {
		t.Errorf("ParallelToolCalls = %v", req.ParallelToolCalls)
	}
	if req.Seed == nil || *req.Seed != 7 {
		t.Errorf("Seed = %v", req.Seed)
	}
	if req.ReasoningEffort != "high" || req.User != "u-1" {
		t.Errorf("ReasoningEffort = %q, User = %q", req.ReasoningEffort, req.User)
	}
	if string(req.ResponseFormat) != `{"type": "json_object"}` {
		t.Errorf("ResponseFormat = %s", req.ResponseFormat)
	}
}

func TestDecodeChatRequestTanpaFieldTakDikenalExtraNil(t *testing.T) {
	req, err := DecodeChatRequest([]byte(`{"model": "m", "messages": [{"role": "user", "content": "a"}]}`))
	if err != nil {
		t.Fatalf("DecodeChatRequest: %v", err)
	}
	if req.Extra != nil {
		t.Errorf("Extra = %v, mau nil", req.Extra)
	}
}

// TestDecodeChatRequestNullTidakDiteruskan menjaga satu jalur yang mudah terlewat: field
// kanonik bernilai null tidak boleh berakhir di Extra, karena dari sana ia akan dikirim
// ulang ke upstream sebagai null padahal bentuk kanoniknya sudah menyatakan "tidak ada".
func TestDecodeChatRequestNullTidakDiteruskan(t *testing.T) {
	req, err := DecodeChatRequest([]byte(`{
		"model": "m",
		"messages": [{"role": "user", "content": "a"}],
		"max_tokens": null,
		"stop": null,
		"tool_choice": null,
		"temperature": null
	}`))
	if err != nil {
		t.Fatalf("DecodeChatRequest: %v", err)
	}
	if req.MaxTokens != nil || req.Temperature != nil {
		t.Errorf("MaxTokens = %v, Temperature = %v; mau keduanya nil", req.MaxTokens, req.Temperature)
	}
	if req.Stop != nil {
		t.Errorf("Stop = %#v, mau nil (bukan [\"\"])", req.Stop)
	}
	if req.ToolChoice != nil {
		t.Errorf("ToolChoice = %s, mau nil", req.ToolChoice)
	}
	if len(req.Extra) != 0 {
		t.Errorf("Extra = %v, mau kosong", kunci(req.Extra))
	}
}

func TestDecodeChatRequestStopDuaBentuk(t *testing.T) {
	for _, tc := range []struct {
		nama string
		body string
		mau  []string
	}{
		{"string tunggal", `{"model":"m","messages":[{"role":"user","content":"a"}],"stop":"AKHIR"}`, []string{"AKHIR"}},
		{"array", `{"model":"m","messages":[{"role":"user","content":"a"}],"stop":["A","B"]}`, []string{"A", "B"}},
	} {
		t.Run(tc.nama, func(t *testing.T) {
			req, err := DecodeChatRequest([]byte(tc.body))
			if err != nil {
				t.Fatalf("DecodeChatRequest: %v", err)
			}
			if len(req.Stop) != len(tc.mau) {
				t.Fatalf("Stop = %#v, mau %#v", req.Stop, tc.mau)
			}
			for i := range tc.mau {
				if req.Stop[i] != tc.mau[i] {
					t.Errorf("Stop[%d] = %q, mau %q", i, req.Stop[i], tc.mau[i])
				}
			}
		})
	}
}

// kunci mengembalikan nama field sebuah peta Extra untuk pesan kegagalan.
func kunci(m map[string]json.RawMessage) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// --- Penolakan ---------------------------------------------------------------

// rahasia adalah penanda yang ditaruh di posisi konten pengguna. Tidak satu pun pesan
// error boleh memuatnya: pesan-pesan itu menjadi badan 400 sekaligus masuk log bersama,
// dan body permintaan adalah milik penyewa yang mengirimnya.
const rahasia = "RAHASIA-MILIK-PENYEWA"

func TestDecodeChatRequestPenolakan(t *testing.T) {
	for _, tc := range []struct {
		nama     string
		body     string
		sebutkan string
	}{
		{"body kosong", ``, "kosong"},
		{"bukan JSON", `{bukan json`, "objek JSON"},
		{"array, bukan objek", `[1,2,3]`, "objek JSON"},
		{"literal null", `null`, "objek JSON"},
		{"tanpa model", `{"messages":[{"role":"user","content":"a"}]}`, `"model"`},
		{"model kosong", `{"model":"","messages":[{"role":"user","content":"a"}]}`, `"model"`},
		{"model bukan string", `{"model":5,"messages":[{"role":"user","content":"a"}]}`, `"model"`},
		{"tanpa messages", `{"model":"m"}`, `"messages"`},
		{"messages kosong", `{"model":"m","messages":[]}`, `"messages"`},
		{"messages bukan array", `{"model":"m","messages":{"role":"user"}}`, `"messages"`},
		{"role kosong", `{"model":"m","messages":[{"content":"` + rahasia + `"}]}`, `"messages[0].role"`},
		{"pesan tanpa isi", `{"model":"m","messages":[{"role":"user"}]}`, `"messages[0]"`},
		{"content bentuk angka", `{"model":"m","messages":[{"role":"user","content":123}]}`, `"messages[0].content"`},
		{"bagian tanpa type", `{"model":"m","messages":[{"role":"user","content":[{"text":"` + rahasia + `"}]}]}`, `"messages[0].content[0].type"`},
		{"bagian type tak dikenal", `{"model":"m","messages":[{"role":"user","content":[{"type":"input_video","url":"` + rahasia + `"}]}]}`, `"messages[0].content[0].type"`},
		{"bagian text tanpa text", `{"model":"m","messages":[{"role":"user","content":[{"type":"text"}]}]}`, `"messages[0].content[0].text"`},
		{"gambar tanpa url", `{"model":"m","messages":[{"role":"user","content":[{"type":"image_url","image_url":{}}]}]}`, `"messages[0].content[0].image_url.url"`},
		{"audio tanpa format", `{"model":"m","messages":[{"role":"user","content":[{"type":"input_audio","input_audio":{"data":"` + rahasia + `"}}]}]}`, `"messages[0].content[0].input_audio.format"`},
		{"tool tanpa nama fungsi", `{"model":"m","messages":[{"role":"user","content":"a"}],"tools":[{"type":"function","function":{}}]}`, `"tools[0].function.name"`},
		{"stop bentuk objek", `{"model":"m","messages":[{"role":"user","content":"a"}],"stop":{"x":1}}`, `"stop"`},
	} {
		t.Run(tc.nama, func(t *testing.T) {
			_, err := DecodeChatRequest([]byte(tc.body))
			if err == nil {
				t.Fatal("body tidak sah diterima, mau ditolak")
			}
			if !strings.Contains(err.Error(), tc.sebutkan) {
				t.Errorf("pesan error tidak menyebut %s: %v", tc.sebutkan, err)
			}
			if strings.Contains(err.Error(), rahasia) {
				t.Errorf("pesan error memuat isi permintaan pengguna: %v", err)
			}
		})
	}
}

// --- Kemampuan ---------------------------------------------------------------

func TestRequiredCapabilities(t *testing.T) {
	teks := func(s string) []providers.Message {
		return []providers.Message{{Role: providers.RoleUser, Content: s}}
	}
	bagian := func(p ...providers.ContentPart) []providers.Message {
		return []providers.Message{{Role: providers.RoleUser, Parts: p}}
	}

	for _, tc := range []struct {
		nama string
		req  *providers.ChatRequest
		mau  []string
	}{
		{"teks biasa", &providers.ChatRequest{Messages: teks("a")}, []string{upstream.CapText}},
		{"nil", nil, []string{upstream.CapText}},
		{
			"ada gambar",
			&providers.ChatRequest{Messages: bagian(
				providers.ContentPart{Type: providers.PartTypeText, Text: "a"},
				providers.ContentPart{Type: providers.PartTypeImageURL, ImageURL: &providers.ImageURL{URL: "u"}},
			)},
			[]string{upstream.CapText, upstream.CapVision},
		},
		{
			// Audio belum punya nilai kemampuan di skema, jadi tidak menuntut apa pun —
			// dan yang penting: TIDAK dipetakan ke vision.
			"ada audio saja",
			&providers.ChatRequest{Messages: bagian(
				providers.ContentPart{Type: providers.PartTypeAudio, Audio: &providers.Audio{Data: "d", Format: "wav"}},
			)},
			[]string{upstream.CapText},
		},
		{
			"ada tools",
			&providers.ChatRequest{Messages: teks("a"), Tools: []providers.Tool{{Function: providers.FunctionDef{Name: "f"}}}},
			[]string{upstream.CapText, upstream.CapTools},
		},
		{
			"tool_choice auto tanpa tools tidak menuntut apa pun",
			&providers.ChatRequest{Messages: teks("a"), ToolChoice: json.RawMessage(`"auto"`)},
			[]string{upstream.CapText},
		},
		{
			"tool_choice none tanpa tools tidak menuntut apa pun",
			&providers.ChatRequest{Messages: teks("a"), ToolChoice: json.RawMessage(`"none"`)},
			[]string{upstream.CapText},
		},
		{
			"tool_choice required tanpa tools tetap menuntut tools",
			&providers.ChatRequest{Messages: teks("a"), ToolChoice: json.RawMessage(`"required"`)},
			[]string{upstream.CapText, upstream.CapTools},
		},
		{
			"tool_choice bentuk objek menuntut tools",
			&providers.ChatRequest{Messages: teks("a"), ToolChoice: json.RawMessage(`{"type":"function","function":{"name":"f"}}`)},
			[]string{upstream.CapText, upstream.CapTools},
		},
		{
			"reasoning_effort",
			&providers.ChatRequest{Messages: teks("a"), ReasoningEffort: "high"},
			[]string{upstream.CapText, upstream.CapReasoning},
		},
		{
			"gabungan",
			&providers.ChatRequest{
				Messages: bagian(providers.ContentPart{Type: providers.PartTypeImageURL, ImageURL: &providers.ImageURL{URL: "u"}}),
				Tools:    []providers.Tool{{Function: providers.FunctionDef{Name: "f"}}},

				ReasoningEffort: "low",
			},
			[]string{upstream.CapText, upstream.CapVision, upstream.CapTools, upstream.CapReasoning},
		},
	} {
		t.Run(tc.nama, func(t *testing.T) {
			got := RequiredCapabilities(tc.req)
			if len(got) != len(tc.mau) {
				t.Fatalf("kemampuan = %v, mau %v", got, tc.mau)
			}
			for i := range tc.mau {
				if got[i] != tc.mau[i] {
					t.Errorf("kemampuan[%d] = %q, mau %q (seluruhnya %v)", i, got[i], tc.mau[i], got)
				}
			}
		})
	}
}

// --- Perkiraan token ---------------------------------------------------------

// TestEstimateTokensMonotonik menjaga sifat yang paling dibutuhkan mesin routing: perkiraan
// yang tidak pernah MENGECIL ketika masukannya membesar. Tanpa itu, penyaring jendela
// konteks bisa meloloskan permintaan panjang yang tadinya ditolak saat masih pendek.
func TestEstimateTokensMonotonik(t *testing.T) {
	sebelumnya := -1
	for _, n := range []int{0, 1, 10, 100, 1000, 10000, 100000} {
		req := &providers.ChatRequest{
			Messages: []providers.Message{{Role: providers.RoleUser, Content: strings.Repeat("a", n)}},
		}
		got := EstimateTokens(req).InputTokens
		if got < sebelumnya {
			t.Errorf("panjang %d menghasilkan %d token, lebih kecil dari %d pada masukan yang lebih pendek",
				n, got, sebelumnya)
		}
		sebelumnya = got
	}
}

func TestEstimateTokensJumlahPesanIkutDihitung(t *testing.T) {
	satu := EstimateTokens(&providers.ChatRequest{
		Messages: []providers.Message{{Role: providers.RoleUser, Content: "aaaa"}},
	}).InputTokens
	banyak := EstimateTokens(&providers.ChatRequest{
		Messages: []providers.Message{
			{Role: providers.RoleUser, Content: "a"},
			{Role: providers.RoleAssistant, Content: "a"},
			{Role: providers.RoleUser, Content: "a"},
			{Role: providers.RoleAssistant, Content: "a"},
		},
	}).InputTokens
	// Panjang teks totalnya sama; yang membedakan hanya pembungkus per pesan. Percakapan
	// panjang berisi pesan pendek harus tetap dihitung lebih besar.
	if banyak <= satu {
		t.Errorf("empat pesan = %d token, satu pesan = %d; mau lebih besar", banyak, satu)
	}
}

func TestEstimateTokensBatasKeluaran(t *testing.T) {
	dasar := []providers.Message{{Role: providers.RoleUser, Content: "a"}}
	empat := 4

	for _, tc := range []struct {
		nama string
		req  *providers.ChatRequest
		mau  int
	}{
		{
			"tanpa batas memakai bawaan",
			&providers.ChatRequest{Messages: dasar},
			perkiraanOutputBawaan,
		},
		{
			"max_tokens dipakai apa adanya",
			&providers.ChatRequest{Messages: dasar, MaxTokens: &empat},
			4,
		},
		{
			"max_completion_tokens dibaca dari Extra",
			&providers.ChatRequest{Messages: dasar, Extra: map[string]json.RawMessage{
				"max_completion_tokens": json.RawMessage(`77`),
			}},
			77,
		},
		{
			"max_tokens menang atas max_completion_tokens",
			&providers.ChatRequest{Messages: dasar, MaxTokens: &empat, Extra: map[string]json.RawMessage{
				"max_completion_tokens": json.RawMessage(`77`),
			}},
			4,
		},
		{
			"n mengalikan keluaran",
			&providers.ChatRequest{Messages: dasar, MaxTokens: &empat, Extra: map[string]json.RawMessage{
				"n": json.RawMessage(`3`),
			}},
			12,
		},
		{
			"n di luar batas dijepit",
			&providers.ChatRequest{Messages: dasar, MaxTokens: &empat, Extra: map[string]json.RawMessage{
				"n": json.RawMessage(`100000`),
			}},
			4 * maksSalinanOutput,
		},
		{
			"n tidak sah diabaikan",
			&providers.ChatRequest{Messages: dasar, MaxTokens: &empat, Extra: map[string]json.RawMessage{
				"n": json.RawMessage(`"dua"`),
			}},
			4,
		},
	} {
		t.Run(tc.nama, func(t *testing.T) {
			if got := EstimateTokens(tc.req).OutputTokens; got != tc.mau {
				t.Errorf("OutputTokens = %d, mau %d", got, tc.mau)
			}
		})
	}
}

// TestEstimateTokensGambarTidakDihitungSebagaiTeks menjaga jalur yang paling mudah salah:
// data URI base64 panjangnya megabyte, dan menghitungnya sebagai teks menghasilkan ratusan
// ribu token — cukup untuk menyaring habis setiap kandidat pada permintaan yang biasa saja.
func TestEstimateTokensGambarTidakDihitungSebagaiTeks(t *testing.T) {
	base64Panjang := "data:image/png;base64," + strings.Repeat("A", 40000)

	gambar := EstimateTokens(&providers.ChatRequest{
		Messages: []providers.Message{{Role: providers.RoleUser, Parts: []providers.ContentPart{
			{Type: providers.PartTypeImageURL, ImageURL: &providers.ImageURL{URL: base64Panjang}},
		}}},
	}).InputTokens

	sebagaiTeks := EstimateTokens(&providers.ChatRequest{
		Messages: []providers.Message{{Role: providers.RoleUser, Content: base64Panjang}},
	}).InputTokens

	if gambar > tokenGambarDetailTinggi+overheadPesan+overheadPermintaan+8 {
		t.Errorf("gambar dihitung %d token; panjang URL-nya ikut terhitung", gambar)
	}
	if sebagaiTeks <= gambar*10 {
		t.Errorf("teks sepanjang URL yang sama dihitung %d, gambar %d; pembandingnya tidak masuk akal",
			sebagaiTeks, gambar)
	}
}

func TestEstimateTokensDetailGambar(t *testing.T) {
	buat := func(detail string) int {
		return EstimateTokens(&providers.ChatRequest{
			Messages: []providers.Message{{Role: providers.RoleUser, Parts: []providers.ContentPart{
				{Type: providers.PartTypeImageURL, ImageURL: &providers.ImageURL{URL: "u", Detail: detail}},
			}}},
		}).InputTokens
	}
	rendah, tinggi, auto := buat("low"), buat("high"), buat("")
	if rendah >= tinggi {
		t.Errorf("detail low = %d, high = %d; mau low lebih kecil", rendah, tinggi)
	}
	// Detail kosong berarti auto, dan ukurannya tidak bisa dibaca dari body — yang dipakai
	// angka detail tinggi, karena keliru ke bawah pada permintaan multimodal berarti
	// dikirim ke jendela yang hampir pasti tidak cukup.
	if auto != tinggi {
		t.Errorf("detail kosong = %d, mau sama dengan high = %d", auto, tinggi)
	}
}

func TestEstimateTokensDefinisiToolIkutDihitung(t *testing.T) {
	dasar := []providers.Message{{Role: providers.RoleUser, Content: "a"}}
	tanpa := EstimateTokens(&providers.ChatRequest{Messages: dasar}).InputTokens
	dengan := EstimateTokens(&providers.ChatRequest{Messages: dasar, Tools: []providers.Tool{{
		Type: "function",
		Function: providers.FunctionDef{
			Name:        "cuaca",
			Description: strings.Repeat("penjelasan panjang ", 20),
			Parameters:  json.RawMessage(`{"type":"object","properties":{"kota":{"type":"string"}}}`),
		},
	}}}).InputTokens
	if dengan <= tanpa {
		t.Errorf("dengan tools = %d, tanpa = %d; skema tool ikut terkirim dan ikut ditagih", dengan, tanpa)
	}
}

func TestEstimateTokensNil(t *testing.T) {
	if got := EstimateTokens(nil); !got.IsZero() {
		t.Errorf("EstimateTokens(nil) = %#v, mau kosong", got)
	}
}

// --- Embeddings --------------------------------------------------------------

func TestDecodeEmbeddingsRequest(t *testing.T) {
	req, err := DecodeEmbeddingsRequest([]byte(`{
		"model": "text-embedding-3-small",
		"input": ["satu", "dua"],
		"dimensions": 256,
		"encoding_format": "base64",
		"user": "u-9",
		"field_masa_depan": true
	}`))
	if err != nil {
		t.Fatalf("DecodeEmbeddingsRequest: %v", err)
	}
	if req.Model != "text-embedding-3-small" {
		t.Errorf("Model = %q", req.Model)
	}
	if len(req.Input) != 2 || req.Input[0] != "satu" || req.Input[1] != "dua" {
		t.Errorf("Input = %#v", req.Input)
	}
	if req.Dimensions == nil || *req.Dimensions != 256 {
		t.Errorf("Dimensions = %v", req.Dimensions)
	}
	if req.EncodingFormat != "base64" || req.User != "u-9" {
		t.Errorf("EncodingFormat = %q, User = %q", req.EncodingFormat, req.User)
	}
	if _, ada := req.Extra["field_masa_depan"]; !ada {
		t.Errorf("field tak dikenal tidak masuk ke Extra; isi = %v", kunci(req.Extra))
	}
	for _, k := range []string{"model", "input", "dimensions", "encoding_format", "user"} {
		if _, ada := req.Extra[k]; ada {
			t.Errorf("field kanonik %q ikut masuk ke Extra", k)
		}
	}
}

func TestDecodeEmbeddingsRequestInputStringTunggal(t *testing.T) {
	req, err := DecodeEmbeddingsRequest([]byte(`{"model":"m","input":"satu saja"}`))
	if err != nil {
		t.Fatalf("DecodeEmbeddingsRequest: %v", err)
	}
	if len(req.Input) != 1 || req.Input[0] != "satu saja" {
		t.Errorf("Input = %#v", req.Input)
	}
}

func TestDecodeEmbeddingsRequestPenolakan(t *testing.T) {
	for _, tc := range []struct {
		nama     string
		body     string
		sebutkan string
	}{
		{"tanpa model", `{"input":"a"}`, `"model"`},
		{"tanpa input", `{"model":"m"}`, `"input"`},
		{"input null", `{"model":"m","input":null}`, `"input"`},
		{"input string kosong", `{"model":"m","input":""}`, `"input"`},
		{"input array kosong", `{"model":"m","input":[]}`, `"input"`},
		{"input memuat string kosong", `{"model":"m","input":["a",""]}`, `"input[1]"`},
		// Array token hanya bermakna terhadap tokenizer yang menghasilkannya. Meneruskannya
		// ke provider lain akan menghasilkan vektor untuk teks yang berbeda, tanpa error.
		{"input array token", `{"model":"m","input":[1,2,3]}`, `"input"`},
		{"input array token bersarang", `{"model":"m","input":[[1,2],[3]]}`, `"input"`},
		{"bukan objek", `[]`, "objek JSON"},
	} {
		t.Run(tc.nama, func(t *testing.T) {
			_, err := DecodeEmbeddingsRequest([]byte(tc.body))
			if err == nil {
				t.Fatal("body tidak sah diterima, mau ditolak")
			}
			if !strings.Contains(err.Error(), tc.sebutkan) {
				t.Errorf("pesan error tidak menyebut %s: %v", tc.sebutkan, err)
			}
		})
	}
}

// --- Round trip ke adapter ---------------------------------------------------

// loopbackSaja mengembalikan pengecualian SSRF untuk kedua bentuk alamat loopback.
//
// Keduanya diperlukan karena httptest.Server bisa mendengarkan di 127.0.0.1 maupun [::1]
// tergantung tumpukan jaringan mesin yang menjalankan test. Yang dipakai pengecualian PER
// ALAMAT, bukan AllowPrivate: AllowPrivate melepas seluruh penjagaan rentang, sehingga test
// tidak akan menyadari kalau kode yang diuji mulai menghubungi alamat internal LAIN.
func loopbackSaja(t *testing.T) []netip.Prefix {
	t.Helper()
	addrs, err := security.ParsePrivateAddrs([]string{"127.0.0.1", "::1"})
	if err != nil {
		t.Fatalf("ParsePrivateAddrs: %v", err)
	}
	return addrs
}

// penangkap adalah upstream tiruan yang menyimpan body permintaan terakhir.
type penangkap struct {
	mu   sync.Mutex
	body []byte
}

func (p *penangkap) simpan(b []byte) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.body = b
}

func (p *penangkap) terakhir() []byte {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.body
}

// TestRoundTripDecodeLaluAdapterOpenAI membuktikan kedua arah sepakat.
//
// Ini test yang paling banyak menangkap di berkas ini: ia menjalankan hasil decode melalui
// serializer SUNGGUHAN milik adapter openai, lalu membandingkan himpunan field yang keluar
// dengan yang masuk. Kalau satu field pindah ke tempat yang salah, akibatnya terlihat di
// sini sebagai field yang hilang atau berlebih — dua kegagalan yang tidak menghasilkan
// error apa pun di produksi.
func TestRoundTripDecodeLaluAdapterOpenAI(t *testing.T) {
	const asli = `{
		"model": "gpt-4o-mini",
		"messages": [
			{"role": "system", "content": "jadilah ringkas"},
			{"role": "user", "content": [
				{"type": "text", "text": "apa ini"},
				{"type": "image_url", "image_url": {"url": "https://contoh/x.png", "detail": "high"}}
			]},
			{"role": "assistant", "content": null, "tool_calls": [
				{"id": "call_1", "type": "function", "function": {"name": "cuaca", "arguments": "{}"}}
			]},
			{"role": "tool", "tool_call_id": "call_1", "content": "31"}
		],
		"max_tokens": 128,
		"temperature": 0.3,
		"top_p": 0.8,
		"stop": ["AKHIR"],
		"tools": [{"type": "function", "function": {"name": "cuaca", "parameters": {"type": "object"}}}],
		"tool_choice": "auto",
		"parallel_tool_calls": true,
		"response_format": {"type": "json_object"},
		"seed": 42,
		"user": "u-1",
		"frequency_penalty": 0.25,
		"field_masa_depan": {"apa pun": [1, 2, 3]}
	}`

	req, err := DecodeChatRequest([]byte(asli))
	if err != nil {
		t.Fatalf("DecodeChatRequest: %v", err)
	}

	tangkap := &penangkap{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		tangkap.simpan(b)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"c1","model":"gpt-4o-mini","created":1,
			"choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	t.Cleanup(srv.Close)

	prov, err := openai.New(openai.Config{
		Name:       "uji",
		Kind:       providers.KindOpenAI,
		BaseURL:    srv.URL,
		Credential: security.Secret("sk-uji"),
		Timeout:    5 * time.Second,
		SSRFPolicy: security.SSRFPolicy{AllowHTTP: true, AllowedPrivateAddrs: loopbackSaja(t)},
	})
	if err != nil {
		t.Fatalf("openai.New: %v", err)
	}
	if _, err := prov.ChatCompletion(context.Background(), req); err != nil {
		t.Fatalf("ChatCompletion: %v", err)
	}

	var masuk, keluar map[string]json.RawMessage
	if err := json.Unmarshal([]byte(asli), &masuk); err != nil {
		t.Fatalf("body asli bukan JSON: %v", err)
	}
	if err := json.Unmarshal(tangkap.terakhir(), &keluar); err != nil {
		t.Fatalf("body terkirim bukan JSON: %v", err)
	}

	// "stream" adalah satu-satunya field tambahan yang sah: adapter selalu menulisnya
	// eksplisit supaya Extra tidak bisa membalik mode transport.
	for k := range masuk {
		if _, ada := keluar[k]; !ada {
			t.Errorf("field %q hilang di perjalanan ke upstream", k)
		}
	}
	for k := range keluar {
		if _, ada := masuk[k]; !ada && k != "stream" {
			t.Errorf("field %q muncul di body upstream tanpa ada di permintaan klien", k)
		}
	}
	if string(keluar["stream"]) != "false" {
		t.Errorf(`field "stream" = %s, mau false pada jalur non-streaming`, keluar["stream"])
	}

	// Nilai yang dibawa apa adanya harus utuh, bukan hasil penyusunan ulang.
	for _, k := range []string{"model", "max_tokens", "temperature", "top_p", "stop", "seed", "user",
		"tool_choice", "parallel_tool_calls", "response_format", "frequency_penalty", "field_masa_depan"} {
		if !sepadan(t, masuk[k], keluar[k]) {
			t.Errorf("field %q berubah: masuk %s, keluar %s", k, masuk[k], keluar[k])
		}
	}
	if a, b := tanpaNull(t, masuk["messages"]), tanpaNull(t, keluar["messages"]); a != b {
		t.Errorf("messages berubah bentuk:\nmasuk  %s\nkeluar %s", a, b)
	}
}

// tanpaNull menormalkan array pesan dengan MEMBUANG field bernilai null.
//
// Satu perbedaan yang sah antara body klien dan body upstream: "content": null pada pesan
// asisten yang isinya hanya tool call tidak dikirim ulang sebagai null, melainkan tidak
// dikirim sama sekali — keduanya berarti hal yang sama bagi upstream, dan sebagian
// implementasi menolak bentuk yang satu. Alasannya ada di openai.messagesToWire.
//
// Yang dinormalkan hanya itu. Field yang hilang atau berubah nilainya tetap terlihat.
func tanpaNull(t *testing.T, raw json.RawMessage) string {
	t.Helper()
	var pesan []map[string]any
	if err := json.Unmarshal(raw, &pesan); err != nil {
		t.Fatalf("array pesan tidak bisa diurai: %v", err)
	}
	for _, m := range pesan {
		for k, v := range m {
			if v == nil {
				delete(m, k)
			}
		}
	}
	out, err := json.Marshal(pesan)
	if err != nil {
		t.Fatalf("array pesan tidak bisa diserialkan ulang: %v", err)
	}
	return string(out)
}

// sepadan membandingkan dua nilai JSON tanpa peduli spasi dan urutan kunci.
func sepadan(t *testing.T, a, b json.RawMessage) bool {
	t.Helper()
	normal := func(raw json.RawMessage) string {
		if len(raw) == 0 {
			return ""
		}
		var v any
		if err := json.Unmarshal(raw, &v); err != nil {
			return string(raw)
		}
		out, err := json.Marshal(v)
		if err != nil {
			return string(raw)
		}
		return string(out)
	}
	return normal(a) == normal(b)
}
