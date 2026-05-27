package service

import (
	"context"
	"encoding/base64"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/structpb"
	"uvoominicms/internal/db"
	"uvoominicms/internal/importer"
)

type Service struct {
	Store          *db.Store
	UploadDir      string
	MaxUploadBytes int64
	SiteName       string
}

func ok(v map[string]any) (*connect.Response[structpb.Struct], error) {
	s, err := structpb.NewStruct(v)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(s), nil
}
func fields(req *connect.Request[structpb.Struct]) map[string]any { return req.Msg.AsMap() }
func str(m map[string]any, k string) string {
	if v, ok := m[k].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}
func boolean(m map[string]any, k string) bool {
	if v, ok := m[k].(bool); ok {
		return v
	}
	return false
}
func number(m map[string]any, k string) int {
	switch v := m[k].(type) {
	case float64:
		return int(v)
	case int:
		return v
	default:
		return 0
	}
}

func (s *Service) Health(ctx context.Context, _ *connect.Request[structpb.Struct]) (*connect.Response[structpb.Struct], error) {
	return ok(map[string]any{"ok": true, "time": time.Now().UTC().Format(time.RFC3339)})
}
func (s *Service) ListPages(ctx context.Context, _ *connect.Request[structpb.Struct]) (*connect.Response[structpb.Struct], error) {
	pages, err := s.Store.ListPages(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	items := make([]any, 0, len(pages))
	for _, p := range pages {
		items = append(items, pageMap(p, false))
	}
	return ok(map[string]any{"pages": items})
}
func (s *Service) GetPage(ctx context.Context, req *connect.Request[structpb.Struct]) (*connect.Response[structpb.Struct], error) {
	p, err := s.Store.GetPage(ctx, str(fields(req), "slug"))
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}
	return ok(map[string]any{"page": pageMap(p, true)})
}
func (s *Service) SavePage(ctx context.Context, req *connect.Request[structpb.Struct]) (*connect.Response[structpb.Struct], error) {
	m := fields(req)
	slug := cleanSlug(str(m, "slug"))
	if slug == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid slug"))
	}
	routePath := cleanPath(str(m, "path"))
	if routePath == "" {
		routePath = "/" + slug
	}
	if slug == "home" {
		routePath = "/"
	} else if routePath == "/" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("only the home page can use /"))
	}
	p, err := s.Store.SavePage(ctx, db.Page{
		Slug:            slug,
		Path:            routePath,
		Title:           str(m, "title"),
		MetaDescription: str(m, "meta_description"),
		ContentType:     cleanContentType(str(m, "content_type")),
		Tags:            cleanTags(str(m, "tags")),
		Markdown:        str(m, "markdown"),
		Published:       boolean(m, "published"),
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	return ok(map[string]any{"page": pageMap(p, true)})
}
func (s *Service) DeletePage(ctx context.Context, req *connect.Request[structpb.Struct]) (*connect.Response[structpb.Struct], error) {
	if err := s.Store.DeletePage(ctx, str(fields(req), "slug")); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return ok(map[string]any{"ok": true})
}
func (s *Service) GetSettings(ctx context.Context, _ *connect.Request[structpb.Struct]) (*connect.Response[structpb.Struct], error) {
	settings, err := s.Store.GetSettings(ctx, s.SiteName)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return ok(map[string]any{"settings": settingsMap(settings)})
}
func (s *Service) SaveSettings(ctx context.Context, req *connect.Request[structpb.Struct]) (*connect.Response[structpb.Struct], error) {
	settings, err := settingsFromMap(fields(req), s.SiteName)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	settings, err = s.Store.SaveSettings(ctx, settings)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	return ok(map[string]any{"settings": settingsMap(settings)})
}
func (s *Service) ListAssets(ctx context.Context, _ *connect.Request[structpb.Struct]) (*connect.Response[structpb.Struct], error) {
	assets, err := s.Store.ListAssets(ctx, 120)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	items := make([]any, 0, len(assets))
	for _, asset := range assets {
		items = append(items, assetMap(asset))
	}
	return ok(map[string]any{"assets": items})
}
func (s *Service) GetACL(ctx context.Context, _ *connect.Request[structpb.Struct]) (*connect.Response[structpb.Struct], error) {
	settings, rules, err := s.Store.GetACL(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return ok(map[string]any{"acl": aclMap(settings, rules)})
}
func (s *Service) SaveACL(ctx context.Context, req *connect.Request[structpb.Struct]) (*connect.Response[structpb.Struct], error) {
	settings, rules, err := aclFromMap(fields(req))
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	settings, rules, err = s.Store.SaveACL(ctx, settings, rules)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	return ok(map[string]any{"acl": aclMap(settings, rules)})
}
func (s *Service) ImportPreview(ctx context.Context, req *connect.Request[structpb.Struct]) (*connect.Response[structpb.Struct], error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	result, err := importer.Importer{Client: &http.Client{Timeout: 4 * time.Second}}.Preview(ctx, importOptions(fields(req)))
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, connect.NewError(connect.CodeDeadlineExceeded, errors.New("import preview timed out after 15 seconds; the source site did not respond quickly enough"))
		}
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	return ok(map[string]any{"import": importResultMap(result)})
}
func (s *Service) ImportSite(ctx context.Context, req *connect.Request[structpb.Struct]) (*connect.Response[structpb.Struct], error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	opts := importOptions(fields(req))
	result, err := importer.Importer{Client: &http.Client{Timeout: 10 * time.Second}}.Import(ctx, s.Store, s.SiteName, opts)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, connect.NewError(connect.CodeDeadlineExceeded, errors.New("import timed out after 3 minutes; try a smaller max page count or check the source site"))
		}
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	if err := s.markImportExisting(ctx, &result); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return ok(map[string]any{"import": importResultMap(result)})
}
func (s *Service) UploadFile(ctx context.Context, req *connect.Request[structpb.Struct]) (*connect.Response[structpb.Struct], error) {
	m := fields(req)
	name := safeName(str(m, "name"))
	dataURL := str(m, "data")
	if name == "" || !strings.Contains(dataURL, ",") {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("name and data URL required"))
	}
	if !allowedUpload(name) {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("file type is not allowed"))
	}
	b64 := strings.SplitN(dataURL, ",", 2)[1]
	if int64(len(b64)) > s.MaxUploadBytes*2 {
		return nil, connect.NewError(connect.CodeResourceExhausted, errors.New("upload too large"))
	}
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	if int64(len(data)) > s.MaxUploadBytes {
		return nil, connect.NewError(connect.CodeResourceExhausted, errors.New("upload too large"))
	}
	day := time.Now().UTC().Format("2006/01/02")
	dir := filepath.Join(s.UploadDir, day)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	name = time.Now().UTC().Format("150405.000-") + name
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, 0640); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	url := "/uploads/" + day + "/" + name
	a, err := s.Store.InsertAsset(ctx, name, path, url, int64(len(data)))
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return ok(map[string]any{"asset": assetMap(a)})
}

