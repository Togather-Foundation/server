# E2E / Playwright Testing

See `README.md` for full docs (test inventory, fixture types, debugging).

## Commands

```bash
make e2e                    # all Python e2e tests (handles uvx split automatically)
make e2e-pytest             # pytest-based only

# First time
uvx --from playwright playwright install chromium
```

Always `source .env` before running tests directly.

## uvx Invocation Rules (non-interchangeable — causes async loop conflicts)

1. **pytest-playwright** (`test_user_management.py`): `uvx --from pytest-playwright --with playwright --with pytest pytest <file> -v`
2. **Plain playwright pytest** (`test_email_validation.py`, `test_password_strength.py`, `test_modal_cleanup.py`): `uvx --from playwright --with playwright --with pytest pytest <file> -v`
3. **Standalone scripts** (`test_review_queue.py`, etc.): `uvx --from playwright --with playwright python <file>`

## Fixtures

```bash
tests/e2e/setup_fixtures.sh 5     # review queue fixtures
tests/e2e/cleanup_fixtures.sh

# Normal events
make build && source .env
./bin/togather-server generate fixtures.json --count 10
./bin/togather-server ingest fixtures.json
```

## Writing New Tests

- Use env vars: `BASE_URL`, `ADMIN_USERNAME`, `ADMIN_PASSWORD` (with standard defaults)
- Prefer pytest classes for new tests
- Use `expect()` assertions — not bare `assert` on locator states
- Wait: `wait_for_load_state("networkidle")` / `wait_for_selector()` — no `time.sleep()`
- Capture console errors: `page.on("console", ...)`; assert none at end
- Screenshot on failure: save to `/tmp/<test_name>_failure.png`

### Pytest pattern

```python
import os, pytest
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

- No shared `conftest.py` — fixture definitions duplicated across files
- `test_admin_ui_live.py` uses `time.sleep()` (avoid as model)
- `test_user_management.py`: invite modal, duplicate `#user-email` DOM id, CSP violations
