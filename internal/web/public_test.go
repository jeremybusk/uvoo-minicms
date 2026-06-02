package web

import (
	"bytes"
	"html/template"
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
	if strings.Contains(html, `style="display:grid"`) {
		t.Fatalf("desktop toggle visibility should be controlled by scoped CSS, got %s", html)
	}
	if strings.Contains(html, `<style>`) {
		t.Fatalf("menu markup should not include its own style block, got %s", html)
	}
	if strings.Contains(html, `onclick=`) {
		t.Fatalf("menu markup should use delegated event handling, got %s", html)
	}
}

func TestNavMenuStyleOwnsNavigationBehavior(t *testing.T) {
	assets := navMenuStyle()
	if !strings.Contains(assets, `.nav .navToggle{display:none!important}`) {
		t.Fatalf("expected top desktop nav toggles to be hidden by default, got %s", assets)
	}
	if !strings.Contains(assets, `.drawerNav .navGroup:hover>.subnav,.drawerNav .navGroup:focus-within>.subnav{display:none}`) {
		t.Fatalf("expected drawer hover/focus override so collapsed submenus stay closed, got %s", assets)
	}
	if !strings.Contains(assets, `document.addEventListener('click'`) || !strings.Contains(assets, `.navToggle,.navSection`) {
		t.Fatalf("expected one delegated nav click handler, got %s", assets)
	}
}

func TestPublicTemplateSideNavDoesNotRenderHiddenTopMenu(t *testing.T) {
	menu := renderMenu([]db.NavItem{
		{ID: "services", Type: "section", Label: "Services", Enabled: true},
		{ID: "support", Type: "link", ParentID: "services", Label: "Support", URL: "/support", Enabled: true},
	})
	var b bytes.Buffer
	err := publicTpl.Execute(&b, map[string]any{
		"SiteName":     "Test",
		"Title":        "Home",
		"Body":         template.HTML("<p>Home</p>"),
		"MenuHTML":     menu,
		"NavMenuStyle": template.HTML(navMenuStyle()),
		"MenuEnabled":  true,
		"SideNav":      true,
	})
	if err != nil {
		t.Fatalf("execute public template: %v", err)
	}
	html := b.String()
	if !strings.Contains(html, `class="drawerNav"`) {
		t.Fatalf("expected drawer nav, got %s", html)
	}
	if strings.Contains(html, `id="nav"`) {
		t.Fatalf("side nav should not render hidden top nav, got %s", html)
	}
	if count := strings.Count(html, `id="nav-sub-services"`); count != 1 {
		t.Fatalf("expected one submenu id in side nav mode, got %d in %s", count, html)
	}
}
