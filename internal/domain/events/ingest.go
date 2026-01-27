package events

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/Togather-Foundation/server/internal/domain/ids"
)

type IngestResult struct {
	Event       *Event
	IsDuplicate bool
	NeedsReview bool
}

type IngestService struct {
	repo       Repository
	nodeDomain string
	defaultTZ  string
}

func NewIngestService(repo Repository, nodeDomain string) *IngestService {
	return &IngestService{repo: repo, nodeDomain: nodeDomain, defaultTZ: "America/Toronto"}
}

func (s *IngestService) Ingest(ctx context.Context, input EventInput) (*IngestResult, error) {
	return s.IngestWithIdempotency(ctx, input, "")
}

func (s *IngestService) IngestWithIdempotency(ctx context.Context, input EventInput, idempotencyKey string) (*IngestResult, error) {
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf("ingest: repository not configured")
	}

	if strings.TrimSpace(idempotencyKey) != "" {
		keyEntry, err := s.repo.GetIdempotencyKey(ctx, idempotencyKey)
		if err == nil && keyEntry != nil {
			if keyEntry.EventULID == nil || *keyEntry.EventULID == "" {
				return nil, ErrConflict
			}
			item, err := s.repo.GetByULID(ctx, *keyEntry.EventULID)
			if err != nil {
				return nil, err
			}
			payloadHash, err := hashInput(normalizedInputForHash(input))
			if err != nil {
				return nil, err
			}
			if payloadHash != keyEntry.RequestHash {
				return nil, ErrConflict
			}
			return &IngestResult{Event: item, IsDuplicate: true}, nil
		}
		if err != nil && err != ErrNotFound {
			return nil, err
		}
		payloadHash, err := hashInput(normalizedInputForHash(input))
		if err != nil {
			return nil, err
		}
		_, err = s.repo.InsertIdempotencyKey(ctx, IdempotencyKeyCreateParams{
			Key:         idempotencyKey,
			RequestHash: payloadHash,
			EventID:     "",
			EventULID:   "",
		})
		if err != nil {
			return nil, err
		}
	}

	validated, err := ValidateEventInput(input, s.nodeDomain)
	if err != nil {
		return nil, err
	}
	normalized := NormalizeEventInput(validated)

	var sourceID string
	if normalized.Source != nil && normalized.Source.URL != "" {
		sourceID, err = s.repo.GetOrCreateSource(ctx, SourceLookupParams{
			Name:        sourceName(normalized.Source, normalized.Name),
			SourceType:  "api",
			BaseURL:     sourceBaseURL(normalized.Source.URL),
			LicenseURL:  licenseURL(sourceLicense(normalized)),
			LicenseType: sourceLicenseType(normalized),
			TrustLevel:  5,
		})
		if err != nil {
			return nil, err
		}

		existing, err := s.repo.FindBySourceExternalID(ctx, sourceID, normalized.Source.EventID)
		if err == nil && existing != nil {
			return &IngestResult{Event: existing, IsDuplicate: true}, nil
		}
		if err != nil && err != ErrNotFound {
			return nil, err
		}
	}

	dedupHash := BuildDedupHash(DedupCandidate{
		Name:      normalized.Name,
		VenueID:   primaryVenueKey(normalized),
		StartDate: normalized.StartDate,
	})
	if dedupHash != "" {
		existing, err := s.repo.FindByDedupHash(ctx, dedupHash)
		if err == nil && existing != nil {
			return &IngestResult{Event: existing, IsDuplicate: true}, nil
		}
		if err != nil && err != ErrNotFound {
			return nil, err
		}
	}

	ulidValue, err := ids.NewULID()
	if err != nil {
		return nil, fmt.Errorf("generate ulid: %w", err)
	}

	needsReview := needsReview(normalized, nil)
	lifecycleState := "published"
	if needsReview {
		lifecycleState = "draft"
	}
	params := EventCreateParams{
		ULID:           ulidValue,
		Name:           normalized.Name,
		Description:    normalized.Description,
		LifecycleState: lifecycleState,
		EventDomain:    "arts",
		OrganizerID:    nil,
		PrimaryVenueID: nil,
		VirtualURL:     virtualURL(normalized),
		ImageURL:       normalized.Image,
		PublicURL:      normalized.URL,
		Keywords:       normalized.Keywords,
		LicenseURL:     licenseURL(normalized.License),
		LicenseStatus:  "cc0",
		Confidence:     floatPtr(reviewConfidence(normalized, needsReview)),
		OriginNodeID:   nil,
	}

	if normalized.Location != nil && normalized.Location.Name != "" {
		placeULID, err := ids.NewULID()
		if err != nil {
			return nil, fmt.Errorf("generate place ulid: %w", err)
		}
		place, err := s.repo.UpsertPlace(ctx, PlaceCreateParams{
			EntityCreateFields: EntityCreateFields{
				ULID:            placeULID,
				Name:            normalized.Location.Name,
				AddressLocality: normalized.Location.AddressLocality,
				AddressRegion:   normalized.Location.AddressRegion,
				AddressCountry:  normalized.Location.AddressCountry,
			},
		})
		if err != nil {
			return nil, err
		}
		params.PrimaryVenueID = &place.ID
	}

	if normalized.Organizer != nil && normalized.Organizer.Name != "" {
		orgULID, err := ids.NewULID()
		if err != nil {
			return nil, fmt.Errorf("generate organizer ulid: %w", err)
		}
		addressLocality := ""
		addressRegion := ""
		addressCountry := ""
		if normalized.Location != nil {
			addressLocality = normalized.Location.AddressLocality
			addressRegion = normalized.Location.AddressRegion
			addressCountry = normalized.Location.AddressCountry
		}
		org, err := s.repo.UpsertOrganization(ctx, OrganizationCreateParams{
			EntityCreateFields: EntityCreateFields{
				ULID:            orgULID,
				Name:            normalized.Organizer.Name,
				AddressLocality: addressLocality,
				AddressRegion:   addressRegion,
				AddressCountry:  addressCountry,
			},
		})
		if err != nil {
			return nil, err
		}
		params.OrganizerID = &org.ID
	}

	// Store the dedup hash so future ingestions can find this event
	params.DedupHash = dedupHash

	event, err := s.repo.Create(ctx, params)
	if err != nil {
		return nil, err
	}

	if err := s.createOccurrences(ctx, event, normalized); err != nil {
		return nil, err
	}

	if err := s.recordSource(ctx, event, normalized, sourceID); err != nil {
		return nil, err
	}

	if strings.TrimSpace(idempotencyKey) != "" {
		if err := s.repo.UpdateIdempotencyKeyEvent(ctx, idempotencyKey, event.ID, event.ULID); err != nil {
			return nil, err
		}
	}

	return &IngestResult{Event: event, NeedsReview: needsReview}, nil
}

