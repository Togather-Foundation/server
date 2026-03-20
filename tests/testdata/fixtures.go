// Package testdata provides synthetic event fixtures for testing the ingestion pipeline.
// These fixtures are based on real scraper field mappings from event_mocks.csv.
package testdata

import (
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/Togather-Foundation/server/internal/domain/events"
)

// Venue represents a test venue with realistic Toronto-area location data.
type Venue struct {
	Name            string
	StreetAddress   string
	AddressLocality string
	AddressRegion   string
	AddressCountry  string
	PostalCode      string
	Latitude        float64
	Longitude       float64
}

// Organizer represents a test event organizer.
type Organizer struct {
	Name string
	URL  string
}

// Source represents a test event source (scraper origin).
type Source struct {
	Name    string
	BaseURL string
	Type    string // API, HTML, ICS, JSONLD, RSS, PLATFORM
}

// TorontoVenues contains realistic venue data based on sources in event_mocks.csv.
var TorontoVenues = []Venue{
	{
		Name:            "The Tranzac",
		StreetAddress:   "292 Brunswick Ave",
		AddressLocality: "Toronto",
		AddressRegion:   "ON",
		AddressCountry:  "CA",
		PostalCode:      "M5S 2M7",
		Latitude:        43.6638,
		Longitude:       -79.4077,
	},
	{
		Name:            "Bampot Tea House",
		StreetAddress:   "1267 Bloor St W",
		AddressLocality: "Toronto",
		AddressRegion:   "ON",
		AddressCountry:  "CA",
		PostalCode:      "M6H 1N7",
		Latitude:        43.6610,
		Longitude:       -79.4463,
	},
	{
		Name:            "The Baby G",
		StreetAddress:   "1608 Dundas St W",
		AddressLocality: "Toronto",
		AddressRegion:   "ON",
		AddressCountry:  "CA",
		PostalCode:      "M6K 1T8",
		Latitude:        43.6489,
		Longitude:       -79.4361,
	},
	{
		Name:            "The Garrison",
		StreetAddress:   "1197 Dundas St W",
		AddressLocality: "Toronto",
		AddressRegion:   "ON",
		AddressCountry:  "CA",
		PostalCode:      "M6J 1X3",
		Latitude:        43.6493,
		Longitude:       -79.4235,
	},
	{
		Name:            "Burdock Music Hall",
		StreetAddress:   "1184 Bloor St W",
		AddressLocality: "Toronto",
		AddressRegion:   "ON",
		AddressCountry:  "CA",
		PostalCode:      "M6H 1N2",
		Latitude:        43.6607,
		Longitude:       -79.4431,
	},
	{
		Name:            "Dovercourt House",
		StreetAddress:   "805 Dovercourt Rd",
		AddressLocality: "Toronto",
		AddressRegion:   "ON",
		AddressCountry:  "CA",
		PostalCode:      "M6H 2X4",
		Latitude:        43.6600,
		Longitude:       -79.4298,
	},
	{
		Name:            "Snakes and Lattes",
		StreetAddress:   "600 Bloor St W",
		AddressLocality: "Toronto",
		AddressRegion:   "ON",
		AddressCountry:  "CA",
		PostalCode:      "M6G 1K4",
		Latitude:        43.6644,
		Longitude:       -79.4156,
	},
	{
		Name:            "Centre for Social Innovation",
		StreetAddress:   "192 Spadina Ave",
		AddressLocality: "Toronto",
		AddressRegion:   "ON",
		AddressCountry:  "CA",
		PostalCode:      "M5T 2C2",
		Latitude:        43.6496,
		Longitude:       -79.3961,
	},
	{
		Name:            "InterAccess",
		StreetAddress:   "950 Dupont St",
		AddressLocality: "Toronto",
		AddressRegion:   "ON",
		AddressCountry:  "CA",
		PostalCode:      "M6H 1Z2",
		Latitude:        43.6683,
		Longitude:       -79.4356,
	},
	{
		Name:            "Glad Day Bookshop",
		StreetAddress:   "499 Church St",
		AddressLocality: "Toronto",
		AddressRegion:   "ON",
		AddressCountry:  "CA",
		PostalCode:      "M4Y 2C6",
		Latitude:        43.6653,
		Longitude:       -79.3810,
	},
}

// SampleOrganizers contains organizers from event_mocks.csv sources.
var SampleOrganizers = []Organizer{
	{Name: "Toronto Arts Foundation", URL: "https://torontoarts.org"},
	{Name: "Trampoline Hall", URL: "https://lu.ma/trampolinehall"},
	{Name: "Epic Llama Events", URL: "https://lu.ma/epicllama"},
	{Name: "Toronto Society", URL: "https://lu.ma/torontosociety"},
	{Name: "HTML in the Park", URL: "https://lu.ma/htmlinthepark"},
	{Name: "Less Wrong Toronto", URL: "https://meetup.com/less-wrong-toronto"},
	{Name: "Book Club for Humans", URL: "https://meetup.com/bookclubforhumans"},
	{Name: "Creative Club Pauza", URL: "https://eventbrite.ca/creativeclubpauza"},
	{Name: "Radical Aliveness Toronto", URL: "https://eventbrite.ca/radicalaliveness"},
	{Name: "Fuckup Nights Toronto", URL: "https://fuckupnights.com/toronto"},
}

// SampleSources contains source configurations based on event_mocks.csv.
var SampleSources = []Source{
	{Name: "Eventbrite", BaseURL: "https://www.eventbrite.ca", Type: "JSONLD"},
	{Name: "Meetup", BaseURL: "https://www.meetup.com", Type: "JSONLD"},
	{Name: "Lu.ma", BaseURL: "https://lu.ma", Type: "ICS"},
	{Name: "Squarespace", BaseURL: "https://example.com", Type: "API"},
	{Name: "BlogTO", BaseURL: "https://www.blogto.com", Type: "API"},
	{Name: "Exclaim", BaseURL: "https://exclaim.ca", Type: "API"},
	{Name: "Showpass", BaseURL: "https://www.showpass.com", Type: "API"},
	{Name: "Google Calendar", BaseURL: "https://calendar.google.com", Type: "ICS"},
}

// EventCategory represents different event types for generating varied test data.
type EventCategory string

const (
	CategoryMusic     EventCategory = "music"
	CategoryArts      EventCategory = "arts"
	CategoryTech      EventCategory = "tech"
	CategorySocial    EventCategory = "social"
	CategoryEducation EventCategory = "education"
	CategoryGames     EventCategory = "games"
)

// eventTitleTemplates provides realistic event name patterns.
var eventTitleTemplates = map[EventCategory][]string{
	CategoryMusic: {
		"%s Live at %s",
		"%s Album Release Party",
		"Jazz Night: %s Quartet",
		"%s & Friends",
		"Open Mic Night",
		"%s Unplugged",
		"Synth Sunday with %s",
	},
	CategoryArts: {
		"%s: New Works Exhibition",
		"Artist Talk: %s",
		"Gallery Opening: %s",
		"Creative Workshop: %s",
		"%s Art Walk",
		"Collective %s Show",
	},
	CategoryTech: {
		"%s Meetup",
		"Tech Talk: %s",
		"Hack Night: %s",
		"%s Workshop",
		"Demo Day: %s",
		"%s User Group",
	},
	CategorySocial: {
		"%s Social",
		"Community Gathering: %s",
		"%s Mixer",
		"Networking Night: %s",
		"%s Potluck",
		"Book Club: %s",
	},
	CategoryEducation: {
		"Lecture: %s",
		"%s Masterclass",
		"Panel Discussion: %s",
		"Workshop: Introduction to %s",
		"%s Salon",
		"Learning Circle: %s",
	},
	CategoryGames: {
		"Board Game Night: %s",
		"%s Tournament",
		"Game Jam: %s",
		"RPG Night: %s",
		"Trivia: %s Edition",
	},
}

// sampleArtists/topics for template substitution.
var sampleSubjects = map[EventCategory][]string{
	CategoryMusic:     {"Moonlight Syndicate", "The Velvet Echo", "Sarah Chen", "DJ Nomad", "Brass Collective", "Night Owl", "Ambient Drift"},
	CategoryArts:      {"Urban Perspectives", "Digital Horizons", "Textile Dreams", "Found Objects", "Light & Shadow", "Contemporary Voices"},
	CategoryTech:      {"Go", "Rust", "AI/ML", "Web3", "Cloud Native", "Open Source", "Data Engineering"},
	CategorySocial:    {"West End", "Creative", "Young Professionals", "Newcomers", "LGBTQ+", "Neighborhood"},
	CategoryEducation: {"Philosophy", "History", "Science", "Literature", "Economics", "Psychology"},
	CategoryGames:     {"Settlers of Catan", "D&D", "Magic: The Gathering", "Strategy", "Cooperative", "Euro Games"},
}

