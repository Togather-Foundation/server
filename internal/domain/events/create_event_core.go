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
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Togather-Foundation/server/internal/domain/ids"
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

	s.logger.Debug().
		Str("event_name", validated.Name).
		Int("validation_warnings", len(warnings)).
		Msg("createEventCore: after validation")

	// ── 1.5. Geographic boundary check ──────────────────────────────────────
	if err := CheckGeographicBoundary(validated, s.geoBoundaryConfig); err != nil {
		switch {
		case s.geoBoundaryConfig.Mode == "review":
			warnings = append(warnings, ValidationWarning{
				Field:   "location",
				Message: err.Error(),
				Code:    "outside_geo_boundary",
				Details: map[string]any{
					"mode": s.geoBoundaryConfig.Mode,
				},
			})
		case s.resolveLocationOnBoundaryFailure(ctx, &validated):
			// The offline parse could not place the event, but the geocoder
			// could. Accept with the resolved address instead of rejecting.
		default:
			return nil, err
		}
	}

	// ── 2. Quality warnings ──────────────────────────────────────────────────
	warnings = appendQualityWarnings(warnings, validated, nil, s.validationConfig, s.logger)

	s.logger.Debug().
		Str("event_name", validated.Name).
		Int("total_warnings", len(warnings)).
		Msg("createEventCore: after quality warnings")

	// Check if review is needed. Warning codes the source has suppressed (e.g.
	// cross_week_series_companion for legitimate recurring programmes) are
	// ignored for gating but still kept in the warnings slice and returned.
	suppressedWarnings := suppressedWarningSet(validated.SuppressReviewWarnings)
	needsReview := warningsRequireReview(warnings, suppressedWarnings) || eventNeedsReview(validated, nil, s.validationConfig)

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
		resolvedID, err := s.repo.GetOrCreateSource(ctx, SourceParamsForFeed(
			sourceName(validated.Source, validated.Name), validated.Source.URL, sourceLicense(validated), 5,
		))
		if err != nil {
			return nil, err
		}
		sourceID = resolvedID

		existing, err := s.repo.FindByFeedEventID(ctx, validated.Source.URL, validated.Source.EventID)
		if err == nil && existing != nil {
			if opts.SkipDedupAutoMerge {
				// Warn instead of auto-merging
				warnings = append(warnings, ValidationWarning{
					Code:    "exact_duplicate",
					Message: fmt.Sprintf("feed event-ID match with existing event %s — review recommended", existing.ULID),
				})
				needsReview = needsReview || warningRequiresReview("exact_duplicate", suppressedWarnings)
			} else {
				// Standard ingest: auto-merge.
				//
				// Identity is (feed URL, sourceEventID) — trailing slashes
				// ignored — so the feed owns this event: a rename in the feed
				// renames the event instead of creating a second one. Trust level
				// only decides gap-filling for fields the feed leaves empty
				// (AutoMergeFields below).
				existingTrust, err := s.repo.GetSourceTrustLevel(ctx, existing.ID)
				if err != nil {
					return nil, fmt.Errorf("get existing source trust for feed event-ID match: %w", err)
				}
				newTrust, err := s.repo.GetSourceTrustLevelBySourceID(ctx, sourceID)
				if err != nil {
					return nil, fmt.Errorf("get new source trust for feed event-ID match: %w", err)
				}
				updates, changed := AutoMergeFields(existing, validated, existingTrust, newTrust)

				// AutoMergeFields deliberately never merges Name (the dedup hash
				// was built from it). For a feed-owned event the feed is
				// authoritative for the name, so apply the rename and keep the
				// Layer-1 hash consistent with the stored name — but only when the
				// rest of the incoming identity proves the event did not move.
				if name := strings.TrimSpace(validated.Name); name != "" && name != existing.Name {
					updates.Name = &name
					changed = true

					incomingVenue := NormalizeVenueKey(validated)
					oldHashFromIncoming := BuildDedupHash(DedupCandidate{
						Name:      existing.Name,
						VenueID:   incomingVenue,
						StartDate: validated.StartDate,
					})
					newHash := BuildDedupHash(DedupCandidate{
						Name:      name,
						VenueID:   incomingVenue,
						StartDate: validated.StartDate,
					})
					// Only rewrite the hash when the incoming payload's non-name
					// identity still reproduces the stored hash, i.e. the feed
					// renamed the event in place. Ingest never updates the venue or
					// the occurrence dates on this path, so a hash built from an
					// incoming venue/date we do not store would drift away from the
					// event's actual identity.
					if existing.DedupHash != "" && oldHashFromIncoming == existing.DedupHash {
						if newHash != "" && newHash != existing.DedupHash {
							updates.DedupHash = &newHash
						}
					} else if existing.DedupHash != "" {
						s.logger.Info().
							Str("event_ulid", existing.ULID).
							Str("previous_name", existing.Name).
							Str("incoming_name", name).
							Msg("feed rename left dedup hash unchanged: incoming venue or start date differs from the stored event")
					}
				}

				if changed {
					existing, err = s.repo.UpdateEvent(ctx, existing.ULID, updates)
					if err != nil {
						return nil, fmt.Errorf("feed event-ID auto-merge update: %w", err)
					}
				}
				if err := s.recordSourceForExisting(ctx, existing, validated, sourceID); err != nil {
					s.logSourceAttributionError(existing, sourceID, err)
				}
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
					needsReview = needsReview || warningRequiresReview("exact_duplicate", suppressedWarnings)
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
					if err := s.recordSourceForExisting(ctx, existing, validated, sourceID); err != nil {
						s.logSourceAttributionError(existing, sourceID, err)
					}
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
					EventAllDay:       &validated.AllDay,
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
		Confidence:          floatPtr(reviewConfidence(validated, len(warnings) > 0, s.validationConfig)),
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
					s.logger.Warn().Err(err).
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
							mergeResult, mergeErr := mergePlacesTombstoned(ctx, s.repo, s.nodeDomain, place.ID, best.ID)
							if mergeErr != nil {
								s.logger.Warn().Err(mergeErr).
									Str("duplicate_place", place.ULID).
									Str("primary_place", best.ULID).
									Msg("Place auto-merge failed, continuing with new place")
							} else {
								if mergeResult.AlreadyMerged {
									s.logger.Info().
										Str("duplicate_place", place.ULID).
										Str("canonical_place", mergeResult.CanonicalID).
										Msg("Place already merged by concurrent operation, using canonical")
								} else {
									s.logger.Info().
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
							needsReview = needsReview || warningRequiresReview("place_possible_duplicate", suppressedWarnings)
						}
					}
				}
			}
		}
	}

	// ── 8. Dedup warning checks (near-dup + cross-week series) ─────────────────
	var dedupWarnings []ValidationWarning
	var seriesCompanion *CrossWeekCompanion
	if params.PrimaryVenueID != nil && validated.StartDate != "" {
		dbRepo := s.repo
		if opts.TxRepo != nil {
			dbRepo = opts.TxRepo
		}
		startTime, parseErr := time.Parse(time.RFC3339, strings.TrimSpace(validated.StartDate))
		if parseErr == nil {
			dedupWarnings, nearDuplicateCandidates, seriesCompanion, _ = s.runDedupWarningChecks(
				ctx, dbRepo, validated.Name, *params.PrimaryVenueID, startTime,
				ulidValue, opts.ExcludeFromNearDup, s.dedupConfig.NearDuplicateThreshold,
			)
			warnings = append(warnings, dedupWarnings...)
			if warningsRequireReview(dedupWarnings, suppressedWarnings) {
				needsReview = true
			}
			// Suppressing cross_week_series_companion also suppresses companion
			// cross-linking. Otherwise a suppressed week-2 event publishes while
			// week-1's already-published event gets a fresh pending review row,
			// re-flooding the queue with exactly the entries this policy exists
			// to prevent. Sources that do not suppress the code keep the
			// bidirectional cross-link.
			if !warningRequiresReview("cross_week_series_companion", suppressedWarnings) {
				seriesCompanion = nil
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
				s.logger.Warn().Err(err).
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
						mergeResult, mergeErr := mergeOrganizationsTombstoned(ctx, s.repo, s.nodeDomain, org.ID, best.ID)
						if mergeErr != nil {
							s.logger.Warn().Err(mergeErr).
								Str("duplicate_org", org.ULID).
								Str("primary_org", best.ULID).
								Msg("Organization auto-merge failed, continuing with new org")
						} else {
							if mergeResult.AlreadyMerged {
								s.logger.Info().
									Str("duplicate_org", org.ULID).
									Str("canonical_org", mergeResult.CanonicalID).
									Msg("Organization already merged by concurrent operation, using canonical")
							} else {
								s.logger.Info().
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
						needsReview = needsReview || warningRequiresReview("org_possible_duplicate", suppressedWarnings)
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
	committed := false
	if ownTx {
		dbRepo, tx, err = s.repo.BeginTx(ctx)
		if err != nil {
			return nil, fmt.Errorf("begin transaction: %w", err)
		}
		defer func() {
			if !committed {
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

	// ── 10.5. Upsert event_series for recurring events ─────────────────────────
	if validated.Recurrence != nil {
		rec := validated.Recurrence
		seriesResult, err := dbRepo.UpsertEventSeries(ctx, UpsertEventSeriesParams{
			ExternalKey: rec.ExternalKey,
			Name:        rec.SeriesName,
			SeriesStart: rec.SeriesStart,
			SeriesEnd:   rec.SeriesEnd,
			RRule:       rec.RRule,
			ExDates:     rec.ExDates,
			RDates:      rec.RDates,
			TZID:        rec.TZID,
		})
		if err != nil {
			return nil, fmt.Errorf("upsert event series: %w", err)
		}
		params.SeriesID = &seriesResult.SeriesID
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
		s.logger.Debug().
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

		startTime, endTime, err := s.parseEventTimes(ctx, dbRepo, validated)
		if err != nil {
			return nil, fmt.Errorf("review queue dates: %w", err)
		}
		reviewEntry, err := dbRepo.UpsertReviewRow(ctx, ReviewQueueUpsertParams{
			EventULID:          event.ULID,
			OriginalPayload:    originalJSON,
			NormalizedPayload:  normalizedJSON,
			Warnings:           warningsJSON,
			SourceID:           sourceIDPtr,
			SourceExternalID:   externalID,
			DedupHash:          dedupHashPtr,
			EventStartTime:     startTime,
			EventEndTime:       endTime,
			EventAllDay:        validated.AllDay,
			DuplicateOfEventID: nearDuplicateOfID,
			WarningMode:        ReviewWarningReplace,
			Promotion:          ReviewPromotionNone,
		})
		if err != nil {
			return nil, fmt.Errorf("create review queue entry: %w", err)
		}

		s.logger.Debug().
			Str("event_ulid", event.ULID).
			Int("review_entry_id", reviewEntry.Entry.ID).
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
		committed = true

		// Post-commit near-dup cross-linking (only when we own the transaction)
		s.crossLinkNearDuplicates(ctx, event, validated, nearDuplicateCandidates)

		// Post-commit series companion cross-linking: flag the companion (Week 1)
		// when the new event (Week 2) is ingested so both sides show the badge.
		s.crossLinkSeriesCompanions(ctx, event, validated, seriesCompanion)

		// Clear candidates — we already handled them
		nearDuplicateCandidates = nil
	}

	// Audit trace: suppression is otherwise invisible once the event is
	// published. Log only when a listed code was actually present and ignored.
	if ignored := ignoredReviewWarningCodes(warnings, suppressedWarnings); len(ignored) > 0 {
		s.logger.Info().
			Str("event_name", event.Name).
			Str("event_ulid", event.ULID).
			Strs("suppressed_warning_codes", ignored).
			Msg("review warnings ignored for gating by source suppression policy")
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

// runDedupWarningChecks runs all dedup-related warning checks and returns
// the accumulated warnings plus near-duplicate candidates. It is called by
// both createEventCore (new event) and the promote path of Consolidate
// (existing canonical event).
//
// Parameters:
//   - ctx, txRepo: repo for DB queries (may be the caller's transaction)
//   - name: the normalized event name
//   - venueID: resolved primary venue ID (empty means no venue)
//   - startTime: first occurrence start time
//   - ulid: this event's ULID (excluded from candidates)
//   - excludeULIDs: additional ULIDs to exclude (e.g., retired events in consolidation)
//   - nearDupThreshold: similarity threshold for near-duplicate detection
//
// Returns warnings (may be empty) and near-duplicate candidates (for cross-linking).
func (s *IngestService) runDedupWarningChecks(
	ctx context.Context,
	txRepo Repository,
	name string,
	venueID string,
	startTime time.Time,
	ulid string,
	excludeULIDs []string,
	nearDupThreshold float64,
) (warnings []ValidationWarning, candidates []NearDuplicateCandidate, seriesCompanion *CrossWeekCompanion, err error) {
	if venueID == "" || nearDupThreshold <= 0 {
		return nil, nil, nil, nil
	}

	excludeSet := make(map[string]struct{}, len(excludeULIDs)+1)
	excludeSet[ulid] = struct{}{}
	for _, u := range excludeULIDs {
		excludeSet[u] = struct{}{}
	}

	candidates, err = txRepo.FindNearDuplicates(ctx, venueID, startTime, name, nearDupThreshold)
	if err != nil {
		s.logger.Warn().Err(err).
			Str("event_name", name).
			Msg("Near-duplicate check failed, continuing")
		return nil, nil, nil, nil
	}

	filtered := make([]NearDuplicateCandidate, 0, len(candidates))
	for _, c := range candidates {
		if _, excl := excludeSet[c.ULID]; excl {
			continue
		}
		notDup, err := txRepo.IsNotDuplicate(ctx, ulid, c.ULID)
		if err != nil {
			s.logger.Warn().Err(err).
				Str("event_ulid", ulid).
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
	}

	companion, err := txRepo.FindSeriesCompanion(ctx, SeriesCompanionQuery{
		NormalizedName: name,
		VenueID:        venueID,
		StartTime:      startTime,
		ExcludeULID:    ulid,
	})
	if err != nil {
		s.logger.Warn().Err(err).
			Str("event_name", name).
			Msg("Cross-week series check failed, continuing")
		return warnings, candidates, nil, nil
	}
	if companion != nil {
		if _, retired := excludeSet[companion.ULID]; !retired {
			warnings = append(warnings, ValidationWarning{
				Field:   "name",
				Message: fmt.Sprintf("Event appears to be part of a recurring series with existing event %q at the same venue on %s", companion.Name, companion.StartDate),
				Code:    "cross_week_series_companion",
				Details: map[string]any{
					"companion_ulid": companion.ULID,
					"companion_name": companion.Name,
					"companion_date": companion.StartDate,
					"companion_time": companion.StartTime,
					"venue_name":     companion.VenueName,
				},
			})
		} else {
			// Companion was excluded (e.g. being consolidated) — don't return it
			companion = nil
		}
	}

	return warnings, candidates, companion, nil
}

// upsertReviewRowWithRetry runs the best-effort cross-link write in a short
// transaction and retries a transient contention failure a couple of times.
// The upsert is idempotent (same event, same warning), so a retry cannot
// duplicate a row or a warning; it just keeps the flag from being dropped on a
// routine lock wait.
func (s *IngestService) upsertReviewRowWithRetry(ctx context.Context, params ReviewQueueUpsertParams) error {
	const attempts = 3

	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(attempt) * 25 * time.Millisecond):
			}
		}

		txRepo, tx, err := s.repo.BeginTx(ctx)
		if err != nil {
			lastErr = err
			if errors.Is(err, ErrRetryable) {
				continue
			}
			return err
		}
		_, err = txRepo.UpsertReviewRow(ctx, params)
		if err != nil {
			_ = tx.Rollback(ctx)
			lastErr = err
			if errors.Is(err, ErrRetryable) {
				continue
			}
			return err
		}
		if err := tx.Commit(ctx); err != nil {
			lastErr = err
			if errors.Is(err, ErrRetryable) {
				continue
			}
			return err
		}
		return nil
	}
	return lastErr
}

// crossLinkSeriesCompanions adds a cross_week_series_companion warning to the
// companion event's review queue entry when the new event (Week 2) is ingested.
// The companion (Week 1) was already identified by FindSeriesCompanion during
// the ingest transaction — we reuse that result rather than issuing another query.
//
// This is non-critical: errors are logged and skipped.
func (s *IngestService) crossLinkSeriesCompanions(
	ctx context.Context,
	newEvent *Event,
	validated EventInput,
	companion *CrossWeekCompanion,
) {
	if companion == nil {
		return
	}

	startTime, _ := parseEventTimesFromEvent(newEvent)
	if startTime.IsZero() {
		if validated.StartDate == "" {
			return
		}
		var err error
		startTime, err = time.Parse(time.RFC3339, strings.TrimSpace(validated.StartDate))
		if err != nil {
			s.logger.Warn().Err(err).Str("start_date", validated.StartDate).
				Msg("Series companion cross-link: failed to parse start_date, skipping")
			return
		}
	}

	newStartDate := startTime.UTC().Format("2006-01-02")
	newStartTimeStr := startTime.UTC().Format("15:04:05")

	companionWarning := ValidationWarning{
		Field: "name",
		Message: fmt.Sprintf("Event appears to be part of a recurring series with existing event %q at the same venue on %s",
			newEvent.Name, newStartDate),
		Code: "cross_week_series_companion",
		Details: map[string]any{
			"companion_ulid": newEvent.ULID,
			"companion_name": newEvent.Name,
			"companion_date": newStartDate,
			"companion_time": newStartTimeStr,
			"venue_name":     companion.VenueName,
		},
	}
	companionWarningJSON, err := toJSON([]ValidationWarning{companionWarning})
	if err != nil {
		s.logger.Warn().Err(err).Str("companion_ulid", companion.ULID).
			Msg("Series companion cross-link: failed to marshal warning, skipping")
		return
	}

	companionEvent, fetchErr := s.repo.GetByULID(ctx, companion.ULID)
	if fetchErr != nil {
		s.logger.Warn().Err(fetchErr).Str("companion_ulid", companion.ULID).
			Msg("Series companion cross-link: failed to fetch companion event, skipping")
		return
	}

	companionStart, companionEnd := parseEventTimesFromEvent(companionEvent)
	reconstructedPayload, payloadErr := reconstructPayloadFromEvent(companionEvent)
	if payloadErr != nil {
		s.logger.Warn().Err(payloadErr).Str("companion_ulid", companion.ULID).
			Msg("Series companion cross-link: failed to reconstruct payload, using empty")
		reconstructedPayload = []byte("{}")
	}

	// The companion may already have a review row (pending or decided). The
	// atomic upsert merges the cross-week warning into it without touching its
	// status or the companion's lifecycle; when no row exists it inserts one.
	// The events row lock inside UpsertReviewRow is the serialisation point, so
	// a concurrent writer cannot make this fail with 23505 after a flip.
	if upsertErr := s.upsertReviewRowWithRetry(ctx, ReviewQueueUpsertParams{
		EventULID:         companion.ULID,
		OriginalPayload:   reconstructedPayload,
		NormalizedPayload: reconstructedPayload,
		EventStartTime:    companionStart,
		EventEndTime:      companionEnd,
		EventAllDay:       eventAllDayFromEvent(companionEvent),
		Warnings:          companionWarningJSON,
		WarningMode:       ReviewWarningMerge,
		Promotion:         ReviewPromotionNone,
	}); upsertErr != nil {
		s.logger.Warn().Err(upsertErr).Str("companion_ulid", companion.ULID).
			Msg("Series companion cross-link: failed to upsert review queue entry, skipping")
	}
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
	for _, c := range candidates {
		existingEvent, fetchErr := s.repo.GetByULID(ctx, c.ULID)
		if fetchErr != nil {
			s.logger.Warn().Err(fetchErr).
				Str("candidate_ulid", c.ULID).
				Msg("Near-duplicate: failed to fetch existing event for review flagging, skipping")
			continue
		}

		existingStart, existingEnd := parseEventTimesFromEvent(existingEvent)
		newEventID := newEvent.ID

		reconstructedPayload, payloadErr := reconstructPayloadFromEvent(existingEvent)
		if payloadErr != nil {
			s.logger.Warn().Err(payloadErr).
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
			s.logger.Warn().Err(warnErr).
				Str("candidate_ulid", c.ULID).
				Msg("Near-duplicate: failed to generate warnings for existing event, using empty")
			existingWarnings = []byte("[]")
		}

		// One short transaction per candidate: the atomic create-or-merge locks
		// the candidate's events row (the serialisation point), merges the warning
		// into any existing row without touching its status, and only flips a
		// published candidate to pending_review when it actually inserts a new
		// row. No pre-check, so a concurrent writer cannot turn this into a
		// 23505 after the lifecycle flip. The helper retries transient
		// contention so the warning is not silently dropped.
		if upsertErr := s.upsertReviewRowWithRetry(ctx, ReviewQueueUpsertParams{
			EventULID:          c.ULID,
			OriginalPayload:    reconstructedPayload,
			NormalizedPayload:  reconstructedPayload,
			EventStartTime:     existingStart,
			EventEndTime:       existingEnd,
			EventAllDay:        eventAllDayFromEvent(existingEvent),
			Warnings:           existingWarnings,
			DuplicateOfEventID: &newEventID,
			WarningMode:        ReviewWarningMerge,
			Promotion:          ReviewPromotionInserted,
		}); upsertErr != nil {
			s.logger.Warn().Err(upsertErr).
				Str("candidate_ulid", c.ULID).
				Msg("Near-duplicate: failed to upsert review queue entry for existing event, skipping")
		}
	}
}
