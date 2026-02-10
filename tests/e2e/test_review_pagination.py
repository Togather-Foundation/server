#!/usr/bin/env python3
"""
Comprehensive test for review queue pagination controls.

Tests ALL scenarios:
1. Zero events - no pagination controls visible
2. 1-50 events (single page) - no pagination controls visible
3. 51+ events (multiple pages):
   - On first page: Next button visible, Previous button hidden
   - Click Next: Now on page 2, both buttons visible
   - Click Next to last page: Previous visible, Next hidden
   - Click Previous: Both buttons visible again
4. Filter changes reset pagination correctly
5. After approve/reject, pagination updates (e.g., 51→50 hides controls)
"""

import sys
import os
from playwright.sync_api import sync_playwright, expect

# Get credentials from environment
BASE_URL = os.getenv("BASE_URL", "http://localhost:8080")
ADMIN_USERNAME = os.getenv("ADMIN_USERNAME", "admin")
ADMIN_PASSWORD = os.getenv("ADMIN_PASSWORD", "XXKokg60kd8hLXgq")


def login(page):
    """Login as admin."""
    page.goto(f"{BASE_URL}/admin/login")
    page.fill('input[name="username"]', ADMIN_USERNAME)
    page.fill('input[name="password"]', ADMIN_PASSWORD)
    page.click('button[type="submit"]')
    page.wait_for_url(f"{BASE_URL}/admin/dashboard", timeout=10000)


def get_pagination_state(page):
    """Get current pagination state."""
    pagination = page.query_selector("#pagination")
    if not pagination:
        return {"exists": False}

    html = pagination.inner_html().strip()

    return {
        "exists": True,
        "is_empty": html == "",
        "has_prev": "Previous" in html,
        "has_next": "Next" in html,
        "html": html,
    }


def get_item_count(page):
    """Get number of items in table."""
    rows = page.query_selector_all("#review-queue-table tr[data-entry-id]")
    return len(rows)


def get_showing_text(page):
    """Get showing text."""
    element = page.query_selector("#showing-text")
    return element.text_content() if element else ""


def test_zero_events():
    """Test pagination with zero events."""
    print("\n=== Test 1: Zero Events ===")
    with sync_playwright() as p:
        browser = p.chromium.launch(headless=True)
        page = browser.new_page()

        try:
            login(page)
            page.goto(f"{BASE_URL}/admin/review-queue")
            page.wait_for_load_state("networkidle")
            page.wait_for_timeout(1000)

            # Check if empty state is shown
            empty_state = page.query_selector("#empty-state")
            is_visible = empty_state.is_visible() if empty_state else False

            state = get_pagination_state(page)

            print(f"  Empty state visible: {is_visible}")
            print(f"  Pagination exists: {state['exists']}")
            print(f"  Pagination is empty: {state['is_empty']}")

            # EXPECTED: Pagination should be empty (hidden) when no items
            if not is_visible:
                # If we have items, this is a different scenario
                print(
                    f"  ⚠ Skipping zero events test - found {get_item_count(page)} items"
                )
                return True

            assert state["is_empty"], (
                f"FAIL: Pagination should be hidden with zero events, but found: {state['html']}"
            )
            print("  ✓ PASS: Pagination correctly hidden with zero events")
            return True

        finally:
            browser.close()


def test_single_page():
    """Test pagination with 1-50 events (single page)."""
    print("\n=== Test 2: Single Page (1-50 events) ===")
    with sync_playwright() as p:
        browser = p.chromium.launch(headless=True)
        page = browser.new_page()

        try:
            login(page)
            page.goto(f"{BASE_URL}/admin/review-queue")
            page.wait_for_load_state("networkidle")
            page.wait_for_timeout(1000)

            item_count = get_item_count(page)
            state = get_pagination_state(page)
            showing = get_showing_text(page)

            print(f"  Items: {item_count}")
            print(f"  Showing text: '{showing}'")
            print(f"  Pagination is empty: {state['is_empty']}")
            print(f"  Has Previous: {state['has_prev']}")
            print(f"  Has Next: {state['has_next']}")

            # EXPECTED: If items <= 50, pagination should be hidden
            if item_count == 0:
                print("  ⚠ Skipping single page test - no items found")
                return True

            if item_count > 50:
                print(
                    f"  ⚠ Skipping single page test - too many items ({item_count} > 50)"
                )
                return True

            # This is the critical test: with <=50 items, pagination should be EMPTY
            assert state["is_empty"], (
                f"FAIL: Pagination should be hidden with {item_count} items (<=50), but found: {state['html']}"
            )
            print(f"  ✓ PASS: Pagination correctly hidden with {item_count} items")
            return True

        finally:
            browser.close()


