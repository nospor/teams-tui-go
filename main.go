package main

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"time"

	"github.com/charmbracelet/bubbletea"
	"github.com/gen2brain/beeep"
)

// ---------------------------------------------------------------------------
// Bubble Tea commands (async → messages)
// ---------------------------------------------------------------------------

// tickCmd returns a command that fires MsgTick after 100ms.
func tickCmd() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg {
		return MsgTick{}
	})
}

// loadChatsCmd fetches the chat list in the background.
func loadChatsCmd(clientID string, existingChats []Chat, currentUserName *string) tea.Cmd {
	return func() tea.Msg {
		token, err := GetValidTokenSilent(clientID)
		if err != nil {
			return MsgChatsLoaded{Chats: existingChats, CurrentUserName: currentUserName}
		}
		chats, currentUser, err := GetChats(token, existingChats, currentUserName)
		if err != nil {
			return MsgChatsLoaded{Chats: existingChats, CurrentUserName: currentUserName}
		}
		return MsgChatsLoaded{Chats: chats, CurrentUserName: currentUser}
	}
}

// loadBackgroundMessagesCmd fetches the latest 10 messages for a chat to inspect reactions.
func loadBackgroundMessagesCmd(clientID, chatID string) tea.Cmd {
	return func() tea.Msg {
		token, err := GetValidTokenSilent(clientID)
		if err != nil {
			return nil
		}
		msgs, _, err := GetMessages(token, chatID, 10)
		if err != nil {
			return nil
		}
		return MsgBackgroundMessagesLoaded{ChatID: chatID, Messages: msgs}
	}
}

// pollReactionsCmd fetches the latest 10 messages for a list of chats in parallel.
func pollReactionsCmd(clientID string, chatIDs []string) tea.Cmd {
	return func() tea.Msg {
		token, err := GetValidTokenSilent(clientID)
		if err != nil {
			return nil
		}

		results := make(map[string][]Message)
		type fetchResult struct {
			chatID string
			msgs   []Message
		}
		ch := make(chan fetchResult, len(chatIDs))
		for _, id := range chatIDs {
			go func(chatID string) {
				msgs, _, err := GetMessages(token, chatID, 10)
				if err != nil {
					ch <- fetchResult{chatID, nil}
					return
				}
				ch <- fetchResult{chatID, msgs}
			}(id)
		}
		for range chatIDs {
			res := <-ch
			if res.msgs != nil {
				results[res.chatID] = res.msgs
			}
		}
		return MsgPollReactionsLoaded{Results: results}
	}
}

// loadMessagesCmd fetches messages for a specific chat in the background.
func loadMessagesCmd(clientID, chatID string, chatIndex int) tea.Cmd {
	return func() tea.Msg {
		token, err := GetValidTokenSilent(clientID)
		if err != nil {
			return nil
		}
		msgs, next, err := GetMessages(token, chatID, ResolveMessageLimit())
		if err != nil {
			return nil
		}
		return MsgMessagesLoaded{ChatIndex: chatIndex, Messages: msgs, NextLink: next}
	}
}

// loadMoreMessagesCmd fetches the next page of messages using a nextLink.
func loadMoreMessagesCmd(clientID, nextLink string, conversationID string, isSearch bool) tea.Cmd {
	return func() tea.Msg {
		token, err := GetValidTokenSilent(clientID)
		if err != nil {
			return nil
		}
		msgs, next, err := GetMessagesFromLink(token, nextLink)
		if err != nil {
			return nil
		}
		return MsgMoreMessagesLoaded{ConversationID: conversationID, Messages: msgs, NextLink: next, IsSearch: isSearch}
	}
}

// searchUsersCmd searches the directory for users by name or email.
func searchUsersCmd(clientID, query string) tea.Cmd {
	return func() tea.Msg {
		token, err := GetValidTokenSilent(clientID)
		if err != nil {
			return MsgUserSearchDone{Err: err}
		}
		users, err := SearchUsers(token, query)
		return MsgUserSearchDone{Users: users, Err: err}
	}
}

// loadHistoryFromDBCmd loads conversation history from the SQLite database asynchronously.
func loadHistoryFromDBCmd(convID string) tea.Cmd {
	return func() tea.Msg {
		dbMsgs, err := GetStoredMessages(convID, 10000)
		if err != nil {
			return MsgHistoryLoaded{ConversationID: convID}
		}
		nextLink, _ := GetNextLink(convID)
		return MsgHistoryLoaded{
			ConversationID: convID,
			Messages:       dbMsgs,
			NextLink:       nextLink,
		}
	}
}

