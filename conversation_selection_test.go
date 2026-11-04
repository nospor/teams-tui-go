package main

import (
	"errors"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestConversationSwitchClearsMessageActions(t *testing.T) {
	m := bookmarkChats([]Chat{{ID: "a"}, {ID: "b"}})
	m.app.Messages = []Message{{ID: "a-message"}}
	m.app.NextLink = "a-next"
	m.app.MessagePopupMode = true
	m.app.DeleteConfirmMode = true
	m.app.AttachmentCursorMode = true
	m.app.PendingScrollID = "a-message"
	m.app.SelectedIndex = 1
	m, _ = m.loadChatMessages("b")
	if m.app.MessagesConversationID != "b" || len(m.app.Messages) != 0 || m.app.NextLink != "" {
		t.Fatal("switch retained another conversation's transcript")
	}
	if m.app.MessagePopupMode || m.app.DeleteConfirmMode || m.app.AttachmentCursorMode || m.app.PendingScrollID != "" {
		t.Fatal("switch retained message-specific actions")
	}
}

func TestDelayedMessagesAfterFilterNeverEnterSelectedTranscript(t *testing.T) {
	m := bookmarkChats([]Chat{{ID: "a", ChatType: "group"}, {ID: "b", ChatType: "oneOnOne"}})
	m.app.Messages = []Message{{ID: "a-message"}}
	m, _ = m.applyChatBookmark(bookmarkPreset("d"))
	m, _ = m.updateInternal(MsgMessagesLoaded{ChatID: "a", Messages: []Message{{ID: "late-a"}}})
	if m.app.GetSelectedChat().ID != "b" || m.app.MessagesConversationID != "b" || len(m.app.Messages) != 0 {
		t.Fatal("delayed chat load crossed the filter boundary")
	}
	m, _ = m.updateInternal(MsgMoreMessagesLoaded{ConversationID: "a", Messages: []Message{{ID: "older-a"}}})
	if len(m.app.Messages) != 0 {
		t.Fatal("delayed pagination crossed the filter boundary")
	}
	m, _ = m.updateInternal(MsgMessagesLoaded{ChatID: "b", Messages: []Message{{ID: "b-message"}}})
	if len(m.app.Messages) != 1 || m.app.Messages[0].ID != "b-message" {
		t.Fatalf("wrong transcript: %+v", m.app.Messages)
	}
}

func TestEmptyFilterClearsTranscriptOwner(t *testing.T) {
	m := bookmarkChats([]Chat{{ID: "a", ChatType: "group"}})
	m.app.Messages = []Message{{ID: "a-message"}}
	m, _ = m.applyChatBookmark(bookmarkPreset("d"))
	if m.app.GetSelectedChat() != nil || m.app.MessagesConversationID != "" || len(m.app.Messages) != 0 {
		t.Fatal("empty list retained a transcript")
	}
}

func TestIdleUpdateRepairsTranscriptOwnership(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := bookmarkChats([]Chat{{ID: "a"}, {ID: "b"}})
	m.app.Messages = []Message{{ID: "a-message"}}
	m.app.SelectedIndex = 1
	m.viewCache.dirty = false
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = updated.(Model)
	if m.app.MessagesConversationID != "b" || len(m.app.Messages) != 0 || !m.viewCache.dirty {
		t.Fatal("update did not reconcile transcript owner")
	}
}

func TestBackgroundChannelErrorDoesNotClearActiveConversation(t *testing.T) {
	m := bookmarkChats([]Chat{{ID: "a"}})
	m.channelSelectedIndex = 0
	m.app.SelectedChannelTeamID, m.app.SelectedChannelID = "team", "active"
	m.app.ActivateConversation("active")
	m.app.Messages = []Message{{ID: "active-message"}}
	m, _ = m.updateInternal(MsgChannelMessagesLoaded{TeamID: "other", ChannelID: "background", Err: errors.New("offline")})
	if m.app.SelectedChannelID != "active" || m.app.SelectedChannelTeamID != "team" || len(m.app.Messages) != 1 {
		t.Fatal("background error changed the active channel")
	}
}

func TestReorderingKeepsChatAndTranscriptIdentity(t *testing.T) {
	m := bookmarkChats([]Chat{{ID: "a"}, {ID: "b"}})
	m.app.Messages = []Message{{ID: "a-message"}}
	m.promoteChat("b")
	m = m.rebuildChatList()
	m, cmd := m.syncChatListSelection("a")
	if m.app.SelectedIndex != 1 || m.app.GetSelectedChat().ID != "a" || m.app.MessagesConversationID != "a" || m.app.Messages[0].ID != "a-message" || cmd != nil {
		t.Fatal("reordering changed conversation identity")
	}
}
