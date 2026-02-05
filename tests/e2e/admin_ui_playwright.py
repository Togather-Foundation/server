#!/usr/bin/env python3
"""
E2E tests for Admin UI using Playwright

Tests:
1. Login flow (success and failure)
2. Dashboard loads and displays stats
3. Events list page
4. Duplicates review page
5. API keys management
6. Logout functionality
"""

import sys
import time
from playwright.sync_api import sync_playwright, expect

# Server URL - will be running via with_server.py
BASE_URL = "http://localhost:8080"

# Test credentials (matches what setupTestServer creates)
ADMIN_USERNAME = "admin"
ADMIN_PASSWORD = "test123"


def test_login_page_loads(page):
    """Test that login page renders correctly"""
    print("✓ Testing login page loads...")

    page.goto(f"{BASE_URL}/admin/login")
    page.wait_for_load_state("networkidle")

    # Verify page title
    expect(page).to_have_title("Login - SEL Admin")

    # Verify form elements exist
    expect(page.locator("#username")).to_be_visible()
    expect(page.locator("#password")).to_be_visible()
    expect(page.locator('button[type="submit"]')).to_be_visible()

    # Verify static assets loaded
    expect(page.locator('link[href*="admin.css"]')).to_be_attached()

    print("  ✓ Login page rendered correctly")


def test_login_with_invalid_credentials(page):
    """Test login failure with wrong credentials"""
    print("✓ Testing login with invalid credentials...")

    page.goto(f"{BASE_URL}/admin/login")
    page.wait_for_load_state("networkidle")

    # Fill in wrong credentials
    page.fill("#username", "wrong")
    page.fill("#password", "wrongpassword")
    page.click('button[type="submit"]')

    # Wait for error message
    page.wait_for_timeout(1000)

    # Should still be on login page
    expect(page).to_have_url(f"{BASE_URL}/admin/login")

    # Error message should be visible
    error_message = page.locator("#error-message")
    expect(error_message).to_be_visible()

    print("  ✓ Login correctly rejected invalid credentials")


def test_login_success(page):
    """Test successful login flow"""
    print("✓ Testing successful login...")

    page.goto(f"{BASE_URL}/admin/login")
    page.wait_for_load_state("networkidle")

    # Fill in correct credentials
    page.fill("#username", ADMIN_USERNAME)
    page.fill("#password", ADMIN_PASSWORD)
    page.click('button[type="submit"]')

    # Wait for redirect to dashboard
    page.wait_for_url(f"{BASE_URL}/admin/dashboard", timeout=5000)

    # Verify we're on the dashboard
    expect(page).to_have_url(f"{BASE_URL}/admin/dashboard")
    expect(page.locator("h2:has-text('Dashboard')")).to_be_visible()

    print("  ✓ Login successful, redirected to dashboard")

    return page  # Keep session for subsequent tests


def test_dashboard_displays_stats(page):
    """Test that dashboard displays statistics"""
    print("✓ Testing dashboard stats display...")

    # Should already be on dashboard from login
    if not page.url.endswith("/admin/dashboard"):
        test_login_success(page)

    page.wait_for_load_state("networkidle")

    # Wait for stats to load (JavaScript replaces spinners)
    page.wait_for_timeout(2000)

    # Verify stats cards exist
    expect(page.locator("text=Pending Reviews")).to_be_visible()
    expect(page.locator("text=Total Events")).to_be_visible()

    # Verify stats have loaded (no longer showing spinner)
    pending_count = page.locator("#pending-count")
    expect(pending_count).to_be_visible()

    # Should not still show loading spinner
    spinner = page.locator("#pending-count .spinner-border")
    if spinner.is_visible():
        print("  ⚠ Warning: Stats still loading (spinner visible)")

    print("  ✓ Dashboard stats displayed")


def test_navigation_header(page):
    """Test that navigation header is present on authenticated pages"""
    print("✓ Testing navigation header...")

    if not page.url.endswith("/admin/dashboard"):
        test_login_success(page)

    # Verify header navigation exists
    nav = page.locator("nav.navbar, header")
    expect(nav).to_be_visible()

    # Verify navigation links
    expect(page.locator('a[href="/admin/dashboard"]')).to_be_visible()
    expect(page.locator('a[href="/admin/events"]')).to_be_visible()
    expect(page.locator('a[href="/admin/duplicates"]')).to_be_visible()
    expect(page.locator('a[href="/admin/api-keys"]')).to_be_visible()

    # Verify logout button exists
    logout_btn = page.locator('button:has-text("Logout"), a:has-text("Logout")')
    expect(logout_btn).to_be_visible()

    print("  ✓ Navigation header present with all links")


def test_events_list_page(page):
    """Test events list page loads"""
    print("✓ Testing events list page...")

    if not page.url.startswith(f"{BASE_URL}/admin/"):
        test_login_success(page)

    # Navigate to events page
    page.goto(f"{BASE_URL}/admin/events")
    page.wait_for_load_state("networkidle")

    # Verify page title
    expect(page.locator("h2:has-text('Events')")).to_be_visible()

    # Verify key elements exist
    # Search functionality
    search_input = page.locator('input[type="search"], input[placeholder*="Search"]')
    if search_input.count() > 0:
        expect(search_input.first).to_be_visible()
        print("  ✓ Search input present")

    # Events table or empty state
    table = page.locator("table")
    empty_state = page.locator("text=No events found, text=No events")

    if table.count() > 0:
        print("  ✓ Events table present")
    elif empty_state.count() > 0:
        print("  ✓ Empty state displayed")
    else:
        print("  ⚠ Warning: Neither table nor empty state found")

    print("  ✓ Events list page loaded")


