package ical

import (
	"regexp"
	"testing"
)

func TestIsVirtualDescription(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		desc string
		want bool
	}{
		{"zoom link", "Join our Zoom meeting at https://zoom.us/j/123", true},
		{"virtual event", "This is a virtual event", true},
		{"online only", "This event is online only", true},
		{"webinar", "Join the webinar", true},
		{"livestream", "Watch the livestream at 7pm", true},
		{"google meet", "Join us on Google Meet", true},
		{"microsoft teams", "Connect via Microsoft Teams", true},
		{"teams meeting", "Teams meeting link below", true},
		{"zoom meeting", "Zoom meeting details", true},
		{"https url not virtual on its own", "Join at https://meet.google.com/abc", false},
		{"http url not virtual on its own", "See http://example.com for details", false},
		{"live stream space", "Live stream available", true},
		{"mixed case", "Join the VIRTUAL event via ZOOM", true},
		{"empty string", "", false},
		{"no match", "Meet at the park", false},
		{"physical only", "Location: 123 Main St, Toronto", false},
		{"no virtual signals at all", "Meet us in person at the park downtown", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := IsVirtualDescription(tt.desc)
			if got != tt.want {
				t.Errorf("IsVirtualDescription(%q) = %v, want %v", tt.desc, got, tt.want)
			}
		})
	}
}

func TestExtractLocationFromDescription(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		desc   string
		want   string
		wantOK bool
	}{
		{
			name:   "meetup-location-label",
			desc:   "Come join us! Meetup Location: Finch Subway Station near the entrance",
			want:   "Finch Subway Station near the entrance",
			wantOK: true,
		},
		{
			name:   "meetup-point-label",
			desc:   "Details: Meet up point: Union Station Great Hall",
			want:   "Union Station Great Hall",
			wantOK: true,
		},
		{
			name:   "location-label at start of line",
			desc:   "Some text before\nLocation: 123 Main St, Toronto\nMore text after",
			want:   "123 Main St, Toronto",
			wantOK: true,
		},
		{
			name:   "venue-label",
			desc:   "Come to our event!\nVenue: The Great Hall\nSee you there!",
			want:   "The Great Hall",
			wantOK: true,
		},
		{
			name:   "address-label",
			desc:   "Address: 100 King St W, Toronto, ON M5X 1A9",
			want:   "100 King St W, Toronto, ON M5X 1A9",
			wantOK: true,
		},
		{
			name:   "meet-at pattern",
			desc:   "We will meet at Trinity Bellwoods Park main gate at 6pm. Bring snacks.",
			want:   "Trinity Bellwoods Park main gate at 6pm",
			wantOK: true,
		},
		{
			name:   "meet-near pattern",
			desc:   "Let's meet near the CN Tower entrance on Front St. Dress warmly.",
			want:   "the CN Tower entrance on Front St",
			wantOK: true,
		},
		{
			name:   "meet-in-front-of pattern",
			desc:   "Meet in front of the ROM main entrance at 7pm sharp.",
			want:   "the ROM main entrance at 7pm sharp",
			wantOK: true,
		},
		{
			name:   "meet-outside pattern",
			desc:   "Meet outside the AGO on Dundas St.",
			want:   "the AGO on Dundas St",
			wantOK: true,
		},
		{
			name:   "meet-inside pattern",
			desc:   "Meet inside the Eaton Centre food court.",
			want:   "the Eaton Centre food court",
			wantOK: true,
		},
		{
			name:   "starting-point label",
			desc:   "Starting point: High Park main entrance",
			want:   "High Park main entrance",
			wantOK: true,
		},
		{
			name:   "start-location label",
			desc:   "Start location: Nathan Phillips Square",
			want:   "Nathan Phillips Square",
			wantOK: true,
		},
		{
			name:   "first match wins - meetup-location before location-label",
			desc:   "Meetup Location: Downtown Library\nLocation: Not this one",
			want:   "Downtown Library",
			wantOK: true,
		},
		{
			name:   "first-line fallback",
			desc:   "High Park Yoga\n**details**",
			want:   "High Park Yoga",
			wantOK: true,
		},
		{
			name:   "first-line matched but label patterns win",
			desc:   "Community Walk\nLocation: Finch Station\nBring water.",
			want:   "Finch Station",
			wantOK: true,
		},
		{
			name:   "first-line fallback generic",
			desc:   "Come hang out with us!",
			want:   "Come hang out with us!",
			wantOK: true,
		},
		{
			name:   "empty description",
			desc:   "",
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := ExtractLocationFromDescription(tt.desc)
			if ok != tt.wantOK {
				t.Errorf("ExtractLocationFromDescription(%q) ok = %v, want %v", tt.desc, ok, tt.wantOK)
			}
			if ok && got != tt.want {
				t.Errorf("ExtractLocationFromDescription(%q) = %q, want %q", tt.desc, got, tt.want)
			}
		})
	}
}

