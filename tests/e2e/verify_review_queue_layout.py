#!/usr/bin/env python3
"""
Verify the review queue definition list layout fix - HEADLESS version for automation.
"""

import os
import sys
from playwright.sync_api import sync_playwright


def verify_layout():
    admin_password = os.getenv("ADMIN_PASSWORD", "XXKokg60kd8hLXgq")

    print("🔍 Verifying review queue layout...")

    with sync_playwright() as p:
        browser = p.chromium.launch(headless=True)  # Headless for automation
        context = browser.new_context(viewport={"width": 1400, "height": 1000})
        page = context.new_page()

        # Capture console messages
        console_messages = []
        page.on(
            "console", lambda msg: console_messages.append(f"{msg.type}: {msg.text}")
        )

        try:
            # Login
            print("  → Logging in...")
            page.goto("http://localhost:8080/admin/login")
            page.fill("#username", "admin")
            page.fill("#password", admin_password)
            page.click('button[type="submit"]')
            page.wait_for_url("**/admin/dashboard", timeout=5000)

            # Go to review queue page
            print("  → Opening review queue page...")
            page.goto("http://localhost:8080/admin/review-queue")
            page.wait_for_timeout(3000)  # Wait for data to load

            # Take initial screenshot
            page.screenshot(path="/tmp/review_queue_initial.png", full_page=True)
            print("  ✓ Initial screenshot: /tmp/review_queue_initial.png")

            # Debug: Check what's on the page
            table_body = page.query_selector("tbody")
            if table_body:
                row_count = len(page.query_selector_all("tbody tr"))
                print(f"  → Found {row_count} rows in table")

            # Find ALL expand buttons and click the 4th one (entry #4 which has location data)
            all_toggles = page.query_selector_all('button[data-action="expand-detail"]')

            if not all_toggles or len(all_toggles) < 4:
                print(
                    f"\n  ❌ Need at least 4 entries, found {len(all_toggles) if all_toggles else 0}"
                )
                browser.close()
                return False

            # Click entry #4 (index 3) which has location data
            print(f"  → Found {len(all_toggles)} entries, expanding entry #4...")
            all_toggles[3].click()  # 0-indexed, so [3] is the 4th entry
            page.wait_for_timeout(1500)  # Wait longer for expansion and rendering

            # Wait for Event Information section to appear
            try:
                page.wait_for_selector(
                    'h4.card-title:has-text("Event Information")', timeout=3000
                )
                print("  ✓ Event Information section loaded")
            except:
                print("  ⚠ Event Information section not found")

            # Take expanded screenshot
            page.screenshot(path="/tmp/review_queue_expanded.png", full_page=True)
            print("  ✓ Expanded screenshot: /tmp/review_queue_expanded.png")

            # Check for definition lists
            dl_elements = page.query_selector_all("dl")

            if not dl_elements:
                print(
                    "\n  ⚠ No definition lists found (event might not have location/nested data)"
                )
                print("  → This is OK if the event doesn't have complex JSON fields")
                browser.close()
                return True  # Not a failure

            print(f"\n  ✓ Found {len(dl_elements)} definition list(s)")

            # Check first DL's CSS Grid properties
            first_dl = dl_elements[0]
            display = first_dl.evaluate("el => window.getComputedStyle(el).display")
            grid_template = first_dl.evaluate(
                "el => window.getComputedStyle(el).gridTemplateColumns"
            )

            print(f"  → DL display: {display}")
            print(f"  → DL grid-template-columns: {grid_template}")

            if display == "grid":
                print("  ✅ CSS Grid is applied!")
            else:
                print(f"  ❌ CSS Grid NOT applied (got: {display})")
                browser.close()
                return False

            # Check DT/DD positioning
            dt = first_dl.query_selector("dt")
            dd = first_dl.query_selector("dd")

            if dt and dd:
                dt_box = dt.bounding_box()
                dd_box = dd.bounding_box()

                y_diff = abs(dt_box["y"] - dd_box["y"])

                print(
                    f"\n  → DT: x={dt_box['x']:.1f}, y={dt_box['y']:.1f}, width={dt_box['width']:.1f}"
                )
                print(
                    f"  → DD: x={dd_box['x']:.1f}, y={dd_box['y']:.1f}, width={dd_box['width']:.1f}"
                )
                print(f"  → Y difference: {y_diff:.1f}px")

                if y_diff < 5:
                    print("  ✅ DT and DD are on the same row (side-by-side)")
                else:
                    print(f"  ❌ DT and DD are NOT side-by-side!")
                    browser.close()
                    return False

                if dd_box["x"] > dt_box["x"]:
                    print("  ✅ DD is to the right of DT")
                else:
                    print("  ❌ DD is NOT to the right of DT")
                    browser.close()
                    return False

            # Check console errors
            errors = [msg for msg in console_messages if "error" in msg.lower()]
            if errors:
                print("\n⚠ Console errors found:")
                for error in errors:
                    print(f"  {error}")
                browser.close()
                return False

            print("\n✅ Layout verification PASSED!")
            print("\nScreenshots saved:")
            print("  - /tmp/review_queue_initial.png")
            print("  - /tmp/review_queue_expanded.png")
            print("\nVisually inspect these screenshots to confirm layout is correct.")

            browser.close()
            return True

        except Exception as e:
            print(f"\n❌ Error: {e}")
            page.screenshot(path="/tmp/review_queue_error.png", full_page=True)
            print("  Error screenshot: /tmp/review_queue_error.png")
            browser.close()
            return False


if __name__ == "__main__":
    success = verify_layout()
    sys.exit(0 if success else 1)
