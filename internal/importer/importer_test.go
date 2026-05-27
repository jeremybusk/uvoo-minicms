package importer

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"uvoominicms/internal/db"
)

func TestPreviewWordPressImportsPagesPostsAndMenu(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/wp-json/", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, map[string]any{"name": "Example", "routes": map[string]any{}})
	})
	mux.HandleFunc("/wp-json/wp/v2/pages", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") != "1" {
			writeJSON(t, w, []map[string]any{})
			return
		}
		writeJSON(t, w, []map[string]any{{
			"id":      1,
			"slug":    "about",
			"link":    serverURL(r) + "/about/",
			"status":  "publish",
			"title":   map[string]string{"rendered": "About Us"},
			"content": map[string]string{"rendered": "<p>Hello <strong>families</strong>.</p>"},
			"excerpt": map[string]string{"rendered": "<p>Short description.</p>"},
		}})
	})
	mux.HandleFunc("/wp-json/wp/v2/posts", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") != "1" {
			writeJSON(t, w, []map[string]any{})
			return
		}
		writeJSON(t, w, []map[string]any{{
			"id":      2,
			"slug":    "news",
			"link":    serverURL(r) + "/news/",
			"status":  "publish",
			"title":   map[string]string{"rendered": "News"},
			"content": map[string]string{"rendered": "<h2>Update</h2><p>Registration opens soon.</p>"},
			"excerpt": map[string]string{"rendered": "Registration opens soon."},
		}})
	})
	mux.HandleFunc("/wp-json/wp/v2/menus", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, []map[string]any{{"id": 9, "name": "Primary", "slug": "primary", "locations": []string{"primary"}}})
	})
	mux.HandleFunc("/wp-json/wp/v2/menu-items", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, []map[string]any{{
			"id":         10,
			"parent":     0,
			"menu_order": 1,
			"url":        serverURL(r) + "/about/",
			"title":      map[string]string{"rendered": "About"},
		}})
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	result, err := (Importer{Client: server.Client()}).Preview(context.Background(), Options{
		URL:          server.URL,
		MaxPages:     10,
		IncludePosts: true,
		ImportMenu:   true,
		Publish:      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.WordPress || result.Source != "wordpress" {
		t.Fatalf("expected wordpress source, got %#v", result)
	}
	if len(result.Pages) != 2 {
		t.Fatalf("expected 2 pages, got %d", len(result.Pages))
	}
	if result.Pages[0].Path != "/about" || result.Pages[0].Slug != "about" || result.Pages[0].ContentType != "page" {
		t.Fatalf("unexpected page mapping: %#v", result.Pages[0])
	}
	if result.Pages[1].ContentType != "post" {
		t.Fatalf("expected post content type, got %#v", result.Pages[1])
	}
	if got := result.Pages[0].Markdown; got != "# About Us\n\nHello **families**.\n" {
		t.Fatalf("unexpected markdown:\n%s", got)
	}
	if len(result.Menu) != 1 || result.Menu[0].URL != "/about" {
		t.Fatalf("unexpected menu: %#v", result.Menu)
	}
}

func TestPreviewHonorsContextDeadline(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := (Importer{Client: server.Client()}).Preview(ctx, Options{URL: server.URL, MaxPages: 5})
	if err == nil {
		t.Fatal("expected deadline error")
	}
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Fatalf("preview did not stop promptly: %s", elapsed)
	}
}

func TestHomepageMenuPreservesDropdownParents(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<nav><ul>
			<li><div><a href="/solutions-overview">Solutions</a></div><nav><a href="/fiber">Fiber</a><a href="/cloud">Cloud</a></nav></li>
			<li><div>Resources</div><nav><a href="https://portal.example.com/">Portal</a><a href="/blog">Blog</a></nav></li>
			<li><a href="/contact">Contact</a></li>
		</ul></nav>`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	base := mustURL(t, server.URL)

	menu := (Importer{Client: server.Client()}).fetchHomepageMenu(context.Background(), base)
	if len(menu) != 7 {
		t.Fatalf("expected 7 menu entries, got %#v", menu)
	}
	if menu[0].Label != "Solutions" || menu[0].ParentID != "" {
		t.Fatalf("unexpected first parent: %#v", menu[0])
	}
	if menu[1].Label != "Fiber" || menu[1].ParentID != menu[0].ID {
		t.Fatalf("expected Fiber under Solutions, got %#v", menu[1])
	}
	if menu[3].Label != "Resources" || menu[3].URL != "/blog" {
		t.Fatalf("expected Resources parent using first child URL, got %#v", menu[3])
	}
	if menu[4].ParentID != menu[3].ID || !menu[4].External {
		t.Fatalf("expected external Portal child, got %#v", menu[4])
	}
}

func TestImportDownloadsImagesToUploads(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<main><h1>Home</h1><p>Welcome.</p><img src="/hero.png" alt="Hero image"><img src="/arrow.svg" alt=""></main>`))
	})
	mux.HandleFunc("/hero.png", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write([]byte("png bytes"))
	})
	mux.HandleFunc("/robots.txt", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	mux.HandleFunc("/sitemap.xml", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	mux.HandleFunc("/wp-sitemap.xml", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	mux.HandleFunc("/sitemap_index.xml", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	mux.HandleFunc("/page-sitemap.xml", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	mux.HandleFunc("/post-sitemap.xml", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	dbPath := t.TempDir() + "/cms.db"
	store, err := db.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.DB.Close()
	uploads := t.TempDir()

	result, err := (Importer{Client: server.Client()}).Import(context.Background(), store, uploads, "Demo", Options{
		URL:            server.URL,
		MaxPages:       1,
		Publish:        true,
		UpdateExisting: true,
		DownloadImages: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Imported != 1 {
		t.Fatalf("expected 1 imported page, got %#v", result)
	}
	page, err := store.GetPage(context.Background(), "home")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(page.Markdown, "![Hero image](/uploads/") {
		t.Fatalf("expected localized image, got markdown:\n%s", page.Markdown)
	}
	if strings.Contains(page.Markdown, "arrow.svg") {
		t.Fatalf("decorative svg should be skipped, got markdown:\n%s", page.Markdown)
	}
	entries, err := os.ReadDir(uploads)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Fatal("expected downloaded image under uploads")
	}
}

func writeJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatal(err)
	}
}

func serverURL(r *http.Request) string {
	return "http://" + r.Host
}

func mustURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u
}
