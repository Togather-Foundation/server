# E2E / Playwright Testing — Agent Instructions

> See `README.md` for comprehensive documentation (test inventory, fixture types, debugging tips).

## Quick Reference

```bash
# Prerequisites (first time only)
uvx --from playwright playwright install chromium

# Standalone script
source .env && uvx --from playwright --with playwright python tests/e2e/test_review_queue.py

# pytest-based test
source .env && uvx --from playwright --with playwright pytest tests/e2e/test_user_management.py -v

# Makefile shortcuts
make e2e                    # Run all Python e2e tests
make e2e-pytest             # Run only pytest-based e2e tests
```


## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `BASE_URL` | `http://localhost:8080` | Server URL to test against |
| `ADMIN_USERNAME` | `admin` | Admin login username |
| `ADMIN_PASSWORD` | `XXKokg60kd8hLXgq` | Admin login password |

**Always `source .env`** before running tests. For `make db-init`, use `ADMIN_PASSWORD=admin123`.


## uvx Invocation Rules

Never mix these in the same `uvx` session — causes async event loop conflicts:

1. **pytest-playwright** (`test_user_management.py`): `uvx --from pytest-playwright --with playwright --with pytest pytest <file> -v`
2. **Plain playwright pytest** (`test_email_validation.py`, `test_password_strength.py`, `test_modal_cleanup.py`): `uvx --from playwright --with playwright --with pytest pytest <file> -v`
3. **Standalone scripts** (`test_review_queue.py`, etc.): `uvx --from playwright --with playwright python <file>`

The `make e2e` target handles the split automatically.


## Test Fixtures

Fixtures use Go-based generation piped through the real ingestion pipeline. Source: `tests/testdata/fixtures.go`, exposed via `server generate` CLI.

```bash
# Review queue fixtures (one-step)
tests/e2e/setup_fixtures.sh 5

# Cleanup
tests/e2e/cleanup_fixtures.sh

# Normal events
make build && source .env
./bin/togather-server generate fixtures.json --count 10
./bin/togather-server ingest fixtures.json
```


## Writing New Tests — Checklist

1. Use env vars for `BASE_URL`, `ADMIN_USERNAME`, `ADMIN_PASSWORD` (with standard defaults)
2. Prefer pytest with classes for new tests
3. Use `expect()` assertions from Playwright, not bare `assert` on locator states
4. Wait properly — `wait_for_load_state("networkidle")`, `wait_for_selector()`, `wait_for_url()`. Avoid `time.sleep()`.
5. Track console errors — capture `page.on("console", ...)` and assert no errors
6. Screenshot on failure — save to `/tmp/<test_name>_failure.png`
7. Handle empty states — admin pages may have no data; tests should pass either way
8. Test auth redirect — verify unauthenticated access redirects to login

### Pytest pattern (preferred)

```python
import os
import pytest
from playwright.sync_api import Page, expect

BASE_URL = os.getenv("BASE_URL", "http://localhost:8080")
ADMIN_USERNAME = os.getenv("ADMIN_USERNAME", "admin")
ADMIN_PASSWORD = os.getenv("ADMIN_PASSWORD", "XXKokg60kd8hLXgq")

@pytest.fixture(scope="session")
def browser_context_args(browser_context_args):
    return {**browser_context_args, "viewport": {"width": 1920, "height": 1080}, "ignore_https_errors": True}

@pytest.fixture(scope="function")
def admin_login(page: Page):
    page.goto(f"{BASE_URL}/admin/login")
    page.fill("#username", ADMIN_USERNAME)
    page.fill("#password", ADMIN_PASSWORD)
    page.click('button[type="submit"]')
    page.wait_for_url(f"{BASE_URL}/admin/dashboard", timeout=5000)
    return page

class TestMyFeature:
    def test_page_loads(self, admin_login):
        page = admin_login
        page.goto(f"{BASE_URL}/admin/my-page")
        page.wait_for_load_state("networkidle")
        expect(page.locator("h2")).to_contain_text("My Page")
```


## Known Issues

- No shared `conftest.py` — each pytest file duplicates fixture definitions
- `test_admin_ui_live.py` uses `time.sleep()` instead of proper waits
- `test_user_management.py` has pre-existing failures: invite modal, duplicate `#user-email` DOM id, CSP violations
- The redirect test in `test_review_queue.py` may fail if auth is API-only