func TestExtractLocationWithPatterns(t *testing.T) {
	t.Parallel()

	t.Run("custom pattern list", func(t *testing.T) {
		t.Parallel()
		patterns := []LocationPattern{
			{Name: "custom", Re: regexp.MustCompile(`(?i)where\s*:\s*([^\n\r]+)`)},
			{Name: "alt", Re: regexp.MustCompile(`(?i)place\s*:\s*([^\n\r]+)`)},
		}

		got, ok := ExtractLocationWithPatterns("Where: The Pub on King St\nMore info below", patterns)
		if !ok {
			t.Fatal("expected match")
		}
		if got != "The Pub on King St" {
			t.Errorf("got %q, want %q", got, "The Pub on King St")
		}
	})

	t.Run("first match wins", func(t *testing.T) {
		t.Parallel()
		patterns := []LocationPattern{
			{Name: "first", Re: regexp.MustCompile(`(?i)first\s*:\s*([^\n\r]+)`)},
			{Name: "second", Re: regexp.MustCompile(`(?i)second\s*:\s*([^\n\r]+)`)},
		}

		got, ok := ExtractLocationWithPatterns("First: Primary Location\nSecond: Secondary Location", patterns)
		if !ok {
			t.Fatal("expected match")
		}
		if got != "Primary Location" {
			t.Errorf("got %q, want %q", got, "Primary Location")
		}
	})

	t.Run("empty patterns", func(t *testing.T) {
		t.Parallel()
		_, ok := ExtractLocationWithPatterns("Location: Test", nil)
		if ok {
			t.Error("expected no match with empty patterns")
		}
	})

	t.Run("no match with patterns", func(t *testing.T) {
		t.Parallel()
		patterns := []LocationPattern{
			{Name: "test", Re: regexp.MustCompile(`(?i)xyz\s*:\s*([^\n\r]+)`)},
		}
		_, ok := ExtractLocationWithPatterns("No matching pattern here", patterns)
		if ok {
			t.Error("expected no match")
		}
	})

	t.Run("empty group match", func(t *testing.T) {
		t.Parallel()
		patterns := []LocationPattern{
			{Name: "empty", Re: regexp.MustCompile(`(?i)empty\s*:\s*([^\n\r]*)`)},
		}
		_, ok := ExtractLocationWithPatterns("Empty:", patterns)
		if ok {
			t.Error("expected no match for empty extracted text")
		}
	})
}

