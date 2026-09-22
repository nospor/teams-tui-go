package main

import (
	"strings"
	"testing"
)

func TestMarkdownToHTMLWithLinks(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Plain text, no links",
			input:    "Hello world",
			expected: "<p>Hello world</p>",
		},
		{
			name:     "Plain URL",
			input:    "Go to https://google.com",
			expected: `<p>Go to <a href="https://google.com">https://google.com</a></p>`,
		},
		{
			name:     "Plain URL with trailing dot",
			input:    "Go to https://google.com.",
			expected: `<p>Go to <a href="https://google.com">https://google.com</a>.</p>`,
		},
		{
			name:     "Plain URL with trailing comma",
			input:    "Go to https://google.com, which is cool",
			expected: `<p>Go to <a href="https://google.com">https://google.com</a>, which is cool</p>`,
		},
		{
			name:     "Plain URL with query parameters",
			input:    "Check https://google.com/search?q=query&hl=en",
			expected: `<p>Check <a href="https://google.com/search?q=query&amp;hl=en">https://google.com/search?q=query&amp;hl=en</a></p>`,
		},
		{
			name:     "Markdown link",
			input:    "Click [Google](https://google.com)",
			expected: `<p>Click <a href="https://google.com">Google</a></p>`,
		},
		{
			name:     "Mixed markdown link and plain URL",
			input:    "Click [Google](https://google.com) or go to https://github.com.",
			expected: `<p>Click <a href="https://google.com">Google</a> or go to <a href="https://github.com">https://github.com</a>.</p>`,
		},
		{
			name:     "URL inside backticks (inline code)",
			input:    "Do not linkify `https://google.com` here",
			expected: `<p>Do not linkify <code>https://google.com</code> here</p>`,
		},
		{
			name:     "URL inside multiline code block",
			input:    "```go\n// https://google.com\nfmt.Println(\"ok\")\n```",
			expected: `<pre><code class="language-go">// https://google.com
fmt.Println(&#34;ok&#34;)</code></pre>`,
		},
		{
			name:     "URL with matching parenthesis",
			input:    "Look at https://en.wikipedia.org/wiki/URL_(web_address)",
			expected: `<p>Look at <a href="https://en.wikipedia.org/wiki/URL_(web_address)">https://en.wikipedia.org/wiki/URL_(web_address)</a></p>`,
		},
		{
			name:     "URL inside outer parenthesis",
			input:    "Please look here (https://google.com)",
			expected: `<p>Please look here (<a href="https://google.com">https://google.com</a>)</p>`,
		},
		{
			name:     "URL with brackets",
			input:    "Look at [https://google.com]",
			expected: `<p>Look at [<a href="https://google.com">https://google.com</a>]</p>`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := markdownToHTML(tc.input)
			if got != tc.expected {
				t.Errorf("\nInput:    %s\nExpected: %s\nGot:      %s", tc.input, tc.expected, got)
			}
		})
	}
}

func TestHTMLToMarkdownConsecutiveURLs(t *testing.T) {
	// Teams HTML often contains formatting newlines between block elements.
	html := `<p>Hi Alex, some questions in tickets</p>
<p>&nbsp;</p>
<p><a href="https://tracker.example.com/issue/PROJ-101">https://tracker.example.com/issue/PROJ-101</a></p>
<p><a href="https://tracker.example.com/issue/PROJ-102">https://tracker.example.com/issue/PROJ-102</a></p>
<p><a href="https://tracker.example.com/issue/PROJ-103">https://tracker.example.com/issue/PROJ-103</a></p>`

	expected := "Hi Alex, some questions in tickets\n\nhttps://tracker.example.com/issue/PROJ-101\nhttps://tracker.example.com/issue/PROJ-102\nhttps://tracker.example.com/issue/PROJ-103"

	got := HTMLToMarkdown(html, nil)
	if got != expected {
		t.Errorf("\nExpected:\n%s\n\nGot:\n%s", expected, got)
	}
}

