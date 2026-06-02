package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/structpb"
	"uvoominicms/internal/db"
)

func TestSetSiteImageOptimizesLogoAndFavicon(t *testing.T) {
	store, err := db.Open(t.TempDir() + "/cms.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.DB.Close()
	svc := &Service{Store: store, UploadDir: t.TempDir(), MaxUploadBytes: 2 << 20, SiteName: "Demo"}

	callSetSiteImage(t, svc, "logo", "wide-logo.png", pngDataURL(t, 1200, 300))
	settings, err := store.GetSettings(context.Background(), "Demo")
	if err != nil {
		t.Fatal(err)
	}
	if settings.LogoURL == "" || !settings.LogoEnabled {
		t.Fatalf("logo was not saved into settings: %#v", settings)
	}
	assets, err := store.ListAssets(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(assets) != 1 {
		t.Fatalf("expected 1 asset, got %d", len(assets))
	}
	w, h := imageSize(t, assets[0].Path)
	if w > 512 || h > 160 {
		t.Fatalf("logo dimensions not constrained: %dx%d", w, h)
	}

	callSetSiteImage(t, svc, "favicon", "favicon-source.jpg", pngDataURL(t, 300, 180))
	settings, err = store.GetSettings(context.Background(), "Demo")
	if err != nil {
		t.Fatal(err)
	}
	if settings.FaviconURL == "" || !settings.FaviconEnabled {
		t.Fatalf("favicon was not saved into settings: %#v", settings)
	}
	assets, err = store.ListAssets(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(assets) != 2 {
		t.Fatalf("expected 2 assets, got %d", len(assets))
	}
	w, h = imageSize(t, assets[0].Path)
	if w != 64 || h != 64 {
		t.Fatalf("favicon should be 64x64, got %dx%d", w, h)
	}
}

func TestSetSiteImageOptimizesFromURLAndKeepsExternalSettingURL(t *testing.T) {
	store, err := db.Open(t.TempDir() + "/cms.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.DB.Close()
	svc := &Service{Store: store, UploadDir: t.TempDir(), MaxUploadBytes: 2 << 20, SiteName: "Demo"}
	source := pngBytes(t, 400, 240)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write(source)
	}))
	defer server.Close()

	callSetSiteImageURL(t, svc, "favicon", server.URL+"/favicon.png")
	settings, err := store.GetSettings(context.Background(), "Demo")
	if err != nil {
		t.Fatal(err)
	}
	if settings.FaviconURL == "" || settings.FaviconURL == server.URL+"/favicon.png" {
		t.Fatalf("expected favicon to be downloaded to local uploads, got %#v", settings.FaviconURL)
	}
	assets, err := store.ListAssets(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	w, h := imageSize(t, assets[0].Path)
	if w != 64 || h != 64 {
		t.Fatalf("favicon should be 64x64, got %dx%d", w, h)
	}

	settings.LogoURL = "https://cdn.example.com/logo.png"
	settings, err = store.SaveSettings(context.Background(), settings)
	if err != nil {
		t.Fatal(err)
	}
	mapped, err := settingsFromMap(settingsMap(settings), "Demo")
	if err != nil {
		t.Fatal(err)
	}
	if mapped.LogoURL != "https://cdn.example.com/logo.png" {
		t.Fatalf("external logo URL should be preserved, got %q", mapped.LogoURL)
	}
}

func TestValidateNavItemsRejectsMissingParent(t *testing.T) {
	err := validateNavItems([]db.NavItem{
		{ID: "child", Type: "link", ParentID: "missing", Label: "Child", URL: "/child", Enabled: true},
	})
	if err == nil || !strings.Contains(err.Error(), "missing parent") {
		t.Fatalf("expected missing parent error, got %v", err)
	}
}

func TestValidateNavItemsRejectsParentCycle(t *testing.T) {
	err := validateNavItems([]db.NavItem{
		{ID: "a", Type: "section", ParentID: "b", Label: "A", Enabled: true},
		{ID: "b", Type: "section", ParentID: "a", Label: "B", Enabled: true},
	})
	if err == nil || !strings.Contains(err.Error(), "parent cycle") {
		t.Fatalf("expected parent cycle error, got %v", err)
	}
}

func TestValidateNavItemsAcceptsNestedTree(t *testing.T) {
	err := validateNavItems([]db.NavItem{
		{ID: "services", Type: "link", Label: "Services", URL: "/services", Enabled: true},
		{ID: "support", Type: "link", ParentID: "services", Label: "Support", URL: "/services/support", Enabled: true},
		{ID: "resources", Type: "section", Label: "Resources", Enabled: true},
		{ID: "blog", Type: "link", ParentID: "resources", Label: "Blog", URL: "/blog", Enabled: true},
	})
	if err != nil {
		t.Fatalf("expected valid tree, got %v", err)
	}
}

func TestSetSiteImagePreservesPNGTransparency(t *testing.T) {
	store, err := db.Open(t.TempDir() + "/cms.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.DB.Close()
	svc := &Service{Store: store, UploadDir: t.TempDir(), MaxUploadBytes: 2 << 20, SiteName: "Demo"}

	callSetSiteImage(t, svc, "logo", "transparent-logo.png", transparentPNGDataURL(t, 1200, 300))
	assets, err := store.ListAssets(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(assets) != 1 {
		t.Fatalf("expected 1 asset, got %d", len(assets))
	}
	if alpha := imageAlphaAt(t, assets[0].Path, 0, 0); alpha != 0 {
		t.Fatalf("logo transparent corner was flattened, alpha=%d", alpha)
	}

	callSetSiteImage(t, svc, "favicon", "transparent-favicon.png", transparentPNGDataURL(t, 128, 128))
	assets, err = store.ListAssets(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(assets) != 2 {
		t.Fatalf("expected 2 assets, got %d", len(assets))
	}
	if alpha := imageAlphaAt(t, assets[0].Path, 0, 0); alpha != 0 {
		t.Fatalf("favicon transparent corner was flattened, alpha=%d", alpha)
	}
}

func TestDeleteAssetRemovesFileAndClearsIdentitySetting(t *testing.T) {
	store, err := db.Open(t.TempDir() + "/cms.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.DB.Close()
	svc := &Service{Store: store, UploadDir: t.TempDir(), MaxUploadBytes: 2 << 20, SiteName: "Demo"}

	callSetSiteImage(t, svc, "logo", "logo.png", pngDataURL(t, 120, 40))
	assets, err := store.ListAssets(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(assets) != 1 {
		t.Fatalf("expected 1 asset, got %d", len(assets))
	}
	path := assets[0].Path

	callDeleteAsset(t, svc, assets[0].ID)
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected uploaded file to be removed, stat err=%v", err)
	}
	settings, err := store.GetSettings(context.Background(), "Demo")
	if err != nil {
		t.Fatal(err)
	}
	if settings.LogoURL != "" || settings.LogoEnabled {
		t.Fatalf("logo setting was not cleared after deleting asset: %#v", settings)
	}
	assets, err = store.ListAssets(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(assets) != 0 {
		t.Fatalf("expected asset row to be deleted, got %d", len(assets))
	}
}

func callSetSiteImage(t *testing.T, svc *Service, kind, name, data string) {
	t.Helper()
	msg, err := structpb.NewStruct(map[string]any{
		"kind": kind,
		"name": name,
		"data": data,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetSiteImage(context.Background(), connect.NewRequest(msg)); err != nil {
		t.Fatal(err)
	}
}

func callDeleteAsset(t *testing.T, svc *Service, id int64) {
	t.Helper()
	msg, err := structpb.NewStruct(map[string]any{"id": float64(id)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.DeleteAsset(context.Background(), connect.NewRequest(msg)); err != nil {
		t.Fatal(err)
	}
}

func callSetSiteImageURL(t *testing.T, svc *Service, kind, rawURL string) {
	t.Helper()
	msg, err := structpb.NewStruct(map[string]any{
		"kind": kind,
		"url":  rawURL,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetSiteImage(context.Background(), connect.NewRequest(msg)); err != nil {
		t.Fatal(err)
	}
}

func pngDataURL(t *testing.T, w, h int) string {
	t.Helper()
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(pngBytes(t, w, h))
}

func pngBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x % 255), G: uint8(y % 255), B: 180, A: 255})
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func transparentPNGDataURL(t *testing.T, w, h int) string {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			alpha := uint8(255)
			if x < w/4 && y < h/4 {
				alpha = 0
			}
			img.SetNRGBA(x, y, color.NRGBA{R: 20, G: 90, B: 180, A: alpha})
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(b.Bytes())
}

func imageSize(t *testing.T, path string) (int, int) {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		t.Fatal(err)
	}
	return cfg.Width, cfg.Height
}

func imageAlphaAt(t *testing.T, path string, x, y int) uint32 {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, alpha := img.At(x, y).RGBA()
	return alpha >> 8
}
