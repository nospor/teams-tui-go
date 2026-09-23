package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestInitConfig(t *testing.T) {
	// Set XDG_CONFIG_HOME to a temporary directory to avoid writing to the user's actual config.
	tmpDir, err := os.MkdirTemp("", "teams-tui-config-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	oldXdg := os.Getenv("XDG_CONFIG_HOME")
	defer os.Setenv("XDG_CONFIG_HOME", oldXdg)
	os.Setenv("XDG_CONFIG_HOME", tmpDir)

	// Case 1: Config does not exist yet.
	InitConfig()

	appDir, err := GetAppDir()
	if err != nil {
		t.Fatalf("GetAppDir failed: %v", err)
	}
	configPath := filepath.Join(appDir, "config.json")

	// Verify config file was created.
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("config file was not created: %v", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("failed to parse generated config: %v", err)
	}

	// Verify defaults.
	if cfg.ClientID == nil || *cfg.ClientID != defaultClientID {
		t.Errorf("expected client ID %q, got %v", defaultClientID, cfg.ClientID)
	}
	if cfg.NotificationMode == nil || *cfg.NotificationMode != NotificationNone {
		t.Errorf("expected notification mode %v, got %v", NotificationNone, cfg.NotificationMode)
	}
	if cfg.NotificationShowPreview == nil || *cfg.NotificationShowPreview != false {
		t.Errorf("expected notification show preview false, got %v", cfg.NotificationShowPreview)
	}
	if cfg.NotificationPreviewLen == nil || *cfg.NotificationPreviewLen != 50 {
		t.Errorf("expected notification preview len 50, got %v", cfg.NotificationPreviewLen)
	}
	if cfg.MessageLimit == nil || *cfg.MessageLimit != 50 {
		t.Errorf("expected message limit 50, got %v", cfg.MessageLimit)
	}
	if cfg.SearchContextLimit == nil || *cfg.SearchContextLimit != 3 {
		t.Errorf("expected search context limit 3, got %v", cfg.SearchContextLimit)
	}
	if cfg.ChatLimit == nil || *cfg.ChatLimit != 50 {
		t.Errorf("expected chat limit 50, got %v", cfg.ChatLimit)
	}
	if cfg.ChannelMsgRefreshMin == nil || *cfg.ChannelMsgRefreshMin != 2 {
		t.Errorf("expected channel message refresh min 2, got %v", cfg.ChannelMsgRefreshMin)
	}
	if cfg.ExternalEditor == nil || *cfg.ExternalEditor != "" {
		t.Errorf("expected external editor to be empty string default, got %v", cfg.ExternalEditor)
	}
	if cfg.BrowserCommand == nil || *cfg.BrowserCommand != "xdg-open" {
		t.Errorf("expected browser command 'xdg-open', got %v", cfg.BrowserCommand)
	}
	if cfg.YoutrackCommand != nil {
		t.Errorf("expected youtrack command to be nil, got %v", cfg.YoutrackCommand)
	}
	if cfg.GitlabCommand != nil {
		t.Errorf("expected gitlab command to be nil, got %v", cfg.GitlabCommand)
	}

	// Case 2: Config exists but is missing some options (e.g. partial).
	// We'll write a custom config with only ClientID and MessageLimit set, and others missing/nil.
	customClientID := "custom-id-123"
	customMsgLimit := 100
	partialCfg := Config{
		ClientID:     &customClientID,
		MessageLimit: &customMsgLimit,
	}
	partialData, err := json.Marshal(partialCfg)
	if err != nil {
		t.Fatalf("failed to marshal partial config: %v", err)
	}
	if err := os.WriteFile(configPath, partialData, 0o600); err != nil {
		t.Fatalf("failed to write partial config: %v", err)
	}

	// Run InitConfig to fill in the missing ones.
	InitConfig()

	// Re-read and check.
	data, err = os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read updated config: %v", err)
	}

	var updatedCfg Config
	if err := json.Unmarshal(data, &updatedCfg); err != nil {
		t.Fatalf("failed to parse updated config: %v", err)
	}

	// Custom values must be preserved.
	if updatedCfg.ClientID == nil || *updatedCfg.ClientID != customClientID {
		t.Errorf("expected preserved client ID %q, got %v", customClientID, updatedCfg.ClientID)
	}
	if updatedCfg.MessageLimit == nil || *updatedCfg.MessageLimit != customMsgLimit {
		t.Errorf("expected preserved message limit %d, got %v", customMsgLimit, updatedCfg.MessageLimit)
	}

	// Missing values must be populated.
	if updatedCfg.NotificationMode == nil || *updatedCfg.NotificationMode != NotificationNone {
		t.Errorf("expected default notification mode, got %v", updatedCfg.NotificationMode)
	}
	if updatedCfg.SearchContextLimit == nil || *updatedCfg.SearchContextLimit != 3 {
		t.Errorf("expected default search context limit 3, got %v", updatedCfg.SearchContextLimit)
	}
	if updatedCfg.ChatLimit == nil || *updatedCfg.ChatLimit != 50 {
		t.Errorf("expected default chat limit 50, got %v", updatedCfg.ChatLimit)
	}
	if updatedCfg.ChannelMsgRefreshMin == nil || *updatedCfg.ChannelMsgRefreshMin != 2 {
		t.Errorf("expected default channel message refresh min 2, got %v", updatedCfg.ChannelMsgRefreshMin)
	}
	if updatedCfg.ExternalEditor == nil || *updatedCfg.ExternalEditor != "" {
		t.Errorf("expected default external editor to be empty string, got %v", updatedCfg.ExternalEditor)
	}
	if updatedCfg.BrowserCommand == nil || *updatedCfg.BrowserCommand != "xdg-open" {
		t.Errorf("expected default browser command 'xdg-open', got %v", updatedCfg.BrowserCommand)
	}
	if updatedCfg.ExportDirectory == nil || *updatedCfg.ExportDirectory != "~/Downloads" {
		t.Errorf("expected default export directory ~/Downloads, got %v", updatedCfg.ExportDirectory)
	}
	if updatedCfg.YoutrackCommand != nil {
		t.Errorf("expected default youtrack command to be nil, got %v", updatedCfg.YoutrackCommand)
	}
	if updatedCfg.GitlabCommand != nil {
		t.Errorf("expected default gitlab command to be nil, got %v", updatedCfg.GitlabCommand)
	}
}

