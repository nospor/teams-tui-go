package main

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	stdhtml "html"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
	"golang.org/x/net/html"
)

const graphAPIBase = "https://graph.microsoft.com/v1.0"
const graphAPIBeta = "https://graph.microsoft.com/beta"

// ---------------------------------------------------------------------------
// Data models
// ---------------------------------------------------------------------------

// ChatMember represents a participant in a chat.
type ChatMember struct {
	ID          *string `json:"id,omitempty"`
	UserID      *string `json:"userId,omitempty"`
	DisplayName *string `json:"displayName,omitempty"`
	Email       *string `json:"email,omitempty"`
}

// Chat represents a Microsoft Teams chat.
type Chat struct {
	ID                 string         `json:"id"`
	Topic              *string        `json:"topic,omitempty"`
	ChatType           string         `json:"chatType"`
	LastUpdated        *string        `json:"lastUpdatedDateTime,omitempty"`
	Viewpoint          *ChatViewpoint `json:"viewpoint,omitempty"`
	LastMessagePreview *Message       `json:"lastMessagePreview,omitempty"`
	Members            []ChatMember   `json:"-"` // populated separately
	CachedDisplayName  *string        `json:"-"` // computed, never from API
}

// ChatViewpoint contains the read state for the current user.
type ChatViewpoint struct {
	LastMessageReadDateTime string `json:"lastMessageReadDateTime"`
}

// Message represents a single message in a chat.
type Message struct {
	ID              string              `json:"id"`
	CreatedDateTime string              `json:"createdDateTime"`
	MessageType     string              `json:"messageType,omitempty"`
	Subject         string              `json:"subject,omitempty"`
	From            *MessageFrom        `json:"from,omitempty"`
	Body            *MessageBody        `json:"body,omitempty"`
	Attachments     []MessageAttachment `json:"attachments,omitempty"`
	Reactions       []MessageReaction   `json:"reactions,omitempty"`
	Mentions        []MessageMention    `json:"mentions,omitempty"`
	PlainTextCached         *string             `json:"-"`
	NormalizedTextCached    *string             `json:"-"`
	NormalizedSubjectCached *string             `json:"-"`
	WrappedWidthCached      int                 `json:"-"`
	WrappedQueryCached      string              `json:"-"`
	WrappedLinesCached      []string            `json:"-"`
	// IsReply is set in-process (not from JSON) for Teams channel thread replies.
	IsReply   bool   `json:"-"`
	ReplyToID string `json:"-"` // ID of the root message this is a reply to
}

type MessageMention struct {
	ID          *int                  `json:"id,omitempty"`
	MentionText *string               `json:"mentionText,omitempty"`
	Mentioned   *MentionedIdentitySet `json:"mentioned,omitempty"`
}

type MentionedIdentitySet struct {
	User *MessageUser `json:"user,omitempty"`
}

// GetPlainText returns the cached plain text of the message, parsing HTML on demand once.
func (msg *Message) GetPlainText() string {
	msg.ProcessInlineImages()
	if msg.PlainTextCached != nil {
		return *msg.PlainTextCached
	}
	if msg.Body == nil || msg.Body.Content == nil {
		empty := ""
		msg.PlainTextCached = &empty
		return empty
	}
	if *msg.Body.Content == "<systemEventMessage/>" {
		text := "── [system event] ──"
		msg.PlainTextCached = &text
		return text
	}
	text := HTMLToText(*msg.Body.Content, msg.Attachments, msg.Mentions)
	msg.PlainTextCached = &text
	return text
}

// GetNormalizedText returns the cached normalized plain text (lowercased, accents removed).
func (msg *Message) GetNormalizedText() string {
	if msg.NormalizedTextCached != nil {
		return *msg.NormalizedTextCached
	}
	rawText := msg.GetPlainText()
	normText := normalizeString(strings.ToLower(rawText))
	msg.NormalizedTextCached = &normText
	return normText
}

// GetNormalizedSubject returns the cached normalized subject (lowercased, accents removed).
func (msg *Message) GetNormalizedSubject() string {
	if msg.NormalizedSubjectCached != nil {
		return *msg.NormalizedSubjectCached
	}
	normSubj := normalizeString(strings.ToLower(msg.Subject))
	msg.NormalizedSubjectCached = &normSubj
	return normSubj
}

func hasExtension(s string) bool {
	ext := filepath.Ext(s)
	return ext != "" && len(ext) <= 5
}

func sanitizeFilename(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || r == ':' || r == '*' || r == '?' || r == '"' || r == '<' || r == '>' || r == '|' || r == '\x00' {
			return '_'
		}
		return r
	}, s)
}

// getAttachmentSavedName returns a safe, collision-resistant filename for saving or downloading
// an attachment, embedding the attachment's ID or a unique hash to prevent files (like pasted "image.png")
// from overwriting each other.
func getAttachmentSavedName(att MessageAttachment, defaultName string) string {
	name := defaultName
	if name == "" {
		name = "attachment"
	}
	if att.Name != nil && *att.Name != "" {
		name = *att.Name
	}

	// Determine unique ID string to append
	var idStr string
	if att.ID != "" {
		idStr = sanitizeFilename(att.ID)
		idStr = strings.TrimSpace(idStr)
	} else if att.ContentURL != nil && *att.ContentURL != "" {
		h := sha256.Sum256([]byte(*att.ContentURL))
		idStr = hex.EncodeToString(h[:])[:8]
	}

	if len(idStr) > 24 {
		idStr = idStr[:24]
	}
	idStr = strings.Trim(idStr, "_- ")

	if idStr == "" {
		return name
	}

	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	if stem == "" {
		stem = defaultName
		if stem == "" {
			stem = "attachment"
		}
	}

	if strings.HasSuffix(stem, "_"+idStr) || strings.HasSuffix(stem, "-"+idStr) {
		return stem + ext
	}

	return fmt.Sprintf("%s_%s%s", stem, idStr, ext)
}


func ExtractInlineImages(htmlContent string) []MessageAttachment {
	if htmlContent == "" {
		return nil
	}
	var list []MessageAttachment
	tokenizer := html.NewTokenizer(strings.NewReader(htmlContent))
	imgCounter := 0
	for {
		tt := tokenizer.Next()
		if tt == html.ErrorToken {
			break
		}
		if tt == html.StartTagToken || tt == html.SelfClosingTagToken {
			token := tokenizer.Token()
			if token.Data == "img" {
				var src, alt string
				for _, attr := range token.Attr {
					if attr.Key == "src" {
						src = attr.Val
					} else if attr.Key == "alt" {
						alt = attr.Val
					}
				}
				if src != "" {
					imgCounter++
					name := alt
					if name == "" {
						name = fmt.Sprintf("inline-image-%d", imgCounter)
					} else {
						name = sanitizeFilename(name)
					}
					if !hasExtension(name) {
						name += ".png"
					}
					contentType := "image/png"
					
					srcCopy := src
					nameCopy := name
					contentTypeCopy := contentType
					
					list = append(list, MessageAttachment{
						ID:          fmt.Sprintf("inline-img-%d", imgCounter),
						Name:        &nameCopy,
						ContentType: &contentTypeCopy,
						ContentURL:  &srcCopy,
					})
				}
			}
		}
	}
	return list
}

func (msg *Message) ProcessInlineImages() {
	if msg.Body == nil || msg.Body.Content == nil {
		return
	}
	inlineAtts := ExtractInlineImages(*msg.Body.Content)
	if len(inlineAtts) > 0 {
		seen := make(map[string]bool)
		for _, a := range msg.Attachments {
			seen[a.ID] = true
		}
		for _, a := range inlineAtts {
			if !seen[a.ID] {
				msg.Attachments = append(msg.Attachments, a)
				seen[a.ID] = true
			}
		}
	}
}

// FilterMessageAttachments removes unwanted rich card and URL-preview attachments
// that are not downloadable files.
func FilterMessageAttachments(msg *Message) {
	if msg == nil {
		return
	}
	var filtered []MessageAttachment
	for _, att := range msg.Attachments {
		if att.ContentType != nil {
			ct := strings.ToLower(*att.ContentType)
			// Ignore rich card attachments
			if strings.Contains(ct, "card") {
				continue
			}
			// Ignore reference attachments that do not point to SharePoint/OneDrive
			if ct == "reference" {
				if att.ContentURL != nil && !isSharePointURL(*att.ContentURL) {
					continue
				}
			}
		}
		filtered = append(filtered, att)
	}
	msg.Attachments = filtered
}

// FilterMessagesAttachments applies FilterMessageAttachments to a slice of messages.
func FilterMessagesAttachments(msgs []Message) {
	for i := range msgs {
		FilterMessageAttachments(&msgs[i])
	}
}

// MessageReaction represents a reaction to a message.
type MessageReaction struct {
	ReactionType    string       `json:"reactionType"`
	CreatedDateTime *string      `json:"createdDateTime,omitempty"`
	User            *MessageFrom `json:"user,omitempty"`
}

// MessageAttachment is a file or card attached to a message.
type MessageAttachment struct {
	ID          string  `json:"id"`
	Name        *string `json:"name,omitempty"`
	ContentType *string `json:"contentType,omitempty"`
	Content     *string `json:"content,omitempty"`
	ContentURL  *string `json:"contentUrl,omitempty"`
}

// MessageFrom holds the sender information.
type MessageFrom struct {
	User *MessageUser `json:"user,omitempty"`
}

// MessageUser holds the sender display name and ID.
type MessageUser struct {
	ID          *string `json:"id,omitempty"`
	DisplayName *string `json:"displayName,omitempty"`
}

// MessageBody holds the message content (HTML).
type MessageBody struct {
	Content *string `json:"content,omitempty"`
}

// User represents the authenticated Microsoft account user.
type User struct {
	DisplayName       string  `json:"displayName"`
	ID                string  `json:"id"`
	UserPrincipalName *string `json:"userPrincipalName,omitempty"`
}

// ---------------------------------------------------------------------------
// API response wrappers
// ---------------------------------------------------------------------------

type chatsResponse struct {
	Value    []Chat  `json:"value"`
	NextLink *string `json:"@odata.nextLink,omitempty"`
}

type membersResponse struct {
	Value []ChatMember `json:"value"`
}

type messagesResponse struct {
	Value    []Message `json:"value"`
	NextLink *string   `json:"@odata.nextLink,omitempty"`
}

type orgResponse struct {
	Value []struct {
		ID string `json:"id"`
	} `json:"value"`
}

// ---------------------------------------------------------------------------
// HTTP helper
// ---------------------------------------------------------------------------