// createChatCmd creates or retrieves a 1-on-1 chat with a user by UPN in the background.
func createChatCmd(clientID, myUserID, otherUPN string) tea.Cmd {
	return func() tea.Msg {
		token, err := GetValidTokenSilent(clientID)
		if err != nil {
			return MsgCreateChatDone{Err: err}
		}
		chat, err := GetOrCreateOneOnOneChat(token, myUserID, otherUPN)
		return MsgCreateChatDone{Chat: chat, Err: err}
	}
}

// sendMessageCmd sends a message to a chat in the background.
func sendMessageCmd(clientID, chatID, content string, members []ChatMember, images []PastedImage, files []PendingFile) tea.Cmd {
	return func() tea.Msg {
		token, err := GetValidTokenSilent(clientID)
		if err != nil {
			return MsgSendDone{Err: err}
		}
		err = SendMessage(token, chatID, content, members, images, files)
		return MsgSendDone{Err: err}
	}
}

// sendChannelMessageCmd sends a message to a Teams channel in the background.
func sendChannelMessageCmd(clientID, teamID, channelID, content string, members []ChatMember, images []PastedImage, files []PendingFile) tea.Cmd {
	return func() tea.Msg {
		token, err := GetValidTokenSilent(clientID)
		if err != nil {
			return MsgSendDone{Err: err}
		}
		err = SendChannelMessage(token, teamID, channelID, content, members, images, files)
		return MsgSendDone{Err: err}
	}
}

// sendChannelReplyCmd posts a reply into an existing Teams channel thread.
func sendChannelReplyCmd(clientID, teamID, channelID, rootMsgID, content string, members []ChatMember, images []PastedImage, files []PendingFile) tea.Cmd {
	return func() tea.Msg {
		token, err := GetValidTokenSilent(clientID)
		if err != nil {
			return MsgSendDone{Err: err}
		}
		err = SendChannelReply(token, teamID, channelID, rootMsgID, content, members, images, files)
		return MsgSendDone{Err: err}
	}
}

// sendMessageWithRefCmd sends a reply message with a Teams messageReference attachment.
func sendMessageWithRefCmd(clientID, chatID, content string, ref *Message, members []ChatMember, images []PastedImage, files []PendingFile) tea.Cmd {
	return func() tea.Msg {
		token, err := GetValidTokenSilent(clientID)
		if err != nil {
			return MsgSendDone{Err: err}
		}
		err = SendMessageWithReference(token, chatID, ref, content, members, images, files)
		return MsgSendDone{Err: err}
	}
}

// setReactionCmd adds a reaction to a chat message in the background.
func setReactionCmd(clientID, chatID, messageID, reactionType string) tea.Cmd {
	return func() tea.Msg {
		token, err := GetValidTokenSilent(clientID)
		if err != nil {
			return MsgSendDone{Err: err}
		}
		err = SetReaction(token, chatID, messageID, reactionType)
		return MsgSendDone{Err: err}
	}
}

// setChannelReactionCmd adds a reaction to a Teams channel message in the background.
func setChannelReactionCmd(clientID, teamID, channelID, messageID, reactionType string) tea.Cmd {
	return func() tea.Msg {
		token, err := GetValidTokenSilent(clientID)
		if err != nil {
			return MsgSendDone{Err: err}
		}
		err = SetChannelReaction(token, teamID, channelID, messageID, reactionType)
		return MsgSendDone{Err: err}
	}
}

// unsetReactionCmd removes a reaction from a chat message in the background.
func unsetReactionCmd(clientID, chatID, messageID, reactionType string) tea.Cmd {
	return func() tea.Msg {
		token, err := GetValidTokenSilent(clientID)
		if err != nil {
			return MsgSendDone{Err: err}
		}
		err = UnsetReaction(token, chatID, messageID, reactionType)
		return MsgSendDone{Err: err}
	}
}

