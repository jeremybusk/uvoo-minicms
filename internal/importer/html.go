package importer

import (
	"bytes"
	"net/url"
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
	switch n.Data {
	case "script", "style", "noscript", "svg", "form":
		return
	case "h1", "h2", "h3", "h4", "h5", "h6":
		level := int(n.Data[1] - '0')
		blankLine(b)
		b.WriteString(strings.Repeat("#", level))
		b.WriteByte(' ')
		b.WriteString(strings.TrimSpace(nodeText(n)))
		blankLine(b)
	case "p", "div", "section", "article", "header", "footer", "main":
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
		b.WriteString(href)
		b.WriteString(")")
	case "img":
		src := absoluteURL(base, attr(n, "src"))
		if src == "" {
			return
		}
		alt := strings.TrimSpace(attr(n, "alt"))
		b.WriteString("![")
		b.WriteString(alt)
		b.WriteString("](")
		b.WriteString(src)
		b.WriteString(")")
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

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
