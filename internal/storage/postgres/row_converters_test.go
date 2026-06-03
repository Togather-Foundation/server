package postgres

import (
	"math/big"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/require"
)

func TestPlaceToDomain_AllFields(t *testing.T) {
	testUUID := uuid.New()
	testPgUUID := pgtype.UUID{}
	err := testPgUUID.Scan(testUUID.String())
	require.NoError(t, err)

	testCreatedAt := time.Date(2026, 2, 19, 10, 0, 0, 0, time.UTC)
	testUpdatedAt := time.Date(2026, 2, 19, 11, 0, 0, 0, time.UTC)

	place := (&Place{
		ID:                      testPgUUID,
		Ulid:                    "01HQZX12345678901234567890",
		Name:                    "Centennial Park",
		Description:             pgtype.Text{String: "A beautiful park", Valid: true},
		StreetAddress:           pgtype.Text{String: "151 Elmcrest Rd", Valid: true},
		AddressLocality:         pgtype.Text{String: "Toronto", Valid: true},
		AddressRegion:           pgtype.Text{String: "ON", Valid: true},
		PostalCode:              pgtype.Text{String: "M9B 6E7", Valid: true},
		AddressCountry:          pgtype.Text{String: "CA", Valid: true},
		Latitude:                pgtype.Numeric{Int: mustNumericInt(43, 6534, 4), Valid: true, Exp: -4},
		Longitude:               pgtype.Numeric{Int: mustNumericInt(-79, 3839, 4), Valid: true, Exp: -4},
		Telephone:               pgtype.Text{String: "+1-416-555-1234", Valid: true},
		Email:                   pgtype.Text{String: "info@park.ca", Valid: true},
		Url:                     pgtype.Text{String: "https://park.ca", Valid: true},
		MaximumAttendeeCapacity: pgtype.Int4{Int32: 500, Valid: true},
		VenueType:               pgtype.Text{String: "Park", Valid: true},
		FederationUri:           pgtype.Text{String: "https://example.org/places/123", Valid: true},
		CreatedAt:               pgtype.Timestamptz{Time: testCreatedAt, Valid: true},
		UpdatedAt:               pgtype.Timestamptz{Time: testUpdatedAt, Valid: true},
	}).toDomain()

	require.Equal(t, testUUID.String(), place.ID)
	require.Equal(t, "01HQZX12345678901234567890", place.ULID)
	require.Equal(t, "Centennial Park", place.Name)
	require.Equal(t, "A beautiful park", place.Description)
	require.Equal(t, "151 Elmcrest Rd", place.StreetAddress)
	require.Equal(t, "Toronto", place.City)
	require.Equal(t, "ON", place.Region)
	require.Equal(t, "M9B 6E7", place.PostalCode)
	require.Equal(t, "CA", place.Country)
	require.Equal(t, "+1-416-555-1234", place.Telephone)
	require.Equal(t, "info@park.ca", place.Email)
	require.Equal(t, "https://park.ca", place.URL)
	require.Equal(t, "Park", place.VenueType)
	require.Equal(t, "https://example.org/places/123", place.FederationURI)
	require.NotNil(t, place.Latitude)
	require.InDelta(t, 43.6534, *place.Latitude, 0.0001)
	require.NotNil(t, place.Longitude)
	require.InDelta(t, -79.3839, *place.Longitude, 0.0001)
	require.NotNil(t, place.MaximumAttendeeCapacity)
	require.Equal(t, 500, *place.MaximumAttendeeCapacity)
	require.Equal(t, testCreatedAt, place.CreatedAt)
	require.Equal(t, testUpdatedAt, place.UpdatedAt)
}

