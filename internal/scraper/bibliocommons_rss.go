package scraper

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Togather-Foundation/server/internal/domain/events"
	"github.com/rs/zerolog"
	"golang.org/x/net/html"
)

// BiblioCommons events RSS feed (extraction_method: "bibliocommons_rss").
//
// BiblioCommons powers most Canadian public-library event calendars (e.g. TPL).
// Its terms of service prohibit automated harvesting of the website "except as may
// be specifically permitted using RSS/XML feeds", so the RSS feed — not the HTML
// listing — is the sanctioned machine-readable surface. Unlike the HTML listing it
// also carries venue coordinates, local start/end times, registration state and
// cancellation flags.
//
// Feed URL pattern (filters are query params and are preserved):
//
//	https://gateway.bibliocommons.com/v2/libraries/<lib>/rss/events?types=<id>&programs=<id>&q=<text>&page=N
//
// The feed is paginated with ?page=N (25 items per page); an empty page ends the list.

// The bc: extension elements live in the namespace
// "http://bibliocommons.com/rss/1.0/modules/event/" (spelled out in the struct tags below,
// which cannot reference a constant).

// defaultBCPageDelay is the pause between page requests to be polite to the gateway.
const defaultBCPageDelay = 500 * time.Millisecond

// defaultBCMaxBodyBytes caps a single feed page (real pages are ~75 KB).
const defaultBCMaxBodyBytes int64 = 5 * 1024 * 1024

// bcPrivacyNoticeMarker starts TPL's per-event privacy boilerplate, which carries no
// event information and would otherwise dominate short descriptions.
const bcPrivacyNoticeMarker = "Information Collection Notice"

type bcFeed struct {
	XMLName xml.Name `xml:"rss"`
	Channel struct {
		Title string   `xml:"title"`
		Items []bcItem `xml:"item"`
	} `xml:"channel"`
}

type bcItem struct {
	Title       string   `xml:"title"`
	Description string   `xml:"description"`
	Link        string   `xml:"link"`
	GUID        string   `xml:"guid"`
	Categories  []string `xml:"category"`
	Enclosure   struct {
		URL string `xml:"url,attr"`
	} `xml:"enclosure"`
	StartUTC     string         `xml:"http://bibliocommons.com/rss/1.0/modules/event/ start_date"`
	StartLocal   string         `xml:"http://bibliocommons.com/rss/1.0/modules/event/ start_date_local"`
	EndUTC       string         `xml:"http://bibliocommons.com/rss/1.0/modules/event/ end_date"`
	EndLocal     string         `xml:"http://bibliocommons.com/rss/1.0/modules/event/ end_date_local"`
	IsCancelled  string         `xml:"http://bibliocommons.com/rss/1.0/modules/event/ is_cancelled"`
	IsVirtual    string         `xml:"http://bibliocommons.com/rss/1.0/modules/event/ is_virtual"`
	Location     *bcLocation    `xml:"http://bibliocommons.com/rss/1.0/modules/event/ location"`
	Registration bcRegistration `xml:"http://bibliocommons.com/rss/1.0/modules/event/ registration_info"`
}

type bcLocation struct {
	Name      string `xml:"http://bibliocommons.com/rss/1.0/modules/event/ name"`
	Number    string `xml:"http://bibliocommons.com/rss/1.0/modules/event/ number"`
	Street    string `xml:"http://bibliocommons.com/rss/1.0/modules/event/ street"`
	City      string `xml:"http://bibliocommons.com/rss/1.0/modules/event/ city"`
	Zip       string `xml:"http://bibliocommons.com/rss/1.0/modules/event/ zip"`
	State     string `xml:"http://bibliocommons.com/rss/1.0/modules/event/ state"`
	Country   string `xml:"http://bibliocommons.com/rss/1.0/modules/event/ country"`
	Latitude  string `xml:"http://bibliocommons.com/rss/1.0/modules/event/ latitude"`
	Longitude string `xml:"http://bibliocommons.com/rss/1.0/modules/event/ longitude"`
	Details   string `xml:"http://bibliocommons.com/rss/1.0/modules/event/ location_details"`
}

type bcRegistration struct {
	IsRequired string `xml:"http://bibliocommons.com/rss/1.0/modules/event/ is_required"`
	IsFull     string `xml:"http://bibliocommons.com/rss/1.0/modules/event/ is_full"`
	Capacity   string `xml:"http://bibliocommons.com/rss/1.0/modules/event/ capacity"`
	Registered string `xml:"http://bibliocommons.com/rss/1.0/modules/event/ number_registered"`
}

