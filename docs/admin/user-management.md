# User Management Guide

**Audience:** System administrators managing user accounts in a Togather SEL node

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
- [Viewing User Activity](#viewing-user-activity)
- [Troubleshooting](#troubleshooting)

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

---

## User States

Users can be in one of these states:

| State | Description | Can Login? | Actions Available |
|-------|-------------|------------|-------------------|
| **Inactive (Pending Invitation)** | User created but hasn't accepted invitation yet | ❌ No | Admin can resend invitation |
| **Active** | User has accepted invitation and set password | ✅ Yes | Admin can deactivate or delete |
| **Inactive (Deactivated)** | Admin has deactivated the account | ❌ No | Admin can reactivate |
| **Deleted** | Account soft-deleted (retained for audit) | ❌ No | Data remains in database, cannot be restored via UI |

---

## Creating Users

### Prerequisites

1. **Email configuration must be set up** - See [Email Setup Guide](email-setup.md)
2. **You must be logged in as an admin**
3. **User's email address must be valid and unique**

### Steps (Admin UI)

1. Navigate to **Users** in the admin navigation menu
2. Click **Create User** button
3. Fill in the user creation form:
   - **Username**: 3-50 alphanumeric characters (unique)
   - **Email**: Valid email address (unique)
   - **Role**: Select admin, editor, or viewer
4. Click **Create & Send Invitation**

### What Happens Next

1. User record is created in **inactive state** (no password yet)
2. Invitation email is sent to the user's email address
3. Email contains a secure invitation link (valid for 7 days)
4. User must click the link and set a password to activate their account

### User Experience: Accepting the Invitation

The invited user receives an email with subject "You've been invited to SEL Admin". The email contains a link to `/admin/accept-invitation?token=...` where they can set their password (minimum 12 characters, must include uppercase, lowercase, number, and special character). Once they submit the form, their account becomes active and they can log in.

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
2. Click **Resend Invitation** button
3. A new invitation email will be sent with a fresh 7-day token

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

## Viewing User Activity

**Status:** This feature is planned but not yet fully implemented.

The audit log will track:
- User login history (successful and failed attempts)
- Password changes
- Role changes
- Account activation/deactivation
- Administrative actions performed by the user

**API Endpoint (coming soon):**

```bash
curl -X GET https://your-domain.com/api/v1/admin/users/{user_id}/activity \
  -H "Authorization: Bearer YOUR_JWT_TOKEN"
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

### Invitation Link Expired

**Problem:** User clicks invitation link and gets "Invalid or expired invitation token" error

**Cause:** Invitation tokens expire after **7 days**

**Solution:** Admin must **resend the invitation**:
1. Go to Users page
2. Find the inactive user
3. Click **Resend Invitation**
4. User will receive a new email with a fresh 7-day token

### User Forgot Password

**Status:** Password reset feature is planned but not yet implemented.

**Temporary Workaround:**
1. Admin deactivates the user account
2. Admin deletes the user account (soft delete)
3. Admin creates a new user account with the same email
4. User receives a new invitation email and can set a new password

**Note:** This workaround will result in loss of the user's activity history. A proper password reset feature will be added in a future update.

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
- [User Administration Plan](user-administration-plan.md) - Technical implementation details
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
