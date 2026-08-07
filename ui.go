package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/atotto/clipboard"
	"github.com/nospor/teams-tui-go/filepicker"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/gen2brain/beeep"
	"regexp"
)

// ---------------------------------------------------------------------------
// Lipgloss colour palette
// ---------------------------------------------------------------------------

var (
	colCyan     = lipgloss.Color("#00D7D7")
	colYellow   = lipgloss.Color("#FFD700")
	colGreen    = lipgloss.Color("#00D75F")
	colDarkGray = lipgloss.Color("#303030")
	colWhite    = lipgloss.Color("#FFFFFF")
	colRed      = lipgloss.Color("#FF4444")
	colDimGray  = lipgloss.Color("#888888")
)

// Panel border styles.
var (
	normalBorder = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(colGreen)

	bellBorder = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(colRed).
			Foreground(colRed).
			Bold(true).
			Background(colWhite)
)

// ---------------------------------------------------------------------------
// Bubble Tea messages (async events → model)
// ---------------------------------------------------------------------------

// MsgChatsLoaded is sent when the background chat refresh completes.
type MsgChatsLoaded struct {
	Chats           []Chat
	CurrentUserName *string
}

// MsgFileAttached is sent when a file has been read from disk.
type MsgFileAttached struct {
	Name        string
	ContentType string
	Data        []byte
	Err         error
}

// MsgMessagesLoaded is sent when messages for a specific chat have loaded.
type MsgMessagesLoaded struct {
	ChatIndex int
	Messages  []Message
	NextLink  string
}

// MsgMoreMessagesLoaded is sent when older messages are loaded via pagination.
type MsgMoreMessagesLoaded struct {
	ConversationID string
	Messages       []Message
	NextLink       string
	IsSearch       bool
}

// MsgTick is the heartbeat used for periodic refresh and bell timeout.
type MsgTick struct{}

// MsgSendDone signals that a message send attempt has completed.
type MsgSendDone struct{ Err error }

// MsgEditDone signals that a message edit (PATCH) has completed.
type MsgEditDone struct {
	ChatID    string
	MessageID string
	Content   string // the markdown content the user typed (used to derive HTML)
	Err       error
}

// MsgUserSearchDone is sent when the directory search completes.
type MsgUserSearchDone struct {
	Users []User
	Err   error
}

// MsgHistoryLoaded is sent when the SQLite message history load completes.
type MsgHistoryLoaded struct {
	ConversationID string
	Messages       []Message
	NextLink       string
}

// MsgCreateChatDone is sent when a chat creation/retrieval completes.
type MsgCreateChatDone struct {
	Chat *Chat
	Err  error
}

// MsgBackgroundMessagesLoaded is sent when messages are fetched in the background to inspect reactions.
type MsgBackgroundMessagesLoaded struct {
	ChatID   string
	Messages []Message
}

// MsgPollReactionsLoaded is returned when messages for active chats are fetched to inspect reactions.
type MsgPollReactionsLoaded struct {
	Results map[string][]Message
}


// MsgPresenceLoaded is sent when a user's presence status has been fetched.
type MsgPresenceLoaded struct {
	UserID   string
	Presence *UserPresence
	Err      error
}

// MsgChatPresenceLoaded is sent when multiple users' presence statuses have been fetched.
type MsgChatPresenceLoaded struct {
	Presences map[string]UserPresence
	Err       error
}

// MsgUserProfileLoaded is sent when a user's profile has been fetched.
type MsgUserProfileLoaded struct {
	UserID  string
	Profile *UserProfile
	Err     error
}

// MsgFileDownloaded is sent when a file download has completed.
type MsgFileDownloaded struct {
	DestPath string
	Err      error
}

// MsgImagesOpened is sent when a batch image download+open has completed.
type MsgImagesOpened struct {
	SelectedPath string
	TotalImages  int
	Err          error
}

// MsgPreviewFinished is sent when a terminal image preview has finished.
type MsgPreviewFinished struct {
	Err error
}

// MsgTeamsChannelsLoaded is sent when the joined teams and their channels have been fetched.
type MsgTeamsChannelsLoaded struct {
	Teams []TeamWithChannels
	Err   error
}

// MsgTeamMembersLoaded is sent when team members have been fetched in the background.
type MsgTeamMembersLoaded struct {
	TeamID  string
	Members []ChatMember
	Err     error
}

// MsgChannelMessagesLoaded is sent when messages for a Teams channel have been fetched.
type MsgChannelMessagesLoaded struct {
	TeamID    string
	ChannelID string
	Messages  []Message
	NextLink  string
	Err       error
}

// MsgEditorFinished is sent when the external editor exits.
type MsgEditorFinished struct {
	Content  string
	ReadOnly bool
	Err      error
}

// MsgURLOpened is sent when a URL-opening command finishes.
type MsgURLOpened struct {
	Err error
}

// ---------------------------------------------------------------------------
// Model — the Bubble Tea application model
// ---------------------------------------------------------------------------

// Model is the central Bubble Tea model. It embeds App state and adds
// all TUI-specific fields.
type Model struct {
	app      *App
	clientID string
	userID   string // authenticated user ID for markChatRead

	// Layout dimensions (set on WindowSizeMsg).
	width  int
	height int

	// Textarea for composing messages.
	textarea textarea.Model

	// Textinput for searching.
	searchInput textinput.Model

	// Textinput for searching users.
	userSearchInput textinput.Model

	// Stable ordering of chat IDs.
	stableChatOrder []string

	// Index in stableChatOrder to start the next reaction poll from.
	nextReactionPollIndex int

	// Track last-seen message IDs and timestamps per chat.
	lastMsgID   map[string]string
	lastMsgTime map[string]time.Time

	// Track latest chats from the API (before applying stable order).
	latestChats []Chat

	// Timer tracking for periodic refreshes.
	lastChatRefresh    time.Time
	lastMessageRefresh time.Time

	// Which chat index was selected when we last issued a message load.
	lastRefreshIndex int

	// Track last-read message IDs per chat to avoid redundant API calls.
	lastReadMsgID map[string]string

	// Track whether a chat list load is in progress.
	loadingChats bool

	// Track last-read reaction keys per chat.
	lastReadReactions map[string]map[string]bool

	// Track whether reactions have been initialized/seen for each chat.
	reactionsInitialized map[string]bool

	// Track which reactions have already triggered a notification.
	notifiedReactions map[string]map[string]bool

	// Timer tracking for reaction polling.
	lastReactionPoll time.Time

	// Track application focus.
	focused bool

	// pendingEdits holds optimistic in-memory edits (messageID → HTML content)
	// that have been PATCHed to the API but may not yet be reflected in GET
	// responses (Graph API has eventual consistency after PATCH). Entries are
	// cleared once the API echoes back the updated content.
	pendingEdits map[string]string

	// favourites holds chat IDs that have been starred by the user.
	// Loaded from and persisted to favourites.json in the app config dir.
	favourites map[string]bool

	// unhiddenChannels holds channel IDs that are unhidden by the user.
	// Loaded from and persisted to unhidden_channels.json in the app config dir.
	unhiddenChannels map[string]bool

	// lastChannelRefresh tracks the last background refresh for channels.
	lastChannelRefresh time.Time

	// originalTeamIndex maps team ID to its original index in TeamsData when loaded.
	originalTeamIndex map[string]int

	// originalChannelIndex maps channel ID to its original index in its team's channel list when loaded.
	originalChannelIndex map[string]int


	// Channel sidebar navigation (used when teams_channels_enabled).
	// channelSelectedIndex is an index into the flat list returned by allChannels().
	// -1 means focus is in the chat list (default).
	channelSelectedIndex int

	// File picker for browsing/attaching files from computer.
	filepicker filepicker.Model

	lastWrittenMessages  int
	lastWrittenReactions int

	// tickDidWork is set by the MsgTick handler when that tick actually
	// changed something visible. Update consults it to decide whether the
	// memoized view can be reused. Reset after every Update.
	tickDidWork bool

	// viewCache holds the last string produced by View(). Bubble Tea calls
	// View() after every message, including the 100ms heartbeat tick, which
	// would otherwise re-render the whole UI ten times a second while idle.
	// The cache is shared via pointer because View() has a value receiver.
	viewCache *viewCache
}

// viewCache memoizes the rendered UI between Update calls that cannot have
// changed it. Update marks the cache dirty for every message except a tick
// that did no work, so anything that mutates state still repaints promptly.
type viewCache struct {
	dirty bool
	// w/h guard against a resize slipping through while dirty is false.
	w, h int
	view string
}

// NewModel creates the initial Bubble Tea model.
func NewModel(app *App, clientID, userID string) Model {
	ta := textarea.New()
	ta.Placeholder = "Type your message..."
	ta.ShowLineNumbers = false
	ta.CharLimit = 0
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle()
	ta.BlurredStyle.CursorLine = lipgloss.NewStyle()

	ti := textinput.New()
	ti.Placeholder = "Search history..."
	ti.CharLimit = 100
	ti.Width = 40

	tiUser := textinput.New()
	tiUser.Placeholder = "Filter local chats, or enter exact email to open..."
	tiUser.CharLimit = 100
	tiUser.Width = 40

	fp := filepicker.New()
	fp.AllowedTypes = []string{} // allow all types
	fp.FileAllowed = true
	sortBy, sortOrder, lastDir := LoadFilepickerSettings()
	if lastDir != "" {
		fp.CurrentDirectory = lastDir
	} else if home, err := os.UserHomeDir(); err == nil {
		fp.CurrentDirectory = home
	}
	if sortBy == "Datetime" {
		fp.SortBy = filepicker.SortByDatetime
	} else {
		fp.SortBy = filepicker.SortByName
	}
	if sortOrder == "desc" {
		fp.SortOrder = filepicker.SortDescending
	} else {
		fp.SortOrder = filepicker.SortAscending
	}
	fp.Styles = filepicker.DefaultStyles()
	fp.Styles.Directory = lipgloss.NewStyle().Foreground(colCyan).Bold(true)
	fp.Styles.File = lipgloss.NewStyle().Foreground(colWhite)
	fp.Styles.Cursor = lipgloss.NewStyle().Foreground(colGreen).Bold(true)
	fp.Styles.Selected = lipgloss.NewStyle().Foreground(colGreen).Bold(true)
	fp.Styles.Permission = lipgloss.NewStyle().Foreground(colDimGray)

	return Model{
		app:                  app,
		clientID:             clientID,
		userID:               userID,
		textarea:             ta,
		searchInput:          ti,
		userSearchInput:      tiUser,
		lastMsgID:            make(map[string]string),
		lastMsgTime:          make(map[string]time.Time),
		lastReadMsgID:        make(map[string]string),
		lastReadReactions:    make(map[string]map[string]bool),
		reactionsInitialized: make(map[string]bool),
		notifiedReactions:    make(map[string]map[string]bool),
		pendingEdits:         make(map[string]string),
		favourites:           make(map[string]bool),
		unhiddenChannels:     make(map[string]bool),
		originalTeamIndex:    make(map[string]int),
		originalChannelIndex: make(map[string]int),
		focused:              true,
		channelSelectedIndex: -1,
		filepicker:           fp,
		lastWrittenMessages:  -1,
		lastWrittenReactions: -1,
		viewCache:            &viewCache{dirty: true},
	}
}

// ---------------------------------------------------------------------------
// Init
// ---------------------------------------------------------------------------

// Init issues the first tick command to start the event loop.
// Note: the initial chat list is already loaded synchronously in main.go, so
// we do not fire a redundant loadChatsCmd here. The periodic tick handles all
// subsequent refreshes.
func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{
		tickCmd(),
		func() tea.Msg {
			fmt.Print("\x1b[?1004h") // Enable focus reporting
			return nil
		},
	}
	if m.app.Features.TeamsChannels {
		m.app.TeamsDataLoading = true
		cmds = append(cmds, loadTeamsChannelsCmd(m.clientID))
	}
	return tea.Batch(cmds...)
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