func TestPlaceToDomain_NullFields(t *testing.T) {
	testUUID := uuid.New()
	testPgUUID := pgtype.UUID{}
	err := testPgUUID.Scan(testUUID.String())
	require.NoError(t, err)

	testCreatedAt := time.Date(2026, 2, 19, 10, 0, 0, 0, time.UTC)

	place := (&Place{
		ID:                      testPgUUID,
		Ulid:                    "01HQZX12345678901234567890",
		Name:                    "Minimal Place",
		Description:             pgtype.Text{Valid: false},
		StreetAddress:           pgtype.Text{Valid: false},
		AddressLocality:         pgtype.Text{Valid: false},
		AddressRegion:           pgtype.Text{Valid: false},
		PostalCode:              pgtype.Text{Valid: false},
		AddressCountry:          pgtype.Text{Valid: false},
		Latitude:                pgtype.Numeric{Valid: false},
		Longitude:               pgtype.Numeric{Valid: false},
		Telephone:               pgtype.Text{Valid: false},
		Email:                   pgtype.Text{Valid: false},
		Url:                     pgtype.Text{Valid: false},
		MaximumAttendeeCapacity: pgtype.Int4{Valid: false},
		VenueType:               pgtype.Text{Valid: false},
		FederationUri:           pgtype.Text{Valid: false},
		CreatedAt:               pgtype.Timestamptz{Time: testCreatedAt, Valid: true},
		UpdatedAt:               pgtype.Timestamptz{Valid: false},
	}).toDomain()

	require.Equal(t, testUUID.String(), place.ID)
	require.Equal(t, "01HQZX12345678901234567890", place.ULID)
	require.Equal(t, "Minimal Place", place.Name)
	require.Equal(t, "", place.Description)
	require.Equal(t, "", place.StreetAddress)
	require.Equal(t, "", place.City)
	require.Equal(t, "", place.Region)
	require.Equal(t, "", place.PostalCode)
	require.Equal(t, "", place.Country)
	require.Equal(t, "", place.Telephone)
	require.Equal(t, "", place.Email)
	require.Equal(t, "", place.URL)
	require.Equal(t, "", place.VenueType)
	require.Equal(t, "", place.FederationURI)
	require.Nil(t, place.Latitude)
	require.Nil(t, place.Longitude)
	require.Nil(t, place.MaximumAttendeeCapacity)
	require.Equal(t, testCreatedAt, place.CreatedAt)
	require.True(t, place.UpdatedAt.IsZero(), "UpdatedAt should be zero time when NULL")
}

func TestOrganizationToDomain_AllFields(t *testing.T) {
	testUUID := uuid.New()
	testPgUUID := pgtype.UUID{}
	err := testPgUUID.Scan(testUUID.String())
	require.NoError(t, err)

	testCreatedAt := time.Date(2026, 2, 19, 10, 0, 0, 0, time.UTC)
	testUpdatedAt := time.Date(2026, 2, 19, 11, 0, 0, 0, time.UTC)

	org := (&Organization{
		ID:               testPgUUID,
		Ulid:             "01HQZX12345678901234567890",
		Name:             "Toronto Arts Foundation",
		LegalName:        pgtype.Text{String: "Toronto Arts Foundation Inc.", Valid: true},
		Description:      pgtype.Text{String: "A non-profit arts organization", Valid: true},
		Email:            pgtype.Text{String: "info@torontoarts.org", Valid: true},
		Telephone:        pgtype.Text{String: "+1-416-555-9876", Valid: true},
		Url:              pgtype.Text{String: "https://torontoarts.org", Valid: true},
		AddressLocality:  pgtype.Text{String: "Toronto", Valid: true},
		AddressRegion:    pgtype.Text{String: "ON", Valid: true},
		AddressCountry:   pgtype.Text{String: "CA", Valid: true},
		StreetAddress:    pgtype.Text{String: "100 Queen St W", Valid: true},
		PostalCode:       pgtype.Text{String: "M5H 2N2", Valid: true},
		OrganizationType: pgtype.Text{String: "NonProfit", Valid: true},
		FederationUri:    pgtype.Text{String: "https://example.org/organizations/456", Valid: true},
		AlternateName:    pgtype.Text{String: "TAF", Valid: true},
		CreatedAt:        pgtype.Timestamptz{Time: testCreatedAt, Valid: true},
		UpdatedAt:        pgtype.Timestamptz{Time: testUpdatedAt, Valid: true},
	}).toDomain()

	require.Equal(t, testUUID.String(), org.ID)
	require.Equal(t, "01HQZX12345678901234567890", org.ULID)
	require.Equal(t, "Toronto Arts Foundation", org.Name)
	require.Equal(t, "Toronto Arts Foundation Inc.", org.LegalName)
	require.Equal(t, "A non-profit arts organization", org.Description)
	require.Equal(t, "info@torontoarts.org", org.Email)
	require.Equal(t, "+1-416-555-9876", org.Telephone)
	require.Equal(t, "https://torontoarts.org", org.URL)
	require.Equal(t, "Toronto", org.AddressLocality)
	require.Equal(t, "ON", org.AddressRegion)
	require.Equal(t, "CA", org.AddressCountry)
	require.Equal(t, "100 Queen St W", org.StreetAddress)
	require.Equal(t, "M5H 2N2", org.PostalCode)
	require.Equal(t, "NonProfit", org.OrganizationType)
	require.Equal(t, "https://example.org/organizations/456", org.FederationURI)
	require.Equal(t, "TAF", org.AlternateName)
	require.Equal(t, testCreatedAt, org.CreatedAt)
	require.Equal(t, testUpdatedAt, org.UpdatedAt)
}

