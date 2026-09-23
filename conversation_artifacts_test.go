package main

import (
	"encoding/json"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func artifactEvent(id, eventType, createdAt, webURL string) Message {
	return Message{
		ID:              id,
		CreatedDateTime: createdAt,
		MessageType:     "systemEventMessage",
		WebURL:          webURL,
		EventDetail: &EventMessageDetail{
			ODataType: eventType,
		},
	}
}

func TestCollectConversationArtifactsPreservesDuplicateEvents(t *testing.T) {
	first := artifactEvent("recording-1", "#microsoft.graph.callRecordingEventMessageDetail", "2026-08-02T15:11:00Z", "https://teams.microsoft.com/l/message/one")
	first.EventDetail.CallRecordingURL = "https://contoso.sharepoint.com/recording-one"
	second := artifactEvent("recording-2", "#microsoft.graph.callRecordingEventMessageDetail", "2026-08-02T15:12:00Z", "https://teams.microsoft.com/l/message/two")
	second.EventDetail.CallRecordingURL = "https://contoso.sharepoint.com/recording-two"

	artifacts := collectConversationArtifacts([]Message{first, second}, "https://teams.microsoft.com/l/chat/fallback")
	if len(artifacts) != 2 {
		t.Fatalf("duplicate recording events were collapsed: %#v", artifacts)
	}
	if artifacts[0].URL != first.EventDetail.CallRecordingURL || artifacts[1].URL != second.EventDetail.CallRecordingURL {
		t.Fatalf("direct recording URLs were not retained: %#v", artifacts)
	}
}

func TestTranscriptArtifactUsesEventThenConversationFallback(t *testing.T) {
	withEventURL := artifactEvent("transcript-1", "#microsoft.graph.callTranscriptEventMessageDetail", "2026-08-02T15:11:00Z", "https://teams.microsoft.com/l/message/transcript")
	withoutEventURL := artifactEvent("transcript-2", "#microsoft.graph.callTranscriptEventMessageDetail", "2026-08-02T15:12:00Z", "")
	fallback := "https://teams.microsoft.com/l/chat/fallback"

	artifacts := collectConversationArtifacts([]Message{withEventURL, withoutEventURL}, fallback)
	if len(artifacts) != 2 {
		t.Fatalf("transcript artifacts = %#v", artifacts)
	}
	if artifacts[0].URL != withEventURL.WebURL || artifacts[0].DirectLink {
		t.Fatalf("transcript event URL fallback incorrect: %#v", artifacts[0])
	}
	if artifacts[1].URL != fallback {
		t.Fatalf("conversation fallback incorrect: %#v", artifacts[1])
	}
}

func TestEventDetailDecodesTranscriptIdentifiers(t *testing.T) {
	var detail EventMessageDetail
	if err := json.Unmarshal([]byte(`{"callId":"call-123","callTranscriptICalUid":"ical-456"}`), &detail); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if detail.CallID != "call-123" || detail.CallTranscriptICalUID != "ical-456" {
		t.Fatalf("transcript identifiers lost: %#v", detail)
	}
}

func TestChatActionPopupOpensArtifactChooser(t *testing.T) {
	name := "Ada"
	app := NewApp()
	app.Chats = []Chat{{ID: "chat-1", CachedDisplayName: &name}}
	app.SelectedIndex = 0
	app.Messages = []Message{artifactEvent("rec-1", "#microsoft.graph.callRecordingEventMessageDetail", "2026-08-02T15:11:00Z", "https://teams.microsoft.com/l/message/one")}
	app.Messages[0].EventDetail.CallRecordingURL = "https://contoso.sharepoint.com/recording-one"
	model := NewModel(app, "client", "user")
	model.app.ChatActionPopupMode = true

	model, cmd := model.handleChatActionPopupKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	if model.app.ChatActionPopupMode {
		t.Fatal("artifacts action left the chat-actions popup open")
	}
	if !model.app.ArtifactPopupMode {
		t.Fatalf("expected artifact popup, status=%q cmd=%v", model.app.Status, cmd)
	}
	if len(model.app.Artifacts) != 1 {
		t.Fatalf("artifacts = %#v", model.app.Artifacts)
	}

	model, cmd = model.handleConversationArtifactPopupKey(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Fatal("Enter should copy the URL, not open the browser")
	}
	if model.app.ArtifactPopupMode && !strings.Contains(model.app.Status, "Could not copy") {
		t.Fatalf("expected popup to close after copy, status=%q", model.app.Status)
	}

	model.app.ArtifactPopupMode = true
	model, cmd = model.handleConversationArtifactPopupKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	if cmd == nil {
		t.Fatal("expected openURL command for o")
	}
}

func TestConversationArtifactPopupKeepsFooter(t *testing.T) {
	name := "SRDS One Project Planning Session"
	app := NewApp()
	app.Chats = []Chat{{ID: "chat-1", CachedDisplayName: &name}}
	app.SelectedIndex = 0
	model := NewModel(app, "client", "user")
	for i := 0; i < 33; i++ {
		title := "SRDS One Project Planning Session-20260916_153012UTC-Meeting Recording.mp4"
		model.app.Artifacts = append(model.app.Artifacts, ConversationArtifact{
			Kind:       ConversationRecording,
			Title:      title,
			URL:        "https://teams.microsoft.com/l/message/" + strings.Repeat("x", 40),
			DirectLink: i%4 == 0,
		})
	}
	view := model.renderConversationArtifactPopup(70, 16)
	if !strings.Contains(view, "copy") {
		t.Fatalf("missing copy footer:\n%s", view)
	}
	if !strings.Contains(view, "close") {
		t.Fatalf("missing close footer:\n%s", view)
	}
	if !strings.Contains(view, "╭") || !strings.Contains(view, "╰") {
		t.Fatalf("missing rounded border:\n%s", view)
	}
	if strings.Count(view, "\n")+1 > 20 {
		t.Fatalf("popup grew past its height: %d lines", strings.Count(view, "\n")+1)
	}
}

func TestChatActionArtifactsKeyIsConfigurable(t *testing.T) {
	km, _ := ResolveKeyMap([]byte(`{"chat_actions": {"artifacts": ["T"]}, "artifacts": {"open_url": ["x"]}}`))
	if got := km.ChatActions.Artifacts.Keys(); !sameKeys(got, []string{"T"}) {
		t.Fatalf("chat_actions.artifacts = %v", got)
	}
	if got := km.Artifacts.OpenURL.Keys(); !sameKeys(got, []string{"x"}) {
		t.Fatalf("artifacts.open_url = %v", got)
	}
	if pressed(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}}, km.ChatActions.Artifacts) {
		t.Fatal("default t still matched after remap")
	}
}

func TestOpenConversationArtifactsRequiresLoadedEvents(t *testing.T) {
	name := "Ada"
	app := NewApp()
	app.Chats = []Chat{{ID: "chat-1", CachedDisplayName: &name}}
	app.SelectedIndex = 0
	model := NewModel(app, "client", "user")
	model, _ = model.openConversationArtifacts()
	if model.app.ArtifactPopupMode {
		t.Fatal("opened artifact popup with no events")
	}
	if !strings.Contains(model.app.Status, "No recordings") {
		t.Fatalf("status = %q", model.app.Status)
	}
}
