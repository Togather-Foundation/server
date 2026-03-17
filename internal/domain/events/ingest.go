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
	Event         *Event
	IsDuplicate   bool
	IsMerged      bool // true when auto-merged with existing event
	NeedsReview   bool
	Warnings      []ValidationWarning
	PlaceULID     string // ULID of created/matched place (for reconciliation)
	OrganizerULID string // ULID of created/matched organization (for reconciliation)
}

// IngestOptions holds optional parameters for Ingest and IngestWithIdempotency.
// Build via the WithSourceID functional option; zero value is safe for all callers
// that do not need these overrides.
type IngestOptions struct {
	// SourceID overrides source resolution when input.Source is nil.
	// The value must be a valid UUID referencing an existing sources row.
	SourceID string
}

// IngestOption is a functional option for Ingest / IngestWithIdempotency.
type IngestOption func(*IngestOptions)

// WithSourceID sets a pre-resolved source ID on the ingest options.
// It is used only when the event input has no Source.URL — if input.Source.URL
// is present, GetOrCreateSource is called instead and this option is ignored.
// An empty id is a no-op.
func WithSourceID(id string) IngestOption {
	return func(o *IngestOptions) {
		o.SourceID = id
	}
}

type IngestService struct {
	repo             Repository
	nodeDomain       string
	defaultTZ        string
	validationConfig config.ValidationConfig
	dedupConfig      config.DedupConfig
}

func NewIngestService(repo Repository, nodeDomain string, defaultTimezone string, validationConfig config.ValidationConfig) *IngestService {
	return &IngestService{
		repo:             repo,
		nodeDomain:       nodeDomain,
		defaultTZ:        defaultTimezone,
		validationConfig: validationConfig.WithDefaults(),
	}
}

// WithDedupConfig sets the deduplication configuration and returns the service for chaining.
func (s *IngestService) WithDedupConfig(cfg config.DedupConfig) *IngestService {
	s.dedupConfig = cfg
	return s
}

func (s *IngestService) Ingest(ctx context.Context, input EventInput, opts ...IngestOption) (*IngestResult, error) {
	return s.IngestWithIdempotency(ctx, input, "", opts...)
}

