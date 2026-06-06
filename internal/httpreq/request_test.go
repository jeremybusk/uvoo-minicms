package httpreq

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBaseURLOnlyTrustsForwardedHeadersWhenEnabled(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://internal.test/blog/feed.xml", nil)
	req.Host = "internal.test"
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Host", "public.example")

	if got := BaseURL(req, false); got != "http://internal.test" {
		t.Fatalf("expected untrusted forwarded headers to be ignored, got %q", got)
	}
	if got := BaseURL(req, true); got != "https://public.example" {
		t.Fatalf("expected trusted forwarded headers to be used, got %q", got)
	}
}

func TestBaseURLIgnoresMalformedForwardedHost(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://internal.test/blog/feed.xml", nil)
	req.Host = "internal.test"
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Host", "public.example/path")

	if got := BaseURL(req, true); got != "https://internal.test" {
		t.Fatalf("expected malformed forwarded host to be ignored, got %q", got)
	}
}

func TestCleanHost(t *testing.T) {
	tests := map[string]string{
		"Example.COM":       "example.com",
		"example.com:8443":  "example.com:8443",
		"[2001:db8::1]:443": "[2001:db8::1]:443",
		"bad host":          "",
		"example.com/path":  "",
		"example.com:bad":   "",
	}
	for input, want := range tests {
		if got := CleanHost(input); got != want {
			t.Fatalf("CleanHost(%q)=%q, want %q", input, got, want)
		}
	}
}
