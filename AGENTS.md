# AI Agent Instructions

---

> [!CAUTION]
> **Never run git write commands** (`git commit`, `git push`, `git tag`, `git rebase`, `git merge`, `git reset`, `git stash`, etc.) unless the user **explicitly requests** it. Only read-only git commands (`git status`, `git diff`, `git log`) may be run freely.

---

## Project Overview

Go-based terminal UI application for Microsoft Teams. Authenticates via OAuth2 Device Code Flow and displays chats and messages using the Microsoft Graph API. Built with the Bubble Tea TUI framework (MVU architecture).

---

## Key Architecture

### Authentication (`auth.go`)
- OAuth2 Device Code Flow with Microsoft Graph API
- Tokens stored in `~/.cache/teams-tui-go/token.json`
- Auto-refreshes expired tokens using `GetValidTokenSilent(clientID)`
- Client ID loaded in order: `.env` → `config.json` → built-in default
- **All background API calls must use `GetValidTokenSilent()`**, never the cached `accessToken` from startup
- **Dynamic scopes**: `StartDeviceFlow(clientID, scopes string)` and `RefreshAccessToken(clientID, refreshToken, scopes string)` accept an explicit scope string. Both callers pass `BuildScopes()` so that any enabled feature flags are included in the token request. The old `scopes` constant has been removed.

### Configuration (`config.go`)
- App data: `~/.config/teams-tui-go/` (via `GetAppDir()`)
- Cache: `~/.cache/teams-tui-go/` (via `GetCacheDir()`)
- Config struct: `ClientID *string`, `NotificationMode *NotificationMode`, `NotificationShowPreview *bool`, `NotificationPreviewLen *int`, `MessageLimit *int`, `SearchContextLimit *int`, `ChatLimit *int`, `ChatIconTheme *string`, `CustomChatIcons map[string]string`, `ExternalEditor *string`, `ExportDirectory *string`, plus six optional feature flags: `FilePreviewEnabled`, `PresenceEnabled`, `UserProfileEnabled`, `UserProfileExtended`, `TeamsChannelsEnabled`, `ChannelMentionsEnabled`
- `ResolveClientID()`, `ResolveMessageLimit()`, `ResolveSearchContextLimit()`, `ResolveChatLimit()`, and `ResolveExternalEditor()` implement the full precedence chain
- `InitConfig()` is run at application startup to populate any missing configuration keys in `config.json` with their default values and persist them to disk. It defaults `ChatIconTheme` to `"unicode"` and all feature flags to `false`.
- `BuildScopes()` assembles the OAuth2 scope string dynamically: always includes the four basic scopes (`User.Read Chat.ReadWrite offline_access`) and appends optional scopes for each enabled feature flag.
- Six `ResolveFeatureXxx()` helpers (one per feature) read the config and return a bool, used by `BuildScopes()` and during startup to populate `App.Features`.

### API Layer (`api.go`)
- **User Detection**: Identifies the current user by counting name frequency across `oneOnOne` chats
- **Display Names**: Pre-computed in `GetChats()` and stored in `Chat.CachedDisplayName` — **never compute display names in the UI layer**
- **Name Abbreviation**: Group chat members shown as "FirstName LastInitial" (`abbreviateName()`)
- **Filtering**: Current user is automatically filtered from all member lists by name match (not by ID — IDs vary per chat)
- **HTMLToText**: Uses `golang.org/x/net/html` tokenizer for robust HTML-to-text conversion, handling `<img>`, `<attachment>`, `<emoji>`, block elements, HTML entities
- **System events**: Graph `eventDetail` is decoded in `system_events.go`. `Message.GetPlainText()` / `SystemEventSummary()` turn meeting/call/member events into readable text; the UI sender for those rows is `Teams` via `SenderName()`. Do not special-case `"── [system event] ──"` in the UI layer.
- **Read State**: `Chat` includes `Viewpoint` containing `LastMessageReadDateTime` from the server
- **Silent errors**: `GetChatMembers()` returns empty slice on error; `MarkChatAsRead()` silently ignores all errors
- **Downloads**: `DownloadFile` streams to a temp file in the destination directory and renames only after a complete copy, so an interrupted transfer does not replace a valid file or leave a cached partial

