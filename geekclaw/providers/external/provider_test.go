package external

import (
	"context"
	"encoding/json"
	"testing"
)

func TestParseInitializeResult(t *testing.T) {
	raw := json.RawMessage(`{"name": "my-llm", "default_model": "my-model-v1", "supports_thinking": true}`)

	result, err := ParseInitializeResult(raw)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if result.Name != "my-llm" {
		t.Fatalf("expected 'my-llm', got %q", result.Name)
	}
	if result.DefaultModel != "my-model-v1" {
		t.Fatalf("expected 'my-model-v1', got %q", result.DefaultModel)
	}
	if !result.SupportsThinking {
		t.Fatal("expected supports_thinking=true")
	}
}

func TestParseInitializeResult_Empty(t *testing.T) {
	raw := json.RawMessage(`{}`)

	result, err := ParseInitializeResult(raw)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if result.Name != "" {
		t.Fatalf("expected empty name, got %q", result.Name)
	}
	if result.DefaultModel != "" {
		t.Fatalf("expected empty default_model, got %q", result.DefaultModel)
	}
	if result.SupportsThinking {
		t.Fatal("expected supports_thinking=false")
	}
}

func TestParseChatResult(t *testing.T) {
	raw := json.RawMessage(`{
		"content": "Hello, world!",
		"finish_reason": "stop",
		"usage": {"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15}
	}`)

	result, err := ParseChatResult(raw)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if result.Content != "Hello, world!" {
		t.Fatalf("unexpected content: %q", result.Content)
	}
	if result.FinishReason != "stop" {
		t.Fatalf("unexpected finish_reason: %q", result.FinishReason)
	}
	if result.Usage == nil {
		t.Fatal("expected usage to be set")
	}
	if result.Usage.TotalTokens != 15 {
		t.Fatalf("expected total_tokens=15, got %d", result.Usage.TotalTokens)
	}
}

func TestParseChatResult_WithToolCalls(t *testing.T) {
	raw := json.RawMessage(`{
		"content": "",
		"finish_reason": "tool_calls",
		"tool_calls": [
			{
				"id": "call_123",
				"type": "function",
				"function": {
					"name": "get_weather",
					"arguments": "{\"location\": \"Tokyo\"}"
				}
			}
		]
	}`)

	result, err := ParseChatResult(raw)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if result.FinishReason != "tool_calls" {
		t.Fatalf("unexpected finish_reason: %q", result.FinishReason)
	}
	if len(result.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(result.ToolCalls))
	}
	if result.ToolCalls[0].Function.Name != "get_weather" {
		t.Fatalf("unexpected function name: %q", result.ToolCalls[0].Function.Name)
	}
}

func TestChatResult_ToLLMResponse(t *testing.T) {
	result := &ChatResult{
		Content:      "Hello",
		FinishReason: "stop",
	}

	resp := result.ToLLMResponse()
	if resp.Content != "Hello" {
		t.Fatalf("unexpected content: %q", resp.Content)
	}
	if resp.FinishReason != "stop" {
		t.Fatalf("unexpected finish_reason: %q", resp.FinishReason)
	}
}

func TestChatResult_ToLLMResponse_WithToolCalls(t *testing.T) {
	raw := json.RawMessage(`{
		"content": "",
		"finish_reason": "tool_calls",
		"tool_calls": [
			{
				"id": "call_456",
				"type": "function",
				"function": {
					"name": "search",
					"arguments": "{\"query\": \"test\"}"
				}
			}
		]
	}`)

	result, err := ParseChatResult(raw)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	resp := result.ToLLMResponse()
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp.ToolCalls))
	}
	tc := resp.ToolCalls[0]
	if tc.Name != "search" {
		t.Fatalf("expected Name='search', got %q", tc.Name)
	}
	if tc.Arguments["query"] != "test" {
		t.Fatalf("expected Arguments[query]='test', got %v", tc.Arguments["query"])
	}
}

func TestExternalLLMProvider_NoCommand(t *testing.T) {
	p := NewExternalLLMProvider("test", PluginConfig{})
	err := p.Start(context.Background())
	if err == nil {
		t.Fatal("expected error for empty command")
	}
}

func TestExternalLLMProvider_Name_BeforeStart(t *testing.T) {
	p := NewExternalLLMProvider("myllm", PluginConfig{})
	if p.Name() != "plugin:myllm" {
		t.Fatalf("expected 'plugin:myllm', got %q", p.Name())
	}
}

func TestExternalLLMProvider_GetDefaultModel_BeforeStart(t *testing.T) {
	p := NewExternalLLMProvider("myllm", PluginConfig{})
	if p.GetDefaultModel() != "plugin:myllm" {
		t.Fatalf("expected 'plugin:myllm', got %q", p.GetDefaultModel())
	}
}

func TestExternalLLMProvider_SupportsThinking_Default(t *testing.T) {
	p := NewExternalLLMProvider("myllm", PluginConfig{})
	if p.SupportsThinking() {
		t.Fatal("expected supports_thinking=false by default")
	}
}