// Generator creates synthetic event fixtures for testing.
type Generator struct {
	rng      *rand.Rand
	baseTime time.Time
}

// NewGenerator creates a new fixture generator with the given seed for reproducibility.
func NewGenerator(seed int64) *Generator {
	return &Generator{
		rng:      rand.New(rand.NewSource(seed)),
		baseTime: time.Now().Truncate(24 * time.Hour).Add(7 * 24 * time.Hour), // 1 week from now
	}
}

// NewDeterministicGenerator creates a generator with a fixed seed for deterministic tests.
func NewDeterministicGenerator() *Generator {
	return NewGenerator(42)
}

// RandomEventInput generates a random valid EventInput.
func (g *Generator) RandomEventInput() events.EventInput {
	category := g.randomCategory()
	venue := g.randomVenue()
	organizer := g.randomOrganizer()
	source := g.randomSource()

	startTime := g.randomFutureTime()
	endTime := startTime.Add(time.Duration(g.rng.Intn(4)+1) * time.Hour)

	return events.EventInput{
		Name:        g.generateTitle(category),
		Description: g.generateDescription(category),
		StartDate:   startTime.Format(time.RFC3339),
		EndDate:     endTime.Format(time.RFC3339),
		Location: &events.PlaceInput{
			Name:            venue.Name,
			StreetAddress:   venue.StreetAddress,
			AddressLocality: venue.AddressLocality,
			AddressRegion:   venue.AddressRegion,
			AddressCountry:  venue.AddressCountry,
			PostalCode:      venue.PostalCode,
			Latitude:        venue.Latitude,
			Longitude:       venue.Longitude,
		},
		Organizer: &events.OrganizationInput{
			Name: organizer.Name,
			URL:  organizer.URL,
		},
		Source: &events.SourceInput{
			URL:     fmt.Sprintf("%s/events/%d", source.BaseURL, g.rng.Intn(100000)),
			EventID: fmt.Sprintf("evt-%d", g.rng.Intn(100000)),
			Name:    source.Name,
		},
		Image:    fmt.Sprintf("https://images.example.com/events/%d.jpg", g.rng.Intn(1000)),
		URL:      fmt.Sprintf("https://example.com/events/%d", g.rng.Intn(100000)),
		Keywords: g.randomKeywords(category),
		License:  "https://creativecommons.org/publicdomain/zero/1.0/",
	}
}

// MinimalEventInput generates a minimal valid EventInput with only required fields.
func (g *Generator) MinimalEventInput() events.EventInput {
	venue := g.randomVenue()
	startTime := g.randomFutureTime()

	return events.EventInput{
		Name:      fmt.Sprintf("Minimal Event %d", g.rng.Intn(1000)),
		StartDate: startTime.Format(time.RFC3339),
		Location: &events.PlaceInput{
			Name: venue.Name,
		},
	}
}

// VirtualEventInput generates an event with only a virtual location.
func (g *Generator) VirtualEventInput() events.EventInput {
	category := g.randomCategory()
	startTime := g.randomFutureTime()
	endTime := startTime.Add(time.Duration(g.rng.Intn(2)+1) * time.Hour)

	return events.EventInput{
		Name:        g.generateTitle(category) + " (Online)",
		Description: g.generateDescription(category),
		StartDate:   startTime.Format(time.RFC3339),
		EndDate:     endTime.Format(time.RFC3339),
		VirtualLocation: &events.VirtualLocationInput{
			Type: "VirtualLocation",
			URL:  fmt.Sprintf("https://zoom.us/j/%d", g.rng.Intn(10000000000)),
			Name: "Zoom Meeting",
		},
		Keywords: g.randomKeywords(category),
		License:  "https://creativecommons.org/publicdomain/zero/1.0/",
	}
}

// HybridEventInput generates an event with both physical and virtual locations.
func (g *Generator) HybridEventInput() events.EventInput {
	input := g.RandomEventInput()
	input.Name = input.Name + " (Hybrid)"
	input.VirtualLocation = &events.VirtualLocationInput{
		Type: "VirtualLocation",
		URL:  fmt.Sprintf("https://meet.google.com/%s", g.randomMeetingCode()),
		Name: "Google Meet",
	}
	return input
}

// EventInputWithOccurrences generates an event with multiple occurrences (recurring).
// Note: Current validation requires startDate even with occurrences, so we set it
// to the first occurrence's start time.
func (g *Generator) EventInputWithOccurrences(count int) events.EventInput {
	input := g.RandomEventInput()
	input.Occurrences = make([]events.OccurrenceInput, count)

	baseTime := g.randomFutureTime()
	for i := 0; i < count; i++ {
		start := baseTime.Add(time.Duration(i*7*24) * time.Hour) // Weekly
		end := start.Add(2 * time.Hour)
		input.Occurrences[i] = events.OccurrenceInput{
			StartDate: start.Format(time.RFC3339),
			EndDate:   end.Format(time.RFC3339),
			Timezone:  "America/Toronto",
		}
	}

	// Set top-level startDate to first occurrence (required by current validation)
	input.StartDate = input.Occurrences[0].StartDate
	input.EndDate = input.Occurrences[0].EndDate

	return input
}

// EventInputNeedsReview generates an event that should trigger review (missing description/image).
func (g *Generator) EventInputNeedsReview() events.EventInput {
	venue := g.randomVenue()
	startTime := g.randomFutureTime()

	return events.EventInput{
		Name:      fmt.Sprintf("Sparse Event %d", g.rng.Intn(1000)),
		StartDate: startTime.Format(time.RFC3339),
		Location: &events.PlaceInput{
			Name:            venue.Name,
			AddressLocality: venue.AddressLocality,
			AddressRegion:   venue.AddressRegion,
		},
		// Missing: Description, Image - should trigger review
		License: "https://creativecommons.org/publicdomain/zero/1.0/",
	}
}

// EventInputFarFuture generates an event more than 2 years in the future (triggers review).
func (g *Generator) EventInputFarFuture() events.EventInput {
	input := g.RandomEventInput()
	farFuture := time.Now().Add(800 * 24 * time.Hour) // ~2.2 years
	input.StartDate = farFuture.Format(time.RFC3339)
	input.EndDate = farFuture.Add(2 * time.Hour).Format(time.RFC3339)
	return input
}

// BatchEventInputs generates a batch of random events.
func (g *Generator) BatchEventInputs(count int) []events.EventInput {
	inputs := make([]events.EventInput, count)
	for i := 0; i < count; i++ {
		inputs[i] = g.RandomEventInput()
	}
	return inputs
}

// DuplicateCandidates generates two events that should be detected as duplicates
// (same name, venue, and start time).
func (g *Generator) DuplicateCandidates() (events.EventInput, events.EventInput) {
	first := g.RandomEventInput()

	second := events.EventInput{
		Name:        first.Name,
		Description: "Different description but same core details",
		StartDate:   first.StartDate,
		EndDate:     first.EndDate,
		Location:    first.Location,
		Organizer: &events.OrganizationInput{
			Name: "Different Organizer",
		},
		Source: &events.SourceInput{
			URL:     "https://different-source.com/events/999",
			EventID: "diff-evt-999",
			Name:    "Different Source",
		},
		License: "https://creativecommons.org/publicdomain/zero/1.0/",
	}

	return first, second
}

// Helper methods

func (g *Generator) randomCategory() EventCategory {
	categories := []EventCategory{
		CategoryMusic, CategoryArts, CategoryTech,
		CategorySocial, CategoryEducation, CategoryGames,
	}
	return categories[g.rng.Intn(len(categories))]
}

func (g *Generator) randomVenue() Venue {
	return TorontoVenues[g.rng.Intn(len(TorontoVenues))]
}

func (g *Generator) randomOrganizer() Organizer {
	return SampleOrganizers[g.rng.Intn(len(SampleOrganizers))]
}

