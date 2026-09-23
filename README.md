# teams-tui-go

A keyboard-driven terminal UI client for Microsoft Teams, written in Go.

Authenticates via **OAuth2 Device Code Flow** (no browser redirect needed), fetches your chats and messages from the **Microsoft Graph API**, and displays them in a fast, minimal TUI.

---

## Features

- 🔐 OAuth2 Device Code Flow — authenticate with your Microsoft account, no browser redirect required
- 💬 List all your Teams chats (1:1, group, meetings) with computed display names
- 📨 View messages in any chat with HTML-to-text rendering (images, attachments, emoji, **bold**, *italic*, ~~strikethrough~~, `code`, lists)
- 📅 Readable system events — meeting started/ended, recordings, transcripts, member changes, and other Teams-generated events show as plain text instead of a generic placeholder
- 📤 Chat actions & Markdown export — press `a` on a chat for compose, favourite, and a complete paginated Markdown transcript
- ❤️ Message Interactions — view and add reactions (Heart, Like, Laugh, etc.) to any message
- 🔗 Clickable, Extractable & Openable URLs — links are clickable in supported terminals, can be extracted/copied via the `u` key, and opened in your browser/app via the `o` key
- ✏️ Message Management — send, edit, and delete messages (includes multi-line support)
- **✍️ Markdown Formatting** — compose messages with `**bold**`, `*italic*`, ~~`~~strike~~`~~, `` `code` ``, fenced code blocks, and bullet/ordered lists; formatting is sent as rich HTML to all Teams clients and rendered with ANSI styles in the TUI
- 📋 **Clipboard Image Pasting** — paste images from your system clipboard directly into the compose text field using **Ctrl+V** (automatically base64 encoded and sent as inline HTML attachments)
- 🗣️ **@Mentions & Autocomplete** — mention users in your messages. Typing `@` displays a dropdown list of chat/channel members. Navigate with Up/Down/Tab/Shift+Tab and press Enter to autocomplete the name. Mentions are sent as native Microsoft Teams mentions.
- 🔔 Notification modes: None / Console (BEL + visual bell) / System (desktop) / Both
- 🔄 Smart Background Polling & Sleep Mode — active chat messages poll every 3 s and chat list updates every 15 s. Polling auto-pauses when the terminal window is unfocused (blurred) or when you manually enter sleep mode via the `Esc` key.
- 😊 Emoticon Auto-replacement — popular text emoticons (like `:)`, `:D`, `<3`) are automatically converted to Unicode emojis
- 🔍 Search History — search messages in any chat, recursively loading and indexing all conversation history in the background
- 🔍 Chat Search & Open — filter locally loaded chats or open/start a 1:1 chat directly by entering a UPN/email (bypassing directory search)
- ⭐ Favourites — pin any chat to the top of the sidebar with `f`; favourites are sorted alphabetically and stay anchored regardless of activity
- ❓ Help Popup — press `?` at any time to show a keyboard shortcuts reference with optional feature status

- 🔵 Unread Indicators — chats with new messages are marked with a dot (●) and bold text
- 😊 Reaction Indicators — chats with new reactions from other users are marked with their corresponding emoji (e.g. ❤️, 👍, 😆) and bold text
- ⬆️ New messages bubble chats to the top of the list
- 📌 Stable chat ordering — order only changes when new messages arrive
- 💾 Token persistence — only authenticate once; tokens refresh automatically

**Optional features** (enable per-feature in `config.json`; see [AZURE_SETUP.md](AZURE_SETUP.md)):
- 📎 **File Preview & Download** (`file_preview_enabled`) — Tab through attachments in the message popup; press **Enter** to download and open, or **d** to download only to `~/Downloads/`
  - **Terminal Image Preview** (`file_preview_in_terminal`) — Displays the highlighted image attachment directly inside the details popup on the right side using the Kitty Graphics Protocol (requires `file_preview_enabled: true`)
