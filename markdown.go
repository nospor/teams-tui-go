package main

import (
	"fmt"
	"html"
	"regexp"
	"strings"

	golanghtml "golang.org/x/net/html"
)

// ---------------------------------------------------------------------------
// Markdown → HTML (send side)
// ---------------------------------------------------------------------------

var (
	bulletListRe  = regexp.MustCompile(`^[*\-]\s+(.*)`)
	orderedListRe = regexp.MustCompile(`^\d+[.)]\s+(.*)`)

	// Inline patterns – order matters: code spans first to protect content.
	inlineRe = regexp.MustCompile(
		"`([^`]+)`" + // 1: `code`
			`|\*\*(.+?)\*\*` + // 2: **bold**
			`|__(.+?)__` + // 3: __bold__
			`|\*(.+?)\*` + // 4: *italic*
			`|_(.+?)_` + // 5: _italic_
			`|~~(.+?)~~` + // 6: ~~strikethrough~~
			`|\[([^\]]+)\]\((https?://[^\s<>"\)]+)\)` + // 7: [text](url)
			`|(https?://[a-zA-Z0-9\-._~:/?#\[\]@!$&'()*+,;%=]+)`, // 8: plain URL
	)
)

// markdownToHTML converts a user-typed message with common markdown syntax
// into Teams-compatible HTML for the Graph API body payload.
func markdownToHTML(content string) string {
	lines := strings.Split(content, "\n")
	var out strings.Builder
	inCodeBlock := false
	var codeBlockBuf strings.Builder
	codeBlockLang := ""
	inUL := false
	inOL := false

	for _, line := range lines {
		// ---- fenced code block ----
		if strings.HasPrefix(line, "```") {
			if !inCodeBlock {
				// close any open list before entering a code block
				if inUL {
					out.WriteString("</ul>")
					inUL = false
				}
				if inOL {
					out.WriteString("</ol>")
					inOL = false
				}
				inCodeBlock = true
				codeBlockLang = strings.TrimSpace(strings.TrimPrefix(line, "```"))
				codeBlockBuf.Reset()
			} else {
				inCodeBlock = false
				code := strings.TrimRight(codeBlockBuf.String(), "\n")
				escaped := html.EscapeString(code)
				if codeBlockLang != "" {
					out.WriteString(`<pre><code class="language-` + html.EscapeString(codeBlockLang) + `">` + escaped + `</code></pre>`)
				} else {
					out.WriteString("<pre><code>" + escaped + "</code></pre>")
				}
				codeBlockLang = ""
			}
			continue
		}
		if inCodeBlock {
			codeBlockBuf.WriteString(line + "\n")
			continue
		}

		// ---- list items ----
		isBullet := bulletListRe.MatchString(line)
		isOrdered := orderedListRe.MatchString(line)

		if !isBullet && inUL {
			out.WriteString("</ul>")
			inUL = false
		}
		if !isOrdered && inOL {
			out.WriteString("</ol>")
			inOL = false
		}

		switch {
		case isBullet:
			if !inUL {
				out.WriteString("<ul>")
				inUL = true
			}
			m := bulletListRe.FindStringSubmatch(line)
			text := ""
			if len(m) > 1 {
				text = m[1]
			}
			out.WriteString("<li>" + inlineMarkdownToHTML(text) + "</li>")

		case isOrdered:
			if !inOL {
				out.WriteString("<ol>")
				inOL = true
			}
			m := orderedListRe.FindStringSubmatch(line)
			text := ""
			if len(m) > 1 {
				text = m[1]
			}
			out.WriteString("<li>" + inlineMarkdownToHTML(text) + "</li>")

		case strings.TrimSpace(line) == "":
			out.WriteString("<p>&nbsp;</p>")

		default:
			out.WriteString("<p>" + inlineMarkdownToHTML(line) + "</p>")
		}
	}

	// close any open lists at end of input
	if inUL {
		out.WriteString("</ul>")
	}
	if inOL {
		out.WriteString("</ol>")
	}

	return out.String()
}