func (g *Generator) randomSource() Source {
	return SampleSources[g.rng.Intn(len(SampleSources))]
}

func (g *Generator) randomFutureTime() time.Time {
	daysAhead := g.rng.Intn(60) + 1  // 1-60 days
	hourOfDay := g.rng.Intn(10) + 10 // 10am - 8pm

	return g.baseTime.Add(time.Duration(daysAhead*24+hourOfDay) * time.Hour)
}

func (g *Generator) generateTitle(category EventCategory) string {
	templates := eventTitleTemplates[category]
	template := templates[g.rng.Intn(len(templates))]

	subjects := sampleSubjects[category]
	subject := subjects[g.rng.Intn(len(subjects))]

	venue := g.randomVenue()

	// Count %s placeholders to avoid fmt.Sprintf EXTRA errors
	placeholderCount := strings.Count(template, "%s")

	switch placeholderCount {
	case 0:
		return template
	case 1:
		return fmt.Sprintf(template, subject)
	case 2:
		return fmt.Sprintf(template, subject, venue.Name)
	default:
		// Fallback: use subject only for safety
		return fmt.Sprintf(template, subject)
	}
}

func (g *Generator) generateDescription(category EventCategory) string {
	descriptions := map[EventCategory][]string{
		CategoryMusic: {
			"Join us for an evening of live music featuring local artists. Doors open at 7pm, music starts at 8pm. All ages welcome.",
			"A night of incredible performances in an intimate setting. Cash bar available. Support local music!",
			"Experience the sounds of Toronto's vibrant music scene. Reserved seating available.",
		},
		CategoryArts: {
			"An exhibition exploring contemporary themes through mixed media. Artist talk at 6pm followed by reception.",
			"Join local artists for a creative workshop. All materials provided. Suitable for all skill levels.",
			"Opening reception for our newest exhibition. Light refreshments served. Free admission.",
		},
		CategoryTech: {
			"Monthly meetup for developers and tech enthusiasts. Lightning talks followed by networking. Pizza provided!",
			"Hands-on workshop covering the latest tools and techniques. Bring your laptop. Beginners welcome.",
			"Join us for an evening of demos, discussions, and debugging. Share your projects and learn from others.",
		},
		CategorySocial: {
			"A casual gathering for community members to connect. Snacks provided. All welcome!",
			"Monthly mixer for professionals and creatives. Cash bar. Come make new friends!",
			"Community potluck and social. Bring a dish to share. Family friendly.",
		},
		CategoryEducation: {
			"An engaging lecture exploring fascinating topics. Q&A to follow. Free admission, donations welcome.",
			"Intensive workshop for those looking to deepen their understanding. Materials included in registration.",
			"Panel discussion featuring experts in the field. Audience questions encouraged.",
		},
		CategoryGames: {
			"Weekly game night at our favorite local venue. Bring your own games or play ours. All skill levels.",
			"Competitive tournament with prizes! Registration required. Spectators welcome.",
			"Learn to play new games in a friendly environment. Snacks available. Newcomers encouraged!",
		},
	}

	descs := descriptions[category]
	return descs[g.rng.Intn(len(descs))]
}

func (g *Generator) randomKeywords(category EventCategory) []string {
	categoryKeywords := map[EventCategory][]string{
		CategoryMusic:     {"music", "live", "concert", "performance", "toronto"},
		CategoryArts:      {"art", "exhibition", "gallery", "creative", "toronto"},
		CategoryTech:      {"tech", "software", "programming", "meetup", "toronto"},
		CategorySocial:    {"social", "community", "networking", "gathering", "toronto"},
		CategoryEducation: {"education", "learning", "lecture", "workshop", "toronto"},
		CategoryGames:     {"games", "board games", "gaming", "fun", "toronto"},
	}

	keywords := categoryKeywords[category]
	// Return 2-4 random keywords
	count := g.rng.Intn(3) + 2
	if count > len(keywords) {
		count = len(keywords)
	}

	// Shuffle and take first count
	shuffled := make([]string, len(keywords))
	copy(shuffled, keywords)
	g.rng.Shuffle(len(shuffled), func(i, j int) {
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	})

	return shuffled[:count]
}

func (g *Generator) randomMeetingCode() string {
	chars := "abcdefghijklmnopqrstuvwxyz"
	code := make([]byte, 12)
	for i := range code {
		if i == 3 || i == 7 {
			code[i] = '-'
		} else {
			code[i] = chars[g.rng.Intn(len(chars))]
		}
	}
	return string(code)
}

// EventInputReversedDates generates an event with end date before start date (timezone issue).
// This simulates a common data quality issue where late-night events (e.g., 11pm-2am)
// have the end time on the same calendar day as the start, causing the end to appear
// before the start when parsed as the same day.
func (g *Generator) EventInputReversedDates() events.EventInput {
	venue := g.randomVenue()
	category := g.randomCategory()
	organizer := g.randomOrganizer()
	source := g.randomSource()

	// Create a late-night event: 11pm start
	startTime := g.randomFutureTime()
	startTime = time.Date(startTime.Year(), startTime.Month(), startTime.Day(), 23, 0, 0, 0, startTime.Location())

	// End time at 2am on same date (which is actually wrong - should be next day)
	endTime := time.Date(startTime.Year(), startTime.Month(), startTime.Day(), 2, 0, 0, 0, startTime.Location())

	return events.EventInput{
		Name:        g.generateTitle(category) + " (Late Night)",
		Description: g.generateDescription(category) + " Event runs past midnight.",
		StartDate:   startTime.Format(time.RFC3339),
		EndDate:     endTime.Format(time.RFC3339), // This will be before startDate!
		Location: &events.PlaceInput{
			Name:            venue.Name,
			StreetAddress:   venue.StreetAddress,
			AddressLocality: venue.AddressLocality,
			AddressRegion:   venue.AddressRegion,
			AddressCountry:  venue.AddressCountry,
			PostalCode:      venue.PostalCode,
			Latitude:        venue.Latitude,
			Longitude:       venue.Longitude,
		},
		Organizer: &events.OrganizationInput{
			Name: organizer.Name,
			URL:  organizer.URL,
		},
		Source: &events.SourceInput{
			URL:     fmt.Sprintf("%s/events/%d", source.BaseURL, g.rng.Intn(100000)),
			EventID: fmt.Sprintf("evt-reversed-%d", g.rng.Intn(100000)),
			Name:    source.Name,
		},
		Image:    fmt.Sprintf("https://images.example.com/events/%d.jpg", g.rng.Intn(1000)),
		URL:      fmt.Sprintf("https://example.com/events/%d", g.rng.Intn(100000)),
		Keywords: g.randomKeywords(category),
		License:  "https://creativecommons.org/publicdomain/zero/1.0/",
	}
}

// EventInputMissingVenue generates an event with no venue location (only virtual or missing).
// This tests handling of events that lack physical location data.
func (g *Generator) EventInputMissingVenue() events.EventInput {
	category := g.randomCategory()
	startTime := g.randomFutureTime()
	endTime := startTime.Add(time.Duration(g.rng.Intn(2)+1) * time.Hour)
	organizer := g.randomOrganizer()
	source := g.randomSource()

	return events.EventInput{
		Name:        g.generateTitle(category) + " (Online)",
		Description: g.generateDescription(category),
		StartDate:   startTime.Format(time.RFC3339),
		EndDate:     endTime.Format(time.RFC3339),
		// No Location field at all - should trigger review
		VirtualLocation: &events.VirtualLocationInput{
			Type: "VirtualLocation",
			URL:  fmt.Sprintf("https://zoom.us/j/%d", g.rng.Intn(10000000000)),
			Name: "Zoom Meeting",
		},
		Organizer: &events.OrganizationInput{
			Name: organizer.Name,
			URL:  organizer.URL,
		},
		Source: &events.SourceInput{
			URL:     fmt.Sprintf("%s/events/%d", source.BaseURL, g.rng.Intn(100000)),
			EventID: fmt.Sprintf("evt-novenue-%d", g.rng.Intn(100000)),
			Name:    source.Name,
		},
		Image:    fmt.Sprintf("https://images.example.com/events/%d.jpg", g.rng.Intn(1000)),
		URL:      fmt.Sprintf("https://example.com/events/%d", g.rng.Intn(100000)),
		Keywords: g.randomKeywords(category),
		License:  "https://creativecommons.org/publicdomain/zero/1.0/",
	}
}

