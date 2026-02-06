#!/usr/bin/env python3
"""
Bootstrap Modal Cleanup Tests
Tests proper cleanup of Bootstrap modals when closed

Run with: uvx --from playwright --with playwright --with pytest python -m pytest tests/e2e/test_modal_cleanup.py -v
"""

import pytest
import os
from playwright.sync_api import Page, expect, sync_playwright


BASE_URL = "http://localhost:8080"
ADMIN_PASSWORD = os.getenv("ADMIN_PASSWORD", "XXKokg60kd8hLXgq")


@pytest.fixture(scope="module")
def browser():
    """Shared browser instance for all tests"""
    with sync_playwright() as p:
        browser = p.chromium.launch(headless=True)
        yield browser
        browser.close()


@pytest.fixture
def page(browser):
    """New page for each test"""
    page = browser.new_page()
    yield page
    page.close()


def login_as_admin(page: Page):
    """Helper to login as admin"""
    page.goto(f"{BASE_URL}/admin/login")
    page.fill("#username", "admin")
    page.fill("#password", ADMIN_PASSWORD)
    page.click('button[type="submit"]')
    page.wait_for_url(f"{BASE_URL}/admin/dashboard")


class TestBootstrapModalCleanup:
    """Test suite for Bootstrap modal cleanup behavior"""

    def test_modal_backdrop_removed_on_close(self, page: Page):
        """Test: Modal backdrop is removed from DOM when modal closes"""
        login_as_admin(page)
        page.goto(f"{BASE_URL}/admin/users")
        page.wait_for_load_state("networkidle")

        # Open modal
        page.click("#create-user-btn")
        page.wait_for_selector(".modal.show", timeout=2000)

        # Verify backdrop exists
        backdrop = page.locator(".modal-backdrop")
        expect(backdrop).to_be_visible()

        # Close modal using the close button
        page.click("#user-modal .btn-close")

        # Wait for modal to be hidden
        page.wait_for_selector(".modal.show", state="hidden", timeout=2000)

        # Verify backdrop is removed from DOM (not just hidden)
        expect(backdrop).to_have_count(0)

    def test_body_scroll_restored_on_close(self, page: Page):
        """Test: Body scroll is restored when modal closes"""
        login_as_admin(page)
        page.goto(f"{BASE_URL}/admin/users")
        page.wait_for_load_state("networkidle")

        # Check initial body state (should not have modal-open class)
        body_class_before = page.evaluate("() => document.body.className")
        assert "modal-open" not in body_class_before

        # Open modal
        page.click("#create-user-btn")
        page.wait_for_selector(".modal.show", timeout=2000)

        # Body should have modal-open class (disables scroll)
        body_class_open = page.evaluate("() => document.body.className")
        assert "modal-open" in body_class_open

        # Close modal
        page.click("#user-modal .btn-close")
        page.wait_for_selector(".modal.show", state="hidden", timeout=2000)

        # Wait a bit for cleanup
        page.wait_for_timeout(300)

        # Body should no longer have modal-open class (scroll restored)
        body_class_after = page.evaluate("() => document.body.className")
        assert "modal-open" not in body_class_after


if __name__ == "__main__":
    # Run with pytest if available, otherwise run basic test
    import sys

    try:
        import pytest

        sys.exit(pytest.main([__file__, "-v"]))
    except ImportError:
        print("pytest not found. Install with: pip install pytest")
        print(
            "Or run with: uvx --from playwright --with playwright --with pytest python",
            __file__,
        )
