package web

import (
	"strings"
	"testing"

	"uvoominicms/internal/db"
)

func TestYouTubeID(t *testing.T) {
	tests := map[string]string{
		"dQw4w9WgXcQ": "dQw4w9WgXcQ",
		"https://www.youtube.com/watch?v=dQw4w9WgXcQ": "dQw4w9WgXcQ",
		"https://youtu.be/dQw4w9WgXcQ":                "dQw4w9WgXcQ",
		"https://www.youtube.com/shorts/dQw4w9WgXcQ":  "dQw4w9WgXcQ",
		"https://example.com/watch?v=dQw4w9WgXcQ":     "",
		"not-a-valid-video":                           "",
	}
	for input, want := range tests {
		if got := youtubeID(input); got != want {
			t.Fatalf("youtubeID(%q)=%q, want %q", input, got, want)
		}
	}
}

func TestVimeoID(t *testing.T) {
	tests := map[string]string{
		"76979871":                                "76979871",
		"https://vimeo.com/76979871":              "76979871",
		"https://player.vimeo.com/video/76979871": "76979871",
		"https://example.com/video/76979871":      "",
		"https://vimeo.com/not-a-number":          "",
	}
	for input, want := range tests {
		if got := vimeoID(input); got != want {
			t.Fatalf("vimeoID(%q)=%q, want %q", input, got, want)
		}
	}
}

func TestRenderMenuSupportsSectionParents(t *testing.T) {
	html := string(renderMenu([]db.NavItem{
		{ID: "resources", Type: "section", Label: "Resources", Enabled: true},
		{ID: "blog", Type: "link", ParentID: "resources", Label: "Blog", URL: "/blog", Enabled: true},
	}))
	if !strings.Contains(html, `<button class="navSection"`) {
		t.Fatalf("expected section parent button, got %s", html)
	}
	if strings.Contains(html, `href=""`) {
		t.Fatalf("section parent should not render an empty link, got %s", html)
	}
	if !strings.Contains(html, `aria-expanded="false"`) || !strings.Contains(html, `▾`) {
		t.Fatalf("expected accessible chevron disclosure, got %s", html)
	}
}

func TestRenderMenuKeepsLinkParentClickable(t *testing.T) {
	html := string(renderMenu([]db.NavItem{
		{ID: "services", Type: "link", Label: "Services", URL: "/services", Enabled: true},
		{ID: "support", Type: "link", ParentID: "services", Label: "Support", URL: "/services/support", Enabled: true},
	}))
	if !strings.Contains(html, `<a href="/services"`) {
		t.Fatalf("expected parent link to remain clickable, got %s", html)
	}
	if !strings.Contains(html, `class="navToggle"`) {
		t.Fatalf("expected separate submenu toggle, got %s", html)
	}
}
