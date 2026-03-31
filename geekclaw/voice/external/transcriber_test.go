package external

import (
	"context"
	"encoding/json"
	"testing"
)

func TestParseInitializeResult(t *testing.T) {
	raw := json.RawMessage(`{
		"name": "whisper",
		"audio_formats": ["ogg", "wav", "mp3"]
	}`)

	result, err := ParseInitializeResult(raw)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if result.Name != "whisper" {
		t.Fatalf("expected 'whisper', got %q", result.Name)
	}
	if len(result.AudioFormats) != 3 {
		t.Fatalf("expected 3 audio formats, got %d", len(result.AudioFormats))
	}
	if result.AudioFormats[0] != "ogg" {
		t.Fatalf("expected 'ogg', got %q", result.AudioFormats[0])
	}
}

func TestParseInitializeResult_Minimal(t *testing.T) {
	raw := json.RawMessage(`{"name": "deepgram"}`)

	result, err := ParseInitializeResult(raw)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if result.Name != "deepgram" {
		t.Fatalf("expected 'deepgram', got %q", result.Name)
	}
	if len(result.AudioFormats) != 0 {
		t.Fatalf("expected 0 audio formats, got %d", len(result.AudioFormats))
	}
}

func TestParseTranscribeResult(t *testing.T) {
	raw := json.RawMessage(`{"text": "hello world", "language": "en", "duration": 3.14}`)

	result, err := ParseTranscribeResult(raw)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if result.Text != "hello world" {
		t.Fatalf("expected 'hello world', got %q", result.Text)
	}
	if result.Language != "en" {
		t.Fatalf("expected 'en', got %q", result.Language)
	}
	if result.Duration != 3.14 {
		t.Fatalf("expected 3.14, got %f", result.Duration)
	}
}

func TestParseTranscribeResult_TextOnly(t *testing.T) {
	raw := json.RawMessage(`{"text": "just text"}`)

	result, err := ParseTranscribeResult(raw)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if result.Text != "just text" {
		t.Fatalf("expected 'just text', got %q", result.Text)
	}
	if result.Language != "" {
		t.Fatalf("expected empty language, got %q", result.Language)
	}
}

func TestExternalTranscriber_NoCommand(t *testing.T) {
	tr := NewExternalTranscriber("test", PluginConfig{})
	err := tr.Start(context.Background())
	if err == nil {
		t.Fatal("expected error for empty command")
	}
}

func TestExternalTranscriber_Name_BeforeStart(t *testing.T) {
	tr := NewExternalTranscriber("myvoice", PluginConfig{})
	if tr.Name() != "plugin:myvoice" {
		t.Fatalf("expected 'plugin:myvoice', got %q", tr.Name())
	}
}
