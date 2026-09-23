package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/joho/godotenv"
)

// ---------------------------------------------------------------------------
// Favourites persistence
// ---------------------------------------------------------------------------

// LoadFavourites reads the list of favourite chat IDs from favourites.json.
// Returns an empty map if the file does not exist or cannot be parsed.
func LoadFavourites() map[string]bool {
	dir, err := GetAppDir()
	if err != nil {
		return make(map[string]bool)
	}
	data, err := os.ReadFile(filepath.Join(dir, "favourites.json"))
	if err != nil {
		return make(map[string]bool)
	}
	var ids []string
	if err := json.Unmarshal(data, &ids); err != nil {
		return make(map[string]bool)
	}
	m := make(map[string]bool, len(ids))
	for _, id := range ids {
		m[id] = true
	}
	return m
}

// SaveFavourites writes the current favourite chat IDs to favourites.json.
func SaveFavourites(favs map[string]bool) error {
	dir, err := GetAppDir()
	if err != nil {
		return err
	}
	ids := make([]string, 0, len(favs))
	for id := range favs {
		ids = append(ids, id)
	}
	// Sort for deterministic output.
	sort.Strings(ids)
	data, err := json.MarshalIndent(ids, "", "  ")
	if err != nil {
		return fmt.Errorf("could not marshal favourites: %w", err)
	}
	return os.WriteFile(filepath.Join(dir, "favourites.json"), data, 0o600)
}

// LoadUnhiddenChannels reads the list of unhidden channel IDs from unhidden_channels.json.
// Returns an empty map if the file does not exist or cannot be parsed.
func LoadUnhiddenChannels() map[string]bool {
	dir, err := GetAppDir()
	if err != nil {
		return make(map[string]bool)
	}
	data, err := os.ReadFile(filepath.Join(dir, "unhidden_channels.json"))
	if err != nil {
		return make(map[string]bool)
	}
	var ids []string
	if err := json.Unmarshal(data, &ids); err != nil {
		return make(map[string]bool)
	}
	m := make(map[string]bool, len(ids))
	for _, id := range ids {
		m[id] = true
	}
	return m
}

// SaveUnhiddenChannels writes the current unhidden channel IDs to unhidden_channels.json.
func SaveUnhiddenChannels(unhidden map[string]bool) error {
	dir, err := GetAppDir()
	if err != nil {
		return err
	}
	ids := make([]string, 0, len(unhidden))
	for id := range unhidden {
		ids = append(ids, id)
	}
	// Sort for deterministic output.
	sort.Strings(ids)
	data, err := json.MarshalIndent(ids, "", "  ")
	if err != nil {
		return fmt.Errorf("could not marshal unhidden channels: %w", err)
	}
	return os.WriteFile(filepath.Join(dir, "unhidden_channels.json"), data, 0o600)
}

// FilepickerSettings holds the filepicker sorting and directory persistence.
type FilepickerSettings struct {
	SortBy           string `json:"sort_by"`
	SortOrder        string `json:"sort_order"`
	CurrentDirectory string `json:"current_directory,omitempty"`
}

// LoadFilepickerSettings reads the filepicker sorting settings from filepicker_settings.json.
// Returns default values if the file does not exist or cannot be parsed.
func LoadFilepickerSettings() (string, string, string) {
	dir, err := GetAppDir()
	if err != nil {
		return "Name", "asc", ""
	}
	data, err := os.ReadFile(filepath.Join(dir, "filepicker_settings.json"))
	if err != nil {
		return "Name", "asc", ""
	}
	var settings FilepickerSettings
	if err := json.Unmarshal(data, &settings); err != nil {
		return "Name", "asc", ""
	}
	if settings.SortBy == "" {
		settings.SortBy = "Name"
	}
	if settings.SortOrder == "" {
		settings.SortOrder = "asc"
	}
	return settings.SortBy, settings.SortOrder, settings.CurrentDirectory
}

// SaveFilepickerSettings writes the current filepicker settings to filepicker_settings.json.
func SaveFilepickerSettings(sortBy string, sortOrder string, currentDirectory string) error {
	dir, err := GetAppDir()
	if err != nil {
		return err
	}
	settings := FilepickerSettings{
		SortBy:           sortBy,
		SortOrder:        sortOrder,
		CurrentDirectory: currentDirectory,
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("could not marshal filepicker settings: %w", err)
	}
	return os.WriteFile(filepath.Join(dir, "filepicker_settings.json"), data, 0o600)
}

