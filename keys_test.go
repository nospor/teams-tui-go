package main

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

func TestResolveKeyMapDefaults(t *testing.T) {
	km, warns := ResolveKeyMap(nil)
	if len(warns) != 0 {
		t.Fatalf("unexpected warnings: %v", warns)
	}
	if got := km.Normal.Next.Keys(); !sameKeys(got, []string{"j", "down"}) {
		t.Fatalf("default next = %v", got)
	}
	// File picker uses enter for both opening a directory and selecting a file.
	if !key.Matches(tea.KeyMsg{Type: tea.KeyEnter}, km.FilePicker.Open) || !key.Matches(tea.KeyMsg{Type: tea.KeyEnter}, km.FilePicker.Select) {
		t.Fatal("enter should stay on both filepicker open and select")
	}
}

func TestResolveKeyMapSharedAndModeOverride(t *testing.T) {
	raw := []byte(`{
		"next": ["down", "ctrl+n"],
		"normal": {"next": ["j", "Down"], "compose": ["j"]}
	}`)
	km, warns := ResolveKeyMap(raw)
	joined := strings.Join(warns, "\n")
	if !strings.Contains(joined, `kept on next`) {
		t.Fatalf("expected next to keep j, got %v", warns)
	}
	// Both actions are set in the mode, so the earlier one (next) keeps j.
	if got := km.Normal.Next.Keys(); !sameKeys(got, []string{"j", "down"}) {
		t.Fatalf("normal.next = %v, warnings %v", got, warns)
	}
	if got := km.Normal.Compose.Keys(); len(got) != 0 {
		t.Fatalf("normal.compose should lose j, got %v", got)
	}
	// Other modes take the shared list.
	if got := km.Message.Next.Keys(); !sameKeys(got, []string{"down", "ctrl+n"}) {
		t.Fatalf("message.next = %v", got)
	}
	if got := km.Normal.Prev.Keys(); !sameKeys(got, []string{"k", "up"}) {
		t.Fatalf("prev should stay default, got %v", got)
	}

	// A mode-only override wins over the default that shared the same key.
	km, warns = ResolveKeyMap([]byte(`{"normal": {"compose": ["j"]}}`))
	if got := km.Normal.Compose.Keys(); !sameKeys(got, []string{"j"}) {
		t.Fatalf("compose = %v", got)
	}
	if got := km.Normal.Next.Keys(); !sameKeys(got, []string{"down"}) {
		t.Fatalf("next should drop j, got %v (warnings %v)", got, warns)
	}
}

func TestResolveKeyMapUnbindAndUnknown(t *testing.T) {
	raw := []byte(`{"normal": {"favourite": []}, "nope": ["x"], "next": "j"}`)
	km, warns := ResolveKeyMap(raw)
	if len(km.Normal.Favourite.Keys()) != 0 {
		t.Fatalf("favourite should be unbound, got %v", km.Normal.Favourite.Keys())
	}
	if pressed(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}}, km.Normal.Favourite) {
		t.Fatal("unbound favourite still matched")
	}
	joined := strings.Join(warns, "\n")
	if !strings.Contains(joined, "unknown key") || !strings.Contains(joined, "expected an array") {
		t.Fatalf("warnings = %v", warns)
	}
	if got := km.Normal.Next.Keys(); !sameKeys(got, []string{"j", "down"}) {
		t.Fatalf("invalid shared next should keep the default, got %v", got)
	}
}

func TestFormatKeysUsesResolvedBindings(t *testing.T) {
	km, _ := ResolveKeyMap([]byte(`{"normal": {"quit": ["ctrl+x", "q"]}}`))
	got := FormatKeys(km.Normal.Quit, " / ")
	if got != "Ctrl+X / q" {
		t.Fatalf("format = %q", got)
	}
	if slashKeys(km.Normal.Next, km.Normal.Prev) != "j/↓/k/↑" {
		t.Fatalf("slash = %q", slashKeys(km.Normal.Next, km.Normal.Prev))
	}
}

func TestResolveKeyMapChatActionsOverlay(t *testing.T) {
	km, warns := ResolveKeyMap([]byte(`{
		"normal": {"actions": ["A"]},
		"chat_actions": {"export": ["x"], "compose": ["c"]}
	}`))
	if len(warns) != 0 {
		t.Fatalf("unexpected warnings: %v", warns)
	}
	if got := km.Normal.Actions.Keys(); !sameKeys(got, []string{"A"}) {
		t.Fatalf("normal.actions = %v", got)
	}
	if got := km.ChatActions.Export.Keys(); !sameKeys(got, []string{"x"}) {
		t.Fatalf("chat_actions.export = %v", got)
	}
	if got := km.ChatActions.Compose.Keys(); !sameKeys(got, []string{"c"}) {
		t.Fatalf("chat_actions.compose = %v", got)
	}
	if got := km.ChatActions.Favourite.Keys(); !sameKeys(got, []string{"f"}) {
		t.Fatalf("favourite should keep default, got %v", got)
	}
	if got := km.Message.Edit.Keys(); !sameKeys(got, []string{"e"}) {
		t.Fatalf("message.edit should stay e, got %v", got)
	}
}

func sameKeys(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
