package httpreq

import (
	"net"
	"net/http"
	"strconv"
	"strings"
)

func BaseURL(r *http.Request, trustProxy bool) string {
	return Scheme(r, trustProxy) + "://" + Host(r, trustProxy)
}

func Scheme(r *http.Request, trustProxy bool) string {
	if trustProxy {
		switch scheme := strings.ToLower(FirstHeaderValue(r.Header.Get("X-Forwarded-Proto"))); scheme {
		case "http", "https":
			return scheme
		}
	}
	if r.TLS != nil {
		return "https"
	}
	return "http"
}

func Host(r *http.Request, trustProxy bool) string {
	if trustProxy {
		if host := CleanHost(FirstHeaderValue(r.Header.Get("X-Forwarded-Host"))); host != "" {
			return host
		}
	}
	return CleanHost(r.Host)
}

func IsHTTPS(r *http.Request, trustProxy bool) bool {
	return Scheme(r, trustProxy) == "https"
}

func FirstHeaderValue(raw string) string {
	if raw == "" {
		return ""
	}
	value := strings.TrimSpace(strings.Split(raw, ",")[0])
	if strings.ContainsAny(value, "\r\n") {
		return ""
	}
	return value
}

func CleanHost(raw string) string {
	host := strings.TrimSpace(raw)
	if host == "" || strings.ContainsAny(host, "\r\n\t /\\?#@") {
		return ""
	}
	if h, p, err := net.SplitHostPort(host); err == nil {
		if !validPort(p) {
			return ""
		}
		h = cleanHostName(h)
		if h == "" {
			return ""
		}
		return net.JoinHostPort(h, p)
	}
	if strings.HasPrefix(host, "[") || strings.HasSuffix(host, "]") {
		ip := net.ParseIP(strings.Trim(host, "[]"))
		if ip == nil {
			return ""
		}
		return "[" + strings.ToLower(ip.String()) + "]"
	}
	if strings.Count(host, ":") > 0 {
		return ""
	}
	return cleanHostName(host)
}

func cleanHostName(host string) string {
	host = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
	if host == "" || len(host) > 253 {
		return ""
	}
	if ip := net.ParseIP(host); ip != nil {
		return strings.ToLower(ip.String())
	}
	labels := strings.Split(host, ".")
	for _, label := range labels {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return ""
		}
		for _, r := range label {
			if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
				return ""
			}
		}
	}
	return host
}

func validPort(port string) bool {
	n, err := strconv.Atoi(port)
	return err == nil && n > 0 && n <= 65535
}
