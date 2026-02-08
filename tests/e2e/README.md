# E2E Testing

Browser-based end-to-end tests for the Togather admin UI using Python Playwright, plus Go HTTP-level tests using testcontainers.

## Setup

### Prerequisites

```bash
# Install Playwright browsers (first time only)
uvx --from playwright playwright install chromium
```

### Running the Server

```bash
# Start the server
make dev    # live reload
# or
make run    # build and run
```

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `BASE_URL` | `http://localhost:8080` | Server URL to test against |
| `ADMIN_USERNAME` | `admin` | Admin login username |
| `ADMIN_PASSWORD` | `XXKokg60kd8hLXgq` | Admin login password |

The default password matches what `make setup` generates. For `make db-init`, use `ADMIN_PASSWORD=admin123`.

**Always `source .env`** before running tests to load the local password.


## Running Tests

### Makefile Targets

```bash
make e2e              # Run all Python E2E tests
make e2e-pytest       # Run only pytest-based tests
```

### Individual Test Files

```bash
source .env

# pytest-based test (uses pytest-playwright plugin)
uvx --from pytest-playwright --with playwright --with pytest \
  pytest tests/e2e/test_user_management.py -v

# pytest-based tests with self-managed browser
uvx --from playwright --with playwright --with pytest \
  pytest tests/e2e/test_email_validation.py tests/e2e/test_password_strength.py -v

# Standalone script
uvx --from playwright --with playwright \
  python tests/e2e/test_review_queue.py
```

### Selective Running

```bash
# Specific test class
uvx --from playwright --with playwright pytest tests/e2e/test_user_management.py::TestUserCRUD -v

# Specific test method
uvx --from playwright --with playwright pytest tests/e2e/test_user_management.py::TestUserCRUD::test_create_user_via_modal -v

# Verbose output (show prints)
uvx --from playwright --with playwright pytest tests/e2e/test_user_management.py -v -s
```

### Against Staging

```bash
BASE_URL=https://staging.toronto.togather.foundation \
ADMIN_PASSWORD=<staging-password> \
uvx --from playwright --with playwright pytest tests/e2e/test_user_management.py -v
```

### Go HTTP Tests

These don't require a running server or browser — they spin up their own PostgreSQL via testcontainers:

```bash
go test -v ./tests/e2e/...
```


## Test File Inventory

### Python Playwright Tests (browser-based)

| File | Framework | Tests | Coverage |
|------|-----------|-------|----------|
| `test_user_management.py` | pytest-playwright | 24 | User CRUD, invitations, XSS, console errors |
| `test_email_validation.py` | pytest + self-managed | 5 | Email validation in invite modal |
| `test_password_strength.py` | pytest + self-managed | 11 | Password strength indicator |
| `test_modal_cleanup.py` | pytest + self-managed | 2 | Bootstrap modal backdrop/scroll cleanup |
| `test_review_queue.py` | standalone script | 12 | Review queue: filters, expand/collapse, actions |
| `test_keyboard_accessibility.py` | standalone script | 1 | Keyboard navigation, focus management |
| `test_admin_ui_python.py` | standalone script | ~10 | Login, dashboard, navigation, theme toggle |
| `admin_ui_playwright.py` | standalone script | ~10 | Login, dashboard, navigation (older) |
| `test_admin_ui_live.py` | standalone script | ~8 | Login, dashboard (oldest, uses `time.sleep`) |

### Go HTTP Tests (no browser)

| File | Coverage |
|------|----------|
| `admin_ui_test.go` | Admin routes: login, dashboard, events, API keys, static assets |
| `admin_login_form_test.go` | Login form: static resources, JS integration, error handling |


## uvx Invocation Patterns

There are two distinct invocation patterns. **Do not mix them** in the same `uvx` session — it causes async event loop conflicts.

### pytest-playwright (provides `page` fixture)

Used by `test_user_management.py`. The `page` fixture is injected by the `pytest-playwright` plugin:

```bash
uvx --from pytest-playwright --with playwright --with pytest pytest <file> -v
```

### Plain playwright (self-managed browser)

Used by `test_email_validation.py`, `test_password_strength.py`, `test_modal_cleanup.py`. These create their own browser via `sync_playwright()` in fixtures:

```bash
uvx --from playwright --with playwright --with pytest pytest <file> -v
```

### Standalone scripts

Used by `test_review_queue.py`, `test_keyboard_accessibility.py`, `test_admin_ui_python.py`, etc. These manage `sync_playwright()` directly and use `python` instead of `pytest`:

```bash
uvx --from playwright --with playwright python <file>
```

The `make e2e` target handles the split automatically.


## Test Fixtures

E2E tests that need data use Go-based fixture generation piped through the real ingestion pipeline. All fixture generators live in `tests/testdata/fixtures.go` and are exposed via the `server generate` CLI command.

### Quick Start

```bash
# Build the server binary
make build

# Generate and ingest normal test events
source .env
./bin/togather-server generate fixtures.json --count 10
./bin/togather-server ingest fixtures.json

# Review queue fixtures (one-step helper script)
tests/e2e/setup_fixtures.sh 5

# Clean up review queue test data
tests/e2e/cleanup_fixtures.sh
```

