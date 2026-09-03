package main

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func TestNormalizeString(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Jérémy", "Jeremy"},
		{"François", "Francois"},
		{"jeremy", "jeremy"},
		{"", ""},
		{"München", "Munchen"},
		{"Álvaro", "Alvaro"},
	}

	for _, test := range tests {
		result := normalizeString(test.input)
		if result != test.expected {
			t.Errorf("normalizeString(%q) = %q; expected %q", test.input, result, test.expected)
		}
	}
}

func TestHighlightQuery(t *testing.T) {
	// Force a color profile so that lipgloss outputs ANSI sequences during headless tests.
	lipgloss.SetColorProfile(termenv.TrueColor)

	tests := []struct {
		text     string
		query    string
		contains string // what the highlighted text should contain
	}{
		{"Hello Jérémy", "jeremy", "Jérémy"},
		{"Hello Jeremy", "jeremy", "Jeremy"},
		{"François is here", "francois", "François"},
		{"No match here", "jeremy", "No match here"},
	}

	for _, test := range tests {
		result := highlightQuery(test.text, test.query)
		if test.text == test.contains {
			// If it's a "No match here", result should be identical to input
			if result != test.text {
				t.Errorf("highlightQuery(%q, %q) = %q; expected no match", test.text, test.query, result)
			}
		} else {
			// If it matches, the original substring (with accents) should be in the result,
			// wrapped in some ANSI escape codes.
			if !strings.Contains(result, test.contains) {
				t.Errorf("highlightQuery(%q, %q) = %q; expected to contain %q", test.text, test.query, result, test.contains)
			}
			// And it should not be identical to original text
			if result == test.text {
				t.Errorf("highlightQuery(%q, %q) = %q; expected highlighted output, got original text", test.text, test.query, result)
			}
		}
	}
}

func TestMessageMatches(t *testing.T) {
	strPtr := func(s string) *string { return &s }

	msg := Message{
		ID: "1",
		From: &MessageFrom{
			User: &MessageUser{
				DisplayName: strPtr("Alice Smith"),
			},
		},
		Body: &MessageBody{
			Content: strPtr("<p>Hello world from Go</p>"),
		},
		Attachments: []MessageAttachment{
			{
				Name: strPtr("report.pdf"),
			},
		},
	}

	model := Model{}

	tests := []struct {
		query    string
		expected bool
	}{
		{"", true},
		{"hello", true},
		{"from Go", true},
		{"report", true},
		{"report.pdf", true},
		{"Alice", false}, // Should NOT match creator display name
		{"Smith", false}, // Should NOT match creator display name
		{"nonexistent", false},
	}

	for _, test := range tests {
		result := model.messageMatches(&msg, test.query)
		if result != test.expected {
			t.Errorf("messageMatches(query=%q) = %v; expected %v", test.query, result, test.expected)
		}
	}
}

