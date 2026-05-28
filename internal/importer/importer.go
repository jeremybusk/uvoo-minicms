package importer

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha1"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	stdhtml "html"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"golang.org/x/net/html"
	"uvoominicms/internal/db"
)

const defaultMaxPages = 100
const defaultRequestTimeout = 6 * time.Second

type Importer struct {
	Client *http.Client
}

type Options struct {
	URL              string
	MaxPages         int
	IncludePosts     bool
	ImportMenu       bool
	Publish          bool
	UpdateExisting   bool
	DownloadImages   bool
	AdvancedScraping bool
	PreviewOnly      bool
}

type Result struct {
	Source       string
	BaseURL      string
	Pages        []Page
	Menu         []db.NavItem
	Imported     int
	Skipped      int
	Errors       []string
	Existing     int
	WordPress    bool
	SitemapURL   string
	PreviewLimit int
}

type Page struct {
	Slug            string
	Path            string
	Title           string
	MetaDescription string
	ContentType     string
	Tags            string
	Markdown        string
	SourceURL       string
	Published       bool
	Exists          bool
}

type pageCollection struct {
	Items []wpPost
	Kind  string
}

type wpPost struct {
	ID      int    `json:"id"`
	Slug    string `json:"slug"`
	Link    string `json:"link"`
	Status  string `json:"status"`
	Parent  int    `json:"parent"`
	Order   int    `json:"menu_order"`
	Title   wpText `json:"title"`
	Content wpText `json:"content"`
	Excerpt wpText `json:"excerpt"`
}

type wpText struct {
	Rendered string `json:"rendered"`
}

type wpMenu struct {
	ID        int      `json:"id"`
	Name      string   `json:"name"`
	Slug      string   `json:"slug"`
	Locations []string `json:"locations"`
}

type wpMenuItem struct {
	ID        int    `json:"id"`
	Parent    int    `json:"parent"`
	MenuOrder int    `json:"menu_order"`
	URL       string `json:"url"`
	Title     wpText `json:"title"`
}

func (i Importer) Preview(ctx context.Context, opts Options) (Result, error) {
	opts = normalizeOptions(opts)
	base, err := normalizeBase(opts.URL)
	if err != nil {
		return Result{}, err
	}
	result := Result{BaseURL: base.String(), PreviewLimit: opts.MaxPages}
	if wp, err := i.previewWordPress(ctx, base, opts); err == nil && len(wp.Pages) > 0 {
		wp.BaseURL = base.String()
		wp.PreviewLimit = opts.MaxPages
		return wp, nil
	} else if ctx.Err() != nil {
		return result, ctx.Err()
	}
	fallback, err := i.previewSitemap(ctx, base, opts)
	if err != nil {
		return result, err
	}
	return fallback, nil
}

func (i Importer) Import(ctx context.Context, store *db.Store, uploadDir, siteName string, opts Options) (Result, error) {
	result, err := i.Preview(ctx, opts)
	if err != nil {
		return result, err
	}
	existing, err := store.ListPages(ctx)
	if err != nil {
		return result, err
	}
	bySlug := map[string]db.Page{}
	byPath := map[string]db.Page{}
	for _, page := range existing {
		bySlug[page.Slug] = page
		byPath[page.Path] = page
	}
	imageCache := map[string]string{}
	imageFailures := map[string]bool{}
	for idx := range result.Pages {
		page := &result.Pages[idx]
		saveSlug := page.Slug
		if current, ok := byPath[page.Path]; ok {
			page.Exists = true
			result.Existing++
			if !opts.UpdateExisting {
				result.Skipped++
				continue
			}
			saveSlug = current.Slug
		} else if current, ok := bySlug[page.Slug]; ok {
			page.Exists = true
			result.Existing++
			if !opts.UpdateExisting {
				result.Skipped++
				continue
			}
			saveSlug = current.Slug
		}
		if opts.DownloadImages {
			markdown, imageErrors := i.localizeImages(ctx, store, uploadDir, page.Markdown, imageCache, imageFailures)
			page.Markdown = markdown
			result.Errors = append(result.Errors, imageErrors...)
		}
		_, err := store.SavePage(ctx, db.Page{
			Slug:            saveSlug,
			Path:            page.Path,
			Title:           page.Title,
			MetaDescription: page.MetaDescription,
			ContentType:     page.ContentType,
			Tags:            page.Tags,
			Markdown:        page.Markdown,
			Published:       opts.Publish,
		})
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", page.SourceURL, err))
			continue
		}
		result.Imported++
	}
	if opts.ImportMenu && len(result.Menu) > 0 {
		settings, err := store.GetSettings(ctx, siteName)
		if err != nil {
			result.Errors = append(result.Errors, "menu: "+err.Error())
		} else {
			settings.Menu = result.Menu
			settings.MenuEnabled = true
			if _, err := store.SaveSettings(ctx, settings); err != nil {
				result.Errors = append(result.Errors, "menu: "+err.Error())
			}
		}
	}
	return result, nil
}

