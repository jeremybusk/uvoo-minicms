package main

import (
	"net/http"
	"net/http/httptest"
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
