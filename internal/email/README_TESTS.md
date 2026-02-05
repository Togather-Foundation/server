# Email Service Testing

This directory contains tests for the email service package.

## Test Coverage

Current coverage: **41.5%** of statements

### What's Covered (100% coverage):
- ✅ `NewService()` - Service initialization with config validation
- ✅ `validateEmailAddress()` - Email format and header injection validation
- ✅ `validateInviteURL()` - URL scheme and XSS protection
- ✅ `renderTemplate()` - Template rendering with XSS escaping
- ✅ `SendInvitation()` validation path (email disabled mode)

### What's Not Covered:
- ⚠️ `send()` - Actual SMTP connection and sending (5% coverage)
- ⚠️ `SendInvitation()` - SMTP sending path when enabled (lines 82-96)

**Why?** Unit tests use `Enabled: false` to avoid requiring a real SMTP server. The SMTP sending logic is tested via the E2E test with MailHog.

## Running Tests

### Unit Tests (Fast, No External Dependencies)
```bash
# Run all tests with coverage
go test -v -cover ./internal/email/

# Short mode (skips E2E tests)
go test -v -short ./internal/email/

# Generate coverage report
go test -coverprofile=coverage.out ./internal/email/
go tool cover -html=coverage.out
```

### E2E Tests (Requires MailHog)

The E2E test `TestSendInvitation_E2E_WithMailHog` verifies actual email sending with a test SMTP server.

**Setup MailHog:**
```bash
docker run -d -p 1025:1025 -p 8025:8025 mailhog/mailhog
```

**Run E2E test:**
```bash
# Run without -short flag to include E2E tests
go test -v ./internal/email/ -run TestSendInvitation_E2E

# Check sent emails at http://localhost:8025
```

**Cleanup:**
```bash
docker stop $(docker ps -q --filter ancestor=mailhog/mailhog)
```

## Test Structure

### Unit Tests (service_test.go)

**Validation Tests:**
- `TestValidateEmailAddress_*` - Email format, header injection, edge cases
- `TestValidateInviteURL_*` - URL schemes, XSS vectors, malformed URLs

**Service Tests:**
- `TestNewService_*` - Service creation with various configs
- `TestSendInvitation_*` - Invitation sending with disabled mode
- `TestRenderTemplate_*` - Template rendering with XSS protection
- `TestSend_ValidatesRecipient` - Recipient validation in send()

**Security Tests:**
- Header injection blocked (CRLF/LF in email addresses)
- XSS protection (javascript:, data:, script tags)
- Template escaping (HTML injection in InvitedBy field)

### Test Templates (testdata/templates/)

- `invitation.html` - Test template matching production template structure

## Coverage Notes

The 41.5% coverage primarily reflects untested SMTP sending code. The critical validation and security logic has near 100% coverage:

| Function | Coverage | Notes |
|----------|----------|-------|
| `NewService()` | 100% | All paths tested |
| `validateEmailAddress()` | 83.3% | All security cases covered |
| `validateInviteURL()` | 100% | All XSS vectors blocked |
| `renderTemplate()` | 100% | XSS escaping verified |
| `SendInvitation()` | 43.8% | Disabled mode + validation covered, SMTP path requires E2E test |
| `send()` | 5% | Requires real SMTP server (tested via E2E) |

**To increase coverage to 80%+**, run E2E tests with MailHog regularly.

## Security Test Checklist

✅ Email header injection blocked (server-a200)  
✅ JavaScript URL schemes blocked (server-l2ki)  
✅ XSS in template fields escaped  
✅ Invalid email formats rejected  
✅ Malformed URLs rejected  
✅ Email sending works when disabled (logs only)

## Future Improvements

- [ ] Mock SMTP client for testing send() without external server
- [ ] Add tests for SMTP connection errors (timeout, auth failure)
- [ ] Add tests for HTML rendering in different email clients
- [ ] Add performance tests (template rendering speed)
