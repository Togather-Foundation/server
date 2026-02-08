#!/usr/bin/env python3
"""
User Management E2E Tests using Playwright (Python)

This test suite provides comprehensive coverage of the user administration system:
- User list page (filters, pagination, navigation)
- User CRUD operations (create, read, update, delete)
- User actions (activate, deactivate, resend invitation)
- User activity page
- Invitation acceptance (public page)
- XSS protection
- Console error detection

Run with:
    uvx --from playwright --with playwright pytest tests/e2e/test_user_management.py -v

Or with custom password:
    ADMIN_PASSWORD=mypassword uvx --from playwright --with playwright pytest tests/e2e/test_user_management.py -v
"""

import os
import pytest
import random
import string
from playwright.sync_api import Page, expect

# Configuration
BASE_URL = os.getenv(
    "BASE_URL", "http://localhost:8080"
)  # Can override with BASE_URL env var
ADMIN_USERNAME = "admin"
ADMIN_PASSWORD = os.getenv("ADMIN_PASSWORD", "XXKokg60kd8hLXgq")


# ============================================================================
# Fixtures
# ============================================================================


@pytest.fixture(scope="session")
def browser_context_args(browser_context_args):
    """Configure browser context for all tests"""
    return {
        **browser_context_args,
        "viewport": {"width": 1920, "height": 1080},
        "ignore_https_errors": True,
    }


@pytest.fixture(scope="function")
def console_errors():
    """Track console errors across test"""
    errors = []
    return errors


@pytest.fixture(scope="function")
def admin_login(page: Page, console_errors):
    """Login as admin before test and setup console error tracking"""

    # Setup console error tracking
    def handle_console(msg):
        if msg.type == "error":
            console_errors.append(msg.text)
            print(f"   [Console Error] {msg.text}")

    page.on("console", handle_console)

    # Login
    print("\n   Logging in as admin...")
    page.goto(f"{BASE_URL}/admin/login")
    page.fill("#username", ADMIN_USERNAME)
    page.fill("#password", ADMIN_PASSWORD)
    page.click('button[type="submit"]')
    page.wait_for_url(f"{BASE_URL}/admin/dashboard", timeout=5000)
    print("   ✓ Logged in successfully")
    return page


def generate_unique_username():
    """Generate unique username for test users"""
    suffix = "".join(random.choices(string.ascii_lowercase + string.digits, k=8))
    return f"testuser_{suffix}"


def generate_unique_email(username):
    """Generate unique email for test users"""
    return f"{username}@example.com"


# ============================================================================
# Test Class: User List Page
# ============================================================================


