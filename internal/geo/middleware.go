package geo

import (
	"net/http"
	"strings"

	"github.com/oschwald/geoip2-golang"
	"uvoo-minicms/internal/auth"
	"uvoo-minicms/internal/db"
)

type Filter struct {
	DB          *geoip2.Reader
	Allow, Deny map[string]bool
	TrustProxy  bool
	Store       *db.Store
	Scope       string
}

func New(dbPath string, allow, deny []string, trustProxy bool) (*Filter, error) {
	if dbPath == "" {
		return &Filter{TrustProxy: trustProxy}, nil
	}
	db, err := geoip2.Open(dbPath)
	if err != nil {
		return nil, err
	}
	return &Filter{DB: db, Allow: set(allow), Deny: set(deny), TrustProxy: trustProxy}, nil
}
func (f *Filter) WithStore(store *db.Store, scope string) *Filter {
	if f == nil {
		return nil
	}
	clone := *f
	clone.Store = store
	clone.Scope = scope
	return &clone
}
func set(xs []string) map[string]bool {
	m := map[string]bool{}
	for _, x := range xs {
		m[strings.ToUpper(strings.TrimSpace(x))] = true
	}
	return m
}

func (f *Filter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if f == nil || f.DB == nil {
			next.ServeHTTP(w, r)
			return
		}
		ip := auth.ClientIP(r, f.TrustProxy)
		rec, err := f.DB.Country(ip)
		if err != nil {
			http.Error(w, "geo denied", http.StatusForbidden)
			return
		}
		cc := strings.ToUpper(rec.Country.IsoCode)
		allow, deny := f.countryRules(r)
		if deny[cc] || (len(allow) > 0 && !allow[cc]) {
			http.Error(w, "geo denied", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}
func (f *Filter) countryRules(r *http.Request) (map[string]bool, map[string]bool) {
	allow := copySet(f.Allow)
	deny := copySet(f.Deny)
	if f.Store == nil {
		return allow, deny
	}
	settings, _, err := f.Store.GetACL(r.Context())
	if err != nil {
		return allow, deny
	}
	if f.Scope == "admin" {
		mergeCountries(allow, settings.AdminAllowCountries)
		mergeCountries(deny, settings.AdminDenyCountries)
	} else {
		mergeCountries(allow, settings.PublicAllowCountries)
		mergeCountries(deny, settings.PublicDenyCountries)
	}
	return allow, deny
}
func copySet(in map[string]bool) map[string]bool {
	out := map[string]bool{}
	for key, value := range in {
		out[key] = value
	}
	return out
}
func mergeCountries(m map[string]bool, raw string) {
	for _, part := range strings.Split(raw, ",") {
		if code := strings.ToUpper(strings.TrimSpace(part)); len(code) == 2 {
			m[code] = true
		}
	}
}
func (f *Filter) Close() error {
	if f != nil && f.DB != nil {
		return f.DB.Close()
	}
	return nil
}
