package app

import (
	"html"
	"html/template"
	"strings"
	"unicode"
	"unicode/utf8"
)

// renderStudyMarkdown renders the deliberately small Markdown subset used by
// lesson notes and homework tasks. Every text fragment is escaped before HTML
// is emitted, so only markup produced by this renderer reaches templates.
func renderStudyMarkdown(source string) template.HTML {
	source = strings.ReplaceAll(source, "\r\n", "\n")
	source = strings.ReplaceAll(source, "\r", "\n")
	lines := strings.Split(source, "\n")
	var out strings.Builder

	for i := 0; i < len(lines); {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			i++
			continue
		}

		if strings.HasPrefix(trimmed, "```") {
			lang := strings.TrimSpace(strings.TrimPrefix(trimmed, "```"))
			i++
			var code []string
			for i < len(lines) && !strings.HasPrefix(strings.TrimSpace(lines[i]), "```") {
				code = append(code, lines[i])
				i++
			}
			if i < len(lines) {
				i++
			}
			out.WriteString(`<pre class="md-code"><div class="md-code-head"><span>`)
			if lang == "" {
				out.WriteString("code")
			} else {
				out.WriteString(html.EscapeString(lang))
			}
			out.WriteString(`</span></div><code>`)
			out.WriteString(highlightCode(strings.Join(code, "\n"), lang))
			out.WriteString(`</code></pre>`)
			continue
		}

		if level, text := markdownHeading(trimmed); level > 0 {
			out.WriteString("<h")
			out.WriteByte(byte('0' + level))
			out.WriteString(">")
			out.WriteString(renderInlineMarkdown(text))
			out.WriteString("</h")
			out.WriteByte(byte('0' + level))
			out.WriteString(">")
			i++
			continue
		}

		if trimmed == "---" || trimmed == "***" {
			out.WriteString("<hr>")
			i++
			continue
		}

		if strings.HasPrefix(trimmed, "> ") {
			var quote []string
			for i < len(lines) {
				t := strings.TrimSpace(lines[i])
				if !strings.HasPrefix(t, "> ") {
					break
				}
				quote = append(quote, strings.TrimSpace(strings.TrimPrefix(t, "> ")))
				i++
			}
			out.WriteString("<blockquote>")
			out.WriteString(renderInlineMarkdown(strings.Join(quote, " ")))
			out.WriteString("</blockquote>")
			continue
		}

		if markdownUnorderedItem(trimmed) != "" {
			out.WriteString("<ul>")
			for i < len(lines) {
				item := markdownUnorderedItem(strings.TrimSpace(lines[i]))
				if item == "" {
					break
				}
				out.WriteString("<li>")
				out.WriteString(renderInlineMarkdown(item))
				out.WriteString("</li>")
				i++
			}
			out.WriteString("</ul>")
			continue
		}

		if item, ok := markdownOrderedItem(trimmed); ok {
			_ = item
			out.WriteString("<ol>")
			for i < len(lines) {
				item, ok := markdownOrderedItem(strings.TrimSpace(lines[i]))
				if !ok {
					break
				}
				out.WriteString("<li>")
				out.WriteString(renderInlineMarkdown(item))
				out.WriteString("</li>")
				i++
			}
			out.WriteString("</ol>")
			continue
		}

		var paragraph []string
		for i < len(lines) {
			t := strings.TrimSpace(lines[i])
			if t == "" || isMarkdownBlockStart(t) {
				break
			}
			paragraph = append(paragraph, t)
			i++
		}
		if len(paragraph) > 0 {
			out.WriteString("<p>")
			out.WriteString(renderInlineMarkdown(strings.Join(paragraph, " ")))
			out.WriteString("</p>")
			continue
		}
		i++
	}

	return template.HTML(out.String()) // #nosec G203 -- all source text is escaped above.
}

func markdownHeading(line string) (int, string) {
	for level := 4; level >= 1; level-- {
		prefix := strings.Repeat("#", level) + " "
		if strings.HasPrefix(line, prefix) {
			return level, strings.TrimSpace(strings.TrimPrefix(line, prefix))
		}
	}
	return 0, ""
}

func markdownUnorderedItem(line string) string {
	for _, prefix := range []string{"- ", "* ", "+ "} {
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(line, prefix))
		}
	}
	return ""
}

func markdownOrderedItem(line string) (string, bool) {
	pos := strings.Index(line, ". ")
	if pos <= 0 {
		return "", false
	}
	for _, r := range line[:pos] {
		if !unicode.IsDigit(r) {
			return "", false
		}
	}
	return strings.TrimSpace(line[pos+2:]), true
}

func isMarkdownBlockStart(line string) bool {
	if line == "" || strings.HasPrefix(line, "```") || strings.HasPrefix(line, "> ") || line == "---" || line == "***" {
		return true
	}
	if level, _ := markdownHeading(line); level > 0 {
		return true
	}
	if markdownUnorderedItem(line) != "" {
		return true
	}
	_, ordered := markdownOrderedItem(line)
	return ordered
}