// graphGet performs an authenticated GET request against the Graph API with retries.
func graphGet(accessToken, path string) ([]byte, error) {
	var body []byte
	var err error
	for i := 0; i < 3; i++ {
		body, err = graphGetOnce(accessToken, path)
		if err == nil {
			return body, nil
		}
		// Retry on transient server errors.
		if strings.Contains(err.Error(), "502") || strings.Contains(err.Error(), "503") || strings.Contains(err.Error(), "504") {
			time.Sleep(time.Duration(i+1) * 500 * time.Millisecond)
			continue
		}
		break
	}
	return body, err
}

// graphGetOnce performs a single authenticated GET request.
func graphGetOnce(accessToken, path string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, graphAPIBase+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", path, err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: HTTP %d: %s", path, resp.StatusCode, body)
	}
	return body, nil
}

// graphPost performs an authenticated POST request against the Graph API.
func graphPost(accessToken, path string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, graphAPIBase+path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("POST %s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("POST %s: HTTP %d: %s", path, resp.StatusCode, body)
	}
	return nil
}

// graphPut performs an authenticated PUT request against the Graph API.
func graphPut(accessToken, path string, content []byte, contentType string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodPut, graphAPIBase+path, bytes.NewReader(content))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", contentType)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("PUT %s: %w", path, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("PUT %s: HTTP %d: %s", path, resp.StatusCode, body)
	}
	return body, nil
}

// DriveItem represents a file or folder inside OneDrive or SharePoint.
type DriveItem struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	WebURL string `json:"webUrl"`
	ETag   string `json:"eTag"`
}

// ChannelFilesFolder represents the response when getting a channel's files folder.
type ChannelFilesFolder struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	ParentReference struct {
		DriveID string `json:"driveId"`
	} `json:"parentReference"`
}

// uploadFileInSession handles uploading large files using upload sessions.
func uploadFileInSession(accessToken, createSessionURL string, content []byte) (*DriveItem, error) {
	payload := map[string]any{
		"item": map[string]any{
			"@microsoft.graph.conflictBehavior": "rename",
		},
	}
	sessionBody, err := graphPostWithResponse(accessToken, createSessionURL, payload)
	if err != nil {
		return nil, fmt.Errorf("create upload session: %w", err)
	}

	var sessionRes struct {
		UploadURL string `json:"uploadUrl"`
	}
	if err := json.Unmarshal(sessionBody, &sessionRes); err != nil {
		return nil, fmt.Errorf("unmarshal upload session response: %w", err)
	}
	if sessionRes.UploadURL == "" {
		return nil, fmt.Errorf("empty upload URL in response")
	}

	totalSize := len(content)
	// Chunk size must be a multiple of 320 KiB (327680 bytes)
	// Let's upload in 3.125 MB chunks
	chunkSize := 3276800
	offset := 0

	var finalBody []byte

	for offset < totalSize {
		end := offset + chunkSize
		if end > totalSize {
			end = totalSize
		}

		chunk := content[offset:end]
		req, err := http.NewRequest(http.MethodPut, sessionRes.UploadURL, bytes.NewReader(chunk))
		if err != nil {
			return nil, fmt.Errorf("create PUT chunk request: %w", err)
		}

		req.Header.Set("Content-Length", fmt.Sprintf("%d", len(chunk)))
		req.Header.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", offset, end-1, totalSize))

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("upload chunk bytes %d-%d: %w", offset, end-1, err)
		}
		defer resp.Body.Close()

		resBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("read chunk response body: %w", err)
		}

		if resp.StatusCode >= 300 {
			return nil, fmt.Errorf("chunk upload HTTP %d: %s", resp.StatusCode, string(resBytes))
		}

		finalBody = resBytes
		offset = end
	}

	var item DriveItem
	if err := json.Unmarshal(finalBody, &item); err != nil {
		return nil, fmt.Errorf("unmarshal DriveItem from final chunk: %w", err)
	}

	return &item, nil
}

// UploadChatFile uploads a file to the user's OneDrive Teams Chat Files folder.
// Uses simple upload for files <= 4MB, and upload sessions for larger files.
func UploadChatFile(accessToken, filename string, content []byte) (*DriveItem, error) {
	escapedFilename := url.PathEscape(filename)
	if len(content) <= 4*1024*1024 {
		path := fmt.Sprintf("/me/drive/root:/Microsoft Teams Chat Files/%s:/content", escapedFilename)
		contentType := http.DetectContentType(content)
		body, err := graphPut(accessToken, path, content, contentType)
		if err != nil {
			return nil, fmt.Errorf("upload to OneDrive (simple): %w", err)
		}
		var item DriveItem
		if err := json.Unmarshal(body, &item); err != nil {
			return nil, fmt.Errorf("unmarshal DriveItem: %w", err)
		}
		return &item, nil
	}

	// Use upload session for files > 4MB
	createSessionURL := fmt.Sprintf("/me/drive/root:/Microsoft Teams Chat Files/%s:/createUploadSession", escapedFilename)
	return uploadFileInSession(accessToken, createSessionURL, content)
}

// GetChannelFilesFolder retrieves the files folder driveItem metadata for a Teams channel.
func GetChannelFilesFolder(accessToken, teamID, channelID string) (*ChannelFilesFolder, error) {
	path := fmt.Sprintf("/teams/%s/channels/%s/filesFolder", teamID, channelID)
	body, err := graphGet(accessToken, path)
	if err != nil {
		return nil, fmt.Errorf("get channel files folder: %w", err)
	}

	var folder ChannelFilesFolder
	if err := json.Unmarshal(body, &folder); err != nil {
		return nil, fmt.Errorf("unmarshal ChannelFilesFolder: %w", err)
	}

	return &folder, nil
}

// UploadChannelFile uploads a file to a Teams channel's files folder in SharePoint.
// Uses simple upload for files <= 4MB, and upload sessions for larger files.
func UploadChannelFile(accessToken, driveID, folderID, filename string, content []byte) (*DriveItem, error) {
	escapedFilename := url.PathEscape(filename)
	if len(content) <= 4*1024*1024 {
		path := fmt.Sprintf("/drives/%s/items/%s:/%s:/content", driveID, folderID, escapedFilename)
		contentType := http.DetectContentType(content)
		body, err := graphPut(accessToken, path, content, contentType)
		if err != nil {
			return nil, fmt.Errorf("upload to SharePoint (simple): %w", err)
		}
		var item DriveItem
		if err := json.Unmarshal(body, &item); err != nil {
			return nil, fmt.Errorf("unmarshal DriveItem: %w", err)
		}
		return &item, nil
	}

	// Use upload session for files > 4MB
	createSessionURL := fmt.Sprintf("/drives/%s/items/%s:/%s:/createUploadSession", driveID, folderID, escapedFilename)
	return uploadFileInSession(accessToken, createSessionURL, content)
}

// graphPatch performs an authenticated PATCH request against the Graph API.
func graphPatch(accessToken, path string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}
	req, err := http.NewRequest(http.MethodPatch, graphAPIBase+path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("PATCH %s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("PATCH %s: HTTP %d: %s", path, resp.StatusCode, body)
	}
	return nil
}

// graphDelete performs an authenticated DELETE request against the Graph API.
func graphDelete(accessToken, path string, useBeta bool) error {
	base := graphAPIBase
	if useBeta {
		base = graphAPIBeta
	}
	req, err := http.NewRequest(http.MethodDelete, base+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("DELETE %s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("DELETE %s: HTTP %d: %s", path, resp.StatusCode, body)
	}
	return nil
}

// ---------------------------------------------------------------------------
// GetMe — current user profile with cache
// ---------------------------------------------------------------------------

// GetMe fetches the authenticated user's profile, using a local cache.
func GetMe(accessToken string) (*User, error) {
	cacheDir, err := GetCacheDir()
	if err == nil {
		profilePath := filepath.Join(cacheDir, "profile.json")
		if data, err := os.ReadFile(profilePath); err == nil {
			var u User
			if json.Unmarshal(data, &u) == nil {
				return &u, nil
			}
		}
	}

	body, err := graphGet(accessToken, "/me")
	if err != nil {
		return nil, fmt.Errorf("GetMe: %w", err)
	}

	var u User
	if err := json.Unmarshal(body, &u); err != nil {
		return nil, fmt.Errorf("GetMe: parse: %w", err)
	}

	// Persist to cache.
	if cacheDir != "" {
		_ = os.WriteFile(filepath.Join(cacheDir, "profile.json"), body, 0o600)
	}
	return &u, nil
}

// ---------------------------------------------------------------------------
// GetChatMembers
// ---------------------------------------------------------------------------

// GetChatMembers returns the members of a chat. On error it returns an empty slice.
func GetChatMembers(accessToken, chatID string) []ChatMember {
	body, err := graphGet(accessToken, "/chats/"+chatID+"/members")
	if err != nil {
		return nil
	}
	var r membersResponse
	if err := json.Unmarshal(body, &r); err != nil {
		return nil
	}
	return r.Value
}

// GetTeamMembers returns the members of a Team. On error it returns an empty slice.
func GetTeamMembers(accessToken, teamID string) []ChatMember {
	body, err := graphGet(accessToken, "/teams/"+teamID+"/members")
	if err != nil {
		return nil
	}
	var r membersResponse
	if err := json.Unmarshal(body, &r); err != nil {
		return nil
	}
	return r.Value
}

// ---------------------------------------------------------------------------
// GetMessages
// ---------------------------------------------------------------------------

// GetMessages returns the messages in a chat (newest first from the API) and a next link for pagination.
func GetMessages(accessToken, chatID string, top int) ([]Message, string, error) {
	pageSize := top
	if pageSize > 50 || pageSize <= 0 {
		pageSize = 50
	}

	url := fmt.Sprintf("/chats/%s/messages?$orderby=createdDateTime%%20desc&$top=%d", chatID, pageSize)
	body, err := graphGet(accessToken, url)
	if err != nil {
		return nil, "", fmt.Errorf("GetMessages: %w", err)
	}
	var r messagesResponse
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, "", fmt.Errorf("GetMessages: parse: %w", err)
	}

	allMsgs := r.Value
	next := ""
	if r.NextLink != nil {
		next = *r.NextLink
	}

	// Keep fetching pages if we need more messages and a next link is available.
	for len(allMsgs) < top && next != "" {
		nextMsgs, newNext, err := GetMessagesFromLink(accessToken, next)
		if err != nil {
			// Return what we have so far instead of failing completely.
			break
		}
		allMsgs = append(allMsgs, nextMsgs...)
		next = newNext
	}

	// Filter out non-downloadable attachments (like rich cards and URL previews)
	FilterMessagesAttachments(allMsgs)

	// Ensure messages are sorted by creation time (newest first).
	sort.Slice(allMsgs, func(i, j int) bool {
		return allMsgs[i].CreatedDateTime > allMsgs[j].CreatedDateTime
	})

	return allMsgs, next, nil
}

