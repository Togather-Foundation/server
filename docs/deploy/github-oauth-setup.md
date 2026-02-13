# GitHub OAuth Setup

**Status**: Optional (Phase 2 feature)  
**Purpose**: Enable developers to sign in with GitHub for zero-friction onboarding

This guide covers setting up GitHub OAuth authentication for the developer portal.

---

## Table of Contents

1. [Overview](#overview)
2. [Prerequisites](#prerequisites)
3. [Create GitHub OAuth App](#create-github-oauth-app)
4. [Configure Server](#configure-server)
5. [Test OAuth Flow](#test-oauth-flow)
6. [Organization Restrictions (Optional)](#organization-restrictions-optional)
7. [Environment-Specific Setup](#environment-specific-setup)
8. [Troubleshooting](#troubleshooting)

---

## Overview

GitHub OAuth enables developers to authenticate using their GitHub account instead of email/password. This provides:

- **Zero-friction onboarding**: No need for admins to manually invite developers
- **Account linking**: Existing email/password accounts can be linked to GitHub
- **Single sign-on**: Use GitHub credentials for authentication
- **Optional org restrictions**: Limit access to members of specific GitHub organizations

**Authentication Flow:**

1. Developer clicks "Sign in with GitHub" on `/dev/login`
2. Redirected to GitHub OAuth authorization
3. After approval, redirected back to server callback URL
4. Server exchanges authorization code for access token
5. Server fetches GitHub user profile (ID, email, name)
6. Auto-creation/matching logic:
   - **GitHub ID match**: Log in existing developer
   - **Email match (no GitHub ID)**: Link GitHub account to existing developer
   - **No match**: Create new developer account
7. Developer redirected to `/dev/dashboard` with session cookie

---

## Prerequisites

- **GitHub account** with permissions to create OAuth Apps
- **Public callback URL** (e.g., `https://staging.toronto.togather.foundation/auth/github/callback`)
- **Server environment** with GitHub OAuth variables (see below)

---

## Create GitHub OAuth App

### 1. Navigate to GitHub OAuth Apps

**For personal account:**
- Go to https://github.com/settings/developers
- Click "OAuth Apps" → "New OAuth App"

**For organization:**
- Go to https://github.com/organizations/{org}/settings/applications
- Click "OAuth Apps" → "New OAuth App"

### 2. Register Application

Fill in the form:

| Field | Value | Example |
|-------|-------|---------|
| **Application name** | `Togather SEL - {environment}` | `Togather SEL - Staging` |
| **Homepage URL** | Your server's public URL | `https://staging.toronto.togather.foundation` |
| **Application description** | (Optional) Description for users | `Togather Shared Events Library developer portal` |
| **Authorization callback URL** | `{PUBLIC_URL}/auth/github/callback` | `https://staging.toronto.togather.foundation/auth/github/callback` |

**Important**: Authorization callback URL **must** match exactly, including protocol (`https://`) and no trailing slash.

### 3. Save OAuth Credentials

After creating the app:

1. **Client ID**: Displayed immediately (e.g., `Iv1.a1b2c3d4e5f6g7h8`)
2. **Client Secret**: Click "Generate a new client secret"
   - ⚠️ **Save immediately** - shown only once
   - Store securely (password manager, secret manager)

---

## Configure Server

### 1. Set Environment Variables

Add these variables to your server environment configuration:

**Required:**

```bash
GITHUB_CLIENT_ID=Iv1.a1b2c3d4e5f6g7h8
GITHUB_CLIENT_SECRET=your-github-oauth-client-secret-here
GITHUB_CALLBACK_URL=https://staging.toronto.togather.foundation/auth/github/callback
```

**Optional (org restrictions):**

```bash
GITHUB_ALLOWED_ORGS=your-org,another-org
```

### 2. Deployment-Specific Configuration

**Local Development (`.env`):**

```bash
# .env
GITHUB_CLIENT_ID=Iv1.dev123...
GITHUB_CLIENT_SECRET=dev_secret_abc123...
GITHUB_CALLBACK_URL=http://localhost:8080/auth/github/callback
# GITHUB_ALLOWED_ORGS=  # Leave empty for local testing
```

**Staging (`.env.staging` or environment):**

```bash
GITHUB_CLIENT_ID=Iv1.staging456...
GITHUB_CLIENT_SECRET=staging_secret_xyz789...
GITHUB_CALLBACK_URL=https://staging.toronto.togather.foundation/auth/github/callback
# GITHUB_ALLOWED_ORGS=your-org  # Optional: restrict to org members
```

**Production (`.env.production` or secret manager):**

```bash
GITHUB_CLIENT_ID=Iv1.prod789...
GITHUB_CLIENT_SECRET=prod_secret_secure123...
GITHUB_CALLBACK_URL=https://toronto.togather.foundation/auth/github/callback
GITHUB_ALLOWED_ORGS=togather-foundation  # Recommended for production
```

### 3. Restart Server

After adding environment variables:

```bash
# Systemd
sudo systemctl restart togather

# Docker
docker-compose restart

# Manual
pkill -f togather-server
./server serve
```

### 4. Verify Configuration

Check server logs on startup:

```bash
# Look for GitHub OAuth initialization
journalctl -u togather -f | grep -i github

# Expected log (if configured):
# {"level":"info","service":"togather-sel-server","component":"routes","message":"GitHub OAuth enabled","callback_url":"https://staging.toronto.togather.foundation/auth/github/callback"}

# If not configured (OK for development):
# {"level":"info","service":"togather-sel-server","component":"routes","message":"GitHub OAuth not configured (optional feature)"}
```

---

## Test OAuth Flow

### 1. Access Developer Login

Navigate to: `https://your-domain.com/dev/login`

You should see a "Sign in with GitHub" button.

### 2. Test Authentication

Click "Sign in with GitHub" and verify:

1. **Redirect to GitHub**: URL should be `https://github.com/login/oauth/authorize?client_id=...`
2. **GitHub authorization page**: Shows app name, organization (if applicable), and requested scopes (`user:email`)
3. **Authorize application**: Click "Authorize {app-name}"
4. **Redirect back**: Should return to `https://your-domain.com/auth/github/callback?code=...&state=...`
5. **Auto-login**: Redirected to `/dev/dashboard` with session cookie

### 3. Verify Developer Account

Check developer was created:

```bash
# Via CLI
./server developer list | grep github

# Via API (requires admin auth)
curl https://your-domain.com/api/v1/admin/developers \
     -H "Authorization: Bearer $ADMIN_JWT" | jq '.items[] | select(.github_username)'

# Via logs
journalctl -u togather -f | grep "developer created via github oauth"
```

### 4. Test Account Linking

To test linking an existing account:

1. **Create developer with email/password**:
   ```bash
   ./server developer invite test@example.com --name "Test User"
   ```

2. **Accept invitation and set password**

3. **Log out and click "Sign in with GitHub"**

4. **Authorize with GitHub account using the same email** (`test@example.com`)

5. **Verify account linking**:
   ```bash
   ./server developer list | grep test@example.com
   # Should show github_username and github_id populated
   ```

---

## Organization Restrictions (Optional)

Restrict GitHub OAuth to members of specific GitHub organizations.

### 1. Configure Organization Filter

Set `GITHUB_ALLOWED_ORGS` to comma-separated list of organization slugs:

```bash
GITHUB_ALLOWED_ORGS=togather-foundation,partner-org
```

### 2. How It Works

When a user authenticates:

1. Server fetches GitHub user profile
2. If `GITHUB_ALLOWED_ORGS` is set:
   - Server checks user's organization memberships via GitHub API
   - If user is **not** a member of any allowed org → deny access
   - If user **is** a member → proceed with login/creation

**Note**: Organization membership visibility:
- **Public members**: Always visible
- **Private members**: Visible only if user has granted `read:org` scope (requires updating scope in GitHub OAuth app)

### 3. Scope Requirements

To check private organization memberships, update scope in your OAuth app:

1. In GitHub OAuth app settings
2. Change scope from `user:email` to `user:email read:org`
3. Update `GenerateAuthURL` in `internal/auth/oauth/github.go` (line 54):
   ```go
   // Before:
   "scope": {"user:email"},

   // After (for org restrictions):
   "scope": {"user:email read:org"},
   ```

### 4. Use Cases

**Open access (no restrictions):**
```bash
# Leave empty or unset
GITHUB_ALLOWED_ORGS=
```

**Single organization:**
```bash
GITHUB_ALLOWED_ORGS=togather-foundation
```

**Multiple organizations:**
```bash
GITHUB_ALLOWED_ORGS=togather-foundation,partner-org,community-org
```

---

## Environment-Specific Setup

### Development

**Goal**: Test OAuth flow locally without org restrictions

**Steps:**
1. Create GitHub OAuth app with callback: `http://localhost:8080/auth/github/callback`
2. Add to `.env`:
   ```bash
   GITHUB_CLIENT_ID=Iv1.dev123...
   GITHUB_CLIENT_SECRET=dev_secret_abc123...
   GITHUB_CALLBACK_URL=http://localhost:8080/auth/github/callback
   ```
3. Run server: `make run` or `make dev`
4. Test: `http://localhost:8080/dev/login`

### Staging

**Goal**: Test OAuth with real domain and optional org restrictions

**Steps:**
1. Create GitHub OAuth app with callback: `https://staging.toronto.togather.foundation/auth/github/callback`
2. Add to remote `.env.staging`:
   ```bash
   GITHUB_CLIENT_ID=Iv1.staging456...
   GITHUB_CLIENT_SECRET=staging_secret_xyz789...
   GITHUB_CALLBACK_URL=https://staging.toronto.togather.foundation/auth/github/callback
   GITHUB_ALLOWED_ORGS=your-org  # Optional
   ```
3. Deploy: `./deploy/scripts/deploy.sh staging --version HEAD`
4. Test: `https://staging.toronto.togather.foundation/dev/login`

### Production

**Goal**: Secure OAuth with org restrictions enabled

**Steps:**
1. Create GitHub OAuth app with callback: `https://toronto.togather.foundation/auth/github/callback`
2. Store credentials in secret manager (recommended) or `.env.production`:
   ```bash
   GITHUB_CLIENT_ID=Iv1.prod789...
   GITHUB_CLIENT_SECRET=prod_secret_secure123...
   GITHUB_CALLBACK_URL=https://toronto.togather.foundation/auth/github/callback
   GITHUB_ALLOWED_ORGS=togather-foundation  # Recommended
   ```
3. Deploy: `./deploy/scripts/deploy.sh production --version v1.0.0`
4. Test with authorized GitHub account
5. Monitor logs for unauthorized access attempts

**Production Security Checklist:**
- ✅ Use separate OAuth app (not shared with dev/staging)
- ✅ Enable organization restrictions (`GITHUB_ALLOWED_ORGS`)
- ✅ Store client secret in secret manager (not `.env` file)
- ✅ Use HTTPS for all URLs (required by GitHub)
- ✅ Monitor audit logs for OAuth events
- ✅ Rotate client secret every 6-12 months

---

## Troubleshooting

### "OAuth not configured" Message

**Symptom**: "Sign in with GitHub" button missing on `/dev/login`

**Cause**: GitHub OAuth environment variables not set or incomplete

**Fix**:
1. Verify all required variables are set:
   ```bash
   echo $GITHUB_CLIENT_ID
   echo $GITHUB_CLIENT_SECRET
   echo $GITHUB_CALLBACK_URL
   ```
2. Restart server after adding variables
3. Check logs for GitHub OAuth initialization message

### "Redirect URI mismatch" Error

**Symptom**: GitHub OAuth error page: "The redirect_uri MUST match the registered callback URL for this application."

**Cause**: `GITHUB_CALLBACK_URL` doesn't match the registered callback URL in GitHub OAuth app settings

**Fix**:
1. Check registered callback in GitHub OAuth app settings
2. Ensure exact match (including `https://`, domain, and path)
3. Common mistakes:
   - Missing trailing `/auth/github/callback`
   - Wrong protocol (`http://` vs `https://`)
   - Wrong domain (`localhost` vs production domain)

### "No Email Found" Error

**Symptom**: OAuth flow redirects back with `?error=no_email`

**Cause**: GitHub user has email set to private and hasn't granted email access

**Fix** (user must do this):
1. Go to https://github.com/settings/emails
2. Uncheck "Keep my email addresses private"
3. Or grant `user:email` scope when authorizing

**Alternative** (update scope to fetch private email):
- Scope `user:email` should fetch private emails automatically
- If not working, check GitHub API response in logs

### "Account Conflict" Error

**Symptom**: OAuth flow redirects back with `?error=account_conflict`

**Cause**: Email address already linked to a different GitHub account

**Scenario**:
1. Developer created account with email `alice@example.com` and GitHub ID `12345`
2. Different user tries to log in with same email but GitHub ID `67890`
3. Server rejects to prevent account takeover

**Fix**:
- User must use the correct GitHub account (ID `12345`)
- Or contact admin to unlink GitHub account from email

### "Account Inactive" Error

**Symptom**: OAuth flow redirects back with `?error=account_inactive`

**Cause**: Developer account exists but is deactivated

**Fix**:
```bash
# Reactivate developer account (requires admin)
./server developer activate <developer-id>
```

### OAuth State Cookie Missing

**Symptom**: Server logs: "oauth state cookie missing"

**Cause**: Browser blocked cookies or cookies expired (5-minute window)

**Fix**:
1. Enable cookies in browser
2. Complete OAuth flow within 5 minutes
3. Check `SameSite` cookie policy (should be `Lax`)

### CSRF State Mismatch

**Symptom**: Server logs: "oauth state mismatch"

**Cause**: State parameter in callback doesn't match stored cookie value

**Possible causes**:
- CSRF attack attempt
- Cookie cleared between redirect and callback
- Multiple OAuth flows in parallel (race condition)

**Fix**:
- Retry authentication
- Clear cookies and try again
- Check for browser extensions interfering with cookies

---

## Quick Reference

### Required Environment Variables

```bash
GITHUB_CLIENT_ID=Iv1.abc123...
GITHUB_CLIENT_SECRET=your-secret-here
GITHUB_CALLBACK_URL=https://your-domain.com/auth/github/callback
```

### Optional Environment Variables

```bash
GITHUB_ALLOWED_ORGS=org1,org2,org3  # Restrict to org members
```

### OAuth Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/auth/github` | Initiate OAuth flow |
| GET | `/auth/github/callback` | OAuth callback handler |

### Related Code

- **OAuth client**: `internal/auth/oauth/github.go`
- **Route handlers**: `internal/api/handlers/dev_oauth.go`
- **Config loading**: `internal/config/config.go` (lines 215-220)
- **OpenAPI spec**: `docs/api/openapi.yaml` (lines 1396-1439)

### Related Documentation

- **Authentication guide**: [docs/integration/AUTHENTICATION.md](../integration/AUTHENTICATION.md)
- **Developer quickstart**: [docs/integration/DEVELOPER_QUICKSTART.md](../integration/DEVELOPER_QUICKSTART.md)

---

**Next Steps:**
- Set up GitHub OAuth apps for each environment (dev, staging, production)
- Configure environment variables
- Test OAuth flow end-to-end
- Monitor audit logs for OAuth events

**Document Version**: 1.0.0  
**Last Updated**: 2026-02-12  
**Maintenance**: Update when OAuth implementation changes or new security features added
