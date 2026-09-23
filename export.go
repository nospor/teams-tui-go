package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// GetAllChatMessages follows every Graph pagination link for a chat and
// returns a deduplicated, chronological transcript.
func GetAllChatMessages(accessToken, chatID string) ([]Message, error) {
	messages, next, err := GetMessages(accessToken, chatID, 50)
	if err != nil {
		return nil, err
	}
	return collectAllChatMessages(messages, next, func(link string) ([]Message, string, error) {
		return GetMessagesFromLink(accessToken, link)
	})
}

func collectAllChatMessages(
	messages []Message,
	next string,
	fetchNext func(string) ([]Message, string, error),
) ([]Message, error) {
	seenLinks := make(map[string]bool)
	for next != "" {
		if seenLinks[next] {
			return nil, fmt.Errorf("message pagination repeated a next link")
		}
		seenLinks[next] = true
		page, following, err := fetchNext(next)
		if err != nil {
			return nil, err
		}
		messages = append(messages, page...)
		next = following
	}

	byID := make(map[string]Message, len(messages))
	for _, message := range messages {
		if message.ID != "" {
			byID[message.ID] = message
		}
	}
	messages = messages[:0]
	for _, message := range byID {
		messages = append(messages, message)
	}
	sortMessagesOldestFirst(messages)
	return messages, nil
}

func sortMessagesOldestFirst(messages []Message) {
	sort.Slice(messages, func(i, j int) bool {
		return messages[i].CreatedDateTime < messages[j].CreatedDateTime
	})
}

func chatExportTitle(chat Chat) string {
	if chat.CachedDisplayName != nil && strings.TrimSpace(*chat.CachedDisplayName) != "" {
		return strings.TrimSpace(*chat.CachedDisplayName)
	}
	if chat.Topic != nil && strings.TrimSpace(*chat.Topic) != "" {
		return strings.TrimSpace(*chat.Topic)
	}
	return "Teams chat"
}

func markdownSender(message Message) string {
	if name := message.SenderName(); name != "" {
		return name
	}
	return "Teams"
}

func markdownMessageTime(value string) string {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return value
	}
	return parsed.Local().Format("2006-01-02 15:04 MST")
}

func isExportSkippedAttachment(att MessageAttachment) bool {
	if att.ContentType == nil {
		return false
	}
	ct := strings.ToLower(strings.TrimSpace(*att.ContentType))
	return ct == "messagereference" || ct == "forwardedmessagereference"
}

func markdownMessageReference(attachment MessageAttachment) string {
	if attachment.ContentType == nil || *attachment.ContentType != "messageReference" || attachment.Content == nil {
		return ""
	}
	var reference struct {
		MessagePreview string `json:"messagePreview"`
		MessageSender  struct {
			User *struct {
				DisplayName string `json:"displayName"`
			} `json:"user"`
		} `json:"messageSender"`
	}
	if err := json.Unmarshal([]byte(*attachment.Content), &reference); err != nil {
		return ""
	}
	preview := strings.TrimSpace(HTMLToMarkdown(reference.MessagePreview, nil))
	if preview == "" {
		return ""
	}
	sender := "message"
	if reference.MessageSender.User != nil && strings.TrimSpace(reference.MessageSender.User.DisplayName) != "" {
		sender = strings.TrimSpace(reference.MessageSender.User.DisplayName)
	}
	lines := strings.Split(preview, "\n")
	for index := range lines {
		lines[index] = "> " + lines[index]
	}
	return fmt.Sprintf("> In reply to **%s**:\n%s", sender, strings.Join(lines, "\n"))
}

func markdownMessageBody(message *Message) string {
	if message == nil {
		return ""
	}
	if message.IsSystemEvent() {
		if body := strings.TrimSpace(message.GetPlainText()); body != "" {
			return body
		}
		return "_[No text]_"
	}
	var sections []string
	for _, attachment := range message.Attachments {
		if reference := markdownMessageReference(attachment); reference != "" {
			sections = append(sections, reference)
		}
	}
	if message.Body != nil && message.Body.Content != nil {
		if body := strings.TrimSpace(HTMLToMarkdown(*message.Body.Content, message.Attachments)); body != "" {
			sections = append(sections, body)
		}
	}
	if len(sections) == 0 {
		if body := strings.TrimSpace(message.GetPlainText()); body != "" {
			sections = append(sections, body)
		}
	}
	if len(sections) == 0 {
		return "_[No text]_"
	}
	return strings.Join(sections, "\n\n")
}

