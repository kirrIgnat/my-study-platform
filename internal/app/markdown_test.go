package app

import (
	"strings"
	"testing"
)

func TestRenderStudyMarkdownEscapesHTMLAndHighlightsCode(t *testing.T) {
	input := "# Заголовок\n\n<script>alert(1)</script>\n\n```go\nfunc main() {\n\treturn\n}\n```"
	got := string(renderStudyMarkdown(input))
	if strings.Contains(got, "<script>") {
		t.Fatalf("raw HTML was not escaped: %s", got)
	}
	if !strings.Contains(got, "&lt;script&gt;") {
		t.Fatalf("escaped HTML missing: %s", got)
	}
	if !strings.Contains(got, `class="tok-keyword">func</span>`) {
		t.Fatalf("Go keyword highlighting missing: %s", got)
	}
}

func TestRenderStudyMarkdownAllowsOnlyHTTPLinks(t *testing.T) {
	got := string(renderStudyMarkdown("[ok](https://example.com) [bad](javascript:alert(1))"))
	if !strings.Contains(got, `href="https://example.com"`) {
		t.Fatalf("https link missing: %s", got)
	}
	if strings.Contains(got, `href="javascript:`) {
		t.Fatalf("unsafe link rendered: %s", got)
	}
}