func importOptions(m map[string]any) importer.Options {
	maxPages := number(m, "max_pages")
	if maxPages <= 0 {
		maxPages = 50
	}
	return importer.Options{
		URL:            str(m, "url"),
		MaxPages:       maxPages,
		IncludePosts:   boolean(m, "include_posts"),
		ImportMenu:     boolean(m, "import_menu"),
		Publish:        boolean(m, "publish"),
		UpdateExisting: boolean(m, "update_existing"),
	}
}

func (s *Service) markImportExisting(ctx context.Context, result *importer.Result) error {
	pages, err := s.Store.ListPages(ctx)
	if err != nil {
		return err
	}
	bySlug := map[string]bool{}
	byPath := map[string]bool{}
	for _, page := range pages {
		bySlug[page.Slug] = true
		byPath[page.Path] = true
	}
	result.Existing = 0
	for i := range result.Pages {
		exists := bySlug[result.Pages[i].Slug] || byPath[result.Pages[i].Path]
		result.Pages[i].Exists = exists
		if exists {
			result.Existing++
		}
	}
	return nil
}

func importResultMap(result importer.Result) map[string]any {
	pages := make([]any, 0, len(result.Pages))
	for _, page := range result.Pages {
		pages = append(pages, map[string]any{
			"slug":             page.Slug,
			"path":             page.Path,
			"title":            page.Title,
			"meta_description": page.MetaDescription,
			"content_type":     page.ContentType,
			"tags":             page.Tags,
			"markdown":         page.Markdown,
			"source_url":       page.SourceURL,
			"published":        page.Published,
			"exists":           page.Exists,
		})
	}
	menu := make([]any, 0, len(result.Menu))
	for _, item := range result.Menu {
		menu = append(menu, map[string]any{
			"id":        item.ID,
			"parent_id": item.ParentID,
			"label":     item.Label,
			"url":       item.URL,
			"external":  item.External,
			"enabled":   item.Enabled,
		})
	}
	errors := make([]any, 0, len(result.Errors))
	for _, err := range result.Errors {
		errors = append(errors, err)
	}
	return map[string]any{
		"source":        result.Source,
		"base_url":      result.BaseURL,
		"pages":         pages,
		"menu":          menu,
		"imported":      result.Imported,
		"skipped":       result.Skipped,
		"errors":        errors,
		"existing":      result.Existing,
		"wordpress":     result.WordPress,
		"sitemap_url":   result.SitemapURL,
		"preview_limit": result.PreviewLimit,
	}
}

