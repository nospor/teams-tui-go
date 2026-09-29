package main

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func runeKey(r rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
}

func bookmarkPreset(key string) chatBookmarkPreset {
	for _, preset := range builtinChatBookmarkPresets() {
		if preset.Key == key {
			return preset
		}
	}
	panic("unknown bookmark preset " + key)
}

func bookmarkChats(chats []Chat) Model {
	app := NewApp()
	app.Chats = chats
	if len(chats) > 0 {
		app.SelectedIndex = 0
	}
	model := NewModel(app, "client", "user")
	model.latestChats = append([]Chat(nil), chats...)
	for _, chat := range chats {
		model.stableChatOrder = append(model.stableChatOrder, chat.ID)
	}
	return model
}

func TestBookmarkUnreadFiltersSidebarAndKeepsInventory(t *testing.T) {
	other := "Bob"
	unread := Chat{
		ID:       "unread",
		ChatType: "oneOnOne",
		LastMessagePreview: &Message{
			ID:   "m1",
			From: &MessageFrom{User: &MessageUser{DisplayName: &other}},
		},
	}
	read := Chat{
		ID:                 "read",
		ChatType:           "group",
		LastMessagePreview: &Message{ID: "m2"},
	}
	model := bookmarkChats([]Chat{unread, read})
	model.lastMsgID["unread"] = "m1"
	model.lastMsgID["read"] = "m2"
	model.lastReadMsgID["read"] = "m2"

	model, _ = model.handleKey(runeKey('b'))
	if !model.app.ChatBookmarkPopupMode {
		t.Fatal("expected bookmark popup to open")
	}
	model, _ = model.handleKey(runeKey('u'))
	if model.app.ChatBookmarkPopupMode {
		t.Fatal("unread left the popup open")
	}
	if !chatFilterIsActive(model.app.ActiveChatFilter) {
		t.Fatal("unread filter was not applied")
	}
	if len(model.latestChats) != 2 {
		t.Fatalf("latestChats = %d, want 2", len(model.latestChats))
	}
	if len(model.app.Chats) != 1 || model.app.Chats[0].ID != "unread" {
		t.Fatalf("visible chats = %+v", model.app.Chats)
	}
	if model.app.SelectedIndex != 0 || model.app.GetSelectedChat().ID != "unread" {
		t.Fatalf("selected %d %+v", model.app.SelectedIndex, model.app.GetSelectedChat())
	}
	if !strings.Contains(model.app.Status, "Unread") {
		t.Fatalf("status = %q", model.app.Status)
	}

	model, _ = model.handleKey(runeKey('b'))
	model, _ = model.handleKey(runeKey('a'))
	if chatFilterIsActive(model.app.ActiveChatFilter) {
		t.Fatal("all did not clear the filter")
	}
	if len(model.app.Chats) != 2 {
		t.Fatalf("all visible = %d", len(model.app.Chats))
	}
}

func TestBookmarkPreservesSelectionByChatID(t *testing.T) {
	chats := []Chat{
		{ID: "one", ChatType: "oneOnOne"},
		{ID: "group", ChatType: "group"},
		{ID: "meet", ChatType: "meeting"},
	}
	model := bookmarkChats(chats)
	model.app.SelectedIndex = 1

	model, _ = model.applyChatBookmark(bookmarkPreset("g")) // groups
	if len(model.app.Chats) != 1 || model.app.Chats[0].ID != "group" {
		t.Fatalf("groups = %+v", model.app.Chats)
	}
	if model.app.GetSelectedChat() == nil || model.app.GetSelectedChat().ID != "group" {
		t.Fatal("selection was not restored by chat ID")
	}
}

func TestBookmarkFavouritesAndToday(t *testing.T) {
	now := time.Now().Format(time.RFC3339Nano)
	old := time.Now().Add(-48 * time.Hour).Format(time.RFC3339Nano)
	starred := Chat{ID: "star", ChatType: "oneOnOne", LastMessagePreview: &Message{CreatedDateTime: now}}
	plain := Chat{ID: "plain", ChatType: "group", LastMessagePreview: &Message{CreatedDateTime: old}}
	model := bookmarkChats([]Chat{starred, plain})
	model.favourites["star"] = true

	model, _ = model.applyChatBookmark(bookmarkPreset("f"))
	if len(model.app.Chats) != 1 || model.app.Chats[0].ID != "star" {
		t.Fatalf("favourites = %+v", model.app.Chats)
	}

	model, _ = model.applyChatBookmark(bookmarkPreset("t"))
	if len(model.app.Chats) != 1 || model.app.Chats[0].ID != "star" {
		t.Fatalf("today = %+v", model.app.Chats)
	}
}

func TestBookmarkPopupHonoursConfiguredOpenKey(t *testing.T) {
	model := bookmarkChats([]Chat{{ID: "one", ChatType: "oneOnOne"}})
	km, warns := ResolveKeyMap([]byte(`{
		"normal": {"bookmarks": ["x"]},
		"bookmarks": {"close": ["esc"]}
	}`))
	if len(warns) != 0 {
		t.Fatalf("warnings = %v", warns)
	}
	model.app.Keys = km

	model, _ = model.handleKey(runeKey('b'))
	if model.app.ChatBookmarkPopupMode {
		t.Fatal("default b still opened bookmarks after remap")
	}
	model, _ = model.handleKey(runeKey('x'))
	if !model.app.ChatBookmarkPopupMode {
		t.Fatal("configured bookmarks key did not open the popup")
	}
	rendered := stripANSI(model.renderChatBookmarkPopup(60, 24))
	if !strings.Contains(rendered, "Unread") || !strings.Contains(rendered, "u") {
		t.Fatalf("popup missing presets:\n%s", rendered)
	}
}

func TestChatHasActivityHelpers(t *testing.T) {
	now := time.Now()
	chat := Chat{LastMessagePreview: &Message{CreatedDateTime: now.Format(time.RFC3339Nano)}}
	if !chatHasActivityOn(chat, now.Local()) {
		t.Fatal("expected activity today")
	}
	if !chatHasActivitySince(chat, now.Add(-time.Hour)) {
		t.Fatal("expected activity within an hour")
	}
	old := Chat{LastMessagePreview: &Message{CreatedDateTime: now.Add(-48 * time.Hour).Format(time.RFC3339Nano)}}
	if chatHasActivityOn(old, now.Local()) {
		t.Fatal("old chat should not match today")
	}
}