func TestExtractAndProcessInlineImages(t *testing.T) {
	strPtr := func(s string) *string { return &s }

	htmlContent := `<p>Check out this image: <img src="https://graph.microsoft.com/v1.0/chats/123/messages/456/hostedContents/abc/$value" alt="My screenshot" /> and another: <img src="https://graph.microsoft.com/v1.0/chats/123/messages/456/hostedContents/def/$value" /></p>`

	// Test ExtractInlineImages
	inlineAtts := ExtractInlineImages(htmlContent)
	if len(inlineAtts) != 2 {
		t.Fatalf("expected 2 inline images, got %d", len(inlineAtts))
	}

	if *inlineAtts[0].Name != "My screenshot.png" {
		t.Errorf("expected first name 'My screenshot.png', got %q", *inlineAtts[0].Name)
	}
	if *inlineAtts[0].ContentURL != "https://graph.microsoft.com/v1.0/chats/123/messages/456/hostedContents/abc/$value" {
		t.Errorf("expected first URL to match, got %q", *inlineAtts[0].ContentURL)
	}

	if *inlineAtts[1].Name != "inline-image-2.png" {
		t.Errorf("expected second name 'inline-image-2.png', got %q", *inlineAtts[1].Name)
	}
	if *inlineAtts[1].ContentURL != "https://graph.microsoft.com/v1.0/chats/123/messages/456/hostedContents/def/$value" {
		t.Errorf("expected second URL to match, got %q", *inlineAtts[1].ContentURL)
	}

	// Test ProcessInlineImages
	msg := Message{
		ID: "1",
		Body: &MessageBody{
			Content: strPtr(htmlContent),
		},
		Attachments: []MessageAttachment{
			{
				ID:         "existing-doc",
				Name:       strPtr("report.pdf"),
				ContentURL: strPtr("https://sharepoint.com/report.pdf"),
			},
		},
	}

	msg.ProcessInlineImages()

	if len(msg.Attachments) != 3 {
		t.Errorf("expected 3 attachments after processing inline images, got %d", len(msg.Attachments))
	}

	// Double processing check
	msg.ProcessInlineImages()
	if len(msg.Attachments) != 3 {
		t.Errorf("expected attachments count to remain 3 on double processing, got %d", len(msg.Attachments))
	}

	// Test HTMLToText inline image naming
	plainText := HTMLToText(htmlContent, msg.Attachments, nil, nil)
	if !strings.Contains(plainText, "My screenshot.png") {
		t.Errorf("expected plainText to contain 'My screenshot.png', got %q", plainText)
	}
	if !strings.Contains(plainText, "inline-image-2.png") {
		t.Errorf("expected plainText to contain 'inline-image-2.png', got %q", plainText)
	}
}

func TestHTMLToTextMentions(t *testing.T) {
	// Force color profile for testing ANSI codes
	oldProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(oldProfile)


	// Helper functions for pointers
	intPtr := func(v int) *int { return &v }
	stringPtr := func(v string) *string { return &v }

	// Case 1: Mention without '@' prefix
	html1 := `Hello <at id="0">John Doe</at>!`
	res1 := HTMLToText(html1, nil, nil, nil)
	plain1 := stripANSI(res1)
	expected1 := "Hello @John Doe!"
	if plain1 != expected1 {
		t.Errorf("expected %q, got %q", expected1, plain1)
	}
	// Check that ANSI styling was applied
	if !strings.Contains(res1, "\x1b[") {
		t.Errorf("expected res1 to contain ANSI escape codes, got %q", res1)
	}

	// Case 2: Mention that already starts with '@'
	html2 := `Hello <at id="1">@Jane Doe</at>!`
	res2 := HTMLToText(html2, nil, nil, nil)
	plain2 := stripANSI(res2)
	expected2 := "Hello @Jane Doe!"
	if plain2 != expected2 {
		t.Errorf("expected %q, got %q", expected2, plain2)
	}

	// Case 3: Mention with non-breaking space
	html3 := `Hello <at id="2">John&nbsp;Doe</at>!`
	res3 := HTMLToText(html3, nil, nil, nil)
	plain3 := stripANSI(res3)
	expected3 := "Hello @John\u00a0Doe!" // \u00a0 is nbsp
	if plain3 != expected3 {
		t.Errorf("expected %q, got %q", expected3, plain3)
	}

	// Case 4: Mention split into two tags with same ID
	html4 := `Hello <at id="0">John</at> <at id="0">Doe</at>!`
	res4 := HTMLToText(html4, nil, nil, nil)
	plain4 := stripANSI(res4)
	expected4 := "Hello @John Doe!"
	if plain4 != expected4 {
		t.Errorf("expected %q, got %q", expected4, plain4)
	}

	// Case 5: Mention split into two tags with different IDs but same user ID
	html5 := `Hello <at id="0">John</at> <at id="1">Doe</at>!`
	mentions5 := []MessageMention{
		{
			ID:          intPtr(0),
			MentionText: stringPtr("John"),
			Mentioned: &MentionedIdentitySet{
				User: &MessageUser{
					ID:          stringPtr("user-123"),
					DisplayName: stringPtr("John Doe"),
				},
			},
		},
		{
			ID:          intPtr(1),
			MentionText: stringPtr("Doe"),
			Mentioned: &MentionedIdentitySet{
				User: &MessageUser{
					ID:          stringPtr("user-123"),
					DisplayName: stringPtr("John Doe"),
				},
			},
		},
	}
	res5 := HTMLToText(html5, nil, mentions5, nil)
	plain5 := stripANSI(res5)
	expected5 := "Hello @John Doe!"
	if plain5 != expected5 {
		t.Errorf("expected %q, got %q", expected5, plain5)
	}
}

