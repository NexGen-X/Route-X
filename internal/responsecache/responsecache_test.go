package responsecache

import (
	"context"
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
