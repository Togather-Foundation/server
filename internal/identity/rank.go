package identity

import (
	"github.com/Togather-Foundation/server/internal/identity/rank"
)

// IdentifierObservation is a single external-identifier observation for a SEL
// entity. Multiple observations for the same (entity, authority) form a group;
// ElectPrimary elects the single row that holds the group's primary slot.
//
// It is a re-export of rank.IdentifierObservation so the canonical ranking lives
// in one dependency-free package shared by the identity store and the storage
// merge path.
type IdentifierObservation = rank.IdentifierObservation

// ElectPrimary returns the observation that should hold the primary slot for a
// group, per the canonical order. It returns (zero, false) when obs is empty.
func ElectPrimary(obs []IdentifierObservation) (IdentifierObservation, bool) {
	return rank.ElectPrimary(obs)
}
