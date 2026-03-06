package scraper

import "net/http"

// safeClient returns a shallow copy of client with the given CheckRedirect
// policy applied. The copy shares Transport, Timeout, and Jar with the
// original but has its own CheckRedirect field, so the caller's client is
// never mutated.
//
// If client is nil, a new zero-value http.Client is used as the base (zero
// Transport/Timeout/Jar), ensuring a valid non-nil client is always returned.
func safeClient(client *http.Client, redirectPolicy func(*http.Request, []*http.Request) error) *http.Client {
	if client == nil {
		return &http.Client{CheckRedirect: redirectPolicy}
	}
	return &http.Client{
		Transport:     client.Transport,
		Timeout:       client.Timeout,
		Jar:           client.Jar,
		CheckRedirect: redirectPolicy,
	}
}

// blockRedirects is a CheckRedirect policy that prevents all HTTP redirects.
// It is used by the JSON-LD and robots.txt fetch paths to mitigate SSRF via
// redirect chains to internal/private addresses.
func blockRedirects(_ *http.Request, _ []*http.Request) error {
	return http.ErrUseLastResponse
}

// limitRedirects returns a CheckRedirect policy that allows up to max redirects
// and blocks any beyond that. Used by the REST scraper, where legitimate
// canonical-URL redirects (e.g. Showpass 301) must be followed.
func limitRedirects(max int) func(*http.Request, []*http.Request) error {
	return func(_ *http.Request, via []*http.Request) error {
		if len(via) >= max {
			return http.ErrUseLastResponse
		}
		return nil
	}
}