func pageMap(p db.Page, body bool) map[string]any {
	m := map[string]any{
		"id":               p.ID,
		"slug":             p.Slug,
		"path":             p.Path,
		"title":            p.Title,
		"meta_description": p.MetaDescription,
		"content_type":     p.ContentType,
		"tags":             p.Tags,
		"published":        p.Published,
		"created_at":       p.CreatedAt,
		"updated_at":       p.UpdatedAt,
	}
	if body {
		m["markdown"] = p.Markdown
	}
	return m
}

func assetMap(a db.Asset) map[string]any {
	return map[string]any{
		"id":         a.ID,
		"name":       a.Name,
		"url":        a.URL,
		"size":       a.Size,
		"created_at": a.CreatedAt,
	}
}

func aclMap(settings db.SecuritySettings, rules []db.ACLRule) map[string]any {
	items := make([]any, 0, len(rules))
	for _, rule := range rules {
		items = append(items, map[string]any{
			"id":         rule.ID,
			"scope":      rule.Scope,
			"action":     rule.Action,
			"cidr":       rule.CIDR,
			"note":       rule.Note,
			"enabled":    rule.Enabled,
			"created_at": rule.CreatedAt,
		})
	}
	return map[string]any{
		"admin_default":          settings.AdminDefault,
		"public_default":         settings.PublicDefault,
		"admin_allow_countries":  settings.AdminAllowCountries,
		"admin_deny_countries":   settings.AdminDenyCountries,
		"public_allow_countries": settings.PublicAllowCountries,
		"public_deny_countries":  settings.PublicDenyCountries,
		"rules":                  items,
	}
}

func aclFromMap(m map[string]any) (db.SecuritySettings, []db.ACLRule, error) {
	settings := db.SecuritySettings{
		AdminDefault:         cleanDefault(str(m, "admin_default")),
		PublicDefault:        cleanDefault(str(m, "public_default")),
		AdminAllowCountries:  str(m, "admin_allow_countries"),
		AdminDenyCountries:   str(m, "admin_deny_countries"),
		PublicAllowCountries: str(m, "public_allow_countries"),
		PublicDenyCountries:  str(m, "public_deny_countries"),
	}
	rawRules, _ := m["rules"].([]any)
	rules := make([]db.ACLRule, 0, len(rawRules))
	for _, raw := range rawRules {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		cidr := strings.TrimSpace(str(item, "cidr"))
		if cidr == "" {
			continue
		}
		if _, _, err := net.ParseCIDR(cidr); err != nil {
			return db.SecuritySettings{}, nil, err
		}
		enabled := true
		if v, ok := item["enabled"].(bool); ok {
			enabled = v
		}
		rules = append(rules, db.ACLRule{
			Scope:   cleanACLScope(str(item, "scope")),
			Action:  cleanACLAction(str(item, "action")),
			CIDR:    cidr,
			Note:    str(item, "note"),
			Enabled: enabled,
		})
	}
	return settings, rules, nil
}