// inlineMarkdownToHTML converts inline markdown within a single line to HTML.
// Plain-text portions are HTML-escaped; markdown spans are converted to tags.
func inlineMarkdownToHTML(s string) string {
	var out strings.Builder
	lastIdx := 0
	matches := inlineRe.FindAllStringSubmatchIndex(s, -1)

	for _, m := range matches {
		// write plain text before this match
		if m[0] > lastIdx {
			out.WriteString(html.EscapeString(s[lastIdx:m[0]]))
		}
		lastIdx = m[1]

		switch {
		case m[2] >= 0: // `code`
			out.WriteString("<code>" + html.EscapeString(s[m[2]:m[3]]) + "</code>")
		case m[4] >= 0: // **bold**
			out.WriteString("<b>" + inlineMarkdownToHTML(s[m[4]:m[5]]) + "</b>")
		case m[6] >= 0: // __bold__
			out.WriteString("<b>" + inlineMarkdownToHTML(s[m[6]:m[7]]) + "</b>")
		case m[8] >= 0: // *italic*
			out.WriteString("<em>" + inlineMarkdownToHTML(s[m[8]:m[9]]) + "</em>")
		case m[10] >= 0: // _italic_
			out.WriteString("<em>" + inlineMarkdownToHTML(s[m[10]:m[11]]) + "</em>")
		case m[12] >= 0: // ~~strikethrough~~
			out.WriteString("<s>" + inlineMarkdownToHTML(s[m[12]:m[13]]) + "</s>")
		case m[14] >= 0: // [text](url)
			linkText := s[m[14]:m[15]]
			linkURL := s[m[16]:m[17]]
			out.WriteString(`<a href="` + html.EscapeString(linkURL) + `">` + inlineMarkdownToHTML(linkText) + `</a>`)
		case m[18] >= 0: // plain URL
			u := s[m[18]:m[19]]
			cleanURL, trailing := trimTrailingPunctuation(u)
			out.WriteString(`<a href="` + html.EscapeString(cleanURL) + `">` + html.EscapeString(cleanURL) + `</a>` + html.EscapeString(trailing))
		}
	}

	// remaining plain text
	if lastIdx < len(s) {
		out.WriteString(html.EscapeString(s[lastIdx:]))
	}
	return out.String()
}

// trimTrailingPunctuation trims trailing punctuation that shouldn't be part of the URL.
func trimTrailingPunctuation(urlStr string) (string, string) {
	trailing := ""
	for len(urlStr) > 0 {
		last := urlStr[len(urlStr)-1]
		if last == '.' || last == ',' || last == '!' || last == '?' || last == ';' || last == ':' || last == '*' || last == '\'' || last == '"' {
			trailing = string(last) + trailing
			urlStr = urlStr[:len(urlStr)-1]
		} else if last == ')' {
			// Only trim trailing ')' if there are no matching '(' in the URL.
			if strings.Count(urlStr, "(") < strings.Count(urlStr, ")") {
				trailing = ")" + trailing
				urlStr = urlStr[:len(urlStr)-1]
			} else {
				break
			}
		} else if last == ']' {
			// Only trim trailing ']' if there are no matching '[' in the URL.
			if strings.Count(urlStr, "[") < strings.Count(urlStr, "]") {
				trailing = "]" + trailing
				urlStr = urlStr[:len(urlStr)-1]
			} else {
				break
			}
		} else {
			break
		}
	}
	return urlStr, trailing
}

// containsURL returns true if the string contains http:// or https://.
func containsURL(s string) bool {
	return strings.Contains(s, "http://") || strings.Contains(s, "https://")
}

// containsMarkdown returns true if s appears to contain markdown syntax.
func containsMarkdown(s string) bool {
	return strings.Contains(s, "**") ||
		strings.Contains(s, "__") ||
		strings.Contains(s, "~~") ||
		strings.Contains(s, "`") ||
		bulletListRe.MatchString(s) ||
		orderedListRe.MatchString(s)
}

