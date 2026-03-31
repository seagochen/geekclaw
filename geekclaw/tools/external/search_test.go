package external

import (
	"context"
	"encoding/json"
	"testing"
)

func TestParseInitializeResult(t *testing.T) {
	raw := json.RawMessage(`{"name": "brave"}`)

	result, err := ParseInitializeResult(raw)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if result.Name != "brave" {
		t.Fatalf("expected 'brave', got %q", result.Name)
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
}

func TestParseSearchResult(t *testing.T) {
	raw := json.RawMessage(`{"results": "1. Test\n   https://example.com\n   A snippet"}`)

	result, err := ParseSearchResult(raw)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if result.Results == "" {
		t.Fatal("expected non-empty results")
	}
	if result.Results != "1. Test\n   https://example.com\n   A snippet" {
		t.Fatalf("unexpected results: %q", result.Results)
	}
}

func TestParseSearchResult_Empty(t *testing.T) {
	raw := json.RawMessage(`{"results": ""}`)

	result, err := ParseSearchResult(raw)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if result.Results != "" {
		t.Fatalf("expected empty results, got %q", result.Results)
	}
}

func TestExternalSearchProvider_NoCommand(t *testing.T) {
	p := NewExternalSearchProvider("test", PluginConfig{})
	err := p.Start(context.Background())
	if err == nil {
		t.Fatal("expected error for empty command")
	}
}

func TestExternalSearchProvider_Name_BeforeStart(t *testing.T) {
	p := NewExternalSearchProvider("mysearch", PluginConfig{})
	if p.Name() != "plugin:mysearch" {
		t.Fatalf("expected 'plugin:mysearch', got %q", p.Name())
	}
}
