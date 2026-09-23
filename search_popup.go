package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type UserSearchItemType int

const (
	UserSearchItemLocal UserSearchItemType = iota
	UserSearchItemParticipant
	UserSearchItemMessage
	UserSearchItemDirectory
	UserSearchItemDirect
	UserSearchItemChannel
)

type UserSearchItem struct {
	Type        UserSearchItemType
	LocalChat   *Chat
	Message     *MessageSearchResult
	DirUser     *User
	DirectEmail string
	Channel     *channelEntry
}

func (m Model) ensureSearchChatInventory() (Model, tea.Cmd) {
	if m.searchChatInventoryLoaded || m.searchChatInventoryLoading {
		return m, nil
	}
	m.searchChatInventoryLoading = true
	return m, loadSearchChatInventoryCmd(m.clientID, m.app.CurrentUserName)
}

func userSearchItemKey(item UserSearchItem) string {
	switch item.Type {
	case UserSearchItemLocal, UserSearchItemParticipant:
		if item.LocalChat != nil {
			return "chat:" + item.LocalChat.ID
		}
	case UserSearchItemMessage:
		if item.Message != nil {
			return "msg:" + item.Message.ChatID + ":" + item.Message.Message.ID
		}
	case UserSearchItemChannel:
		if item.Channel != nil {
			return "chan:" + item.Channel.teamID + ":" + item.Channel.channelID
		}
	}
	return ""
}

func (m Model) selectedUserSearchItemKey() string {
	items := m.getUserSearchItems()
	if m.app.UserSearchSelectedIndex < 0 || m.app.UserSearchSelectedIndex >= len(items) {
		return ""
	}
	return userSearchItemKey(items[m.app.UserSearchSelectedIndex])
}

func (m *Model) restoreUserSearchSelection(key string) {
	items := m.getUserSearchItems()
	if key != "" {
		for index, item := range items {
			if userSearchItemKey(item) == key {
				m.app.UserSearchSelectedIndex = index
				return
			}
		}
	}
	if m.app.UserSearchSelectedIndex >= len(items) {
		m.app.UserSearchSelectedIndex = len(items) - 1
	}
	if m.app.UserSearchSelectedIndex < 0 {
		m.app.UserSearchSelectedIndex = 0
	}
}

func (m Model) getUserSearchItems() []UserSearchItem {
	var items []UserSearchItem
	for i := range m.app.UserSearchLocalResults {
		items = append(items, UserSearchItem{
			Type:      UserSearchItemLocal,
			LocalChat: &m.app.UserSearchLocalResults[i],
		})
	}
	for i := range m.app.UserSearchMemberResults {
		items = append(items, UserSearchItem{
			Type:      UserSearchItemParticipant,
			LocalChat: &m.app.UserSearchMemberResults[i],
		})
	}
	for i := range m.app.UserSearchMessageResults {
		items = append(items, UserSearchItem{
			Type:    UserSearchItemMessage,
			Message: &m.app.UserSearchMessageResults[i],
		})
	}
	for i := range m.app.UserSearchChannelResults {
		items = append(items, UserSearchItem{
			Type:    UserSearchItemChannel,
			Channel: &m.app.UserSearchChannelResults[i],
		})
	}
	return items
}

func (m Model) knownChatsForSearch() []Chat {
	known := make(map[string]Chat)
	for _, chat := range m.searchChatInventory {
		known[chat.ID] = chat
	}
	if m.app != nil {
		for _, chat := range m.app.Chats {
			known[chat.ID] = chat
		}
	}
	for _, chat := range m.latestChats {
		known[chat.ID] = chat
	}

	ordered := make([]Chat, 0, len(known))
	used := make(map[string]bool)
	for _, id := range m.stableChatOrder {
		if chat, ok := known[id]; ok && !used[id] {
			ordered = append(ordered, chat)
			used[id] = true
		}
	}
	for _, chat := range m.searchChatInventory {
		if knownChat, ok := known[chat.ID]; ok && !used[chat.ID] {
			ordered = append(ordered, knownChat)
			used[chat.ID] = true
		}
	}
	var rest []Chat
	for id, chat := range known {
		if !used[id] {
			rest = append(rest, chat)
		}
	}
	sort.Slice(rest, func(i, j int) bool {
		return strings.ToLower(chatDisplayName(rest[i])) < strings.ToLower(chatDisplayName(rest[j]))
	})
	return append(ordered, rest...)
}