func (s *IngestService) createOccurrences(ctx context.Context, event *Event, input EventInput) error {
	if event == nil {
		return fmt.Errorf("create occurrences: missing event")
	}

	if len(input.Occurrences) == 0 {
		start, err := time.Parse(time.RFC3339, strings.TrimSpace(input.StartDate))
		if err != nil {
			return fmt.Errorf("parse startDate: %w", err)
		}
		end, err := parseRFC3339Optional("endDate", input.EndDate)
		if err != nil {
			return fmt.Errorf("parse end date: %w", err)
		}
		venueID := event.PrimaryVenueID
		virtual := nullableString(virtualURL(input))
		if venueID == nil && virtual == nil && event.VirtualURL != "" {
			virtual = nullableString(event.VirtualURL)
		}
		occurrence := OccurrenceCreateParams{
			EventID:    event.ID,
			StartTime:  start,
			EndTime:    end,
			Timezone:   s.defaultTZ,
			VenueID:    venueID,
			VirtualURL: virtual,
		}
		return s.repo.CreateOccurrence(ctx, occurrence)
	}

	for _, occ := range input.Occurrences {
		start, err := time.Parse(time.RFC3339, strings.TrimSpace(occ.StartDate))
		if err != nil {
			return fmt.Errorf("parse occurrence startDate: %w", err)
		}
		end, err := parseRFC3339Optional("endDate", occ.EndDate)
		if err != nil {
			return fmt.Errorf("parse occurrence end date: %w", err)
		}
		var door *time.Time
		if occ.DoorTime != "" {
			value, err := time.Parse(time.RFC3339, strings.TrimSpace(occ.DoorTime))
			if err != nil {
				return fmt.Errorf("parse occurrence doorTime: %w", err)
			}
			door = &value
		}
		tz := strings.TrimSpace(occ.Timezone)
		if tz == "" {
			tz = s.defaultTZ
		}
		occurrence := OccurrenceCreateParams{
			EventID:    event.ID,
			StartTime:  start,
			EndTime:    end,
			Timezone:   tz,
			DoorTime:   door,
			VenueID:    nullableString(occ.VenueID),
			VirtualURL: nullableString(occ.VirtualURL),
		}
		if err := s.repo.CreateOccurrence(ctx, occurrence); err != nil {
			return fmt.Errorf("create occurrence: %w", err)
		}
	}

	return nil
}