const appDirName = "teams-tui-go"

// defaultClientID is the Microsoft Teams client ID fallback.
const defaultClientID = "d3590ed6-52b3-4102-aeff-aad2292ab01c"

// Config holds persistent application settings.
type Config struct {
	ClientID                *string           `json:"client_id,omitempty"`
	NotificationMode        *NotificationMode `json:"notification_mode,omitempty"`
	NotificationShowPreview *bool             `json:"notification_show_preview,omitempty"`
	NotificationPreviewLen  *int              `json:"notification_preview_len,omitempty"`
	MessageLimit            *int              `json:"message_limit,omitempty"`
	SearchContextLimit      *int              `json:"search_context_limit,omitempty"`
	ChatLimit               *int              `json:"chat_limit,omitempty"`
	ChatIconTheme           *string           `json:"chat_icon_theme,omitempty"`
	CustomChatIcons         map[string]string `json:"custom_chat_icons,omitempty"`

	// Optional feature flags — each defaults to false (disabled).
	// When enabled, the corresponding Graph API permission must be granted
	// in the Azure app registration and the cached token refreshed.
	FilePreviewEnabled     *bool   `json:"file_preview_enabled,omitempty"`     // requires Files.Read
	FilePreviewInTerminal  *bool   `json:"file_preview_in_terminal,omitempty"` // show image in terminal if file_preview_enabled is true
	FileUploadEnabled      *bool   `json:"file_upload_enabled,omitempty"`      // requires Files.ReadWrite
	PresenceEnabled        *bool   `json:"presence_enabled,omitempty"`         // requires Presence.Read.All
	UserProfileEnabled     *bool   `json:"user_profile_enabled,omitempty"`     // requires User.ReadBasic.All
	UserProfileExtended    *bool   `json:"user_profile_extended,omitempty"`    // requires User.Read.All (admin consent)
	TeamsChannelsEnabled   *bool   `json:"teams_channels_enabled,omitempty"`   // requires Team.ReadBasic.All + Channel.ReadBasic.All + ChannelMessage.Read.All + ChannelMessage.Send + ChannelMessage.ReadWrite
	ChannelMentionsEnabled *bool   `json:"channel_mentions_enabled,omitempty"` // requires TeamMember.Read.All to load members for autocomplete in channels
	ChannelMsgRefreshMin   *int    `json:"channel_msg_refresh_min,omitempty"`
	SqliteEnabled          *bool   `json:"sqlite_enabled,omitempty"`
	ExternalEditor         *string `json:"external_editor,omitempty"`
	BrowserCommand         *string `json:"browser_command,omitempty"`
	ImageViewer            *string `json:"image_viewer,omitempty"`
	YoutrackCommand        *string `json:"youtrack_command,omitempty"`
	GitlabCommand          *string `json:"gitlab_command,omitempty"`
	ExportDirectory        *string `json:"export_directory,omitempty"`

	// Keybindings overrides default keys. Omitted actions keep their defaults.
	// See README for the action ids. Not written by InitConfig.
	Keybindings json.RawMessage `json:"keybindings,omitempty"`
}

// GetAppDir returns ~/.config/teams-tui-go/, creating it if necessary.
func GetAppDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("could not determine config dir: %w", err)
	}
	dir := filepath.Join(base, appDirName)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("could not create config dir: %w", err)
	}
	return dir, nil
}

// GetCacheDir returns ~/.cache/teams-tui-go/, creating it if necessary.
func GetCacheDir() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("could not determine cache dir: %w", err)
	}
	dir := filepath.Join(base, appDirName)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("could not create cache dir: %w", err)
	}
	return dir, nil
}

// LoadConfig reads config.json from the app dir.
// Returns nil if the file does not exist or cannot be parsed.
func LoadConfig() *Config {
	dir, err := GetAppDir()
	if err != nil {
		return nil
	}
	data, err := os.ReadFile(filepath.Join(dir, "config.json"))
	if err != nil {
		return nil
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil
	}
	return &cfg
}

