package events

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Togather-Foundation/server/internal/config"
	"github.com/Togather-Foundation/server/internal/domain/ids"
	"github.com/rs/zerolog/log"
)

type IngestResult struct {
	Event       *Event
	IsDuplicate bool
	IsMerged    bool // true when auto-merged with existing event
	NeedsReview bool
	Warnings    []ValidationWarning
}

type IngestService struct {
	repo             Repository
	nodeDomain       string
	defaultTZ        string
	validationConfig config.ValidationConfig
	dedupConfig      config.DedupConfig
}

func NewIngestService(repo Repository, nodeDomain string, validationConfig config.ValidationConfig) *IngestService {
	return &IngestService{
		repo:             repo,
		nodeDomain:       nodeDomain,
		defaultTZ:        "America/Toronto",
		validationConfig: validationConfig,
	}
}

// WithDedupConfig sets the deduplication configuration and returns the service for chaining.
func (s *IngestService) WithDedupConfig(cfg config.DedupConfig) *IngestService {
	s.dedupConfig = cfg
	return s
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

	// FIX: Normalize FIRST, then validate
	// This allows timezone corrections and other normalizations to run before validation
	normalized := NormalizeEventInput(input)

	// Pass original input so validation can detect auto-corrections
	validationResult, err := ValidateEventInputWithWarnings(normalized, s.nodeDomain, &input)
	if err != nil {
		return nil, err
	}
	validated := validationResult.Input
	warnings := validationResult.Warnings

	log.Debug().
		Str("event_name", validated.Name).
		Int("validation_warnings", len(warnings)).
		Msg("Ingest: Before appendQualityWarnings")

	// Add synthetic warnings for quality issues that trigger review
	warnings = appendQualityWarnings(warnings, validated, nil, s.validationConfig)

	log.Debug().
		Str("event_name", validated.Name).
		Int("total_warnings", len(warnings)).
		Msg("Ingest: After appendQualityWarnings")

	// Check if review is needed due to validation warnings OR metadata quality issues
	needsReview := len(warnings) > 0 || needsReview(validated, nil, s.validationConfig)

	var sourceID string
	if validated.Source != nil && validated.Source.URL != "" {
		sourceID, err = s.repo.GetOrCreateSource(ctx, SourceLookupParams{
			Name:        sourceName(validated.Source, validated.Name),
			SourceType:  "api",
			BaseURL:     sourceBaseURL(validated.Source.URL),
			LicenseURL:  licenseURL(sourceLicense(validated)),
			LicenseType: sourceLicenseType(validated),
			TrustLevel:  5,
		})
		if err != nil {
			return nil, err
		}

		existing, err := s.repo.FindBySourceExternalID(ctx, sourceID, validated.Source.EventID)
		if err == nil && existing != nil {
			return &IngestResult{Event: existing, IsDuplicate: true, Warnings: warnings}, nil
		}
		if err != nil && err != ErrNotFound {
			return nil, err
		}
	}

	dedupHash := BuildDedupHash(DedupCandidate{
		Name:      validated.Name,
		VenueID:   primaryVenueKey(validated),
		StartDate: validated.StartDate,
	})
	if dedupHash != "" {
		existing, err := s.repo.FindByDedupHash(ctx, dedupHash)
		if err == nil && existing != nil {
			// Auto-merge: fill gaps and overwrite if new source has higher trust
			existingTrust, err := s.repo.GetSourceTrustLevel(ctx, existing.ID)
			if err != nil {
				return nil, fmt.Errorf("get existing source trust: %w", err)
			}
			newTrust := 5 // default trust level
			if sourceID != "" {
				newTrust, err = s.repo.GetSourceTrustLevelBySourceID(ctx, sourceID)
				if err != nil {
					return nil, fmt.Errorf("get new source trust: %w", err)
				}
			}

			updates, changed := AutoMergeFields(existing, validated, existingTrust, newTrust)
			if changed {
				existing, err = s.repo.UpdateEvent(ctx, existing.ULID, updates)
				if err != nil {
					return nil, fmt.Errorf("auto-merge update: %w", err)
				}
			}

			// Record the new source's contribution to this event
			if sourceID != "" {
				_ = s.recordSourceForExisting(ctx, existing, validated, sourceID)
			}

			return &IngestResult{Event: existing, IsDuplicate: true, IsMerged: changed, Warnings: warnings}, nil
		}
		if err != nil && err != ErrNotFound {
			return nil, err
		}
	}

	// Check for existing review queue entry if this event needs review
	if needsReview {
		var externalID *string
		if validated.Source != nil && validated.Source.EventID != "" {
			externalID = &validated.Source.EventID
		}
		var dedupHashPtr *string
		if dedupHash != "" {
			dedupHashPtr = &dedupHash
		}
		var sourceIDPtr *string
		if sourceID != "" {
			sourceIDPtr = &sourceID
		}

		existingReview, err := s.repo.FindReviewByDedup(ctx, sourceIDPtr, externalID, dedupHashPtr)
		if err != nil && err != ErrNotFound {
			return nil, fmt.Errorf("check existing review: %w", err)
		}

		if existingReview != nil {
			switch existingReview.Status {
			case "rejected":
				// Check if rejection is still valid (event hasn't passed yet)
				if !isEventPast(existingReview.EventEndTime) {
					if stillHasSameIssues(existingReview.Warnings, warnings) {
						return nil, ErrPreviouslyRejected{
							Reason:     stringOrEmpty(existingReview.RejectionReason),
							ReviewedAt: timeOrZero(existingReview.ReviewedAt),
							ReviewedBy: stringOrEmpty(existingReview.ReviewedBy),
						}
					}
				}
				// Event passed or different issues - allow resubmission (continue to create new event)

			case "pending":
				// Already in queue - check if fixed
				if len(warnings) == 0 {
					// Fixed! Approve and publish
					_, err := s.repo.ApproveReview(ctx, existingReview.ID, "system", stringPtr("Auto-approved: resubmission with no warnings"))
					if err != nil {
						return nil, fmt.Errorf("approve review: %w", err)
					}
					// Update the event to published
					updatedEvent, err := s.repo.UpdateEvent(ctx, existingReview.EventULID, UpdateEventParams{
						LifecycleState: stringPtr("published"),
					})
					if err != nil {
						return nil, fmt.Errorf("update event to published: %w", err)
					}
					return &IngestResult{Event: updatedEvent, NeedsReview: false, Warnings: nil}, nil
				}
				// Still has issues - update queue entry with new payloads
				originalJSON, err := toJSON(input)
				if err != nil {
					return nil, fmt.Errorf("marshal original for update: %w", err)
				}
				normalizedJSON, err := toJSON(validated)
				if err != nil {
					return nil, fmt.Errorf("marshal normalized for update: %w", err)
				}
				warningsJSON, err := toJSON(warnings)
				if err != nil {
					return nil, fmt.Errorf("marshal warnings for update: %w", err)
				}
				_, err = s.repo.UpdateReviewQueueEntry(ctx, existingReview.ID, ReviewQueueUpdateParams{
					OriginalPayload:   &originalJSON,
					NormalizedPayload: &normalizedJSON,
					Warnings:          &warningsJSON,
				})
				if err != nil {
					return nil, fmt.Errorf("update review queue entry: %w", err)
				}
				// Return the existing event
				event, err := s.repo.GetByULID(ctx, existingReview.EventULID)
				if err != nil {
					return nil, fmt.Errorf("get event for pending update: %w", err)
				}
				return &IngestResult{Event: event, NeedsReview: true, Warnings: warnings}, nil
			}
		}
	}

	ulidValue, err := ids.NewULID()
	if err != nil {
		return nil, fmt.Errorf("generate ulid: %w", err)
	}

	// Determine lifecycle state based on whether review is needed
	lifecycleState := "published"
	if needsReview {
		lifecycleState = "pending_review"
	}
	// Determine event domain from input (set during normalization from @type)
	// or fall back to default "arts"
	eventDomain := validated.EventDomain
	if eventDomain == "" {
		eventDomain = "arts"
	}
	params := EventCreateParams{
		ULID:                ulidValue,
		Name:                validated.Name,
		Description:         validated.Description,
		LifecycleState:      lifecycleState,
		EventDomain:         eventDomain,
		OrganizerID:         nil,
		PrimaryVenueID:      nil,
		VirtualURL:          virtualURL(validated),
		ImageURL:            validated.Image,
		PublicURL:           validated.URL,
		Keywords:            validated.Keywords,
		InLanguage:          validated.InLanguage,
		IsAccessibleForFree: validated.IsAccessibleForFree,
		LicenseURL:          licenseURL(validated.License),
		LicenseStatus:       "cc0",
		Confidence:          floatPtr(reviewConfidence(validated, needsReview, s.validationConfig)),
		OriginNodeID:        nil,
	}

	if validated.Location != nil && validated.Location.Name != "" {
		placeULID, err := ids.NewULID()
		if err != nil {
			return nil, fmt.Errorf("generate place ulid: %w", err)
		}
		place, err := s.repo.UpsertPlace(ctx, PlaceCreateParams{
			EntityCreateFields: EntityCreateFields{
				ULID:            placeULID,
				Name:            validated.Location.Name,
				StreetAddress:   validated.Location.StreetAddress,
				PostalCode:      validated.Location.PostalCode,
				AddressLocality: validated.Location.AddressLocality,
				AddressRegion:   validated.Location.AddressRegion,
				AddressCountry:  validated.Location.AddressCountry,
			},
			Latitude:  float64PtrNonZero(validated.Location.Latitude),
			Longitude: float64PtrNonZero(validated.Location.Longitude),
		})
		if err != nil {
			return nil, err
		}
		params.PrimaryVenueID = &place.ID
	}

	// Layer 2: Near-duplicate detection via pg_trgm fuzzy name matching.
	// After place reconciliation, check if a similar-named event already exists
	// at the same venue on the same date. This is a soft warning — if the check
	// fails (DB error), we log and continue rather than failing the ingest.
	if params.PrimaryVenueID != nil && s.dedupConfig.NearDuplicateThreshold > 0 {
		startTime, err := time.Parse(time.RFC3339, strings.TrimSpace(validated.StartDate))
		if err == nil {
			candidates, err := s.repo.FindNearDuplicates(ctx, *params.PrimaryVenueID, startTime, validated.Name, s.dedupConfig.NearDuplicateThreshold)
			if err != nil {
				log.Warn().Err(err).
					Str("event_name", validated.Name).
					Msg("Near-duplicate check failed, continuing ingestion")
			} else if len(candidates) > 0 {
				// Build details about the matches for the review queue
				matches := make([]map[string]any, 0, len(candidates))
				for _, c := range candidates {
					matches = append(matches, map[string]any{
						"ulid":       c.ULID,
						"name":       c.Name,
						"similarity": c.Similarity,
					})
				}
				warnings = append(warnings, ValidationWarning{
					Field:   "name",
					Message: fmt.Sprintf("Potential duplicate: found %d similar event(s) at the same venue on the same date", len(candidates)),
					Code:    "potential_duplicate",
					Details: map[string]any{
						"matches": matches,
					},
				})
				needsReview = true
			}
		}
	}

	if validated.Organizer != nil && validated.Organizer.Name != "" {
		orgULID, err := ids.NewULID()
		if err != nil {
			return nil, fmt.Errorf("generate organizer ulid: %w", err)
		}
		addressLocality := ""
		addressRegion := ""
		addressCountry := ""
		if validated.Location != nil {
			addressLocality = validated.Location.AddressLocality
			addressRegion = validated.Location.AddressRegion
			addressCountry = validated.Location.AddressCountry
		}
		org, err := s.repo.UpsertOrganization(ctx, OrganizationCreateParams{
			EntityCreateFields: EntityCreateFields{
				ULID:            orgULID,
				Name:            validated.Organizer.Name,
				AddressLocality: addressLocality,
				AddressRegion:   addressRegion,
				AddressCountry:  addressCountry,
			},
			URL: validated.Organizer.URL,
		})
		if err != nil {
			return nil, err
		}
		params.OrganizerID = &org.ID
	}

	// Store the dedup hash so future ingestions can find this event
	params.DedupHash = dedupHash

	// Wrap event creation, occurrence creation, source recording, and review queue entry
	// in a transaction to ensure atomicity. If any operation fails, all changes are rolled back.
	txRepo, tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	event, err := txRepo.Create(ctx, params)
	if err != nil {
		return nil, err
	}

	if err := s.createOccurrencesWithRepo(ctx, txRepo, event, validated); err != nil {
		return nil, err
	}

	if err := s.recordSourceWithRepo(ctx, txRepo, event, validated, sourceID); err != nil {
		return nil, err
	}

	// Create review queue entry if needed
	if needsReview {
		log.Debug().
			Str("event_ulid", event.ULID).
			Str("event_name", event.Name).
			Int("warnings_count", len(warnings)).
			Msg("Creating review queue entry")

		originalJSON, err := toJSON(input)
		if err != nil {
			return nil, fmt.Errorf("marshal original payload: %w", err)
		}
		normalizedJSON, err := toJSON(validated)
		if err != nil {
			return nil, fmt.Errorf("marshal normalized payload: %w", err)
		}
		warningsJSON, err := toJSON(warnings)
		if err != nil {
			return nil, fmt.Errorf("marshal warnings: %w", err)
		}

		log.Debug().
			Str("event_ulid", event.ULID).
			Str("warnings_json", string(warningsJSON)).
			Msg("Marshaled warnings to JSON")

		var externalID *string
		if validated.Source != nil && validated.Source.EventID != "" {
			externalID = &validated.Source.EventID
		}
		var dedupHashPtr *string
		if dedupHash != "" {
			dedupHashPtr = &dedupHash
		}
		var sourceIDPtr *string
		if sourceID != "" {
			sourceIDPtr = &sourceID
		}

		startTime, endTime := parseEventTimes(validated)
		reviewEntry, err := txRepo.CreateReviewQueueEntry(ctx, ReviewQueueCreateParams{
			EventID:           event.ID, // Use UUID, not ULID
			OriginalPayload:   originalJSON,
			NormalizedPayload: normalizedJSON,
			Warnings:          warningsJSON,
			SourceID:          sourceIDPtr,
			SourceExternalID:  externalID,
			DedupHash:         dedupHashPtr,
			EventStartTime:    startTime,
			EventEndTime:      endTime,
		})
		if err != nil {
			return nil, fmt.Errorf("create review queue entry: %w", err)
		}

		log.Debug().
			Str("event_ulid", event.ULID).
			Int("review_entry_id", reviewEntry.ID).
			Int("warnings_in_db", len(reviewEntry.Warnings)).
			Msg("Created review queue entry")
	}

	if strings.TrimSpace(idempotencyKey) != "" {
		if err := txRepo.UpdateIdempotencyKeyEvent(ctx, idempotencyKey, event.ID, event.ULID); err != nil {
			return nil, err
		}
	}

	// Commit transaction - all operations succeeded
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit transaction: %w", err)
	}

	return &IngestResult{Event: event, NeedsReview: needsReview, Warnings: warnings}, nil
}