func (s *IngestService) recordSource(ctx context.Context, event *Event, input EventInput, sourceID string) error {
	if input.Source == nil || input.Source.URL == "" || sourceID == "" {
		return nil
	}

	payload, err := json.Marshal(input)
	if err != nil {
		return fmt.Errorf("source payload: %w", err)
	}
	payloadHash := sha256.Sum256(payload)

	params := EventSourceCreateParams{
		EventID:       event.ID,
		SourceID:      sourceID,
		SourceURL:     input.Source.URL,
		SourceEventID: input.Source.EventID,
		Payload:       payload,
		PayloadHash:   hex.EncodeToString(payloadHash[:]),
	}

	return s.repo.CreateSource(ctx, params)
}

func primaryVenueKey(input EventInput) string {
	if input.Location != nil {
		if input.Location.ID != "" {
			return input.Location.ID
		}
		return input.Location.Name
	}
	if input.VirtualLocation != nil {
		return input.VirtualLocation.URL
	}
	return ""
}

func locationID(place *PlaceInput) string {
	if place == nil {
		return ""
	}
	return strings.TrimSpace(place.ID)
}

func virtualURL(input EventInput) string {
	if input.VirtualLocation == nil {
		return ""
	}
	return strings.TrimSpace(input.VirtualLocation.URL)
}

func sourceName(source *SourceInput, fallback string) string {
	if source == nil {
		return fallbackOrUnknown(fallback)
	}
	if strings.TrimSpace(source.Name) != "" {
		return strings.TrimSpace(source.Name)
	}
	if strings.TrimSpace(source.EventID) != "" {
		return strings.TrimSpace(source.EventID)
	}
	return fallbackOrUnknown(fallback)
}

func sourceLicense(input EventInput) string {
	if input.Source != nil {
		if strings.TrimSpace(input.Source.License) != "" {
			return input.Source.License
		}
	}
	return input.License
}

func sourceLicenseType(input EventInput) string {
	license := strings.TrimSpace(strings.ToLower(sourceLicense(input)))
	if license == "" {
		return "unknown"
	}
	if strings.Contains(license, "creativecommons.org/publicdomain/zero") || license == "cc0" || license == "cc0-1.0" {
		return "CC0"
	}
	if strings.Contains(license, "creativecommons.org/licenses/by") || strings.Contains(license, "cc-by") {
		return "CC-BY"
	}
	return "unknown"
}

func fallbackOrUnknown(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "unknown"
	}
	return trimmed
}

func sourceBaseURL(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return trimmed
	}
	return fmt.Sprintf("%s://%s", parsed.Scheme, parsed.Host)
}

func licenseURL(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "https://creativecommons.org/publicdomain/zero/1.0/"
	}
	return trimmed
}

func nullableString(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func needsReview(input EventInput, linkStatuses map[string]int) bool {
	if reviewConfidence(input, false) < 0.6 {
		return true
	}
	if strings.TrimSpace(input.Description) == "" {
		return true
	}
	if strings.TrimSpace(input.Image) == "" {
		return true
	}
	if isTooFarFuture(input.StartDate, 730) {
		return true
	}
	for _, code := range linkStatuses {
		if code >= 400 {
			return true
		}
	}
	return false
}

func reviewConfidence(input EventInput, flagged bool) float64 {
	confidence := 0.9
	if strings.TrimSpace(input.Description) == "" {
		confidence -= 0.2
	}
	if strings.TrimSpace(input.Image) == "" {
		confidence -= 0.2
	}
	if isTooFarFuture(input.StartDate, 730) {
		confidence -= 0.2
	}
	if flagged {
		confidence -= 0.1
	}
	if confidence < 0 {
		confidence = 0
	}
	return confidence
}

func isTooFarFuture(startDate string, days int) bool {
	trimmed := strings.TrimSpace(startDate)
	if trimmed == "" {
		return false
	}
	parsed, err := time.Parse(time.RFC3339, trimmed)
	if err != nil {
		return false
	}
	return parsed.After(time.Now().Add(time.Duration(days) * 24 * time.Hour))
}

func floatPtr(value float64) *float64 {
	return &value
}

func normalizedInputForHash(input EventInput) EventInput {
	return NormalizeEventInput(input)
}

func hashInput(input EventInput) (string, error) {
	payload, err := json.Marshal(input)
	if err != nil {
		return "", fmt.Errorf("hash input: %w", err)
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}