- ⬆️ **File Browsing & Uploading** (`file_upload_enabled`) — Press `Ctrl+f` in compose mode to open a file browser and attach small files (up to 50MB) from your computer. Files are uploaded to OneDrive/SharePoint and attached to your message.
- 🟢 **User Presence** (`presence_enabled`) — press `p` in message selection mode to see real-time availability of the message sender
- 👤 **User Profile** (`user_profile_enabled`) — press `i` in message selection mode to view extended profile info (name, email, job title, department)
- 🏢 **Teams Channels** (`teams_channels_enabled`) — Teams channels appear in the main sidebar below your chats; navigate with `j`/`k` and read messages just like chats. Supports background polling, global activity sorting (most active unhidden channels on top), unread indicators, and user-toggleable hidden channels (press `h` to toggle).

---

## Installation

### Prerequisites

- Go 1.22 or later
- A Microsoft account with access to Microsoft Teams

### Build

```bash
git clone https://github.com/nospor/teams-tui-go
cd teams-tui-go
go build -o teams-tui-go .

# or (builds slower, but binary is smaller)
go build -trimpath -ldflags="-s -w" -o teams-tui-go .

# then run
./teams-tui-go

# you may also want to copy the binary to your PATH (and run it from any place), e.g.:
sudo cp teams-tui-go /usr/local/bin/
```

### Install to PATH

```bash
go install github.com/nospor/teams-tui-go@latest
```

---

## Configuration

### Client ID (optional)

By default the app uses Microsoft's public Teams client ID. To use your own Azure AD app registration:

1. Follow the instructions in [AZURE_SETUP.md](AZURE_SETUP.md).
2. Set your client ID using one of:

   **Option A — environment variable:**
   ```bash
   cp .env.example .env
   # Edit .env and set CLIENT_ID=<your-client-id>
   ```

   **Option B — config file** (`~/.config/teams-tui-go/config.json`):
   ```json
   {
     "client_id": "your-client-id-here"
   }
   ```

### Notifications
- **Toggle Mode**: Cycle through notification modes at runtime by pressing `n`. The chosen mode is automatically saved.
- **Message Previews**: Configure desktop notifications in `~/.config/teams-tui-go/config.json`:

  ```json
  {
    "notification_mode": "System",
    "notification_show_preview": true,
    "notification_preview_len": 80
  }
  ```
  - `notification_show_preview`: Set to `true` to include the message content in the desktop notification.
  - `notification_preview_len`: The maximum number of characters to show in the preview.

### Message Limit
Configure how many messages to fetch when opening a chat in `~/.config/teams-tui-go/config.json`:

  ```json
  {
    "message_limit": 50
  }
  ```
  - `message_limit`: The number of messages to fetch (default: 50). For limits greater than 50, the app automatically makes sequential paginated requests. Capped at `200` to prevent excessive API requests.

### Chat Limit
Configure how many chats to load in the sidebar in `~/.config/teams-tui-go/config.json`:

  ```json
  {
    "chat_limit": 50
  }
  ```
  - `chat_limit`: The maximum number of chats to fetch and display (default: 50). Automatically makes paginated requests if needed. Capped at `100` to prevent API rate-limiting during member loading.

### Search Context Limit
Configure how many context messages (before and after each search match) to display in the search history popup in `~/.config/teams-tui-go/config.json`:

  ```json
  {
    "search_context_limit": 3
  }
  ```
  - `search_context_limit`: The number of context messages before and after each match to include (default: 3).

### Markdown Export Directory
Complete chat transcripts (from the chat actions popup) are written here:

  ```json
  {
    "export_directory": "~/Downloads"
  }
  ```
  - `export_directory`: Destination folder for Markdown exports (default: `"~/Downloads"`). `~` is expanded to the home directory.

### Channel Message Refresh
Configure background refresh rate for unhidden channels (in minutes) in `~/.config/teams-tui-go/config.json`:

  ```json
  {
    "channel_msg_refresh_min": 2
  }
  ```
  - `channel_msg_refresh_min`: The background polling interval in minutes for unhidden channels (default: 2).

