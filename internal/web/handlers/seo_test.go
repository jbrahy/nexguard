package handlers

import (
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRobotsTXT(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/robots.txt", nil)
	rec := httptest.NewRecorder()

	RobotsTXT()(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "text/plain; charset=utf-8" {
		t.Fatalf("Content-Type = %q, want text/plain; charset=utf-8", got)
	}
	want := "User-agent: *\nAllow: /\nDisallow: /dashboard\nDisallow: /checkout\n\nSitemap: https://nexguardhq.com/sitemap.xml\n"
	if got := rec.Body.String(); got != want {
		t.Fatalf("body = %q, want %q", got, want)
	}
}

func TestSitemapXMLListsStaticPagesAndEveryArticle(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/sitemap.xml", nil)
	rec := httptest.NewRecorder()

	SitemapXML()(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/xml; charset=utf-8" {
		t.Fatalf("Content-Type = %q, want application/xml; charset=utf-8", got)
	}

	var doc sitemapURLSet
	if err := xml.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatalf("sitemap is not valid XML: %v", err)
	}
	if doc.XMLName.Local != "urlset" || doc.XMLName.Space != "http://www.sitemaps.org/schemas/sitemap/0.9" {
		t.Fatalf("root = {%q %q}, want sitemap urlset namespace", doc.XMLName.Space, doc.XMLName.Local)
	}

	want := make(map[string]bool, len(sitemapStaticPaths)+len(Articles))
	for _, path := range sitemapStaticPaths {
		want[siteURL(path)] = true
	}
	for _, article := range Articles {
		want[siteURL("/articles/"+article.Slug)] = true
	}
	if len(doc.URLs) != len(want) {
		t.Fatalf("sitemap has %d URLs, want %d", len(doc.URLs), len(want))
	}
	for _, u := range doc.URLs {
		if !want[u.Location] {
			t.Errorf("unexpected or duplicate sitemap URL %q", u.Location)
		}
		delete(want, u.Location)
	}
	for missing := range want {
		t.Errorf("sitemap missing %q", missing)
	}
	for _, privatePath := range []string{"/dashboard", "/checkout"} {
		if strings.Contains(rec.Body.String(), siteURL(privatePath)) {
			t.Errorf("sitemap includes private path %q", privatePath)
		}
	}
}

func TestSitemapXMLDerivesNewArticleFromArticles(t *testing.T) {
	original := Articles
	Articles = append(append([]Article(nil), Articles...), Article{Slug: "sitemap-source-sentinel"})
	t.Cleanup(func() { Articles = original })

	req := httptest.NewRequest(http.MethodGet, "/sitemap.xml", nil)
	rec := httptest.NewRecorder()
	SitemapXML()(rec, req)

	if !strings.Contains(rec.Body.String(), "https://nexguardhq.com/articles/sitemap-source-sentinel") {
		t.Fatal("sitemap did not derive the appended article from Articles")
	}
}