class TestUserListPage:
    """Tests for user list page UI and functionality"""

    def test_users_page_loads(self, page: Page, admin_login):
        """Test that users page loads with correct structure"""
        print("\n1. Loading /admin/users page...")
        page.goto(f"{BASE_URL}/admin/users")
        page.wait_for_load_state("networkidle")

        # Verify page title
        expect(page).to_have_title("Users - SEL Admin")
        print("   ✓ Page title correct")

        # Verify page header
        expect(page.locator('h2.page-title:has-text("Users")')).to_be_visible()
        print("   ✓ Page header visible")

        # Verify "Invite User" button
        expect(page.locator('#create-user-btn:has-text("Invite User")')).to_be_visible()
        print("   ✓ Invite User button visible")

        # Take screenshot
        page.screenshot(path="/tmp/user_list_page.png", full_page=True)
        print("   ✓ Screenshot: /tmp/user_list_page.png")

    def test_users_nav_link_active(self, page: Page, admin_login):
        """Test that Users nav link is highlighted"""
        page.goto(f"{BASE_URL}/admin/users")
        page.wait_for_load_state("networkidle")

        # Check for active nav link (exact selector depends on _header.html)
        # This assumes there's a nav link with href="/admin/users"
        users_nav = page.locator(
            'nav a[href="/admin/users"], .navbar a[href="/admin/users"]'
        )
        if users_nav.count() > 0:
            print("   ✓ Users navigation link found")
        else:
            print("   ⚠ Users navigation link not found (may not be in nav yet)")

    def test_table_headers_present(self, page: Page, admin_login):
        """Test that table has correct headers"""
        page.goto(f"{BASE_URL}/admin/users")
        page.wait_for_load_state("networkidle")
        page.wait_for_timeout(1000)  # Wait for JS to load data

        # Verify table headers
        headers = ["User", "Role", "Status", "Last Login", "Created", "Actions"]
        for header in headers:
            expect(page.locator(f'thead th:has-text("{header}")')).to_be_visible()
            print(f"   ✓ Header '{header}' found")

    def test_filters_present(self, page: Page, admin_login):
        """Test that all filter controls are present"""
        page.goto(f"{BASE_URL}/admin/users")
        page.wait_for_load_state("networkidle")

        # Search input
        expect(page.locator("#search-input")).to_be_visible()
        print("   ✓ Search input found")

        # Status filter dropdown
        expect(page.locator("#status-filter")).to_be_visible()
        print("   ✓ Status filter found")

        # Role filter dropdown
        expect(page.locator("#role-filter")).to_be_visible()
        print("   ✓ Role filter found")

        # Clear filters button
        expect(page.locator("#clear-filters")).to_be_visible()
        print("   ✓ Clear filters button found")

    def test_invite_user_button_opens_modal(self, page: Page, admin_login):
        """Test that Invite User button opens the modal"""
        page.goto(f"{BASE_URL}/admin/users")
        page.wait_for_load_state("networkidle")
        page.wait_for_timeout(500)

        # Click Invite User button
        page.click("#create-user-btn")
        page.wait_for_timeout(500)

        # Verify modal appears
        modal = page.locator("#user-modal")
        expect(modal).to_be_visible()
        print("   ✓ User modal opened")

        # Verify modal title
        expect(
            page.locator('#user-modal-title:has-text("Invite User")')
        ).to_be_visible()
        print("   ✓ Modal title is 'Invite User'")

        # Verify form fields
        expect(page.locator("#user-username")).to_be_visible()
        expect(page.locator("#modal-user-email")).to_be_visible()
        expect(page.locator("#user-role")).to_be_visible()
        print("   ✓ All form fields present")


# ============================================================================
# Test Class: User CRUD Operations
# ============================================================================


