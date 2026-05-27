package importer

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
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