func (m Model) updateInternal(msg tea.Msg) (Model, tea.Cmd) {
	var cmds []tea.Cmd
	wasInputMode := m.app.InputMode
	wasSearchMode := m.app.SearchMode
	wasUserSearchMode := m.app.UserSearchMode

	switch msg := msg.(type) {

	// ── Window resize ────────────────────────────────────────────────────
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.textarea.SetWidth(msgPanelWidth(m.width) - 4)
		popupH := m.height * 80 / 100
		if popupH < 15 {
			popupH = 15
		}
		m.filepicker.SetHeight(popupH - 7)

	// ── Heartbeat tick ───────────────────────────────────────────────────
	case MsgTick:
		cmds = append(cmds, tickCmd())

		// The tick fires 10x/sec purely to drive timers. Most do nothing, so
		// the branches below set tickDidWork when they change something the
		// user can see; if none do, Update skips the repaint and the unread
		// scan. Commands dispatched here don't count — their results arrive
		// as their own messages, which mark the view dirty when they land.

		// Clear expired status messages.
		if m.app.StatusUntil != nil && time.Now().After(*m.app.StatusUntil) {
			m.app.Status = ""
			m.app.StatusUntil = nil
			m.tickDidWork = true
		}
		if m.app.SearchStatusUntil != nil && time.Now().After(*m.app.SearchStatusUntil) {
			m.app.SearchStatus = ""
			m.app.SearchStatusUntil = nil
			m.tickDidWork = true
		}

		// Periodic chat refresh every ~15 s.
		if !m.loadingChats && time.Since(m.lastChatRefresh) >= 15*time.Second {
			m.lastChatRefresh = time.Now()
			m.loadingChats = true
			cmds = append(cmds, loadChatsCmd(m.clientID, m.app.Chats, m.app.CurrentUserName))
		}

		isSleepMode := (m.app.SelectedIndex < 0 && m.channelSelectedIndex < 0)

		// Periodic channel message refresh every ChannelMsgRefreshMin minutes.
		refreshDur := time.Duration(m.app.ChannelMsgRefreshMin) * time.Minute
		if refreshDur <= 0 {
			refreshDur = 2 * time.Minute
		}
		if m.app.Features.TeamsChannels && time.Since(m.lastChannelRefresh) >= refreshDur {
			m.lastChannelRefresh = time.Now()
			for _, twc := range m.app.TeamsData {
				for _, ch := range twc.Channels {
					if m.unhiddenChannels[ch.ID] {
						cmds = append(cmds, loadChannelMessagesCmd(m.clientID, twc.Team.ID, ch.ID))
					}
				}
			}
		}

		// Periodic message refresh every ~3 s — skip when unfocused or in sleep/idle mode.
		if m.focused && !isSleepMode && time.Since(m.lastMessageRefresh) >= 3*time.Second {
			if m.channelSelectedIndex < 0 && m.app.GetSelectedChat() != nil {
				m.lastMessageRefresh = time.Now()
				chat := m.app.GetSelectedChat()
				idx := m.app.SelectedIndex
				cmds = append(cmds, loadMessagesCmd(m.clientID, chat.ID, idx))
			} else if m.channelSelectedIndex >= 0 {
				chans := m.allChannels()
				if m.channelSelectedIndex < len(chans) {
					m.lastMessageRefresh = time.Now()
					entry := chans[m.channelSelectedIndex]
					cmds = append(cmds, loadChannelMessagesCmd(m.clientID, entry.teamID, entry.channelID))
				}
			}
		}

		// Periodic reaction poll every ~10 s for other active chats.
		if time.Since(m.lastReactionPoll) >= 10*time.Second {
			m.lastReactionPoll = time.Now()

			var chatsToPoll []string
			selectedID := ""
			if chat := m.app.GetSelectedChat(); chat != nil {
				selectedID = chat.ID
			}

			numChats := len(m.stableChatOrder)
			if numChats > 0 {
				// Poll up to 5 chats in round-robin fashion, excluding the selected one.
				if m.nextReactionPollIndex >= numChats {
					m.nextReactionPollIndex = 0
				}
				startIdx := m.nextReactionPollIndex
				polledCount := 0

				for step := 0; step < numChats; step++ {
					currIdx := (startIdx + step) % numChats
					id := m.stableChatOrder[currIdx]
					if id == selectedID {
						continue
					}
					chatsToPoll = append(chatsToPoll, id)
					polledCount++
					m.nextReactionPollIndex = (currIdx + 1) % numChats
					if polledCount >= 5 {
						break
					}
				}
			}

			if len(chatsToPoll) > 0 {
				cmds = append(cmds, pollReactionsCmd(m.clientID, chatsToPoll))
			}
		}

	// ── Chat list loaded ─────────────────────────────────────────────────
	case MsgChatsLoaded:
		m.loadingChats = false
		m.latestChats = msg.Chats
		if msg.CurrentUserName != nil {
			m.app.SetCurrentUser(*msg.CurrentUserName)
		}

		// Preserve current selection by chat ID.
		selectedID := ""
		if chat := m.app.GetSelectedChat(); chat != nil {
			selectedID = chat.ID
		}

		// Detect new messages in any of the loaded chats.
		for _, c := range m.latestChats {
			if c.LastMessagePreview == nil {
				continue
			}
			prevID, ok := m.lastMsgID[c.ID]
			newID := c.LastMessagePreview.ID

			newTime, _ := time.Parse(time.RFC3339Nano, c.LastMessagePreview.CreatedDateTime)

			isNewMsgInExistingChat := ok && prevID != newID && !m.lastMsgTime[c.ID].IsZero() && newTime.After(m.lastMsgTime[c.ID].Add(time.Second))
			isBrandNewChat := !ok && newTime.After(m.app.AppStartTime)

			if isNewMsgInExistingChat || isBrandNewChat {
				m.lastMsgID[c.ID] = newID
				m.lastMsgTime[c.ID] = newTime
				m.app.ChatCacheDirty[c.ID] = true

				// Save/cache the new message immediately so it's visible if we navigate to the chat.
				m.app.HistoryMessages[c.ID] = mergeHistoryMessages(m.app.HistoryMessages[c.ID], []Message{*c.LastMessagePreview})

				existingCached := m.app.CachedMessages[c.ID]
				if len(existingCached) > 0 {
					foundInCache := false
					for _, cm := range existingCached {
						if cm.ID == c.LastMessagePreview.ID {
							foundInCache = true
							break
						}
					}
					if !foundInCache {
						existingCached = append([]Message{*c.LastMessagePreview}, existingCached...)
						sort.Slice(existingCached, func(i, j int) bool {
							return existingCached[i].CreatedDateTime > existingCached[j].CreatedDateTime
						})
						m.app.CachedMessages[c.ID] = existingCached
					}
				} else {
					m.app.CachedMessages[c.ID] = []Message{*c.LastMessagePreview}
				}

				if m.app.Features.SqliteEnabled {
					go SaveMessages(c.ID, []Message{*c.LastMessagePreview})
				}

				// Determine if it was sent by us.
				isOwnMsg := false
				if m.app.CurrentUserName != nil && c.LastMessagePreview.From != nil &&
					c.LastMessagePreview.From.User != nil && c.LastMessagePreview.From.User.DisplayName != nil {
					isOwnMsg = *c.LastMessagePreview.From.User.DisplayName == *m.app.CurrentUserName
				}

				isActiveChat := false
				if m.channelSelectedIndex < 0 {
					if selChat := m.app.GetSelectedChat(); selChat != nil && selChat.ID == c.ID && m.focused {
						isActiveChat = true
					}
				}

				if isActiveChat {
					// Append/merge into active messages list as well so it's shown immediately
					existingMsgs := m.app.Messages
					foundInActive := false
					for _, am := range existingMsgs {
						if am.ID == c.LastMessagePreview.ID {
							foundInActive = true
							break
						}
					}
					if !foundInActive {
						existingMsgs = append([]Message{*c.LastMessagePreview}, existingMsgs...)
						sort.Slice(existingMsgs, func(i, j int) bool {
							return existingMsgs[i].CreatedDateTime > existingMsgs[j].CreatedDateTime
						})
						m.app.Messages = existingMsgs
					}
				}

				if isOwnMsg || isActiveChat {
					m.lastReadMsgID[c.ID] = newID
					m.promoteChat(c.ID)
					if isActiveChat {
						go MarkChatAsRead(func() string {
							t, _ := GetValidTokenSilent(m.clientID)
							return t
						}(), c.ID, m.userID)
					}
				} else {
					// Track the unread message ID locally to ensure isUnread returns true
					if isBrandNewChat {
						m.lastReadMsgID[c.ID] = ""
					} else {
						m.lastReadMsgID[c.ID] = prevID
					}

					// Trigger notification.
					senderName := ""
					if c.LastMessagePreview.From != nil && c.LastMessagePreview.From.User != nil && c.LastMessagePreview.From.User.DisplayName != nil {
						senderName = *c.LastMessagePreview.From.User.DisplayName
					}

					// Build a temporary Message object for notification.
					tempMsg := Message{
						ID:              newID,
						CreatedDateTime: c.LastMessagePreview.CreatedDateTime,
						From:            c.LastMessagePreview.From,
						Body:            c.LastMessagePreview.Body,
					}
					m.notify(senderName, tempMsg)
					m.promoteChat(c.ID)
				}
			} else if !ok || (ok && m.lastMsgTime[c.ID].IsZero()) {
				// Initialize cache for newly seen or uninitialized chat.
				m.lastMsgID[c.ID] = newID
				m.lastMsgTime[c.ID] = newTime
			}

			// Detect new reactions on LastMessagePreview
			if m.lastReadReactions[c.ID] == nil {
				m.lastReadReactions[c.ID] = make(map[string]bool)
			}
			if ok {
				var newReactions []MessageReaction
				for _, rKey := range m.getReactionKeys(c.LastMessagePreview) {
					if !m.lastReadReactions[c.ID][rKey] {
						for _, r := range c.LastMessagePreview.Reactions {
							if getReactionKey(c.LastMessagePreview.ID, r) == rKey {
								newReactions = append(newReactions, r)
								break
							}
						}
					}
				}
				if len(newReactions) > 0 {
					isActiveChat := false
					if m.channelSelectedIndex < 0 {
						if selChat := m.app.GetSelectedChat(); selChat != nil && selChat.ID == c.ID && m.focused {
							isActiveChat = true
						}
					}
					if !isActiveChat {
						m.notifyReaction(c, c.LastMessagePreview, newReactions)
						m.promoteChat(c.ID)
					} else {
						for _, r := range newReactions {
							m.lastReadReactions[c.ID][getReactionKey(c.LastMessagePreview.ID, r)] = true
						}
					}
					// Update caches and SQLite DB immediately with the new reaction on LastMessagePreview
					m = m.updateCachedMessages(c.ID, []Message{*c.LastMessagePreview})
				}
			} else {
				for _, rKey := range m.getReactionKeys(c.LastMessagePreview) {
					m.lastReadReactions[c.ID][rKey] = true
				}
			}
		}

		// Build stable order.
		m = m.mergeChats(m.latestChats)

		// Restore selection.
		if selectedID != "" {
			for i, c := range m.app.Chats {
				if c.ID == selectedID {
					m.app.SelectedIndex = i
					break
				}
			}
		}

		// Refresh messages if selected chat is set.
		if chat := m.app.GetSelectedChat(); chat != nil {
			cmds = append(cmds, loadMessagesCmd(m.clientID, chat.ID, m.app.SelectedIndex))
		}

	case MsgBackgroundMessagesLoaded:
		if m.lastReadReactions[msg.ChatID] == nil {
			m.lastReadReactions[msg.ChatID] = make(map[string]bool)
		}
		if m.notifiedReactions[msg.ChatID] == nil {
			m.notifiedReactions[msg.ChatID] = make(map[string]bool)
		}

		var chatObj *Chat
		for _, c := range m.latestChats {
			if c.ID == msg.ChatID {
				chatObj = &c
				break
			}
		}

		var newReactions []MessageReaction
		isInit := !m.reactionsInitialized[msg.ChatID]

		for _, msgObj := range msg.Messages {
			var msgNewReactions []MessageReaction
			for _, rKey := range m.getReactionKeys(&msgObj) {
				if !m.lastReadReactions[msg.ChatID][rKey] {
					for _, r := range msgObj.Reactions {
						if getReactionKey(msgObj.ID, r) == rKey {
							if isInit && m.isOldReaction(msgObj, r) {
								m.lastReadReactions[msg.ChatID][rKey] = true
							} else {
								if !m.notifiedReactions[msg.ChatID][rKey] {
									msgNewReactions = append(msgNewReactions, r)
									m.notifiedReactions[msg.ChatID][rKey] = true
								}
							}
							break
						}
					}
				}
			}
			if len(msgNewReactions) > 0 && chatObj != nil {
				m.notifyReaction(*chatObj, &msgObj, msgNewReactions)
				newReactions = append(newReactions, msgNewReactions...)
			}
		}

		m.reactionsInitialized[msg.ChatID] = true
		m = m.updateCachedMessages(msg.ChatID, msg.Messages)

		if len(newReactions) > 0 {
			m.promoteChat(msg.ChatID)
		}

	case MsgPollReactionsLoaded:
		for chatID, msgs := range msg.Results {
			if m.lastReadReactions[chatID] == nil {
				m.lastReadReactions[chatID] = make(map[string]bool)
			}
			if m.notifiedReactions[chatID] == nil {
				m.notifiedReactions[chatID] = make(map[string]bool)
			}

			var chatObj *Chat
			for _, c := range m.latestChats {
				if c.ID == chatID {
					chatObj = &c
					break
				}
			}

			var newReactions []MessageReaction
			isInit := !m.reactionsInitialized[chatID]

			for _, msgObj := range msgs {
				var msgNewReactions []MessageReaction
				for _, rKey := range m.getReactionKeys(&msgObj) {
					if !m.lastReadReactions[chatID][rKey] {
						for _, r := range msgObj.Reactions {
							if getReactionKey(msgObj.ID, r) == rKey {
								if isInit && m.isOldReaction(msgObj, r) {
									m.lastReadReactions[chatID][rKey] = true
								} else {
									if !m.notifiedReactions[chatID][rKey] {
										msgNewReactions = append(msgNewReactions, r)
										m.notifiedReactions[chatID][rKey] = true
									}
								}
								break
							}
						}
					}
				}
				if len(msgNewReactions) > 0 && chatObj != nil {
					m.notifyReaction(*chatObj, &msgObj, msgNewReactions)
					newReactions = append(newReactions, msgNewReactions...)
				}
			}

			m.reactionsInitialized[chatID] = true
			m = m.updateCachedMessages(chatID, msgs)

			if len(newReactions) > 0 {
				m.promoteChat(chatID)
			}
		}

	// ── Messages loaded ──────────────────────────────────────────────────
	case MsgMessagesLoaded:
		// Always update the cache for the chat that was loaded, even if the
		// user has since switched away. This ensures that revisiting the chat
		// later shows fresh data immediately.
		// Retrieve the chat ID from the index at the time the load was issued.
		if msg.ChatIndex >= 0 && msg.ChatIndex < len(m.app.Chats) {
			loadedChatID := m.app.Chats[msg.ChatIndex].ID
			if loadedChatID != "" {
				m.app.ChatMessagesLoadedOnce[loadedChatID] = true
				m.app.ChatCacheDirty[loadedChatID] = false
				if len(msg.Messages) > 0 {
					// Merge into the existing cache rather than overwriting it.
					// A blind overwrite would discard older pages that were already
					// loaded via pagination, and would also wipe pending edit patches.
					existing := m.app.CachedMessages[loadedChatID]
					if len(existing) == 0 {
						m.app.CachedMessages[loadedChatID] = msg.Messages
					} else {
						// Update/add only the messages present in the new batch.
						idxMap := make(map[string]int, len(existing))
						for i, em := range existing {
							idxMap[em.ID] = i
						}
						for _, nm := range msg.Messages {
							if idx, ok := idxMap[nm.ID]; ok {
								existing[idx] = nm
							} else {
								existing = append(existing, nm)
							}
						}
						sort.Slice(existing, func(i, j int) bool {
							return existing[i].CreatedDateTime > existing[j].CreatedDateTime
						})
						m.app.CachedMessages[loadedChatID] = existing
					}
					m.app.CachedNextLink[loadedChatID] = msg.NextLink
					if m.app.Features.SqliteEnabled {
						go SaveMessages(loadedChatID, msg.Messages)
						if msg.NextLink != "" {
							go SaveNextLink(loadedChatID, msg.NextLink)
						}
					}
				}
			}
		}
		// Discard UI update if the selected chat changed since we issued the load,
		// or if we're now viewing a Teams channel instead.
		if msg.ChatIndex != m.app.SelectedIndex || m.channelSelectedIndex >= 0 {
			break
		}
		m.app.SetLoadingMessages(false)
		prev := m.app.Messages
		// Only update if content changed.
		if !messagesEqual(prev, msg.Messages) {
			isNewMessage := len(prev) == 0 || (len(msg.Messages) > 0 && prev[0].ID != msg.Messages[0].ID)
			m.app.SetMessages(msg.Messages, msg.NextLink)

			// Re-apply any pending optimistic edits that the API may not have
			// reflected yet (Graph API has eventual consistency after PATCH).
			if chat := m.app.GetSelectedChat(); chat != nil {
				m.applyPendingEdits(chat.ID)
			}

			// Detect new reactions in the loaded messages
			chat := m.app.GetSelectedChat()
			if chat != nil {
				if m.lastReadReactions[chat.ID] == nil {
					m.lastReadReactions[chat.ID] = make(map[string]bool)
				}
				if m.notifiedReactions[chat.ID] == nil {
					m.notifiedReactions[chat.ID] = make(map[string]bool)
				}
				isInit := !m.reactionsInitialized[chat.ID]
				for _, msgObj := range msg.Messages {
					var msgNewReactions []MessageReaction
					for _, rKey := range m.getReactionKeys(&msgObj) {
						if !m.lastReadReactions[chat.ID][rKey] {
							for _, r := range msgObj.Reactions {
								if getReactionKey(msgObj.ID, r) == rKey {
									if isInit && m.isOldReaction(msgObj, r) {
										m.lastReadReactions[chat.ID][rKey] = true
									} else {
										if !m.notifiedReactions[chat.ID][rKey] {
											msgNewReactions = append(msgNewReactions, r)
											if m.focused {
												m.lastReadReactions[chat.ID][rKey] = true
											} else {
												m.notifiedReactions[chat.ID][rKey] = true
											}
										}
									}
									break
								}
							}
						}
					}
					if len(msgNewReactions) > 0 && !m.focused {
						m.notifyReaction(*chat, &msgObj, msgNewReactions)
					}
				}
				m.reactionsInitialized[chat.ID] = true
			}

			// Keep history cache in sync with new incoming messages
			if chat != nil {
				if hist, ok := m.app.HistoryMessages[chat.ID]; ok && len(hist) > 0 {
					m.app.HistoryMessages[chat.ID] = mergeHistoryMessages(hist, msg.Messages)
					if m.app.SearchPopupMode {
						m.RebuildSearchPopupResults()
						m.saveSearchState()
					}
				}
			}

			// Only snap to bottom if a new message arrived and the user isn't
			// currently busy selecting/reacting to an older message.
			if isNewMessage && !m.app.MessageSelectionMode {
				m.app.SnapToBottom = true
			}

			// If there is a new message, move this chat to the top.
			if len(msg.Messages) > 0 {
				if chat != nil {
					newLastID := ""
					var newTime time.Time
					var latestMsg Message
					if len(msg.Messages) > 0 {
						latestMsg = msg.Messages[0]
						newLastID = latestMsg.ID
						newTime, _ = time.Parse(time.RFC3339Nano, latestMsg.CreatedDateTime)
					}

					if newLastID != "" {
						old, ok := m.lastMsgID[chat.ID]
						if !ok || m.lastMsgTime[chat.ID].IsZero() {
							m.lastMsgID[chat.ID] = newLastID
							m.lastMsgTime[chat.ID] = newTime
						} else if old != newLastID && newTime.After(m.lastMsgTime[chat.ID].Add(time.Second)) {
							m.lastMsgID[chat.ID] = newLastID
							m.lastMsgTime[chat.ID] = newTime
							m.promoteChat(chat.ID)

							isOwnMsg := m.isOwn(latestMsg)
							if isOwnMsg || m.focused {
								m.lastReadMsgID[chat.ID] = newLastID
								if m.focused {
									go MarkChatAsRead(func() string {
										t, _ := GetValidTokenSilent(m.clientID)
										return t
									}(), chat.ID, m.userID)
								}
							} else {
								// Trigger notification if blurred, and mark as unread locally
								m.lastReadMsgID[chat.ID] = old
								senderName := ""
								if latestMsg.From != nil && latestMsg.From.User != nil && latestMsg.From.User.DisplayName != nil {
									senderName = *latestMsg.From.User.DisplayName
								}
								m.notify(senderName, latestMsg)
							}
						}
					}
				}
			}
		}
		// Keep the per-chat message cache in sync for the active chat.
		if chat := m.app.GetSelectedChat(); chat != nil {
			if len(m.app.Messages) > 0 {
				m.app.CachedMessages[chat.ID] = m.app.Messages
				m.app.CachedNextLink[chat.ID] = m.app.NextLink
			}
		}
		// Re-apply pending edits to any cache entries that were just updated.
		if chat := m.app.GetSelectedChat(); chat != nil {
			m.applyPendingEdits(chat.ID)
		}
		m.updateScroll()

	// ── More messages loaded (pagination) ───────────────────────────────
	case MsgMoreMessagesLoaded:
		convID := m.activeConversationID()
		if msg.ConversationID != convID || convID == "" {
			break
		}

		if msg.IsSearch {
			m.app.SetSearchLoadingMessages(false)

			// Update history messages cache directly
			m.app.HistoryMessages[convID] = mergeHistoryMessages(m.app.HistoryMessages[convID], msg.Messages)
			hist := m.app.HistoryMessages[convID]
			m.app.HistoryNextLink[convID] = msg.NextLink

			// Save newly loaded search messages and next link to SQLite!
			if m.app.Features.SqliteEnabled {
				go SaveMessages(convID, msg.Messages)
				if msg.NextLink != "" {
					go SaveNextLink(convID, msg.NextLink)
				}
			}

			// Rebuild search popup results dynamically if search popup is still open
			if m.app.SearchPopupMode {
				m.RebuildSearchPopupResults()
				m.saveSearchState()

				if msg.NextLink != "" {
					m.app.SetSearchLoadingMessages(true)
					m.app.SetSearchStatus(fmt.Sprintf("Searching all history for '%s'... Loaded %d messages", m.app.SearchQuery, len(hist)), 0)
					cmds = append(cmds, loadMoreMessagesCmd(m.clientID, msg.NextLink, convID, true))
				} else {
					m.app.SetSearchStatus(fmt.Sprintf("Search finished. Loaded all %d messages in history.", len(hist)), 5*time.Second)
				}
			} else {
				// Search popup is closed, stop loading!
				m.app.SetSearchLoadingMessages(false)
			}
		} else {
			// Standard main chat scroll pagination
			if len(m.app.Messages) > 0 {
				m.app.PendingScrollID = m.app.Messages[len(m.app.Messages)-1].ID
			}
			m.app.AppendOlderMessages(msg.Messages, msg.NextLink)
			m.updateScroll()
			// Update the per-chat cache with the newly paginated messages.
			m.app.CachedMessages[convID] = m.app.Messages
			m.app.CachedNextLink[convID] = msg.NextLink
			if m.app.Features.SqliteEnabled {
				go SaveMessages(convID, msg.Messages)
				if msg.NextLink != "" {
					go SaveNextLink(convID, msg.NextLink)
				}
			}
		}

	case MsgHistoryLoaded:
		convID := m.activeConversationID()
		if msg.ConversationID == convID && convID != "" {
			m.app.SetSearchLoadingMessages(false)
			m.app.SetSearchStatus("", 0)

			existingIDs := make(map[string]bool)
			for _, mObj := range msg.Messages {
				existingIDs[mObj.ID] = true
			}
			var toAdd []Message
			for _, mainM := range m.app.Messages {
				if !existingIDs[mainM.ID] {
					toAdd = append(toAdd, mainM)
				}
			}
			m.app.HistoryMessages[convID] = mergeHistoryMessages(msg.Messages, toAdd)
			m.app.HistoryNextLink[convID] = msg.NextLink
			m.app.HistoryInitialized[convID] = true

			if m.app.SearchQuery != "" {
				m.RebuildSearchPopupResults()
				m.saveSearchState()
			}
		} else {
			if msg.ConversationID != "" {
				m.app.HistoryMessages[msg.ConversationID] = msg.Messages
				m.app.HistoryNextLink[msg.ConversationID] = msg.NextLink
				m.app.HistoryInitialized[msg.ConversationID] = true
			}
		}

	case MsgUserSearchDone:
		m.app.UserSearchLoading = false
		if msg.Err != nil {
			errStr := msg.Err.Error()
			if strings.Contains(errStr, "403") {
				m.app.UserSearchStatus = "⚠️ Directory search not allowed (missing permissions). Enter the exact email/UPN (e.g. user@domain.com) to open/create the chat directly."
			} else {
				m.app.UserSearchStatus = "⚠️ Search error: " + errStr
			}
			m.app.UserSearchDirectoryResults = nil
		} else {
			m.app.UserSearchDirectoryResults = msg.Users
			if len(msg.Users) == 0 {
				m.app.UserSearchStatus = "No directory users found matching query."
			} else {
				m.app.UserSearchStatus = fmt.Sprintf("Found %d directory users.", len(msg.Users))
			}
		}
		m.app.UserSearchSelectedIndex = 0

	case MsgCreateChatDone:
		m.app.UserSearchLoading = false
		if msg.Err != nil {
			m.app.UserSearchStatus = "⚠️ Create chat error: " + msg.Err.Error()
		} else if msg.Chat != nil {
			m.app.UserSearchPopupMode = false
			m.app.UserSearchMode = false
			m.app.UserSearchQuery = ""
			m.app.UserSearchLocalResults = nil
			m.app.UserSearchDirectoryResults = nil

			chat := *msg.Chat
			if m.app.CurrentUserName != nil {
				chat.Members = filterMember(chat.Members, *m.app.CurrentUserName)
			}
			name := computeDisplayName(&chat)
			chat.CachedDisplayName = &name

			existsInLatest := false
			for _, c := range m.latestChats {
				if c.ID == chat.ID {
					existsInLatest = true
					break
				}
			}
			if !existsInLatest {
				m.latestChats = append([]Chat{chat}, m.latestChats...)
			}

			m.promoteChat(chat.ID)
			m = m.mergeChats(m.latestChats)

			for i, c := range m.app.Chats {
				if c.ID == chat.ID {
					m.app.SelectedIndex = i
					break
				}
			}

			m.app.Messages = nil
			m.app.NextLink = ""
			m.app.SetLoadingMessages(true)
			m.app.SnapToBottom = true

			cmds = append(cmds, loadMessagesCmd(m.clientID, chat.ID, m.app.SelectedIndex))
		}

	// ── Focus / Blur ─────────────────────────────────────────────────────
	case tea.FocusMsg:
		m.focused = true
		m = m.markRead()
		// Instantly load messages on focus if a chat or channel is selected.
		if m.channelSelectedIndex >= 0 {
			chans := m.allChannels()
			if m.channelSelectedIndex < len(chans) {
				entry := chans[m.channelSelectedIndex]
				m.lastMessageRefresh = time.Now()
				cmds = append(cmds, loadChannelMessagesCmd(m.clientID, entry.teamID, entry.channelID))
			}
		} else if chat := m.app.GetSelectedChat(); chat != nil {
			m.lastMessageRefresh = time.Now()
			cmds = append(cmds, loadMessagesCmd(m.clientID, chat.ID, m.app.SelectedIndex))
		}

	case tea.BlurMsg:
		m.focused = false

	// ── Message send result ───────────────────────────────────────────────
	case MsgSendDone:
		if msg.Err != nil {
			m.app.SetStatus("Send error: "+msg.Err.Error(), 5*time.Second)
		} else {
			m.app.SetStatus("Sent!", 2*time.Second)
			// Immediately reload messages after send.
			if m.channelSelectedIndex >= 0 {
				// In channel mode — reload the channel messages.
				chans := m.allChannels()
				if m.channelSelectedIndex < len(chans) {
					entry := chans[m.channelSelectedIndex]
					m.lastMessageRefresh = time.Now()
					cmds = append(cmds, loadChannelMessagesCmd(m.clientID, entry.teamID, entry.channelID))
				}
			} else if chat := m.app.GetSelectedChat(); chat != nil {
				m.lastMessageRefresh = time.Now()
				cmds = append(cmds, loadMessagesCmd(m.clientID, chat.ID, m.app.SelectedIndex))
			}
		}

	// ── Message edit result ───────────────────────────────────────────────
	case MsgEditDone:
		if msg.Err != nil {
			m.app.SetStatus("Edit error: "+msg.Err.Error(), 5*time.Second)
		} else {
			// Convert the user's markdown to HTML (same path as formatMessageBody).
			newHTML := markdownToHTML(msg.Content)

			// Record this as a pending edit. Subsequent MsgMessagesLoaded
			// handlers will re-apply it after SetMessages, because Graph API
			// GET may return the old content for several seconds after a PATCH
			// (eventual consistency). The entry is cleared once the API echoes
			// back the updated content.
			m.pendingEdits[msg.MessageID] = newHTML
			if m.app.Features.SqliteEnabled {
				go UpdateStoredMessageBody(msg.MessageID, newHTML)
			}

			// Also patch the live caches immediately so the UI updates now.
			m.applyPendingEdits(msg.ChatID)
		}

	// ── External editor finished ──────────────────────────────────────────
	case MsgEditorFinished:
		if msg.Err != nil {
			m.app.SetStatus("Editor error: "+msg.Err.Error(), 5*time.Second)
		} else if !msg.ReadOnly {
			m.textarea.SetValue(msg.Content)
			m.app.InputBuffer = msg.Content
			m.textarea.CursorEnd()
		}

	// ── URL opened finished ────────────────────────────────────────────────
	case MsgURLOpened:
		if msg.Err != nil {
			errStr := "Failed to open URL: " + msg.Err.Error()
			if m.app.SearchPopupMode {
				m.app.SetSearchStatus(errStr, 3*time.Second)
			} else {
				m.app.SetStatus(errStr, 3*time.Second)
			}
		} else {
			if m.app.SearchPopupMode {
				m.app.SetSearchStatus("URL opened", 3*time.Second)
			} else {
				m.app.SetStatus("URL opened", 3*time.Second)
			}
		}

	// ── Presence loaded ───────────────────────────────────────
	case MsgPresenceLoaded:
		m.app.PresenceLoading = false
		if msg.Err == nil {
			m.app.PresenceData = msg.Presence
		} else {
			m.app.PresencePopupMode = false
			m.app.SetStatus("Presence unavailable: "+msg.Err.Error(), 4*time.Second)
		}

	// ── Chat Presence loaded ──────────────────────────────────
	case MsgChatPresenceLoaded:
		m.app.PresenceLoading = false
		if msg.Err == nil {
			var entries []PresenceEntry
			chat := m.app.GetSelectedChat()
			if chat != nil {
				for _, member := range chat.Members {
					if member.UserID != nil && member.DisplayName != nil {
						uID := *member.UserID
						displayName := *member.DisplayName
						if p, ok := msg.Presences[uID]; ok {
							entries = append(entries, PresenceEntry{
								UserName:     displayName,
								Availability: p.Availability,
								Activity:     p.Activity,
							})
						} else {
							entries = append(entries, PresenceEntry{
								UserName:     displayName,
								Availability: "PresenceUnknown",
								Activity:     "",
							})
						}
					}
				}
			}
			m.app.PresenceChatData = entries
		} else {
			m.app.PresencePopupMode = false
			m.app.PresenceChatMode = false
			m.app.SetStatus("Presence unavailable: "+msg.Err.Error(), 4*time.Second)
		}

	// ── User profile loaded ────────────────────────────────────
	case MsgUserProfileLoaded:
		m.app.UserProfileLoading = false
		if msg.Err == nil {
			m.app.UserProfileData = msg.Profile
		} else {
			m.app.UserProfilePopupMode = false
			m.app.SetStatus("Profile unavailable: "+msg.Err.Error(), 4*time.Second)
		}

	// ── File downloaded ─────────────────────────────────────────
	case MsgFileDownloaded:
		if msg.Err == nil {
			m.app.SetStatus("Saved to: "+msg.DestPath, 6*time.Second)
			_ = openFile(msg.DestPath)
		} else {
			m.app.SetStatus("Download failed: "+msg.Err.Error(), 5*time.Second)
		}

	// ── Images opened with image viewer ─────────────────────────
	case MsgImagesOpened:
		if msg.Err == nil {
			statusMsg := "Opened in image viewer: " + msg.SelectedPath
			if msg.TotalImages > 1 {
				statusMsg = fmt.Sprintf("Opened %d images in viewer (selected: %s)", msg.TotalImages, filepath.Base(msg.SelectedPath))
			}
			m.app.SetStatus(statusMsg, 6*time.Second)
		} else {
			m.app.SetStatus("Image viewer error: "+msg.Err.Error(), 5*time.Second)
		}

	case MsgPreviewDownloaded:
		if msg.Err != nil {
			m.app.SetStatus("Preview error: "+msg.Err.Error(), 3*time.Second)
		}

	// ── Teams channels loaded ───────────────────────────────────
	case MsgTeamsChannelsLoaded:
		m.app.TeamsDataLoading = false
		if msg.Err == nil {
			m.app.TeamsData = msg.Teams
			// Record original order indexes
			m.originalTeamIndex = make(map[string]int)
			m.originalChannelIndex = make(map[string]int)
			for i, twc := range m.app.TeamsData {
				m.originalTeamIndex[twc.Team.ID] = i
				for j, ch := range twc.Channels {
					m.originalChannelIndex[ch.ID] = j
				}
			}

			// Re-sort teams and channels
			m.sortTeamsAndChannels()

			// Fetch messages for all unhidden channels immediately
			if m.app.Features.TeamsChannels {
				for _, twc := range m.app.TeamsData {
					for _, ch := range twc.Channels {
						if m.unhiddenChannels[ch.ID] {
							cmds = append(cmds, loadChannelMessagesCmd(m.clientID, twc.Team.ID, ch.ID))
						}
					}
				}
			}
		} else {
			m.app.SetStatus("Teams unavailable: "+msg.Err.Error(), 4*time.Second)
		}

	// ── Channel messages loaded ──────────────────────────────────
	case MsgChannelMessagesLoaded:
		if msg.Err != nil {
			m.app.SetStatus("Channel messages unavailable: "+msg.Err.Error(), 5*time.Second)
			m.app.SelectedChannelTeamID = ""
			m.app.SelectedChannelID = ""
		} else {
			// Cache messages and next link
			if len(msg.Messages) > 0 {
				// Merge into the existing cache rather than overwriting it.
				existing := m.app.CachedMessages[msg.ChannelID]
				if len(existing) == 0 {
					m.app.CachedMessages[msg.ChannelID] = msg.Messages
				} else {
					idxMap := make(map[string]int, len(existing))
					for i, em := range existing {
						idxMap[em.ID] = i
					}
					for _, nm := range msg.Messages {
						if idx, ok := idxMap[nm.ID]; ok {
							existing[idx] = nm
						} else {
							existing = append(existing, nm)
						}
					}
					sort.Slice(existing, func(i, j int) bool {
						return existing[i].CreatedDateTime > existing[j].CreatedDateTime
					})
					m.app.CachedMessages[msg.ChannelID] = existing
				}
				m.app.CachedNextLink[msg.ChannelID] = msg.NextLink
				if m.app.Features.SqliteEnabled {
					go SaveMessages(msg.ChannelID, msg.Messages)
					if msg.NextLink != "" {
						go SaveNextLink(msg.ChannelID, msg.NextLink)
					}
				}
			}

			if m.app.Features.ChannelMentions && len(m.app.TeamMembersCache[msg.TeamID]) == 0 {
				cmds = append(cmds, loadTeamMembersCmd(m.clientID, msg.TeamID))
			}

			// Check if active channel
			isActiveChannel := (m.channelSelectedIndex >= 0 &&
				msg.TeamID == m.app.SelectedChannelTeamID &&
				msg.ChannelID == m.app.SelectedChannelID)

			if isActiveChannel {
				prev := m.app.Messages
				if !messagesEqual(prev, msg.Messages) {
					m.app.SetMessages(msg.Messages, msg.NextLink)
				}
				m.app.SetLoadingMessages(false)
			}

			// Update lastMsgID and lastMsgTime
			if len(msg.Messages) > 0 {
				newest := msg.Messages[0]
				prevID, ok := m.lastMsgID[msg.ChannelID]
				newTime, _ := time.Parse(time.RFC3339Nano, newest.CreatedDateTime)

				if isActiveChannel && m.focused {
					m.lastMsgID[msg.ChannelID] = newest.ID
					m.lastMsgTime[msg.ChannelID] = newTime
					m.lastReadMsgID[msg.ChannelID] = newest.ID
				} else {
					if ok && prevID != newest.ID && !m.lastMsgTime[msg.ChannelID].IsZero() && newTime.After(m.lastMsgTime[msg.ChannelID].Add(time.Second)) {
						m.lastMsgID[msg.ChannelID] = newest.ID
						m.lastMsgTime[msg.ChannelID] = newTime

						isOwnMsg := m.isOwn(newest)
						if isOwnMsg {
							m.lastReadMsgID[msg.ChannelID] = newest.ID
						} else {
							// Trigger notification
							senderName := ""
							if newest.From != nil && newest.From.User != nil && newest.From.User.DisplayName != nil {
								senderName = *newest.From.User.DisplayName
							}
							m.notify(senderName, newest)
						}
					} else if !ok || m.lastMsgTime[msg.ChannelID].IsZero() {
						// First time loading for channel in this session, mark as read
						m.lastMsgID[msg.ChannelID] = newest.ID
						m.lastMsgTime[msg.ChannelID] = newTime
						m.lastReadMsgID[msg.ChannelID] = newest.ID
					}
				}
			}

			// Keep history cache in sync with new incoming messages
			if msg.ChannelID != "" {
				if hist, ok := m.app.HistoryMessages[msg.ChannelID]; ok && len(hist) > 0 {
					m.app.HistoryMessages[msg.ChannelID] = mergeHistoryMessages(hist, msg.Messages)
					if m.app.SearchPopupMode {
						m.RebuildSearchPopupResults()
						m.saveSearchState()
					}
				}
			}

			// Re-sort teams and channels and preserve channelSelectedIndex.
			var selectedChanID string
			if entry := m.activeChannelEntry(); entry != nil {
				selectedChanID = entry.channelID
			}

			m.sortTeamsAndChannels()

			if selectedChanID != "" {
				chans := m.allChannels()
				for idx, entry := range chans {
					if entry.channelID == selectedChanID {
						m.channelSelectedIndex = idx
						break
					}
				}
			}
		}

	case MsgTeamMembersLoaded:
		if msg.Err == nil {
			m.app.TeamMembersCache[msg.TeamID] = msg.Members
		}

	case MsgFileAttached:
		if msg.Err != nil {
			m.app.SetStatus("File read error: "+msg.Err.Error(), 5*time.Second)
			return m, nil
		}
		if len(msg.Data) > 50*1024*1024 {
			m.app.SetStatus("Error: File exceeds the 50MB limit", 5*time.Second)
			return m, nil
		}
		m.app.ComposedFiles = append(m.app.ComposedFiles, PendingFile{
			Name:        msg.Name,
			ContentType: msg.ContentType,
			Data:        msg.Data,
		})
		placeholder := fmt.Sprintf("[File: %s]", msg.Name)
		m.textarea.InsertString(placeholder)
		m.app.InputBuffer = m.textarea.Value()
		m.app.SetStatus("Attached "+msg.Name, 3*time.Second)
		m.app.SkipTextareaUpdate = true
		return m, nil

	// ── Keyboard input ────────────────────────────────────────────────────
	case tea.KeyMsg:
		m = m.markRead()
		var cmd tea.Cmd
		m, cmd = m.handleKey(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}

	// Update textarea if in input mode.
	if m.app.InputMode && wasInputMode && !m.app.FilePickerPopupMode {
		if m.app.SkipTextareaUpdate {
			m.app.SkipTextareaUpdate = false
		} else {
			var cmd tea.Cmd
			oldVal := m.textarea.Value()
			m.textarea, cmd = m.textarea.Update(msg)
			cmds = append(cmds, cmd)

			newVal := m.textarea.Value()
			if oldVal != newVal {
				replaced := replaceEmoticons(newVal)
				if replaced != newVal {
					// In bubbles/textarea v1.0.0, we can get the current column offset.
					// SetValue resets the cursor to the start, so we attempt to restore it.
					// For multi-line messages, we fallback to moving the cursor to the end
					// to avoid jumping to the wrong line.
					col := m.textarea.LineInfo().ColumnOffset
					diff := len([]rune(newVal)) - len([]rune(replaced))
					m.textarea.SetValue(replaced)
					if m.textarea.LineCount() <= 1 {
						m.textarea.SetCursor(col - diff)
					} else {
						m.textarea.CursorEnd()
					}
				}
			}
			m.app.InputBuffer = m.textarea.Value()
		}

		// Update mention popup state based on the current text and cursor position.
		val := m.textarea.Value()
		cursor := getCursorPos(m.textarea)
		startIdx, query, ok := getMentionQuery(val, cursor)
		if ok {
			if startIdx == m.app.MentionCanceledStartIndex {
				m.app.MentionPopupMode = false
				m.app.MentionSuggestions = nil
			} else {
				m.app.MentionCanceledStartIndex = -1
				if !m.app.MentionPopupMode {
					m.app.MentionScrollOffset = 0
				}
				m.app.MentionPopupMode = true
				m.app.MentionSearch = query
				m.app.MentionStartIndex = startIdx
				m = m.rebuildMentionSuggestions()
				if len(m.app.MentionSuggestions) == 0 {
					m.app.MentionPopupMode = false
					m.app.MentionSuggestions = nil
				}
			}
		} else {
			m.app.MentionCanceledStartIndex = -1
			m.app.MentionPopupMode = false
			m.app.MentionSuggestions = nil
		}
	}

	// Update search input if in search mode.
	if m.app.SearchMode && wasSearchMode {
		var cmd tea.Cmd
		m.searchInput, cmd = m.searchInput.Update(msg)
		cmds = append(cmds, cmd)
	}

	// Update user search input if in user search mode.
	if m.app.UserSearchMode && wasUserSearchMode {
		var cmd tea.Cmd
		oldVal := m.userSearchInput.Value()
		m.userSearchInput, cmd = m.userSearchInput.Update(msg)
		cmds = append(cmds, cmd)

		newVal := m.userSearchInput.Value()
		if oldVal != newVal {
			m.app.UserSearchQuery = newVal
			m.updateUserSearchLocalResults()
			m.app.UserSearchSelectedIndex = 0
		}
	}

	// Update filepicker if in filepicker mode (for non-keyboard messages like directory read results)
	if m.app.FilePickerPopupMode {
		if _, ok := msg.(tea.KeyMsg); !ok {
			var cmd tea.Cmd
			m.filepicker, cmd = m.filepicker.Update(msg)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
	}

	return m, tea.Batch(cmds...)
}

// handleKey processes keyboard input and returns the updated model + command.
func (m Model) handleKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	if m.app.FilePickerPopupMode {
		return m.handleFilePickerKey(msg)
	}
	if m.app.HelpPopupMode {
		return m.handleHelpPopupKey(msg)
	}
	if m.app.PresencePopupMode {
		return m.handlePresencePopupKey(msg)
	}
	if m.app.UserProfilePopupMode {
		return m.handleUserProfilePopupKey(msg)
	}
	if m.app.MessagePopupMode {
		return m.handleMessagePopupKey(msg)
	}
	if m.app.InputMode {
		return m.handleInputModeKey(msg)
	}
	if m.app.UserSearchPopupMode {
		if m.app.UserSearchMode {
			return m.handleUserSearchInputModeKey(msg)
		}
		return m.handleUserSearchNavigationKey(msg)
	}
	if m.app.SearchPopupMode {
		if m.app.SearchMode {
			return m.handleSearchModeKey(msg)
		}
		return m.handleSearchPopupNavigationKey(msg)
	}
	return m.handleNormalModeKey(msg)
}

func (m Model) handleNormalModeKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	if m.app.DeleteConfirmMode {
		return m.handleDeleteConfirmModeKey(msg)
	}
	if m.app.ReactionMode {
		return m.handleReactionModeKey(msg)
	}
	if m.app.UrlSelectionMode {
		return m.handleUrlSelectionModeKey(msg)
	}
	if m.app.MessageSelectionMode {
		return m.handleMessageSelectionModeKey(msg)
	}

	prevIdx := m.app.SelectedIndex

	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit

	case "j", "down":
		if m.channelSelectedIndex >= 0 {
			// Channel section: wrap around at the bottom.
			chans := m.allChannels()
			if m.channelSelectedIndex < len(chans)-1 {
				m.channelSelectedIndex++
			} else {
				m.channelSelectedIndex = 0 // wrap to top of channels
			}
		} else {
			// Chat section: wrap around at the bottom.
			if m.app.SelectedIndex < len(m.app.Chats)-1 {
				m.app.NextChat()
			} else {
				m.app.SelectedIndex = 0 // wrap to top of chats
			}
		}

	case "k", "up":
		if m.channelSelectedIndex >= 0 {
			// Channel section: wrap around at the top.
			if m.channelSelectedIndex > 0 {
				m.channelSelectedIndex--
			} else {
				chans := m.allChannels()
				m.channelSelectedIndex = len(chans) - 1 // wrap to bottom of channels
			}
		} else {
			// Chat section: wrap around at the top.
			if m.app.SelectedIndex > 0 {
				m.app.PreviousChat()
			} else {
				m.app.SelectedIndex = len(m.app.Chats) - 1 // wrap to bottom of chats
			}
		}

	case "tab":
		// Switch between chat section and channel section.
		if !m.app.Features.TeamsChannels || len(m.allChannels()) == 0 {
			break
		}
		if m.channelSelectedIndex >= 0 {
			// Currently in channels → switch to chats.
			m.channelSelectedIndex = -1
			m.app.SelectedChannelTeamID = ""
			m.app.SelectedChannelID = ""
			m.app.SnapToBottom = true
			if chat := m.app.GetSelectedChat(); chat != nil {
				m = m.markRead()
				return m.loadChatMessages(chat.ID, m.app.SelectedIndex)
			}
		} else {
			// Currently in chats → switch to channels (go to first channel).
			m.channelSelectedIndex = 0
			chans := m.allChannels()
			if len(chans) > 0 {
				entry := chans[0]
				m.app.SelectedChannelTeamID = entry.teamID
				m.app.SelectedChannelID = entry.channelID
				return m.loadChannelMessages(entry.teamID, entry.channelID)
			}
		}

	case "n":
		m.app.ToggleNotificationMode()
		nm := m.app.NotificationMode
		cfg := LoadConfig()
		if cfg == nil {
			cfg = &Config{}
		}
		cfg.NotificationMode = &nm
		_ = SaveConfig(cfg)

	case "?":
		m.app.HelpPopupMode = true
		m.app.HelpScrollOffset = 0

	case "i":
		if m.app.SelectedIndex < 0 && m.channelSelectedIndex < 0 {
			break
		}
		m.app.InputMode = true
		m.app.InputBuffer = ""
		m.textarea.Reset()
		return m, m.textarea.Focus()

	case "c":
		m.app.UserSearchPopupMode = true
		m.app.UserSearchMode = true
		m.app.UserSearchQuery = ""
		m.app.UserSearchStatus = ""
		m.app.UserSearchLocalResults = nil
		m.app.UserSearchDirectoryResults = nil
		m.app.UserSearchSelectedIndex = 0
		m.app.UserSearchLoading = false
		m.userSearchInput.SetValue("")
		m.userSearchInput.Focus()
		return m, textinput.Blink

	case "/":
		if m.app.SelectedIndex < 0 && m.channelSelectedIndex < 0 {
			break
		}
		m.app.MainChatScrollOffset = m.app.ScrollOffset
		m.app.MainChatSnapToBottom = m.app.SnapToBottom
		m.app.SearchPopupMode = true
		m.app.SearchMode = true
		convID := m.activeConversationID()
		var initCmd tea.Cmd
		if convID != "" {
			if m.app.Features.SqliteEnabled && !m.app.HistoryInitialized[convID] {
				m.app.SetSearchLoadingMessages(true)
				m.app.SetSearchStatus("Loading history from database...", 0)
				initCmd = loadHistoryFromDBCmd(convID)
			} else {
				hist := m.app.HistoryMessages[convID]
				existingIDs := make(map[string]bool)
				for _, mObj := range hist {
					existingIDs[mObj.ID] = true
				}
				var toAdd []Message
				for _, mainM := range m.app.Messages {
					if !existingIDs[mainM.ID] {
						toAdd = append(toAdd, mainM)
					}
				}
				m.app.HistoryMessages[convID] = mergeHistoryMessages(hist, toAdd)
				if !m.app.HistoryInitialized[convID] {
					m.app.HistoryNextLink[convID] = m.app.NextLink
					m.app.HistoryInitialized[convID] = true
				}
			}
		}
		m.loadSearchState()
		m.searchInput.SetValue(m.app.SearchQuery)
		m.searchInput.Focus()
		if initCmd != nil {
			return m, tea.Batch(textinput.Blink, initCmd)
		}
		return m, textinput.Blink

	case "esc":
		if m.app.SearchActive {
			m.app.SearchActive = false
			m.app.SearchQuery = ""
			m.app.SetStatus("Highlights cleared.", 3*time.Second)
		} else {
			m.app.SelectedIndex = -1
			m.channelSelectedIndex = -1
			m.app.SelectedChannelTeamID = ""
			m.app.SelectedChannelID = ""
			m.app.Messages = nil
			m.app.NextLink = ""
			m.app.SetLoadingMessages(false)
			m.app.SetStatus("💤 Entered sleep mode. No chat active.", 3*time.Second)
		}

	case "K", "pgup":
		if m.app.SelectedIndex < 0 && m.channelSelectedIndex < 0 {
			break
		}
		if m.app.ScrollOffset == 0 && m.app.NextLink != "" && !m.app.LoadingMessages {
			m.app.SetLoadingMessages(true)
			return m, loadMoreMessagesCmd(m.clientID, m.app.NextLink, m.activeConversationID(), false)
		}
		m.app.ScrollOffset -= 10
		if m.app.ScrollOffset < 0 {
			m.app.ScrollOffset = 0
		}
		m.app.SnapToBottom = false

	case "J", "pgdown":
		if m.app.SelectedIndex < 0 && m.channelSelectedIndex < 0 {
			break
		}
		m.app.ScrollOffset += 10
		if m.app.ScrollOffset >= m.app.MaxScroll {
			m.app.ScrollOffset = m.app.MaxScroll
			m.app.SnapToBottom = true
		}

	case "m":
		if m.app.SelectedIndex < 0 && m.channelSelectedIndex < 0 {
			break
		}
		if len(m.app.Messages) > 0 {
			m.app.MessageSelectionMode = true
			if m.app.SnapToBottom {
				m.app.MessageSelectedIndex = 0
			} else {
				// Try to start selection at the message currently at the top of the viewport.
				m.app.MessageSelectedIndex = 0
				for i := 0; i < len(m.app.Messages); i++ {
					if i < len(m.app.MessageLineOffsets) && m.app.MessageLineOffsets[i] <= m.app.ScrollOffset {
						m.app.MessageSelectedIndex = i
						break
					}
				}
			}
		}

	case "f":
		// Toggle favourite on the selected chat — no-op in channel mode.
		if m.channelSelectedIndex >= 0 {
			break
		}
		if chat := m.app.GetSelectedChat(); chat != nil {
			if m.favourites[chat.ID] {
				delete(m.favourites, chat.ID)
				m.app.SetStatus("★ Removed from favourites: "+*chat.CachedDisplayName, 3*time.Second)
			} else {
				m.favourites[chat.ID] = true
				m.app.SetStatus("★ Added to favourites: "+*chat.CachedDisplayName, 3*time.Second)
			}
			// Persist and rebuild the list to reorder immediately.
			_ = SaveFavourites(m.favourites)
			m = m.rebuildChatList()
			// Restore selection to the toggled chat.
			for i, c := range m.app.Chats {
				if c.ID == chat.ID {
					m.app.SelectedIndex = i
					break
				}
			}
		}

	case "p":
		// Show presence popup for chats (requires presence_enabled feature).
		if !m.app.Features.Presence {
			m.app.SetStatus("Presence feature disabled — enable 'presence_enabled' in config.json", 5*time.Second)
			return m, nil
		}
		if m.channelSelectedIndex >= 0 {
			break
		}
		if chat := m.app.GetSelectedChat(); chat != nil {
			if chat.ChatType != "oneOnOne" && chat.ChatType != "group" {
				m.app.SetStatus("Presence only supported for oneOnOne and group chats", 4*time.Second)
				return m, nil
			}
			m.app.PresencePopupMode = true
			m.app.PresenceChatMode = true
			m.app.PresenceData = nil
			m.app.PresenceChatData = nil
			m.app.PresenceLoading = true
			m.app.PresenceScrollOffset = 0
			if chat.Topic != nil {
				m.app.PresenceUserName = *chat.Topic
			} else if chat.CachedDisplayName != nil {
				m.app.PresenceUserName = *chat.CachedDisplayName
			} else {
				m.app.PresenceUserName = "Chat"
			}

			// Build list of user IDs to load presence for
			var userIDs []string
			for _, member := range chat.Members {
				if member.UserID != nil && *member.UserID != "" {
					userIDs = append(userIDs, *member.UserID)
				}
			}
			if len(userIDs) == 0 {
				m.app.PresenceLoading = false
				m.app.SetStatus("No members found in this chat to query presence", 4*time.Second)
				m.app.PresencePopupMode = false
				m.app.PresenceChatMode = false
				return m, nil
			}
			return m, loadChatPresenceCmd(m.clientID, userIDs)
		}

	case "h":
		// Toggle hide/unhide on the selected channel — no-op in chat mode.
		if m.channelSelectedIndex < 0 {
			break
		}
		chans := m.allChannels()
		if m.channelSelectedIndex < len(chans) {
			entry := chans[m.channelSelectedIndex]
			if m.unhiddenChannels[entry.channelID] {
				delete(m.unhiddenChannels, entry.channelID)
				m.app.SetStatus("Muted channel (hidden): "+entry.channelName, 3*time.Second)
			} else {
				m.unhiddenChannels[entry.channelID] = true
				m.app.SetStatus("Unhidden channel: "+entry.channelName, 3*time.Second)

				_ = SaveUnhiddenChannels(m.unhiddenChannels)
				m.sortTeamsAndChannels()
				for idx, e := range m.allChannels() {
					if e.channelID == entry.channelID {
						m.channelSelectedIndex = idx
						break
					}
				}
				return m, loadChannelMessagesCmd(m.clientID, entry.teamID, entry.channelID)
			}
			_ = SaveUnhiddenChannels(m.unhiddenChannels)
			m.sortTeamsAndChannels()
			for idx, e := range m.allChannels() {
				if e.channelID == entry.channelID {
					m.channelSelectedIndex = idx
					break
				}
			}
		}
	}

	// If channel is selected, load its messages on any navigation change.
	if m.channelSelectedIndex >= 0 {
		chans := m.allChannels()
		if m.channelSelectedIndex < len(chans) {
			entry := chans[m.channelSelectedIndex]
			if entry.teamID != m.app.SelectedChannelTeamID || entry.channelID != m.app.SelectedChannelID {
				m.app.SelectedChannelTeamID = entry.teamID
				m.app.SelectedChannelID = entry.channelID
				return m.loadChannelMessages(entry.teamID, entry.channelID)
			}
		}
		return m, nil
	}

	// If chat selection changed, reload messages.
	if m.app.SelectedIndex != prevIdx {
		// Left channel mode when switching to a chat.
		m.app.SelectedChannelTeamID = ""
		m.app.SelectedChannelID = ""
		m.app.SearchMode = false
		m.app.SearchActive = false
		m.app.SearchQuery = ""
		m.app.SnapToBottom = true
		if chat := m.app.GetSelectedChat(); chat != nil {
			m = m.markRead()
			return m.loadChatMessages(chat.ID, m.app.SelectedIndex)
		}
	}

	return m, nil
}

