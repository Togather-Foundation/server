// Package schema provides typed Go structs for schema.org JSON-LD output.
// These replace hand-built map[string]any in API handlers, giving compile-time
// safety and consistent JSON serialization for schema.org entities.
package schema

// Event represents a full schema.org Event for JSON-LD output.
type Event struct {
	Context             any            `json:"@context,omitempty"`
	Type                string         `json:"@type"`
	ID                  string         `json:"@id,omitempty"`
	Name                string         `json:"name"`
	Description         string         `json:"description,omitempty"`
	StartDate           string         `json:"startDate,omitempty"`
	EndDate             string         `json:"endDate,omitempty"`
	DoorTime            string         `json:"doorTime,omitempty"`
	Location            any            `json:"location,omitempty"`
	Organizer           any            `json:"organizer,omitempty"`
	Image               string         `json:"image,omitempty"`
	URL                 string         `json:"url,omitempty"`
	Keywords            []string       `json:"keywords,omitempty"`
	InLanguage          []string       `json:"inLanguage,omitempty"`
	IsAccessibleForFree *bool          `json:"isAccessibleForFree,omitempty"`
	EventStatus         string         `json:"eventStatus,omitempty"`
	EventAttendanceMode string         `json:"eventAttendanceMode,omitempty"`
	Offers              []Offer        `json:"offers,omitempty"`
	License             string         `json:"license,omitempty"`
	SameAs              []string       `json:"sameAs,omitempty"`
	SubEvents           []EventSummary `json:"subEvent,omitempty"`
}

// NewEvent creates an Event with @type pre-set.
func NewEvent(name string) *Event {
	return &Event{
		Type: "Event",
		Name: name,
	}
}

// EventSummary is a compact event representation for lists and sub-events.
type EventSummary struct {
	Context   any    `json:"@context,omitempty"`
	Type      string `json:"@type"`
	ID        string `json:"@id,omitempty"`
	Name      string `json:"name"`
	StartDate string `json:"startDate,omitempty"`
	EndDate   string `json:"endDate,omitempty"`
	Location  any    `json:"location,omitempty"`
}

// NewEventSummary creates an EventSummary with @type pre-set.
func NewEventSummary(name string) *EventSummary {
	return &EventSummary{
		Type: "Event",
		Name: name,
	}
}

// Place represents a full schema.org Place for JSON-LD output.
type Place struct {
	Context                 any             `json:"@context,omitempty"`
	Type                    string          `json:"@type"`
	ID                      string          `json:"@id,omitempty"`
	Name                    string          `json:"name"`
	Description             string          `json:"description,omitempty"`
	Address                 *PostalAddress  `json:"address,omitempty"`
	Geo                     *GeoCoordinates `json:"geo,omitempty"`
	Telephone               string          `json:"telephone,omitempty"`
	Email                   string          `json:"email,omitempty"`
	URL                     string          `json:"url,omitempty"`
	MaximumAttendeeCapacity int             `json:"maximumAttendeeCapacity,omitempty"`
	DistanceKm              *float64        `json:"sel:distanceKm,omitempty"` // Custom SEL field for proximity search
}

// NewPlace creates a Place with @type pre-set.
func NewPlace(name string) *Place {
	return &Place{
		Type: "Place",
		Name: name,
	}
}

// PostalAddress represents a schema.org PostalAddress.
type PostalAddress struct {
	Type            string `json:"@type"`
	StreetAddress   string `json:"streetAddress,omitempty"`
	AddressLocality string `json:"addressLocality,omitempty"`
	AddressRegion   string `json:"addressRegion,omitempty"`
	PostalCode      string `json:"postalCode,omitempty"`
	AddressCountry  string `json:"addressCountry,omitempty"`
}

// GeoCoordinates represents a schema.org GeoCoordinates.
// Latitude and Longitude use pointers to distinguish "not set" from zero,
// since lat=0/lng=0 is a valid location (Gulf of Guinea).
type GeoCoordinates struct {
	Type      string   `json:"@type"`
	Latitude  *float64 `json:"latitude"`
	Longitude *float64 `json:"longitude"`
}

// Organization represents a full schema.org Organization for JSON-LD output.
type Organization struct {
	Context     any            `json:"@context,omitempty"`
	Type        string         `json:"@type"`
	ID          string         `json:"@id,omitempty"`
	Name        string         `json:"name"`
	LegalName   string         `json:"legalName,omitempty"`
	Description string         `json:"description,omitempty"`
	URL         string         `json:"url,omitempty"`
	Email       string         `json:"email,omitempty"`
	Telephone   string         `json:"telephone,omitempty"`
	Address     *PostalAddress `json:"address,omitempty"`
}

// NewOrganization creates an Organization with @type pre-set.
func NewOrganization(name string) *Organization {
	return &Organization{
		Type: "Organization",
		Name: name,
	}
}

// VirtualLocation represents a schema.org VirtualLocation.
type VirtualLocation struct {
	Type string `json:"@type"`
	URL  string `json:"url"`
	Name string `json:"name,omitempty"`
}

// NewVirtualLocation creates a VirtualLocation with @type pre-set.
func NewVirtualLocation(url string) *VirtualLocation {
	return &VirtualLocation{
		Type: "VirtualLocation",
		URL:  url,
	}
}

// Offer represents a schema.org Offer.
// Price uses a pointer to distinguish "not set" from zero (free).
type Offer struct {
	Type          string   `json:"@type"`
	URL           string   `json:"url,omitempty"`
	Price         *float64 `json:"price,omitempty"`
	PriceCurrency string   `json:"priceCurrency,omitempty"`
	Availability  string   `json:"availability,omitempty"`
}

// NewOffer creates an Offer with @type pre-set.
func NewOffer() *Offer {
	return &Offer{
		Type: "Offer",
	}
}

// ListResponse is a generic schema.org ItemList envelope for paginated responses.
type ListResponse struct {
	Context    any    `json:"@context,omitempty"`
	Type       string `json:"@type"`
	Items      any    `json:"itemListElement"`
	NextCursor string `json:"nextCursor,omitempty"`
	TotalItems int    `json:"totalItems,omitempty"`
}

// NewListResponse creates a ListResponse with @type pre-set.
func NewListResponse(items any) *ListResponse {
	return &ListResponse{
		Type:  "ItemList",
		Items: items,
	}
}
