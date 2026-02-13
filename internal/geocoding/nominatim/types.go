package nominatim

// SearchOptions contains optional parameters for geocoding searches.
type SearchOptions struct {
	// CountryCodes limits results to specific countries (comma-separated ISO 3166-1 alpha-2 codes, e.g. "ca,us")
	CountryCodes string
	// Limit controls the maximum number of results (default: 1, max: 50)
	Limit int
	// Viewbox biases results toward a specific geographic bounding box
	Viewbox *Viewbox
}

// Viewbox defines a geographic bounding box for biasing search results.
type Viewbox struct {
	MinLat float64 // Southwest corner latitude
	MinLon float64 // Southwest corner longitude
	MaxLat float64 // Northeast corner latitude
	MaxLon float64 // Northeast corner longitude
}

// SearchResult represents a single geocoding result from Nominatim search endpoint (format=jsonv2).
type SearchResult struct {
	PlaceID     int64   `json:"place_id"`
	Lat         string  `json:"lat"`
	Lon         string  `json:"lon"`
	DisplayName string  `json:"display_name"`
	Type        string  `json:"type"`
	Class       string  `json:"class"`
	Importance  float64 `json:"importance"`
	OSMID       int64   `json:"osm_id"`
	OSMType     string  `json:"osm_type"`
	// Address contains structured address components if included
	Address *Address `json:"address,omitempty"`
}

// ReverseResult represents a reverse geocoding result (coordinates -> address).
type ReverseResult struct {
	PlaceID     int64   `json:"place_id"`
	Lat         string  `json:"lat"`
	Lon         string  `json:"lon"`
	DisplayName string  `json:"display_name"`
	Type        string  `json:"type"`
	Class       string  `json:"class"`
	OSMID       int64   `json:"osm_id"`
	OSMType     string  `json:"osm_type"`
	Address     Address `json:"address"`
}

// Address contains structured address components from Nominatim.
type Address struct {
	Road        string `json:"road,omitempty"`
	Suburb      string `json:"suburb,omitempty"`
	City        string `json:"city,omitempty"`
	County      string `json:"county,omitempty"`
	State       string `json:"state,omitempty"`
	Postcode    string `json:"postcode,omitempty"`
	Country     string `json:"country,omitempty"`
	CountryCode string `json:"country_code,omitempty"`
}
