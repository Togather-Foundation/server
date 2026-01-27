// Package testdata provides synthetic event fixtures for testing the ingestion pipeline.
// These fixtures are based on real scraper field mappings from event_mocks.csv.
package testdata

import (
	"fmt"
	"math/rand"
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

	return fmt.Sprintf(template, subject, venue.Name)
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
