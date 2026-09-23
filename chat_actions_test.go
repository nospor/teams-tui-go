package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestChatActionPopupOpensOnlyForChats(t *testing.T) {
	name := "Ada"
	app := NewApp()
	app.Chats = []Chat{{ID: "chat-1", CachedDisplayName: &name}}
	app.SelectedIndex = 0
	model := NewModel(app, "client", "user")

	model = model.openChatActionPopup()
	if !model.app.ChatActionPopupMode {
		t.Fatal("expected chat action popup to open for a selected chat")
	}

	model.app.ChatActionPopupMode = false
	model.channelSelectedIndex = 0
	model = model.openChatActionPopup()
	if model.app.ChatActionPopupMode {
		t.Fatal("popup opened in channel mode")
	}
	if !strings.Contains(model.app.Status, "Select a chat") {
		t.Fatalf("status = %q", model.app.Status)
	}
}

func TestChatActionPopupExportKeyClosesAndStartsExport(t *testing.T) {
	name := "Ada"
	app := NewApp()
	app.Chats = []Chat{{ID: "chat-1", CachedDisplayName: &name}}
	app.SelectedIndex = 0
	model := NewModel(app, "client", "user")
	model.app.ChatActionPopupMode = true

	model, cmd := model.handleChatActionPopupKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	if model.app.ChatActionPopupMode {
		t.Fatal("export left the popup open")
	}
	if cmd == nil {
		t.Fatal("export did not return a command")
	}
	if !strings.Contains(model.app.Status, "Exporting") {
		t.Fatalf("status = %q", model.app.Status)
	}
}

func TestChatActionPopupHonoursConfiguredExportKey(t *testing.T) {
	name := "Ada"
	app := NewApp()
	app.Chats = []Chat{{ID: "chat-1", CachedDisplayName: &name}}
	app.SelectedIndex = 0
	km, _ := ResolveKeyMap([]byte(`{"chat_actions": {"export": ["x"]}}`))
	app.Keys = km
	model := NewModel(app, "client", "user")
	model.app.ChatActionPopupMode = true

	model, cmd := model.handleChatActionPopupKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	if !model.app.ChatActionPopupMode || cmd != nil {
		t.Fatal("default e still exported after remap")
	}
	model, cmd = model.handleChatActionPopupKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if model.app.ChatActionPopupMode || cmd == nil {
		t.Fatal("configured export key did not run export")
	}
	rendered := stripANSI(model.renderChatActionPopup(60, 20))
	if !strings.Contains(rendered, "x") {
		t.Fatalf("popup did not show remapped export key:\n%s", rendered)
	}
}