// GetMessagesFromLink fetches messages from a full Graph API URL (used for pagination).
func GetMessagesFromLink(accessToken, nextLink string) ([]Message, string, error) {
	// nextLink is a full URL, but graphGet expects a path starting with /.
	// However, graphGetOnce uses graphAPIBase + path.
	// We should probably add a helper for full URLs or just strip the base.
	path := nextLink
	if strings.HasPrefix(path, graphAPIBase) {
		path = path[len(graphAPIBase):]
	} else if strings.HasPrefix(path, graphAPIBeta) {
		path = path[len(graphAPIBeta):]
	}

	body, err := graphGet(accessToken, path)
	if err != nil {
		return nil, "", fmt.Errorf("GetMessagesFromLink: %w", err)
	}
	var r messagesResponse
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, "", fmt.Errorf("GetMessagesFromLink: parse: %w", err)
	}

	// Filter out non-downloadable attachments (like rich cards and URL previews)
	FilterMessagesAttachments(r.Value)

	sort.Slice(r.Value, func(i, j int) bool {
		return r.Value[i].CreatedDateTime > r.Value[j].CreatedDateTime
	})

	next := ""
	if r.NextLink != nil {
		next = *r.NextLink
	}

	return r.Value, next, nil
}

// ---------------------------------------------------------------------------
// SendMessage
// ---------------------------------------------------------------------------

// replaceEmoticons replaces popular text emoticons with their Unicode equivalents.
func replaceEmoticons(s string) string {
	// Order matters: replace longer versions first to avoid partial matches.
	replacements := []struct{ from, to string }{
		{":-D", "😀"},
		{":D", "😀"},
		{":-)", "🙂"},
		{":)", "🙂"},
		{";-)", "😉"},
		{";)", "😉"},
		{":-(", "🙁"},
		{":(", "🙁"},
		{":-P", "😛"},
		{":P", "😛"},
		{"<3", "❤️"},
		{"(y)", "👍"},
		{"(n)", "👎"},
	}

	for _, r := range replacements {
		s = strings.ReplaceAll(s, r.from, r.to)
	}
	return s
}

// parseMentions scans content for "@DisplayName" patterns corresponding to the members list.
// It returns the updated HTML content containing <at id="..."> tags and the Graph API mentions payload.
func parseMentions(content string, members []ChatMember) (string, []map[string]any) {
	if len(members) == 0 {
		return content, nil
	}

	// Sort members by display name length descending to avoid partial matches
	// (e.g. "@John Smith" matching "@John" first).
	sortedMembers := make([]ChatMember, len(members))
	copy(sortedMembers, members)
	sort.Slice(sortedMembers, func(i, j int) bool {
		nameI := ""
		if sortedMembers[i].DisplayName != nil {
			nameI = *sortedMembers[i].DisplayName
		}
		nameJ := ""
		if sortedMembers[j].DisplayName != nil {
			nameJ = *sortedMembers[j].DisplayName
		}
		return len(nameI) > len(nameJ)
	})

	var mentions []map[string]any
	mentionIndex := 0

	for _, m := range sortedMembers {
		if m.DisplayName == nil || m.UserID == nil {
			continue
		}
		displayName := *m.DisplayName
		userID := *m.UserID
		if displayName == "" || userID == "" {
			continue
		}

		mentionTag := "@" + displayName
		if strings.Contains(content, mentionTag) {
			atTag := fmt.Sprintf(`<at id="%d">%s</at>`, mentionIndex, stdhtml.EscapeString(displayName))
			content = strings.ReplaceAll(content, mentionTag, atTag)

			mentions = append(mentions, map[string]any{
				"id":          mentionIndex,
				"mentionText": displayName,
				"mentioned": map[string]any{
					"user": map[string]any{
						"id":          userID,
						"displayName": displayName,
					},
				},
			})
			mentionIndex++
		}
	}

	return content, mentions
}

// formatMessageBody prepares the payload body for sending or updating a message.
func formatMessageBody(content string, members []ChatMember) (map[string]any, []map[string]any) {
	content = replaceEmoticons(content)

	// Detect whether content needs HTML rendering (markdown, multi-line, or containing a URL).
	hasMarkdown := containsMarkdown(content)
	isMultiLine := strings.Contains(content, "\n")
	isIndented := strings.HasPrefix(content, " ") || strings.HasPrefix(content, "\t")
	hasURL := containsURL(content)

	hasMentions := false
	for _, m := range members {
		if m.DisplayName != nil && *m.DisplayName != "" && m.UserID != nil && *m.UserID != "" {
			if strings.Contains(content, "@"+*m.DisplayName) {
				hasMentions = true
				break
			}
		}
	}

	if !hasMarkdown && !isMultiLine && !isIndented && !hasMentions && !hasURL {
		// Plain single-line text — send as-is.
		return map[string]any{
			"content": content,
		}, nil
	}

	// Convert markdown (and handle multi-line/URLs) to Teams-compatible HTML.
	var htmlContent string
	if hasMarkdown || isMultiLine || isIndented || hasURL {
		htmlContent = markdownToHTML(content)
	} else {
		htmlContent = stdhtml.EscapeString(content)
	}

	htmlContent, mentions := parseMentions(htmlContent, members)

	return map[string]any{
		"contentType": "html",
		"content":     htmlContent,
	}, mentions
}

func generateGUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func extractGUIDFromETag(eTag string) string {
	re := regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`)
	match := re.FindString(eTag)
	if match != "" {
		return strings.ToLower(match)
	}
	return ""
}

func shareOneDriveFileWithMembers(accessToken, itemID string, members []ChatMember) error {
	var recipients []map[string]any
	for _, m := range members {
		if m.Email != nil && *m.Email != "" {
			recipients = append(recipients, map[string]any{
				"email": *m.Email,
			})
		}
	}
	if len(recipients) == 0 {
		return nil
	}

	payload := map[string]any{
		"recipients":     recipients,
		"roles":          []string{"read"},
		"sendInvitation": false,
		"requireSignIn":  true,
	}

	path := fmt.Sprintf("/me/drive/items/%s/invite", itemID)
	err := graphPost(accessToken, path, payload)
	if err != nil {
		return fmt.Errorf("invite members: %w", err)
	}
	return nil
}

// uploadChatFiles uploads multiple small files to OneDrive and returns reference attachments.
func uploadChatFiles(accessToken string, files []PendingFile, members []ChatMember) ([]MessageAttachment, error) {
	var refAttachments []MessageAttachment
	for _, f := range files {
		item, err := UploadChatFile(accessToken, f.Name, f.Data)
		if err != nil {
			return nil, fmt.Errorf("upload to OneDrive: %w", err)
		}
		if err := shareOneDriveFileWithMembers(accessToken, item.ID, members); err != nil {
			return nil, fmt.Errorf("share OneDrive file: %w", err)
		}
		guid := extractGUIDFromETag(item.ETag)
		if guid == "" {
			guid = generateGUID()
		}
		refAttachments = append(refAttachments, MessageAttachment{
			ID:         guid,
			Name:       &f.Name,
			ContentURL: &item.WebURL,
		})
	}
	return refAttachments, nil
}

// uploadChannelFiles uploads multiple small files to SharePoint and returns reference attachments.
func uploadChannelFiles(accessToken, teamID, channelID string, files []PendingFile) ([]MessageAttachment, error) {
	var refAttachments []MessageAttachment
	if len(files) == 0 {
		return nil, nil
	}
	folder, err := GetChannelFilesFolder(accessToken, teamID, channelID)
	if err != nil {
		return nil, fmt.Errorf("get channel folder: %w", err)
	}
	for _, f := range files {
		item, err := UploadChannelFile(accessToken, folder.ParentReference.DriveID, folder.ID, f.Name, f.Data)
		if err != nil {
			return nil, fmt.Errorf("upload to SharePoint: %w", err)
		}
		guid := extractGUIDFromETag(item.ETag)
		if guid == "" {
			guid = generateGUID()
		}
		refAttachments = append(refAttachments, MessageAttachment{
			ID:         guid,
			Name:       &f.Name,
			ContentURL: &item.WebURL,
		})
	}
	return refAttachments, nil
}

// formatMessageBodyWithImagesAndFiles prepares the payload body and attachments for inline images and file references.
func formatMessageBodyWithImagesAndFiles(content string, members []ChatMember, images []PastedImage, referenceAttachments []MessageAttachment) (map[string]any, []map[string]any, []map[string]any, []map[string]any) {
	var htmlContent string
	if containsMarkdown(content) || strings.Contains(content, "\n") || strings.HasPrefix(content, " ") || strings.HasPrefix(content, "\t") || containsURL(content) {
		htmlContent = markdownToHTML(content)
	} else {
		htmlContent = stdhtml.EscapeString(content)
	}

	htmlContent, mentions := parseMentions(htmlContent, members)

	var hostedContents []map[string]any
	usedImages := make(map[int]bool)

	// Replace [Image N] with hostedContents reference
	reImg := regexp.MustCompile(`\[[Ii]mage\s+(\d+)\]`)
	htmlContent = reImg.ReplaceAllStringFunc(htmlContent, func(match string) string {
		sub := reImg.FindStringSubmatch(match)
		if len(sub) < 2 {
			return match
		}
		var idx int
		_, err := fmt.Sscanf(sub[1], "%d", &idx)
		if err != nil || idx < 1 || idx > len(images) {
			return match
		}
		usedImages[idx-1] = true
		return fmt.Sprintf(`<img src="../hostedContents/%d/$value" />`, idx)
	})

	// Only include images that are actually referenced in the HTML body.
	for i, img := range images {
		if usedImages[i] {
			b64Content := base64.StdEncoding.EncodeToString(img.Bytes)
			hostedContents = append(hostedContents, map[string]any{
				"@microsoft.graph.temporaryId": fmt.Sprintf("%d", i+1),
				"contentBytes":                 b64Content,
				"contentType":                  img.ContentType,
			})
		}
	}

	// Handle reference attachments
	var attachmentsPayload []map[string]any
	for _, att := range referenceAttachments {
		attachmentsPayload = append(attachmentsPayload, map[string]any{
			"id":          att.ID,
			"contentType": "reference",
			"contentUrl":  *att.ContentURL,
			"name":        *att.Name,
		})

		// Replace [File: filename] inline or append to the bottom
		pattern := fmt.Sprintf(`\[[Ff]ile:\s*%s\]`, regexp.QuoteMeta(*att.Name))
		reFile := regexp.MustCompile(pattern)
		if reFile.MatchString(htmlContent) {
			htmlContent = reFile.ReplaceAllString(htmlContent, fmt.Sprintf(`<attachment id="%s"></attachment>`, att.ID))
		} else {
			htmlContent += fmt.Sprintf(`<br /><attachment id="%s"></attachment>`, att.ID)
		}
	}

	bodyPayload := map[string]any{
		"contentType": "html",
		"content":     htmlContent,
	}

	return bodyPayload, mentions, hostedContents, attachmentsPayload
}

// SendMessage posts a message to the given chat.
func SendMessage(accessToken, chatID, content string, members []ChatMember, images []PastedImage, files []PendingFile) error {
	refAttachments, err := uploadChatFiles(accessToken, files, members)
	if err != nil {
		return err
	}

	body, mentions, hostedContents, attachments := formatMessageBodyWithImagesAndFiles(content, members, images, refAttachments)
	payload := map[string]any{
		"body": body,
	}
	if len(mentions) > 0 {
		payload["mentions"] = mentions
	}
	if len(hostedContents) > 0 {
		payload["hostedContents"] = hostedContents
	}
	if len(attachments) > 0 {
		payload["attachments"] = attachments
	}
	return graphPost(accessToken, "/chats/"+chatID+"/messages", payload)
}

// SendChannelMessage posts a new message to a Teams channel.
// Requires ChannelMessage.Read.All delegated permission.
func SendChannelMessage(accessToken, teamID, channelID, content string, members []ChatMember, images []PastedImage, files []PendingFile) error {
	refAttachments, err := uploadChannelFiles(accessToken, teamID, channelID, files)
	if err != nil {
		return err
	}

	body, mentions, hostedContents, attachments := formatMessageBodyWithImagesAndFiles(content, members, images, refAttachments)
	payload := map[string]any{
		"body": body,
	}
	if len(mentions) > 0 {
		payload["mentions"] = mentions
	}
	if len(hostedContents) > 0 {
		payload["hostedContents"] = hostedContents
	}
	if len(attachments) > 0 {
		payload["attachments"] = attachments
	}
	return graphPost(accessToken, fmt.Sprintf("/teams/%s/channels/%s/messages", teamID, channelID), payload)
}

// SendChannelReply posts a reply into an existing Teams channel thread.
// rootMsgID is the ID of the root (top-level) message in the thread.
// Requires ChannelMessage.Send delegated permission.
func SendChannelReply(accessToken, teamID, channelID, rootMsgID, content string, members []ChatMember, images []PastedImage, files []PendingFile) error {
	refAttachments, err := uploadChannelFiles(accessToken, teamID, channelID, files)
	if err != nil {
		return err
	}

	body, mentions, hostedContents, attachments := formatMessageBodyWithImagesAndFiles(content, members, images, refAttachments)
	payload := map[string]any{
		"body": body,
	}
	if len(mentions) > 0 {
		payload["mentions"] = mentions
	}
	if len(hostedContents) > 0 {
		payload["hostedContents"] = hostedContents
	}
	if len(attachments) > 0 {
		payload["attachments"] = attachments
	}
	return graphPost(accessToken, fmt.Sprintf("/teams/%s/channels/%s/messages/%s/replies", teamID, channelID, rootMsgID), payload)
}

// SendMessageWithReference posts a reply-to-message using a Teams messageReference
// attachment, making it appear as a proper quoted reply in the Teams client.
func SendMessageWithReference(accessToken, chatID string, ref *Message, content string, members []ChatMember, images []PastedImage, files []PendingFile) error {
	refAttachments, err := uploadChatFiles(accessToken, files, members)
	if err != nil {
		return err
	}

	if ref == nil {
		return SendMessage(accessToken, chatID, content, members, images, files)
	}

	// Build the sender JSON for the attachment content field.
	senderName := ""
	senderID := ""
	if ref.From != nil && ref.From.User != nil {
		if ref.From.User.DisplayName != nil {
			senderName = *ref.From.User.DisplayName
		}
		if ref.From.User.ID != nil {
			senderID = *ref.From.User.ID
		}
	}

	// messagePreview is the plain-text snippet of the quoted message.
	preview := ""
	if ref.Body != nil && ref.Body.Content != nil {
		preview = stripBasicHTML(*ref.Body.Content)
	}
	const maxPreview = 200
	if len([]rune(preview)) > maxPreview {
		preview = string([]rune(preview)[:maxPreview]) + "…"
	}

	attContent := map[string]any{
		"messageId":      ref.ID,
		"messagePreview": preview,
		"messageSender": map[string]any{
			"application": nil,
			"device":      nil,
			"user": map[string]any{
				"userIdentityType": "aadUser",
				"id":               senderID,
				"displayName":      senderName,
			},
		},
	}
	attContentJSON, _ := json.Marshal(attContent)

	// The body MUST be HTML and MUST contain <attachment id="..."></attachment> as a
	// placeholder so Teams knows where to render the quote bubble.
	marker := fmt.Sprintf(`<attachment id="%s"></attachment>`, ref.ID)
	body, mentions, hostedContents, attachments := formatMessageBodyWithImagesAndFiles(content, members, images, refAttachments)
	bodyHTML := marker + "\n" + body["content"].(string)

	// Merge the message reference attachment with any file reference attachments
	var finalAttachments []map[string]any
	finalAttachments = append(finalAttachments, map[string]any{
		"id":          ref.ID,
		"contentType": "messageReference",
		"content":     string(attContentJSON),
	})
	finalAttachments = append(finalAttachments, attachments...)

	payload := map[string]any{
		"body": map[string]any{
			"contentType": "html",
			"content":     bodyHTML,
		},
		"attachments": finalAttachments,
	}
	if len(mentions) > 0 {
		payload["mentions"] = mentions
	}
	if len(hostedContents) > 0 {
		payload["hostedContents"] = hostedContents
	}
	return graphPost(accessToken, "/chats/"+chatID+"/messages", payload)
}

// stripBasicHTML removes HTML tags to produce a plain-text preview.
// It is a lightweight alternative to HTMLToText for building attachment content fields.
func stripBasicHTML(s string) string {
	tokenizer := html.NewTokenizer(strings.NewReader(s))
	var sb strings.Builder
	for {
		tt := tokenizer.Next()
		if tt == html.ErrorToken {
			break
		}
		if tt == html.TextToken {
			sb.WriteString(html.UnescapeString(tokenizer.Token().Data))
		}
	}
	return strings.TrimSpace(sb.String())
}

// UpdateMessage modifies an existing message in a chat.
func UpdateMessage(accessToken, chatID, messageID, content string, members []ChatMember) error {
	body, mentions := formatMessageBody(content, members)
	payload := map[string]any{
		"body": body,
	}
	if len(mentions) > 0 {
		payload["mentions"] = mentions
	}
	return graphPatch(accessToken, "/chats/"+chatID+"/messages/"+messageID, payload)
}

// UpdateChannelMessage modifies an existing message in a Teams channel.
func UpdateChannelMessage(accessToken, teamID, channelID, messageID, content string, members []ChatMember) error {
	body, mentions := formatMessageBody(content, members)
	payload := map[string]any{
		"body": body,
	}
	if len(mentions) > 0 {
		payload["mentions"] = mentions
	}
	return graphPatch(accessToken, fmt.Sprintf("/teams/%s/channels/%s/messages/%s", teamID, channelID, messageID), payload)
}

// ---------------------------------------------------------------------------
// SetReaction
// ---------------------------------------------------------------------------

// SetReaction adds or updates a reaction on a message.
func SetReaction(accessToken, chatID, messageID, reactionType string) error {
	payload := map[string]any{
		"reactionType": reactionType,
	}
	return graphPost(accessToken, "/chats/"+chatID+"/messages/"+messageID+"/setReaction", payload)
}

// SetChannelReaction adds or updates a reaction on a Teams channel message.
func SetChannelReaction(accessToken, teamID, channelID, messageID, reactionType string) error {
	payload := map[string]any{
		"reactionType": reactionType,
	}
	return graphPost(accessToken, fmt.Sprintf("/teams/%s/channels/%s/messages/%s/setReaction", teamID, channelID, messageID), payload)
}

// UnsetReaction removes a reaction from a chat message.
func UnsetReaction(accessToken, chatID, messageID, reactionType string) error {
	payload := map[string]any{
		"reactionType": reactionType,
	}
	return graphPost(accessToken, "/chats/"+chatID+"/messages/"+messageID+"/unsetReaction", payload)
}

// UnsetChannelReaction removes a reaction from a Teams channel message.
func UnsetChannelReaction(accessToken, teamID, channelID, messageID, reactionType string) error {
	payload := map[string]any{
		"reactionType": reactionType,
	}
	return graphPost(accessToken, fmt.Sprintf("/teams/%s/channels/%s/messages/%s/unsetReaction", teamID, channelID, messageID), payload)
}

// DeleteMessage removes a message from a chat (soft-delete via PATCH).
func DeleteMessage(accessToken, chatID, messageID string) error {
	payload := map[string]any{
		"body": map[string]any{
			"content": "*(deleted)*",
		},
	}
	return graphPatch(accessToken, "/chats/"+chatID+"/messages/"+messageID, payload)
}

// DeleteChannelMessage removes a message from a Teams channel (soft-delete via PATCH).
func DeleteChannelMessage(accessToken, teamID, channelID, messageID string) error {
	payload := map[string]any{
		"body": map[string]any{
			"content": "*(deleted)*",
		},
	}
	return graphPatch(accessToken, fmt.Sprintf("/teams/%s/channels/%s/messages/%s", teamID, channelID, messageID), payload)
}

// ---------------------------------------------------------------------------
// MarkChatAsRead
// ---------------------------------------------------------------------------

// MarkChatAsRead marks the chat as read for the current user.
// All errors are silently ignored so as not to disrupt the UX.
func MarkChatAsRead(accessToken, chatID, userID string) {
	// Fetch tenant ID.
	body, err := graphGet(accessToken, "/organization")
	if err != nil {
		return
	}
	var org orgResponse
	if err := json.Unmarshal(body, &org); err != nil || len(org.Value) == 0 {
		return
	}
	tenantID := org.Value[0].ID

	payload := map[string]any{
		"user": map[string]string{
			"id":       userID,
			"tenantId": tenantID,
		},
	}
	_ = graphPost(accessToken, "/chats/"+chatID+"/markChatReadForUser", payload)
}

// ---------------------------------------------------------------------------
// GetChats — main chat list with member fetch + display name computation
// ---------------------------------------------------------------------------

// GetChats fetches the user's chats, fetches members for each,
// detects the current user (by frequency analysis), filters the current user
// from member lists, computes CachedDisplayName, and returns
// (chats, detectedCurrentUserName).
func GetChats(accessToken string, existingChats []Chat, currentUserName *string) ([]Chat, *string, error) {
	limit := ResolveChatLimit()

	// Build a map of existing members to avoid fetching them again in background refreshes.
	// We copy the slice to prevent background threads from sharing/mutating slice backing arrays with the UI thread.
	existingMembers := make(map[string][]ChatMember)
	for _, c := range existingChats {
		if len(c.Members) > 0 {
			membersCopy := make([]ChatMember, len(c.Members))
			copy(membersCopy, c.Members)
			existingMembers[c.ID] = membersCopy
		}
	}

	// We load a larger batch of chat metadata first to ensure we don't miss recent chats,
	// since the Graph API does not guarantee chronological order on /me/chats pages.
	metadataLimit := 150
	if limit > metadataLimit {
		metadataLimit = limit
	}

	url := "/me/chats?$expand=lastMessagePreview"
	body, err := graphGet(accessToken, url)
	if err != nil {
		return nil, nil, fmt.Errorf("GetChats: %w", err)
	}
	var r chatsResponse
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, nil, fmt.Errorf("GetChats: parse: %w", err)
	}

	chats := r.Value
	next := ""
	if r.NextLink != nil {
		next = *r.NextLink
	}

	// Keep fetching pages of chats if we need more to satisfy the metadata limit.
	for len(chats) < metadataLimit && next != "" {
		path := next
		if strings.HasPrefix(path, graphAPIBase) {
			path = path[len(graphAPIBase):]
		} else if strings.HasPrefix(path, graphAPIBeta) {
			path = path[len(graphAPIBeta):]
		}

		nextBody, err := graphGet(accessToken, path)
		if err != nil {
			break
		}
		var nextR chatsResponse
		if err := json.Unmarshal(nextBody, &nextR); err != nil {
			break
		}
		chats = append(chats, nextR.Value...)
		if nextR.NextLink != nil {
			next = *nextR.NextLink
		} else {
			next = ""
		}
	}

	// Filter out meeting chats with no messages (LastMessagePreview is nil)
	var filtered []Chat
	for _, c := range chats {
		if c.ChatType == "meeting" && c.LastMessagePreview == nil {
			continue
		}
		filtered = append(filtered, c)
	}
	chats = filtered

	// Deduplicate chats by ID. The Graph API can return the same chat on
	// multiple pages when a new message causes it to shift position during
	// pagination (cursor drift). Keep the entry with the newest timestamp.
	{
		seen := make(map[string]int, len(chats)) // ID → index in deduped slice
		deduped := make([]Chat, 0, len(chats))
		for _, c := range chats {
			if idx, ok := seen[c.ID]; ok {
				// Replace only if this copy has a newer last-message time.
				existing := deduped[idx]
				existingT := time.Time{}
				if existing.LastMessagePreview != nil {
					existingT, _ = time.Parse(time.RFC3339Nano, existing.LastMessagePreview.CreatedDateTime)
				} else if existing.LastUpdated != nil {
					if lut, err := time.Parse(time.RFC3339Nano, *existing.LastUpdated); err == nil {
						existingT = lut
					}
				}
				newT := time.Time{}
				if c.LastMessagePreview != nil {
					newT, _ = time.Parse(time.RFC3339Nano, c.LastMessagePreview.CreatedDateTime)
				} else if c.LastUpdated != nil {
					if lut, err := time.Parse(time.RFC3339Nano, *c.LastUpdated); err == nil {
						newT = lut
					}
				}
				if newT.After(existingT) {
					deduped[idx] = c
				}
			} else {
				seen[c.ID] = len(deduped)
				deduped = append(deduped, c)
			}
		}
		chats = deduped
	}

	// Sort the entire list of chats by latest activity (message or update time) descending.
	type chatWithTime struct {
		chat Chat
		t    time.Time
	}
	combined := make([]chatWithTime, len(chats))
	for i, c := range chats {
		t := time.Time{}
		if c.LastMessagePreview != nil {
			t, _ = time.Parse(time.RFC3339Nano, c.LastMessagePreview.CreatedDateTime)
		} else if c.LastUpdated != nil {
			lut, _ := time.Parse(time.RFC3339Nano, *c.LastUpdated)
			t = lut
		}
		combined[i] = chatWithTime{c, t}
	}

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
	chats = sorted

	// Truncate to the user's requested chat limit.
	if len(chats) > limit {
		chats = chats[:limit]
	}

	// Fetch members concurrently only for the truncated, active chats (if not already cached).
	type result struct {
		index   int
		members []ChatMember
	}
	ch := make(chan result, len(chats))
	for i, c := range chats {
		go func(i int, id string) {
			if cached, ok := existingMembers[id]; ok {
				ch <- result{i, cached}
			} else {
				ch <- result{i, GetChatMembers(accessToken, id)}
			}
		}(i, c.ID)
	}
	for range chats {
		res := <-ch
		chats[res.index].Members = res.members
	}

	// Detect current user by name frequency across oneOnOne chats if not already provided.
	if currentUserName == nil {
		currentUserName = detectCurrentUser(chats)
	}

	// Filter current user from member lists and compute display names.
	for i := range chats {
		if chats[i].LastMessagePreview != nil {
			FilterMessageAttachments(chats[i].LastMessagePreview)
		}
		if currentUserName != nil {
			chats[i].Members = filterMember(chats[i].Members, *currentUserName)
		}
		chats[i].CachedDisplayName = new(string)
		*chats[i].CachedDisplayName = computeDisplayName(&chats[i])
	}

	return chats, currentUserName, nil
}

// ---------------------------------------------------------------------------
// GetChat — fetch a single chat by ID
// ---------------------------------------------------------------------------

// GetChat fetches a single chat by ID, populates its members, filters the
// current user, and computes CachedDisplayName. Used to hydrate favourited
// chats that fall outside the regular chat_limit window.
func GetChat(accessToken, chatID string, currentUserName *string) (*Chat, error) {
	body, err := graphGet(accessToken, "/chats/"+chatID+"?$expand=lastMessagePreview")
	if err != nil {
		return nil, fmt.Errorf("GetChat: %w", err)
	}
	var c Chat
	if err := json.Unmarshal(body, &c); err != nil {
		return nil, fmt.Errorf("GetChat: parse: %w", err)
	}

	// Fetch members (same as GetChats does per-chat).
	c.Members = GetChatMembers(accessToken, chatID)

	if c.LastMessagePreview != nil {
		FilterMessageAttachments(c.LastMessagePreview)
	}

	// Filter current user and compute display name.
	if currentUserName != nil {
		c.Members = filterMember(c.Members, *currentUserName)
	}
	c.CachedDisplayName = new(string)
	*c.CachedDisplayName = computeDisplayName(&c)
	return &c, nil
}

// detectCurrentUser identifies the current user by finding the display name
// that appears most frequently across oneOnOne chats.
func detectCurrentUser(chats []Chat) *string {
	freq := map[string]int{}
	for _, c := range chats {
		if c.ChatType != "oneOnOne" {
			continue
		}
		for _, m := range c.Members {
			if m.DisplayName != nil {
				freq[*m.DisplayName]++
			}
		}
	}
	if len(freq) == 0 {
		return nil
	}

	var best string
	var bestCount int
	for name, count := range freq {
		if count > bestCount {
			best = name
			bestCount = count
		}
	}

	// Only treat as current user if appears ≥2 times, or is the sole member in all oneOnOne chats.
	oneOnOneCount := 0
	for _, c := range chats {
		if c.ChatType == "oneOnOne" {
			oneOnOneCount++
		}
	}
	if bestCount >= 2 || oneOnOneCount == 1 {
		return &best
	}
	return nil
}

// filterMember removes the named member from the slice by allocating a new slice (never modifying in-place).
func filterMember(members []ChatMember, name string) []ChatMember {
	var out []ChatMember
	for _, m := range members {
		if m.DisplayName == nil || *m.DisplayName != name {
			out = append(out, m)
		}
	}
	return out
}

// computeDisplayName derives a human-readable display name for a chat.
func computeDisplayName(c *Chat) string {
	switch c.ChatType {
	case "oneOnOne":
		if len(c.Members) > 0 && c.Members[0].DisplayName != nil {
			return *c.Members[0].DisplayName
		}
		return "Unknown"

	case "group", "meeting":
		if c.Topic != nil && *c.Topic != "" {
			return *c.Topic
		}
		parts, hasMore := memberAbbreviations(c.Members, 3)
		if len(parts) > 0 {
			name := strings.Join(parts, ", ")
			if hasMore {
				name += " ..."
			}
			return name
		}
		if c.ChatType == "group" {
			return "Unnamed Group"
		}
		return "Unnamed Meeting"

	default:
		if c.Topic != nil && *c.Topic != "" {
			return *c.Topic
		}
		parts, hasMore := memberAbbreviations(c.Members, 3)
		if len(parts) > 0 {
			name := strings.Join(parts, ", ")
			if hasMore {
				name += " ..."
			}
			return name
		}
		return "Unknown Chat"
	}
}

// memberAbbreviations returns up to n abbreviated member display names and a boolean indicating if there are more.
func memberAbbreviations(members []ChatMember, n int) ([]string, bool) {
	var names []string
	for _, m := range members {
		if m.DisplayName == nil {
			continue
		}
		names = append(names, abbreviateName(*m.DisplayName))
	}

	sort.Strings(names)

	var out []string
	for i := 0; i < len(names) && i < n; i++ {
		out = append(out, names[i])
	}
	return out, len(names) > n
}

// abbreviateName converts "Matt Davidson" → "Matt D", single word stays as-is.
func abbreviateName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	parts := strings.Fields(name)
	if len(parts) == 1 {
		return parts[0]
	}
	return parts[0] + " " + string([]rune(parts[len(parts)-1])[0])
}

// ---------------------------------------------------------------------------
// HTML-to-text rendering
// ---------------------------------------------------------------------------

func decodeSafeLink(u string) string {
	parsed, err := url.Parse(u)
	if err != nil {
		return u
	}
	if strings.Contains(parsed.Host, "safelinks.protection.outlook.com") {
		realURL := parsed.Query().Get("url")
		if realURL != "" {
			return realURL
		}
	}
	return u
}

// ExtractURLs extracts all unique URLs from a Teams message HTML body.
func ExtractURLs(htmlContent string) []string {
	tokenizer := html.NewTokenizer(strings.NewReader(htmlContent))
	var urls []string
	urlMap := make(map[string]bool)

	addURL := func(u string) {
		u = decodeSafeLink(u)
		if u == "" {
			return
		}
		if !urlMap[u] {
			urls = append(urls, u)
			urlMap[u] = true
		}
	}

	for {
		tt := tokenizer.Next()
		if tt == html.ErrorToken {
			break
		}
		token := tokenizer.Token()
		switch tt {
		case html.StartTagToken, html.SelfClosingTagToken:
			if token.Data == "a" {
				for _, a := range token.Attr {
					if a.Key == "href" {
						addURL(a.Val)
					}
				}
			}
		case html.TextToken:
			text := html.UnescapeString(token.Data)
			matches := urlRegex.FindAllString(text, -1)
			for _, m := range matches {
				addURL(m)
			}
		}
	}
	return urls
}

var urlRegex = regexp.MustCompile(`https?://[^\s<>"]+`)

// HTMLToText converts a Teams message HTML body to plain text suitable for
// terminal display. It returns the rendered text and a lipgloss-compatible
// styled string (where special elements are coloured).
func HTMLToText(htmlContent string, attachments []MessageAttachment, mentions []MessageMention) string {
	if htmlContent == "" {
		return ""
	}

	// Build an attachment lookup by ID.
	attByID := make(map[string]MessageAttachment, len(attachments))
	for _, a := range attachments {
		attByID[a.ID] = a
	}

	// Build a map of mention ID string to user ID.
	mentionUserByID := make(map[string]string)
	for _, m := range mentions {
		if m.ID != nil && m.Mentioned != nil && m.Mentioned.User != nil && m.Mentioned.User.ID != nil {
			idStr := fmt.Sprintf("%d", *m.ID)
			mentionUserByID[idStr] = *m.Mentioned.User.ID
		}
	}

	tokenizer := html.NewTokenizer(strings.NewReader(htmlContent))
	var sb strings.Builder
	var lastChar rune
	var tagAddedNewline bool
	imgCounter := 0

	// ---- existing state ----
	var inPre bool
	var inLink bool
	var currentLinkURL string
	var linkText strings.Builder

	// ---- NEW: inline formatting state ----
	inBold := false
	inItalic := false
	inStrike := false
	inCode := false // <code> tag (inline or inside <pre>)
	inMention := false
	mentionPrefixAdded := false
	seenMentions := make(map[string]bool)
	seenUserMentions := make(map[string]bool)

	// ---- NEW: list state ----
	type listInfo struct {
		ordered bool
		counter int
	}
	var listStack []listInfo

	// ---- lipgloss styles ----
	preCodeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#98C379"))
	bulletStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))

	// applyInlineStyles applies the currently active inline text styles.
	applyInlineStyles := func(text string) string {
		if inCode && inPre {
			return preCodeStyle.Render(text)
		}
		// Build a single SGR sequence manually. lipgloss's Strikethrough(true)
		// and Underline(true) render per-character escape pairs, which corrupt
		// embedded ANSI sequences when styles are nested (e.g. a struck ticket
		// inside a hyperlink). Emitting one combined sequence keeps the output
		// compact and corruption-free.
		var params []string
		if inBold || inMention {
			params = append(params, "1")
		}
		if inItalic {
			params = append(params, "3")
		}
		if inStrike {
			params = append(params, "9")
		}
		switch {
		case inLink:
			params = append(params, "4", "38;2;0;175;255")
		case inMention:
			params = append(params, "38;2;95;135;255")
		case inCode:
			params = append(params, "38;2;229;192;123")
		}
		if len(params) == 0 {
			return text
		}
		return "\x1b[" + strings.Join(params, ";") + "m" + text + "\x1b[0m"
	}

	for {
		tt := tokenizer.Next()
		if tt == html.ErrorToken {
			break
		}

		token := tokenizer.Token()

		switch tt {
		case html.StartTagToken, html.SelfClosingTagToken:
			tag := token.Data
			if tag == "pre" {
				inPre = true
			}
			switch tag {
			// ---- inline formatting on ----
			case "b", "strong":
				inBold = true
			case "em", "i":
				inItalic = true
			case "s", "strike", "del":
				inStrike = true
			case "code":
				inCode = true
			case "at":
				inMention = true
				mentionPrefixAdded = false
				var atID string
				for _, attr := range token.Attr {
					if attr.Key == "id" {
						atID = attr.Val
						break
					}
				}
				if atID != "" {
					userID := mentionUserByID[atID]
					if userID != "" {
						if seenUserMentions[userID] {
							mentionPrefixAdded = true
						} else {
							seenUserMentions[userID] = true
						}
					} else {
						// Fallback to tag ID deduplication
						if seenMentions[atID] {
							mentionPrefixAdded = true
						} else {
							seenMentions[atID] = true
						}
					}
				}

			// ---- lists ----
			case "ul":
				listStack = append(listStack, listInfo{ordered: false})
			case "ol":
				listStack = append(listStack, listInfo{ordered: true})
			case "li":
				if lastChar != '\n' && sb.Len() > 0 {
					sb.WriteRune('\n')
					lastChar = '\n'
				}
				if len(listStack) > 0 {
					info := &listStack[len(listStack)-1]
					indent := strings.Repeat("  ", len(listStack)-1)
					var prefix string
					if info.ordered {
						info.counter++
						prefix = bulletStyle.Render(fmt.Sprintf("%s%d. ", indent, info.counter))
					} else {
						prefix = bulletStyle.Render(indent + "• ")
					}
					sb.WriteString(prefix)
					lastChar = ' '
				}
				tagAddedNewline = false

			case "img":
				imgCounter++
				var altText string
				for _, attr := range token.Attr {
					if attr.Key == "alt" {
						altText = attr.Val
						break
					}
				}
				imgName := altText
				if imgName == "" {
					imgName = fmt.Sprintf("inline-image-%d.png", imgCounter)
				} else {
					imgName = sanitizeFilename(imgName)
					if !hasExtension(imgName) {
						imgName += ".png"
					}
				}
				orangeText := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF8700")).Render(imgName)
				content := "🖼️  " + orangeText
				if inLink {
					content = fmt.Sprintf("\x1b]8;;%s\x1b\\%s\x1b]8;;\x1b\\", currentLinkURL, content)
				}
				sb.WriteString(content)
				lastChar = 'e'

			case "attachment":
				var attID string
				for _, a := range token.Attr {
					if a.Key == "id" {
						attID = a.Val
						break
					}
				}
				if att, ok := attByID[attID]; ok {
					if att.ContentType != nil && *att.ContentType == "messageReference" {
						// Render a quoted-message block: ▎ Sender: preview text
						if att.Content != nil {
							quote := renderMessageReference(*att.Content)
							if quote != "" {
								if sb.Len() > 0 && lastChar != '\n' {
									sb.WriteRune('\n')
								}
								sb.WriteString(quote)
								sb.WriteRune('\n')
								lastChar = '\n'
							}
						}
						continue
					}
					orangeText := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF8700")).Render("Attachment")
					sb.WriteString("📎 " + orangeText)
					lastChar = 't'
				}

			case "emoji":
				var altText string
				for _, a := range token.Attr {
					if a.Key == "alt" {
						altText = a.Val
						break
					}
				}
				if altText != "" {
					sb.WriteString(altText)
					r, _ := utf8.DecodeLastRuneInString(altText)
					lastChar = r
				}

			case "br":
				if lastChar != '\n' && sb.Len() > 0 {
					sb.WriteRune('\n')
					lastChar = '\n'
				}
				tagAddedNewline = true

			// Block-level elements — closing tag emits newline.
			case "p", "div", "pre":
				// Do nothing — closing tag will emit newline.

			case "a":
				for _, a := range token.Attr {
					if a.Key == "href" {
						currentLinkURL = decodeSafeLink(a.Val)
						inLink = true
						linkText.Reset()
						break
					}
				}
			}

		case html.EndTagToken:
			tag := token.Data
			if tag == "pre" {
				inPre = false
			}
			switch tag {
			// ---- inline formatting off ----
			case "b", "strong":
				inBold = false
			case "em", "i":
				inItalic = false
			case "s", "strike", "del":
				inStrike = false
			case "code":
				inCode = false
			case "at":
				inMention = false

			// ---- lists ----
			case "ul", "ol":
				if len(listStack) > 0 {
					listStack = listStack[:len(listStack)-1]
				}
			case "li":
				if lastChar != '\n' && sb.Len() > 0 {
					sb.WriteRune('\n')
					lastChar = '\n'
				}
				tagAddedNewline = true

			case "p", "div", "pre":
				if lastChar != '\n' && sb.Len() > 0 {
					sb.WriteRune('\n')
					lastChar = '\n'
				}
				tagAddedNewline = true
			case "br":
				if lastChar != '\n' && sb.Len() > 0 {
					sb.WriteRune('\n')
					lastChar = '\n'
				}
				tagAddedNewline = true
			case "a":
				if inLink {
					lt := strings.TrimSpace(linkText.String())
					if lt != "" && lt != currentLinkURL && !strings.Contains(currentLinkURL, lt) {
						diag := lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")).Render(" (" + currentLinkURL + ")")
						sb.WriteString(diag)
					}
					inLink = false
					currentLinkURL = ""
					linkText.Reset()
				}
			}

		case html.TextToken:
			text := html.UnescapeString(token.Data)
			if tagAddedNewline {
				// Consume exactly one leading newline if a tag just added one.
				if strings.HasPrefix(text, "\n") {
					text = text[1:]
				} else if strings.HasPrefix(text, "\r\n") {
					text = text[2:]
				}
				tagAddedNewline = false
			}
			if text != "" {
				// Skip whitespace-only tokens if they follow a newline and we're not in pre.
				// IMPORTANT: We do NOT skip non-breaking spaces (\u00A0) as they are used
				// to represent intentional empty lines or indentation.
				if !inPre && lastChar == '\n' && strings.TrimSpace(text) == "" && !strings.Contains(text, "\u00A0") {
					continue
				}

				if inMention && !mentionPrefixAdded {
					if !strings.HasPrefix(text, "@") {
						text = "@" + text
					}
					mentionPrefixAdded = true
				}

				// Apply inline formatting styles.
				styledText := applyInlineStyles(text)

				if inLink && currentLinkURL != "" {
					linkText.WriteString(text)
					// Link colour/underline are applied inside applyInlineStyles;
					// here we only wrap the (already styled) text in an OSC 8
					// hyperlink so no ANSI sequences get re-wrapped.
					styledText = fmt.Sprintf("\x1b]8;;%s\x1b\\%s\x1b]8;;\x1b\\", currentLinkURL, styledText)
				} else if !inBold && !inItalic && !inStrike && !inCode && !inMention {
					// Plain text: detect and style bare URLs.
					styledText = urlRegex.ReplaceAllStringFunc(text, func(u string) string {
						styled := "\x1b[4;38;2;0;175;255m" + u + "\x1b[0m"
						return fmt.Sprintf("\x1b]8;;%s\x1b\\%s\x1b]8;;\x1b\\", u, styled)
					})
				}

				sb.WriteString(styledText)
				r, _ := utf8.DecodeLastRuneInString(styledText)
				lastChar = r
			}
		}
	}

	result := sb.String()

	// Collapse runs of more than 2 consecutive newlines.
	for strings.Contains(result, "\n\n\n") {
		result = strings.ReplaceAll(result, "\n\n\n", "\n\n")
	}

	return strings.Trim(result, "\n\r")
}

