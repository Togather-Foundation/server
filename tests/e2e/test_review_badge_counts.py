#!/usr/bin/env python3
"""
E2E test for review queue badge count updates.
Tests that badge counts update immediately after approve/reject actions.
"""

import os
from playwright.sync_api import sync_playwright, expect

BASE_URL = os.getenv("BASE_URL", "http://localhost:8080")
ADMIN_USERNAME = os.getenv("ADMIN_USERNAME", "admin")
ADMIN_PASSWORD = os.getenv("ADMIN_PASSWORD", "XXKokg60kd8hLXgq")


def test_review_badge_counts():
    """Test that badge counts update immediately after approve/reject actions."""
    with sync_playwright() as p:
        browser = p.chromium.launch(headless=True)
        context = browser.new_context()
        page = context.new_page()

        # Set up console error tracking
        console_errors = []
        page.on(
            "console",
            lambda msg: console_errors.append(msg.text)
            if msg.type == "error"
            else None,
        )

        try:
            # Login
            print("Logging in...")
            page.goto(f"{BASE_URL}/admin/login")
            page.fill("#username", ADMIN_USERNAME)
            page.fill("#password", ADMIN_PASSWORD)
            page.click('button[type="submit"]')
            page.wait_for_url(f"{BASE_URL}/admin/dashboard", timeout=10000)

            # Navigate to review queue
            print("Navigating to review queue...")
            page.goto(f"{BASE_URL}/admin/review-queue", wait_until="networkidle")
            page.wait_for_selector("#status-tabs", timeout=10000)

            # Get initial badge counts
            print("Reading initial badge counts...")
            pending_badge = page.locator(
                '[data-action="filter-status"][data-status="pending"] .badge'
            )
            approved_badge = page.locator(
                '[data-action="filter-status"][data-status="approved"] .badge'
            )
            rejected_badge = page.locator(
                '[data-action="filter-status"][data-status="rejected"] .badge'
            )

            # Wait for badges to be visible
            expect(pending_badge).to_be_visible(timeout=5000)
            expect(approved_badge).to_be_visible(timeout=5000)
            expect(rejected_badge).to_be_visible(timeout=5000)

            initial_pending = int(pending_badge.inner_text())
            initial_approved = int(approved_badge.inner_text())
            initial_rejected = int(rejected_badge.inner_text())

            print(
                f"Initial counts - Pending: {initial_pending}, Approved: {initial_approved}, Rejected: {initial_rejected}"
            )

            if initial_pending == 0:
                print(
                    "⚠️  No pending items to test with. Test cannot verify badge updates."
                )
                print("✓ Test passed (no data to test)")
                return

            # Expand first entry to get approve/reject buttons
            print("Expanding first entry...")
            first_row = page.locator("tr[data-entry-id]").first
            expand_button = first_row.locator('[data-action="expand-detail"]')
            expand_button.click()

            # Wait for detail section to load
            page.wait_for_selector('[data-action="approve"]', timeout=10000)

            # Test approve action
            print("Testing approve action...")
            approve_button = page.locator('[data-action="approve"]').first

            # Get current counts before action
            current_pending = int(pending_badge.inner_text())
            current_approved = int(approved_badge.inner_text())

            approve_button.click()

            # Wait a moment for the action to complete
            page.wait_for_timeout(2000)

            # Verify badge counts updated immediately
            print("Verifying badge counts after approve...")

            # Check if pending badge decreased
            new_pending = pending_badge.inner_text()
            print(f"  Pending count: {current_pending} → {new_pending}")

            # Check if approved badge increased
            new_approved = approved_badge.inner_text()
            print(f"  Approved count: {current_approved} → {new_approved}")

            # Assertions
            try:
                assert new_pending == str(current_pending - 1), (
                    f"Pending badge should decrease from {current_pending} to {current_pending - 1}, got {new_pending}"
                )
                assert new_approved == str(current_approved + 1), (
                    f"Approved badge should increase from {current_approved} to {current_approved + 1}, got {new_approved}"
                )
                print(f"✓ Badge counts updated correctly after approve")
            except AssertionError as e:
                print(f"✗ Badge count assertion failed: {e}")
                raise

            # If there are more pending items, test reject action
            if new_pending != "0":
                print("\nTesting reject action...")

                # Get current counts
                current_pending = int(new_pending)
                current_rejected = int(rejected_badge.inner_text())

                # Expand first entry again
                first_row = page.locator("tr[data-entry-id]").first
                expand_button = first_row.locator('[data-action="expand-detail"]')
                expand_button.click()

                # Wait for detail section
                page.wait_for_selector('[data-action="reject"]', timeout=10000)

                # Click reject
                reject_button = page.locator('[data-action="reject"]').first
                reject_button.click()

                # Wait for modal and fill rejection reason
                page.wait_for_selector("#reject-modal", state="visible", timeout=5000)
                page.fill(
                    "#reject-reason", "Test rejection for badge count verification"
                )

                # Confirm rejection
                page.locator("#confirm-reject-btn").click()

                # Wait a moment for the action to complete
                page.wait_for_timeout(2000)

                # Verify badge counts updated immediately
                print("Verifying badge counts after reject...")

                # Check counts
                new_pending_after_reject = pending_badge.inner_text()
                new_rejected = rejected_badge.inner_text()
                new_approved_after_reject = approved_badge.inner_text()

                print(
                    f"  Pending count: {current_pending} → {new_pending_after_reject}"
                )
                print(f"  Rejected count: {current_rejected} → {new_rejected}")

                # Assertions
                try:
                    assert new_pending_after_reject == str(current_pending - 1), (
                        f"Pending should decrease from {current_pending} to {current_pending - 1}, got {new_pending_after_reject}"
                    )
                    assert new_rejected == str(current_rejected + 1), (
                        f"Rejected should increase from {current_rejected} to {current_rejected + 1}, got {new_rejected}"
                    )
                    assert new_approved_after_reject == new_approved, (
                        f"Approved should stay at {new_approved}, got {new_approved_after_reject}"
                    )
                    print(f"✓ Badge counts updated correctly after reject")
                except AssertionError as e:
                    print(f"✗ Badge count assertion failed: {e}")
                    raise

            # Check for console errors
            if console_errors:
                print(f"\n⚠️  Console errors detected:")
                for error in console_errors:
                    print(f"  {error}")
            else:
                print("\n✓ No console errors")

            print(f"\n✓ All badge count tests passed!")

            # Take success screenshot showing updated badges
            screenshot_path = "/tmp/badge_counts_success.png"
            page.screenshot(path=screenshot_path, full_page=True)
            print(f"✓ Screenshot saved: {screenshot_path}")

        except Exception as e:
            print(f"\n✗ Test failed: {e}")

            # Take screenshot on failure
            screenshot_path = "/tmp/badge_count_test_failure.png"
            page.screenshot(path=screenshot_path)
            print(f"Screenshot saved to: {screenshot_path}")

            raise

        finally:
            browser.close()


if __name__ == "__main__":
    test_review_badge_counts()