func (m Model) handleInputModeKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	if m.app.MentionPopupMode {
		switch msg.String() {
		case "esc":
			m.app.MentionPopupMode = false
			m.app.MentionSuggestions = nil
			m.app.SkipTextareaUpdate = true
			m.app.MentionCanceledStartIndex = m.app.MentionStartIndex
			return m, nil

		case "up", "shift+tab":
			if len(m.app.MentionSuggestions) > 0 {
				m.app.MentionSelectedIndex--
				if m.app.MentionSelectedIndex < 0 {
					m.app.MentionSelectedIndex = len(m.app.MentionSuggestions) - 1
				}
				// Adjust scroll offset
				limit := 5
				if m.app.MentionSelectedIndex < m.app.MentionScrollOffset {
					m.app.MentionScrollOffset = m.app.MentionSelectedIndex
				} else if m.app.MentionSelectedIndex >= m.app.MentionScrollOffset+limit {
					// Handle wrap-around from top to bottom
					m.app.MentionScrollOffset = m.app.MentionSelectedIndex - limit + 1
				}
			}
			m.app.SkipTextareaUpdate = true
			return m, nil

		case "down", "tab":
			if len(m.app.MentionSuggestions) > 0 {
				m.app.MentionSelectedIndex++
				if m.app.MentionSelectedIndex >= len(m.app.MentionSuggestions) {
					m.app.MentionSelectedIndex = 0
				}
				// Adjust scroll offset
				limit := 5
				if m.app.MentionSelectedIndex >= m.app.MentionScrollOffset+limit {
					m.app.MentionScrollOffset = m.app.MentionSelectedIndex - limit + 1
				} else if m.app.MentionSelectedIndex < m.app.MentionScrollOffset {
					// Handle wrap-around from bottom to top
					m.app.MentionScrollOffset = 0
				}
			}
			m.app.SkipTextareaUpdate = true
			return m, nil

		case "enter":
			if len(m.app.MentionSuggestions) > 0 && m.app.MentionSelectedIndex >= 0 && m.app.MentionSelectedIndex < len(m.app.MentionSuggestions) {
				selected := m.app.MentionSuggestions[m.app.MentionSelectedIndex]
				if selected.DisplayName != nil {
					displayName := *selected.DisplayName
					val := m.textarea.Value()
					runes := []rune(val)
					startIdx := m.app.MentionStartIndex
					cursor := getCursorPos(m.textarea)
					if startIdx >= 0 && startIdx < cursor && cursor <= len(runes) {
						prefix := string(runes[:startIdx])
						suffix := string(runes[cursor:])
						newVal := prefix + "@" + displayName + " " + suffix
						m.textarea.SetValue(newVal)
						newCursor := startIdx + len([]rune("@"+displayName+" "))
						m.textarea.SetCursor(newCursor)
						m.app.InputBuffer = newVal
					}
				}
			}
			m.app.MentionPopupMode = false
			m.app.MentionSuggestions = nil
			m.app.SkipTextareaUpdate = true
			return m, nil
		}
	}

	switch msg.String() {
	case "esc":
		m.app.InputMode = false
		m.app.InputBuffer = ""
		m.app.EditingMessageID = nil
		m.app.ReplyToMessage = nil
		m.app.ChannelReplyToID = ""
		m.app.ComposedImages = nil
		m.app.ComposedFiles = nil
		m.textarea.Reset()
		return m, nil

	case "ctrl+g":
		editorCmd := m.app.ExternalEditor
		if editorCmd == "" {
			editorCmd = "vim"
		}
		return m, openExternalEditorCmd(m.textarea.Value(), editorCmd, false)

	case "ctrl+f":
		if m.app.Features.FileUpload {
			m.app.FilePickerPopupMode = true
			return m, m.filepicker.Init()
		}
		return m, nil

	case "ctrl+v", "ctrl+shift+v", "ctrl+V":
		imgBytes, contentType, err := GetClipboardImage()
		if err == nil && len(imgBytes) > 0 {
			m.app.ComposedImages = append(m.app.ComposedImages, PastedImage{
				Bytes:       imgBytes,
				ContentType: contentType,
			})
			placeholder := fmt.Sprintf("[Image %d]", len(m.app.ComposedImages))
			m.textarea.InsertString(placeholder)
			m.app.InputBuffer = m.textarea.Value()
			m.app.SetStatus("Image pasted from clipboard", 3*time.Second)
			m.app.SkipTextareaUpdate = true
			return m, nil
		}

	case "enter":
		content := strings.Trim(m.textarea.Value(), "\n\r")
		if content == "" {
			return m, nil
		}
		m.app.InputMode = false
		m.app.InputBuffer = ""
		m.textarea.Reset()

		images := m.app.ComposedImages
		m.app.ComposedImages = nil
		files := m.app.ComposedFiles
		m.app.ComposedFiles = nil

		if m.app.EditingMessageID != nil {
			m.app.SetStatus("Updating message...", 0)
		} else {
			sendingMsg := "Sending message..."
			if len(images) > 0 || len(files) > 0 {
				sendingMsg = "Uploading files and sending message..."
			}
			m.app.SetStatus(sendingMsg, 0)
		}

		// If we're viewing a Teams channel, send to that channel.
		if ch := m.activeChannelEntry(); ch != nil {
			var members []ChatMember
			if m.app.Features.ChannelMentions {
				members = m.app.TeamMembersCache[ch.teamID]
			}
			if m.app.EditingMessageID != nil {
				msgID := *m.app.EditingMessageID
				m.app.EditingMessageID = nil
				return m, updateChannelMessageCmd(m.clientID, ch.teamID, ch.channelID, msgID, content, members)
			}
			if m.app.ChannelReplyToID != "" {
				rootID := m.app.ChannelReplyToID
				m.app.ChannelReplyToID = ""
				return m, sendChannelReplyCmd(m.clientID, ch.teamID, ch.channelID, rootID, content, members, images, files)
			}
			return m, sendChannelMessageCmd(m.clientID, ch.teamID, ch.channelID, content, members, images, files)
		}

		chat := m.app.GetSelectedChat()
		if chat == nil {
			return m, nil
		}
		members := chat.Members
		if m.app.EditingMessageID != nil {
			msgID := *m.app.EditingMessageID
			m.app.EditingMessageID = nil
			return m, updateMessageCmd(m.clientID, chat.ID, msgID, content, members)
		}
		if m.app.ReplyToMessage != nil {
			ref := m.app.ReplyToMessage
			m.app.ReplyToMessage = nil
			return m, sendMessageWithRefCmd(m.clientID, chat.ID, content, ref, members, images, files)
		}
		return m, sendMessageCmd(m.clientID, chat.ID, content, members, images, files)

	case "alt+enter", "shift+enter", "ctrl+enter":
		m.textarea.InsertString("\n")
		return m, nil
	}

	// All other keys are forwarded to the textarea (handled in Update).
	return m, nil
}

func (m Model) handleSearchModeKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.app.SearchMode = false
		m.searchInput.Blur()
		return m, nil

	case "enter":
		query := strings.TrimSpace(m.searchInput.Value())
		m.app.SearchMode = false
		m.searchInput.Blur()
		if query == "" {
			m.app.SearchActive = false
			m.app.SearchQuery = ""
			m.app.SearchPopupResults = nil
			m.app.SearchPopupSelectedIndex = 0
			m.saveSearchState()
			return m, nil
		}
		m.app.SearchActive = true
		m.app.SearchQuery = query

		convID := m.activeConversationID()
		if convID != "" {
			// Clear previous search query results to start fresh
			m.app.SearchPopupResults = nil
			m.app.SearchPopupSelectedIndex = 0
			m.app.SearchPopupScrollOffset = 0

			m.RebuildSearchPopupResults()

			// Start the background API fetch loop to load missing messages from the API
			// if we haven't loaded all history yet (nextLink is not empty).
			nextLink := m.app.HistoryNextLink[convID]
			if nextLink != "" {
				if !m.app.SearchLoadingMessages {
					m.app.SetSearchLoadingMessages(true)
					m.app.SetSearchStatus(fmt.Sprintf("Searching all history for '%s'... Loaded %d messages", query, len(m.app.HistoryMessages[convID])), 0)
					m.saveSearchState()
					return m, loadMoreMessagesCmd(m.clientID, nextLink, convID, true)
				} else {
					m.app.SetSearchStatus(fmt.Sprintf("Searching all history for '%s'... (already loading background pages)", query), 0)
					m.saveSearchState()
					return m, nil
				}
			}
		}

		m.app.SetSearchStatus("Search finished.", 3*time.Second)
		m.saveSearchState()
		return m, nil
	}

	return m, nil
}

func (m Model) handleMessagePopupKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q", "v":
		var cmd tea.Cmd
		if m.app.AttachmentCursorMode {
			m.app.AttachmentCursorMode = false
			cmd = clearKittyImagesCmd()
		} else {
			m.app.MessagePopupMode = false
			m.app.AttachmentCursorMode = false
			cmd = clearKittyImagesCmd()
		}
		return m, cmd

	case "ctrl+g":
		if m.app.MessageSelectedIndex < len(m.app.Messages) {
			msgObj := m.app.Messages[m.app.MessageSelectedIndex]
			editorCmd := m.app.ExternalEditor
			if editorCmd == "" {
				editorCmd = "vim"
			}
			content := ""
			if msgObj.Body != nil && msgObj.Body.Content != nil {
				content = HTMLToMarkdown(*msgObj.Body.Content)
			}
			return m, openExternalEditorCmd(content, editorCmd, true)
		}
		return m, nil

	case "enter":
		if m.app.AttachmentCursorMode {
			// Download/open selected attachment.
			if m.app.MessageSelectedIndex < len(m.app.Messages) {
				msgObj := m.app.Messages[m.app.MessageSelectedIndex]
				vAtts := viewableAttachments(msgObj)
				if m.app.AttachmentSelectedIndex < len(vAtts) {
					att := vAtts[m.app.AttachmentSelectedIndex]
					if !m.app.Features.FilePreview {
						m.app.SetStatus("File preview disabled — enable 'file_preview_enabled' in config.json", 5*time.Second)
					} else if att.ContentURL != nil && *att.ContentURL != "" {
						name := getAttachmentSavedName(att, "attachment")
						// If this is an image and image_viewer is configured, use the
						// batch image viewer path (downloads all images in the message).
						if isImageAttachment(att) && m.app.ImageViewer != "" {
							m.app.SetStatus("Downloading images for viewer...", 0)
							return m, downloadAndOpenImagesCmd(m.clientID, att, vAtts, m.app.ImageViewer)
						}
						// Fallback: single-file download + default opener.
						destPath := filepath.Join(getDownloadsDir(), name)
						m.app.SetStatus("Downloading: "+name+" ...", 0)
						return m, downloadFileCmd(m.clientID, *att.ContentURL, destPath)
					} else {
						m.app.SetStatus("No download URL for this attachment", 3*time.Second)
					}
				}
			}
			return m, nil
		}
		m.app.MessagePopupMode = false
		m.app.AttachmentCursorMode = false
		return m, clearKittyImagesCmd()

	case "tab":
		var cmd tea.Cmd
		if m.app.MessageSelectedIndex < len(m.app.Messages) {
			msgObj := m.app.Messages[m.app.MessageSelectedIndex]
			if len(viewableAttachments(msgObj)) > 0 {
				m.app.AttachmentCursorMode = !m.app.AttachmentCursorMode
				m.app.AttachmentSelectedIndex = 0
				if m.app.AttachmentCursorMode {
					cmd = m.checkAndTriggerPreviewDownload()
				} else {
					cmd = clearKittyImagesCmd()
				}
			}
		}
		return m, cmd

	case "j", "down":
		if m.app.AttachmentCursorMode {
			if m.app.MessageSelectedIndex < len(m.app.Messages) {
				attCount := len(viewableAttachments(m.app.Messages[m.app.MessageSelectedIndex]))
				if m.app.AttachmentSelectedIndex < attCount-1 {
					m.app.AttachmentSelectedIndex++
					return m, m.checkAndTriggerPreviewDownload()
				}
			}
			return m, nil
		} else if m.app.MessageSelectedIndex > 0 {
			m.app.MessageSelectedIndex--
			m.app.MessagePopupScrollOffset = 0
			m.app.AttachmentSelectedIndex = 0
			return m, clearKittyImagesCmd()
		}

	case "k", "up":
		if m.app.AttachmentCursorMode {
			if m.app.AttachmentSelectedIndex > 0 {
				m.app.AttachmentSelectedIndex--
				return m, m.checkAndTriggerPreviewDownload()
			}
			return m, nil
		} else if m.app.MessageSelectedIndex < len(m.app.Messages)-1 {
			m.app.MessageSelectedIndex++
			m.app.MessagePopupScrollOffset = 0
			m.app.AttachmentSelectedIndex = 0
			return m, clearKittyImagesCmd()
		} else if m.app.NextLink != "" && !m.app.LoadingMessages {
			// Already at the oldest loaded message — fetch the next page.
			m.app.SetLoadingMessages(true)
			m.app.MessagePopupScrollOffset = 0
			return m, tea.Batch(
				loadMoreMessagesCmd(m.clientID, m.app.NextLink, m.activeConversationID(), false),
				clearKittyImagesCmd(),
			)
		}

	case "J", "shift+down", "pgdown":
		m.app.MessagePopupScrollOffset += 3

	case "K", "shift+up", "pgup":
		m.app.MessagePopupScrollOffset -= 3
		if m.app.MessagePopupScrollOffset < 0 {
			m.app.MessagePopupScrollOffset = 0
		}
	}
	return m, nil
}

func (m Model) handleMessageSelectionModeKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "m":
		m.app.MessageSelectionMode = false
		return m, nil

	case "j", "down":
		if m.app.MessageSelectedIndex > 0 {
			m.app.MessageSelectedIndex--
		}

	case "k", "up":
		if m.app.MessageSelectedIndex < len(m.app.Messages)-1 {
			m.app.MessageSelectedIndex++
		} else if m.app.NextLink != "" && !m.app.LoadingMessages {
			// Already at the oldest loaded message — fetch the next page.
			m.app.SetLoadingMessages(true)
			return m, loadMoreMessagesCmd(m.clientID, m.app.NextLink, m.activeConversationID(), false)
		}

	case "r":
		m.app.ReactionMode = true
		return m, nil

	case "y":
		if m.app.MessageSelectedIndex < len(m.app.Messages) {
			msgObj := m.app.Messages[m.app.MessageSelectedIndex]
			if msgObj.Body != nil && msgObj.Body.Content != nil {
				text := stripANSI(HTMLToText(*msgObj.Body.Content, msgObj.Attachments, msgObj.Mentions))
				if err := clipboard.WriteAll(text); err == nil {
					m.app.SetStatus("Message copied to clipboard", 3*time.Second)
				} else {
					m.app.SetStatus("Clipboard error: "+err.Error(), 5*time.Second)
				}
			}
			m.app.MessageSelectionMode = false
		}
		return m, nil

	case "d":
		if m.app.MessageSelectedIndex < len(m.app.Messages) {
			msgObj := m.app.Messages[m.app.MessageSelectedIndex]
			if m.isOwn(msgObj) {
				m.app.DeleteConfirmMode = true
			} else {
				m.app.SetStatus("Cannot delete messages from others", 3*time.Second)
			}
		}
		return m, nil

	case "ctrl+g":
		if m.app.MessageSelectedIndex < len(m.app.Messages) {
			msgObj := m.app.Messages[m.app.MessageSelectedIndex]
			editorCmd := m.app.ExternalEditor
			if editorCmd == "" {
				editorCmd = "vim"
			}
			content := ""
			if msgObj.Body != nil && msgObj.Body.Content != nil {
				content = HTMLToMarkdown(*msgObj.Body.Content)
			}
			return m, openExternalEditorCmd(content, editorCmd, true)
		}
		return m, nil

	case "e":
		if m.app.MessageSelectedIndex < len(m.app.Messages) {
			msgObj := m.app.Messages[m.app.MessageSelectedIndex]
			if m.isOwn(msgObj) {
				m.app.MessageSelectionMode = false
				m.app.EditingMessageID = &msgObj.ID
				m.app.InputMode = true
				content := ""
				if msgObj.Body != nil && msgObj.Body.Content != nil {
					content = HTMLToMarkdown(*msgObj.Body.Content)
				}
				m.textarea.SetValue(content)
				return m, m.textarea.Focus()
			} else {
				m.app.SetStatus("Cannot edit messages from others", 3*time.Second)
			}
		}
		return m, nil

	case "a":
		if m.app.MessageSelectedIndex < len(m.app.Messages) {
			msgObj := m.app.Messages[m.app.MessageSelectedIndex]
			m.app.MessageSelectionMode = false
			m.app.InputMode = true
			m.textarea.Reset()
			if m.activeChannelEntry() != nil {
				// Channel thread reply: resolve the root message ID.
				// If the selected message is itself a reply, reply to the same root.
				rootID := msgObj.ID
				if msgObj.IsReply && msgObj.ReplyToID != "" {
					rootID = msgObj.ReplyToID
				}
				m.app.ChannelReplyToID = rootID
			} else {
				// Chat: use the full message reference for quoted-reply formatting.
				ref := msgObj
				m.app.ReplyToMessage = &ref
			}
			return m, m.textarea.Focus()
		}
		return m, nil
	case "u":
		if m.app.MessageSelectedIndex < len(m.app.Messages) {
			msgObj := m.app.Messages[m.app.MessageSelectedIndex]
			if msgObj.Body != nil && msgObj.Body.Content != nil {
				urls := ExtractURLs(*msgObj.Body.Content)
				if len(urls) == 0 {
					m.app.SetStatus("No URLs found in message", 3*time.Second)
				} else if len(urls) == 1 {
					if err := clipboard.WriteAll(urls[0]); err == nil {
						m.app.SetStatus("URL copied to clipboard", 3*time.Second)
					}
					m.app.MessageSelectionMode = false
				} else {
					m.app.UrlSelectionMode = true
					m.app.UrlSelectionOpenMode = false
					m.app.UrlSelectedIndex = 0
					m.app.UrlsInMessage = urls
				}
			}
		}
		return m, nil
	case "o":
		if m.app.MessageSelectedIndex < len(m.app.Messages) {
			msgObj := m.app.Messages[m.app.MessageSelectedIndex]
			if msgObj.Body != nil && msgObj.Body.Content != nil {
				urls := ExtractURLs(*msgObj.Body.Content)
				if len(urls) == 0 {
					m.app.SetStatus("No URLs found in message", 3*time.Second)
				} else if len(urls) == 1 {
					m.app.MessageSelectionMode = false
					return m, openURLCmd(urls[0], m.app.BrowserCommand, m.app.YoutrackCommand, m.app.GitlabCommand)
				} else {
					m.app.UrlSelectionMode = true
					m.app.UrlSelectionOpenMode = true
					m.app.UrlSelectedIndex = 0
					m.app.UrlsInMessage = urls
				}
			}
		}
		return m, nil
	case "v":
		if len(m.app.Messages) > 0 && m.app.MessageSelectedIndex < len(m.app.Messages) {
			m.app.Messages[m.app.MessageSelectedIndex].ProcessInlineImages()
			m.app.MessagePopupMode = true
			m.app.MessagePopupScrollOffset = 0
			m.app.AttachmentCursorMode = false
			m.app.AttachmentSelectedIndex = 0
			// Auto-jump to attachment cursor mode on the first image attachment.
			msgObj := m.app.Messages[m.app.MessageSelectedIndex]
			vAtts := viewableAttachments(msgObj)
			for i, att := range vAtts {
				if isImageAttachment(att) {
					m.app.AttachmentCursorMode = true
					m.app.AttachmentSelectedIndex = i
					return m, m.checkAndTriggerPreviewDownload()
				}
			}
		}
		return m, nil

	case "p":
		// Show presence popup (requires presence_enabled feature).
		if !m.app.Features.Presence {
			m.app.SetStatus("Presence feature disabled — enable 'presence_enabled' in config.json", 5*time.Second)
			return m, nil
		}
		if m.app.MessageSelectedIndex < len(m.app.Messages) {
			msgObj := m.app.Messages[m.app.MessageSelectedIndex]
			if msgObj.From != nil && msgObj.From.User != nil && msgObj.From.User.ID != nil {
				userID := *msgObj.From.User.ID
				displayName := ""
				if msgObj.From.User.DisplayName != nil {
					displayName = *msgObj.From.User.DisplayName
				}
				m.app.PresencePopupMode = true
				m.app.PresenceData = nil
				m.app.PresenceLoading = true
				m.app.PresenceUserName = displayName
				return m, loadPresenceCmd(m.clientID, userID, displayName)
			}
		}
		return m, nil

	case "i":
		// Show user profile popup (requires user_profile_enabled feature).
		if !m.app.Features.UserProfile {
			m.app.SetStatus("User profile feature disabled — enable 'user_profile_enabled' in config.json", 5*time.Second)
			return m, nil
		}
		if m.app.MessageSelectedIndex < len(m.app.Messages) {
			msgObj := m.app.Messages[m.app.MessageSelectedIndex]
			if msgObj.From != nil && msgObj.From.User != nil && msgObj.From.User.ID != nil {
				userID := *msgObj.From.User.ID
				m.app.UserProfilePopupMode = true
				m.app.UserProfileData = nil
				m.app.UserProfileLoading = true
				return m, loadUserProfileCmd(m.clientID, userID)
			}
		}
		return m, nil
	}

	return m, nil
}

func (m Model) handleReactionModeKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "r":
		m.app.ReactionMode = false
		return m, nil

	case "1", "2", "3", "4", "5", "6":
		types := []string{"👍", "❤️", "😂", "😮", "😢", "😡"}
		idx := int(msg.String()[0] - '1')
		if idx >= 0 && idx < len(types) {
			reactionType := types[idx]
			if m.app.MessageSelectedIndex < len(m.app.Messages) {
				msgObj := m.app.Messages[m.app.MessageSelectedIndex]

				// Check if current user already has this reaction.
				hasReaction := false
				if m.app.CurrentUserName != nil {
					for _, r := range msgObj.Reactions {
						rType := strings.ToLower(r.ReactionType)
						targetType := strings.ToLower(reactionType)
						// Match either keyword or emoji directly.
						match := rType == targetType ||
							(rType == "like" && targetType == "👍") ||
							(rType == "heart" && targetType == "❤️") ||
							(rType == "laugh" && targetType == "😂") ||
							(rType == "surprised" && targetType == "😮") ||
							(rType == "sad" && targetType == "😢") ||
							(rType == "angry" && targetType == "😡")

						if match && r.User != nil && r.User.User != nil &&
							r.User.User.ID != nil &&
							*r.User.User.ID == m.app.CurrentUserID {
							hasReaction = true
							break
						}
					}
				}

				m.app.ReactionMode = false
				m.app.MessageSelectionMode = false
				if ch := m.activeChannelEntry(); ch != nil {
					if hasReaction {
						return m, unsetChannelReactionCmd(m.clientID, ch.teamID, ch.channelID, msgObj.ID, reactionType)
					}
					return m, setChannelReactionCmd(m.clientID, ch.teamID, ch.channelID, msgObj.ID, reactionType)
				}
				chat := m.app.GetSelectedChat()
				if chat != nil {
					if hasReaction {
						return m, unsetReactionCmd(m.clientID, chat.ID, msgObj.ID, reactionType)
					}
					return m, setReactionCmd(m.clientID, chat.ID, msgObj.ID, reactionType)
				}
			}
		}
	}
	return m, nil
}

func (m Model) handleDeleteConfirmModeKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		m.app.DeleteConfirmMode = false
		m.app.MessageSelectionMode = false
		if m.app.MessageSelectedIndex < len(m.app.Messages) {
			msgObj := m.app.Messages[m.app.MessageSelectedIndex]
			if ch := m.activeChannelEntry(); ch != nil {
				return m, deleteChannelMessageCmd(m.clientID, ch.teamID, ch.channelID, msgObj.ID)
			}
			if chat := m.app.GetSelectedChat(); chat != nil {
				return m, deleteMessageCmd(m.clientID, chat.ID, msgObj.ID)
			}
		}
	case "n", "N", "esc":
		m.app.DeleteConfirmMode = false
	}
	return m, nil
}

func (m Model) updateUrlSelection(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		m.app.UrlSelectionMode = false
		return m, nil

	case "j", "down":
		if m.app.UrlSelectedIndex < len(m.app.UrlsInMessage)-1 {
			m.app.UrlSelectedIndex++
		}

	case "k", "up":
		if m.app.UrlSelectedIndex > 0 {
			m.app.UrlSelectedIndex--
		}

	case "enter":
		url := m.app.UrlsInMessage[m.app.UrlSelectedIndex]
		m.app.UrlSelectionMode = false
		m.app.MessageSelectionMode = false
		if m.app.UrlSelectionOpenMode {
			return m, openURLCmd(url, m.app.BrowserCommand, m.app.YoutrackCommand, m.app.GitlabCommand)
		} else {
			if err := clipboard.WriteAll(url); err == nil {
				m.app.SetStatus("URL copied to clipboard", 3*time.Second)
			}
			return m, nil
		}

	case "y":
		if !m.app.UrlSelectionOpenMode {
			url := m.app.UrlsInMessage[m.app.UrlSelectedIndex]
			m.app.UrlSelectionMode = false
			m.app.MessageSelectionMode = false
			if err := clipboard.WriteAll(url); err == nil {
				m.app.SetStatus("URL copied to clipboard", 3*time.Second)
			}
		}

	case "o":
		if m.app.UrlSelectionOpenMode {
			url := m.app.UrlsInMessage[m.app.UrlSelectedIndex]
			m.app.UrlSelectionMode = false
			m.app.MessageSelectionMode = false
			return m, openURLCmd(url, m.app.BrowserCommand, m.app.YoutrackCommand, m.app.GitlabCommand)
		}
	}
	return m, nil
}

func (m Model) handleUrlSelectionModeKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		m.app.UrlSelectionMode = false
		return m, nil

	case "j", "down":
		if m.app.UrlSelectedIndex < len(m.app.UrlsInMessage)-1 {
			m.app.UrlSelectedIndex++
		}

	case "k", "up":
		if m.app.UrlSelectedIndex > 0 {
			m.app.UrlSelectedIndex--
		}

	case "enter":
		url := m.app.UrlsInMessage[m.app.UrlSelectedIndex]
		m.app.UrlSelectionMode = false
		m.app.MessageSelectionMode = false
		if m.app.UrlSelectionOpenMode {
			return m, openURLCmd(url, m.app.BrowserCommand, m.app.YoutrackCommand, m.app.GitlabCommand)
		} else {
			if err := clipboard.WriteAll(url); err == nil {
				m.app.SetStatus("URL copied to clipboard", 3*time.Second)
			}
			return m, nil
		}

	case "y":
		if !m.app.UrlSelectionOpenMode {
			url := m.app.UrlsInMessage[m.app.UrlSelectedIndex]
			m.app.UrlSelectionMode = false
			m.app.MessageSelectionMode = false
			if err := clipboard.WriteAll(url); err == nil {
				m.app.SetStatus("URL copied to clipboard", 3*time.Second)
			}
		}

	case "o":
		if m.app.UrlSelectionOpenMode {
			url := m.app.UrlsInMessage[m.app.UrlSelectedIndex]
			m.app.UrlSelectionMode = false
			m.app.MessageSelectionMode = false
			return m, openURLCmd(url, m.app.BrowserCommand, m.app.YoutrackCommand, m.app.GitlabCommand)
		}
	}
	return m, nil
}

// Update is the Bubble Tea update function — processes messages and returns
// the new model plus any commands.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	updatedModel, cmd := m.updateInternal(msg)

	// A heartbeat tick that changed nothing cannot have changed the unread
	// counts or the rendered view either, so skip both. Everything else falls
	// through to the full path — defaulting to "do the work" keeps this safe
	// when new message types are added.
	_, isTick := msg.(MsgTick)
	idleTick := isTick && !updatedModel.tickDidWork

	if !idleTick {
		// Scanning every chat for unread messages/reactions is O(chats) and
		// costs more than a repaint on large accounts; it only needs to run
		// when something might actually have changed.
		updatedModel = updatedModel.writeAppState()
		if updatedModel.viewCache != nil {
			updatedModel.viewCache.dirty = true
		}
	}
	updatedModel.tickDidWork = false

	return updatedModel, cmd
}

func (m Model) writeAppState() Model {
	newMessages := 0
	newReactions := 0

	for _, c := range m.app.Chats {
		if m.isUnread(c) {
			newMessages++
		}
		if m.hasUnreadReactions(c) {
			newReactions++
		}
	}

	if m.app.Features.TeamsChannels {
		for _, entry := range m.allChannels() {
			lastID := m.lastMsgID[entry.channelID]
			if lastID != "" && m.lastReadMsgID[entry.channelID] != lastID {
				newMessages++
			}
		}
	}

	// Only write if something has changed.
	if m.lastWrittenMessages == newMessages && m.lastWrittenReactions == newReactions {
		return m
	}

	m.lastWrittenMessages = newMessages
	m.lastWrittenReactions = newReactions

	type StateJSON struct {
		NewMessages  int `json:"new_messages"`
		NewReactions int `json:"new_reactions"`
	}

	data := StateJSON{
		NewMessages:  newMessages,
		NewReactions: newReactions,
	}

	jsonData, err := json.MarshalIndent(data, "", "    ")
	if err == nil {
		if cacheDir, err := GetCacheDir(); err == nil {
			stateFile := filepath.Join(cacheDir, "state.json")
			_ = os.WriteFile(stateFile, jsonData, 0o644)
		}
	}

	return m
}

// ---------------------------------------------------------------------------
// Main View
// ---------------------------------------------------------------------------

// View returns the rendered TUI, reusing the previous render when nothing has
// changed since it was produced.
//
// Bubble Tea calls View() after every message, and the 100ms heartbeat tick
// means that happens 10x/sec even on a completely idle screen. Rendering costs
// ~1-3ms depending on history size, which burned a few percent of a CPU core
// continuously. Bubble Tea already skips the terminal write when the output is
// unchanged, but it does so only after calling View(), so the work still had to
// be avoided here.
//
// The render functions are pure with respect to the terminal — they compute
// strings from model state without performing I/O — so skipping a redundant
// call is unobservable. Update() marks the cache dirty for everything except a
// tick that did no work.
func (m Model) View() string {
	if m.viewCache != nil && !m.viewCache.dirty &&
		m.viewCache.w == m.width && m.viewCache.h == m.height {
		return m.viewCache.view
	}

	out := m.renderView()

	if m.viewCache != nil {
		m.viewCache.dirty = false
		m.viewCache.w = m.width
		m.viewCache.h = m.height
		m.viewCache.view = out
	}
	return out
}

// renderView builds the complete TUI from scratch.
func (m Model) renderView() string {
	if m.width == 0 {
		return "Loading..."
	}

	chatW := chatPanelWidth(m.width)
	msgW := msgPanelWidth(m.width)
	// Account for borders on both panels.
	innerH := m.height - 5 // subtract status bar (3) + border rows (2)
	if innerH < 1 {
		innerH = 1
	}

	chatPanel := m.renderChatList(chatW-2, innerH)

	right := m.renderRightPanel(msgW-2, innerH)

	left := normalBorder.Width(chatW - 2).Height(innerH).Render(chatPanel)

	top := lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	mainView := lipgloss.JoinVertical(lipgloss.Left, top, m.renderStatusBar(m.width))

	var result string
	if m.app.UrlSelectionMode {
		modal := m.renderUrlSelection(m.width, m.height)
		result = lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, modal)
	} else if m.app.FilePickerPopupMode {
		popupW := m.width * 85 / 100
		popupH := m.height * 80 / 100
		if popupW < 45 {
			popupW = 45
		}
		if popupH < 15 {
			popupH = 15
		}
		modal := m.renderFilePickerPopup(popupW, popupH)
		result = lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, modal)
	} else if m.app.UserSearchPopupMode {
		popupW := m.width * 85 / 100
		popupH := m.height * 80 / 100
		if popupW < 40 {
			popupW = 40
		}
		if popupH < 10 {
			popupH = 10
		}
		modal := m.renderUserSearchPopup(popupW, popupH)
		result = lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, modal)
	} else if m.app.SearchPopupMode {
		popupW := m.width * 85 / 100
		popupH := m.height * 80 / 100
		if popupW < 40 {
			popupW = 40
		}
		if popupH < 10 {
			popupH = 10
		}
		modal := m.renderSearchPopup(popupW, popupH)
		result = lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, modal)
	} else if m.app.HelpPopupMode {
		popupW := m.width * 70 / 100
		popupH := m.height * 85 / 100
		if popupW < 50 {
			popupW = 50
		}
		if popupH < 10 {
			popupH = 10
		}
		modal := m.renderHelpPopup(popupW, popupH)
		result = lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, modal)
	} else if m.app.PresencePopupMode {
		var popupW, popupH int
		if m.app.PresenceChatMode {
			popupW = m.width * 70 / 100
			popupH = m.height * 65 / 100
		} else {
			popupW = m.width * 55 / 100
			popupH = m.height * 40 / 100
		}
		if popupW < 40 {
			popupW = 40
		}
		if popupH < 10 {
			popupH = 10
		}
		modal := m.renderPresencePopup(popupW, popupH)
		result = lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, modal)
	} else if m.app.UserProfilePopupMode {
		popupW := m.width * 60 / 100
		popupH := m.height * 50 / 100
		if popupW < 40 {
			popupW = 40
		}
		if popupH < 10 {
			popupH = 10
		}
		modal := m.renderUserProfilePopup(popupW, popupH)
		result = lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, modal)
	} else if m.app.MessagePopupMode {
		popupW := m.width * 85 / 100
		popupH := m.height * 80 / 100
		if popupW < 40 {
			popupW = 40
		}
		if popupH < 10 {
			popupH = 10
		}
		modal := m.renderMessagePopup(popupW, popupH)
		result = lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, modal)
	} else {
		result = mainView
	}

	var kittySeq string
	if m.app.MessagePopupMode && m.app.Features.FilePreviewInTerminal && m.app.AttachmentCursorMode {
		if m.app.MessageSelectedIndex >= 0 && m.app.MessageSelectedIndex < len(m.app.Messages) {
			msgObj := m.app.Messages[m.app.MessageSelectedIndex]
			vAtts := viewableAttachments(msgObj)
			if m.app.AttachmentSelectedIndex >= 0 && m.app.AttachmentSelectedIndex < len(vAtts) {
				att := vAtts[m.app.AttachmentSelectedIndex]
				if isImageAttachment(att) {
					if cp, err := getAttachmentCachePath(att); err == nil {
						if _, err := os.Stat(cp); err == nil {
							popupW := m.width * 85 / 100
							popupH := m.height * 80 / 100
							if popupW < 40 {
								popupW = 40
							}
							if popupH < 10 {
								popupH = 10
							}

							popupX := (m.width - popupW) / 2
							popupY := (m.height - popupH) / 2

							innerW := popupW - 6
							innerH := popupH - 4
							if innerW < 10 {
								innerW = 10
							}
							if innerH < 4 {
								innerH = 4
							}

							previewW := innerW * 45 / 100
							if previewW < 15 {
								previewW = 15
							}
							if previewW > innerW-25 {
								previewW = innerW - 25
							}

							if previewW >= 15 {
								leftW := innerW - previewW - 2
								rightPanelX := popupX + 3 + leftW + 2
								imgX := rightPanelX + 1
								imgY := popupY + 3
								imgW := previewW - 2
								imgH := (innerH - 1) - 2 // targetH - 2

								// Clear all, then draw image
								kittySeq = "\x1b_Ga=d,d=a\x1b\\" + kittyImageSequence(cp, imgX, imgY, imgW, imgH)
							}
						}
					}
				}
			}
		}
	}

	return result + kittySeq
}

// renderRightPanel renders the messages panel (with optional input area).
func (m Model) renderRightPanel(w, h int) string {
	if m.app.SelectedIndex < 0 && m.channelSelectedIndex < 0 {
		idleMsg := "💤 Sleep Mode\n\nNo chat selected. Polling is paused.\n\nPress 'j' or 'k' to select a chat\nand resume polling."
		msgContent := lipgloss.Place(w, h-2, lipgloss.Center, lipgloss.Center,
			lipgloss.NewStyle().Foreground(colDimGray).Align(lipgloss.Center).Render(idleMsg),
		)
		return normalBorder.Width(w).Height(h).
			BorderForeground(colDimGray).
			Render(lipgloss.JoinVertical(lipgloss.Left,
				lipgloss.NewStyle().Foreground(colDimGray).Render("Idle"),
				msgContent,
			))
	}

	if !m.app.InputMode {
		title := "Messages (i:compose, m:select, K/J:scroll, /:search, ?:help, ESC:sleep mode)"
		if m.channelSelectedIndex >= 0 {
			chans := m.allChannels()
			if m.channelSelectedIndex < len(chans) {
				entry := chans[m.channelSelectedIndex]
				title = lipgloss.NewStyle().Foreground(lipgloss.Color("#5F87FF")).Bold(true).Render("#") +
					" " + entry.teamName + " » " + entry.channelName +
					lipgloss.NewStyle().Foreground(colDimGray).Render("  (K/J:scroll, m:select, ?:help)")
			}
		} else if m.app.MessageSelectionMode {
			title = "MESSAGE MODE (j/k:nav, r:react, y:yank, u:url, o:open, d:delete, e:edit, a:answer, v:view, ctrl+g: editor, p:presence, i:profile, ESC/m:exit)"
		}
		msgContent := m.renderMessages(w, h-1)
		return normalBorder.Width(w).Height(h).
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(colGreen).
			Render(lipgloss.JoinVertical(lipgloss.Left,
				lipgloss.NewStyle().Foreground(colDimGray).Render(title),
				msgContent,
			))
	}

	// Input mode: split height between messages and textarea.
	// When replying, add 2 extra lines for the quote preview.
	inputH := 5
	if m.app.ReplyToMessage != nil {
		inputH = 7
	}

	mentionH := 0
	var mentionView string
	if m.app.MentionPopupMode && len(m.app.MentionSuggestions) > 0 {
		limit := 5
		if len(m.app.MentionSuggestions) < limit {
			limit = len(m.app.MentionSuggestions)
		}
		mentionH = limit + 2

		var items []string
		start := m.app.MentionScrollOffset
		for i := start; i < start+limit; i++ {
			if i < 0 || i >= len(m.app.MentionSuggestions) {
				continue
			}
			sug := m.app.MentionSuggestions[i]
			displayName := ""
			if sug.DisplayName != nil {
				displayName = *sug.DisplayName
			}
			email := ""
			if sug.Email != nil {
				email = *sug.Email
			}

			line := fmt.Sprintf(" %s", displayName)
			if email != "" {
				line += fmt.Sprintf(" (%s)", email)
			}

			if i == m.app.MentionSelectedIndex {
				line = lipgloss.NewStyle().
					Background(lipgloss.Color("#5F87FF")).
					Foreground(lipgloss.Color("#FFFFFF")).
					Bold(true).
					Render(" >" + line)
			} else {
				line = lipgloss.NewStyle().Foreground(lipgloss.Color("#E2E2E2")).Render("  " + line)
			}
			items = append(items, line)
		}

		mentionView = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(colYellow).
			Width(w).
			Height(limit).
			Render(lipgloss.JoinVertical(lipgloss.Left, items...))
	}

	msgH := h - inputH - mentionH - 1
	if msgH < 1 {
		msgH = 1
	}

	msgContent := m.renderMessages(w, msgH-1)
	title := "Messages (ESC to cancel)"
	if m.app.EditingMessageID != nil {
		title = "EDITING MESSAGE (ESC to cancel)"
	} else if m.app.ChannelReplyToID != "" {
		title = "REPLYING TO THREAD (ESC to cancel)"
	} else if m.app.ReplyToMessage != nil {
		ref := m.app.ReplyToMessage
		sender := "someone"
		if ref.From != nil && ref.From.User != nil && ref.From.User.DisplayName != nil {
			sender = *ref.From.User.DisplayName
			if m.isOwn(*ref) {
				sender = "yourself"
			}
		}
		title = "REPLYING TO " + sender + " (ESC to cancel)"
	}
	msgBox := normalBorder.Width(w).Height(msgH).
		Render(lipgloss.JoinVertical(lipgloss.Left,
			lipgloss.NewStyle().Foreground(colDimGray).Render(title),
			msgContent,
		))

	m.textarea.SetWidth(w)
	m.textarea.SetHeight(inputH - 2)

	// Build input box contents — add quote preview when replying.
	hintText := "Type your message (Enter: send, Alt+Enter: new line, ESC: cancel, @: mention, paste IMAGE"
	if m.app.Features.FileUpload {
		hintText += ", Ctrl+f: attach file"
	}
	hintText += ", Ctrl+g: open external editor)"

	hintLine := lipgloss.NewStyle().Foreground(colDimGray).Render(hintText)
	inputParts := []string{hintLine}

	if m.app.ReplyToMessage != nil {
		ref := m.app.ReplyToMessage
		// Build a one-line preview using the same style as renderMessageReference.
		preview := ""
		if ref.Body != nil && ref.Body.Content != nil {
			preview = stripANSI(HTMLToText(*ref.Body.Content, ref.Attachments, ref.Mentions))
		}
		preview = strings.ReplaceAll(preview, "\n", " ")
		const maxPrev = 80
		if len([]rune(preview)) > maxPrev {
			preview = string([]rune(preview)[:maxPrev]) + "…"
		}
		sender := ""
		if ref.From != nil && ref.From.User != nil && ref.From.User.DisplayName != nil {
			sender = *ref.From.User.DisplayName
			if m.isOwn(*ref) {
				sender = "Me"
			}
		}
		bar := lipgloss.NewStyle().Foreground(lipgloss.Color("#4A90D9")).Bold(true).Render("▎")
		name := lipgloss.NewStyle().Foreground(lipgloss.Color("#7EC8E3")).Bold(true).Render(sender)
		text := lipgloss.NewStyle().Foreground(lipgloss.Color("#6C7A89")).Render(": " + preview)
		quoteLine := bar + " " + name + text
		inputParts = append(inputParts, quoteLine)
		// Separator between quote and textarea.
		inputParts = append(inputParts, lipgloss.NewStyle().Foreground(colDimGray).Render(strings.Repeat("─", w)))
		m.textarea.SetHeight(inputH - 4) // hint + quote + separator lines
	}

	inputParts = append(inputParts, m.textarea.View())

	inputBox := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(colGreen).
		Width(w).Height(inputH - 1).
		Render(lipgloss.JoinVertical(lipgloss.Left, inputParts...))

	if mentionView != "" {
		return lipgloss.JoinVertical(lipgloss.Left, msgBox, mentionView, inputBox)
	}
	return lipgloss.JoinVertical(lipgloss.Left, msgBox, inputBox)
}

// ---------------------------------------------------------------------------
// Chat list rendering
// ---------------------------------------------------------------------------

// chatTypeToIcon maps a Graph API chatType string to an icon or label based on configuration.
func (m Model) chatTypeToIcon(chatType string) string {
	// 1. Check custom overrides
	if icon, ok := m.app.CustomChatIcons[chatType]; ok {
		return icon
	}
	if chatType != "default" {
		if icon, ok := m.app.CustomChatIcons["default"]; ok {
			return icon
		}
	}

	// 2. Preset themes
	switch m.app.ChatIconTheme {
	case "emoji":
		switch chatType {
		case "oneOnOne":
			return "👤"
		case "group":
			return "👥"
		case "meeting":
			return "📅"
		default:
			return "💬"
		}
	case "text":
		switch chatType {
		case "oneOnOne":
			return "[oneOnOne]"
		case "group":
			return "[group]"
		case "meeting":
			return "[meeting]"
		case "channel":
			return "[channel]"
		default:
			return "[" + chatType + "]"
		}
	case "unicode":
		fallthrough
	default:
		switch chatType {
		case "oneOnOne":
			return "◉"
		case "group":
			return "⊞"
		case "meeting":
			return "⊛"
		case "channel":
			return "☰"
		default:
			return "◈"
		}
	}
}