func chatDisplayName(chat Chat) string {
	if chat.CachedDisplayName != nil && strings.TrimSpace(*chat.CachedDisplayName) != "" {
		return *chat.CachedDisplayName
	}
	if chat.Topic != nil && strings.TrimSpace(*chat.Topic) != "" {
		return *chat.Topic
	}
	return chat.ID
}

func (m Model) knownMessagesForSearch(chat Chat) []Message {
	seen := make(map[string]bool)
	var messages []Message
	add := func(candidates []Message) {
		for _, message := range candidates {
			key := message.ID
			if key == "" {
				key = message.CreatedDateTime + "\x00" + message.SenderName() + "\x00" + message.GetPlainText()
			}
			if !seen[key] {
				seen[key] = true
				messages = append(messages, message)
			}
		}
	}
	if m.app != nil {
		add(m.app.CachedMessages[chat.ID])
		add(m.app.HistoryMessages[chat.ID])
		if sel := m.app.GetSelectedChat(); sel != nil && sel.ID == chat.ID {
			add(m.app.Messages)
		}
	}
	if chat.LastMessagePreview != nil {
		add([]Message{*chat.LastMessagePreview})
	}
	return messages
}

func (m *Model) updateUserSearchLocalResults() {
	parsed := parseSearchQuery(m.app.UserSearchQuery)
	if len(parsed.Terms) == 0 {
		m.app.UserSearchLocalResults = nil
		m.app.UserSearchMemberResults = nil
		m.app.UserSearchMessageResults = nil
		m.app.UserSearchChannelResults = nil
		return
	}

	type scoredChat struct {
		chat  Chat
		score int
	}
	var nameMatches []scoredChat
	var participantMatches []scoredChat
	var messageMatches []MessageSearchResult
	for _, chat := range m.knownChatsForSearch() {
		unread := m.isUnread(chat)
		favorite := m.favourites[chat.ID]
		if score, matched := parsed.Match(chatNameSearchTarget(chat, unread, favorite)); matched {
			nameMatches = append(nameMatches, scoredChat{chat: chat, score: score})
		} else if score, matched := parsed.Match(chatParticipantSearchTarget(chat, unread, favorite)); matched {
			participantMatches = append(participantMatches, scoredChat{chat: chat, score: score})
		}
		for _, message := range m.knownMessagesForSearch(chat) {
			if score, matched := parsed.Match(messageSearchTarget(&message, chat, unread, favorite)); matched {
				messageMatches = append(messageMatches, MessageSearchResult{
					ChatID:   chat.ID,
					ChatName: chatDisplayName(chat),
					Message:  message,
					Score:    score,
				})
			}
		}
	}
	sort.SliceStable(nameMatches, func(i, j int) bool { return nameMatches[i].score > nameMatches[j].score })
	sort.SliceStable(participantMatches, func(i, j int) bool { return participantMatches[i].score > participantMatches[j].score })
	m.app.UserSearchLocalResults = nil
	for index, match := range nameMatches {
		if index >= 20 {
			break
		}
		m.app.UserSearchLocalResults = append(m.app.UserSearchLocalResults, match.chat)
	}
	m.app.UserSearchMemberResults = nil
	for index, match := range participantMatches {
		if index >= 20 {
			break
		}
		m.app.UserSearchMemberResults = append(m.app.UserSearchMemberResults, match.chat)
	}
	sortMessageSearchResults(messageMatches)
	if len(messageMatches) > 40 {
		messageMatches = messageMatches[:40]
	}
	m.app.UserSearchMessageResults = messageMatches

	var chanMatches []channelEntry
	if m.app.Features.TeamsChannels {
		for _, ch := range m.allChannels() {
			target := searchTarget{
				Text:         []string{ch.channelName, ch.teamName},
				Conversation: []string{ch.channelName, ch.teamName},
				Kind:         "channel",
			}
			if _, matched := parsed.Match(target); matched {
				chanMatches = append(chanMatches, ch)
			}
		}
	}
	m.app.UserSearchChannelResults = chanMatches
}

func (m Model) handleUserSearchInputModeKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	k := m.app.Keys.ChatSearchInput
	switch {
	case pressed(msg, k.Cancel):
		m.app.UserSearchMode = false
		m.userSearchInput.Blur()
		return m, nil

	case pressed(msg, k.FocusResults):
		m.app.UserSearchMode = false
		m.userSearchInput.Blur()
		m.app.UserSearchSelectedIndex = 0
		return m, nil

	case pressed(msg, k.Submit):
		query := strings.TrimSpace(m.userSearchInput.Value())
		m.app.UserSearchQuery = query
		m.updateUserSearchLocalResults()
		if len(m.getUserSearchItems()) > 0 {
			m.app.UserSearchMode = false
			m.userSearchInput.Blur()
			m.app.UserSearchSelectedIndex = 0
			return m.handleUserSearchNavigationKey(tea.KeyMsg{Type: tea.KeyEnter})
		}
		if strings.Contains(query, "@") {
			m.app.UserSearchMode = false
			m.userSearchInput.Blur()
			m.app.UserSearchLoading = true
			m.app.UserSearchStatus = "Opening chat..."
			return m, createChatCmd(m.clientID, m.userID, query)
		}
		m.app.UserSearchStatus = "No matching local chat. Keep typing or enter an exact email."
		return m, nil
	}

	return m, nil
}

func (m Model) revealChatFromSearch(chatID string) (Model, bool) {
	for i, chat := range m.app.Chats {
		if chat.ID == chatID {
			m.app.SelectedIndex = i
			m.channelSelectedIndex = -1
			m.app.SelectedChannelTeamID = ""
			m.app.SelectedChannelID = ""
			return m, true
		}
	}
	chat := m.chatForSearch(chatID)
	if chat.ID != chatID {
		return m, false
	}
	existsInLatest := false
	for _, existing := range m.latestChats {
		if existing.ID == chat.ID {
			existsInLatest = true
			break
		}
	}
	if !existsInLatest {
		m.latestChats = append([]Chat{chat}, m.latestChats...)
	}
	m.promoteChat(chat.ID)
	m = m.mergeChats(m.latestChats)
	for i, existing := range m.app.Chats {
		if existing.ID == chatID {
			m.app.SelectedIndex = i
			m.channelSelectedIndex = -1
			m.app.SelectedChannelTeamID = ""
			m.app.SelectedChannelID = ""
			return m, true
		}
	}
	return m, false
}

func (m Model) closeUserSearch() Model {
	m.channelSelectedIndex = -1
	m.app.SelectedChannelTeamID = ""
	m.app.SelectedChannelID = ""
	m.app.UserSearchPopupMode = false
	m.app.UserSearchMode = false
	m.app.SnapToBottom = true
	return m
}

func (m Model) handleUserSearchNavigationKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	items := m.getUserSearchItems()
	k := m.app.Keys.ChatSearchResults

	switch {
	case pressed(msg, k.Close):
		m.app.UserSearchPopupMode = false
		m.app.UserSearchMode = false
		return m, nil

	case pressed(msg, k.Next):
		if len(items) > 0 && m.app.UserSearchSelectedIndex < len(items)-1 {
			m.app.UserSearchSelectedIndex++
		}
		return m, nil

	case pressed(msg, k.Prev):
		if len(items) > 0 && m.app.UserSearchSelectedIndex > 0 {
			m.app.UserSearchSelectedIndex--
		}
		return m, nil

	case pressed(msg, k.EditQuery):
		m.app.UserSearchMode = true
		m.userSearchInput.Focus()
		return m, textinput.Blink

	case pressed(msg, k.Open):
		if len(items) == 0 || m.app.UserSearchSelectedIndex >= len(items) {
			return m, nil
		}

		item := items[m.app.UserSearchSelectedIndex]
		switch item.Type {
		case UserSearchItemLocal, UserSearchItemParticipant:
			targetID := item.LocalChat.ID
			var selected bool
			m, selected = m.revealChatFromSearch(targetID)
			if selected {
				m = m.closeUserSearch()
				return m.loadChatMessages(targetID, m.app.SelectedIndex)
			}
		case UserSearchItemMessage:
			result := *item.Message
			var selected bool
			m, selected = m.revealChatFromSearch(result.ChatID)
			if !selected {
				return m, nil
			}
			chat := m.chatForSearch(result.ChatID)
			knownMessages := m.knownMessagesForSearch(chat)
			m = m.closeUserSearch()
			m, loadCmd := m.loadChatMessages(result.ChatID, m.app.SelectedIndex)
			m.app.Messages = mergeHistoryMessages(m.app.Messages, knownMessages)
			m.app.CachedMessages[result.ChatID] = mergeHistoryMessages(m.app.CachedMessages[result.ChatID], knownMessages)
			for index := range m.app.Messages {
				if m.app.Messages[index].ID == result.Message.ID {
					m.app.MessageSelectionMode = true
					m.app.MessageSelectedIndex = index
					m.app.PendingScrollID = result.Message.ID
					m.app.SnapToBottom = false
					break
				}
			}
			return m, loadCmd
		case UserSearchItemChannel:
			chans := m.allChannels()
			idx := -1
			for i, ch := range chans {
				if ch.channelID == item.Channel.channelID && ch.teamID == item.Channel.teamID {
					idx = i
					break
				}
			}
			if idx != -1 {
				m.channelSelectedIndex = idx
				m.app.SelectedChannelTeamID = item.Channel.teamID
				m.app.SelectedChannelID = item.Channel.channelID
				m.app.UserSearchPopupMode = false
				m.app.UserSearchMode = false
				return m.loadChannelMessages(item.Channel.teamID, item.Channel.channelID)
			}
		}
	}

	return m, nil
}

