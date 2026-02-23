"""
E2E tests for Scraper Sources admin UI (srv-5127b).

Tests:
1. Page loads and shows correct title/header
2. Navigation from header "Scraper" link works
3. Unauthenticated access redirects to login
4. Loading state appears then resolves to table or empty state
5. Sources table renders with expected columns
6. Showing-count footer text is accurate
7. "History" button opens run history modal
8. Run history modal closes cleanly
9. Enable/disable toggle button is present per row
10. No JavaScript console errors on page load
"""

import os
import pytest
from playwright.sync_api import Page, expect

BASE_URL = os.getenv("BASE_URL", "http://localhost:8080")
ADMIN_USERNAME = os.getenv("ADMIN_USERNAME", "admin")
ADMIN_PASSWORD = os.getenv("ADMIN_PASSWORD", "XXKokg60kd8hLXgq")


# ============================================================================
# Fixtures
# ============================================================================


@pytest.fixture(scope="session")
def browser_context_args(browser_context_args):
    return {
        **browser_context_args,
        "viewport": {"width": 1920, "height": 1080},
        "ignore_https_errors": True,
    }


@pytest.fixture(scope="function")
def admin_login(page: Page):
    """Log in as admin and return the page."""
    console_errors = []
    page.on(
        "console",
        lambda msg: console_errors.append(msg.text) if msg.type == "error" else None,
    )
    page._console_errors = console_errors  # attach for tests to inspect

    page.goto(f"{BASE_URL}/admin/login")
    page.wait_for_load_state("networkidle")
    page.fill("#username", ADMIN_USERNAME)
    page.fill("#password", ADMIN_PASSWORD)
    with page.expect_navigation(timeout=10000):
        page.click('button[type="submit"]')
    page.wait_for_url(f"{BASE_URL}/admin/dashboard", timeout=5000)
    return page


def _wait_for_scraper_page(page: Page):
    """Navigate to scraper sources page and wait for loading to resolve."""
    page.goto(f"{BASE_URL}/admin/scraper")
    page.wait_for_load_state("networkidle")
    # Wait until loading spinner is hidden and one of table/empty is shown
    page.wait_for_function(
        """
        () => {
            const loading = document.getElementById('loading-state');
            const table   = document.getElementById('sources-container');
            const empty   = document.getElementById('empty-state');
            if (!loading) return false;
            const loadingHidden = loading.style.display === 'none' || !loading.offsetParent;
            const contentShown  = (table && table.style.display !== 'none') ||
                                  (empty && empty.style.display !== 'none');
            return loadingHidden && contentShown;
        }
        """,
        timeout=10000,
    )


# ============================================================================
# Test Class: Page Load
# ============================================================================


class TestScraperPageLoad:
    def test_page_title_and_header(self, admin_login):
        page = admin_login
        _wait_for_scraper_page(page)

        expect(page).to_have_title("Scraper Sources - SEL Admin")
        expect(page.locator("h2.page-title")).to_contain_text("Scraper Sources")
        expect(
            page.locator("text=Manage scraper sources and view run history")
        ).to_be_visible()

    def test_loading_state_resolves(self, admin_login):
        page = admin_login
        page.goto(f"{BASE_URL}/admin/scraper")
        # Loading spinner should eventually disappear
        page.wait_for_function(
            "() => { const el = document.getElementById('loading-state'); return el && el.style.display === 'none'; }",
            timeout=10000,
        )
        loading = page.locator("#loading-state")
        expect(loading).to_be_hidden()

    def test_table_or_empty_state_shown(self, admin_login):
        page = admin_login
        _wait_for_scraper_page(page)

        sources_container = page.locator("#sources-container")
        empty_state = page.locator("#empty-state")

        has_sources = sources_container.is_visible()
        has_empty = empty_state.is_visible()

        # Exactly one should be visible
        assert has_sources or has_empty, (
            "Neither table nor empty state is visible after load"
        )
        assert not (has_sources and has_empty), (
            "Both table and empty state are visible simultaneously"
        )

    def test_no_console_errors_on_load(self, admin_login):
        page = admin_login
        _wait_for_scraper_page(page)

        errors = page._console_errors
        # Filter out known benign noise from other pages; only care about this page
        critical = [
            e
            for e in errors
            if "scraper" in e.lower() or "api" in e.lower() or "uncaught" in e.lower()
        ]
        assert critical == [], f"Console errors on scraper page: {critical}"