// EventInputLikelyDuplicate generates an event similar to another (for duplicate warnings).
// Uses recognizable patterns in name and timing to simulate duplicate detection scenarios.
func (g *Generator) EventInputLikelyDuplicate() events.EventInput {
	venue := g.randomVenue()
	startTime := g.randomFutureTime()
	endTime := startTime.Add(2 * time.Hour)
	organizer := g.randomOrganizer()
	source := g.randomSource()

	// Use a highly recognizable event name pattern that suggests duplication
	eventName := "Weekly Community Meetup at " + venue.Name

	return events.EventInput{
		Name:        eventName,
		Description: "Regular weekly community gathering. Bring your friends!",
		StartDate:   startTime.Format(time.RFC3339),
		EndDate:     endTime.Format(time.RFC3339),
		Location: &events.PlaceInput{
			Name:            venue.Name,
			StreetAddress:   venue.StreetAddress,
			AddressLocality: venue.AddressLocality,
			AddressRegion:   venue.AddressRegion,
			AddressCountry:  venue.AddressCountry,
			PostalCode:      venue.PostalCode,
			Latitude:        venue.Latitude,
			Longitude:       venue.Longitude,
		},
		Organizer: &events.OrganizationInput{
			Name: organizer.Name,
			URL:  organizer.URL,
		},
		Source: &events.SourceInput{
			URL:     fmt.Sprintf("%s/events/%d", source.BaseURL, g.rng.Intn(100000)),
			EventID: fmt.Sprintf("evt-dupe-%d", g.rng.Intn(100000)),
			Name:    source.Name,
		},
		Image:    fmt.Sprintf("https://images.example.com/events/%d.jpg", g.rng.Intn(1000)),
		URL:      fmt.Sprintf("https://example.com/events/%d", g.rng.Intn(100000)),
		Keywords: []string{"community", "meetup", "weekly", "toronto"},
		License:  "https://creativecommons.org/publicdomain/zero/1.0/",
	}
}

// EventInputMultipleWarnings generates an event with several data quality issues.
// This simulates real-world scraper data with multiple fixable problems.
func (g *Generator) EventInputMultipleWarnings() events.EventInput {
	venue := g.randomVenue()
	category := g.randomCategory()
	source := g.randomSource()

	startTime := g.randomFutureTime()

	return events.EventInput{
		Name: g.generateTitle(category),
		// Missing: Description (should trigger review)
		StartDate: startTime.Format("2006-01-02"), // Date only, no time (should trigger warning)
		// Missing: EndDate (should be inferred)
		Location: &events.PlaceInput{
			Name: venue.Name,
			// Missing: detailed address fields (partial location data)
			AddressLocality: venue.AddressLocality,
			AddressRegion:   venue.AddressRegion,
		},
		// Missing: Organizer
		Source: &events.SourceInput{
			URL:     fmt.Sprintf("%s/events/%d", source.BaseURL, g.rng.Intn(100000)),
			EventID: fmt.Sprintf("evt-multi-%d", g.rng.Intn(100000)),
			Name:    source.Name,
		},
		// Missing: Image (should trigger review)
		URL:      fmt.Sprintf("https://example.com/events/%d", g.rng.Intn(100000)),
		Keywords: g.randomKeywords(category),
		License:  "https://creativecommons.org/publicdomain/zero/1.0/",
	}
}

// BatchReviewQueueInputs generates multiple review-triggering events with varied scenarios.
// This is useful for populating a review queue with diverse test data.
func (g *Generator) BatchReviewQueueInputs(count int) []events.EventInput {
	inputs := make([]events.EventInput, count)

	for i := 0; i < count; i++ {
		// Rotate through different warning scenarios
		switch i % 4 {
		case 0:
			inputs[i] = g.EventInputReversedDates()
		case 1:
			inputs[i] = g.EventInputMissingVenue()
		case 2:
			inputs[i] = g.EventInputLikelyDuplicate()
		case 3:
			inputs[i] = g.EventInputMultipleWarnings()
		}
	}

	return inputs
}

// ---------------------------------------------------------------------------
// Recurring Series Builder (for add-occurrence workflow testing)
// ---------------------------------------------------------------------------

// RecurringSeriesBuilder provides a fluent API for constructing recurring-series
// events with multiple occurrences, useful for testing add-occurrence workflows.
type RecurringSeriesBuilder struct {
	gen         *Generator
	name        string
	description string
	venue       *Venue
	organizer   *Organizer
	source      *Source
	occurrences []events.OccurrenceInput
	timezone    string
	duration    time.Duration // per occurrence
	image       string
	url         string
	keywords    []string
	license     string
}

// NewRecurringSeriesBuilder creates a new builder with sensible defaults.
func (g *Generator) NewRecurringSeriesBuilder() *RecurringSeriesBuilder {
	v := g.randomVenue()
	o := g.randomOrganizer()
	s := g.randomSource()
	return &RecurringSeriesBuilder{
		gen:         g,
		name:        "Weekly Series",
		description: "A recurring weekly event series",
		venue:       &v,
		organizer:   &o,
		source:      &s,
		timezone:    "America/Toronto",
		duration:    2 * time.Hour,
		occurrences: []events.OccurrenceInput{},
		image:       unsplashImage(g.rng.Intn(11)),
		url:         fmt.Sprintf("%s/events/%d", s.BaseURL, g.rng.Intn(100000)),
		keywords:    []string{"recurring", "series", "toronto"},
		license:     "https://creativecommons.org/publicdomain/zero/1.0/",
	}
}

// WithName sets the event name.
func (b *RecurringSeriesBuilder) WithName(name string) *RecurringSeriesBuilder {
	b.name = name
	return b
}

// WithDescription sets the event description.
func (b *RecurringSeriesBuilder) WithDescription(description string) *RecurringSeriesBuilder {
	b.description = description
	return b
}

// WithVenue sets the venue.
func (b *RecurringSeriesBuilder) WithVenue(venue Venue) *RecurringSeriesBuilder {
	b.venue = &venue
	return b
}

// WithOrganizer sets the organizer.
func (b *RecurringSeriesBuilder) WithOrganizer(organizer Organizer) *RecurringSeriesBuilder {
	b.organizer = &organizer
	return b
}

// WithTimezone sets the timezone for occurrences (default: America/Toronto).
func (b *RecurringSeriesBuilder) WithTimezone(tz string) *RecurringSeriesBuilder {
	b.timezone = tz
	return b
}

// WithDuration sets the duration for each occurrence (default: 2 hours).
func (b *RecurringSeriesBuilder) WithDuration(d time.Duration) *RecurringSeriesBuilder {
	b.duration = d
	return b
}

// WithKeywords sets the keywords.
func (b *RecurringSeriesBuilder) WithKeywords(keywords []string) *RecurringSeriesBuilder {
	b.keywords = keywords
	return b
}

// WithWeeklyOccurrences generates `count` weekly occurrences starting from `startTime`.
func (b *RecurringSeriesBuilder) WithWeeklyOccurrences(startTime time.Time, count int) *RecurringSeriesBuilder {
	b.occurrences = make([]events.OccurrenceInput, count)
	for i := 0; i < count; i++ {
		occStart := startTime.Add(time.Duration(i*7*24) * time.Hour) // Weekly
		occEnd := occStart.Add(b.duration)
		b.occurrences[i] = events.OccurrenceInput{
			StartDate: occStart.Format(time.RFC3339),
			EndDate:   occEnd.Format(time.RFC3339),
			Timezone:  b.timezone,
		}
	}
	return b
}

// WithBiweeklyOccurrences generates `count` biweekly occurrences starting from `startTime`.
func (b *RecurringSeriesBuilder) WithBiweeklyOccurrences(startTime time.Time, count int) *RecurringSeriesBuilder {
	b.occurrences = make([]events.OccurrenceInput, count)
	for i := 0; i < count; i++ {
		occStart := startTime.Add(time.Duration(i*14*24) * time.Hour) // Biweekly
		occEnd := occStart.Add(b.duration)
		b.occurrences[i] = events.OccurrenceInput{
			StartDate: occStart.Format(time.RFC3339),
			EndDate:   occEnd.Format(time.RFC3339),
			Timezone:  b.timezone,
		}
	}
	return b
}

// WithOccurrences sets explicit occurrences (overrides weekly/biweekly generation).
func (b *RecurringSeriesBuilder) WithOccurrences(occs []events.OccurrenceInput) *RecurringSeriesBuilder {
	b.occurrences = occs
	return b
}

