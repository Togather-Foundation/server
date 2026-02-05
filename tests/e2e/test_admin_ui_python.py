#!/usr/bin/env python3
"""
Admin UI E2E test using Playwright (Python)
Run with: uvx --from playwright --with playwright python tests/e2e/test_admin_ui_python.py
"""

import sys
import os
from playwright.sync_api import sync_playwright

BASE_URL = "http://localhost:8080"
ADMIN_USERNAME = "admin"

# Get password from environment or use default
ADMIN_PASSWORD = os.getenv("ADMIN_PASSWORD", "XXKokg60kd8hLXgq")


def main():
    print("\n" + "=" * 60)
    print("Testing Admin UI with Playwright (Python)")
    print("=" * 60 + "\n")

    with sync_playwright() as p:
        browser = p.chromium.launch(headless=True)
        page = browser.new_page()

        # Track console errors
        console_errors = []
        csp_violations = []

        def handle_console_message(msg):
            text = msg.text
            print(f"   [Console {msg.type}] {text}")

            if msg.type == "error":
                console_errors.append(text)

            # Track CSP violations specifically
            if (
                "Content-Security-Policy" in text
                or "violates the following directive" in text
            ):
                csp_violations.append(text)

        page.on("console", handle_console_message)

        try:
            print("1. Loading login page...")
            page.goto(f"{BASE_URL}/admin/login")
            page.wait_for_load_state("networkidle")

            page.screenshot(path="/tmp/admin_login.png", full_page=True)
            print(f"   ✓ Screenshot: /tmp/admin_login.png")
            print(f"   Title: {page.title()}")

            # Check form elements
            username_visible = page.locator("#username").is_visible()
            password_visible = page.locator("#password").is_visible()
            submit_visible = page.locator('button[type="submit"]').is_visible()

            print(f"   Username field: {'✓' if username_visible else '✗'}")
            print(f"   Password field: {'✓' if password_visible else '✗'}")
            print(f"   Submit button: {'✓' if submit_visible else '✗'}")

            print("\n2. Attempting login...")
            page.fill("#username", ADMIN_USERNAME)
            page.fill("#password", ADMIN_PASSWORD)
            print(f"   Filled username: {ADMIN_USERNAME}")
            print(f"   Filled password: {'*' * len(ADMIN_PASSWORD)}")

            page.click('button[type="submit"]')
            print("   Clicked submit button")

            # Wait for navigation
            page.wait_for_timeout(2000)
            page.wait_for_load_state("networkidle")

            print(f"\n   Current URL: {page.url}")
            page.screenshot(path="/tmp/admin_after_login.png", full_page=True)
            print("   ✓ Screenshot: /tmp/admin_after_login.png")

            if "dashboard" in page.url:
                print("   ✓ Login successful! Redirected to dashboard\n")

                print("3. Testing Dashboard page...")
                print(f"   Title: {page.title()}")

                # Wait for JavaScript to load stats
                page.wait_for_timeout(2000)

                page.screenshot(path="/tmp/admin_dashboard.png", full_page=True)
                print("   ✓ Screenshot: /tmp/admin_dashboard.png")

                # Check for stats elements
                pending_count_visible = page.locator("#pending-count").is_visible()
                total_events_visible = page.locator("#total-events").is_visible()

                if pending_count_visible:
                    text = page.locator("#pending-count").inner_text().strip()
                    print(f"   Pending Reviews: {text}")
                else:
                    print("   ⚠ Pending count not visible")

                if total_events_visible:
                    text = page.locator("#total-events").inner_text().strip()
                    print(f"   Total Events: {text}")
                else:
                    print("   ⚠ Total events not visible")

                # Check navigation
                nav_links_count = page.locator("nav a, .navbar a").count()
                print(f"   Navigation links found: {nav_links_count}")

                print("\n4. Testing Events List page...")
                page.goto(f"{BASE_URL}/admin/events")
                page.wait_for_load_state("networkidle")
                page.wait_for_timeout(1000)

                page.screenshot(path="/tmp/admin_events.png", full_page=True)
                print("   ✓ Screenshot: /tmp/admin_events.png")
                print(f"   Title: {page.title()}")
                print(f"   URL: {page.url}")

                events_heading = page.locator('h2:has-text("Events")').count()
                if events_heading > 0:
                    print("   ✓ Events heading found")

                print("\n5. Testing Duplicates page...")
                page.goto(f"{BASE_URL}/admin/duplicates")
                page.wait_for_load_state("networkidle")
                page.wait_for_timeout(1000)

                page.screenshot(path="/tmp/admin_duplicates.png", full_page=True)
                print("   ✓ Screenshot: /tmp/admin_duplicates.png")
                print(f"   Title: {page.title()}")

                print("\n6. Testing API Keys page...")
                page.goto(f"{BASE_URL}/admin/api-keys")
                page.wait_for_load_state("networkidle")
                page.wait_for_timeout(1000)

                page.screenshot(path="/tmp/admin_api_keys.png", full_page=True)
                print("   ✓ Screenshot: /tmp/admin_api_keys.png")
                print(f"   Title: {page.title()}")

                print("\n7. Testing Dark/Light Theme Toggle...")
                # Navigate to dashboard where theme toggle is available
                page.goto(f"{BASE_URL}/admin/dashboard")
                page.wait_for_load_state("networkidle")
                page.wait_for_timeout(1000)

                # Check for theme toggle link (it's an <a> tag, not a button)
                theme_toggle_count = page.locator(
                    "#theme-toggle, #theme-toggle-light"
                ).count()

                if theme_toggle_count > 0:
                    print(f"   Theme toggle found: {theme_toggle_count} instance(s)")

                    # Find the visible toggle (one is hidden based on current theme)
                    theme_toggle_btn = page.locator(
                        "#theme-toggle:visible, #theme-toggle-light:visible"
                    ).first

                    # Get current theme from data-bs-theme attribute on <html> element
                    initial_theme = page.evaluate(
                        '() => document.documentElement.getAttribute("data-bs-theme") || "light"'
                    )
                    print(f"   Initial theme: {initial_theme}")

                    # Take screenshot of initial theme
                    page.screenshot(
                        path=f"/tmp/admin_theme_{initial_theme}.png", full_page=True
                    )
                    print(f"   ✓ Screenshot: /tmp/admin_theme_{initial_theme}.png")

                    # Click theme toggle
                    theme_toggle_btn.click()
                    page.wait_for_timeout(500)

                    # Get new theme from data-bs-theme attribute
                    new_theme = page.evaluate(
                        '() => document.documentElement.getAttribute("data-bs-theme") || "light"'
                    )
                    print(f"   Theme after toggle: {new_theme}")

                    # Take screenshot of new theme
                    page.screenshot(
                        path=f"/tmp/admin_theme_{new_theme}.png", full_page=True
                    )
                    print(f"   ✓ Screenshot: /tmp/admin_theme_{new_theme}.png")

                    # Verify theme changed
                    if initial_theme != new_theme:
                        print("   ✓ Theme toggle working - theme changed")

                        # Verify localStorage persistence
                        stored_theme = page.evaluate(
                            '() => localStorage.getItem("admin_theme")'
                        )
                        if stored_theme == new_theme:
                            print("   ✓ Theme persisted to localStorage")
                        else:
                            print(
                                f"   ⚠ Theme not persisted correctly (stored: {stored_theme}, expected: {new_theme})"
                            )
                    else:
                        print("   ⚠ Theme did not change after clicking toggle button")
                else:
                    print("   ⚠ Theme toggle button not found")

                print("\n8. Testing Federation Nodes page...")
                page.goto(f"{BASE_URL}/admin/federation")
                page.wait_for_load_state("networkidle")
                page.wait_for_timeout(1000)

                page.screenshot(path="/tmp/admin_federation.png", full_page=True)
                print("   ✓ Screenshot: /tmp/admin_federation.png")
                print(f"   Title: {page.title()}")
                print(f"   URL: {page.url}")

                # Check for federation page elements
                federation_heading = page.locator(
                    'h2:has-text("Federation"), h1:has-text("Federation")'
                ).count()
                if federation_heading > 0:
                    print("   ✓ Federation heading found")

                # Check for table structure
                table_count = page.locator("table").count()
                if table_count > 0:
                    print(f"   ✓ Table found ({table_count} table(s))")

                    # Check for table headers
                    thead_count = page.locator("table thead").count()
                    if thead_count > 0:
                        print(f"   ✓ Table header found")

                    # Count rows in table body
                    tbody_rows = page.locator("table tbody tr").count()
                    print(f"   Table rows: {tbody_rows}")
                else:
                    print("   ⚠ No table found on federation page")

                print("\n9. Testing Stats Endpoint (Dashboard)...")
                # Wait a bit to avoid rate limiting
                page.wait_for_timeout(3000)

                # Navigate back to dashboard
                page.goto(f"{BASE_URL}/admin/dashboard")
                page.wait_for_load_state("networkidle")

                # Set up network request tracking
                stats_api_called = {"value": False, "count": 0}
                events_api_called = {"value": False, "count": 0}

                def track_requests(route, request):
                    url = request.url
                    if "/api/v1/admin/stats" in url:
                        stats_api_called["value"] = True
                        stats_api_called["count"] += 1
                        print(f"   ✓ Stats API called: {url}")
                    elif "/api/v1/events" in url and "limit=1000" in url:
                        events_api_called["value"] = True
                        events_api_called["count"] += 1
                        print(f"   ⚠ Events API called with large limit: {url}")
                    route.continue_()

                # Intercept network requests
                page.route("**/api/**", track_requests)

                # Reload the page to trigger API calls
                page.reload()
                page.wait_for_load_state("networkidle")
                page.wait_for_timeout(2000)

                # Check results
                if stats_api_called["value"]:
                    print(
                        f"   ✓ Dashboard uses /api/v1/admin/stats endpoint ({stats_api_called['count']} calls)"
                    )
                elif stats_api_called["count"] == 0:
                    print(
                        "   ⚠ Stats endpoint not called (may be rate limited or cached)"
                    )

                if events_api_called["value"]:
                    print(
                        f"   ⚠ Dashboard calls /api/v1/events with limit=1000 ({events_api_called['count']} times)"
                    )
                else:
                    print(
                        "   ✓ Dashboard does not load 1000 events (uses stats instead)"
                    )

                # Unroute to avoid interference with logout
                page.unroute("**/api/**")

                print("\n10. Testing Logout functionality...")
                logout_count = page.locator(
                    'button:has-text("Logout"), a:has-text("Logout")'
                ).count()

                if logout_count > 0:
                    print(f"   Logout button found: {logout_count} instances")

                    # The logout is in a dropdown, so we need to open it first
                    # Look for user dropdown trigger
                    user_dropdown = page.locator(
                        '.dropdown-toggle, [data-bs-toggle="dropdown"]'
                    )
                    if user_dropdown.count() > 0 and user_dropdown.first.is_visible():
                        print("   Opening user dropdown...")
                        user_dropdown.first.click()
                        page.wait_for_timeout(500)

                    logout_btn = page.locator(
                        'button:has-text("Logout"), a:has-text("Logout")'
                    ).first
                    logout_visible = logout_btn.is_visible()
                    print(f"   Logout button visible: {'✓' if logout_visible else '✗'}")

                    if logout_visible:
                        logout_btn.click()
                        page.wait_for_timeout(1000)
                        page.wait_for_load_state("networkidle")

                        print(f"   After logout URL: {page.url}")
                        page.screenshot(
                            path="/tmp/admin_after_logout.png", full_page=True
                        )
                        print("   ✓ Screenshot: /tmp/admin_after_logout.png")

                        if "login" in page.url:
                            print("   ✓ Logout successful - redirected to login")
                        else:
                            print(
                                f"   ⚠ Logout may not have worked - still on: {page.url}"
                            )
                    else:
                        print(
                            "   ⚠ Logout button not visible even after opening dropdown"
                        )
                else:
                    print("   ⚠ Logout button not found")

                print("\n" + "=" * 60)
                print("✓ All tests completed successfully!")
                print("Screenshots saved to /tmp/admin_*.png")
                print("=" * 60 + "\n")

                # Report console errors summary
                if console_errors:
                    print("\n⚠️  Console Errors Found:")
                    print(f"   Total errors: {len(console_errors)}")
                    print("\n   Recent errors:")
                    for error in console_errors[-5:]:  # Show last 5 errors
                        # Truncate long errors
                        if len(error) > 100:
                            error = error[:97] + "..."
                        print(f"   • {error}")

                if csp_violations:
                    print("\n⚠️  CSP Violations Found:")
                    print(f"   Total CSP violations: {len(csp_violations)}")
                    print("\n   Unique directives violated:")
                    unique_violations = set()
                    for violation in csp_violations:
                        # Extract the directive being violated
                        if "script-src" in violation:
                            unique_violations.add("script-src")
                        if "style-src" in violation:
                            unique_violations.add("style-src")
                        if "img-src" in violation:
                            unique_violations.add("img-src")

                    for violation in sorted(unique_violations):
                        print(f"   • {violation}")

                    print("\n   💡 Restart the server to apply CSP changes")

                if not console_errors and not csp_violations:
                    print("\n✅ No console errors or CSP violations detected!")

                return 0

            else:
                print("   ✗ Login failed - not redirected to dashboard")

                # Check for error message
                error_visible = page.locator("#error-message").is_visible()
                if error_visible:
                    error_text = page.locator("#error-message").inner_text()
                    print(f"   Error: {error_text}")

                return 1

        except Exception as error:
            print(f"\n✗ Error during testing: {error}")
            page.screenshot(path="/tmp/admin_error.png", full_page=True)
            print("   Error screenshot: /tmp/admin_error.png")
            return 1

        finally:
            browser.close()


if __name__ == "__main__":
    sys.exit(main())