func (i Importer) previewWordPress(ctx context.Context, base *url.URL, opts Options) (Result, error) {
	indexURL := base.ResolveReference(&url.URL{Path: "/wp-json/"}).String()
	var index map[string]any
	if err := i.getJSON(ctx, indexURL, &index); err != nil {
		return Result{}, err
	}
	var pages []Page
	collections := []pageCollection{{Kind: "page"}}
	if opts.IncludePosts {
		collections = append(collections, pageCollection{Kind: "post"})
	}
	for _, collection := range collections {
		endpoint := "pages"
		if collection.Kind == "post" {
			endpoint = "posts"
		}
		items, err := i.fetchWPPosts(ctx, base, endpoint, opts.MaxPages-len(pages), !opts.PreviewOnly)
		if err != nil {
			continue
		}
		for _, item := range items {
			page := wpPostToPage(base, item, collection.Kind, opts.Publish, opts.AdvancedScraping)
			if page.Title != "" && page.Path != "" {
				pages = append(pages, page)
			}
			if len(pages) >= opts.MaxPages {
				break
			}
		}
		if len(pages) >= opts.MaxPages {
			break
		}
	}
	result := Result{Source: "wordpress", Pages: pages, WordPress: true, BaseURL: base.String()}
	if opts.ImportMenu {
		result.Menu = i.fetchWPMenu(ctx, base)
	}
	if len(result.Menu) == 0 && opts.ImportMenu {
		result.Menu = i.fetchHomepageMenu(ctx, base)
	}
	if len(result.Menu) > 0 {
		result.Pages = i.prioritizeMenuPages(ctx, base, result.Pages, result.Menu, opts.MaxPages, opts.Publish, opts.PreviewOnly, opts.AdvancedScraping)
	}
	return result, nil
}

type menuPageLink struct {
	Path  string
	Label string
}

func (i Importer) prioritizeMenuPages(ctx context.Context, base *url.URL, pages []Page, menu []db.NavItem, max int, publish, previewOnly, advanced bool) []Page {
	if max <= 0 || len(menu) == 0 {
		return pages
	}
	byPath := map[string]Page{}
	for _, page := range pages {
		if page.Path != "" {
			byPath[page.Path] = page
		}
	}
	menuLinks := append([]menuPageLink{{Path: "/", Label: "Home"}}, menuPageLinks(menu)...)
	merged := make([]Page, 0, len(pages)+len(menuLinks))
	added := map[string]bool{}
	for _, link := range menuLinks {
		if page, ok := byPath[link.Path]; ok {
			merged = append(merged, page)
			added[link.Path] = true
			continue
		}
		raw := base.ResolveReference(&url.URL{Path: link.Path}).String()
		if previewOnly {
			title := firstNonEmpty(link.Label, titleFromPath(link.Path))
			merged = append(merged, Page{
				Slug:        slugFromPath(link.Path),
				Path:        link.Path,
				Title:       title,
				ContentType: "page",
				Markdown:    "# " + title + "\n",
				SourceURL:   raw,
				Published:   publish,
			})
			added[link.Path] = true
			if len(merged) >= max {
				return merged[:max]
			}
			continue
		}
		page, err := i.fetchHTMLPageWithOptions(ctx, base, raw, publish, advanced)
		if err != nil || page.Path == "" || added[page.Path] {
			continue
		}
		merged = append(merged, page)
		added[page.Path] = true
		if len(merged) >= max {
			return merged[:max]
		}
	}
	for _, page := range pages {
		if page.Path == "" || added[page.Path] {
			continue
		}
		merged = append(merged, page)
		added[page.Path] = true
		if len(merged) >= max {
			return merged[:max]
		}
	}
	return merged
}

func menuPageLinks(menu []db.NavItem) []menuPageLink {
	var out []menuPageLink
	seen := map[string]bool{}
	for _, item := range menu {
		route := cleanMenuRoute(item.URL)
		if route == "" || seen[route] {
			continue
		}
		seen[route] = true
		out = append(out, menuPageLink{Path: route, Label: item.Label})
	}
	return out
}

func cleanMenuRoute(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.HasPrefix(raw, "//") || strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") || strings.HasPrefix(raw, "mailto:") || strings.HasPrefix(raw, "tel:") || strings.HasPrefix(raw, "#") {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	if u.Path == "" {
		return ""
	}
	cleaned := path.Clean("/" + strings.Trim(u.EscapedPath(), "/"))
	if cleaned == "/." {
		return "/"
	}
	return cleaned
}

