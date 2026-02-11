package schema

import (
	"github.com/Togather-Foundation/server/internal/domain/ids"
)

// BuildEventURI constructs a canonical URI: https://domain/events/ULID
func BuildEventURI(baseURL, ulid string) string {
	uri, err := ids.BuildCanonicalURI(baseURL, "events", ulid)
	if err != nil {
		return ""
	}
	return uri
}

// BuildPlaceURI constructs a canonical URI: https://domain/places/ULID
func BuildPlaceURI(baseURL, ulid string) string {
	uri, err := ids.BuildCanonicalURI(baseURL, "places", ulid)
	if err != nil {
		return ""
	}
	return uri
}

// BuildOrganizationURI constructs a canonical URI: https://domain/organizations/ULID
func BuildOrganizationURI(baseURL, ulid string) string {
	uri, err := ids.BuildCanonicalURI(baseURL, "organizations", ulid)
	if err != nil {
		return ""
	}
	return uri
}

// NewPostalAddress creates a PostalAddress if any field is non-empty, otherwise returns nil.
func NewPostalAddress(street, locality, region, postalCode, country string) *PostalAddress {
	if street == "" && locality == "" && region == "" && postalCode == "" && country == "" {
		return nil
	}
	return &PostalAddress{
		Type:            "PostalAddress",
		StreetAddress:   street,
		AddressLocality: locality,
		AddressRegion:   region,
		PostalCode:      postalCode,
		AddressCountry:  country,
	}
}

// NewGeoCoordinates creates GeoCoordinates if lat/lng are non-zero, otherwise returns nil.
// Note: lat=0/lng=0 is a valid coordinate but extremely unlikely for real-world venues.
// Use the GeoCoordinates struct directly for the rare case of Gulf of Guinea coordinates.
func NewGeoCoordinates(lat, lng float64) *GeoCoordinates {
	if lat == 0 && lng == 0 {
		return nil
	}
	return &GeoCoordinates{
		Type:      "GeoCoordinates",
		Latitude:  &lat,
		Longitude: &lng,
	}
}