// getAttachmentIcon returns an emoji icon based on the attachment's file extension
// or content type.
func getAttachmentIcon(att MessageAttachment) string {
	name := ""
	if att.Name != nil {
		name = strings.ToLower(*att.Name)
	}
	ct := ""
	if att.ContentType != nil {
		ct = strings.ToLower(*att.ContentType)
	}

	// Check extension first.
	ext := ""
	if idx := strings.LastIndex(name, "."); idx >= 0 {
		ext = name[idx+1:]
	}

	switch ext {
	case "jpg", "jpeg", "png", "gif", "bmp", "svg", "webp":
		return "🖼️"
	case "pdf", "txt":
		return "📄"
	case "doc", "docx":
		return "📝"
	case "xls", "xlsx", "csv":
		return "📊"
	case "ppt", "pptx":
		return "📊"
	case "mp4", "avi", "mov", "mkv", "webm":
		return "🎥"
	case "mp3", "wav", "ogg", "flac":
		return "🎵"
	case "zip", "rar", "7z", "tar", "gz":
		return "📦"
	case "html", "htm":
		return "🌐"
	case "json", "xml":
		return "📋"
	}

	// Fall back to content type.
	switch {
	case strings.HasPrefix(ct, "image/"):
		return "🖼️"
	case strings.HasPrefix(ct, "video/"):
		return "🎥"
	case strings.HasPrefix(ct, "audio/"):
		return "🎵"
	case strings.Contains(ct, "word") || strings.Contains(ct, "document"):
		return "📝"
	case strings.Contains(ct, "excel") || strings.Contains(ct, "spreadsheet"):
		return "📊"
	case strings.Contains(ct, "powerpoint") || strings.Contains(ct, "presentation"):
		return "📊"
	case strings.Contains(ct, "zip") || strings.Contains(ct, "archive"):
		return "📦"
	}

	return "📎"
}

