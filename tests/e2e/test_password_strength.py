#!/usr/bin/env python3
"""
Password Strength Indicator Tests
Tests the password strength calculation logic in accept-invitation.js

Run with: uvx --from playwright --with playwright pytest tests/e2e/test_password_strength.py -v
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


def create_test_invitation(page: Page) -> str:
    """
    Helper to create a test user invitation and return the token.
    This requires admin login first.
    """
    # Login as admin
    page.goto(f"{BASE_URL}/admin/login")
    page.fill("#username", "admin")
    page.fill("#password", ADMIN_PASSWORD)
    page.click('button[type="submit"]')
    page.wait_for_url(f"{BASE_URL}/admin/dashboard")

    # Navigate to users page
    page.goto(f"{BASE_URL}/admin/users")
    page.wait_for_load_state("networkidle")

    # Click create user button
    page.click("#create-user-btn")

    # Fill in user details with unique email
    import time

    timestamp = int(time.time() * 1000)
    test_email = f"test-pw-strength-{timestamp}@example.com"
    test_username = f"testpw{timestamp}"

    page.fill("#username", test_username)
    page.fill("#email", test_email)
    page.select_option("#role", "viewer")

    # Submit form
    page.click("#create-user-modal button[type='submit']")

    # Wait for success toast
    page.wait_for_selector(".toast.show", timeout=5000)

    # Extract invitation token from the created user's action
    # In a real scenario, we'd need to either:
    # 1. Mock the email service to capture the token
    # 2. Query the database directly
    # 3. Use the resend invitation endpoint
    # For now, we'll use a mock token for testing the UI logic

    # Logout
    page.click("#logout-btn")
    page.wait_for_url(f"{BASE_URL}/admin/login")

    # Return a placeholder token (in real tests, extract from invitation email)
    # For testing password strength UI, we can use any token format
    return "test-token-abc123"


class TestPasswordStrength:
    """Test suite for password strength calculation"""

    def test_empty_password_shows_no_strength(self, page: Page):
        """Test: Empty password should show 0% strength"""
        # Navigate directly to accept-invitation page
        # The password strength logic doesn't depend on valid token for UI testing
        page.goto(f"{BASE_URL}/accept-invitation?token=mock-token")
        page.wait_for_load_state("networkidle")

        # Password field should be empty
        password_field = page.locator("#password")
        expect(password_field).to_be_visible()
        expect(password_field).to_have_value("")

        # Check strength indicator
        strength_bar = page.locator("#password-strength")
        strength_text = page.locator("#password-strength-text")

        # Should show 0% width (note: may have trailing space in style attribute)
        bar_style = strength_bar.get_attribute("style")
        assert "width: 0%" in bar_style
        expect(strength_text).to_contain_text("None")

    def test_only_lowercase_shows_50_percent(self, page: Page):
        """Test: Password with only lowercase + length should show 50% (fair)"""
        page.goto(f"{BASE_URL}/accept-invitation?token=mock-token")
        page.wait_for_load_state("networkidle")

        # Type password with only lowercase AND sufficient length (50 points: lowercase + length)
        page.fill(
            "#password", "abcdefghijklmnop"
        )  # 16 chars = lowercase (25) + length (25) = 50%

        # Trigger strength update (happens on input event)
        page.dispatch_event("#password", "input")

        # Check strength indicator
        strength_bar = page.locator("#password-strength")
        strength_text = page.locator("#password-strength-text")

        # Should show 50% (lowercase + length requirements met)
        bar_style = strength_bar.get_attribute("style")
        assert "width: 50%" in bar_style
        expect(strength_bar).to_have_class(
            "progress-bar bg-warning"
        )  # 50% is in warning range (25-74%)
        expect(strength_text).to_contain_text("Fair")
        expect(strength_text).to_contain_text(
            "missing:"
        )  # Changed from "needs:" to "missing:"

    def test_all_criteria_met_shows_100_percent(self, page: Page):
        """Test: Password meeting all criteria should show 100% (strong)"""
        page.goto(f"{BASE_URL}/accept-invitation?token=mock-token")
        page.wait_for_load_state("networkidle")

        # Strong password: 12+ chars, upper, lower, number, special
        page.fill("#password", "Strong@Pass123!")
        page.dispatch_event("#password", "input")

        # Check strength indicator
        strength_bar = page.locator("#password-strength")
        strength_text = page.locator("#password-strength-text")

        # Should show 100%
        expect(strength_bar).to_have_attribute("style", "width: 100%;")
        expect(strength_bar).to_have_class("progress-bar bg-success")
        expect(strength_text).to_contain_text("Strong")

    def test_11_chars_fails_length_requirement(self, page: Page):
        """Test: Password with 11 characters should fail length requirement"""
        page.goto(f"{BASE_URL}/accept-invitation?token=mock-token")
        page.wait_for_load_state("networkidle")

        # Wait for password input to be visible and interactable
        page.wait_for_selector("#password:visible", state="visible", timeout=2000)

        # 11 chars with all other criteria met
        page.fill("#password", "Short@Pass1")  # 11 chars
        page.dispatch_event("#password", "input")

        # Check strength indicator
        strength_text = page.locator("#password-strength-text")

        # Should mention needing 12 characters
        expect(strength_text).to_contain_text("at least 12 characters")

        # Should not be 100% (missing 25 points for length)
        strength_bar = page.locator("#password-strength")
        bar_style = strength_bar.get_attribute("style")
        assert "width: 75%" in bar_style or "width: 75.0%" in bar_style

    def test_special_chars_in_different_positions(self, page: Page):
        """Test: Special characters work regardless of position"""
        page.goto(f"{BASE_URL}/accept-invitation?token=mock-token")
        page.wait_for_load_state("networkidle")

        # Wait for password input to be visible and interactable
        page.wait_for_selector("#password:visible", state="visible", timeout=2000)

        test_cases = [
            "@StartSpecial123Aa",  # Special at start
            "MiddleSpec!al123Aa",  # Special in middle
            "EndSpecialAa123!",  # Special at end
        ]

        for password in test_cases:
            page.fill("#password", password)
            page.dispatch_event("#password", "input")

            # All should show 100% since all criteria are met
            strength_bar = page.locator("#password-strength")
            expect(strength_bar).to_have_attribute("style", "width: 100%;")

    def test_missing_uppercase_shows_feedback(self, page: Page):
        """Test: Missing uppercase shows appropriate feedback"""
        page.goto(f"{BASE_URL}/accept-invitation?token=mock-token")
        page.wait_for_load_state("networkidle")

        # Wait for password input to be visible and interactable
        page.wait_for_selector("#password:visible", state="visible", timeout=2000)

        # 12+ chars, lower, number, special, but NO uppercase
        page.evaluate("""
            const input = document.getElementById('password');
            input.value = 'nouppercase123!';
            input.dispatchEvent(new Event('input', { bubbles: true }));
        """)
        page.wait_for_timeout(200)

        strength_text = page.locator("#password-strength-text")
        expect(strength_text).to_contain_text("uppercase letter")

    def test_missing_number_shows_feedback(self, page: Page):
        """Test: Missing number shows appropriate feedback"""
        page.goto(f"{BASE_URL}/accept-invitation?token=mock-token")
        page.wait_for_load_state("networkidle")

        # 12+ chars, upper, lower, special, but NO number
        page.fill("#password", "NoNumberHere!Aa")
        page.dispatch_event("#password", "input")

        strength_text = page.locator("#password-strength-text")
        expect(strength_text).to_contain_text("number")

    def test_missing_special_char_shows_feedback(self, page: Page):
        """Test: Missing special character shows appropriate feedback"""
        page.goto(f"{BASE_URL}/accept-invitation?token=mock-token")
        page.wait_for_load_state("networkidle")

        # Wait for password input to be visible and interactable
        page.wait_for_selector("#password:visible", state="visible", timeout=2000)

        # 12+ chars, upper, lower, number, but NO special
        page.evaluate("""
            const input = document.getElementById('password');
            input.value = 'NoSpecialChar123Aa';
            input.dispatchEvent(new Event('input', { bubbles: true }));
        """)
        page.wait_for_timeout(200)

        strength_text = page.locator("#password-strength-text")
        expect(strength_text).to_contain_text("special character")

    def test_strength_colors_match_score(self, page: Page):
        """Test: Strength bar colors match the score ranges"""
        page.goto(f"{BASE_URL}/accept-invitation?token=mock-token")
        page.wait_for_load_state("networkidle")

        test_cases = [
            # (password, expected_class, expected_score_info)
            ("abc", "bg-warning", "25%"),  # lowercase only (short)
            ("abcdefghijkl", "bg-warning", "50%"),  # lowercase + length
            (
                "Abcdefghijk1!2",
                "bg-success",
                "100%",
            ),  # All criteria met (12 chars, upper, lower, number, special)
        ]

        for password, expected_class, score_info in test_cases:
            page.fill("#password", password)
            page.dispatch_event("#password", "input")

            strength_bar = page.locator("#password-strength")
            actual_class = strength_bar.get_attribute("class")

            assert expected_class in actual_class, (
                f"Password '{password}' expected {expected_class} but got {actual_class} ({score_info})"
            )

    def test_clearing_password_resets_indicator(self, page: Page):
        """Test: Clearing password resets indicator to 0%"""
        page.goto(f"{BASE_URL}/accept-invitation?token=mock-token")
        page.wait_for_load_state("networkidle")

        # First, enter a strong password
        page.fill("#password", "Strong@Pass123!")
        page.dispatch_event("#password", "input")

        # Verify it shows 100%
        strength_bar = page.locator("#password-strength")
        expect(strength_bar).to_have_attribute("style", "width: 100%;")

        # Now clear the password
        page.fill("#password", "")
        page.dispatch_event("#password", "input")

        # Should reset to 0%
        expect(strength_bar).to_have_attribute("style", "width: 0%;")
        strength_text = page.locator("#password-strength-text")
        expect(strength_text).to_contain_text("None")

    def test_real_time_update_on_typing(self, page: Page):
        """Test: Strength updates in real-time as user types"""
        page.goto(f"{BASE_URL}/accept-invitation?token=mock-token")
        page.wait_for_load_state("networkidle")

        # Wait for password input to be visible and interactable
        page.wait_for_selector("#password:visible", state="visible", timeout=2000)

        strength_bar = page.locator("#password-strength")

        # Type character by character and verify it updates
        page.type("#password", "a")
        expect(strength_bar).to_have_attribute("style", "width: 25%;")

        page.type("#password", "A")
        # Should now have 50% (lowercase + uppercase)
        bar_style = strength_bar.get_attribute("style")
        assert "width: 50%" in bar_style or "width: 50.0%" in bar_style

        page.type("#password", "1")
        # Should now have 62.5% (lowercase + uppercase + number)
        bar_style = strength_bar.get_attribute("style")
        assert "62.5%" in bar_style

    def test_various_special_characters(self, page: Page):
        """Test: All types of special characters are recognized"""
        page.goto(f"{BASE_URL}/accept-invitation?token=mock-token")
        page.wait_for_load_state("networkidle")

        # Wait for password input to be visible and interactable
        page.wait_for_selector("#password:visible", state="visible", timeout=2000)

        # Test a few common special characters
        special_chars = ["!", "@", "#", "$", "%"]

        for char in special_chars:
            password = f"Password123{char}A"

            # Use page.evaluate to set value directly
            page.evaluate(f"""
                const input = document.getElementById('password');
                input.value = {repr(password)};
                input.dispatchEvent(new Event('input', {{ bubbles: true }}));
            """)

            # Wait for update
            page.wait_for_timeout(300)

            # Should show 100% for all
            strength_bar = page.locator("#password-strength")
            bar_style = strength_bar.get_attribute("style")
            assert "100%" in bar_style, (
                f"Password with {char} should be 100%, got: {bar_style}"
            )

        # Test a few common special characters
        special_chars = ["!", "@", "#", "$", "%"]

        for char in special_chars:
            password = f"Password123{char}A"

            # Use page.evaluate to set value directly
            page.evaluate(f"""
                const input = document.getElementById('password');
                input.value = {repr(password)};
                input.dispatchEvent(new Event('input', {{ bubbles: true }}));
            """)

            # Wait for update
            page.wait_for_timeout(300)

            # Should show 100% for all
            strength_bar = page.locator("#password-strength")
            bar_style = strength_bar.get_attribute("style")
            assert "100%" in bar_style, (
                f"Password with {char} should be 100%, got: {bar_style}"
            )


if __name__ == "__main__":
    # Run with pytest if available, otherwise run basic test
    import sys

    try:
        import pytest

        sys.exit(pytest.main([__file__, "-v"]))
    except ImportError:
        print("pytest not found. Install with: pip install pytest")
        print("Or run with: uvx --from playwright --with playwright pytest", __file__)
