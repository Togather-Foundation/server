# User Management Guide

**Audience:** System administrators managing user accounts in a Togather SEL node

> For component library and JS patterns, see [Admin UI Components](admin-ui-components.md).

This guide explains how to manage user accounts in the Togather Shared Events Library (SEL) Backend. The system uses an invitation-based workflow to ensure email ownership verification and secure account creation.

---

## Table of Contents

- [Overview](#overview)
- [User Roles](#user-roles)
- [User States](#user-states)
- [Creating Users](#creating-users)
- [Managing Users](#managing-users)
- [Deactivating and Reactivating Users](#deactivating-and-reactivating-users)
- [Deleting Users](#deleting-users)
- [Invitation Acceptance Flow](#invitation-acceptance-flow)
- [Viewing User Activity](#viewing-user-activity)
- [Troubleshooting](#troubleshooting)
- [User Management API Reference](#user-management-api-reference)

---

## Overview

The Togather user administration system follows these principles:

- **Invitation-based signup**: All users are invited by administrators via email
- **Email ownership verification**: Users must accept invitations to prove they own the email address
- **Role-based access control**: Three roles (admin, editor, viewer) with different permissions
- **Audit trail**: All user management actions are logged for security and compliance
- **Soft delete**: Deleted users are retained in the database for audit history

**Key Workflow:**

1. Admin creates a new user account (user is inactive)
2. System sends invitation email with secure token link (valid for 7 days)
3. User clicks link and sets their password
4. User account becomes active and can log in

---

## User Roles

The system has three built-in roles with different permission levels:

### Admin

**Permissions:**
- Full system access
- Create, edit, and delete users
- Manage all events, places, and organizations
- View audit logs
- Configure system settings

**Use Cases:**
- Node operators
- System administrators
- Technical staff responsible for the SEL node

### Editor

**Permissions:**
- Create and edit events, places, and organizations
- View own created content
- Cannot manage users or system settings

**Use Cases:**
- Content managers
- Event organizers
- Community contributors adding events to the SEL

**Note:** Editor permissions for event/place/organization management are planned but not yet implemented. Currently editors have read-only access like viewers.

### Viewer

**Permissions:**
- Read-only access to the system
- Can view events, places, and organizations
- Cannot create, edit, or delete any content

**Use Cases:**
- Auditors
- Read-only API consumers
- Observers needing visibility without edit rights

### Role Badge Colors

In the admin UI, roles are displayed with these badge styles:

```html
<span class="badge bg-danger">Admin</span>
<span class="badge bg-primary">Editor</span>
<span class="badge bg-secondary">Viewer</span>
```

---

## User States

Users can be in one of these states:

| State | Description | Can Login? | Actions Available |
|-------|-------------|------------|-------------------|
| **Inactive (Pending Invitation)** | User created but hasn't accepted invitation yet | ❌ No | Admin can resend invitation |
| **Active** | User has accepted invitation and set password | ✅ Yes | Admin can deactivate or delete |
| **Inactive (Deactivated)** | Admin has deactivated the account | ❌ No | Admin can reactivate |
| **Deleted** | Account soft-deleted (retained for audit) | ❌ No | Data remains in database, cannot be restored via UI |

**State Badges:**

```html
<span class="badge bg-warning">Pending Invitation</span>
<span class="badge bg-success">Active</span>
<span class="badge bg-secondary">Inactive</span>
```

---

## Creating Users

### Prerequisites

1. **Email configuration must be set up** - See [Email Setup Guide](email-setup.md)
2. **You must be logged in as an admin**
3. **User's email address must be valid and unique**

### Steps (Admin UI)

1. Navigate to **Users** in the admin navigation menu
2. Click **Create User** button (top-right; keyboard shortcut: `Ctrl+N`)
3. Fill in the user creation form:
   - **Username**: 3-50 alphanumeric characters (unique)
   - **Email**: Valid email address (unique)
   - **Role**: Select admin, editor, or viewer
4. Click **Create & Send Invitation**

**Create User Modal HTML:**

```html
<form id="create-user-form" class="modal-content">
    <div class="modal-header">
        <h5 class="modal-title">Create New User</h5>
        <button type="button" class="btn-close" data-bs-dismiss="modal"></button>
    </div>
    <div class="modal-body">
        <div class="mb-3">
            <label class="form-label required">Username</label>
            <input type="text" class="form-control" name="username" required>
            <small class="form-hint">Alphanumeric, 3-20 characters</small>
        </div>
        
        <div class="mb-3">
            <label class="form-label required">Email</label>
            <input type="email" class="form-control" name="email" required>
            <small class="form-hint">Invitation will be sent to this address</small>
        </div>
        
        <div class="mb-3">
            <label class="form-label required">Role</label>
            <select class="form-select" name="role" required>
                <option value="viewer">Viewer - Read-only access</option>
                <option value="editor">Editor - Create and edit events</option>
                <option value="admin">Admin - Full access</option>
            </select>
        </div>
    </div>
    <div class="modal-footer">
        <button type="button" class="btn" data-bs-dismiss="modal">Cancel</button>
        <button type="submit" class="btn btn-primary">Create & Send Invitation</button>
    </div>
</form>
```

**Validation Rules:**
- Username: 3-20 alphanumeric characters, no spaces
- Email: Valid email format, must be unique
- Role: One of admin, editor, viewer

### What Happens Next

1. User record is created in **inactive state** (no password yet)
2. Invitation email is sent to the user's email address
3. Email contains a secure invitation link (valid for 7 days)
4. User must click the link and set a password to activate their account

### User Experience: Accepting the Invitation

The invited user receives an email with subject "You've been invited to SEL Admin". The email contains a link to `/admin/accept-invitation?token=...` where they can set their password (minimum 12 characters, must include uppercase, lowercase, number, and special character). Once they submit the form, their account becomes active and they can log in.

For the full invitation acceptance UI flow and HTML templates, see [Invitation Acceptance Flow](#invitation-acceptance-flow).

### API Endpoint

```bash
curl -X POST https://your-domain.com/api/v1/admin/users \
  -H "Authorization: Bearer YOUR_JWT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "username": "janedoe",
    "email": "jane@example.com",
    "role": "editor"
  }'
```

**Response:** User object with `is_active: false`

---

## Managing Users

### Viewing All Users

**Admin UI:**
- Navigate to **Users** page
- View paginated list of all users
- Filter by:
  - **Status**: Active or Inactive
  - **Role**: Admin, Editor, or Viewer
- Search by username or email

**User List Table HTML:**

```html
<table class="table table-vcenter card-table">
    <thead>
        <tr>
            <th>Username</th>
            <th>Email</th>
            <th>Role</th>
            <th>Status</th>
            <th>Created</th>
            <th>Last Active</th>
            <th class="w-1">Actions</th>
        </tr>
    </thead>
    <tbody id="users-table">
        <!-- Populated via JS -->
    </tbody>
</table>
```

**API Endpoint:**

```bash
# List all users
curl -X GET https://your-domain.com/api/v1/admin/users \
  -H "Authorization: Bearer YOUR_JWT_TOKEN"

# Filter by active admins
curl -X GET "https://your-domain.com/api/v1/admin/users?status=active&role=admin" \
  -H "Authorization: Bearer YOUR_JWT_TOKEN"
```

### Viewing a Single User

**Admin UI:**
- Click on a user in the user list
- View detailed user information:
  - Username, email, role
  - Active status
  - Created date
  - Last login date

**API Endpoint:**

```bash
curl -X GET https://your-domain.com/api/v1/admin/users/{user_id} \
  -H "Authorization: Bearer YOUR_JWT_TOKEN"
```

### Updating User Details

You can update a user's username, email, or role.

**Admin UI:**
1. Click **Edit** button next to the user
2. Modify username, email, or role
3. Click **Save Changes**

**Edit User Modal HTML:**

```html
<form id="edit-user-form">
    <div class="modal-body">
        <div class="mb-3">
            <label class="form-label">User ID</label>
            <input type="text" class="form-control" disabled value="01HQXYZ...">
            <small class="form-hint">Immutable identifier</small>
        </div>
        
        <div class="mb-3">
            <label class="form-label required">Username</label>
            <input type="text" class="form-control" name="username" required>
        </div>
        
        <div class="mb-3">
            <label class="form-label required">Email</label>
            <input type="email" class="form-control" name="email" required>
        </div>
        
        <div class="mb-3">
            <label class="form-label required">Role</label>
            <select class="form-select" name="role" required>
                <option value="viewer">Viewer</option>
                <option value="editor">Editor</option>
                <option value="admin">Admin</option>
            </select>
        </div>
    </div>
    <div class="modal-footer">
        <button type="button" class="btn" data-bs-dismiss="modal">Cancel</button>
        <button type="submit" class="btn btn-primary">Save Changes</button>
    </div>
</form>
```

**Non-Editable Fields:**
- User ID (ULID, immutable)
- Created date
- Password (changed via separate flow)

**API Endpoint:**

```bash
curl -X PUT https://your-domain.com/api/v1/admin/users/{user_id} \
  -H "Authorization: Bearer YOUR_JWT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "username": "jane.doe",
    "email": "jane.doe@example.com",
    "role": "admin"
  }'
```

**Validation:**
- Username must be unique (3-50 alphanumeric characters)
- Email must be unique and valid format
- Role must be `admin`, `editor`, or `viewer`

### Resending Invitations

If a user didn't receive their invitation email or the token expired:

**Admin UI:**
1. Find the inactive user in the user list
2. Click **Resend Invitation** button (visible only for pending users; keyboard shortcut: `Shift+R`)
3. A new invitation email will be sent with a fresh 7-day token

**Resend Button HTML:**

```html
<button class="btn btn-sm btn-ghost-secondary" onclick="resendInvitation('${user.id}')">
    Resend Invitation
</button>
```

**API Endpoint:**

```bash
curl -X POST https://your-domain.com/api/v1/admin/users/{user_id}/resend-invitation \
  -H "Authorization: Bearer YOUR_JWT_TOKEN"
```

**Notes:**
- Only works for **inactive users** who haven't accepted their invitation
- Generates a **new token** (previous tokens become invalid)
- Resets expiration to **7 days from now**

---

## Deactivating and Reactivating Users

### Deactivating a User

Deactivation prevents a user from logging in **without deleting their account**. This is useful for:
- Temporarily suspending access
- Users who no longer need access but whose data should remain
- Security incidents requiring immediate access revocation

**Admin UI:**
1. Find the user in the user list
2. Click **Deactivate** button
3. Confirm the action

**Deactivate Modal HTML:**

```html
<div class="modal-body text-center py-4">
    <svg class="icon mb-2 text-warning icon-lg">...</svg>
    <h3>Deactivate User?</h3>
    <div class="text-muted">
        <strong id="deactivate-username"></strong> will no longer be able to log in.
        You can reactivate them later.
    </div>
</div>
<div class="modal-footer">
    <button type="button" class="btn" data-bs-dismiss="modal">Cancel</button>
    <button type="button" class="btn btn-warning" id="confirm-deactivate">Deactivate</button>
</div>
```

**API Endpoint:**

```bash
curl -X POST https://your-domain.com/api/v1/admin/users/{user_id}/deactivate \
  -H "Authorization: Bearer YOUR_JWT_TOKEN"
```

**Effects:**
- User cannot log in
- User data remains in database
- Can be reactivated at any time

### Reactivating a User

**Admin UI:**
1. Find the deactivated user in the user list (filter by Status: Inactive)
2. Click **Activate** button
3. User can now log in again

**API Endpoint:**

```bash
curl -X POST https://your-domain.com/api/v1/admin/users/{user_id}/activate \
  -H "Authorization: Bearer YOUR_JWT_TOKEN"
```

---

## Deleting Users

**⚠️ Important:** User deletion is a **soft delete**. User data remains in the database for audit purposes but the account cannot be restored via the UI.

### When to Delete vs Deactivate

| Use Case | Action | Reason |
|----------|--------|--------|
| User no longer needs access | **Deactivate** | Can be reactivated later |
| User requests account deletion (GDPR) | **Delete** | Marks user as deleted, data retained for audit |
| Security incident | **Deactivate first**, then delete if needed | Preserves audit trail |
| User violated terms | **Delete** | Account permanently disabled |

### Deleting a User

**Admin UI:**
1. Find the user in the user list
2. Click **Delete** button
3. Confirm the deletion (this action is permanent)

**Delete Modal HTML:**

```html
<div class="modal-body text-center py-4">
    <svg class="icon mb-2 text-danger icon-lg">...</svg>
    <h3>Delete User?</h3>
    <div class="text-muted">
        This will permanently remove <strong id="delete-username"></strong> from the system.
        This action cannot be undone, but activity logs will be preserved.
    </div>
</div>
<div class="modal-footer">
    <button type="button" class="btn" data-bs-dismiss="modal">Cancel</button>
    <button type="button" class="btn btn-danger" id="confirm-delete">Delete User</button>
</div>
```

**⚠️ Warning:** Deleting yourself will log you out immediately.

**API Endpoint:**

```bash
curl -X DELETE https://your-domain.com/api/v1/admin/users/{user_id} \
  -H "Authorization: Bearer YOUR_JWT_TOKEN"
```

**Effects:**
- User cannot log in
- User marked as deleted in database (`deleted_at` timestamp set)
- User excluded from normal user lists
- User data retained for audit purposes
- **Cannot be restored via UI** (database recovery required)

---

## Invitation Acceptance Flow

### What Users Receive

When an admin creates a user account, the invited user receives:

**Email Subject:** "You've been invited to SEL Admin"

**Email Body:**
```
Hi [Username],

You've been invited to join the SEL Admin interface as a [Role].

Click the link below to accept your invitation and set your password:
https://toronto.togather.foundation/admin/accept-invitation?token=abc123xyz...

This invitation expires on [Date] (7 days from now).

Your Role: [admin/editor/viewer]
- [List of permissions for this role]

If you didn't expect this invitation, please ignore this email.

Questions? Contact your administrator.
```

### Accepting the Invitation

**Step 1: Click Invitation Link**
- Link opens to `/admin/accept-invitation?token=...`
- Token is validated (not expired, not already used)
- Username and email are pre-filled

**Step 2: Set Password**

```html
<form id="accept-invitation-form" class="card card-md">
    <div class="card-body">
        <h2 class="card-title text-center mb-4">Set Your Password</h2>
        
        <div class="mb-3">
            <label class="form-label">Username</label>
            <input type="text" class="form-control" disabled value="john_doe">
        </div>
        
        <div class="mb-3">
            <label class="form-label">Email</label>
            <input type="email" class="form-control" disabled value="john@example.com">
        </div>
        
        <div class="mb-3">
            <label class="form-label required">Password</label>
            <input type="password" class="form-control" name="password" required>
            <small class="form-hint">
                Minimum 12 characters, must include:
                <ul class="mb-0">
                    <li>Uppercase letter (A-Z)</li>
                    <li>Lowercase letter (a-z)</li>
                    <li>Number (0-9)</li>
                    <li>Special character (!@#$%^&*)</li>
                </ul>
            </small>
        </div>
        
        <div class="mb-3">
            <label class="form-label required">Confirm Password</label>
            <input type="password" class="form-control" name="password_confirm" required>
        </div>
        
        <div class="form-footer">
            <button type="submit" class="btn btn-primary w-100">
                Accept Invitation & Set Password
            </button>
        </div>
    </div>
</form>
```

**Password Requirements:**
- Minimum 12 characters
- At least one uppercase letter
- At least one lowercase letter
- At least one number
- At least one special character (!@#$%^&*)
- Passwords must match
- Avoid common passwords (checked against list)

**Step 3: Confirmation**

Upon successful password set:
1. User state changes from `pending_invitation` to `active`
2. Invitation token is marked as used
3. User is redirected to login page
4. Success message: "Password set successfully. Please log in."

### First Login After Acceptance

**Step 1: Login**
- Navigate to `/admin/login`
- Enter username and password
- Click **Sign In**

**Step 2: Welcome Screen (First-Time Users)**

```html
<div class="modal modal-blur" id="welcome-modal">
    <div class="modal-dialog modal-lg modal-dialog-centered">
        <div class="modal-content">
            <div class="modal-header">
                <h5 class="modal-title">Welcome to SEL Admin!</h5>
            </div>
            <div class="modal-body">
                <p>You're logged in as <strong>[Username]</strong> with <strong>[Role]</strong> access.</p>
                
                <h6>Quick Start:</h6>
                <ul>
                    <li><strong>Dashboard:</strong> View system statistics and recent activity</li>
                    <li><strong>Events:</strong> Manage events, duplicates, and occurrences</li>
                    <li><strong>API Keys:</strong> [Admin only] Generate keys for agents</li>
                    <li><strong>Federation:</strong> [Admin only] Manage trusted nodes</li>
                    <li><strong>Users:</strong> [Admin only] Manage admin accounts</li>
                </ul>
                
                <h6>Keyboard Shortcuts:</h6>
                <ul>
                    <li><kbd>Ctrl+K</kbd> - Quick search</li>
                    <li><kbd>Ctrl+N</kbd> - Create new (context-aware)</li>
                    <li><kbd>Ctrl+S</kbd> - Save current form</li>
                </ul>
            </div>
            <div class="modal-footer">
                <button type="button" class="btn btn-primary" data-bs-dismiss="modal">
                    Get Started
                </button>
            </div>
        </div>
    </div>
</div>
```

### Expired Invitation Handling

If the invitation token is expired:

```html
<div class="empty">
    <div class="empty-icon">
        <svg class="icon text-warning icon-lg">...</svg>
    </div>
    <p class="empty-title">Invitation Expired</p>
    <p class="empty-subtitle text-muted">
        This invitation link has expired. Please contact your administrator to resend the invitation.
    </p>
</div>
```

Admin must resend the invitation (see [Resending Invitations](#resending-invitations)).

---

## Viewing User Activity

**Status:** This feature is planned but not yet fully implemented.

The audit log will track:
- User login history (successful and failed attempts)
- Password changes
- Role changes
- Account activation/deactivation
- Administrative actions performed by the user

### Activity Log UI

**Activity Log Table HTML:**

```html
<div class="card">
    <div class="card-header">
        <h3 class="card-title">Activity Log</h3>
        <div class="card-actions">
            <button class="btn btn-sm" onclick="exportActivity()">
                Export CSV
            </button>
        </div>
    </div>
    <div class="table-responsive">
        <table class="table table-vcenter card-table">
            <thead>
                <tr>
                    <th>Timestamp</th>
                    <th>User</th>
                    <th>Event Type</th>
                    <th>Resource</th>
                    <th>IP Address</th>
                    <th>Details</th>
                </tr>
            </thead>
            <tbody id="activity-table">
                <!-- Populated via JS -->
            </tbody>
        </table>
    </div>
</div>
```

### Activity Event Types

| Event Type | Description | Example |
|------------|-------------|---------|
| `user.login` | Successful login | User logged in from 192.168.1.1 |
| `user.logout` | User-initiated logout | User logged out |
| `user.login_failed` | Failed login attempt | Invalid password attempt |
| `event.created` | Event created | Created event "Jazz Festival" |
| `event.updated` | Event modified | Updated event "Jazz Festival" |
| `event.deleted` | Event deleted | Deleted event "Jazz Festival" |
| `event.merged` | Events merged | Merged event A into event B |
| `apikey.created` | API key generated | Created API key "bot-scraper" |
| `apikey.revoked` | API key revoked | Revoked API key "bot-scraper" |
| `user.created` | User account created | Created user "john_doe" |
| `user.updated` | User account modified | Updated user "john_doe" role to editor |
| `user.deactivated` | User deactivated | Deactivated user "john_doe" |
| `user.deleted` | User deleted | Deleted user "john_doe" |

**Event Type Badge Color Map:**

```javascript
function getEventTypeColor(eventType) {
    const colors = {
        'user.login': 'success',
        'user.logout': 'info',
        'user.login_failed': 'danger',
        'event.created': 'success',
        'event.updated': 'info',
        'event.deleted': 'danger',
        'event.merged': 'warning',
        'apikey.created': 'success',
        'apikey.revoked': 'danger',
        'user.created': 'success',
        'user.updated': 'info',
        'user.deactivated': 'warning',
        'user.deleted': 'danger'
    };
    return colors[eventType] || 'secondary';
}
```

### Filtering Activity

Filter activity logs by date range, event type, and user:

```html
<!-- Date Range -->
<div class="row g-2">
    <div class="col-auto">
        <input type="date" class="form-control" id="start-date" placeholder="Start date">
    </div>
    <div class="col-auto">
        <input type="date" class="form-control" id="end-date" placeholder="End date">
    </div>
</div>

<!-- Event Type -->
<select class="form-select" id="event-type-filter">
    <option value="">All Event Types</option>
    <option value="user.login">Logins</option>
    <option value="user.login_failed">Failed Logins</option>
    <option value="event.created">Event Created</option>
    <option value="event.updated">Event Updated</option>
    <option value="event.deleted">Event Deleted</option>
</select>

<!-- User (admin view only) -->
<select class="form-select" id="user-filter">
    <option value="">All Users</option>
    <!-- Populated from user list -->
</select>
```

### Understanding Audit Trail Entries

Each activity log entry contains:

- **Timestamp:** Exact time of action (ISO 8601 format)
- **User ID:** ULID of user who performed action
- **Username:** Display name of user
- **Event Type:** Action category (see table above)
- **Resource Type:** Type of resource affected (event, user, apikey)
- **Resource ID:** ULID of affected resource
- **IP Address:** Source IP of request
- **User Agent:** Browser/client information
- **Details:** JSON payload with additional context

**Example Activity Entry:**
```json
{
    "id": "01HQXYZ123...",
    "timestamp": "2026-02-05T14:30:00Z",
    "user_id": "01HQABC456...",
    "username": "admin_user",
    "event_type": "event.updated",
    "resource_type": "event",
    "resource_id": "01HQDEF789...",
    "ip_address": "192.168.1.100",
    "user_agent": "Mozilla/5.0...",
    "details": {
        "changed_fields": ["name", "start_date"],
        "old_values": {"name": "Jazz Fest"},
        "new_values": {"name": "Toronto Jazz Festival"}
    }
}
```

### Exporting Activity Data

Export activity logs to CSV for external analysis (`Ctrl+E` to export current filtered view):

1. Apply desired filters (date range, event type, user)
2. Click **Export CSV** button
3. File downloads as `activity_log_YYYYMMDD.csv`

**CSV Format:**
```csv
Timestamp,User,Event Type,Resource,IP Address,Details
2026-02-05T14:30:00Z,admin_user,event.updated,event:01HQ...,192.168.1.100,"Changed: name, start_date"
```

### Activity API Endpoint

```bash
GET /api/v1/admin/activity?user_id=01HQABC...&event_type=user.login&start_date=2026-02-01&limit=50
```

**Current Behavior:** Returns empty result set with message "Audit log storage not yet implemented"

---

## Troubleshooting

### User Didn't Receive Invitation Email

**Possible Causes:**
1. Email service is disabled (`EMAIL_ENABLED=false` in `.env`)
2. SMTP credentials are incorrect
3. Email went to spam/junk folder
4. Email address is incorrect

**Solutions:**
1. **Check email configuration** - See [Email Setup Guide](email-setup.md)
2. **Check server logs** for email sending errors:
   ```bash
   grep "invitation email" /path/to/server.log
   ```
3. **Verify EMAIL_ENABLED=true** in `.env`
4. **Test email delivery** with a known good address
5. **Resend invitation** from admin UI

Also check: `grep "invitation" /var/log/sel-backend/email.log`

### Invitation Link Expired

**Problem:** User clicks invitation link and gets "Invalid or expired invitation token" error

**Cause:** Invitation tokens expire after **7 days**

**Solution:** Admin must **resend the invitation**:
1. Go to Users page
2. Find the inactive user
3. Click **Resend Invitation**
4. User will receive a new email with a fresh 7-day token

### Invitation Link Shows "Invalid Token"

- Token may be expired (7-day limit)
- Token may have already been used
- Admin should resend invitation

### User Forgot Password

**Status:** Password reset feature is planned but not yet implemented.

**Temporary Workaround:**
1. Admin deactivates the user account
2. Admin deletes the user account (soft delete)
3. Admin creates a new user account with the same email
4. User receives a new invitation email and can set a new password

**Note:** This workaround will result in loss of the user's activity history. A proper password reset feature will be added in a future update.

### Cannot Set Password (Validation Errors)

- Ensure password meets all requirements:
  - Minimum 12 characters
  - Uppercase, lowercase, number, special character
- Passwords must match
- Avoid common passwords (checked against list)

### User Still Can't Log In After Accepting Invitation

- Verify user state is `active` (not `pending_invitation` or `inactive`)
- Clear browser cookies and try again
- Check authentication logs: `grep "login.*username" /var/log/sel-backend/auth.log`

### Cannot Create User - Email Already Taken

**Problem:** Error message "Email is already taken" when creating a user

**Cause:** Another user (active, inactive, or deleted) already has this email address

**Solutions:**
1. **Check if user already exists** - Search for the email in the user list
2. **Check deleted users** - Email may be in use by a soft-deleted account (requires database query)
3. **Use a different email address** if the user has multiple emails

### Cannot Update User - Username Already Taken

**Problem:** Error message "Username is already taken" when updating a user

**Cause:** Another user already has this username

**Solution:**
1. Check the user list for existing usernames
2. Choose a different username
3. Use naming conventions like `firstname.lastname` or `firstname_lastname`

### SMTP Authentication Failed

**Problem:** Server logs show "SMTP authentication failed" when sending invitation emails

**Cause:** Using regular Gmail password instead of App Password

**Solution:** See [Email Setup Guide](email-setup.md) for detailed Gmail SMTP configuration

### Email Sending Connection Refused

**Problem:** Server logs show "connection refused" or "dial tcp: i/o timeout"

**Possible Causes:**
1. SMTP port is blocked by firewall
2. Wrong SMTP host or port
3. Server cannot reach internet

**Solutions:**
1. **Verify SMTP settings** in `.env`:
   - `SMTP_HOST=smtp.gmail.com`
   - `SMTP_PORT=587`
2. **Check firewall rules** - Ensure port 587 outbound is allowed
3. **Test connectivity**:
   ```bash
   telnet smtp.gmail.com 587
   ```
4. **Check server internet access**

### Deleted User Still Appears in Activity Logs

This is expected behavior (audit trail preservation). Soft-deleted users show as `[Deleted User]` in logs. Original user ID is preserved in logs.

### Admin Actions Log Location

All admin actions are logged to:
```
/var/log/sel-backend/admin-audit.log
```

**Example Log Entry:**
```
2026-02-05T14:30:00Z [INFO] user_id=01HQABC... action=user.created target_user_id=01HQDEF... target_username=john_doe role=editor ip=192.168.1.100
```

### Database Queries for Troubleshooting

**Check user status:**
```sql
SELECT id, username, email, role, status, created_at, deleted_at
FROM users
WHERE username = 'john_doe';
```

**Check pending invitations:**
```sql
SELECT u.username, u.email, i.token, i.expires_at, i.used_at
FROM users u
JOIN invitations i ON u.id = i.user_id
WHERE u.status = 'pending_invitation'
AND i.expires_at > NOW();
```

**Check recent activity for user:**
```sql
SELECT timestamp, event_type, resource_type, resource_id, ip_address
FROM activity_logs
WHERE user_id = '01HQABC...'
ORDER BY timestamp DESC
LIMIT 50;
```

---

## User Management API Reference

For developers building integrations. See full documentation in `../api/admin-users.md`.

**Create User:**
```bash
POST /api/v1/admin/users
Content-Type: application/json

{
    "username": "john_doe",
    "email": "john@example.com",
    "role": "editor"
}
```

**List Users:**
```bash
GET /api/v1/admin/users?status=active&role=editor&limit=50&after=cursor
```

**Update User:**
```bash
PUT /api/v1/admin/users/01HQABC...
Content-Type: application/json

{
    "username": "john_doe",
    "email": "john@example.com",
    "role": "admin"
}
```

**Deactivate User:**
```bash
POST /api/v1/admin/users/01HQABC.../deactivate
```

**Delete User:**
```bash
DELETE /api/v1/admin/users/01HQABC...
```

**Resend Invitation:**
```bash
POST /api/v1/admin/users/01HQABC.../resend-invitation
```

**Get Activity Logs:**
```bash
GET /api/v1/admin/activity?user_id=01HQABC...&event_type=user.login&start_date=2026-02-01&limit=50
```

---

## Best Practices

### Security

- **Use strong admin passwords** - Minimum 12 characters with uppercase, lowercase, numbers, and special characters
- **Limit admin accounts** - Only create admin accounts for trusted system administrators
- **Review user list regularly** - Deactivate accounts that are no longer needed
- **Monitor audit logs** - Check for suspicious activity (planned feature)

### Email Configuration

- **Use a dedicated email address** for sending invitations (e.g., `noreply@yourdomain.com`)
- **Set up SPF, DKIM, and DMARC** records to prevent emails from going to spam
- **Test email delivery** before deploying to production
- **Use App Passwords** for Gmail SMTP (never use your regular password)

### User Management

- **Assign the minimum necessary role** - Start with `viewer` and upgrade to `editor` or `admin` as needed
- **Use descriptive usernames** - Make it easy to identify users (e.g., `jane.doe` instead of `user123`)
- **Document role assignments** - Keep a record of why users were given admin or editor roles
- **Deactivate instead of delete** when possible - Preserves data and allows reactivation

### SEL Community

This is civic infrastructure. Good user management practices help:
- **Reduce support burden** - Clear invitations prevent confusion
- **Enable collaboration** - Appropriate roles let community members contribute
- **Build trust** - Transparent audit trails demonstrate good stewardship
- **Ease adoption** - Simple onboarding gets nodes operational quickly

---

## Related Documentation

- [Email Setup Guide](email-setup.md) - Configure SMTP for sending invitation emails
- [Admin Users API Reference](../api/admin-users.md) - Complete API endpoint documentation
- [SEL Interoperability Profile](../togather_SEL_Interoperability_Profile_v0.1.md) - SEL specification and requirements

---

## Support

For issues not covered in this guide:

- **Check server logs** - Most issues are logged with helpful error messages
- **Review Email Setup Guide** - Most problems are email configuration related
- **Check database connectivity** - User operations require database access
- **Report bugs** - File issues at the Togather project repository

---

**Last Updated:** February 5, 2026  
**Version:** 1.0