// sortTeamsAndChannels is a no-op because sorting is now performed globally inside allChannels.
func (m Model) sortTeamsAndChannels() {}

// channelEntry is a flat representation of one Team channel for sidebar navigation.
type channelEntry struct {
	teamID      string
	teamName    string
	channelID   string
	channelName string
}

// allChannels returns the flat ordered list of all channels across all teams,
// sorted globally: unhidden channels first (sorted by last message activity),
// followed by hidden channels (sorted in their original default order).
func (m Model) allChannels() []channelEntry {
	var list []channelEntry
	for _, twc := range m.app.TeamsData {
		for _, ch := range twc.Channels {
			list = append(list, channelEntry{
				teamID:      twc.Team.ID,
				teamName:    twc.Team.DisplayName,
				channelID:   ch.ID,
				channelName: ch.DisplayName,
			})
		}
	}

	sort.Slice(list, func(i, j int) bool {
		idA := list[i].channelID
		idB := list[j].channelID
		isAHidden := !m.unhiddenChannels[idA]
		isBHidden := !m.unhiddenChannels[idB]

		if isAHidden != isBHidden {
			return !isAHidden
		}

		if !isAHidden {
			timeA := m.lastMsgTime[idA]
			timeB := m.lastMsgTime[idB]
			if !timeA.Equal(timeB) {
				return timeA.After(timeB)
			}
		}

		// Fallback to original default order
		if len(m.originalChannelIndex) == 0 {
			if list[i].teamName != list[j].teamName {
				return list[i].teamName < list[j].teamName
			}
			return list[i].channelName < list[j].channelName
		}
		flatIndexA := m.originalTeamIndex[list[i].teamID]*1000 + m.originalChannelIndex[idA]
		flatIndexB := m.originalTeamIndex[list[j].teamID]*1000 + m.originalChannelIndex[idB]
		return flatIndexA < flatIndexB
	})

	return list
}

// activeChannelEntry returns the currently selected channelEntry when in channel
// mode (channelSelectedIndex >= 0), or nil if the user is in chat mode.
func (m Model) activeChannelEntry() *channelEntry {
	if m.channelSelectedIndex < 0 {
		return nil
	}
	chans := m.allChannels()
	if m.channelSelectedIndex >= len(chans) {
		return nil
	}
	e := chans[m.channelSelectedIndex]
	return &e
}

// activeConversationID returns the active chat ID or channel ID, or "" if none.
func (m Model) activeConversationID() string {
	if m.channelSelectedIndex >= 0 {
		if entry := m.activeChannelEntry(); entry != nil {
			return entry.channelID
		}
		return m.app.SelectedChannelID
	}
	if chat := m.app.GetSelectedChat(); chat != nil {
		return chat.ID
	}
	return ""
}

func (m Model) renderChatList(w, h int) string {
	titleText := "Chats (j/k: nav, c: find, f: ★ fav, q: quit)"
	if m.app.Features.TeamsChannels {
		titleText = "Chats (j/k: nav, Tab: switch, c: find, f: ★ fav, q: quit)"
	}
	title := lipgloss.NewStyle().Foreground(colDimGray).Render(titleText)

	if len(m.app.Chats) == 0 {
		return lipgloss.JoinVertical(lipgloss.Left, title, m.app.Status)
	}

	// Total lines available = h minus the title row.
	budget := h - 1
	if budget < 1 {
		budget = 1
	}

	// ── Calculate how many lines the Teams section will consume ──────────
	chans := m.allChannels()
	teamsLines := 0 // lines consumed by the Teams section (divider + entries)
	if m.app.Features.TeamsChannels {
		if m.app.TeamsDataLoading {
			teamsLines = 1 // just the loading divider
		} else if len(chans) > 0 {
			teamsLines = 1 + len(chans) // divider + all channel rows
		}
	}

	// Give the Teams section at most half the budget so chats always remain visible.
	if teamsLines > budget/2 {
		teamsLines = budget / 2
		if teamsLines < 1 {
			teamsLines = 1
		}
	}

	chatBudget := budget - teamsLines
	if chatBudget < 1 {
		chatBudget = 1
	}

	// ── Chats section ────────────────────────────────────────────────────
	if m.app.SelectedIndex >= 0 {
		padding := 3
		maxPadding := (chatBudget - 1) / 2
		if padding > maxPadding {
			padding = maxPadding
		}
		if padding < 0 {
			padding = 0
		}

		if m.app.SelectedIndex < m.app.ChatScrollOffset+padding {
			m.app.ChatScrollOffset = m.app.SelectedIndex - padding
		} else if m.app.SelectedIndex >= m.app.ChatScrollOffset+chatBudget-padding {
			m.app.ChatScrollOffset = m.app.SelectedIndex - chatBudget + 1 + padding
		}

		// Clamp ChatScrollOffset to [0, len(m.app.Chats) - chatBudget]
		maxOffset := len(m.app.Chats) - chatBudget
		if maxOffset < 0 {
			maxOffset = 0
		}
		if m.app.ChatScrollOffset > maxOffset {
			m.app.ChatScrollOffset = maxOffset
		}
		if m.app.ChatScrollOffset < 0 {
			m.app.ChatScrollOffset = 0
		}
	}

	lines := []string{title}

	start := m.app.ChatScrollOffset
	if start < 0 {
		start = 0
	}
	end := start + chatBudget
	if end > len(m.app.Chats) {
		end = len(m.app.Chats)
	}

	for i := start; i < end; i++ {
		c := m.app.Chats[i]
		chatTypeIcon := m.chatTypeToIcon(c.ChatType)
		displayName := ""
		if c.CachedDisplayName != nil {
			displayName = *c.CachedDisplayName
		}

		unread := m.isUnread(c)
		reactionEmoji := m.getLatestUnreadReactionEmoji(c)
		isFav := m.favourites[c.ID]

		prefix := ""
		if isFav {
			prefix += "★ "
		}
		if unread {
			prefix += "● "
		}
		if reactionEmoji != "" {
			prefix += reactionEmoji + " "
		}

		labelStr := prefix + chatTypeIcon + " " + displayName

		var label string
		if i == m.app.SelectedIndex && m.channelSelectedIndex < 0 {
			label = lipgloss.NewStyle().
				Foreground(colYellow).
				Bold(unread || reactionEmoji != "").
				Background(colDarkGray).
				Width(w).
				MaxWidth(w).
				Render(labelStr)
		} else {
			typeTag := lipgloss.NewStyle().Foreground(colCyan).Render(chatTypeIcon)
			base := typeTag + " " + displayName
			if isFav {
				star := lipgloss.NewStyle().Foreground(colYellow).Render("★ ")
				base = star + base
			}
			if unread || reactionEmoji != "" {
				pfx := ""
				if unread {
					pfx += "● "
				}
				if reactionEmoji != "" {
					pfx += reactionEmoji + " "
				}
				base = lipgloss.NewStyle().Bold(true).Render(pfx) + base
			}
			label = lipgloss.NewStyle().MaxWidth(w).Render(base)
		}
		lines = append(lines, label)
	}

	// ── Teams channels section ───────────────────────────────────────────
	if m.app.Features.TeamsChannels && teamsLines > 0 {
		if m.app.TeamsDataLoading {
			divider := lipgloss.NewStyle().Foreground(colDimGray).Render("Teams (loading…)")
			lines = append(lines, divider)
		} else if len(chans) > 0 {
			dividerText := "Teams"
			if m.channelSelectedIndex >= 0 {
				dividerText = "Teams (h: toggle hide)"
			}
			divider := lipgloss.NewStyle().Foreground(colDimGray).Render(dividerText)
			lines = append(lines, divider)

			// Channel rows available = teamsLines - 1 (divider).
			chanVisible := teamsLines - 1
			if chanVisible < 1 {
				chanVisible = 1
			}

			// Clamp ChannelScrollOffset so the selected entry stays visible with padding.
			if m.channelSelectedIndex >= 0 {
				chanPadding := 3
				maxChanPadding := (chanVisible - 1) / 2
				if chanPadding > maxChanPadding {
					chanPadding = maxChanPadding
				}
				if chanPadding < 0 {
					chanPadding = 0
				}

				if m.channelSelectedIndex < m.app.ChannelScrollOffset+chanPadding {
					m.app.ChannelScrollOffset = m.channelSelectedIndex - chanPadding
				} else if m.channelSelectedIndex >= m.app.ChannelScrollOffset+chanVisible-chanPadding {
					m.app.ChannelScrollOffset = m.channelSelectedIndex - chanVisible + 1 + chanPadding
				}

				// Clamp ChannelScrollOffset to [0, len(chans) - chanVisible]
				maxChanOffset := len(chans) - chanVisible
				if maxChanOffset < 0 {
					maxChanOffset = 0
				}
				if m.app.ChannelScrollOffset > maxChanOffset {
					m.app.ChannelScrollOffset = maxChanOffset
				}
				if m.app.ChannelScrollOffset < 0 {
					m.app.ChannelScrollOffset = 0
				}
			}

			cStart := m.app.ChannelScrollOffset
			if cStart < 0 {
				cStart = 0
			}
			cEnd := cStart + chanVisible
			if cEnd > len(chans) {
				cEnd = len(chans)
			}
			for ci := cStart; ci < cEnd; ci++ {
				entry := chans[ci]
				isHidden := !m.unhiddenChannels[entry.channelID]
				unread := m.lastMsgID[entry.channelID] != "" && m.lastReadMsgID[entry.channelID] != m.lastMsgID[entry.channelID]

				prefix := ""
				if unread {
					prefix = "● "
				}

				var label string
				if ci == m.channelSelectedIndex {
					label = lipgloss.NewStyle().
						Foreground(colYellow).
						Background(colDarkGray).
						Bold(unread).
						Width(w).
						MaxWidth(w).
						Render(prefix + "# " + entry.teamName + " » " + entry.channelName)
				} else {
					var textStyle lipgloss.Style
					if isHidden {
						textStyle = lipgloss.NewStyle().Foreground(colDimGray)
					} else {
						textStyle = lipgloss.NewStyle()
					}
					if unread {
						textStyle = textStyle.Bold(true)
					}

					var icon string
					if isHidden {
						icon = lipgloss.NewStyle().Foreground(colDimGray).Render("#")
					} else {
						icon = lipgloss.NewStyle().Foreground(lipgloss.Color("#5F87FF")).Render("#")
					}

					labelStr := prefix + icon + " " + entry.teamName + " » " + entry.channelName
					label = textStyle.MaxWidth(w).Render(labelStr)
				}
				lines = append(lines, label)
			}
		}
	}

	return strings.Join(lines, "\n")
}


// ---------------------------------------------------------------------------
// Messages rendering
// ---------------------------------------------------------------------------

func (m Model) renderMessages(w, h int) string {
	if m.app.LoadingMessages && len(m.app.Messages) == 0 {
		return lipgloss.NewStyle().Foreground(colDimGray).Render("Loading messages...")
	}
	if len(m.app.Messages) == 0 {
		return lipgloss.NewStyle().Foreground(colDimGray).Render("No messages.")
	}

	maxW := w * 9 / 10 // 90% of panel width

	// Messages arrive newest-first from API; render newest at the bottom.
	msgs := m.app.Messages
	start := 0
	msgs = msgs[start:]

	var lines []string
	var prevSender string
	var prevTime time.Time

	var selectedStartLine, selectedEndLine int = -1, -1
	var pendingScrollLine int = -1

	m.app.MessageLineOffsets = make([]int, len(msgs))

	// Iterate in reverse (slice is newest-first) → append → shows newest at bottom.
	for i := len(msgs) - 1; i >= 0; i-- {
		msg := msgs[i]
		m.app.MessageLineOffsets[i] = len(lines)

		alignRight := false
		if m.channelSelectedIndex >= 0 {
			alignRight = msg.IsReply
		} else {
			alignRight = m.isOwn(msg)
		}

		if m.app.PendingScrollID != "" && msg.ID == m.app.PendingScrollID {
			pendingScrollLine = len(lines)
		}

		sender := ""
		if msg.From != nil && msg.From.User != nil && msg.From.User.DisplayName != nil {
			sender = *msg.From.User.DisplayName
		}

		msgTime, _ := time.Parse(time.RFC3339Nano, msg.CreatedDateTime)
		msgTime = msgTime.Local()
		senderChanged := sender != prevSender
		timeGap := !msgTime.IsZero() && !prevTime.IsZero() && (msgTime.Year() != prevTime.Year() ||
			msgTime.Month() != prevTime.Month() ||
			msgTime.Day() != prevTime.Day() ||
			msgTime.Hour() != prevTime.Hour())

		// Channel root messages always get their own header.
		// Channel replies and regular chat messages group by sender/hour.
		showHeader := senderChanged || timeGap || (m.channelSelectedIndex >= 0 && !msg.IsReply)

		if showHeader {
			if len(lines) > 0 {
				lines = append(lines, "")
			}
			dateStr := ""
			if !msgTime.IsZero() {
				dateStr = msgTime.Format("Jan 02 15:04")
			}
			var header string
			senderName := sender
			if m.isOwn(msg) {
				senderName = "Me"
			}
			if m.app.SearchActive && m.app.SearchQuery != "" {
				senderName = highlightQuery(senderName, m.app.SearchQuery)
			}

			if msg.IsReply {
				if alignRight {
					// Right-aligned reply.
					color := lipgloss.Color("#5F87AF") // others reply color (blueish)
					if m.isOwn(msg) {
						color = lipgloss.Color("#5FAF87") // own reply color (greenish)
					}
					h := lipgloss.NewStyle().Foreground(color).Render(dateStr + " " + senderName + " ↳")
					header = padLeft(h, w)
				} else {
					// Left-aligned reply.
					color := lipgloss.Color("#5F87AF") // others reply color (blueish)
					if m.isOwn(msg) {
						color = lipgloss.Color("#5FAF87") // own reply color (greenish)
					}
					replyPrefix := lipgloss.NewStyle().Foreground(colDimGray).Render("  ↳ ")
					header = replyPrefix + lipgloss.NewStyle().Foreground(color).Render(senderName + " " + dateStr)
				}
			} else {
				if alignRight {
					// Right-aligned main thread message.
					h := lipgloss.NewStyle().Foreground(colGreen).Render(dateStr + " " + senderName)
					header = padLeft(h, w)
				} else {
					// Left-aligned main thread message.
					color := colCyan
					if m.isOwn(msg) {
						color = colGreen
					}
					header = lipgloss.NewStyle().Foreground(color).Render(senderName + " " + dateStr)
				}
			}
			lines = append(lines, header)
		}
		prevSender = sender
		prevTime = msgTime
		// After a root channel message, reset grouping so replies start fresh
		// and don't accidentally group with the previous thread's replies.
		if m.channelSelectedIndex >= 0 && !msg.IsReply {
			prevSender = ""
			prevTime = time.Time{}
		}

		// Replies from others (or replies in chats when not ours) are left-indented.
		// In channels, replies are always right-aligned, so they do not have left indentation.
		replyIndent := ""
		if msg.IsReply && !alignRight {
			replyIndent = "    " // 4 spaces aligning under "↳ "
		}

		msgLines := m.getWrappedMessageLines(&msgs[i], maxW-len(replyIndent), m.app.SearchQuery, m.app.SearchActive)
		padding := 0
		if alignRight {
			maxMsgW := 0
			for _, l := range msgLines {
				lw := lipgloss.Width(l)
				if lw > maxMsgW {
					maxMsgW = lw
				}
			}
			padding = w - maxMsgW
			if padding < 0 {
				padding = 0
			}
		}

		padStr := strings.Repeat(" ", padding)
		isSelected := m.app.MessageSelectionMode && (start+i == m.app.MessageSelectedIndex)
		if isSelected {
			selectedStartLine = len(lines)
		}
		for _, line := range msgLines {
			content := replyIndent + padStr + line
			if isSelected {
				content = lipgloss.NewStyle().
					Background(colDarkGray).
					Foreground(colYellow).
					Width(w).
					Render(content)
			}
			lines = append(lines, content)
		}
		if isSelected {
			selectedEndLine = len(lines)
		}
	}

	// Apply scroll.
	total := len(lines)
	m.app.MaxScroll = total - h
	if m.app.MaxScroll < 0 {
		m.app.MaxScroll = 0
	}

	if m.app.SnapToBottom {
		m.app.ScrollOffset = m.app.MaxScroll
	}

	// Auto-scroll to keep selection visible.
	if m.app.MessageSelectionMode && selectedStartLine != -1 {
		msgHeight := selectedEndLine - selectedStartLine
		if selectedStartLine < m.app.ScrollOffset {
			// Selection scrolled above the top — bring it back into view.
			m.app.ScrollOffset = selectedStartLine
			m.app.SnapToBottom = false
		} else if selectedEndLine > m.app.ScrollOffset+h {
			if msgHeight >= h {
				// Message is taller than the viewport — anchor to its top so
				// the user sees the beginning rather than jumping to the end.
				m.app.ScrollOffset = selectedStartLine
			} else {
				// Message fits — scroll just enough to expose its bottom.
				m.app.ScrollOffset = selectedEndLine - h
			}
			m.app.SnapToBottom = false
		}
	}

	if m.app.ScrollOffset < 0 {
		m.app.ScrollOffset = 0
	}
	if m.app.ScrollOffset > m.app.MaxScroll {
		m.app.ScrollOffset = m.app.MaxScroll
	}

	// Apply pending scroll jump (pagination context).
	// Apply pending scroll jump (pagination context).
	if m.app.PendingScrollID != "" {
		if pendingScrollLine != -1 {
			// Jump to where the old top message moved.
			// "with few line down" -> subtract 3 so the user sees some new context.
			m.app.ScrollOffset = pendingScrollLine - 3
			m.app.SnapToBottom = false

			// Clamp again after jump.
			if m.app.ScrollOffset < 0 {
				m.app.ScrollOffset = 0
			}
			if m.app.ScrollOffset > m.app.MaxScroll {
				m.app.ScrollOffset = m.app.MaxScroll
			}
		}
		m.app.PendingScrollID = ""
	}

	// Slice lines for the visible window.
	start2 := m.app.ScrollOffset
	end := start2 + h
	if end > len(lines) {
		end = len(lines)
	}
	if start2 > len(lines) {
		start2 = len(lines)
	}

	return strings.Join(lines[start2:end], "\n")
}

// ---------------------------------------------------------------------------
// Status bar
// ---------------------------------------------------------------------------

func (m Model) renderStatusBar(w int) string {
	if m.app.DeleteConfirmMode {
		return bellBorder.Width(w - 2).Height(1).Render(
			lipgloss.NewStyle().Foreground(colRed).Bold(true).Render(
				"DELETE MESSAGE? (y:yes / n:no)",
			),
		)
	}
	if m.app.ReactionMode {
		return normalBorder.Width(w - 2).Height(1).Render(
			lipgloss.NewStyle().Foreground(colYellow).Render(
				"REACT: 1:👍 2:❤️ 3:😂 4:😮 5:😢 6:😡 (ESC:cancel)",
			),
		)
	}
	text := fmt.Sprintf("%s | Notification (n): %s", m.app.Status, m.app.NotificationMode)
	if m.app.LoadingMessages && len(m.app.Messages) > 0 {
		text = "⏳ Loading older messages... | " + text
	}
	if m.app.VisualBellActive() {
		return bellBorder.Width(w - 2).Height(1).Render(text)
	}
	return normalBorder.Width(w - 2).Height(1).Render(
		lipgloss.NewStyle().Foreground(colGreen).Render(text),
	)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func chatPanelWidth(total int) int {
	return total * 30 / 100
}

func msgPanelWidth(total int) int {
	return total - chatPanelWidth(total)
}

func truncate(s string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= maxLen {
		return s
	}
	return string(r[:maxLen-1]) + "…"
}

// padLeft right-aligns text within width w by prepending spaces.
func padLeft(s string, w int) string {
	visLen := lipgloss.Width(s)
	pad := w - visLen
	if pad <= 0 {
		return s
	}
	return strings.Repeat(" ", pad) + s
}

func wordWrap(s string, maxW int) []string {
	if maxW <= 0 {
		return []string{s}
	}
	// Use lipgloss to perform ANSI-aware wrapping. lipgloss (via reflow)
	// correctly handles repeating escape sequences (like OSC 8 links) across
	// line breaks.
	res := lipgloss.NewStyle().Width(maxW).Render(s)
	lines := strings.Split(res, "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " ")
	}
	return lines
}

func (m Model) getWrappedMessageLines(msg *Message, maxW int, searchQuery string, searchActive bool) []string {
	queryKey := ""
	if searchActive {
		queryKey = searchQuery
	}
	if msg.WrappedWidthCached == maxW && msg.WrappedQueryCached == queryKey && len(msg.WrappedLinesCached) > 0 {
		return msg.WrappedLinesCached
	}

	body := msg.GetPlainText()
	if searchActive && searchQuery != "" {
		body = highlightQuery(body, searchQuery)
	}

	if msg.Subject != "" {
		subjText := msg.Subject
		if searchActive && searchQuery != "" {
			subjText = highlightQuery(subjText, searchQuery)
		}
		subjStyled := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF")).Render(subjText)
		if body != "" {
			body = subjStyled + "\n" + body
		} else {
			body = subjStyled
		}
	}

	reactionsStr := renderReactions(msg.Reactions)
	if reactionsStr != "" {
		if body != "" {
			body += "\n" + reactionsStr
		} else {
			body = reactionsStr
		}
	}

	lines := wordWrap(body, maxW)
	msg.WrappedWidthCached = maxW
	msg.WrappedQueryCached = queryKey
	msg.WrappedLinesCached = lines
	return lines
}

// updateScroll recalculates scroll bounds after messages change.
func (m *Model) updateScroll() {
	if m.app.SnapToBottom {
		m.app.ScrollOffset = m.app.MaxScroll
	}
}

// messagesEqual returns true if the two slices have the same count and last ID.
func messagesEqual(a, b []Message) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].ID != b[i].ID {
			return false
		}
		// Compare body content.
		contentA := ""
		if a[i].Body != nil && a[i].Body.Content != nil {
			contentA = *a[i].Body.Content
		}
		contentB := ""
		if b[i].Body != nil && b[i].Body.Content != nil {
			contentB = *b[i].Body.Content
		}
		if contentA != contentB {
			return false
		}
		if len(a[i].Reactions) != len(b[i].Reactions) {
			return false
		}
		// Deep compare reactions.
		for j := range a[i].Reactions {
			if a[i].Reactions[j].ReactionType != b[i].Reactions[j].ReactionType {
				return false
			}
			// If we really want to be sure, we could compare user IDs,
			// but type changes are the most common visible change.
		}
	}
	return true
}

func (m Model) markRead() Model {
	if !m.focused {
		return m
	}
	if m.channelSelectedIndex >= 0 {
		chans := m.allChannels()
		if m.channelSelectedIndex < len(chans) {
			entry := chans[m.channelSelectedIndex]
			lastID := m.lastMsgID[entry.channelID]
			if lastID == "" && len(m.app.Messages) > 0 {
				lastID = m.app.Messages[0].ID
			}
			if lastID != "" && m.lastReadMsgID[entry.channelID] != lastID {
				m.lastReadMsgID[entry.channelID] = lastID
			}
		}
		return m
	}

	chat := m.app.GetSelectedChat()
	if chat == nil {
		return m
	}

	lastID := m.lastMsgID[chat.ID]
	if lastID == "" && len(m.app.Messages) > 0 {
		lastID = m.app.Messages[0].ID
	}

	if lastID != "" && m.lastReadMsgID[chat.ID] != lastID {
		m.lastReadMsgID[chat.ID] = lastID
		go MarkChatAsRead(func() string {
			t, _ := GetValidTokenSilent(m.clientID)
			return t
		}(), chat.ID, m.userID)
	}

	// Mark reactions in the selected chat as read.
	if m.lastReadReactions[chat.ID] == nil {
		m.lastReadReactions[chat.ID] = make(map[string]bool)
	}
	for _, msgObj := range m.app.Messages {
		for _, rKey := range m.getReactionKeys(&msgObj) {
			m.lastReadReactions[chat.ID][rKey] = true
		}
	}
	if hist, ok := m.app.HistoryMessages[chat.ID]; ok {
		for _, msgObj := range hist {
			for _, rKey := range m.getReactionKeys(&msgObj) {
				m.lastReadReactions[chat.ID][rKey] = true
			}
		}
	}
	for _, c := range m.latestChats {
		if c.ID == chat.ID {
			for _, rKey := range m.getReactionKeys(c.LastMessagePreview) {
				m.lastReadReactions[chat.ID][rKey] = true
			}
			break
		}
	}

	return m
}

func (m Model) isUnread(c Chat) bool {
	// If it's the currently selected chat, we consider it read (or about to be).
	// However, to keep the UI consistent, let's only rely on the IDs.

	lastID, hasLast := m.lastMsgID[c.ID]
	if !hasLast {
		return false
	}

	// Check local read state first.
	if readID, ok := m.lastReadMsgID[c.ID]; ok && readID == lastID {
		return false
	}

	// If we have a local lastReadMsgID but it's not the lastID, it's definitely unread
	// (unless we were about to fall back to viewpoint).
	if _, ok := m.lastReadMsgID[c.ID]; ok {
		return true
	}

	// Fallback to server-side viewpoint.
	if c.Viewpoint != nil {
		readTime, _ := time.Parse(time.RFC3339Nano, c.Viewpoint.LastMessageReadDateTime)
		lastTime := m.lastMsgTime[c.ID]
		if !lastTime.IsZero() && !readTime.IsZero() {
			// If latest message is newer than last read time, it's unread.
			// Add 1s buffer for safety.
			return lastTime.After(readTime.Add(time.Second))
		}
	}
	// Fallback when server viewpoint read time is missing/uninitialized:
	// a chat is unread if its last message was sent by someone else.
	if c.LastMessagePreview != nil {
		return !m.isOwn(*c.LastMessagePreview)
	}

	return false
}

func (m Model) isOwn(msg Message) bool {
	if m.app.CurrentUserName == nil {
		return false
	}
	if msg.From == nil || msg.From.User == nil || msg.From.User.DisplayName == nil {
		return false
	}
	return *msg.From.User.DisplayName == *m.app.CurrentUserName
}

func renderReactions(reactions []MessageReaction) string {
	if len(reactions) == 0 {
		return ""
	}

	counts := make(map[string]int)
	for _, r := range reactions {
		counts[strings.ToLower(r.ReactionType)]++
	}

	var parts []string
	// Known reaction types for stable ordering.
	types := []string{"like", "heart", "laugh", "surprised", "sad", "angry"}
	for _, t := range types {
		if count, ok := counts[t]; ok {
			emoji := reactionEmoji(t)
			if count > 1 {
				parts = append(parts, fmt.Sprintf("%s %d", emoji, count))
			} else {
				parts = append(parts, emoji)
			}
		}
	}

	// Any other types?
	var otherTypes []string
	for t := range counts {
		found := false
		for _, known := range types {
			if t == known {
				found = true
				break
			}
		}
		if !found {
			otherTypes = append(otherTypes, t)
		}
	}
	sort.Strings(otherTypes)

	for _, t := range otherTypes {
		count := counts[t]
		emoji := reactionEmoji(t)
		if count > 1 {
			parts = append(parts, fmt.Sprintf("%s %d", emoji, count))
		} else {
			parts = append(parts, emoji)
		}
	}

	if len(parts) == 0 {
		return ""
	}

	return lipgloss.NewStyle().Foreground(colDimGray).Render(strings.Join(parts, "  "))
}

func reactionEmoji(t string) string {
	switch strings.ToLower(t) {
	case "like", "👍":
		return "👍"
	case "heart", "❤️":
		return "❤️"
	case "laugh", "😂":
		return "😂"
	case "surprised", "😮":
		return "😮"
	case "sad", "😢":
		return "😢"
	case "angry", "😡":
		return "😡"
	default:
		return t
	}
}

