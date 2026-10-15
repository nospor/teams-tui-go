package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestSleepEnterRestoresChatByID(t *testing.T) {
	alice := "Alice"
	bob := "Bob"
	chats := []Chat{
		{ID: "bob", CachedDisplayName: &bob},
		{ID: "alice", CachedDisplayName: &alice},
	}
	model := bookmarkChats(chats)
	model.app.SelectedIndex = 1

	model, _ = model.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
	if model.app.SelectedIndex != -1 || model.channelSelectedIndex != -1 {
		t.Fatalf("sleep left selection chat=%d channel=%d", model.app.SelectedIndex, model.channelSelectedIndex)
	}
	if model.sleepReturnChatID != "alice" {
		t.Fatalf("remembered %q", model.sleepReturnChatID)
	}
	if !strings.Contains(model.renderRightPanel(40, 16), "Alice") {
		t.Fatal("sleep screen did not name the remembered chat")
	}

	// A new message can move the row while asleep. Restore follows the id.
	model.app.Chats = []Chat{chats[1], chats[0]}

	model, _ = model.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
	if model.sleepReturnChatID != "alice" || model.app.SelectedIndex != -1 {
		t.Fatalf("second esc cleared memory: id=%q idx=%d", model.sleepReturnChatID, model.app.SelectedIndex)
	}

	model, _ = model.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	got := model.app.GetSelectedChat()
	if got == nil || got.ID != "alice" || model.app.SelectedIndex != 0 {
		t.Fatalf("restored idx=%d chat=%v", model.app.SelectedIndex, got)
	}
}

func TestSleepEnterStaysAsleepWhenChatIsFiltered(t *testing.T) {
	alice := "Alice"
	bob := "Bob"
	hidden := Chat{ID: "alice", CachedDisplayName: &alice}
	visible := Chat{ID: "bob", CachedDisplayName: &bob}
	model := bookmarkChats([]Chat{visible, hidden})
	model.app.SelectedIndex = 1

	model, _ = model.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
	model.app.Chats = []Chat{visible}
	model.chatListCache["alice"] = hidden

	model, _ = model.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if model.app.SelectedIndex != -1 {
		t.Fatalf("filtered chat was opened at index %d", model.app.SelectedIndex)
	}
	if model.sleepReturnChatID != "alice" {
		t.Fatalf("memory cleared: %q", model.sleepReturnChatID)
	}
	if !strings.Contains(model.app.Status, "Alice") {
		t.Fatalf("status = %q", model.app.Status)
	}
}

func TestSleepEnterRestoresChannel(t *testing.T) {
	bob := "Bob"
	model := bookmarkChats([]Chat{{ID: "bob", CachedDisplayName: &bob}})
	model.app.SelectedIndex = 0
	model.app.TeamsData = []TeamWithChannels{{
		Team:     Team{ID: "team", DisplayName: "Engineering"},
		Channels: []Channel{{ID: "chan", DisplayName: "General"}},
	}}
	model.channelSelectedIndex = 0
	model.app.SelectedChannelTeamID = "team"
	model.app.SelectedChannelID = "chan"

	model, _ = model.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
	if model.sleepReturnChannelID != "chan" || model.sleepReturnTeamID != "team" || model.sleepReturnChatID != "bob" {
		t.Fatalf("remembered chat=%q team=%q channel=%q", model.sleepReturnChatID, model.sleepReturnTeamID, model.sleepReturnChannelID)
	}
	if model.app.SelectedIndex != -1 || model.channelSelectedIndex != -1 {
		t.Fatal("channel sleep did not clear selection")
	}

	model, _ = model.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if model.channelSelectedIndex != 0 || model.app.SelectedChannelID != "chan" {
		t.Fatalf("channel idx=%d id=%q", model.channelSelectedIndex, model.app.SelectedChannelID)
	}
	if model.app.GetSelectedChat() == nil || model.app.GetSelectedChat().ID != "bob" {
		t.Fatalf("sidebar chat = %+v", model.app.GetSelectedChat())
	}
}

func TestEnterOutsideSleepDoesNothing(t *testing.T) {
	bob := "Bob"
	model := bookmarkChats([]Chat{{ID: "bob", CachedDisplayName: &bob}})
	model.app.SelectedIndex = 0

	model, _ = model.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if model.app.SelectedIndex != 0 || model.sleepReturnChatID != "" {
		t.Fatalf("enter changed selection=%d memory=%q", model.app.SelectedIndex, model.sleepReturnChatID)
	}
}