// InitConfig loads the configuration, populates any missing options with their
// default values, and saves the updated configuration back to config.json.
func InitConfig() {
	dir, err := GetAppDir()
	if err != nil {
		return
	}
	path := filepath.Join(dir, "config.json")

	var cfg Config
	exists := true
	data, err := os.ReadFile(path)
	if err != nil {
		exists = false
	} else {
		if err := json.Unmarshal(data, &cfg); err != nil {
			exists = false
		}
	}

	modified := false

	if cfg.ClientID == nil {
		id := defaultClientID
		cfg.ClientID = &id
		modified = true
	}
	if cfg.NotificationMode == nil {
		mode := NotificationNone
		cfg.NotificationMode = &mode
		modified = true
	}
	if cfg.NotificationShowPreview == nil {
		show := false
		cfg.NotificationShowPreview = &show
		modified = true
	}
	if cfg.NotificationPreviewLen == nil {
		length := 50
		cfg.NotificationPreviewLen = &length
		modified = true
	}
	if cfg.MessageLimit == nil {
		limit := 50
		cfg.MessageLimit = &limit
		modified = true
	}
	if cfg.SearchContextLimit == nil {
		limit := 3
		cfg.SearchContextLimit = &limit
		modified = true
	}
	if cfg.ChatLimit == nil {
		limit := 50
		cfg.ChatLimit = &limit
		modified = true
	}
	if cfg.ChatIconTheme == nil {
		theme := "unicode"
		cfg.ChatIconTheme = &theme
		modified = true
	}
	// Feature flags default to false (disabled) — written so users can see them in config.json.
	if cfg.FilePreviewEnabled == nil {
		v := false
		cfg.FilePreviewEnabled = &v
		modified = true
	}
	if cfg.FilePreviewInTerminal == nil {
		v := false
		cfg.FilePreviewInTerminal = &v
		modified = true
	}
	if cfg.FileUploadEnabled == nil {
		v := false
		cfg.FileUploadEnabled = &v
		modified = true
	}
	if cfg.PresenceEnabled == nil {
		v := false
		cfg.PresenceEnabled = &v
		modified = true
	}
	if cfg.UserProfileEnabled == nil {
		v := false
		cfg.UserProfileEnabled = &v
		modified = true
	}
	if cfg.UserProfileExtended == nil {
		v := false
		cfg.UserProfileExtended = &v
		modified = true
	}
	if cfg.TeamsChannelsEnabled == nil {
		v := false
		cfg.TeamsChannelsEnabled = &v
		modified = true
	}
	if cfg.ChannelMentionsEnabled == nil {
		v := false
		cfg.ChannelMentionsEnabled = &v
		modified = true
	}
	if cfg.ChannelMsgRefreshMin == nil {
		val := 2
		cfg.ChannelMsgRefreshMin = &val
		modified = true
	}
	if cfg.SqliteEnabled == nil {
		v := false
		cfg.SqliteEnabled = &v
		modified = true
	}
	if cfg.ExternalEditor == nil {
		editor := ""
		cfg.ExternalEditor = &editor
		modified = true
	}
	if cfg.BrowserCommand == nil {
		cmd := "xdg-open"
		cfg.BrowserCommand = &cmd
		modified = true
	}
	if cfg.ImageViewer == nil {
		v := ""
		cfg.ImageViewer = &v
		modified = true
	}
	if cfg.ExportDirectory == nil {
		dir := "~/Downloads"
		cfg.ExportDirectory = &dir
		modified = true
	}

	if !exists || modified {
		_ = SaveConfig(&cfg)
	}
}

// SaveConfig writes config.json (pretty-printed) to the app dir.
func SaveConfig(cfg *Config) error {
	dir, err := GetAppDir()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("could not marshal config: %w", err)
	}
	return os.WriteFile(filepath.Join(dir, "config.json"), data, 0o600)
}

// ResolveClientID returns the client ID using the precedence:
//  1. CLIENT_ID environment variable (loads .env first)
//  2. config.json → client_id
//  3. Built-in default
func ResolveClientID() string {
	// Load .env file if present; ignore errors (file may not exist).
	_ = godotenv.Load()

	if id := os.Getenv("CLIENT_ID"); id != "" {
		return id
	}
	cfg := LoadConfig()
	if cfg != nil && cfg.ClientID != nil && *cfg.ClientID != "" {
		return *cfg.ClientID
	}
	return defaultClientID
}

