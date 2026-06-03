package auth

import (
	"crypto/subtle"
	"net"
	"net/http"
	"strings"
)

type Basic struct{ User, Pass string }

func (b Basic) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, p, ok := r.BasicAuth()
		if !ok || subtle.ConstantTimeCompare([]byte(u), []byte(b.User)) != 1 || subtle.ConstantTimeCompare([]byte(p), []byte(b.Pass)) != 1 {
			w.Header().Set("WWW-Authenticate", `Basic realm="uvoo-minicms", charset="UTF-8"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func ClientIP(r *http.Request, trustProxy bool) net.IP {
	if trustProxy {
		for _, h := range []string{"CF-Connecting-IP", "X-Real-IP", "X-Forwarded-For"} {
			if v := strings.TrimSpace(r.Header.Get(h)); v != "" {
				if h == "X-Forwarded-For" {
					v = strings.TrimSpace(strings.Split(v, ",")[0])
				}
				if ip := net.ParseIP(v); ip != nil {
					return ip
				}
			}
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	return net.ParseIP(host)
}

type IPFilter struct {
	Allow, Deny []*net.IPNet
	TrustProxy  bool
}

func NewIPFilter(allow, deny []string, trustProxy bool) (IPFilter, error) {
	parse := func(items []string) ([]*net.IPNet, error) {
		var out []*net.IPNet
		for _, s := range items {
			_, n, err := net.ParseCIDR(s)
			if err != nil {
				return nil, err
			}
			out = append(out, n)
		}
		return out, nil
	}
	a, err := parse(allow)
	if err != nil {
		return IPFilter{}, err
	}
	d, err := parse(deny)
	if err != nil {
		return IPFilter{}, err
	}
	return IPFilter{Allow: a, Deny: d, TrustProxy: trustProxy}, nil
}

func (f IPFilter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := ClientIP(r, f.TrustProxy)
		if ip == nil || contains(f.Deny, ip) || (len(f.Allow) > 0 && !contains(f.Allow, ip)) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}
func contains(list []*net.IPNet, ip net.IP) bool {
	for _, n := range list {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}
