package acl

import (
	"net"
	"net/http"

	"uvoo-minicms/internal/auth"
	"uvoo-minicms/internal/db"
)

type Filter struct {
	Store      *db.Store
	Scope      string
	TrustProxy bool
}

func (f Filter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if f.Store == nil {
			next.ServeHTTP(w, r)
			return
		}
		ip := auth.ClientIP(r, f.TrustProxy)
		if ip == nil || !f.allowed(r, ip) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (f Filter) allowed(r *http.Request, ip net.IP) bool {
	settings, rules, err := f.Store.GetACL(r.Context())
	if err != nil {
		return false
	}
	defaultAllow := settings.PublicDefault != "deny"
	if f.Scope == "admin" {
		defaultAllow = settings.AdminDefault != "deny"
	}
	allowed := defaultAllow
	for _, rule := range rules {
		if !rule.Enabled || (rule.Scope != "all" && rule.Scope != f.Scope) {
			continue
		}
		_, network, err := net.ParseCIDR(rule.CIDR)
		if err != nil || !network.Contains(ip) {
			continue
		}
		if rule.Action == "deny" {
			return false
		}
		allowed = true
	}
	return allowed
}