// unsetChannelReactionCmd removes a reaction from a Teams channel message in the background.
func unsetChannelReactionCmd(clientID, teamID, channelID, messageID, reactionType string) tea.Cmd {
	return func() tea.Msg {
		token, err := GetValidTokenSilent(clientID)
		if err != nil {
			return MsgSendDone{Err: err}
		}
		err = UnsetChannelReaction(token, teamID, channelID, messageID, reactionType)
		return MsgSendDone{Err: err}
	}
}

// deleteMessageCmd removes a chat message in the background.
func deleteMessageCmd(clientID, chatID, messageID string) tea.Cmd {
	return func() tea.Msg {
		token, err := GetValidTokenSilent(clientID)
		if err != nil {
			return MsgSendDone{Err: err}
		}
		err = DeleteMessage(token, chatID, messageID)
		if err == nil && ResolveFeatureSqlite() {
			_ = UpdateStoredMessageBody(messageID, "*(deleted)*")
		}
		return MsgSendDone{Err: err}
	}
}

// deleteChannelMessageCmd removes a Teams channel message in the background.
func deleteChannelMessageCmd(clientID, teamID, channelID, messageID string) tea.Cmd {
	return func() tea.Msg {
		token, err := GetValidTokenSilent(clientID)
		if err != nil {
			return MsgSendDone{Err: err}
		}
		err = DeleteChannelMessage(token, teamID, channelID, messageID)
		if err == nil && ResolveFeatureSqlite() {
			_ = UpdateStoredMessageBody(messageID, "*(deleted)*")
		}
		return MsgSendDone{Err: err}
	}
}

// updateMessageCmd modifies a chat message in the background.
func updateMessageCmd(clientID, chatID, messageID, content string, members []ChatMember, images []PastedImage, files []PendingFile, existingRefAttachments []MessageAttachment, existingInlineImageURLs []string) tea.Cmd {
	return func() tea.Msg {
		token, err := GetValidTokenSilent(clientID)
		if err != nil {
			return MsgEditDone{ChatID: chatID, MessageID: messageID, Content: content, Members: members, Images: images, ExistingRefAttachments: existingRefAttachments, ExistingInlineImageURLs: existingInlineImageURLs, Err: err}
		}
		err = UpdateMessage(token, chatID, messageID, content, members, images, files, existingRefAttachments, existingInlineImageURLs)
		return MsgEditDone{ChatID: chatID, MessageID: messageID, Content: content, Members: members, Images: images, ExistingRefAttachments: existingRefAttachments, ExistingInlineImageURLs: existingInlineImageURLs, Err: err}
	}
}

// updateChannelMessageCmd modifies a Teams channel message in the background.
func updateChannelMessageCmd(clientID, teamID, channelID, messageID, content string, members []ChatMember, images []PastedImage, files []PendingFile, existingRefAttachments []MessageAttachment, existingInlineImageURLs []string) tea.Cmd {
	return func() tea.Msg {
		token, err := GetValidTokenSilent(clientID)
		if err != nil {
			return MsgEditDone{ChatID: channelID, MessageID: messageID, Content: content, Members: members, Images: images, ExistingRefAttachments: existingRefAttachments, ExistingInlineImageURLs: existingInlineImageURLs, Err: err}
		}
		err = UpdateChannelMessage(token, teamID, channelID, messageID, content, members, images, files, existingRefAttachments, existingInlineImageURLs)
		return MsgEditDone{ChatID: channelID, MessageID: messageID, Content: content, Members: members, Images: images, ExistingRefAttachments: existingRefAttachments, ExistingInlineImageURLs: existingInlineImageURLs, Err: err}
	}
}

// loadTeamMembersCmd fetches the members of a Team in the background.
// Requires TeamMember.Read.All scope; returns MsgTeamMembersLoaded.
func loadTeamMembersCmd(clientID, teamID string) tea.Cmd {
	return func() tea.Msg {
		token, err := GetValidTokenSilent(clientID)
		if err != nil {
			return MsgTeamMembersLoaded{TeamID: teamID, Err: err}
		}
		members := GetTeamMembers(token, teamID)
		return MsgTeamMembersLoaded{TeamID: teamID, Members: members}
	}
}

// loadPresenceCmd fetches the presence status for a user by their Azure AD user ID.
// Requires Presence.Read.All scope; returns MsgPresenceLoaded.
func loadPresenceCmd(clientID, userID, displayName string) tea.Cmd {
	return func() tea.Msg {
		token, err := GetValidTokenSilent(clientID)
		if err != nil {
			return MsgPresenceLoaded{UserID: userID, Err: err}
		}
		p, err := GetUserPresence(token, userID)
		return MsgPresenceLoaded{UserID: userID, Presence: p, Err: err}
	}
}