func TestHTMLToTextStrikethrough(t *testing.T) {
	// Force color profile for testing ANSI codes.
	oldProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(oldProfile)

	// Single strikethrough span: must be exactly one SGR 9 pair, not
	// per-character toggling.
	single := HTMLToText(`<p>closed <s>SRDS-73 </s>.</p>`, nil, nil, nil)
	if strings.Count(single, "\x1b[9m") != 1 {
		t.Errorf("expected 1 strikethrough opening, got %d in %q", strings.Count(single, "\x1b[9m"), single)
	}
	if strings.Count(single, "\x1b[0m") != 1 {
		t.Errorf("expected 1 reset, got %d in %q", strings.Count(single, "\x1b[0m"), single)
	}
	if strings.Contains(single, "\x1b[0m\x1b[9m") {
		t.Errorf("found per-character strikethrough toggling in %q", single)
	}

	// Strikethrough nested inside a hyperlink (e.g. a closed ticket). The
	// link styling must not corrupt the embedded strikethrough sequence.
	linked := HTMLToText(`<p>closed <a href="https://example.com/SRDS-73"><s>SRDS-73 </s></a>.</p>`, nil, nil, nil)
	if strings.Contains(linked, "\x1b[0m\x1b[9m") {
		t.Errorf("found per-character toggling in %q", linked)
	}
	if strings.Contains(linked, "\x1b[0m\x1b[4") {
		t.Errorf("found per-character link toggling in %q", linked)
	}
	// The strikethrough + underline must be a single combined SGR sequence.
	if !strings.Contains(linked, "\x1b[9;4;") && !strings.Contains(linked, "\x1b[4;") {
		t.Errorf("expected a combined strikethrough+underline sequence in %q", linked)
	}

	// Multiple spans (as in the real "SRDS-73" message) still strip cleanly.
	multi := HTMLToText(`<p>I see you have closed <s>SRDS-73 </s><s>.</s> Is that all?</p>`, nil, nil, nil)
	plain := stripANSI(multi)
	expected := "I see you have closed SRDS-73 . Is that all?"
	if plain != expected {
		t.Errorf("expected %q, got %q", expected, plain)
	}
}