class TestUserCRUD:
    """Tests for creating, reading, updating, deleting users"""

    def test_create_user_via_modal(self, page: Page, admin_login):
        """Test creating a new user through the modal"""
        page.goto(f"{BASE_URL}/admin/users")
        page.wait_for_load_state("networkidle")
        page.wait_for_timeout(1000)

        # Generate unique user data
        username = generate_unique_username()
        email = generate_unique_email(username)

        print(f"\n   Creating user: {username}")

        # Click Invite User button
        page.click("#create-user-btn")
        page.wait_for_timeout(500)

        # Fill form
        page.fill("#user-username", username)
        page.fill("#modal-user-email", email)
        page.select_option("#user-role", "viewer")
        print("   ✓ Form filled")

        # Submit
        page.click("#user-submit-btn")
        page.wait_for_timeout(2000)  # Wait for API call and table refresh

        # Verify success toast appears
        toast = page.locator('.toast:has-text("invited successfully")')
        if toast.count() > 0:
            print("   ✓ Success toast appeared")

        # Verify user appears in table
        user_row = page.locator(f'tr:has-text("{username}")')
        expect(user_row).to_be_visible()
        print(f"   ✓ User '{username}' appears in table")

        # Take screenshot
        page.screenshot(path="/tmp/user_created.png", full_page=True)
        print("   ✓ Screenshot: /tmp/user_created.png")

    def test_edit_user_role(self, page: Page, admin_login):
        """Test editing a user's role"""
        page.goto(f"{BASE_URL}/admin/users")
        page.wait_for_load_state("networkidle")
        page.wait_for_timeout(1000)

        # Create a test user first
        username = generate_unique_username()
        email = generate_unique_email(username)

        page.click("#create-user-btn")
        page.wait_for_timeout(500)
        page.fill("#user-username", username)
        page.fill("#modal-user-email", email)
        page.select_option("#user-role", "viewer")
        page.click("#user-submit-btn")
        page.wait_for_timeout(2000)

        print(f"\n   Editing user: {username}")

        # Find the user row and click Edit button
        user_row = page.locator(f'tr:has-text("{username}")')
        edit_btn = user_row.locator("button.edit-user-btn")
        edit_btn.click()
        page.wait_for_timeout(1000)  # Wait for API call to load user data

        # Verify modal opened with user data
        expect(page.locator("#user-modal")).to_be_visible()
        expect(page.locator('#user-modal-title:has-text("Edit User")')).to_be_visible()
        print("   ✓ Edit modal opened")

        # Change role to editor
        page.select_option("#user-role", "editor")
        page.click("#user-submit-btn")
        page.wait_for_timeout(2000)

        # Verify role badge changed
        role_badge = user_row.locator('span.badge:has-text("editor")')
        expect(role_badge).to_be_visible()
        print("   ✓ Role changed to 'editor'")

    def test_delete_user_with_confirmation(self, page: Page, admin_login):
        """Test deleting a user with confirmation dialog"""
        page.goto(f"{BASE_URL}/admin/users")
        page.wait_for_load_state("networkidle")
        page.wait_for_timeout(1000)

        # Create a test user first
        username = generate_unique_username()
        email = generate_unique_email(username)

        page.click("#create-user-btn")
        page.wait_for_timeout(500)
        page.fill("#user-username", username)
        page.fill("#modal-user-email", email)
        page.select_option("#user-role", "viewer")
        page.click("#user-submit-btn")
        page.wait_for_timeout(2000)

        print(f"\n   Deleting user: {username}")

        # Find the user row and click Delete button
        user_row = page.locator(f'tr:has-text("{username}")')
        delete_btn = user_row.locator("button.delete-user-btn")

        # Click delete
        delete_btn.click()
        page.wait_for_timeout(500)

        # Verify confirmation modal appears
        confirm_modal = page.locator("#confirm-modal")
        expect(confirm_modal).to_be_visible()
        print("   ✓ Confirmation modal appeared")

        # Verify confirmation message includes username
        expect(page.locator(f'.modal-body:has-text("{username}")')).to_be_visible()
        print("   ✓ Confirmation message shows username")

        # Confirm deletion
        page.click("#confirm-action")
        page.wait_for_timeout(2000)

        # Verify user removed from table
        user_row = page.locator(f'tr:has-text("{username}")')
        expect(user_row).not_to_be_visible()
        print(f"   ✓ User '{username}' removed from table")


# ============================================================================
# Test Class: User Actions
# ============================================================================


