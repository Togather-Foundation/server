# Email Configuration Guide

**Audience:** System administrators setting up email for user invitations

This guide explains how to configure email sending for the Togather SEL Backend. Email is required to send invitation links to new users. The system supports two email providers: **Resend API** (recommended) and **SMTP** (legacy, backward compatible).

---

## Table of Contents

- [Overview](#overview)
- [Resend API Setup (Recommended)](#resend-api-setup-recommended)
- [Gmail SMTP Setup (Legacy)](#gmail-smtp-setup-legacy)
- [Provider Selection](#provider-selection)
- [Environment Configuration](#environment-configuration)
- [Testing Email in Development](#testing-email-in-development)
- [Testing Email Delivery](#testing-email-delivery)
- [Troubleshooting](#troubleshooting)
- [Alternative SMTP Providers](#alternative-smtp-providers)
- [Production Deployment](#production-deployment)

---

## Overview

The Togather user administration system sends invitation emails when admins create new user accounts. Email configuration is **required** for the invitation workflow to function properly.

**Email Features:**
- Sends invitation emails with secure token links (valid 7 days)
- Supports resending invitations
- HTML email templates with branding
- Secure delivery via Resend API or SMTP with TLS encryption
- Can be disabled for development/testing

**Recommended Provider:** Resend API (modern, secure, API-based)

**Why Resend over SMTP?**
- **Send-only API key** vs full account access (Gmail app passwords)
- **Modern REST API** with official Go SDK
- **No deprecated auth methods** (no app passwords or OAuth setup)
- **Built-in rate limiting** with clear error messages
- **Free tier:** 3,000 emails/month (100/day) - sufficient for invitations

---

## Resend API Setup (Recommended)

Resend is a modern email API designed for transactional emails. This is the **recommended setup** for new deployments.

### Prerequisites

- Domain with DNS access (for domain verification)
- Resend account (free tier available)

### Step-by-Step Setup

#### 1. Create a Resend Account

1. Visit [resend.com](https://resend.com/) and sign up for a free account
2. Verify your email address
3. Log in to the Resend dashboard

#### 2. Add Your Domain

For production emails, you need to verify your domain:

1. In the Resend dashboard, go to **Domains**
2. Click **Add Domain**
3. Enter your domain (e.g., `togather.foundation`)
4. Resend will display DNS records you need to add

#### 3. Configure DNS Records

Add the following DNS records to your domain (values are examples - use what Resend provides):

**SPF Record (TXT):**
```
Name: @
Type: TXT
Value: v=spf1 include:_spf.resend.com ~all
TTL: 3600
```

**DKIM Records (TXT):**
Resend provides 3 DKIM records for authentication. Add all of them:
```
Name: resend._domainkey
Type: TXT
Value: <provided by Resend>
TTL: 3600

Name: resend2._domainkey
Type: TXT
Value: <provided by Resend>
TTL: 3600

Name: resend3._domainkey
Type: TXT
Value: <provided by Resend>
TTL: 3600
```

**DMARC Record (TXT) - Optional but recommended:**
```
Name: _dmarc
Type: TXT
Value: v=DMARC1; p=none; rua=mailto:admin@togather.foundation
TTL: 3600
```

**Note:** DNS propagation can take 5-60 minutes. Resend will verify automatically once records are detected.

#### 4. Get Your API Key

1. In the Resend dashboard, go to **API Keys**
2. Click **Create API Key**
3. Name it (e.g., "Togather Production")
4. Select **Sending access** (send-only, most secure)
5. Optionally restrict to your domain
6. Click **Add**
7. **Copy the API key** - you won't be able to see it again
   - Format: `re_xxxxxxxxxxxxxxxxxxxx`

#### 5. Configure Environment Variables

Edit your `.env` file and add the following email configuration:

```bash
# Email Configuration (Resend API)
EMAIL_ENABLED=true
EMAIL_PROVIDER=resend
EMAIL_FROM=noreply@togather.foundation
RESEND_API_KEY=re_xxxxxxxxxxxxxxxxxxxx
```

**Field Explanations:**

| Variable | Description | Example |
|----------|-------------|---------|
| `EMAIL_ENABLED` | Set to `true` to enable email sending | `true` |
| `EMAIL_PROVIDER` | Set to `resend` for Resend API | `resend` |
| `EMAIL_FROM` | Email address that appears as sender (must match verified domain) | `noreply@togather.foundation` |
| `RESEND_API_KEY` | API key from Resend dashboard | `re_abc123xyz...` |

**Notes:**
- `EMAIL_FROM` must use your verified domain (e.g., `noreply@togather.foundation`)
- Use a no-reply address for transactional emails
- **Never commit `.env` to version control** - it contains secrets

#### 6. Restart the Server

After configuring `.env`, restart the server for changes to take effect:

```bash
# Using Docker Compose
docker compose restart

# Using systemd
sudo systemctl restart togather-server

# Direct execution
./server
```

### Free Tier Limits

Resend's free tier includes:

- **3,000 emails per month**
- **100 emails per day**
- All features included
- No credit card required

These limits are sufficient for most invitation workflows. If you exceed the daily limit, emails will fail with a clear rate limit error message that admins can see in the logs.

### Rate Limit Behavior

When rate limits are exceeded:
- Email sending fails immediately (no retry)
- Error is logged with clear message including reset time
- Admin sees error in server logs
- User account is still created (admin can resend invitation later)

**Example Rate Limit Error:**
```json
{
  "level": "error",
  "component": "email",
  "error": "email rate limit exceeded (limit: 100, resets in: 3600 seconds)",
  "message": "failed to send invitation email"
}
```

---

## Gmail SMTP Setup (Legacy)

Gmail provides free SMTP service that works for invitation emails. This setup is **kept for backward compatibility** but is no longer the recommended approach.

**Why SMTP is Legacy:**
- Requires full account access via App Passwords
- App Passwords are deprecated by Google (may be removed in future)
- More complex setup (2FA, app password generation)
- Less secure than API-based authentication

### Prerequisites

- **Gmail account** (personal or Google Workspace)
- **2-Step Verification** enabled on the account
- **App Password** generated for SMTP access

### Step-by-Step Setup

#### 1. Enable 2-Step Verification

Gmail requires 2-Step Verification before you can generate App Passwords.

1. Go to your [Google Account Security Settings](https://myaccount.google.com/security)
2. Under "How you sign in to Google", click **2-Step Verification**
3. Follow the prompts to enable 2-Step Verification
4. Verify your phone number and set up a backup method

#### 2. Generate an App Password

App Passwords are 16-character codes that let apps and devices sign in to your Google Account.

**⚠️ Important:** You must use an App Password, **not your regular Gmail password**. Regular passwords will not work with SMTP authentication.

1. Visit [Google App Passwords](https://myaccount.google.com/apppasswords)
   - If you don't see this option, your account may not have 2-Step Verification enabled
2. In the "Select app" dropdown, choose **Mail**
3. In the "Select device" dropdown, choose **Other (Custom name)**
4. Enter a name like "Togather SEL Backend"
5. Click **Generate**
6. Google will display a 16-character App Password (format: `xxxx xxxx xxxx xxxx`)
7. **Copy this password** - you won't be able to see it again
8. **Remove the spaces** when entering it in your `.env` file

**Example App Password:**
```
Generated: abcd efgh ijkl mnop
In .env:   abcdefghijklmnop
```

#### 3. Configure Environment Variables

Edit your `.env` file and add the following email configuration:

```bash
# Email Configuration (Gmail SMTP)
EMAIL_ENABLED=true
EMAIL_PROVIDER=smtp
EMAIL_FROM=your-email@gmail.com
SMTP_HOST=smtp.gmail.com
SMTP_PORT=587
SMTP_USER=your-email@gmail.com
SMTP_PASSWORD=your-app-password-here
```

**Field Explanations:**

| Variable | Description | Example |
|----------|-------------|---------|
| `EMAIL_ENABLED` | Set to `true` to enable email sending | `true` |
| `EMAIL_PROVIDER` | Set to `smtp` for SMTP delivery | `smtp` |
| `EMAIL_FROM` | Email address that appears as sender | `noreply@yourdomain.com` |
| `SMTP_HOST` | Gmail SMTP server hostname | `smtp.gmail.com` |
| `SMTP_PORT` | SMTP port (587 for TLS) | `587` |
| `SMTP_USER` | Your Gmail address | `yourname@gmail.com` |
| `SMTP_PASSWORD` | App Password (16 chars, no spaces) | `abcdefghijklmnop` |

**Notes:**
- `EMAIL_FROM` can be different from `SMTP_USER` but Gmail may override it
- For Google Workspace accounts, use your work email for `SMTP_USER`
- **Never commit `.env` to version control** - it contains secrets

#### 4. Restart the Server

After configuring `.env`, restart the server for changes to take effect:

```bash
# Using Docker Compose
docker compose restart

# Using systemd
sudo systemctl restart togather-server

# Direct execution
./server
```

---

## Provider Selection

The email system supports multiple providers via the `EMAIL_PROVIDER` environment variable.

### Available Providers

| Provider | Value | Description | Recommended For |
|----------|-------|-------------|-----------------|
| Resend | `resend` | Modern API-based email delivery | New deployments, production |
| SMTP | `smtp` | Traditional SMTP (Gmail, SendGrid, etc.) | Legacy systems, backward compatibility |

### Configuration Examples

**Resend (Recommended):**
```bash
EMAIL_ENABLED=true
EMAIL_PROVIDER=resend
EMAIL_FROM=noreply@togather.foundation
RESEND_API_KEY=re_xxxxxxxxxxxxxxxxxxxx
```

**SMTP (Gmail):**
```bash
EMAIL_ENABLED=true
EMAIL_PROVIDER=smtp
EMAIL_FROM=your-email@gmail.com
SMTP_HOST=smtp.gmail.com
SMTP_PORT=587
SMTP_USER=your-email@gmail.com
SMTP_PASSWORD=your-app-password
```

**SMTP (Other providers):**
```bash
EMAIL_ENABLED=true
EMAIL_PROVIDER=smtp
EMAIL_FROM=noreply@yourdomain.com
SMTP_HOST=smtp.sendgrid.net
SMTP_PORT=587
SMTP_USER=apikey
SMTP_PASSWORD=your-sendgrid-api-key
```

### Default Behavior

- If `EMAIL_PROVIDER` is **not set**, defaults to `smtp` (for backward compatibility with existing deployments)
- If `EMAIL_PROVIDER=resend`, `RESEND_API_KEY` is required
- If `EMAIL_PROVIDER=smtp`, SMTP settings (`SMTP_HOST`, `SMTP_USER`, etc.) are required

### Startup Validation

On server startup, the email configuration is validated:

**Resend Provider:**
```json
{"level":"info","component":"email","provider":"resend","from":"noreply@togather.foundation","message":"email service initialized"}
```

**SMTP Provider:**
```json
{"level":"info","component":"email","provider":"smtp","from":"your-email@gmail.com","message":"email service initialized"}
```

**Email Disabled:**
```json
{"level":"warn","component":"email","message":"email service disabled, invitation emails will be logged only"}
```

---

## Environment Configuration

The email system is configured via environment variables in `.env`. Here's the complete configuration reference:

### Required Variables (All Providers)

```bash
EMAIL_ENABLED=true                        # Enable/disable email sending
EMAIL_PROVIDER=resend                     # Provider: "resend" or "smtp" (default: "smtp")
EMAIL_FROM=noreply@togather.foundation    # Sender email address
```

### Resend-Specific Variables

Required when `EMAIL_PROVIDER=resend`:

```bash
RESEND_API_KEY=re_xxxxxxxxxxxxxxxxxxxx    # Resend API key from dashboard
```

### SMTP-Specific Variables

Required when `EMAIL_PROVIDER=smtp`:

```bash
SMTP_HOST=smtp.gmail.com                  # SMTP server hostname
SMTP_PORT=587                             # SMTP port (587 for TLS)
SMTP_USER=your-email@gmail.com            # SMTP authentication username
SMTP_PASSWORD=your-app-password           # SMTP authentication password
```

### Optional Variables

The following variables are set automatically and don't need configuration:

- **Invitation expiry**: 7 days (hardcoded, may become configurable in future)
- **Email templates directory**: `web/email/templates/` (automatically resolved)

### Configuration Validation

On startup, the server validates email configuration:

- **If `EMAIL_ENABLED=false`**: Email is skipped, invitations are logged to console
- **If `EMAIL_ENABLED=true` and `EMAIL_PROVIDER=resend`**: `RESEND_API_KEY` must be set or server fails to start
- **If `EMAIL_ENABLED=true` and `EMAIL_PROVIDER=smtp`**: All SMTP settings must be valid or server fails to start

**Startup Logs (Resend Enabled):**
```json
{"level":"info","component":"email","provider":"resend","from":"noreply@togather.foundation","message":"email service initialized"}
```

**Startup Logs (SMTP Enabled):**
```json
{"level":"info","component":"email","provider":"smtp","from":"your-email@gmail.com","message":"email service initialized"}
```

**Startup Logs (Email Disabled):**
```json
{"level":"warn","component":"email","message":"email service disabled, invitation emails will be logged only"}
```

---

## Testing Email in Development

During development, you may want to **disable actual email sending** to avoid spamming test addresses.

### Disable Email Sending

Set `EMAIL_ENABLED=false` in `.env`:

```bash
EMAIL_ENABLED=false
```

**Behavior:**
- Invitation emails are **not sent**
- Email content is **logged to console** instead
- User accounts are still created normally
- You can test the invitation flow by copying the link from logs

**Example Log Output:**
```json
{
  "level": "info",
  "component": "email",
  "to": "test@example.com",
  "invited_by": "admin",
  "link": "http://localhost:8080/accept-invitation?token=abc123...",
  "message": "email service disabled, skipping invitation email"
}
```

### Using Real Email in Development

If you want to test real email delivery in development:

**Option 1: Resend (Recommended)**
1. Set `EMAIL_ENABLED=true`
2. Set `EMAIL_PROVIDER=resend`
3. Use a verified domain or Resend's sandbox domain
4. Configure Resend API key in `.env`
5. Create a test user with your own email address
6. Check your inbox for the invitation

**Option 2: Gmail SMTP**
1. Set `EMAIL_ENABLED=true`
2. Set `EMAIL_PROVIDER=smtp`
3. Use your personal Gmail account
4. Generate an App Password (see [Gmail Setup](#gmail-smtp-setup-legacy))
5. Configure SMTP settings in `.env`
6. Create a test user with your own email address
7. Check your inbox for the invitation

---

## Testing Email Delivery

After configuring email, test that invitations are delivered successfully.

### Method 1: Create a Test User (Recommended)

1. Start the server with email enabled
2. Log in as admin
3. Navigate to **Users** page
4. Create a new user with **your own email address**
5. Check your inbox for the invitation email
6. Click the invitation link and set a password
7. Verify you can log in with the new account

### Method 2: Check Server Logs

If email is disabled or failing, check server logs for email events:

```bash
# Docker Compose logs
docker compose logs -f server | grep "email"

# Systemd logs
journalctl -u togather-server -f | grep "email"

# Direct logs (if logging to file)
tail -f server.log | grep "email"
```

**Successful Email Log:**
```json
{
  "level": "info",
  "component": "email",
  "to": "test@example.com",
  "invited_by": "admin",
  "message": "invitation email sent successfully"
}
```

**Failed Email Log:**
```json
{
  "level": "error",
  "component": "email",
  "email": "test@example.com",
  "error": "SMTP authentication failed: 535-5.7.8 Username and Password not accepted",
  "message": "failed to send invitation email"
}
```

### Method 3: Test SMTP Connection

Use `telnet` or `openssl` to test SMTP connectivity:

```bash
# Test SMTP connection
telnet smtp.gmail.com 587

# If telnet is not available, use openssl
openssl s_client -connect smtp.gmail.com:587 -starttls smtp
```

**Expected Output:**
```
220 smtp.gmail.com ESMTP ready
```

If connection fails, check:
- Firewall rules (port 587 outbound must be allowed)
- DNS resolution (`nslookup smtp.gmail.com`)
- Internet connectivity

---

## Troubleshooting

This section covers common issues for both Resend and SMTP providers.

### Resend: Invalid API Key

**Error Message:**
```
resend API error: unauthorized
```

**Causes:**
1. API key is incorrect or expired
2. API key doesn't have sending permissions
3. API key is from a different Resend account

**Solutions:**
1. **Verify API key** in Resend dashboard:
   - Go to [resend.com](https://resend.com/) → API Keys
   - Check that the key is active and has "Sending access"
   - Copy the key exactly (format: `re_xxxxxxxxxxxxxxxxxxxx`)
2. **Regenerate API key** if needed:
   - Delete old key in dashboard
   - Create new key with "Sending access"
   - Update `RESEND_API_KEY` in `.env`
3. **Check key format**:
   - Should start with `re_`
   - No spaces or line breaks
   - Copy/paste carefully from dashboard

### Resend: Rate Limit Exceeded

**Error Message:**
```
email rate limit exceeded (limit: 100, resets in: 3600 seconds)
```

**Causes:**
1. Exceeded daily limit (100 emails/day on free tier)
2. Exceeded monthly limit (3,000 emails/month on free tier)

**Solutions:**
1. **Wait for reset** - Daily limits reset at midnight UTC
2. **Check usage** in Resend dashboard:
   - Go to [resend.com](https://resend.com/) → Usage
   - See how many emails sent today/this month
3. **Upgrade plan** if you need higher limits:
   - Free: 3,000/month, 100/day
   - Paid plans: Higher limits available
4. **Throttle invitations** - Avoid bulk invitation sends
5. **Resend failed invitations** after reset time

### Resend: Domain Not Verified

**Error Message:**
```
resend API error: domain not verified
```

**Causes:**
1. DNS records not added or not propagated yet
2. DNS records incorrect or incomplete
3. Using domain that wasn't added to Resend

**Solutions:**
1. **Check domain status** in Resend dashboard:
   - Go to Domains → Select your domain
   - Verify all DNS records show "Verified"
2. **Wait for DNS propagation** (5-60 minutes)
3. **Verify DNS records** externally:
   ```bash
   # Check SPF record
   dig TXT togather.foundation | grep spf
   
   # Check DKIM records
   dig TXT resend._domainkey.togather.foundation
   ```
4. **Use Resend sandbox** for testing (no domain verification needed):
   ```bash
   EMAIL_FROM=onboarding@resend.dev
   ```

### SMTP: Authentication Failed (535 Error)

**Error Message:**
```
SMTP authentication failed: 535-5.7.8 Username and Password not accepted
```

**Causes:**
1. Using regular Gmail password instead of App Password
2. App Password contains spaces
3. Wrong username

**Solutions:**
1. **Verify you're using an App Password**, not your regular Gmail password
   - Regular passwords will **not work** even if correct
2. **Remove spaces** from App Password (should be 16 characters with no spaces)
3. **Check SMTP_USER** matches your Gmail address exactly
4. **Regenerate App Password** if you've lost the original:
   - Go to [Google App Passwords](https://myaccount.google.com/apppasswords)
   - Delete old App Password
   - Generate new one

### SMTP: Connection Refused (Connection Error)

**Error Message:**
```
failed to connect to SMTP server: dial tcp 142.251.16.108:587: connect: connection refused
```

**Causes:**
1. SMTP port blocked by firewall
2. Wrong SMTP host or port
3. Server cannot reach internet

**Solutions:**
1. **Check SMTP settings** in `.env`:
   ```bash
   SMTP_HOST=smtp.gmail.com
   SMTP_PORT=587
   ```
2. **Test connectivity** with telnet:
   ```bash
   telnet smtp.gmail.com 587
   ```
3. **Check firewall rules** - Port 587 outbound must be allowed
4. **Verify internet access** - Try pinging Google:
   ```bash
   ping smtp.gmail.com
   ```

### SMTP: TLS Handshake Error

**Error Message:**
```
failed to start TLS: x509: certificate signed by unknown authority
```

**Causes:**
1. System certificates not installed
2. Proxy or firewall intercepting TLS
3. System clock significantly wrong

**Solutions:**
1. **Update system certificates**:
   ```bash
   # Debian/Ubuntu
   sudo apt-get update && sudo apt-get install ca-certificates
   
   # Alpine Linux (Docker)
   apk add ca-certificates
   
   # RHEL/CentOS
   sudo yum install ca-certificates
   ```
2. **Check system time** - TLS requires accurate time:
   ```bash
   date
   # If wrong, sync with NTP
   sudo ntpdate -s time.nist.gov
   ```
3. **Use correct SMTP host**:
   ```bash
   SMTP_HOST=smtp.gmail.com  # Correct
   # NOT smtp.googlemail.com or other variants
   ```

### Email Goes to Spam (Both Providers)

**Problem:** Invitation emails are delivered but go to spam/junk folder

**Causes:**
1. No SPF/DKIM/DMARC records for sender domain
2. Sender domain doesn't match provider domain
3. Poor email reputation
4. New domain with no sending history

**Solutions:**

**For Resend:**
1. **Verify domain properly** with all DNS records (SPF, DKIM, DMARC)
2. **Warm up domain** - Send small volumes initially
3. **Use proper sender address**:
   ```bash
   EMAIL_FROM=noreply@togather.foundation  # Matches verified domain
   ```
4. **Check DNS records are active**:
   ```bash
   dig TXT togather.foundation | grep spf
   dig TXT resend._domainkey.togather.foundation
   ```

**For SMTP (Gmail):**
1. Ask users to check spam folders
2. Use `EMAIL_FROM` with Gmail domain:
   ```bash
   EMAIL_FROM=your-email@gmail.com
   ```
3. **Long-term:** Set up SPF/DKIM/DMARC for your domain:
   ```
   # SPF record
   v=spf1 include:_spf.google.com ~all
   
   # DMARC record
   v=DMARC1; p=none; rua=mailto:admin@yourdomain.com
   ```

### Email Service Disabled Warning (Both Providers)

**Warning Message:**
```json
{"level":"warn","message":"email service disabled, invitation emails will be logged only"}
```

**Cause:** `EMAIL_ENABLED=false` in `.env`

**Solution:**
- If intentional (development), this is expected behavior
- If unintentional (production), set `EMAIL_ENABLED=true`

### SMTP: Cannot Generate App Password

**Problem:** "App Passwords" option not available in Google Account settings

**Causes:**
1. 2-Step Verification not enabled
2. Using a Google Workspace account with security policies
3. Using Google Advanced Protection

**Solutions:**
1. **Enable 2-Step Verification**:
   - Go to [Google Security](https://myaccount.google.com/security)
   - Enable 2-Step Verification
2. **For Google Workspace accounts**:
   - Contact your workspace admin
   - Admin must enable "Less secure app access" or "App Passwords"
3. **For Advanced Protection users**:
   - Use OAuth2 instead of SMTP (not currently supported)
   - Or use a different email service provider

---

## Alternative SMTP Providers

If you're using the SMTP provider (`EMAIL_PROVIDER=smtp`), you can use any SMTP server, not just Gmail. Here are common alternatives:

### SendGrid

```bash
EMAIL_ENABLED=true
EMAIL_PROVIDER=smtp
EMAIL_FROM=noreply@yourdomain.com
SMTP_HOST=smtp.sendgrid.net
SMTP_PORT=587
SMTP_USER=apikey
SMTP_PASSWORD=your-sendgrid-api-key
```

**Pros:** Free tier (100 emails/day), good deliverability, no 2FA requirement  
**Cons:** Requires signup, API key management

### Mailgun

```bash
EMAIL_ENABLED=true
EMAIL_PROVIDER=smtp
EMAIL_FROM=noreply@yourdomain.com
SMTP_HOST=smtp.mailgun.org
SMTP_PORT=587
SMTP_USER=postmaster@yourdomain.mailgun.org
SMTP_PASSWORD=your-mailgun-smtp-password
```

**Pros:** Free tier (5000 emails/month), transactional email focused  
**Cons:** Requires signup, domain verification

### AWS SES (Simple Email Service)

```bash
EMAIL_ENABLED=true
EMAIL_PROVIDER=smtp
EMAIL_FROM=noreply@yourdomain.com
SMTP_HOST=email-smtp.us-east-1.amazonaws.com
SMTP_PORT=587
SMTP_USER=your-aws-smtp-username
SMTP_PASSWORD=your-aws-smtp-password
```

**Pros:** Pay-as-you-go ($0.10/1000 emails), highly scalable  
**Cons:** Requires AWS account, initial sandbox restrictions

### Postfix (Self-Hosted)

If you run your own mail server with Postfix:

```bash
EMAIL_ENABLED=true
EMAIL_PROVIDER=smtp
EMAIL_FROM=noreply@yourdomain.com
SMTP_HOST=localhost
SMTP_PORT=587
SMTP_USER=your-username
SMTP_PASSWORD=your-password
```

**Pros:** Full control, no external dependencies  
**Cons:** Requires mail server setup, deliverability challenges, maintenance burden

---

## Production Deployment

### Security Checklist

Before deploying to production:

**For Resend:**
- [ ] **Domain verified** - All DNS records (SPF, DKIM, DMARC) configured and verified
- [ ] **API key scoped** - Use send-only API key, restrict to domain if possible
- [ ] **Store API key securely** - Use environment variables, never commit to git
- [ ] **Use HTTPS for invite links** - Set `PUBLIC_URL=https://yourdomain.com` in `.env`
- [ ] **Use verified domain address** (e.g., `noreply@togather.foundation`)
- [ ] **Monitor rate limits** - Check Resend dashboard for usage
- [ ] **Monitor email delivery** - Check logs for failures

**For SMTP:**
- [ ] **Use App Passwords** (Gmail) or API keys (other providers), never regular passwords
- [ ] **Store credentials securely** - Use environment variables, not hardcoded config
- [ ] **Use HTTPS for invite links** - Set `PUBLIC_URL=https://yourdomain.com` in `.env`
- [ ] **Set up SPF/DKIM/DMARC** to prevent spoofing and improve deliverability
- [ ] **Use a dedicated sending address** (e.g., `noreply@yourdomain.com`)
- [ ] **Monitor email delivery** - Check logs for failures
- [ ] **Enable TLS** - Port 587 with STARTTLS (default configuration)

### Performance Considerations

**Resend:**
- **Free tier limits**: 3,000 emails/month (100/day)
- **Rate limit behavior**: Fails immediately with clear error (no retry)
- **Invitation emails are low volume** - Typically 1-10 per day for small nodes
- **Failed emails are logged** - Admins can resend invitations manually

**SMTP (Gmail):**
- **Gmail limits**: 500 emails/day for personal accounts, 2000/day for Google Workspace
- **Invitation emails are low volume** - Typically 1-10 per day for small nodes
- **Email sending is asynchronous** - Won't block user creation API calls
- **Failed emails are logged** - Admins can resend invitations manually

### Monitoring

Monitor email delivery in production:

```bash
# Check for email failures (both providers)
grep "failed to send invitation email" /path/to/server.log

# Count successful invitations
grep "invitation email sent successfully" /path/to/server.log | wc -l

# Resend-specific: Monitor rate limits
grep "resend rate limit exceeded" /path/to/server.log

# SMTP-specific: Monitor authentication issues
grep "SMTP authentication failed" /path/to/server.log
```

### Scaling

For high-volume deployments:

1. **Use Resend or transactional email service** (Resend recommended for modern deployments)
2. **Upgrade plan** if exceeding free tier limits (Resend, SendGrid, Mailgun)
3. **Implement email queue** with retry logic (planned feature)
4. **Rate limit user creation** to avoid exceeding provider limits
5. **Monitor bounce rates** and maintain email list hygiene

---

## Best Practices

### Email Content

- Invitation emails are **HTML formatted** with responsive design
- Links are **bold and prominent** for easy clicking
- Tokens are **URL-safe** and work in all email clients
- Emails include **sender information** for transparency

### Sender Address

- Use a **no-reply address** for invitations (e.g., `noreply@yourdomain.com`)
- **Don't use personal email addresses** as `EMAIL_FROM` in production
- **Match sender domain** to your service domain for trust

### Domain Reputation

- **Warm up new domains** gradually (don't send large batches immediately)
- **Monitor bounce rates** and remove invalid addresses
- **Handle unsubscribes** appropriately (invitations are transactional, not marketing)
- **Set up feedback loops** with email providers

### Testing

- **Test in staging first** before production deployment
- **Use real email addresses** for testing (not `test@example.com`)
- **Check multiple email clients** (Gmail, Outlook, Apple Mail)
- **Verify invitation links** open correctly in browsers

---

## Related Documentation

- [User Management Guide](user-management.md) - Managing users and invitations
- [Admin Users API Reference](../api/admin-users.md) - API endpoint documentation
- [Deployment Guide](../deploy/quickstart.md) - Production deployment instructions

---

## Support

For email configuration issues:

**Resend:**
- **Check Resend status**: [status.resend.com](https://status.resend.com/)
- **Resend documentation**: [resend.com/docs](https://resend.com/docs)
- **Verify domain in dashboard**: [resend.com/domains](https://resend.com/domains)
- **Check API key**: [resend.com/api-keys](https://resend.com/api-keys)

**SMTP (Gmail):**
- **Check Gmail App Password setup**: [Google Help](https://support.google.com/accounts/answer/185833)
- **Test SMTP connectivity** with telnet or openssl
- **Verify firewall rules** allow port 587 outbound

**Both Providers:**
- **Review server logs** for detailed error messages
- **Report bugs** at the Togather project repository

---

**Last Updated:** February 18, 2026  
**Version:** 2.0