// bcLanguageCodes maps the language names BiblioCommons emits as <category> values
// to BCP 47 codes. Unknown languages stay as keywords.
var bcLanguageCodes = map[string]string{
	"English": "en",
	"French":  "fr",
}

// BiblioCommonsRSSExtractor fetches and maps a BiblioCommons events RSS feed.
type BiblioCommonsRSSExtractor struct {
	client       *http.Client
	maxBodyBytes int64
	pageDelay    time.Duration
	logger       zerolog.Logger
}

// NewBiblioCommonsRSSExtractor creates an extractor. maxBodyBytes defaults to 5 MB per
// page when <= 0.
func NewBiblioCommonsRSSExtractor(client *http.Client, maxBodyBytes int64, logger zerolog.Logger) *BiblioCommonsRSSExtractor {
	if maxBodyBytes <= 0 {
		maxBodyBytes = defaultBCMaxBodyBytes
	}
	return &BiblioCommonsRSSExtractor{
		client:       client,
		maxBodyBytes: maxBodyBytes,
		pageDelay:    defaultBCPageDelay,
		logger:       logger,
	}
}

// Extract walks the feed pages (?page=1..cfg.MaxPages) and returns one EventInput per
// non-cancelled item. It stops early on an empty page or a page that adds nothing new.
func (e *BiblioCommonsRSSExtractor) Extract(ctx context.Context, cfg SourceConfig) ([]events.EventInput, []string, error) {
	base, err := url.Parse(cfg.URL)
	if err != nil {
		return nil, nil, fmt.Errorf("bibliocommons_rss: parse url %q: %w", cfg.URL, err)
	}
	loc, err := time.LoadLocation(cfg.GetTimezone())
	if err != nil {
		return nil, nil, fmt.Errorf("bibliocommons_rss: load timezone %q: %w", cfg.GetTimezone(), err)
	}
	maxPages := cfg.MaxPages
	if maxPages <= 0 {
		maxPages = 10
	}

	var (
		out      []events.EventInput
		warnings []string
		seen     = make(map[string]bool)
	)
	for page := 1; page <= maxPages; page++ {
		if page > 1 && e.pageDelay > 0 {
			select {
			case <-ctx.Done():
				return out, warnings, ctx.Err()
			case <-time.After(e.pageDelay):
			}
		}

		feed, err := e.fetchPage(ctx, base, page, cfg.Headers)
		if err != nil {
			return out, warnings, fmt.Errorf("bibliocommons_rss page %d: %w", page, err)
		}
		if len(feed.Channel.Items) == 0 {
			break
		}

		organizer := organizerFromChannelTitle(feed.Channel.Title)
		added := 0
		for _, item := range feed.Channel.Items {
			id := bcEventID(item)
			if id == "" {
				warnings = append(warnings, fmt.Sprintf("skipping item %q: no guid/link", item.Title))
				continue
			}
			if seen[id] {
				continue
			}
			seen[id] = true
			added++

			if strings.EqualFold(strings.TrimSpace(item.IsCancelled), "true") {
				warnings = append(warnings, fmt.Sprintf("skipping cancelled event %q (%s)", strings.TrimSpace(item.Title), id))
				continue
			}
			input, warn, ok := mapBCItem(item, id, cfg, loc, organizer)
			if warn != "" {
				warnings = append(warnings, warn)
			}
			if ok {
				out = append(out, input)
			}
		}
		if added == 0 {
			break // page repeated what we already have (server ignored ?page)
		}
	}
	return out, warnings, nil
}

func (e *BiblioCommonsRSSExtractor) fetchPage(ctx context.Context, base *url.URL, page int, extraHeaders map[string]string) (*bcFeed, error) {
	u := *base
	q := u.Query()
	q.Set("page", strconv.Itoa(page))
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/rss+xml, application/xml;q=0.9, text/xml;q=0.8")
	req.Header.Set("User-Agent", ScraperUserAgent)
	for k, v := range extraHeaders {
		req.Header.Set(k, v)
	}

	client := *e.client
	client.CheckRedirect = limitRedirects(10)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // best-effort close on HTTP response body

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, e.maxBodyBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	if int64(len(data)) > e.maxBodyBytes {
		return nil, fmt.Errorf("body exceeds %d bytes limit", e.maxBodyBytes)
	}

	var feed bcFeed
	if err := xml.Unmarshal(data, &feed); err != nil {
		return nil, fmt.Errorf("parse rss: %w", err)
	}
	return &feed, nil
}