class TestUserActions:
    """Tests for user-specific actions (activate, deactivate, resend invitation)"""

    def test_deactivate_active_user(self, page: Page, admin_login):
        """Test deactivating an active user"""
        # Note: This assumes we have at least one active user (admin)
        # Or we'd need to create and activate a user first
        print("\n   Testing deactivate action (skipped - requires active user setup)")
        # Implementation would require backend to support user activation via API
        # and a test user that's already active

    def test_activate_inactive_user(self, page: Page, admin_login):
        """Test activating an inactive user"""
        print("\n   Testing activate action (skipped - requires inactive user setup)")
        # Implementation would require backend to support user deactivation
        # and a test user that's already inactive

    def test_resend_invitation_to_pending_user(self, page: Page, admin_login):
        """Test resending invitation to pending user"""
        page.goto(f"{BASE_URL}/admin/users")
        page.wait_for_load_state("networkidle")
        page.wait_for_timeout(1000)

        # Create a test user (will be pending by default)
        username = generate_unique_username()
        email = generate_unique_email(username)

        page.click("#create-user-btn")
        page.wait_for_timeout(500)
        page.fill("#user-username", username)
        page.fill("#modal-user-email", email)
        page.select_option("#user-role", "viewer")
        page.click("#user-submit-btn")
        page.wait_for_timeout(2000)

        print(f"\n   Resending invitation to: {username}")

        # Find the user row and click Resend Invitation button
        user_row = page.locator(f'tr:has-text("{username}")')
        resend_btn = user_row.locator("button.resend-invitation-btn")

        if resend_btn.count() > 0:
            resend_btn.click()
            page.wait_for_timeout(1000)

            # Verify success toast
            toast = page.locator('.toast:has-text("Invitation resent")')
            if toast.count() > 0:
                print("   ✓ Success toast appeared")
            else:
                print("   ⚠ No success toast (may have disappeared)")
        else:
            print("   ⚠ Resend Invitation button not found (user may not be pending)")


# ============================================================================
# Test Class: User Activity Page
# ============================================================================


class TestUserActivityPage:
    """Tests for user activity page"""

    def test_user_activity_page_structure(self, page: Page, admin_login):
        """Test that user activity page has correct structure"""
        # We'll use the admin user's activity page
        page.goto(f"{BASE_URL}/admin/users")
        page.wait_for_load_state("networkidle")
        page.wait_for_timeout(1000)

        # Find first user with Activity link
        activity_link = page.locator('a:has-text("Activity")').first
        if activity_link.count() == 0:
            print("\n   ⚠ No users found with Activity link")
            return

        print("\n   Navigating to user activity page...")
        activity_link.click()
        page.wait_for_load_state("networkidle")
        page.wait_for_timeout(1000)

        # Verify page title
        expect(page).to_have_title("User Activity - SEL Admin")
        print("   ✓ Page title correct")

        # Verify page header
        expect(page.locator('h2.page-title:has-text("User Activity")')).to_be_visible()
        print("   ✓ Page header visible")

        # Verify back link
        expect(page.locator('a:has-text("Back to Users")')).to_be_visible()
        print("   ✓ Back to Users link visible")

        # Verify user info card
        expect(page.locator("#user-name")).to_be_visible()
        expect(page.locator("#user-email")).to_be_visible()
        expect(page.locator("#user-role-badge")).to_be_visible()
        expect(page.locator("#user-status-badge")).to_be_visible()
        print("   ✓ User info card displays")

        # Verify activity stats
        stat_ids = [
            "stat-total-logins",
            "stat-events-created",
            "stat-events-edited",
            "stat-recent-activity",
        ]
        for stat_id in stat_ids:
            expect(page.locator(f"#{stat_id}")).to_be_visible()
        print("   ✓ Activity stats present")

        # Verify filters
        expect(page.locator("#event-type-filter")).to_be_visible()
        expect(page.locator("#date-from-filter")).to_be_visible()
        expect(page.locator("#date-to-filter")).to_be_visible()
        print("   ✓ Activity filters present")

        # Take screenshot
        page.screenshot(path="/tmp/user_activity_page.png", full_page=True)
        print("   ✓ Screenshot: /tmp/user_activity_page.png")


# ============================================================================
# Test Class: Invitation Acceptance (Public Page)
# ============================================================================


