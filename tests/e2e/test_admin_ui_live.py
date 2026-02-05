#!/usr/bin/env python3
"""
Interactive admin UI testing with Playwright
Tests the live admin UI on localhost:8080
"""

from playwright.sync_api import sync_playwright
import sys
import time

BASE_URL = "http://localhost:8080"
ADMIN_USERNAME = "admin"
ADMIN_PASSWORD = "test123"


def main():
    print("\n" + "=" * 60)
    print("Testing Admin UI with Playwright")
    print("=" * 60 + "\n")

    with sync_playwright() as p:
        browser = p.chromium.launch(headless=True)
        page = browser.new_page()

        # Enable console logging
        page.on("console", lambda msg: print(f"   [Console {msg.type}] {msg.text}"))

        try:
            print("1. Loading login page...")
            page.goto(f"{BASE_URL}/admin/login")
            page.wait_for_load_state("networkidle")

            page.screenshot(path="/tmp/admin_login.png", full_page=True)
            print(f"   ✓ Screenshot: /tmp/admin_login.png")
            print(f"   Title: {page.title()}")

            # Check form elements
            username_field = page.locator("#username")
            password_field = page.locator("#password")
            submit_button = page.locator('button[type="submit"]')

            print(f"   Username field: {'✓' if username_field.is_visible() else '✗'}")
            print(f"   Password field: {'✓' if password_field.is_visible() else '✗'}")
            print(f"   Submit button: {'✓' if submit_button.is_visible() else '✗'}")

            print("\n2. Attempting login with credentials...")
            page.fill("#username", ADMIN_USERNAME)
            page.fill("#password", ADMIN_PASSWORD)
            print(f"   Filled username: {ADMIN_USERNAME}")
            print(f"   Filled password: {'*' * len(ADMIN_PASSWORD)}")

            page.click('button[type="submit"]')
            print("   Clicked submit button")

            # Wait for response
            time.sleep(2)
            page.wait_for_load_state("networkidle")

            print(f"\n   Current URL: {page.url}")
            page.screenshot(path="/tmp/admin_after_login.png", full_page=True)
            print(f"   ✓ Screenshot: /tmp/admin_after_login.png")

            # Check if login succeeded
            if "dashboard" in page.url:
                print("   ✓ Login successful! Redirected to dashboard\n")

                print("3. Testing Dashboard page...")
                print(f"   Title: {page.title()}")

                # Wait for JavaScript to load stats
                time.sleep(2)

                page.screenshot(path="/tmp/admin_dashboard.png", full_page=True)
                print(f"   ✓ Screenshot: /tmp/admin_dashboard.png")

                # Check for stats elements
                pending_count = page.locator("#pending-count")
                total_events = page.locator("#total-events")

                if pending_count.is_visible():
                    text = pending_count.inner_text().strip()
                    print(f"   Pending Reviews: {text}")
                else:
                    print("   ⚠ Pending count not visible")

                if total_events.is_visible():
                    text = total_events.inner_text().strip()
                    print(f"   Total Events: {text}")
                else:
                    print("   ⚠ Total events not visible")

                # Check for navigation
                nav_links = page.locator("nav a, .navbar a")
                print(f"   Navigation links found: {nav_links.count()}")

                print("\n4. Testing Events List page...")
                page.goto(f"{BASE_URL}/admin/events")
                page.wait_for_load_state("networkidle")
                time.sleep(1)

                page.screenshot(path="/tmp/admin_events.png", full_page=True)
                print(f"   ✓ Screenshot: /tmp/admin_events.png")
                print(f"   Title: {page.title()}")
                print(f"   URL: {page.url}")

                # Check page content
                if page.locator("h2:has-text('Events')").count() > 0:
                    print("   ✓ Events heading found")

                print("\n5. Testing Duplicates page...")
                page.goto(f"{BASE_URL}/admin/duplicates")
                page.wait_for_load_state("networkidle")
                time.sleep(1)

                page.screenshot(path="/tmp/admin_duplicates.png", full_page=True)
                print(f"   ✓ Screenshot: /tmp/admin_duplicates.png")
                print(f"   Title: {page.title()}")

                print("\n6. Testing API Keys page...")
                page.goto(f"{BASE_URL}/admin/api-keys")
                page.wait_for_load_state("networkidle")
                time.sleep(1)

                page.screenshot(path="/tmp/admin_api_keys.png", full_page=True)
                print(f"   ✓ Screenshot: /tmp/admin_api_keys.png")
                print(f"   Title: {page.title()}")

                print("\n7. Testing Logout functionality...")
                logout_btn = page.locator(
                    'button:has-text("Logout"), a:has-text("Logout")'
                )

                if logout_btn.count() > 0:
                    print(f"   Logout button found: {logout_btn.count()} instances")
                    print(
                        f"   Logout button visible: {'✓' if logout_btn.first.is_visible() else '✗'}"
                    )

                    logout_btn.first.click()
                    time.sleep(1)
                    page.wait_for_load_state("networkidle")

                    print(f"   After logout URL: {page.url}")
                    page.screenshot(path="/tmp/admin_after_logout.png", full_page=True)
                    print(f"   ✓ Screenshot: /tmp/admin_after_logout.png")

                    if "login" in page.url:
                        print("   ✓ Logout successful - redirected to login")
                    else:
                        print("   ⚠ Logout may not have worked - still on:", page.url)
                else:
                    print("   ⚠ Logout button not found")

                print("\n" + "=" * 60)
                print("✓ All tests completed successfully!")
                print("Screenshots saved to /tmp/admin_*.png")
                print("=" * 60 + "\n")

                return 0

            else:
                print("   ✗ Login failed - not redirected to dashboard")

                # Check for error messages
                error_msg = page.locator("#error-message")
                if error_msg.is_visible():
                    print(f"   Error: {error_msg.inner_text()}")

                # Check page content
                print(f"\n   Page HTML (first 500 chars):")
                print(f"   {page.content()[:500]}")

                return 1

        except Exception as e:
            print(f"\n✗ Error during testing: {e}")
            page.screenshot(path="/tmp/admin_error.png", full_page=True)
            print(f"   Error screenshot: /tmp/admin_error.png")
            return 1

        finally:
            browser.close()


if __name__ == "__main__":
    sys.exit(main())