// ResolveMessageLimit returns the number of messages to fetch, using precedence:
//  1. config.json -> message_limit
//  2. Default (50)
//     Note: Capped at 200 to prevent excessive API requests.
func ResolveMessageLimit() int {
	cfg := LoadConfig()
	if cfg != nil && cfg.MessageLimit != nil && *cfg.MessageLimit > 0 {
		limit := *cfg.MessageLimit
		if limit > 200 {
			return 200
		}
		return limit
	}
	return 50
}

// ResolveSearchContextLimit returns the number of context messages to show before/after matching message:
//  1. config.json -> search_context_limit
//  2. Default (3)
func ResolveSearchContextLimit() int {
	cfg := LoadConfig()
	if cfg != nil && cfg.SearchContextLimit != nil && *cfg.SearchContextLimit >= 0 {
		return *cfg.SearchContextLimit
	}
	return 3
}

// ResolveChatLimit returns the number of chats to fetch, using precedence:
//  1. config.json -> chat_limit
//  2. Default (50)
//     Note: Capped at 100 to prevent API throttling during concurrent member fetching.
func ResolveChatLimit() int {
	cfg := LoadConfig()
	if cfg != nil && cfg.ChatLimit != nil && *cfg.ChatLimit > 0 {
		limit := *cfg.ChatLimit
		if limit > 100 {
			return 100
		}
		return limit
	}
	return 50
}

// ResolveChannelMsgRefreshMin returns the channel messages refresh interval in minutes.
// Precedence:
//  1. config.json -> channel_msg_refresh_min
//  2. Default (2)
func ResolveChannelMsgRefreshMin() int {
	cfg := LoadConfig()
	if cfg != nil && cfg.ChannelMsgRefreshMin != nil && *cfg.ChannelMsgRefreshMin > 0 {
		return *cfg.ChannelMsgRefreshMin
	}
	return 2
}

// ---------------------------------------------------------------------------
// Feature flag resolvers
// ---------------------------------------------------------------------------

// ResolveFeatureFilePreview returns true when file preview/download is enabled.
func ResolveFeatureFilePreview() bool {
	cfg := LoadConfig()
	return cfg != nil && cfg.FilePreviewEnabled != nil && *cfg.FilePreviewEnabled
}

// ResolveFeatureFilePreviewInTerminal returns true when file preview in terminal is enabled.
func ResolveFeatureFilePreviewInTerminal() bool {
	cfg := LoadConfig()
	return cfg != nil && cfg.FilePreviewInTerminal != nil && *cfg.FilePreviewInTerminal
}

// ResolveFeatureFileUpload returns true when file upload to OneDrive is enabled.
func ResolveFeatureFileUpload() bool {
	cfg := LoadConfig()
	return cfg != nil && cfg.FileUploadEnabled != nil && *cfg.FileUploadEnabled
}

// ResolveFeaturePresence returns true when user presence status is enabled.
func ResolveFeaturePresence() bool {
	cfg := LoadConfig()
	return cfg != nil && cfg.PresenceEnabled != nil && *cfg.PresenceEnabled
}

// ResolveFeatureUserProfile returns true when user profile info is enabled.
func ResolveFeatureUserProfile() bool {
	cfg := LoadConfig()
	return cfg != nil && cfg.UserProfileEnabled != nil && *cfg.UserProfileEnabled
}

// ResolveFeatureUserProfileExtended returns true when extended profile (job title etc.) is enabled.
// Requires User.Read.All which needs admin consent.
func ResolveFeatureUserProfileExtended() bool {
	cfg := LoadConfig()
	return cfg != nil && cfg.UserProfileExtended != nil && *cfg.UserProfileExtended
}

// ResolveFeatureTeamsChannels returns true when Teams channels browsing is enabled.
func ResolveFeatureTeamsChannels() bool {
	cfg := LoadConfig()
	return cfg != nil && cfg.TeamsChannelsEnabled != nil && *cfg.TeamsChannelsEnabled
}

