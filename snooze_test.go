package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func snoozeTestDir(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
}

func TestQuickSnoozeHidesChatUntilBookmark(t *testing.T) {
	snoozeTestDir(t)
	chats := []Chat{
		{ID: "one", ChatType: "oneOnOne"},
		{ID: "two", ChatType: "group"},
	}
	model := bookmarkChats(chats)
	model.app.DefaultSnoozeMinutes = 180

	model, _ = model.handleKey(runeKey('z'))
	if model.chatSnoozed("one", time.Now().Add(time.Minute)) == false {
		t.Fatal("expected chat one to be snoozed")
	}
	if len(model.latestChats) != 2 {
		t.Fatalf("latestChats = %d", len(model.latestChats))
	}
	if len(model.app.Chats) != 1 || model.app.Chats[0].ID != "two" {
		t.Fatalf("visible after snooze = %+v", model.app.Chats)
	}
	if model.app.GetSelectedChat() == nil || model.app.GetSelectedChat().ID != "two" {
		t.Fatal("selection should move to the next visible chat")
	}

	model, _ = model.handleKey(runeKey('b'))
	model, _ = model.handleKey(runeKey('z'))
	if len(model.app.Chats) != 1 || model.app.Chats[0].ID != "one" {
		t.Fatalf("snoozed bookmark = %+v", model.app.Chats)
	}

	path := filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "teams-tui-go", "snoozed_chats.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected snoozed_chats.json: %v", err)
	}
}

func TestSnoozeMenuUnsnoozeAndRemap(t *testing.T) {
	snoozeTestDir(t)
	model := bookmarkChats([]Chat{{ID: "one", ChatType: "oneOnOne"}, {ID: "two", ChatType: "group"}})
	model = model.openSnoozePopup()
	if !model.app.SnoozePopupMode {
		t.Fatal("expected snooze popup")
	}
	model, _ = model.handleSnoozePopupKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
	if model.app.SnoozePopupMode {
		t.Fatal("3h left the popup open")
	}
	if !model.chatSnoozed("one", time.Now().Add(time.Hour)) {
		t.Fatal("expected a 3-hour snooze")
	}

	km, _ := ResolveKeyMap([]byte(`{
		"normal": {"snooze_menu": ["X"]},
		"snooze": {"unsnooze": ["c"]}
	}`))
	model.app.Keys = km
	model.app.SelectedIndex = 0
	model, _ = model.applyChatBookmark(bookmarkPreset("z"))
	model = model.openSnoozePopup()
	model, _ = model.handleSnoozePopupKey(runeKey('u'))
	if !model.chatSnoozed("one", time.Now()) {
		t.Fatal("default u should not unsnooze after remap")
	}
	model = model.openSnoozePopup()
	model, _ = model.handleSnoozePopupKey(runeKey('c'))
	if model.chatSnoozed("one", time.Now()) {
		t.Fatal("configured unsnooze key did not clear the snooze")
	}
	rendered := stripANSI(model.renderSnoozePopup(60, 22))
	if !strings.Contains(rendered, "c") {
		t.Fatalf("popup missing remapped unsnooze key:\n%s", rendered)
	}
}

func TestWakeChatAndExpiredPrune(t *testing.T) {
	snoozeTestDir(t)
	model := bookmarkChats([]Chat{{ID: "one", ChatType: "oneOnOne"}, {ID: "two", ChatType: "group"}})
	model.snoozed["one"] = time.Now().Add(time.Hour)
	model = model.rebuildChatList()
	if len(model.app.Chats) != 1 {
		t.Fatalf("snoozed chat still visible = %+v", model.app.Chats)
	}
	if !model.wakeChat("one") {
		t.Fatal("wakeChat should remove the snooze")
	}
	model = model.rebuildChatList()
	if len(model.app.Chats) != 2 {
		t.Fatalf("woken chat missing = %+v", model.app.Chats)
	}

	model.snoozed["two"] = time.Now().Add(-time.Minute)
	if !model.pruneExpiredSnoozes(time.Now()) {
		t.Fatal("expected expired snooze to be pruned")
	}
	if _, ok := model.snoozed["two"]; ok {
		t.Fatal("expired snooze remained")
	}
}

func TestNextLocalClock(t *testing.T) {
	now := time.Date(2026, 9, 29, 19, 0, 0, 0, time.Local)
	got := nextLocalClock(now, "07:00", 7, true)
	if got.Hour() != 7 || got.Day() != 30 {
		t.Fatalf("tomorrow morning = %v", got)
	}
	got = nextLocalClock(now, "18:00", 18, false)
	if got.Hour() != 7 || got.Day() != 30 {
		t.Fatalf("past workday end should fall to next morning = %v", got)
	}
}
