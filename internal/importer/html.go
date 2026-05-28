package importer

import (
	"bytes"
	stdhtml "html"
	"net/url"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

func HTMLToText(raw string) string {
	doc, err := html.Parse(strings.NewReader(raw))
	if err != nil {
		return strings.Join(strings.Fields(raw), " ")
	}
	return strings.Join(strings.Fields(nodeText(doc)), " ")
}

func HTMLToMarkdown(raw string, base *url.URL) string {
	doc, err := html.Parse(strings.NewReader(raw))
	if err != nil {
		return strings.TrimSpace(HTMLToText(raw))
	}
	body := firstNode(doc, "body")
	if body == nil {
		body = doc
	}
	return cleanMarkdown(HTMLNodeToMarkdown(body, base))
}

func HTMLNodeToMarkdown(n *html.Node, base *url.URL) string {
	var b strings.Builder
	renderChildren(&b, n, base, 0)
	return cleanMarkdown(b.String())
}

func renderChildren(b *strings.Builder, n *html.Node, base *url.URL, depth int) {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		renderNode(b, c, base, depth)
	}
}

func renderNode(b *strings.Builder, n *html.Node, base *url.URL, depth int) {
	if n.Type == html.TextNode {
		text := strings.Join(strings.Fields(n.Data), " ")
		if text == "" {
			return
		}
		if needsSpace(b.String()) && !startsPunctuation(text) {
			b.WriteByte(' ')
		}
		b.WriteString(text)
		return
	}
	if n.Type != html.ElementNode {
		renderChildren(b, n, base, depth)
		return
	}
	if shouldSkipElement(n) {
		return
	}
	switch n.Data {
	case "script", "style", "noscript", "svg", "form", "nav", "header", "footer":
		return
	case "h1", "h2", "h3", "h4", "h5", "h6":
		level := int(n.Data[1] - '0')
		blankLine(b)
		b.WriteString(strings.Repeat("#", level))
		b.WriteByte(' ')
		b.WriteString(strings.TrimSpace(nodeText(n)))
		blankLine(b)
	case "p", "div", "section", "article", "main":
		blankLine(b)
		renderChildren(b, n, base, depth)
		blankLine(b)
	case "br":
		b.WriteByte('\n')
	case "ul", "ol":
		blankLine(b)
		renderChildren(b, n, base, depth+1)
		blankLine(b)
	case "li":
		blankLine(b)
		b.WriteString(strings.Repeat("  ", max(0, depth-1)))
		b.WriteString("- ")
		renderChildren(b, n, base, depth)
	case "blockquote":
		blankLine(b)
		text := cleanMarkdown(renderToString(n, base, depth))
		for _, line := range strings.Split(text, "\n") {
			if strings.TrimSpace(line) == "" {
				continue
			}
			b.WriteString("> ")
			b.WriteString(line)
			b.WriteByte('\n')
		}
		blankLine(b)
	case "strong", "b":
		text := strings.TrimSpace(nodeText(n))
		if text == "" {
			return
		}
		if needsSpace(b.String()) && !startsPunctuation(text) {
			b.WriteByte(' ')
		}
		b.WriteString("**")
		b.WriteString(text)
		b.WriteString("**")
	case "em", "i":
		text := strings.TrimSpace(nodeText(n))
		if text == "" {
			return
		}
		if needsSpace(b.String()) && !startsPunctuation(text) {
			b.WriteByte(' ')
		}
		b.WriteString("*")
		b.WriteString(text)
		b.WriteString("*")
	case "code":
		text := strings.TrimSpace(nodeText(n))
		if text != "" {
			b.WriteByte('`')
			b.WriteString(strings.ReplaceAll(text, "`", "'"))
			b.WriteByte('`')
		}
	case "pre":
		blankLine(b)
		b.WriteString("```\n")
		b.WriteString(strings.TrimSpace(nodeText(n)))
		b.WriteString("\n```")
		blankLine(b)
	case "a":
		label := strings.TrimSpace(nodeText(n))
		href := absoluteURL(base, attr(n, "href"))
		if label == "" {
			return
		}
		if href == "" || strings.HasPrefix(href, "#") {
			b.WriteString(label)
			return
		}
		if sameHostURL(base, href) {
			href = routePathFromURL(base, href)
		}
		b.WriteString("[")
		b.WriteString(label)
		b.WriteString("](")
		b.WriteString(markdownURL(href))
		b.WriteString(")")
	case "img":
		src := absoluteURL(base, attr(n, "src"))
		if src == "" || isDecorativeImage(n, src) {
			return
		}
		alt := firstNonEmpty(attr(n, "alt"), attr(n, "title"), titleFromPath(src))
		blankLine(b)
		b.WriteString("![")
		b.WriteString(alt)
		b.WriteString("](")
		b.WriteString(markdownURL(src))
		b.WriteString(")")
		blankLine(b)
	default:
		renderChildren(b, n, base, depth)
	}
}