// ResolveFeatureChannelMentions returns true when channel mentions (and member autocomplete) are enabled.
func ResolveFeatureChannelMentions() bool {
	cfg := LoadConfig()
	return cfg != nil && cfg.ChannelMentionsEnabled != nil && *cfg.ChannelMentionsEnabled
}

// ResolveFeatureSqlite returns true when SQLite caching is enabled.
func ResolveFeatureSqlite() bool {
	cfg := LoadConfig()
	return cfg != nil && cfg.SqliteEnabled != nil && *cfg.SqliteEnabled
}

// BuildScopes constructs the OAuth2 scope string from config feature flags.
// Basic scopes are always included; additional scopes are appended for enabled features.
func BuildScopes() string {
	base := "User.Read Chat.ReadWrite offline_access"
	if ResolveFeatureFileUpload() {
		base += " Files.ReadWrite"
	} else if ResolveFeatureFilePreview() {
		base += " Files.Read"
	}
	if ResolveFeaturePresence() {
		base += " Presence.Read.All"
	}
	if ResolveFeatureUserProfile() {
		if ResolveFeatureUserProfileExtended() {
			base += " User.Read.All"
		} else {
			base += " User.ReadBasic.All"
		}
	}
	if ResolveFeatureTeamsChannels() {
		base += " Team.ReadBasic.All Channel.ReadBasic.All ChannelMessage.Read.All ChannelMessage.Send ChannelMessage.ReadWrite"
	}
	if ResolveFeatureChannelMentions() {
		base += " TeamMember.Read.All"
	}
	return base
}

// ResolveExternalEditor returns the command for the external editor, using precedence:
//  1. config.json -> external_editor
//  2. EDITOR environment variable
//  3. VISUAL environment variable
//  4. Default ("vim")
func ResolveExternalEditor() string {
	cfg := LoadConfig()
	if cfg != nil && cfg.ExternalEditor != nil && *cfg.ExternalEditor != "" {
		return *cfg.ExternalEditor
	}
	if ed := os.Getenv("EDITOR"); ed != "" {
		return ed
	}
	if vis := os.Getenv("VISUAL"); vis != "" {
		return vis
	}
	return "vim"
}

// ResolveExportDirectory returns the Markdown export directory:
//  1. config.json -> export_directory
//  2. Default ("~/Downloads")
func ResolveExportDirectory() string {
	cfg := LoadConfig()
	if cfg != nil && cfg.ExportDirectory != nil && strings.TrimSpace(*cfg.ExportDirectory) != "" {
		return strings.TrimSpace(*cfg.ExportDirectory)
	}
	return "~/Downloads"
}

// ResolveBrowserCommand returns the browser command, using precedence:
//  1. config.json -> browser_command
//  2. Default ("xdg-open")
func ResolveBrowserCommand() string {
	cfg := LoadConfig()
	if cfg != nil && cfg.BrowserCommand != nil && *cfg.BrowserCommand != "" {
		return *cfg.BrowserCommand
	}
	return "xdg-open"
}

// ResolveImageViewer returns the image viewer command, using precedence:
//  1. config.json -> image_viewer
//  2. Default ("") — empty means fall back to the default file opener
func ResolveImageViewer() string {
	cfg := LoadConfig()
	if cfg != nil && cfg.ImageViewer != nil {
		return *cfg.ImageViewer
	}
	return ""
}

// ResolveYoutrackCommand returns the youtrack command, using precedence:
//  1. config.json -> youtrack_command
//  2. Default ("") - meaning fall back to browser command if not set
func ResolveYoutrackCommand() string {
	cfg := LoadConfig()
	if cfg != nil && cfg.YoutrackCommand != nil && *cfg.YoutrackCommand != "" {
		return *cfg.YoutrackCommand
	}
	return ""
}

// ResolveGitlabCommand returns the gitlab command, using precedence:
//  1. config.json -> gitlab_command
//  2. Default ("") - meaning fall back to browser command if not set
func ResolveGitlabCommand() string {
	cfg := LoadConfig()
	if cfg != nil && cfg.GitlabCommand != nil && *cfg.GitlabCommand != "" {
		return *cfg.GitlabCommand
	}
	return ""
}
