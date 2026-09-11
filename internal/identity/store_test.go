package identity

import (
	"context"
	"errors"
	"testing"
)

// TestRecordObservationTx_InvalidEntityType verifies that an unsupported entity
// type is rejected structurally before any database access (q may be nil).
func TestRecordObservationTx_InvalidEntityType(t *testing.T) {
	store := NewStore(nil)

	cases := []struct {
		name string
		ref  IdentityRef
	}{
		{name: "event", ref: IdentityRef{Type: EntityType("event"), ULID: "01ARZ3NDEKTSV4RRFFQ69G5FAV"}},
		{name: "person", ref: IdentityRef{Type: EntityType("person"), ULID: "01ARZ3NDEKTSV4RRFFQ69G5FAV"}},
		{name: "empty", ref: IdentityRef{Type: EntityType(""), ULID: "01ARZ3NDEKTSV4RRFFQ69G5FAV"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := store.RecordObservationTx(context.Background(), nil, tc.ref, IdentifierObservation{
				Authority: "artsdata",
				URI:       "https://kg.artsdata.ca/resource/K11-0",
				Method:    "manual",
			})
			if !errors.Is(err, ErrInvalidEntityType) {
				t.Fatalf("got err %v, want ErrInvalidEntityType", err)
			}
		})
	}
}
