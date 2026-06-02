package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type Store struct{ DB *sql.DB }

type Page struct {
	ID              int64  `json:"id"`
	Slug            string `json:"slug"`
	Path            string `json:"path"`
	Title           string `json:"title"`
	MetaDescription string `json:"meta_description"`
	ContentType     string `json:"content_type"`
	Tags            string `json:"tags"`
	Markdown        string `json:"markdown,omitempty"`
	Published       bool   `json:"published"`
	CreatedAt       string `json:"created_at"`
	UpdatedAt       string `json:"updated_at"`
}

type Asset struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Path      string `json:"path"`
	URL       string `json:"url"`
	Size      int64  `json:"size"`
	CreatedAt string `json:"created_at"`
}

type SecuritySettings struct {
	AdminDefault         string `json:"admin_default"`
	PublicDefault        string `json:"public_default"`
	AdminAllowCountries  string `json:"admin_allow_countries"`
	AdminDenyCountries   string `json:"admin_deny_countries"`
	PublicAllowCountries string `json:"public_allow_countries"`
	PublicDenyCountries  string `json:"public_deny_countries"`
}

type ACLRule struct {
	ID        int64  `json:"id"`
	Scope     string `json:"scope"`
	Action    string `json:"action"`
	CIDR      string `json:"cidr"`
	Note      string `json:"note"`
	Enabled   bool   `json:"enabled"`
	CreatedAt string `json:"created_at"`
}

type NavItem struct {
	ID       string `json:"id"`
	Type     string `json:"type,omitempty"`
	ParentID string `json:"parent_id"`
	Label    string `json:"label"`
	URL      string `json:"url"`
	External bool   `json:"external"`
	Enabled  bool   `json:"enabled"`
}

type Settings struct {
	SiteName             string    `json:"site_name"`
	LogoURL              string    `json:"logo_url"`
	FaviconURL           string    `json:"favicon_url"`
	DefaultTheme         string    `json:"default_theme"`
	PublicThemeStyle     string    `json:"public_theme_style"`
	PublicPrimaryColor   string    `json:"public_primary_color"`
	PublicSecondaryColor string    `json:"public_secondary_color"`
	PublicHeaderStyle    string    `json:"public_header_style"`
	AdminTheme           string    `json:"admin_theme"`
	ThemeStyle           string    `json:"theme_style"`
	AdminPrimaryColor    string    `json:"admin_primary_color"`
	AdminSecondaryColor  string    `json:"admin_secondary_color"`
	AdminPalette         string    `json:"admin_palette"`
	FooterMarkdown       string    `json:"footer_markdown"`
	Menu                 []NavItem `json:"menu"`
	LogoEnabled          bool      `json:"logo_enabled"`
	FaviconEnabled       bool      `json:"favicon_enabled"`
	MenuEnabled          bool      `json:"menu_enabled"`
	FooterEnabled        bool      `json:"footer_enabled"`
	ThemeToggleEnabled   bool      `json:"theme_toggle_enabled"`
	IconsEnabled         bool      `json:"icons_enabled"`
	SearchEnabled        bool      `json:"search_enabled"`
	NavLayout            string    `json:"nav_layout"`
}

type ThemeHistory struct {
	ID                   int64  `json:"id"`
	AdminTheme           string `json:"admin_theme"`
	ThemeStyle           string `json:"theme_style"`
	AdminPrimaryColor    string `json:"admin_primary_color"`
	AdminSecondaryColor  string `json:"admin_secondary_color"`
	AdminPalette         string `json:"admin_palette"`
	PublicTheme          string `json:"public_theme"`
	PublicThemeStyle     string `json:"public_theme_style"`
	PublicPrimaryColor   string `json:"public_primary_color"`
	PublicSecondaryColor string `json:"public_secondary_color"`
	PublicHeaderStyle    string `json:"public_header_style"`
	UpdatedAt            string `json:"updated_at"`
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(dir(path), 0700); err != nil {
		return nil, err
	}
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(ON)", path)
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	st := &Store{DB: db}
	return st, st.migrate()
}