func cleanDefault(s string) string {
	if strings.EqualFold(s, "deny") {
		return "deny"
	}
	return "allow"
}
func cleanACLScope(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "admin", "public":
		return strings.ToLower(strings.TrimSpace(s))
	default:
		return "all"
	}
}
func cleanACLAction(s string) string {
	if strings.EqualFold(s, "allow") {
		return "allow"
	}
	return "deny"
}

func settingsMap(settings db.Settings) map[string]any {
	menu := make([]any, 0, len(settings.Menu))
	for _, item := range settings.Menu {
		menu = append(menu, map[string]any{
			"id":        item.ID,
			"parent_id": item.ParentID,
			"label":     item.Label,
			"url":       item.URL,
			"external":  item.External,
			"enabled":   item.Enabled,
		})
	}
	return map[string]any{
		"site_name":              settings.SiteName,
		"logo_url":               settings.LogoURL,
		"favicon_url":            settings.FaviconURL,
		"default_theme":          settings.DefaultTheme,
		"public_primary_color":   settings.PublicPrimaryColor,
		"public_secondary_color": settings.PublicSecondaryColor,
		"public_header_style":    settings.PublicHeaderStyle,
		"admin_theme":            settings.AdminTheme,
		"admin_primary_color":    settings.AdminPrimaryColor,
		"admin_secondary_color":  settings.AdminSecondaryColor,
		"admin_palette":          settings.AdminPalette,
		"footer_markdown":        settings.FooterMarkdown,
		"menu":                   menu,
		"logo_enabled":           settings.LogoEnabled,
		"favicon_enabled":        settings.FaviconEnabled,
		"menu_enabled":           settings.MenuEnabled,
		"footer_enabled":         settings.FooterEnabled,
		"theme_toggle_enabled":   settings.ThemeToggleEnabled,
		"icons_enabled":          settings.IconsEnabled,
		"search_enabled":         settings.SearchEnabled,
		"nav_layout":             settings.NavLayout,
	}
}

func settingsFromMap(m map[string]any, fallbackSiteName string) (db.Settings, error) {
	settings := db.Settings{
		SiteName:             firstNonEmpty(str(m, "site_name"), fallbackSiteName),
		LogoURL:              cleanAssetURL(str(m, "logo_url")),
		FaviconURL:           cleanAssetURL(str(m, "favicon_url")),
		DefaultTheme:         cleanTheme(str(m, "default_theme")),
		PublicPrimaryColor:   cleanHexColor(str(m, "public_primary_color")),
		PublicSecondaryColor: cleanHexColorWithDefault(str(m, "public_secondary_color"), "#64748b"),
		PublicHeaderStyle:    cleanHeaderStyle(str(m, "public_header_style")),
		AdminTheme:           cleanTheme(str(m, "admin_theme")),
		AdminPrimaryColor:    cleanHexColor(str(m, "admin_primary_color")),
		AdminSecondaryColor:  cleanHexColorWithDefault(str(m, "admin_secondary_color"), "#64748b"),
		AdminPalette:         cleanPalette(str(m, "admin_palette")),
		FooterMarkdown:       str(m, "footer_markdown"),
		Menu:                 navItems(m["menu"]),
		LogoEnabled:          boolean(m, "logo_enabled"),
		FaviconEnabled:       boolean(m, "favicon_enabled"),
		MenuEnabled:          boolean(m, "menu_enabled"),
		FooterEnabled:        boolean(m, "footer_enabled"),
		ThemeToggleEnabled:   boolean(m, "theme_toggle_enabled"),
		IconsEnabled:         boolean(m, "icons_enabled"),
		SearchEnabled:        boolean(m, "search_enabled"),
		NavLayout:            cleanNavLayout(str(m, "nav_layout")),
	}
	if settings.SiteName == "" {
		return db.Settings{}, errors.New("site name required")
	}
	return settings, nil
}

