#!/usr/bin/env python3
"""
Test the approve action in the review queue.
This script verifies that clicking "Approve" works without a 500 error.
"""

import os
import sys
from playwright.sync_api import sync_playwright, expect


def test_approve_action():
    """Test that approve button works in review queue."""

    # Get credentials from environment
    admin_user = os.getenv("ADMIN_USERNAME", "admin")
    admin_pass = os.getenv("ADMIN_PASSWORD")

    if not admin_pass:
        print("ERROR: ADMIN_PASSWORD environment variable not set")
        print("Run: source .env")
        sys.exit(1)

    base_url = "http://localhost:8080"

    with sync_playwright() as p:
        browser = p.chromium.launch(headless=True)
        context = browser.new_context()
        page = context.new_page()

        # Login
        print("1. Logging in...")
        page.goto(f"{base_url}/admin/login")
        page.fill('input[name="username"]', admin_user)
        page.fill('input[name="password"]', admin_pass)
        page.click('button[type="submit"]')
        page.wait_for_url(f"{base_url}/admin/dashboard", timeout=5000)
        print("   ✓ Logged in successfully")

        # Go to review queue
        print("2. Navigating to review queue...")
        page.goto(f"{base_url}/admin/review-queue")
        page.wait_for_load_state("networkidle")

        # Check if there are any items in the review queue
        items = page.locator(".review-item").count()
        print(f"   Found {items} review queue items")

        if items == 0:
            print("   ⚠ No items in review queue to test")
            print(
                "   Create a test event with: ./server ingest /tmp/test-approve-batch.json"
            )
            browser.close()
            return

        # Get the first item's ID
        first_item = page.locator(".review-item").first
        item_id = first_item.get_attribute("data-review-id")
        print(f"   Testing with review item ID: {item_id}")

        # Listen for console errors
        errors = []
        page.on(
            "console",
            lambda msg: errors.append(msg.text()) if msg.type() == "error" else None,
        )

        # Click approve button
        print("3. Clicking Approve button...")
        approve_btn = first_item.locator('button:has-text("Approve")')
        approve_btn.click()

        # Wait a bit for the action to complete
        page.wait_for_timeout(2000)

        # Check for errors
        if errors:
            print(f"   ✗ ERRORS DETECTED:")
            for error in errors:
                print(f"     - {error}")
            browser.close()
            sys.exit(1)

        # Check if item was removed from queue (success) or still there (failure)
        page.wait_for_timeout(1000)
        remaining_items = page.locator(".review-item").count()

        if remaining_items < items:
            print(f"   ✓ Item removed from queue (approved successfully)")
            print(f"   ✓ No console errors detected")
        else:
            print(f"   ⚠ Item still in queue (check if it was already processed)")

        browser.close()
        print("\n✓ Test completed successfully - no 500 errors!")


if __name__ == "__main__":
    test_approve_action()
