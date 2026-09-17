package gateway

import (
	"testing"

	"github.com/NexGen-X/Route-X/internal/providers"
)

func TestDecodeAnthropicMessagesRequest(t *testing.T) {
	t.Run("basic text message with system", func(t *testing.T) {
		body := []byte(`{
			"model": "claude-3-7-sonnet",
			"system": "You are a helpful assistant.",
			"max_tokens": 1024,
			"messages": [
				{"role": "user", "content": "Hello world"}
			]
		}`)

		req, err := DecodeAnthropicMessagesRequest(body)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if req.Model != "claude-3-7-sonnet" {
			t.Errorf("expected model claude-3-7-sonnet, got %s", req.Model)
		}
		if req.MaxTokens == nil || *req.MaxTokens != 1024 {
			t.Errorf("expected max_tokens 1024, got %v", req.MaxTokens)
		}
		if len(req.Messages) != 2 {
			t.Fatalf("expected 2 messages (system + user), got %d", len(req.Messages))
		}
		if req.Messages[0].Role != providers.RoleSystem || req.Messages[0].Content != "You are a helpful assistant." {
			t.Errorf("unexpected system message: %+v", req.Messages[0])
		}
		if req.Messages[1].Role != providers.RoleUser || req.Messages[1].Content != "Hello world" {
			t.Errorf("unexpected user message: %+v", req.Messages[1])
		}
	})

	t.Run("content blocks with text and tool use and tool result", func(t *testing.T) {
		body := []byte(`{
			"model": "claude-3-5-sonnet",
			"messages": [
				{
					"role": "user",
					"content": [
						{"type": "text", "text": "What is the weather?"}
					]
				},
				{
					"role": "assistant",
					"content": [
						{
							"type": "tool_use",
							"id": "toolu_123",
							"name": "get_weather",
							"input": {"location": "Jakarta"}
						}
					]
				},
				{
					"role": "user",
					"content": [
						{
							"type": "tool_result",
							"tool_use_id": "toolu_123",
							"content": "Sunny 30C"
						}
					]
				}
			],
			"tools": [
				{
					"name": "get_weather",
					"description": "Get weather info",
					"input_schema": {
						"type": "object",
						"properties": {
							"location": {"type": "string"}
						}
					}
				}
			]
		}`)

		req, err := DecodeAnthropicMessagesRequest(body)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(req.Messages) != 3 {
			t.Fatalf("expected 3 messages, got %d", len(req.Messages))
		}
		if req.Messages[0].Content != "What is the weather?" {
			t.Errorf("unexpected message[0]: %s", req.Messages[0].Content)
		}
		if len(req.Messages[1].ToolCalls) != 1 {
			t.Fatalf("expected 1 tool call, got %d", len(req.Messages[1].ToolCalls))
		}
		tc := req.Messages[1].ToolCalls[0]
		if tc.ID != "toolu_123" || tc.Function.Name != "get_weather" {
			t.Errorf("unexpected tool call: %+v", tc)
		}
		if req.Messages[2].Role != providers.RoleTool || req.Messages[2].ToolCallID != "toolu_123" || req.Messages[2].Content != "Sunny 30C" {
			t.Errorf("unexpected tool result message: %+v", req.Messages[2])
		}
		if len(req.Tools) != 1 {
			t.Fatalf("expected 1 tool definition, got %d", len(req.Tools))
		}
		if req.Tools[0].Function.Name != "get_weather" {
			t.Errorf("unexpected tool name: %s", req.Tools[0].Function.Name)
		}
	})

	t.Run("missing model", func(t *testing.T) {
		body := []byte(`{"messages": [{"role": "user", "content": "hi"}]}`)
		_, err := DecodeAnthropicMessagesRequest(body)
		if err == nil {
			t.Fatalf("expected error when model is missing")
		}
	})
}