// Build constructs and returns the final EventInput.
func (b *RecurringSeriesBuilder) Build() events.EventInput {
	result := events.EventInput{
		Name:        b.name,
		Description: b.description,
		Location: &events.PlaceInput{
			Name:            b.venue.Name,
			StreetAddress:   b.venue.StreetAddress,
			AddressLocality: b.venue.AddressLocality,
			AddressRegion:   b.venue.AddressRegion,
			AddressCountry:  b.venue.AddressCountry,
			PostalCode:      b.venue.PostalCode,
			Latitude:        b.venue.Latitude,
			Longitude:       b.venue.Longitude,
		},
		Organizer: &events.OrganizationInput{
			Name: b.organizer.Name,
			URL:  b.organizer.URL,
		},
		Source: &events.SourceInput{
			URL:     fmt.Sprintf("%s/events/%d", b.source.BaseURL, b.gen.rng.Intn(100000)),
			EventID: fmt.Sprintf("evt-series-%d", b.gen.rng.Intn(100000)),
			Name:    b.source.Name,
		},
		Image:       b.image,
		URL:         b.url,
		Keywords:    b.keywords,
		License:     b.license,
		Occurrences: b.occurrences,
	}

	// Set top-level startDate/endDate to first occurrence (required by validation)
	if len(b.occurrences) > 0 {
		result.StartDate = b.occurrences[0].StartDate
		result.EndDate = b.occurrences[0].EndDate
	} else {
		// If no occurrences specified, use a default future time
		baseTime := b.gen.randomFutureTime()
		result.StartDate = baseTime.Format(time.RFC3339)
		result.EndDate = baseTime.Add(b.duration).Format(time.RFC3339)
	}

	return result
}

// ---------------------------------------------------------------------------
// Review Event Fixtures (RS-XX scenario groups)
// ---------------------------------------------------------------------------

// ReviewEventScenario groups a set of related EventInputs for exercising review
// queue workflows. Each scenario has a unique GroupID (e.g. "RS-01") and a
// human-readable description of the intended test scenario.
type ReviewEventScenario struct {
	GroupID     string              // "RS-01", "RS-02", etc.
	Description string              // Human-readable scenario description
	Events      []events.EventInput // Events in this scenario group
}

// reviewSources returns source configs that are safe for staging (no example.com domains).
// These are separate from SampleSources to guarantee no blocked domains appear.
var reviewSources = []Source{
	{Name: "Eventbrite", BaseURL: "https://www.eventbrite.ca", Type: "JSONLD"},
	{Name: "Meetup", BaseURL: "https://www.meetup.com", Type: "JSONLD"},
	{Name: "Lu.ma", BaseURL: "https://lu.ma", Type: "ICS"},
	{Name: "BlogTO", BaseURL: "https://www.blogto.com", Type: "API"},
	{Name: "Showpass", BaseURL: "https://www.showpass.com", Type: "API"},
	{Name: "Google Calendar", BaseURL: "https://calendar.google.com", Type: "ICS"},
}

// unsplashImage returns a deterministic Unsplash placeholder URL for a given slot index.
func unsplashImage(slot int) string {
	ids := []string{
		"1501281668745-f7f57925be31",
		"1523580846011-d3a5bc25702b",
		"1551434678-e076c223a692",
		"1516450360452-9312f5463d52",
		"1511795409834-ef04bbd61622",
		"1464366400600-7168b8af9bc3",
		"1514525253161-7a46d19cd819",
		"1470229722913-7c0e2dbbafd3",
		"1533174072545-7a4b6ad7a6c3",
		"1548199973-03cce0bbc87b",
		"1580587771525-4e99a40d8d5a",
	}
	return "https://images.unsplash.com/photo-" + ids[slot%len(ids)]
}

// reviewSourceURL builds a stable event URL from a source and a fixture ID string
// so that different fixtures never share the same EventID.
func reviewSourceURL(src Source, fixtureID string) (string, string) {
	url := fmt.Sprintf("%s/e/%s", src.BaseURL, fixtureID)
	return url, fixtureID
}