// GetTenantID fetches the first tenant ID from the /organization endpoint.
func GetTenantID(accessToken string) (string, error) {
	body, err := graphGet(accessToken, "/organization")
	if err != nil {
		return "", err
	}
	var org orgResponse
	if err := json.Unmarshal(body, &org); err != nil || len(org.Value) == 0 {
		return "", fmt.Errorf("could not parse organization response")
	}
	return org.Value[0].ID, nil
}

// graphPostWithResponse performs an authenticated POST request and returns the response body.
func graphPostWithResponse(accessToken, path string, payload any) ([]byte, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, graphAPIBase+path, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("POST %s: %w", path, err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("POST %s: HTTP %d: %s", path, resp.StatusCode, body)
	}
	return body, nil
}

// SearchUsers searches for users in the tenant directory.
func SearchUsers(accessToken, query string) ([]User, error) {
	escaped := strings.ReplaceAll(query, "'", "''")
	filterExpr := fmt.Sprintf("startsWith(displayName,'%s') or startsWith(userPrincipalName,'%s')", escaped, escaped)
	path := "/users?$filter=" + url.QueryEscape(filterExpr) + "&$top=10"

	body, err := graphGet(accessToken, path)
	if err != nil {
		return nil, fmt.Errorf("SearchUsers: %w", err)
	}

	var r struct {
		Value []User `json:"value"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("SearchUsers parse: %w", err)
	}
	return r.Value, nil
}

// renderMessageReference parses a messageReference attachment content JSON and
// returns a styled terminal quote block: "▎ SenderName [2 Jan 15:04]: message preview".
// Returns an empty string if the content cannot be parsed.
func renderMessageReference(content string) string {
	var ref struct {
		MessageID      string `json:"messageId"`
		MessagePreview string `json:"messagePreview"`
		MessageSender  struct {
			User *struct {
				DisplayName string `json:"displayName"`
			} `json:"user"`
		} `json:"messageSender"`
	}
	if err := json.Unmarshal([]byte(content), &ref); err != nil {
		return ""
	}

	preview := strings.TrimSpace(ref.MessagePreview)
	if preview == "" {
		return ""
	}

	// Truncate very long previews.
	const maxPreview = 120
	if len([]rune(preview)) > maxPreview {
		runes := []rune(preview)
		preview = string(runes[:maxPreview]) + "…"
	}

	// Teams message IDs are Unix timestamps in milliseconds.
	var timeStr string
	if ms, err := strconv.ParseInt(ref.MessageID, 10, 64); err == nil && ms > 0 {
		t := time.UnixMilli(ms).Local()
		now := time.Now()
		if t.Year() == now.Year() {
			timeStr = t.Format("2 Jan 15:04")
		} else {
			timeStr = t.Format("2 Jan 2006 15:04")
		}
	}

	quoteStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#6C7A89"))
	barStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#4A90D9")).Bold(true)
	nameStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#7EC8E3")).Bold(true)
	timeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#4A5568"))

	bar := barStyle.Render("▎")

	var meta string
	if ref.MessageSender.User != nil && ref.MessageSender.User.DisplayName != "" {
		meta = nameStyle.Render(ref.MessageSender.User.DisplayName)
	}
	if timeStr != "" {
		meta += timeStyle.Render(" [" + timeStr + "]")
	}

	if meta != "" {
		return bar + " " + meta + quoteStyle.Render(": "+preview)
	}
	return bar + " " + quoteStyle.Render(preview)
}

// GetOrCreateOneOnOneChat creates a new 1-on-1 chat with the user specified by their UPN (email).
// If the chat already exists, the Graph API returns the existing one.
func GetOrCreateOneOnOneChat(accessToken, myUserID, otherUPN string) (*Chat, error) {
	payload := map[string]any{
		"chatType": "oneOnOne",
		"members": []map[string]any{
			{
				"@odata.type":     "#microsoft.graph.aadUserConversationMember",
				"roles":           []string{"owner"},
				"user@odata.bind": fmt.Sprintf("https://graph.microsoft.com/v1.0/users('%s')", myUserID),
			},
			{
				"@odata.type":     "#microsoft.graph.aadUserConversationMember",
				"roles":           []string{"owner"},
				"user@odata.bind": fmt.Sprintf("https://graph.microsoft.com/v1.0/users('%s')", otherUPN),
			},
		},
	}

	body, err := graphPostWithResponse(accessToken, "/chats", payload)
	if err != nil {
		return nil, fmt.Errorf("create chat: %w", err)
	}

	var chat Chat
	if err := json.Unmarshal(body, &chat); err != nil {
		return nil, fmt.Errorf("unmarshal chat response: %w", err)
	}

	// Fetch members for the chat to compute display name properly
	chat.Members = GetChatMembers(accessToken, chat.ID)

	return &chat, nil
}

// ---------------------------------------------------------------------------
// Optional Feature: User Presence (requires Presence.Read.All)
// ---------------------------------------------------------------------------

// UserPresence holds the availability and activity state of a user.
type UserPresence struct {
	Availability string `json:"availability"` // Available, Busy, Away, BeRightBack, DoNotDisturb, Offline, PresenceUnknown
	Activity     string `json:"activity"`     // InACall, InAMeeting, InAConferenceCall, etc.
}

// PresenceEntry holds the presence status for a single user, used in the chat presence popup.
type PresenceEntry struct {
	UserName     string
	Availability string
	Activity     string
}

// GetUserPresence fetches the presence status for a user by their Azure AD user ID.
// Returns an error if the token does not include Presence.Read.All.
func GetUserPresence(accessToken, userID string) (*UserPresence, error) {
	body, err := graphGet(accessToken, "/users/"+userID+"/presence")
	if err != nil {
		return nil, fmt.Errorf("GetUserPresence: %w", err)
	}
	var p UserPresence
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, fmt.Errorf("GetUserPresence: parse: %w", err)
	}
	return &p, nil
}

// GetUsersPresence fetches the presence status for multiple users by their Azure AD user IDs.
// Returns a map of userID -> UserPresence.
func GetUsersPresence(accessToken string, userIDs []string) (map[string]UserPresence, error) {
	if len(userIDs) == 0 {
		return make(map[string]UserPresence), nil
	}
	reqBody := struct {
		IDs []string `json:"ids"`
	}{
		IDs: userIDs,
	}
	body, err := graphPostWithResponse(accessToken, "/communications/getPresencesByUserId", reqBody)
	if err != nil {
		return nil, fmt.Errorf("GetUsersPresence: %w", err)
	}
	var res struct {
		Value []struct {
			ID           string `json:"id"`
			Availability string `json:"availability"`
			Activity     string `json:"activity"`
		} `json:"value"`
	}
	if err := json.Unmarshal(body, &res); err != nil {
		return nil, fmt.Errorf("GetUsersPresence: parse: %w", err)
	}
	m := make(map[string]UserPresence)
	for _, v := range res.Value {
		m[v.ID] = UserPresence{
			Availability: v.Availability,
			Activity:     v.Activity,
		}
	}
	return m, nil
}

// ---------------------------------------------------------------------------
// Optional Feature: User Profile (requires User.ReadBasic.All or User.Read.All)
// ---------------------------------------------------------------------------

// UserProfile holds extended profile information for a user.
type UserProfile struct {
	ID                string  `json:"id"`
	DisplayName       string  `json:"displayName"`
	Mail              *string `json:"mail,omitempty"`
	UserPrincipalName *string `json:"userPrincipalName,omitempty"`
	JobTitle          *string `json:"jobTitle,omitempty"`       // available with User.Read.All
	Department        *string `json:"department,omitempty"`     // available with User.Read.All
	OfficeLocation    *string `json:"officeLocation,omitempty"` // available with User.Read.All
	MobilePhone       *string `json:"mobilePhone,omitempty"`
}

// profileCache is an in-memory session cache to avoid redundant profile fetches.
var profileCache = make(map[string]*UserProfile)

// GetUserProfile fetches profile information for a user by their Azure AD user ID.
// Results are cached in memory for the duration of the session.
// Returns an error if the token does not include User.ReadBasic.All.
func GetUserProfile(accessToken, userID string) (*UserProfile, error) {
	if p, ok := profileCache[userID]; ok {
		return p, nil
	}
	body, err := graphGet(accessToken, "/users/"+userID+"?$select=id,displayName,mail,userPrincipalName,jobTitle,department,officeLocation,mobilePhone")
	if err != nil {
		return nil, fmt.Errorf("GetUserProfile: %w", err)
	}
	var p UserProfile
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, fmt.Errorf("GetUserProfile: parse: %w", err)
	}
	profileCache[userID] = &p
	return &p, nil
}

// ---------------------------------------------------------------------------
// Optional Feature: File Download (requires Files.Read)
// ---------------------------------------------------------------------------

// isSharePointURL returns true when the URL is a SharePoint/OneDrive file URL
// that must be resolved via the Shares API rather than fetched directly.
func isSharePointURL(fileURL string) bool {
	lower := strings.ToLower(fileURL)
	return strings.Contains(lower, "sharepoint.com") ||
		strings.Contains(lower, "1drv.ms") ||
		strings.Contains(lower, "onedrive.live.com")
}

// resolveSharePointDownloadURL takes a SharePoint/OneDrive contentUrl and
// returns a direct download URL by going through the Graph Shares API.
// Requires Files.Read (or Files.Read.All) permission.
func resolveSharePointDownloadURL(accessToken, shareURL string) (string, error) {
	// Encode the sharing URL as a base64 token required by the Shares API.
	// Format: "u!" + base64(url), then replace +→-, /→_, remove trailing =
	encoded := base64.StdEncoding.EncodeToString([]byte(shareURL))
	encoded = strings.TrimRight(encoded, "=")
	encoded = strings.ReplaceAll(encoded, "+", "-")
	encoded = strings.ReplaceAll(encoded, "/", "_")
	sharesPath := "/shares/u!" + encoded + "/driveItem"

	body, err := graphGet(accessToken, sharesPath)
	if err != nil {
		return "", fmt.Errorf("resolveSharePointDownloadURL: %w", err)
	}

	var item struct {
		ID              string `json:"id"`
		ParentReference struct {
			DriveID string `json:"driveId"`
		} `json:"parentReference"`
		DownloadURL string `json:"@microsoft.graph.downloadUrl"`
	}
	if err := json.Unmarshal(body, &item); err != nil {
		return "", fmt.Errorf("resolveSharePointDownloadURL: parse: %w", err)
	}

	// Prefer the pre-authenticated download URL if present.
	if item.DownloadURL != "" {
		return item.DownloadURL, nil
	}

	// Fall back to constructing the drive content URL.
	if item.ID != "" && item.ParentReference.DriveID != "" {
		return "/drives/" + item.ParentReference.DriveID + "/items/" + item.ID + "/content", nil
	}

	return "", fmt.Errorf("resolveSharePointDownloadURL: no download URL in response")
}

// DownloadFile downloads the content at the given URL (contentUrl from a message attachment)
// and writes it to destPath. Uses the Bearer token for authentication.
// For SharePoint reference attachments, automatically resolves via the Shares API.
func DownloadFile(accessToken, fileURL, destPath string) error {
	actualURL := fileURL

	if isSharePointURL(fileURL) {
		// SharePoint URLs must be resolved to a direct download URL first.
		resolved, err := resolveSharePointDownloadURL(accessToken, fileURL)
		if err != nil {
			return fmt.Errorf("DownloadFile: resolve SharePoint URL: %w", err)
		}
		// If resolved is a Graph API path (starts with /), prepend the base URL.
		if strings.HasPrefix(resolved, "/") {
			actualURL = graphAPIBase + resolved
		} else {
			actualURL = resolved
		}
	}

	req, err := http.NewRequest(http.MethodGet, actualURL, nil)
	if err != nil {
		return fmt.Errorf("DownloadFile: build request: %w", err)
	}
	// Only set the Bearer token for Graph API URLs; pre-authenticated
	// @microsoft.graph.downloadUrl URLs already contain credentials.
	if strings.Contains(actualURL, "graph.microsoft.com") {
		req.Header.Set("Authorization", "Bearer "+accessToken)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("DownloadFile: request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("DownloadFile: HTTP %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("DownloadFile: read body: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return fmt.Errorf("DownloadFile: create directories: %w", err)
	}
	return os.WriteFile(destPath, data, 0o600)
}

// ---------------------------------------------------------------------------
// Optional Feature: Teams Channels (requires Team.ReadBasic.All + Channel.ReadBasic.All)
// ---------------------------------------------------------------------------

// Team represents a Microsoft Teams team.
type Team struct {
	ID          string  `json:"id"`
	DisplayName string  `json:"displayName"`
	Description *string `json:"description,omitempty"`
}

// Channel represents a channel within a Team.
type Channel struct {
	ID          string  `json:"id"`
	DisplayName string  `json:"displayName"`
	Description *string `json:"description,omitempty"`
}

// TeamWithChannels holds a team and its list of channels.
type TeamWithChannels struct {
	Team     Team
	Channels []Channel
}

type teamsResponse struct {
	Value    []Team  `json:"value"`
	NextLink *string `json:"@odata.nextLink,omitempty"`
}

type channelsResponse struct {
	Value []Channel `json:"value"`
}

// GetJoinedTeams returns all Teams the current user is a member of.
// Requires Team.ReadBasic.All delegated permission.
func GetJoinedTeams(accessToken string) ([]Team, error) {
	body, err := graphGet(accessToken, "/me/joinedTeams?$select=id,displayName,description")
	if err != nil {
		return nil, fmt.Errorf("GetJoinedTeams: %w", err)
	}
	var r teamsResponse
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("GetJoinedTeams: parse: %w", err)
	}
	return r.Value, nil
}

// GetTeamChannels returns all channels in a given team.
// Requires Channel.ReadBasic.All delegated permission.
func GetTeamChannels(accessToken, teamID string) ([]Channel, error) {
	body, err := graphGet(accessToken, "/teams/"+teamID+"/channels?$select=id,displayName,description")
	if err != nil {
		return nil, fmt.Errorf("GetTeamChannels: %w", err)
	}
	var r channelsResponse
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("GetTeamChannels: parse: %w", err)
	}
	return r.Value, nil
}

// GetTeamsWithChannels fetches all joined teams and their channels in parallel.
// Requires Team.ReadBasic.All + Channel.ReadBasic.All delegated permissions.
func GetTeamsWithChannels(accessToken string) ([]TeamWithChannels, error) {
	teams, err := GetJoinedTeams(accessToken)
	if err != nil {
		return nil, err
	}

	type result struct {
		idx      int
		channels []Channel
		err      error
	}
	ch := make(chan result, len(teams))
	for i, t := range teams {
		go func(idx int, teamID string) {
			chans, err := GetTeamChannels(accessToken, teamID)
			ch <- result{idx: idx, channels: chans, err: err}
		}(i, t.ID)
	}

	teamsWithChannels := make([]TeamWithChannels, len(teams))
	for i, t := range teams {
		teamsWithChannels[i].Team = t
	}
	for range teams {
		r := <-ch
		if r.err == nil {
			teamsWithChannels[r.idx].Channels = r.channels
		}
	}
	return teamsWithChannels, nil
}

// GetChannelMessages fetches the most recent messages from a Teams channel,
// filters out system events, and fetches replies for each thread in parallel.
// Requires ChannelMessage.Read.All delegated permission.
func GetChannelMessages(accessToken, teamID, channelID string, top int) ([]Message, string, error) {
	pageSize := top
	if pageSize > 50 || pageSize <= 0 {
		pageSize = 50
	}
	path := fmt.Sprintf("/teams/%s/channels/%s/messages?$top=%d", teamID, channelID, pageSize)
	body, err := graphGet(accessToken, path)
	if err != nil {
		return nil, "", fmt.Errorf("GetChannelMessages: %w", err)
	}
	var r messagesResponse
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, "", fmt.Errorf("GetChannelMessages: parse: %w", err)
	}
	next := ""
	if r.NextLink != nil {
		next = *r.NextLink
	}

	// Filter out system events — only keep real "message" entries.
	var rootMsgs []Message
	for _, m := range r.Value {
		if m.MessageType == "" || m.MessageType == "message" {
			rootMsgs = append(rootMsgs, m)
		}
	}

	// Fetch replies for each root message in parallel.
	type replyResult struct {
		msgs []Message
	}
	replyCh := make(chan replyResult, len(rootMsgs))
	for _, rm := range rootMsgs {
		go func(msgID string) {
			replyPath := fmt.Sprintf("/teams/%s/channels/%s/messages/%s/replies?$top=50", teamID, channelID, msgID)
			rb, err := graphGet(accessToken, replyPath)
			if err != nil {
				replyCh <- replyResult{}
				return
			}
			var rr messagesResponse
			if err := json.Unmarshal(rb, &rr); err != nil {
				replyCh <- replyResult{}
				return
			}
			// Filter system events from replies and mark them as replies.
			var filtered []Message
			for _, m := range rr.Value {
				if m.MessageType == "" || m.MessageType == "message" {
					m.IsReply = true
					m.ReplyToID = msgID
					filtered = append(filtered, m)
				}
			}
			replyCh <- replyResult{msgs: filtered}
		}(rm.ID)
	}

	// Collect replies keyed by root message ID.
	replyMap := make(map[string][]Message, len(rootMsgs))
	for range rootMsgs {
		res := <-replyCh
		for _, r := range res.msgs {
			replyMap[r.ReplyToID] = append(replyMap[r.ReplyToID], r)
		}
	}

	// Sort root messages by last activity (the latest timestamp among the root and all its
	// replies), oldest-first. This ensures that a thread receiving a new reply is promoted
	// to the bottom of the visible channel view — consistent with Teams/Slack/Discord UX.
	lastActivity := func(root Message) string {
		ts := root.CreatedDateTime
		for _, r := range replyMap[root.ID] {
			if r.CreatedDateTime > ts {
				ts = r.CreatedDateTime
			}
		}
		return ts
	}
	sort.Slice(rootMsgs, func(i, j int) bool {
		return lastActivity(rootMsgs[i]) < lastActivity(rootMsgs[j])
	})

	// Build thread-grouped list: each root followed by its replies in chronological order.
	// The final slice is oldest-first; the UI iterates in reverse to render newest at bottom.
	var allMsgs []Message
	for _, root := range rootMsgs {
		allMsgs = append(allMsgs, root)
		replies := replyMap[root.ID]
		sort.Slice(replies, func(i, j int) bool {
			return replies[i].CreatedDateTime < replies[j].CreatedDateTime
		})
		allMsgs = append(allMsgs, replies...)
	}

	// Filter out non-downloadable attachments (like rich cards and URL previews)
	FilterMessagesAttachments(allMsgs)

	// Reverse so the slice is newest-first (matching the chat message convention;
	// the UI iterates from len-1 downward to render oldest at top, newest at bottom).
	for i, j := 0, len(allMsgs)-1; i < j; i, j = i+1, j-1 {
		allMsgs[i], allMsgs[j] = allMsgs[j], allMsgs[i]
	}

	return allMsgs, next, nil
}
