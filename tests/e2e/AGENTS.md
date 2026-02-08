# E2E / Playwright Testing — Agent Instructions

This directory contains browser-based end-to-end tests for the admin UI using Python Playwright, plus Go-based HTTP-level tests using testcontainers.

## Quick Reference

```bash
# Prerequisites (first time only)
uvx --from playwright playwright install chromium

# Run a specific test file (standalone script pattern)
source .env && uvx --from playwright --with playwright python tests/e2e/test_review_queue.py

# Run a pytest-based test file
source .env && uvx --from playwright --with playwright pytest tests/e2e/test_user_management.py -v

# Run against staging
source .env && BASE_URL=https://staging.example.com ADMIN_PASSWORD=<password> \
  uvx --from playwright --with playwright pytest tests/e2e/test_user_management.py -v

# Makefile shortcuts
make e2e                    # Run all Python e2e tests
make e2e-pytest             # Run only pytest-based e2e tests
```


## Configuration

All test files should use these environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `BASE_URL` | `http://localhost:8080` | Server URL to test against |
| `ADMIN_USERNAME` | `admin` | Admin login username |
| `ADMIN_PASSWORD` | `XXKokg60kd8hLXgq` | Admin login password |

Read from environment: `os.getenv("BASE_URL", "http://localhost:8080")`

The password default `XXKokg60kd8hLXgq` matches what `make setup` generates. For local dev with `make db-init`, the password is `admin123` — override via `ADMIN_PASSWORD=admin123`.

**Always `source .env`** before running tests to pick up the local password.


## Test File Inventory

### Python Playwright Tests (browser-based)

| File | Framework | Tests | What it covers |
|------|-----------|-------|----------------|
| `test_user_management.py` | pytest + classes | 24 | User CRUD, invitations, XSS protection, console errors |
| `test_email_validation.py` | pytest + classes | 5 | Email validation in invite modal |
| `test_password_strength.py` | pytest + classes | 11 | Password strength indicator |
| `test_modal_cleanup.py` | pytest + classes | 2 | Bootstrap modal backdrop/scroll cleanup |
| `test_review_queue.py` | standalone script | 12 | Review queue UI: filters, expand/collapse, actions |
| `test_keyboard_accessibility.py` | standalone script | 1 (multi-check) | Keyboard navigation, focus management |
| `test_admin_ui_python.py` | standalone script | ~10 | Login, dashboard, navigation, theme toggle |
| `admin_ui_playwright.py` | standalone script | ~10 | Login, dashboard, navigation (older version) |
| `test_admin_ui_live.py` | standalone script | ~8 | Login, dashboard (oldest, uses time.sleep) |

### Go HTTP Tests (no browser)

| File | What it covers |
|------|----------------|
| `admin_ui_test.go` | Admin routes: login, dashboard, events, API keys, static assets |
| `admin_login_form_test.go` | Login form: static resources, JS integration, error handling |

Go tests use `testcontainers-go` for a real PostgreSQL instance and `httptest.Server`. Run with `go test ./tests/e2e/...`.


## Two Framework Patterns

### 1. Pytest with Classes (preferred for new tests)

Best example: `test_user_management.py`

```python
import os
import pytest
from playwright.sync_api import Page, expect

BASE_URL = os.getenv("BASE_URL", "http://localhost:8080")
ADMIN_USERNAME = os.getenv("ADMIN_USERNAME", "admin")
ADMIN_PASSWORD = os.getenv("ADMIN_PASSWORD", "XXKokg60kd8hLXgq")


@pytest.fixture(scope="session")
def browser_context_args(browser_context_args):
    return {
        **browser_context_args,
        "viewport": {"width": 1920, "height": 1080},
        "ignore_https_errors": True,
    }


@pytest.fixture(scope="function")
def console_errors():
    errors = []
    return errors


@pytest.fixture(scope="function")
def admin_login(page: Page, console_errors):
    def handle_console(msg):
        if msg.type == "error":
            console_errors.append(msg.text)
    page.on("console", handle_console)

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

    def test_no_console_errors(self, admin_login, console_errors):
        page = admin_login
        page.goto(f"{BASE_URL}/admin/my-page")
        page.wait_for_load_state("networkidle")
        assert len(console_errors) == 0, f"Console errors: {console_errors}"
```

