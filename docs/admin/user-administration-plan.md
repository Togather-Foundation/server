# User Administration UI & Endpoints - Implementation Plan

## Executive Summary

This document outlines the implementation of a comprehensive user administration system for the Togather SEL Backend, including:
- **Full CRUD operations** for user management (Create, Read, Update, Delete)
- **Email invitation system** for new users with secure token-based password setup
- **Role management** for admin/editor/viewer permissions with potential for future granular permissions
- **Comprehensive audit trail** tracking all user actions, login history, and administrative changes
- **Admin UI pages** matching the existing Tabler-based design system

## Current System Analysis

### Existing Infrastructure ✅

**Database Schema** (Already in place):
- `users` table with all necessary fields (username, email, password_hash, role, is_active, created_at, last_login_at)
- Roles: `admin`, `editor`, `viewer`
- Indexes on `role` and `is_active`
- Location: `internal/storage/postgres/migrations/000004_auth.up.sql`

**SQLc Queries** (Already defined in `internal/storage/postgres/queries/auth.sql`):
- ✅ `GetUserByEmail`, `GetUserByUsername`, `GetUserByID`
- ✅ `CreateUser`, `UpdateUser`, `UpdateUserPassword`
- ✅ `ListUsers`, `UpdateLastLogin`

**Authentication Infrastructure**:
- ✅ JWT-based authentication with token generation/validation
- ✅ Audit logging system (`internal/audit/logger.go`)
- ✅ Admin middleware for cookie-based UI auth
- ✅ CSRF protection for HTML forms
- ✅ bcrypt password hashing

**Admin UI Framework**:
- ✅ Tabler UI framework with navigation header
- ✅ `components.js` with shared utilities (toasts, modals, confirmations)
- ✅ `api.js` centralized API client with JWT authentication
- ✅ Established patterns for list views, forms, and actions

### Gaps to Address 🚧

1. **User Invitation System**: No mechanism for email-based user invitations with secure token links
2. **User Management Endpoints**: No REST API endpoints for user CRUD operations
3. **User Management UI**: No admin pages for viewing/managing users
4. **Activity Tracking**: Audit logs exist but no UI to view user activity history
5. **Password Reset**: No self-service or admin-initiated password reset flow
6. **Email Integration**: No email sending infrastructure for invitations/notifications

---

## Architecture & Design Decisions

### 1. User Invitation Flow

**Chosen Approach**: Admin Creates + Email Invitation with secure tokens

```
┌─────────────┐     ┌──────────────┐     ┌─────────────┐     ┌──────────────┐
│ Admin       │     │ Backend      │     │ Email       │     │ New User     │
│ Creates     │────>│ Generates    │────>│ Service     │────>│ Receives     │
│ User        │     │ Token +      │     │ Sends       │     │ Invitation   │
│ Account     │     │ Invite Link  │     │ Email       │     │ Link         │
└─────────────┘     └──────────────┘     └─────────────┘     └──────────────┘
                                                                      │
                                                                      ▼
┌──────────────┐    ┌──────────────┐    ┌──────────────────────────┐
│ User is      │<───│ Password Set │<───│ User Clicks Link &       │
│ Active       │    │ & Activated  │    │ Sets Password            │
└──────────────┘    └──────────────┘    └──────────────────────────┘
```

**Database Schema Addition** (New table required):
```sql
CREATE TABLE user_invitations (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  token TEXT NOT NULL UNIQUE,
  email TEXT NOT NULL,
  expires_at TIMESTAMPTZ NOT NULL,
  accepted_at TIMESTAMPTZ,
  created_by UUID REFERENCES users(id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_user_invitations_token ON user_invitations(token, expires_at);
CREATE INDEX idx_user_invitations_user ON user_invitations(user_id);
```

**Security Considerations**:
- Invitation tokens: 32-byte cryptographically secure random strings (URL-safe base64)
- Token expiration: 7 days default (configurable)
- One-time use: Token invalidated after password is set
- Users created in inactive state until they accept invitation
- All invitation actions audit logged

### 2. Role Management System

**Current Roles** (from database constraint):
- `admin`: Full system access, can manage all resources
- `editor`: Can create/edit events, places, organizations (future implementation)
- `viewer`: Read-only access to admin UI (future implementation)

**Endpoint-Level Authorization** (To be implemented):
```go
// Middleware checks
func RequireAdmin(next http.Handler) http.Handler {
    // Only admin role can proceed
}

func RequireEditor(next http.Handler) http.Handler {
    // Admin or editor roles can proceed
}
```