// loadChatPresenceCmd fetches the presence status for multiple users by their Azure AD user IDs.
// Requires Presence.Read.All scope; returns MsgChatPresenceLoaded.
func loadChatPresenceCmd(clientID string, userIDs []string) tea.Cmd {
	return func() tea.Msg {
		token, err := GetValidTokenSilent(clientID)
		if err != nil {
			return MsgChatPresenceLoaded{Err: err}
		}
		p, err := GetUsersPresence(token, userIDs)
		return MsgChatPresenceLoaded{Presences: p, Err: err}
	}
}

// loadUserProfileCmd fetches the full profile for a user by their Azure AD user ID.
// Requires User.ReadBasic.All (or User.Read.All for extended info); returns MsgUserProfileLoaded.
func loadUserProfileCmd(clientID, userID string) tea.Cmd {
	return func() tea.Msg {
		token, err := GetValidTokenSilent(clientID)
		if err != nil {
			return MsgUserProfileLoaded{UserID: userID, Err: err}
		}
		p, err := GetUserProfile(token, userID)
		return MsgUserProfileLoaded{UserID: userID, Profile: p, Err: err}
	}
}

// downloadFileCmd downloads a file attachment to destPath.
// Requires Files.Read scope; returns MsgFileDownloaded.
func downloadFileCmd(clientID, fileURL, destPath string, openAfterDownload bool) tea.Cmd {
	return func() tea.Msg {
		token, err := GetValidTokenSilent(clientID)
		if err != nil {
			return MsgFileDownloaded{Err: err}
		}
		err = DownloadFile(token, fileURL, destPath)
		return MsgFileDownloaded{DestPath: destPath, Err: err, OpenAfterDownload: openAfterDownload}
	}
}

// exportChatMarkdownCmd fetches every page for chat and writes a Markdown transcript.
func exportChatMarkdownCmd(clientID string, chat Chat, directory string) tea.Cmd {
	return func() tea.Msg {
		path, count, err := exportCompleteChatMarkdown(clientID, chat, directory)
		return MsgChatExported{Path: path, Count: count, Err: err}
	}
}

func exportCompleteChatMarkdown(clientID string, chat Chat, directory string) (string, int, error) {
	token, err := GetValidTokenSilent(clientID)
	if err != nil {
		return "", 0, err
	}
	messages, err := GetAllChatMessages(token, chat.ID)
	if err != nil {
		return "", 0, err
	}
	path, err := ExportChatMarkdown(directory, chat, messages, time.Now())
	return path, len(messages), err
}

