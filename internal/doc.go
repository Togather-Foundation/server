// Package internal documents the shared event library server internals.
//
// The internal tree is organized by responsibility:
// - api: HTTP handlers, middleware, rendering, and routing
// - domain: business logic and domain models
// - storage: database access and repositories (SQLc + Postgres)
// - jobs: background workers and queues
// - auth, audit, config, metrics, jsonld: shared infrastructure
//
// Code in internal/ is not meant for external import.
package internal