**UI Role Assignment**:
- Dropdown selection on user create/edit forms
- Visual role badges on user list
- Role descriptions with permission summaries

**Future Extensibility** (Out of scope for initial implementation):
- Granular permissions table (e.g., `user_permissions`)
- Resource-level permissions (e.g., "can delete events")
- Permission groups/templates

### 3. Audit Trail Design

**What to Track**:

| Event Type | Tracked Data | Retention |
|------------|-------------|-----------|
| User login | Username, IP, timestamp, success/failure | 90 days |
| User created | Admin who created, email, role | Permanent |
| User updated | Fields changed, admin who updated | Permanent |
| User deactivated | Reason, admin who deactivated | Permanent |
| User deleted | Reason, admin who deleted | Permanent |
| Password changed | Initiated by (self/admin), timestamp | Permanent |
| Role changed | Old role → new role, admin | Permanent |
| Invitation sent | Email, expiry, admin who sent | Permanent |
| Invitation accepted | Timestamp, IP address | Permanent |

**Event Types** (extending existing audit log):
```go
const (
    EventUserCreated       = "user.created"
    EventUserUpdated       = "user.updated"
    EventUserDeactivated   = "user.deactivated"
    EventUserDeleted       = "user.deleted"
    EventPasswordChanged   = "user.password_changed"
    EventRoleChanged       = "user.role_changed"
    EventInvitationSent    = "user.invitation_sent"
    EventInvitationAccepted = "user.invitation_accepted"
)
```

**Audit Log UI**:
- Per-user activity tab showing all actions for that user
- System-wide audit log page (admin only)
- Filters: date range, event type, user, admin who performed action
- Export to CSV for compliance reporting

### 4. Email Integration Strategy

**Implementation**: Gmail SMTP with environment config

Gmail SMTP Configuration:
- Host: `smtp.gmail.com`
- Port: `587` (TLS)
- Authentication: App Password (not regular Gmail password)
- Setup: https://support.google.com/accounts/answer/185833

```go
// config/email.go
type EmailConfig struct {
    From         string // "noreply@togather.foundation"
    SMTPHost     string // "smtp.gmail.com"
    SMTPPort     int    // 587
    SMTPUser     string // Gmail address
    SMTPPassword string // Gmail App Password
    Enabled      bool   // Allow disabling in dev/test
}
```

**Note**: For production, generate a Gmail App Password at https://myaccount.google.com/apppasswords

**Email Templates**:
- `invitation.html` - User invitation with password setup link
- `password_reset.html` - Password reset link (future)
- `account_activated.html` - Welcome message after account activation

---

## Implementation Phases

### Phase 1: Backend Foundation

#### 1.1 Database Migrations
- **File**: `internal/storage/postgres/migrations/000018_user_invitations.up.sql`
- **File**: `internal/storage/postgres/migrations/000018_user_invitations.down.sql`
- Create `user_invitations` table with proper indexes and constraints

#### 1.2 SQLc Queries
- **File**: `internal/storage/postgres/queries/auth.sql` (additions)
- Add queries for: `CreateUserInvitation`, `GetUserInvitationByToken`, `MarkInvitationAccepted`
- Add queries for: `ListPendingInvitationsForUser`, `DeactivateUser`, `ActivateUser`, `DeleteUser`
- Add filtered list query: `ListUsersWithFilters`, `CountUsers`

#### 1.3 Email Service
- **File**: `internal/email/service.go`
- **File**: `web/email/templates/invitation.html`
- Implement SMTP-based email service with template rendering
- Support for disabled mode (development/testing)

#### 1.4 User Management Domain Service
- **File**: `internal/domain/users/service.go`
- Methods: `CreateUserAndInvite`, `AcceptInvitation`, `UpdateUser`, `DeactivateUser`, `DeleteUser`, `ListUsers`
- Integrate with email service and audit logger
- Secure token generation (32-byte cryptographically secure)

### Phase 2: REST API Endpoints

#### 2.1 User Management Handler
- **File**: `internal/api/handlers/admin_users.go`
- Endpoints:
  - `POST /api/v1/admin/users` - Create user and send invitation
  - `GET /api/v1/admin/users` - List all users with filters
  - `GET /api/v1/admin/users/{id}` - Get single user details
  - `PUT /api/v1/admin/users/{id}` - Update user details
  - `DELETE /api/v1/admin/users/{id}` - Delete user
  - `POST /api/v1/admin/users/{id}/deactivate` - Deactivate user
  - `POST /api/v1/admin/users/{id}/activate` - Reactivate user
  - `POST /api/v1/admin/users/{id}/resend-invitation` - Resend invitation
  - `GET /api/v1/admin/users/{id}/activity` - Get user activity audit log

