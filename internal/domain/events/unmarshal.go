package events

import (
	"encoding/json"
	"fmt"
	"strings"
)

// UnmarshalJSON implements custom JSON unmarshaling for EventInput to handle
// schema.org compatibility:
//   - location as text string (creates PlaceInput with Name)
//   - image as ImageObject (extracts url field)
//   - organizer as string URI (creates OrganizationInput with ID)
//   - inLanguage as single string (wraps in array)
//   - keywords as comma-separated string (splits into array)
//   - @context field (accepted but ignored)
func (e *EventInput) UnmarshalJSON(data []byte) error {
	// Use a raw intermediate to handle flexible fields
	type rawEvent struct {
		Context             json.RawMessage       `json:"@context,omitempty"`
		ID                  string                `json:"@id,omitempty"`
		Type                string                `json:"@type,omitempty"`
		Name                string                `json:"name,omitempty"`
		Description         string                `json:"description,omitempty"`
		StartDate           string                `json:"startDate,omitempty"`
		EndDate             string                `json:"endDate,omitempty"`
		DoorTime            string                `json:"doorTime,omitempty"`
		EventDomain         string                `json:"eventDomain,omitempty"`
		Location            json.RawMessage       `json:"location,omitempty"`
		VirtualLocation     *VirtualLocationInput `json:"virtualLocation,omitempty"`
		Organizer           json.RawMessage       `json:"organizer,omitempty"`
		Image               json.RawMessage       `json:"image,omitempty"`
		URL                 string                `json:"url,omitempty"`
		Keywords            json.RawMessage       `json:"keywords,omitempty"`
		InLanguage          json.RawMessage       `json:"inLanguage,omitempty"`
		IsAccessibleForFree *bool                 `json:"isAccessibleForFree,omitempty"`
		Offers              json.RawMessage       `json:"offers,omitempty"`
		SameAs              []string              `json:"sameAs,omitempty"`
		License             string                `json:"license,omitempty"`
		Source              *SourceInput          `json:"source,omitempty"`
		Occurrences         []OccurrenceInput     `json:"occurrences,omitempty"`
	}

	var raw rawEvent
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("unmarshal event input: %w", err)
	}

	// Copy direct fields
	e.Context = raw.Context
	e.ID = raw.ID
	e.Type = raw.Type
	e.Name = raw.Name
	e.Description = raw.Description
	e.StartDate = raw.StartDate
	e.EndDate = raw.EndDate
	e.DoorTime = raw.DoorTime
	e.EventDomain = raw.EventDomain
	e.VirtualLocation = raw.VirtualLocation
	e.URL = raw.URL
	e.IsAccessibleForFree = raw.IsAccessibleForFree
	e.SameAs = raw.SameAs
	e.License = raw.License
	e.Source = raw.Source
	e.Occurrences = raw.Occurrences

	// Handle location: object or text string
	if len(raw.Location) > 0 {
		loc, err := unmarshalFlexiblePlace(raw.Location)
		if err != nil {
			return fmt.Errorf("unmarshal location: %w", err)
		}
		e.Location = loc
	}

	// Handle organizer: object or string URI
	if len(raw.Organizer) > 0 {
		org, err := unmarshalFlexibleOrganizer(raw.Organizer)
		if err != nil {
			return fmt.Errorf("unmarshal organizer: %w", err)
		}
		e.Organizer = org
	}

	// Handle image: string URL or ImageObject
	if len(raw.Image) > 0 {
		img, err := unmarshalFlexibleImage(raw.Image)
		if err != nil {
			return fmt.Errorf("unmarshal image: %w", err)
		}
		e.Image = img
	}

	// Handle keywords: array of strings or comma-separated string
	if len(raw.Keywords) > 0 {
		kw, err := unmarshalFlexibleStringList(raw.Keywords, true)
		if err != nil {
			return fmt.Errorf("unmarshal keywords: %w", err)
		}
		e.Keywords = kw
	}

	// Handle inLanguage: array of strings or single string
	if len(raw.InLanguage) > 0 {
		lang, err := unmarshalFlexibleStringList(raw.InLanguage, false)
		if err != nil {
			return fmt.Errorf("unmarshal inLanguage: %w", err)
		}
		e.InLanguage = lang
	}

	// Handle offers: object or array (use first)
	if len(raw.Offers) > 0 {
		offer, err := unmarshalFlexibleOffer(raw.Offers)
		if err != nil {
			return fmt.Errorf("unmarshal offers: %w", err)
		}
		e.Offers = offer
	}

	return nil
}