// bcEventID returns the BiblioCommons event id: the last path segment of guid or link.
func bcEventID(item bcItem) string {
	for _, s := range []string{item.GUID, item.Link} {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		s = strings.TrimRight(s, "/")
		if i := strings.LastIndex(s, "/"); i >= 0 {
			s = s[i+1:]
		}
		if s != "" {
			return s
		}
	}
	return ""
}

// organizerFromChannelTitle turns "Events | Toronto Public Library" into an organizer.
func organizerFromChannelTitle(title string) *events.OrganizationInput {
	title = strings.TrimSpace(title)
	if i := strings.Index(title, "|"); i >= 0 {
		title = strings.TrimSpace(title[i+1:])
	}
	if title == "" {
		return nil
	}
	return &events.OrganizationInput{Name: title}
}

func mapBCItem(item bcItem, id string, cfg SourceConfig, loc *time.Location, organizer *events.OrganizationInput) (events.EventInput, string, bool) {
	name := strings.TrimSpace(item.Title)
	if name == "" {
		return events.EventInput{}, fmt.Sprintf("skipping item %s: empty title", id), false
	}
	start, ok := parseBCTime(item.StartLocal, item.StartUTC, loc)
	if !ok {
		return events.EventInput{}, fmt.Sprintf("skipping %q (%s): no usable start time", name, id), false
	}
	end, _ := parseBCTime(item.EndLocal, item.EndUTC, loc)

	link := strings.TrimSpace(item.Link)
	if link == "" {
		link = strings.TrimSpace(item.GUID)
	}

	in := events.EventInput{
		Name:        name,
		Description: buildBCDescription(item),
		StartDate:   start,
		EndDate:     end,
		URL:         link,
		Image:       strings.Replace(strings.TrimSpace(item.Enclosure.URL), "http://", "https://", 1),
		EventDomain: cfg.Domain,
		License:     cfg.License,
		Organizer:   organizer,
		Source: &events.SourceInput{
			URL:     cfg.URL,
			EventID: id,
			Name:    cfg.Name,
			License: cfg.License,
		},
	}

	for _, c := range item.Categories {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		if code, isLang := bcLanguageCodes[c]; isLang {
			in.InLanguage = append(in.InLanguage, code)
			continue
		}
		in.Keywords = append(in.Keywords, c)
	}

	if strings.EqualFold(strings.TrimSpace(item.IsVirtual), "true") || item.Location == nil {
		// Online events have no <bc:location>; the event page hosts registration/join info.
		in.VirtualLocation = &events.VirtualLocationInput{Type: "VirtualLocation", URL: link, Name: "Online event"}
	} else {
		in.Location = bcPlace(item.Location)
	}
	return in, "", true
}

func bcPlace(l *bcLocation) *events.PlaceInput {
	p := &events.PlaceInput{
		Name:            strings.TrimSpace(l.Name),
		StreetAddress:   strings.TrimSpace(strings.TrimSpace(l.Number) + " " + strings.TrimSpace(l.Street)),
		AddressLocality: strings.TrimSpace(l.City),
		AddressRegion:   strings.TrimSpace(l.State),
		PostalCode:      strings.TrimSpace(l.Zip),
		AddressCountry:  strings.TrimSpace(l.Country),
	}
	p.Latitude, _ = strconv.ParseFloat(strings.TrimSpace(l.Latitude), 64)
	p.Longitude, _ = strconv.ParseFloat(strings.TrimSpace(l.Longitude), 64)
	return p
}

// parseBCTime prefers the venue-local wall-clock value ("2026-10-16T18:00") and falls
// back to the UTC value converted to loc. Returns RFC 3339 with the zone offset.
func parseBCTime(local, utc string, loc *time.Location) (string, bool) {
	local = strings.TrimSpace(local)
	for _, layout := range []string{"2006-01-02T15:04", "2006-01-02T15:04:05"} {
		if t, err := time.ParseInLocation(layout, local, loc); err == nil {
			return t.Format(time.RFC3339), true
		}
	}
	if t, err := time.Parse(time.RFC3339, strings.TrimSpace(utc)); err == nil {
		return t.In(loc).Format(time.RFC3339), true
	}
	return "", false
}