Run with: `uvx --from playwright --with playwright pytest tests/e2e/test_my_feature.py -v`

### 2. Standalone Script (existing pattern, acceptable)

```python
import sys
import os
from playwright.sync_api import sync_playwright, expect

BASE_URL = os.getenv("BASE_URL", "http://localhost:8080")
ADMIN_USERNAME = os.getenv("ADMIN_USERNAME", "admin")
ADMIN_PASSWORD = os.getenv("ADMIN_PASSWORD", "XXKokg60kd8hLXgq")


def login(page):
    page.goto(f"{BASE_URL}/admin/login")
    page.fill("#username", ADMIN_USERNAME)
    page.fill("#password", ADMIN_PASSWORD)
    with page.expect_navigation(timeout=10000):
        page.click('button[type="submit"]')
    page.wait_for_url(f"{BASE_URL}/admin/dashboard", timeout=5000)


def test_my_feature(page):
    login(page)
    page.goto(f"{BASE_URL}/admin/my-page")
    page.wait_for_load_state("networkidle")
    expect(page.locator("h2")).to_contain_text("My Page")


def run_all_tests():
    with sync_playwright() as p:
        browser = p.chromium.launch(headless=True)
        context = browser.new_context()
        page = context.new_page()
        try:
            test_my_feature(page)
            print("All tests passed!")
            return 0
        except Exception as e:
            print(f"Test failed: {e}")
            page.screenshot(path="/tmp/test_failure.png", full_page=True)
            return 1
        finally:
            browser.close()


if __name__ == "__main__":
    sys.exit(run_all_tests())
```

Run with: `uvx --from playwright --with playwright python tests/e2e/test_my_feature.py`


## Writing New Tests — Checklist

1. **Use env vars** for `BASE_URL`, `ADMIN_USERNAME`, `ADMIN_PASSWORD` (with standard defaults above)
2. **Prefer pytest** with classes for new tests (better isolation, selective running, CI integration)
3. **Use `expect()`** assertions from Playwright (not bare `assert` on locator states)
4. **Wait properly** — use `wait_for_load_state("networkidle")`, `wait_for_selector()`, `wait_for_url()`. Avoid `time.sleep()` and `wait_for_timeout()` except as last resort.
5. **Track console errors** — capture `page.on("console", ...)` and assert no errors
6. **Screenshot on failure** — save to `/tmp/<test_name>_failure.png`
7. **Handle empty states** — admin pages may have no data; tests should pass either way
8. **Test auth redirect** — verify unauthenticated access redirects to login


## Test Fixtures (Go-based)

E2E tests that need data in the database use Go-based fixture generation piped through the real ingestion pipeline. All fixtures live in `tests/testdata/fixtures.go` and are exposed via the `server generate` CLI command.

### Quick start

```bash
# Build the server binary first
make build

# Generate and ingest normal test events
source .env
./bin/togather-server generate fixtures.json --count 10
./bin/togather-server ingest fixtures.json

# Or use the review queue helper script (generates + ingests in one step)
tests/e2e/setup_fixtures.sh 5

# Clean up review queue test data afterward
tests/e2e/cleanup_fixtures.sh
```

### CLI: `server generate`

```bash
# Normal random events (full fields, Toronto venues)
./bin/togather-server generate fixtures.json --count 10

# Review queue events (data quality issues that trigger review)
./bin/togather-server generate fixtures.json --count 5 --review-queue

# Reproducible output with seed
./bin/togather-server generate fixtures.json --count 10 --seed 42

# Print to stdout (for piping)
./bin/togather-server generate --count 3
```