func (m Model) renderUserSearchPopup(w, h int) string {
	titleStyle := lipgloss.NewStyle().Foreground(colCyan).Bold(true)
	title := titleStyle.Render("Search Chats and Loaded Messages")

	ck := m.app.Keys.ChatSearchResults
	ci := m.app.Keys.ChatSearchInput
	openHint := FormatKeys(ck.Open, "/")
	if submit := FormatKeys(ci.Submit, "/"); submit != openHint {
		openHint += "/" + submit
	}
	instructions := lipgloss.NewStyle().Foreground(colDimGray).Render(fmt.Sprintf(
		" %s: Nav | %s: Open result or typed email | %s: Edit | %s: Close",
		slashKeys(ck.Next, ck.Prev),
		openHint,
		FormatKeys(ck.EditQuery, "/"),
		FormatKeys(ck.Close, "/"),
	))

	var list strings.Builder
	list.WriteString(title + "\n")
	list.WriteString(instructions + "\n\n")

	items := m.getUserSearchItems()
	msgH := h - 10
	if msgH < 3 {
		msgH = 3
	}

	if len(items) == 0 {
		if m.app.UserSearchQuery == "" {
			list.WriteString(lipgloss.NewStyle().Foreground(colDimGray).Render("Type literal/regexp components or an exact email.") + "\n")
		} else {
			list.WriteString(lipgloss.NewStyle().Foreground(colDimGray).Render("No matching chats, loaded messages, or channels found.") + "\n")
		}
		for l := 1; l < msgH; l++ {
			list.WriteString("\n")
		}
	} else {
		if m.app.UserSearchSelectedIndex >= len(items) {
			m.app.UserSearchSelectedIndex = len(items) - 1
		}
		if m.app.UserSearchSelectedIndex < 0 {
			m.app.UserSearchSelectedIndex = 0
		}

		sectionName := func(item UserSearchItem) string {
			switch item.Type {
			case UserSearchItemLocal:
				return "Chats by name"
			case UserSearchItemParticipant:
				return "Chats by participant"
			case UserSearchItemMessage:
				return "Loaded message matches"
			case UserSearchItemChannel:
				return "Channels"
			default:
				return "Results"
			}
		}
		startItem := 0
		if m.app.UserSearchSelectedIndex >= msgH-2 {
			startItem = m.app.UserSearchSelectedIndex - msgH + 3
		}
		linesRendered := 0
		lastSection := ""
		for idx := startItem; idx < len(items); idx++ {
			item := items[idx]
			section := sectionName(item)
			if section != lastSection {
				if linesRendered >= msgH {
					break
				}
				list.WriteString(lipgloss.NewStyle().Foreground(colCyan).Bold(true).Render(section) + "\n")
				linesRendered++
				lastSection = section
			}
			if linesRendered >= msgH {
				break
			}
			isSelected := idx == m.app.UserSearchSelectedIndex
			prefix := "  "
			if isSelected {
				prefix = "> "
			}

			var line string
			switch item.Type {
			case UserSearchItemLocal, UserSearchItemParticipant:
				chatName := chatDisplayName(*item.LocalChat)
				tagText := "[Name]"
				if item.Type == UserSearchItemParticipant {
					tagText = "[Participant]"
				}
				tag := lipgloss.NewStyle().Foreground(colGreen).Render(tagText)
				lineStr := fmt.Sprintf("%s %s %s", prefix, chatName, tag)
				if isSelected {
					line = lipgloss.NewStyle().Background(colDarkGray).Foreground(colWhite).Bold(true).Render(lineStr)
				} else {
					line = lineStr
				}
			case UserSearchItemMessage:
				sender := item.Message.Message.SenderName()
				if sender == "" {
					sender = "Teams"
				}
				snippet := strings.Join(strings.Fields(item.Message.Message.GetPlainText()), " ")
				available := w - lipgloss.Width(prefix) - lipgloss.Width(item.Message.ChatName) - lipgloss.Width(sender) - 18
				if available < 12 {
					available = 12
				}
				snippet = truncate(snippet, available)
				tag := lipgloss.NewStyle().Foreground(colYellow).Render("[Message]")
				lineStr := fmt.Sprintf("%s%s · %s: %s %s", prefix, item.Message.ChatName, sender, snippet, tag)
				if isSelected {
					line = lipgloss.NewStyle().Background(colDarkGray).Foreground(colWhite).Bold(true).Render(lineStr)
				} else {
					line = lineStr
				}
			case UserSearchItemChannel:
				chanName := item.Channel.channelName
				teamName := item.Channel.teamName
				tag := lipgloss.NewStyle().Foreground(colCyan).Render("[Channel]")
				lineStr := fmt.Sprintf("%s %s > %s %s", prefix, teamName, chanName, tag)
				if isSelected {
					line = lipgloss.NewStyle().Background(colDarkGray).Foreground(colWhite).Bold(true).Render(lineStr)
				} else {
					line = lineStr
				}
			}

			list.WriteString(line + "\n")
			linesRendered++
			if linesRendered >= msgH {
				break
			}
		}

		for l := linesRendered; l < msgH; l++ {
			list.WriteString("\n")
		}
	}

	m.userSearchInput.Width = w - 10
	tiView := m.userSearchInput.View()

	borderCol := colCyan
	if !m.app.UserSearchMode {
		borderCol = colDimGray
	}

	inputBox := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(borderCol).
		Width(w - 6).Height(3).
		Render(lipgloss.JoinHorizontal(lipgloss.Left,
			lipgloss.NewStyle().Foreground(borderCol).Bold(true).Render("🔍 "),
			tiView,
		))

	statusText := ""
	if m.app.UserSearchStatus != "" {
		statusText = "  " + lipgloss.NewStyle().Foreground(colYellow).Italic(true).Render(m.app.UserSearchStatus)
	} else if m.app.UserSearchLoading {
		statusText = "  " + lipgloss.NewStyle().Foreground(colYellow).Italic(true).Render("⏳ Opening chat...")
	} else if m.searchChatInventoryLoading {
		statusText = "  " + lipgloss.NewStyle().Foreground(colYellow).Italic(true).Render("Loading complete chat inventory...")
	}

	list.WriteString(statusText + "\n" + inputBox)

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colCyan).
		Padding(1, 2).
		Width(w).Height(h).
		Render(list.String())
}