// createOccurrencesWithRepo creates occurrences using the provided repository (supports transactions)
func (s *IngestService) createOccurrencesWithRepo(ctx context.Context, repo Repository, event *Event, input EventInput) error {
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

		// SAFETY: If endDate is before startDate, don't create occurrence
		// This can happen when dates don't meet auto-correction criteria but the event
		// goes to review queue. The admin will fix the dates and then occurrence can be created.
		if end != nil && end.Before(start) {
			// Skip creating occurrence - will be created after admin review fixes the dates
			return nil
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
		if input.Offers != nil {
			occurrence.TicketURL = nullableString(input.Offers.URL)
			occurrence.PriceCurrency = input.Offers.PriceCurrency
			if price, err := parsePrice(input.Offers.Price); err == nil && price != nil {
				occurrence.PriceMin = price
				occurrence.PriceMax = price
			}
		}
		return repo.CreateOccurrence(ctx, occurrence)
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

		// SAFETY: If endDate is before startDate, skip this occurrence
		// This can happen when dates don't meet auto-correction criteria but the event
		// goes to review queue. The admin will fix the dates and then occurrence can be created.
		if end != nil && end.Before(start) {
			// Skip creating occurrence - will be created after admin review fixes the dates
			continue
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
		// Apply event-level offers to each occurrence as defaults
		if input.Offers != nil {
			occurrence.TicketURL = nullableString(input.Offers.URL)
			occurrence.PriceCurrency = input.Offers.PriceCurrency
			if price, err := parsePrice(input.Offers.Price); err == nil && price != nil {
				occurrence.PriceMin = price
				occurrence.PriceMax = price
			}
		}
		if err := repo.CreateOccurrence(ctx, occurrence); err != nil {
			return fmt.Errorf("create occurrence: %w", err)
		}
	}

	return nil
}

// recordSourceWithRepo records the source using the provided repository (supports transactions)
func (s *IngestService) recordSourceWithRepo(ctx context.Context, repo Repository, event *Event, input EventInput, sourceID string) error {
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
		SourceURL:     input.Source.URL,
		SourceEventID: input.Source.EventID,
		SourceID:      sourceID,
		Payload:       payload,
		PayloadHash:   hex.EncodeToString(payloadHash[:]),
	}

	return repo.CreateSource(ctx, params)
}