### External Editor
Configure the external editor to open when pressing `Ctrl+g` in compose mode in `~/.config/teams-tui-go/config.json`:

  ```json
  {
    "external_editor": "vim"
  }
  ```
  - `external_editor`: The editor command or path to run (e.g. `"vim"`, `"neovim"`, `"nano"`). If empty/unspecified, it falls back to `$EDITOR`, then `$VISUAL`, and defaults to `"vim"`.

### URL Opening Commands
Configure the commands used to open URLs when pressing `o` on a message or from the URL selection menu in `~/.config/teams-tui-go/config.json`:

  ```json
  {
    "browser_command": "xdg-open",
    "youtrack_command": "yt-tui",
    "gitlab_command": "gitlab-tui"
  }
  ```
  - `browser_command`: The command used to open general URLs (default: `"xdg-open"`, but you can specify e.g. `"firefox"` or `"google-chrome"`). This key is always initialized in `config.json`.
  - `youtrack_command`: The optional command to open YouTrack URLs (default: `"yt-tui"`, but you can specify e.g. `"youtrack-cli"` or `"yt-cli"`). If a URL contains `"youtrack"`, this command is executed. Useful with tools like [yt-tui](https://github.com/nospor/yt-tui).
  - `gitlab_command`: The optional command to open GitLab URLs (default: `"gitlab-tui"`). If a URL contains `"gitlab"` (for example, merge requests, pipelines, or jobs), this command is executed. Useful with tools like [gitlab-tui](https://github.com/nospor/gitlab-tui).

### Image Viewer
Configure a dedicated image viewer used when pressing `Enter` on an image attachment (inside the `v` popup with `Tab` to enter attachment cursor mode).
When set, **all** image attachments from the current message are downloaded and loaded into the viewer together, automatically starting at the selected one.

  ```json
  {
    "image_viewer": "sxiv"
  }
  ```
  - `image_viewer`: The command or executable to open image attachments (e.g. `"sxiv"`, `"nsxiv"`, `"feh"`, `"imv"`, or any viewer). If empty or not set (the default), images fall back to the regular single-file opener (`xdg-open` / `open`). When set, pressing `Enter` on an image attachment downloads **all** image attachments in that message and loads them all into the viewer, automatically starting at the selected one. Viewer-specific start-index flags are handled automatically:
    - **`sxiv`** / **`nsxiv`**: uses `-n INDEX` (1-based).
    - **`feh`**: uses `--start-at PATH`.
    - **`imv`**: uses `-n PATH`.
    - **Other viewers**: the selected image is passed first in the argument list.

### Chat Icon Themes
You can configure the style of chat type indicators in the sidebar using `~/.config/teams-tui-go/config.json`:

```json
{
  "chat_icon_theme": "unicode"
}
```
- `chat_icon_theme`: Choose between presets (default: `"unicode"`):
  - `"unicode"`: Minimal single-width geometric icons (`◉` 1:1, `⊞` group, `⊛` meeting, `☰` channel).
  - `"emoji"`: Colorful emojis (`👤` 1:1, `👥` group, `📅` meeting, `#️⃣` channel).
  - `"text"`: The original bracketed text headers (`[oneOnOne]`, `[group]`, `[meeting]`, `[channel]`).

You can also completely override icons individually by defining a `"custom_chat_icons"` map:

```json
{
  "custom_chat_icons": {
    "oneOnOne": "💬",
    "group": "👥",
    "meeting": "⏱️",
    "channel": "📢",
    "default": "◈"
  }
}
```

### Optional Features

Each feature is disabled by default and requires an additional Graph API permission. Enable them in `~/.config/teams-tui-go/config.json` and **delete `~/.cache/teams-tui-go/token.json`** to force re-authentication with the new scopes:

```json
{
  "sqlite_enabled": false,
  "file_preview_enabled": true,
  "file_preview_in_terminal": false,
  "file_upload_enabled": false,
  "presence_enabled": true,
  "user_profile_enabled": true,
  "user_profile_extended": false,
  "teams_channels_enabled": false,
  "channel_mentions_enabled": false
}
```

| Config key | Default | Required permission | Effect |
|---|---|---|---|
| `sqlite_enabled` | `false` | - | Enables offline caching via SQLite (`~/.cache/teams-tui-go/teams-tui-go.db`). Instantly loads messages when entering chats/channels, syncing updates in the background. |
| `file_preview_enabled` | `false` | `Files.Read` | Tab through attachments in the `v` popup and press Enter to download to `~/Downloads/` |
| `file_preview_in_terminal` | `false` | `Files.Read` | Previews the highlighted image attachment inside the details popup on the right side using the Kitty Graphics Protocol (requires `file_preview_enabled: true`) |
| `file_upload_enabled` | `false` | `Files.ReadWrite` | Press `Ctrl+f` in compose mode to open a file browser and attach files under 4MB from the computer |
| `presence_enabled` | `false` | `Presence.Read.All` | Press `p` in message selection mode to see sender availability |
| `user_profile_enabled` | `false` | `User.ReadBasic.All` | Press `i` in message selection mode to view sender's profile |
| `user_profile_extended` | `false` | `User.Read.All` *(admin consent)* | Adds job title, department, office to the profile popup (requires `user_profile_enabled: true`) |
| `teams_channels_enabled` | `false` | `Team.ReadBasic.All` + `Channel.ReadBasic.All` + `ChannelMessage.Read.All` *(admin consent)* + `ChannelMessage.Send` + `ChannelMessage.ReadWrite` | Teams channels appear in the sidebar below chats; navigate with `j`/`k`. Supports background polling, activity sorting, unread dots, and hidden channels (`h` key). |
| `channel_mentions_enabled` | `false` | `TeamMember.Read.All` | Enables autocomplete suggestion dropdown list of team members in Teams channels when typing `@` mentions. |

See [AZURE_SETUP.md](AZURE_SETUP.md) for full permission setup instructions.

---

## Usage

```bash
# Run directly
./teams-tui-go

# Or if installed
teams-tui-go
```

On first run (or after token expiry) you will be prompted to visit a URL and enter a short code. Subsequent runs use the cached token (auto-refreshed).

---

## Markdown Formatting

Messages support a subset of markdown that is converted to rich HTML when sent, so recipients on **any Teams client** (Desktop, Web, Mobile) see proper formatting.

### Inline syntax

| Syntax                   | Result     |
| ------------------------ | ---------- |
| `**bold**` or `__bold__` | **Bold**   |
| `*italic*` or `_italic_` | *Italic*   |
| `~~strikethrough~~`      | ~~Strike~~ |
| `` `inline code` ``      | `code`     |

### Block syntax

| Syntax                        | Result                  |
| ----------------------------- | ----------------------- |
| `* item` or `- item`          | Unordered (bullet) list |
| `1. item` or `1) item`        | Ordered (numbered) list |
| ` ``` ` fence on its own line | Multi-line code block   |

**Example:**

````
**Meeting notes** for *Project X*

* Review PR #42
* Deploy to staging

```
fmt.Println("hello")
```
````

> **Note:** Language hints (e.g. ` ```go `) are accepted syntax but have no visible effect — Teams strips the `class` attribute from the stored HTML, so the hint is not preserved or displayed.

### Receive side rendering

Incoming messages from all clients are rendered with matching ANSI styles in the TUI:

- Bold, italic, and strikethrough use terminal text attributes
- Inline `code` is highlighted in amber
- Code blocks are highlighted in green
- Bullet and numbered lists show `•` / `1.` prefixes

### Edit round-trip

When you press `e` to edit an existing message the edit box is pre-filled with the **original markdown source** (e.g. `**bold**` rather than stripped plain text), so formatting is preserved after saving.

### Clipboard Image Pasting

When in compose mode (`i`), you can paste images (PNG/JPEG) directly from your system clipboard using **`Ctrl+V`**.
- A placeholder like `[Image 1]` will be inserted into the text field.
- You can move, copy, or delete this placeholder to control where the image appears in the sent message. If deleted, the image won't be sent.
- When the message is sent, the image is automatically base64-encoded and uploaded inline.

> [!NOTE]
> On Linux, `Ctrl+Shift+V` is intercepted by most terminal emulators to perform text-only paste. To paste clipboard images, make sure to use **`Ctrl+V`** instead, which is passed directly to the TUI.

### File Browsing & Uploading

When `file_upload_enabled` is set to `true` in `config.json`, you can attach small files (under 4MB) from your local computer to chat or channel messages.
- In compose mode (`i`), press **`Ctrl+f`** to open the offline file browser overlay.
- Navigate directories using `j`/`k` (or arrow keys) and enter directories with `Enter`. Move to parent directories via `..`.
- Press `.` to toggle the display of hidden files/folders (e.g. `.config`, `.cache`).
- Highlight a file and press **`Enter`** to select and attach it.
- A placeholder like `[File: filename.ext]` is inserted into the textarea. You can move, copy, or delete it to control inline message rendering.
- When sending the message, files are automatically uploaded to OneDrive (for chats) or SharePoint (for channels) and attached as reference attachments to the message.

> [!NOTE]
> **Inline file placement in this TUI vs. the official Teams client:** While composing, a `[File: filename.ext]` placeholder controls where the attachment appears in the message text. teams-tui-go preserves that placement when displaying sent messages (including in the external editor via **Ctrl+g**). The official Teams desktop and web clients, however, typically render reference file attachments as separate file cards at the bottom of the message bubble, regardless of where you inserted the placeholder. This is Teams platform behaviour — not a limitation of teams-tui-go. Clipboard images (**Ctrl+V**) behave differently and can appear truly inline in both clients.

### External Editor (Composing & Viewing)

You can use an external editor (such as `vim`, `neovim`, or `nano`) to either compose a new message or view an existing message:

- **Composing/Editing**: When in compose mode (`i`), press **`Ctrl+g`** to open the external editor. The current input text is saved to a temporary file and loaded into the editor. When you save and exit, the TUI loads the changes back into the compose field.
- **Viewing**: When in message mode (`m`) or message details popup (`v`), press **`Ctrl+g`** to open the selected message's full content in the external editor in read-only mode. Edits made in the editor are discarded when you exit, returning you directly to your previous position in the TUI.

The external editor command can be configured in your `config.json` via the `"external_editor"` option. If not specified, it falls back to the `$EDITOR` environment variable, then `$VISUAL` environment variable, and defaults to `"vim"`.

---

## Keyboard Controls

Defaults are vim-style. The `?` help popup and the hints on each panel use whatever you have configured.

Override keys in `~/.config/teams-tui-go/config.json` under `keybindings`. Each value is a list, so one action can be both `j` and `down`. Omit an action to keep its default. Setting an action **replaces** its list, so to add a key you must repeat the ones you still want. An empty list unbinds that action. `Ctrl+C` always quits from the main screen.

A name at the top level (`next`, `prev`, `page_down`, `page_up`, `close`, `quit`, `yank`, `yank_url`, `open_url`, `confirm`, `cancel`) applies to every mode that has that action. A block named after a mode replaces that action only there. Key names are Bubble Tea names: `j`, `K`, `up`, `down`, `pgup`, `pgdown`, `esc`, `enter`, `tab`, `ctrl+g`, `alt+enter`. `K` is shift+k.

```json
{
  "keybindings": {
    "next": ["down", "j"],
    "prev": ["up", "k"],
    "normal": {
      "compose": ["i", "enter"]
    },
    "message": {
      "delete": ["x"]
    }
  }
}
```

If two actions in the same mode are given the same key, the one you set explicitly wins over a default. If you set both, the earlier action in that mode keeps the key, and a warning is printed at startup.

### normal

| Action          | Default keys  | What it does                          |
| --------------- | ------------- | ------------------------------------- |
| `next`          | `j`, `down`   | Next chat or channel                  |
| `prev`          | `k`, `up`     | Previous chat or channel              |
| `section`       | `tab`         | Switch between chats and channels     |
| `page_down`     | `J`, `pgdown` | Scroll messages down                  |
| `page_up`       | `K`, `pgup`   | Scroll messages up                    |
| `notifications` | `n`           | Cycle notification mode               |
| `help`          | `?`           | Open help                             |
| `compose`       | `i`           | Compose                               |
| `chat_search`   | `c`           | Find or start a chat                  |
| `search`        | `/`           | Search history                        |
| `sleep`         | `esc`         | Clear highlights, or enter sleep mode |
| `messages`      | `m`           | Select a message                      |
| `favourite`     | `f`           | Toggle favourite                      |
| `actions`       | `a`           | Open chat actions popup               |
| `presence`      | `p`           | Chat presence                         |
| `channel_hide`  | `h`           | Hide or unhide a channel              |
| `quit`          | `q`           | Quit (`ctrl+c` always quits too)      |

### compose

Only these keys are commands. Everything else is typed into the message.

| Action | Default keys | What it does |
| --- | --- | --- |
| `cancel` | `esc` | Leave compose |
| `send` | `enter` | Send |
| `newline` | `alt+enter`, `shift+enter`, `ctrl+enter` | New line |
| `editor` | `ctrl+g` | External editor |
| `attach` | `ctrl+f` | Attach a file |
| `paste_image` | `ctrl+v`, `ctrl+shift+v` | Paste a clipboard image |

### mention

| Action | Default keys | What it does |
| --- | --- | --- |
| `cancel` | `esc` | Close suggestions |
| `prev` | `up`, `shift+tab` | Previous suggestion |
| `next` | `down`, `tab` | Next suggestion |
| `confirm` | `enter` | Insert the suggestion |

### message

| Action | Default keys | What it does |
| --- | --- | --- |
| `close` | `esc`, `m` | Leave selection |
| `next` | `j`, `down` | Newer message |
| `prev` | `k`, `up` | Older message |
| `react` | `r` | Open reactions |
| `yank` | `y` | Copy the message |
| `delete` | `d` | Delete your message |
| `editor` | `ctrl+g` | Open in the external editor |
| `edit` | `e` | Edit your message |
| `reply` | `a` | Reply |
| `yank_url` | `u` | Copy a URL |
| `open_url` | `o` | Open a URL |
| `view` | `v` | Open the message popup |
| `presence` | `p` | Sender presence |
| `profile` | `i` | Sender profile |

### message_view

| Action | Default keys | What it does |
| --- | --- | --- |
| `close` | `esc`, `q`, `v` | Close the popup |
| `editor` | `ctrl+g` | External editor |
| `confirm` | `enter` | Download and open the attachment, or close |
| `download` | `d` | Download without opening |
| `attachments` | `tab` | Toggle the attachment cursor |
| `next` | `j`, `down` | Next attachment or message |
| `prev` | `k`, `up` | Previous attachment or message |
| `page_down` | `J`, `shift+down`, `pgdown` | Scroll the body |
| `page_up` | `K`, `shift+up`, `pgup` | Scroll the body |

### reaction

| Action | Default keys | What it does |
| --- | --- | --- |
| `close` | `esc`, `r` | Close the picker |
| `like` | `1` | 👍 |
| `heart` | `2` | ❤️ |
| `laugh` | `3` | 😂 |
| `surprised` | `4` | 😮 |
| `sad` | `5` | 😢 |
| `angry` | `6` | 😡 |

### delete_confirm

| Action | Default keys | What it does |
| --- | --- | --- |
| `yes` | `y`, `Y` | Delete |
| `no` | `n`, `N`, `esc` | Cancel |

### url_list

| Action | Default keys | What it does |
| --- | --- | --- |
| `close` | `esc`, `q` | Close the list |
| `next` | `j`, `down` | Next URL |
| `prev` | `k`, `up` | Previous URL |
| `confirm` | `enter` | Copy or open the selected URL |
| `yank` | `y` | Copy |
| `open_url` | `o` | Open |

### search_input

| Action | Default keys | What it does |
| --- | --- | --- |
| `cancel` | `esc` | Close search |
| `submit` | `enter` | Run the query |

### search_results

| Action | Default keys | What it does |
| --- | --- | --- |
| `close` | `esc`, `q` | Close search |
| `next` | `j`, `down` | Next result |
| `prev` | `k`, `up` | Previous result |
| `edit_query` | `/` | Focus the query |
| `goto` | `g` | Jump to the message |
| `yank` | `y` | Copy the message |
| `expand` | `enter` | Show more context |
| `yank_url` | `u` | Copy a URL |
| `open_url` | `o` | Open a URL |

### chat_search_input

| Action | Default keys | What it does |
| --- | --- | --- |
| `cancel` | `esc` | Leave the field |
| `focus_results` | `down`, `up`, `tab` | Move into the result list |
| `submit` | `enter` | Open by email, or focus results |

### chat_search_results

| Action | Default keys | What it does |
| --- | --- | --- |
| `close` | `esc`, `q` | Close the popup |
| `next` | `j`, `down` | Next result |
| `prev` | `k`, `up` | Previous result |
| `edit_query` | `/` | Focus the field |
| `open` | `enter` | Open the selected chat |

### help, presence, profile

| Mode | Action | Default keys | What it does |
| --- | --- | --- | --- |
| `help` | `close` | `esc`, `q`, `?`, `enter` | Close help |
| `help` | `next` / `prev` | `j` `down` / `k` `up` | Scroll |
| `presence` | `close` | `esc`, `q`, `p`, `enter` | Close presence |
| `presence` | `next` / `prev` | `j` `down` / `k` `up` | Scroll the list |
| `profile` | `close` | `esc`, `q`, `i`, `enter` | Close the profile |

### filepicker

| Action | Default keys | What it does |
| --- | --- | --- |
| `next` | `j`, `down`, `ctrl+n` | Next entry |
| `prev` | `k`, `up`, `ctrl+p` | Previous entry |
| `page_down` | `J`, `pgdown` | Page down |
| `page_up` | `K`, `pgup` | Page up |
| `top` | `g` | First entry |
| `bottom` | `G` | Last entry |
| `back` | `h`, `backspace`, `left`, `esc` | Parent directory |
| `open` | `l`, `right`, `enter` | Enter a directory |
| `select` | `enter` | Choose the file |
| `sort` | `s`, `ctrl+s` | Change sort type |
| `sort_order` | `o`, `ctrl+o` | Toggle sort order |
| `hidden` | `.` | Toggle hidden files |
| `close` | `esc`, `q` | Cancel |

### chat_actions

Opened with `normal.actions` (`a`) on a selected chat. Export is only bound here, so `e` in the chat list and in message mode is unchanged.

| Action | Default keys | What it does |
| --- | --- | --- |
| `next` | `j`, `down` | Next action |
| `prev` | `k`, `up` | Previous action |
| `confirm` | `enter` | Run the highlighted action |
| `compose` | `i` | Compose a message |
| `favourite` | `f` | Toggle favourite |
| `export` | `e` | Export the complete chat as Markdown |
| `close` | `esc`, `q` | Close the popup |

---

## File Locations

| File                                          | Purpose                             |
| --------------------------------------------- | ----------------------------------- |
| `~/.config/teams-tui-go/config.json`           | Client ID, notification mode, limits, keybindings, export directory |
| `~/.config/teams-tui-go/favourites.json`       | Pinned/favourite chat IDs           |
| `~/.cache/teams-tui-go/token.json`             | OAuth2 access + refresh tokens      |
| `~/.cache/teams-tui-go/profile.json`           | Cached user profile                 |
| `~/.cache/teams-tui-go/teams-tui-go.db`       | SQLite database caching messages    |
| `~/Downloads/*.md`                             | Complete chat Markdown exports (default) |

---

## Development

```bash
# Run in development
go run .

# Build binary
go build -o teams-tui-go .

# Lint
go vet ./...
```

---

## License

See [LICENSE](LICENSE).

## Thanks For Visiting
Hope you liked it. Wanna **[buy Me a coffee](https://www.buymeacoffee.com/nospor)**?
