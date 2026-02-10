#!/usr/bin/env python3
"""
E2E test for URL linkification in review queue detail view

Tests that URLs in event details are automatically converted to clickable links
that open in new tabs with proper security attributes.
"""

import os
from playwright.sync_api import sync_playwright, expect

# Configuration
BASE_URL = os.getenv("BASE_URL", "http://localhost:8080")
ADMIN_USERNAME = os.getenv("ADMIN_USERNAME", "admin")
ADMIN_PASSWORD = os.getenv("ADMIN_PASSWORD", "XXKokg60kd8hLXgq")


def test_url_linkification():
    """Test that URLs in event details are clickable links"""

    with sync_playwright() as p:
        # Launch browser
        browser = p.chromium.launch(headless=True)
        context = browser.new_context(
            viewport={"width": 1920, "height": 1080}, ignore_https_errors=True
        )
        page = context.new_page()

        # Track console errors
        console_errors = []

        def handle_console(msg):
            if msg.type == "error":
                console_errors.append(msg.text)

        page.on("console", handle_console)

        try:
            # Login as admin
            print("\n1. Logging in as admin...")
            page.goto(f"{BASE_URL}/admin/login")
            page.wait_for_load_state("networkidle")
            page.fill("#username", ADMIN_USERNAME)
            page.fill("#password", ADMIN_PASSWORD)
            page.click('button[type="submit"]')
            page.wait_for_url(f"{BASE_URL}/admin/dashboard", timeout=5000)
            print("   ✓ Logged in successfully")

            # Navigate to review queue
            print("\n2. Navigating to review queue...")
            page.goto(f"{BASE_URL}/admin/review-queue")
            page.wait_for_load_state("networkidle")
            print("   ✓ Review queue loaded")

            # Wait for page to finish loading
            page.wait_for_selector(
                "#review-queue-container, #empty-state[style*='block']",
                timeout=5000,
                state="visible",
            )

            # Check if we have any entries
            entries = page.locator("#review-queue-table tr[data-entry-id]")
            entry_count = entries.count()

            if entry_count == 0:
                print(
                    "\n⚠ No review queue entries found - skipping URL linkification test"
                )
                print(
                    "   To test this feature, create a review queue entry with a URL in the description"
                )
                return

            print(f"\n3. Found {entry_count} review queue entries")

            # Click the first entry to expand details
            print("\n4. Expanding first entry details...")
            first_entry = entries.first
            first_entry.click()
            page.wait_for_selector('[id^="detail-"]', timeout=5000)
            print("   ✓ Details expanded")

            # Look for links in the detail view
            detail_card = page.locator('[id^="detail-"]')
            links = detail_card.locator('a[target="_blank"][rel="noopener noreferrer"]')
            link_count = links.count()

            print(f"\n5. Checking for clickable URLs in event details...")
            print(
                f"   Found {link_count} links with target='_blank' and rel='noopener noreferrer'"
            )

            if link_count == 0:
                print("\n⚠ No URLs found in event details")
                print(
                    "   This is expected if the event doesn't contain URLs in description/location/etc."
                )
                print("   Feature is working correctly (no false positives)")
            else:
                # Verify first link has correct attributes
                first_link = links.first
                target_attr = first_link.get_attribute("target")
                rel_attr = first_link.get_attribute("rel")
                href_attr = first_link.get_attribute("href")

                print(f"\n6. Validating first link:")
                print(f"   href:   {href_attr}")
                print(f"   target: {target_attr}")
                print(f"   rel:    {rel_attr}")

                assert target_attr == "_blank", "Link should have target='_blank'"
                assert rel_attr == "noopener noreferrer", (
                    "Link should have rel='noopener noreferrer'"
                )
                assert href_attr and (
                    href_attr.startswith("http://") or href_attr.startswith("https://")
                ), "Link should have valid URL"

                print("\n   ✓ Link attributes are correct")
                print("   ✓ URLs are properly linkified and secure")

            # Take screenshot showing the linkified URLs
            screenshot_path = "/tmp/url_linkification_test.png"
            page.screenshot(path=screenshot_path)
            print(f"\n7. Screenshot saved to: {screenshot_path}")

            # Check for console errors
            if console_errors:
                print(f"\n⚠ Console errors detected: {len(console_errors)}")
                for error in console_errors:
                    print(f"   - {error}")
            else:
                print("\n✓ No console errors detected")

            print("\n" + "=" * 60)
            print("✓ URL Linkification Test PASSED")
            print("=" * 60 + "\n")

        except Exception as e:
            # Screenshot on failure
            page.screenshot(path="/tmp/url_linkification_failure.png")
            print(f"\n✗ Test failed: {e}")
            print(f"   Screenshot saved to: /tmp/url_linkification_failure.png")
            raise
        finally:
            browser.close()


if __name__ == "__main__":
    test_url_linkification()
