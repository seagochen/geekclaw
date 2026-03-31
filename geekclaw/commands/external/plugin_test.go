package external

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/seagosoft/geekclaw/geekclaw/commands"
)

func TestParseInitializeResult(t *testing.T) {
	raw := json.RawMessage(`{
		"commands": [
			{"name": "hello", "description": "Say hello", "usage": "/hello [name]", "aliases": ["hi"]},
			{"name": "ping", "description": "Pong"}
		]
	}`)

	result, err := ParseInitializeResult(raw)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(result.Commands) != 2 {
		t.Fatalf("expected 2 commands, got %d", len(result.Commands))
	}
	if result.Commands[0].Name != "hello" {
		t.Fatalf("expected 'hello', got %q", result.Commands[0].Name)
	}
	if len(result.Commands[0].Aliases) != 1 || result.Commands[0].Aliases[0] != "hi" {
		t.Fatalf("expected alias 'hi', got %v", result.Commands[0].Aliases)
	}
}

func TestParseExecuteResult(t *testing.T) {
	raw := json.RawMessage(`{"reply": "Hello, world!"}`)

	result, err := ParseExecuteResult(raw)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if result.Reply != "Hello, world!" {
		t.Fatalf("expected 'Hello, world!', got %q", result.Reply)
	}
}

func TestExternalCommandPlugin_NoCommand(t *testing.T) {
	plugin := NewExternalCommandPlugin("test", PluginConfig{})
	err := plugin.Start(context.Background())
	if err == nil {
		t.Fatal("expected error for empty command")
	}
}

func TestExternalCommandPlugin_Source(t *testing.T) {
	plugin := NewExternalCommandPlugin("myplugin", PluginConfig{})
	if plugin.Source() != "plugin:myplugin" {
		t.Fatalf("expected 'plugin:myplugin', got %q", plugin.Source())
	}
}

func TestExternalCommandPlugin_CommandsBeforeStart(t *testing.T) {
	plugin := NewExternalCommandPlugin("test", PluginConfig{})
	defs := plugin.Commands()
	if len(defs) != 0 {
		t.Fatalf("expected 0 commands before start, got %d", len(defs))
	}
}

// TestMergeDefinitionsConflictDetection tests the conflict detection in Registry.
func TestMergeDefinitionsConflictDetection(t *testing.T) {
	reg := commands.NewRegistry([]commands.Definition{
		{Name: "help", Description: "Built-in help"},
		{Name: "list", Aliases: []string{"ls"}},
	})

	extra := []commands.Definition{
		{Name: "hello", Description: "From plugin", Source: "plugin:test"},       // OK
		{Name: "help", Description: "Duplicate help", Source: "plugin:test"},      // conflict: name
		{Name: "greet", Aliases: []string{"ls"}, Source: "plugin:test"},           // conflict: alias
		{Name: "ping", Description: "From plugin", Source: "plugin:test"},         // OK
	}

	conflicts := reg.MergeDefinitions(extra)
	if len(conflicts) != 2 {
		t.Fatalf("expected 2 conflicts, got %d: %v", len(conflicts), conflicts)
	}

	// hello and ping should be registered
	if _, ok := reg.Lookup("hello"); !ok {
		t.Fatal("expected 'hello' to be registered")
	}
	if _, ok := reg.Lookup("ping"); !ok {
		t.Fatal("expected 'ping' to be registered")
	}

	// help conflict — original stays
	def, ok := reg.Lookup("help")
	if !ok {
		t.Fatal("expected 'help' to still be registered")
	}
	if def.Source != "" {
		t.Fatalf("expected builtin help (empty source), got %q", def.Source)
	}

	// greet should NOT be registered (alias conflict)
	if _, ok := reg.Lookup("greet"); ok {
		t.Fatal("expected 'greet' NOT to be registered due to alias conflict")
	}
}

func TestHelpFormatWithPluginCommands(t *testing.T) {
	defs := []commands.Definition{
		{Name: "help", Description: "Show help", Usage: "/help"},
		{Name: "hello", Description: "Say hello", Usage: "/hello", Source: "plugin:test"},
	}

	reg := commands.NewRegistry(defs)
	allDefs := reg.Definitions()

	// Simulate /help handler
	var hasPluginSection bool
	for i, def := range allDefs {
		if def.Source != "" {
			hasPluginSection = true
			_ = i
		}
	}
	if !hasPluginSection {
		t.Fatal("expected plugin commands in definitions")
	}
}