func TestDecomposeLocation(t *testing.T) {
	t.Parallel()

	t.Run("full address extracts city and region", func(t *testing.T) {
		t.Parallel()
		opts := DecomposeOpts{}
		pi := DecomposeLocation("100 King St W, Toronto, ON M5X 1A9", opts)

		if pi.Name != "100 King St W, Toronto, ON M5X 1A9" {
			t.Errorf("Name = %q, want %q", pi.Name, "100 King St W, Toronto, ON M5X 1A9")
		}
		if pi.StreetAddress == "" {
			t.Error("StreetAddress should not be empty")
		}
		if pi.AddressLocality != "Toronto" {
			t.Errorf("AddressLocality = %q, want %q", pi.AddressLocality, "Toronto")
		}
		if pi.AddressRegion != "ON" {
			t.Errorf("AddressRegion = %q, want %q", pi.AddressRegion, "ON")
		}
	})

	t.Run("meetup escaped comma format", func(t *testing.T) {
		t.Parallel()
		opts := DecomposeOpts{}
		pi := DecomposeLocation("MaRS Discovery District\\, 101 College St\\, Toronto\\, ON", opts)

		if pi.AddressLocality != "Toronto" {
			t.Errorf("AddressLocality = %q, want %q", pi.AddressLocality, "Toronto")
		}
		if pi.AddressRegion != "ON" {
			t.Errorf("AddressRegion = %q, want %q", pi.AddressRegion, "ON")
		}
	})

	t.Run("meetup venue and city only", func(t *testing.T) {
		t.Parallel()
		opts := DecomposeOpts{}
		pi := DecomposeLocation("High Park\\, Toronto\\, ON", opts)

		if pi.AddressLocality != "Toronto" {
			t.Errorf("AddressLocality = %q, want %q", pi.AddressLocality, "Toronto")
		}
		if pi.AddressRegion != "ON" {
			t.Errorf("AddressRegion = %q, want %q", pi.AddressRegion, "ON")
		}
	})

	t.Run("non-toronto city extracted", func(t *testing.T) {
		t.Parallel()
		opts := DecomposeOpts{}
		pi := DecomposeLocation("Dance Studio\\, 123 Main St\\, New York\\, NY 10001", opts)

		if pi.AddressLocality != "New York" {
			t.Errorf("AddressLocality = %q, want %q", pi.AddressLocality, "New York")
		}
		if pi.AddressRegion != "NY" {
			t.Errorf("AddressRegion = %q, want %q", pi.AddressRegion, "NY")
		}
	})

	t.Run("extracted overrides defaults", func(t *testing.T) {
		t.Parallel()
		opts := DecomposeOpts{
			DefaultLocality: "Toronto",
			DefaultRegion:   "ON",
		}
		pi := DecomposeLocation("Venue\\, Montreal\\, QC", opts)

		if pi.AddressLocality != "Montreal" {
			t.Errorf("AddressLocality = %q, want Montreal (extracted overrides default)", pi.AddressLocality)
		}
		if pi.AddressRegion != "QC" {
			t.Errorf("AddressRegion = %q, want QC (extracted overrides default)", pi.AddressRegion)
		}
	})

	t.Run("partial address with defaults", func(t *testing.T) {
		t.Parallel()
		opts := DecomposeOpts{
			DefaultLocality: "Toronto",
			DefaultRegion:   "ON",
			DefaultCountry:  "CA",
		}
		pi := DecomposeLocation("Trinity Bellwoods Park", opts)

		if pi.Name != "Trinity Bellwoods Park" {
			t.Errorf("Name = %q, want %q", pi.Name, "Trinity Bellwoods Park")
		}
		if pi.AddressLocality != "Toronto" {
			t.Errorf("AddressLocality = %q, want %q", pi.AddressLocality, "Toronto")
		}
		if pi.AddressRegion != "ON" {
			t.Errorf("AddressRegion = %q, want %q", pi.AddressRegion, "ON")
		}
		if pi.AddressCountry != "CA" {
			t.Errorf("AddressCountry = %q, want %q", pi.AddressCountry, "CA")
		}
	})

	t.Run("minimal address no extraction", func(t *testing.T) {
		t.Parallel()
		opts := DecomposeOpts{}
		pi := DecomposeLocation("Some Venue", opts)

		if pi.Name != "Some Venue" {
			t.Errorf("Name = %q, want %q", pi.Name, "Some Venue")
		}
		if pi.StreetAddress != "Some Venue" {
			t.Errorf("StreetAddress = %q, want %q", pi.StreetAddress, "Some Venue")
		}
		if pi.AddressLocality != "" {
			t.Errorf("AddressLocality = %q, want empty", pi.AddressLocality)
		}
	})

	t.Run("empty name", func(t *testing.T) {
		t.Parallel()
		opts := DecomposeOpts{
			DefaultLocality: "Toronto",
			DefaultRegion:   "ON",
			DefaultCountry:  "CA",
		}
		pi := DecomposeLocation("", opts)

		if pi.Name != "" {
			t.Errorf("Name = %q, want empty", pi.Name)
		}
		if pi.StreetAddress != "" {
			t.Errorf("StreetAddress = %q, want empty", pi.StreetAddress)
		}
		if pi.AddressLocality != "Toronto" {
			t.Errorf("AddressLocality = %q, want %q", pi.AddressLocality, "Toronto")
		}
		if pi.AddressRegion != "ON" {
			t.Errorf("AddressRegion = %q, want %q", pi.AddressRegion, "ON")
		}
		if pi.AddressCountry != "CA" {
			t.Errorf("AddressCountry = %q, want %q", pi.AddressCountry, "CA")
		}
	})

	t.Run("default country applied", func(t *testing.T) {
		t.Parallel()
		opts := DecomposeOpts{
			DefaultCountry: "CA",
		}
		pi := DecomposeLocation("Some Venue", opts)

		if pi.AddressCountry != "CA" {
			t.Errorf("AddressCountry = %q, want %q", pi.AddressCountry, "CA")
		}
	})
}

