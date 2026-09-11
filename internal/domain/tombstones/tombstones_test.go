package tombstones

import (
	"encoding/json"
	"testing"

	"github.com/oklog/ulid/v2"
)

func TestBuildPlaceURI(t *testing.T) {
	base := "https://toronto.togather.foundation"
	ulid := ulid.Make().String()

	uri := BuildPlaceURI(base, ulid)
	if uri != "https://toronto.togather.foundation/places/"+ulid {
		t.Fatalf("BuildPlaceURI = %q, want %q", uri, "https://toronto.togather.foundation/places/"+ulid)
	}

	if got := BuildPlaceURI("", ulid); got != "" {
		t.Fatalf("BuildPlaceURI with empty base = %q, want empty", got)
	}
	if got := BuildPlaceURI(base, ""); got != "" {
		t.Fatalf("BuildPlaceURI with empty ulid = %q, want empty", got)
	}
}

func TestBuildOrganizationURI(t *testing.T) {
	base := "https://toronto.togather.foundation"
	ulid := ulid.Make().String()

	uri := BuildOrganizationURI(base, ulid)
	if uri != "https://toronto.togather.foundation/organizations/"+ulid {
		t.Fatalf("BuildOrganizationURI = %q, want %q", uri, "https://toronto.togather.foundation/organizations/"+ulid)
	}

	if got := BuildOrganizationURI("", ulid); got != "" {
		t.Fatalf("BuildOrganizationURI with empty base = %q, want empty", got)
	}
	if got := BuildOrganizationURI(base, ""); got != "" {
		t.Fatalf("BuildOrganizationURI with empty ulid = %q, want empty", got)
	}
}

func TestBuildPlaceTombstonePayload(t *testing.T) {
	base := "https://toronto.togather.foundation"
	ulid := ulid.Make().String()

	payload, err := BuildPlaceTombstonePayload(ulid, "Massey Hall", "merged", base)
	if err != nil {
		t.Fatalf("BuildPlaceTombstonePayload: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(payload, &m); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}

	if m["@type"] != "Place" {
		t.Fatalf("@type = %v, want Place", m["@type"])
	}
	if m["@id"] != "https://toronto.togather.foundation/places/"+ulid {
		t.Fatalf("@id = %v", m["@id"])
	}
	if m["name"] != "Massey Hall" {
		t.Fatalf("name = %v", m["name"])
	}
	if m["sel:tombstone"] != true {
		t.Fatalf("sel:tombstone = %v, want true", m["sel:tombstone"])
	}
	if m["sel:deletionReason"] != "merged" {
		t.Fatalf("sel:deletionReason = %v, want merged", m["sel:deletionReason"])
	}
	if _, ok := m["sel:deletedAt"].(string); !ok {
		t.Fatalf("sel:deletedAt missing or non-string: %v", m["sel:deletedAt"])
	}
}

func TestBuildOrganizationTombstonePayload(t *testing.T) {
	base := "https://toronto.togather.foundation"
	ulid := ulid.Make().String()

	payload, err := BuildOrganizationTombstonePayload(ulid, "Arts Council", "merged", base)
	if err != nil {
		t.Fatalf("BuildOrganizationTombstonePayload: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(payload, &m); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}

	if m["@type"] != "Organization" {
		t.Fatalf("@type = %v, want Organization", m["@type"])
	}
	if m["@id"] != "https://toronto.togather.foundation/organizations/"+ulid {
		t.Fatalf("@id = %v", m["@id"])
	}
	if m["sel:tombstone"] != true {
		t.Fatalf("sel:tombstone = %v, want true", m["sel:tombstone"])
	}
	if m["sel:deletionReason"] != "merged" {
		t.Fatalf("sel:deletionReason = %v, want merged", m["sel:deletionReason"])
	}
}