### Application State (`app.go`)
- `App` struct holds all runtime state: chats, messages, selection, input mode, notification mode, etc.
- `NotificationMode` enum is JSON-serialised as a string ("None", "Console", "System", "Both")
- `CurrentUserName` is used for filtering and message alignment; it is **not displayed in the UI**
- `FeatureFlags` struct (populated once at startup in `main.go` from `ResolveFeatureXxx()`) exposes booleans for each optional feature. **Always read feature state from `app.Features`** — never call `ResolveFeatureXxx()` inside the Bubble Tea event loop.
- New optional-feature popup / state fields on `App`: `PresencePopupMode`, `PresenceData`, `PresenceLoading`, `PresenceUserName`; `UserProfilePopupMode`, `UserProfileData`, `UserProfileLoading`; `AttachmentCursorMode`, `AttachmentSelectedIndex`; `TeamsData []TeamWithChannels`, `TeamsDataLoading`, `SelectedChannelTeamID`, `SelectedChannelID`; `HelpPopupMode`; `ChatActionPopupMode`, `ChatActionSelectedIndex`; `MentionPopupMode`, `MentionSearch`, `MentionSelectedIndex`, `MentionSuggestions`, `MentionStartIndex`, `TeamMembersCache`.
- **Teams Channels**: `TeamsData` is `[]TeamWithChannels` (loaded once at startup via `loadTeamsChannelsCmd` fired from `Init()`). The sidebar shows a `── Teams ──` divider below chats; `Model.channelSelectedIndex` (-1 = chat mode, ≥0 = channel index into `allChannels()`) drives navigation. Pressing `j` at the last chat enters channel mode; `k` at index 0 exits back to chats. Selecting a channel fires `loadChannelMessagesCmd` and displays messages in the right panel; `MsgChannelMessagesLoaded` populates `app.Messages`. `SelectedChannelTeamID`/`SelectedChannelID` track the active channel (`""` = chat mode).

### UI (`ui.go`)
- Bubble Tea `Model` struct implementing `Init()`, `Update()`, `View()`
- Layout: 30% chat list (left) | 70% messages (right) | status bar (3 lines, bottom)
- Uses `CachedDisplayName` from `Chat` struct — **do not compute display names here**
- Stable chat ordering maintained in `stableChatOrder []string` (list of chat IDs)
  - Only changes when a new message arrives (chat → position 0) or a brand-new chat is added
  - On every API refresh the display list is **rebuilt** from `stableChatOrder`
  - `mergeChats()` adds brand-new chats from the API result by checking `c.LastMessagePreview != nil` directly (not via `m.lastMsgID`) — chats with a preview go to the top; chats without are appended
  - `Init()` does **not** fire `loadChatsCmd` — initial chats are already loaded synchronously in `main.go`; the first background refresh fires ~15 s after startup via the tick timer
- **Favourites**:
  - Stored in `Model.favourites map[string]bool` (chat ID set)
  - Persisted to `~/.config/teams-tui-go/favourites.json` via `LoadFavourites()` / `SaveFavourites()`
  - Loaded at startup in `main.go` and applied before the first render
  - Toggled with `f` key in normal mode; status message confirms add/remove
  - Favourited chats are pinned at the top of the sidebar, sorted **alphabetically by display name**
  - `rebuildChatList()` splits chats into favourites (sorted by name) + non-favourites (stable order)
  - `promoteChat()` is a no-op for favourited chats so new messages don't displace them
  - Favourited chats with old/unloaded activity still show up once their data is in `byID` cache
  - The `★` icon appears before the chat type tag in the sidebar (yellow for non-selected, inline for selected)
