package main

import (
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
