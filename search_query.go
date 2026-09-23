package main

import (
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
)

type searchQueryTerm struct {
	Field   string
	Value   string
	Negated bool
	Pattern *regexp.Regexp
}

type searchQuery struct {
	Terms []searchQueryTerm
}

type searchTarget struct {
	Text         []string
	Sender       []string
	Conversation []string
	Kind         string
	CreatedAt    time.Time
	Unread       bool
	Favorite     bool
	HasFile      bool
	HasImage     bool
	HasLink      bool
}

type MessageSearchResult struct {
	ChatID   string
	ChatName string
	Message  Message
	Score    int
}

var queryFields = map[string]bool{
	"from":   true,
	"in":     true,
	"is":     true,
	"type":   true,
	"has":    true,
	"after":  true,
	"before": true,
}

func tokenizeSearchQuery(input string) []string {
	var tokens []string
	var current strings.Builder
	quoted := false
	escaped := false
	flush := func() {
		if current.Len() > 0 {
			tokens = append(tokens, current.String())
			current.Reset()
		}
	}
	for _, r := range input {
		if escaped {
			current.WriteRune(r)
			escaped = false
			continue
		}
		if r == '\\' {
			escaped = true
			continue
		}
		if r == '"' {
			quoted = !quoted
			continue
		}
		if unicode.IsSpace(r) && !quoted {
			flush()
			continue
		}
		current.WriteRune(r)
	}
	if escaped {
		current.WriteRune('\\')
	}
	flush()
	return tokens
}

func parseSearchQuery(input string) searchQuery {
	query := searchQuery{}
	for _, raw := range tokenizeSearchQuery(input) {
		term := searchQueryTerm{}
		if strings.HasPrefix(raw, "-") && len(raw) > 1 {
			term.Negated = true
			raw = strings.TrimPrefix(raw, "-")
		}
		if field, value, found := strings.Cut(raw, ":"); found && queryFields[strings.ToLower(field)] && value != "" {
			term.Field = strings.ToLower(field)
			term.Value = value
		} else {
			term.Value = raw
		}
		term.Value = strings.TrimSpace(term.Value)
		if term.Value != "" {
			term.Pattern, _ = regexp.Compile(normalizedSearchText(term.Value))
			query.Terms = append(query.Terms, term)
		}
	}
	return query
}

func normalizedSearchText(text string) string {
	return normalizeString(strings.ToLower(strings.TrimSpace(text)))
}

// componentTextScore lets each component match literally or as a regexp, but
// deliberately does not support arbitrary character-subsequence matching.
func componentTextScore(text, component string) (int, bool) {
	pattern, _ := regexp.Compile(normalizedSearchText(component))
	return componentTextScoreWithPattern(text, component, pattern)
}

func componentTextScoreWithPattern(text, component string, pattern *regexp.Regexp) (int, bool) {
	haystack := normalizedSearchText(text)
	wanted := normalizedSearchText(component)
	if wanted == "" {
		return 0, true
	}
	if haystack == "" {
		return 0, false
	}
	if index := strings.Index(haystack, wanted); index >= 0 {
		score := 1200 - index*2 - max(0, len([]rune(haystack))-len([]rune(wanted)))
		if index == 0 {
			score += 100
		}
		if haystack == wanted {
			score += 200
		}
		return max(1, score), true
	}
	if pattern == nil {
		return 0, false
	}
	location := pattern.FindStringIndex(haystack)
	if location == nil {
		return 0, false
	}
	return max(1, 800-location[0]*2), true
}

func componentFieldsScore(fields []string, value string) (int, bool) {
	pattern, _ := regexp.Compile(normalizedSearchText(value))
	return componentFieldsScoreWithPattern(fields, value, pattern)
}

func componentFieldsScoreWithPattern(fields []string, value string, pattern *regexp.Regexp) (int, bool) {
	best := 0
	matched := false
	for _, field := range fields {
		if score, ok := componentTextScoreWithPattern(field, value, pattern); ok {
			matched = true
			if score > best {
				best = score
			}
		}
	}
	return best, matched
}

func normalizedSearchKind(kind string) string {
	switch strings.ToLower(kind) {
	case "oneonone", "direct", "1:1":
		return "direct"
	case "group":
		return "group"
	case "meeting":
		return "meeting"
	case "event", "systemeventmessage":
		return "event"
	default:
		return strings.ToLower(kind)
	}
}

func (term searchQueryTerm) match(target searchTarget) (int, bool) {
	value := normalizedSearchText(term.Value)
	switch term.Field {
	case "from":
		return componentFieldsScoreWithPattern(target.Sender, value, term.Pattern)
	case "in":
		return componentFieldsScoreWithPattern(target.Conversation, value, term.Pattern)
	case "is":
		switch value {
		case "unread":
			return 100, target.Unread
		case "read":
			return 100, !target.Unread
		case "favorite", "favourite":
			return 100, target.Favorite
		}
		return 0, false
	case "type":
		return 100, normalizedSearchKind(target.Kind) == normalizedSearchKind(value)
	case "has":
		switch value {
		case "file", "attachment":
			return 100, target.HasFile
		case "image":
			return 100, target.HasImage
		case "link":
			return 100, target.HasLink
		}
		return 0, false
	case "after", "before":
		day, err := time.ParseInLocation("2006-01-02", value, time.Local)
		if err != nil || target.CreatedAt.IsZero() {
			return 0, false
		}
		if term.Field == "after" {
			return 100, target.CreatedAt.After(day)
		}
		return 100, target.CreatedAt.Before(day)
	default:
		return componentFieldsScoreWithPattern(target.Text, value, term.Pattern)
	}
}

