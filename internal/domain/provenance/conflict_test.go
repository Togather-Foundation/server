package provenance_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestConflictResolutionByTrustLevel verifies that higher trust level sources win
// per the priority rule: trust_level DESC, confidence DESC, observed_at DESC
func TestConflictResolutionByTrustLevel(t *testing.T) {
	tests := []struct {
		name          string
		source1Trust  int
		source2Trust  int
		source1Conf   float64
		source2Conf   float64
		expectSource1 bool
		description   string
	}{
		{
			name:          "higher trust wins regardless of confidence",
			source1Trust:  9,
			source2Trust:  5,
			source1Conf:   0.7,
			source2Conf:   0.95,
			expectSource1: true,
			description:   "source with trust 9 should win over trust 5, even with lower confidence",
		},
		{
			name:          "much higher trust wins",
			source1Trust:  10,
			source2Trust:  1,
			source1Conf:   0.5,
			source2Conf:   0.9,
			expectSource1: true,
			description:   "highest trust level always wins first",
		},
		{
			name:          "equal trust falls back to confidence",
			source1Trust:  7,
			source2Trust:  7,
			source1Conf:   0.95,
			source2Conf:   0.75,
			expectSource1: true,
			description:   "when trust is equal, higher confidence wins",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This test documents the expected behavior for conflict resolution
			// The actual resolution logic will be implemented in the provenance service

			// Priority: trust_level DESC > confidence DESC > observed_at DESC
			var winningSource int
			if tt.source1Trust > tt.source2Trust {
				winningSource = 1
			} else if tt.source1Trust < tt.source2Trust {
				winningSource = 2
			} else {
				// Trust levels are equal, check confidence
				if tt.source1Conf > tt.source2Conf {
					winningSource = 1
				} else if tt.source1Conf < tt.source2Conf {
					winningSource = 2
				}
			}

			if tt.expectSource1 {
				require.Equal(t, 1, winningSource, tt.description)
			} else {
				require.Equal(t, 2, winningSource, tt.description)
			}
		})
	}
}

// TestConflictResolutionByConfidence verifies that when trust levels are equal,
// confidence is the tiebreaker
func TestConflictResolutionByConfidence(t *testing.T) {
	tests := []struct {
		name        string
		trust       int
		confidence1 float64
		confidence2 float64
		expectFirst bool
	}{
		{
			name:        "higher confidence wins with equal trust",
			trust:       7,
			confidence1: 0.95,
			confidence2: 0.80,
			expectFirst: true,
		},
		{
			name:        "much higher confidence wins",
			trust:       5,
			confidence1: 0.99,
			confidence2: 0.50,
			expectFirst: true,
		},
		{
			name:        "lower confidence loses",
			trust:       6,
			confidence1: 0.60,
			confidence2: 0.85,
			expectFirst: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Both sources have the same trust level
			// Winner is determined by confidence
			winner := 1
			if tt.confidence2 > tt.confidence1 {
				winner = 2
			}

			if tt.expectFirst {
				require.Equal(t, 1, winner, "higher confidence should win when trust is equal")
			} else {
				require.Equal(t, 2, winner, "lower confidence should lose when trust is equal")
			}
		})
	}
}

// TestConflictResolutionByTimestamp verifies that when trust and confidence are equal,
// the most recently observed value wins
func TestConflictResolutionByTimestamp(t *testing.T) {
	now := time.Now()
	oneHourAgo := now.Add(-1 * time.Hour)
	oneDayAgo := now.Add(-24 * time.Hour)

	tests := []struct {
		name        string
		trust       int
		confidence  float64
		observed1   time.Time
		observed2   time.Time
		expectFirst bool
	}{
		{
			name:        "more recent observation wins",
			trust:       7,
			confidence:  0.85,
			observed1:   now,
			observed2:   oneHourAgo,
			expectFirst: true,
		},
		{
			name:        "older observation loses",
			trust:       6,
			confidence:  0.90,
			observed1:   oneDayAgo,
			observed2:   now,
			expectFirst: false,
		},
		{
			name:        "much more recent wins",
			trust:       8,
			confidence:  0.75,
			observed1:   now,
			observed2:   oneDayAgo,
			expectFirst: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Both sources have same trust and confidence
			// Winner is determined by observed_at DESC (most recent)
			winner := 1
			if tt.observed2.After(tt.observed1) {
				winner = 2
			}

			if tt.expectFirst {
				require.Equal(t, 1, winner, "more recent observation should win when trust and confidence are equal")
			} else {
				require.Equal(t, 2, winner, "older observation should lose when trust and confidence are equal")
			}
		})
	}
}