// BatchReviewEventInputs returns a curated, named fixture set for exercising review
// queue workflows. Each ReviewEventScenario is a "scenario group" — events that belong
// together (e.g. a base series + its near-duplicate).
//
// All URLs use real-looking domains (eventbrite.ca, meetup.com, lu.ma, etc.) and
// image URLs use images.unsplash.com. No example.com domains are used.
func (g *Generator) BatchReviewEventInputs() []ReviewEventScenario {
	// Fixed anchor time: next Monday at 10:00 AM (deterministic regardless of when tests run).
	// We compute the upcoming Monday from the generator's baseTime.
	anchor := g.baseTime.Truncate(24 * time.Hour)
	for anchor.Weekday() != time.Monday {
		anchor = anchor.Add(24 * time.Hour)
	}
	anchor = anchor.Add(10 * time.Hour) // 10:00 AM

	week := 7 * 24 * time.Hour

	eb := reviewSources[0]     // Eventbrite
	mu := reviewSources[1]     // Meetup
	luma := reviewSources[2]   // Lu.ma
	blogto := reviewSources[3] // BlogTO
	sp := reviewSources[4]     // Showpass
	gcal := reviewSources[5]   // Google Calendar

	yoga := TorontoVenues[0]       // The Tranzac
	bookclub := TorontoVenues[6]   // Snakes and Lattes
	techMeetup := TorontoVenues[7] // Centre for Social Innovation
	artWalk := TorontoVenues[3]    // The Garrison
	workshop := TorontoVenues[4]   // Burdock Music Hall
	jazz := TorontoVenues[5]       // Dovercourt House
	dance := TorontoVenues[2]      // The Baby G
	potluck := TorontoVenues[8]    // InterAccess
	film := TorontoVenues[9]       // Glad Day Bookshop
	choir := TorontoVenues[1]      // Bampot Tea House
	pottery := TorontoVenues[0]    // The Tranzac (reused)

	yogaOrg := SampleOrganizers[0]    // Toronto Arts Foundation
	bookOrg := SampleOrganizers[6]    // Book Club for Humans
	techOrg := SampleOrganizers[5]    // Less Wrong Toronto
	artOrg := SampleOrganizers[7]     // Creative Club Pauza
	wsOrg := SampleOrganizers[1]      // Trampoline Hall
	jazzOrg := SampleOrganizers[8]    // Radical Aliveness Toronto
	danceOrg := SampleOrganizers[2]   // Epic Llama Events
	potluckOrg := SampleOrganizers[3] // Toronto Society
	filmOrg := SampleOrganizers[4]    // HTML in the Park
	choirOrg := SampleOrganizers[9]   // Fuckup Nights Toronto
	potteryOrg := SampleOrganizers[0] // Toronto Arts Foundation

	loc := func(v Venue) *events.PlaceInput {
		return &events.PlaceInput{
			Name:            v.Name,
			StreetAddress:   v.StreetAddress,
			AddressLocality: v.AddressLocality,
			AddressRegion:   v.AddressRegion,
			AddressCountry:  v.AddressCountry,
			PostalCode:      v.PostalCode,
			Latitude:        v.Latitude,
			Longitude:       v.Longitude,
		}
	}
	org := func(o Organizer) *events.OrganizationInput {
		return &events.OrganizationInput{Name: o.Name, URL: o.URL}
	}
	src := func(s Source, id string) *events.SourceInput {
		u, eid := reviewSourceURL(s, id)
		return &events.SourceInput{URL: u, EventID: eid, Name: s.Name}
	}

	// -----------------------------------------------------------------------
	// RS-01: Weekly Yoga — Same session ingested from two scrapers (Eventbrite
	// + Lu.ma) on the same date → near-dup fires → companion pair → merge
	// duplicate path.
	// -----------------------------------------------------------------------
	rs01Base := g.NewRecurringSeriesBuilder().
		WithName("RS-01 Weekly Yoga at The Tranzac").
		WithDescription("Beginner-friendly yoga every Monday morning. Bring your own mat. All levels welcome.").
		WithVenue(yoga).
		WithOrganizer(yogaOrg).
		WithWeeklyOccurrences(anchor, 4).
		Build()
	rs01Base.Source = src(eb, "rs01-yoga-series")
	rs01Base.Image = unsplashImage(0)
	rs01Base.URL = fmt.Sprintf("%s/e/rs01-yoga-series", eb.BaseURL)
	rs01Base.License = "https://creativecommons.org/publicdomain/zero/1.0/"

	// New occurrence is on the SAME date as week 1 (anchor), from a different
	// scraper (Lu.ma). Same venue + same date + name similarity ~0.65 → near-dup
	// detection fires → companion pair created → merge duplicate path.
	rs01NewOcc := events.EventInput{
		Name:        "RS-01 Weekly Yoga",
		Description: "Weekly yoga session at The Tranzac. Drop-in friendly, all levels welcome.",
		StartDate:   anchor.Format(time.RFC3339),
		EndDate:     anchor.Add(90 * time.Minute).Format(time.RFC3339),
		Location:    loc(yoga),
		Organizer:   org(yogaOrg),
		Source:      src(luma, "rs01-yoga-occ5"),
		Image:       unsplashImage(0),
		URL:         fmt.Sprintf("%s/e/rs01-yoga-occ5", luma.BaseURL),
		Keywords:    []string{"yoga", "wellness", "monday", "toronto"},
		License:     "https://creativecommons.org/publicdomain/zero/1.0/",
	}

	// -----------------------------------------------------------------------
	// RS-02: Book Club — Near-dup path (companion reviews on both sides)
	// -----------------------------------------------------------------------
	rs02Base := g.NewRecurringSeriesBuilder().
		WithName("RS-02 Book Club — Tuesday Evening").
		WithDescription("Monthly book club for lovers of literary fiction. Light snacks provided.").
		WithVenue(bookclub).
		WithOrganizer(bookOrg).
		WithWeeklyOccurrences(anchor.Add(2*24*time.Hour), 2). // Tuesday anchor
		Build()
	rs02Base.Source = src(mu, "rs02-bookclub-series")
	rs02Base.Image = unsplashImage(1)
	rs02Base.URL = fmt.Sprintf("%s/events/rs02-bookclub-series", mu.BaseURL)
	rs02Base.License = "https://creativecommons.org/publicdomain/zero/1.0/"

	// Near-dup: similar name (similarity ~0.63) + same venue + same date → triggers dedup.
	// Occurrence is on the same calendar day as Base's first occurrence (Tuesday) but at a
	// later time (2pm-4pm vs 10am-12pm) so the overlap guard doesn't fire when absorbing
	// this event into Base via add-occurrence.
	rs02NearDupStart := anchor.Add(2*24*time.Hour + 4*time.Hour) // Tuesday 2pm
	rs02NearDupEnd := rs02NearDupStart.Add(2 * time.Hour)        // Tuesday 4pm
	rs02NearDup := events.EventInput{
		Name:        "RS-02 Book Club — Tuesday Night",
		Description: "Book club gathering at Snakes and Lattes — same evening, different listing source.",
		StartDate:   rs02NearDupStart.Format(time.RFC3339),
		EndDate:     rs02NearDupEnd.Format(time.RFC3339),
		Location:    loc(bookclub),
		Organizer:   org(bookOrg),
		Source:      src(blogto, "rs02-bookclub-neardup"),
		Image:       unsplashImage(1),
		URL:         fmt.Sprintf("%s/e/rs02-bookclub-neardup", blogto.BaseURL),
		Keywords:    []string{"book club", "reading", "social", "toronto"},
		License:     "https://creativecommons.org/publicdomain/zero/1.0/",
	}

	// -----------------------------------------------------------------------
	// RS-03: Tech Meetup — Pending series lifecycle
	// -----------------------------------------------------------------------
	rs03Base := g.NewRecurringSeriesBuilder().
		WithName("RS-03 Tech Meetup — Pending Series").
		WithDescription("Bi-weekly tech meetup for Go and Rust developers. Lightning talks + networking.").
		WithVenue(techMeetup).
		WithOrganizer(techOrg).
		WithWeeklyOccurrences(anchor.Add(1*24*time.Hour), 2). // Tuesday anchor
		Build()
	rs03Base.Source = src(mu, "rs03-tech-series")
	rs03Base.Image = unsplashImage(2)
	rs03Base.URL = fmt.Sprintf("%s/events/rs03-tech-series", mu.BaseURL)
	rs03Base.License = "https://creativecommons.org/publicdomain/zero/1.0/"

	rs03AddlOcc := events.EventInput{
		Name:        "RS-03 Tech Meetup — Additional Occurrence",
		Description: "Extra session added mid-cycle. Add-occurrence resolves review; lifecycle stays pending.",
		StartDate:   anchor.Add(3 * week).Add(1 * 24 * time.Hour).Format(time.RFC3339),
		EndDate:     anchor.Add(3 * week).Add(1*24*time.Hour + 2*time.Hour).Format(time.RFC3339),
		Location:    loc(techMeetup),
		Organizer:   org(techOrg),
		Source:      src(luma, "rs03-tech-addl"),
		Image:       unsplashImage(2),
		URL:         fmt.Sprintf("%s/e/rs03-tech-addl", luma.BaseURL),
		Keywords:    []string{"tech", "go", "rust", "meetup", "toronto"},
		License:     "https://creativecommons.org/publicdomain/zero/1.0/",
	}

	// -----------------------------------------------------------------------
	// RS-04: Art Walk — Draft series (add-occurrence on draft target)
	// -----------------------------------------------------------------------
	rs04Base := g.NewRecurringSeriesBuilder().
		WithName("RS-04 Art Walk — Draft Series").
		WithDescription("Self-guided art walk through the Garrison neighbourhood. Free admission.").
		WithVenue(artWalk).
		WithOrganizer(artOrg).
		WithWeeklyOccurrences(anchor.Add(5*24*time.Hour), 2). // Saturday anchor
		Build()
	rs04Base.Source = src(eb, "rs04-artwalk-series")
	rs04Base.Image = unsplashImage(3)
	rs04Base.URL = fmt.Sprintf("%s/e/rs04-artwalk-series", eb.BaseURL)
	rs04Base.License = "https://creativecommons.org/publicdomain/zero/1.0/"

	rs04NewOcc := events.EventInput{
		Name:        "RS-04 Art Walk — New Occurrence",
		Description: "Additional art walk session added to draft series.",
		StartDate:   anchor.Add(3 * week).Add(5 * 24 * time.Hour).Format(time.RFC3339),
		EndDate:     anchor.Add(3 * week).Add(5*24*time.Hour + 3*time.Hour).Format(time.RFC3339),
		Location:    loc(artWalk),
		Organizer:   org(artOrg),
		Source:      src(sp, "rs04-artwalk-newocc"),
		Image:       unsplashImage(3),
		URL:         fmt.Sprintf("%s/e/rs04-artwalk-newocc", sp.BaseURL),
		Keywords:    []string{"art", "walk", "gallery", "toronto"},
		License:     "https://creativecommons.org/publicdomain/zero/1.0/",
	}

	// -----------------------------------------------------------------------
	// RS-05: Workshop — Overlapping occurrence (add-occurrence → 409 Conflict)
	// -----------------------------------------------------------------------
	// Target series: occurrences at anchor+Wednesday, anchor+Wednesday+1week
	wsAnchor := anchor.Add(3 * 24 * time.Hour) // Wednesday
	rs05Base := g.NewRecurringSeriesBuilder().
		WithName("RS-05 Workshop — Overlap Target").
		WithDescription("Hands-on weekend workshop series. Laptop required. Beginners welcome.").
		WithVenue(workshop).
		WithOrganizer(wsOrg).
		WithWeeklyOccurrences(wsAnchor, 2).
		Build()
	rs05Base.Source = src(luma, "rs05-workshop-series")
	rs05Base.Image = unsplashImage(4)
	rs05Base.URL = fmt.Sprintf("%s/e/rs05-workshop-series", luma.BaseURL)
	rs05Base.License = "https://creativecommons.org/publicdomain/zero/1.0/"

	// Overlapping occurrence: starts 30 minutes into the first existing occurrence
	overlapStart := wsAnchor.Add(30 * time.Minute)
	overlapEnd := overlapStart.Add(2 * time.Hour)
	rs05Overlap := events.EventInput{
		Name:        "RS-05 Workshop — Overlapping Occurrence",
		Description: "A second listing of the same workshop session — overlaps existing occurrence.",
		StartDate:   overlapStart.Format(time.RFC3339),
		EndDate:     overlapEnd.Format(time.RFC3339),
		Location:    loc(workshop),
		Organizer:   org(wsOrg),
		Source:      src(mu, "rs05-workshop-overlap"),
		Image:       unsplashImage(4),
		URL:         fmt.Sprintf("%s/events/rs05-workshop-overlap", mu.BaseURL),
		Keywords:    []string{"workshop", "learning", "toronto"},
		License:     "https://creativecommons.org/publicdomain/zero/1.0/",
	}

	// -----------------------------------------------------------------------
	// RS-06: Jazz Night — Reversed dates + potential duplicate warning
	// -----------------------------------------------------------------------
	// 11pm start, 2am "end" on same calendar date → reversed dates
	jazzDate := anchor.Add(4 * 24 * time.Hour) // Friday
	jazzStart := time.Date(jazzDate.Year(), jazzDate.Month(), jazzDate.Day(), 23, 0, 0, 0, jazzDate.Location())
	jazzEnd := time.Date(jazzDate.Year(), jazzDate.Month(), jazzDate.Day(), 2, 0, 0, 0, jazzDate.Location())
	rs06Jazz := events.EventInput{
		Name:        "RS-06 Jazz Night — Reversed Dates Late Show",
		Description: "Late-night jazz at Dovercourt House. Sets from 11pm until 2am. Cash bar. 19+.",
		StartDate:   jazzStart.Format(time.RFC3339),
		EndDate:     jazzEnd.Format(time.RFC3339), // reversed: 2am before 11pm
		Location:    loc(jazz),
		Organizer:   org(jazzOrg),
		Source:      src(eb, "rs06-jazz-lateshow"),
		Image:       unsplashImage(5),
		URL:         fmt.Sprintf("%s/e/rs06-jazz-lateshow", eb.BaseURL),
		Keywords:    []string{"jazz", "live music", "late night", "toronto"},
		License:     "https://creativecommons.org/publicdomain/zero/1.0/",
	}

	// -----------------------------------------------------------------------
	// RS-07: Dance Class — Not-a-duplicate (approve with record_not_duplicates)
	// -----------------------------------------------------------------------
	rs07Base := g.NewRecurringSeriesBuilder().
		WithName("RS-07 Dance Class — Wednesday Series").
		WithDescription("Contemporary dance classes for adults. No experience required.").
		WithVenue(dance).
		WithOrganizer(danceOrg).
		WithWeeklyOccurrences(anchor.Add(2*24*time.Hour), 3). // Wednesday anchor
		Build()
	rs07Base.Source = src(eb, "rs07-dance-series")
	rs07Base.Image = unsplashImage(6)
	rs07Base.URL = fmt.Sprintf("%s/e/rs07-dance-series", eb.BaseURL)
	rs07Base.License = "https://creativecommons.org/publicdomain/zero/1.0/"

	// Not-a-duplicate: similar name (similarity ~0.71) + same venue + same day → triggers dedup,
	// but admin should approve with record_not_duplicates (it's a social dance, not the class).
	rs07NotDup := events.EventInput{
		Name:        "RS-07 Dance Class — Wednesday Social",
		Description: "A different dance event at The Baby G — social dancing, not the structured class series.",
		StartDate:   anchor.Add(2*24*time.Hour + 4*time.Hour).Format(time.RFC3339), // same day, later time
		EndDate:     anchor.Add(2*24*time.Hour + 6*time.Hour).Format(time.RFC3339),
		Location:    loc(dance),
		Organizer:   org(danceOrg),
		Source:      src(luma, "rs07-dance-notdup"),
		Image:       unsplashImage(6),
		URL:         fmt.Sprintf("%s/e/rs07-dance-notdup", luma.BaseURL),
		Keywords:    []string{"dance", "social", "evening", "toronto"},
		License:     "https://creativecommons.org/publicdomain/zero/1.0/",
	}

	// -----------------------------------------------------------------------
	// RS-08: Community Potluck — Exact duplicate (merge → soft-delete source)
	// -----------------------------------------------------------------------
	potluckStart := anchor.Add(6 * 24 * time.Hour) // Sunday
	potluckEnd := potluckStart.Add(3 * time.Hour)
	rs08Original := events.EventInput{
		Name:        "RS-08 Community Potluck — Original",
		Description: "Bring a dish to share! Community gathering at InterAccess. Family-friendly.",
		StartDate:   potluckStart.Format(time.RFC3339),
		EndDate:     potluckEnd.Format(time.RFC3339),
		Location:    loc(potluck),
		Organizer:   org(potluckOrg),
		Source:      src(mu, "rs08-potluck-original"),
		Image:       unsplashImage(7),
		URL:         fmt.Sprintf("%s/events/rs08-potluck-original", mu.BaseURL),
		Keywords:    []string{"potluck", "community", "food", "toronto"},
		License:     "https://creativecommons.org/publicdomain/zero/1.0/",
	}
	rs08ExactDup := events.EventInput{
		Name:        "RS-08 Community Potluck — Exact Duplicate",
		Description: "Bring a dish to share! Community gathering at InterAccess. Family-friendly.",
		StartDate:   potluckStart.Format(time.RFC3339),
		EndDate:     potluckEnd.Format(time.RFC3339),
		Location:    loc(potluck),
		Organizer:   org(potluckOrg),
		Source:      src(blogto, "rs08-potluck-exactdup"),
		Image:       unsplashImage(7),
		URL:         fmt.Sprintf("%s/e/rs08-potluck-exactdup", blogto.BaseURL),
		Keywords:    []string{"potluck", "community", "food", "toronto"},
		License:     "https://creativecommons.org/publicdomain/zero/1.0/",
	}

	// -----------------------------------------------------------------------
	// RS-09: Film Screening — Multi-session warning
	// -----------------------------------------------------------------------
	filmStart := anchor.Add(1 * 24 * time.Hour) // Tuesday
	filmEnd := filmStart.Add(6 * time.Hour)     // Long duration (6h) for 8 sessions
	rs09Film := events.EventInput{
		Name:        "RS-09 Film Screening (8 sessions) — Multi-Session",
		Description: "Eight consecutive short-film screenings with Q&A breaks between each session. Doors at 10am.",
		StartDate:   filmStart.Format(time.RFC3339),
		EndDate:     filmEnd.Format(time.RFC3339),
		Location:    loc(film),
		Organizer:   org(filmOrg),
		Source:      src(eb, "rs09-film-multisession"),
		Image:       unsplashImage(8),
		URL:         fmt.Sprintf("%s/e/rs09-film-multisession", eb.BaseURL),
		Keywords:    []string{"film", "screening", "cinema", "toronto"},
		License:     "https://creativecommons.org/publicdomain/zero/1.0/",
	}

	// -----------------------------------------------------------------------
	// RS-10: Choir Rehearsal — Order-independent consolidation pair
	// -----------------------------------------------------------------------
	choirAStart := anchor.Add(3 * 24 * time.Hour) // Wednesday 10am
	choirAEnd := choirAStart.Add(2 * time.Hour)   // Wednesday 12pm
	rs10SourceA := events.EventInput{
		Name:        "RS-10 Choir Rehearsal — Source A",
		Description: "Weekly choir rehearsal at Bampot Tea House. All voice types welcome.",
		StartDate:   choirAStart.Format(time.RFC3339),
		EndDate:     choirAEnd.Format(time.RFC3339),
		Location:    loc(choir),
		Organizer:   org(choirOrg),
		Source:      src(gcal, "rs10-choir-sourcea"),
		Image:       unsplashImage(9),
		URL:         fmt.Sprintf("%s/e/rs10-choir-sourcea", gcal.BaseURL),
		Keywords:    []string{"choir", "singing", "rehearsal", "toronto"},
		License:     "https://creativecommons.org/publicdomain/zero/1.0/",
	}
	// Source B: same calendar date as Source A (dedup requires same date + same venue + similarity > 0.4).
	// Occurrence is at 2pm-4pm on the same Wednesday — non-overlapping with Source A's 10am-12pm slot,
	// so add-occurrence can absorb Source B into Source A without a 409 Conflict.
	// Similarity between "RS-10 Choir Rehearsal — Source A" and "RS-10 Choir Rehearsal — Source B"
	// is ~0.875, well above the 0.4 threshold.
	choirBStart := anchor.Add(3*24*time.Hour + 4*time.Hour) // Wednesday 2pm
	choirBEnd := choirBStart.Add(2 * time.Hour)             // Wednesday 4pm
	rs10SourceB := events.EventInput{
		Name:        "RS-10 Choir Rehearsal — Source B",
		Description: "Choir rehearsal — same ensemble, listed from a second source.",
		StartDate:   choirBStart.Format(time.RFC3339),
		EndDate:     choirBEnd.Format(time.RFC3339),
		Location:    loc(choir),
		Organizer:   org(choirOrg),
		Source:      src(luma, "rs10-choir-sourceb"),
		Image:       unsplashImage(9),
		URL:         fmt.Sprintf("%s/e/rs10-choir-sourceb", luma.BaseURL),
		Keywords:    []string{"choir", "singing", "rehearsal", "toronto"},
		License:     "https://creativecommons.org/publicdomain/zero/1.0/",
	}

	// -----------------------------------------------------------------------
	// RS-11: Pottery Studio — Same-day-different-times cluster (4 events)
	// -----------------------------------------------------------------------
	mon10am := anchor                        // Monday 10am
	mon10amEnd := mon10am.Add(2 * time.Hour) // Monday 12pm
	mon2pm := anchor.Add(4 * time.Hour)      // Monday 2pm
	mon2pmEnd := mon2pm.Add(2 * time.Hour)   // Monday 4pm
	mon7_10am := mon10am.Add(week)           // Next Monday 10am
	mon7_10amEnd := mon7_10am.Add(2 * time.Hour)
	mon7_2pm := mon2pm.Add(week) // Next Monday 2pm
	mon7_2pmEnd := mon7_2pm.Add(2 * time.Hour)

	rs11Mon10am := events.EventInput{
		Name:        "RS-11 Pottery Studio — Mon 10am Session",
		Description: "Hand-building pottery session. All clay and tools provided. Beginners welcome.",
		StartDate:   mon10am.Format(time.RFC3339),
		EndDate:     mon10amEnd.Format(time.RFC3339),
		Location:    loc(pottery),
		Organizer:   org(potteryOrg),
		Source:      src(eb, "rs11-pottery-mon10am"),
		Image:       unsplashImage(10),
		URL:         fmt.Sprintf("%s/e/rs11-pottery-mon10am", eb.BaseURL),
		Keywords:    []string{"pottery", "ceramic", "workshop", "toronto"},
		License:     "https://creativecommons.org/publicdomain/zero/1.0/",
	}
	rs11Mon2pm := events.EventInput{
		Name:        "RS-11 Pottery Studio — Mon 2pm Session",
		Description: "Afternoon hand-building pottery session. All clay and tools provided.",
		StartDate:   mon2pm.Format(time.RFC3339),
		EndDate:     mon2pmEnd.Format(time.RFC3339),
		Location:    loc(pottery),
		Organizer:   org(potteryOrg),
		Source:      src(eb, "rs11-pottery-mon2pm"),
		Image:       unsplashImage(10),
		URL:         fmt.Sprintf("%s/e/rs11-pottery-mon2pm", eb.BaseURL),
		Keywords:    []string{"pottery", "ceramic", "workshop", "toronto"},
		License:     "https://creativecommons.org/publicdomain/zero/1.0/",
	}
	rs11Mon7_10am := events.EventInput{
		Name:        "RS-11 Pottery Studio — Mon+7 10am Session",
		Description: "Hand-building pottery session, next Monday morning. All clay and tools provided.",
		StartDate:   mon7_10am.Format(time.RFC3339),
		EndDate:     mon7_10amEnd.Format(time.RFC3339),
		Location:    loc(pottery),
		Organizer:   org(potteryOrg),
		Source:      src(eb, "rs11-pottery-mon7-10am"),
		Image:       unsplashImage(10),
		URL:         fmt.Sprintf("%s/e/rs11-pottery-mon7-10am", eb.BaseURL),
		Keywords:    []string{"pottery", "ceramic", "workshop", "toronto"},
		License:     "https://creativecommons.org/publicdomain/zero/1.0/",
	}
	rs11Mon7_2pm := events.EventInput{
		Name:        "RS-11 Pottery Studio — Mon+7 2pm Session",
		Description: "Afternoon pottery session, next Monday. All clay and tools provided.",
		StartDate:   mon7_2pm.Format(time.RFC3339),
		EndDate:     mon7_2pmEnd.Format(time.RFC3339),
		Location:    loc(pottery),
		Organizer:   org(potteryOrg),
		Source:      src(eb, "rs11-pottery-mon7-2pm"),
		Image:       unsplashImage(10),
		URL:         fmt.Sprintf("%s/e/rs11-pottery-mon7-2pm", eb.BaseURL),
		Keywords:    []string{"pottery", "ceramic", "workshop", "toronto"},
		License:     "https://creativecommons.org/publicdomain/zero/1.0/",
	}

	return []ReviewEventScenario{
		{
			GroupID:     "RS-01",
			Description: "Near-dup + multi-session: same session from two scrapers, same date → merge duplicate",
			Events:      []events.EventInput{rs01Base, rs01NewOcc},
		},
		{
			GroupID:     "RS-02",
			Description: "Near-dup path: companion reviews on both sides",
			Events:      []events.EventInput{rs02Base, rs02NearDup},
		},
		{
			GroupID:     "RS-03",
			Description: "Lifecycle-stays-pending: add-occurrence on pending series",
			Events:      []events.EventInput{rs03Base, rs03AddlOcc},
		},
		{
			GroupID:     "RS-04",
			Description: "Draft-state add-occurrence: forward path on draft target",
			Events:      []events.EventInput{rs04Base, rs04NewOcc},
		},
		{
			GroupID:     "RS-05",
			Description: "add-occurrence conflict: overlapping occurrence → 409 Conflict",
			Events:      []events.EventInput{rs05Base, rs05Overlap},
		},
		{
			GroupID:     "RS-06",
			Description: "Multi-warning: reversed_dates + potential_duplicate",
			Events:      []events.EventInput{rs06Jazz},
		},
		{
			GroupID:     "RS-07",
			Description: "Not-a-duplicate: approve with record_not_duplicates",
			Events:      []events.EventInput{rs07Base, rs07NotDup},
		},
		{
			GroupID:     "RS-08",
			Description: "Exact duplicate: merge → soft-delete source",
			Events:      []events.EventInput{rs08Original, rs08ExactDup},
		},
		{
			GroupID:     "RS-09",
			Description: "Multi-session: multi_session_likely warning",
			Events:      []events.EventInput{rs09Film},
		},
		{
			GroupID:     "RS-10",
			Description: "Order-independent consolidation pair",
			Events:      []events.EventInput{rs10SourceA, rs10SourceB},
		},
		{
			GroupID:     "RS-11",
			Description: "Same-day-different-times cluster: 4 events",
			Events:      []events.EventInput{rs11Mon10am, rs11Mon2pm, rs11Mon7_10am, rs11Mon7_2pm},
		},
	}
}

