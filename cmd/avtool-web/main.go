package main

import (
	"database/sql"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jbrahy/AntiVirus/internal/web/auth"
	"github.com/jbrahy/AntiVirus/internal/web/billing"
	"github.com/jbrahy/AntiVirus/internal/web/config"
	webdb "github.com/jbrahy/AntiVirus/internal/web/db"
	"github.com/jbrahy/AntiVirus/internal/web/handlers"
	"github.com/jbrahy/AntiVirus/internal/web/ratelimit"
)

func newRouter(db *sql.DB, tmpl *template.Template, cfg *config.Config) *chi.Mux {
	r := chi.NewRouter()

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.Dir("web/static"))))

	r.Group(func(r chi.Router) {
		r.Use(auth.OptionalAuth(db))
		r.Get("/", handlers.Landing(db, tmpl))
	})
	r.Get("/about", handlers.StaticPage(tmpl, "about.html"))
	r.Get("/contact", handlers.StaticPage(tmpl, "contact.html"))
	r.Get("/privacy", handlers.StaticPage(tmpl, "privacy.html"))
	r.Get("/terms", handlers.StaticPage(tmpl, "terms.html"))
	r.Get("/articles", handlers.ArticlesIndex(tmpl))
	r.Get("/articles/{slug}", handlers.ArticleShow(tmpl))
	r.Get("/alternatives", handlers.Alternatives(tmpl))
	r.Get("/robots.txt", handlers.RobotsTXT())
	r.Get("/sitemap.xml", handlers.SitemapXML())

	// Login attempts and license validation are both credential-guessing
	// surfaces (a password, a license key) and both get a per-IP rate limit
	// for it. Separate limiters: license validation is a machine client
	// calling periodically, not a human filling out a form, so it gets a
	// higher ceiling.
	loginLimiter := ratelimit.New(5, 15*time.Minute)
	licenseLimiter := ratelimit.New(30, time.Minute)

	r.Get("/signup", handlers.SignupPage(db, tmpl))
	r.Post("/signup", handlers.SignupPage(db, tmpl))
	r.Get("/login", handlers.LoginPage(db, tmpl))
	r.With(ratelimit.Middleware(loginLimiter)).Post("/login", handlers.LoginPage(db, tmpl))

	r.Group(func(r chi.Router) {
		r.Use(auth.RequireAuth(db))
		r.Get("/dashboard", handlers.Dashboard(db, tmpl))
		r.Get("/checkout", handlers.CheckoutRedirect(db, cfg.StripeSecretKey, cfg.StripePriceID, cfg.CheckoutSuccessURL, cfg.CheckoutCancelURL))
		r.Post("/logout", handlers.LogoutHandler(db))
	})

	r.With(ratelimit.Middleware(licenseLimiter)).Post("/api/v1/license/validate", handlers.ValidateLicenseAPI(db))

	// Stripe's webhook caller has no session cookie; its authentication is
	// the webhook signature verification that HandleWebhook itself performs,
	// so this must stay outside the RequireAuth group above.
	r.Post("/webhooks/stripe", billing.HandleWebhook(db, cfg.StripeWebhookSecret))

	return r
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	db, err := webdb.Open(cfg.DBDSN)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer db.Close()

	tmpl, err := template.ParseGlob("web/templates/*.html")
	if err != nil {
		log.Fatalf("templates: %v", err)
	}

	r := newRouter(db, tmpl, cfg)
	// Listen on 127.0.0.1 only, per the design spec's Deployment section:
	// this service sits behind an Apache reverse proxy that terminates TLS.
	// Binding all interfaces would let anyone who can reach this box on the
	// VPC/VPN talk to it directly over plain HTTP, bypassing TLS.
	addr := fmt.Sprintf("127.0.0.1:%s", cfg.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	log.Printf("avtool-web listening on %s", addr)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("server: %v", err)
	}
}