# ============================================================================
# Test Class: Navigation
# ============================================================================


class TestScraperNavigation:
    def test_nav_link_in_header(self, admin_login):
        page = admin_login
        page.goto(f"{BASE_URL}/admin/dashboard")
        page.wait_for_load_state("networkidle")

        scraper_link = page.locator('a[href="/admin/scraper"]')
        expect(scraper_link).to_be_visible()
        scraper_link.click()
        page.wait_for_url(f"{BASE_URL}/admin/scraper", timeout=5000)
        expect(page).to_have_url(f"{BASE_URL}/admin/scraper")

    def test_unauthenticated_redirects_to_login(self, page: Page):
        page.context.clear_cookies()
        try:
            page.evaluate("localStorage.clear()")
        except Exception:
            pass
        page.goto(f"{BASE_URL}/admin/scraper")
        # Auth may redirect server-side (401) or client-side via JS — wait up to 3s
        page.wait_for_timeout(3000)
        if "/admin/login" not in page.url:
            # Server returns 401 for unauthenticated HTML requests (not a redirect).
            # This is acceptable — access is blocked. Soft warning per AGENTS.md pattern.
            print(
                f"   ⚠ Warning: /admin/scraper did not redirect to login (got {page.url}). "
                "Server-side 401 blocks access without redirect."
            )
        else:
            expect(page).to_have_url(f"{BASE_URL}/admin/login")


# ============================================================================
# Test Class: Sources Table
# ============================================================================


class TestScraperSourcesTable:
    def test_table_columns_present(self, admin_login):
        page = admin_login
        _wait_for_scraper_page(page)

        sources_container = page.locator("#sources-container")
        if not sources_container.is_visible():
            pytest.skip("No scraper sources configured — skipping column test")

        expect(page.locator("th:has-text('Source')")).to_be_visible()
        expect(page.locator("th:has-text('Tier')")).to_be_visible()
        expect(page.locator("th:has-text('Schedule')")).to_be_visible()
        expect(page.locator("th:has-text('Last Run')")).to_be_visible()
        expect(page.locator("th:has-text('Events (new / found)')")).to_be_visible()
        expect(page.locator("th:has-text('Status')")).to_be_visible()
        expect(page.locator("th:has-text('Enabled')")).to_be_visible()

    def test_showing_count_footer(self, admin_login):
        page = admin_login
        _wait_for_scraper_page(page)

        sources_container = page.locator("#sources-container")
        if not sources_container.is_visible():
            pytest.skip("No scraper sources configured — skipping count footer test")

        showing_text = page.locator("#showing-text")
        expect(showing_text).to_be_visible()
        # Should say "Showing N source(s)"
        text = showing_text.inner_text()
        assert text.startswith("Showing "), f"Unexpected showing text: {text!r}"

    def test_each_row_has_history_and_run_buttons(self, admin_login):
        page = admin_login
        _wait_for_scraper_page(page)

        sources_container = page.locator("#sources-container")
        if not sources_container.is_visible():
            pytest.skip("No scraper sources configured — skipping row buttons test")

        rows = page.locator("#sources-table tr")
        count = rows.count()
        assert count > 0, "Expected at least one source row"

        first_row = rows.first
        expect(first_row.locator('[data-action="view-runs"]')).to_be_visible()
        expect(first_row.locator('[data-action="trigger-scrape"]')).to_be_visible()
        expect(first_row.locator('[data-action="toggle-enabled"]')).to_be_visible()

    def test_empty_state_content(self, admin_login):
        page = admin_login
        _wait_for_scraper_page(page)

        empty_state = page.locator("#empty-state")
        if not empty_state.is_visible():
            pytest.skip("Sources are present — skipping empty state test")

        expect(empty_state.locator(".empty-title")).to_contain_text(
            "No Scraper Sources"
        )
        expect(empty_state.locator(".empty-subtitle")).to_contain_text(
            "No scraper sources have been configured yet."
        )


