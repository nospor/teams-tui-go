package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCollectAllChatMessagesPaginatesDeduplicatesAndSorts(t *testing.T) {
	initial := []Message{
		{ID: "two", CreatedDateTime: "2026-07-03T10:00:00Z"},
		{ID: "one", CreatedDateTime: "2026-07-03T09:00:00Z"},
	}
	calls := 0
	got, err := collectAllChatMessages(initial, "next-1", func(link string) ([]Message, string, error) {
		calls++
		if link != "next-1" {
			return nil, "", errors.New("unexpected link")
		}
		return []Message{
			{ID: "two", CreatedDateTime: "2026-07-03T10:00:00Z"},
			{ID: "three", CreatedDateTime: "2026-07-03T11:00:00Z"},
		}, "", nil
	})
	if err != nil {
		t.Fatalf("collectAllChatMessages failed: %v", err)
	}
	if calls != 1 || len(got) != 3 {
		t.Fatalf("expected one next-page request and three messages, calls=%d messages=%d", calls, len(got))
	}
	for index, id := range []string{"one", "two", "three"} {
		if got[index].ID != id {
			t.Fatalf("message %d: expected %s, got %s", index, id, got[index].ID)
		}
	}
}

func TestCollectAllChatMessagesRejectsRepeatedLink(t *testing.T) {
	_, err := collectAllChatMessages(nil, "loop", func(string) ([]Message, string, error) {
		return nil, "loop", nil
	})
	if err == nil || !strings.Contains(err.Error(), "repeated") {
		t.Fatalf("expected repeated-link error, got %v", err)
	}
}

func TestRenderChatMarkdown(t *testing.T) {
	title := "Design review"
	sender := "Ada Lovelace"
	body := "<p>Hello <b>team</b>.</p><ul><li>First</li><li>Second</li></ul>"
	attachmentName := "notes.pdf"
	attachmentURL := "https://example.sharepoint.com/notes.pdf"
	chat := Chat{ChatType: "group", CachedDisplayName: &title}
	messages := []Message{{
		ID:              "1",
		CreatedDateTime: "2026-07-03T09:15:00Z",
		From:            &MessageFrom{User: &MessageUser{DisplayName: &sender}},
		Body:            &MessageBody{Content: &body},
		Attachments: []MessageAttachment{{
			Name:       &attachmentName,
			ContentURL: &attachmentURL,
		}},
		Reactions: []MessageReaction{{ReactionType: "like"}, {ReactionType: "like"}},
	}}

	exportedAt := time.Date(2026, time.July, 4, 12, 0, 0, 0, time.UTC)
	got := RenderChatMarkdown(chat, messages, exportedAt)
	for _, want := range []string{
		"# Design review",
		"**Messages:** 1",
		"### Ada Lovelace",
		"Hello **team**.",
		"- First",
		"[notes.pdf](https://example.sharepoint.com/notes.pdf)",
		"👍 2",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("export missing %q:\n%s", want, got)
		}
	}
}

func TestRenderChatMarkdownUsesSystemEventSummary(t *testing.T) {
	message := Message{
		ID:              "event-1",
		CreatedDateTime: "2026-08-02T12:00:00Z",
		MessageType:     "systemEventMessage",
		EventDetail: &EventMessageDetail{
			ODataType:     "#microsoft.graph.callEndedEventMessageDetail",
			CallEventType: "meeting",
			CallDuration:  "PT10S",
		},
	}
	got := RenderChatMarkdown(Chat{}, []Message{message}, time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC))
	for _, want := range []string{"### Teams", "Meeting ended (10s)"} {
		if !strings.Contains(got, want) {
			t.Fatalf("Markdown export missing %q:\n%s", want, got)
		}
	}
}

func TestExportChatMarkdownCreatesPrivateFile(t *testing.T) {
	directory := t.TempDir()
	title := "Team / Alpha"
	chat := Chat{ChatType: "group", CachedDisplayName: &title}
	exportedAt := time.Date(2026, time.July, 4, 12, 0, 0, 0, time.Local)
	path, err := ExportChatMarkdown(directory, chat, nil, exportedAt)
	if err != nil {
		t.Fatalf("ExportChatMarkdown failed: %v", err)
	}
	if filepath.Dir(path) != directory {
		t.Fatalf("expected export in %s, got %s", directory, path)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("export does not exist: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("expected mode 0600, got %o", info.Mode().Perm())
	}
}