// recordSourceForExisting records a new source against an existing event during auto-merge.
// Unlike recordSourceWithRepo, this works outside a transaction and does not fail the
// ingest if source recording fails (the merge already succeeded).
func (s *IngestService) recordSourceForExisting(ctx context.Context, event *Event, input EventInput, sourceID string) error {
	if input.Source == nil || input.Source.URL == "" {
		return nil
	}

	payload, err := json.Marshal(input)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}
	payloadHash := sha256.Sum256(payload)

	return s.repo.CreateSource(ctx, EventSourceCreateParams{
		EventID:       event.ID,
		SourceID:      sourceID,
		SourceURL:     input.Source.URL,
		SourceEventID: input.Source.EventID,
		Payload:       payload,
		PayloadHash:   hex.EncodeToString(payloadHash[:]),
	})
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

// parsePrice parses a user-provided price string into a float64.
// Handles: empty → nil, "Free"/"free" → 0.0, "0" → 0.0, "25.00" → 25.0, "$25" → 25.0
func parsePrice(s string) (*float64, error) {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return nil, nil
	}
	lower := strings.ToLower(trimmed)
	if lower == "free" {
		zero := 0.0
		return &zero, nil
	}
	// Strip common currency symbols
	trimmed = strings.TrimLeft(trimmed, "$€£¥")
	trimmed = strings.TrimSpace(trimmed)
	if trimmed == "" {
		return nil, nil
	}
	v, err := strconv.ParseFloat(trimmed, 64)
	if err != nil {
		return nil, fmt.Errorf("parse price %q: %w", s, err)
	}
	return &v, nil
}