// SingleOccurrenceMatch creates a single-occurrence event that is a duplicate of one
// occurrence from a recurring series. This is used to test add-occurrence workflows.
//
// Parameters:
//   - series: A recurring-series EventInput (typically built with RecurringSeriesBuilder)
//   - occIdx: Index of the occurrence to duplicate (0-based)
//
// Returns: An EventInput with one occurrence matching the series occurrence at occIdx
func SingleOccurrenceMatch(series events.EventInput, occIdx int) events.EventInput {
	if occIdx < 0 || occIdx >= len(series.Occurrences) {
		// Fallback: use first occurrence if index out of bounds
		occIdx = 0
	}

	selectedOcc := series.Occurrences[occIdx]

	return events.EventInput{
		Name:        series.Name + " (single)",
		Description: series.Description,
		StartDate:   selectedOcc.StartDate,
		EndDate:     selectedOcc.EndDate,
		Location:    series.Location,
		Organizer: &events.OrganizationInput{
			Name: series.Organizer.Name + " (variant)",
			URL:  series.Organizer.URL,
		},
		Source: &events.SourceInput{
			URL:     series.Source.URL + "/single",
			EventID: series.Source.EventID + "-single",
			Name:    series.Source.Name,
		},
		Image:    series.Image,
		URL:      series.URL + "/single",
		Keywords: series.Keywords,
		License:  series.License,
		// No occurrences — single occurrence event
	}
}
