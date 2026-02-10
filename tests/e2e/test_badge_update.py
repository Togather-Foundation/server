#!/usr/bin/env python3
"""
E2E test for Review Queue Badge Update (srv-s3m)

Tests that badge counts update immediately after approve/reject actions.

Run with:
    source .env && uvx --from pytest-playwright --with playwright --with pytest pytest tests/e2e/test_badge_update.py -v
"""

import os
import subprocess
from pathlib import Path
import pytest
from playwright.sync_api import Page, expect


# Configuration
BASE_URL = os.getenv("BASE_URL", "http://localhost:8080")
ADMIN_USERNAME = os.getenv("ADMIN_USERNAME", "admin")
ADMIN_PASSWORD = os.getenv("ADMIN_PASSWORD", "XXKokg60kd8hLXgq")


@pytest.fixture(scope="session")
def browser_context_args(browser_context_args):
    """Configure browser context for all tests"""
    return {
        **browser_context_args,
        "viewport": {"width": 1920, "height": 1080},
        "ignore_https_errors": True,
    }


@pytest.fixture(scope="function")
def admin_login(page: Page):
    """Login as admin before test"""
    print("\n   Logging in as admin...")

    page.goto(f"{BASE_URL}/admin/login")
    page.wait_for_load_state("networkidle")

    # Fill in credentials
    page.fill("#username", ADMIN_USERNAME)
    page.fill("#password", ADMIN_PASSWORD)

    # Click submit and wait for navigation
    with page.expect_navigation(timeout=10000):
        page.click('button[type="submit"]')

    # Verify we're on the dashboard
    page.wait_for_url(f"{BASE_URL}/admin/dashboard", timeout=5000)
    print("   ✓ Logged in successfully")
    return page


@pytest.fixture(scope="session")
def fixture_data():
    """Setup test fixtures using bash script and Go commands"""
    script_dir = Path(__file__).parent
    setup_script = script_dir / "setup_fixtures.sh"
    project_root = script_dir.parent.parent
    env_file = project_root / ".env"

    if not setup_script.exists():
        raise FileNotFoundError(f"Setup script not found: {setup_script}")

    # Load environment from .env file
    env = os.environ.copy()
    if env_file.exists():
        with open(env_file) as f:
            for line in f:
                line = line.strip()
                if line and not line.startswith("#") and "=" in line:
                    key, value = line.split("=", 1)
                    env[key.strip()] = value.strip()

    print("\n" + "=" * 60)
    print("Setting up Review Queue Test Fixtures (via Go)")
    print("=" * 60 + "\n")

    try:
        # Run setup script with 5 fixtures
        result = subprocess.run(
            [str(setup_script), "3"],
            cwd=project_root,
            capture_output=True,
            text=True,
            timeout=30,
            env=env,
        )

        if result.returncode != 0:
            print(f"✗ Setup script failed with exit code {result.returncode}")
            print(f"STDOUT:\n{result.stdout}")
            print(f"STDERR:\n{result.stderr}")
            raise RuntimeError("Setup script failed")

        print(result.stdout)
        print("✓ Fixtures setup completed via bash script\n")

        yield True

        # Cleanup after all tests
        cleanup_script = script_dir / "cleanup_fixtures.sh"
        if cleanup_script.exists():
            print("\n" + "=" * 60)
            print("Cleaning up Review Queue Test Fixtures")
            print("=" * 60 + "\n")

            cleanup_result = subprocess.run(
                [str(cleanup_script)],
                cwd=project_root,
                capture_output=True,
                text=True,
                timeout=30,
                env=env,
            )

            if cleanup_result.returncode == 0:
                print(cleanup_result.stdout)
                print("✓ Fixtures cleanup completed\n")
            else:
                print(
                    f"⚠ Cleanup script failed with exit code {cleanup_result.returncode}"
                )
                print(f"STDERR:\n{cleanup_result.stderr}")
        else:
            print(f"⚠ Cleanup script not found: {cleanup_script}")

    except subprocess.TimeoutExpired:
        print("✗ Setup script timed out after 30 seconds")
        raise
    except Exception as e:
        print(f"✗ Failed to setup fixtures: {e}")
        raise