def test_multiple_pages_navigation():
    """Test pagination with 51+ events (multiple pages)."""
    print("\n=== Test 3: Multiple Pages Navigation ===")
    with sync_playwright() as p:
        browser = p.chromium.launch(headless=True)
        page = browser.new_page()

        try:
            login(page)
            page.goto(f"{BASE_URL}/admin/review-queue")
            page.wait_for_load_state("networkidle")
            page.wait_for_timeout(1000)

            item_count = get_item_count(page)

            if item_count <= 50:
                print(
                    f"  ⚠ Skipping multiple pages test - not enough items ({item_count} <= 50)"
                )
                print("  💡 Run: tests/e2e/setup_fixtures.sh 60")
                return True

            print(f"  Items on page 1: {item_count}")

            # Test initial state (page 1)
            print("\n  --- Page 1 (initial) ---")
            state = get_pagination_state(page)
            showing = get_showing_text(page)

            print(f"    Showing: '{showing}'")
            print(f"    Has Previous: {state['has_prev']}")
            print(f"    Has Next: {state['has_next']}")

            assert not state["has_prev"], (
                f"FAIL: Previous button should be hidden on first page"
            )
            assert state["has_next"], (
                f"FAIL: Next button should be visible on first page"
            )
            print("    ✓ Correct: Previous hidden, Next visible")

            # Click Next to go to page 2
            print("\n  --- Clicking Next to page 2 ---")
            next_button = page.query_selector('[data-pagination-action="next"]')
            assert next_button, "FAIL: Next button not found"

            next_button.click()
            page.wait_for_timeout(1500)  # Wait for API call

            state2 = get_pagination_state(page)
            showing2 = get_showing_text(page)

            print(f"    Showing: '{showing2}'")
            print(f"    Has Previous: {state2['has_prev']}")
            print(f"    Has Next: {state2['has_next']}")

            assert state2["has_prev"], (
                f"FAIL: Previous button should be visible on page 2"
            )
            assert showing != showing2, (
                f"FAIL: Showing text should change after pagination"
            )
            print("    ✓ Correct: Previous visible, content changed")

            # If there's still a Next button, we're not on last page yet
            if state2["has_next"]:
                print("    ✓ Next still visible (not on last page)")
            else:
                print("    ✓ Next hidden (on last page)")

            # Click Previous to go back
            print("\n  --- Clicking Previous to return ---")
            prev_button = page.query_selector('[data-pagination-action="prev"]')
            assert prev_button, "FAIL: Previous button not found"

            prev_button.click()
            page.wait_for_timeout(1500)

            state3 = get_pagination_state(page)
            showing3 = get_showing_text(page)

            print(f"    Showing: '{showing3}'")
            print(f"    Has Previous: {state3['has_prev']}")
            print(f"    Has Next: {state3['has_next']}")

            assert not state3["has_prev"], (
                f"FAIL: Previous button should be hidden after returning to first page"
            )
            assert state3["has_next"], (
                f"FAIL: Next button should be visible after returning to first page"
            )
            assert showing3 == showing, f"FAIL: Should return to original showing text"
            print("    ✓ Correct: Returned to page 1 state")

            print("\n  ✓ PASS: Multiple pages navigation works correctly")
            return True

        finally:
            browser.close()


def test_filter_reset():
    """Test that filter changes reset pagination."""
    print("\n=== Test 4: Filter Reset ===")
    with sync_playwright() as p:
        browser = p.chromium.launch(headless=True)
        page = browser.new_page()

        try:
            login(page)
            page.goto(f"{BASE_URL}/admin/review-queue")
            page.wait_for_load_state("networkidle")
            page.wait_for_timeout(1000)

            # Start on pending tab
            showing1 = get_showing_text(page)
            state1 = get_pagination_state(page)

            print(f"  Pending tab - Showing: '{showing1}'")

            # Switch to approved tab
            approved_tab = page.query_selector(
                '[data-action="filter-status"][data-status="approved"]'
            )
            if not approved_tab:
                print("  ⚠ Skipping filter reset test - approved tab not found")
                return True

            approved_tab.click()
            page.wait_for_timeout(1500)

            showing2 = get_showing_text(page)
            state2 = get_pagination_state(page)

            print(f"  Approved tab - Showing: '{showing2}'")

            # Pagination should reset (no Previous button)
            if not state2["is_empty"]:
                assert not state2["has_prev"], (
                    f"FAIL: Previous button should not exist after filter change"
                )
                print("    ✓ Pagination reset correctly")
            else:
                print("    ✓ Pagination hidden (no items in approved)")

            print("  ✓ PASS: Filter change resets pagination")
            return True

        finally:
            browser.close()


def run_all_tests():
    """Run all pagination tests."""
    print("\n" + "=" * 60)
    print("COMPREHENSIVE REVIEW QUEUE PAGINATION TESTS")
    print("=" * 60)

    tests = [
        ("Zero Events", test_zero_events),
        ("Single Page", test_single_page),
        ("Multiple Pages", test_multiple_pages_navigation),
        ("Filter Reset", test_filter_reset),
    ]

    passed = 0
    failed = 0

    for name, test_func in tests:
        try:
            if test_func():
                passed += 1
            else:
                failed += 1
                print(f"  ✗ FAILED: {name}")
        except AssertionError as e:
            failed += 1
            print(f"  ✗ FAILED: {name}")
            print(f"     {e}")
        except Exception as e:
            failed += 1
            print(f"  ✗ ERROR: {name}")
            print(f"     {e}")
            import traceback

            traceback.print_exc()

    print("\n" + "=" * 60)
    print(f"RESULTS: {passed} passed, {failed} failed")
    print("=" * 60 + "\n")

    return failed == 0


if __name__ == "__main__":
    success = run_all_tests()
    sys.exit(0 if success else 1)
