package identity

import (
	"math"
	"testing"
	"time"
)

// obs is a shorthand builder for IdentifierObservation in tests.
func obs(id int32, method string, trust, priority int32, confidence float64, at time.Time) IdentifierObservation {
	return IdentifierObservation{
		ID:         id,
		Authority:  "artsdata",
		URI:        "https://kg.artsdata.ca/resource/K11-0",
		Method:     method,
		Confidence: confidence,
		ObservedAt: at,
		TrustLevel: trust,
		Priority:   priority,
	}
}

func TestElectPrimary_Empty(t *testing.T) {
	_, ok := ElectPrimary(nil)
	if ok {
		t.Fatal("ElectPrimary(nil) should return ok=false")
	}
	_, ok = ElectPrimary([]IdentifierObservation{})
	if ok {
		t.Fatal("ElectPrimary([]) should return ok=false")
	}
}

func TestElectPrimary_MethodRank(t *testing.T) {
	// The canonical method order: manual > imported > auto_high > auto_low > enrichment_sameas.
	order := []string{"manual", "imported", "auto_high", "auto_low", "enrichment_sameas"}
	for i := 0; i < len(order)-1; i++ {
		higher := obs(1, order[i], 1, 1, 0.5, time.Unix(100, 0))
		lower := obs(2, order[i+1], 1, 1, 0.5, time.Unix(100, 0))
		// Higher method must win regardless of the lower method's otherwise-higher
		// trust/priority/confidence; give the lower method better secondary values.
		lower.TrustLevel = 10
		lower.Confidence = 0.99

		got, ok := ElectPrimary([]IdentifierObservation{lower, higher})
		if !ok {
			t.Fatalf("%s vs %s: unexpected ok=false", order[i], order[i+1])
		}
		if got.Method != higher.Method {
			t.Errorf("%s vs %s: got winner method %q, want %q", order[i], order[i+1], got.Method, higher.Method)
		}
	}
}

func TestElectPrimary_TrustLevelDesc(t *testing.T) {
	high := obs(1, "auto_high", 9, 50, 0.5, time.Unix(100, 0))
	low := obs(2, "auto_high", 5, 50, 0.99, time.Unix(200, 0)) // higher confidence, newer — trust still wins

	got, ok := ElectPrimary([]IdentifierObservation{low, high})
	if !ok {
		t.Fatal("unexpected ok=false")
	}
	if got.ID != high.ID {
		t.Errorf("got winner id %d, want %d (trust_level DESC must precede confidence/observed_at)", got.ID, high.ID)
	}
}

func TestElectPrimary_PriorityAsc(t *testing.T) {
	betterPriority := obs(1, "auto_high", 9, 10, 0.5, time.Unix(100, 0))
	worsePriority := obs(2, "auto_high", 9, 40, 0.99, time.Unix(200, 0))

	got, ok := ElectPrimary([]IdentifierObservation{worsePriority, betterPriority})
	if !ok {
		t.Fatal("unexpected ok=false")
	}
	if got.ID != betterPriority.ID {
		t.Errorf("got winner id %d, want %d (priority_order ASC, lower wins)", got.ID, betterPriority.ID)
	}
}

func TestElectPrimary_ConfidenceDesc(t *testing.T) {
	highConf := obs(1, "auto_high", 9, 10, 0.95, time.Unix(100, 0))
	lowConf := obs(2, "auto_high", 9, 10, 0.90, time.Unix(200, 0))

	got, ok := ElectPrimary([]IdentifierObservation{lowConf, highConf})
	if !ok {
		t.Fatal("unexpected ok=false")
	}
	if got.ID != highConf.ID {
		t.Errorf("got winner id %d, want %d (confidence DESC)", got.ID, highConf.ID)
	}
}

func TestElectPrimary_ObservedAtDesc(t *testing.T) {
	newer := obs(1, "auto_high", 9, 10, 0.90, time.Unix(300, 0))
	older := obs(2, "auto_high", 9, 10, 0.90, time.Unix(100, 0))

	got, ok := ElectPrimary([]IdentifierObservation{older, newer})
	if !ok {
		t.Fatal("unexpected ok=false")
	}
	if got.ID != newer.ID {
		t.Errorf("got winner id %d, want %d (observed_at DESC, newest wins)", got.ID, newer.ID)
	}
}

func TestElectPrimary_IDDescTerminalTieBreak(t *testing.T) {
	// Every other ranking attribute is identical; only id differs.
	higherID := obs(42, "auto_high", 9, 10, 0.90, time.Unix(100, 0))
	lowerID := obs(7, "auto_high", 9, 10, 0.90, time.Unix(100, 0))

	got, ok := ElectPrimary([]IdentifierObservation{lowerID, higherID})
	if !ok {
		t.Fatal("unexpected ok=false")
	}
	if got.ID != higherID.ID {
		t.Errorf("got winner id %d, want %d (id DESC terminal tie-break)", got.ID, higherID.ID)
	}

	// Order in the slice must not matter for the terminal tie-break.
	got2, ok := ElectPrimary([]IdentifierObservation{higherID, lowerID})
	if !ok {
		t.Fatal("unexpected ok=false")
	}
	if got2.ID != higherID.ID {
		t.Errorf("got winner id %d, want %d (id DESC tie-break must be order-independent)", got2.ID, higherID.ID)
	}
}

func TestElectPrimary_Single(t *testing.T) {
	only := obs(1, "manual", 9, 10, 1.0, time.Unix(100, 0))
	got, ok := ElectPrimary([]IdentifierObservation{only})
	if !ok {
		t.Fatal("unexpected ok=false")
	}
	if got.ID != only.ID {
		t.Errorf("got id %d, want %d", got.ID, only.ID)
	}
}

func TestElectPrimary_NaNConfidenceTreatedLowest(t *testing.T) {
	// A NaN confidence must sort below every valid confidence (including 0.0),
	// so the valid-confidence observation wins regardless of id.
	nanConf := obs(99, "auto_high", 9, 10, math.NaN(), time.Unix(100, 0))
	zeroConf := obs(1, "auto_high", 9, 10, 0.0, time.Unix(100, 0))

	got, ok := ElectPrimary([]IdentifierObservation{nanConf, zeroConf})
	if !ok {
		t.Fatal("unexpected ok=false")
	}
	if got.ID != zeroConf.ID {
		t.Errorf("got winner id %d, want %d (NaN confidence must be lowest)", got.ID, zeroConf.ID)
	}

	// Two NaN confidences fall through to the terminal id tie-break.
	nanLow := obs(2, "auto_high", 9, 10, math.NaN(), time.Unix(100, 0))
	nanHigh := obs(7, "auto_high", 9, 10, math.NaN(), time.Unix(100, 0))

	got2, ok := ElectPrimary([]IdentifierObservation{nanLow, nanHigh})
	if !ok {
		t.Fatal("unexpected ok=false")
	}
	if got2.ID != nanHigh.ID {
		t.Errorf("got winner id %d, want %d (NaN vs NaN falls to id DESC)", got2.ID, nanHigh.ID)
	}
}
