#!/usr/bin/env python3
"""
Batch Event Scraper with Rate Limiting
Submit multiple events while respecting API rate limits
"""

import os
import time
import requests
from datetime import datetime
from typing import List, Dict, Any

API_KEY = os.environ["SEL_API_KEY"]
BASE_URL = os.environ.get("SEL_BASE_URL", "https://sel.togather.events")

# Agent tier: 300 requests per minute
RATE_LIMIT = 300
CALLS_PER_SECOND = RATE_LIMIT / 60.0


class RateLimiter:
    """Simple rate limiter using token bucket algorithm"""

    def __init__(self, calls_per_second: float):
        self.calls_per_second = calls_per_second
        self.last_call = 0.0

    def wait_if_needed(self):
        """Sleep if needed to respect rate limit"""
        now = time.time()
        time_since_last_call = now - self.last_call
        time_between_calls = 1.0 / self.calls_per_second

        if time_since_last_call < time_between_calls:
            sleep_time = time_between_calls - time_since_last_call
            time.sleep(sleep_time)

        self.last_call = time.time()


def submit_event(
    event_data: Dict[str, Any], rate_limiter: RateLimiter
) -> Dict[str, Any]:
    """Submit a single event with rate limiting"""
    rate_limiter.wait_if_needed()

    response = requests.post(
        f"{BASE_URL}/api/v1/events",
        headers={
            "Content-Type": "application/json",
            "Authorization": f"Bearer {API_KEY}",
        },
        json=event_data,
        timeout=30,
    )

    if response.status_code in [201, 409]:
        result = response.json()
        status = "created" if response.status_code == 201 else "duplicate"
        return {"status": status, "id": result.get("@id"), "data": result}
    else:
        error = response.json()
        return {
            "status": "error",
            "code": response.status_code,
            "message": error.get("detail", "Unknown error"),
        }


def submit_batch(events: List[Dict[str, Any]]) -> Dict[str, int]:
    """Submit a batch of events with rate limiting and error handling"""
    rate_limiter = RateLimiter(CALLS_PER_SECOND)

    results = {"created": 0, "duplicates": 0, "errors": 0}

    print(f"Submitting {len(events)} events...")
    print(f"Rate limit: {RATE_LIMIT} req/min ({CALLS_PER_SECOND:.2f} req/sec)")

    for i, event in enumerate(events, 1):
        try:
            result = submit_event(event, rate_limiter)

            if result["status"] == "created":
                results["created"] += 1
                print(f"✓ [{i}/{len(events)}] Created: {result['id']}")
            elif result["status"] == "duplicate":
                results["duplicates"] += 1
                print(f"ℹ [{i}/{len(events)}] Duplicate: {result['id']}")
            else:
                results["errors"] += 1
                print(
                    f"✗ [{i}/{len(events)}] Error {result['code']}: {result['message']}"
                )

        except requests.exceptions.Timeout:
            results["errors"] += 1
            print(f"✗ [{i}/{len(events)}] Timeout")

        except Exception as e:
            results["errors"] += 1
            print(f"✗ [{i}/{len(events)}] Error: {e}")

    print("\nResults:")
    print(f"  Created: {results['created']}")
    print(f"  Duplicates: {results['duplicates']}")
    print(f"  Errors: {results['errors']}")
    print(f"  Total: {len(events)}")

    return results


# Example usage
if __name__ == "__main__":
    # Sample events to submit
    events = [
        {
            "name": "Open Mic Night",
            "startDate": "2026-02-15T19:00:00-05:00",
            "location": {"name": "The Cameron House"},
            "source": {"url": "https://example.com/open-mic-1"},
        },
        {
            "name": "Jazz Jam Session",
            "startDate": "2026-02-16T20:00:00-05:00",
            "location": {"name": "The Rex Hotel"},
            "source": {"url": "https://example.com/jazz-jam-1"},
        },
        {
            "name": "Comedy Show",
            "startDate": "2026-02-17T21:00:00-05:00",
            "location": {"name": "The Tranzac"},
            "source": {"url": "https://example.com/comedy-1"},
        },
    ]

    results = submit_batch(events)

    if results["errors"] > 0:
        exit(1)