class TestBadgeUpdate:
    """Test that badge counts update immediately after actions"""

    def test_approve_increments_approved_badge(self, admin_login, fixture_data):
        """Test that approving an entry increments approved badge count"""
        page = admin_login
        print("\n   Testing approve action increments approved badge...")

        page.goto(f"{BASE_URL}/admin/review-queue")
        page.wait_for_load_state("networkidle")
        page.wait_for_timeout(1500)

        # Get initial badge counts
        pending_badge = page.locator(
            '[data-action="filter-status"][data-status="pending"] .badge'
        )
        approved_badge = page.locator(
            '[data-action="filter-status"][data-status="approved"] .badge'
        )

        initial_pending = int(pending_badge.inner_text())
        initial_approved = int(approved_badge.inner_text())

        print(f"   Initial pending count: {initial_pending}")
        print(f"   Initial approved count: {initial_approved}")

        # Find first expand button
        expand_buttons = page.locator('[data-action="expand-detail"]')
        if expand_buttons.count() == 0:
            print("   ⚠ No review queue items to test")
            pytest.skip("No review queue items available")
            return

        # Expand first item
        first_expand_btn = expand_buttons.first
        entry_id = first_expand_btn.get_attribute("data-id")
        first_expand_btn.click()
        page.wait_for_timeout(1000)

        # Find approve button
        approve_btn = page.locator(f'[data-action="approve"][data-id="{entry_id}"]')

        if approve_btn.count() == 0:
            print("   ⚠ No approve button found")
            pytest.skip("No approve button available")
            return

        # Click approve
        approve_btn.click()
        print("   Clicked approve button")

        # Wait for action to complete
        page.wait_for_timeout(2000)

        # Get updated badge counts
        updated_pending = int(pending_badge.inner_text())
        updated_approved = int(approved_badge.inner_text())

        print(f"   Updated pending count: {updated_pending}")
        print(f"   Updated approved count: {updated_approved}")

        # Verify counts changed correctly
        assert updated_pending == initial_pending - 1, (
            f"Pending count should decrease by 1 (was {initial_pending}, now {updated_pending})"
        )
        assert updated_approved == initial_approved + 1, (
            f"Approved count should increase by 1 (was {initial_approved}, now {updated_approved})"
        )

        print("   ✓ Badge counts updated correctly after approve")

    def test_reject_increments_rejected_badge(self, admin_login, fixture_data):
        """Test that rejecting an entry increments rejected badge count"""
        page = admin_login
        print("\n   Testing reject action increments rejected badge...")

        page.goto(f"{BASE_URL}/admin/review-queue")
        page.wait_for_load_state("networkidle")
        page.wait_for_timeout(1500)

        # Get initial badge counts
        pending_badge = page.locator(
            '[data-action="filter-status"][data-status="pending"] .badge'
        )
        rejected_badge = page.locator(
            '[data-action="filter-status"][data-status="rejected"] .badge'
        )

        initial_pending = int(pending_badge.inner_text())
        initial_rejected = int(rejected_badge.inner_text())

        print(f"   Initial pending count: {initial_pending}")
        print(f"   Initial rejected count: {initial_rejected}")

        # Find first expand button
        expand_buttons = page.locator('[data-action="expand-detail"]')
        if expand_buttons.count() == 0:
            print("   ⚠ No review queue items to test")
            pytest.skip("No review queue items available")
            return

        # Expand first item
        first_expand_btn = expand_buttons.first
        entry_id = first_expand_btn.get_attribute("data-id")
        first_expand_btn.click()
        page.wait_for_timeout(1000)

        # Find reject button
        reject_btn = page.locator(f'[data-action="reject"][data-id="{entry_id}"]')

        if reject_btn.count() == 0:
            print("   ⚠ No reject button found")
            pytest.skip("No reject button available")
            return

        # Click reject
        reject_btn.click()
        page.wait_for_timeout(500)

        # Fill in rejection reason in modal
        modal = page.locator("#reject-modal")
        expect(modal).to_be_visible()

        reason_textarea = modal.locator("#reject-reason")
        reason_textarea.fill("Test rejection for badge update verification")

        # Confirm rejection
        confirm_btn = modal.locator("#confirm-reject-btn")
        confirm_btn.click()
        print("   Clicked confirm reject button")

        # Wait for action to complete
        page.wait_for_timeout(2000)

        # Get updated badge counts
        updated_pending = int(pending_badge.inner_text())
        updated_rejected = int(rejected_badge.inner_text())

        print(f"   Updated pending count: {updated_pending}")
        print(f"   Updated rejected count: {updated_rejected}")

        # Verify counts changed correctly
        assert updated_pending == initial_pending - 1, (
            f"Pending count should decrease by 1 (was {initial_pending}, now {updated_pending})"
        )
        assert updated_rejected == initial_rejected + 1, (
            f"Rejected count should increase by 1 (was {initial_rejected}, now {updated_rejected})"
        )

        print("   ✓ Badge counts updated correctly after reject")


if __name__ == "__main__":
    print(__doc__)
    print("\nPlease run with pytest (see instructions above)")