func navItems(v any) []db.NavItem {
	items, _ := v.([]any)
	out := make([]db.NavItem, 0, len(items))
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		label := str(m, "label")
		url := cleanURL(str(m, "url"))
		if label == "" || url == "" {
			continue
		}
		out = append(out, db.NavItem{
			ID:       cleanID(str(m, "id")),
			ParentID: cleanID(str(m, "parent_id")),
			Label:    label,
			URL:      url,
			External: boolean(m, "external") || strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://"),
			Enabled:  boolean(m, "enabled"),
		})
	}
	return out
}

var slugRe = regexp.MustCompile(`[^a-z0-9-]+`)

func cleanSlug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, " ", "-")
	s = slugRe.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}
func cleanPath(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return ""
	}
	if s == "/" {
		return "/"
	}
	parts := strings.Split(strings.Trim(s, "/"), "/")
	clean := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = cleanSlug(part); part != "" {
			clean = append(clean, part)
		}
	}
	if len(clean) == 0 {
		return "/"
	}
	return "/" + strings.Join(clean, "/")
}
func cleanContentType(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "post":
		return "post"
	default:
		return "page"
	}
}
func cleanTheme(s string) string {
	if strings.EqualFold(s, "dark") {
		return "dark"
	}
	return "light"
}
func cleanPalette(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "forest", "ember", "mono", "custom":
		return strings.ToLower(strings.TrimSpace(s))
	default:
		return "slate"
	}
}
func cleanHeaderStyle(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "accent-line", "accent-bg":
		return strings.ToLower(strings.TrimSpace(s))
	default:
		return "neutral"
	}
}
func cleanHexColor(s string) string {
	return cleanHexColorWithDefault(s, "#386bc0")
}
func cleanHexColorWithDefault(s, fallback string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "#") {
		s = "#" + s
	}
	if regexp.MustCompile(`^#[0-9a-fA-F]{6}$`).MatchString(s) {
		return strings.ToLower(s)
	}
	return fallback
}
func cleanNavLayout(s string) string {
	if strings.EqualFold(s, "side") {
		return "side"
	}
	return "top"
}
func cleanTags(s string) string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if tag := cleanSlug(part); tag != "" {
			out = append(out, tag)
		}
	}
	return strings.Join(out, ", ")
}
func cleanID(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	return slugRe.ReplaceAllString(strings.ToLower(s), "-")
}
func cleanAssetURL(s string) string {
	if strings.HasPrefix(s, "/uploads/") {
		return s
	}
	return ""
}
func cleanURL(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") || strings.HasPrefix(s, "mailto:") || strings.HasPrefix(s, "tel:") {
		return s
	}
	if s == "" {
		return ""
	}
	if !strings.HasPrefix(s, "/") {
		s = "/" + s
	}
	return cleanPath(s)
}
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
func safeName(s string) string {
	s = filepath.Base(s)
	e := ext(s)
	name := strings.TrimSuffix(s, filepath.Ext(s))
	name = strings.ReplaceAll(name, " ", "-")
	name = strings.Trim(slugRe.ReplaceAllString(strings.ToLower(name), "-"), "-")
	if name == "" {
		name = "upload"
	}
	return name + e
}
func ext(s string) string {
	e := strings.ToLower(filepath.Ext(s))
	if len(e) > 12 {
		return ""
	}
	return e
}

func allowedUpload(name string) bool {
	switch ext(name) {
	case ".avif", ".gif", ".ico", ".jpeg", ".jpg", ".png", ".webp", ".pdf", ".txt", ".md", ".csv":
		return true
	default:
		return false
	}
}

func JSONError(w http.ResponseWriter, code int, msg string) { http.Error(w, msg, code) }