func TestComputeDisplayName(t *testing.T) {
	strPtr := func(s string) *string { return &s }

	tests := []struct {
		name     string
		chat     Chat
		expected string
	}{
		{
			name: "One-on-one chat",
			chat: Chat{
				ChatType: "oneOnOne",
				Members: []ChatMember{
					{DisplayName: strPtr("Alice Smith")},
				},
			},
			expected: "Alice Smith",
		},
		{
			name: "Group chat with topic",
			chat: Chat{
				ChatType: "group",
				Topic:    strPtr("Project Alpha"),
				Members: []ChatMember{
					{DisplayName: strPtr("Alice Smith")},
					{DisplayName: strPtr("Bob Jones")},
				},
			},
			expected: "Project Alpha",
		},
		{
			name: "Group chat with 2 members",
			chat: Chat{
				ChatType: "group",
				Members: []ChatMember{
					{DisplayName: strPtr("Alice Smith")},
					{DisplayName: strPtr("Bob Jones")},
				},
			},
			expected: "Alice S, Bob J",
		},
		{
			name: "Group chat with 3 members",
			chat: Chat{
				ChatType: "group",
				Members: []ChatMember{
					{DisplayName: strPtr("Alice Smith")},
					{DisplayName: strPtr("Bob Jones")},
					{DisplayName: strPtr("Charlie Brown")},
				},
			},
			expected: "Alice S, Bob J, Charlie B",
		},
		{
			name: "Group chat with 4 members",
			chat: Chat{
				ChatType: "group",
				Members: []ChatMember{
					{DisplayName: strPtr("Alice Smith")},
					{DisplayName: strPtr("Bob Jones")},
					{DisplayName: strPtr("Charlie Brown")},
					{DisplayName: strPtr("David Miller")},
				},
			},
			expected: "Alice S, Bob J, Charlie B ...",
		},
		{
			name: "Group chat with some nil DisplayNames",
			chat: Chat{
				ChatType: "group",
				Members: []ChatMember{
					{DisplayName: strPtr("Alice Smith")},
					{DisplayName: nil},
					{DisplayName: strPtr("Bob Jones")},
				},
			},
			expected: "Alice S, Bob J",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := computeDisplayName(&tt.chat)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestFilterMessageAttachments(t *testing.T) {
	strPtr := func(s string) *string { return &s }

	cardType := "application/vnd.microsoft.card.thumbnail"
	referenceType := "reference"
	imageType := "image/png"
	messageRefType := "messageReference"

	msg := Message{
		Attachments: []MessageAttachment{
			{
				ID:          "1",
				ContentType: &cardType,
				ContentURL:  strPtr("https://www.youtube.com/watch?v=123"),
			},
			{
				ID:          "2",
				ContentType: &referenceType,
				ContentURL:  strPtr("https://some-tenant.sharepoint.com/personal/file.docx"),
			},
			{
				ID:          "3",
				ContentType: &referenceType,
				ContentURL:  strPtr("https://www.youtube.com/watch?v=456"),
			},
			{
				ID:          "4",
				ContentType: &imageType,
				ContentURL:  strPtr("https://graph.microsoft.com/v1.0/chats/inline-img"),
			},
			{
				ID:          "5",
				ContentType: &messageRefType,
			},
		},
	}

	FilterMessageAttachments(&msg)

	if len(msg.Attachments) != 3 {
		t.Fatalf("expected 3 attachments after filtering, got %d", len(msg.Attachments))
	}

	expectedIDs := map[string]bool{"2": true, "4": true, "5": true}
	for _, att := range msg.Attachments {
		if !expectedIDs[att.ID] {
			t.Errorf("unexpected attachment ID remaining: %s", att.ID)
		}
	}
}

func TestGetAttachmentSavedName(t *testing.T) {
	strPtr := func(s string) *string { return &s }

	tests := []struct {
		name        string
		att         MessageAttachment
		defaultName string
		expected    string
	}{
		{
			name: "Pasted image with ID and extension",
			att: MessageAttachment{
				ID:   "inline-img-1",
				Name: strPtr("image.png"),
			},
			defaultName: "image",
			expected:    "image_inline-img-1.png",
		},
		{
			name: "Attachment with custom name and numeric ID",
			att: MessageAttachment{
				ID:   "123",
				Name: strPtr("screenshot.jpg"),
			},
			defaultName: "image",
			expected:    "screenshot_123.jpg",
		},
		{
			name: "Attachment with nil name using default name and ID",
			att: MessageAttachment{
				ID: "att-456",
			},
			defaultName: "attachment",
			expected:    "attachment_att-456",
		},
		{
			name: "Stem already has ID suffix",
			att: MessageAttachment{
				ID:   "123",
				Name: strPtr("image_123.png"),
			},
			defaultName: "image",
			expected:    "image_123.png",
		},
		{
			name: "No ID but has ContentURL",
			att: MessageAttachment{
				Name:       strPtr("file.txt"),
				ContentURL: strPtr("https://example.com/file.txt"),
			},
			defaultName: "attachment",
			// URL sha256 prefix will be appended
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getAttachmentSavedName(tt.att, tt.defaultName)
			if tt.expected != "" && got != tt.expected {
				t.Errorf("getAttachmentSavedName() = %q, expected %q", got, tt.expected)
			}
			if tt.expected == "" && got == "file.txt" {
				t.Errorf("getAttachmentSavedName() expected unique hash appended, got %q", got)
			}
		})
	}
}

func TestGetMentionQuery(t *testing.T) {
	tests := []struct {
		name       string
		val        string
		cursor     int
		ok         bool
		wantStart  int
		wantQuery  string
	}{
		{
			name:      "email at end of message",
			val:       "PUB, ADV\u2014 Jamie Vernon, Publisher, TF: 800-282-0444 Ext. 223, E-mail jvernon@amsci.org",
			cursor:    86,
			ok:        false,
		},
		{
			name:      "mention after space",
			val:       "hello @jane",
			cursor:    11,
			ok:        true,
			wantStart: 6,
			wantQuery: "jane",
		},
		{
			name:      "mention at start of string",
			val:       "@jane",
			cursor:    5,
			ok:        true,
			wantStart: 0,
			wantQuery: "jane",
		},
		{
			name:      "mention at very end",
			val:       "hi @",
			cursor:    4,
			ok:        true,
			wantStart: 3,
			wantQuery: "",
		},
		{
			name:      "at sign embedded mid-word",
			val:       "jvernon@amsci.org",
			cursor:    16,
			ok:        false,
		},
		{
			name:      "at sign preceded by punctuation",
			val:       "(@jane",
			cursor:    6,
			ok:        false,
		},
		{
			name:      "no at sign",
			val:       "plain text",
			cursor:    10,
			ok:        false,
		},
		{
			name:      "email then mention",
			val:       "mail me@x.com then @jane",
			cursor:    24,
			ok:        true,
			wantStart: 19,
			wantQuery: "jane",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, query, ok := getMentionQuery(tt.val, tt.cursor)
			if ok != tt.ok {
				t.Errorf("getMentionQuery() ok = %v, expected %v", ok, tt.ok)
			}
			if tt.ok {
				if start != tt.wantStart {
					t.Errorf("getMentionQuery() start = %d, expected %d", start, tt.wantStart)
				}
				if query != tt.wantQuery {
					t.Errorf("getMentionQuery() query = %q, expected %q", query, tt.wantQuery)
				}
			}
		})
	}
}

func TestRenderForwardedMessageReference(t *testing.T) {
	content := `{"originalMessageId":"1727881360458","originalMessageContent":"\n<p>hello world</p>\n","originalConversationId":"chat-planning","originalSentDateTime":"2024-10-02T15:02:40.458+00:00","originalMessageSender":{"user":{"displayName":"Jane Smith"}}}`

	quote := renderForwardedMessageReference(content, nil)
	if quote == "" {
		t.Fatal("expected non-empty quote")
	}
	if !strings.Contains(stripANSI(quote), "hello world") {
		t.Errorf("expected preview text in quote, got %q", stripANSI(quote))
	}
	if !strings.Contains(stripANSI(quote), "Jane Smith") {
		t.Errorf("expected sender in quote, got %q", stripANSI(quote))
	}

	chatNames := map[string]string{"chat-planning": "SRDS Planning"}
	quoteWithChat := renderForwardedMessageReference(content, chatNames)
	if !strings.Contains(stripANSI(quoteWithChat), "SRDS Planning") {
		t.Errorf("expected chat name in quote, got %q", stripANSI(quoteWithChat))
	}
}

func TestHTMLToTextForwardedMessageReference(t *testing.T) {
	strPtr := func(s string) *string { return &s }
	fwdType := "forwardedMessageReference"
	attID := "1727881360458"
	content := `{"originalMessageId":"1727881360458","originalMessageContent":"<p>custom FTP accounts</p>","originalConversationId":"chat-1","originalSentDateTime":"2024-09-02T19:25:00+00:00","originalMessageSender":{"user":{"displayName":"Jun"}}}`

	html := `<p>Hi All, see below:</p><attachment id="1727881360458"></attachment>`
	attachments := []MessageAttachment{{
		ID:          attID,
		ContentType: &fwdType,
		Content:     strPtr(content),
	}}

	text := stripANSI(HTMLToText(html, attachments, nil, map[string]string{"chat-1": "Other Chat"}))
	if !strings.Contains(text, "Hi All") {
		t.Errorf("expected main message text, got %q", text)
	}
	if !strings.Contains(text, "custom FTP accounts") {
		t.Errorf("expected forwarded preview, got %q", text)
	}
	if !strings.Contains(text, "Jun") {
		t.Errorf("expected forwarded sender, got %q", text)
	}
	if !strings.Contains(text, "Other Chat") {
		t.Errorf("expected source chat name, got %q", text)
	}
	if strings.Contains(text, "📎") {
		t.Errorf("did not expect generic attachment marker, got %q", text)
	}
}


