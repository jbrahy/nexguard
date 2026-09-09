package handlers

import (
	"encoding/xml"
	"fmt"
	"net/http"
	"strings"

	"github.com/jbrahy/AntiVirus/internal/web/config"
)

var sitemapStaticPaths = []string{
	"/",
	"/about",
	"/contact",
	"/privacy",
	"/terms",
	"/articles",
	"/alternatives",
}

type sitemapURLSet struct {
	XMLName xml.Name     `xml:"urlset"`
	XMLNS   string       `xml:"xmlns,attr"`
	URLs    []sitemapURL `xml:"url"`
}

type sitemapURL struct {
	Location string `xml:"loc"`
}

func siteURL(path string) string {
	return "https://" + strings.TrimSuffix(config.Site.Domain, "/") + path
}

// RobotsTXT publishes crawler rules for the public site while keeping the
// authenticated account and checkout surfaces out of search crawls.
func RobotsTXT() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprintf(w, "User-agent: *\nAllow: /\nDisallow: /dashboard\nDisallow: /checkout\n\nSitemap: %s\n", siteURL("/sitemap.xml"))
	}
}

// SitemapXML publishes every indexable static page and derives article URLs
// directly from Articles so the sitemap cannot drift from the article router.
func SitemapXML() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		urls := make([]sitemapURL, 0, len(sitemapStaticPaths)+len(Articles))
		for _, path := range sitemapStaticPaths {
			urls = append(urls, sitemapURL{Location: siteURL(path)})
		}
		for _, article := range Articles {
			urls = append(urls, sitemapURL{Location: siteURL("/articles/" + article.Slug)})
		}

		body, err := xml.MarshalIndent(sitemapURLSet{
			XMLNS: "http://www.sitemaps.org/schemas/sitemap/0.9",
			URLs:  urls,
		}, "", "  ")
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/xml; charset=utf-8")
		w.Write([]byte(xml.Header))
		w.Write(body)
		w.Write([]byte("\n"))
	}
}
