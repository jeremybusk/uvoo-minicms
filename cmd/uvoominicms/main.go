package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"uvoominicms/cmsv1connect"
	"uvoominicms/internal/acl"
	"uvoominicms/internal/auth"
	"uvoominicms/internal/config"
	"uvoominicms/internal/db"
	"uvoominicms/internal/geo"
	"uvoominicms/internal/service"
	"uvoominicms/internal/web"
)

func main() {
	cfg := config.Load()
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
	adminACL := acl.Filter{Store: store, Scope: "admin", TrustProxy: cfg.TrustProxyHeaders}
	publicACL := acl.Filter{Store: store, Scope: "public", TrustProxy: cfg.TrustProxyHeaders}
	adminGeo := geof.WithStore(store, "admin")
	publicGeo := geof.WithStore(store, "public")

	mux := http.NewServeMux()
	mux.Handle("/cms.v1.CMSService/", chain(api, ipf.Middleware, adminACL.Middleware, adminGeo.Middleware, auth.Basic{User: cfg.AdminUser, Pass: cfg.AdminPass}.Middleware))
	mux.Handle("/uploads/", chain(uploads, ipf.Middleware, publicACL.Middleware, publicGeo.Middleware))
	mux.Handle("/admin/", chain(http.StripPrefix("/admin/", admin), ipf.Middleware, adminACL.Middleware, adminGeo.Middleware, auth.Basic{User: cfg.AdminUser, Pass: cfg.AdminPass}.Middleware))
	mux.Handle("/", chain(pub, ipf.Middleware, publicACL.Middleware, publicGeo.Middleware))

	tlsEnabled := cfg.TLSCertFile != "" && cfg.TLSKeyFile != ""
	if (cfg.TLSCertFile == "") != (cfg.TLSKeyFile == "") {
		log.Fatal("both TLS cert and key must be provided")
	}
	srv := &http.Server{Addr: cfg.Addr, Handler: secureHeaders(mux), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 4 * time.Minute, IdleTimeout: 120 * time.Second, MaxHeaderBytes: 1 << 20}
	log.Printf("uvoominicms listening on %s db=%s uploads=%s web-root=%s tls=%t", cfg.Addr, cfg.DBPath, filepath.Clean(cfg.UploadDir), filepath.Clean(cfg.WebRoot), tlsEnabled)
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
func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