func dir(p string) string {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' {
			return p[:i]
		}
	}
	return "."
}

func (s *Store) migrate() error {
	_, err := s.DB.Exec(`
CREATE TABLE IF NOT EXISTS pages (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  slug TEXT NOT NULL UNIQUE,
  path TEXT NOT NULL DEFAULT '',
  title TEXT NOT NULL,
  meta_description TEXT NOT NULL DEFAULT '',
  content_type TEXT NOT NULL DEFAULT 'page',
  tags TEXT NOT NULL DEFAULT '',
  markdown TEXT NOT NULL DEFAULT '',
  published INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX IF NOT EXISTS idx_pages_published_slug ON pages(published, slug);
CREATE TABLE IF NOT EXISTS assets (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL,
  path TEXT NOT NULL,
  url TEXT NOT NULL,
  size INTEGER NOT NULL,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE TABLE IF NOT EXISTS settings (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL,
  updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE TABLE IF NOT EXISTS theme_history (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  admin_theme TEXT NOT NULL,
  theme_style TEXT NOT NULL DEFAULT 'soft',
  admin_primary_color TEXT NOT NULL,
  admin_secondary_color TEXT NOT NULL,
  admin_palette TEXT NOT NULL,
  public_theme TEXT NOT NULL,
  public_theme_style TEXT NOT NULL DEFAULT 'soft',
  public_primary_color TEXT NOT NULL,
  public_secondary_color TEXT NOT NULL,
  public_header_style TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  UNIQUE(admin_theme, theme_style, admin_primary_color, admin_secondary_color, admin_palette, public_theme, public_theme_style, public_primary_color, public_secondary_color, public_header_style)
);
CREATE TABLE IF NOT EXISTS acl_settings (
  id INTEGER PRIMARY KEY CHECK (id=1),
  admin_default TEXT NOT NULL DEFAULT 'allow',
  public_default TEXT NOT NULL DEFAULT 'allow',
  admin_allow_countries TEXT NOT NULL DEFAULT '',
  admin_deny_countries TEXT NOT NULL DEFAULT '',
  public_allow_countries TEXT NOT NULL DEFAULT '',
  public_deny_countries TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
INSERT OR IGNORE INTO acl_settings(id) VALUES(1);
CREATE TABLE IF NOT EXISTS acl_rules (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  scope TEXT NOT NULL,
  action TEXT NOT NULL,
  cidr TEXT NOT NULL,
  note TEXT NOT NULL DEFAULT '',
  enabled INTEGER NOT NULL DEFAULT 1,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
INSERT INTO pages(slug,title,markdown,published)
SELECT 'home','Home','# Welcome to UvooMiniCMS\n\nEdit this page from /admin.',1
WHERE NOT EXISTS (SELECT 1 FROM pages WHERE slug='home');`)
	if err != nil {
		return err
	}
	for _, stmt := range []string{
		`ALTER TABLE pages ADD COLUMN path TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE pages ADD COLUMN meta_description TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE pages ADD COLUMN content_type TEXT NOT NULL DEFAULT 'page'`,
		`ALTER TABLE pages ADD COLUMN tags TEXT NOT NULL DEFAULT ''`,
	} {
		if _, err := s.DB.Exec(stmt); err != nil && !strings.Contains(err.Error(), "duplicate column name") {
			return err
		}
	}
	_, err = s.DB.Exec(`
UPDATE pages SET path='/' WHERE slug='home' AND path='';
UPDATE pages SET path='/' || slug WHERE slug <> 'home' AND path='';
CREATE UNIQUE INDEX IF NOT EXISTS idx_pages_path ON pages(path);
CREATE INDEX IF NOT EXISTS idx_pages_published_path ON pages(published, path);`)
	return err
}

