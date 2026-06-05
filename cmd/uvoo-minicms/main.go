package main

import (
	"errors"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"uvoo-minicms/cmsv1connect"
	"uvoo-minicms/internal/acl"
	"uvoo-minicms/internal/auth"
	"uvoo-minicms/internal/config"
	"uvoo-minicms/internal/db"
	"uvoo-minicms/internal/geo"
	"uvoo-minicms/internal/service"
	"uvoo-minicms/internal/web"
)

func main() {
	cfg := config.Load()
	must(validateRuntimeSecurity(cfg))
	must(os.MkdirAll(cfg.UploadDir, 0750))
	store, err := db.Open(cfg.DBPath)
	must(err)
	ipf, err := auth.NewIPFilter(cfg.AllowedCIDRs, cfg.DeniedCIDRs, cfg.TrustProxyHeaders)
	must(err)
	geof, err := geo.New(cfg.MaxMindDBPath, cfg.AllowedCountries, cfg.DeniedCountries, cfg.TrustProxyHeaders)
	must(err)
	defer geof.Close()

	svc := &service.Service{Store: store, UploadDir: cfg.UploadDir, MaxUploadBytes: cfg.MaxUploadBytes, SiteName: cfg.PublicSiteName}
	_, api := cmsv1connect.NewCMSServiceHandler(svc)
	admin := http.FileServer(http.Dir(cfg.WebRoot))
	uploads := http.StripPrefix("/uploads/", http.FileServer(http.Dir(cfg.UploadDir)))
	pub := web.NewPublic(store, cfg.PublicSiteName)
	pub.TrustProxy = cfg.TrustProxyHeaders
	adminACL := acl.Filter{Store: store, Scope: "admin", TrustProxy: cfg.TrustProxyHeaders}
	publicACL := acl.Filter{Store: store, Scope: "public", TrustProxy: cfg.TrustProxyHeaders}
	adminGeo := geof.WithStore(store, "admin")
	publicGeo := geof.WithStore(store, "public")

	mux := http.NewServeMux()
	mux.Handle("/cms.v1.CMSService/", chain(api, ipf.Middleware, adminACL.Middleware, adminGeo.Middleware, sameOrigin(cfg.TrustProxyHeaders), auth.Basic{User: cfg.AdminUser, Pass: cfg.AdminPass}.Middleware))
	mux.Handle("/uploads/", chain(uploads, ipf.Middleware, publicACL.Middleware, publicGeo.Middleware, cacheUploads))
	mux.Handle("/admin/", chain(http.StripPrefix("/admin/", admin), ipf.Middleware, adminACL.Middleware, adminGeo.Middleware, auth.Basic{User: cfg.AdminUser, Pass: cfg.AdminPass}.Middleware))
	mux.Handle("/", chain(pub, ipf.Middleware, publicACL.Middleware, publicGeo.Middleware))

	tlsEnabled := cfg.TLSCertFile != "" && cfg.TLSKeyFile != ""
	if (cfg.TLSCertFile == "") != (cfg.TLSKeyFile == "") {
		log.Fatal("both TLS cert and key must be provided")
	}
	srv := &http.Server{Addr: cfg.Addr, Handler: secureHeaders(mux), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 4 * time.Minute, IdleTimeout: 120 * time.Second, MaxHeaderBytes: 1 << 20}
	log.Printf("uvoo-minicms listening on %s db=%s uploads=%s web-root=%s tls=%t", cfg.Addr, cfg.DBPath, filepath.Clean(cfg.UploadDir), filepath.Clean(cfg.WebRoot), tlsEnabled)
	if tlsEnabled {
		log.Fatal(srv.ListenAndServeTLS(cfg.TLSCertFile, cfg.TLSKeyFile))
	}
	log.Fatal(srv.ListenAndServe())
}

type mw func(http.Handler) http.Handler

func chain(h http.Handler, mws ...mw) http.Handler {
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h
}
func secureHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("X-Frame-Options", "DENY")
		next.ServeHTTP(w, r)
	})
}

func cacheUploads(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		next.ServeHTTP(w, r)
	})
}

func sameOrigin(trustProxy bool) mw {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
				next.ServeHTTP(w, r)
				return
			}
			if !sameOriginRequest(r, trustProxy) {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func sameOriginRequest(r *http.Request, trustProxy bool) bool {
	expected := requestBaseURL(r, trustProxy)
	for _, raw := range []string{r.Header.Get("Origin"), r.Header.Get("Referer")} {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		u, err := url.Parse(raw)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return false
		}
		if strings.EqualFold(u.Scheme+"://"+u.Host, expected) {
			return true
		}
		return false
	}
	return true
}

func requestBaseURL(r *http.Request, trustProxy bool) string {
	scheme := ""
	if trustProxy {
		scheme = strings.ToLower(firstHeaderValue(r.Header.Get("X-Forwarded-Proto")))
		if scheme != "http" && scheme != "https" {
			scheme = ""
		}
	}
	if scheme == "" && r.TLS != nil {
		scheme = "https"
	}
	if scheme == "" {
		scheme = "http"
	}
	host := r.Host
	if trustProxy {
		if forwardedHost := firstHeaderValue(r.Header.Get("X-Forwarded-Host")); forwardedHost != "" {
			host = forwardedHost
		}
	}
	return scheme + "://" + host
}

func firstHeaderValue(raw string) string {
	if raw == "" {
		return ""
	}
	value := strings.TrimSpace(strings.Split(raw, ",")[0])
	if strings.ContainsAny(value, "\r\n") {
		return ""
	}
	return value
}

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func validateRuntimeSecurity(cfg config.Config) error {
	if insecureAdminPass(cfg.AdminPass) && exposedBind(cfg.Addr) {
		return errors.New("refusing to start with default or empty CMS_ADMIN_PASS on a non-loopback bind address")
	}
	return nil
}

func insecureAdminPass(pass string) bool {
	switch strings.TrimSpace(pass) {
	case "", "change-me", "change-me-now":
		return true
	default:
		return false
	}
}

func exposedBind(addr string) bool {
	host := strings.TrimSpace(addr)
	if host == "" {
		return true
	}
	if strings.HasPrefix(host, ":") {
		return true
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	switch strings.ToLower(host) {
	case "", "*", "0.0.0.0", "::":
		return true
	case "localhost":
		return false
	}
	ip := net.ParseIP(host)
	return ip == nil || !ip.IsLoopback()
}