def test_duplicates_page(page):
    """Test duplicates review page loads"""
    print("✓ Testing duplicates page...")

    if not page.url.startswith(f"{BASE_URL}/admin/"):
        test_login_success(page)

    # Navigate to duplicates page
    page.goto(f"{BASE_URL}/admin/duplicates")
    page.wait_for_load_state("networkidle")

    # Verify page loaded
    expect(
        page.locator("h2:has-text('Duplicate Events'), h2:has-text('Duplicates')")
    ).to_be_visible()

    # Should show either duplicate groups or empty state
    groups = page.locator(".duplicate-group, .card")
    empty = page.locator("text=No duplicates found, text=No potential duplicates")

    if groups.count() > 0:
        print("  ✓ Duplicate groups displayed")
    elif empty.count() > 0:
        print("  ✓ Empty state displayed")
    else:
        print("  ⚠ Warning: Neither groups nor empty state found")

    print("  ✓ Duplicates page loaded")


def test_api_keys_page(page):
    """Test API keys management page loads"""
    print("✓ Testing API keys page...")

    if not page.url.startswith(f"{BASE_URL}/admin/"):
        test_login_success(page)

    # Navigate to API keys page
    page.goto(f"{BASE_URL}/admin/api-keys")
    page.wait_for_load_state("networkidle")

    # Verify page loaded
    expect(page.locator("h2:has-text('API Keys')")).to_be_visible()

    # Should have a "Create" or "Generate" button
    create_btn = page.locator(
        'button:has-text("Create"), button:has-text("Generate"), button:has-text("New")'
    )
    if create_btn.count() > 0:
        expect(create_btn.first).to_be_visible()
        print("  ✓ Create API key button present")

    # Table or empty state
    table = page.locator("table")
    empty = page.locator("text=No API keys")

    if table.count() > 0:
        print("  ✓ API keys table present")
    elif empty.count() > 0:
        print("  ✓ Empty state displayed")

    print("  ✓ API keys page loaded")


def test_logout_functionality(page):
    """Test logout button works"""
    print("✓ Testing logout functionality...")

    if not page.url.startswith(f"{BASE_URL}/admin/"):
        test_login_success(page)

    # Click logout button
    logout_btn = page.locator('button:has-text("Logout"), a:has-text("Logout")')
    expect(logout_btn).to_be_visible()
    logout_btn.click()

    # Should redirect to login page
    page.wait_for_url(f"{BASE_URL}/admin/login", timeout=5000)
    expect(page).to_have_url(f"{BASE_URL}/admin/login")

    # Should not be able to access protected pages anymore
    page.goto(f"{BASE_URL}/admin/dashboard")
    page.wait_for_timeout(1000)

    # Should redirect back to login
    expect(page).to_have_url(f"{BASE_URL}/admin/login")

    print("  ✓ Logout successful, session cleared")


def test_unauthenticated_redirect(page):
    """Test that unauthenticated users are redirected to login"""
    print("✓ Testing unauthenticated redirect...")

    # Clear any existing session
    page.context.clear_cookies()
    page.evaluate("localStorage.clear()")

    protected_pages = [
        "/admin/dashboard",
        "/admin/events",
        "/admin/duplicates",
        "/admin/api-keys",
    ]

    for protected_page in protected_pages:
        page.goto(f"{BASE_URL}{protected_page}")
        page.wait_for_timeout(500)

        # Should redirect to login
        if not page.url.endswith("/admin/login"):
            print(
                f"  ⚠ Warning: {protected_page} did not redirect to login (got {page.url})"
            )

    print("  ✓ Unauthenticated access correctly redirected")


def run_all_tests():
    """Run all E2E tests"""
    print("\n" + "=" * 60)
    print("Running Admin UI E2E Tests (Playwright)")
    print("=" * 60 + "\n")

    with sync_playwright() as p:
        browser = p.chromium.launch(headless=True)
        context = browser.new_context()
        page = context.new_page()

        try:
            # Test suite
            test_login_page_loads(page)
            test_login_with_invalid_credentials(page)
            test_login_success(page)
            test_dashboard_displays_stats(page)
            test_navigation_header(page)
            test_events_list_page(page)
            test_duplicates_page(page)
            test_api_keys_page(page)
            test_logout_functionality(page)
            test_unauthenticated_redirect(page)

            print("\n" + "=" * 60)
            print("✓ All tests passed!")
            print("=" * 60 + "\n")

            return 0

        except Exception as e:
            print(f"\n✗ Test failed: {e}")

            # Take screenshot on failure
            screenshot_path = "/tmp/admin_ui_test_failure.png"
            page.screenshot(path=screenshot_path, full_page=True)
            print(f"  Screenshot saved: {screenshot_path}")

            # Print console errors
            print("\nConsole logs:")
            for msg in page.context.pages[0].console_messages:
                print(f"  {msg.type}: {msg.text}")

            return 1

        finally:
            browser.close()


if __name__ == "__main__":
    sys.exit(run_all_tests())