#### 2.2 Public Invitation Acceptance Handler
- **File**: `internal/api/handlers/public_invitations.go`
- Endpoints:
  - `GET /accept-invitation?token=...` - Public page to accept invitation
  - `POST /api/v1/accept-invitation` - Submit new password

#### 2.3 Router Registration
- **File**: `internal/api/router.go` (additions)
- Register all user management routes with proper middleware
- JWT auth + admin rate limiting for admin endpoints
- No auth for public invitation endpoints

### Phase 3: Admin UI Pages

#### 3.1 User List Page
- **File**: `web/admin/templates/users_list.html`
- **File**: `web/admin/static/js/users.js`
- Features: list view, filters (status, role, search), create button, action buttons
- Table columns: username, email, role, status, last login, created, actions

#### 3.2 User Create/Edit Modal
- **File**: `web/admin/templates/_user_modal.html` (included in _footer.html)
- **JavaScript**: Handled in `users.js`
- Form fields: username, email, role dropdown
- Validation and submission logic

#### 3.3 User Activity Page
- **File**: `web/admin/templates/user_activity.html`
- **File**: `web/admin/static/js/user-activity.js`
- Sections: user info card, activity summary, activity timeline
- Filters by event type, date range
- Export to CSV functionality

#### 3.4 Invitation Acceptance Page
- **File**: `web/admin/templates/accept_invitation.html`
- **File**: `web/admin/static/js/accept-invitation.js`
- Public-facing page (no auth required)
- Password strength validation
- Confirm password matching

#### 3.5 Update Navigation Header
- **File**: `web/admin/templates/_header.html`
- Add "Users" link to navigation menu

#### 3.6 API Client Updates
- **File**: `web/admin/static/js/api.js`
- Add `users` namespace with all CRUD methods

#### 3.7 Handler Registration
- **File**: `internal/api/handlers/admin_html.go`
- Add handlers for: `ServeUsersList`, `ServeUserActivity`

### Phase 4: Testing & Documentation

#### 4.1 Integration Tests
- **File**: `tests/integration/admin_users_test.go`
- Test: CreateUserAndInvite, AcceptInvitation, UpdateUser, DeactivateUser
- Test: Audit logging, email sending (mocked)

#### 4.2 E2E Tests
- **File**: `tests/e2e/admin_users_test.go`
- Test full user management flows with Playwright
- Test invitation acceptance flow

#### 4.3 Documentation
- **File**: `docs/admin/admin-ui-components.md` (update — user management section)
- **File**: `docs/api/admin-users.md` (new)
- Document user management features, API endpoints, configuration

---

## Configuration Changes

### Environment Variables

Add to `.env.example` and deployment configs:

```bash
# Email Configuration
EMAIL_ENABLED=true
EMAIL_FROM=noreply@togather.foundation
SMTP_HOST=smtp.gmail.com
SMTP_PORT=587
SMTP_USER=your-email@gmail.com
SMTP_PASSWORD=your-app-password

# Invitation Settings
INVITATION_EXPIRY_HOURS=168  # 7 days default
```

### Config Struct Updates

**File**: `internal/config/config.go`

```go
type Config struct {
    // ... existing fields
    
    Email EmailConfig `mapstructure:"email"`
}

type EmailConfig struct {
    Enabled      bool   `mapstructure:"enabled"`
    From         string `mapstructure:"from"`
    SMTPHost     string `mapstructure:"smtp_host"`
    SMTPPort     int    `mapstructure:"smtp_port"`
    SMTPUser     string `mapstructure:"smtp_user"`
    SMTPPassword string `mapstructure:"smtp_password"`
}
```

---

## Security Considerations

### Threat Model & Mitigations

| Threat | Mitigation |
|--------|------------|
| Email spoofing | SPF/DKIM/DMARC records configured for domain |
| Token prediction | Cryptographically secure random token generation (32 bytes) |
| Token reuse | One-time use enforced in database |
| Expired tokens | Database query filters out expired tokens |
| Brute force password guessing | Rate limiting on password submission endpoint |
| XSS in user inputs | All user inputs escaped before rendering |
| CSRF on user actions | CSRF tokens on all state-changing operations |
| Privilege escalation | Role checks enforced at middleware level + handler level |
| Audit log tampering | Audit logs write-only, separate from user tables |

### Password Requirements

- Minimum 8 characters (enforced client-side and server-side)
- No complexity requirements initially (follow modern NIST guidelines)
- Hashed with bcrypt (cost factor 12)
- No password reuse checking initially

### Audit Logging