func markdownAttachments(message Message) string {
	var lines []string
	for _, attachment := range message.Attachments {
		if isExportSkippedAttachment(attachment) {
			continue
		}
		name := "Attachment"
		if attachment.Name != nil && strings.TrimSpace(*attachment.Name) != "" {
			name = strings.TrimSpace(*attachment.Name)
		}
		if attachment.ContentURL != nil && strings.TrimSpace(*attachment.ContentURL) != "" {
			lines = append(lines, fmt.Sprintf("- [%s](%s)", name, strings.TrimSpace(*attachment.ContentURL)))
		} else {
			lines = append(lines, "- "+name)
		}
	}
	if len(lines) == 0 {
		return ""
	}
	return "**Attachments**\n\n" + strings.Join(lines, "\n")
}

func markdownReactions(message Message) string {
	if len(message.Reactions) == 0 {
		return ""
	}
	counts := make(map[string]int)
	for _, reaction := range message.Reactions {
		counts[reaction.ReactionType]++
	}
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		label := reactionEmoji(key)
		if label == "" {
			label = key
		}
		parts = append(parts, fmt.Sprintf("%s %d", label, counts[key]))
	}
	return "**Reactions:** " + strings.Join(parts, "  ")
}

// RenderChatMarkdown produces a portable, chronological Markdown transcript.
func RenderChatMarkdown(chat Chat, messages []Message, exportedAt time.Time) string {
	var out strings.Builder
	title := chatExportTitle(chat)
	fmt.Fprintf(&out, "# %s\n\n", title)
	fmt.Fprintf(&out, "- **Source:** Microsoft Teams\n")
	fmt.Fprintf(&out, "- **Chat type:** %s\n", chat.ChatType)
	fmt.Fprintf(&out, "- **Exported:** %s\n", exportedAt.Local().Format("2006-01-02 15:04 MST"))
	fmt.Fprintf(&out, "- **Messages:** %d\n", len(messages))
	out.WriteString("\n")
	out.WriteString("---\n\n")

	lastDay := ""
	for index := range messages {
		message := &messages[index]
		day := ""
		if parsed, err := time.Parse(time.RFC3339Nano, message.CreatedDateTime); err == nil {
			day = parsed.Local().Format("2006-01-02")
		}
		if day != "" && day != lastDay {
			fmt.Fprintf(&out, "## %s\n\n", day)
			lastDay = day
		}
		fmt.Fprintf(&out, "### %s - %s", markdownSender(*message), markdownMessageTime(message.CreatedDateTime))
		if message.Subject != "" {
			fmt.Fprintf(&out, " - %s", message.Subject)
		}
		out.WriteString("\n\n")
		out.WriteString(markdownMessageBody(message))
		out.WriteString("\n\n")
		if attachments := markdownAttachments(*message); attachments != "" {
			out.WriteString(attachments)
			out.WriteString("\n\n")
		}
		if reactions := markdownReactions(*message); reactions != "" {
			out.WriteString(reactions)
			out.WriteString("\n\n")
		}
		out.WriteString("---\n\n")
	}
	return out.String()
}

func expandExportDirectory(directory string) (string, error) {
	directory = strings.TrimSpace(directory)
	if directory == "" {
		directory = "~/Downloads"
	}
	if directory == "~" || strings.HasPrefix(directory, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if directory == "~" {
			directory = home
		} else {
			directory = filepath.Join(home, strings.TrimPrefix(directory, "~/"))
		}
	}
	return filepath.Clean(directory), nil
}

func exportFilename(title string, exportedAt time.Time) string {
	title = sanitizeFilename(strings.TrimSpace(title))
	title = strings.Join(strings.Fields(title), "-")
	if title == "" {
		title = "teams-chat"
	}
	return fmt.Sprintf("%s-%s.md", title, exportedAt.Local().Format("20060102-150405"))
}

// ExportChatMarkdown writes a full transcript with private file permissions.
func ExportChatMarkdown(directory string, chat Chat, messages []Message, exportedAt time.Time) (string, error) {
	directory, err := expandExportDirectory(directory)
	if err != nil {
		return "", fmt.Errorf("resolve export directory: %w", err)
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return "", fmt.Errorf("create export directory: %w", err)
	}
	path := filepath.Join(directory, exportFilename(chatExportTitle(chat), exportedAt))
	if err := os.WriteFile(path, []byte(RenderChatMarkdown(chat, messages, exportedAt)), 0o600); err != nil {
		return "", fmt.Errorf("write Markdown export: %w", err)
	}
	return path, nil
}
