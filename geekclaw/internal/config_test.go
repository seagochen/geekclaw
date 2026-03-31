package internal

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetConfigPath(t *testing.T) {
	t.Setenv("HOME", "/tmp/home")

	got := GetConfigPath()
	// No config file exists in /tmp/home/.geekclaw, so the default .yaml path is returned.
	want := filepath.Join("/tmp/home", ".geekclaw", "configs", "config.yaml")

	assert.Equal(t, want, got)
}

func TestGetConfigPath_WithGEEKCLAW_HOME(t *testing.T) {
	t.Setenv("GEEKCLAW_HOME", "/custom/geekclaw")
	t.Setenv("HOME", "/tmp/home")

	got := GetConfigPath()
	want := filepath.Join("/custom/geekclaw", "configs", "config.yaml")

	assert.Equal(t, want, got)
}

func TestGetConfigPath_WithGEEKCLAW_CONFIG(t *testing.T) {
	t.Setenv("GEEKCLAW_CONFIG", "/custom/config.json")
	t.Setenv("GEEKCLAW_HOME", "/custom/geekclaw")
	t.Setenv("HOME", "/tmp/home")

	got := GetConfigPath()
	want := "/custom/config.json"

	assert.Equal(t, want, got)
}

func TestGetConfigPath_Windows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows-specific HOME behavior varies; run on windows")
	}

	testUserProfilePath := `C:\Users\Test`
	t.Setenv("USERPROFILE", testUserProfilePath)

	got := GetConfigPath()
	want := filepath.Join(testUserProfilePath, ".geekclaw", "configs", "config.yaml")

	require.True(t, strings.EqualFold(got, want), "GetConfigPath() = %q, want %q", got, want)
}