func renderToString(n *html.Node, base *url.URL, depth int) string {
	var b strings.Builder
	renderChildren(&b, n, base, depth)
	return b.String()
}

func cleanMarkdown(s string) string {
	s = scrubBuilderShortcodes(s)
	lines := strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
	var out []string
	blank := false
	for _, line := range lines {
		line = strings.TrimRight(line, " \t")
		if strings.TrimSpace(line) == "" {
			if !blank && len(out) > 0 {
				out = append(out, "")
			}
			blank = true
			continue
		}
		out = append(out, line)
		blank = false
	}
	return strings.TrimSpace(strings.Join(out, "\n")) + "\n"
}

var (
	rawShortcodeRe    = regexp.MustCompile(`(?is)\[(vc_raw_html|vc_raw_js)[^\]]*\].*?\[/\s*(vc_raw_html|vc_raw_js)\s*\]`)
	shortcodeRe       = regexp.MustCompile(`\[\s*(/?)\s*([A-Za-z0-9_-]+)([^\]]*)\]`)
	shortcodeTextRe   = regexp.MustCompile(`(?is)(?:^|\s)(text|title)\s*=\s*(?:"([^"]*)"|'([^']*)'|[“”]([^“”]*)[“”])`)
	shortcodeHTMLRe   = regexp.MustCompile(`(?is)<[^>]+>`)
	shortcodeSpacesRe = regexp.MustCompile(`[ \t]{2,}`)
)

func scrubBuilderShortcodes(s string) string {
	s = rawShortcodeRe.ReplaceAllString(s, "")
	s = shortcodeRe.ReplaceAllStringFunc(s, func(match string) string {
		parts := shortcodeRe.FindStringSubmatch(match)
		if len(parts) != 4 {
			return match
		}
		closing, name, attrs := parts[1] == "/", strings.ToLower(parts[2]), parts[3]
		if !isBuilderShortcode(name) {
			return match
		}
		if closing {
			if isBuilderBlockShortcode(name) {
				return "\n\n"
			}
			return ""
		}
		switch name {
		case "vc_custom_heading", "vc_btn":
			if text := shortcodeAttrText(attrs); text != "" {
				return "\n\n" + text + "\n\n"
			}
		}
		if isBuilderBlockShortcode(name) {
			return "\n\n"
		}
		return ""
	})
	s = strings.ReplaceAll(s, "[/]", "")
	s = shortcodeSpacesRe.ReplaceAllString(s, " ")
	return s
}

func isBuilderShortcode(name string) bool {
	if strings.HasPrefix(name, "vc_") || strings.HasPrefix(name, "vc-") {
		return true
	}
	switch name {
	case "contact-form-7", "rev_slider", "gravityform":
		return true
	default:
		return false
	}
}

func isBuilderBlockShortcode(name string) bool {
	switch name {
	case "vc_row", "vc_row_inner", "vc_column", "vc_column_inner", "vc_column_text", "vc_message", "vc_cta", "vc_toggle", "vc_tta_section", "vc_tta_tabs", "vc_tta_accordion":
		return true
	default:
		return false
	}
}

