package events

// create_event_core.go — shared event-creation pipeline.
//
// createEventCore is a method on IngestService that encapsulates the core logic
// for normalising, validating, deduplicating and persisting a new event.  It is
// called by both:
//
//   - IngestWithIdempotency — the standard ingest path (SkipDedupAutoMerge=false,
//     TxRepo=nil, ExcludeFromNearDup=nil).
//   - AdminService.Consolidate — the admin create path (SkipDedupAutoMerge=true,
//     TxRepo=caller's tx, ExcludeFromNearDup=retire list).
//
// This file only contains the shared types and the createEventCore function.
// The post-commit near-dup cross-linking helper (used by IngestWithIdempotency)
// lives in ingest.go so it can stay close to its only caller there.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Togather-Foundation/server/internal/domain/ids"
	"github.com/rs/zerolog/log"
)

// CreateEventCoreOptions controls behavioural differences between callers.
type CreateEventCoreOptions struct {
	// SkipDedupAutoMerge: when true, a Layer 1 dedup hash match does NOT return
	// the existing event.  Instead, the new event is still created and the match
	// is recorded as an "exact_duplicate" warning.  Used by Consolidate — the
	// admin has explicitly chosen to create this canonical event.
	SkipDedupAutoMerge bool

	// ExcludeFromNearDup is a set of event ULIDs to filter out of near-dup
	// results.  Used by Consolidate to exclude the events being retired from
	// the near-dup warning list.
	ExcludeFromNearDup []string

	// TxRepo, when non-nil, is used for all DB operations instead of opening a
	// new transaction.  The caller owns commit/rollback.
	// When nil, createEventCore begins and commits its own transaction and
	// returns NearDuplicateCandidates for the caller to cross-link post-commit.
	TxRepo Repository

	// SourceID is a pre-resolved source UUID (same as WithSourceID ingest
	// option).  Ignored when input.Source.URL is present.
	SourceID string

	// IdempotencyKey, when non-empty, is updated with the new event's ID/ULID
	// inside the same transaction as the event creation.  Only meaningful when
	// TxRepo is nil (caller-owned transactions should update the key themselves).
	IdempotencyKey string
}

// CreateEventCoreResult extends IngestResult with additional data that callers
// may need after the function returns.
type CreateEventCoreResult struct {
	*IngestResult
	// NearDuplicateCandidates holds the raw candidates found during Layer 2
	// detection.  When TxRepo is nil, createEventCore performs the post-commit
	// cross-linking itself and this slice is always empty on return.
	// When TxRepo is non-nil, cross-linking is deferred; the caller must use
	// this slice to flag existing events after committing.
	NearDuplicateCandidates []NearDuplicateCandidate
	// DedupHash is the Layer 1 hash computed for the new event.
	DedupHash string
}