func needsReview(input EventInput, linkStatuses map[string]int, validationConfig config.ValidationConfig) bool {
	// Use zero-value defaults if config is uninitialized (RequireImage defaults to false)
	// This should never happen in practice since all callers pass initialized config,
	// but defensive check prevents potential panics.

	if reviewConfidence(input, false, validationConfig) < 0.6 {
		return true
	}
	if strings.TrimSpace(input.Description) == "" {
		return true
	}
	if validationConfig.RequireImage && strings.TrimSpace(input.Image) == "" {
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

func reviewConfidence(input EventInput, flagged bool, validationConfig config.ValidationConfig) float64 {
	confidence := 0.9
	if strings.TrimSpace(input.Description) == "" {
		confidence -= 0.2
	}
	if validationConfig.RequireImage && strings.TrimSpace(input.Image) == "" {
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

// float64PtrNonZero returns a pointer to the value if non-zero, nil otherwise.
// Used for coordinates where 0 from JSON omitempty means "not provided" rather
// than the actual coordinate 0,0 (Gulf of Guinea). Input PlaceInput uses plain
// float64 with omitempty, so zero genuinely means absent.
func float64PtrNonZero(value float64) *float64 {
	if value == 0 {
		return nil
	}
	return &value
}

// appendQualityWarnings adds synthetic validation warnings for quality issues
// that trigger review but aren't structural validation errors.
// This ensures admins can see WHY an event was flagged for review.
func appendQualityWarnings(warnings []ValidationWarning, input EventInput, linkStatuses map[string]int, validationConfig config.ValidationConfig) []ValidationWarning {
	log.Debug().
		Str("event_name", input.Name).
		Int("initial_warnings", len(warnings)).
		Str("has_description", fmt.Sprintf("%v", input.Description != "")).
		Str("has_image", fmt.Sprintf("%v", input.Image != "")).
		Msg("appendQualityWarnings: START")

	// Pre-allocate capacity for expected quality warnings to avoid reallocation:
	// - Up to 4 deterministic warnings (description, image, future date, confidence)
	// - Plus variable link check failures
	// Conservative estimate: assume max 2 link failures in typical case
	expectedCapacity := len(warnings) + 6
	result := make([]ValidationWarning, len(warnings), expectedCapacity)
	copy(result, warnings)

	// Check for missing description
	if strings.TrimSpace(input.Description) == "" {
		result = append(result, ValidationWarning{
			Field:   "description",
			Message: "Event is missing a description. A description helps users understand what the event is about.",
			Code:    "missing_description",
		})
	}

	// Check for missing image (only if configured to require it)
	if validationConfig.RequireImage && strings.TrimSpace(input.Image) == "" {
		result = append(result, ValidationWarning{
			Field:   "image",
			Message: "Event is missing an image. Images significantly improve event discoverability and appeal.",
			Code:    "missing_image",
		})
	}

	// Check for too far in future (>730 days = ~2 years)
	if isTooFarFuture(input.StartDate, 730) {
		result = append(result, ValidationWarning{
			Field:   "startDate",
			Message: "Event is scheduled more than 2 years in the future. This may indicate a data quality issue.",
			Code:    "too_far_future",
		})
	}

	// Check for low confidence score
	confidence := reviewConfidence(input, false, validationConfig)
	if confidence < 0.6 {
		result = append(result, ValidationWarning{
			Field:   "event",
			Message: fmt.Sprintf("Event has low data quality score (%.0f%%). Review recommended.", confidence*100),
			Code:    "low_confidence",
		})
	}

	// Check for failed link checks (if provided)
	for url, code := range linkStatuses {
		if code >= 400 {
			result = append(result, ValidationWarning{
				Field:   "url",
				Message: fmt.Sprintf("Link check failed for %s (HTTP %d)", url, code),
				Code:    "link_check_failed",
			})
		}
	}

	log.Debug().
		Str("event_name", input.Name).
		Int("final_warnings", len(result)).
		Int("added_warnings", len(result)-len(warnings)).
		Msg("appendQualityWarnings: END")

	return result
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

// Helper functions for review queue workflow

// stillHasSameIssues checks if the new warnings match the previously rejected warnings
func stillHasSameIssues(oldWarningsJSON []byte, newWarnings []ValidationWarning) bool {
	if len(oldWarningsJSON) == 0 {
		return len(newWarnings) == 0
	}

	var oldWarnings []ValidationWarning
	if err := json.Unmarshal(oldWarningsJSON, &oldWarnings); err != nil {
		return false
	}

	// Build maps of warning codes for comparison
	oldCodes := make(map[string]bool)
	for _, w := range oldWarnings {
		oldCodes[w.Code] = true
	}
	newCodes := make(map[string]bool)
	for _, w := range newWarnings {
		newCodes[w.Code] = true
	}

	// Check if the sets of warning codes match
	if len(oldCodes) != len(newCodes) {
		return false
	}
	for code := range oldCodes {
		if !newCodes[code] {
			return false
		}
	}
	return true
}

// isEventPast checks if an event has already ended
func isEventPast(endTime *time.Time) bool {
	if endTime == nil {
		return false
	}
	return endTime.Before(time.Now())
}

// toJSON marshals a value to JSON
func toJSON(v any) ([]byte, error) {
	return json.Marshal(v)
}

// parseEventTimes extracts start and end times from validated event input
func parseEventTimes(input EventInput) (time.Time, *time.Time) {
	start, err := time.Parse(time.RFC3339, strings.TrimSpace(input.StartDate))
	if err != nil {
		start = time.Now() // fallback, should not happen after validation
	}

	var end *time.Time
	if input.EndDate != "" {
		parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(input.EndDate))
		if err == nil {
			end = &parsed
		}
	}

	return start, end
}

// stringOrEmpty safely extracts string from pointer or returns empty string
func stringOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// timeOrZero safely extracts time from pointer or returns zero time
func timeOrZero(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}

// Helper functions shared with tests
func stringPtr(s string) *string {
	return &s
}
