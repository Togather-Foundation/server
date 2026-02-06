# Email Configuration Guide

**Audience:** System administrators setting up email for user invitations

This guide explains how to configure email sending for the Togather SEL Backend. Email is required to send invitation links to new users. The system supports SMTP-based email delivery with Gmail as the recommended provider.

---

## Table of Contents

- [Overview](#overview)
- [Gmail SMTP Setup (Recommended)](#gmail-smtp-setup-recommended)
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
- Secure SMTP with TLS encryption
- Can be disabled for development/testing

**Recommended Provider:** Gmail SMTP (free, reliable, well-documented)

---

## Gmail SMTP Setup (Recommended)

Gmail provides free SMTP service that works well for invitation emails. This is the **recommended setup** for most deployments.

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

## Environment Configuration

The email system is configured via environment variables in `.env`. Here's the complete configuration reference:

### Required Variables

```bash
EMAIL_ENABLED=true                        # Enable/disable email sending
EMAIL_FROM=noreply@togather.foundation    # Sender email address
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
- **If `EMAIL_ENABLED=true`**: All SMTP settings must be valid or server fails to start

**Startup Logs (Email Enabled):**
```json
{"level":"info","component":"email","message":"email service initialized","from":"noreply@togather.foundation"}
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

1. Set `EMAIL_ENABLED=true`
2. Use your personal Gmail account
3. Generate an App Password (see [Gmail Setup](#gmail-smtp-setup-recommended))
4. Configure SMTP settings in `.env`
5. Create a test user with your own email address
6. Check your inbox for the invitation

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

### Authentication Failed (535 Error)

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

### Connection Refused (Connection Error)

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

### TLS Handshake Error

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

### Email Goes to Spam

**Problem:** Invitation emails are delivered but go to spam/junk folder

**Causes:**
1. No SPF/DKIM/DMARC records for sender domain
2. Sender domain doesn't match SMTP domain
3. Poor email reputation

**Solutions:**

**Short-term:**
1. Ask users to check spam folders
2. Use `EMAIL_FROM` with Gmail domain:
   ```bash
   EMAIL_FROM=your-email@gmail.com
   ```

**Long-term (Production):**
1. **Set up SPF record** for your domain:
   ```
   v=spf1 include:_spf.google.com ~all
   ```
2. **Set up DKIM** via Google Admin Console (Google Workspace)
3. **Set up DMARC record**:
   ```
   v=DMARC1; p=none; rua=mailto:admin@yourdomain.com
   ```
4. **Use dedicated email subdomain**:
   ```bash
   EMAIL_FROM=noreply@mail.yourdomain.com
   ```

### Email Service Disabled Warning

**Warning Message:**
```json
{"level":"warn","message":"email service disabled, invitation emails will be logged only"}
```

**Cause:** `EMAIL_ENABLED=false` in `.env`

**Solution:**
- If intentional (development), this is expected behavior
- If unintentional (production), set `EMAIL_ENABLED=true`

### Cannot Generate App Password

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

While Gmail is recommended, you can use any SMTP provider. Here are common alternatives:

### SendGrid

```bash
EMAIL_ENABLED=true
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

- [ ] **Use App Passwords**, never regular passwords
- [ ] **Store credentials securely** - Use environment variables, not hardcoded config
- [ ] **Use HTTPS for invite links** - Set `PUBLIC_URL=https://yourdomain.com` in `.env`
- [ ] **Set up SPF/DKIM/DMARC** to prevent spoofing and improve deliverability
- [ ] **Use a dedicated sending address** (e.g., `noreply@yourdomain.com`)
- [ ] **Monitor email delivery** - Check logs for failures
- [ ] **Enable TLS** - Port 587 with STARTTLS (default configuration)

### Performance Considerations

- **Gmail limits**: 500 emails/day for personal accounts, 2000/day for Google Workspace
- **Invitation emails are low volume** - Typically 1-10 per day for small nodes
- **Email sending is asynchronous** - Won't block user creation API calls
- **Failed emails are logged** - Admins can resend invitations manually

### Monitoring

Monitor email delivery in production:

```bash
# Check for email failures
grep "failed to send invitation email" /path/to/server.log

# Count successful invitations
grep "invitation email sent successfully" /path/to/server.log | wc -l

# Monitor SMTP authentication issues
grep "SMTP authentication failed" /path/to/server.log
```

### Scaling

For high-volume deployments:

1. **Use transactional email service** (SendGrid, Mailgun, AWS SES)
2. **Implement email queue** with retry logic (planned feature)
3. **Rate limit user creation** to avoid exceeding SMTP provider limits
4. **Monitor bounce rates** and maintain email list hygiene

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

- **Check Gmail App Password setup**: [Google Help](https://support.google.com/accounts/answer/185833)
- **Review server logs** for detailed error messages
- **Test SMTP connectivity** with telnet or openssl
- **Verify firewall rules** allow port 587 outbound
- **Report bugs** at the Togather project repository

---

**Last Updated:** February 5, 2026  
**Version:** 1.0