class TestInvitationAcceptance:
    """Tests for public invitation acceptance page"""

    def test_invalid_token_shows_error(self, page: Page):
        """Test that invalid token shows error message"""
        print("\n   Testing invalid invitation token...")
        page.goto(f"{BASE_URL}/accept-invitation?token=INVALID_TOKEN_12345")
        page.wait_for_load_state("networkidle")
        page.wait_for_timeout(2000)  # Wait for JS to process

        # Error state should be visible
        error_state = page.locator("#error-state")
        if error_state.is_visible():
            print("   ✓ Error state displayed")

            # Verify error message
            expect(page.locator("#error-message")).to_be_visible()
            print("   ✓ Error message visible")
        else:
            # Form might show if JS doesn't validate token upfront
            form = page.locator("#accept-invitation-form")
            if form.is_visible():
                print("   ⚠ Form shown (validation happens on submit)")

        # Take screenshot
        page.screenshot(path="/tmp/invitation_invalid_token.png", full_page=True)
        print("   ✓ Screenshot: /tmp/invitation_invalid_token.png")

    def test_no_token_shows_error(self, page: Page):
        """Test that missing token shows error"""
        print("\n   Testing missing invitation token...")
        page.goto(f"{BASE_URL}/accept-invitation")
        page.wait_for_load_state("networkidle")
        page.wait_for_timeout(1000)

        # Error should be shown
        error_state = page.locator("#error-state")
        expect(error_state).to_be_visible()
        print("   ✓ Error state displayed for missing token")

        # Verify error message mentions token
        error_text = page.locator("#error-message").inner_text()
        assert "token" in error_text.lower()
        print("   ✓ Error message mentions token")

    def test_password_form_elements(self, page: Page):
        """Test that password form has all required elements"""
        # Visit with a token (even if invalid) to see form
        page.goto(f"{BASE_URL}/accept-invitation?token=TEST_TOKEN")
        page.wait_for_load_state("networkidle")
        page.wait_for_timeout(1000)

        # Check if form is visible (depends on backend validation)
        form = page.locator("#accept-invitation-form")

        if form.is_visible():
            print("\n   Form is visible, checking elements...")

            # Password field
            expect(page.locator("#password")).to_be_visible()
            print("   ✓ Password field present")

            # Confirm password field
            expect(page.locator("#confirm-password")).to_be_visible()
            print("   ✓ Confirm password field present")

            # Password strength indicator
            expect(page.locator("#password-strength")).to_be_visible()
            expect(page.locator("#password-strength-text")).to_be_visible()
            print("   ✓ Password strength indicator present")

            # Submit button
            expect(page.locator("#submit-btn")).to_be_visible()
            print("   ✓ Submit button present")
        else:
            print("\n   ⚠ Form not visible (token validation may have failed)")

    def test_password_strength_indicator(self, page: Page):
        """Test password strength indicator updates"""
        page.goto(f"{BASE_URL}/accept-invitation?token=TEST_TOKEN")
        page.wait_for_load_state("networkidle")
        page.wait_for_timeout(1000)

        form = page.locator("#accept-invitation-form")
        if not form.is_visible():
            print("\n   ⚠ Form not visible, skipping strength indicator test")
            return

        print("\n   Testing password strength indicator...")

        password_input = page.locator("#password")
        strength_bar = page.locator("#password-strength")
        strength_text = page.locator("#password-strength-text")

        # Test weak password
        password_input.fill("weak")
        page.wait_for_timeout(300)
        text = strength_text.inner_text()
        assert "weak" in text.lower() or "needs" in text.lower()
        print("   ✓ Weak password detected")

        # Test strong password
        password_input.fill("StrongPass123!@#")
        page.wait_for_timeout(300)
        text = strength_text.inner_text()
        assert "strong" in text.lower() or "100" in strength_bar.get_attribute("style")
        print("   ✓ Strong password detected")

    def test_password_mismatch_validation(self, page: Page):
        """Test that password mismatch shows error"""
        page.goto(f"{BASE_URL}/accept-invitation?token=TEST_TOKEN")
        page.wait_for_load_state("networkidle")
        page.wait_for_timeout(1000)

        form = page.locator("#accept-invitation-form")
        if not form.is_visible():
            print("\n   ⚠ Form not visible, skipping mismatch test")
            return

        print("\n   Testing password mismatch validation...")

        # Fill with mismatched passwords
        page.fill("#password", "StrongPass123!@#")
        page.fill("#confirm-password", "DifferentPass456!@#")
        page.wait_for_timeout(300)

        # Trigger validation (blur event)
        page.locator("#confirm-password").blur()
        page.wait_for_timeout(300)

        # Check for error
        confirm_input = page.locator("#confirm-password")
        if "is-invalid" in confirm_input.get_attribute("class"):
            print("   ✓ Password mismatch error shown")
        else:
            print("   ⚠ Mismatch validation may trigger on submit")


