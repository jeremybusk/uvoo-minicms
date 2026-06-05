package netguard

import (
	"net/url"
	"testing"
)

func TestValidateURLRejectsLocalAddresses(t *testing.T) {
	for _, raw := range []string{
		"http://127.0.0.1/image.png",
		"http://[::1]/image.png",
		"http://10.0.0.5/image.png",
		"http://169.254.169.254/latest/meta-data/",
	} {
		u, err := url.Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		if err := ValidateURL(u); err == nil {
			t.Fatalf("expected %s to be rejected", raw)
		}
	}
}

func TestValidateURLAcceptsPublicHTTPURL(t *testing.T) {
	u, err := url.Parse("https://example.com/image.png")
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateURL(u); err != nil {
		t.Fatalf("expected public URL to be accepted: %v", err)
	}
}
