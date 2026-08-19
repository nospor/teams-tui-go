# Changelog

## [1.2.6] - 2026-08-19

### Bug Fixes

- **Render strikethrough as a single SGR sequence** - ([08c89f0](https://github.com/nospor/teams-tui-go/commit/08c89f067737e779566870996eceb550b78fc11e))



### Miscellaneous Tasks

- **Update CHANGELOG.md for v1.2.5 [skip ci]** - ([d3b0d95](https://github.com/nospor/teams-tui-go/commit/d3b0d959197a5249725c7dc60b6719d7cca534dd))



## [1.2.5] - 2026-08-18

### Features

- *(filepicker)* **Add "." to toggle hidden files and folders** - ([ab1ffdf](https://github.com/nospor/teams-tui-go/commit/ab1ffdf47f4cb12c3fad2c23f88a63666b33cdd4))



### Miscellaneous Tasks

- **Update CHANGELOG.md for v1.2.4 [skip ci]** - ([aaa8e12](https://github.com/nospor/teams-tui-go/commit/aaa8e129fee94e6bde02c992fa8ccf7c81407c3e))



## [1.2.4] - 2026-08-07

### Bug Fixes

- *(compose)* **Enter key blocked when composing message containing an email address** - ([35627cd](https://github.com/nospor/teams-tui-go/commit/35627cd6dd1a75c18d69472918f2d7df824b69bf))



### Other

- **Merge pull request #1 from carun/perf/idle-cpu-usage

perf(tui): eliminate idle CPU usage from the 100ms heartbeat tick** - ([dbf6d3b](https://github.com/nospor/teams-tui-go/commit/dbf6d3b5fc886d186cae14f8061c4492596f4491))



### Performance

- *(tui)* **Eliminate idle CPU usage from the 100ms heartbeat tick** - ([64e31bb](https://github.com/nospor/teams-tui-go/commit/64e31bbb86679a63f7cd028c04bd0c76f059ebd4))


> The app consumed several percent of a CPU core while completely idle.
> Two independent sinks, both driven by the 100ms heartbeat tick that
> Bubble Tea turns into an Update+View cycle ten times a second:
> 
> 1. View() rebuilt the entire UI on every tick. Bubble Tea skips the
>    terminal write when output is unchanged, but only after calling
>    View(), so the full render cost (~1-3ms, scaling with history size)
>    was paid regardless.
> 
> 2. writeAppState() ran on every Update, scanning every chat via
>    isUnread/hasUnreadReactions to compute badge counts — ~166us per
>    call. It already skipped the file write when nothing changed, but
>    the scan itself ran unconditionally, costing more than the render.
> 
> Both are now skipped when a tick did no work. The tick handler sets
> tickDidWork when it changes something visible (e.g. expiring a status
> message); otherwise Update reuses the memoized view and skips the scan.
> 
> The logic defaults to doing the work, so message types added later
> repaint correctly without opting in. A width/height check catches
> resizes even if the dirty flag were stale.
> 
> Measured over 100 idle ticks (10s idle, 100 chats / 500 messages):
>   before:            ~3.1% of one core
>   render cache:       0.161%
>   + writeAppState:    0.014%
> Cached render path: 837us -> 1.08us.



### Miscellaneous Tasks

- **Update CHANGELOG.md for v1.2.3 [skip ci]** - ([6187c59](https://github.com/nospor/teams-tui-go/commit/6187c592b5142182e223bf2b8fff643ae67c75e9))



## [1.2.3] - 2026-07-24

### Features

- *(search)* **Add 'g' shortcut to jump to message in normal view & optimize TUI rendering** - ([fa68cf6](https://github.com/nospor/teams-tui-go/commit/fa68cf6e1a0cd2816195e3044f8b8e220d382171))


> - Implement 'g' key in search popup to merge loaded history, select, and
> scroll-jump to the target message in normal view.
> - Fix channel messages refresh cache overwrite to merge paginated
> results instead of overwriting.
> - Cache word-wrapped message lines on `Message` struct to avoid
> rendering slowdowns (O(N) layout rendering bottleneck in
> `renderMessages`).



### Miscellaneous Tasks

- **Update CHANGELOG.md for v1.2.2 [skip ci]** - ([77da5a4](https://github.com/nospor/teams-tui-go/commit/77da5a406cd9ae30d93c6c9a43e6e337da2792d8))



## [1.2.2] - 2026-07-20

### Features

- *(attachments)* **Append attachment ID to saved filenames to prevent collisions** - ([e21801a](https://github.com/nospor/teams-tui-go/commit/e21801a4df2f382844d2b5619a68fbd3cb8aa00d))



### Miscellaneous Tasks

- **Update CHANGELOG.md for v1.2.1 [skip ci]** - ([0fb4da9](https://github.com/nospor/teams-tui-go/commit/0fb4da91fc513c41479114649325e3cf71d28c1f))



## [1.2.1] - 2026-07-15

### Features

- **Add image_viewer config option for batch image attachment opening** - ([bddb42a](https://github.com/nospor/teams-tui-go/commit/bddb42aa37f5c0f86704de787a45109c773656f1))


> When image_viewer is set (e.g. "sxiv", "feh", "imv"), pressing Enter on
> an image attachment in attachment cursor mode downloads all image
> attachments from that message and opens them together in the configured
> viewer, starting at the selected one.
> 
> Viewer-specific start flags are handled automatically:
> - sxiv/nsxiv: -n INDEX (1-based)
> - feh: --start-at PATH
> - imv: -n PATH
> - generic: selected image passed first
> 
>  Falls back to xdg-open single-file behaviour when image_viewer is
> empty.



### Other

- **Auto-focus first image attachment when pressing v** - ([015e52a](https://github.com/nospor/teams-tui-go/commit/015e52a57684027f8a5e5e60e312081ae732f0ff))



### Miscellaneous Tasks

- **Update CHANGELOG.md for v1.2.0 [skip ci]** - ([abef84a](https://github.com/nospor/teams-tui-go/commit/abef84aec889e6035a6f67b6870164ddc7be8f75))



## [1.2.0] - 2026-07-07

### Features

- **Allow viewing messages in external editor using Ctrl+g in message mode and popup** - ([83e1066](https://github.com/nospor/teams-tui-go/commit/83e1066f812a34548b77f24e5ff56d7fe20892bd))


- **Add URL opening capability with custom YouTrack/GitLab command overrides and split yank/open workflows** - ([cca5cb5](https://github.com/nospor/teams-tui-go/commit/cca5cb50f97bc1d2dab57cc64c99a3da58a36eb5))



### Miscellaneous Tasks

- **Update CHANGELOG.md for v1.1.9 [skip ci]** - ([4ef572a](https://github.com/nospor/teams-tui-go/commit/4ef572a3a506c872e6c92050994b113d462311fb))



## [1.1.9] - 2026-07-06

### Features

- **Write total unread messages and reactions counts to state.json cache file** - ([9d768b0](https://github.com/nospor/teams-tui-go/commit/9d768b053d88af93a81680307b6eae77f65b7571))



### Miscellaneous Tasks

- **Update CHANGELOG.md for v1.1.8 [skip ci]** - ([e610966](https://github.com/nospor/teams-tui-go/commit/e610966da6da7749e589c6f84ebe0a9ed126bd3e))



## [1.1.8] - 2026-07-01

### Bug Fixes

- **Trigger instant message refresh for chats with new background activity** - ([5e5d93f](https://github.com/nospor/teams-tui-go/commit/5e5d93f1f76e13e7d9a512d58b9fdb74ecaf1654))



### Miscellaneous Tasks

- **Update CHANGELOG.md for v1.1.7 [skip ci]** - ([bc8d695](https://github.com/nospor/teams-tui-go/commit/bc8d695d13cec0c8e07e548a07ef012731695fee))



## [1.1.7] - 2026-06-23

### Bug Fixes

- *(ui)* **Poll reactions in round-robin so all chats update in background and sleep mode** - ([e24e66f](https://github.com/nospor/teams-tui-go/commit/e24e66f8fef79849ad1930cc730bef9b81778bf3))



### Miscellaneous Tasks

- **Update CHANGELOG.md for v1.1.6 [skip ci]** - ([ba40213](https://github.com/nospor/teams-tui-go/commit/ba402135a7297b6e3cd487a9b1468380b8c94464))



## [1.1.6] - 2026-06-22

### Bug Fixes

- *(ui)* **Enable channel background polling during sleep and blur modes** - ([b0d1ae7](https://github.com/nospor/teams-tui-go/commit/b0d1ae730da9a5f19947814c9e7def96e76f450c))


- *(ui)* **Resolve message list jumping and initial load delay** - ([5145eb1](https://github.com/nospor/teams-tui-go/commit/5145eb1af3c4a589d778efc27545ef4e3d5438ac))



### Miscellaneous Tasks

- **Update CHANGELOG.md for v1.1.5 [skip ci]** - ([7b8fd59](https://github.com/nospor/teams-tui-go/commit/7b8fd590ac384a869de36662dbac7b92ac4b4d0a))



## [1.1.5] - 2026-06-18

### Features

- *(filepicker)* **Support sorting by datetime and persistence** - ([2cd8a45](https://github.com/nospor/teams-tui-go/commit/2cd8a45b400a583818fd97d731a94965f7b82f5e))


- **Add compose mode file attachment and OneDrive/SharePoint upload support** - ([13d7400](https://github.com/nospor/teams-tui-go/commit/13d74000aaad93da677a8e654b35c29ac2b07c26))


> - Implement file picker popup (Ctrl+f) in compose mode gated by
> `file_upload_enabled`.
> - Support files up to 50MB with automated upload sessions for chunked
> writes (> 4MB).
> - Resolve reference attachment mapping issue by matching
> `listItemUniqueId` parsed from OneDrive/SharePoint response ETags.
> - Add status indicators for message uploading, sending, and updating
> states.



### Miscellaneous Tasks

- **Update CHANGELOG.md for v1.1.4 [skip ci]** - ([2117acd](https://github.com/nospor/teams-tui-go/commit/2117acd7c7dc7ad97c9d9e30e254074b2f49ac2d))



## [1.1.4] - 2026-06-18

### Features

- *(ui)* **Allow dismissing mention autocomplete with Esc** - ([3f68ace](https://github.com/nospor/teams-tui-go/commit/3f68ace808d6daec00ee3fa68f03dd3f25b4964d))


- *(channels)* **Sort threads by last activity timestamp** - ([c041b3c](https://github.com/nospor/teams-tui-go/commit/c041b3cdcd5f5b852b8b43474ce913f781e72506))


> Instead of sorting root channel messages strictly by their root creation
> date, sort them by their last activity (the newest timestamp between the
> root message and all its replies).
> 
> This ensures that:
> 1. Active threads with new replies bubble up to the bottom of the
>    visible channel view, rather than staying scrolled off-screen.
> 2. The sidebar unread indicator and channel sorting correctly trigger
>    and bubble up based on the true latest reply timestamp.


- *(ui)* **Add bulk user presence status popup for chats** - ([1d9a8f0](https://github.com/nospor/teams-tui-go/commit/1d9a8f0d5463369841c01d01fd7d9341c0c517c5))


> Add support for checking the real-time presence status of all
> participants in oneOnOne and group chats by pressing 'p' in the sidebar
> chat list (when the 'presence_enabled' configuration is enabled).



### Other

- **Exclude quoted-reply references from view popup attachments** - ([c6abfbe](https://github.com/nospor/teams-tui-go/commit/c6abfbedbefe4acca045730056cfcea37b25200f))



### Miscellaneous Tasks

- **Update CHANGELOG.md for v1.1.3 [skip ci]** - ([e149b6e](https://github.com/nospor/teams-tui-go/commit/e149b6e0920d563056454314be7fd1b62b409902))



## [1.1.3] - 2026-06-17

### Features

- *(api)* **Filter out rich card and URL-preview attachments** - ([e047ae0](https://github.com/nospor/teams-tui-go/commit/e047ae02dcedfbc3a4876a2060c61cdfa76c478f))


> Only keep actual file attachments (SharePoint/OneDrive), inline images,
> and message references. Filter out link unfurls, adaptive cards, and
> video preview attachments (such as YouTube link cards) to prevent them
> from showing up in the UI as downloadable file attachments.


- *(api)* **Append ellipsis to group/meeting chat names with more than 3 members** - ([0077041](https://github.com/nospor/teams-tui-go/commit/0077041098ecd17ca321da12ed603db24b7955f6))



### Bug Fixes

- **Prevent channel messages leaking into chat view on Tab switch and chat refresh** - ([d087200](https://github.com/nospor/teams-tui-go/commit/d0872004e68b14bf9fa125871807dbdd3f47ca7f))


- **Prevent slice out-of-bounds panic in notifyReaction** - ([822c70e](https://github.com/nospor/teams-tui-go/commit/822c70e9ed8a04eb7b2afb574d229d7dc8edaba8))


- **Notify and highlight brand-new chats as unread** - ([d33ba1c](https://github.com/nospor/teams-tui-go/commit/d33ba1c18e12dba8a8e3759f052fbbdfb7b832ee))



### Miscellaneous Tasks

- **Update CHANGELOG.md for v1.1.2 [skip ci]** - ([6ce7ba0](https://github.com/nospor/teams-tui-go/commit/6ce7ba0277558763831a63459725930bd9864283))



## [1.1.2] - 2026-06-16

### Features

- **Support composing messages in external editor via Ctrl+g** - ([278392c](https://github.com/nospor/teams-tui-go/commit/278392c0219c570bd493d9ef7b1c3b82de3b0fb2))



### Bug Fixes

- *(ui)* **Resolve reactor display name from chat members in notifications** - ([f0b2206](https://github.com/nospor/teams-tui-go/commit/f0b2206cf0d3ae71cba7fbcc8d0ac303902929bc))


- *(ui)* **Immediately persist and cache reaction updates to prevent loading delays** - ([3464f34](https://github.com/nospor/teams-tui-go/commit/3464f34891e823d2401ddd2e0f9616738f557866))



### Miscellaneous Tasks

- **Update CHANGELOG.md for v1.1.1 [skip ci]** - ([58b3ab4](https://github.com/nospor/teams-tui-go/commit/58b3ab43a76b08c90774256aebe467aadc203cad))



## [1.1.1] - 2026-06-16

### Features

- *(ui)* **Display channel message subjects** - ([7a49385](https://github.com/nospor/teams-tui-go/commit/7a49385ae1590f41cce086ebdb2425b30f5bdd30))



### Bug Fixes

- *(search)* **Persist search status text per conversation to avoid stale footers** - ([5afad6e](https://github.com/nospor/teams-tui-go/commit/5afad6ed6b40425011cf5384ac850f2840a663a7))


- *(ui)* **Prevent duplicate '@' prefix on split user mentions** - ([1005207](https://github.com/nospor/teams-tui-go/commit/100520789f8d7c39955d0c43951f172704fb5f28))



### Other

- **Align main thread messages left and replies right in channels** - ([423107c](https://github.com/nospor/teams-tui-go/commit/423107c2bc2e998a1cd44dd26314fcc9bdf000ba))



### Performance

- *(search)* **Load SQLite history asynchronously and optimize channel searching** - ([022d9b4](https://github.com/nospor/teams-tui-go/commit/022d9b4db6e0be6678775581b79bc9b6a3d97d58))



### Miscellaneous Tasks

- **Update CHANGELOG.md for v1.1.0 [skip ci]** - ([bc02e8a](https://github.com/nospor/teams-tui-go/commit/bc02e8aa368e3b0c5cc17ce14c27af5f018ddc8b))



## [1.1.0] - 2026-06-16

### Features

- **Add SQLite database message caching** - ([96dc196](https://github.com/nospor/teams-tui-go/commit/96dc196c24e106ad409e38d01b94c32ec47eff52))


> - Implements local SQLite database caching under
> `~/.cache/teams-tui-go/` using a pure-Go SQLite driver.
> - Adds `sqlite_enabled` optional feature flag in configuration (disabled
> by default).
> - Caches and loads chat/channel messages instantly on startup or select,
> using a write-through background update check.
> - Caches incoming message notifications immediately to render them
> instantly when entering the target chat.
> - Restores chronological ordering and absolute gap-detection logic for
> the search popup, selecting the latest match closest to today by
> default.
> - Prevents search execution freezes on partial queries by querying only
> on explicit Enter submission.



### Miscellaneous Tasks

- **Update CHANGELOG.md for v1.0.4 [skip ci]** - ([3e051e1](https://github.com/nospor/teams-tui-go/commit/3e051e17b43911818a3e575296fb0fc380dd9af8))



## [1.0.4] - 2026-06-13

### Features

- **Format URLs as clickable HTML links when sending messages** - ([d00b69a](https://github.com/nospor/teams-tui-go/commit/d00b69a3e01ebcfe9f3b304eb9de5feed22d6a3b))


- **Highlight and prefix user mentions in messages** - ([36bc4fa](https://github.com/nospor/teams-tui-go/commit/36bc4fa05d18b6af4fb03751256d260e934049d9))


- *(ui)* **Add scrolling and progress indicator to help popup** - ([515979f](https://github.com/nospor/teams-tui-go/commit/515979f30d51b773ce0cd0a108dc2d169ecae00a))


- **Add user @mentions autocomplete in compose input** - ([cc939ae](https://github.com/nospor/teams-tui-go/commit/cc939ae001a5fa97117809d71a5394dda963e7ee))


> - Triggers suggestions dropdown when typing `@` in compose textarea
> - Supports scrollable dropdown navigation (Up/Down/Tab) for large group
> chats/channels
> - Autocompletes name on Enter on the same line (appends a trailing
> space)
> - Generates rich HTML `&lt;at>` tags and packs Graph API mentions payload
> - Adds `channel_mentions_enabled` config key (requests
> `TeamMember.Read.All` scope)


- *(ui)* **Add scroll padding and persist channel scroll offset in sidebar** - ([e5924a4](https://github.com/nospor/teams-tui-go/commit/e5924a418b9eb8de43b15939c753f16a019d4d5b))


> - Scroll the chat and channel lists 3 rows before hitting the top/bottom
> boundary.
> - Persist the channel list scroll offset in the App struct to prevent it
> from resetting on render.
> - Clamp scroll padding dynamically for small viewports to avoid
> off-screen selections.



### Other

- **Merge branch 'main' of github.com:nospor/teams-tui-go** - ([74cfee6](https://github.com/nospor/teams-tui-go/commit/74cfee6b4bd1080d006092441f7980dfc39ca5e9))



### Performance

- *(channels)* **Restore cached messages instantly when switching channels** - ([76e2525](https://github.com/nospor/teams-tui-go/commit/76e25253a3a6b8fcb9aa2e30a40d672877a7e662))


> Improve TUI responsiveness by pre-rendering cached channel messages
> on selection change to avoid showing a blank loading screen. Fetch
> fresh messages silently in the background immediately after.



### Miscellaneous Tasks

- **Update CHANGELOG.md for v1.0.3 [skip ci]** - ([0c4dfe3](https://github.com/nospor/teams-tui-go/commit/0c4dfe33c88c07c0599ea71f302e7f36003346e4))



## [1.0.3] - 2026-06-12

### Features

- *(channels)* **Add background loading, activity sorting, and hidden channels** - ([a6f3f11](https://github.com/nospor/teams-tui-go/commit/a6f3f11eedb1db0c10095d58f151473a871e02b9))


> - Add `channel_msg_refresh_min` configuration parameter (default: 2
> minutes).
> - Group unhidden channels at the top, sorted by latest message activity.
> - Keep hidden channels at the bottom in their original default order.
> - Toggle channel hidden state using the `h` key in normal mode.
> - Render hidden channels in a grayed-out color (`colDimGray`) and skip
> them in background polling.
> - Pause background and active channel polling when in sleep/idle mode.



### Miscellaneous Tasks

- **Update CHANGELOG.md for v1.0.2 [skip ci]** - ([9906c8a](https://github.com/nospor/teams-tui-go/commit/9906c8af853df3a76ce4c78269cf13195a63c032))



## [1.0.2] - 2026-06-12

### Features

- *(ui)* **Implement sleep mode and focus-aware background polling** - ([a77ce90](https://github.com/nospor/teams-tui-go/commit/a77ce9082f6d1316ae69c3cd69dcc7c57361c100))


> - Pause 3-second active chat polling when terminal window is blurred
> (unfocused) or when no chat is selected.
> - Add normal mode Esc shortcut to manually de-select chat/channel and
> enter sleep mode.
> - Render a centered dimmed sleep mode placeholder in the right panel
> when inactive.
> - Immediately wake up and fetch fresh messages when focusing the window
> or selecting a chat/channel.
> - Guard keys requiring an active chat context (input, search, scroll)
> when sleep mode is active.
> - Fix a bug where the '?' key did not trigger the help popup in normal
> mode.



### Miscellaneous Tasks

- **Update CHANGELOG.md for v1.0.1 [skip ci]** - ([3b80713](https://github.com/nospor/teams-tui-go/commit/3b8071391ace491e13bbdb2485b521af124ec1a3))



## [1.0.1] - 2026-06-12

### Bug Fixes

- **Fixing pipeline for windows** - ([0df7bf1](https://github.com/nospor/teams-tui-go/commit/0df7bf1551676cebcd9ed4f2646ba9c86e65d1b2))



### Other

- **Merge branch 'main' of github.com:nospor/teams-tui-go** - ([95ae7d6](https://github.com/nospor/teams-tui-go/commit/95ae7d63bed3146456522073763fa6fd9bb26eab))



### Miscellaneous Tasks

- **Update CHANGELOG.md for v1.0.0 [skip ci]** - ([7c9253d](https://github.com/nospor/teams-tui-go/commit/7c9253d4b686d09f771be1a4cd75270bc830bbde))



## [1.0.0] - 2026-06-12

### Features

- **Add optional features (Presence, Profiles, Channels, File Downloads, Kitty Image Previews, Clipboard)** - ([f730cad](https://github.com/nospor/teams-tui-go/commit/f730cad576ea44889e2b8a9fa4db222c5fd219a2))


> Introduce dynamic, configuration-gated feature flags to request
> corresponding
> Microsoft Graph permissions scopes and expose several interactive TUI
> components:
> 
> - **Presence Popup ('p')**: View real-time availability and activity
> statuses.
> - **User Profile Popup ('i')**: Display directory contact cards (and
> extended job/office info).
> - **Teams & Channels**: Support browsing joined teams and channel
> message feeds.
> - **Attachment Viewer**: Navigate ('Tab') and download ('Enter')
> attachments, with inline image rendering using the Kitty Graphics
> Protocol.
> - **Clipboard Pasting**: Pastes images directly from the system
> clipboard (Linux wl-paste/xclip, macOS, Windows).
> - **Dynamic Scopes**: Build OAuth2 scopes dynamically at startup based
> on enabled feature flags in `config.json`.



### Miscellaneous Tasks

- **Update CHANGELOG.md for v0.9.5 [skip ci]** - ([79388e1](https://github.com/nospor/teams-tui-go/commit/79388e13d168de5ca6108798769b4387fde9c535))



## [0.9.5] - 2026-06-03

### Features

- **Add customizable chat type icons and themes** - ([5f47863](https://github.com/nospor/teams-tui-go/commit/5f478630330e39422570ffb26915f78e68e9a3f6))


- *(search)* **Implement accent-insensitive matching for chats and messages** - ([64d46b0](https://github.com/nospor/teams-tui-go/commit/64d46b06fb005b3d8f866ac972c643858134280f))



### Bug Fixes

- *(api)* **Deduplicate chats returned by GetChats to prevent sidebar duplicates** - ([5cecd84](https://github.com/nospor/teams-tui-go/commit/5cecd845477737583c978127a3f135f108d3de33))


> The Microsoft Graph API can return the same chat on multiple pages when
> a new message arrives mid-pagination (cursor drift: the chat shifts
> position between page fetches). Without deduplication, the same chat ID
> could appear twice in the initial stableChatOrder, causing it to render
> twice in the sidebar.
> 
> Added a dedup pass in GetChats() after all pages are collected. When
> a duplicate ID is found, we keep the entry with the newer last-message
> or last-updated timestamp so that fresh data always wins.


- *(ui)* **Prevent new chats from being missed during background refresh** - ([d6f8a0d](https://github.com/nospor/teams-tui-go/commit/d6f8a0dfe5196547f2f0e63b768bb74afc509a04))


> - Remove redundant loadChatsCmd from Init() since the initial chat list is already loaded synchronously in main.go. This prevents a potential concurrent race condition with the first tick-driven refresh.
> - Set model.lastChatRefresh = time.Now() at startup in main.go so the first tick refresh triggers 15s after startup rather than immediately.
> - In mergeChats(), check c.LastMessagePreview != nil directly instead of checking m.lastMsgID to determine whether a chat is new and has messages, removing a fragile dependency on event handler loop ordering.
> - Remove unused freshByID map from mergeChats().
> - Update AGENTS.md documentation to reflect these architectural changes.


- *(search)* **Exclude message creators from chat history search** - ([afd4be1](https://github.com/nospor/teams-tui-go/commit/afd4be10f662555b714d26a279951bb2009305cd))



### Other

- **Merge branch 'main' of github.com:nospor/teams-tui-go** - ([968b869](https://github.com/nospor/teams-tui-go/commit/968b869018009bd7c53fe05ed21f1c19c0ea0531))



### Miscellaneous Tasks

- **Update CHANGELOG.md for v0.9.4 [skip ci]** - ([6911999](https://github.com/nospor/teams-tui-go/commit/691199926c1e496fb11674ad1220c7268162d0ef))



## [0.9.4] - 2026-06-01

### Features

- **Preview message in select mode** - ([32c3355](https://github.com/nospor/teams-tui-go/commit/32c3355e5946cd6d4625317f00e60104662df0a5))



### Bug Fixes

- **Reactions shows in chat lists now** - ([b10b0c0](https://github.com/nospor/teams-tui-go/commit/b10b0c036a97caa831f92f899881312e0504ae54))



### Miscellaneous Tasks

- **Update CHANGELOG.md for v0.9.3 [skip ci]** - ([2b23adc](https://github.com/nospor/teams-tui-go/commit/2b23adc8bfbedd006b1ffe573685a9a93ef8d677))



## [0.9.3] - 2026-05-28

### Features

- *(ui)* **Add favourite chats with persistent storage** - ([8cd3635](https://github.com/nospor/teams-tui-go/commit/8cd36355493a1f2ccfa75968bc22bc21e12a1585))


> Pin any chat to the top of the sidebar with `f`. Favourites are stored
> in ~/.config/teams-tui-go/favourites.json and restored on startup.
> - Favourited chats are anchored at the top, sorted alphabetically by
>   display name, regardless of message activity
> - The ★ icon is shown in yellow next to favourited chats in the sidebar
> - Remap chat search popup from `f` to `c` to free up `f` for favourites
> - Gracefully handles favourited chats with old activity not in the
>   current API load (preserved in file, shown once data is available)



### Miscellaneous Tasks

- **Update CHANGELOG.md for v0.9.2 [skip ci]** - ([f3167b9](https://github.com/nospor/teams-tui-go/commit/f3167b90091356c2befad2fd11e051ae882c38e8))



## [0.9.2] - 2026-05-27

### Bug Fixes

- **Use full GitHub commit URLs in changelog** - ([8ba9d6c](https://github.com/nospor/teams-tui-go/commit/8ba9d6c836c56235055c94ecfc667c57d4aa6f58))


> Bare commit hashes were being used as link targets, producing
> invalid URLs. Now generates proper /commit/&lt;hash> links.


- **Escape HTML tags in changelog commit bodies** - ([d3db5d4](https://github.com/nospor/teams-tui-go/commit/d3db5d4b48760d6c86527a0707cfd951e235bedf))


- **Proper changelog generation** - ([ee09472](https://github.com/nospor/teams-tui-go/commit/ee094724eb5a62ad99de9685d47983c7b4de3ec4))



### Other

- **Merge branch 'main' of github.com:nospor/teams-tui-go** - ([ff782cb](https://github.com/nospor/teams-tui-go/commit/ff782cb532a742eeaf598cee7553eadf92e39472))


- **Merge branch 'main' of github.com:nospor/teams-tui-go** - ([4876936](https://github.com/nospor/teams-tui-go/commit/4876936a9d6c49c21e3efb44feb9f836b53bebc5))



### Miscellaneous Tasks

- **Update CHANGELOG.md for v0.9.2 [skip ci]** - ([de3a09d](https://github.com/nospor/teams-tui-go/commit/de3a09d87ce69f3a0d35e084d798f8dfd0a9d286))


- **Update CHANGELOG.md for v0.9.2 [skip ci]** - ([51bf712](https://github.com/nospor/teams-tui-go/commit/51bf712f9ec6845f80fc0d0dd33e4bab302bfbed))


- **Update CHANGELOG.md for v0.9.0 [skip ci]** - ([da31b06](https://github.com/nospor/teams-tui-go/commit/da31b06a31d99788023d55400276494a75e839a5))



## [0.9.1] - 2026-05-27

### Features

- **Add native Teams quoted message support (receive & send)** - ([63a3085](https://github.com/nospor/teams-tui-go/commit/63a3085ebebe0bc155088c3b1c02f153433c81e7))


- **Markdown formatting for send/receive and edit round-trip** - ([b97d410](https://github.com/nospor/teams-tui-go/commit/b97d4100bef463233205a3124dd8bea6204b7ebb))


> Add markdown-to-HTML conversion on send and HTML-to-styled-terminal
> rendering on receive. Also preserve markdown syntax when editing
> existing messages.
> Send side (markdown.go):
> - New markdownToHTML() converts **bold**, *italic*, ~~strike~~,
>   `inline code`, fenced code blocks, and bullet/ordered lists to
>   Teams-compatible HTML before posting via the Graph API
> - Single-line plain-text messages bypass conversion entirely
> - formatMessageBody() in api.go updated to use markdownToHTML()
> Receive side (api.go - HTMLToText):
> - Track &lt;b>/&lt;strong>, &lt;em>/&lt;i>, &lt;s>/&lt;strike>/&lt;del>, &lt;code> state
>   and apply lipgloss ANSI styles (bold, italic, strikethrough, amber)
> - &lt;pre>&lt;code> blocks rendered in green
> - &lt;ul>/&lt;ol>/&lt;li> rendered with • / 1. prefixes (dimmed, indented)
> - Inline styles compose correctly with existing link/URL rendering
> Edit round-trip (markdown.go - HTMLToMarkdown):
> - New HTMLToMarkdown() converts stored HTML back to markdown syntax
>   when 'e' is pressed, so bold/italic/code/lists are preserved in
>   the edit textarea rather than being stripped to plain text
> - &lt;code> content is buffered and emitted as a fenced block if
>   multi-line, inline backtick if single-line (handles Teams stripping
>   &lt;pre> wrappers from the stored HTML)
> - Whitespace-only paragraphs (both &nbsp; and plain space variants)
>   are treated as blank-line placeholders with correct blank-line
>   preservation around code blocks


- **Cache messages per chat to eliminate reload flash on revisit** - ([6f756a0](https://github.com/nospor/teams-tui-go/commit/6f756a05ea1262c64ea078de28f4874d23b71ed2))



### Bug Fixes

- **When in "select" mode, it now loads history messages** - ([d9f1a3a](https://github.com/nospor/teams-tui-go/commit/d9f1a3ad61d8c9e023c119107982a6e3af4dc3f5))


- **Edited older messages now update immediately in chat view** - ([4583554](https://github.com/nospor/teams-tui-go/commit/458355416af66fede10194b2a03c3ebea53bc259))


- **Preventing long messages in select mode to jump** - ([0cbb053](https://github.com/nospor/teams-tui-go/commit/0cbb053b978dc75818bd283ddf65854d098f3e84))



## [0.9.0] - 2026-05-26

### Features

- **More messages and chats** - ([3f6080f](https://github.com/nospor/teams-tui-go/commit/3f6080fd82ce81dffd6e9bb69de49fee92e94391))


- **Creating default config** - ([ecd939c](https://github.com/nospor/teams-tui-go/commit/ecd939c73410899fc58bd9dad96708edfdf8b660))


- **Search chats** - ([1a8fe9c](https://github.com/nospor/teams-tui-go/commit/1a8fe9c93b3530e584a5e3f702df6212871fa28b))


- **Indicators for chats with new reactions** - ([f99e832](https://github.com/nospor/teams-tui-go/commit/f99e832b389c61f3777e9db2f0a7fd6bca05c3db))



### Bug Fixes

- **Improve chat read tracking, notification logic, and reload stability** - ([b3c7ab1](https://github.com/nospor/teams-tui-go/commit/b3c7ab1b2299c1650bacc76e3e48e68183ce3af7))


> - Set default HTTP client timeout to 15s to prevent hanging background
> requests.
> - Avoid returning nil from background chat loaders on token refresh/API
> error; fallback to existing chat states instead.
> - Refine unread message detection using last message times and a
> 1-second threshold to prevent duplicate notification triggers.
> - Mark the active chat as read on the server when updates arrive and the
> terminal is focused.
> - Ensure proper tracking of `LastUpdated` times in initial chat
> ordering.


- **Reactions order** - ([cd334a9](https://github.com/nospor/teams-tui-go/commit/cd334a9d1edd2a097511d0cb5d01f0af68b1d9b9))



### Miscellaneous Tasks

- **Remove boilerplate sentence from changelog header** - ([8230583](https://github.com/nospor/teams-tui-go/commit/8230583445d816fa6bec275daffcdb94ea3d7635))


- **Update CHANGELOG.md for v0.9.0 [skip ci]** - ([579cdaa](https://github.com/nospor/teams-tui-go/commit/579cdaa319f4f8337dc777b0c219691bc2259d14))


- **Suppress Node 20 deprecation warnings and commit CHANGELOG.md to repo** - ([42725c4](https://github.com/nospor/teams-tui-go/commit/42725c4b9c59a3981bfdeaaf7cef16f57fe38259))


- **Revert to standard GitHub Actions now that suspension is lifted** - ([d1d5500](https://github.com/nospor/teams-tui-go/commit/d1d5500578cbcab23149c4475f8d9f0410cf2e0d))


- **Bypass broken github actions by using golang container and wget** - ([496a60a](https://github.com/nospor/teams-tui-go/commit/496a60a0b9a3d92509465623b5e7fbd96423a3a1))


- **Upgrade actions to v5/v6 and force Node 24 to fix runner downloads** - ([aae325d](https://github.com/nospor/teams-tui-go/commit/aae325db8ee50d9a65c14a816d6a2861bafb62fd))


- **Bump git-cliff-action to v4 to bypass GitHub CDN errors** - ([d7ae7d1](https://github.com/nospor/teams-tui-go/commit/d7ae7d16a97922e12c75e9a58df30e063369e32f))


- **Downgrade setup-go to v4 to mitigate GitHub CDN tarball errors** - ([6ddbfa3](https://github.com/nospor/teams-tui-go/commit/6ddbfa33b9d4028ef5cb95458302fd3391744b95))


- **Setup GitHub Actions for testing, releases, and changelogs** - ([81fea23](https://github.com/nospor/teams-tui-go/commit/81fea23cb8b520a80a4dab50ae91fd87785c949c))


> - Add test workflow (`test.yml`) to run Go tests automatically on pushes
> and PRs to main
> - Add release workflow (`release.yml`) to build multi-platform binaries
> (Linux, macOS, Windows) and publish GitHub Releases automatically when
> pushing version tags (`v*`)
> - Introduce `cliff.toml` to automate conventional changelog generation
> via `git-cliff`, formatting commit bodies using native Markdown newlines
> - Inject dynamic release versions into the binary during the CI build
> process and print the version in the application's startup banner



## [0.8.7] - 2026-05-21

### Bug Fixes

- **Grouping messages by hour** - ([855dce7](https://github.com/nospor/teams-tui-go/commit/855dce7ac1a811d77d4d819abee6bf960e37fe8a))


- **Empty meetings problems** - ([519d8d1](https://github.com/nospor/teams-tui-go/commit/519d8d12fbb73125f5eda5dff3b36738a6e44a3f))


- **Some messages sorting** - ([ef2c95a](https://github.com/nospor/teams-tui-go/commit/ef2c95a54d9f227f0cb1146c631f7a60372ddaf8))



## [0.8.6] - 2026-05-19

### Features

- **Search chats** - ([d45cd0a](https://github.com/nospor/teams-tui-go/commit/d45cd0a2faf0daf2e13bcba5cd345b6ddc35cdc6))



## [0.8.5] - 2026-05-12

### Features

- **Emoticons in composing message** - ([fcb8a53](https://github.com/nospor/teams-tui-go/commit/fcb8a53ab3578748b004532a4ec572864be85e83))


- **Urls** - ([db86d81](https://github.com/nospor/teams-tui-go/commit/db86d8107bec58449dcc4461b831e2b19eb214fd))


- **Showing urls** - ([c7f5065](https://github.com/nospor/teams-tui-go/commit/c7f50657b6cbcb342f710a3a827c7ab37d936341))


- **Emoticons when writing messages** - ([84a740f](https://github.com/nospor/teams-tui-go/commit/84a740f7cb8b7401372f13d6006bd56d1a9c2ae9))



### Bug Fixes

- **Message mode starts now when user left** - ([980e48b](https://github.com/nospor/teams-tui-go/commit/980e48bbf4a05da4615c9ac2c76cab9562e85f26))



## [0.8.3] - 2026-05-12

### Features

- **Answer to messages** - ([8643997](https://github.com/nospor/teams-tui-go/commit/8643997bc2436fc6104480d9d513092e5a82f271))


- **Update/edit messages** - ([1b30aca](https://github.com/nospor/teams-tui-go/commit/1b30aca9ec093c3a40f128aae7b0c7e1b75f9d17))



## [0.8.2] - 2026-05-11

### Features

- **Loading history** - ([77fc082](https://github.com/nospor/teams-tui-go/commit/77fc08222920f079563a6f413bc309a6f0701bc8))



## [0.8.1] - 2026-05-11

### Features

- **Clearing copy messages** - ([bd1c224](https://github.com/nospor/teams-tui-go/commit/bd1c2249ca34546cc9e19e4caf1dbf934f656c75))


- **Yank(copy) message** - ([ec3a6ae](https://github.com/nospor/teams-tui-go/commit/ec3a6ae567ee4fe3a81294e19a82ee06ffe858a1))



## [0.8] - 2026-05-11

### Features

- **Message limit** - ([801a57c](https://github.com/nospor/teams-tui-go/commit/801a57c441a2ed5c4fb4396e12dc8d18cfa22612))



## [0.7.9] - 2026-05-11

### Features

- **Delete messages** - ([6adae23](https://github.com/nospor/teams-tui-go/commit/6adae2351922fec6e2fc7d57e49ab9eca4229b54))



## [0.7.8] - 2026-05-11

### Features

- **Adding reactions to messages** - ([4a7ac0c](https://github.com/nospor/teams-tui-go/commit/4a7ac0cae34866aeb7ec63b8f1a9650e3102893a))


- **Add reaction to a message** - ([93709c9](https://github.com/nospor/teams-tui-go/commit/93709c93c7c1af821530cbe5ef5834a6d3452bc8))


- **Messages interactions** - ([aeca458](https://github.com/nospor/teams-tui-go/commit/aeca4585a4b38fcead7982e6be71ed0c4a646cf5))



### Bug Fixes

- **Normal multiline text** - ([08ec699](https://github.com/nospor/teams-tui-go/commit/08ec6994b15b7d334766e13a006cb3ed11ee777e))



## [0.7.7] - 2026-05-08

### Bug Fixes

- **Wrong formatted text** - ([f814c6d](https://github.com/nospor/teams-tui-go/commit/f814c6d37a3658b9c27a645bc71aa5a237cd762f))



## [0.7.6] - 2026-05-08

### Bug Fixes

- **Not readchat after sending messsage** - ([bccf3cf](https://github.com/nospor/teams-tui-go/commit/bccf3cfcafd6b7e63f94000163d7afa6d0dadf0d))



## [0.7.5] - 2026-05-08

### Features

- **Notification preview** - ([6cc856e](https://github.com/nospor/teams-tui-go/commit/6cc856ecfa58b9e9ecd00154469645d25bdb1fe5))



### Bug Fixes

- **Not read messages** - ([e274b02](https://github.com/nospor/teams-tui-go/commit/e274b022171051cda105af9f5724b175c45344f1))


- **Local time** - ([daa6e69](https://github.com/nospor/teams-tui-go/commit/daa6e69d9d988332b35e4d3d17c8c69e955eff14))


- **Attachments view** - ([d49ad97](https://github.com/nospor/teams-tui-go/commit/d49ad97f7c03c09c03cfd7888859e33ca28e1e2f))



## [0.7] - 2026-05-07

### Bug Fixes

- **Sorting chats** - ([50431eb](https://github.com/nospor/teams-tui-go/commit/50431eb7b317015d9cede33de558f68266781b00))


- **Names in group chats** - ([ae38ea7](https://github.com/nospor/teams-tui-go/commit/ae38ea73bf3100eac1b1006691f3e745cdccc987))


- **Fixing some initial problems** - ([6b10473](https://github.com/nospor/teams-tui-go/commit/6b10473a3c38c797c073738011334b9707d60b86))



### Other

- **Readme** - ([c7ac8cf](https://github.com/nospor/teams-tui-go/commit/c7ac8cfab35bcbe55b095ff4fa57f2baf73e51ed))


- **Init first version** - ([bdd0271](https://github.com/nospor/teams-tui-go/commit/bdd027164e42e4f7a22c849dd1bc9ed42dddd030))


- **Initial commit** - ([f30e39e](https://github.com/nospor/teams-tui-go/commit/f30e39e2f191df8142a594c5482557760a42025e))