// unmarshalFlexiblePlace handles location as either a PlaceInput object or a plain text string.
// Schema.org allows location to be Text, Place, PostalAddress, or VirtualLocation.
func unmarshalFlexiblePlace(data json.RawMessage) (*PlaceInput, error) {
	// Try string first (lightweight check)
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		return &PlaceInput{Name: s}, nil
	}

	// Try object
	var place PlaceInput
	if err := json.Unmarshal(data, &place); err != nil {
		return nil, fmt.Errorf("location must be an object or string: %w", err)
	}
	return &place, nil
}

// unmarshalFlexibleOrganizer handles organizer as either an OrganizationInput object or a string URI.
func unmarshalFlexibleOrganizer(data json.RawMessage) (*OrganizationInput, error) {
	// Try string first
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		return &OrganizationInput{ID: s}, nil
	}

	// Try object
	var org OrganizationInput
	if err := json.Unmarshal(data, &org); err != nil {
		return nil, fmt.Errorf("organizer must be an object or string: %w", err)
	}
	return &org, nil
}

// unmarshalFlexibleImage handles image as either a URL string or an ImageObject.
func unmarshalFlexibleImage(data json.RawMessage) (string, error) {
	// Try string first
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		return s, nil
	}

	// Try ImageObject
	var obj struct {
		URL        string `json:"url"`
		ContentURL string `json:"contentUrl"`
	}
	if err := json.Unmarshal(data, &obj); err != nil {
		return "", fmt.Errorf("image must be a string URL or ImageObject: %w", err)
	}
	if obj.URL != "" {
		return obj.URL, nil
	}
	return obj.ContentURL, nil
}

// unmarshalFlexibleStringList handles a field that can be either a JSON array of strings
// or a single string. If splitCommas is true, a single string is split on commas.
func unmarshalFlexibleStringList(data json.RawMessage, splitCommas bool) ([]string, error) {
	// Try array first
	var arr []string
	if err := json.Unmarshal(data, &arr); err == nil {
		return arr, nil
	}

	// Try single string
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("must be a string or array of strings: %w", err)
	}

	if splitCommas {
		parts := strings.Split(s, ",")
		result := make([]string, 0, len(parts))
		for _, p := range parts {
			trimmed := strings.TrimSpace(p)
			if trimmed != "" {
				result = append(result, trimmed)
			}
		}
		return result, nil
	}

	return []string{s}, nil
}

