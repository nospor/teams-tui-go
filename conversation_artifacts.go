package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type ConversationArtifactKind string

const (
	ConversationRecording  ConversationArtifactKind = "recording"
	ConversationTranscript ConversationArtifactKind = "transcript"
)

type ConversationArtifact struct {
	Kind       ConversationArtifactKind
	MessageID  string
	Title      string
	URL        string
	CreatedAt  time.Time
	DirectLink bool
}

func artifactTime(value string) time.Time {
	when, _ := time.Parse(time.RFC3339Nano, value)
	return when.Local()
}

func collectConversationArtifacts(messages []Message, conversationURL string) []ConversationArtifact {
	artifacts := make([]ConversationArtifact, 0)
	for _, message := range messages {
		if message.EventDetail == nil {
			continue
		}
		kind := message.EventDetail.shortType()
		artifact := ConversationArtifact{
			MessageID: message.ID,
			CreatedAt: artifactTime(message.CreatedDateTime),
		}
		switch kind {
		case "callRecording":
			artifact.Kind = ConversationRecording
			artifact.Title = strings.TrimSpace(message.EventDetail.CallRecordingDisplayName)
			if artifact.Title == "" {
				artifact.Title = message.SystemEventSummary()
			}
			artifact.URL = strings.TrimSpace(message.EventDetail.CallRecordingURL)
			artifact.DirectLink = artifact.URL != ""
		case "callTranscript":
			artifact.Kind = ConversationTranscript
			artifact.Title = message.SystemEventSummary()
		default:
			continue
		}
		if artifact.URL == "" {
			artifact.URL = strings.TrimSpace(message.WebURL)
		}
		if artifact.URL == "" {
			artifact.URL = strings.TrimSpace(conversationURL)
		}
		artifacts = append(artifacts, artifact)
	}
	return artifacts
}

func (m Model) loadedMessagesForChat(chat Chat) []Message {
	seen := make(map[string]bool)
	var messages []Message
	add := func(candidates []Message) {
		for _, message := range candidates {
			key := message.ID
			if key == "" {
				continue
			}
			if !seen[key] {
				seen[key] = true
				messages = append(messages, message)
			}
		}
	}
	add(m.app.CachedMessages[chat.ID])
	add(m.app.HistoryMessages[chat.ID])
	if sel := m.app.GetSelectedChat(); sel != nil && sel.ID == chat.ID {
		add(m.app.Messages)
	}
	if chat.LastMessagePreview != nil {
		add([]Message{*chat.LastMessagePreview})
	}
	return messages
}

func (m Model) openConversationArtifacts() (Model, tea.Cmd) {
	if m.channelSelectedIndex >= 0 {
		m.app.SetStatus("Select a chat first", 3*time.Second)
		return m, nil
	}
	chat := m.app.GetSelectedChat()
	if chat == nil {
		m.app.SetStatus("Select a chat first", 3*time.Second)
		return m, nil
	}
	messages := m.loadedMessagesForChat(*chat)
	m.app.Artifacts = collectConversationArtifacts(messages, strings.TrimSpace(chat.WebURL))
	if len(m.app.Artifacts) == 0 {
		m.app.SetStatus("No recordings or transcripts in loaded conversation history", 4*time.Second)
		return m, nil
	}
	m.app.ArtifactSelectedIndex = 0
	m.app.ArtifactPopupMode = true
	return m, nil
}

func (m Model) selectedConversationArtifact() *ConversationArtifact {
	index := m.app.ArtifactSelectedIndex
	if index < 0 || index >= len(m.app.Artifacts) {
		return nil
	}
	return &m.app.Artifacts[index]
}

func (m Model) openSelectedConversationArtifact() (Model, tea.Cmd) {
	artifact := m.selectedConversationArtifact()
	if artifact == nil || artifact.URL == "" {
		m.app.SetStatus("This resource did not include an openable link", 4*time.Second)
		return m, nil
	}
	m.app.ArtifactPopupMode = false
	m.app.SetStatus("Opening "+string(artifact.Kind)+"…", 3*time.Second)
	return m, openURLCmd(artifact.URL, m.app.BrowserCommand, m.app.YoutrackCommand, m.app.GitlabCommand)
}

func (m Model) copySelectedConversationArtifact() Model {
	artifact := m.selectedConversationArtifact()
	if artifact == nil || artifact.URL == "" {
		m.app.SetStatus("This resource did not include an openable link", 4*time.Second)
		return m
	}
	if err := clipboard.WriteAll(artifact.URL); err != nil {
		m.app.SetStatus("Could not copy resource link: "+err.Error(), 4*time.Second)
		return m
	}
	m.app.SetStatus("Resource link copied", 3*time.Second)
	m.app.ArtifactPopupMode = false
	return m
}

func (m Model) handleConversationArtifactPopupKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	k := m.app.Keys.Artifacts
	switch {
	case pressed(msg, k.Close):
		m.app.ArtifactPopupMode = false
		return m, nil
	case pressed(msg, k.Next):
		if len(m.app.Artifacts) > 0 {
			m.app.ArtifactSelectedIndex = (m.app.ArtifactSelectedIndex + 1) % len(m.app.Artifacts)
		}
		return m, nil
	case pressed(msg, k.Prev):
		if len(m.app.Artifacts) > 0 {
			m.app.ArtifactSelectedIndex--
			if m.app.ArtifactSelectedIndex < 0 {
				m.app.ArtifactSelectedIndex = len(m.app.Artifacts) - 1
			}
		}
		return m, nil
	case pressed(msg, k.Confirm), pressed(msg, k.YankURL):
		return m.copySelectedConversationArtifact(), nil
	case pressed(msg, k.OpenURL):
		return m.openSelectedConversationArtifact()
	}
	return m, nil
}

func (m Model) renderConversationArtifactPopup(w, h int) string {
	if w < 1 {
		w = 1
	}
	if h < 8 {
		h = 8
	}
	innerW := w - 4
	if innerW < 1 {
		innerW = 1
	}
	innerH := h - 4
	if innerH < 6 {
		innerH = 6
	}

	k := m.app.Keys.Artifacts
	title := fitLine(lipgloss.NewStyle().Foreground(colYellow).Bold(true).Render(
		fmt.Sprintf("Recordings and transcripts (%d)", len(m.app.Artifacts))), innerW)
	chatName := ""
	if chat := m.app.GetSelectedChat(); chat != nil {
		chatName = fitLine(lipgloss.NewStyle().Foreground(colDimGray).Render(chatExportTitle(*chat)), innerW)
	}
	footer := fitLine(lipgloss.NewStyle().Foreground(colDimGray).Render(
		fmt.Sprintf("%s copy · %s open · %s close",
			slashKeys(k.Confirm, k.YankURL),
			FormatKeys(k.OpenURL, "/"),
			FormatKeys(k.Close, "/"))), innerW)

	headerRows := 2
	if chatName != "" {
		headerRows = 3
	}
	viewportH := innerH - headerRows - 2
	if viewportH < 1 {
		viewportH = 1
	}

	start, end, showTop, showBottom := artifactListWindow(len(m.app.Artifacts), m.app.ArtifactSelectedIndex, viewportH)
	var list []string
	if showTop {
		list = append(list, fitLine(lipgloss.NewStyle().Foreground(colDimGray).Render(fmt.Sprintf("  ↑ %d more", start)), innerW))
	}
	for index := start; index < end; index++ {
		artifact := m.app.Artifacts[index]
		cursor := "  "
		style := lipgloss.NewStyle()
		if index == m.app.ArtifactSelectedIndex {
			cursor = "› "
			style = style.Foreground(colCyan).Bold(true)
		}
		kind := "TRANSCRIPT"
		if artifact.Kind == ConversationRecording {
			kind = "RECORDING"
		}
		when := ""
		if !artifact.CreatedAt.IsZero() {
			when = artifact.CreatedAt.Format("2006-01-02 15:04") + " · "
		}
		source := "Teams event"
		if artifact.DirectLink {
			source = "direct link"
		}
		line := fmt.Sprintf("%s%s · %s%s · %s", cursor, kind, when, artifact.Title, source)
		list = append(list, style.Render(fitLine(line, innerW)))
	}
	if showBottom {
		list = append(list, fitLine(lipgloss.NewStyle().Foreground(colDimGray).Render(fmt.Sprintf("  ↓ %d more", len(m.app.Artifacts)-end)), innerW))
	}
	for len(list) < viewportH {
		list = append(list, "")
	}
	if len(list) > viewportH {
		list = list[:viewportH]
	}

	var lines []string
	lines = append(lines, title)
	if chatName != "" {
		lines = append(lines, chatName)
	}
	lines = append(lines, "")
	lines = append(lines, list...)
	lines = append(lines, "", footer)

	return lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(colGreen).
		Padding(1, 2).
		Width(w).Height(h).
		Render(fitBlock(strings.Join(lines, "\n"), innerW, innerH))
}

func artifactListWindow(count, selected, maxLines int) (start, end int, showTop, showBottom bool) {
	if maxLines < 1 {
		maxLines = 1
	}
	if count <= maxLines {
		return 0, count, false, false
	}
	itemLines := maxLines - 1
	if selected > 0 && selected < count-1 && maxLines >= 3 {
		itemLines = maxLines - 2
	}
	if itemLines < 1 {
		itemLines = 1
	}
	start = selected - itemLines/2
	if start < 0 {
		start = 0
	}
	end = start + itemLines
	if end > count {
		end = count
		start = end - itemLines
		if start < 0 {
			start = 0
		}
	}
	return start, end, start > 0, end < count
}
