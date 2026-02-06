# Admin UI Testing with Playwright

## Quick Start

Run E2E tests against the live admin UI:

```bash
# Start the server
make dev

# In another terminal, run the tests
# Option 1: Run original admin UI test script
uvx --from playwright --with playwright python tests/e2e/test_admin_ui_python.py

# Option 2: Run user management test suite with pytest
uvx --from playwright --with playwright pytest tests/e2e/test_user_management.py -v
```

## Setup

### First time only: Install Playwright browsers

```bash
uvx --from playwright playwright install chromium
```

## Test Scripts

### `tests/e2e/test_admin_ui_python.py`
Comprehensive E2E test script that:
- Tests login flow (success and failure)
- Verifies dashboard loads and displays stats correctly
- Tests all admin pages (events, duplicates, API keys, federation)
- Verifies navigation and logout functionality
- **Captures and reports console errors** (including CSP violations)
- **Takes screenshots** of each page for debugging

### `tests/e2e/test_user_management.py`
Comprehensive pytest-based test suite for user administration:

**Test Coverage:**
- **User List Page** (5 tests)
  - Page loads with correct structure
  - Navigation link highlighting
  - Table headers (User, Role, Status, Last Login, Created, Actions)
  - Filters (search, status dropdown, role dropdown)
  - Invite User button opens modal

- **User CRUD Operations** (3 tests)
  - Create user via modal
  - Edit user (change role)
  - Delete user with confirmation

- **User Actions** (3 tests)
  - Deactivate active user
  - Activate inactive user
  - Resend invitation to pending user

- **User Activity Page** (1 test)
  - Page structure and elements
  - Activity stats display
  - Activity filters

- **Invitation Acceptance** (5 tests)
  - Invalid token shows error
  - Missing token shows error
  - Password form elements present
  - Password strength indicator
  - Password mismatch validation

- **XSS Protection** (4 tests)
  - Malicious username with `<script>` tag escaped
  - Malicious username with `<img>` tag escaped
  - Malicious search input escaped
  - Data attributes properly escaped

- **Console Errors** (3 tests)
  - Users page has no console errors
  - User activity page has no console errors
  - Invitation page has no console errors

**Total:** 24 test cases covering user management end-to-end

## Running Tests

### Run all user management tests
```bash
uvx --from playwright --with playwright pytest tests/e2e/test_user_management.py -v
```

### Run specific test class
```bash
uvx --from playwright --with playwright pytest tests/e2e/test_user_management.py::TestUserCRUD -v
```

### Run specific test
```bash
uvx --from playwright --with playwright pytest tests/e2e/test_user_management.py::TestUserCRUD::test_create_user_via_modal -v
```

### Run with custom password
```bash
ADMIN_PASSWORD=mypassword uvx --from playwright --with playwright pytest tests/e2e/test_user_management.py -v
```

### Show detailed output
```bash
uvx --from playwright --with playwright pytest tests/e2e/test_user_management.py -v -s
```

## Console Error Detection

The tests automatically capture all browser console messages and provide summaries at the end:

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

## Screenshots

### test_admin_ui_python.py
All screenshots are saved to `/tmp/admin_*.png` for debugging:
- `/tmp/admin_login.png` - Login page
- `/tmp/admin_dashboard.png` - Dashboard after login
- `/tmp/admin_events.png` - Events list page
- `/tmp/admin_duplicates.png` - Duplicates page
- `/tmp/admin_api_keys.png` - API keys page
- `/tmp/admin_after_logout.png` - After logout
- `/tmp/admin_error.png` - If test fails

### test_user_management.py
Screenshots are saved to `/tmp/` for debugging:
- `/tmp/user_list_page.png` - Users list page
- `/tmp/user_created.png` - After creating user
- `/tmp/user_activity_page.png` - User activity page
- `/tmp/invitation_invalid_token.png` - Invalid invitation token

## Test Fixtures

The user management tests use pytest fixtures for setup:

- `page` - Playwright page with console error tracking
- `admin_login` - Logs in as admin before test
- `console_errors` - List that accumulates console errors during test
- `browser_context_args` - Browser configuration

## Environment Variables

Both test scripts read `ADMIN_PASSWORD` from environment or use the default from `.env`.

```bash
# Run with custom password
ADMIN_PASSWORD=mypassword uvx --from playwright --with playwright python tests/e2e/test_admin_ui_python.py
ADMIN_PASSWORD=mypassword uvx --from playwright --with playwright pytest tests/e2e/test_user_management.py -v
```

## Integration with OpenCode

This test uses Python Playwright which is compatible with OpenCode's webapp-testing skill and the `with_server.py` helper script.
