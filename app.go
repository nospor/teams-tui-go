package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"
)

// NotificationMode controls how the app notifies the user of new messages.
type NotificationMode int

const (
	NotificationNone    NotificationMode = iota // 0 — no notifications
	NotificationConsole                         // 1 — BEL + visual bell
	NotificationSystem                          // 2 — desktop notification
	NotificationBoth                            // 3 — BEL + visual bell + desktop
)

// String returns the human-readable name for a NotificationMode.
func (n NotificationMode) String() string {
	switch n {
	case NotificationConsole:
		return "Console"
	case NotificationSystem:
		return "System"
	case NotificationBoth:
		return "Both"
	default:
		return "None"
	}
}

// MarshalJSON serialises NotificationMode as a string so config.json is readable.
func (n NotificationMode) MarshalJSON() ([]byte, error) {
	return json.Marshal(n.String())
}

// UnmarshalJSON parses a NotificationMode from its string representation.
func (n *NotificationMode) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		// Also accept integer representation for backward compatibility.
		var i int
		if err2 := json.Unmarshal(data, &i); err2 != nil {
			return err
		}
		*n = NotificationMode(i)
		return nil
	}
	switch s {
	case "Console":
		*n = NotificationConsole
	case "System":
		*n = NotificationSystem
	case "Both":
		*n = NotificationBoth
	default:
		*n = NotificationNone
	}
	return nil
}

// ---------------------------------------------------------------------------
// App — central application state
// ---------------------------------------------------------------------------

// FeatureFlags holds resolved optional feature flags for the running session.
// Populated once at startup from config to avoid repeated file reads.
type FeatureFlags struct {
	FilePreview           bool // requires Files.Read
	FilePreviewInTerminal bool // show image in terminal if FilePreview is enabled
	FileUpload            bool // requires Files.ReadWrite
	Presence              bool // requires Presence.Read.All
	UserProfile           bool // requires User.ReadBasic.All (or User.Read.All if Extended)
	ProfileExtended       bool // requires User.Read.All (admin consent)
	TeamsChannels         bool // requires Team.ReadBasic.All + Channel.ReadBasic.All
	ChannelMentions       bool // requires TeamMember.Read.All
	SqliteEnabled         bool
}

