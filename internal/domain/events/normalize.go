package events

import (
	"sort"
	"strings"
)

// NormalizeEventInput trims and normalizes values for consistent storage and hashing.
func NormalizeEventInput(input EventInput) EventInput {
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	input.StartDate = strings.TrimSpace(input.StartDate)
	input.EndDate = strings.TrimSpace(input.EndDate)
	input.DoorTime = strings.TrimSpace(input.DoorTime)
	input.Image = strings.TrimSpace(input.Image)
	input.URL = strings.TrimSpace(input.URL)
	input.License = strings.TrimSpace(input.License)

	input.Keywords = normalizeStringSlice(input.Keywords, true)
	input.InLanguage = normalizeStringSlice(input.InLanguage, true)
	input.SameAs = normalizeStringSlice(input.SameAs, false)

	if input.Location != nil {
		input.Location = normalizePlaceInput(*input.Location)
	}
	if input.VirtualLocation != nil {
		input.VirtualLocation = normalizeVirtualLocationInput(*input.VirtualLocation)
	}
	if input.Organizer != nil {
		input.Organizer = normalizeOrganizationInput(*input.Organizer)
	}
	if input.Offers != nil {
		input.Offers = normalizeOfferInput(*input.Offers)
	}
	if input.Source != nil {
		input.Source = normalizeSourceInput(*input.Source)
	}
	if len(input.Occurrences) > 0 {
		input.Occurrences = normalizeOccurrences(input.Occurrences)
	}

	return input
}

func normalizePlaceInput(place PlaceInput) *PlaceInput {
	place.ID = strings.TrimSpace(place.ID)
	place.Name = strings.TrimSpace(place.Name)
	place.StreetAddress = strings.TrimSpace(place.StreetAddress)
	place.AddressLocality = strings.TrimSpace(place.AddressLocality)
	place.AddressRegion = strings.TrimSpace(place.AddressRegion)
	place.PostalCode = strings.TrimSpace(place.PostalCode)
	place.AddressCountry = strings.TrimSpace(place.AddressCountry)
	return &place
}

func normalizeVirtualLocationInput(location VirtualLocationInput) *VirtualLocationInput {
	location.Type = strings.TrimSpace(location.Type)
	location.URL = strings.TrimSpace(location.URL)
	location.Name = strings.TrimSpace(location.Name)
	return &location
}

func normalizeOrganizationInput(org OrganizationInput) *OrganizationInput {
	org.ID = strings.TrimSpace(org.ID)
	org.Name = strings.TrimSpace(org.Name)
	org.URL = strings.TrimSpace(org.URL)
	return &org
}

func normalizeOfferInput(offer OfferInput) *OfferInput {
	offer.URL = strings.TrimSpace(offer.URL)
	offer.Price = strings.TrimSpace(offer.Price)
	offer.PriceCurrency = strings.TrimSpace(offer.PriceCurrency)
	return &offer
}

func normalizeSourceInput(source SourceInput) *SourceInput {
	source.URL = strings.TrimSpace(source.URL)
	source.EventID = strings.TrimSpace(source.EventID)
	return &source
}

func normalizeOccurrences(values []OccurrenceInput) []OccurrenceInput {
	result := make([]OccurrenceInput, 0, len(values))
	for _, occ := range values {
		occ.StartDate = strings.TrimSpace(occ.StartDate)
		occ.EndDate = strings.TrimSpace(occ.EndDate)
		occ.DoorTime = strings.TrimSpace(occ.DoorTime)
		occ.Timezone = strings.TrimSpace(occ.Timezone)
		occ.VenueID = strings.TrimSpace(occ.VenueID)
		occ.VirtualURL = strings.TrimSpace(occ.VirtualURL)
		result = append(result, occ)
	}
	return result
}

func normalizeStringSlice(values []string, lower bool) []string {
	if len(values) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if lower {
			trimmed = strings.ToLower(trimmed)
		}
		set[trimmed] = struct{}{}
	}
	if len(set) == 0 {
		return nil
	}
	result := make([]string, 0, len(set))
	for value := range set {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
