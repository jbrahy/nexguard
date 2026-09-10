package main

import (
	"database/sql"
	"html/template"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/jbrahy/AntiVirus/internal/web/config"
	webdb "github.com/jbrahy/AntiVirus/internal/web/db"
)

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DB_DSN")
	if dsn == "" {
		dsn = "root@tcp(127.0.0.1:3306)/avtool_web_test"
	}
	d, err := webdb.Open(dsn)
	if err != nil {
		t.Skipf("no reachable test MariaDB, skipping: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func testTemplates(t *testing.T) *template.Template {
	t.Helper()
	tmpl, err := template.ParseGlob("../../web/templates/*.html")
	if err != nil {
		t.Fatalf("parsing templates: %v", err)
	}
	return tmpl
}

func testConfig() *config.Config {
	return &config.Config{StripeSecretKey: "sk_test_x", StripeWebhookSecret: "whsec_test_x", StripePriceID: "price_x", CheckoutSuccessURL: "https://example.com/success", CheckoutCancelURL: "https://example.com/cancel"}
}

func TestHealthzReturnsOK(t *testing.T) {
	d := testDB(t)
	tmpl := testTemplates(t)
	r := newRouter(d, tmpl, testConfig())

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if rec.Body.String() != "ok" {
		t.Fatalf("body = %q, want %q", rec.Body.String(), "ok")
	}
}

func TestLandingPageServesWithoutAuth(t *testing.T) {
	dsn := os.Getenv("TEST_DB_DSN")
	if dsn == "" {
		dsn = "root@tcp(127.0.0.1:3306)/avtool_web_test"
	}
	d, err := webdb.Open(dsn)
	if err != nil {
		t.Skipf("no reachable test MariaDB, skipping: %v", err)
	}
	defer d.Close()

	tmpl, err := template.ParseGlob("../../web/templates/*.html")
	if err != nil {
		t.Fatalf("parsing templates: %v", err)
	}

	cfg := &config.Config{StripeSecretKey: "sk_test_x", StripePriceID: "price_x", CheckoutSuccessURL: "https://example.com/success", CheckoutCancelURL: "https://example.com/cancel"}
	r := newRouter(d, tmpl, cfg)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "avtool") {
		t.Fatalf("body = %q, want it to mention avtool", rec.Body.String())
	}
}

func TestStripeWebhookRouteIsRegisteredAndNotBehindAuth(t *testing.T) {
	d := testDB(t)
	tmpl := testTemplates(t)
	r := newRouter(d, tmpl, testConfig())

	// No Stripe-Signature header, and no session cookie either. If the route
	// exists and is reachable without auth, HandleWebhook's own signature
	// verification rejects this with 400. A 404 would mean the route isn't
	// registered; a 302 redirect to /login would mean it was accidentally
	// placed behind auth.RequireAuth.
	req := httptest.NewRequest(http.MethodPost, "/webhooks/stripe", strings.NewReader("{}"))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (route registered, not behind auth, signature rejected)", rec.Code)
	}
}

func TestServeGracefulShutsDownOnSignal(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: mux}
	// Replace ListenAndServe path by setting Addr and using a custom serve via Serve in goroutine —
	// serveGraceful calls ListenAndServe, so bind Addr to the free port.
	srv.Addr = ln.Addr().String()
	_ = ln.Close() // free the port for ListenAndServe

	stop := make(chan os.Signal, 1)
	done := make(chan error, 1)
	go func() { done <- serveGraceful(srv, stop, time.Second) }()

	deadline := time.Now().Add(2 * time.Second)
	for {
		res, err := http.Get("http://" + srv.Addr + "/")
		if err == nil {
			res.Body.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("server not ready: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	stop <- syscall.SIGTERM
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("shutdown timed out")
	}
}

func TestCrawlerRoutesAreRegistered(t *testing.T) {
	r := newRouter(nil, nil, testConfig())
	for _, path := range []string{"/robots.txt", "/sitemap.xml"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("GET %s status = %d, want 200", path, rec.Code)
		}
	}
}
