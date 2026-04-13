// Package ical provides ICS (RFC 5545) parsing, VEVENT-to-EventInput mapping,
// and RRULE expansion for the Togather scraper pipeline.
//
// This package depends on:
//   - github.com/arran4/golang-ical for VCALENDAR/VEVENT parsing
//   - github.com/teambition/rrule-go for RFC 5545 RRULE expansion
//
// See specs/005-ics-integration/spec-phase1.md for the full design.
package ical