- All user management actions logged
- Logs include: who, what, when, from where (IP)
- Logs stored in PostgreSQL (consider append-only table)
- Log retention: 90 days for logins, permanent for account changes

---

## Deployment Checklist

### Pre-Deployment

- [ ] Run database migrations
- [ ] Configure email settings (SMTP credentials)
- [ ] Test email delivery in staging
- [ ] Verify invitation links work with production domain
- [ ] Run integration tests
- [ ] Run E2E tests
- [ ] Review security settings (CSRF keys, JWT secrets)

### Post-Deployment

- [ ] Create initial admin user via CLI or SQL
- [ ] Test login with admin user
- [ ] Test creating new user and invitation flow
- [ ] Verify audit logging works
- [ ] Monitor email delivery metrics
- [ ] Check error logs for any issues

### Rollback Plan

- [ ] Database migration rollback scripts ready
- [ ] Previous version deployment artifacts available
- [ ] Communication plan for affected users
- [ ] Manual user creation process documented as fallback

---

## Future Enhancements (Out of Scope)

These are intentionally deferred for later iterations:

1. **Password Reset Flow**: Self-service password reset via email
2. **Two-Factor Authentication (2FA)**: TOTP or SMS-based 2FA
3. **Session Management**: Active sessions view, force logout
4. **Permission System**: Granular resource-level permissions
5. **User Groups**: Group-based permission assignment
6. **OAuth/SSO Integration**: Google, GitHub, SAML support
7. **Bulk User Operations**: CSV import/export, bulk deactivation
8. **Advanced Audit**: Query builder UI, advanced filtering, dashboards
9. **Email Notifications**: Account activity alerts, security notifications
10. **API Rate Limits Per User**: User-specific rate limiting

---

## Success Criteria

### Functional Requirements ✅

- [ ] Admins can create new users with role assignment
- [ ] New users receive email invitations with secure links
- [ ] Users can set passwords and activate accounts via invitation
- [ ] Admins can view list of all users with filtering
- [ ] Admins can edit user details (username, email, role)
- [ ] Admins can deactivate/reactivate users
- [ ] Admins can view individual user activity logs
- [ ] All user management actions are audit logged
- [ ] UI matches existing Tabler design system
- [ ] API endpoints follow RESTful patterns
- [ ] Invitation links expire after 7 days
- [ ] Invitation tokens are cryptographically secure

### Non-Functional Requirements ✅

- [ ] Page load times < 2 seconds
- [ ] No JavaScript console errors
- [ ] Mobile-responsive UI
- [ ] WCAG 2.1 Level AA accessibility compliance
- [ ] 50%+ test coverage for user service
- [ ] E2E tests cover critical user flows
- [ ] CSP-compliant (no inline scripts)
- [ ] Audit logs queryable and exportable

---

## Estimated Timeline

**Total: 5 days** (assuming one developer, full-time)

- **Day 1**: Database migrations, email service, domain service foundation
- **Day 2**: REST API endpoints, integration with router
- **Day 3**: User list page, create/edit modals, API client updates
- **Day 4**: User activity page, invitation acceptance page
- **Day 5**: Testing (integration + E2E), documentation, deployment prep

**Parallel work opportunities**:
- Frontend and backend can be developed simultaneously
- Email templates can be designed while code is being written
- Documentation can be written incrementally

---

## Design Trade-offs

| Decision | Alternative Considered | Rationale |
|----------|----------------------|-----------|
| Email invitations | Admin sets initial password | More secure, better UX, prevents password transmission |
| 7-day invitation expiry | 24 hours or 30 days | Balances security and convenience |
| Inactive state until acceptance | Active immediately | Prevents unauthorized access before user confirms |
| JWT for API, Cookie for UI | JWT only | Better UX for browser-based admin UI |
| Simple role system | Granular permissions | Faster implementation, can extend later |
| Database audit log | File-based logging | Queryable, structured, survives container restarts |

---

## Summary

This plan delivers a **production-ready user administration system** with:

✅ **Full CRUD** for user management  
✅ **Secure email invitations** with token-based password setup  
✅ **Role management** (admin/editor/viewer)  
✅ **Comprehensive audit trail** for compliance  
✅ **Polished admin UI** matching existing design system  
✅ **RESTful API** following project conventions  
✅ **Complete test coverage** (unit + integration + E2E)  
✅ **Security-first design** (bcrypt, CSRF, XSS protection, audit logging)  

The implementation follows the existing codebase patterns (SQLc, Tabler UI, middleware architecture) and can be extended in the future with additional features like 2FA, SSO, and granular permissions.
