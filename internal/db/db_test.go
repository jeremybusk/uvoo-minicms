package db

import (
	"context"
	"testing"
)

func TestSavePageWithRevisionsHonorsLimit(t *testing.T) {
	store, err := Open(t.TempDir() + "/cms.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.DB.Close()
	ctx := context.Background()

	page := Page{Slug: "about", Path: "/about", Title: "Version 1", ContentType: "page", Markdown: "one", Published: true}
	if _, err := store.SavePageWithRevisions(ctx, page, 2); err != nil {
		t.Fatal(err)
	}
	for i, title := range []string{"Version 2", "Version 3", "Version 4"} {
		page.Title = title
		page.Markdown = title
		if _, err := store.SavePageWithRevisions(ctx, page, 2); err != nil {
			t.Fatalf("save %d: %v", i, err)
		}
	}

	revisions, err := store.ListPageRevisions(ctx, "about")
	if err != nil {
		t.Fatal(err)
	}
	if len(revisions) != 2 {
		t.Fatalf("expected 2 revisions, got %d", len(revisions))
	}
	if revisions[0].Title != "Version 3" || revisions[1].Title != "Version 2" {
		t.Fatalf("unexpected revisions: %#v", revisions)
	}
}

func TestListPublishedPostsOrdersByPublishedAt(t *testing.T) {
	store, err := Open(t.TempDir() + "/cms.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.DB.Close()
	ctx := context.Background()

	pages := []Page{
		{Slug: "old-post", Path: "/blog/old-post", Title: "Old", ContentType: "post", Published: true, PublishedAt: "2026-01-02"},
		{Slug: "new-post", Path: "/blog/new-post", Title: "New", ContentType: "post", Published: true, PublishedAt: "2026-03-04"},
		{Slug: "draft-post", Path: "/blog/draft-post", Title: "Draft", ContentType: "post", Published: false, PublishedAt: "2026-04-05"},
		{Slug: "regular-page", Path: "/regular-page", Title: "Page", ContentType: "page", Published: true, PublishedAt: "2026-05-06"},
	}
	for _, page := range pages {
		if _, err := store.SavePage(ctx, page); err != nil {
			t.Fatalf("save %s: %v", page.Slug, err)
		}
	}

	posts, err := store.ListPublishedPosts(ctx, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(posts) != 2 {
		t.Fatalf("expected 2 posts, got %d", len(posts))
	}
	if posts[0].Slug != "new-post" || posts[1].Slug != "old-post" {
		t.Fatalf("unexpected post order: %#v", posts)
	}
}