// downloadAndOpenImagesCmd downloads all image attachments in a message and opens
// them together with the configured image viewer. selectedAtt is the attachment the
// user pressed Enter on; allAtts is the full list of viewable attachments for that
// message. imageViewer is the command string from config (e.g. "sxiv" or "feh").
// Supports sxiv/nsxiv (-n INDEX), feh (--start-at PATH), imv (-n PATH),
// and generic viewers (selected image put first in the args list).
func downloadAndOpenImagesCmd(clientID string, selectedAtt MessageAttachment, allAtts []MessageAttachment, imageViewer string) tea.Cmd {
	return func() tea.Msg {
		token, err := GetValidTokenSilent(clientID)
		if err != nil {
			return MsgImagesOpened{Err: err}
		}

		// Collect only the image attachments from allAtts.
		var imageAtts []MessageAttachment
		selectedImageIndex := -1
		for _, a := range allAtts {
			if isImageAttachment(a) {
				if a.ID == selectedAtt.ID {
					selectedImageIndex = len(imageAtts)
				}
				imageAtts = append(imageAtts, a)
			}
		}
		// Fallback: if selected wasn't found (e.g. duplicate IDs), treat it as sole image.
		if selectedImageIndex == -1 {
			selectedImageIndex = len(imageAtts)
			imageAtts = append(imageAtts, selectedAtt)
		}

		// Download all images to the downloads dir.
		downloadsDir := getDownloadsDir()
		var savedPaths []string
		var selectedPath string
		for i, img := range imageAtts {
			name := getAttachmentSavedName(img, "image")
			path := filepath.Join(downloadsDir, name)

			// Download only if the URL is present.
			if img.ContentURL != nil && *img.ContentURL != "" {
				if dlErr := DownloadFile(token, *img.ContentURL, path); dlErr != nil {
					if i == selectedImageIndex {
						// Cannot download the selected image — abort with error.
						return MsgImagesOpened{Err: dlErr}
					}
					// Non-selected image failed — skip it silently.
					continue
				}
			}
			savedPaths = append(savedPaths, path)
			if i == selectedImageIndex {
				selectedPath = path
			}
		}

		if len(savedPaths) == 0 {
			return MsgImagesOpened{Err: fmt.Errorf("no images could be downloaded")}
		}

		// Recalculate selected index within the actually saved paths.
		newSelectedIdx := -1
		for idx, p := range savedPaths {
			if p == selectedPath {
				newSelectedIdx = idx
				break
			}
		}

		// Build the viewer command args.
		parts := strings.Fields(imageViewer)
		if len(parts) == 0 {
			return MsgImagesOpened{Err: fmt.Errorf("empty image_viewer command")}
		}
		viewerExe := parts[0]
		viewerBase := filepath.Base(viewerExe)
		extraArgs := parts[1:]

		var args []string
		switch viewerBase {
		case "sxiv", "nsxiv":
			// sxiv/nsxiv: -n INDEX (1-based) to start at a specific image.
			if newSelectedIdx != -1 {
				args = append(extraArgs, "-n", fmt.Sprintf("%d", newSelectedIdx+1))
			} else {
				args = extraArgs
			}
			args = append(args, savedPaths...)
		case "feh":
			// feh: --start-at PATH to start at a specific file.
			if selectedPath != "" {
				args = append(extraArgs, "--start-at", selectedPath)
			} else {
				args = extraArgs
			}
			args = append(args, savedPaths...)
		case "imv":
			// imv: -n PATH to start at a specific file.
			if selectedPath != "" {
				args = append(extraArgs, "-n", selectedPath)
			} else {
				args = extraArgs
			}
			args = append(args, savedPaths...)
		default:
			// Generic viewer: put the selected image first, rest follow.
			var reordered []string
			if selectedPath != "" {
				reordered = append(reordered, selectedPath)
			}
			for idx, p := range savedPaths {
				if idx != newSelectedIdx {
					reordered = append(reordered, p)
				}
			}
			args = append(extraArgs, reordered...)
		}

		cmd := exec.Command(viewerExe, args...)
		if startErr := cmd.Start(); startErr != nil {
			return MsgImagesOpened{Err: startErr}
		}
		return MsgImagesOpened{SelectedPath: selectedPath, TotalImages: len(savedPaths)}
	}
}

// loadTeamsChannelsCmd fetches the list of joined Teams with their channels.
// Requires Team.ReadBasic.All + Channel.ReadBasic.All scopes; returns MsgTeamsChannelsLoaded.
func loadTeamsChannelsCmd(clientID string) tea.Cmd {
	return func() tea.Msg {
		token, err := GetValidTokenSilent(clientID)
		if err != nil {
			return MsgTeamsChannelsLoaded{Err: err}
		}
		teams, err := GetTeamsWithChannels(token)
		return MsgTeamsChannelsLoaded{Teams: teams, Err: err}
	}
}

// loadChannelMessagesCmd fetches messages for a specific Teams channel.
// Requires ChannelMessage.Read.All scope; returns MsgChannelMessagesLoaded.
func loadChannelMessagesCmd(clientID, teamID, channelID string) tea.Cmd {
	return func() tea.Msg {
		token, err := GetValidTokenSilent(clientID)
		if err != nil {
			return MsgChannelMessagesLoaded{TeamID: teamID, ChannelID: channelID, Err: err}
		}
		msgs, next, err := GetChannelMessages(token, teamID, channelID, ResolveMessageLimit())
		return MsgChannelMessagesLoaded{TeamID: teamID, ChannelID: channelID, Messages: msgs, NextLink: next, Err: err}
	}
}