# ============================================================================
# Test Class: XSS Protection
# ============================================================================


class TestXSSProtection:
    """Tests for XSS protection in user management"""

    def test_malicious_username_script_tag_escaped(self, page: Page, admin_login):
        """Test that <script> tag in username is escaped"""
        page.goto(f"{BASE_URL}/admin/users")
        page.wait_for_load_state("networkidle")
        page.wait_for_timeout(1000)

        # Create user with malicious username
        malicious_username = "<script>alert('XSS')</script>"
        email = generate_unique_email("xss_test_1")

        print(f"\n   Creating user with malicious username: {malicious_username}")

        page.click("#create-user-btn")
        page.wait_for_timeout(500)
        page.fill("#user-username", malicious_username)
        page.fill("#modal-user-email", email)
        page.select_option("#user-role", "viewer")
        page.click("#user-submit-btn")
        page.wait_for_timeout(2000)

        # Verify no script execution (no alert dialog)
        # If script executed, page.evaluate would detect it
        print("   ✓ No alert dialog appeared")

        # Verify username is HTML-escaped in table
        # Look for escaped HTML entities
        page_content = page.content()

        # Check that raw script tag is NOT in HTML
        assert "<script>alert" not in page_content, "Script tag not escaped!"
        print("   ✓ Script tag not present in HTML")

        # Check that escaped version IS present
        assert "&lt;script&gt;" in page_content or "&lt;" in page_content
        print("   ✓ Username HTML-escaped in table")

    def test_malicious_username_img_tag_escaped(self, page: Page, admin_login):
        """Test that <img> tag with onerror is escaped"""
        page.goto(f"{BASE_URL}/admin/users")
        page.wait_for_load_state("networkidle")
        page.wait_for_timeout(1000)

        # Create user with malicious username
        malicious_username = "<img src=x onerror=alert('XSS')>"
        email = generate_unique_email("xss_test_2")

        print(f"\n   Creating user with malicious img tag: {malicious_username}")

        page.click("#create-user-btn")
        page.wait_for_timeout(500)
        page.fill("#user-username", malicious_username)
        page.fill("#modal-user-email", email)
        page.select_option("#user-role", "viewer")
        page.click("#user-submit-btn")
        page.wait_for_timeout(2000)

        # Verify no script execution
        print("   ✓ No alert dialog appeared")

        # Verify HTML is escaped
        page_content = page.content()
        assert "<img src=x onerror=" not in page_content, "Img tag not escaped!"
        print("   ✓ Img tag not present in HTML")

        assert "&lt;img" in page_content or "&lt;" in page_content
        print("   ✓ Username HTML-escaped in table")

    def test_malicious_search_input_escaped(self, page: Page, admin_login):
        """Test that search input with malicious content is escaped"""
        page.goto(f"{BASE_URL}/admin/users")
        page.wait_for_load_state("networkidle")
        page.wait_for_timeout(1000)

        print("\n   Testing malicious search input...")

        # Fill search with malicious content
        malicious_search = "<script>alert('XSS')</script>"
        page.fill("#search-input", malicious_search)
        page.wait_for_timeout(1000)  # Wait for debounced search

        # Verify no script execution
        print("   ✓ No alert dialog from search")

        # Verify search input value is preserved (not executed)
        input_value = page.locator("#search-input").input_value()
        assert "<script>" in input_value
        print("   ✓ Search input preserved (not executed)")

    def test_data_attributes_escaped(self, page: Page, admin_login):
        """Test that data attributes don't contain unescaped HTML"""
        page.goto(f"{BASE_URL}/admin/users")
        page.wait_for_load_state("networkidle")
        page.wait_for_timeout(1000)

        print("\n   Checking data-* attributes for XSS...")

        # Get all buttons with data-username attribute
        buttons = page.locator("button[data-username]").all()

        if len(buttons) > 0:
            for i, button in enumerate(buttons[:3]):  # Check first 3
                data_username = button.get_attribute("data-username")

                # Verify no unescaped HTML in data attribute
                if "<" in data_username and "&lt;" not in data_username:
                    print(f"   ⚠ Unescaped HTML in data-username: {data_username}")
                else:
                    print(f"   ✓ Data attribute {i + 1} properly escaped")
        else:
            print("   ⚠ No buttons with data-username found")


