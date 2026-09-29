package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type snoozeChoice struct {
	Binding key.Binding
	Label   string
	Until   func(Model, time.Time) time.Time
}

func parseLocalClock(value string, fallbackHour int) (int, int) {
	parsed, err := time.Parse("15:04", strings.TrimSpace(value))
	if err != nil {
		return fallbackHour, 0
	}
	return parsed.Hour(), parsed.Minute()
}

func nextLocalClock(now time.Time, clock string, fallbackHour int, forceTomorrow bool) time.Time {
	hour, minute := parseLocalClock(clock, fallbackHour)
	day := now
	if forceTomorrow {
		day = day.AddDate(0, 0, 1)
	}
	target := time.Date(day.Year(), day.Month(), day.Day(), hour, minute, 0, 0, now.Location())
	if !forceTomorrow && !target.After(now) {
		startHour, startMinute := parseLocalClock("07:00", 7)
		tomorrow := now.AddDate(0, 0, 1)
		target = time.Date(tomorrow.Year(), tomorrow.Month(), tomorrow.Day(), startHour, startMinute, 0, 0, now.Location())
	}
	return target
}

func (m Model) snoozeChoices() []snoozeChoice {
	k := m.app.Keys.Snooze
	all := []snoozeChoice{
		{Binding: k.Minutes10, Label: "10 minutes", Until: func(_ Model, now time.Time) time.Time { return now.Add(10 * time.Minute) }},
		{Binding: k.Hour, Label: "1 hour", Until: func(_ Model, now time.Time) time.Time { return now.Add(time.Hour) }},
		{Binding: k.Hours3, Label: "3 hours", Until: func(_ Model, now time.Time) time.Time { return now.Add(3 * time.Hour) }},
		{Binding: k.WorkdayEnd, Label: "End of workday", Until: func(m Model, now time.Time) time.Time {
			endHour, endMinute := parseLocalClock(m.app.WorkdayEnd, 18)
			endToday := time.Date(now.Year(), now.Month(), now.Day(), endHour, endMinute, 0, 0, now.Location())
			if !endToday.After(now) {
				return nextLocalClock(now, m.app.WorkdayStart, 7, true)
			}
			return nextLocalClock(now, m.app.WorkdayEnd, 18, false)
		}},
		{Binding: k.Tomorrow, Label: "Tomorrow morning", Until: func(m Model, now time.Time) time.Time {
			return nextLocalClock(now, m.app.WorkdayStart, 7, true)
		}},
		{Binding: k.NextWeek, Label: "Next week", Until: func(m Model, now time.Time) time.Time {
			return nextLocalClock(now.AddDate(0, 0, 6), m.app.WorkdayStart, 7, true)
		}},
		{Binding: k.Unsnooze, Label: "Unsnooze", Until: nil},
	}
	out := make([]snoozeChoice, 0, len(all))
	for _, choice := range all {
		if len(choice.Binding.Keys()) == 0 {
			continue
		}
		out = append(out, choice)
	}
	return out
}

func (m Model) chatSnoozed(chatID string, now time.Time) bool {
	until, ok := m.snoozed[chatID]
	return ok && until.After(now)
}

func (m Model) wakeChat(chatID string) bool {
	if _, ok := m.snoozed[chatID]; !ok {
		return false
	}
	delete(m.snoozed, chatID)
	_ = SaveSnoozedChats(m.snoozed)
	return true
}

func (m Model) pruneExpiredSnoozes(now time.Time) bool {
	changed := false
	for id, until := range m.snoozed {
		if !until.After(now) {
			delete(m.snoozed, id)
			changed = true
		}
	}
	if changed {
		_ = SaveSnoozedChats(m.snoozed)
	}
	return changed
}

func (m Model) nextVisibleChatID(except string) string {
	first := ""
	next := ""
	seen := false
	for _, chat := range m.app.Chats {
		if chat.ID == except {
			seen = true
			continue
		}
		if first == "" {
			first = chat.ID
		}
		if seen && next == "" {
			next = chat.ID
		}
	}
	if next != "" {
		return next
	}
	return first
}

func (m Model) selectChatByID(id string) (Model, bool) {
	for i, chat := range m.app.Chats {
		if chat.ID == id {
			m.app.SelectedIndex = i
			return m, true
		}
	}
	return m, false
}

