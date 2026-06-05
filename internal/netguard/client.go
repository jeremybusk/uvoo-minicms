package netguard

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"
)

const maxRedirects = 5

func NewHTTPClient(timeout time.Duration) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	dialer := &net.Dialer{Timeout: timeout, KeepAlive: 30 * time.Second}
	transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}
		ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil {
			return nil, err
		}
		for _, item := range ips {
			if err := validateIP(item.IP); err != nil {
				return nil, err
			}
		}
		for _, item := range ips {
			conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(item.IP.String(), port))
			if err == nil {
				return conn, nil
			}
		}
		return nil, fmt.Errorf("no reachable address for %s", host)
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: validatingTransport{next: transport},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return errors.New("too many redirects")
			}
			return ValidateURL(req.URL)
		},
	}
}

type validatingTransport struct {
	next http.RoundTripper
}

func (t validatingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := ValidateURL(req.URL); err != nil {
		return nil, err
	}
	return t.next.RoundTrip(req)
}

func ValidateURL(u *url.URL) error {
	if u == nil {
		return errors.New("URL required")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return errors.New("only http and https URLs are supported")
	}
	if u.Host == "" {
		return errors.New("host required")
	}
	if u.User != nil {
		return errors.New("URLs with embedded credentials are not allowed")
	}
	if ip := net.ParseIP(u.Hostname()); ip != nil {
		return validateIP(ip)
	}
	return nil
}

func validateIP(ip net.IP) error {
	if ip == nil {
		return errors.New("invalid IP address")
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() {
		return fmt.Errorf("private or local address %s is not allowed", ip.String())
	}
	return nil
}