// UnmarshalJSON implements custom JSON unmarshaling for PlaceInput to handle
// both the flat format (addressLocality at top level) and the nested schema.org
// format (address.addressLocality, geo.latitude).
func (p *PlaceInput) UnmarshalJSON(data []byte) error {
	// Intermediate type with raw fields for flexible parsing
	type rawPlace struct {
		ID   string `json:"@id,omitempty"`
		Type string `json:"@type,omitempty"`
		Name string `json:"name,omitempty"`
		// Flat format fields
		StreetAddress   string  `json:"streetAddress,omitempty"`
		AddressLocality string  `json:"addressLocality,omitempty"`
		AddressRegion   string  `json:"addressRegion,omitempty"`
		PostalCode      string  `json:"postalCode,omitempty"`
		AddressCountry  string  `json:"addressCountry,omitempty"`
		Latitude        float64 `json:"latitude,omitempty"`
		Longitude       float64 `json:"longitude,omitempty"`
		// Nested schema.org format
		Address json.RawMessage `json:"address,omitempty"`
		Geo     json.RawMessage `json:"geo,omitempty"`
	}

	var raw rawPlace
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("unmarshal place input: %w", err)
	}

	p.ID = raw.ID
	p.Name = raw.Name

	// Start with flat format values
	p.StreetAddress = raw.StreetAddress
	p.AddressLocality = raw.AddressLocality
	p.AddressRegion = raw.AddressRegion
	p.PostalCode = raw.PostalCode
	p.AddressCountry = raw.AddressCountry
	p.Latitude = raw.Latitude
	p.Longitude = raw.Longitude

	// Override with nested address if present
	if len(raw.Address) > 0 {
		// Address can be a string (schema.org allows text) or PostalAddress object
		var addrStr string
		if err := json.Unmarshal(raw.Address, &addrStr); err == nil {
			// Plain text address — use as street address if no street address is set
			if p.StreetAddress == "" {
				p.StreetAddress = addrStr
			}
		} else {
			var addr struct {
				StreetAddress   string `json:"streetAddress,omitempty"`
				AddressLocality string `json:"addressLocality,omitempty"`
				AddressRegion   string `json:"addressRegion,omitempty"`
				PostalCode      string `json:"postalCode,omitempty"`
				AddressCountry  string `json:"addressCountry,omitempty"`
			}
			if err := json.Unmarshal(raw.Address, &addr); err != nil {
				return fmt.Errorf("unmarshal address: %w", err)
			}
			// Nested values override flat values
			if addr.StreetAddress != "" {
				p.StreetAddress = addr.StreetAddress
			}
			if addr.AddressLocality != "" {
				p.AddressLocality = addr.AddressLocality
			}
			if addr.AddressRegion != "" {
				p.AddressRegion = addr.AddressRegion
			}
			if addr.PostalCode != "" {
				p.PostalCode = addr.PostalCode
			}
			if addr.AddressCountry != "" {
				p.AddressCountry = addr.AddressCountry
			}
		}
	}

	// Override with nested geo if present
	if len(raw.Geo) > 0 {
		var geo struct {
			Latitude  float64 `json:"latitude,omitempty"`
			Longitude float64 `json:"longitude,omitempty"`
		}
		if err := json.Unmarshal(raw.Geo, &geo); err != nil {
			return fmt.Errorf("unmarshal geo: %w", err)
		}
		if geo.Latitude != 0 {
			p.Latitude = geo.Latitude
		}
		if geo.Longitude != 0 {
			p.Longitude = geo.Longitude
		}
	}

	return nil
}

// UnmarshalJSON implements custom JSON unmarshaling for OfferInput to handle:
//   - price as both string ("25.00") and number (25.00)
//   - offers as both a single object and an array (uses first element)
func (o *OfferInput) UnmarshalJSON(data []byte) error {
	// Use an intermediate with flexible price
	type rawOffer struct {
		URL           string          `json:"url,omitempty"`
		Price         json.RawMessage `json:"price,omitempty"`
		PriceCurrency string          `json:"priceCurrency,omitempty"`
	}

	var raw rawOffer
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("unmarshal offer input: %w", err)
	}

	o.URL = raw.URL
	o.PriceCurrency = raw.PriceCurrency

	// Handle price as string or number
	if len(raw.Price) > 0 {
		price, err := unmarshalFlexiblePrice(raw.Price)
		if err != nil {
			return fmt.Errorf("unmarshal price: %w", err)
		}
		o.Price = price
	}

	return nil
}

// unmarshalFlexiblePrice handles price as either a string or a number.
func unmarshalFlexiblePrice(data json.RawMessage) (string, error) {
	// Try string first
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		return s, nil
	}

	// Try number
	var n float64
	if err := json.Unmarshal(data, &n); err != nil {
		return "", fmt.Errorf("price must be a string or number: %w", err)
	}

	// Format without trailing zeros: 25.00 -> "25", 25.50 -> "25.5"
	if n == float64(int64(n)) {
		return fmt.Sprintf("%.0f", n), nil
	}
	return fmt.Sprintf("%g", n), nil
}

// unmarshalFlexibleOffer handles the offers field as either a single OfferInput
// object or an array of offers (uses the first one).
func unmarshalFlexibleOffer(data json.RawMessage) (*OfferInput, error) {
	// Try single object first
	var offer OfferInput
	if err := json.Unmarshal(data, &offer); err == nil {
		return &offer, nil
	}

	// Try array
	var offers []OfferInput
	if err := json.Unmarshal(data, &offers); err != nil {
		return nil, fmt.Errorf("offers must be an object or array: %w", err)
	}
	if len(offers) == 0 {
		return nil, nil
	}
	return &offers[0], nil
}