// ---------------------------------------------------------------------------
// HTML → Markdown (edit round-trip)
// ---------------------------------------------------------------------------

// HTMLToMarkdown converts a Teams message HTML body back to editable markdown
// text. This is used when the user presses 'e' to edit an existing message so
// that **bold**, *italic*, `code`, etc. are preserved in the edit box.
// When attachments are provided, reference file <attachment> tags are rendered
// as [File: name] placeholders for readability in the external editor.
func HTMLToMarkdown(htmlContent string, attachments []MessageAttachment) string {
	if htmlContent == "" {
		return ""
	}
	// Unwrap the system-event sentinel.
	if htmlContent == "<systemEventMessage/>" {
		return ""
	}

	attByID := make(map[string]MessageAttachment, len(attachments))
	for _, a := range attachments {
		attByID[strings.ToLower(a.ID)] = a
	}
	var refFileNames []string
	tokenizer := golanghtml.NewTokenizer(strings.NewReader(htmlContent))
	var sb strings.Builder

	// Inline tag stack — we emit the closing markdown delimiter when we see </tag>.
	type inlineSpan struct {
		open  string // markdown open delimiter already written
		close string // markdown close delimiter to write on </tag>
	}
	var spanStack []inlineSpan

	// List state.
	type listInfo struct {
		ordered bool
		counter int
	}
	var listStack []listInfo

	inPre := false
	// Code span buffering: we don't know if <code> is inline or a block until
	// we see its content. Buffer everything inside <code> and decide on </code>.
	inCodeSpan := false
	var codeSpanBuf strings.Builder

	lastChar := rune(0)
	tagAddedNewline := false
	ensureNewline := func() {
		if lastChar != '\n' && sb.Len() > 0 {
			sb.WriteRune('\n')
			lastChar = '\n'
		}
	}
	write := func(s string) {
		if s == "" {
			return
		}
		sb.WriteString(s)
		lastChar = rune(s[len(s)-1])
	}

	// Helper: push a span onto the stack and write its open delimiter.
	pushSpan := func(open, close string) {
		sb.WriteString(open)
		spanStack = append(spanStack, inlineSpan{open, close})
	}
	// Helper: pop the most-recently-pushed matching span and write its close.
	popSpan := func(open string) {
		for i := len(spanStack) - 1; i >= 0; i-- {
			if spanStack[i].open == open {
				sb.WriteString(spanStack[i].close)
				spanStack = append(spanStack[:i], spanStack[i+1:]...)
				return
			}
		}
	}

	for {
		tt := tokenizer.Next()
		if tt == golanghtml.ErrorToken {
			break
		}
		token := tokenizer.Token()

		switch tt {
		case golanghtml.StartTagToken, golanghtml.SelfClosingTagToken:
			tag := token.Data
			switch tag {
			case "b", "strong":
				pushSpan("**", "**")
			case "em", "i":
				pushSpan("*", "*")
			case "s", "strike", "del":
				pushSpan("~~", "~~")
			case "code":
				if !inPre {
					// Start buffering — we decide fence vs inline on close.
					inCodeSpan = true
					codeSpanBuf.Reset()
				}
				// Inside <pre>, <code> is silently ignored — the fence handles it.
			case "pre":
				inPre = true
				ensureNewline()
				write("```")
				ensureNewline()
			case "ul":
				listStack = append(listStack, listInfo{ordered: false})
			case "ol":
				listStack = append(listStack, listInfo{ordered: true})
			case "li":
				ensureNewline()
				if len(listStack) > 0 {
					info := &listStack[len(listStack)-1]
					indent := strings.Repeat("  ", len(listStack)-1)
					if info.ordered {
						info.counter++
						write(fmt.Sprintf("%s%d. ", indent, info.counter))
					} else {
						write(indent + "- ")
					}
				}
			case "br":
				if inCodeSpan {
					codeSpanBuf.WriteRune('\n')
				} else {
					write("\n")
					lastChar = '\n'
					tagAddedNewline = true
				}
			case "attachment":
				var attID string
				for _, a := range token.Attr {
					if a.Key == "id" {
						attID = a.Val
						break
					}
				}
				if att, ok := attByID[strings.ToLower(attID)]; ok && att.ContentType != nil {
					ct := strings.ToLower(*att.ContentType)
					if ct == "reference" {
						name := "attachment"
						if att.Name != nil && *att.Name != "" {
							name = *att.Name
						}
						refFileNames = append(refFileNames, name)
					}
				}
			case "p", "div":
				// handled on close
			}

		case golanghtml.EndTagToken:
			tag := token.Data
			switch tag {
			case "b", "strong":
				popSpan("**")
			case "em", "i":
				popSpan("*")
			case "s", "strike", "del":
				popSpan("~~")
			case "code":
				if inCodeSpan {
					inCodeSpan = false
					content := codeSpanBuf.String()
					if strings.Contains(content, "\n") {
						// Multi-line → fenced code block.
						ensureNewline()
						write("```")
						ensureNewline()
						write(strings.TrimRight(content, "\n"))
						ensureNewline()
						write("```")
						ensureNewline()
						tagAddedNewline = true
					} else {
						// Single-line → inline code span.
						write("`" + content + "`")
					}
				}
				// Inside <pre>, closing </code> is silently ignored.
			case "pre":
				inPre = false
				ensureNewline()
				write("```")
				ensureNewline()
				tagAddedNewline = true
			case "ul", "ol":
				if len(listStack) > 0 {
					listStack = listStack[:len(listStack)-1]
				}
			case "li":
				ensureNewline()
				tagAddedNewline = true
			case "p", "div":
				ensureNewline()
				tagAddedNewline = true
			}

		case golanghtml.TextToken:
			text := golanghtml.UnescapeString(token.Data)
			if inCodeSpan {
				codeSpanBuf.WriteString(text)
				continue
			}
			if inPre {
				write(text)
				continue
			}
			if tagAddedNewline {
				// Consume exactly one leading newline emitted by a closing block
				// tag so inter-element formatting whitespace is not treated as a
				// blank line (mirrors HTMLToText behaviour).
				if strings.HasPrefix(text, "\n") {
					text = text[1:]
				} else if strings.HasPrefix(text, "\r\n") {
					text = text[2:]
				}
				tagAddedNewline = false
			}
			// Outside code/pre: treat any whitespace-only text node as a
			// blank-line placeholder (Teams uses both &nbsp; and plain spaces
			// for empty paragraphs depending on context).
			if strings.TrimSpace(strings.ReplaceAll(text, "\u00A0", "")) == "" {
				if text == "" {
					continue
				}
				ensureNewline()
				sb.WriteRune('\n') // blank line
				lastChar = '\n'
				continue
			}
			write(text)
		}
	}

	result := sb.String()
	// Collapse excess blank lines.
	for strings.Contains(result, "\n\n\n") {
		result = strings.ReplaceAll(result, "\n\n\n", "\n\n")
	}
	result = strings.Trim(result, "\n\r")
	if len(refFileNames) > 0 {
		result = restoreFilePlaceholdersInText(result, refFileNames)
	}
	return result
}

// restoreFilePlaceholdersInText reinserts [File: name] markers into message text.
// Teams often strips inline attachment tags but leaves double-space gaps where files belonged.
func restoreFilePlaceholdersInText(text string, fileNames []string) string {
	if len(fileNames) == 0 {
		return text
	}

	gapRe := regexp.MustCompile(`\s{2,}`)
	gapLocs := gapRe.FindAllStringIndex(text, -1)
	if len(gapLocs) >= len(fileNames) {
		for i := len(fileNames) - 1; i >= 0; i-- {
			loc := gapLocs[i]
			placeholder := fmt.Sprintf("[File: %s]", fileNames[i])
			text = text[:loc[0]] + " " + placeholder + " " + text[loc[1]:]
		}
		return text
	}

	for _, name := range fileNames {
		text += fmt.Sprintf(" [File: %s]", name)
	}
	return text
}
