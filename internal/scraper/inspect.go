package scraper

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// InspectResult summarises the DOM structure of a page to help identify
// CSS selectors for Tier 1 scraping.
type InspectResult struct {
	URL         string
	StatusCode  int
	BodyBytes   int
	TopClasses  []ClassCount // most frequent CSS classes
	DataAttrs   []ClassCount // most frequent data-* attribute names
	EventLinks  []string     // href values containing "event" or "program"
	SampleCards []SampleCard // first few likely event containers
}

// ClassCount is a CSS class (or data-attr) name and how often it appeared.
type ClassCount struct {
	Name  string
	Count int
}

// SampleCard is a snippet of a candidate event container element.
type SampleCard struct {
	Selector string // e.g. "article.event-card"
	HTML     string // trimmed outer HTML (first 300 chars)
}

// Inspect fetches url and analyses its DOM, returning a structured summary
// useful for discovering CSS selectors. It uses the standard SEL User-Agent.
func Inspect(ctx context.Context, rawURL string) (*InspectResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("inspect: build request: %w", err)
	}
	req.Header.Set("User-Agent", "Togather-SEL-Scraper/0.1 (+https://togather.foundation; events@togather.foundation)")

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("inspect: fetch: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("inspect: read body: %w", err)
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("inspect: parse HTML: %w", err)
	}

	result := &InspectResult{
		URL:        rawURL,
		StatusCode: resp.StatusCode,
		BodyBytes:  len(body),
	}

	// --- CSS class frequency ---
	classCounts := map[string]int{}
	doc.Find("[class]").Each(func(_ int, s *goquery.Selection) {
		cls, _ := s.Attr("class")
		for _, part := range strings.Fields(cls) {
			classCounts[part]++
		}
	})
	result.TopClasses = topN(classCounts, 30)

	// --- data-* attribute frequency ---
	dataCounts := map[string]int{}
	doc.Find("*").Each(func(_ int, s *goquery.Selection) {
		for _, node := range s.Nodes {
			for _, attr := range node.Attr {
				if strings.HasPrefix(attr.Key, "data-") {
					dataCounts[attr.Key]++
				}
			}
		}
	})
	result.DataAttrs = topN(dataCounts, 15)

	// --- event/program href links ---
	seen := map[string]bool{}
	doc.Find("a[href]").Each(func(_ int, s *goquery.Selection) {
		href, _ := s.Attr("href")
		lower := strings.ToLower(href)
		if (strings.Contains(lower, "/event") || strings.Contains(lower, "/program")) && !seen[href] {
			seen[href] = true
			result.EventLinks = append(result.EventLinks, href)
		}
	})
	if len(result.EventLinks) > 20 {
		result.EventLinks = result.EventLinks[:20]
	}

	// --- candidate event container elements ---
	eventWords := []string{"event", "film", "show", "program", "card", "item", "listing", "performance"}
	cardSeen := map[string]bool{}
	for _, tag := range []string{"article", "li", "div", "section"} {
		doc.Find(tag + "[class]").Each(func(_ int, s *goquery.Selection) {
			cls, _ := s.Attr("class")
			lower := strings.ToLower(cls)
			for _, w := range eventWords {
				if strings.Contains(lower, w) {
					// Build a short selector
					firstClass := strings.Fields(cls)[0]
					sel := tag + "." + firstClass
					if cardSeen[sel] {
						return
					}
					cardSeen[sel] = true
					h, _ := goquery.OuterHtml(s)
					if len(h) > 300 {
						h = h[:300] + "…"
					}
					result.SampleCards = append(result.SampleCards, SampleCard{
						Selector: sel,
						HTML:     h,
					})
					if len(result.SampleCards) >= 8 {
						return
					}
					break
				}
			}
		})
		if len(result.SampleCards) >= 8 {
			break
		}
	}

	return result, nil
}

// FormatInspectResult formats an InspectResult as human-readable terminal output.
func FormatInspectResult(r *InspectResult) string {
	var b strings.Builder

	fmt.Fprintf(&b, "URL:    %s\n", r.URL)
	fmt.Fprintf(&b, "Status: %d\n", r.StatusCode)
	fmt.Fprintf(&b, "Size:   %d bytes\n\n", r.BodyBytes)

	b.WriteString("── Top CSS Classes ─────────────────────────────────────\n")
	for i, c := range r.TopClasses {
		fmt.Fprintf(&b, "  %-40s %d\n", c.Name, c.Count)
		if i >= 19 {
			break
		}
	}

	if len(r.DataAttrs) > 0 {
		b.WriteString("\n── data-* Attributes ────────────────────────────────────\n")
		for _, c := range r.DataAttrs {
			fmt.Fprintf(&b, "  %-40s %d\n", c.Name, c.Count)
		}
	}

	if len(r.EventLinks) > 0 {
		b.WriteString("\n── Event/Program hrefs (sample) ─────────────────────────\n")
		for _, l := range r.EventLinks {
			fmt.Fprintf(&b, "  %s\n", l)
		}
	}

	if len(r.SampleCards) > 0 {
		b.WriteString("\n── Candidate Event Containers ───────────────────────────\n")
		for _, card := range r.SampleCards {
			fmt.Fprintf(&b, "\n  selector: %s\n  html:     %s\n", card.Selector, card.HTML)
		}
	}

	return b.String()
}

// topN returns the N most frequent entries from a count map, sorted desc.
func topN(m map[string]int, n int) []ClassCount {
	out := make([]ClassCount, 0, len(m))
	for k, v := range m {
		out = append(out, ClassCount{Name: k, Count: v})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Name < out[j].Name
	})
	if len(out) > n {
		out = out[:n]
	}
	return out
}