func (i Importer) fetchWPPosts(ctx context.Context, base *url.URL, endpoint string, limit int, includeContent bool) ([]wpPost, error) {
	if limit <= 0 {
		return nil, nil
	}
	var out []wpPost
	for page := 1; page <= 20 && len(out) < limit; page++ {
		u := base.ResolveReference(&url.URL{Path: "/wp-json/wp/v2/" + endpoint})
		q := u.Query()
		q.Set("per_page", "100")
		q.Set("page", fmt.Sprint(page))
		fields := "id,slug,link,status,parent,menu_order,title,excerpt"
		if includeContent {
			fields += ",content"
		}
		q.Set("_fields", fields)
		u.RawQuery = q.Encode()
		var items []wpPost
		if err := i.getJSON(ctx, u.String(), &items); err != nil {
			if page == 1 {
				return out, err
			}
			break
		}
		if len(items) == 0 {
			break
		}
		out = append(out, items...)
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (i Importer) fetchWPMenu(ctx context.Context, base *url.URL) []db.NavItem {
	menusURL := base.ResolveReference(&url.URL{Path: "/wp-json/wp/v2/menus"}).String()
	var menus []wpMenu
	if err := i.getJSON(ctx, menusURL, &menus); err != nil || len(menus) == 0 {
		return nil
	}
	sort.SliceStable(menus, func(a, b int) bool {
		return menuRank(menus[a]) < menuRank(menus[b])
	})
	menuID := menus[0].ID
	u := base.ResolveReference(&url.URL{Path: "/wp-json/wp/v2/menu-items"})
	q := u.Query()
	q.Set("menus", fmt.Sprint(menuID))
	q.Set("per_page", "100")
	u.RawQuery = q.Encode()
	var items []wpMenuItem
	if err := i.getJSON(ctx, u.String(), &items); err != nil {
		return nil
	}
	sort.SliceStable(items, func(a, b int) bool { return items[a].MenuOrder < items[b].MenuOrder })
	out := make([]db.NavItem, 0, len(items))
	idMap := map[int]string{}
	for _, item := range items {
		idMap[item.ID] = fmt.Sprintf("wp-%d", item.ID)
	}
	for _, item := range items {
		label := strings.TrimSpace(HTMLToText(item.Title.Rendered))
		link := internalOrExternalURL(base, item.URL)
		if label == "" || link == "" {
			continue
		}
		parent := ""
		if item.Parent > 0 {
			parent = idMap[item.Parent]
		}
		out = append(out, db.NavItem{
			ID:       idMap[item.ID],
			ParentID: parent,
			Label:    label,
			URL:      link,
			External: strings.HasPrefix(link, "http://") || strings.HasPrefix(link, "https://"),
			Enabled:  true,
		})
	}
	return out
}

func (i Importer) previewSitemap(ctx context.Context, base *url.URL, opts Options) (Result, error) {
	sitemaps := i.discoverSitemaps(ctx, base)
	var urls []string
	var sitemapURL string
	for _, sm := range sitemaps {
		if ctx.Err() != nil {
			return Result{Source: "sitemap", BaseURL: base.String(), SitemapURL: sitemapURL}, ctx.Err()
		}
		found, err := i.readSitemap(ctx, sm, base, opts.MaxPages-len(urls))
		if err != nil {
			continue
		}
		if len(found) > 0 && sitemapURL == "" {
			sitemapURL = sm
		}
		urls = appendUnique(urls, found...)
		if len(urls) >= opts.MaxPages {
			break
		}
	}
	if len(urls) == 0 {
		if ctx.Err() != nil {
			return Result{Source: "sitemap", BaseURL: base.String(), SitemapURL: sitemapURL}, ctx.Err()
		}
		urls = i.discoverHomepageLinks(ctx, base, opts.MaxPages)
	}
	if len(urls) == 0 {
		if ctx.Err() != nil {
			return Result{Source: "sitemap", BaseURL: base.String(), SitemapURL: sitemapURL}, ctx.Err()
		}
		return Result{Source: "sitemap", BaseURL: base.String()}, errors.New("no importable URLs found")
	}
	pages := make([]Page, 0, len(urls))
	for _, rawURL := range urls {
		page, err := i.fetchHTMLPageWithOptions(ctx, base, rawURL, opts.Publish, opts.AdvancedScraping)
		if err != nil {
			continue
		}
		pages = append(pages, page)
		if len(pages) >= opts.MaxPages {
			break
		}
	}
	return Result{
		Source:     "sitemap",
		BaseURL:    base.String(),
		Pages:      pages,
		Menu:       i.fetchHomepageMenu(ctx, base),
		SitemapURL: sitemapURL,
	}, nil
}

func (i Importer) discoverSitemaps(ctx context.Context, base *url.URL) []string {
	var out []string
	robots := base.ResolveReference(&url.URL{Path: "/robots.txt"}).String()
	if body, _, err := i.getBytes(ctx, robots); err == nil {
		for _, line := range strings.Split(string(body), "\n") {
			key, value, ok := strings.Cut(line, ":")
			if ok && strings.EqualFold(strings.TrimSpace(key), "sitemap") {
				out = appendUnique(out, strings.TrimSpace(value))
			}
		}
	}
	for _, candidate := range []string{"/sitemap.xml", "/wp-sitemap.xml", "/sitemap_index.xml", "/page-sitemap.xml", "/post-sitemap.xml"} {
		out = appendUnique(out, base.ResolveReference(&url.URL{Path: candidate}).String())
	}
	return out
}

func (i Importer) readSitemap(ctx context.Context, sitemapURL string, base *url.URL, limit int) ([]string, error) {
	if limit <= 0 {
		return nil, nil
	}
	body, contentType, err := i.getBytes(ctx, sitemapURL)
	if err != nil {
		return nil, err
	}
	if ct, _, _ := mime.ParseMediaType(contentType); ct != "" && !strings.Contains(ct, "xml") && !strings.Contains(ct, "text/plain") {
		return nil, fmt.Errorf("not XML: %s", contentType)
	}
	decoder := xml.NewDecoder(bytes.NewReader(body))
	for {
		tok, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		start, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		switch start.Name.Local {
		case "sitemapindex":
			var index struct {
				Sitemaps []struct {
					Loc string `xml:"loc"`
				} `xml:"sitemap"`
			}
			if err := decoder.DecodeElement(&index, &start); err != nil {
				return nil, err
			}
			children := make([]string, 0, len(index.Sitemaps))
			for _, child := range index.Sitemaps {
				if loc := strings.TrimSpace(child.Loc); loc != "" && !skipSitemapChild(loc) {
					children = append(children, loc)
				}
			}
			sort.SliceStable(children, func(a, b int) bool {
				return sitemapChildRank(children[a]) < sitemapChildRank(children[b])
			})
			var urls []string
			for _, child := range children {
				found, err := i.readSitemap(ctx, child, base, limit-len(urls))
				if err == nil {
					urls = appendUnique(urls, found...)
				}
				if len(urls) >= limit {
					break
				}
			}
			return urls, nil
		case "urlset":
			var set struct {
				URLs []struct {
					Loc string `xml:"loc"`
				} `xml:"url"`
			}
			if err := decoder.DecodeElement(&set, &start); err != nil {
				return nil, err
			}
			var urls []string
			for _, item := range set.URLs {
				raw := strings.TrimSpace(item.Loc)
				if sameHostURL(base, raw) {
					urls = appendUnique(urls, raw)
				}
				if len(urls) >= limit {
					break
				}
			}
			return urls, nil
		default:
			return nil, fmt.Errorf("unsupported sitemap root: %s", start.Name.Local)
		}
	}
}

func sitemapChildRank(raw string) int {
	lower := strings.ToLower(raw)
	switch {
	case strings.Contains(lower, "wp-sitemap-posts-page") || strings.Contains(lower, "page-sitemap"):
		return 0
	case strings.Contains(lower, "wp-sitemap-posts-post") || strings.Contains(lower, "post-sitemap"):
		return 1
	case strings.Contains(lower, "wp-sitemap-posts-"):
		return 2
	default:
		return 3
	}
}

func skipSitemapChild(raw string) bool {
	lower := strings.ToLower(raw)
	skip := []string{
		"wp-sitemap-taxonomies-",
		"wp-sitemap-users-",
		"wp-sitemap-posts-wdt_headers",
		"wp-sitemap-posts-wdt_footers",
		"wp-sitemap-posts-elementor_library",
		"wp-sitemap-posts-attachment",
	}
	for _, marker := range skip {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func (i Importer) fetchHTMLPage(ctx context.Context, base *url.URL, rawURL string, publish bool) (Page, error) {
	return i.fetchHTMLPageWithOptions(ctx, base, rawURL, publish, false)
}

func (i Importer) fetchHTMLPageWithOptions(ctx context.Context, base *url.URL, rawURL string, publish, advanced bool) (Page, error) {
	body, _, err := i.getBytes(ctx, rawURL)
	if err != nil {
		return Page{}, err
	}
	doc, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return Page{}, err
	}
	title := firstNonEmpty(findMeta(doc, "og:title"), findTitle(doc), firstHeading(doc), titleFromPath(rawURL))
	description := firstNonEmpty(findMeta(doc, "description"), findMeta(doc, "og:description"))
	content := firstNode(doc, "main", "article")
	if content == nil {
		content = firstNode(doc, "body")
	}
	markdown := "# " + title + "\n\n" + HTMLNodeToMarkdown(content, base)
	if advanced {
		markdown += supplementalMediaMarkdown(string(body), base, markdown)
	}
	routePath := routePathFromURL(base, rawURL)
	return Page{
		Slug:            slugFromPath(routePath),
		Path:            routePath,
		Title:           title,
		MetaDescription: truncate(description, 180),
		ContentType:     "page",
		Markdown:        cleanMarkdown(markdown),
		SourceURL:       rawURL,
		Published:       publish,
	}, nil
}

func (i Importer) fetchHomepageMenu(ctx context.Context, base *url.URL) []db.NavItem {
	body, _, err := i.getBytes(ctx, base.String())
	if err != nil {
		return nil
	}
	doc, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return nil
	}
	root := firstNode(doc, "nav")
	if root == nil {
		root = firstNode(doc, "header")
	}
	if root == nil {
		return nil
	}
	out := buildMenuFromNav(root, base)
	if len(out) == 0 {
		out = buildFlatMenu(root, base)
	}
	if len(out) > 25 {
		out = out[:25]
	}
	return out
}

var markdownImageRe = regexp.MustCompile(`!\[([^\]]*)\]\((https?://[^)\s]+)\)`)
var cssImageURLRe = regexp.MustCompile(`(?is)background(?:-image)?\s*:[^;{}]*url\(\s*['"]?([^'")\s]+)`)

func (i Importer) localizeImages(ctx context.Context, store *db.Store, uploadDir, markdown string, cache map[string]string, failures map[string]bool) (string, []string) {
	var errors []string
	if cache == nil {
		cache = map[string]string{}
	}
	if failures == nil {
		failures = map[string]bool{}
	}
	out := markdownImageRe.ReplaceAllStringFunc(markdown, func(match string) string {
		parts := markdownImageRe.FindStringSubmatch(match)
		if len(parts) != 3 {
			return match
		}
		remote := parts[2]
		if cached, ok := cache[remote]; ok {
			return fmt.Sprintf("![%s](%s)", parts[1], cached)
		}
		if failures[remote] {
			return match
		}
		localURL, err := i.downloadImage(ctx, store, uploadDir, remote)
		if err != nil {
			errors = append(errors, fmt.Sprintf("image %s: %v", remote, err))
			failures[remote] = true
			return match
		}
		cache[remote] = localURL
		return fmt.Sprintf("![%s](%s)", parts[1], localURL)
	})
	return out, errors
}

func supplementalMediaMarkdown(raw string, base *url.URL, existing string) string {
	var b strings.Builder
	for _, imageURL := range supplementalImageURLs(raw, base) {
		if strings.Contains(existing, imageURL) || strings.Contains(existing, markdownURL(imageURL)) {
			continue
		}
		alt := titleFromPath(imageURL)
		b.WriteString("\n\n![")
		b.WriteString(alt)
		b.WriteString("](")
		b.WriteString(markdownURL(imageURL))
		b.WriteString(")")
	}
	return b.String()
}

func supplementalImageURLs(raw string, base *url.URL) []string {
	var out []string
	add := func(rawURL string) {
		imageURL := absoluteURL(base, stdhtml.UnescapeString(rawURL))
		if imageURL == "" || strings.HasPrefix(imageURL, "data:") || imageExt(imageURL, "") == "" || isLikelyDecorativeImageURL(imageURL) {
			return
		}
		out = appendUnique(out, imageURL)
	}
	if doc, err := html.Parse(strings.NewReader(raw)); err == nil {
		var walk func(*html.Node)
		walk = func(n *html.Node) {
			if n.Type == html.ElementNode {
				if n.Data == "img" {
					if src := imageNodeURL(n, base); src != "" && !isDecorativeImage(n, src) {
						add(src)
					}
				}
				for _, key := range []string{"style", "data-bg", "data-background", "data-bg-image"} {
					if value := attr(n, key); value != "" {
						for _, match := range cssImageURLRe.FindAllStringSubmatch(stdhtml.UnescapeString(value), -1) {
							if len(match) == 2 {
								add(match[1])
							}
						}
					}
				}
			}
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				walk(c)
			}
		}
		walk(doc)
	}
	for _, match := range cssImageURLRe.FindAllStringSubmatch(stdhtml.UnescapeString(raw), -1) {
		if len(match) == 2 {
			add(match[1])
		}
	}
	return out
}

func isLikelyDecorativeImageURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return true
	}
	name := strings.ToLower(path.Base(u.Path))
	for _, marker := range []string{"arrow", "caret", "chevron", "icon", "logo", "overlay", "pattern", "shape", "spacer", "spinner", "texture", "vector"} {
		if strings.Contains(name, marker) {
			return true
		}
	}
	return false
}