func (query searchQuery) Match(target searchTarget) (int, bool) {
	total := 0
	for _, term := range query.Terms {
		score, matched := term.match(target)
		if term.Negated {
			if matched {
				return 0, false
			}
			continue
		}
		if !matched {
			return 0, false
		}
		total += score
	}
	return total, true
}

func (query searchQuery) HighlightTerms() []string {
	var terms []string
	for _, term := range query.Terms {
		if !term.Negated && (term.Field == "" || term.Field == "from" || term.Field == "in") {
			terms = append(terms, term.Value)
		}
	}
	return terms
}

func searchTime(value string) time.Time {
	when, _ := time.Parse(time.RFC3339Nano, value)
	return when.Local()
}

func attachmentSearchState(attachments []MessageAttachment) (hasFile, hasImage, hasLink bool) {
	for _, attachment := range attachments {
		contentType := ""
		if attachment.ContentType != nil {
			contentType = strings.ToLower(*attachment.ContentType)
		}
		if !strings.EqualFold(contentType, "messageReference") {
			hasFile = true
		}
		if strings.HasPrefix(contentType, "image/") {
			hasImage = true
		}
		if attachment.ContentURL != nil && strings.TrimSpace(*attachment.ContentURL) != "" {
			hasLink = true
		}
	}
	return
}

func messageSearchTarget(message *Message, chat Chat, unread, favorite bool) searchTarget {
	name := ""
	if chat.CachedDisplayName != nil {
		name = *chat.CachedDisplayName
	}
	hasFile, hasImage, hasLink := attachmentSearchState(message.Attachments)
	body := message.GetPlainText()
	text := []string{body, message.Subject, message.Summary}
	for _, attachment := range message.Attachments {
		if attachment.Name != nil {
			text = append(text, *attachment.Name)
		}
	}
	if strings.Contains(strings.ToLower(body), "http://") || strings.Contains(strings.ToLower(body), "https://") {
		hasLink = true
	}
	kind := "message"
	if message.IsSystemEvent() {
		kind = "event"
	}
	return searchTarget{
		Text:         text,
		Sender:       []string{message.SenderName()},
		Conversation: []string{name, chat.ID},
		Kind:         kind,
		CreatedAt:    searchTime(message.CreatedDateTime),
		Unread:       unread,
		Favorite:     favorite,
		HasFile:      hasFile,
		HasImage:     hasImage,
		HasLink:      hasLink,
	}
}

func chatSearchTarget(chat Chat, unread, favorite bool) searchTarget {
	var text []string
	var senders []string
	var conversation []string
	if chat.CachedDisplayName != nil {
		text = append(text, *chat.CachedDisplayName)
		conversation = append(conversation, *chat.CachedDisplayName)
	}
	if chat.Topic != nil {
		text = append(text, *chat.Topic)
		conversation = append(conversation, *chat.Topic)
	}
	conversation = append(conversation, chat.ID)
	for _, member := range chat.Members {
		if member.DisplayName != nil {
			text = append(text, *member.DisplayName)
		}
		if member.Email != nil {
			text = append(text, *member.Email)
		}
	}
	createdAt := time.Time{}
	hasFile, hasImage, hasLink := false, false, false
	if chat.LastMessagePreview != nil {
		preview := chat.LastMessagePreview
		text = append(text, preview.GetPlainText(), preview.Subject, preview.Summary)
		senders = append(senders, preview.SenderName())
		createdAt = searchTime(preview.CreatedDateTime)
		hasFile, hasImage, hasLink = attachmentSearchState(preview.Attachments)
		body := strings.ToLower(preview.GetPlainText())
		if strings.Contains(body, "http://") || strings.Contains(body, "https://") {
			hasLink = true
		}
	} else if chat.LastUpdated != nil {
		createdAt = searchTime(*chat.LastUpdated)
	}
	return searchTarget{
		Text:         text,
		Sender:       senders,
		Conversation: conversation,
		Kind:         chat.ChatType,
		CreatedAt:    createdAt,
		Unread:       unread,
		Favorite:     favorite,
		HasFile:      hasFile,
		HasImage:     hasImage,
		HasLink:      hasLink,
	}
}

// chatNameSearchTarget intentionally excludes member names and message text.
// Global search uses it for the first, highest-priority result section.
func chatNameSearchTarget(chat Chat, unread, favorite bool) searchTarget {
	target := chatSearchTarget(chat, unread, favorite)
	target.Text = nil
	target.Conversation = nil
	if chat.CachedDisplayName != nil {
		target.Text = append(target.Text, *chat.CachedDisplayName)
		target.Conversation = append(target.Conversation, *chat.CachedDisplayName)
	}
	if chat.Topic != nil {
		target.Text = append(target.Text, *chat.Topic)
		target.Conversation = append(target.Conversation, *chat.Topic)
	}
	target.Conversation = append(target.Conversation, chat.ID)
	return target
}

// chatParticipantSearchTarget includes chat identity plus participant names and
// addresses, while still excluding message bodies. This keeps message-content
// hits in their own lower-priority section.
func chatParticipantSearchTarget(chat Chat, unread, favorite bool) searchTarget {
	target := chatNameSearchTarget(chat, unread, favorite)
	for _, member := range chat.Members {
		if member.DisplayName != nil {
			target.Text = append(target.Text, *member.DisplayName)
		}
		if member.Email != nil {
			target.Text = append(target.Text, *member.Email)
		}
	}
	return target
}

func sortMessageSearchResults(results []MessageSearchResult) {
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].Score != results[j].Score {
			return results[i].Score > results[j].Score
		}
		return searchTime(results[i].Message.CreatedDateTime).After(searchTime(results[j].Message.CreatedDateTime))
	})
}
