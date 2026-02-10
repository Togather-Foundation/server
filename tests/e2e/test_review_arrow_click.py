#!/usr/bin/env -S uvx --from playwright --with playwright python
"""
E2E test: Verify arrow button toggles expand/collapse correctly

Tests that clicking the arrow button (▼/▲) properly toggles the detail view
without double-toggling due to event propagation conflicts.

Usage:
    python tests/e2e/test_review_arrow_click.py [--headed]
"""

import sys
import os
from pathlib import Path
from playwright.sync_api import sync_playwright, expect

# Add project root to path for shared utilities
sys.path.insert(0, str(Path(__file__).parent))


def test_arrow_click_toggle(base_url: str, admin_password: str, headed: bool = False):
    """Test that arrow button properly toggles expand/collapse"""
    with sync_playwright() as p:
        browser = p.chromium.launch(headless=not headed)
        context = browser.new_context()
        page = context.new_page()

        try:
            # Login
            page.goto(f"{base_url}/admin/login")
            page.fill('input[name="username"]', "admin")
            page.fill('input[name="password"]', admin_password)
            page.click('button[type="submit"]')
            page.wait_for_url(f"{base_url}/admin/dashboard")

            # Navigate to review queue
            page.goto(f"{base_url}/admin/review-queue")
            page.wait_for_selector("#review-queue-table")

            # Wait for entries to load
            page.wait_for_timeout(1000)

            # Check if we have any entries
            rows = page.locator("tr[data-entry-id]").count()
            if rows == 0:
                print("✓ No review queue entries to test (skipping arrow click test)")
                return True

            # Get first entry ID
            first_row = page.locator("tr[data-entry-id]").first
            entry_id = first_row.get_attribute("data-entry-id")
            arrow_button = first_row.locator(".expand-arrow")

            print(f"Testing arrow button for entry: {entry_id}")

            # Initial state: detail should not be visible
            detail_row = page.locator(f"#detail-{entry_id}")
            expect(detail_row).not_to_be_attached()
            print("✓ Detail initially collapsed")

            # Click arrow to expand
            arrow_button.click()
            page.wait_for_timeout(500)  # Wait for API call and render

            # Detail should now be visible
            expect(detail_row).to_be_attached()
            expect(detail_row).to_be_visible()

            # Arrow should point up
            arrow_icon = arrow_button.locator("polyline")
            points = arrow_icon.get_attribute("points")
            assert points == "6 15 12 9 18 15", (
                f"Arrow should point up, got points: {points}"
            )
            print("✓ Arrow click expanded detail (arrow points up)")

            # Click arrow again to collapse
            arrow_button.click()
            page.wait_for_timeout(300)

            # Detail should now be hidden
            expect(detail_row).not_to_be_attached()

            # Arrow should point down
            points = arrow_icon.get_attribute("points")
            assert points == "6 9 12 15 18 9", (
                f"Arrow should point down, got points: {points}"
            )
            print("✓ Arrow click collapsed detail (arrow points down)")

            # Click arrow once more to expand again
            arrow_button.click()
            page.wait_for_timeout(500)

            # Detail should be visible again
            expect(detail_row).to_be_attached()
            expect(detail_row).to_be_visible()
            print("✓ Arrow click re-expanded detail")

            # Verify it stays expanded (no double-toggle)
            page.wait_for_timeout(500)
            expect(detail_row).to_be_visible()
            print("✓ Detail remains expanded (no double-toggle)")

            return True

        except Exception as e:
            print(f"✗ Test failed: {e}")
            # Save screenshot for debugging
            screenshot_path = "/tmp/arrow_click_test_failure.png"
            page.screenshot(path=screenshot_path)
            print(f"Screenshot saved to: {screenshot_path}")
            return False
        finally:
            browser.close()


if __name__ == "__main__":
    # Get configuration from environment
    base_url = os.getenv("BASE_URL", "http://localhost:8080")
    admin_password = os.getenv("ADMIN_PASSWORD", "admin")
    headed = "--headed" in sys.argv

    print(f"\n=== Testing Arrow Button Toggle on {base_url} ===\n")
    success = test_arrow_click_toggle(base_url, admin_password, headed)
    sys.exit(0 if success else 1)
