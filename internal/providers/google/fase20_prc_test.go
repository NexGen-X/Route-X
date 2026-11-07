package google

// fase20_prc_test.go — pengunci Fase 20 PR-C (sisi Google).
//
//	B-460 error peristiwa-pertama streaming membawa Retry-After
//	      header pembuka aliran.
//	B-462 nama properti skema tidak lagi ditafsirkan sebagai kata
//	      kunci oleh normalizeSchema.
//	B-463 part fileData keluaran Gemini terbaca (gambar).
//	B-464 audio hasil tool ikut sebagai inlineData pendamping;
//	      dokumen hasil tool ditolak eksplisit.
//	B-466 groundingMetadata dibuang sebagai KEPUTUSAN TERCATAT;
//	      teks utuh.

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/NexGen-X/Route-X/internal/providers"
)

// TestB462NamaPropertiBukanKataKunci membuktikan properti yang
// NAMANYA sama dengan kata kunci skema selamat dari normalisasi:
// propertinya utuh, kata kunci DI DALAM skema propertinya tetap
// dinormalkan.
func TestB462NamaPropertiBukanKataKunci(t *testing.T) {
	schema := json.RawMessage(`{
		"type": "object",
		"properties": {
			"const": {"type": "string"},
			"not": {"type": "string"},
			"additionalProperties": {"type": "string"},
			"type": {"type": "string", "const": "z"},
			"biasa": {"type": "string", "const": "tetap"}
		}
	}`)
	out := string(sanitizeSchema(schema))

	var doc struct {
		Properties map[string]struct {
			Type string   `json:"type"`
			Enum []string `json:"enum"`
		} `json:"properties"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("skema hasil tidak terurai: %v (%s)", err, out)
	}
	for _, nama := range []string{"const", "not", "additionalProperties", "type", "biasa"} {
		if _, ok := doc.Properties[nama]; !ok {
			t.Errorf("properti %q hilang dari skema hasil: %s", nama, out)
		}
	}
	// Kata kunci const DI DALAM skema properti bernama "type" harus
	// tetap diterjemahkan menjadi enum — properti itu tidak lagi
	// lolos dari normalisasi hanya karena namanya.
	if e := doc.Properties["type"].Enum; len(e) != 1 || e[0] != "z" {
		t.Errorf("properti bernama type: enum = %v, mau [z] (const di dalamnya harus diterjemahkan): %s", e, out)
	}
	if e := doc.Properties["biasa"].Enum; len(e) != 1 || e[0] != "tetap" {
		t.Errorf("properti biasa: enum = %v, mau [tetap]", e)
	}
	// Level wadah tidak boleh kedatangan enum karangan dari nama
	// properti "const".
	var mentah map[string]any
	_ = json.Unmarshal([]byte(out), &mentah)
	if _, ada := mentah["properties"].(map[string]any)["enum"]; ada {
		t.Errorf("enum karangan muncul di level properties: %s", out)
	}
}

// TestB463FileDataKeluaranTerbaca membuktikan respons Gemini yang
// hanya memuat fileData gambar adalah jawaban sah (bukan kegagalan
// server), dan teks + fileData keduanya selamat.
func TestB463FileDataKeluaranTerbaca(t *testing.T) {
	body := `{"candidates":[{"index":0,"finishReason":"STOP","content":{"role":"model","parts":[
		{"fileData":{"mimeType":"image/png","fileUri":"https://berkas.contoh.test/hasil-b463.png"}}]}}],
		"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":5,"totalTokenCount":15}}`
	p, _ := newServer(t, jsonHandler(http.StatusOK, body))

	resp, err := p.ChatCompletion(context.Background(), simpleRequest())
	if err != nil {
		t.Fatalf("ChatCompletion fileData-saja: %v — jawaban sah dilaporkan gagal", err)
	}
	if len(resp.Choices) == 0 {
		t.Fatal("tidak ada choice pada respons fileData")
	}
	var ketemu bool
	for _, part := range resp.Choices[0].Message.Parts {
		if part.Type == providers.PartTypeImageURL && part.ImageURL != nil &&
			part.ImageURL.URL == "https://berkas.contoh.test/hasil-b463.png" {
			ketemu = true
		}
	}
	if !ketemu {
		t.Errorf("part gambar fileData tidak ada di bentuk kanonik: %+v", resp.Choices[0].Message.Parts)
	}

	body2 := `{"candidates":[{"index":0,"finishReason":"STOP","content":{"role":"model","parts":[
		{"text":"ini hasilnya"},
		{"fileData":{"mimeType":"image/png","fileUri":"https://berkas.contoh.test/kedua.png"}}]}}],
		"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":5,"totalTokenCount":15}}`
	p2, _ := newServer(t, jsonHandler(http.StatusOK, body2))
	resp2, err := p2.ChatCompletion(context.Background(), simpleRequest())
	if err != nil {
		t.Fatalf("ChatCompletion teks+fileData: %v", err)
	}
	if !strings.Contains(string(resp2.Raw), "kedua.png") || !strings.Contains(string(resp2.Raw), "ini hasilnya") {
		t.Errorf("teks atau fileData hilang dari Raw: %s", resp2.Raw)
	}
}

// TestB464AudioDokumenHasilTool membuktikan dua kaki B-464 sisi
// Google: audio hasil tool menjadi part inlineData pendamping;
// dokumen hasil tool ditolak eksplisit.
func TestB464AudioDokumenHasilTool(t *testing.T) {
	p, log := newServer(t, jsonHandler(http.StatusOK, textResponse))
	req := &providers.ChatRequest{
		Model: "gemini-2.5-flash",
		Messages: []providers.Message{
			{Role: providers.RoleUser, Content: "pakai tool"},
			{Role: providers.RoleAssistant, ToolCalls: []providers.ToolCall{{
				ID: "call_b464", Type: "function",
				Function: providers.FunctionCall{Name: "dengar", Arguments: "{}"},
			}}},
			{Role: providers.RoleTool, ToolCallID: "call_b464", Parts: []providers.ContentPart{
				{Type: providers.PartTypeText, Text: "rekamannya:"},
				{Type: providers.PartTypeAudio, Audio: &providers.Audio{Data: "UklGRg==", Format: "wav"}},
			}},
		},
	}
	if _, err := p.ChatCompletion(context.Background(), req); err != nil {
		t.Fatalf("ChatCompletion audio hasil tool: %v", err)
	}
	body := string(log.raw(t, 0))
	if !strings.Contains(body, `"inlineData"`) || !strings.Contains(body, "UklGRg==") ||
		!strings.Contains(body, "audio/wav") {
		t.Errorf("part audio pendamping tidak ada di body upstream: %s", body)
	}

	req.Messages[2].Parts = []providers.ContentPart{
		{Type: providers.PartTypeText, Text: "berkasnya:"},
		{Type: providers.PartTypeDocument, Document: &providers.Document{
			MediaType: "application/pdf", Data: "JVBERi0xLjQ=",
		}},
	}
	_, err := p.ChatCompletion(context.Background(), req)
	pe := providers.AsError(err)
	if pe == nil || pe.Kind != providers.ErrKindInvalidRequest {
		t.Fatalf("dokumen hasil tool: err = %v, mau invalid_request eksplisit", err)
	}
}

// TestB460GoogleRetryAfterPembukaAliran membuktikan error yang tiba
// sebagai peristiwa pertama aliran membawa jeda dari header respons
// pembuka (bukan 0, bukan hanya dari RetryInfo body).
func TestB460GoogleRetryAfterPembukaAliran(t *testing.T) {
	p, _ := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Retry-After", "17")
		_, _ = w.Write([]byte(`data: {"error":{"code":429,"message":"kuota habis","status":"RESOURCE_EXHAUSTED"}}` + "\n\n"))
	})
	st, err := p.ChatCompletionStream(context.Background(), simpleRequest())
	if err != nil {
		t.Fatalf("ChatCompletionStream: %v", err)
	}
	defer st.Close()
	_, err = st.Recv()
	pe := providers.AsError(err)
	if pe == nil {
		t.Fatalf("Recv: err = %v, mau *providers.Error", err)
	}
	if pe.Kind != providers.ErrKindRateLimit {
		t.Errorf("Kind = %q, mau rate_limit", pe.Kind)
	}
	if pe.RetryAfter != 17*time.Second {
		t.Errorf("RetryAfter = %v, mau 17s dari header pembuka aliran", pe.RetryAfter)
	}
}

// TestB466GroundingGeminiDibuangTercatat mengunci keputusan B-466:
// respons ber-groundingMetadata diterjemahkan dengan teks UTUH dan
// tanpa atribusi yang dikarang — grounding gugur sesuai keputusan
// tercatat di translate.go.
func TestB466GroundingGeminiDibuangTercatat(t *testing.T) {
	body := `{"candidates":[{"index":0,"finishReason":"STOP",
		"groundingMetadata":{"webSearchQueries":["ibu kota indonesia"],
			"groundingChunks":[{"web":{"uri":"https://contoh.test/sumber-b466","title":"Sumber B466"}}]},
		"content":{"role":"model","parts":[{"text":"Jakarta adalah ibu kota Indonesia."}]}}],
		"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":5,"totalTokenCount":15}}`
	p, _ := newServer(t, jsonHandler(http.StatusOK, body))

	resp, err := p.ChatCompletion(context.Background(), simpleRequest())
	if err != nil {
		t.Fatalf("ChatCompletion ber-grounding: %v", err)
	}
	if len(resp.Choices) == 0 || resp.Choices[0].Message.Content != "Jakarta adalah ibu kota Indonesia." {
		t.Fatalf("teks jawaban tidak utuh: %+v", resp.Choices)
	}
	if strings.Contains(string(resp.Raw), "sumber-b466") {
		t.Errorf("grounding muncul di Raw — keputusan B-466 berubah tanpa catatan: %s", resp.Raw)
	}
}
