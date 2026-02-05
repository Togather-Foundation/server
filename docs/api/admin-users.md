# Admin User Management API

This document describes the RESTful API endpoints for managing users in the Togather SEL Backend. These endpoints are used by administrators to create, manage, and monitor user accounts.

## Table of Contents

- [Authentication](#authentication)
- [Error Responses](#error-responses)
- [Endpoints](#endpoints)
  - [Create User](#1-create-user-and-send-invitation)
  - [List Users](#2-list-users)
  - [Get User](#3-get-single-user)
  - [Update User](#4-update-user)
  - [Delete User](#5-delete-user-soft-delete)
  - [Deactivate User](#6-deactivate-user)
  - [Activate User](#7-activate-user)
  - [Resend Invitation](#8-resend-invitation)
  - [Get User Activity](#9-get-user-activity-audit-log)
  - [Accept Invitation](#10-accept-invitation-public-endpoint)

---

## Authentication

All admin endpoints (except **Accept Invitation**) require JWT authentication with admin privileges.

**Authentication Methods:**

1. **JWT Token in Authorization Header** (for API clients):
   ```
   Authorization: Bearer <jwt-token>
   ```

2. **Session Cookie** (for browser-based admin UI):
   ```
   Cookie: auth_token=<jwt-token>
   ```

**Required Permissions:**
- All endpoints require the `admin` role (enforced by `JWTAuth` middleware)
- Attempting to access these endpoints without authentication returns `401 Unauthorized`
- Attempting to access with insufficient permissions returns `403 Forbidden`

---

## Error Responses

All endpoints use [RFC 7807](https://tools.ietf.org/html/rfc7807) Problem Details for HTTP APIs.

**Error Response Structure:**
```json
{
  "type": "https://sel.events/problems/validation-error",
  "title": "Invalid Request",
  "status": 400,
  "detail": "Email is required",
  "instance": "/api/v1/admin/users"
}
```

**Common Error Types:**

| HTTP Status | Problem Type | Description |
|-------------|--------------|-------------|
| `400` | `validation-error` | Invalid request parameters or body |
| `401` | `unauthorized` | Missing or invalid authentication |
| `403` | `forbidden` | Insufficient permissions |
| `404` | `not-found` | User not found |
| `409` | `conflict` | Email or username already taken |
| `500` | `server-error` | Internal server error |

---

## Endpoints

### 1. Create User and Send Invitation

Creates a new user account and sends an invitation email with a secure token for password setup.

**Endpoint:** `POST /api/v1/admin/users`

**Authentication:** Required (Admin)

**Rate Limit:** Admin tier (100 req/min per user)

#### Request Body

```json
{
  "username": "johndoe",
  "email": "john@example.com",
  "role": "editor"
}
```

**Parameters:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `username` | string | Yes | Unique username (alphanumeric, underscore, hyphen) |
| `email` | string | Yes | Valid email address (unique) |
| `role` | string | No | User role: `admin`, `editor`, or `viewer`. Defaults to `viewer` |

#### Response

**Status:** `201 Created`

```json
{
  "id": "01HQXYZ123456789ABCDEFGHJ",
  "username": "johndoe",
  "email": "john@example.com",
  "role": "editor",
  "is_active": false,
  "created_at": "2024-02-05T12:00:00Z",
  "last_login_at": null
}
```

**Response Fields:**

| Field | Type | Description |
|-------|------|-------------|
| `id` | string | User's unique UUID |
| `username` | string | Username |
| `email` | string | Email address |
| `role` | string | User role (`admin`, `editor`, `viewer`) |
| `is_active` | boolean | User activation status (false until invitation accepted) |
| `created_at` | string | ISO 8601 timestamp of user creation |
| `last_login_at` | string\|null | ISO 8601 timestamp of last login (null for new users) |

#### Error Responses

- **400 Bad Request**: Invalid request body or missing required fields
  ```json
  {
    "type": "https://sel.events/problems/validation-error",
    "title": "Invalid Request",
    "status": 400,
    "detail": "Email is required"
  }
  ```

- **409 Conflict**: Email or username already taken
  ```json
  {
    "type": "https://sel.events/problems/conflict",
    "title": "Email already taken",
    "status": 409
  }
  ```

#### Example

```bash
curl -X POST https://api.sel.events/api/v1/admin/users \
  -H "Authorization: Bearer <jwt-token>" \
  -H "Content-Type: application/json" \
  -d '{
    "username": "johndoe",
    "email": "john@example.com",
    "role": "editor"
  }'
```

**Behavior:**
- Creates user in **inactive state** (`is_active = false`)
- Generates secure invitation token (32-byte random, URL-safe)
- Sends invitation email with link: `https://yourdomain.com/accept-invitation?token=<token>`
- Invitation expires in 7 days (configurable)
- Logs audit event: `user.created`

---

### 2. List Users

Retrieves a paginated list of all users with optional filtering.

**Endpoint:** `GET /api/v1/admin/users`

**Authentication:** Required (Admin)

**Rate Limit:** Admin tier (100 req/min per user)

#### Query Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `status` | string | No | Filter by activation status: `active` or `inactive` |
| `role` | string | No | Filter by role: `admin`, `editor`, or `viewer` |
| `limit` | integer | No | Number of results per page (1-100, default: 50) |
| `offset` | integer | No | Pagination offset (default: 0) |

#### Response

**Status:** `200 OK`

```json
{
  "items": [
    {
      "id": "01HQXYZ123456789ABCDEFGHJ",
      "username": "johndoe",
      "email": "john@example.com",
      "role": "editor",
      "is_active": true,
      "created_at": "2024-02-05T12:00:00Z",
      "last_login_at": "2024-02-06T08:30:00Z"
    },
    {
      "id": "01HQXYZ987654321ZYXWVUTSR",
      "username": "janedoe",
      "email": "jane@example.com",
      "role": "viewer",
      "is_active": false,
      "created_at": "2024-02-07T10:00:00Z",
      "last_login_at": null
    }
  ],
  "next_cursor": "100",
  "total": 150
}
```

**Response Fields:**

| Field | Type | Description |
|-------|------|-------------|
| `items` | array | Array of user objects (see [Create User response](#response)) |
| `next_cursor` | string\|null | Cursor for next page (offset value), `null` if no more results |
| `total` | integer | Total number of users matching filters |

#### Examples

**List all users:**
```bash
curl -X GET https://api.sel.events/api/v1/admin/users \
  -H "Authorization: Bearer <jwt-token>"
```

**List only active admins:**
```bash
curl -X GET "https://api.sel.events/api/v1/admin/users?status=active&role=admin" \
  -H "Authorization: Bearer <jwt-token>"
```

**Paginate results:**
```bash
curl -X GET "https://api.sel.events/api/v1/admin/users?limit=25&offset=0" \
  -H "Authorization: Bearer <jwt-token>"
```

---

### 3. Get Single User

Retrieves detailed information about a specific user.

**Endpoint:** `GET /api/v1/admin/users/{id}`

**Authentication:** Required (Admin)

**Rate Limit:** Admin tier (100 req/min per user)

#### Path Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | string | User's UUID |

#### Response

**Status:** `200 OK`

```json
{
  "id": "01HQXYZ123456789ABCDEFGHJ",
  "username": "johndoe",
  "email": "john@example.com",
  "role": "editor",
  "is_active": true,
  "created_at": "2024-02-05T12:00:00Z",
  "last_login_at": "2024-02-06T08:30:00Z"
}
```

#### Error Responses

- **400 Bad Request**: Invalid UUID format
- **404 Not Found**: User does not exist

#### Example

```bash
curl -X GET https://api.sel.events/api/v1/admin/users/01HQXYZ123456789ABCDEFGHJ \
  -H "Authorization: Bearer <jwt-token>"
```

---

### 4. Update User

Updates user details (username, email, or role).

**Endpoint:** `PUT /api/v1/admin/users/{id}`

**Authentication:** Required (Admin)

**Rate Limit:** Admin tier (100 req/min per user)

#### Path Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | string | User's UUID |

#### Request Body

All fields are optional. Only provided fields will be updated.

```json
{
  "username": "johndoe_updated",
  "email": "newemail@example.com",
  "role": "admin"
}
```

**Parameters:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `username` | string | No | New username (must be unique) |
| `email` | string | No | New email address (must be unique) |
| `role` | string | No | New role: `admin`, `editor`, or `viewer` |

#### Response

**Status:** `200 OK`

Returns the updated user object (same structure as [Get User response](#response-2)).

```json
{
  "id": "01HQXYZ123456789ABCDEFGHJ",
  "username": "johndoe_updated",
  "email": "newemail@example.com",
  "role": "admin",
  "is_active": true,
  "created_at": "2024-02-05T12:00:00Z",
  "last_login_at": "2024-02-06T08:30:00Z"
}
```

#### Error Responses

- **400 Bad Request**: Invalid request body or UUID
- **404 Not Found**: User does not exist
- **409 Conflict**: New email or username already taken

#### Example

```bash
curl -X PUT https://api.sel.events/api/v1/admin/users/01HQXYZ123456789ABCDEFGHJ \
  -H "Authorization: Bearer <jwt-token>" \
  -H "Content-Type: application/json" \
  -d '{
    "role": "admin"
  }'
```

**Behavior:**
- Partial updates supported (only send fields to update)
- Empty or whitespace-only values are ignored
- Logs audit event: `user.updated`

---

### 5. Delete User (Soft Delete)

Soft deletes a user account by setting `deleted_at` timestamp.

**Endpoint:** `DELETE /api/v1/admin/users/{id}`

**Authentication:** Required (Admin)

**Rate Limit:** Admin tier (100 req/min per user)

#### Path Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | string | User's UUID |

#### Response

**Status:** `204 No Content`

No response body.

#### Error Responses

- **400 Bad Request**: Invalid UUID format
- **404 Not Found**: User does not exist

#### Example

```bash
curl -X DELETE https://api.sel.events/api/v1/admin/users/01HQXYZ123456789ABCDEFGHJ \
  -H "Authorization: Bearer <jwt-token>"
```

**Behavior:**
- **Soft delete**: Sets `deleted_at` timestamp (user data retained for audit purposes)
- User cannot log in after deletion
- Deleted users excluded from list endpoints by default
- Logs audit event: `user.deleted`

---

### 6. Deactivate User

Deactivates a user account, preventing login without deleting the account.

**Endpoint:** `POST /api/v1/admin/users/{id}/deactivate`

**Authentication:** Required (Admin)

**Rate Limit:** Admin tier (100 req/min per user)

#### Path Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | string | User's UUID |

#### Response

**Status:** `200 OK`

Returns the updated user object with `is_active = false`.

```json
{
  "id": "01HQXYZ123456789ABCDEFGHJ",
  "username": "johndoe",
  "email": "john@example.com",
  "role": "editor",
  "is_active": false,
  "created_at": "2024-02-05T12:00:00Z",
  "last_login_at": "2024-02-06T08:30:00Z"
}
```

#### Error Responses

- **400 Bad Request**: Invalid UUID format
- **404 Not Found**: User does not exist

#### Example

```bash
curl -X POST https://api.sel.events/api/v1/admin/users/01HQXYZ123456789ABCDEFGHJ/deactivate \
  -H "Authorization: Bearer <jwt-token>"
```

**Behavior:**
- Sets `is_active = false`
- User cannot log in while deactivated
- Can be reactivated using [Activate User](#7-activate-user)
- Logs audit event: `user.deactivated`

---

### 7. Activate User

Reactivates a previously deactivated user account.

**Endpoint:** `POST /api/v1/admin/users/{id}/activate`

**Authentication:** Required (Admin)

**Rate Limit:** Admin tier (100 req/min per user)

#### Path Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | string | User's UUID |

#### Response

**Status:** `200 OK`

Returns the updated user object with `is_active = true`.

```json
{
  "id": "01HQXYZ123456789ABCDEFGHJ",
  "username": "johndoe",
  "email": "john@example.com",
  "role": "editor",
  "is_active": true,
  "created_at": "2024-02-05T12:00:00Z",
  "last_login_at": "2024-02-06T08:30:00Z"
}
```

#### Error Responses

- **400 Bad Request**: Invalid UUID format
- **404 Not Found**: User does not exist

#### Example

```bash
curl -X POST https://api.sel.events/api/v1/admin/users/01HQXYZ123456789ABCDEFGHJ/activate \
  -H "Authorization: Bearer <jwt-token>"
```

**Behavior:**
- Sets `is_active = true`
- User can log in after activation
- Logs audit event: `user.activated`

---

### 8. Resend Invitation

Resends the invitation email to a user who has not yet accepted their invitation.

**Endpoint:** `POST /api/v1/admin/users/{id}/resend-invitation`

**Authentication:** Required (Admin)

**Rate Limit:** Admin tier (100 req/min per user)

#### Path Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | string | User's UUID |

#### Response

**Status:** `200 OK`

```json
{
  "message": "Invitation email has been resent successfully"
}
```

#### Error Responses

- **400 Bad Request**: User is already active (invitation already accepted)
  ```json
  {
    "type": "https://sel.events/problems/validation-error",
    "title": "User is already active",
    "status": 400
  }
  ```
- **404 Not Found**: User does not exist

#### Example

```bash
curl -X POST https://api.sel.events/api/v1/admin/users/01HQXYZ123456789ABCDEFGHJ/resend-invitation \
  -H "Authorization: Bearer <jwt-token>"
```

**Behavior:**
- Only works for inactive users who haven't accepted invitation
- Generates new invitation token (previous token invalidated)
- New token expires in 7 days from resend time
- Sends new invitation email
- Logs audit event: `user.invitation_resent`

---

### 9. Get User Activity Audit Log

Retrieves audit log of user actions and administrative changes.

**Endpoint:** `GET /api/v1/admin/users/{id}/activity`

**Authentication:** Required (Admin)

**Rate Limit:** Admin tier (100 req/min per user)

#### Path Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | string | User's UUID |

#### Query Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `event_type` | string | No | Filter by event type (e.g., `user.login`, `user.updated`) |
| `limit` | integer | No | Number of results per page (1-100, default: 50) |
| `cursor` | string | No | Pagination cursor from previous response |

#### Response

**Status:** `200 OK`

```json
{
  "items": [],
  "next_cursor": null,
  "message": "Audit log storage not yet implemented"
}
```

**Note:** Audit log querying is planned but not yet fully implemented. The endpoint currently returns an empty result set.

#### Error Responses

- **400 Bad Request**: Invalid UUID format
- **404 Not Found**: User does not exist

#### Example

```bash
curl -X GET https://api.sel.events/api/v1/admin/users/01HQXYZ123456789ABCDEFGHJ/activity \
  -H "Authorization: Bearer <jwt-token>"
```

**Planned Features:**
- User login history
- Password changes
- Role changes
- Activation/deactivation events
- Administrative actions performed by the user

---

### 10. Accept Invitation (Public Endpoint)

Allows a new user to accept their invitation by setting a password.

**Endpoint:** `POST /api/v1/accept-invitation`

**Authentication:** None (Public endpoint)

**Rate Limit:** Public tier (10 req/min per IP)

#### Request Body

```json
{
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "password": "SecurePassword123!"
}
```

**Parameters:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `token` | string | Yes | Invitation token from email link |
| `password` | string | Yes | New password (min 8 chars, must contain uppercase, lowercase, number) |

#### Response

**Status:** `200 OK`

```json
{
  "message": "Invitation accepted successfully. You can now log in.",
  "user": {
    "id": "01HQXYZ123456789ABCDEFGHJ",
    "username": "johndoe",
    "email": "john@example.com",
    "role": "editor"
  }
}
```

#### Error Responses

- **400 Bad Request**: Invalid or expired token
  ```json
  {
    "type": "https://sel.events/problems/invalid-invitation",
    "title": "Invalid or Expired Invitation",
    "status": 400,
    "detail": "The invitation token is invalid or has expired."
  }
  ```

- **400 Bad Request**: Weak password
  ```json
  {
    "type": "https://sel.events/problems/weak-password",
    "title": "Password Does Not Meet Requirements",
    "status": 400,
    "detail": "Password must be at least 8 characters long and contain uppercase, lowercase, and numbers"
  }
  ```

#### Example

```bash
curl -X POST https://api.sel.events/api/v1/accept-invitation \
  -H "Content-Type: application/json" \
  -d '{
    "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
    "password": "SecurePassword123!"
  }'
```

**Behavior:**
- Validates invitation token (must not be expired or already used)
- Validates password strength
- Sets user's password (bcrypt hashed)
- Activates user account (`is_active = true`)
- Marks invitation as accepted (`accepted_at` timestamp)
- Logs audit event: `user.invitation_accepted`

**Password Requirements:**
- Minimum 8 characters
- Maximum 72 characters (bcrypt limitation)
- Must contain at least one uppercase letter
- Must contain at least one lowercase letter
- Must contain at least one number

---

## Implementation Notes

### Rate Limiting

All endpoints are rate-limited by tier:

| Tier | Rate Limit | Applied To |
|------|------------|------------|
| Public | 10 req/min per IP | Public invitation acceptance |
| Admin | 100 req/min per user | Admin user management operations |
| Login | 5 req/min per IP | Admin login (not covered in this doc) |

Rate limit headers are included in responses:
```
X-RateLimit-Limit: 100
X-RateLimit-Remaining: 95
X-RateLimit-Reset: 1612345678
```

### Audit Logging

All administrative actions are logged with:
- Action type (e.g., `user.created`, `user.updated`)
- Admin user who performed the action
- Target user ID
- Timestamp
- Client IP address
- Additional metadata (changed fields, etc.)

Audit logs are written to:
1. Structured application logs (JSON format)
2. Database audit table (planned feature)

### Security Considerations

1. **Invitation Tokens**:
   - 32-byte cryptographically secure random strings
   - URL-safe base64 encoded
   - One-time use (invalidated after acceptance)
   - Expire after 7 days

2. **Password Security**:
   - Bcrypt hashing with cost factor 10
   - Passwords never logged or returned in API responses
   - Password strength validation enforced

3. **CSRF Protection**:
   - All state-changing endpoints use CSRF tokens when accessed via browser cookies
   - API key authentication bypasses CSRF (stateless)

4. **Input Validation**:
   - Email format validation
   - Username format validation (alphanumeric, underscore, hyphen)
   - Role validation (must be `admin`, `editor`, or `viewer`)
   - UUID format validation for user IDs

### Content Negotiation

All endpoints support JSON format only:
- Request: `Content-Type: application/json`
- Response: `Content-Type: application/json`

### Idempotency

- `POST` operations (Create, Activate, Deactivate) are **not idempotent**
- `PUT` operations (Update) are **idempotent** (same request produces same result)
- `DELETE` operations are **idempotent** (deleting already-deleted user returns 404)

---

## Related Documentation

- [User Administration UI Guide](/docs/admin-ui-guide.md)
- [User Administration Plan](/docs/admin/user-administration-plan.md)
- [SEL Interoperability Profile](/docs/togather_SEL_Interoperability_Profile_v0.1.md)
- [Authentication & Authorization Design](/docs/togather_SEL_server_architecture_design_v1.md#authentication--authorization)

---

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2024-02-05 | Initial API documentation |
