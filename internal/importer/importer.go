package importer

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"

	"golang.org/x/net/html"
	"uvoominicms/internal/db"
)

const defaultMaxPages = 50
const defaultRequestTimeout = 6 * time.Second

type Importer struct {
	Client *http.Client
}

type Options struct {
	URL            string
	MaxPages       int
	IncludePosts   bool
	ImportMenu     bool
	Publish        bool
	UpdateExisting bool
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

func (i Importer) Import(ctx context.Context, store *db.Store, siteName string, opts Options) (Result, error) {
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
		items, err := i.fetchWPPosts(ctx, base, endpoint, opts.MaxPages-len(pages))
		if err != nil {
			continue
		}
		for _, item := range items {
			page := wpPostToPage(base, item, collection.Kind, opts.Publish)
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
	return result, nil
}

func (i Importer) fetchWPPosts(ctx context.Context, base *url.URL, endpoint string, limit int) ([]wpPost, error) {
	if limit <= 0 {
		return nil, nil
	}
	var out []wpPost
	for page := 1; page <= 20 && len(out) < limit; page++ {
		u := base.ResolveReference(&url.URL{Path: "/wp-json/wp/v2/" + endpoint})
		q := u.Query()
		q.Set("per_page", "100")
		q.Set("page", fmt.Sprint(page))
		q.Set("_fields", "id,slug,link,status,parent,menu_order,title,content,excerpt")
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
		page, err := i.fetchHTMLPage(ctx, base, rawURL, opts.Publish)
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
			var urls []string
			for _, child := range index.Sitemaps {
				found, err := i.readSitemap(ctx, strings.TrimSpace(child.Loc), base, limit-len(urls))
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

func (i Importer) fetchHTMLPage(ctx context.Context, base *url.URL, rawURL string, publish bool) (Page, error) {
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
	var out []db.NavItem
	seen := map[string]bool{}
	var walk func(*html.Node)
	walk = func(n *html.Node) {
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
	if len(out) > 25 {
		out = out[:25]
	}
	return out
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

func wpPostToPage(base *url.URL, post wpPost, contentType string, publish bool) Page {
	title := firstNonEmpty(HTMLToText(post.Title.Rendered), titleFromPath(post.Link), post.Slug)
	description := truncate(HTMLToText(post.Excerpt.Rendered), 180)
	markdownBody := HTMLToMarkdown(post.Content.Rendered, base)
	if markdownBody == "" {
		markdownBody = HTMLToText(post.Excerpt.Rendered)
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
