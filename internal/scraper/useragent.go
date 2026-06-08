package scraper

const (
	// ScraperUserAgent is the standard User-Agent string used by all scraper tiers
	// (JSON-LD, Colly, Rod, GraphQL, REST, sitemap, inspect, and ingest client).
	ScraperUserAgent = "Togather-SEL-Scraper/0.1 (+https://togather.foundation; events@togather.foundation)"

	// ICSUserAgent uses a browser-compatible Mozilla/5.0 prefix because some CDNs
	// (e.g. Meetup.com) block Go's default User-Agent ("Go-http-client/2.0").
	// ICS fetchers make direct HTTP requests rather than using a headless browser.
	ICSUserAgent = "Mozilla/5.0 (compatible; Togather-ICS/1.0)"
)