### CLI: `server generate`

```bash
# Normal random events (realistic Toronto venues, organizers, sources)
./bin/togather-server generate fixtures.json --count 10

# Review queue events (data quality issues that trigger review)
./bin/togather-server generate fixtures.json --count 5 --review-queue

# Reproducible output
./bin/togather-server generate fixtures.json --count 10 --seed 42

# Print to stdout (for piping)
./bin/togather-server generate --count 3
```

Output is a JSON batch (`{"events": [...]}`) compatible with `server ingest`.

### Available Fixture Types

The `Generator` in `tests/testdata/fixtures.go` provides:

**Standard events:**

| Method | Description |
|--------|-------------|
| `RandomEventInput()` | Full event with venue, organizer, source, image, keywords |
| `MinimalEventInput()` | Only required fields (name, startDate, venue name) |
| `VirtualEventInput()` | Online-only event with virtual location |
| `HybridEventInput()` | Both physical venue and virtual location |
| `EventInputWithOccurrences(n)` | Recurring event with `n` weekly occurrences |
| `EventInputNeedsReview()` | Sparse event missing description/image (triggers review) |
| `EventInputFarFuture()` | Event >2 years out (triggers review) |
| `DuplicateCandidates()` | Two events with same name/venue/time (dedup testing) |
| `BatchEventInputs(n)` | Batch of `n` random events |

**Review queue fixtures** (used with `--review-queue` flag):

| Method | Name Pattern | Issue |
|--------|-------------|-------|
| `EventInputReversedDates()` | `"... (Late Night)"` | End before start (11pm-2am same-day) |
| `EventInputMissingVenue()` | `"... (Online)"` | Virtual event, no physical location |
| `EventInputLikelyDuplicate()` | `"Weekly Community Meetup..."` | Duplication pattern |
| `EventInputMultipleWarnings()` | varies | Missing description/image/endDate, date-only, partial location |
| `BatchReviewQueueInputs(n)` | mixed | Rotates through all scenarios |

### Using Fixtures in Go Tests

```go
import "github.com/Togather-Foundation/server/tests/testdata"

gen := testdata.NewDeterministicGenerator() // fixed seed
gen := testdata.NewGenerator(42)            // custom seed

event := gen.RandomEventInput()
batch := gen.BatchEventInputs(10)
reviewBatch := gen.BatchReviewQueueInputs(5)
original, duplicate := gen.DuplicateCandidates()
```

### Using Fixtures in Python E2E Tests

Call the Go CLI via subprocess. The review queue tests have helper scripts:

```python
import subprocess
from pathlib import Path

def setup_fixtures(count=5):
    script = Path(__file__).parent / "setup_fixtures.sh"
    result = subprocess.run(
        [str(script), str(count)],
        cwd=script.parent.parent.parent,
        capture_output=True, text=True, timeout=30,
    )
    if result.returncode != 0:
        raise RuntimeError(f"Fixture setup failed: {result.stderr}")

def cleanup_fixtures():
    script = Path(__file__).parent / "cleanup_fixtures.sh"
    subprocess.run(
        [str(script)],
        cwd=script.parent.parent.parent,
        capture_output=True, text=True, timeout=30,
    )
```

See `test_review_queue.py` for a complete working example.


## Writing New Tests

1. **Use env vars** for `BASE_URL`, `ADMIN_USERNAME`, `ADMIN_PASSWORD` (with standard defaults)
2. **Prefer pytest** with classes for new tests (better isolation, selective running)
3. **Use `expect()`** assertions from Playwright, not bare `assert` on locator states
4. **Wait properly** — `wait_for_load_state("networkidle")`, `wait_for_selector()`, `wait_for_url()`. Avoid `time.sleep()`.
5. **Track console errors** — capture `page.on("console", ...)` and assert no errors
6. **Screenshot on failure** — save to `/tmp/<test_name>_failure.png`
7. **Handle empty states** — admin pages may have no data; tests should pass either way
8. **Test auth redirect** — verify unauthenticated access redirects to login


## Debugging

- **Screenshots**: Saved to `/tmp/` — check test code for exact filenames (e.g., `/tmp/admin_dashboard.png`, `/tmp/user_list_page.png`)
- **Console errors**: Most test files capture and report browser console errors automatically
- **Headed mode**: Modify the test to use `headless=False`:
  ```python
  browser = p.chromium.launch(headless=False, slow_mo=500)
  ```
- **Playwright inspector**: `PWDEBUG=1` before the command opens the inspector
- **Verbose pytest**: Add `-s` flag to see print output


## Known Issues

- No shared `conftest.py` — each pytest file duplicates fixture definitions
- Screenshot paths are all `/tmp/` — no organized output directory
- `test_admin_ui_live.py` uses `time.sleep()` instead of proper Playwright waits
- Some standalone scripts overlap in coverage
- `test_user_management.py` has pre-existing failures: invite modal user creation, duplicate `#user-email` DOM id, CSP violation console errors
- The unauthenticated redirect test in `test_review_queue.py` may fail if the server only checks auth at the API level