// buildBCDescription renders the HTML description as text and appends the practical
// details (room, registration) the feed carries as separate fields.
func buildBCDescription(item bcItem) string {
	text := htmlToPlainText(item.Description)
	if i := strings.Index(text, bcPrivacyNoticeMarker); i >= 0 {
		text = strings.TrimSpace(text[:i])
	}

	var facts []string
	if item.Location != nil {
		if d := strings.TrimSpace(item.Location.Details); d != "" {
			facts = append(facts, "Room: "+d)
		}
	}
	facts = append(facts, bcRegistrationLine(item.Registration))

	parts := []string{}
	if text != "" {
		parts = append(parts, text)
	}
	parts = append(parts, strings.Join(facts, "\n"))
	return strings.Join(parts, "\n\n")
}

func bcRegistrationLine(r bcRegistration) string {
	if !strings.EqualFold(strings.TrimSpace(r.IsRequired), "true") {
		return "No registration required."
	}
	if strings.EqualFold(strings.TrimSpace(r.IsFull), "true") {
		return "Registration required — this event is full."
	}
	capacity, errC := strconv.Atoi(strings.TrimSpace(r.Capacity))
	registered, errR := strconv.Atoi(strings.TrimSpace(r.Registered))
	if errC == nil && errR == nil && capacity > 0 {
		left := capacity - registered
		if left < 0 {
			left = 0
		}
		unit := "spots"
		if left == 1 {
			unit = "spot"
		}
		return fmt.Sprintf("Registration required (%d %s remaining).", left, unit)
	}
	return "Registration required."
}

// htmlToPlainText converts an HTML fragment to readable text: block elements become
// line/paragraph breaks, entities are decoded, script/style are dropped and runs of
// whitespace are collapsed.
func htmlToPlainText(src string) string {
	var b strings.Builder
	z := html.NewTokenizer(strings.NewReader(src))
	skip := 0
	for {
		switch z.Next() {
		case html.ErrorToken:
			return tidyPlainText(b.String())
		case html.TextToken:
			if skip == 0 {
				b.WriteString(z.Token().Data)
			}
		case html.StartTagToken, html.SelfClosingTagToken:
			tok := z.Token()
			switch tok.Data {
			case "script", "style":
				if tok.Type == html.StartTagToken {
					skip++
				}
			case "br":
				b.WriteString("\n")
			}
		case html.EndTagToken:
			switch z.Token().Data {
			case "script", "style":
				if skip > 0 {
					skip--
				}
			case "p", "div", "table", "ul", "ol", "h1", "h2", "h3", "h4", "h5", "h6", "blockquote":
				b.WriteString("\n\n")
			case "li", "tr":
				b.WriteString("\n")
			}
		}
	}
}

// tidyPlainText normalises whitespace: NBSP → space, trims each line, collapses runs of
// spaces and limits blank lines to one.
func tidyPlainText(s string) string {
	s = strings.ReplaceAll(s, " ", " ")
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	blank := 0
	for _, ln := range lines {
		ln = strings.Join(strings.Fields(ln), " ")
		if ln == "" {
			blank++
			if blank > 1 || len(out) == 0 {
				continue
			}
		} else {
			blank = 0
		}
		out = append(out, ln)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

// scrapeBiblioCommonsRSS handles extraction_method: bibliocommons_rss for a single source.
func (s *Scraper) scrapeBiblioCommonsRSS(ctx context.Context, source SourceConfig, opts ScrapeOptions) (ScrapeResult, error) {
	result := ScrapeResult{
		SourceName: source.Name,
		SourceURL:  source.URL,
		Tier:       source.Tier,
		DryRun:     opts.DryRun,
	}

	return s.runWithTracking(ctx, &result, func(ctx context.Context) (int, []events.EventInput, []string, error) {
		timeout := fetchTimeout
		if source.RequestTimeoutSeconds > 0 {
			timeout = time.Duration(source.RequestTimeoutSeconds) * time.Second
		}
		opts.TLSFingerprint = source.TLSFingerprint
		extractor := NewBiblioCommonsRSSExtractor(opts.HTTPClient(timeout), source.MaxBodyBytes, s.logger)
		if s.bcPageDelay != nil {
			extractor.pageDelay = *s.bcPageDelay
		}

		eventInputs, warnings, err := extractor.Extract(ctx, source)
		if err != nil {
			return 0, nil, warnings, err
		}
		if opts.Limit > 0 && len(eventInputs) > opts.Limit {
			eventInputs = eventInputs[:opts.Limit]
		}
		return len(eventInputs), eventInputs, warnings, nil
	}), nil
}