// App holds all runtime state for the Teams TUI application.
type App struct {
	Chats                      []Chat
	Status                     string
	SearchStatus               string
	SelectedIndex              int
	CurrentUserName            *string
	CurrentUserID              string // used for markChatRead
	Messages                   []Message
	LoadingMessages            bool
	SearchLoadingMessages      bool
	InputMode                  bool
	InputBuffer                string
	ScrollOffset               int
	MaxScroll                  int
	ChatScrollOffset           int
	ChannelScrollOffset        int
	SnapToBottom               bool
	MessageSelectedIndex       int
	MessageSelectionMode       bool
	MessagePopupMode           bool
	MessagePopupScrollOffset   int
	ReactionMode               bool
	DeleteConfirmMode          bool
	NotificationMode           NotificationMode
	NotificationShowPreview    bool
	NotificationPreviewLen     int
	VisualBellUntil            *time.Time
	StatusUntil                *time.Time
	SearchStatusUntil          *time.Time
	MessagePopupStatus         string
	MessagePopupStatusUntil    *time.Time
	NextLink                   string
	PendingScrollID            string
	EditingMessageID              *string
	EditingReferenceAttachments   []MessageAttachment // reference file attachments from the message being edited
	EditingInlineImageURLs        []string            // original inline <img> src URLs in placeholder order
	ReplyToMessage             *Message // set when user presses 'a' to reply-quote a message
	UrlSelectionMode           bool
	UrlSelectionOpenMode       bool // true if opening, false if yanking/copying
	UrlSelectedIndex           int
	UrlsInMessage              []string
	MessageLineOffsets         []int
	SearchMode                 bool
	SearchActive               bool
	SearchQuery                string
	SearchPopupMode            bool
	SearchPopupSelectedIndex   int
	SearchPopupScrollOffset    int
	SearchPopupResults         []SearchPopupItem
	HistoryMessages            map[string][]Message
	HistoryNextLink            map[string]string
	HistoryInitialized         map[string]bool
	ChatMessagesLoadedOnce     map[string]bool
	ChatCacheDirty             map[string]bool
	SearchStates               map[string]*ChatSearchState
	CachedMessages             map[string][]Message // per-chat message cache for instant restore on revisit
	CachedNextLink             map[string]string    // per-chat NextLink cache
	MainChatScrollOffset       int
	MainChatSnapToBottom       bool
	UserSearchPopupMode        bool
	UserSearchMode             bool
	UserSearchQuery            string
	UserSearchStatus           string
	UserSearchStatusUntil      *time.Time
	UserSearchLocalResults     []Chat
	UserSearchChannelResults   []channelEntry
	UserSearchDirectoryResults []User
	UserSearchSelectedIndex    int
	UserSearchLoading          bool
	AppStartTime               time.Time
	ChatIconTheme              string
	CustomChatIcons            map[string]string
	Features                   FeatureFlags

	// ── Presence popup (Feature: presence_enabled) ───────────────────────
	PresencePopupMode    bool
	PresenceChatMode     bool
	PresenceData         *UserPresence
	PresenceChatData     []PresenceEntry
	PresenceUserName     string // display name of the user whose presence is shown
	PresenceLoading      bool
	PresenceScrollOffset int

	// ── User Profile popup (Feature: user_profile_enabled) ───────────────
	UserProfilePopupMode bool
	UserProfileData      *UserProfile
	UserProfileLoading   bool

	// ── Attachment cursor in message view popup (Feature: file_preview_enabled) ──
	AttachmentSelectedIndex int
	AttachmentCursorMode    bool // true when navigating attachments inside the v popup

	// ── Teams channels data (Feature: teams_channels_enabled) ───────────
	TeamsData             []TeamWithChannels // cached joined teams + channels; populated at startup
	TeamsDataLoading      bool
	SelectedChannelTeamID string // teamID of the currently viewed channel ("" = chat mode)
	SelectedChannelID     string // channelID of the currently viewed channel ("" = chat mode)
	ChannelReplyToID      string // root message ID when replying to a channel thread ("" = new root post)
	ChannelMsgRefreshMin  int
	ExternalEditor        string // command/path for the external editor
	BrowserCommand        string // command to open URLs
	ImageViewer           string // command to open images (empty = use default file opener)
	YoutrackCommand       string // command to open YouTrack URLs
	GitlabCommand         string // command to open GitLab URLs

	// ── Mention Popup Autocomplete ───────────────────────────────────────
	MentionPopupMode          bool
	MentionSearch             string
	MentionSelectedIndex      int
	MentionScrollOffset       int
	MentionSuggestions        []ChatMember
	MentionStartIndex         int
	MentionCanceledStartIndex int
	TeamMembersCache          map[string][]ChatMember

	// ── Help popup ───────────────────────────────────────────────────────
	HelpPopupMode    bool
	HelpScrollOffset int

	// ── File Picker popup ────────────────────────────────────────────────
	FilePickerPopupMode bool

	// ── Composed images (pasted from clipboard) ──────────────────────────
	ComposedImages     []PastedImage
	ComposedFiles      []PendingFile
	SkipTextareaUpdate bool
}

// PendingFile represents a file selected from the file system.
type PendingFile struct {
	Name        string
	Path        string
	Data        []byte
	ContentType string
}

// ChatSearchState holds the search-specific query and viewport navigation state for a chat.
type ChatSearchState struct {
	Query           string
	Results         []SearchPopupItem
	SelectedIndex   int
	ScrollOffset    int
	ExpandedIndices map[int]bool
	Status          string
}

// SearchPopupItem represents a message displayed inside the search popup (with context flag).
type SearchPopupItem struct {
	Message      Message
	IsMatch      bool
	HistoryIndex int
}

// NewApp creates an App with sensible initial defaults.
func NewApp() *App {
	return &App{
		Status:                    "Loading...",
		SnapToBottom:              true,
		NotificationMode:          NotificationNone,
		NotificationShowPreview:   false,
		NotificationPreviewLen:    50,
		HistoryMessages:           make(map[string][]Message),
		HistoryNextLink:           make(map[string]string),
		HistoryInitialized:        make(map[string]bool),
		ChatMessagesLoadedOnce:    make(map[string]bool),
		ChatCacheDirty:            make(map[string]bool),
		SearchStates:              make(map[string]*ChatSearchState),
		CachedMessages:            make(map[string][]Message),
		CachedNextLink:            make(map[string]string),
		TeamMembersCache:          make(map[string][]ChatMember),
		ChatIconTheme:             "unicode",
		CustomChatIcons:           make(map[string]string),
		AppStartTime:              time.Now(),
		MentionCanceledStartIndex: -1,
	}
}

// SetChats replaces the chat list and updates the status line.
func (a *App) SetChats(chats []Chat) {
	a.Chats = chats
	a.SetStatus(fmt.Sprintf("Loaded %d chats", len(chats)), 5*time.Second)
}

// SetCurrentUser records the detected current user's display name.
func (a *App) SetCurrentUser(name string) {
	a.CurrentUserName = &name
}