func shortcodeAttrText(attrs string) string {
	attrs = stdhtml.UnescapeString(attrs)
	match := shortcodeTextRe.FindStringSubmatch(attrs)
	if len(match) == 0 {
		return ""
	}
	for _, value := range match[2:] {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		value = stdhtml.UnescapeString(value)
		value = shortcodeHTMLRe.ReplaceAllString(value, "")
		value = strings.Join(strings.Fields(value), " ")
		return value
	}
	return ""
}

func needsSpace(current string) bool {
	if current == "" {
		return false
	}
	last := current[len(current)-1]
	return last != ' ' && last != '\n' && last != '(' && last != '['
}

func startsPunctuation(s string) bool {
	if s == "" {
		return false
	}
	return strings.ContainsRune(".,;:!?)]}", []rune(s)[0])
}

func blankLine(b *strings.Builder) {
	s := b.String()
	if s == "" {
		return
	}
	if strings.HasSuffix(s, "\n\n") {
		return
	}
	if strings.HasSuffix(s, "\n") {
		b.WriteByte('\n')
		return
	}
	b.WriteString("\n\n")
}

func nodeText(n *html.Node) string {
	var b bytes.Buffer
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.TextNode {
			b.WriteString(node.Data)
			b.WriteByte(' ')
			return
		}
		if node.Type == html.ElementNode {
			switch node.Data {
			case "script", "style", "noscript", "svg":
				return
			}
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return strings.Join(strings.Fields(b.String()), " ")
}

func firstNode(n *html.Node, names ...string) *html.Node {
	nameSet := map[string]bool{}
	for _, name := range names {
		nameSet[name] = true
	}
	var found *html.Node
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if found != nil {
			return
		}
		if node.Type == html.ElementNode && nameSet[node.Data] {
			found = node
			return
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return found
}

func findTitle(n *html.Node) string {
	if title := firstNode(n, "title"); title != nil {
		return strings.TrimSpace(nodeText(title))
	}
	return ""
}

func firstHeading(n *html.Node) string {
	if heading := firstNode(n, "h1", "h2"); heading != nil {
		return strings.TrimSpace(nodeText(heading))
	}
	return ""
}

func findMeta(n *html.Node, name string) string {
	name = strings.ToLower(name)
	var found string
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if found != "" {
			return
		}
		if node.Type == html.ElementNode && node.Data == "meta" {
			if strings.EqualFold(attr(node, "name"), name) || strings.EqualFold(attr(node, "property"), name) {
				found = strings.TrimSpace(attr(node, "content"))
				return
			}
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return found
}

func attr(n *html.Node, key string) string {
	for _, attr := range n.Attr {
		if strings.EqualFold(attr.Key, key) {
			return strings.TrimSpace(attr.Val)
		}
	}
	return ""
}

func shouldSkipElement(n *html.Node) bool {
	if n.Type != html.ElementNode {
		return false
	}
	className := strings.ToLower(attr(n, "class"))
	id := strings.ToLower(attr(n, "id"))
	skipClasses := []string{
		"header-wrapper",
		"footer-body-wrapper",
		"nav-menu",
		"navbar",
		"menu-button",
		"tab-none",
		"show-in-tab",
		"w-nav",
		"w-form",
		"form-wrapper",
		"w-embed",
		"w-script",
		"elfsight",
	}
	for _, marker := range skipClasses {
		if strings.Contains(className, marker) || strings.Contains(id, marker) {
			return true
		}
	}
	return false
}

func isDecorativeImage(n *html.Node, src string) bool {
	alt := strings.TrimSpace(attr(n, "alt"))
	className := strings.ToLower(attr(n, "class"))
	lowerSrc := strings.ToLower(src)
	if strings.HasSuffix(lowerSrc, ".svg") && alt == "" {
		return true
	}
	decorativeWords := []string{"arrow", "chevron", "caret", "icon", "spinner", "vector"}
	for _, word := range decorativeWords {
		if strings.Contains(lowerSrc, word) || strings.Contains(className, word) {
			return true
		}
	}
	return false
}

func markdownURL(raw string) string {
	replacer := strings.NewReplacer("(", "%28", ")", "%29", " ", "%20")
	return replacer.Replace(raw)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
