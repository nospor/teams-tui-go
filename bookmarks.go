package main

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type chatBookmarkPreset struct {
	Key    string
	Name   string
	Filter ChatListFilter
}

func builtinChatBookmarkPresets() []chatBookmarkPreset {
	preset := func(key, name string, configure func(*ChatListFilter)) chatBookmarkPreset {
		filter := newChatListFilter()
		if configure != nil {
			configure(&filter)
		}
		return chatBookmarkPreset{Key: key, Name: name, Filter: filter}
	}

	return []chatBookmarkPreset{
		preset("a", "All", nil),
		preset("u", "Unread", func(filter *ChatListFilter) { filter.ReadState = ChatReadUnread }),
		preset("r", "Read", func(filter *ChatListFilter) { filter.ReadState = ChatReadRead }),
		preset("t", "Today", func(filter *ChatListFilter) { filter.TodayOnly = true }),
		preset("2", "Last 24 hours", func(filter *ChatListFilter) { filter.WithinHours = 24 }),
		preset("w", "Last 7 days", func(filter *ChatListFilter) { filter.WithinHours = 7 * 24 }),
		preset("z", "Snoozed", func(filter *ChatListFilter) { filter.SnoozedOnly = true }),
		preset("f", "Favourites", func(filter *ChatListFilter) { filter.FavouritesOnly = true }),
		preset("d", "Direct (1:1)", func(filter *ChatListFilter) { filter.ChatTypes["oneOnOne"] = true }),
		preset("g", "Groups", func(filter *ChatListFilter) { filter.ChatTypes["group"] = true }),
		preset("m", "Meetings", func(filter *ChatListFilter) { filter.ChatTypes["meeting"] = true }),
	}
}

func (m Model) openChatBookmarkPopup() Model {
	m.app.ChatBookmarkPopupMode = true
	m.app.ChatBookmarkSelectedIndex = 0
	return m
}

func (m Model) applyChatBookmark(preset chatBookmarkPreset) (Model, tea.Cmd) {
	prevID := ""
	if m.channelSelectedIndex < 0 {
		if chat := m.app.GetSelectedChat(); chat != nil {
			prevID = chat.ID
		}
	}

	m.app.ChatBookmarkPopupMode = false
	m.app.ActiveChatFilter = cloneChatListFilter(preset.Filter)
	m.app.ActiveChatBookmark = preset.Name
	m.app.ChatScrollOffset = 0
	m = m.rebuildChatList()
	m.app.SetStatus(fmt.Sprintf("Bookmark %s: %d shown", preset.Name, len(m.app.Chats)), 4*time.Second)

	if m.channelSelectedIndex >= 0 {
		return m, nil
	}
	if chat := m.app.GetSelectedChat(); chat != nil && chat.ID != prevID {
		return m.loadChatMessages(chat.ID, m.app.SelectedIndex)
	}
	if m.app.GetSelectedChat() == nil && len(m.app.Chats) == 0 {
		m.app.Messages = nil
	}
	return m, nil
}

func (m Model) handleChatBookmarkPopupKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	presets := builtinChatBookmarkPresets()
	k := m.app.Keys.Bookmarks
	switch {
	case pressed(msg, k.Close), pressed(msg, m.app.Keys.Normal.Bookmarks):
		m.app.ChatBookmarkPopupMode = false
		return m, nil
	case pressed(msg, k.Next):
		if len(presets) == 0 {
			return m, nil
		}
		m.app.ChatBookmarkSelectedIndex = (m.app.ChatBookmarkSelectedIndex + 1) % len(presets)
		return m, nil
	case pressed(msg, k.Prev):
		if len(presets) == 0 {
			return m, nil
		}
		m.app.ChatBookmarkSelectedIndex--
		if m.app.ChatBookmarkSelectedIndex < 0 {
			m.app.ChatBookmarkSelectedIndex = len(presets) - 1
		}
		return m, nil
	case pressed(msg, k.Confirm):
		index := m.app.ChatBookmarkSelectedIndex
		if index >= 0 && index < len(presets) {
			return m.applyChatBookmark(presets[index])
		}
		return m, nil
	}

	for _, preset := range presets {
		if pressed(msg, bind(preset.Key)) {
			return m.applyChatBookmark(preset)
		}
	}
	return m, nil
}

func (m Model) renderChatBookmarkPopup(w, h int) string {
	if w < 42 {
		w = 42
	}
	innerW := w - 6
	if innerW < 1 {
		innerW = 1
	}
	innerH := h - 2
	if innerH < 1 {
		innerH = 1
	}

	presets := builtinChatBookmarkPresets()
	k := m.app.Keys.Bookmarks
	lines := []string{
		lipgloss.NewStyle().Foreground(colYellow).Bold(true).Render("Chat bookmarks"),
		lipgloss.NewStyle().Foreground(colDimGray).Render("Filter the sidebar. Full chat set stays loaded."),
		"",
	}
	for index, preset := range presets {
		cursor := "  "
		style := lipgloss.NewStyle()
		if index == m.app.ChatBookmarkSelectedIndex {
			cursor = "› "
			style = style.Foreground(colCyan).Bold(true)
		}
		lines = append(lines, style.Render(fmt.Sprintf("%s%s  %s", cursor, preset.Key, preset.Name)))
	}
	lines = append(lines, "", lipgloss.NewStyle().Foreground(colDimGray).Render(
		fmt.Sprintf("%s apply · %s cancel", FormatKeys(k.Confirm, "/"), FormatKeys(k.Close, "/"))))

	return lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(colGreen).
		Padding(1, 2).
		Width(w - 2).
		Height(h).
		Render(fitBlock(strings.Join(lines, "\n"), innerW, innerH))
}