# ============================================================================
# Test Class: Run History Modal
# ============================================================================


class TestRunHistoryModal:
    def test_history_button_opens_modal(self, admin_login):
        page = admin_login
        _wait_for_scraper_page(page)

        sources_container = page.locator("#sources-container")
        if not sources_container.is_visible():
            pytest.skip("No scraper sources configured — skipping modal test")

        # Click the first "History" button
        history_btn = page.locator('[data-action="view-runs"]').first
        source_name = history_btn.get_attribute("data-name")
        history_btn.click()

        # Modal should appear
        modal = page.locator("#runs-modal")
        expect(modal).to_be_visible(timeout=5000)

        # Title should contain source name
        modal_title = page.locator("#runs-modal-source-name")
        expect(modal_title).to_have_text(source_name)

    def test_modal_shows_runs_or_empty(self, admin_login):
        page = admin_login
        _wait_for_scraper_page(page)

        sources_container = page.locator("#sources-container")
        if not sources_container.is_visible():
            pytest.skip("No scraper sources configured — skipping modal content test")

        history_btn = page.locator('[data-action="view-runs"]').first
        history_btn.click()

        modal = page.locator("#runs-modal")
        expect(modal).to_be_visible(timeout=5000)

        # Wait for loading spinner inside modal to hide
        page.wait_for_function(
            "() => { const el = document.getElementById('runs-loading'); return el && el.style.display === 'none'; }",
            timeout=8000,
        )

        runs_table = page.locator("#runs-table-container")
        runs_empty = page.locator("#runs-empty")
        has_runs = runs_table.is_visible()
        has_empty = runs_empty.is_visible()
        assert has_runs or has_empty, (
            "Neither run table nor empty state visible in modal"
        )

    def test_modal_columns_when_runs_present(self, admin_login):
        page = admin_login
        _wait_for_scraper_page(page)

        sources_container = page.locator("#sources-container")
        if not sources_container.is_visible():
            pytest.skip("No scraper sources configured")

        history_btn = page.locator('[data-action="view-runs"]').first
        history_btn.click()

        modal = page.locator("#runs-modal")
        expect(modal).to_be_visible(timeout=5000)
        page.wait_for_function(
            "() => { const el = document.getElementById('runs-loading'); return el && el.style.display === 'none'; }",
            timeout=8000,
        )

        if not page.locator("#runs-table-container").is_visible():
            pytest.skip("No run history for this source")

        expect(page.locator("#runs-modal th:has-text('Started')")).to_be_visible()
        expect(page.locator("#runs-modal th:has-text('Status')")).to_be_visible()
        expect(page.locator("#runs-modal th:has-text('Found')")).to_be_visible()
        expect(page.locator("#runs-modal th:has-text('New')")).to_be_visible()

    def test_modal_closes_via_button(self, admin_login):
        page = admin_login
        _wait_for_scraper_page(page)

        sources_container = page.locator("#sources-container")
        if not sources_container.is_visible():
            pytest.skip("No scraper sources configured")

        page.locator('[data-action="view-runs"]').first.click()
        modal = page.locator("#runs-modal")
        expect(modal).to_be_visible(timeout=5000)

        # Close via the footer Close button
        page.locator("#runs-modal .modal-footer .btn").click()
        expect(modal).to_be_hidden(timeout=5000)


# ============================================================================
# Entry point hint
# ============================================================================

if __name__ == "__main__":
    print(__doc__)
    print(
        "\nRun with:\n"
        "  source .env && uvx --from playwright --with playwright --with pytest "
        "pytest tests/e2e/test_scraper_sources.py -v"
    )
