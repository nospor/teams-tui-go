package main

import (
	"fmt"
	"strings"
	"time"
)

// ChatReadFilter controls which local/server read states appear in the chat list.
type ChatReadFilter string

const (
	ChatReadAll    ChatReadFilter = "all"
	ChatReadUnread ChatReadFilter = "unread"
	ChatReadRead   ChatReadFilter = "read"
)

// ChatListFilter is a local, zero-network filter applied to the visible sidebar.
// An empty ChatTypes map means all chat types are included.
type ChatListFilter struct {
	ReadState      ChatReadFilter
	ChatTypes      map[string]bool
	FavouritesOnly bool
	TodayOnly      bool
	WithinHours    int
}

func newChatListFilter() ChatListFilter {
	return ChatListFilter{
		ReadState: ChatReadAll,
		ChatTypes: make(map[string]bool),
	}
}

func cloneChatListFilter(filter ChatListFilter) ChatListFilter {
	clone := filter
	clone.ChatTypes = make(map[string]bool, len(filter.ChatTypes))
	for chatType, enabled := range filter.ChatTypes {
		clone.ChatTypes[chatType] = enabled
	}
	return clone
}

func chatFilterIsActive(filter ChatListFilter) bool {
	return (filter.ReadState != "" && filter.ReadState != ChatReadAll) ||
		filter.FavouritesOnly ||
		filter.TodayOnly ||
		filter.WithinHours > 0 ||
		len(filter.ChatTypes) > 0
}

func chatFilterSummary(filter ChatListFilter) string {
	var parts []string
	if filter.ReadState != "" && filter.ReadState != ChatReadAll {
		parts = append(parts, string(filter.ReadState))
	}
	for _, entry := range []struct {
		chatType string
		label    string
	}{
		{"oneOnOne", "1:1"},
		{"group", "group"},
		{"meeting", "meeting"},
	} {
		if filter.ChatTypes[entry.chatType] {
			parts = append(parts, entry.label)
		}
	}
	if filter.FavouritesOnly {
		parts = append(parts, "favourites")
	}
	if filter.TodayOnly {
		parts = append(parts, "today")
	}
	if filter.WithinHours > 0 {
		parts = append(parts, fmt.Sprintf("last %dh", filter.WithinHours))
	}
	if len(parts) == 0 {
		return "all"
	}
	return strings.Join(parts, " · ")
}

func chatHasActivityOn(chat Chat, day time.Time) bool {
	when := chatActivityTime(chat)
	if when.IsZero() {
		return false
	}
	when = when.In(day.Location())
	return when.Year() == day.Year() && when.YearDay() == day.YearDay()
}

func chatHasActivitySince(chat Chat, since time.Time) bool {
	when := chatActivityTime(chat)
	return !when.IsZero() && !when.Before(since)
}

func (m Model) chatMatchesFilter(chat Chat, filter ChatListFilter) bool {
	unread := m.isUnread(chat)
	switch filter.ReadState {
	case ChatReadUnread:
		if !unread {
			return false
		}
	case ChatReadRead:
		if unread {
			return false
		}
	}

	if filter.FavouritesOnly && !m.favourites[chat.ID] {
		return false
	}
	if len(filter.ChatTypes) > 0 && !filter.ChatTypes[chat.ChatType] {
		return false
	}
	if filter.TodayOnly && !chatHasActivityOn(chat, time.Now().Local()) {
		return false
	}
	if filter.WithinHours > 0 && !chatHasActivitySince(chat, time.Now().Add(-time.Duration(filter.WithinHours)*time.Hour)) {
		return false
	}
	return true
}