// ---------------------------------------------------------------------------
// Chat ordering helpers
// ---------------------------------------------------------------------------

// promoteChat moves chatID to position 0 in the stable order.
// Favourited chats are anchored at the top by rebuildChatList, so they are
// skipped here to avoid disrupting the alphabetical favourites group.
func (m *Model) promoteChat(chatID string) {
	// Don't promote favourited chats — they stay anchored at the top.
	if m.favourites[chatID] {
		return
	}
	for i, id := range m.stableChatOrder {
		if id == chatID {
			m.stableChatOrder = append([]string{chatID},
				append(m.stableChatOrder[:i], m.stableChatOrder[i+1:]...)...)
			return
		}
	}
	// Not found — prepend.
	m.stableChatOrder = append([]string{chatID}, m.stableChatOrder...)
}

// mergeChats integrates fresh chats from the API into the stable order.
// New chats with messages are prepended; new chats without messages are appended.
func (m Model) mergeChats(fresh []Chat) Model {
	// Build a set of known IDs.
	known := make(map[string]bool, len(m.stableChatOrder))
	for _, id := range m.stableChatOrder {
		known[id] = true
	}

	// Add new chats. Use LastMessagePreview directly to determine whether the
	// chat has a message — this avoids depending on m.lastMsgID being populated
	// by the loop in MsgChatsLoaded (a fragile side-effect ordering dependency).
	var newWithMsg []string
	var newWithout []string
	for _, c := range fresh {
		if !known[c.ID] {
			if c.LastMessagePreview != nil {
				// Only prepend if the message was sent after the app started.
				// Otherwise, it is an old chat that drifted in, so append it.
				newTime, err := time.Parse(time.RFC3339Nano, c.LastMessagePreview.CreatedDateTime)
				if err == nil && newTime.After(m.app.AppStartTime) {
					newWithMsg = append(newWithMsg, c.ID)
				} else {
					newWithout = append(newWithout, c.ID)
				}
			} else {
				newWithout = append(newWithout, c.ID)
			}
		}
	}

	m.stableChatOrder = append(newWithMsg, append(m.stableChatOrder, newWithout...)...)
	return m.rebuildChatList()
}

func (m Model) rebuildChatList() Model {
	byID := make(map[string]Chat)
	// Retain previously loaded chats so they don't disappear from the UI
	for _, c := range m.app.Chats {
		byID[c.ID] = c
	}
	// Overwrite/add with fresh chat list data from the API
	for _, c := range m.latestChats {
		byID[c.ID] = c
	}

	// Split into favourites and non-favourites.
	var favChats []Chat
	var normalChats []Chat

	// Non-favourite chats follow the stable order.
	for _, id := range m.stableChatOrder {
		if c, ok := byID[id]; ok {
			if m.favourites[id] {
				favChats = append(favChats, c)
			} else {
				normalChats = append(normalChats, c)
			}
		}
	}

	// Also include favourited chats that are not in stableChatOrder yet
	// (e.g. chats with old activity not loaded from API this session).
	knownInOrder := make(map[string]bool, len(m.stableChatOrder))
	for _, id := range m.stableChatOrder {
		knownInOrder[id] = true
	}
	for id := range m.favourites {
		if !knownInOrder[id] {
			if c, ok := byID[id]; ok {
				favChats = append(favChats, c)
			}
			// If the chat data isn't loaded yet, it simply won't appear until
			// the API returns it. The favourite status is preserved in the file.
		}
	}

	// Sort favourites alphabetically by display name.
	sort.Slice(favChats, func(i, j int) bool {
		namei := ""
		namej := ""
		if favChats[i].CachedDisplayName != nil {
			namei = *favChats[i].CachedDisplayName
		}
		if favChats[j].CachedDisplayName != nil {
			namej = *favChats[j].CachedDisplayName
		}
		return strings.ToLower(namei) < strings.ToLower(namej)
	})

	m.app.Chats = append(favChats, normalChats...)
	return m
}

func (m Model) renderUrlSelection(w, h int) string {
	if len(m.app.UrlsInMessage) == 0 {
		return ""
	}

	var titleText string
	if m.app.UrlSelectionOpenMode {
		titleText = "Select URL to open (Enter to open, Esc/q to cancel):"
	} else {
		titleText = "Select URL to yank (Enter/y to copy, Esc/q to cancel):"
	}

	title := lipgloss.NewStyle().Foreground(colYellow).Bold(true).Render(titleText)
	var list strings.Builder
	list.WriteString(title + "\n\n")

	for i, u := range m.app.UrlsInMessage {
		prefix := "  "
		style := lipgloss.NewStyle()
		if i == m.app.UrlSelectedIndex {
			prefix = "> "
			style = style.Foreground(colYellow).Background(colDarkGray)
		}

		displayURL := u
		if len(displayURL) > w-10 {
			displayURL = displayURL[:w-13] + "..."
		}
		list.WriteString(style.Render(prefix+displayURL) + "\n")
	}

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colYellow).
		Padding(1, 2).
		Render(list.String())

	return box
}

// ---------------------------------------------------------------------------
// Notifications
// ---------------------------------------------------------------------------