- **Read Logic**:
  - `lastMsgID` and `lastMsgTime` track latest content
  - `lastReadMsgID` tracks what was read locally in this session
  - `isUnread(chat)` compares latest message time with server viewpoint and local read state
  - `markRead()` triggers `MarkChatAsRead` API on focus, selection change, or key press
  - **Reactions Read Tracking**: `lastReadReactions` maps chat ID to a set of unique reaction keys (`messageID:reactorNameOrID:reactionType`). Any reaction from another user that is not in this map is considered unread, causing the reaction's actual emoji prefix (e.g. `❤️`, `👍`, `😆`, etc.) to be displayed on the chat in the sidebar and trigger desktop notifications if the app is blurred or not active.
- **Focus Tracking & Sleep/Idle Mode**:
  - Terminal focus reporting enabled via `\x1b[?1004h`; `tea.FocusMsg`/`BlurMsg` update `focused` state.
  - The 3-second message polling for the active chat is automatically paused when the window is blurred (unfocused) or when manually entering sleep mode.
  - Pressing `Esc` in normal mode manually triggers sleep/idle mode, which de-selects the active chat (`SelectedIndex = -1`, `channelSelectedIndex = -1`), clears messages from memory, pauses active polling, and shows a sleep placeholder screen on the right.
  - Navigating back to a chat/channel or refocusing the window instantly wakes up the polling and pulls the latest messages.
- Background tasks issued as Bubble Tea `Cmd` functions returning typed messages (`MsgChatsLoaded`, `MsgMessagesLoaded`, `MsgTick`, `MsgSendDone`)
- **Search Architecture**:
  - Activated by `/` in normal mode, which opens a beautiful, responsive fullscreen-budgeted modal overlay popup (`SearchPopupMode`) so the main chat view remains completely responsive and lag-free.
  - Pressing `Enter` in the search textinput submits the query, focuses the results navigation list, and triggers background recursive loading of older messages directly into a separate `HistoryMessages` cache (updating `HistoryNextLink`) using an `IsSearch` flag to separate background loads from main chat lists.
  - Matching messages are dynamically parsed with surrounding context window messages (`search_context_limit`, default 3) before and after, automatically deduplicated, sorted chronologically, and drawn with high-contrast gap indicators (`─── [gap in history] ───`) for breaks in conversation flow.
  - In navigation mode, `j`/`k` scroll results, `y` yanks the selected message body, `u` extracts/selection-copies URLs, and `g` jumps to the selected message in the normal chat view (merging paged search history and setting selection/scrolling focus).
  - History cache, query, selected result index, and viewport scroll offsets are fully preserved and persisted *per chat* on close/reopen, avoiding redundant downloads and maintaining independent search states when switching between chats.
  - Main chat viewport offsets and snap-to-bottom values are preserved and restored cleanly when entering and exiting search popup mode.
- **Chat Search & Open Popup**:
  - Activated by `c` in normal mode, which opens a fullscreen-budgeted modal overlay popup (`UserSearchPopupMode`).
  - While typing (`UserSearchMode`), it filters already loaded chats/members on-the-fly and populates `UserSearchLocalResults` under the `[Local Chat]` category.
  - Pressing `Enter` in the input field:
    - If the input contains `@` (looks like an email/UPN), it blurs the input and triggers a background `createChatCmd` calling `POST /chats` with type `oneOnOne` to retrieve/open the chat directly.
    - Otherwise, it blurs the input to focus the results navigation list.
  - Displays a filtered list of local chats.
  - In navigation mode, `j`/`k` move the selection, `/` refocuses the input, and `Enter` opens the selected local chat.
  - On success of `createChatCmd`, the chat is added/promoted, stable order is rebuilt, and the chat is opened and selected automatically.