// SetMessages updates the current message list, merging new messages with existing ones.
func (a *App) SetMessages(messages []Message, nextLink string) {
	if len(a.Messages) == 0 {
		a.Messages = messages
		a.NextLink = nextLink
		a.LoadingMessages = false
		return
	}

	// Create a map of existing messages by ID.
	m := make(map[string]Message)
	for _, msg := range a.Messages {
		m[msg.ID] = msg
	}
	// Overwrite/add with fresh messages.
	for _, msg := range messages {
		m[msg.ID] = msg
	}

	result := make([]Message, 0, len(m))
	for _, msg := range m {
		result = append(result, msg)
	}

	// Sort newest first.
	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedDateTime > result[j].CreatedDateTime
	})

	// Maintain selection by ID if in message selection mode.
	if a.MessageSelectionMode && len(a.Messages) > 0 && a.MessageSelectedIndex < len(a.Messages) {
		selectedID := a.Messages[a.MessageSelectedIndex].ID
		a.Messages = result // set here so we can find index in new list
		for i, m := range result {
			if m.ID == selectedID {
				a.MessageSelectedIndex = i
				goto done
			}
		}
	}

	a.Messages = result

done:
	// Clamp index if message was deleted or we're out of bounds.
	if a.MessageSelectedIndex >= len(a.Messages) {
		if len(a.Messages) > 0 {
			a.MessageSelectedIndex = len(a.Messages) - 1
		} else {
			a.MessageSelectedIndex = 0
		}
	}

	// Only update NextLink if it's currently empty (e.g. first successful load).
	// If we already have a NextLink, it points to the older history we've reached.
	if a.NextLink == "" {
		a.NextLink = nextLink
	}
	a.LoadingMessages = false
}

// AppendOlderMessages adds older messages to the end of the current list.
func (a *App) AppendOlderMessages(messages []Message, nextLink string) {
	a.Messages = append(a.Messages, messages...)
	a.NextLink = nextLink
	a.LoadingMessages = false
}

// SetLoadingMessages toggles the loading indicator.
func (a *App) SetLoadingMessages(loading bool) {
	a.LoadingMessages = loading
}

// SetSearchLoadingMessages toggles the search loading indicator.
func (a *App) SetSearchLoadingMessages(loading bool) {
	a.SearchLoadingMessages = loading
}

// SetSearchStatus sets the search status text, optionally clearing it after duration.
func (a *App) SetSearchStatus(msg string, duration time.Duration) {
	a.SearchStatus = msg
	if duration > 0 {
		t := time.Now().Add(duration)
		a.SearchStatusUntil = &t
	} else {
		a.SearchStatusUntil = nil
	}
}

// SetMessagePopupStatus sets the message view popup status text, optionally clearing it after duration.
func (a *App) SetMessagePopupStatus(msg string, duration time.Duration) {
	a.MessagePopupStatus = msg
	if duration > 0 {
		t := time.Now().Add(duration)
		a.MessagePopupStatusUntil = &t
	} else {
		a.MessagePopupStatusUntil = nil
	}
}

// GetSelectedChat returns the currently highlighted chat, or nil.
func (a *App) GetSelectedChat() *Chat {
	if len(a.Chats) == 0 || a.SelectedIndex < 0 || a.SelectedIndex >= len(a.Chats) {
		return nil
	}
	return &a.Chats[a.SelectedIndex]
}

// NextChat moves the selection one step down, wrapping around.
func (a *App) NextChat() {
	if len(a.Chats) == 0 {
		return
	}
	a.SelectedIndex = (a.SelectedIndex + 1) % len(a.Chats)
}

// PreviousChat moves the selection one step up, wrapping around.
func (a *App) PreviousChat() {
	if len(a.Chats) == 0 {
		return
	}
	a.SelectedIndex = (a.SelectedIndex - 1 + len(a.Chats)) % len(a.Chats)
}

// ToggleNotificationMode cycles None → Console → System → Both → None.
func (a *App) ToggleNotificationMode() {
	a.NotificationMode = (a.NotificationMode + 1) % 4
}

// TriggerVisualBell sets VisualBellUntil to 200 ms from now.
func (a *App) TriggerVisualBell() {
	t := time.Now().Add(200 * time.Millisecond)
	a.VisualBellUntil = &t
}

// VisualBellActive reports whether the visual bell should be showing.
func (a *App) VisualBellActive() bool {
	return a.VisualBellUntil != nil && time.Now().Before(*a.VisualBellUntil)
}

// SetStatus sets the status line, optionally clearing it after duration.
func (a *App) SetStatus(msg string, duration time.Duration) {
	a.Status = msg
	if duration > 0 {
		t := time.Now().Add(duration)
		a.StatusUntil = &t
	} else {
		a.StatusUntil = nil
	}
}