func renderInlineMarkdown(input string) string {
	var out strings.Builder
	for i := 0; i < len(input); {
		if input[i] == '`' {
			if end := strings.IndexByte(input[i+1:], '`'); end >= 0 {
				end += i + 1
				out.WriteString("<code>")
				out.WriteString(html.EscapeString(input[i+1 : end]))
				out.WriteString("</code>")
				i = end + 1
				continue
			}
		}
		if strings.HasPrefix(input[i:], "**") {
			if end := strings.Index(input[i+2:], "**"); end >= 0 {
				end += i + 2
				out.WriteString("<strong>")
				out.WriteString(renderInlineMarkdown(input[i+2 : end]))
				out.WriteString("</strong>")
				i = end + 2
				continue
			}
		}
		if input[i] == '*' {
			if end := strings.IndexByte(input[i+1:], '*'); end >= 0 {
				end += i + 1
				out.WriteString("<em>")
				out.WriteString(renderInlineMarkdown(input[i+1 : end]))
				out.WriteString("</em>")
				i = end + 1
				continue
			}
		}
		if input[i] == '[' {
			closeLabel := strings.IndexByte(input[i+1:], ']')
			if closeLabel >= 0 {
				closeLabel += i + 1
				if closeLabel+1 < len(input) && input[closeLabel+1] == '(' {
					closeURL := strings.IndexByte(input[closeLabel+2:], ')')
					if closeURL >= 0 {
						closeURL += closeLabel + 2
						url := strings.TrimSpace(input[closeLabel+2 : closeURL])
						if validHTTPURL(url) {
							out.WriteString(`<a href="`)
							out.WriteString(html.EscapeString(url))
							out.WriteString(`" target="_blank" rel="noopener noreferrer">`)
							out.WriteString(renderInlineMarkdown(input[i+1 : closeLabel]))
							out.WriteString("</a>")
							i = closeURL + 1
							continue
						}
					}
				}
			}
		}
		_, size := utf8.DecodeRuneInString(input[i:])
		if size <= 0 {
			size = 1
		}
		out.WriteString(html.EscapeString(input[i : i+size]))
		i += size
	}
	return out.String()
}

func highlightCode(code, language string) string {
	lang := strings.ToLower(strings.TrimSpace(language))
	keywords := codeKeywords(lang)
	commentHash := lang == "python" || lang == "py" || lang == "bash" || lang == "sh" || lang == "shell" || lang == "yaml" || lang == "yml"
	var out strings.Builder

	for i := 0; i < len(code); {
		if code[i] == '\n' {
			out.WriteByte('\n')
			i++
			continue
		}
		if (commentHash && code[i] == '#') || (!commentHash && i+1 < len(code) && code[i:i+2] == "//") {
			end := strings.IndexByte(code[i:], '\n')
			if end < 0 {
				end = len(code) - i
			}
			out.WriteString(`<span class="tok-comment">`)
			out.WriteString(html.EscapeString(code[i : i+end]))
			out.WriteString(`</span>`)
			i += end
			continue
		}
		if code[i] == '"' || code[i] == '\'' || code[i] == '`' {
			quote := code[i]
			j := i + 1
			for j < len(code) {
				if code[j] == '\\' && quote != '`' && j+1 < len(code) {
					j += 2
					continue
				}
				if code[j] == quote {
					j++
					break
				}
				j++
			}
			out.WriteString(`<span class="tok-string">`)
			out.WriteString(html.EscapeString(code[i:j]))
			out.WriteString(`</span>`)
			i = j
			continue
		}
		if isIdentStart(code[i]) {
			j := i + 1
			for j < len(code) && isIdentPart(code[j]) {
				j++
			}
			word := code[i:j]
			if keywords[word] {
				out.WriteString(`<span class="tok-keyword">`)
				out.WriteString(html.EscapeString(word))
				out.WriteString(`</span>`)
			} else {
				out.WriteString(html.EscapeString(word))
			}
			i = j
			continue
		}
		if code[i] >= '0' && code[i] <= '9' {
			j := i + 1
			for j < len(code) && ((code[j] >= '0' && code[j] <= '9') || strings.ContainsRune("._xXabcdefABCDEF", rune(code[j]))) {
				j++
			}
			out.WriteString(`<span class="tok-number">`)
			out.WriteString(html.EscapeString(code[i:j]))
			out.WriteString(`</span>`)
			i = j
			continue
		}
		out.WriteString(html.EscapeString(code[i : i+1]))
		i++
	}
	return out.String()
}

func isIdentStart(b byte) bool { return b == '_' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' }
func isIdentPart(b byte) bool  { return isIdentStart(b) || b >= '0' && b <= '9' }

func codeKeywords(lang string) map[string]bool {
	words := "if else for while return break continue switch case default true false null nil new class struct enum public private protected static const import from as try catch throw defer async await"
	switch lang {
	case "go", "golang":
		words = "break default func interface select case defer go map struct chan else goto package switch const fallthrough if range type continue for import return var nil true false"
	case "c", "cpp", "c++", "cc", "h", "hpp":
		words = "auto break case char const continue default do double else enum extern float for goto if int long register return short signed sizeof static struct switch typedef union unsigned void volatile while class namespace public private protected template typename using new delete nullptr true false bool"
	case "python", "py":
		words = "False None True and as assert async await break class continue def del elif else except finally for from global if import in is lambda nonlocal not or pass raise return try while with yield"
	case "bash", "sh", "shell":
		words = "if then else elif fi for while in do done case esac function select time until coproc"
	case "javascript", "js", "typescript", "ts":
		words = "break case catch class const continue debugger default delete do else export extends finally for function if import in instanceof let new return super switch this throw try typeof var void while with yield async await true false null undefined"
	case "json":
		words = "true false null"
	}
	result := make(map[string]bool)
	for _, word := range strings.Fields(words) {
		result[word] = true
	}
	return result
}
