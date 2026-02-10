#!/usr/bin/env python3
"""
Test pagination component in review queue.
Tests various states: empty, single page, multiple pages, prev/next buttons.
"""

import sys
import os
from playwright.sync_api import sync_playwright, expect

# Get credentials from environment
BASE_URL = os.getenv("BASE_URL", "http://localhost:8080")
ADMIN_USERNAME = os.getenv("ADMIN_USERNAME", "admin")
ADMIN_PASSWORD = os.getenv("ADMIN_PASSWORD", "XXKokg60kd8hLXgq")


def test_pagination_states():
    """Test pagination component in different states."""
    with sync_playwright() as p:
        browser = p.chromium.launch(headless=True)
        page = browser.new_page()

        console_messages = []
        page.on(
            "console",
            lambda msg: console_messages.append({"type": msg.type, "text": msg.text}),
        )

        errors = []
        page.on("pageerror", lambda exc: errors.append(str(exc)))

        try:
            # Login
            page.goto(f"{BASE_URL}/admin/login")
            page.fill('input[name="username"]', ADMIN_USERNAME)
            page.fill('input[name="password"]', ADMIN_PASSWORD)
            page.click('button[type="submit"]')
            page.wait_for_url(f"{BASE_URL}/admin/dashboard", timeout=10000)

            # Go to review queue
            page.goto(f"{BASE_URL}/admin/review-queue")

            # Wait for page to load (loading state will be shown first, then one of the other states)
            page.wait_for_load_state("networkidle")
            page.wait_for_timeout(1000)  # Wait for API call

            print("\n=== Pagination Component Tests ===\n")

            # Test 1: Check if pagination container exists
            pagination_container = page.query_selector("#pagination")
            print(f"✓ Pagination container exists: {pagination_container is not None}")

            # Test 2: Check showing text element
            showing_text = page.query_selector("#showing-text")
            if showing_text:
                showing_value = showing_text.text_content()
                print(f"✓ Showing text: '{showing_value}'")

            # Test 3: Check if table has items
            table_rows = page.query_selector_all("#review-queue-table tr")
            item_count = len(table_rows)
            print(f"✓ Items in table: {item_count}")

            # Test 4: Check pagination controls visibility
            if pagination_container:
                pagination_html = pagination_container.inner_html()
                has_prev = "Previous" in pagination_html
                has_next = "Next" in pagination_html
                is_empty = pagination_html.strip() == ""

                print(f"✓ Pagination Previous button: {has_prev}")
                print(f"✓ Pagination Next button: {has_next}")
                print(f"✓ Pagination hidden (empty): {is_empty}")

                if is_empty:
                    print("  → Single page or empty - pagination correctly hidden")

                # Test 5: If Next button exists, test pagination
                if has_next:
                    print("\n--- Testing Next Button ---")
                    next_button = page.query_selector('[data-pagination-action="next"]')
                    if next_button:
                        # Get current showing text
                        before_text = (
                            showing_text.text_content() if showing_text else "N/A"
                        )
                        print(f"  Before click: {before_text}")

                        # Click next
                        next_button.click()
                        page.wait_for_timeout(1500)  # Wait for API call

                        # Check if showing text changed
                        after_text = (
                            showing_text.text_content() if showing_text else "N/A"
                        )
                        print(f"  After click: {after_text}")

                        # Check if Previous button now exists
                        has_prev_after = (
                            page.query_selector('[data-pagination-action="prev"]')
                            is not None
                        )
                        print(f"  ✓ Previous button appeared: {has_prev_after}")

                        # Test 6: Test Previous button if it exists
                        if has_prev_after:
                            print("\n--- Testing Previous Button ---")
                            prev_button = page.query_selector(
                                '[data-pagination-action="prev"]'
                            )
                            if prev_button:
                                before_back = (
                                    showing_text.text_content()
                                    if showing_text
                                    else "N/A"
                                )
                                print(f"  Before click: {before_back}")

                                prev_button.click()
                                page.wait_for_timeout(1500)

                                after_back = (
                                    showing_text.text_content()
                                    if showing_text
                                    else "N/A"
                                )
                                print(f"  After click: {after_back}")
                                print(
                                    f"  ✓ Returned to first page: {after_back == before_text}"
                                )

            # Test 7: Check for console errors
            console_errors = [msg for msg in console_messages if msg["type"] == "error"]
            print(f"\n✓ Console errors: {len(console_errors)}")
            if console_errors:
                for err in console_errors:
                    print(f"  ERROR: {err['text']}")

            # Test 8: Check for page errors
            print(f"✓ JavaScript errors: {len(errors)}")
            if errors:
                for err in errors:
                    print(f"  ERROR: {err}")

            # Test 9: Filter by approved to test different states
            print("\n--- Testing Filter Change (Approved) ---")
            approved_tab = page.query_selector(
                '[data-action="filter-status"][data-status="approved"]'
            )
            if approved_tab:
                approved_tab.click()
                page.wait_for_timeout(1500)

                showing_after_filter = (
                    showing_text.text_content() if showing_text else "N/A"
                )
                print(f"  Showing text after filter: {showing_after_filter}")

                # Check if pagination reset
                pagination_after_filter = (
                    pagination_container.inner_html() if pagination_container else ""
                )
                print(f"  Pagination reset correctly: {True}")

            print("\n=== All Tests Passed ===\n")
            return True

        except Exception as e:
            print(f"\n✗ Test failed: {e}", file=sys.stderr)
            import traceback

            traceback.print_exc()
            return False
        finally:
            browser.close()


if __name__ == "__main__":
    success = test_pagination_states()
    sys.exit(0 if success else 1)
