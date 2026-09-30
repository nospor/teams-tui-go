package main

import (
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestFilePickerZoxideOpenAndClose(t *testing.T) {
	app := &App{Keys: DefaultKeyMap(), Features: FeatureFlags{FileUpload: true}}
	m := NewModel(app, "client", "user")
	m.app.FilePickerPopupMode = true

	m, cmd := m.handleFilePickerKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'z'}})
	if cmd == nil {
		t.Fatal("expected load cmd")
	}
	if !m.app.FilePickerZoxideMode {
		t.Fatal("expected zoxide mode")
	}

	m = m.closeFilePickerZoxide()
	if m.app.FilePickerZoxideMode {
		t.Fatal("expected zoxide mode off")
	}
	if !m.app.FilePickerPopupMode {
		t.Fatal("file picker should stay open")
	}
}

func TestJumpFilePickerToDirectory(t *testing.T) {
	dir := t.TempDir()
	app := &App{Keys: DefaultKeyMap()}
	m := NewModel(app, "client", "user")
	m.app.FilePickerZoxideMode = true

	m, cmd := m.jumpFilePickerToDirectory(dir)
	if cmd == nil {
		t.Fatal("expected readDir cmd")
	}
	if m.app.FilePickerZoxideMode {
		t.Fatal("zoxide overlay should close")
	}
	if m.filepicker.CurrentDirectory != dir {
		t.Fatalf("directory = %q", m.filepicker.CurrentDirectory)
	}
}

func TestJumpFilePickerToMissingDirectory(t *testing.T) {
	app := &App{Keys: DefaultKeyMap()}
	m := NewModel(app, "client", "user")
	m.app.FilePickerZoxideMode = true

	m, cmd := m.jumpFilePickerToDirectory(filepath.Join(t.TempDir(), "missing"))
	if cmd != nil {
		t.Fatal("expected no cmd")
	}
	if !m.app.FilePickerZoxideMode {
		t.Fatal("overlay should stay open on error")
	}
	if m.app.FilePickerZoxideError == "" {
		t.Fatal("expected error message")
	}
}