Generated events use realistic Toronto venues, organizers, and source metadata. Output is a JSON batch (`{"events": [...]}`) compatible with `server ingest`.

### Available fixture types

The `Generator` in `tests/testdata/fixtures.go` provides these methods:

| Method | Description |
|--------|-------------|
| `RandomEventInput()` | Full event with venue, organizer, source, image, keywords |
| `MinimalEventInput()` | Only required fields (name, startDate, venue name) |
| `VirtualEventInput()` | Online-only event with virtual location, no physical venue |
| `HybridEventInput()` | Both physical venue and virtual location |
| `EventInputWithOccurrences(n)` | Recurring event with `n` weekly occurrences |
| `EventInputNeedsReview()` | Sparse event missing description/image (triggers review) |
| `EventInputFarFuture()` | Event >2 years out (triggers review) |
| `DuplicateCandidates()` | Two events with same name/venue/time (dedup testing) |
| `BatchEventInputs(n)` | Batch of `n` random events |

**Review queue fixtures** (used with `--review-queue` flag):

| Method | Name Pattern | What's wrong |
|--------|-------------|-------------|
| `EventInputReversedDates()` | `"... (Late Night)"` | End time before start (11pm-2am same-day bug) |
| `EventInputMissingVenue()` | `"... (Online)"` | Virtual event with no physical location field |
| `EventInputLikelyDuplicate()` | `"Weekly Community Meetup..."` | Pattern suggesting duplication |
| `EventInputMultipleWarnings()` | varies | Missing description, image, endDate; date-only format; partial location |
| `BatchReviewQueueInputs(n)` | mixed | Rotates through all four scenarios |

### Using fixtures in Go tests

```go
import "github.com/Togather-Foundation/server/tests/testdata"

gen := testdata.NewDeterministicGenerator() // fixed seed for reproducibility
// or
gen := testdata.NewGenerator(42)            // custom seed

// Standard events
event    := gen.RandomEventInput()
minimal  := gen.MinimalEventInput()
virtual  := gen.VirtualEventInput()
hybrid   := gen.HybridEventInput()
withOccs := gen.EventInputWithOccurrences(3)
batch    := gen.BatchEventInputs(10)

// Events that trigger review
needsReview := gen.EventInputNeedsReview()
farFuture   := gen.EventInputFarFuture()
reversed    := gen.EventInputReversedDates()
noVenue     := gen.EventInputMissingVenue()
maybeDupe   := gen.EventInputLikelyDuplicate()
multiWarn   := gen.EventInputMultipleWarnings()
reviewBatch := gen.BatchReviewQueueInputs(10)

// Dedup testing
original, duplicate := gen.DuplicateCandidates()
```

### Using fixtures in Python E2E tests

For E2E tests that need database data, generate and ingest via the CLI. The review queue tests have helper scripts:

```python
import subprocess
from pathlib import Path

def setup_fixtures(count=5):
    """Generate and ingest test events via Go CLI."""
    script = Path(__file__).parent / "setup_fixtures.sh"
    result = subprocess.run(
        [str(script), str(count)],
        cwd=script.parent.parent.parent,  # project root
        capture_output=True, text=True, timeout=30,
    )
    if result.returncode != 0:
        raise RuntimeError(f"Fixture setup failed: {result.stderr}")

def cleanup_fixtures():
    """Remove test data created by setup_fixtures."""
    script = Path(__file__).parent / "cleanup_fixtures.sh"
    subprocess.run(
        [str(script)],
        cwd=script.parent.parent.parent,
        capture_output=True, text=True, timeout=30,
    )
```

Call `setup_fixtures()` at the start of your test run and `cleanup_fixtures()` in a `finally` block. See `test_review_queue.py` for a complete example.

For tests that need normal events (not review queue), call the generate and ingest commands directly:

```python
import subprocess, tempfile

def ingest_test_events(count=10, seed=42):
    """Generate and ingest normal test events."""
    with tempfile.NamedTemporaryFile(suffix=".json", delete=False) as f:
        tmpfile = f.name
    
    project_root = Path(__file__).parent.parent.parent
    server_bin = project_root / "bin" / "togather-server"
    
    # Generate
    subprocess.run(
        [str(server_bin), "generate", tmpfile, "--count", str(count), "--seed", str(seed)],
        cwd=str(project_root), check=True, capture_output=True, text=True,
    )
    # Ingest
    subprocess.run(
        [str(server_bin), "ingest", tmpfile],
        cwd=str(project_root), check=True, capture_output=True, text=True,
    )
```


## Running Tests

### Local (server must be running)

```bash
# Start server first
make dev  # or make run

# In another terminal:
source .env  # loads ADMIN_PASSWORD

# All e2e tests (Makefile target)
make e2e

# Specific pytest file
uvx --from playwright --with playwright pytest tests/e2e/test_user_management.py -v

# Specific test class
uvx --from playwright --with playwright pytest tests/e2e/test_user_management.py::TestUserCRUD -v

# Specific test method
uvx --from playwright --with playwright pytest tests/e2e/test_user_management.py::TestUserCRUD::test_create_user_via_modal -v

# Standalone script
uvx --from playwright --with playwright python tests/e2e/test_review_queue.py

# With verbose pytest output (show prints)
uvx --from playwright --with playwright pytest tests/e2e/test_user_management.py -v -s
```

### Against Staging

```bash
BASE_URL=https://staging.toronto.togather.foundation \
ADMIN_PASSWORD=<staging-password> \
uvx --from playwright --with playwright pytest tests/e2e/test_user_management.py -v
```

### Go HTTP Tests (no browser needed)

```bash
go test -v ./tests/e2e/...
# or
make test-ci  # includes Go e2e tests
```


## Debugging

- **Screenshots**: Saved to `/tmp/` — check test code for exact filenames
- **Console errors**: Most test files capture and report browser console errors
- **Headed mode**: For interactive debugging, modify the test to use `headless=False`:
  ```python
  browser = p.chromium.launch(headless=False, slow_mo=500)
  ```
- **Playwright inspector**: Add `PWDEBUG=1` before the command to open the inspector


## uvx Invocation Patterns

There are two distinct `uvx` invocation patterns depending on how the test manages the browser:

### pytest-playwright (provides `page` fixture)

`test_user_management.py` uses the `page` fixture injected by the `pytest-playwright` plugin. This requires installing pytest-playwright as the primary package:

```bash
uvx --from pytest-playwright --with playwright --with pytest pytest tests/e2e/test_user_management.py -v
```

### Plain playwright (self-managed browser)

`test_email_validation.py`, `test_password_strength.py`, and `test_modal_cleanup.py` create their own browser via `sync_playwright()` in fixtures. These **conflict** with pytest-playwright's async event loop if run in the same session. Use plain playwright:

```bash
uvx --from playwright --with playwright --with pytest pytest tests/e2e/test_email_validation.py tests/e2e/test_password_strength.py tests/e2e/test_modal_cleanup.py -v
```

### Standalone scripts

Scripts like `test_review_queue.py` use `sync_playwright()` directly and are invoked with `python` instead of `pytest`:

```bash
uvx --from playwright --with playwright python tests/e2e/test_review_queue.py
```

**Key rule**: Never mix pytest-playwright (`page` fixture) files and self-managed browser files in the same `uvx` invocation. The Makefile `e2e` target handles this split automatically.


## Known Issues

- No shared `conftest.py` — each pytest file duplicates fixture definitions
- Screenshot paths are all `/tmp/` — no organized output directory
- `test_admin_ui_live.py` uses `time.sleep()` instead of proper Playwright waits
- Some standalone scripts overlap in coverage with each other
- The unauthenticated redirect test in `test_review_queue.py` may fail if the server doesn't redirect HTML pages to login (API-only auth check)
- `test_user_management.py` has pre-existing failures: user creation via invite modal doesn't work, duplicate `#user-email` DOM id, CSP violation console errors
