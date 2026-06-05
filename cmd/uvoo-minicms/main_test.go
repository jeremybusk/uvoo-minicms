package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"uvoo-minicms/internal/config"
)

func TestValidateRuntimeSecurityRejectsDefaultPasswordOnExposedBind(t *testing.T) {
	err := validateRuntimeSecurity(config.Config{Addr: ":8080", AdminPass: "change-me"})
	if err == nil {
		t.Fatal("expected exposed default password to be rejected")
	}
}

func TestValidateRuntimeSecurityAllowsDefaultPasswordOnLoopback(t *testing.T) {
	err := validateRuntimeSecurity(config.Config{Addr: "127.0.0.1:8080", AdminPass: "change-me"})
	if err != nil {
		t.Fatalf("expected loopback default password to be allowed: %v", err)
	}
}

func TestValidateRuntimeSecurityAllowsStrongPasswordOnExposedBind(t *testing.T) {
	err := validateRuntimeSecurity(config.Config{Addr: ":8080", AdminPass: "not-the-default"})
	if err != nil {
		t.Fatalf("expected non-default password to be allowed: %v", err)
	}
}

func TestSameOriginRejectsCrossOriginPost(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "http://cms.example/cms.v1.CMSService/SavePage", nil)
	req.Host = "cms.example"
	req.Header.Set("Origin", "https://evil.example")
	if sameOriginRequest(req, false) {
		t.Fatal("expected cross-origin POST to be rejected")
	}
}

func TestSameOriginAllowsMatchingOriginPost(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "http://cms.example/cms.v1.CMSService/SavePage", nil)
	req.Host = "cms.example"
	req.Header.Set("Origin", "http://cms.example")
	if !sameOriginRequest(req, false) {
		t.Fatal("expected matching origin POST to be allowed")
	}
}

func TestSameOriginUsesForwardedHeadersOnlyWhenTrusted(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "http://internal/cms.v1.CMSService/SavePage", nil)
	req.Host = "internal"
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Host", "cms.example")
	req.Header.Set("Origin", "https://cms.example")
	if sameOriginRequest(req, false) {
		t.Fatal("expected untrusted forwarded origin to be rejected")
	}
	if !sameOriginRequest(req, true) {
		t.Fatal("expected trusted forwarded origin to be allowed")
	}
}

func TestSameOriginMiddlewareBehindTrustedProxy(t *testing.T) {
	handler := sameOrigin(true)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodPost, "http://backend/cms.v1.CMSService/GetSettings", nil)
	req.Host = "backend"
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Host", "cms.example")
	req.Header.Set("Origin", "https://cms.example")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected trusted proxy same-origin request to pass, got %d", rec.Code)
	}
}

func TestSameOriginMiddlewareRejectsUntrustedForwardedOrigin(t *testing.T) {
	handler := sameOrigin(false)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodPost, "http://backend/cms.v1.CMSService/GetSettings", nil)
	req.Host = "backend"
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Host", "cms.example")
	req.Header.Set("Origin", "https://cms.example")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected untrusted forwarded origin to be rejected, got %d", rec.Code)
	}
}

func TestSecureHeadersAddsConservativeCSP(t *testing.T) {
	handler := secureHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	csp := rec.Header().Get("Content-Security-Policy")
	for _, want := range []string{
		"default-src 'self'",
		"object-src 'none'",
		"frame-ancestors 'none'",
		"https://cdn.jsdelivr.net",
		"https://www.youtube-nocookie.com",
	} {
		if !strings.Contains(csp, want) {
			t.Fatalf("expected CSP to contain %q, got %q", want, csp)
		}
	}
}
