#!/usr/bin/env python3
"""
Test cursor behavior in review queue - pointer only on event row, not on details.
Tests fix for srv-252: Entire details section should allow text selection.
"""

import os
import sys
from playwright.sync_api import sync_playwright, expect


def test_review_queue_cursor():
    """
    Verify cursor is pointer on event row but default on expanded details.
    """
    with sync_playwright() as p:
        browser = p.chromium.launch(headless=True)
        page = browser.new_page()

        # Get credentials from environment
        admin_user = os.environ.get("ADMIN_USERNAME", "admin")
        admin_pass = os.environ.get("ADMIN_PASSWORD", "admin")
        base_url = os.environ.get("BASE_URL", "http://localhost:8080")

        try:
            # Login
            print("🔐 Logging in to admin...")
            page.goto(f"{base_url}/admin/login")
            page.fill('input[name="username"]', admin_user)
            page.fill('input[name="password"]', admin_pass)
            page.click('button[type="submit"]')
            page.wait_for_url(f"{base_url}/admin/dashboard", timeout=5000)
            print("✓ Login successful")

            # Navigate to review queue
            print("📋 Navigating to review queue...")
            page.goto(f"{base_url}/admin/review-queue")
            page.wait_for_load_state("networkidle")

            # Wait for table to load (either with data or empty state)
            try:
                page.wait_for_selector(
                    "#review-queue-table tr[data-entry-id]", timeout=3000
                )
                has_entries = True
                print("✓ Review queue has entries")
            except:
                has_entries = False
                print("ℹ Review queue is empty (no entries to test)")
                browser.close()
                return

            if not has_entries:
                print("⚠ Cannot test cursor behavior without entries")
                browser.close()
                return

            # Get first entry row
            first_row = page.locator("#review-queue-table tr[data-entry-id]").first
            entry_id = first_row.get_attribute("data-entry-id")
            print(f"📌 Testing entry ID: {entry_id}")

            # Check cursor on event row (should be pointer)
            print("🖱️ Checking cursor on event row...")
            row_cursor = first_row.evaluate(
                "(el) => window.getComputedStyle(el).cursor"
            )
            print(f"  Event row cursor: {row_cursor}")
            assert row_cursor == "pointer", (
                f"Event row should have pointer cursor, got: {row_cursor}"
            )
            print("✓ Event row has pointer cursor")

            # Expand the detail by clicking the row
            print("🔽 Expanding detail card...")
            first_row.click()

            # Wait for detail row to appear
            detail_row = page.locator(f"#detail-{entry_id}")
            expect(detail_row).to_be_visible(timeout=5000)
            print("✓ Detail card expanded")

            # Check cursor on detail row (should be default, not pointer)
            print("🖱️ Checking cursor on detail card...")
            detail_cursor = detail_row.evaluate(
                "(el) => window.getComputedStyle(el).cursor"
            )
            print(f"  Detail row cursor: {detail_cursor}")
            assert detail_cursor in ["default", "auto", "text"], (
                f"Detail row should have default/auto/text cursor (not pointer), got: {detail_cursor}"
            )
            print("✓ Detail row has default cursor (allows text selection)")

            # Check cursor on text content within detail card
            print("🖱️ Checking cursor on detail card text content...")
            detail_text = detail_row.locator(".card-body").first
            text_cursor = detail_text.evaluate(
                "(el) => window.getComputedStyle(el).cursor"
            )
            print(f"  Detail text cursor: {text_cursor}")
            assert text_cursor in ["default", "auto", "text"], (
                f"Detail text should have default/auto/text cursor (not pointer), got: {text_cursor}"
            )
            print("✓ Detail text has default cursor (text selection enabled)")

            # Take a screenshot for manual verification
            screenshot_path = "/tmp/review_queue_cursor_test.png"
            page.screenshot(path=screenshot_path, full_page=True)
            print(f"📸 Screenshot saved: {screenshot_path}")

            print("\n✅ All cursor behavior tests passed!")
            print("   - Event row: pointer cursor (clickable)")
            print("   - Detail card: default cursor (text selectable)")

        except Exception as e:
            print(f"\n❌ Test failed: {e}")
            screenshot_path = "/tmp/review_queue_cursor_test_error.png"
            page.screenshot(path=screenshot_path, full_page=True)
            print(f"📸 Error screenshot saved: {screenshot_path}")
            raise
        finally:
            browser.close()


if __name__ == "__main__":
    # Load .env file if it exists
    env_file = os.path.join(os.path.dirname(__file__), "..", "..", ".env")
    if os.path.exists(env_file):
        print(f"📁 Loading environment from {env_file}")
        with open(env_file) as f:
            for line in f:
                line = line.strip()
                if line and not line.startswith("#") and "=" in line:
                    key, value = line.split("=", 1)
                    # Remove quotes if present
                    value = value.strip('"').strip("'")
                    os.environ[key] = value

    try:
        test_review_queue_cursor()
        sys.exit(0)
    except Exception as e:
        print(f"\n❌ Test failed with error: {e}")
        sys.exit(1)