func TestHTMLToMarkdownPreservesIntentionalBlankLines(t *testing.T) {
	tests := []struct {
		name     string
		html     string
		expected string
	}{
		{
			name:     "nbsp empty paragraph",
			html:     `<p>line above</p><p>&nbsp;</p><p>line below</p>`,
			expected: "line above\n\nline below",
		},
		{
			name:     "plain-space empty paragraph",
			html:     `<p>line above</p><p> </p><p>line below</p>`,
			expected: "line above\n\nline below",
		},
		{
			name:     "bold and blank line",
			html:     `<p><b>Title</b></p><p>&nbsp;</p><p>Body text</p>`,
			expected: "**Title**\n\nBody text",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := HTMLToMarkdown(tc.html, nil)
			if got != tc.expected {
				t.Errorf("\nExpected:\n%s\n\nGot:\n%s", tc.expected, got)
			}
		})
	}
}

func TestHTMLToMarkdownInlineImages(t *testing.T) {
	html := `<p>before <img src="https://graph.microsoft.com/v1.0/chats/123/messages/456/hostedContents/abc/$value" alt="shot" /> after</p>`
	got := HTMLToMarkdown(html, nil)
	if !strings.Contains(got, "[Image 1]") {
		t.Fatalf("expected [Image 1] placeholder, got %q", got)
	}
	if strings.Contains(got, "<img") {
		t.Fatalf("expected img tag converted to placeholder, got %q", got)
	}
}

func TestHTMLToMarkdownReferenceAttachment(t *testing.T) {
	strPtr := func(s string) *string { return &s }
	refType := "reference"
	attachments := []MessageAttachment{
		{
			ID:          "file-guid-1",
			Name:        strPtr("CustomFiles2026.docx"),
			ContentType: &refType,
		},
		{
			ID:          "file-guid-2",
			Name:        strPtr("Custom Files 2026 to share.xlsx"),
			ContentType: &refType,
		},
	}
	html := `<p>one  more  test</p><attachment id="file-guid-1"></attachment><attachment id="file-guid-2"></attachment>`
	got := HTMLToMarkdown(html, attachments)
	if !strings.Contains(got, "[File: CustomFiles2026.docx]") {
		t.Fatalf("expected first file placeholder, got %q", got)
	}
	if !strings.Contains(got, "[File: Custom Files 2026 to share.xlsx]") {
		t.Fatalf("expected second file placeholder, got %q", got)
	}
	if strings.Contains(got, "  more") || strings.Contains(got, "  test") {
		t.Fatalf("expected gaps replaced by placeholders, got %q", got)
	}
}

func TestRestoreFilePlaceholdersInText(t *testing.T) {
	got := restoreFilePlaceholdersInText("one  more  test", []string{"a.docx", "b.xlsx"})
	want := "one [File: a.docx] more [File: b.xlsx] test"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestWrapBodyHTMLForInlineAttachments(t *testing.T) {
	in := `hello <attachment id="abc"></attachment> world`
	got := wrapBodyHTMLForInlineAttachments(in)
	if !strings.HasPrefix(got, "<p>") || !strings.HasSuffix(got, "</p>") {
		t.Fatalf("expected paragraph wrapper, got %q", got)
	}
}

func TestHTMLToMarkdownURLRoundTrip(t *testing.T) {
	input := "Hi Alex, ticket links\n\nhttps://tracker.example.com/issue/PROJ-101\nhttps://tracker.example.com/issue/PROJ-102\nhttps://tracker.example.com/issue/PROJ-103"

	got := HTMLToMarkdown(markdownToHTML(input), nil)
	if got != input {
		t.Errorf("\nRound-trip changed content.\nExpected:\n%s\n\nGot:\n%s", input, got)
	}
}

func TestContainsURL(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"http://google.com", true},
		{"https://github.com", true},
		{"Hello http://world", true},
		{"Hello", false},
		{"ftp://files.com", false},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got := containsURL(tc.input)
			if got != tc.expected {
				t.Errorf("containsURL(%q) = %v; expected %v", tc.input, got, tc.expected)
			}
		})
	}
}