func (s *IngestService) IngestWithIdempotency(ctx context.Context, input EventInput, idempotencyKey string, opts ...IngestOption) (*IngestResult, error) {
	var options IngestOptions
	for _, o := range opts {
		o(&options)
	}

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
	validationResult, err := ValidateEventInputWithWarnings(normalized, s.nodeDomain, &input, s.validationConfig)
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
	needsReview := len(warnings) > 0 || eventNeedsReview(validated, nil, s.validationConfig)

	// nearDuplicateOfID holds the UUID of the first near-duplicate candidate's event record.
	// Used to cross-link review queue entries between the new event and existing matched events.
	var nearDuplicateOfID *string
	// nearDuplicateCandidates stores candidates found during Layer 2 detection.
	// Actual flagging is deferred until after the new event is created (to use its UUID as cross-link).
	var nearDuplicateCandidates []NearDuplicateCandidate

	// Honour scraper-set lifecycle hint: "review" forces pending_review regardless
	// of other quality checks (e.g., truncated description flagged before fetching).
	if input.LifecycleState == "review" {
		needsReview = true
	}

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
			// Same source re-ingestion: merge fields to enrich/update existing event.
			// Both events share the same source, so trust levels should match —
			// the merge will primarily fill empty fields.
			existingTrust, err := s.repo.GetSourceTrustLevel(ctx, existing.ID)
			if err != nil {
				return nil, fmt.Errorf("get existing source trust for source-external-id match: %w", err)
			}
			newTrust, err := s.repo.GetSourceTrustLevelBySourceID(ctx, sourceID)
			if err != nil {
				return nil, fmt.Errorf("get new source trust for source-external-id match: %w", err)
			}

			updates, changed := AutoMergeFields(existing, validated, existingTrust, newTrust)
			if changed {
				existing, err = s.repo.UpdateEvent(ctx, existing.ULID, updates)
				if err != nil {
					return nil, fmt.Errorf("source-external-id auto-merge update: %w", err)
				}
			}

			// Record the source's contribution (may add updated payload)
			_ = s.recordSourceForExisting(ctx, existing, validated, sourceID)

			return &IngestResult{Event: existing, IsDuplicate: true, IsMerged: changed, Warnings: warnings}, nil
		}
		if err != nil && err != ErrNotFound {
			return nil, err
		}
	} else if options.SourceID != "" {
		// No source URL in payload; use the pre-resolved source ID from options
		// (e.g. derived from the authenticated API key).
		sourceID = options.SourceID
	}

	dedupHash := BuildDedupHash(DedupCandidate{
		Name:      validated.Name,
		VenueID:   NormalizeVenueKey(validated),
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

	// Check for existing review queue entry for this event.
	// This runs regardless of needsReview so we can:
	// 1. Detect previously rejected events (block resubmission)
	// 2. Auto-approve pending reviews when a resubmission has no warnings (S5.1)
	// 3. Update pending reviews with new payloads when still has issues
	{
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
				// Already in queue - check if the resubmission fixed the issues
				if len(warnings) == 0 {
					// Fixed! Approve and publish (S5.1 auto-approve path)
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

	// Track place/org ULIDs for downstream reconciliation jobs
	var placeULID, orgULID string

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

	if validated.Location != nil {
		locID := locationID(validated.Location)

		if locID != "" {
			// --- Canonical @id path ---
			// location.@id is authoritative: resolve the place by ULID from the URI.
			// This completely bypasses UpsertPlace and name-based matching.
			parsed, parseErr := ids.ParseEntityURI(s.nodeDomain, "places", locID, "")
			if parseErr != nil {
				return nil, fmt.Errorf("ingest: invalid parent location.@id %q: %w", locID, parseErr)
			}
			placeRecord, lookupErr := s.repo.GetPlaceByULID(ctx, parsed.ULID)
			if lookupErr != nil {
				if lookupErr == ErrNotFound {
					return nil, ValidationError{
						Field:   "location.@id",
						Message: fmt.Sprintf("canonical place %q not found; ensure the place exists before referencing it by @id", locID),
					}
				}
				return nil, fmt.Errorf("ingest: resolve parent location.@id %q: %w", locID, lookupErr)
			}

			// If name is also supplied, it must match the canonical record to prevent
			// silent venue misattribution (e.g. typo in @id attaching the wrong place).
			if locationName := strings.TrimSpace(validated.Location.Name); locationName != "" {
				if !placeNamesMatch(locationName, placeRecord.Name) {
					return nil, ValidationError{
						Field: "location.name",
						Message: fmt.Sprintf(
							"location.name %q does not match the canonical place name %q for @id %s; "+
								"either omit name or correct it to match the referenced place",
							locationName, placeRecord.Name, locID,
						),
					}
				}
			}

			params.PrimaryVenueID = &placeRecord.ID
			placeULID = placeRecord.ULID
		} else if strings.TrimSpace(validated.Location.Name) != "" {
			// --- Name-based path (no @id) ---
			// Resolve or create the place by name via UpsertPlace.
			generatedPlaceULID, err := ids.NewULID()
			if err != nil {
				return nil, fmt.Errorf("generate place ulid: %w", err)
			}
			place, err := s.repo.UpsertPlace(ctx, PlaceCreateParams{
				EntityCreateFields: EntityCreateFields{
					ULID:            generatedPlaceULID,
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
			placeULID = place.ULID

			// Layer 3: Fuzzy place dedup. Only check when a NEW place was just created
			// (returned ULID matches our generated one) — if UpsertPlace returned an
			// existing record, the names already matched exactly under normalization.
			if place.ULID == generatedPlaceULID && s.dedupConfig.PlaceReviewThreshold > 0 {
				placeCandidates, err := s.repo.FindSimilarPlaces(ctx,
					validated.Location.Name,
					validated.Location.AddressLocality,
					validated.Location.AddressRegion,
					s.dedupConfig.PlaceReviewThreshold,
				)
				if err != nil {
					log.Warn().Err(err).
						Str("place_name", validated.Location.Name).
						Msg("Place similarity check failed, continuing ingestion")
				} else {
					// Filter out self-match (the place we just created)
					var filtered []SimilarPlaceCandidate
					for _, c := range placeCandidates {
						if c.ID != place.ID {
							filtered = append(filtered, c)
						}
					}
					if len(filtered) > 0 {
						best := filtered[0] // highest similarity, sorted DESC
						if best.Similarity >= s.dedupConfig.PlaceAutoMergeThreshold {
							// Auto-merge: the new place is almost certainly the same.
							// Merge new into existing (existing is primary).
							mergeResult, mergeErr := s.repo.MergePlaces(ctx, place.ID, best.ID)
							if mergeErr != nil {
								log.Warn().Err(mergeErr).
									Str("duplicate_place", place.ULID).
									Str("primary_place", best.ULID).
									Msg("Place auto-merge failed, continuing with new place")
							} else {
								if mergeResult.AlreadyMerged {
									log.Info().
										Str("duplicate_place", place.ULID).
										Str("canonical_place", mergeResult.CanonicalID).
										Msg("Place already merged by concurrent operation, using canonical")
								} else {
									log.Info().
										Str("duplicate_place", place.ULID).
										Str("primary_place", best.ULID).
										Float64("similarity", best.Similarity).
										Msg("Auto-merged duplicate place")
								}
								// Use the canonical place for this event
								params.PrimaryVenueID = &mergeResult.CanonicalID
							}
						} else {
							// Below auto-merge but above review threshold — flag for review
							matches := make([]map[string]any, 0, len(filtered))
							for _, c := range filtered {
								match := map[string]any{
									"ulid":       c.ULID,
									"name":       c.Name,
									"similarity": c.Similarity,
								}
								if c.AddressStreet != nil {
									match["address_street"] = *c.AddressStreet
								}
								if c.AddressLocality != nil {
									match["address_locality"] = *c.AddressLocality
								}
								if c.AddressRegion != nil {
									match["address_region"] = *c.AddressRegion
								}
								if c.PostalCode != nil {
									match["postal_code"] = *c.PostalCode
								}
								if c.URL != nil {
									match["url"] = *c.URL
								}
								if c.Telephone != nil {
									match["telephone"] = *c.Telephone
								}
								if c.Email != nil {
									match["email"] = *c.Email
								}
								matches = append(matches, match)
							}
							// NOTE: PlaceInput intentionally does not carry url, telephone, or email.
							// Venue data attached to event ingest inputs is a location stub (name + address)
							// not a full place record. Contact info is not captured at ingest time.
							// The frontend sets url/telephone/email to null for the "new place" diff card,
							// which prevents false "missing" highlights on fields that were never available.
							placeDetails := map[string]any{
								"matches":        matches,
								"new_place_ulid": place.ULID,
								"new_place_name": validated.Location.Name,
							}
							if validated.Location.StreetAddress != "" {
								placeDetails["new_place_street"] = validated.Location.StreetAddress
							}
							if validated.Location.AddressLocality != "" {
								placeDetails["new_place_locality"] = validated.Location.AddressLocality
							}
							if validated.Location.AddressRegion != "" {
								placeDetails["new_place_region"] = validated.Location.AddressRegion
							}
							if validated.Location.PostalCode != "" {
								placeDetails["new_place_postal_code"] = validated.Location.PostalCode
							}
							warnings = append(warnings, ValidationWarning{
								Field:   "location.name",
								Message: fmt.Sprintf("Possible duplicate place: found %d similar place(s) in the same area", len(filtered)),
								Code:    "place_possible_duplicate",
								Details: placeDetails,
							})
							needsReview = true
						}
					}
				}
			} // end fuzzy dedup block
		} // end else if name != ""
	} // end if validated.Location != nil

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
				// Filter out candidates previously marked as not-duplicates of this event.
				// This prevents re-flagging pairs that an admin already reviewed and confirmed
				// are distinct events. Uses the new event's ULID (generated above at ulidValue).
				filtered := make([]NearDuplicateCandidate, 0, len(candidates))
				for _, c := range candidates {
					notDup, err := s.repo.IsNotDuplicate(ctx, ulidValue, c.ULID)
					if err != nil {
						log.Warn().Err(err).
							Str("event_ulid", ulidValue).
							Str("candidate_ulid", c.ULID).
							Msg("Not-duplicate check failed, keeping candidate")
						filtered = append(filtered, c)
					} else if !notDup {
						filtered = append(filtered, c)
					}
				}
				candidates = filtered

				if len(candidates) > 0 {
					// Build details about the matches for the review queue
					matches := make([]map[string]any, 0, len(candidates))
					for _, c := range candidates {
						match := map[string]any{
							"ulid":       c.ULID,
							"name":       c.Name,
							"similarity": c.Similarity,
						}
						if c.StartDate != "" {
							match["startDate"] = c.StartDate
						}
						if c.EndDate != "" {
							match["endDate"] = c.EndDate
						}
						if c.VenueName != "" {
							match["location"] = map[string]any{"name": c.VenueName}
						}
						matches = append(matches, match)
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
					// Store candidates for cross-linking review entries after the new event is created
					// (we need the new event's UUID, which is not available until after txRepo.Create).
					nearDuplicateCandidates = candidates
				}
			}
		}
	}

	if validated.Organizer != nil && validated.Organizer.Name != "" {
		generatedOrgULID, err := ids.NewULID()
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
				ULID:            generatedOrgULID,
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
		orgULID = org.ULID

		// Layer 3: Fuzzy organization dedup. Same pattern as places — only check
		// when a NEW org was just created (returned ULID matches our generated one).
		if org.ULID == generatedOrgULID && s.dedupConfig.OrgReviewThreshold > 0 {
			orgCandidates, err := s.repo.FindSimilarOrganizations(ctx,
				validated.Organizer.Name,
				addressLocality,
				addressRegion,
				s.dedupConfig.OrgReviewThreshold,
			)
			if err != nil {
				log.Warn().Err(err).
					Str("org_name", validated.Organizer.Name).
					Msg("Organization similarity check failed, continuing ingestion")
			} else {
				// Filter out self-match (the org we just created)
				var filtered []SimilarOrgCandidate
				for _, c := range orgCandidates {
					if c.ID != org.ID {
						filtered = append(filtered, c)
					}
				}
				if len(filtered) > 0 {
					best := filtered[0] // highest similarity, sorted DESC
					if best.Similarity >= s.dedupConfig.OrgAutoMergeThreshold {
						// Auto-merge: the new org is almost certainly the same.
						mergeResult, mergeErr := s.repo.MergeOrganizations(ctx, org.ID, best.ID)
						if mergeErr != nil {
							log.Warn().Err(mergeErr).
								Str("duplicate_org", org.ULID).
								Str("primary_org", best.ULID).
								Msg("Organization auto-merge failed, continuing with new org")
						} else {
							if mergeResult.AlreadyMerged {
								log.Info().
									Str("duplicate_org", org.ULID).
									Str("canonical_org", mergeResult.CanonicalID).
									Msg("Organization already merged by concurrent operation, using canonical")
							} else {
								log.Info().
									Str("duplicate_org", org.ULID).
									Str("primary_org", best.ULID).
									Float64("similarity", best.Similarity).
									Msg("Auto-merged duplicate organization")
							}
							// Use the canonical org for this event
							params.OrganizerID = &mergeResult.CanonicalID
						}
					} else {
						// Below auto-merge but above review threshold — flag for review
						matches := make([]map[string]any, 0, len(filtered))
						for _, c := range filtered {
							match := map[string]any{
								"ulid":       c.ULID,
								"name":       c.Name,
								"similarity": c.Similarity,
							}
							if c.AddressLocality != nil {
								match["address_locality"] = *c.AddressLocality
							}
							if c.AddressRegion != nil {
								match["address_region"] = *c.AddressRegion
							}
							if c.URL != nil {
								match["url"] = *c.URL
							}
							if c.Telephone != nil {
								match["telephone"] = *c.Telephone
							}
							if c.Email != nil {
								match["email"] = *c.Email
							}
							matches = append(matches, match)
						}
						orgDetails := map[string]any{
							"matches":      matches,
							"new_org_ulid": org.ULID,
							"new_org_name": validated.Organizer.Name,
						}
						if addressLocality != "" {
							orgDetails["new_org_locality"] = addressLocality
						}
						if addressRegion != "" {
							orgDetails["new_org_region"] = addressRegion
						}
						if validated.Organizer.URL != "" {
							orgDetails["new_org_url"] = validated.Organizer.URL
						}
						if validated.Organizer.Email != "" {
							orgDetails["new_org_email"] = validated.Organizer.Email
						}
						if validated.Organizer.Telephone != "" {
							orgDetails["new_org_telephone"] = validated.Organizer.Telephone
						}
						warnings = append(warnings, ValidationWarning{
							Field:   "organizer.name",
							Message: fmt.Sprintf("Possible duplicate organization: found %d similar org(s) in the same area", len(filtered)),
							Code:    "org_possible_duplicate",
							Details: orgDetails,
						})
						needsReview = true
					}
				}
			}
		}
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

	// Re-evaluate lifecycle state: needsReview may have been updated by near-duplicate detection
	// (Layer 2) or other checks that run after the initial params were built.
	if needsReview {
		params.LifecycleState = "pending_review"
	}

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

	// Capture first near-duplicate candidate's UUID for cross-linking in the new event's
	// review entry (created below inside the transaction).
	if len(nearDuplicateCandidates) > 0 {
		existingEvent, fetchErr := s.repo.GetByULID(ctx, nearDuplicateCandidates[0].ULID)
		if fetchErr == nil {
			existingID := existingEvent.ID
			nearDuplicateOfID = &existingID
		}
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
			EventID:            event.ID, // Use UUID, not ULID
			OriginalPayload:    originalJSON,
			NormalizedPayload:  normalizedJSON,
			Warnings:           warningsJSON,
			SourceID:           sourceIDPtr,
			SourceExternalID:   externalID,
			DedupHash:          dedupHashPtr,
			EventStartTime:     startTime,
			EventEndTime:       endTime,
			DuplicateOfEventID: nearDuplicateOfID,
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

	// Flag near-duplicate existing events for review AFTER the transaction commits.
	// This must happen post-commit because the review queue entry for existing events
	// cross-references the new event's UUID via duplicate_of_event_id (FK constraint).
	// Non-critical: errors are logged and skipped — the new event is already committed.
	if len(nearDuplicateCandidates) > 0 {
		pendingState := "pending_review"
		for _, c := range nearDuplicateCandidates {
			existingEvent, fetchErr := s.repo.GetByULID(ctx, c.ULID)
			if fetchErr != nil {
				log.Warn().Err(fetchErr).
					Str("candidate_ulid", c.ULID).
					Msg("Near-duplicate: failed to fetch existing event for review flagging, skipping")
				continue
			}

			// Only flag events that are currently published (don't re-flag already-pending or closed events).
			if existingEvent.LifecycleState == "published" {
				if _, updateErr := s.repo.UpdateEvent(ctx, c.ULID, UpdateEventParams{
					LifecycleState: &pendingState,
				}); updateErr != nil {
					log.Warn().Err(updateErr).
						Str("candidate_ulid", c.ULID).
						Msg("Near-duplicate: failed to update existing event lifecycle state, skipping")
				}
			}

			// Create review queue entry for the existing event, cross-linked to the new event's UUID.
			existingStart, existingEnd := parseEventTimesFromEvent(existingEvent)
			newEventID := event.ID

			// Reconstruct payloads from stored event data so reviewers can compare.
			reconstructedPayload, payloadErr := reconstructPayloadFromEvent(existingEvent)
			if payloadErr != nil {
				log.Warn().Err(payloadErr).
					Str("candidate_ulid", c.ULID).
					Msg("Near-duplicate: failed to reconstruct payload for existing event, using empty")
				reconstructedPayload = []byte("{}")
			}
			existingWarnings, warnErr := nearDuplicateWarnings(existingEvent, event.ULID, nearDupNewEventData{
				Name:      validated.Name,
				StartDate: validated.StartDate,
				EndDate:   validated.EndDate,
				VenueName: func() string {
					if validated.Location != nil {
						return validated.Location.Name
					}
					return ""
				}(),
			})
			if warnErr != nil {
				log.Warn().Err(warnErr).
					Str("candidate_ulid", c.ULID).
					Msg("Near-duplicate: failed to generate warnings for existing event, using empty")
				existingWarnings = []byte("[]")
			}

			if _, createErr := s.repo.CreateReviewQueueEntry(ctx, ReviewQueueCreateParams{
				EventID:            existingEvent.ID,
				OriginalPayload:    reconstructedPayload,
				NormalizedPayload:  reconstructedPayload, // same as original for reconstructed data
				EventStartTime:     existingStart,
				EventEndTime:       existingEnd,
				Warnings:           existingWarnings,
				DuplicateOfEventID: &newEventID,
			}); createErr != nil {
				log.Warn().Err(createErr).
					Str("candidate_ulid", c.ULID).
					Msg("Near-duplicate: failed to create review queue entry for existing event (may already exist), skipping")
			}
		}
	}

	return &IngestResult{Event: event, NeedsReview: needsReview, Warnings: warnings, PlaceULID: placeULID, OrganizerULID: orgULID}, nil
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
		// Only set a virtual URL when the occurrence has no physical venue.
		// If both a physical location and a virtualLocation were supplied on the
		// parent event, using the physical venue takes priority — setting virtual
		// here too would create a hybrid occurrence (venue_id AND virtual_url set),
		// violating the location contract (an occurrence is either physical or virtual).
		var virtual *string
		if venueID == nil {
			virtual = nullableString(virtualURL(input))
			if virtual == nil && event.VirtualURL != "" {
				virtual = nullableString(event.VirtualURL)
			}
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

	for i, occ := range input.Occurrences {
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
		// Inherit venue/virtual from the parent event when the occurrence
		// doesn't specify its own — mirrors the single-occurrence path above.
		//
		// occ.VenueID is a canonical place URI (e.g. https://host/places/<ULID>).
		// The DB column event_occurrences.venue_id is a UUID.
		// Resolve: parse ULID from URI → look up place row UUID via GetPlaceByULID.
		var venueID *string
		if occ.VenueID != "" {
			parsed, parseErr := ids.ParseEntityURI(s.nodeDomain, "places", occ.VenueID, "")
			if parseErr != nil {
				return fmt.Errorf("create occurrence[%d]: invalid venueId canonical URI %q: %w", i, occ.VenueID, parseErr)
			}
			placeRecord, lookupErr := repo.GetPlaceByULID(ctx, parsed.ULID)
			if lookupErr != nil {
				return fmt.Errorf("create occurrence[%d]: venueId %q not found (place ULID %s): %w", i, occ.VenueID, parsed.ULID, lookupErr)
			}
			venueID = &placeRecord.ID
		}
		if venueID == nil {
			venueID = event.PrimaryVenueID
		}
		virtual := nullableString(occ.VirtualURL)
		// Only inherit the parent's virtualURL when the occurrence has no physical
		// venue of its own. Inheriting it unconditionally creates hybrid occurrences
		// that carry both a physical venue_id and a virtual_url, violating the
		// location contract (an occurrence is either physical or virtual, not both).
		// This mirrors the single-occurrence path where virtualURL is only set when
		// venueID is nil.
		if virtual == nil && venueID == nil && event.VirtualURL != "" {
			virtual = nullableString(event.VirtualURL)
		}
		// Guard: DB requires venue_id OR virtual_url on every occurrence row.
		// Validation should have caught this already, but defend here so a
		// programming error or race doesn't produce a cryptic DB constraint failure.
		if venueID == nil && virtual == nil {
			return fmt.Errorf("create occurrence[%d]: no venue or virtual URL resolved (occurrence has no venueId/virtualUrl and parent event has no location)", i)
		}
		occurrence := OccurrenceCreateParams{
			EventID:    event.ID,
			StartTime:  start,
			EndTime:    end,
			Timezone:   tz,
			DoorTime:   door,
			VenueID:    venueID,
			VirtualURL: virtual,
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

// primaryVenueKey returns the raw venue key from the input without normalization.
// Deprecated: Use NormalizeVenueKey for dedup hashing. This function is kept for
// backward compatibility in non-dedup contexts.
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

// placeNamesMatch reports whether the submitted name agrees with the canonical
// place name.  The comparison is case-insensitive and trims leading/trailing
// whitespace so minor casing differences don't produce false mismatches.
// This is intentionally lenient — callers should only reject on a mismatch, not
// require an exact byte-for-byte match.
func placeNamesMatch(submitted, canonical string) bool {
	return strings.EqualFold(strings.TrimSpace(submitted), strings.TrimSpace(canonical))
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

func eventNeedsReview(input EventInput, linkStatuses map[string]int, validationConfig config.ValidationConfig) bool {
	// Use zero-value defaults if config is uninitialized (RequireImage defaults to false)
	// This should never happen in practice since all callers pass initialized config,
	// but defensive check prevents potential panics.

	if reviewConfidence(input, false, validationConfig) < validationConfig.ReviewConfidenceThreshold {
		return true
	}
	if strings.TrimSpace(input.Description) == "" {
		return true
	}
	if validationConfig.RequireImage && strings.TrimSpace(input.Image) == "" {
		return true
	}
	if isTooFarFuture(input.StartDate, validationConfig.MaxFutureDays) {
		return true
	}
	if !input.SkipMultiSessionCheck {
		if isMulti, _ := IsMultiSessionEvent(input); isMulti {
			return true
		}
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
	if isTooFarFuture(input.StartDate, validationConfig.MaxFutureDays) {
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

	// Check for too far in future (> MaxFutureDays)
	if isTooFarFuture(input.StartDate, validationConfig.MaxFutureDays) {
		result = append(result, ValidationWarning{
			Field:   "startDate",
			Message: "Event is scheduled more than 2 years in the future. This may indicate a data quality issue.",
			Code:    "too_far_future",
		})
	}

	// Check for low confidence score
	confidence := reviewConfidence(input, false, validationConfig)
	if confidence < validationConfig.ReviewConfidenceThreshold {
		result = append(result, ValidationWarning{
			Field:   "event",
			Message: fmt.Sprintf("Event has low data quality score (%.0f%%). Review recommended.", confidence*100),
			Code:    "low_confidence",
		})
	}

	// Check for multi-session / recurring events
	if !input.SkipMultiSessionCheck {
		if isMulti, reason := IsMultiSessionEvent(input); isMulti {
			result = append(result, ValidationWarning{
				Field:   "event",
				Message: fmt.Sprintf("Event appears to be a multi-session or recurring event: %s. Review recommended to split into individual occurrences or confirm as single event.", reason),
				Code:    "multi_session_likely",
			})
		}
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

// reconstructPayloadFromEvent builds a JSON representation of a stored event
// for use in review queue entries. This is NOT the original EventInput — it's
// a "reconstructed snapshot" containing the event's current stored data so
// reviewers can compare near-duplicate pairs side-by-side.
func reconstructPayloadFromEvent(event *Event) ([]byte, error) {
	if event == nil {
		return []byte("{}"), fmt.Errorf("reconstruct payload: nil event")
	}
	payload := map[string]any{
		"_reconstructed": true, // flag so UI knows this isn't an original submission
		"name":           event.Name,
	}

	if event.Description != "" {
		payload["description"] = event.Description
	}
	if event.ImageURL != "" {
		payload["image"] = event.ImageURL
	}
	if event.PublicURL != "" {
		payload["url"] = event.PublicURL
	}
	if event.VirtualURL != "" {
		payload["virtual_url"] = event.VirtualURL
	}
	if len(event.Keywords) > 0 {
		payload["keywords"] = event.Keywords
	}
	if len(event.InLanguage) > 0 {
		payload["in_language"] = event.InLanguage
	}
	if event.IsAccessibleForFree != nil {
		payload["is_accessible_for_free"] = *event.IsAccessibleForFree
	}
	if event.AttendanceMode != "" {
		payload["attendance_mode"] = event.AttendanceMode
	}
	if event.EventStatus != "" {
		payload["event_status"] = event.EventStatus
	}
	if event.EventDomain != "" {
		payload["event_domain"] = event.EventDomain
	}

	// Include occurrence data (schedule)
	if len(event.Occurrences) > 0 {
		occs := make([]map[string]any, 0, len(event.Occurrences))
		for _, occ := range event.Occurrences {
			o := map[string]any{
				"start_date": occ.StartTime.Format(time.RFC3339),
			}
			if occ.EndTime != nil {
				o["end_date"] = occ.EndTime.Format(time.RFC3339)
			}
			if occ.Timezone != "" {
				o["timezone"] = occ.Timezone
			}
			if occ.DoorTime != nil {
				o["door_time"] = occ.DoorTime.Format(time.RFC3339)
			}
			if occ.VirtualURL != nil && *occ.VirtualURL != "" {
				o["virtual_url"] = *occ.VirtualURL
			}
			if occ.TicketURL != "" {
				o["ticket_url"] = occ.TicketURL
			}
			if occ.PriceMin != nil {
				o["price_min"] = *occ.PriceMin
			}
			if occ.PriceMax != nil {
				o["price_max"] = *occ.PriceMax
			}
			if occ.PriceCurrency != "" {
				o["price_currency"] = occ.PriceCurrency
			}
			if occ.Availability != "" {
				o["availability"] = occ.Availability
			}
			occs = append(occs, o)
		}
		payload["occurrences"] = occs
	}

	// Include identifiers for cross-referencing
	payload["ulid"] = event.ULID
	payload["lifecycle_state"] = event.LifecycleState
	if event.DedupHash != "" {
		payload["dedup_hash"] = event.DedupHash
	}
	if event.PrimaryVenueID != nil {
		payload["primary_venue_id"] = *event.PrimaryVenueID
	}
	if event.PrimaryVenueULID != nil {
		payload["primary_venue_ulid"] = *event.PrimaryVenueULID
	}
	if event.OrganizerID != nil {
		payload["organizer_id"] = *event.OrganizerID
	}

	// Emit top-level startDate/endDate and location.name in camelCase so the review
	// queue frontend (extractMergeFields) can display date and venue without needing
	// to parse the occurrences array.
	if len(event.Occurrences) > 0 {
		first := event.Occurrences[0]
		payload["startDate"] = first.StartTime.UTC().Format(time.RFC3339)
		if first.EndTime != nil {
			payload["endDate"] = first.EndTime.UTC().Format(time.RFC3339)
		}
	}
	if event.PrimaryVenueName != nil && *event.PrimaryVenueName != "" {
		payload["location"] = map[string]any{"name": *event.PrimaryVenueName}
	}

	return json.Marshal(payload)
}

// nearDupNewEventData holds the data to embed from the newly-ingested event
// so the review queue can render a side-by-side diff card.
type nearDupNewEventData struct {
	Name      string
	StartDate string // RFC3339, empty if unknown
	EndDate   string // RFC3339, empty if unknown
	VenueName string // empty if unknown
}

// nearDuplicateWarnings generates validation warnings for an existing event
// being flagged as a near-duplicate of a newly ingested event.
func nearDuplicateWarnings(existingEvent *Event, newEventULID string, newEvent nearDupNewEventData) ([]byte, error) {
	msg := fmt.Sprintf("This existing event may be a near-duplicate of newly ingested event %s", newEventULID)
	if existingEvent != nil && existingEvent.Name != "" {
		msg = fmt.Sprintf("Existing event %q may be a near-duplicate of newly ingested event %s", existingEvent.Name, newEventULID)
	}

	details := map[string]any{
		"new_event_name": newEvent.Name,
	}
	if newEvent.StartDate != "" {
		details["new_event_startDate"] = newEvent.StartDate
	}
	if newEvent.EndDate != "" {
		details["new_event_endDate"] = newEvent.EndDate
	}
	if newEvent.VenueName != "" {
		details["new_event_venue"] = newEvent.VenueName
	}

	warnings := []ValidationWarning{
		{
			Field:   "near_duplicate",
			Code:    "near_duplicate_of_new_event",
			Message: msg,
			Details: details,
		},
	}
	return json.Marshal(warnings)
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

// parseEventTimesFromEvent extracts start/end times from an existing Event's first occurrence.
// Used when creating review queue entries for existing near-duplicate events.
func parseEventTimesFromEvent(event *Event) (time.Time, *time.Time) {
	if len(event.Occurrences) == 0 {
		return time.Now(), nil
	}
	occ := event.Occurrences[0]
	return occ.StartTime, occ.EndTime
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
