package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type chatActionID string

const (
	chatActionCompose   chatActionID = "compose"
	chatActionFavourite chatActionID = "favourite"
	chatActionExport    chatActionID = "export"
)

type chatAction struct {
	Binding key.Binding
	Label   string
	ID      chatActionID
}

func (m Model) configuredChatActions() []chatAction {
	k := m.app.Keys.ChatActions
	all := []chatAction{
		{Binding: k.Compose, Label: "Compose message", ID: chatActionCompose},
		{Binding: k.Favourite, Label: "Toggle favourite", ID: chatActionFavourite},
		{Binding: k.Export, Label: "Export complete Markdown transcript", ID: chatActionExport},
	}
	out := make([]chatAction, 0, len(all))
	for _, action := range all {
		if len(action.Binding.Keys()) == 0 {
			continue
		}
		out = append(out, action)
	}
	return out
}

func (m Model) openChatActionPopup() Model {
	if m.channelSelectedIndex >= 0 || m.app.GetSelectedChat() == nil {
		m.app.SetStatus("Select a chat first", 3*time.Second)
		return m
	}
	m.app.ChatActionPopupMode = true
	m.app.ChatActionSelectedIndex = 0
	return m
}

func (m Model) startCompose() (Model, tea.Cmd) {
	if m.app.SelectedIndex < 0 && m.channelSelectedIndex < 0 {
		return m, nil
	}
	m.app.InputMode = true
	m.app.InputBuffer = ""
	m.textarea.Reset()
	return m, m.textarea.Focus()
}

func (m Model) toggleFavourite() Model {
	if m.channelSelectedIndex >= 0 {
		return m
	}
	chat := m.app.GetSelectedChat()
	if chat == nil {
		return m
	}
	name := ""
	if chat.CachedDisplayName != nil {
		name = *chat.CachedDisplayName
	}
	if m.favourites[chat.ID] {
		delete(m.favourites, chat.ID)
		m.app.SetStatus("★ Removed from favourites: "+name, 3*time.Second)
	} else {
		m.favourites[chat.ID] = true
		m.app.SetStatus("★ Added to favourites: "+name, 3*time.Second)
	}
	_ = SaveFavourites(m.favourites)
	m = m.rebuildChatList()
	for i, c := range m.app.Chats {
		if c.ID == chat.ID {
			m.app.SelectedIndex = i
			break
		}
	}
	return m
}

func (m Model) startChatExport() (Model, tea.Cmd) {
	chat := m.app.GetSelectedChat()
	if m.channelSelectedIndex >= 0 || chat == nil {
		m.app.SetStatus("Select a chat first", 3*time.Second)
		return m, nil
	}
	m.app.SetStatus("Exporting complete transcript…", 0)
	return m, exportChatMarkdownCmd(m.clientID, *chat, m.app.ExportDirectory)
}

func (m Model) executeChatAction(id chatActionID) (Model, tea.Cmd) {
	switch id {
	case chatActionCompose:
		return m.startCompose()
	case chatActionFavourite:
		return m.toggleFavourite(), nil
	case chatActionExport:
		return m.startChatExport()
	default:
		return m, nil
	}
}

func (m Model) handleChatActionPopupKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	actions := m.configuredChatActions()
	k := m.app.Keys.ChatActions
	switch {
	case pressed(msg, k.Close):
		m.app.ChatActionPopupMode = false
		return m, nil
	case pressed(msg, k.Next):
		if len(actions) == 0 {
			return m, nil
		}
		m.app.ChatActionSelectedIndex = (m.app.ChatActionSelectedIndex + 1) % len(actions)
		return m, nil
	case pressed(msg, k.Prev):
		if len(actions) == 0 {
			return m, nil
		}
		m.app.ChatActionSelectedIndex--
		if m.app.ChatActionSelectedIndex < 0 {
			m.app.ChatActionSelectedIndex = len(actions) - 1
		}
		return m, nil
	case pressed(msg, k.Confirm):
		if m.app.ChatActionSelectedIndex < 0 || m.app.ChatActionSelectedIndex >= len(actions) {
			return m, nil
		}
		m.app.ChatActionPopupMode = false
		return m.executeChatAction(actions[m.app.ChatActionSelectedIndex].ID)
	}

	for _, action := range actions {
		if pressed(msg, action.Binding) {
			m.app.ChatActionPopupMode = false
			return m.executeChatAction(action.ID)
		}
	}
	return m, nil
}

func (m Model) renderChatActionPopup(w, h int) string {
	if w < 48 {
		w = 48
	}
	chatName := "Selected chat"
	if chat := m.app.GetSelectedChat(); chat != nil {
		chatName = chatExportTitle(*chat)
	}
	k := m.app.Keys.ChatActions
	lines := []string{
		lipgloss.NewStyle().Foreground(colYellow).Bold(true).Render("Chat actions"),
		lipgloss.NewStyle().Foreground(colDimGray).Render(chatName),
		"",
	}
	actions := m.configuredChatActions()
	for index, action := range actions {
		cursor := "  "
		style := lipgloss.NewStyle()
		if index == m.app.ChatActionSelectedIndex {
			cursor = "› "
			style = style.Foreground(colCyan).Bold(true)
		}
		keyLabel := FormatKeys(action.Binding, "/")
		lines = append(lines, style.Render(fmt.Sprintf("%s%s  %s", cursor, keyLabel, action.Label)))
	}
	if len(actions) == 0 {
		lines = append(lines, lipgloss.NewStyle().Foreground(colDimGray).Render("No actions bound"))
	}
	lines = append(lines, "", lipgloss.NewStyle().Foreground(colDimGray).Render(
		fmt.Sprintf("%s run · %s cancel", FormatKeys(k.Confirm, "/"), FormatKeys(k.Close, "/"))))
	content := strings.Join(lines, "\n")
	return lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(colGreen).
		Padding(1, 2).
		Width(w - 2).
		MaxHeight(h).
		Render(content)
}