// openExternalEditorCmd launches an external editor with the current content,
// allowing the user to edit/compose or view a message, and returns MsgEditorFinished on exit.
func openExternalEditorCmd(currentText, editorCmd string, readOnly bool) tea.Cmd {
	tmpFile, err := os.CreateTemp("", "teams-tui-msg-*.txt")
	if err != nil {
		return func() tea.Msg {
			return MsgEditorFinished{Err: fmt.Errorf("could not create temporary file: %w", err)}
		}
	}
	tmpPath := tmpFile.Name()

	if _, err := tmpFile.WriteString(currentText); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return func() tea.Msg {
			return MsgEditorFinished{Err: fmt.Errorf("could not write to temporary file: %w", err)}
		}
	}
	tmpFile.Close()

	c := exec.Command(editorCmd, tmpPath)
	return tea.ExecProcess(c, func(err error) tea.Msg {
		defer os.Remove(tmpPath)
		if err != nil {
			return MsgEditorFinished{Err: fmt.Errorf("editor failed: %w", err)}
		}
		if readOnly {
			return MsgEditorFinished{
				ReadOnly: true,
			}
		}
		contentBytes, err := os.ReadFile(tmpPath)
		if err != nil {
			return MsgEditorFinished{Err: fmt.Errorf("could not read file: %w", err)}
		}
		return MsgEditorFinished{
			Content:  string(contentBytes),
			ReadOnly: false,
		}
	})
}

// openURLCmd launches a command to open the given URL, suspending the TUI if needed.
func openURLCmd(url, browserCmd, youtrackCmd, gitlabCmd string) tea.Cmd {
	lowerURL := strings.ToLower(url)
	var cmdStr string
	if strings.Contains(lowerURL, "youtrack") && youtrackCmd != "" {
		cmdStr = youtrackCmd
	} else if strings.Contains(lowerURL, "gitlab") && gitlabCmd != "" {
		cmdStr = gitlabCmd
	} else {
		cmdStr = browserCmd
	}

	fields := strings.Fields(cmdStr)
	if len(fields) == 0 {
		return func() tea.Msg {
			return MsgURLOpened{Err: fmt.Errorf("empty command configured")}
		}
	}

	cmdName := fields[0]
	var args []string
	if len(fields) > 1 {
		args = append(args, fields[1:]...)
	}
	args = append(args, url)

	c := exec.Command(cmdName, args...)
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return MsgURLOpened{Err: err}
	})
}

// attachFileFromFilepathCmd asynchronously reads a file from disk, detects its content type,
// and returns MsgFileAttached.
func attachFileFromFilepathCmd(path string) tea.Cmd {
	return func() tea.Msg {
		data, err := os.ReadFile(path)
		if err != nil {
			return MsgFileAttached{Err: err}
		}

		filename := filepath.Base(path)
		contentType := http.DetectContentType(data)

		// DetectContentType can be generic; map common extension overrides for accuracy
		ext := strings.ToLower(filepath.Ext(filename))
		switch ext {
		case ".png":
			contentType = "image/png"
		case ".jpg", ".jpeg":
			contentType = "image/jpeg"
		case ".gif":
			contentType = "image/gif"
		}

		return MsgFileAttached{
			Name:        filename,
			ContentType: contentType,
			Data:        data,
		}
	}
}

// ---------------------------------------------------------------------------
// Desktop notification
// ---------------------------------------------------------------------------

// sendDesktopNotification sends a native desktop notification.
func sendDesktopNotification(senderName string, body string) {
	title := "TeamsTUI: New Message"
	if senderName != "" {
		title = "TeamsTUI: " + senderName
	}

	finalBody := "New message received"
	if body != "" {
		finalBody = body
	}

	beeep.AppName = "TeamsTUI"
	_ = beeep.Notify(title, finalBody, "")
}

// ---------------------------------------------------------------------------
// Initial chat sort by most recent message
// ---------------------------------------------------------------------------

// chatTimestamp holds a chat together with the most recent message timestamp
// observed during the initial load.
type chatTimestamp struct {
	chat      Chat
	latestMsg time.Time
}

