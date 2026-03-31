package external

import (
	"context"
	"encoding/json"
	"testing"
)

func TestParseToolInitializeResult(t *testing.T) {
	raw := json.RawMessage(`{
		"tools": [
			{"name": "i2c", "description": "I2C bus tool", "parameters": {"type": "object"}},
			{"name": "spi", "description": "SPI bus tool"}
		]
	}`)

	result, err := ParseToolInitializeResult(raw)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(result.Tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(result.Tools))
	}
	if result.Tools[0].Name != "i2c" {
		t.Errorf("expected 'i2c', got %q", result.Tools[0].Name)
	}
	if result.Tools[1].Name != "spi" {
		t.Errorf("expected 'spi', got %q", result.Tools[1].Name)
	}
}

func TestParseToolInitializeResult_Empty(t *testing.T) {
	raw := json.RawMessage(`{"tools": []}`)

	result, err := ParseToolInitializeResult(raw)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(result.Tools) != 0 {
		t.Fatalf("expected 0 tools, got %d", len(result.Tools))
	}
}

func TestParseToolInitializeResult_MissingTools(t *testing.T) {
	raw := json.RawMessage(`{}`)

	result, err := ParseToolInitializeResult(raw)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if result.Tools != nil {
		t.Fatalf("expected nil tools, got %v", result.Tools)
	}
}

func TestParseToolExecuteResult_Success(t *testing.T) {
	raw := json.RawMessage(`{"content": "read 2 bytes: [0x1a, 0x2b]"}`)

	result, err := ParseToolExecuteResult(raw)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if result.Content != "read 2 bytes: [0x1a, 0x2b]" {
		t.Errorf("unexpected content: %q", result.Content)
	}
	if result.Error {
		t.Error("expected error=false")
	}
}

func TestParseToolExecuteResult_Error(t *testing.T) {
	raw := json.RawMessage(`{"content": "device not found", "error": true}`)

	result, err := ParseToolExecuteResult(raw)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if !result.Error {
		t.Error("expected error=true")
	}
	if result.Content != "device not found" {
		t.Errorf("unexpected content: %q", result.Content)
	}
}

func TestExternalToolPlugin_NoCommand(t *testing.T) {
	p := NewExternalToolPlugin("test", PluginConfig{})
	err := p.Start(context.Background())
	if err == nil {
		t.Fatal("expected error for empty command")
	}
}

func TestExternalToolPlugin_Tools_BeforeStart(t *testing.T) {
	p := NewExternalToolPlugin("hw", PluginConfig{})
	if tools := p.Tools(); tools != nil {
		t.Fatalf("expected nil tools before start, got %v", tools)
	}
}
