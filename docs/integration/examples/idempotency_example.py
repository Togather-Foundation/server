#!/usr/bin/env python3
"""
Idempotent Event Scraper
Use idempotency keys for safe retries and preventing duplicates
"""

import os
import requests
from datetime import datetime
from typing import Dict, Any, Optional

API_KEY = os.environ["SEL_API_KEY"]
BASE_URL = os.environ.get("SEL_BASE_URL", "https://sel.togather.events")


def generate_idempotency_key(scraper_id: str, run_date: str, event_id: str) -> str:
    """
    Generate idempotency key from scraper run metadata

    Format: {scraper-id}-{date}-{event-id}
    Example: tranzac-scraper-20260127-evt123
    """
    return f"{scraper_id}-{run_date}-{event_id}"


def submit_event_idempotent(
    event_data: Dict[str, Any],
    scraper_id: str,
    event_id: str,
    run_date: Optional[str] = None,
) -> Dict[str, Any]:
    """
    Submit event with idempotency key for safe retries

    Args:
        event_data: Event payload (name, startDate, location, etc.)
        scraper_id: Unique scraper identifier
        event_id: Unique event ID from source system
        run_date: Optional run date (defaults to today)

    Returns:
        Dict with status, id, and message
    """
    if run_date is None:
        run_date = datetime.now().strftime("%Y%m%d")

    idempotency_key = generate_idempotency_key(scraper_id, run_date, event_id)

    response = requests.post(
        f"{BASE_URL}/api/v1/events",
        headers={
            "Content-Type": "application/json",
            "X-API-Key": API_KEY,
            "Idempotency-Key": idempotency_key,
        },
        json=event_data,
        timeout=30,
    )

    if response.status_code == 201:
        result = response.json()
        return {
            "status": "created",
            "id": result.get("@id"),
            "message": "Event created successfully",
        }

    elif response.status_code == 409:
        result = response.json()
        # Check if this is idempotency conflict or duplicate source
        if "idempotency" in response.text.lower():
            return {
                "status": "cached",
                "id": result.get("@id"),
                "message": "Idempotency key matched - returning cached response",
            }
        else:
            return {
                "status": "duplicate",
                "id": result.get("@id"),
                "message": "Event already exists (duplicate source.url)",
            }

    else:
        error = response.json()
        return {
            "status": "error",
            "code": response.status_code,
            "message": error.get("detail", "Unknown error"),
        }


# Example 1: Basic idempotent submission
def example_basic():
    """Submit event with idempotency"""
    event = {
        "name": "Comedy Night",
        "startDate": "2026-02-15T20:00:00-05:00",
        "location": {"name": "The Tranzac"},
        "source": {"url": "https://thetranzac.com/events/comedy-night"},
    }

    result = submit_event_idempotent(
        event_data=event, scraper_id="tranzac-scraper", event_id="evt-123"
    )

    print(f"Status: {result['status']}")
    print(f"ID: {result['id']}")
    print(f"Message: {result['message']}")


# Example 2: Retry after network failure
def example_retry():
    """Demonstrate safe retry with same idempotency key"""
    event = {
        "name": "Jazz Night",
        "startDate": "2026-02-20T20:00:00-05:00",
        "location": {"name": "The Rex Hotel"},
        "source": {"url": "https://therex.ca/events/jazz-night"},
    }

    scraper_id = "rex-scraper"
    event_id = "jazz-2026-02-20"
    run_date = "20260127"

    # First attempt
    print("First attempt...")
    result1 = submit_event_idempotent(event, scraper_id, event_id, run_date)
    print(f"  Result: {result1['status']}")

    # Retry with same key (safe!)
    print("\nRetrying with same key...")
    result2 = submit_event_idempotent(event, scraper_id, event_id, run_date)
    print(f"  Result: {result2['status']}")

    # Both should return the same event ID
    assert result1["id"] == result2["id"]
    print(f"\n✓ Both requests returned same event: {result1['id']}")


# Example 3: Scraper run with idempotency
def example_scraper_run():
    """Complete scraper run with idempotency keys"""
    scraper_id = "venue-scraper-v1"
    run_date = datetime.now().strftime("%Y%m%d")

    # Events scraped from source
    scraped_events = [
        {
            "source_id": "evt-001",
            "data": {
                "name": "Event 1",
                "startDate": "2026-02-15T19:00:00-05:00",
                "location": {"name": "Venue A"},
                "source": {"url": "https://example.com/event-001"},
            },
        },
        {
            "source_id": "evt-002",
            "data": {
                "name": "Event 2",
                "startDate": "2026-02-16T20:00:00-05:00",
                "location": {"name": "Venue B"},
                "source": {"url": "https://example.com/event-002"},
            },
        },
    ]

    print(f"Scraper run: {scraper_id} on {run_date}")
    print(f"Submitting {len(scraped_events)} events...\n")

    results = {"created": 0, "cached": 0, "duplicate": 0, "error": 0}

    for item in scraped_events:
        result = submit_event_idempotent(
            event_data=item["data"],
            scraper_id=scraper_id,
            event_id=item["source_id"],
            run_date=run_date,
        )

        results[result["status"]] = results.get(result["status"], 0) + 1
        print(f"  {item['source_id']}: {result['status']}")

    print(f"\nResults: {results}")


if __name__ == "__main__":
    print("=== Example 1: Basic Idempotent Submission ===\n")
    example_basic()

    print("\n\n=== Example 2: Safe Retry ===\n")
    example_retry()

    print("\n\n=== Example 3: Complete Scraper Run ===\n")
    example_scraper_run()
