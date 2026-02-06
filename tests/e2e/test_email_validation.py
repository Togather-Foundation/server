#!/usr/bin/env python3
"""
Email Validation Tests
Tests client-side email validation in user creation modal

Run with: uvx --from playwright --with playwright --with pytest python -m pytest tests/e2e/test_email_validation.py -v
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


def login_and_open_user_modal(page: Page):
    """Helper to login as admin and open user creation modal"""
    # Always start fresh from login page
    page.goto(f"{BASE_URL}/admin/login", wait_until="domcontentloaded")

    # Fill login form
    page.fill("#username", "admin")
    page.fill("#password", ADMIN_PASSWORD)
    page.click('button[type="submit"]')
    page.wait_for_url(f"{BASE_URL}/admin/dashboard", timeout=10000)

    # Navigate to users page
    page.goto(f"{BASE_URL}/admin/users", wait_until="domcontentloaded")
    page.wait_for_load_state("networkidle")

    # Open create user modal
    page.click("#create-user-btn", timeout=10000)
    page.wait_for_selector(".modal.show", timeout=5000)
    page.wait_for_timeout(300)


class TestEmailValidation:
    """Test suite for email validation"""

    def test_empty_email_blocked_by_html5(self, page: Page):
        """Test: Empty email is blocked by HTML5 required attribute"""
        login_and_open_user_modal(page)

        # HTML5 validation should require email
        email_input = page.locator("#user-email")
        expect(email_input).to_have_attribute("required", "")

    def test_invalid_email_shows_error_on_submit(self, page: Page):
        """Test: Invalid email shows error when trying to submit"""
        login_and_open_user_modal(page)

        # Fill in valid username, invalid email
        page.fill("#user-username", "testuser123")
        page.fill("#user-email", "invalid-email")
        page.select_option("#user-role", "viewer")

        # Try to submit
        page.click("#user-submit-btn")

        # Should show error toast
        toast = page.locator(
            ".toast.show:has-text('email'),.toast.show:has-text('valid')"
        )
        expect(toast).to_be_visible(timeout=3000)

    def test_consecutive_dots_email_rejected(self, page: Page):
        """Test: Email with consecutive dots is rejected"""
        login_and_open_user_modal(page)

        page.fill("#user-username", "testuser123")
        page.fill("#user-email", "user..name@example.com")
        page.select_option("#user-role", "viewer")

        page.click("#user-submit-btn")

        # Should show error toast
        toast = page.locator(".toast.show")
        expect(toast).to_be_visible(timeout=3000)

    def test_missing_tld_email_rejected(self, page: Page):
        """Test: Email without TLD is rejected"""
        login_and_open_user_modal(page)

        page.fill("#user-username", "testuser123")
        page.fill("#user-email", "user@domain")
        page.select_option("#user-role", "viewer")

        page.click("#user-submit-btn")

        # Should show error toast
        toast = page.locator(".toast.show")
        expect(toast).to_be_visible(timeout=3000)

    def test_missing_at_sign_rejected(self, page: Page):
        """Test: Email without @ sign is rejected"""
        login_and_open_user_modal(page)

        page.fill("#user-username", "testuser123")
        page.fill("#user-email", "notanemail.com")
        page.select_option("#user-role", "viewer")

        page.click("#user-submit-btn")

        # Should show error toast
        toast = page.locator(".toast.show")
        expect(toast).to_be_visible(timeout=3000)


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