// TestConflictResolutionFullPriority tests the complete priority chain
func TestConflictResolutionFullPriority(t *testing.T) {
	now := time.Now()

	type source struct {
		trust      int
		confidence float64
		observed   time.Time
	}

	tests := []struct {
		name     string
		sources  []source
		expected int // index of winning source
	}{
		{
			name: "trust level is primary factor",
			sources: []source{
				{trust: 9, confidence: 0.70, observed: now.Add(-2 * time.Hour)},
				{trust: 5, confidence: 0.95, observed: now},
			},
			expected: 0, // first source wins due to higher trust
		},
		{
			name: "confidence breaks trust tie",
			sources: []source{
				{trust: 7, confidence: 0.95, observed: now.Add(-1 * time.Hour)},
				{trust: 7, confidence: 0.80, observed: now},
			},
			expected: 0, // first source wins due to higher confidence
		},
		{
			name: "timestamp breaks trust and confidence tie",
			sources: []source{
				{trust: 7, confidence: 0.85, observed: now.Add(-1 * time.Hour)},
				{trust: 7, confidence: 0.85, observed: now},
			},
			expected: 1, // second source wins due to more recent observation
		},
		{
			name: "three-way conflict",
			sources: []source{
				{trust: 8, confidence: 0.75, observed: now.Add(-1 * time.Hour)},
				{trust: 6, confidence: 0.95, observed: now},
				{trust: 9, confidence: 0.60, observed: now.Add(-2 * time.Hour)},
			},
			expected: 2, // third source wins due to highest trust (9)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Find winner using priority: trust_level DESC, confidence DESC, observed_at DESC
			winnerIdx := 0
			for i := 1; i < len(tt.sources); i++ {
				if shouldReplace(tt.sources[winnerIdx], tt.sources[i]) {
					winnerIdx = i
				}
			}

			require.Equal(t, tt.expected, winnerIdx, "wrong winner selected")
		})
	}
}

// shouldReplace returns true if newSource should replace currentWinner
// based on the priority rule: trust_level DESC, confidence DESC, observed_at DESC
func shouldReplace(currentWinner, newSource struct {
	trust      int
	confidence float64
	observed   time.Time
}) bool {
	// Higher trust wins
	if newSource.trust > currentWinner.trust {
		return true
	}
	if newSource.trust < currentWinner.trust {
		return false
	}

	// Trust is equal, higher confidence wins
	if newSource.confidence > currentWinner.confidence {
		return true
	}
	if newSource.confidence < currentWinner.confidence {
		return false
	}

	// Trust and confidence are equal, more recent observation wins
	return newSource.observed.After(currentWinner.observed)
}

// TestConflictResolutionEdgeCases tests boundary conditions
func TestConflictResolutionEdgeCases(t *testing.T) {
	now := time.Now()

	t.Run("identical sources", func(t *testing.T) {
		s1 := struct {
			trust      int
			confidence float64
			observed   time.Time
		}{trust: 7, confidence: 0.85, observed: now}

		s2 := s1 // identical

		// When all factors are equal, neither should replace the other
		require.False(t, shouldReplace(s1, s2), "identical sources should not replace each other")
	})

	t.Run("minimum vs maximum trust", func(t *testing.T) {
		minTrust := struct {
			trust      int
			confidence float64
			observed   time.Time
		}{trust: 1, confidence: 1.0, observed: now}

		maxTrust := struct {
			trust      int
			confidence float64
			observed   time.Time
		}{trust: 10, confidence: 0.01, observed: now.Add(-24 * time.Hour)}

		require.True(t, shouldReplace(minTrust, maxTrust), "max trust should always win over min trust")
	})

	t.Run("zero confidence", func(t *testing.T) {
		zeroConf := struct {
			trust      int
			confidence float64
			observed   time.Time
		}{trust: 7, confidence: 0.0, observed: now}

		highConf := struct {
			trust      int
			confidence float64
			observed   time.Time
		}{trust: 7, confidence: 0.5, observed: now.Add(-1 * time.Hour)}

		require.True(t, shouldReplace(zeroConf, highConf), "higher confidence should win even if modest")
	})
}

// TestSupersededProvenance tests that superseded provenance records are not considered
func TestSupersededProvenance(t *testing.T) {
	// This test documents the expected behavior:
	// - Field provenance records with superseded_at NOT NULL should be excluded from conflict resolution
	// - Only records where applied_to_canonical = true AND superseded_at IS NULL participate in resolution

	t.Run("superseded records excluded", func(t *testing.T) {
		// In SQL query, this would be:
		// WHERE applied_to_canonical = true AND superseded_at IS NULL

		type record struct {
			appliedToCanonical bool
			supersededAt       *time.Time
			shouldBeConsidered bool
		}

		now := time.Now()
		records := []record{
			{appliedToCanonical: true, supersededAt: nil, shouldBeConsidered: true},
			{appliedToCanonical: true, supersededAt: &now, shouldBeConsidered: false},
			{appliedToCanonical: false, supersededAt: nil, shouldBeConsidered: false},
			{appliedToCanonical: false, supersededAt: &now, shouldBeConsidered: false},
		}

		for i, rec := range records {
			isActive := rec.appliedToCanonical && rec.supersededAt == nil
			require.Equal(t, rec.shouldBeConsidered, isActive, "record %d active status mismatch", i)
		}
	})
}