func TestResolveMessageLimit(t *testing.T) {
	// Set XDG_CONFIG_HOME to a temporary directory.
	tmpDir, err := os.MkdirTemp("", "teams-tui-config-test-limit")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	oldXdg := os.Getenv("XDG_CONFIG_HOME")
	defer os.Setenv("XDG_CONFIG_HOME", oldXdg)
	os.Setenv("XDG_CONFIG_HOME", tmpDir)

	// Write custom config with message_limit = 52.
	appDir, err := GetAppDir()
	if err != nil {
		t.Fatalf("GetAppDir failed: %v", err)
	}
	configPath := filepath.Join(appDir, "config.json")

	limit := 52
	cfg := Config{
		MessageLimit: &limit,
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	if err := os.WriteFile(configPath, data, 0o600); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	// Resolve the message limit. It should return 52 now.
	resolved := ResolveMessageLimit()
	if resolved != 52 {
		t.Errorf("expected message limit to be 52, got %d", resolved)
	}

	// Test capping limit at 200.
	limit = 300
	cfg = Config{
		MessageLimit: &limit,
	}
	data, err = json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	if err := os.WriteFile(configPath, data, 0o600); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	resolved = ResolveMessageLimit()
	if resolved != 200 {
		t.Errorf("expected message limit to be capped at 200, got %d", resolved)
	}
}

func TestResolveChatLimit(t *testing.T) {
	// Set XDG_CONFIG_HOME to a temporary directory.
	tmpDir, err := os.MkdirTemp("", "teams-tui-config-test-chat-limit")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	oldXdg := os.Getenv("XDG_CONFIG_HOME")
	defer os.Setenv("XDG_CONFIG_HOME", oldXdg)
	os.Setenv("XDG_CONFIG_HOME", tmpDir)

	// Write custom config with chat_limit = 72.
	appDir, err := GetAppDir()
	if err != nil {
		t.Fatalf("GetAppDir failed: %v", err)
	}
	configPath := filepath.Join(appDir, "config.json")

	limit := 72
	cfg := Config{
		ChatLimit: &limit,
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	if err := os.WriteFile(configPath, data, 0o600); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	// Resolve the chat limit. It should return 72.
	resolved := ResolveChatLimit()
	if resolved != 72 {
		t.Errorf("expected chat limit to be 72, got %d", resolved)
	}

	// Test capping limit at 100.
	limit = 150
	cfg = Config{
		ChatLimit: &limit,
	}
	data, err = json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	if err := os.WriteFile(configPath, data, 0o600); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	resolved = ResolveChatLimit()
	if resolved != 100 {
		t.Errorf("expected chat limit to be capped at 100, got %d", resolved)
	}
}
func TestBuildScopes(t *testing.T) {
	// Set XDG_CONFIG_HOME to a temporary directory.
	tmpDir, err := os.MkdirTemp("", "teams-tui-scopes-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	oldXdg := os.Getenv("XDG_CONFIG_HOME")
	defer os.Setenv("XDG_CONFIG_HOME", oldXdg)
	os.Setenv("XDG_CONFIG_HOME", tmpDir)

	// Case 1: No features enabled — only basic scopes.
	InitConfig()
	scopes := BuildScopes()
	for _, required := range []string{"User.Read", "Chat.ReadWrite", "offline_access"} {
		found := false
		for _, s := range splitScopes(scopes) {
			if s == required {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("basic scope %q missing from %q", required, scopes)
		}
	}
	for _, unexpected := range []string{"Presence.Read.All", "Files.Read", "User.ReadBasic.All", "User.Read.All", "Team.ReadBasic.All", "TeamMember.Read.All"} {
		for _, s := range splitScopes(scopes) {
			if s == unexpected {
				t.Errorf("unexpected scope %q present when feature is disabled", unexpected)
			}
		}
	}

	// Case 2: Enable presence — Presence.Read.All should appear.
	appDir, _ := GetAppDir()
	cfgPath := filepath.Join(appDir, "config.json")
	cfg := LoadConfig()
	if cfg == nil {
		cfg = &Config{}
	}
	presenceOn := true
	cfg.PresenceEnabled = &presenceOn
	if err := SaveConfig(cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	scopes2 := BuildScopes()
	found := false
	for _, s := range splitScopes(scopes2) {
		if s == "Presence.Read.All" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Presence.Read.All missing when presence_enabled=true, scopes=%q", scopes2)
	}

	// Case 3: Enable channel mentions — TeamMember.Read.All should appear.
	cfg = LoadConfig()
	channelMentionsOn := true
	cfg.ChannelMentionsEnabled = &channelMentionsOn
	if err := SaveConfig(cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	scopes3 := BuildScopes()
	foundMentions := false
	for _, s := range splitScopes(scopes3) {
		if s == "TeamMember.Read.All" {
			foundMentions = true
			break
		}
	}
	if !foundMentions {
		t.Errorf("TeamMember.Read.All missing when channel_mentions_enabled=true, scopes=%q", scopes3)
	}
	_ = cfgPath
}

// splitScopes splits a space-separated scope string into individual scopes.
func splitScopes(s string) []string {
	var result []string
	current := ""
	for _, c := range s {
		if c == ' ' {
			if current != "" {
				result = append(result, current)
				current = ""
			}
		} else {
			current += string(c)
		}
	}
	if current != "" {
		result = append(result, current)
	}
	return result
}

func TestResolveFeatureFilePreviewInTerminal(t *testing.T) {
	// Set XDG_CONFIG_HOME to a temporary directory.
	tmpDir, err := os.MkdirTemp("", "teams-tui-preview-terminal-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	oldXdg := os.Getenv("XDG_CONFIG_HOME")
	defer os.Setenv("XDG_CONFIG_HOME", oldXdg)
	os.Setenv("XDG_CONFIG_HOME", tmpDir)

	// Write custom config with file_preview_in_terminal = true.
	appDir, err := GetAppDir()
	if err != nil {
		t.Fatalf("GetAppDir failed: %v", err)
	}
	configPath := filepath.Join(appDir, "config.json")

	val := true
	cfg := Config{
		FilePreviewInTerminal: &val,
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	if err := os.WriteFile(configPath, data, 0o600); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	resolved := ResolveFeatureFilePreviewInTerminal()
	if !resolved {
		t.Errorf("expected file_preview_in_terminal to resolve to true, got false")
	}
}

func TestResolveExternalEditor(t *testing.T) {
	// Set XDG_CONFIG_HOME to a temporary directory.
	tmpDir, err := os.MkdirTemp("", "teams-tui-editor-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	oldXdg := os.Getenv("XDG_CONFIG_HOME")
	defer os.Setenv("XDG_CONFIG_HOME", oldXdg)
	os.Setenv("XDG_CONFIG_HOME", tmpDir)

	// Clean env vars to test fallbacks
	oldEditor := os.Getenv("EDITOR")
	oldVisual := os.Getenv("VISUAL")
	defer func() {
		os.Setenv("EDITOR", oldEditor)
		os.Setenv("VISUAL", oldVisual)
	}()
	os.Unsetenv("EDITOR")
	os.Unsetenv("VISUAL")

	// Case 1: Default fallback when config and env vars are not set -> "vim"
	resolved := ResolveExternalEditor()
	if resolved != "vim" {
		t.Errorf("expected default editor to resolve to 'vim', got %q", resolved)
	}

	// Case 2: Environment variable EDITOR is set -> "nano"
	os.Setenv("EDITOR", "nano")
	resolved = ResolveExternalEditor()
	if resolved != "nano" {
		t.Errorf("expected EDITOR env var to resolve to 'nano', got %q", resolved)
	}
	os.Unsetenv("EDITOR")

	// Case 3: Environment variable VISUAL is set -> "emacs"
	os.Setenv("VISUAL", "emacs")
	resolved = ResolveExternalEditor()
	if resolved != "emacs" {
		t.Errorf("expected VISUAL env var to resolve to 'emacs', got %q", resolved)
	}
	os.Unsetenv("VISUAL")

	// Case 4: Config file specifies editor -> "neovim"
	appDir, err := GetAppDir()
	if err != nil {
		t.Fatalf("GetAppDir failed: %v", err)
	}
	configPath := filepath.Join(appDir, "config.json")

	val := "neovim"
	cfg := Config{
		ExternalEditor: &val,
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	if err := os.WriteFile(configPath, data, 0o600); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	resolved = ResolveExternalEditor()
	if resolved != "neovim" {
		t.Errorf("expected config file editor to resolve to 'neovim', got %q", resolved)
	}
}

func TestResolveURLCommands(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "teams-tui-url-cmd-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	oldXdg := os.Getenv("XDG_CONFIG_HOME")
	defer os.Setenv("XDG_CONFIG_HOME", oldXdg)
	os.Setenv("XDG_CONFIG_HOME", tmpDir)

	// Case 1: Defaults.
	if r := ResolveBrowserCommand(); r != "xdg-open" {
		t.Errorf("expected ResolveBrowserCommand to resolve to 'xdg-open', got %q", r)
	}
	if r := ResolveYoutrackCommand(); r != "" {
		t.Errorf("expected ResolveYoutrackCommand to resolve to '', got %q", r)
	}
	if r := ResolveGitlabCommand(); r != "" {
		t.Errorf("expected ResolveGitlabCommand to resolve to '', got %q", r)
	}

	// Case 2: Config overrides.
	appDir, err := GetAppDir()
	if err != nil {
		t.Fatalf("GetAppDir failed: %v", err)
	}
	configPath := filepath.Join(appDir, "config.json")

	browserVal := "firefox"
	youtrackVal := "yt-cli"
	gitlabVal := "gitlab-cli"
	cfg := Config{
		BrowserCommand:  &browserVal,
		YoutrackCommand: &youtrackVal,
		GitlabCommand:   &gitlabVal,
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	if err := os.WriteFile(configPath, data, 0o600); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	if r := ResolveBrowserCommand(); r != "firefox" {
		t.Errorf("expected ResolveBrowserCommand to resolve to 'firefox', got %q", r)
	}
	if r := ResolveYoutrackCommand(); r != "yt-cli" {
		t.Errorf("expected ResolveYoutrackCommand to resolve to 'yt-cli', got %q", r)
	}
	if r := ResolveGitlabCommand(); r != "gitlab-cli" {
		t.Errorf("expected ResolveGitlabCommand to resolve to 'gitlab-cli', got %q", r)
	}
}