// createEventCore runs the shared event-creation pipeline.
//
// See CreateEventCoreOptions for the behavioural knobs available to callers.
//
// The function returns ErrPreviouslyRejected when the review queue contains a
// rejected entry that still applies to this submission.  Any other non-nil
// error means the event was not created.
func (s *IngestService) createEventCore(
	ctx context.Context,
	input EventInput,
	opts CreateEventCoreOptions,
) (*CreateEventCoreResult, error) {
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf("createEventCore: repository not configured")
	}

	// ── 1. Normalize then validate ───────────────────────────────────────────
	normalized := NormalizeEventInput(input)
	validationResult, err := ValidateEventInputWithWarnings(normalized, s.nodeDomain, &input, s.validationConfig)
	if err != nil {
		return nil, err
	}
	validated := validationResult.Input
	warnings := validationResult.Warnings

	log.Debug().
		Str("event_name", validated.Name).
		Int("validation_warnings", len(warnings)).
		Msg("createEventCore: after validation")

	// ── 2. Quality warnings ──────────────────────────────────────────────────
	warnings = appendQualityWarnings(warnings, validated, nil, s.validationConfig)

	log.Debug().
		Str("event_name", validated.Name).
		Int("total_warnings", len(warnings)).
		Msg("createEventCore: after quality warnings")

	// Check if review is needed
	needsReview := len(warnings) > 0 || eventNeedsReview(validated, nil, s.validationConfig)

	// Honour scraper-set lifecycle hint
	if input.LifecycleState == "review" {
		needsReview = true
	}

	// nearDuplicateCandidates is populated during Layer 2 detection; actual
	// cross-linking is deferred until after the new event is persisted.
	var nearDuplicateCandidates []NearDuplicateCandidate
	var nearDuplicateOfID *string

	// ── 3. Source resolution ─────────────────────────────────────────────────
	sourceID := opts.SourceID
	if validated.Source != nil && validated.Source.URL != "" {
		resolvedID, err := s.repo.GetOrCreateSource(ctx, SourceLookupParams{
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
		sourceID = resolvedID

		existing, err := s.repo.FindBySourceExternalID(ctx, sourceID, validated.Source.EventID)
		if err == nil && existing != nil {
			if opts.SkipDedupAutoMerge {
				// Warn instead of auto-merging
				warnings = append(warnings, ValidationWarning{
					Code:    "exact_duplicate",
					Message: fmt.Sprintf("source external-ID match with existing event %s — review recommended", existing.ULID),
				})
				needsReview = true
			} else {
				// Standard ingest: auto-merge
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
				_ = s.recordSourceForExisting(ctx, existing, validated, sourceID)
				return &CreateEventCoreResult{
					IngestResult: &IngestResult{Event: existing, IsDuplicate: true, IsMerged: changed, Warnings: warnings},
				}, nil
			}
		} else if err != nil && err != ErrNotFound {
			return nil, err
		}
	}

	// ── 4. Layer 1 dedup hash ────────────────────────────────────────────────
	dedupHash := BuildDedupHash(DedupCandidate{
		Name:      validated.Name,
		VenueID:   NormalizeVenueKey(validated),
		StartDate: validated.StartDate,
	})
	isDuplicate := false
	if dedupHash != "" {
		existing, err := s.repo.FindByDedupHash(ctx, dedupHash)
		if err == nil && existing != nil {
			if opts.SkipDedupAutoMerge {
				// Consolidation path: check if the match is one of the excluded ULIDs
				excluded := false
				for _, excl := range opts.ExcludeFromNearDup {
					if excl == existing.ULID {
						excluded = true
						break
					}
				}
				if !excluded {
					isDuplicate = true
					needsReview = true
					warnings = append(warnings, ValidationWarning{
						Code:    "exact_duplicate",
						Message: fmt.Sprintf("exact dedup-hash match with existing event %s — review recommended", existing.ULID),
					})
				}
				// Fall through — create the event anyway
			} else {
				// Standard ingest: auto-merge
				existingTrust, err := s.repo.GetSourceTrustLevel(ctx, existing.ID)
				if err != nil {
					return nil, fmt.Errorf("get existing source trust: %w", err)
				}
				newTrust := 5
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
				if sourceID != "" {
					_ = s.recordSourceForExisting(ctx, existing, validated, sourceID)
				}
				return &CreateEventCoreResult{
					IngestResult: &IngestResult{Event: existing, IsDuplicate: true, IsMerged: changed, Warnings: warnings},
					DedupHash:    dedupHash,
				}, nil
			}
		} else if err != nil && err != ErrNotFound {
			return nil, err
		}
	}

	// ── 5. Review queue re-check ─────────────────────────────────────────────
	// Skip for consolidation path (SkipDedupAutoMerge=true): the admin is
	// explicitly creating a canonical replacement; don't block on prior rejections
	// or update stale queue entries.
	if !opts.SkipDedupAutoMerge {
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
				if !isEventPast(existingReview.EventEndTime) {
					if stillHasSameIssues(existingReview.Warnings, warnings) {
						return nil, ErrPreviouslyRejected{
							Reason:     stringOrEmpty(existingReview.RejectionReason),
							ReviewedAt: timeOrZero(existingReview.ReviewedAt),
							ReviewedBy: stringOrEmpty(existingReview.ReviewedBy),
						}
					}
				}
			case "pending":
				if len(warnings) == 0 {
					_, err := s.repo.ApproveReview(ctx, existingReview.ID, "system", stringPtr("Auto-approved: resubmission with no warnings"))
					if err != nil {
						return nil, fmt.Errorf("approve review: %w", err)
					}
					updatedEvent, err := s.repo.UpdateEvent(ctx, existingReview.EventULID, UpdateEventParams{
						LifecycleState: stringPtr("published"),
					})
					if err != nil {
						return nil, fmt.Errorf("update event to published: %w", err)
					}
					return &CreateEventCoreResult{
						IngestResult: &IngestResult{Event: updatedEvent, NeedsReview: false, Warnings: nil},
					}, nil
				}
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
				event, err := s.repo.GetByULID(ctx, existingReview.EventULID)
				if err != nil {
					return nil, fmt.Errorf("get event for pending update: %w", err)
				}
				return &CreateEventCoreResult{
					IngestResult: &IngestResult{Event: event, NeedsReview: true, Warnings: warnings},
				}, nil
			}
		}
	}

	// ── 6. Generate ULID ─────────────────────────────────────────────────────
	ulidValue, err := ids.NewULID()
	if err != nil {
		return nil, fmt.Errorf("generate ulid: %w", err)
	}

	var placeULID, orgULID string
	lifecycleState := "published"
	if needsReview {
		lifecycleState = "pending_review"
	}

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

	// ── 7. Place resolution (Layer 1 + 2, optional Layer 3 fuzzy dedup) ──────
	if validated.Location != nil {
		locID := locationID(validated.Location)

		if locID != "" && s.nodeDomain != "" {
			// Canonical @id path
			parsed, parseErr := ids.ParseEntityURI(s.nodeDomain, "places", locID, "")
			if parseErr != nil {
				return nil, fmt.Errorf("invalid parent location.@id %q: %w", locID, parseErr)
			}
			placeRecord, lookupErr := s.repo.GetPlaceByULID(ctx, parsed.ULID)
			if lookupErr != nil {
				if lookupErr == ErrNotFound {
					return nil, ValidationError{
						Field:   "location.@id",
						Message: fmt.Sprintf("canonical place %q not found; ensure the place exists before referencing it by @id", locID),
					}
				}
				return nil, fmt.Errorf("resolve parent location.@id %q: %w", locID, lookupErr)
			}
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
			// Name-based path
			dbRepo := s.repo
			if opts.TxRepo != nil {
				dbRepo = opts.TxRepo
			}
			generatedPlaceULID, err := ids.NewULID()
			if err != nil {
				return nil, fmt.Errorf("generate place ulid: %w", err)
			}
			place, err := dbRepo.UpsertPlace(ctx, PlaceCreateParams{
				EntityCreateFields: EntityCreateFields{
					ULID:            generatedPlaceULID,
					Name:            validated.Location.Name,
					StreetAddress:   validated.Location.StreetAddress,
					PostalCode:      validated.Location.PostalCode,
					AddressLocality: validated.Location.AddressLocality,
					AddressRegion:   normalizeRegion(validated.Location.AddressRegion),
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

			// Layer 3: Fuzzy place dedup (only when a NEW place was just created)
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
						Msg("Place similarity check failed, continuing")
				} else {
					var filtered []SimilarPlaceCandidate
					for _, c := range placeCandidates {
						if c.ID != place.ID {
							filtered = append(filtered, c)
						}
					}
					if len(filtered) > 0 {
						best := filtered[0]
						if best.Similarity >= s.dedupConfig.PlaceAutoMergeThreshold {
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
								params.PrimaryVenueID = &mergeResult.CanonicalID
							}
						} else {
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
			}
		}
	}

	// ── 8. Layer 2: Near-duplicate detection ─────────────────────────────────
	if params.PrimaryVenueID != nil && s.dedupConfig.NearDuplicateThreshold > 0 {
		startTime, parseErr := time.Parse(time.RFC3339, strings.TrimSpace(validated.StartDate))
		if parseErr == nil {
			candidates, err := s.repo.FindNearDuplicates(ctx, *params.PrimaryVenueID, startTime, validated.Name, s.dedupConfig.NearDuplicateThreshold)
			if err != nil {
				log.Warn().Err(err).
					Str("event_name", validated.Name).
					Msg("Near-duplicate check failed, continuing")
			} else if len(candidates) > 0 {
				// Filter excluded ULIDs (retired events in consolidation path, and not-duplicate pairs)
				excludeSet := make(map[string]struct{}, len(opts.ExcludeFromNearDup))
				for _, u := range opts.ExcludeFromNearDup {
					excludeSet[u] = struct{}{}
				}
				filtered := make([]NearDuplicateCandidate, 0, len(candidates))
				for _, c := range candidates {
					if _, excl := excludeSet[c.ULID]; excl {
						continue
					}
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
						Details: map[string]any{"matches": matches},
					})
					needsReview = true
					nearDuplicateCandidates = candidates
				}
			}
		}
	}

	// ── 9. Organizer resolution (with optional Layer 3 fuzzy dedup) ──────────
	if validated.Organizer != nil && validated.Organizer.Name != "" {
		dbRepo := s.repo
		if opts.TxRepo != nil {
			dbRepo = opts.TxRepo
		}
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
		org, err := dbRepo.UpsertOrganization(ctx, OrganizationCreateParams{
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

		// Layer 3: Fuzzy org dedup (only when a NEW org was just created)
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
					Msg("Organization similarity check failed, continuing")
			} else {
				var filtered []SimilarOrgCandidate
				for _, c := range orgCandidates {
					if c.ID != org.ID {
						filtered = append(filtered, c)
					}
				}
				if len(filtered) > 0 {
					best := filtered[0]
					if best.Similarity >= s.dedupConfig.OrgAutoMergeThreshold {
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
							params.OrganizerID = &mergeResult.CanonicalID
						}
					} else {
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

	// Store the dedup hash on the event params
	params.DedupHash = dedupHash

	// ── 10. Determine or use the caller's transaction ────────────────────────
	var dbRepo Repository
	var tx TxCommitter
	ownTx := opts.TxRepo == nil
	if ownTx {
		dbRepo, tx, err = s.repo.BeginTx(ctx)
		if err != nil {
			return nil, fmt.Errorf("begin transaction: %w", err)
		}
		defer func() {
			if err != nil {
				_ = tx.Rollback(ctx)
			}
		}()
	} else {
		dbRepo = opts.TxRepo
	}

	// Re-evaluate lifecycle state: needsReview may have been updated since params were built
	if needsReview {
		params.LifecycleState = "pending_review"
	}

	// ── 11. Create event ─────────────────────────────────────────────────────
	event, err := dbRepo.Create(ctx, params)
	if err != nil {
		return nil, err
	}

	// ── 12. Create occurrences ───────────────────────────────────────────────
	if err := s.createOccurrencesWithRepo(ctx, dbRepo, event, validated); err != nil {
		return nil, err
	}

	// ── 13. Record source ────────────────────────────────────────────────────
	if err := s.recordSourceWithRepo(ctx, dbRepo, event, validated, sourceID); err != nil {
		return nil, err
	}

	// Capture first near-duplicate candidate's UUID for cross-linking in the
	// new event's review entry
	if len(nearDuplicateCandidates) > 0 {
		existingEvent, fetchErr := s.repo.GetByULID(ctx, nearDuplicateCandidates[0].ULID)
		if fetchErr == nil {
			existingID := existingEvent.ID
			nearDuplicateOfID = &existingID
		}
	}

	// ── 14. Create review queue entry if needed ──────────────────────────────
	if needsReview {
		log.Debug().
			Str("event_ulid", event.ULID).
			Int("warnings_count", len(warnings)).
			Msg("createEventCore: creating review queue entry")

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
		reviewEntry, err := dbRepo.CreateReviewQueueEntry(ctx, ReviewQueueCreateParams{
			EventID:            event.ID,
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
			Msg("createEventCore: created review queue entry")
	}

	// ── 15. Update idempotency key (inside own transaction, if provided) ───────
	if ownTx && strings.TrimSpace(opts.IdempotencyKey) != "" {
		if err := dbRepo.UpdateIdempotencyKeyEvent(ctx, opts.IdempotencyKey, event.ID, event.ULID); err != nil {
			return nil, err
		}
	}

	// ── 16. Commit own transaction (if we opened one) ─────────────────────────
	if ownTx {
		if err := tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("commit transaction: %w", err)
		}

		// Post-commit near-dup cross-linking (only when we own the transaction)
		s.crossLinkNearDuplicates(ctx, event, validated, nearDuplicateCandidates)

		// Clear candidates — we already handled them
		nearDuplicateCandidates = nil
	}

	return &CreateEventCoreResult{
		IngestResult: &IngestResult{
			Event:         event,
			IsDuplicate:   isDuplicate,
			NeedsReview:   needsReview,
			Warnings:      warnings,
			PlaceULID:     placeULID,
			OrganizerULID: orgULID,
		},
		NearDuplicateCandidates: nearDuplicateCandidates,
		DedupHash:               dedupHash,
	}, nil
}

// crossLinkNearDuplicates flags existing near-duplicate events for review after
// the new event has been committed.  This must happen post-commit because the
// review queue entry for existing events cross-references the new event's UUID
// via duplicate_of_event_id (FK constraint).
// Non-critical: errors are logged and skipped.
func (s *IngestService) crossLinkNearDuplicates(
	ctx context.Context,
	newEvent *Event,
	validated EventInput,
	candidates []NearDuplicateCandidate,
) {
	if len(candidates) == 0 {
		return
	}
	pendingState := "pending_review"
	for _, c := range candidates {
		existingEvent, fetchErr := s.repo.GetByULID(ctx, c.ULID)
		if fetchErr != nil {
			log.Warn().Err(fetchErr).
				Str("candidate_ulid", c.ULID).
				Msg("Near-duplicate: failed to fetch existing event for review flagging, skipping")
			continue
		}

		if existingEvent.LifecycleState == "published" {
			if _, updateErr := s.repo.UpdateEvent(ctx, c.ULID, UpdateEventParams{
				LifecycleState: &pendingState,
			}); updateErr != nil {
				log.Warn().Err(updateErr).
					Str("candidate_ulid", c.ULID).
					Msg("Near-duplicate: failed to update existing event lifecycle state, skipping")
			}
		}

		existingStart, existingEnd := parseEventTimesFromEvent(existingEvent)
		newEventID := newEvent.ID

		reconstructedPayload, payloadErr := reconstructPayloadFromEvent(existingEvent)
		if payloadErr != nil {
			log.Warn().Err(payloadErr).
				Str("candidate_ulid", c.ULID).
				Msg("Near-duplicate: failed to reconstruct payload for existing event, using empty")
			reconstructedPayload = []byte("{}")
		}
		existingWarnings, warnErr := nearDuplicateWarnings(existingEvent, newEvent.ULID, nearDupNewEventData{
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

		// If the existing event already has a pending review entry (e.g. it got
		// multi_session_likely on its own ingest), merge the near-duplicate warning
		// into that entry rather than inserting a second row (which would hit the
		// UNIQUE constraint on event_id and be silently dropped).
		existingReview, lookupErr := s.repo.GetPendingReviewByEventUlid(ctx, c.ULID)
		if lookupErr != nil {
			log.Warn().Err(lookupErr).
				Str("candidate_ulid", c.ULID).
				Msg("Near-duplicate: failed to look up existing review entry, skipping")
			continue
		}

		if existingReview != nil {
			// Merge: append near_duplicate_of_new_event warning and set duplicate_of_event_id.
			mergedWarnings, mergeErr := appendWarnings(existingReview.Warnings, existingWarnings)
			if mergeErr != nil {
				log.Warn().Err(mergeErr).
					Str("candidate_ulid", c.ULID).
					Msg("Near-duplicate: failed to merge warnings into existing review entry, skipping")
				continue
			}
			if _, updateErr := s.repo.UpdateReviewQueueEntry(ctx, existingReview.ID, ReviewQueueUpdateParams{
				Warnings:           &mergedWarnings,
				DuplicateOfEventID: &newEventID,
			}); updateErr != nil {
				log.Warn().Err(updateErr).
					Str("candidate_ulid", c.ULID).
					Msg("Near-duplicate: failed to update existing review entry with near-duplicate warning, skipping")
			}
			continue
		}

		if _, createErr := s.repo.CreateReviewQueueEntry(ctx, ReviewQueueCreateParams{
			EventID:            existingEvent.ID,
			OriginalPayload:    reconstructedPayload,
			NormalizedPayload:  reconstructedPayload,
			EventStartTime:     existingStart,
			EventEndTime:       existingEnd,
			Warnings:           existingWarnings,
			DuplicateOfEventID: &newEventID,
		}); createErr != nil {
			log.Warn().Err(createErr).
				Str("candidate_ulid", c.ULID).
				Msg("Near-duplicate: failed to create review queue entry for existing event, skipping")
		}
	}
}

// appendWarnings merges two JSON arrays of ValidationWarning objects, deduplicating
// by (field, code, message) so that distinct warnings with the same code (e.g. two
// separate near_duplicate_of_new_event warnings for different new events) are both
// retained, while truly identical warnings (same event re-ingested) are dropped.
func appendWarnings(existing []byte, toAdd []byte) ([]byte, error) {
	var existingList []ValidationWarning
	if len(existing) > 0 {
		if err := json.Unmarshal(existing, &existingList); err != nil {
			return nil, fmt.Errorf("unmarshal existing warnings: %w", err)
		}
	}
	var addList []ValidationWarning
	if len(toAdd) > 0 {
		if err := json.Unmarshal(toAdd, &addList); err != nil {
			return nil, fmt.Errorf("unmarshal warnings to add: %w", err)
		}
	}
	// Dedup by (field, code, message) — message includes the new event ULID for
	// near_duplicate_of_new_event, so two different near-duplicate pairings produce
	// distinct entries while re-ingesting the same pair is idempotent.
	type key struct{ field, code, message string }
	seen := make(map[key]struct{}, len(existingList))
	for _, w := range existingList {
		seen[key{w.Field, w.Code, w.Message}] = struct{}{}
	}
	for _, w := range addList {
		if _, ok := seen[key{w.Field, w.Code, w.Message}]; !ok {
			existingList = append(existingList, w)
		}
	}
	return json.Marshal(existingList)
}