func TestOrganizationToDomain_NullFields(t *testing.T) {
	testUUID := uuid.New()
	testPgUUID := pgtype.UUID{}
	err := testPgUUID.Scan(testUUID.String())
	require.NoError(t, err)

	testCreatedAt := time.Date(2026, 2, 19, 10, 0, 0, 0, time.UTC)

	org := (&Organization{
		ID:               testPgUUID,
		Ulid:             "01HQZX12345678901234567890",
		Name:             "Minimal Organization",
		LegalName:        pgtype.Text{Valid: false},
		Description:      pgtype.Text{Valid: false},
		Email:            pgtype.Text{Valid: false},
		Telephone:        pgtype.Text{Valid: false},
		Url:              pgtype.Text{Valid: false},
		AddressLocality:  pgtype.Text{Valid: false},
		AddressRegion:    pgtype.Text{Valid: false},
		AddressCountry:   pgtype.Text{Valid: false},
		StreetAddress:    pgtype.Text{Valid: false},
		PostalCode:       pgtype.Text{Valid: false},
		OrganizationType: pgtype.Text{Valid: false},
		FederationUri:    pgtype.Text{Valid: false},
		AlternateName:    pgtype.Text{Valid: false},
		CreatedAt:        pgtype.Timestamptz{Time: testCreatedAt, Valid: true},
		UpdatedAt:        pgtype.Timestamptz{Valid: false},
	}).toDomain()

	require.Equal(t, testUUID.String(), org.ID)
	require.Equal(t, "01HQZX12345678901234567890", org.ULID)
	require.Equal(t, "Minimal Organization", org.Name)
	require.Equal(t, "", org.LegalName)
	require.Equal(t, "", org.Description)
	require.Equal(t, "", org.Email)
	require.Equal(t, "", org.Telephone)
	require.Equal(t, "", org.URL)
	require.Equal(t, "", org.AddressLocality)
	require.Equal(t, "", org.AddressRegion)
	require.Equal(t, "", org.AddressCountry)
	require.Equal(t, "", org.StreetAddress)
	require.Equal(t, "", org.PostalCode)
	require.Equal(t, "", org.OrganizationType)
	require.Equal(t, "", org.FederationURI)
	require.Equal(t, "", org.AlternateName)
	require.Equal(t, testCreatedAt, org.CreatedAt)
	require.True(t, org.UpdatedAt.IsZero(), "UpdatedAt should be zero time when NULL")
}

// mustNumericInt is a helper for creating pgtype.Numeric values in tests
func mustNumericInt(wholePart int64, fractionalPart int64, decimalPlaces int) *big.Int {
	multiplier := int64(1)
	for i := 0; i < decimalPlaces; i++ {
		multiplier *= 10
	}

	total := wholePart * multiplier
	if wholePart < 0 {
		total -= fractionalPart
	} else {
		total += fractionalPart
	}

	return big.NewInt(total)
}