func (s *Store) ListPages(ctx context.Context) ([]Page, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id,slug,path,title,meta_description,content_type,tags,published,created_at,updated_at FROM pages ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var pages []Page
	for rows.Next() {
		var p Page
		var pub int
		if err := rows.Scan(&p.ID, &p.Slug, &p.Path, &p.Title, &p.MetaDescription, &p.ContentType, &p.Tags, &pub, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		p.Published = pub == 1
		pages = append(pages, p)
	}
	return pages, rows.Err()
}

func (s *Store) GetPage(ctx context.Context, slug string) (Page, error) {
	var p Page
	var pub int
	err := s.DB.QueryRowContext(ctx, `SELECT id,slug,path,title,meta_description,content_type,tags,markdown,published,created_at,updated_at FROM pages WHERE slug=?`, slug).Scan(&p.ID, &p.Slug, &p.Path, &p.Title, &p.MetaDescription, &p.ContentType, &p.Tags, &p.Markdown, &pub, &p.CreatedAt, &p.UpdatedAt)
	p.Published = pub == 1
	return p, err
}

func (s *Store) GetPublishedByPath(ctx context.Context, path string) (Page, error) {
	var p Page
	var pub int
	err := s.DB.QueryRowContext(ctx, `SELECT id,slug,path,title,meta_description,content_type,tags,markdown,published,created_at,updated_at FROM pages WHERE path=? AND published=1`, path).Scan(&p.ID, &p.Slug, &p.Path, &p.Title, &p.MetaDescription, &p.ContentType, &p.Tags, &p.Markdown, &pub, &p.CreatedAt, &p.UpdatedAt)
	p.Published = pub == 1
	return p, err
}

func (s *Store) SavePage(ctx context.Context, p Page) (Page, error) {
	if p.Slug == "" || p.Path == "" || p.Title == "" {
		return Page{}, errors.New("slug, path, and title required")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.DB.ExecContext(ctx, `INSERT INTO pages(slug,path,title,meta_description,content_type,tags,markdown,published,updated_at) VALUES(?,?,?,?,?,?,?,?,?)
ON CONFLICT(slug) DO UPDATE SET path=excluded.path, title=excluded.title, meta_description=excluded.meta_description, content_type=excluded.content_type, tags=excluded.tags, markdown=excluded.markdown, published=excluded.published, updated_at=excluded.updated_at`, p.Slug, p.Path, p.Title, p.MetaDescription, p.ContentType, p.Tags, p.Markdown, boolInt(p.Published), now)
	if err != nil {
		return Page{}, err
	}
	return s.GetPage(ctx, p.Slug)
}

func (s *Store) SearchPages(ctx context.Context, query string) ([]Page, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}
	like := "%" + query + "%"
	rows, err := s.DB.QueryContext(ctx, `SELECT id,slug,path,title,meta_description,content_type,tags,published,created_at,updated_at FROM pages
WHERE published=1 AND (title LIKE ? OR meta_description LIKE ? OR tags LIKE ? OR markdown LIKE ?)
ORDER BY updated_at DESC LIMIT 50`, like, like, like, like)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var pages []Page
	for rows.Next() {
		var p Page
		var pub int
		if err := rows.Scan(&p.ID, &p.Slug, &p.Path, &p.Title, &p.MetaDescription, &p.ContentType, &p.Tags, &pub, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		p.Published = pub == 1
		pages = append(pages, p)
	}
	return pages, rows.Err()
}

func (s *Store) DeletePage(ctx context.Context, slug string) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM pages WHERE slug=? AND slug <> 'home'`, slug)
	return err
}

func (s *Store) InsertAsset(ctx context.Context, name, path, url string, size int64) (Asset, error) {
	res, err := s.DB.ExecContext(ctx, `INSERT INTO assets(name,path,url,size) VALUES(?,?,?,?)`, name, path, url, size)
	if err != nil {
		return Asset{}, err
	}
	id, _ := res.LastInsertId()
	return Asset{ID: id, Name: name, Path: path, URL: url, Size: size, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)}, nil
}

func (s *Store) ListAssets(ctx context.Context, limit int) ([]Asset, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT id,name,path,url,size,created_at FROM assets ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var assets []Asset
	for rows.Next() {
		var a Asset
		if err := rows.Scan(&a.ID, &a.Name, &a.Path, &a.URL, &a.Size, &a.CreatedAt); err != nil {
			return nil, err
		}
		assets = append(assets, a)
	}
	return assets, rows.Err()
}

func (s *Store) GetAsset(ctx context.Context, id int64) (Asset, error) {
	var a Asset
	err := s.DB.QueryRowContext(ctx, `SELECT id,name,path,url,size,created_at FROM assets WHERE id=?`, id).Scan(&a.ID, &a.Name, &a.Path, &a.URL, &a.Size, &a.CreatedAt)
	if err != nil {
		return Asset{}, err
	}
	return a, nil
}

func (s *Store) DeleteAsset(ctx context.Context, id int64) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM assets WHERE id=?`, id)
	return err
}