func (m Model) requireSelectedChat() (*Chat, Model, bool) {
	if m.channelSelectedIndex >= 0 || m.app.GetSelectedChat() == nil {
		m.app.SetStatus("Select a chat first", 3*time.Second)
		return nil, m, false
	}
	return m.app.GetSelectedChat(), m, true
}

func (m Model) openSnoozePopup() Model {
	if _, updated, ok := m.requireSelectedChat(); !ok {
		return updated
	}
	m.app.SnoozePopupMode = true
	m.app.SnoozeSelectedIndex = 0
	return m
}

func (m Model) applySnooze(until time.Time) (Model, tea.Cmd) {
	chat, m, ok := m.requireSelectedChat()
	if !ok {
		m.app.SnoozePopupMode = false
		return m, nil
	}
	chatID := chat.ID
	nextID := m.nextVisibleChatID(chatID)
	if until.IsZero() {
		delete(m.snoozed, chatID)
		m.app.SetStatus("Unsnoozed "+chatExportTitle(*chat), 3*time.Second)
	} else {
		if m.snoozed == nil {
			m.snoozed = make(map[string]time.Time)
		}
		m.snoozed[chatID] = until
		m.app.SetStatus("Snoozed until "+until.Format("Mon 15:04"), 4*time.Second)
	}
	_ = SaveSnoozedChats(m.snoozed)
	m.app.SnoozePopupMode = false
	m = m.rebuildChatList()
	if updated, found := m.selectChatByID(chatID); found {
		return updated, nil
	}
	if nextID != "" {
		if updated, found := m.selectChatByID(nextID); found {
			return updated.loadChatMessages(nextID, updated.app.SelectedIndex)
		}
	}
	m.app.SelectedIndex = -1
	m.app.Messages = nil
	return m, nil
}

func (m Model) quickSnooze() (Model, tea.Cmd) {
	minutes := m.app.DefaultSnoozeMinutes
	if minutes <= 0 {
		minutes = 180
	}
	return m.applySnooze(time.Now().Add(time.Duration(minutes) * time.Minute))
}

func (m Model) handleSnoozePopupKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	choices := m.snoozeChoices()
	k := m.app.Keys.Snooze
	switch {
	case pressed(msg, k.Close), pressed(msg, m.app.Keys.Normal.SnoozeMenu):
		m.app.SnoozePopupMode = false
		return m, nil
	case pressed(msg, k.Next):
		if len(choices) == 0 {
			return m, nil
		}
		m.app.SnoozeSelectedIndex = (m.app.SnoozeSelectedIndex + 1) % len(choices)
		return m, nil
	case pressed(msg, k.Prev):
		if len(choices) == 0 {
			return m, nil
		}
		m.app.SnoozeSelectedIndex--
		if m.app.SnoozeSelectedIndex < 0 {
			m.app.SnoozeSelectedIndex = len(choices) - 1
		}
		return m, nil
	case pressed(msg, k.Confirm):
		index := m.app.SnoozeSelectedIndex
		if index < 0 || index >= len(choices) {
			return m, nil
		}
		choice := choices[index]
		if choice.Until == nil {
			return m.applySnooze(time.Time{})
		}
		return m.applySnooze(choice.Until(m, time.Now()))
	}

	for _, choice := range choices {
		if pressed(msg, choice.Binding) {
			if choice.Until == nil {
				return m.applySnooze(time.Time{})
			}
			return m.applySnooze(choice.Until(m, time.Now()))
		}
	}
	return m, nil
}

func (m Model) renderSnoozePopup(w, h int) string {
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

	k := m.app.Keys.Snooze
	chatName := "Selected chat"
	if chat := m.app.GetSelectedChat(); chat != nil {
		chatName = chatExportTitle(*chat)
	}
	choices := m.snoozeChoices()
	lines := []string{
		lipgloss.NewStyle().Foreground(colYellow).Bold(true).Render("Snooze chat"),
		lipgloss.NewStyle().Foreground(colDimGray).Render(chatName),
		"",
	}
	for index, choice := range choices {
		cursor := "  "
		style := lipgloss.NewStyle()
		if index == m.app.SnoozeSelectedIndex {
			cursor = "› "
			style = style.Foreground(colCyan).Bold(true)
		}
		keyLabel := FormatKeys(choice.Binding, "/")
		lines = append(lines, style.Render(fmt.Sprintf("%s%s  %s", cursor, keyLabel, choice.Label)))
	}
	if len(choices) == 0 {
		lines = append(lines, lipgloss.NewStyle().Foreground(colDimGray).Render("No durations bound"))
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