- **Message View/Preview Popup**:
  - Activated by pressing the `v` key in message selection mode (`m` in normal mode).
  - Opens a fullscreen-budgeted modal overlay popup displaying the full message body, sender info, exact timestamps, listed attachments, and grouped reactions.
  - Groups reaction emojis and maps reactor user IDs to their actual names and surnames by looking them up in the cached chat members list.
  - While active, `j`/`k` (or `down`/`up` arrow keys) navigate directly to younger/older messages in the chat history, instantly updating the popup contents, and automatically fetching older messages when reaching the top.
  - If a message is too long to fit in the popup, the message body becomes a scrollable viewport using `Shift+J`/`Shift+K` (or `Shift+down`/`Shift+up`), keeping the header, reactions, and attachments visible at all times.
  - **Attachment cursor mode** (Tab key while popup is open): `AttachmentCursorMode` is toggled. `j`/`k` then navigate the attachment list. `Enter` downloads the selected attachment to `~/Downloads/` via `downloadFileCmd` (requires `file_preview_enabled` feature). `Tab` or `ESC` exits cursor mode.
- **Presence Popup** (requires `presence_enabled`):
  - Activated by `p` in message selection mode. Triggers `loadPresenceCmd` for the sender's user ID.
  - `MsgPresenceLoaded` updates `app.PresenceData`; the popup `renderPresencePopup` renders availability and activity with color-coded icons.
  - Closed with `ESC`/`q`/`p`/`Enter`. Handled by `handlePresencePopupKey` in `ui.go`.
- **User Profile Popup** (requires `user_profile_enabled`):
  - Activated by `i` in message selection mode. Triggers `loadUserProfileCmd` for the sender's user ID.
  - `MsgUserProfileLoaded` updates `app.UserProfileData`; `renderUserProfilePopup` shows name, email, and (if `user_profile_extended`) job title, department, office.
  - Results are cached in `profileCache` (in-memory, session lifetime) in `api.go` to avoid redundant Graph API calls.
  - Closed with `ESC`/`q`/`i`/`Enter`. Handled by `handleUserProfilePopupKey` in `ui.go`.
- **Help Popup**:
  - Activated by `?` in normal mode. Renders a keyboard reference and live optional-feature status (enabled/disabled per flag).
  - Handled by `handleHelpPopupKey` / `renderHelpPopup` in `ui.go`. Closed with `ESC`/`q`/`?`/`Enter`.
- **Chat Actions Popup**:
  - Activated by `a` (`normal.actions`) on a selected chat (not channels). `ChatActionPopupMode` overlay; keys live in `app.Keys.ChatActions`.
  - Actions: compose, favourite, export complete Markdown transcript, choose a recording or transcript. Export and recordings/transcripts are bound only in this mode (`e` / `t` by default).
  - Labels must use `FormatKeys`. Close with `esc`/`q`; `j`/`k`/`enter` navigate and run.
  - Export follows every Graph next-link via `GetAllChatMessages` in `export.go`, writes under `export_directory` (default `~/Downloads`), and returns `MsgChatExported`.
- **Recordings / transcripts popup**:
  - Opened from chat actions (`t`). Collects `callRecording` / `callTranscript` events from loaded cache, history, and the current message list (`conversation_artifacts.go`).
  - Recordings prefer `eventDetail.callRecordingUrl`; transcripts fall back to the message or chat `webUrl`. `Enter`/`y` copy the link; `o` opens it. The list is clipped to the popup height so the footer and border stay visible.
- **External Editor Composing**:
  - Activated by `ctrl+g` in compose mode.
  - Temporarily saves the current textarea value to a temporary file, opens the configured editor (`ExternalEditor`), and updates the textarea value on success.
  - Uses Bubble Tea's `tea.ExecProcess` to pause the TUI while the editor executes in the terminal foreground, resuming when the editor process exits.

### Main / Entry Point (`main.go`)
- Startup: banner → auth → profile → chats (with expanded last message preview) → sort → init model → run
- All Bubble Tea commands (async API calls) are defined here
- Initial chat order computed in `loadInitialChatOrder()` using the pre-fetched `LastMessagePreview` field
- **Optional-feature commands**: `loadPresenceCmd`, `loadUserProfileCmd`, `downloadFileCmd`, `loadTeamsChannelsCmd`, `exportChatMarkdownCmd` — each calls `GetValidTokenSilent` then the corresponding function and returns a typed `MsgXxxLoaded` / `MsgFileDownloaded` / `MsgChatExported`.
- `app.Features` is populated here at startup (after loading config) by calling each `ResolveFeatureXxx()` helper.

