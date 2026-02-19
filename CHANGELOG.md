# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Changed

#### Email Service API - Context Parameter Addition (Breaking Change)

**What Changed:**
The `email.Sender.SendInvitation()` function signature has been updated to accept a `context.Context` as the first parameter:

- **Old signature:** `SendInvitation(to, inviteLink, invitedBy string) error`
- **New signature:** `SendInvitation(ctx context.Context, to, inviteLink, invitedBy string) error`

**Why:**
This change was necessary to support the Resend API's context-aware operations, enable proper timeout control, and follow Go best practices for cancelable operations. The context parameter allows callers to:
- Set deadlines and timeouts for email operations
- Cancel email sending operations if needed
- Propagate request-scoped values (e.g., request IDs for tracing)

**Migration Path:**
If you have code that calls `SendInvitation()`, add a `context.Context` parameter as the first argument:

```go
// Before
err := emailService.SendInvitation(email, link, admin)

// After
err := emailService.SendInvitation(ctx, email, link, admin)
```

For background operations without a parent context, use `context.Background()`:
```go
err := emailService.SendInvitation(context.Background(), email, link, admin)
```

**Impact:**
- **Scope:** Internal package only (`internal/email`)
- **External Impact:** Low - this is an internal package, not exposed in public APIs
- **Internal Callers:** All internal callers have been updated:
  - `internal/domain/users/service.go:321` (CreateUserAndInvite)
  - `internal/domain/users/service.go:896` (ResendInvitation)

**Related:**
- See `docs/admin/email-setup.md` for complete email configuration documentation
- See `internal/email/README_TESTS.md` for testing patterns with the updated API

---

## [0.1.0] - Initial Release

### Added
- Shared Events Library (SEL) backend implementation in Go
- PostgreSQL storage with PostGIS and pgvector support
- Event ingestion and query APIs
- JSON-LD support with context negotiation
- User authentication and API key management
- Admin UI for user and developer management
- Docker Compose deployment setup
- Database migrations with golang-migrate
- Comprehensive test suite

[Unreleased]: https://github.com/Togather-Foundation/server/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/Togather-Foundation/server/releases/tag/v0.1.0