func TestExtractAddressComponents(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		raw          string
		wantLocality string
		wantRegion   string
	}{
		{
			name:         "meetup escaped comma full address",
			raw:          "MaRS Discovery District\\, 101 College St\\, Toronto\\, ON",
			wantLocality: "Toronto",
			wantRegion:   "ON",
		},
		{
			name:         "meetup escaped comma venue and city",
			raw:          "High Park\\, Toronto\\, ON",
			wantLocality: "Toronto",
			wantRegion:   "ON",
		},
		{
			name:         "meetup with postal code",
			raw:          "100 King St W\\, Toronto\\, ON M5X 1A9",
			wantLocality: "Toronto",
			wantRegion:   "ON",
		},
		{
			name:         "standard comma address",
			raw:          "123 Main St, Toronto, ON",
			wantLocality: "Toronto",
			wantRegion:   "ON",
		},
		{
			name:         "non-toronto city",
			raw:          "Dance Studio\\, 123 Main St\\, New York\\, NY 10001",
			wantLocality: "New York",
			wantRegion:   "NY",
		},
		{
			name:         "montreal",
			raw:          "Venue\\, Montreal\\, QC",
			wantLocality: "Montreal",
			wantRegion:   "QC",
		},
		{
			name:         "vancouver",
			raw:          "Studio, Vancouver, BC V6B 1A1",
			wantLocality: "Vancouver",
			wantRegion:   "BC",
		},
		{
			name:         "simple venue no comma",
			raw:          "Toronto Reference Library",
			wantLocality: "",
			wantRegion:   "",
		},
		{
			name:         "single comma only",
			raw:          "Venue, Toronto",
			wantLocality: "",
			wantRegion:   "",
		},
		{
			name:         "empty string",
			raw:          "",
			wantLocality: "",
			wantRegion:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			loc, reg := extractAddressComponents(tt.raw)
			if loc != tt.wantLocality {
				t.Errorf("locality = %q, want %q", loc, tt.wantLocality)
			}
			if reg != tt.wantRegion {
				t.Errorf("region = %q, want %q", reg, tt.wantRegion)
			}
		})
	}
}

func TestDefaultLocationPatterns(t *testing.T) {
	t.Parallel()

	patterns := DefaultLocationPatterns()
	if len(patterns) != 9 {
		t.Errorf("DefaultLocationPatterns length = %d, want 9", len(patterns))
	}

	names := make(map[string]bool)
	for _, p := range patterns {
		if p.Re == nil {
			t.Errorf("pattern %q has nil regexp", p.Name)
		}
		if names[p.Name] {
			t.Errorf("duplicate pattern name: %q", p.Name)
		}
		names[p.Name] = true
	}

	expectedNames := []string{
		"meetup-location-label",
		"meetup-point-label",
		"location-label",
		"venue-label",
		"address-label",
		"meet-at-near",
		"starting-point",
		"start-location",
		"first-line",
	}
	for _, name := range expectedNames {
		if !names[name] {
			t.Errorf("expected pattern %q not found", name)
		}
	}

	p1 := DefaultLocationPatterns()
	if &patterns[0] != &p1[0] {
		t.Error("DefaultLocationPatterns should return the same slice (sync.Once)")
	}
}
