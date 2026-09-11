package identity

// EntityType identifies the SEL entity kind an identity is scoped to.
// Phase 1 covers places and organizations only; events are deferred to Phase 4.
type EntityType string

const (
	EntityTypePlace        EntityType = "place"
	EntityTypeOrganization EntityType = "organization"
)

// IdentityRef addresses a SEL entity by its canonical ULID. External URIs are
// sameAs aliases; the ULID is the authoritative identity.
type IdentityRef struct {
	Type EntityType `json:"entity_type"`
	ULID string     `json:"entity_id"`
}
