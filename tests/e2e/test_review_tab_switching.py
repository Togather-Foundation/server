"""
E2E test for review queue tab switching functionality.
Tests that tabs can be clicked and switched between without errors.
"""

import os
from playwright.sync_api import sync_playwright, expect


def test_tab_switching():
    """Test that clicking between tabs works without JavaScript errors"""
    admin_password = os.getenv("ADMIN_PASSWORD", "XXKokg60kd8hLXgq")

    with sync_playwright() as p:
        browser = p.chromium.launch(headless=True)
        context = browser.new_context(viewport={"width": 1280, "height": 720})
        page = context.new_page()

        # Capture console errors (filter out expected X-Frame-Options warnings)
        console_errors = []

        def capture_error(msg):
            if msg.type == "error" and "X-Frame-Options" not in msg.text:
                console_errors.append(msg.text)

        page.on("console", capture_error)

        try:
            # Login
            page.goto("http://localhost:8080/admin/login")
            page.fill("#username", "admin")
            page.fill("#password", admin_password)
            page.click('button[type="submit"]')
            page.wait_for_url("**/admin/dashboard", timeout=5000)

            # Navigate to review queue
            page.goto("http://localhost:8080/admin/review-queue")
            page.wait_for_selector('[data-action="filter-status"]', timeout=10000)

            # Wait for page to fully load and tabs to be interactive
            page.wait_for_load_state("networkidle")

            # Verify no console errors on page load
            if console_errors:
                print(f"❌ Console errors on page load: {console_errors}")
                assert False, f"Console errors found on load: {console_errors}"

            print("✅ Page loaded without errors")

            # Get all tab links
            pending_tab = page.locator(
                '[data-action="filter-status"][data-status="pending"]'
            )
            approved_tab = page.locator(
                '[data-action="filter-status"][data-status="approved"]'
            )
            rejected_tab = page.locator(
                '[data-action="filter-status"][data-status="rejected"]'
            )

            # Test clicking Approved tab
            print("Testing Approved tab click...")
            approved_tab.click()
            # Wait for tab to become active after click
            expect(approved_tab).to_have_class("nav-link active", timeout=2000)

            if console_errors:
                print(f"❌ Console errors after clicking Approved: {console_errors}")
                assert False, f"Console errors after Approved click: {console_errors}"

            # Verify Approved tab is active
            expect(approved_tab).to_have_class("nav-link active")
            print("✅ Approved tab activated successfully")

            # Test clicking Rejected tab
            print("Testing Rejected tab click...")
            rejected_tab.click()
            # Wait for tab to become active after click
            expect(rejected_tab).to_have_class("nav-link active", timeout=2000)

            if console_errors:
                print(f"❌ Console errors after clicking Rejected: {console_errors}")
                assert False, f"Console errors after Rejected click: {console_errors}"

            # Verify Rejected tab is active
            expect(rejected_tab).to_have_class("nav-link active")
            print("✅ Rejected tab activated successfully")

            # Test clicking back to Pending tab
            print("Testing Pending tab click...")
            pending_tab.click()
            # Wait for tab to become active after click
            expect(pending_tab).to_have_class("nav-link active", timeout=2000)

            if console_errors:
                print(f"❌ Console errors after clicking Pending: {console_errors}")
                assert False, f"Console errors after Pending click: {console_errors}"

            # Verify Pending tab is active
            expect(pending_tab).to_have_class("nav-link active")
            print("✅ Pending tab activated successfully")

            # Take screenshot
            page.screenshot(path="/tmp/tab_switching_success.png")
            print("✅ Screenshot saved to /tmp/tab_switching_success.png")

            print("\n✅ All tab switching tests passed!")

        finally:
            browser.close()


if __name__ == "__main__":
    test_tab_switching()
