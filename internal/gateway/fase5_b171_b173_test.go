package gateway

import (
	"encoding/json"
	"testing"

	"github.com/NexGen-X/Route-X/internal/providers"
)

// Test pengunci B-171 dan B-173 (Fase 5 kampanye tuntas-logika).
//
// B-171: permukaan OpenAI & Responses tidak memvalidasi rentang
// parameter sama sekali — max_tokens negatif / temperature 99 lolos
// ke upstream untuk ditolak di sana sebagai kegagalan provider.
// Permukaan Anthropic sudah menolak max_tokens <= 0 secara lokal;
// kedua permukaan ini menyamakan.
// B-173: /v1/responses melaporkan status "completed" untuk jawaban
// yang dihentikan filter konten; satu-satunya pemetaan yang ada
// hanya finish_reason "length" -> incomplete.

// --- B-171 -------------------------------------------------------------------

func TestB171CodecMenolakRentangTakSah(t *testing.T) {
	kasus := []struct {
		nama string
		body string
	}{
		{"max_tokens negatif", `{"model":"m","messages":[{"role":"user","content":"x"}],"max_tokens":-5}`},
		{"max_tokens nol", `{"model":"m","messages":[{"role":"user","content":"x"}],"max_tokens":0}`},
		{"temperature di atas 2", `{"model":"m","messages":[{"role":"user","content":"x"}],"temperature":99}`},
		{"temperature negatif", `{"model":"m","messages":[{"role":"user","content":"x"}],"temperature":-0.5}`},
		{"top_p di atas 1", `{"model":"m","messages":[{"role":"user","content":"x"}],"top_p":1.5}`},
		{"top_p negatif", `{"model":"m","messages":[{"role":"user","content":"x"}],"top_p":-0.1}`},
	}
	for _, tc := range kasus {
		t.Run(tc.nama, func(t *testing.T) {
			if _, err := DecodeChatRequest([]byte(tc.body)); err == nil {
				t.Errorf("B-171: body %s tidak ditolak — nilai di luar rentang wajib ditolak lokal, bukan diteruskan ke upstream", tc.body)
			}
		})
	}
	// Kontrol: nilai di tepi rentang yang sah tetap diterima.
	sah := `{"model":"m","messages":[{"role":"user","content":"x"}],"max_tokens":1,"temperature":2,"top_p":1}`
	if _, err := DecodeChatRequest([]byte(sah)); err != nil {
		t.Errorf("kontrol: body sah ditolak: %v", err)
	}
}

func TestB171ResponsesMenolakRentangTakSah(t *testing.T) {
	kasus := []struct {
		nama string
		body string
	}{
		{"max_output_tokens negatif", `{"model":"m","input":"x","max_output_tokens":-1}`},
		{"max_output_tokens nol", `{"model":"m","input":"x","max_output_tokens":0}`},
		{"temperature di atas 2", `{"model":"m","input":"x","temperature":2.5}`},
		{"top_p di atas 1", `{"model":"m","input":"x","top_p":7}`},
	}
	for _, tc := range kasus {
		t.Run(tc.nama, func(t *testing.T) {
			if _, err := uraiPermintaanResponses([]byte(tc.body)); err == nil {
				t.Errorf("B-171: body %s tidak ditolak di permukaan Responses", tc.body)
			}
		})
	}
	sah := `{"model":"m","input":"x","max_output_tokens":16,"temperature":0,"top_p":0.5}`
	if _, err := uraiPermintaanResponses([]byte(sah)); err != nil {
		t.Errorf("kontrol: body sah ditolak: %v", err)
	}
}

// --- B-173 -------------------------------------------------------------------

func TestB173ResponsesContentFilterBukanCompleted(t *testing.T) {
	mentah := kodekanResponsResponses(&providers.ChatResponse{
		ID:    "chatcmpl-uji",
		Model: "model-uji",
		Choices: []providers.Choice{{
			FinishReason: providers.FinishContentFilter,
			Message:      providers.Message{Role: providers.RoleAssistant},
		}},
	}, "model-uji", false)
	var obj struct {
		Status            string         `json:"status"`
		IncompleteDetails map[string]any `json:"incomplete_details"`
	}
	if err := json.Unmarshal(mentah, &obj); err != nil {
		t.Fatalf("respons bukan JSON: %v (%s)", err, mentah)
	}
	if obj.Status != "incomplete" {
		t.Errorf("B-173: status = %q, mau \"incomplete\" — jawaban yang dihentikan filter konten bukan jawaban tuntas", obj.Status)
	}
	if obj.IncompleteDetails["reason"] != "content_filter" {
		t.Errorf("B-173: incomplete_details.reason = %v, mau content_filter", obj.IncompleteDetails["reason"])
	}

	// Kontrol: pemetaan lama tidak berubah — length tetap
	// incomplete/max_output_tokens, stop tetap completed.
	mentah = kodekanResponsResponses(&providers.ChatResponse{
		ID:    "chatcmpl-uji",
		Model: "model-uji",
		Choices: []providers.Choice{{
			FinishReason: providers.FinishLength,
			Message:      providers.Message{Role: providers.RoleAssistant, Content: "potong"},
		}},
	}, "model-uji", false)
	obj.Status, obj.IncompleteDetails = "", nil
	if err := json.Unmarshal(mentah, &obj); err != nil {
		t.Fatal(err)
	}
	if obj.Status != "incomplete" || obj.IncompleteDetails["reason"] != "max_output_tokens" {
		t.Errorf("kontrol length: status/reason = %q/%v, mau incomplete/max_output_tokens", obj.Status, obj.IncompleteDetails["reason"])
	}

	mentah = kodekanResponsResponses(&providers.ChatResponse{
		ID:    "chatcmpl-uji",
		Model: "model-uji",
		Choices: []providers.Choice{{
			FinishReason: providers.FinishStop,
			Message:      providers.Message{Role: providers.RoleAssistant, Content: "tuntas"},
		}},
	}, "model-uji", false)
	obj.Status = ""
	if err := json.Unmarshal(mentah, &obj); err != nil {
		t.Fatal(err)
	}
	if obj.Status != "completed" {
		t.Errorf("kontrol stop: status = %q, mau completed", obj.Status)
	}
}