---

## Important Patterns

1. **Display Names**: Always pre-compute in `api.go → GetChats()` and read from `CachedDisplayName`. Never compute in `ui.go`.
2. **User Filtering**: Done by name matching, not ID. IDs are per-chat base64 strings that vary.
3. **Background Calls**: All API calls in the event loop use `loadMessagesCmd()`, `loadChatsCmd()`, etc. — they return `tea.Cmd` functions, never block.
4. **Token Refresh**: Use `GetValidTokenSilent(clientID)` in all `tea.Cmd` functions, not the startup access token.
5. **No Debug Output**: No `fmt.Printf` / `log.Printf` in production code paths.
6. **Message Order**: API returns messages newest-first. The UI iterates in reverse to display newest at the bottom.
7. **Stable Order**: `stableChatOrder` is the source of truth for display order. It is only mutated by `promoteChat()` and `mergeChats()`.
8. **Silent Failures**: Background refresh errors return `nil` from `tea.Cmd` functions (Bubble Tea ignores nil messages).
9. **Feature Gates**: Check `m.app.Features.XxxEnabled` (not `ResolveFeatureXxx()`) inside the Bubble Tea event loop. Features are resolved once at startup into `app.Features` to avoid repeated file I/O per keypress.
10. **Dynamic Scopes**: Always pass `BuildScopes()` to `StartDeviceFlow` and `RefreshAccessToken`. Never hard-code the scope string.
11. **Rendering Performance Optimization**: When rendering long lists of items in the TUI (e.g., rendering hundreds of messages in `m.app.Messages`), avoid running expensive operations like HTML-to-text parsing and Lipgloss-based `wordWrap` dynamically inside the loop in `View()`. Instead, use cache fields directly on the structs (e.g. `WrappedLinesCached` on `Message` struct) to compute and store wrapped text once, invalidating only when the terminal width or search query changes.
12. **Keybindings**: Defaults live in `DefaultKeyMap()` (`keys.go`). `config.json` `keybindings` is an overlay resolved once at startup into `app.Keys` via `ResolveKeyMap` — do not call `LoadConfig()` per keypress. Handlers match with `pressed(msg, binding)`. Help text and panel hints must read `FormatKeys` / `slashKeys` from `app.Keys`, not hardcoded key names. `ctrl+c` always quits from normal mode. A top-level action name (`next`, `prev`, `close`, …) applies to every mode that shares it; a mode object replaces that action in that mode only. Setting an action replaces its key list.

---

## Common Tasks

### Adding a new Graph API call
1. Add the function to `api.go`, using `graphGet()` or `graphPost()` helpers.
2. Create a `tea.Cmd` wrapper in `main.go` (e.g. `loadXxxCmd()`).
3. Define a typed message struct in `ui.go` (e.g. `MsgXxxLoaded`).
4. Handle the message in `Model.Update()` in `ui.go`.

### Adding UI state
1. Add the field to `App` struct in `app.go`.
2. Add a setter method to `App` if needed.
3. Reference it in `Model.View()` in `ui.go`.
4. Update via `Model.Update()` when a relevant message arrives.

### Changing the layout
- Panel widths: `chatPanelWidth()` / `msgPanelWidth()` in `ui.go`.
- Status bar height is hardcoded to 3 lines in `Model.View()`.
- Use `lipgloss` styles for borders and colours; the palette is at the top of `ui.go`.

---

## Documentation Maintenance

When making significant changes:
1. Update `README.md` with new features, usage instructions, or requirements.
2. Update `AZURE_SETUP.md` if API permission requirements change.
3. Update this `AGENTS.md` with architectural changes or new patterns.