// stripANSI removes ANSI escape codes (including OSC 8 links) from a string.
func stripANSI(s string) string {
	// Standard ANSI CSI sequences + OSC 8 sequences.
	re := regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]|\x1b\]8;.*?\x1b\\`)
	return re.ReplaceAllString(s, "")
}

// notify triggers the appropriate notification based on the app's mode.
func (m *Model) notify(senderName string, msg Message) {
	body := ""
	if m.app.NotificationShowPreview {
		if msg.Body != nil && msg.Body.Content != nil {
			body = stripANSI(HTMLToText(*msg.Body.Content, msg.Attachments, msg.Mentions))
			// Remove newlines and collapse spaces for a cleaner notification body.
			body = strings.ReplaceAll(body, "\n", " ")
			body = strings.Join(strings.Fields(body), " ")
			if m.app.NotificationPreviewLen > 0 && utf8.RuneCountInString(body) > m.app.NotificationPreviewLen {
				body = string([]rune(body)[:m.app.NotificationPreviewLen]) + "..."
			}
		}
	}

	switch m.app.NotificationMode {
	case NotificationConsole:
		fmt.Print("\a") // BEL
		m.app.TriggerVisualBell()
	case NotificationSystem:
		go sendDesktopNotification(senderName, body)
	case NotificationBoth:
		fmt.Print("\a")
		m.app.TriggerVisualBell()
		go sendDesktopNotification(senderName, body)
	}
}

// messageMatches reports whether the given message contains the case-insensitive query in its body, subject, or attachment names.
func (m Model) messageMatches(msg *Message, query string) bool {
	if query == "" {
		return true
	}
	normQuery := normalizeString(strings.TrimSpace(strings.ToLower(query)))

	// Check subject
	if msg.Subject != "" && strings.Contains(msg.GetNormalizedSubject(), normQuery) {
		return true
	}

	// Check body
	if strings.Contains(msg.GetNormalizedText(), normQuery) {
		return true
	}

	// Check attachments
	for _, att := range msg.Attachments {
		if att.Name != nil && strings.Contains(normalizeString(strings.ToLower(*att.Name)), normQuery) {
			return true
		}
	}

	return false
}

// highlightQuery highlights occurrences of query inside text without breaking ANSI sequences.
func highlightQuery(text, query string) string {
	query = strings.TrimSpace(query)
	if query == "" {
		return text
	}

	lowerText := normalizeString(strings.ToLower(text))
	lowerQuery := normalizeString(strings.ToLower(query))
	if !strings.Contains(lowerText, lowerQuery) {
		return text
	}

	type runeInfo struct {
		isANSI  bool
		val     rune
		bytePos int
	}

	runes := []rune(text)
	infos := make([]runeInfo, 0, len(runes))

	bytePos := 0
	inANSI := false

	for i := 0; i < len(runes); i++ {
		r := runes[i]
		rLen := utf8.RuneLen(r)

		if r == '\x1b' {
			inANSI = true
		}

		infos = append(infos, runeInfo{
			isANSI:  inANSI,
			val:     r,
			bytePos: bytePos,
		})

		if inANSI {
			if r == 'm' && i > 0 && runes[i-1] != '\x1b' {
				inANSI = false
			}
			if r == '\\' && i > 0 && runes[i-1] == '\x1b' {
				inANSI = false
			}
		}

		bytePos += rLen
	}

	var plainText strings.Builder
	var plainToOriginal []int

	for _, info := range infos {
		if !info.isANSI {
			plainText.WriteRune(info.val)
			plainToOriginal = append(plainToOriginal, info.bytePos)
		}
	}

	plainStr := plainText.String()
	plainRunes := []rune(plainStr)
	var plainNormRunes []rune
	// plainNormToOriginalRuneIdx maps the index in plainNormRunes to the index in plainRunes
	plainNormToOriginalRuneIdx := make([]int, 0, len(plainRunes))

	for origRuneIdx, r := range plainRunes {
		normStr := normalizeString(string(r))
		for _, normRune := range normStr {
			plainNormRunes = append(plainNormRunes, normRune)
			plainNormToOriginalRuneIdx = append(plainNormToOriginalRuneIdx, origRuneIdx)
		}
	}

	plainLowerNormRunes := []rune(strings.ToLower(string(plainNormRunes)))
	queryNormRunes := []rune(normalizeString(strings.ToLower(query)))

	var matches [][]int
	if len(queryNormRunes) > 0 {
		for i := 0; i <= len(plainLowerNormRunes)-len(queryNormRunes); i++ {
			match := true
			for j := 0; j < len(queryNormRunes); j++ {
				if plainLowerNormRunes[i+j] != queryNormRunes[j] {
					match = false
					break
				}
			}
			if match {
				startNormIdx := i
				endNormIdx := i + len(queryNormRunes)

				startRuneIdx := plainNormToOriginalRuneIdx[startNormIdx]
				endRuneIdx := len(plainRunes)
				if endNormIdx < len(plainNormToOriginalRuneIdx) {
					endRuneIdx = plainNormToOriginalRuneIdx[endNormIdx]
				}

				matches = append(matches, []int{startRuneIdx, endRuneIdx})
				// Advance index to avoid overlapping matches
				i += len(queryNormRunes) - 1
			}
		}
	}

	if len(matches) == 0 {
		return text
	}

	var result strings.Builder
	lastOrigByte := 0

	for _, match := range matches {
		startRune := match[0]
		endRune := match[1]

		origStartByte := plainToOriginal[startRune]
		origEndByte := len(text)
		if endRune < len(plainToOriginal) {
			origEndByte = plainToOriginal[endRune]
		}

		result.WriteString(text[lastOrigByte:origStartByte])

		matchText := text[origStartByte:origEndByte]
		highlighted := lipgloss.NewStyle().Foreground(colYellow).Bold(true).Render(matchText)
		result.WriteString(highlighted)

		lastOrigByte = origEndByte
	}

	result.WriteString(text[lastOrigByte:])
	return result.String()
}

// RebuildSearchPopupResults filters the history messages by search query, merges context, and updates state.
func (m *Model) RebuildSearchPopupResults() {
	query := m.app.SearchQuery
	if query == "" {
		m.app.SearchPopupResults = nil
		m.app.SearchPopupSelectedIndex = 0
		return
	}

	convID := m.activeConversationID()
	if convID == "" {
		return
	}

	// Retrieve all loaded messages for this conversation so far
	history, ok := m.app.HistoryMessages[convID]
	if !ok || len(history) == 0 {
		m.app.SearchPopupResults = nil
		m.app.SearchPopupSelectedIndex = 0
		return
	}

	// Save the currently selected message ID to maintain selection focus.
	var prevSelectedID string
	if len(m.app.SearchPopupResults) > 0 && m.app.SearchPopupSelectedIndex < len(m.app.SearchPopupResults) {
		prevSelectedID = m.app.SearchPopupResults[m.app.SearchPopupSelectedIndex].Message.ID
	}
	// Save the first visible message ID to freeze screen position.
	var prevFirstVisibleID string
	if len(m.app.SearchPopupResults) > 0 && m.app.SearchPopupScrollOffset < len(m.app.SearchPopupResults) {
		prevFirstVisibleID = m.app.SearchPopupResults[m.app.SearchPopupScrollOffset].Message.ID
	}

	// First, find all matching indices in the history list.
	matches := make(map[int]bool)
	for i := range history {
		if m.messageMatches(&history[i], query) {
			matches[i] = true
		}
	}

	if len(matches) == 0 {
		m.app.SearchPopupResults = nil
		m.app.SearchPopupSelectedIndex = 0
		return
	}

	// We want to include context messages: x before and x after each match.
	x := ResolveSearchContextLimit()

	// Gather all indices we should include. Use a map to automatically avoid duplicates.
	includedIndices := make(map[int]bool)
	for matchIdx := range matches {
		includedIndices[matchIdx] = true
		for j := 1; j <= x; j++ {
			// Older context
			if matchIdx+j < len(history) {
				includedIndices[matchIdx+j] = true
			}
			// Newer context
			if matchIdx-j >= 0 {
				includedIndices[matchIdx-j] = true
			}
		}
	}

	// Merge expanded indices for this conversation
	if state, ok := m.app.SearchStates[convID]; ok && state.ExpandedIndices != nil {
		for idx := range state.ExpandedIndices {
			if idx >= 0 && idx < len(history) {
				includedIndices[idx] = true
			}
		}
	}

	// Sort the included indices. Since history is newest-first (index 0 is newest, len-1 is oldest),
	// sorting included indices in descending order (from largest index to smallest)
	// will render the popup list in chronological order (oldest at the top, newest at the bottom).
	var sortedIndices []int
	for i := len(history) - 1; i >= 0; i-- {
		if includedIndices[i] {
			sortedIndices = append(sortedIndices, i)
		}
	}

	// Build the items list.
	var items []SearchPopupItem
	for _, idx := range sortedIndices {
		items = append(items, SearchPopupItem{
			Message:      history[idx],
			IsMatch:      matches[idx],
			HistoryIndex: idx,
		})
	}

	// Explicitly sort items oldest-first (chronological, closest to today at index len-1).
	sort.Slice(items, func(i, j int) bool {
		return items[i].Message.CreatedDateTime < items[j].Message.CreatedDateTime
	})

	m.app.SearchPopupResults = items

	// Restore selection focus by message ID to prevent scrolling/jumping when older history loads
	if prevSelectedID != "" {
		found := false
		for i, item := range items {
			if item.Message.ID == prevSelectedID {
				m.app.SearchPopupSelectedIndex = i
				found = true
				break
			}
		}
		if !found {
			m.app.SearchPopupSelectedIndex = 0
			for i := len(items) - 1; i >= 0; i-- {
				if items[i].IsMatch {
					m.app.SearchPopupSelectedIndex = i
					break
				}
			}
		}
	} else {
		// New search: default selection to the last actual match (latest/newest match chronologically)
		m.app.SearchPopupSelectedIndex = 0
		for i := len(items) - 1; i >= 0; i-- {
			if items[i].IsMatch {
				m.app.SearchPopupSelectedIndex = i
				break
			}
		}
	}
	if m.app.SearchPopupSelectedIndex < 0 {
		m.app.SearchPopupSelectedIndex = 0
	}

	// Restore scroll offset by message ID to freeze screen position
	if prevFirstVisibleID != "" {
		for i, item := range items {
			if item.Message.ID == prevFirstVisibleID {
				m.app.SearchPopupScrollOffset = i
				break
			}
		}
	}
}

// renderSearchPopup draws the beautiful search interface modal on top of screen.
func (m Model) renderSearchPopup(w, h int) string {
	var displayName string
	if m.channelSelectedIndex >= 0 {
		if entry := m.activeChannelEntry(); entry != nil {
			displayName = entry.teamName + " » " + entry.channelName
		} else {
			displayName = "Channel"
		}
	} else {
		chat := m.app.GetSelectedChat()
		if chat != nil && chat.CachedDisplayName != nil {
			displayName = *chat.CachedDisplayName
		} else {
			displayName = "Chat"
		}
	}

	titleStyle := lipgloss.NewStyle().Foreground(colYellow).Bold(true)
	titleText := "Search History (Enter to search)"
	if m.app.SearchQuery != "" {
		titleText = fmt.Sprintf("Search History: %s | Results for '%s'", displayName, m.app.SearchQuery)
	}
	title := titleStyle.Render(titleText)

	instructions := lipgloss.NewStyle().Foreground(colDimGray).Render(
		" j/k: Nav | g: Go to msg | y: Yank | u: URL | o: Open URL | Enter: Expand | /: Edit | Esc: Close",
	)

	var list strings.Builder
	list.WriteString(title + "\n")
	list.WriteString(instructions + "\n\n")

	results := m.app.SearchPopupResults
	msgH := h - 10
	if msgH < 3 {
		msgH = 3
	}

	if len(results) == 0 {
		if m.app.SearchQuery == "" {
			list.WriteString(lipgloss.NewStyle().Foreground(colDimGray).Render("Type a query and press Enter to search.") + "\n")
		} else {
			list.WriteString(lipgloss.NewStyle().Foreground(colDimGray).Render("No matching messages found.") + "\n")
		}
		// Fill remaining lines to keep height stable
		for l := 1; l < msgH; l++ {
			list.WriteString("\n")
		}
	} else {
		// Keep selected index visible
		if m.app.SearchPopupSelectedIndex < m.app.SearchPopupScrollOffset {
			m.app.SearchPopupScrollOffset = m.app.SearchPopupSelectedIndex
		}
		if m.app.SearchPopupScrollOffset < 0 {
			m.app.SearchPopupScrollOffset = 0
		}

		// Adjust scroll offset to keep selected item visible at the bottom:
		for {
			hSum := 0
			for i := m.app.SearchPopupScrollOffset; i <= m.app.SearchPopupSelectedIndex && i < len(results); i++ {
				item := results[i]
				body := item.Message.GetPlainText()
				if item.IsMatch {
					body = highlightQuery(body, m.app.SearchQuery)
				}
				if item.Message.Subject != "" {
					subjText := item.Message.Subject
					if item.IsMatch {
						subjText = highlightQuery(subjText, m.app.SearchQuery)
					}
					if body != "" {
						body = subjText + "\n" + body
					} else {
						body = subjText
					}
				}
				bodyW := w - 8
				if bodyW < 10 {
					bodyW = 10
				}
				bodyLines := wordWrap(body, bodyW)
				hSum += len(bodyLines) + 1 // body lines + 1 header line

				// Add gap indicator height if applicable
				if i > 0 {
					prevItem := results[i-1]
					diff := item.HistoryIndex - prevItem.HistoryIndex
					if diff < 0 {
						diff = -diff
					}
					if diff > 1 {
						hSum += 3 // 3 blank/separator lines
					}
				}
			}
			if hSum <= msgH || m.app.SearchPopupScrollOffset >= m.app.SearchPopupSelectedIndex {
				break
			}
			m.app.SearchPopupScrollOffset++
		}

		// Clamp scroll offset to valid bounds
		if m.app.SearchPopupScrollOffset >= len(results) {
			m.app.SearchPopupScrollOffset = len(results) - 1
		}
		if m.app.SearchPopupScrollOffset < 0 {
			m.app.SearchPopupScrollOffset = 0
		}

		// Now render ONLY the visible items starting from SearchPopupScrollOffset
		var visibleLines []string
		linesRendered := 0

		for idx := m.app.SearchPopupScrollOffset; idx < len(results); idx++ {
			item := results[idx]

			// Render gap indicator if applicable
			var gapLines []string
			if idx > 0 {
				prevItem := results[idx-1]
				diff := item.HistoryIndex - prevItem.HistoryIndex
				if diff < 0 {
					diff = -diff
				}
				if diff > 1 {
					gapLines = append(gapLines, "")
					gapLines = append(gapLines, lipgloss.NewStyle().Foreground(colDimGray).Render("  ─── [gap in history] ───"))
					gapLines = append(gapLines, "")
				}
			}

			// Render sender + date
			sender := "Unknown"
			if item.Message.From != nil && item.Message.From.User != nil && item.Message.From.User.DisplayName != nil {
				sender = *item.Message.From.User.DisplayName
			}
			msgTime, _ := time.Parse(time.RFC3339Nano, item.Message.CreatedDateTime)
			msgTime = msgTime.Local()
			dateStr := ""
			if !msgTime.IsZero() {
				dateStr = msgTime.Format("2006 Jan 02 15:04")
			}

			isSelected := idx == m.app.SearchPopupSelectedIndex
			prefix := "  "
			if isSelected {
				prefix = "> "
			}

			var header string
			if m.isOwn(item.Message) {
				senderName := "Me"
				if item.IsMatch {
					senderName = highlightQuery(senderName, m.app.SearchQuery)
				}
				header = lipgloss.NewStyle().Foreground(colGreen).Render(prefix + dateStr + " " + senderName)
			} else {
				senderName := sender
				if item.IsMatch {
					senderName = highlightQuery(senderName, m.app.SearchQuery)
				}
				header = lipgloss.NewStyle().Foreground(colCyan).Render(prefix + senderName + " " + dateStr)
			}

			// Render body
			body := item.Message.GetPlainText()
			if item.IsMatch {
				body = highlightQuery(body, m.app.SearchQuery)
			}

			if item.Message.Subject != "" {
				subjText := item.Message.Subject
				if item.IsMatch {
					subjText = highlightQuery(subjText, m.app.SearchQuery)
				}
				subjStyled := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF")).Render(subjText)
				if body != "" {
					body = subjStyled + "\n" + body
				} else {
					body = subjStyled
				}
			}

			// Wrap body lines
			bodyW := w - 8
			if bodyW < 10 {
				bodyW = 10
			}
			bodyLines := wordWrap(body, bodyW)

			var itemLines []string
			itemLines = append(itemLines, header)
			for _, bl := range bodyLines {
				lineStyle := lipgloss.NewStyle()
				if isSelected {
					lineStyle = lineStyle.Background(colDarkGray)
				}
				itemLines = append(itemLines, lineStyle.Render("    "+bl))
			}

			// Check if we have space to draw this item (or if it's the very first item we must draw it)
			totalNewLines := len(gapLines) + len(itemLines)
			if linesRendered+totalNewLines <= msgH || idx == m.app.SearchPopupScrollOffset {
				visibleLines = append(visibleLines, gapLines...)
				visibleLines = append(visibleLines, itemLines...)
				linesRendered += totalNewLines
			} else {
				// No more room in viewport, stop rendering!
				break
			}
		}

		// Write viewport lines to buffer
		for _, line := range visibleLines {
			list.WriteString(line + "\n")
		}

		// Fill remaining height with blank lines to keep popup height perfectly stable
		for l := linesRendered; l < msgH; l++ {
			list.WriteString("\n")
		}
	}

	m.searchInput.Width = w - 10
	tiView := m.searchInput.View()

	borderCol := colYellow
	if !m.app.SearchMode {
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

	// Render status/loader row above the input box
	statusText := ""
	if m.app.SearchStatus != "" {
		statusText = "  " + lipgloss.NewStyle().Foreground(colYellow).Italic(true).Render(m.app.SearchStatus)
	} else if m.app.SearchLoadingMessages {
		statusText = "  " + lipgloss.NewStyle().Foreground(colYellow).Italic(true).Render("⏳ Searching history in background...")
	}

	list.WriteString(statusText + "\n" + inputBox)

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colYellow).
		Padding(1, 2).
		Width(w).Height(h).
		Render(list.String())

	return box
}

// handleSearchPopupNavigationKey handles keystrokes inside results list navigation mode.
func (m Model) handleSearchPopupNavigationKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		m.app.SearchPopupMode = false
		m.app.SearchMode = false
		m.app.SetSearchLoadingMessages(false)
		m.saveSearchState()
		m.app.ScrollOffset = m.app.MainChatScrollOffset
		m.app.SnapToBottom = m.app.MainChatSnapToBottom
		return m, nil

	case "j", "down":
		if m.app.SearchPopupSelectedIndex < len(m.app.SearchPopupResults)-1 {
			m.app.SearchPopupSelectedIndex++
		}
		m.saveSearchState()
		return m, nil

	case "k", "up":
		if m.app.SearchPopupSelectedIndex > 0 {
			m.app.SearchPopupSelectedIndex--
		} else {
			// At the top of the search results list (oldest match currently loaded)
			convID := m.activeConversationID()
			if convID != "" {
				nextLink := m.app.HistoryNextLink[convID]
				if nextLink != "" && !m.app.SearchLoadingMessages {
					m.app.SetSearchLoadingMessages(true)
					m.app.SetSearchStatus(fmt.Sprintf("Loading older messages for '%s'...", m.app.SearchQuery), 0)
					m.saveSearchState()
					return m, loadMoreMessagesCmd(m.clientID, nextLink, convID, true)
				}
			}
		}
		m.saveSearchState()
		return m, nil

	case "/":
		m.app.SearchMode = true
		m.searchInput.Focus()
		m.saveSearchState()
		return m, textinput.Blink

	case "g":
		if len(m.app.SearchPopupResults) > 0 && m.app.SearchPopupSelectedIndex < len(m.app.SearchPopupResults) {
			item := m.app.SearchPopupResults[m.app.SearchPopupSelectedIndex]
			convID := m.activeConversationID()
			if convID != "" {
				// 1. Close search mode
				m.app.SearchPopupMode = false
				m.app.SearchMode = false
				m.app.SetSearchLoadingMessages(false)
				m.saveSearchState()

				// 2. Merge all history messages fetched during search/history navigation into the main message list
				if hist, ok := m.app.HistoryMessages[convID]; ok && len(hist) > 0 {
					m.app.Messages = mergeHistoryMessages(m.app.Messages, hist)
					m.app.CachedMessages[convID] = m.app.Messages
				}
				if link, ok := m.app.HistoryNextLink[convID]; ok && link != "" {
					m.app.NextLink = link
					m.app.CachedNextLink[convID] = link
				}

				// 3. Find the message index in the merged messages list
				targetIdx := -1
				for i, msg := range m.app.Messages {
					if msg.ID == item.Message.ID {
						targetIdx = i
						break
					}
				}

				// 4. Activate selection mode and jump to that message
				if targetIdx != -1 {
					m.app.MessageSelectionMode = true
					m.app.MessageSelectedIndex = targetIdx
					m.app.PendingScrollID = item.Message.ID
					m.app.SnapToBottom = false
					m.app.SetStatus("Jumped to message in normal view.", 3*time.Second)
				}
			}
		}
		return m, nil


	case "y":
		if len(m.app.SearchPopupResults) > 0 && m.app.SearchPopupSelectedIndex < len(m.app.SearchPopupResults) {
			msgObj := m.app.SearchPopupResults[m.app.SearchPopupSelectedIndex].Message
			if msgObj.Body != nil && msgObj.Body.Content != nil {
				text := stripANSI(HTMLToText(*msgObj.Body.Content, msgObj.Attachments, msgObj.Mentions))
				if err := clipboard.WriteAll(text); err == nil {
					m.app.SetSearchStatus("Message copied to clipboard", 3*time.Second)
				} else {
					m.app.SetSearchStatus("Clipboard error: "+err.Error(), 5*time.Second)
				}
			}
		}
	case "enter":
		if len(m.app.SearchPopupResults) > 0 && m.app.SearchPopupSelectedIndex < len(m.app.SearchPopupResults) {
			item := m.app.SearchPopupResults[m.app.SearchPopupSelectedIndex]
			convID := m.activeConversationID()
			if convID != "" {
				state, ok := m.app.SearchStates[convID]
				if !ok {
					state = &ChatSearchState{
						ExpandedIndices: make(map[int]bool),
					}
					m.app.SearchStates[convID] = state
				}
				if state.ExpandedIndices == nil {
					state.ExpandedIndices = make(map[int]bool)
				}

				idx := item.HistoryIndex
				history := m.app.HistoryMessages[convID]
				for j := 1; j <= 5; j++ {
					if idx+j < len(history) {
						state.ExpandedIndices[idx+j] = true
					}
					if idx-j >= 0 {
						state.ExpandedIndices[idx-j] = true
					}
				}

				m.RebuildSearchPopupResults()
				m.app.SetSearchStatus("Expanded context by 5 messages before/after", 3*time.Second)
				m.saveSearchState()
			}
		}
		return m, nil

	case "u":
		if len(m.app.SearchPopupResults) > 0 && m.app.SearchPopupSelectedIndex < len(m.app.SearchPopupResults) {
			msgObj := m.app.SearchPopupResults[m.app.SearchPopupSelectedIndex].Message
			if msgObj.Body != nil && msgObj.Body.Content != nil {
				urls := ExtractURLs(*msgObj.Body.Content)
				if len(urls) == 0 {
					m.app.SetSearchStatus("No URLs found in message", 3*time.Second)
				} else if len(urls) == 1 {
					if err := clipboard.WriteAll(urls[0]); err == nil {
						m.app.SetSearchStatus("URL copied to clipboard", 3*time.Second)
					}
				} else {
					m.app.UrlSelectionMode = true
					m.app.UrlSelectionOpenMode = false
					m.app.UrlSelectedIndex = 0
					m.app.UrlsInMessage = urls
				}
			}
		}
	case "o":
		if len(m.app.SearchPopupResults) > 0 && m.app.SearchPopupSelectedIndex < len(m.app.SearchPopupResults) {
			msgObj := m.app.SearchPopupResults[m.app.SearchPopupSelectedIndex].Message
			if msgObj.Body != nil && msgObj.Body.Content != nil {
				urls := ExtractURLs(*msgObj.Body.Content)
				if len(urls) == 0 {
					m.app.SetSearchStatus("No URLs found in message", 3*time.Second)
				} else if len(urls) == 1 {
					return m, openURLCmd(urls[0], m.app.BrowserCommand, m.app.YoutrackCommand, m.app.GitlabCommand)
				} else {
					m.app.UrlSelectionMode = true
					m.app.UrlSelectionOpenMode = true
					m.app.UrlSelectedIndex = 0
					m.app.UrlsInMessage = urls
				}
			}
		}
	}

	return m, nil
}

// saveSearchState stores the current conversation's active search state to the SearchStates map.
func (m *Model) saveSearchState() {
	convID := m.activeConversationID()
	if convID == "" {
		return
	}
	state, ok := m.app.SearchStates[convID]
	if !ok {
		state = &ChatSearchState{
			ExpandedIndices: make(map[int]bool),
		}
		m.app.SearchStates[convID] = state
	}
	if state.ExpandedIndices == nil {
		state.ExpandedIndices = make(map[int]bool)
	}
	state.Query = m.app.SearchQuery
	state.Results = m.app.SearchPopupResults
	state.SelectedIndex = m.app.SearchPopupSelectedIndex
	state.ScrollOffset = m.app.SearchPopupScrollOffset
	state.Status = m.app.SearchStatus
}

// loadSearchState restores the search state for the selected conversation into active fields.
func (m *Model) loadSearchState() {
	convID := m.activeConversationID()
	if convID == "" {
		return
	}
	state, ok := m.app.SearchStates[convID]
	if !ok {
		m.app.SearchQuery = ""
		m.app.SearchPopupResults = nil
		m.app.SearchPopupSelectedIndex = 0
		m.app.SearchPopupScrollOffset = 0
		m.app.SearchStatus = ""
		return
	}
	m.app.SearchQuery = state.Query
	m.app.SearchPopupResults = state.Results
	m.app.SearchPopupSelectedIndex = state.SelectedIndex
	m.app.SearchPopupScrollOffset = state.ScrollOffset
	m.app.SearchStatus = state.Status
}

// getReactionKey creates a unique identifier for a reaction.
func getReactionKey(msgID string, r MessageReaction) string {
	userID := ""
	if r.User != nil && r.User.User != nil {
		if r.User.User.ID != nil {
			userID = *r.User.User.ID
		} else if r.User.User.DisplayName != nil {
			userID = *r.User.User.DisplayName
		}
	}
	return msgID + ":" + userID + ":" + r.ReactionType
}

// isOwnReaction checks if the reaction was added by the current user.
func (m Model) isOwnReaction(r MessageReaction) bool {
	if r.User == nil || r.User.User == nil {
		return false
	}
	if m.userID != "" && r.User.User.ID != nil && *r.User.User.ID == m.userID {
		return true
	}
	if m.app.CurrentUserName != nil && r.User.User.DisplayName != nil && *r.User.User.DisplayName == *m.app.CurrentUserName {
		return true
	}
	return false
}

// isOldReaction checks if the reaction was created before the app started.
func (m Model) isOldReaction(msg Message, r MessageReaction) bool {
	if r.CreatedDateTime != nil {
		if t, err := time.Parse(time.RFC3339, *r.CreatedDateTime); err == nil {
			return t.Before(m.app.AppStartTime)
		}
	}
	if t, err := time.Parse(time.RFC3339Nano, msg.CreatedDateTime); err == nil {
		return t.Before(m.app.AppStartTime)
	}
	return true
}

// getReactionKeys extracts unique reaction keys from a message, ignoring the current user's reactions.
func (m Model) getReactionKeys(msg *Message) []string {
	if msg == nil || len(msg.Reactions) == 0 {
		return nil
	}
	var keys []string
	for _, r := range msg.Reactions {
		if m.isOwnReaction(r) {
			continue
		}
		keys = append(keys, getReactionKey(msg.ID, r))
	}
	return keys
}

// hasUnreadReactions checks if the chat has any new reactions that the user has not seen.
func (m Model) hasUnreadReactions(c Chat) bool {
	return m.getLatestUnreadReactionEmoji(c) != ""
}

// getLatestUnreadReactionEmoji checks if the chat has any new reactions and returns the emoji of the most recent one.
func (m Model) getLatestUnreadReactionEmoji(c Chat) string {
	if chat := m.app.GetSelectedChat(); chat != nil && chat.ID == c.ID && m.focused {
		return ""
	}

	// Check reactions on LastMessagePreview first (as it represents the latest message)
	if c.LastMessagePreview != nil {
		for _, rKey := range m.getReactionKeys(c.LastMessagePreview) {
			if readMap, ok := m.lastReadReactions[c.ID]; !ok || !readMap[rKey] {
				for _, r := range c.LastMessagePreview.Reactions {
					if getReactionKey(c.LastMessagePreview.ID, r) == rKey {
						return reactionEmoji(r.ReactionType)
					}
				}
			}
		}
	}

	// Check reactions on cached HistoryMessages if we have them
	if hist, ok := m.app.HistoryMessages[c.ID]; ok {
		for _, msg := range hist {
			for _, rKey := range m.getReactionKeys(&msg) {
				if readMap, ok := m.lastReadReactions[c.ID]; !ok || !readMap[rKey] {
					for _, r := range msg.Reactions {
						if getReactionKey(msg.ID, r) == rKey {
							return reactionEmoji(r.ReactionType)
						}
					}
				}
			}
		}
	}

	return ""
}

// notifyReaction triggers a notification for newly detected reactions.
func (m *Model) notifyReaction(chat Chat, msg *Message, newReactions []MessageReaction) {
	chatName := ""
	if chat.CachedDisplayName != nil {
		chatName = *chat.CachedDisplayName
	}

	for _, r := range newReactions {
		reactorName := m.resolveReactorName(&chat, r)
		if r.User != nil && r.User.User != nil && r.User.User.ID != nil && reactorName == *r.User.User.ID {
			reactorName = "Someone"
		}

		emoji := reactionEmoji(r.ReactionType)
		msgBody := msg.GetPlainText()
		msgBody = stripANSI(msgBody)
		msgBody = strings.ReplaceAll(msgBody, "\n", " ")
		msgBody = strings.Join(strings.Fields(msgBody), " ")
		runes := []rune(msgBody)
		if len(runes) > 40 {
			msgBody = string(runes[:40]) + "..."
		}

		title := fmt.Sprintf("TeamsTUI: %s", reactorName)
		body := fmt.Sprintf("Reacted %s to: \"%s\"", emoji, msgBody)
		if chatName != "" && chatName != reactorName {
			body = fmt.Sprintf("Reacted %s to \"%s\" in %s", emoji, msgBody, chatName)
		}

		switch m.app.NotificationMode {
		case NotificationConsole:
			fmt.Print("\a") // BEL
			m.app.TriggerVisualBell()
		case NotificationSystem:
			beeep.AppName = "TeamsTUI"
			_ = beeep.Notify(title, body, "")
		case NotificationBoth:
			fmt.Print("\a")
			m.app.TriggerVisualBell()
			beeep.AppName = "TeamsTUI"
			_ = beeep.Notify(title, body, "")
		}
	}
}

// applyPendingEdits patches any in-memory pending edits into the live message
// caches for the given chat. It also removes entries from pendingEdits once
// the API has confirmed the updated content (i.e. the slice already contains
// the new HTML), preventing stale overrides from lingering indefinitely.
func (m *Model) applyPendingEdits(chatID string) {
	if len(m.pendingEdits) == 0 {
		return
	}
	patchSlice := func(msgs []Message) {
		for i := range msgs {
			newHTML, isPending := m.pendingEdits[msgs[i].ID]
			if !isPending {
				continue
			}
			// Check if the API has already reflected this edit.
			if msgs[i].Body != nil && msgs[i].Body.Content != nil && *msgs[i].Body.Content == newHTML {
				delete(m.pendingEdits, msgs[i].ID)
				continue
			}
			// Overwrite with our optimistic content.
			if msgs[i].Body == nil {
				body := MessageBody{Content: &newHTML}
				msgs[i].Body = &body
			} else {
				msgs[i].Body.Content = &newHTML
			}
			msgs[i].PlainTextCached = nil // force re-render
		}
	}
	patchSlice(m.app.Messages)
	if cached, ok := m.app.CachedMessages[chatID]; ok {
		patchSlice(cached)
	}
	if hist, ok := m.app.HistoryMessages[chatID]; ok {
		patchSlice(hist)
	}
}

// mergeHistoryMessages combines existing history messages with newly fetched ones,
// updating existing ones and prepending/sorting them newest-first.
func mergeHistoryMessages(existing []Message, newMsgs []Message) []Message {
	msgMap := make(map[string]Message)
	for _, msg := range existing {
		msgMap[msg.ID] = msg
	}
	for _, msg := range newMsgs {
		msgMap[msg.ID] = msg
	}

	var merged []Message
	for _, msg := range msgMap {
		merged = append(merged, msg)
	}

	sort.Slice(merged, func(i, j int) bool {
		return merged[i].CreatedDateTime > merged[j].CreatedDateTime
	})
	return merged
}

// updateCachedMessages merges a list of messages into the in-memory caches (CachedMessages, HistoryMessages, and the active Messages list if current) and persists them in SQLite.
func (m Model) updateCachedMessages(chatID string, msgs []Message) Model {
	if len(msgs) == 0 {
		return m
	}
	m.app.CachedMessages[chatID] = mergeHistoryMessages(m.app.CachedMessages[chatID], msgs)
	m.app.HistoryMessages[chatID] = mergeHistoryMessages(m.app.HistoryMessages[chatID], msgs)

	isActive := false
	if m.channelSelectedIndex < 0 {
		if chat := m.app.GetSelectedChat(); chat != nil && chat.ID == chatID {
			isActive = true
		}
	} else {
		if m.app.SelectedChannelID == chatID {
			isActive = true
		}
	}
	if isActive {
		m.app.Messages = mergeHistoryMessages(m.app.Messages, msgs)
	}

	if m.app.Features.SqliteEnabled {
		go SaveMessages(chatID, msgs)
	}
	return m
}


type UserSearchItemType int

const (
	UserSearchItemLocal UserSearchItemType = iota
	UserSearchItemDirectory
	UserSearchItemDirect
	UserSearchItemChannel
)

type UserSearchItem struct {
	Type        UserSearchItemType
	LocalChat   *Chat
	DirUser     *User
	DirectEmail string
	Channel     *channelEntry
}

func (m Model) getUserSearchItems() []UserSearchItem {
	var items []UserSearchItem

	// Local chats
	for i := range m.app.UserSearchLocalResults {
		items = append(items, UserSearchItem{
			Type:      UserSearchItemLocal,
			LocalChat: &m.app.UserSearchLocalResults[i],
		})
	}

	// Channels
	for i := range m.app.UserSearchChannelResults {
		items = append(items, UserSearchItem{
			Type:    UserSearchItemChannel,
			Channel: &m.app.UserSearchChannelResults[i],
		})
	}

	return items
}

func (m *Model) updateUserSearchLocalResults() {
	query := normalizeString(strings.ToLower(strings.TrimSpace(m.app.UserSearchQuery)))
	if query == "" {
		m.app.UserSearchLocalResults = nil
		m.app.UserSearchChannelResults = nil
		return
	}

	var matches []Chat
	for _, c := range m.app.Chats {
		name := ""
		if c.CachedDisplayName != nil {
			name = normalizeString(strings.ToLower(*c.CachedDisplayName))
		}

		memberMatch := false
		for _, mem := range c.Members {
			if mem.DisplayName != nil && strings.Contains(normalizeString(strings.ToLower(*mem.DisplayName)), query) {
				memberMatch = true
				break
			}
			if mem.Email != nil && strings.Contains(normalizeString(strings.ToLower(*mem.Email)), query) {
				memberMatch = true
				break
			}
		}

		if strings.Contains(name, query) || memberMatch {
			matches = append(matches, c)
		}
	}
	m.app.UserSearchLocalResults = matches

	var chanMatches []channelEntry
	if m.app.Features.TeamsChannels {
		for _, ch := range m.allChannels() {
			chanName := normalizeString(strings.ToLower(ch.channelName))
			teamName := normalizeString(strings.ToLower(ch.teamName))
			if strings.Contains(chanName, query) || strings.Contains(teamName, query) {
				chanMatches = append(chanMatches, ch)
			}
		}
	}
	m.app.UserSearchChannelResults = chanMatches
}

func (m Model) handleUserSearchInputModeKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.app.UserSearchMode = false
		m.userSearchInput.Blur()
		return m, nil

	case "down", "up", "tab":
		m.app.UserSearchMode = false
		m.userSearchInput.Blur()
		m.app.UserSearchSelectedIndex = 0
		return m, nil

	case "enter":
		query := strings.TrimSpace(m.userSearchInput.Value())
		m.app.UserSearchMode = false
		m.userSearchInput.Blur()
		if query != "" {
			if strings.Contains(query, "@") {
				m.app.UserSearchLoading = true
				m.app.UserSearchStatus = "Opening chat..."
				return m, createChatCmd(m.clientID, m.userID, query)
			}
		}
		return m, nil
	}

	return m, nil
}

func (m Model) handleUserSearchNavigationKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	items := m.getUserSearchItems()

	switch msg.String() {
	case "esc", "q":
		m.app.UserSearchPopupMode = false
		m.app.UserSearchMode = false
		return m, nil

	case "j", "down":
		if len(items) > 0 && m.app.UserSearchSelectedIndex < len(items)-1 {
			m.app.UserSearchSelectedIndex++
		}
		return m, nil

	case "k", "up":
		if len(items) > 0 && m.app.UserSearchSelectedIndex > 0 {
			m.app.UserSearchSelectedIndex--
		}
		return m, nil

	case "/":
		m.app.UserSearchMode = true
		m.userSearchInput.Focus()
		return m, textinput.Blink

	case "enter":
		if len(items) == 0 || m.app.UserSearchSelectedIndex >= len(items) {
			return m, nil
		}

		item := items[m.app.UserSearchSelectedIndex]
		if item.Type == UserSearchItemLocal {
			targetID := item.LocalChat.ID
			idx := -1
			for i, c := range m.app.Chats {
				if c.ID == targetID {
					idx = i
					break
				}
			}
			if idx != -1 {
				m.app.SelectedIndex = idx
				m.channelSelectedIndex = -1
				m.app.SelectedChannelTeamID = ""
				m.app.SelectedChannelID = ""
				m.app.UserSearchPopupMode = false
				m.app.UserSearchMode = false
				m.app.SnapToBottom = true
				return m.loadChatMessages(targetID, idx)
			}
		} else if item.Type == UserSearchItemChannel {
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
	title := titleStyle.Render("Find Local Chat or Start Direct Chat")

	instructions := lipgloss.NewStyle().Foreground(colDimGray).Render(
		" j/k: Nav | Enter: Open selected chat or typed email | /: Edit | Esc: Close",
	)

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
			list.WriteString(lipgloss.NewStyle().Foreground(colDimGray).Render("Type a name/email and press Enter/arrows.") + "\n")
		} else {
			list.WriteString(lipgloss.NewStyle().Foreground(colDimGray).Render("No matching local chats or channels found.") + "\n")
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
 
		linesRendered := 0
		for idx, item := range items {
			isSelected := idx == m.app.UserSearchSelectedIndex
			prefix := "  "
			if isSelected {
				prefix = "> "
			}
 
			var line string
			switch item.Type {
			case UserSearchItemLocal:
				chatName := "Unknown"
				if item.LocalChat.CachedDisplayName != nil {
					chatName = *item.LocalChat.CachedDisplayName
				}
				tag := lipgloss.NewStyle().Foreground(colGreen).Render("[Local Chat]")
				lineStr := fmt.Sprintf("%s %s %s", prefix, chatName, tag)
				if isSelected {
					line = lipgloss.NewStyle().Background(colDarkGray).Render(lineStr)
				} else {
					line = lineStr
				}
			case UserSearchItemChannel:
				chanName := item.Channel.channelName
				teamName := item.Channel.teamName
				tag := lipgloss.NewStyle().Foreground(colCyan).Render("[Channel]")
				lineStr := fmt.Sprintf("%s %s > %s %s", prefix, teamName, chanName, tag)
				if isSelected {
					line = lipgloss.NewStyle().Background(colDarkGray).Render(lineStr)
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
	}

	list.WriteString(statusText + "\n" + inputBox)

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colCyan).
		Padding(1, 2).
		Width(w).Height(h).
		Render(list.String())

	return box
}

// viewableAttachments returns the subset of a message's attachments that should
// be shown in the view popup — i.e. real files, excluding quoted-reply references
// (contentType "messageReference") which are already rendered inline as blockquotes.
func viewableAttachments(msg Message) []MessageAttachment {
	var out []MessageAttachment
	for _, att := range msg.Attachments {
		if att.ContentType != nil && strings.EqualFold(*att.ContentType, "messageReference") {
			continue
		}
		out = append(out, att)
	}
	return out
}

func (m Model) renderMessagePopup(w, h int) string {
	if len(m.app.Messages) == 0 || m.app.MessageSelectedIndex < 0 || m.app.MessageSelectedIndex >= len(m.app.Messages) {
		return ""
	}

	m.app.Messages[m.app.MessageSelectedIndex].ProcessInlineImages()
	msg := m.app.Messages[m.app.MessageSelectedIndex]

	sender := "Unknown"
	if msg.From != nil && msg.From.User != nil && msg.From.User.DisplayName != nil {
		sender = *msg.From.User.DisplayName
		if m.app.CurrentUserName != nil && sender == *m.app.CurrentUserName {
			sender = "Me"
		}
	}

	msgTime, _ := time.Parse(time.RFC3339Nano, msg.CreatedDateTime)
	msgTime = msgTime.Local()
	timeStr := ""
	if !msgTime.IsZero() {
		timeStr = msgTime.Format("Jan 02, 2006 15:04:05")
	}

	innerW := w - 6
	innerH := h - 4
	if innerW < 10 {
		innerW = 10
	}
	if innerH < 4 {
		innerH = 4
	}

	var showImagePreview bool
	var previewDownloading bool

	if m.app.Features.FilePreviewInTerminal && m.app.AttachmentCursorMode {
		if m.app.MessageSelectedIndex >= 0 && m.app.MessageSelectedIndex < len(m.app.Messages) {
			msgObj := m.app.Messages[m.app.MessageSelectedIndex]
			vAtts := viewableAttachments(msgObj)
			if m.app.AttachmentSelectedIndex >= 0 && m.app.AttachmentSelectedIndex < len(vAtts) {
				att := vAtts[m.app.AttachmentSelectedIndex]
				if isImageAttachment(att) {
					showImagePreview = true
					if cp, err := getAttachmentCachePath(att); err == nil {
						if _, err := os.Stat(cp); err != nil {
							previewDownloading = true
						}
					}
				}
			}
		}
	}

	previewW := 0
	contentW := innerW
	if showImagePreview {
		previewW = innerW * 45 / 100
		if previewW < 15 {
			previewW = 15
		}
		if previewW > innerW-25 {
			previewW = innerW - 25
		}
		if previewW >= 15 {
			contentW = innerW - previewW - 2
		} else {
			showImagePreview = false
		}
	}

	headerLines := []string{
		lipgloss.NewStyle().Foreground(colCyan).Bold(true).Render("From: ") + sender,
		lipgloss.NewStyle().Foreground(colCyan).Bold(true).Render("Date: ") + timeStr,
	}
	if msg.Subject != "" {
		headerLines = append(headerLines, lipgloss.NewStyle().Foreground(colCyan).Bold(true).Render("Subject: ") + msg.Subject)
	}
	headerLines = append(headerLines, "")

	vAtts := viewableAttachments(msg)
	attachmentsLines := []string{}
	if len(vAtts) > 0 {
		attHeaderStyle := lipgloss.NewStyle().Foreground(colYellow).Bold(true)
		attHeader := "Attachments:"
		if m.app.AttachmentCursorMode {
			attHeader += " [Tab:exit | ↑↓:select | Enter:download]"
		} else if m.app.Features.FilePreview {
			attHeader += " [Tab to select & download]"
		}
		attachmentsLines = append(attachmentsLines, attHeaderStyle.Render(attHeader))
		for i, att := range vAtts {
			name := "Unnamed attachment"
			if att.Name != nil && *att.Name != "" {
				name = *att.Name
			}
			contentType := ""
			if att.ContentType != nil && *att.ContentType != "" {
				contentType = fmt.Sprintf(" (%s)", *att.ContentType)
			}
			hasURL := att.ContentURL != nil && *att.ContentURL != ""
			urlIndicator := ""
			if hasURL && m.app.Features.FilePreview {
				urlIndicator = " ↓"
			}
			lineText := fmt.Sprintf("📎 %s%s%s", name, contentType, urlIndicator)
			var line string
			if m.app.AttachmentCursorMode && i == m.app.AttachmentSelectedIndex {
				line = lipgloss.NewStyle().Foreground(colCyan).Bold(true).Render("  ▶ " + lineText)
			} else {
				line = "  " + lineText
			}
			attachmentsLines = append(attachmentsLines, line)
		}
	}

	reactionsLines := []string{}
	reactionsGrouped := make(map[string][]string)
	var reactionOrder []string
	seenReactions := make(map[string]bool)

	for _, r := range msg.Reactions {
		rType := strings.ToLower(r.ReactionType)
		emoji := reactionEmoji(rType)
		name := m.resolveReactorName(nil, r)

		reactionsGrouped[emoji] = append(reactionsGrouped[emoji], name)
		if !seenReactions[emoji] {
			seenReactions[emoji] = true
			reactionOrder = append(reactionOrder, emoji)
		}
	}

	if len(msg.Reactions) > 0 {
		reactionsLines = append(reactionsLines, lipgloss.NewStyle().Foreground(colYellow).Bold(true).Render("Reactions:"))
		reactorsW := contentW - 6
		if reactorsW < 10 {
			reactorsW = 10
		}
		for _, emoji := range reactionOrder {
			names := reactionsGrouped[emoji]
			sort.Strings(names)
			namesStr := strings.Join(names, ", ")
			wrappedReactors := wordWrap(namesStr, reactorsW)
			for i, wrLine := range wrappedReactors {
				if i == 0 {
					reactionsLines = append(reactionsLines, fmt.Sprintf("  %s %s", emoji, wrLine))
				} else {
					reactionsLines = append(reactionsLines, fmt.Sprintf("    %s", wrLine))
				}
			}
		}
	}

	footer := lipgloss.NewStyle().Foreground(colDimGray).Italic(true).Render("Press ESC/q/v/Enter to close | j/k to navigate | J/K to scroll | ctrl+g to open in external editor")

	nonBodyH := len(headerLines) + 1
	if len(attachmentsLines) > 0 {
		nonBodyH += len(attachmentsLines) + 1
	}
	if len(reactionsLines) > 0 {
		nonBodyH += len(reactionsLines) + 1
	}

	bodyMaxH := innerH - nonBodyH
	if bodyMaxH < 4 {
		bodyMaxH = 4
	}

	body := msg.GetPlainText()
	var wrappedBody []string
	if body != "" {
		wrappedBody = wordWrap(body, contentW)
	}

	var bodyLines []string
	if len(wrappedBody) > 0 {
		viewportH := bodyMaxH - 2
		if viewportH < 1 {
			viewportH = 1
		}

		maxScroll := len(wrappedBody) - viewportH
		if maxScroll < 0 {
			maxScroll = 0
		}
		if m.app.MessagePopupScrollOffset > maxScroll {
			m.app.MessagePopupScrollOffset = maxScroll
		}
		if m.app.MessagePopupScrollOffset < 0 {
			m.app.MessagePopupScrollOffset = 0
		}

		visibleBody := wrappedBody
		if len(wrappedBody) > viewportH {
			start := m.app.MessagePopupScrollOffset
			end := start + viewportH
			if end > len(wrappedBody) {
				end = len(wrappedBody)
			}
			visibleBody = wrappedBody[start:end]
		}

		headerText := lipgloss.NewStyle().Foreground(colYellow).Bold(true).Render("Message:")
		if len(wrappedBody) > viewportH {
			headerText += lipgloss.NewStyle().Foreground(colDimGray).Render(fmt.Sprintf(" (Shift+J/K to scroll - %d/%d)", m.app.MessagePopupScrollOffset+1, len(wrappedBody)))
		}
		bodyLines = append(bodyLines, headerText)
		bodyLines = append(bodyLines, visibleBody...)
		bodyLines = append(bodyLines, "")
	}

	var finalLines []string
	finalLines = append(finalLines, headerLines...)
	finalLines = append(finalLines, bodyLines...)
	if len(attachmentsLines) > 0 {
		finalLines = append(finalLines, attachmentsLines...)
		finalLines = append(finalLines, "")
	}
	if len(reactionsLines) > 0 {
		finalLines = append(finalLines, reactionsLines...)
		finalLines = append(finalLines, "")
	}

	targetH := innerH - 1
	if len(finalLines) > targetH {
		finalLines = finalLines[:targetH]
	} else {
		for len(finalLines) < targetH {
			finalLines = append(finalLines, "")
		}
	}
	finalLines = append(finalLines, footer)

	var combinedContent string
	if showImagePreview {
		var previewText string
		borderColor := colDarkGray
		if previewDownloading {
			previewText = "\n⏳ Loading preview..."
		} else {
			borderColor = colGreen
		}

		rightPanelStr := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(borderColor).
			Width(previewW).Height(targetH).
			Align(lipgloss.Center, lipgloss.Center).
			Render(previewText)

		leftPanelBodyStr := strings.Join(finalLines[:targetH], "\n")
		leftAndRight := lipgloss.JoinHorizontal(lipgloss.Top,
			lipgloss.NewStyle().Width(contentW).Render(leftPanelBodyStr),
			"  ",
			rightPanelStr,
		)
		combinedContent = leftAndRight + "\n" + footer
	} else {
		combinedContent = strings.Join(finalLines, "\n")
	}

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colCyan).
		Padding(1, 2).
		Width(w).Height(h).
		Render(combinedContent)

	return box
}

func (m Model) resolveReactorName(chat *Chat, r MessageReaction) string {
	if r.User == nil || r.User.User == nil {
		return "Someone"
	}

	if r.User.User.ID != nil && *r.User.User.ID != "" && m.app.CurrentUserID != "" && *r.User.User.ID == m.app.CurrentUserID {
		return "Me"
	}
	if r.User.User.DisplayName != nil && *r.User.User.DisplayName != "" {
		if m.app.CurrentUserName != nil && *r.User.User.DisplayName == *m.app.CurrentUserName {
			return "Me"
		}
	}

	if r.User.User.DisplayName != nil && *r.User.User.DisplayName != "" {
		return *r.User.User.DisplayName
	}

	if r.User.User.ID != nil && *r.User.User.ID != "" {
		if chat != nil {
			for _, member := range chat.Members {
				match := false
				if member.UserID != nil && *member.UserID == *r.User.User.ID {
					match = true
				} else if member.ID != nil && *member.ID == *r.User.User.ID {
					match = true
				}
				if match {
					if member.DisplayName != nil && *member.DisplayName != "" {
						return *member.DisplayName
					}
				}
			}
		}
		if ch := m.activeChannelEntry(); ch != nil {
			for _, member := range m.app.TeamMembersCache[ch.teamID] {
				match := false
				if member.UserID != nil && *member.UserID == *r.User.User.ID {
					match = true
				} else if member.ID != nil && *member.ID == *r.User.User.ID {
					match = true
				}
				if match {
					if member.DisplayName != nil && *member.DisplayName != "" {
						return *member.DisplayName
					}
				}
			}
		}
		if selChat := m.app.GetSelectedChat(); selChat != nil {
			for _, member := range selChat.Members {
				match := false
				if member.UserID != nil && *member.UserID == *r.User.User.ID {
					match = true
				} else if member.ID != nil && *member.ID == *r.User.User.ID {
					match = true
				}
				if match {
					if member.DisplayName != nil && *member.DisplayName != "" {
						return *member.DisplayName
					}
				}
			}
		}
	}

	if r.User.User.ID != nil && *r.User.User.ID != "" {
		return *r.User.User.ID
	}

	return "Someone"
}

// ---------------------------------------------------------------------------
// Help popup
// ---------------------------------------------------------------------------

func (m Model) getHelpContentLines() []string {
	labelStyle := lipgloss.NewStyle().Foreground(colYellow).Bold(true)
	keyStyle := lipgloss.NewStyle().Foreground(colCyan)
	dimStyle := lipgloss.NewStyle().Foreground(colDimGray)

	sections := []struct {
		name  string
		binds [][2]string
	}{
		{"Navigation", [][2]string{
			{"j / ↓", "Navigate list down (within section)"},
			{"k / ↑", "Navigate list up (within section)"},
			{"Tab", "Switch between Chats & Channels"},
			{"m", "Enter message selection mode"},
			{"i", "Compose new message"},
			{"c", "Open chat search / open chat"},
			{"/", "Search message history"},
			{"f", "Toggle favourite (chats only)"},
			{"h", "Toggle hide/unhide channel (channels only)"},
			{"p", "Presence status of chat participants (chats only, feature: presence_enabled)"},
			{"n", "Cycle notification mode"},
			{"ESC", "Enter sleep / idle mode (stop polling)"},
			{"?", "Show this help"},
			{"q / Ctrl+C", "Quit"},
		}},
		{"Message Selection (m)", [][2]string{
			{"j / k", "Navigate messages"},
			{"v", "View message popup"},
			{"y", "Yank message to clipboard"},
			{"u", "Extract URLs"},
			{"o", "Open URLs"},
			{"r", "React to message"},
			{"a", "Reply (quote) message"},
			{"d", "Delete message"},
			{"e", "Edit message"},
			{"p", "Presence status (feature: presence_enabled)"},
			{"i", "User profile info (feature: user_profile_enabled)"},
			{"ESC / m", "Exit selection mode"},
		}},
		{"Message View Popup (v)", [][2]string{
			{"j / k", "Navigate to next/prev message"},
			{"J / K", "Scroll message body"},
			{"Tab", "Switch to attachment cursor mode"},
			{"Enter", "Download selected attachment (feature: file_preview_enabled)"},
			{"ESC / q / v", "Close popup"},
		}},
		{"History Search (/)", [][2]string{
			{"Enter", "Submit query / focus results / Expand context"},
			{"j / k", "Navigate results"},
			{"y", "Yank selected message"},
			{"u", "Extract URLs"},
			{"o", "Open URLs"},
			{"g", "Go to message in normal view"},
			{"ESC", "Close search popup"},
		}},
		{"Chat Search (c)", [][2]string{
			{"Type", "Filter local chats"},
			{"Enter", "Open selected chat / direct open by email"},
			{"j / k", "Navigate results"},
			{"ESC", "Close popup"},
		}},
		{"Composing Messages", [][2]string{
			{"Type", "Write message (Alt+Enter for newline)"},
			{"@", "Open autocomplete mention popup"},
			{"j / k / Tab", "Navigate suggestions (when mention popup is open)"},
			{"Enter", "Select suggestion (when open) / Send message"},
			{"Ctrl+v", "Paste image from clipboard"},
			{"Ctrl+f", "Browse and attach file (feature: file_upload_enabled)"},
			{"Ctrl+g", "Compose/edit in external editor (e.g. vim)"},
			{"ESC", "Cancel composing"},
		}},
	}

	var contentLines []string

	// Optional features status
	contentLines = append(contentLines, labelStyle.Render("Optional Features:"))
	featureState := func(enabled bool) string {
		if enabled {
			return lipgloss.NewStyle().Foreground(colGreen).Render("✓ enabled")
		}
		return dimStyle.Render("✗ disabled")
	}
	contentLines = append(contentLines,
		fmt.Sprintf("  file_preview_enabled      %s", featureState(m.app.Features.FilePreview)),
		fmt.Sprintf("  file_preview_in_terminal  %s", featureState(m.app.Features.FilePreviewInTerminal)),
		fmt.Sprintf("  file_upload_enabled       %s", featureState(m.app.Features.FileUpload)),
		fmt.Sprintf("  presence_enabled          %s", featureState(m.app.Features.Presence)),
		fmt.Sprintf("  user_profile_enabled      %s", featureState(m.app.Features.UserProfile)),
		fmt.Sprintf("  teams_channels_enabled    %s", featureState(m.app.Features.TeamsChannels)),
	)
	contentLines = append(contentLines, "")

	for _, sec := range sections {
		contentLines = append(contentLines, labelStyle.Render(sec.name))
		for _, bind := range sec.binds {
			key := keyStyle.Render(fmt.Sprintf("  %-22s", bind[0]))
			contentLines = append(contentLines, key+bind[1])
		}
		contentLines = append(contentLines, "")
	}

	return contentLines
}

func (m Model) clampHelpScrollOffset() {
	popupH := m.height * 85 / 100
	if popupH < 10 {
		popupH = 10
	}
	innerH := popupH - 4
	if innerH < 4 {
		innerH = 4
	}
	viewportH := innerH - 3
	if viewportH < 1 {
		viewportH = 1
	}

	totalLines := len(m.getHelpContentLines())
	maxScroll := totalLines - viewportH
	if maxScroll < 0 {
		maxScroll = 0
	}
	if m.app.HelpScrollOffset > maxScroll {
		m.app.HelpScrollOffset = maxScroll
	}
	if m.app.HelpScrollOffset < 0 {
		m.app.HelpScrollOffset = 0
	}
}

func (m Model) handleHelpPopupKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q", "?", "enter":
		m.app.HelpPopupMode = false
	case "j", "down":
		m.app.HelpScrollOffset++
		m.clampHelpScrollOffset()
	case "k", "up":
		m.app.HelpScrollOffset--
		m.clampHelpScrollOffset()
	}
	return m, nil
}

func (m Model) handleFilePickerKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		m.app.FilePickerPopupMode = false
		return m, nil
	}

	var cmd tea.Cmd
	m.filepicker, cmd = m.filepicker.Update(msg)

	if msg.String() == "s" || msg.String() == "ctrl+s" || msg.String() == "o" || msg.String() == "ctrl+o" {
		_ = SaveFilepickerSettings(m.filepicker.SortBy.String(), m.filepicker.SortOrder.String(), m.filepicker.CurrentDirectory)
	}

	if didSelect, path := m.filepicker.DidSelectFile(msg); didSelect {
		_ = SaveFilepickerSettings(m.filepicker.SortBy.String(), m.filepicker.SortOrder.String(), m.filepicker.CurrentDirectory)
		m.app.FilePickerPopupMode = false
		m.app.SkipTextareaUpdate = true
		return m, tea.Batch(cmd, attachFileFromFilepathCmd(path))
	}

	return m, cmd
}

func (m Model) renderHelpPopup(w, h int) string {
	dimStyle := lipgloss.NewStyle().Foreground(colDimGray)

	// Ensure HelpScrollOffset is properly clamped (e.g. if terminal resized)
	m.clampHelpScrollOffset()

	innerH := h - 4
	if innerH < 4 {
		innerH = 4
	}

	viewportH := innerH - 3
	if viewportH < 1 {
		viewportH = 1
	}

	contentLines := m.getHelpContentLines()
	totalContentLines := len(contentLines)
	maxScroll := totalContentLines - viewportH
	if maxScroll < 0 {
		maxScroll = 0
	}

	var scrollIndicator string
	if totalContentLines > viewportH {
		percent := int(float64(m.app.HelpScrollOffset) / float64(maxScroll) * 100)
		scrollIndicator = fmt.Sprintf(" %s %d%%", dimStyle.Render("• Scroll j/k or ↓/↑ •"), percent)
	}

	title := lipgloss.NewStyle().Foreground(colCyan).Bold(true).Render("Keyboard Shortcuts") + scrollIndicator

	var visibleContent []string
	if totalContentLines > 0 {
		start := m.app.HelpScrollOffset
		end := start + viewportH
		if end > totalContentLines {
			end = totalContentLines
		}
		visibleContent = contentLines[start:end]
	}

	// Pad if visibleContent is less than viewportH
	for len(visibleContent) < viewportH {
		visibleContent = append(visibleContent, "")
	}

	var lines []string
	lines = append(lines, title, "")
	lines = append(lines, visibleContent...)

	footer := dimStyle.Italic(true).Render("Press ESC / q / ? to close")
	lines = append(lines, footer)

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colYellow).
		Padding(1, 2).
		Width(w).Height(h).
		Render(strings.Join(lines, "\n"))
}

// ---------------------------------------------------------------------------
// Presence popup
// ---------------------------------------------------------------------------

func (m Model) handlePresencePopupKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q", "p", "enter":
		m.app.PresencePopupMode = false
		m.app.PresenceChatMode = false
		m.app.PresenceData = nil
		m.app.PresenceChatData = nil
	case "up", "k":
		if m.app.PresenceChatMode && m.app.PresenceScrollOffset > 0 {
			m.app.PresenceScrollOffset--
		}
	case "down", "j":
		if m.app.PresenceChatMode {
			popupH := m.height * 65 / 100
			if popupH < 10 {
				popupH = 10
			}
			availableHeight := popupH - 6
			if availableHeight < 1 {
				availableHeight = 1
			}
			maxOffset := len(m.app.PresenceChatData) - availableHeight
			if maxOffset < 0 {
				maxOffset = 0
			}
			if m.app.PresenceScrollOffset < maxOffset {
				m.app.PresenceScrollOffset++
			}
		}
	}
	return m, nil
}

func presenceAvailabilityColor(availability string) lipgloss.Color {
	switch availability {
	case "Available":
		return colGreen
	case "Busy", "DoNotDisturb":
		return colRed
	case "Away", "BeRightBack":
		return colYellow
	default:
		return colDimGray
	}
}

func presenceAvailabilityIcon(availability string) string {
	switch availability {
	case "Available":
		return "🟢"
	case "Busy":
		return "🔴"
	case "DoNotDisturb":
		return "⛔"
	case "Away", "BeRightBack":
		return "🟡"
	case "Offline", "PresenceUnknown":
		return "⚫"
	default:
		return "❓"
	}
}

func (m Model) renderPresencePopup(w, h int) string {
	innerW := w - 6
	if innerW < 20 {
		innerW = 20
	}

	title := lipgloss.NewStyle().Foreground(colCyan).Bold(true).Render("User Presence")
	labelStyle := lipgloss.NewStyle().Foreground(colCyan).Bold(true)
	dimStyle := lipgloss.NewStyle().Foreground(colDimGray)

	var lines []string
	lines = append(lines, title, "")

	if m.app.PresenceChatMode {
		lines = append(lines, labelStyle.Render("Chat: ")+m.app.PresenceUserName, "")
		if m.app.PresenceLoading {
			lines = append(lines, dimStyle.Render("Loading presence..."))
		} else if len(m.app.PresenceChatData) == 0 {
			lines = append(lines, lipgloss.NewStyle().Foreground(colRed).Render("No presence data available"))
		} else {
			headerLines := 4
			footerLines := 2
			availableHeight := h - headerLines - footerLines
			if availableHeight < 1 {
				availableHeight = 1
			}

			maxOffset := len(m.app.PresenceChatData) - availableHeight
			if maxOffset < 0 {
				maxOffset = 0
			}
			if m.app.PresenceScrollOffset > maxOffset {
				m.app.PresenceScrollOffset = maxOffset
			}

			for i := m.app.PresenceScrollOffset; i < len(m.app.PresenceChatData) && i < m.app.PresenceScrollOffset+availableHeight; i++ {
				entry := m.app.PresenceChatData[i]
				availColor := presenceAvailabilityColor(entry.Availability)
				icon := presenceAvailabilityIcon(entry.Availability)
				availStr := lipgloss.NewStyle().Foreground(availColor).Bold(true).Render(entry.Availability)

				name := entry.UserName
				maxNameLen := innerW - 15
				if maxNameLen < 10 {
					maxNameLen = 10
				}
				if len(name) > maxNameLen {
					name = name[:maxNameLen-3] + "..."
				}

				statusLine := fmt.Sprintf("%-*s %s %s", maxNameLen, name, icon, availStr)
				if entry.Activity != "" && entry.Activity != entry.Availability {
					statusLine += dimStyle.Render(" (" + entry.Activity + ")")
				}
				lines = append(lines, statusLine)
			}

			if len(m.app.PresenceChatData) > availableHeight {
				scrollIndicator := dimStyle.Render(fmt.Sprintf(" (Showing %d-%d of %d, use j/k to scroll)",
					m.app.PresenceScrollOffset+1,
					min(m.app.PresenceScrollOffset+availableHeight, len(m.app.PresenceChatData)),
					len(m.app.PresenceChatData)))
				lines[2] = labelStyle.Render("Chat: ") + m.app.PresenceUserName + scrollIndicator
			}
		}
	} else {
		lines = append(lines, labelStyle.Render("User: ")+m.app.PresenceUserName, "")

		if m.app.PresenceLoading {
			lines = append(lines, dimStyle.Render("Loading presence..."))
		} else if m.app.PresenceData == nil {
			lines = append(lines, lipgloss.NewStyle().Foreground(colRed).Render("Presence data unavailable"))
		} else {
			p := m.app.PresenceData
			availColor := presenceAvailabilityColor(p.Availability)
			icon := presenceAvailabilityIcon(p.Availability)
			availStr := lipgloss.NewStyle().Foreground(availColor).Bold(true).Render(p.Availability)
			lines = append(lines,
				labelStyle.Render("Status:   ")+icon+" "+availStr,
			)
			if p.Activity != "" && p.Activity != p.Availability {
				lines = append(lines, labelStyle.Render("Activity: ")+p.Activity)
			}
		}
	}

	footer := dimStyle.Italic(true).Render("Press ESC / q / p to close")
	innerH := h - 4
	if innerH < 4 {
		innerH = 4
	}
	for len(lines) < innerH-1 {
		lines = append(lines, "")
	}
	if len(lines) > innerH-1 {
		lines = lines[:innerH-1]
	}
	lines = append(lines, footer)

	borderCol := colCyan
	if m.app.PresenceChatMode {
		borderCol = colGreen
	}

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderCol).
		Padding(1, 2).
		Width(w).Height(h).
		Render(strings.Join(lines, "\n"))
}

// ---------------------------------------------------------------------------
// User Profile popup
// ---------------------------------------------------------------------------

func (m Model) handleUserProfilePopupKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q", "i", "enter":
		m.app.UserProfilePopupMode = false
		m.app.UserProfileData = nil
	}
	return m, nil
}

func (m Model) renderUserProfilePopup(w, h int) string {
	innerW := w - 6
	if innerW < 20 {
		innerW = 20
	}
	_ = innerW

	title := lipgloss.NewStyle().Foreground(colCyan).Bold(true).Render("User Profile")
	labelStyle := lipgloss.NewStyle().Foreground(colCyan).Bold(true)
	dimStyle := lipgloss.NewStyle().Foreground(colDimGray)

	strOr := func(s *string) string {
		if s == nil || *s == "" {
			return dimStyle.Render("—")
		}
		return *s
	}

	var lines []string
	lines = append(lines, title, "")

	if m.app.UserProfileLoading {
		lines = append(lines, dimStyle.Render("Loading profile..."))
	} else if m.app.UserProfileData == nil {
		lines = append(lines, lipgloss.NewStyle().Foreground(colRed).Render("Profile data unavailable"))
	} else {
		p := m.app.UserProfileData
		lines = append(lines,
			labelStyle.Render("Name:       ")+p.DisplayName,
		)
		if p.Mail != nil && *p.Mail != "" {
			lines = append(lines, labelStyle.Render("Email:      ")+*p.Mail)
		} else if p.UserPrincipalName != nil && *p.UserPrincipalName != "" {
			lines = append(lines, labelStyle.Render("UPN:        ")+*p.UserPrincipalName)
		}
		if m.app.Features.ProfileExtended {
			lines = append(lines,
				labelStyle.Render("Job Title:  ")+strOr(p.JobTitle),
				labelStyle.Render("Department: ")+strOr(p.Department),
				labelStyle.Render("Office:     ")+strOr(p.OfficeLocation),
			)
			if p.MobilePhone != nil && *p.MobilePhone != "" {
				lines = append(lines, labelStyle.Render("Mobile:     ")+*p.MobilePhone)
			}
		} else {
			lines = append(lines, "", dimStyle.Italic(true).Render("Enable 'user_profile_extended' in config.json for job title, department, and more."))
		}
	}

	footer := dimStyle.Italic(true).Render("Press ESC / q / i to close")
	innerH := h - 4
	if innerH < 4 {
		innerH = 4
	}
	for len(lines) < innerH-1 {
		lines = append(lines, "")
	}
	if len(lines) > innerH-1 {
		lines = lines[:innerH-1]
	}
	lines = append(lines, footer)

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colGreen).
		Padding(1, 2).
		Width(w).Height(h).
		Render(strings.Join(lines, "\n"))
}

// ---------------------------------------------------------------------------
// getDownloadsDir returns the XDG downloads directory or ~/Downloads
// ---------------------------------------------------------------------------
func getDownloadsDir() string {
	// Try XDG_DOWNLOAD_DIR environment variable first.
	if dir := os.Getenv("XDG_DOWNLOAD_DIR"); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return filepath.Join(home, "Downloads")
}

// openFile attempts to open a file using the OS-specific default application.
func openFile(path string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", path)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", path)
	default:
		cmd = exec.Command("xdg-open", path)
	}
	return cmd.Start()
}

// checkAndTriggerPreviewDownload verifies the current selected attachment in cursor mode,
// and triggers an asynchronous preview download if it's an image and not yet cached.
func (m Model) checkAndTriggerPreviewDownload() tea.Cmd {
	if !m.app.Features.FilePreviewInTerminal || !m.app.AttachmentCursorMode {
		return nil
	}
	if m.app.MessageSelectedIndex < 0 || m.app.MessageSelectedIndex >= len(m.app.Messages) {
		return nil
	}
	msgObj := m.app.Messages[m.app.MessageSelectedIndex]
	vAtts := viewableAttachments(msgObj)
	if m.app.AttachmentSelectedIndex < 0 || m.app.AttachmentSelectedIndex >= len(vAtts) {
		return nil
	}
	att := vAtts[m.app.AttachmentSelectedIndex]
	if !isImageAttachment(att) {
		// Not an image, clear any displayed preview
		return clearKittyImagesCmd()
	}
	if att.ContentURL == nil || *att.ContentURL == "" {
		return nil
	}

	cachePath, err := getAttachmentCachePath(att)
	if err != nil {
		return nil
	}

	// Check if already downloaded
	if _, err := os.Stat(cachePath); err == nil {
		// Already exists! Just trigger a redraw
		return func() tea.Msg {
			return MsgPreviewDownloaded{DestPath: cachePath}
		}
	}

	// Download in background
	return downloadPreviewCmd(m.clientID, *att.ContentURL, cachePath)
}

// getCursorPos calculates the absolute cursor index in runes inside the textarea's value.
func getCursorPos(ta textarea.Model) int {
	lines := strings.Split(ta.Value(), "\n")
	cursorLine := ta.Line()
	if cursorLine < 0 || cursorLine >= len(lines) {
		return len([]rune(ta.Value()))
	}
	pos := 0
	for i := 0; i < cursorLine; i++ {
		pos += len([]rune(lines[i])) + 1
	}
	pos += ta.LineInfo().CharOffset
	return pos
}

// getMentionQuery looks backward from the cursor in a string to find an active '@' mention search.
// An '@' only counts as a mention when it starts a word: it must be at the start of the
// string or immediately preceded by whitespace. Embedded '@' characters (e.g. inside an
// email address like jvernon@amsci.org) are ignored.
func getMentionQuery(val string, cursor int) (int, string, bool) {
	runes := []rune(val)
	if cursor < 0 || cursor > len(runes) {
		return -1, "", false
	}
	for i := cursor - 1; i >= 0; i-- {
		r := runes[i]
		if r == ' ' || r == '\n' || r == '\r' || r == '\t' {
			break
		}
		if r == '@' {
			if i > 0 {
				prev := runes[i-1]
				if prev != ' ' && prev != '\n' && prev != '\r' && prev != '\t' {
					break
				}
			}
			query := string(runes[i+1 : cursor])
			return i, query, true
		}
	}
	return -1, "", false
}

// rebuildMentionSuggestions filters the list of available members for mentions.
func (m Model) rebuildMentionSuggestions() Model {
	var candidates []ChatMember

	if m.channelSelectedIndex >= 0 {
		if m.app.Features.ChannelMentions {
			chans := m.allChannels()
			if m.channelSelectedIndex < len(chans) {
				teamID := chans[m.channelSelectedIndex].teamID
				candidates = m.app.TeamMembersCache[teamID]
			}
		}
	} else {
		chat := m.app.GetSelectedChat()
		if chat != nil {
			candidates = chat.Members
		}
	}

	var suggestions []ChatMember
	searchLower := strings.ToLower(m.app.MentionSearch)

	for _, c := range candidates {
		if c.DisplayName == nil || c.UserID == nil {
			continue
		}
		if m.app.CurrentUserName != nil && *c.DisplayName == *m.app.CurrentUserName {
			continue
		}

		name := strings.ToLower(*c.DisplayName)
		email := ""
		if c.Email != nil {
			email = strings.ToLower(*c.Email)
		}

		if strings.Contains(name, searchLower) || strings.Contains(email, searchLower) {
			suggestions = append(suggestions, c)
		}
	}

	m.app.MentionSuggestions = suggestions
	if m.app.MentionSelectedIndex >= len(suggestions) {
		m.app.MentionSelectedIndex = 0
	}
	if m.app.MentionSelectedIndex < 0 {
		m.app.MentionSelectedIndex = 0
	}

	limit := 5
	if m.app.MentionScrollOffset+limit > len(suggestions) {
		m.app.MentionScrollOffset = len(suggestions) - limit
	}
	if m.app.MentionScrollOffset < 0 {
		m.app.MentionScrollOffset = 0
	}

	return m
}

func (m Model) loadChatMessages(chatID string, chatIndex int) (Model, tea.Cmd) {
	m.lastMessageRefresh = time.Now()
	// 1. Check in-memory cache first if it has been fully loaded once in this session.
	if m.app.ChatMessagesLoadedOnce[chatID] {
		if cached, ok := m.app.CachedMessages[chatID]; ok && len(cached) > 0 {
			m.app.Messages = cached
			m.app.NextLink = m.app.CachedNextLink[chatID]
			m.app.SnapToBottom = true
			if m.app.ChatCacheDirty[chatID] {
				m.app.SetLoadingMessages(true)
				return m, loadMessagesCmd(m.clientID, chatID, chatIndex)
			}
			m.app.SetLoadingMessages(false)
			return m, nil
		}
	}

	// 2. If SQLite enabled, check DB
	if m.app.Features.SqliteEnabled {
		dbMsgs, err := GetStoredMessages(chatID, ResolveMessageLimit())
		if err == nil && len(dbMsgs) > 0 {
			m.app.CachedMessages[chatID] = dbMsgs
			nextLink, _ := GetNextLink(chatID)
			m.app.CachedNextLink[chatID] = nextLink

			m.app.Messages = dbMsgs
			m.app.NextLink = nextLink
			m.app.SetLoadingMessages(false)
			m.app.SnapToBottom = true
			m.app.ChatMessagesLoadedOnce[chatID] = true
			// Still fetch the latest messages in the background to update the DB and cache!
			return m, loadMessagesCmd(m.clientID, chatID, chatIndex)
		}
	}

	// 3. Fallback to API load (using cached message, e.g. LastMessagePreview, as a placeholder if available)
	if cached, ok := m.app.CachedMessages[chatID]; ok && len(cached) > 0 {
		m.app.Messages = cached
		m.app.NextLink = m.app.CachedNextLink[chatID]
		m.app.SetLoadingMessages(true)
		m.app.SnapToBottom = true
	} else {
		m.app.Messages = nil
		m.app.NextLink = ""
		m.app.SetLoadingMessages(true)
		m.app.SnapToBottom = true
	}
	return m, loadMessagesCmd(m.clientID, chatID, chatIndex)
}

func (m Model) loadChannelMessages(teamID string, channelID string) (Model, tea.Cmd) {
	m.lastMessageRefresh = time.Now()
	// 1. Check in-memory cache first
	if cached, ok := m.app.CachedMessages[channelID]; ok && len(cached) > 0 {
		m.app.Messages = cached
		m.app.NextLink = m.app.CachedNextLink[channelID]
		m.app.SetLoadingMessages(false)
		m.app.SnapToBottom = true
		return m, nil
	}

	// 2. If SQLite enabled, check DB
	if m.app.Features.SqliteEnabled {
		dbMsgs, err := GetStoredMessages(channelID, ResolveMessageLimit())
		if err == nil && len(dbMsgs) > 0 {
			m.app.CachedMessages[channelID] = dbMsgs
			nextLink, _ := GetNextLink(channelID)
			m.app.CachedNextLink[channelID] = nextLink

			m.app.Messages = dbMsgs
			m.app.NextLink = nextLink
			m.app.SetLoadingMessages(false)
			m.app.SnapToBottom = true
			// Still fetch the latest messages in the background to update the DB and cache!
			return m, loadChannelMessagesCmd(m.clientID, teamID, channelID)
		}
	}

	// 3. Fallback to API load
	m.app.Messages = nil
	m.app.NextLink = ""
	m.app.SetLoadingMessages(true)
	m.app.SnapToBottom = true
	return m, loadChannelMessagesCmd(m.clientID, teamID, channelID)
}

func (m Model) renderFilePickerPopup(w, h int) string {
	dimStyle := lipgloss.NewStyle().Foreground(colDimGray)
	title := lipgloss.NewStyle().Foreground(colCyan).Bold(true).Render("Select File to Attach")

	currentDir := lipgloss.NewStyle().Foreground(colWhite).Bold(true).Render("Directory: " + m.filepicker.CurrentDirectory)
	sortMode := lipgloss.NewStyle().Foreground(colYellow).Render(fmt.Sprintf("Sorted by: %s (%s)", m.filepicker.SortBy.String(), m.filepicker.SortOrder.String()))

	var lines []string
	lines = append(lines, title, currentDir, sortMode, "")

	// Render the filepicker component
	lines = append(lines, m.filepicker.View())

	footer := dimStyle.Italic(true).Render("j/k or ↑/↓: Navigate • s: Change Sort • o: Change Order • Enter: Attach • Esc / q: Cancel")
	lines = append(lines, "", footer)

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colGreen).
		Padding(1, 2).
		Width(w).Height(h).
		Render(strings.Join(lines, "\n"))
}