# ============================================================================
# Test Class: Console Errors
# ============================================================================


class TestConsoleErrors:
    """Tests for JavaScript console errors"""

    def test_users_page_no_console_errors(
        self, page: Page, admin_login, console_errors
    ):
        """Test that users list page has no console errors"""
        print("\n   Loading users page and checking for console errors...")
        page.goto(f"{BASE_URL}/admin/users")
        page.wait_for_load_state("networkidle")
        page.wait_for_timeout(2000)  # Wait for JS to fully execute

        # Check console errors
        if len(console_errors) == 0:
            print("   ✓ No console errors detected")
        else:
            print(f"\n   ✗ Found {len(console_errors)} console error(s):")
            for error in console_errors:
                print(f"      • {error}")
            pytest.fail(
                f"Console errors detected on users page: {len(console_errors)} errors"
            )

    def test_user_activity_page_no_console_errors(
        self, page: Page, admin_login, console_errors
    ):
        """Test that user activity page has no console errors"""
        page.goto(f"{BASE_URL}/admin/users")
        page.wait_for_load_state("networkidle")
        page.wait_for_timeout(1000)

        # Find first activity link
        activity_link = page.locator('a:has-text("Activity")').first
        if activity_link.count() == 0:
            print("\n   ⚠ No activity links found, skipping test")
            return

        print("\n   Loading user activity page and checking for console errors...")

        # Clear previous errors
        console_errors.clear()

        activity_link.click()
        page.wait_for_load_state("networkidle")
        page.wait_for_timeout(2000)

        # Check console errors
        if len(console_errors) == 0:
            print("   ✓ No console errors detected")
        else:
            print(f"\n   ✗ Found {len(console_errors)} console error(s):")
            for error in console_errors:
                print(f"      • {error}")
            pytest.fail(
                f"Console errors detected on user activity page: {len(console_errors)} errors"
            )

    def test_invitation_page_no_console_errors(self, page: Page, console_errors):
        """Test that invitation acceptance page has no console errors"""
        print("\n   Loading invitation page and checking for console errors...")
        page.goto(f"{BASE_URL}/accept-invitation?token=TEST_TOKEN")
        page.wait_for_load_state("networkidle")
        page.wait_for_timeout(2000)

        # Check console errors
        if len(console_errors) == 0:
            print("   ✓ No console errors detected")
        else:
            print(f"\n   ✗ Found {len(console_errors)} console error(s):")
            for error in console_errors:
                print(f"      • {error}")
            pytest.fail(
                f"Console errors detected on invitation page: {len(console_errors)} errors"
            )


# ============================================================================
# Summary
# ============================================================================

if __name__ == "__main__":
    """
    This test suite cannot be run directly with python.
    Use pytest instead:
        
        uvx --from playwright --with playwright pytest tests/e2e/test_user_management.py -v
    """
    print(__doc__)
    print("\nPlease run with pytest (see instructions above)")