// loadInitialChatOrder returns the chats sorted by most recent message timestamp (descending),
// using the pre-fetched LastMessagePreview field, along with the last message IDs and timestamps.
func loadInitialChatOrder(chats []Chat) ([]Chat, map[string]string, map[string]time.Time) {
	lastMsgIDs := make(map[string]string)
	lastMsgTimes := make(map[string]time.Time)

	type chatWithTime struct {
		chat Chat
		t    time.Time
	}
	combined := make([]chatWithTime, len(chats))
	for i, c := range chats {
		t := time.Time{}
		if c.LastMessagePreview != nil {
			t, _ = time.Parse(time.RFC3339Nano, c.LastMessagePreview.CreatedDateTime)
			lastMsgIDs[c.ID] = c.LastMessagePreview.ID
			lastMsgTimes[c.ID] = t
		} else if c.LastUpdated != nil {
			lut, _ := time.Parse(time.RFC3339Nano, *c.LastUpdated)
			t = lut
			lastMsgTimes[c.ID] = t
		}
		combined[i] = chatWithTime{c, t}
	}

	// Sort by latest message timestamp descending.
	sort.Slice(combined, func(a, b int) bool {
		ta := combined[a].t
		tb := combined[b].t
		if ta.IsZero() && tb.IsZero() {
			return false
		}
		if ta.IsZero() {
			return false
		}
		if tb.IsZero() {
			return true
		}
		return ta.After(tb)
	})

	sorted := make([]Chat, len(chats))
	for i, cw := range combined {
		sorted[i] = cw.chat
	}

	return sorted, lastMsgIDs, lastMsgTimes
}

// ---------------------------------------------------------------------------
// main
// ---------------------------------------------------------------------------

var version = "dev"