func (i Importer) downloadImage(ctx context.Context, store *db.Store, uploadDir, remote string) (string, error) {
	ext := imageExt(remote, "")
	if ext == "" {
		return "", errors.New("unsupported image type")
	}
	body, contentType, err := i.getBytes(ctx, remote)
	if err != nil {
		return "", err
	}
	if ext = imageExt(remote, contentType); ext == "" {
		return "", fmt.Errorf("unsupported content type %s", contentType)
	}
	if len(body) == 0 {
		return "", errors.New("empty image")
	}
	if len(body) > 8<<20 {
		return "", errors.New("image too large")
	}
	day := time.Now().UTC().Format("2006/01/02")
	dir := filepath.Join(uploadDir, day)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return "", err
	}
	name := importedImageName(remote, ext, body)
	filePath := filepath.Join(dir, name)
	if err := os.WriteFile(filePath, body, 0640); err != nil {
		return "", err
	}
	url := "/uploads/" + day + "/" + name
	if _, err := store.InsertAsset(ctx, name, filePath, url, int64(len(body))); err != nil {
		return "", err
	}
	return url, nil
}

func (i Importer) discoverHomepageLinks(ctx context.Context, base *url.URL, limit int) []string {
	body, _, err := i.getBytes(ctx, base.String())
	if err != nil {
		return nil
	}
	doc, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return nil
	}
	out := []string{base.String()}
	seen := map[string]bool{base.String(): true}
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if len(out) >= limit {
			return
		}
		if n.Type == html.ElementNode && n.Data == "a" {
			if href := absoluteURL(base, attr(n, "href")); href != "" && sameHostURL(base, href) && !seen[href] {
				seen[href] = true
				out = append(out, href)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return out
}

func buildMenuFromNav(root *html.Node, base *url.URL) []db.NavItem {
	var out []db.NavItem
	seen := map[string]bool{}
	for c := root.FirstChild; c != nil; c = c.NextSibling {
		buildMenuItems(c, base, "", &out, seen)
	}
	return out
}

func buildMenuItems(n *html.Node, base *url.URL, parentID string, out *[]db.NavItem, seen map[string]bool) {
	if shouldSkipMenuNode(n) {
		return
	}
	if n.Type == html.ElementNode {
		switch n.Data {
		case "li":
			parent := firstDirectLink(n, base)
			childRoot := firstChildNode(n, "ul", "nav")
			if parent.Label == "" && childRoot != nil {
				parent.Label = menuLabelFromNode(n)
				parent.URL = firstDescendantInternalLink(childRoot, base)
				if parent.URL == "" {
					parent.URL = firstDescendantLink(childRoot, base)
				}
			}
			if parent.Label != "" && parent.URL != "" && !seen[parent.ParentID+"|"+parent.Label+"|"+parent.URL] {
				parent.ID = fmt.Sprintf("menu-%d", len(*out)+1)
				parent.ParentID = parentID
				parent.Enabled = true
				parent.External = strings.HasPrefix(parent.URL, "http://") || strings.HasPrefix(parent.URL, "https://")
				*out = append(*out, parent)
				seen[parent.ParentID+"|"+parent.Label+"|"+parent.URL] = true
				if childRoot != nil {
					for c := childRoot.FirstChild; c != nil; c = c.NextSibling {
						buildMenuItems(c, base, parent.ID, out, seen)
					}
					return
				}
			}
		case "a":
			label := strings.TrimSpace(nodeText(n))
			link := internalOrExternalURL(base, attr(n, "href"))
			key := parentID + "|" + label + "|" + link
			if label != "" && link != "" && !seen[key] {
				seen[key] = true
				*out = append(*out, db.NavItem{
					ID:       fmt.Sprintf("menu-%d", len(*out)+1),
					ParentID: parentID,
					Label:    label,
					URL:      link,
					External: strings.HasPrefix(link, "http://") || strings.HasPrefix(link, "https://"),
					Enabled:  true,
				})
			}
			return
		case "nav":
			if parentID != "" {
				for c := n.FirstChild; c != nil; c = c.NextSibling {
					buildMenuItems(c, base, parentID, out, seen)
				}
				return
			}
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		buildMenuItems(c, base, parentID, out, seen)
	}
}

func firstDirectLink(n *html.Node, base *url.URL) db.NavItem {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if shouldSkipMenuNode(c) {
			continue
		}
		if c.Type == html.ElementNode && c.Data == "a" {
			label := strings.TrimSpace(nodeText(c))
			link := internalOrExternalURL(base, attr(c, "href"))
			if label != "" && link != "" {
				return db.NavItem{Label: label, URL: link}
			}
		}
		if c.Type == html.ElementNode && c.Data != "ul" && c.Data != "nav" {
			if item := firstDirectLink(c, base); item.Label != "" {
				return item
			}
		}
	}
	return db.NavItem{}
}

func firstChildNode(n *html.Node, names ...string) *html.Node {
	nameSet := map[string]bool{}
	for _, name := range names {
		nameSet[name] = true
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if shouldSkipMenuNode(c) {
			continue
		}
		if c.Type == html.ElementNode && nameSet[c.Data] {
			return c
		}
		if c.Type == html.ElementNode {
			if found := firstChildNode(c, names...); found != nil {
				return found
			}
		}
	}
	return nil
}

func firstDescendantInternalLink(n *html.Node, base *url.URL) string {
	link := firstDescendantLink(n, base)
	if link != "" && !strings.HasPrefix(link, "http://") && !strings.HasPrefix(link, "https://") && !strings.HasPrefix(link, "mailto:") && !strings.HasPrefix(link, "tel:") {
		return link
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if found := firstDescendantInternalLink(c, base); found != "" {
			return found
		}
	}
	return ""
}

func firstDescendantLink(n *html.Node, base *url.URL) string {
	if shouldSkipMenuNode(n) {
		return ""
	}
	if n.Type == html.ElementNode && n.Data == "a" {
		if link := internalOrExternalURL(base, attr(n, "href")); link != "" {
			return link
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if link := firstDescendantLink(c, base); link != "" {
			return link
		}
	}
	return ""
}

func menuLabelFromNode(n *html.Node) string {
	var labels []string
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if shouldSkipMenuNode(node) {
			return
		}
		if node.Type == html.ElementNode && (node.Data == "ul" || node.Data == "nav") {
			return
		}
		if node.Type == html.TextNode {
			if text := strings.TrimSpace(node.Data); text != "" {
				labels = append(labels, text)
			}
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return truncate(strings.Join(labels, " "), 60)
}

func buildFlatMenu(root *html.Node, base *url.URL) []db.NavItem {
	var out []db.NavItem
	seen := map[string]bool{}
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if shouldSkipMenuNode(n) {
			return
		}
		if n.Type == html.ElementNode && n.Data == "a" {
			label := strings.TrimSpace(nodeText(n))
			href := attr(n, "href")
			link := internalOrExternalURL(base, href)
			if label != "" && link != "" && !seen[link] {
				seen[link] = true
				out = append(out, db.NavItem{
					ID:       fmt.Sprintf("menu-%d", len(out)+1),
					Label:    label,
					URL:      link,
					External: strings.HasPrefix(link, "http://") || strings.HasPrefix(link, "https://"),
					Enabled:  true,
				})
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)
	return out
}

func shouldSkipMenuNode(n *html.Node) bool {
	if n == nil || n.Type != html.ElementNode {
		return false
	}
	className := strings.ToLower(attr(n, "class"))
	skip := []string{"show-in-tab", "tab-none", "menu-button", "w-nav-button"}
	for _, marker := range skip {
		if strings.Contains(className, marker) {
			return true
		}
	}
	return false
}

func (i Importer) getJSON(ctx context.Context, rawURL string, target any) error {
	body, _, err := i.getBytes(ctx, rawURL)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, target)
}

func (i Importer) getBytes(ctx context.Context, rawURL string) ([]byte, string, error) {
	client := i.Client
	if client == nil {
		client = &http.Client{Timeout: defaultRequestTimeout}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", "UvooMiniCMS Importer/1.0")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml,application/json,text/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Encoding", "gzip")
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, resp.Header.Get("Content-Type"), fmt.Errorf("GET %s: %s", rawURL, resp.Status)
	}
	var reader io.Reader = resp.Body
	if strings.EqualFold(resp.Header.Get("Content-Encoding"), "gzip") {
		gz, err := gzip.NewReader(resp.Body)
		if err != nil {
			return nil, "", err
		}
		defer gz.Close()
		reader = gz
	}
	body, err := io.ReadAll(io.LimitReader(reader, 8<<20))
	return body, resp.Header.Get("Content-Type"), err
}

func wpPostToPage(base *url.URL, post wpPost, contentType string, publish, advanced bool) Page {
	title := firstNonEmpty(HTMLToText(post.Title.Rendered), titleFromPath(post.Link), post.Slug)
	description := truncate(HTMLToText(post.Excerpt.Rendered), 180)
	markdownBody := HTMLToMarkdown(post.Content.Rendered, base)
	if markdownBody == "" {
		markdownBody = HTMLToText(post.Excerpt.Rendered)
	}
	if advanced {
		markdownBody += supplementalMediaMarkdown(post.Content.Rendered, base, markdownBody)
	}
	routePath := routePathFromURL(base, post.Link)
	return Page{
		Slug:            slugFromPath(firstNonEmpty(routePath, post.Slug)),
		Path:            routePath,
		Title:           title,
		MetaDescription: description,
		ContentType:     contentType,
		Markdown:        cleanMarkdown("# " + title + "\n\n" + markdownBody),
		SourceURL:       post.Link,
		Published:       publish,
	}
}

func normalizeOptions(opts Options) Options {
	opts.URL = strings.TrimSpace(opts.URL)
	if opts.MaxPages <= 0 || opts.MaxPages > 200 {
		opts.MaxPages = defaultMaxPages
	}
	return opts
}

func imageExt(rawURL, contentType string) string {
	if ct, _, _ := mime.ParseMediaType(contentType); strings.HasPrefix(ct, "image/") {
		switch ct {
		case "image/jpeg":
			return ".jpg"
		case "image/png":
			return ".png"
		case "image/gif":
			return ".gif"
		case "image/webp":
			return ".webp"
		case "image/avif":
			return ".avif"
		case "image/x-icon", "image/vnd.microsoft.icon":
			return ".ico"
		}
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	switch strings.ToLower(path.Ext(u.Path)) {
	case ".avif", ".gif", ".ico", ".jpeg", ".jpg", ".png", ".webp":
		return strings.ToLower(path.Ext(u.Path))
	default:
		return ""
	}
}

func importedImageName(rawURL, ext string, data []byte) string {
	u, _ := url.Parse(rawURL)
	base := strings.TrimSuffix(path.Base(u.Path), path.Ext(u.Path))
	base = cleanSlug(base)
	if base == "" || base == "." {
		base = "image"
	}
	sum := sha1.Sum(data)
	return fmt.Sprintf("%s-%x%s", base, sum[:5], ext)
}

func normalizeBase(raw string) (*url.URL, error) {
	if raw == "" {
		return nil, errors.New("URL required")
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, errors.New("only http and https URLs are supported")
	}
	if u.Host == "" {
		return nil, errors.New("host required")
	}
	u.Path = "/"
	u.RawQuery = ""
	u.Fragment = ""
	return u, nil
}

func routePathFromURL(base *url.URL, raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "/"
	}
	if !u.IsAbs() {
		u = base.ResolveReference(u)
	}
	cleaned := path.Clean("/" + strings.Trim(u.EscapedPath(), "/"))
	if cleaned == "/." {
		return "/"
	}
	return cleaned
}

func slugFromPath(p string) string {
	if p == "/" || p == "" {
		return "home"
	}
	slug := strings.Trim(strings.ReplaceAll(strings.Trim(p, "/"), "/", "-"), "-")
	return cleanSlug(slug)
}

var slugRe = regexp.MustCompile(`[^a-z0-9-]+`)

func cleanSlug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, " ", "-")
	s = slugRe.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}