func (s *Store) GetACL(ctx context.Context) (SecuritySettings, []ACLRule, error) {
	settings := SecuritySettings{AdminDefault: "allow", PublicDefault: "allow"}
	err := s.DB.QueryRowContext(ctx, `SELECT admin_default,public_default,admin_allow_countries,admin_deny_countries,public_allow_countries,public_deny_countries FROM acl_settings WHERE id=1`).Scan(&settings.AdminDefault, &settings.PublicDefault, &settings.AdminAllowCountries, &settings.AdminDenyCountries, &settings.PublicAllowCountries, &settings.PublicDenyCountries)
	if err != nil {
		return SecuritySettings{}, nil, err
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT id,scope,action,cidr,note,enabled,created_at FROM acl_rules ORDER BY id`)
	if err != nil {
		return SecuritySettings{}, nil, err
	}
	defer rows.Close()
	var rules []ACLRule
	for rows.Next() {
		var rule ACLRule
		var enabled int
		if err := rows.Scan(&rule.ID, &rule.Scope, &rule.Action, &rule.CIDR, &rule.Note, &enabled, &rule.CreatedAt); err != nil {
			return SecuritySettings{}, nil, err
		}
		rule.Enabled = enabled == 1
		rules = append(rules, rule)
	}
	return normalizeSecurity(settings), rules, rows.Err()
}

func (s *Store) SaveACL(ctx context.Context, settings SecuritySettings, rules []ACLRule) (SecuritySettings, []ACLRule, error) {
	settings = normalizeSecurity(settings)
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return SecuritySettings{}, nil, err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `UPDATE acl_settings SET admin_default=?, public_default=?, admin_allow_countries=?, admin_deny_countries=?, public_allow_countries=?, public_deny_countries=?, updated_at=? WHERE id=1`, settings.AdminDefault, settings.PublicDefault, settings.AdminAllowCountries, settings.AdminDenyCountries, settings.PublicAllowCountries, settings.PublicDenyCountries, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return SecuritySettings{}, nil, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM acl_rules`); err != nil {
		return SecuritySettings{}, nil, err
	}
	for _, rule := range rules {
		rule.Scope = normalizeScope(rule.Scope)
		rule.Action = normalizeAction(rule.Action)
		if rule.CIDR = strings.TrimSpace(rule.CIDR); rule.CIDR == "" {
			continue
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO acl_rules(scope,action,cidr,note,enabled) VALUES(?,?,?,?,?)`, rule.Scope, rule.Action, rule.CIDR, strings.TrimSpace(rule.Note), boolInt(rule.Enabled))
		if err != nil {
			return SecuritySettings{}, nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return SecuritySettings{}, nil, err
	}
	return s.GetACL(ctx)
}

func (s *Store) GetSettings(ctx context.Context, fallbackSiteName string) (Settings, error) {
	settings := DefaultSettings(fallbackSiteName)
	var raw string
	err := s.DB.QueryRowContext(ctx, `SELECT value FROM settings WHERE key='site'`).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return settings, nil
	}
	if err != nil {
		return Settings{}, err
	}
	if err := json.Unmarshal([]byte(raw), &settings); err != nil {
		return Settings{}, err
	}
	if !strings.Contains(raw, "_enabled") {
		settings.LogoEnabled = true
		settings.FaviconEnabled = true
		settings.MenuEnabled = true
		settings.FooterEnabled = true
		settings.ThemeToggleEnabled = true
		settings.IconsEnabled = true
		settings.SearchEnabled = true
	}
	if !strings.Contains(raw, `"enabled"`) {
		for i := range settings.Menu {
			settings.Menu[i].Enabled = true
		}
	}
	if settings.SiteName == "" {
		settings.SiteName = fallbackSiteName
	}
	if settings.DefaultTheme != "dark" {
		settings.DefaultTheme = "light"
	}
	if settings.PublicPrimaryColor == "" {
		settings.PublicPrimaryColor = "#386bc0"
	}
	if settings.PublicSecondaryColor == "" {
		settings.PublicSecondaryColor = "#64748b"
	}
	settings.PublicThemeStyle = normalizeThemeStyle(settings.PublicThemeStyle)
	if settings.PublicHeaderStyle != "accent-line" && settings.PublicHeaderStyle != "accent-bg" {
		settings.PublicHeaderStyle = "neutral"
	}
	if settings.AdminTheme != "dark" {
		settings.AdminTheme = "light"
	}
	settings.ThemeStyle = normalizeThemeStyle(settings.ThemeStyle)
	if settings.PublicHeaderStyle != "accent-line" && settings.PublicHeaderStyle != "accent-bg" {
		settings.PublicHeaderStyle = "neutral"
	}
	if settings.AdminPrimaryColor == "" {
		settings.AdminPrimaryColor = "#386bc0"
	}
	if settings.AdminSecondaryColor == "" {
		settings.AdminSecondaryColor = "#64748b"
	}
	if settings.AdminPalette == "" {
		settings.AdminPalette = "slate"
	}
	if settings.NavLayout == "" {
		settings.NavLayout = "top"
	}
	normalizeSettings(&settings)
	return settings, nil
}

func (s *Store) SaveSettings(ctx context.Context, settings Settings) (Settings, error) {
	if settings.SiteName == "" {
		return Settings{}, errors.New("site name required")
	}
	if settings.DefaultTheme != "dark" {
		settings.DefaultTheme = "light"
	}
	settings.PublicThemeStyle = normalizeThemeStyle(settings.PublicThemeStyle)
	if settings.AdminTheme != "dark" {
		settings.AdminTheme = "light"
	}
	settings.ThemeStyle = normalizeThemeStyle(settings.ThemeStyle)
	if settings.AdminPrimaryColor == "" {
		settings.AdminPrimaryColor = "#386bc0"
	}
	if settings.AdminSecondaryColor == "" {
		settings.AdminSecondaryColor = "#64748b"
	}
	if settings.AdminPalette == "" {
		settings.AdminPalette = "slate"
	}
	if settings.NavLayout != "side" {
		settings.NavLayout = "top"
	}
	if settings.Menu == nil {
		settings.Menu = []NavItem{}
	}
	normalizeSettings(&settings)
	raw, err := json.Marshal(settings)
	if err != nil {
		return Settings{}, err
	}
	_, err = s.DB.ExecContext(ctx, `INSERT INTO settings(key,value,updated_at) VALUES('site',?,?)
ON CONFLICT(key) DO UPDATE SET value=excluded.value, updated_at=excluded.updated_at`, string(raw), time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return Settings{}, err
	}
	if err := s.SaveThemeHistory(ctx, settings); err != nil {
		return Settings{}, err
	}
	return settings, nil
}

func (s *Store) SaveThemeHistory(ctx context.Context, settings Settings) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.DB.ExecContext(ctx, `INSERT INTO theme_history(admin_theme,theme_style,admin_primary_color,admin_secondary_color,admin_palette,public_theme,public_theme_style,public_primary_color,public_secondary_color,public_header_style,updated_at)
VALUES(?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(admin_theme, theme_style, admin_primary_color, admin_secondary_color, admin_palette, public_theme, public_theme_style, public_primary_color, public_secondary_color, public_header_style)
DO UPDATE SET updated_at=excluded.updated_at`,
		settings.AdminTheme,
		settings.ThemeStyle,
		settings.AdminPrimaryColor,
		settings.AdminSecondaryColor,
		settings.AdminPalette,
		settings.DefaultTheme,
		settings.PublicThemeStyle,
		settings.PublicPrimaryColor,
		settings.PublicSecondaryColor,
		settings.PublicHeaderStyle,
		now,
	)
	return err
}

func (s *Store) ListThemeHistory(ctx context.Context, limit int) ([]ThemeHistory, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT id,admin_theme,theme_style,admin_primary_color,admin_secondary_color,admin_palette,public_theme,public_theme_style,public_primary_color,public_secondary_color,public_header_style,updated_at FROM theme_history ORDER BY updated_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []ThemeHistory
	for rows.Next() {
		var item ThemeHistory
		if err := rows.Scan(&item.ID, &item.AdminTheme, &item.ThemeStyle, &item.AdminPrimaryColor, &item.AdminSecondaryColor, &item.AdminPalette, &item.PublicTheme, &item.PublicThemeStyle, &item.PublicPrimaryColor, &item.PublicSecondaryColor, &item.PublicHeaderStyle, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func DefaultSettings(siteName string) Settings {
	if siteName == "" {
		siteName = "UvooMiniCMS"
	}
	return Settings{
		SiteName:             siteName,
		DefaultTheme:         "light",
		PublicThemeStyle:     "soft",
		PublicPrimaryColor:   "#386bc0",
		PublicSecondaryColor: "#64748b",
		PublicHeaderStyle:    "neutral",
		AdminTheme:           "light",
		ThemeStyle:           "soft",
		AdminPrimaryColor:    "#386bc0",
		AdminSecondaryColor:  "#64748b",
		AdminPalette:         "slate",
		FooterMarkdown:       fmt.Sprintf("© %d %s. All rights reserved.", time.Now().UTC().Year(), siteName),
		LogoEnabled:          true,
		FaviconEnabled:       true,
		MenuEnabled:          true,
		FooterEnabled:        true,
		ThemeToggleEnabled:   true,
		IconsEnabled:         true,
		SearchEnabled:        true,
		NavLayout:            "top",
		Menu: []NavItem{
			{ID: "home", Type: "link", Label: "Home", URL: "/", External: false, Enabled: true},
		},
	}
}

func normalizeSettings(settings *Settings) {
	for i := range settings.Menu {
		if settings.Menu[i].ID == "" {
			settings.Menu[i].ID = fmt.Sprintf("item-%d", i+1)
		}
		if settings.Menu[i].Type != "section" {
			settings.Menu[i].Type = "link"
		}
		if settings.Menu[i].Type == "section" {
			settings.Menu[i].URL = ""
			settings.Menu[i].External = false
		}
	}
}

func normalizeThemeStyle(style string) string {
	switch strings.ToLower(strings.TrimSpace(style)) {
	case "square", "material":
		return strings.ToLower(strings.TrimSpace(style))
	default:
		return "soft"
	}
}

func normalizeSecurity(settings SecuritySettings) SecuritySettings {
	if settings.AdminDefault != "deny" {
		settings.AdminDefault = "allow"
	}
	if settings.PublicDefault != "deny" {
		settings.PublicDefault = "allow"
	}
	settings.AdminAllowCountries = normalizeCountries(settings.AdminAllowCountries)
	settings.AdminDenyCountries = normalizeCountries(settings.AdminDenyCountries)
	settings.PublicAllowCountries = normalizeCountries(settings.PublicAllowCountries)
	settings.PublicDenyCountries = normalizeCountries(settings.PublicDenyCountries)
	return settings
}

func normalizeScope(scope string) string {
	switch strings.ToLower(strings.TrimSpace(scope)) {
	case "admin", "public":
		return strings.ToLower(strings.TrimSpace(scope))
	default:
		return "all"
	}
}

func normalizeAction(action string) string {
	if strings.EqualFold(strings.TrimSpace(action), "allow") {
		return "allow"
	}
	return "deny"
}

func normalizeCountries(raw string) string {
	seen := map[string]bool{}
	var out []string
	for _, part := range strings.Split(raw, ",") {
		code := strings.ToUpper(strings.TrimSpace(part))
		if len(code) == 2 && !seen[code] {
			seen[code] = true
			out = append(out, code)
		}
	}
	return strings.Join(out, ",")
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
