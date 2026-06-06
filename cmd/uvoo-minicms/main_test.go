package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"uvoo-minicms/internal/auth"
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

func TestSameOriginRejectsMalformedForwardedHost(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "http://internal/cms.v1.CMSService/SavePage", nil)
	req.Host = "internal"
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Host", "cms.example/path")
	req.Header.Set("Origin", "https://cms.example")
	if sameOriginRequest(req, true) {
		t.Fatal("expected malformed forwarded host to be rejected")
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

func TestAdminAPIMiddlewareBehindProxyWithRateLimit(t *testing.T) {
	limiter := auth.NewRateLimiter(1, time.Minute, true)
	handler := chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}), sameOrigin(true), limiter.Middleware)

	req := httptest.NewRequest(http.MethodPost, "http://backend/cms.v1.CMSService/GetSettings", nil)
	req.Host = "backend"
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Host", "cms.example")
	req.Header.Set("X-Forwarded-For", "203.0.113.10")
	req.Header.Set("Origin", "https://cms.example")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected first proxied same-origin API request to pass, got %d", rec.Code)
	}
	if rec.Header().Get("X-RateLimit-Remaining") != "0" {
		t.Fatalf("expected remaining limit header to be 0, got %#v", rec.Header())
	}

	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected second proxied API request to be rate limited, got %d", rec.Code)
	}
}

func TestAdminAPIMiddlewareRejectsCrossOriginBeforeRateLimit(t *testing.T) {
	limiter := auth.NewRateLimiter(1, time.Minute, true)
	handler := chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}), sameOrigin(true), limiter.Middleware)

	req := httptest.NewRequest(http.MethodPost, "http://backend/cms.v1.CMSService/GetSettings", nil)
	req.Host = "backend"
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Host", "cms.example")
	req.Header.Set("X-Forwarded-For", "203.0.113.10")
	req.Header.Set("Origin", "https://evil.example")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected cross-origin API request to be rejected, got %d", rec.Code)
	}
	if rec.Header().Get("X-RateLimit-Limit") != "" {
		t.Fatalf("expected same-origin guard to run before rate limiter, got %#v", rec.Header())
	}
}

func TestNoStoreSetsAdminCacheHeader(t *testing.T) {
	handler := noStore(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin/", nil))
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("expected no-store cache header, got %#v", rec.Header())
	}
}

func TestSecureHeadersAddsConservativeCSP(t *testing.T) {
	handler := secureHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}), "enforce", false, 0, false)
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

func TestSecureHeadersSupportsCSPReportOnlyAndOff(t *testing.T) {
	for _, tc := range []struct {
		mode       string
		wantHeader string
	}{
		{mode: "report-only", wantHeader: "Content-Security-Policy-Report-Only"},
		{mode: "off", wantHeader: ""},
	} {
		handler := secureHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}), tc.mode, false, 0, false)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
		if tc.wantHeader == "" {
			if rec.Header().Get("Content-Security-Policy") != "" || rec.Header().Get("Content-Security-Policy-Report-Only") != "" {
				t.Fatalf("mode %s: expected no CSP headers, got %#v", tc.mode, rec.Header())
			}
			continue
		}
		if rec.Header().Get(tc.wantHeader) == "" {
			t.Fatalf("mode %s: expected %s header, got %#v", tc.mode, tc.wantHeader, rec.Header())
		}
	}
}

func TestSecureHeadersDoesNotAddHSTSByDefault(t *testing.T) {
	handler := secureHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}), "enforce", false, 15552000, false)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "https://cms.example/", nil))
	if rec.Header().Get("Strict-Transport-Security") != "" {
		t.Fatalf("expected no HSTS header by default, got %#v", rec.Header())
	}
}

func TestSecureHeadersAddsHSTSOnHTTPSWhenEnabled(t *testing.T) {
	handler := secureHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}), "enforce", true, 31536000, false)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "https://cms.example/", nil))
	if got := rec.Header().Get("Strict-Transport-Security"); got != "max-age=31536000" {
		t.Fatalf("expected HSTS header, got %q", got)
	}
}

func TestSecureHeadersUsesTrustedForwardedProtoForHSTS(t *testing.T) {
	handler := secureHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}), "enforce", true, 15552000, true)
	req := httptest.NewRequest(http.MethodGet, "http://backend/", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if got := rec.Header().Get("Strict-Transport-Security"); got != "max-age=15552000" {
		t.Fatalf("expected HSTS header behind trusted HTTPS proxy, got %q", got)
	}
}

func TestSecureHeadersIgnoresUntrustedForwardedProtoForHSTS(t *testing.T) {
	handler := secureHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}), "enforce", true, 15552000, false)
	req := httptest.NewRequest(http.MethodGet, "http://backend/", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Header().Get("Strict-Transport-Security") != "" {
		t.Fatalf("expected no HSTS header for untrusted forwarded HTTPS, got %#v", rec.Header())
	}
}