func titleFromPath(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "Untitled"
	}
	last := path.Base(strings.Trim(u.Path, "/"))
	if last == "." || last == "/" || last == "" {
		return "Home"
	}
	words := strings.Fields(strings.ReplaceAll(strings.ReplaceAll(last, "-", " "), "_", " "))
	for i := range words {
		words[i] = strings.ToUpper(words[i][:1]) + words[i][1:]
	}
	return strings.Join(words, " ")
}

func internalOrExternalURL(base *url.URL, raw string) string {
	if raw == "" || strings.HasPrefix(raw, "#") {
		return ""
	}
	if strings.HasPrefix(raw, "mailto:") || strings.HasPrefix(raw, "tel:") {
		return raw
	}
	abs := absoluteURL(base, raw)
	if abs == "" {
		return ""
	}
	if sameHostURL(base, abs) {
		return routePathFromURL(base, abs)
	}
	return abs
}

func absoluteURL(base *url.URL, raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	if !u.IsAbs() {
		u = base.ResolveReference(u)
	}
	u.Fragment = ""
	return u.String()
}

func sameHostURL(base *url.URL, raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	if !u.IsAbs() {
		u = base.ResolveReference(u)
	}
	return strings.EqualFold(u.Hostname(), base.Hostname())
}

func appendUnique(values []string, next ...string) []string {
	seen := map[string]bool{}
	for _, value := range values {
		seen[value] = true
	}
	for _, value := range next {
		value = strings.TrimSpace(value)
		if value != "" && !seen[value] {
			seen[value] = true
			values = append(values, value)
		}
	}
	return values
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func truncate(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) <= max {
		return s
	}
	cut := max
	for cut > 0 && s[cut-1] != ' ' {
		cut--
	}
	if cut < max/2 {
		cut = max
	}
	return strings.TrimSpace(s[:cut])
}

func menuRank(menu wpMenu) int {
	name := strings.ToLower(menu.Name + " " + menu.Slug + " " + strings.Join(menu.Locations, " "))
	switch {
	case strings.Contains(name, "primary"), strings.Contains(name, "main"):
		return 0
	case strings.Contains(name, "header"), strings.Contains(name, "top"):
		return 1
	default:
		return 2
	}
}
