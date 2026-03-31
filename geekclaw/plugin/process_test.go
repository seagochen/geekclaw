package plugin

import (
	"context"
	"testing"
)

func TestNewProcess(t *testing.T) {
	p := NewProcess("test", Config{Command: "echo"})
	if p.Name() != "test" {
		t.Fatalf("expected 'test', got %q", p.Name())
	}
	if p.PluginConfig().Command != "echo" {
		t.Fatalf("expected 'echo', got %q", p.PluginConfig().Command)
	}
	if p.Transport() != nil {
		t.Fatal("expected nil transport before spawn")
	}
}

func TestProcess_SpawnNoCommand(t *testing.T) {
	p := NewProcess("test", Config{})
	_, err := p.Spawn(context.Background(), SpawnOpts{
		LogCategory: "test",
		InitMethod:  "test.initialize",
		StopMethod:  "test.stop",
		LogMethod:   "test.log",
	})
	if err == nil {
		t.Fatal("expected error for empty command")
	}
}

func TestLogWriter(t *testing.T) {
	w := &LogWriter{Name: "test", Category: "test"}
	n, err := w.Write([]byte("hello"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 5 {
		t.Fatalf("expected 5, got %d", n)
	}
}
