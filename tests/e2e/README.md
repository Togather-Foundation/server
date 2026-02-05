# Admin UI Testing with Playwright

## Quick Start

Run E2E tests against the live admin UI:

```bash
# Start the server
make dev

# In another terminal, run the tests
uvx --from playwright --with playwright python tests/e2e/test_admin_ui_python.py
```

## Setup

### First time only: Install Playwright browsers

```bash
uvx --from playwright playwright install chromium
```

## Test Script

`tests/e2e/test_admin_ui_python.py` - Comprehensive E2E test that:
- Tests login flow (success and failure)
- Verifies dashboard loads and displays stats correctly
- Tests all admin pages (events, duplicates, API keys)
- Verifies navigation and logout functionality
- **Captures and reports console errors** (including CSP violations)
- **Takes screenshots** of each page for debugging

## Console Error Detection

The test automatically captures all browser console messages and provides a summary at the end:

```
⚠️  Console Errors Found:
   Total errors: 2

   Recent errors:
   • Failed to load resource: the server responded with a status of 401 (Unauthorized)
   • Failed to load duplicates: Error: Failed to load duplicates: Unauthorized
```

**Always check the console error summary** after running tests - it will catch:
- JavaScript errors
- Failed network requests
- CSP (Content Security Policy) violations
- API errors

## Environment Variables

The test reads `ADMIN_PASSWORD` from environment or uses the default from `.env`.

```bash
# Run with custom password
ADMIN_PASSWORD=mypassword uvx --from playwright --with playwright python tests/e2e/test_admin_ui_python.py
```

## Screenshots

All screenshots are saved to `/tmp/admin_*.png` for debugging:
- `/tmp/admin_login.png` - Login page
- `/tmp/admin_dashboard.png` - Dashboard after login
- `/tmp/admin_events.png` - Events list page
- `/tmp/admin_duplicates.png` - Duplicates page
- `/tmp/admin_api_keys.png` - API keys page
- `/tmp/admin_after_logout.png` - After logout
- `/tmp/admin_error.png` - If test fails

## Integration with OpenCode

This test uses Python Playwright which is compatible with OpenCode's webapp-testing skill and the `with_server.py` helper script.
