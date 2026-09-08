package responsecache

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/NexGen-X/Route-X/internal/providers"
)

func TestComputeKey(t *testing.T) {
	e := NewEngine(nil, nil)

	temp := 0.7
	maxTokens := 100

	req1 := &providers.ChatRequest{
		Model:       "gpt-4o",
		Temperature: &temp,
		MaxTokens:   &maxTokens,
		Messages: []providers.Message{
			{Role: "user", Content: "Halo dunia"},
		},
	}

	req2 := &providers.ChatRequest{
		Model:       "gpt-4o",
		Temperature: &temp,
		MaxTokens:   &maxTokens,
		Messages: []providers.Message{
			{Role: "user", Content: "Halo dunia"},
		},
	}

	req3 := &providers.ChatRequest{
		Model:       "gpt-4o",
		Temperature: &temp,
		MaxTokens:   &maxTokens,
		Messages: []providers.Message{
			{Role: "user", Content: "Halo dunia yang lain"},
		},
	}

	key1 := e.ComputeKey(req1)
	key2 := e.ComputeKey(req2)
	key3 := e.ComputeKey(req3)

	if key1 == "" {
		t.Fatal("key1 tidak boleh kosong")
	}
	if key1 != key2 {
		t.Fatalf("key1 (%s) dan key2 (%s) untuk prompt identik harus sama", key1, key2)
	}
	if key1 == key3 {
		t.Fatalf("key1 (%s) dan key3 (%s) untuk prompt berbeda tidak boleh sama", key1, key3)
	}
}

// TestComputeKeyFieldCoverage membuktikan setiap field pembeda request ikut
// membentuk kunci cache (BE-004): request yang hanya berbeda satu field tidak
// boleh berbagi kunci.
func TestComputeKeyFieldCoverage(t *testing.T) {
	e := NewEngine(nil, nil)

	one := 1
	tempA := 0.5
	tempB := 0.9
	tempPrecise := 0.50001
	topP := 0.8
	seed := 42
	parallel := true

	base := func() *providers.ChatRequest {
		return &providers.ChatRequest{
			Model: "gpt-4o",
			Messages: []providers.Message{
				{Role: "user", Content: "halo"},
			},
		}
	}
	baseKey := e.ComputeKey(base())

	cases := []struct {
		name   string
		mutate func(r *providers.ChatRequest)
	}{
		{"MaxTokens", func(r *providers.ChatRequest) { r.MaxTokens = &one }},
		{"TemperatureA", func(r *providers.ChatRequest) { r.Temperature = &tempA }},
		{"TemperatureB", func(r *providers.ChatRequest) { r.Temperature = &tempB }},
		{"TemperaturePrecision", func(r *providers.ChatRequest) { r.Temperature = &tempPrecise }},
		{"TopP", func(r *providers.ChatRequest) { r.TopP = &topP }},
		{"Stop", func(r *providers.ChatRequest) { r.Stop = []string{"###"} }},
		{"Seed", func(r *providers.ChatRequest) { r.Seed = &seed }},
		{"Stream", func(r *providers.ChatRequest) { r.Stream = true }},
		{"ParallelToolCalls", func(r *providers.ChatRequest) { r.ParallelToolCalls = &parallel }},
		{"ToolChoice", func(r *providers.ChatRequest) { r.ToolChoice = json.RawMessage(`"auto"`) }},
		{"ResponseFormat", func(r *providers.ChatRequest) { r.ResponseFormat = json.RawMessage(`{"type":"json_object"}`) }},
		{"ReasoningEffort", func(r *providers.ChatRequest) { r.ReasoningEffort = "high" }},
		{"User", func(r *providers.ChatRequest) { r.User = "u-123" }},
		{"Extra", func(r *providers.ChatRequest) {
			r.Extra = map[string]json.RawMessage{"logprobs": json.RawMessage(`true`)}
		}},
		{"MessageName", func(r *providers.ChatRequest) { r.Messages[0].Name = "andi" }},
		{"MessageToolCalls", func(r *providers.ChatRequest) {
			r.Messages[0].ToolCalls = []providers.ToolCall{{ID: "c1", Function: providers.FunctionCall{Name: "f", Arguments: "{}"}}}
		}},
		{"ReasoningContent", func(r *providers.ChatRequest) { r.Messages[0].ReasoningContent = "jejak" }},
		{"MultimodalPart", func(r *providers.ChatRequest) {
			r.Messages[0].Parts = []providers.ContentPart{{Type: providers.PartTypeText, Text: "halo"}}
		}},
		{"ImagePart", func(r *providers.ChatRequest) {
			r.Messages[0].Parts = []providers.ContentPart{
				{Type: providers.PartTypeImageURL, ImageURL: &providers.ImageURL{URL: "https://x/y.png", Detail: "high"}},
			}
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := base()
			tc.mutate(r)
			got := e.ComputeKey(r)
			if got == baseKey {
				t.Errorf("field %s tidak memengaruhi kunci: request berbeda berbagi kunci %s", tc.name, got)
			}
		})
	}

	// Determinisme urutan Extra: dua map dengan isi sama harus menghasilkan
	// kunci sama meskipun urutan penyisipan berbeda.
	r1 := base()
	r1.Extra = map[string]json.RawMessage{"a": json.RawMessage(`1`), "b": json.RawMessage(`2`)}
	r2 := base()
	r2.Extra = map[string]json.RawMessage{"b": json.RawMessage(`2`), "a": json.RawMessage(`1`)}
	if e.ComputeKey(r1) != e.ComputeKey(r2) {
		t.Error("kunci tidak deterministik terhadap urutan kunci Extra")
	}

	// Batas elemen Stop harus ikut dikanonisasi; konkatenasi polos membuat dua
	// daftar berikut identik sebagai "abc".
	r1 = base()
	r1.Stop = []string{"ab", "c"}
	r2 = base()
	r2.Stop = []string{"a", "bc"}
	if e.ComputeKey(r1) == e.ComputeKey(r2) {
		t.Error("batas elemen Stop tidak memengaruhi kunci")
	}
}

func TestEngineSettings(t *testing.T) {
	e := NewEngine(nil, nil)

	if !e.enabled {
		t.Fatal("default engine enabled harus true")
	}
	if e.TTL() != DefaultTTL {
		t.Fatalf("default TTL harus %v, dapat %v", DefaultTTL, e.TTL())
	}

	e.SetSettings(false, 30*time.Minute)
	if e.enabled {
		t.Fatal("engine enabled harus false setelah disetel")
	}
	if e.TTL() != 30*time.Minute {
		t.Fatalf("TTL harus 30m, dapat %v", e.TTL())
	}

	// Nilai Get ketika redis nil harus mengembalikan false tanpa error
	_, hit, err := e.Get(context.Background(), "test-key")
	if err != nil {
		t.Fatalf("Get tanpa redis tidak boleh error: %v", err)
	}
	if hit {
		t.Fatal("Get tanpa redis tidak boleh hit")
	}
}