func main() {
	if len(os.Args) >= 3 && os.Args[1] == "preview-image" {
		previewImage(os.Args[2])
		os.Exit(0)
	}

	// Set default HTTP client timeout to prevent background refreshes from hanging indefinitely.
	http.DefaultClient.Timeout = 15 * time.Second

	// 1. Banner.
	fmt.Printf("TeamsTUI %s\n", version)
	fmt.Println("================================")

	// Initialize configuration and write defaults for any missing keys.
	InitConfig()

	// 2. Resolve client ID and authenticate.
	clientID := ResolveClientID()
	accessToken, err := GetAccessToken(clientID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "authentication error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✓ Authentication successful!")

	// 3. Fetch user profile.
	me, err := GetMe(accessToken)
	if err != nil {
		fmt.Fprintf(os.Stderr, "could not fetch profile: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✓ Logged in as: %s\n", me.DisplayName)

	// 4. Fetch initial chat list.
	chats, currentUserName, err := GetChats(accessToken, nil, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "could not fetch chats: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✓ Loaded %d chats\n", len(chats))

	// 5 & 6. Sort chats by most recent message.
	var lastMsgIDs map[string]string
	var lastMsgTimes map[string]time.Time
	chats, lastMsgIDs, lastMsgTimes = loadInitialChatOrder(chats)

	// 7 & 8. Initialise application state.
	app := NewApp()
	app.SetChats(chats)
	if currentUserName != nil {
		app.SetCurrentUser(*currentUserName)
	}
	app.CurrentUserID = me.ID

	// Load persisted notification mode and settings.
	app.ChannelMsgRefreshMin = ResolveChannelMsgRefreshMin()
	app.ExternalEditor = ResolveExternalEditor()
	app.BrowserCommand = ResolveBrowserCommand()
	app.ImageViewer = ResolveImageViewer()
	app.YoutrackCommand = ResolveYoutrackCommand()
	app.GitlabCommand = ResolveGitlabCommand()
	app.ExportDirectory = ResolveExportDirectory()
	if cfg := LoadConfig(); cfg != nil {
		if cfg.NotificationMode != nil {
			app.NotificationMode = *cfg.NotificationMode
		}
		if cfg.NotificationShowPreview != nil {
			app.NotificationShowPreview = *cfg.NotificationShowPreview
		}
		if cfg.NotificationPreviewLen != nil {
			app.NotificationPreviewLen = *cfg.NotificationPreviewLen
		}
		if cfg.ChatIconTheme != nil {
			app.ChatIconTheme = *cfg.ChatIconTheme
		}
		if cfg.CustomChatIcons != nil {
			app.CustomChatIcons = cfg.CustomChatIcons
		}
		if len(cfg.Keybindings) > 0 {
			km, warns := ResolveKeyMap(cfg.Keybindings)
			app.Keys = km
			for _, w := range warns {
				fmt.Printf("⚠️ %s\n", w)
			}
			if len(warns) > 0 {
				app.SetStatus(warns[0], 8*time.Second)
			}
		}
	}

	// Resolve optional feature flags once at startup.
	app.Features = FeatureFlags{
		FilePreview:           ResolveFeatureFilePreview(),
		FilePreviewInTerminal: ResolveFeatureFilePreview() && ResolveFeatureFilePreviewInTerminal(),
		FileUpload:            ResolveFeatureFileUpload(),
		Presence:              ResolveFeaturePresence(),
		UserProfile:           ResolveFeatureUserProfile(),
		ProfileExtended:       ResolveFeatureUserProfileExtended(),
		TeamsChannels:         ResolveFeatureTeamsChannels(),
		ChannelMentions:       ResolveFeatureChannelMentions(),
		SqliteEnabled:         ResolveFeatureSqlite(),
	}

	if app.Features.SqliteEnabled {
		if err := InitDB(); err != nil {
			fmt.Printf("⚠️ Warning: Could not initialize SQLite database: %v\n", err)
		} else {
			defer CloseDB()
		}
	}

	// Build initial stable chat order.
	model := NewModel(app, clientID, me.ID)
	// Chats are already loaded synchronously above; set lastChatRefresh so the
	// first tick-driven background refresh fires ~15 s from now rather than
	// immediately (Init() no longer fires a redundant loadChatsCmd).
	model.lastChatRefresh = time.Now()
	model.lastChannelRefresh = time.Now()
	model.latestChats = chats
	model.lastMsgID = lastMsgIDs
	model.lastMsgTime = lastMsgTimes
	model.lastReadReactions = make(map[string]map[string]bool)
	model.reactionsInitialized = make(map[string]bool)
	model.notifiedReactions = make(map[string]map[string]bool)
	for _, c := range chats {
		model.lastReadReactions[c.ID] = make(map[string]bool)
		model.notifiedReactions[c.ID] = make(map[string]bool)
		for _, rKey := range model.getReactionKeys(c.LastMessagePreview) {
			model.lastReadReactions[c.ID][rKey] = true
		}
	}
	stableOrder := make([]string, len(chats))
	for i, c := range chats {
		stableOrder[i] = c.ID
	}
	model.stableChatOrder = stableOrder

	// Load persisted favourites and apply them so favourites appear at the top on launch.
	model.favourites = LoadFavourites()
	model.unhiddenChannels = LoadUnhiddenChannels()

	// Fetch any favourited chats that weren't returned by the regular API call
	// (e.g. chats with very old activity that fell outside chat_limit).
	// We do this concurrently to keep startup fast.
	if len(model.favourites) > 0 {
		loadedIDs := make(map[string]bool, len(chats))
		for _, c := range chats {
			loadedIDs[c.ID] = true
		}

		type fetchResult struct {
			chat *Chat
		}
		missingIDs := make([]string, 0)
		for id := range model.favourites {
			if !loadedIDs[id] {
				missingIDs = append(missingIDs, id)
			}
		}

		if len(missingIDs) > 0 {
			fmt.Printf("⭐ Fetching %d favourited chat(s) not in recent activity...\n", len(missingIDs))
			ch := make(chan fetchResult, len(missingIDs))
			for _, id := range missingIDs {
				go func(chatID string) {
					c, err := GetChat(accessToken, chatID, currentUserName)
					if err != nil {
						ch <- fetchResult{nil}
						return
					}
					ch <- fetchResult{c}
				}(id)
			}
			for range missingIDs {
				res := <-ch
				if res.chat == nil {
					continue
				}
				c := *res.chat
				model.latestChats = append(model.latestChats, c)
				model.stableChatOrder = append(model.stableChatOrder, c.ID)
				model.lastReadReactions[c.ID] = make(map[string]bool)
				model.notifiedReactions[c.ID] = make(map[string]bool)
				if c.LastMessagePreview != nil {
					model.lastMsgID[c.ID] = c.LastMessagePreview.ID
					t, _ := time.Parse(time.RFC3339Nano, c.LastMessagePreview.CreatedDateTime)
					model.lastMsgTime[c.ID] = t
					for _, rKey := range model.getReactionKeys(c.LastMessagePreview) {
						model.lastReadReactions[c.ID][rKey] = true
					}
				}
			}
		}
	}

	model = model.rebuildChatList()
	model = model.writeAppState()

	// 9. Start Bubble Tea program.
	p := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "TUI error: %v\n", err)
		os.Exit(1)
	}
}
