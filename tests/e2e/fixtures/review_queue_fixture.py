#!/usr/bin/env python3
"""
Review Queue Test Fixtures

Populates the database with test data for review queue E2E testing.
Creates events with various warning scenarios for testing the review workflow.

Dependencies:
    pip install psycopg2-binary
    # or
    uvx --with psycopg2-binary python tests/e2e/fixtures/review_queue_fixture.py setup

Usage:
    from tests.e2e.fixtures.review_queue_fixture import (
        setup_review_queue_fixtures,
        cleanup_review_queue_fixtures
    )

    # In your test setup
    entry_ids = setup_review_queue_fixtures()

    # Run your tests...

    # In your test teardown
    cleanup_review_queue_fixtures(entry_ids)
"""

import os
import json

try:
    import psycopg2
    from psycopg2.extras import RealDictCursor
except ImportError:
    print("\n✗ Error: psycopg2 is not installed")
    print("\nInstall with:")
    print("  pip install psycopg2-binary")
    print("\nOr run with uvx:")
    print(
        "  uvx --with psycopg2-binary python tests/e2e/fixtures/review_queue_fixture.py setup"
    )
    raise

from datetime import datetime, timezone, timedelta


# Configuration
DATABASE_URL = os.getenv("DATABASE_URL")

if not DATABASE_URL:
    raise ValueError("DATABASE_URL environment variable is required")


def get_db_connection():
    """
    Create a database connection using DATABASE_URL from environment.

    Returns:
        psycopg2.connection: Database connection

    Raises:
        psycopg2.Error: If connection fails
    """
    try:
        conn = psycopg2.connect(DATABASE_URL)
        return conn
    except psycopg2.Error as e:
        print(f"✗ Failed to connect to database: {e}")
        raise


def create_test_place(conn, name, locality, region):
    """
    Create a test place (venue) in the database.

    Args:
        conn: Database connection
        name: Place name
        locality: City/locality
        region: Province/state

    Returns:
        dict: Place data with 'id' and 'ulid'
    """
    cursor = conn.cursor(cursor_factory=RealDictCursor)
    try:
        cursor.execute(
            """
            INSERT INTO places (ulid, name, address_locality, address_region)
            VALUES (
                'PLACE' || substring(md5(random()::text) from 1 for 20),
                %s, %s, %s
            )
            RETURNING id, ulid
            """,
            (name, locality, region),
        )
        place = cursor.fetchone()
        conn.commit()
        return dict(place)
    except psycopg2.Error as e:
        conn.rollback()
        print(f"✗ Failed to create test place: {e}")
        raise
    finally:
        cursor.close()


def create_test_event(
    conn, ulid, name, description, venue_id, lifecycle_state="pending_review"
):
    """
    Create a test event in the database.

    Args:
        conn: Database connection
        ulid: Event ULID
        name: Event name
        description: Event description
        venue_id: Primary venue UUID
        lifecycle_state: Lifecycle state (default: 'pending_review')

    Returns:
        dict: Event data with 'id' and 'ulid'
    """
    cursor = conn.cursor(cursor_factory=RealDictCursor)
    try:
        cursor.execute(
            """
            INSERT INTO events (
                ulid, name, description, lifecycle_state, event_domain,
                primary_venue_id, license_url, license_status
            )
            VALUES (%s, %s, %s, %s, 'arts', %s, 
                    'https://creativecommons.org/publicdomain/zero/1.0/', 'cc0')
            RETURNING id, ulid
            """,
            (ulid, name, description, lifecycle_state, venue_id),
        )
        event = cursor.fetchone()
        conn.commit()
        return dict(event)
    except psycopg2.Error as e:
        conn.rollback()
        print(f"✗ Failed to create test event: {e}")
        raise
    finally:
        cursor.close()


def create_test_occurrence(
    conn, event_id, start_time, venue_id, end_time=None, timezone="America/Toronto"
):
    """
    Create a test event occurrence.

    Args:
        conn: Database connection
        event_id: Event UUID
        start_time: Start datetime
        venue_id: Venue UUID
        end_time: End datetime (optional)
        timezone: Timezone string
    """
    cursor = conn.cursor()
    try:
        cursor.execute(
            """
            INSERT INTO event_occurrences (
                event_id, start_time, end_time, venue_id, timezone
            )
            VALUES (%s, %s, %s, %s, %s)
            """,
            (event_id, start_time, end_time, venue_id, timezone),
        )
        conn.commit()
    except psycopg2.Error as e:
        conn.rollback()
        print(f"✗ Failed to create test occurrence: {e}")
        raise
    finally:
        cursor.close()


def create_review_queue_entry(
    conn,
    event_id,
    original_payload,
    normalized_payload,
    warnings,
    event_start_time,
    event_end_time=None,
    source_id=None,
    source_external_id=None,
    dedup_hash=None,
):
    """
    Create a review queue entry for testing.

    Args:
        conn: Database connection
        event_id: Event UUID
        original_payload: Original event payload (dict)
        normalized_payload: Normalized event payload (dict)
        warnings: List of warning dicts
        event_start_time: Event start datetime
        event_end_time: Event end datetime (optional)
        source_id: Source identifier (optional)
        source_external_id: External source ID (optional)
        dedup_hash: Deduplication hash (optional)

    Returns:
        int: Review queue entry ID
    """
    cursor = conn.cursor(cursor_factory=RealDictCursor)
    try:
        cursor.execute(
            """
            INSERT INTO event_review_queue (
                event_id, original_payload, normalized_payload, warnings,
                event_start_time, event_end_time, source_id, 
                source_external_id, dedup_hash, status
            )
            VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s, 'pending')
            RETURNING id
            """,
            (
                event_id,
                json.dumps(original_payload),
                json.dumps(normalized_payload),
                json.dumps(warnings),
                event_start_time,
                event_end_time,
                source_id,
                source_external_id,
                dedup_hash,
            ),
        )
        entry = cursor.fetchone()
        conn.commit()
        return entry["id"]
    except psycopg2.Error as e:
        conn.rollback()
        print(f"✗ Failed to create review queue entry: {e}")
        raise
    finally:
        cursor.close()


def setup_review_queue_fixtures():
    """
    Create test review queue entries with various warning scenarios.

    Creates:
    1. Event with reversed date warning (end before start due to timezone)
    2. Event with missing venue warning
    3. Event with duplicate detection warning
    4. Event with multiple warnings
    5. Event with ambiguous time warning

    Returns:
        dict: Dictionary with keys:
            - entry_ids: List of review queue entry IDs
            - event_ids: List of event UUIDs
            - place_ids: List of place UUIDs

    Raises:
        psycopg2.Error: If database operations fail
    """
    print("\n" + "=" * 60)
    print("Setting up Review Queue Test Fixtures")
    print("=" * 60)

    conn = None
    created_data = {"entry_ids": [], "event_ids": [], "place_ids": []}

    try:
        conn = get_db_connection()
        print("✓ Connected to database")

        # Create test venue
        venue = create_test_place(conn, "The Rivoli", "Toronto", "ON")
        created_data["place_ids"].append(venue["id"])
        print(f"✓ Created test venue: {venue['ulid']}")

        # Get timestamps for test events
        now = datetime.now(timezone.utc)
        future_date = now + timedelta(days=7)

        # ------------------------------------------------------------------------
        # Fixture 1: Reversed date warning (end before start due to timezone)
        # ------------------------------------------------------------------------
        event1_start = future_date.replace(hour=23, minute=0, second=0, microsecond=0)
        event1_end_wrong = future_date.replace(
            hour=2, minute=0, second=0, microsecond=0
        )  # Looks like same day
        event1_end_correct = event1_end_wrong + timedelta(days=1)  # Actually next day

        event1 = create_test_event(
            conn,
            ulid="TESTRQ0001000000000001",
            name="Late Night Jazz at The Rivoli",
            description="Jazz performance starting 11pm, ends 2am next day",
            venue_id=venue["id"],
        )
        created_data["event_ids"].append(event1["id"])

        create_test_occurrence(
            conn, event1["id"], event1_start, venue["id"], event1_end_correct
        )

        entry1_id = create_review_queue_entry(
            conn,
            event_id=event1["id"],
            original_payload={
                "@context": "https://schema.org",
                "@type": "Event",
                "name": "Late Night Jazz at The Rivoli",
                "startDate": event1_start.isoformat(),
                "endDate": event1_end_wrong.isoformat(),  # Wrong - appears before start
                "location": {"name": "The Rivoli"},
            },
            normalized_payload={
                "@context": "https://schema.org",
                "@type": "Event",
                "name": "Late Night Jazz at The Rivoli",
                "startDate": event1_start.isoformat(),
                "endDate": event1_end_correct.isoformat(),  # Corrected
                "location": {"name": "The Rivoli"},
            },
            warnings=[
                {
                    "field": "endDate",
                    "code": "reversed_dates_timezone_likely",
                    "message": "endDate was 21h before startDate, likely timezone confusion. Corrected by adding 1 day.",
                    "severity": "warning",
                }
            ],
            event_start_time=event1_start,
            event_end_time=event1_end_correct,
            source_id="test-source-1",
            source_external_id="jazz-001",
        )
        created_data["entry_ids"].append(entry1_id)
        print(f"✓ Created fixture 1: Reversed dates (entry_id={entry1_id})")

        # ------------------------------------------------------------------------
        # Fixture 2: Missing venue warning
        # ------------------------------------------------------------------------
        event2_start = future_date.replace(hour=19, minute=0, second=0, microsecond=0)

        event2 = create_test_event(
            conn,
            ulid="TESTRQ0002000000000002",
            name="Online Poetry Reading",
            description="Virtual poetry event - no physical venue",
            venue_id=venue["id"],  # We still need a venue for DB constraints
        )
        created_data["event_ids"].append(event2["id"])

        create_test_occurrence(conn, event2["id"], event2_start, venue["id"])

        entry2_id = create_review_queue_entry(
            conn,
            event_id=event2["id"],
            original_payload={
                "@context": "https://schema.org",
                "@type": "Event",
                "name": "Online Poetry Reading",
                "startDate": event2_start.isoformat(),
                "eventAttendanceMode": "https://schema.org/OnlineEventAttendanceMode",
                # No location field
            },
            normalized_payload={
                "@context": "https://schema.org",
                "@type": "Event",
                "name": "Online Poetry Reading",
                "startDate": event2_start.isoformat(),
                "eventAttendanceMode": "https://schema.org/OnlineEventAttendanceMode",
                "location": {"@type": "VirtualLocation"},
            },
            warnings=[
                {
                    "field": "location",
                    "code": "missing_venue_added_virtual",
                    "message": "No location specified for online event. Added VirtualLocation.",
                    "severity": "info",
                }
            ],
            event_start_time=event2_start,
            source_id="test-source-2",
            source_external_id="poetry-002",
        )
        created_data["entry_ids"].append(entry2_id)
        print(f"✓ Created fixture 2: Missing venue (entry_id={entry2_id})")

        # ------------------------------------------------------------------------
        # Fixture 3: Duplicate detection warning
        # ------------------------------------------------------------------------
        event3_start = future_date.replace(hour=20, minute=0, second=0, microsecond=0)

        event3 = create_test_event(
            conn,
            ulid="TESTRQ0003000000000003",
            name="Art Exhibition Opening",
            description="Gallery opening reception",
            venue_id=venue["id"],
        )
        created_data["event_ids"].append(event3["id"])

        create_test_occurrence(conn, event3["id"], event3_start, venue["id"])

        entry3_id = create_review_queue_entry(
            conn,
            event_id=event3["id"],
            original_payload={
                "@context": "https://schema.org",
                "@type": "Event",
                "name": "Art Exhibition Opening",
                "startDate": event3_start.isoformat(),
                "location": {"name": "The Rivoli"},
            },
            normalized_payload={
                "@context": "https://schema.org",
                "@type": "Event",
                "name": "Art Exhibition Opening",
                "startDate": event3_start.isoformat(),
                "location": {"name": "The Rivoli"},
            },
            warnings=[
                {
                    "field": "event",
                    "code": "potential_duplicate",
                    "message": "Similar event found: 'Art Exhibition Opening' at The Rivoli on similar date",
                    "severity": "warning",
                    "details": {
                        "similar_event_id": "01HQRS7T8G0000000000000099",
                        "similarity_score": 0.87,
                    },
                }
            ],
            event_start_time=event3_start,
            dedup_hash="test-dedup-hash-003",
        )
        created_data["entry_ids"].append(entry3_id)
        print(f"✓ Created fixture 3: Duplicate detection (entry_id={entry3_id})")

        # ------------------------------------------------------------------------
        # Fixture 4: Multiple warnings
        # ------------------------------------------------------------------------
        event4_start = future_date.replace(hour=14, minute=0, second=0, microsecond=0)
        event4_end = future_date.replace(hour=16, minute=0, second=0, microsecond=0)

        event4 = create_test_event(
            conn,
            ulid="TESTRQ0004000000000004",
            name="Community Workshop",
            description="Hands-on arts workshop",
            venue_id=venue["id"],
        )
        created_data["event_ids"].append(event4["id"])

        create_test_occurrence(
            conn, event4["id"], event4_start, venue["id"], event4_end
        )

        entry4_id = create_review_queue_entry(
            conn,
            event_id=event4["id"],
            original_payload={
                "@context": "https://schema.org",
                "@type": "Event",
                "name": "Community Workshop",
                "startDate": "2025-04-15",  # Date only, no time
                "location": "Rivoli",  # String instead of object
            },
            normalized_payload={
                "@context": "https://schema.org",
                "@type": "Event",
                "name": "Community Workshop",
                "startDate": event4_start.isoformat(),
                "endDate": event4_end.isoformat(),
                "location": {"@type": "Place", "name": "The Rivoli"},
            },
            warnings=[
                {
                    "field": "startDate",
                    "code": "time_inferred",
                    "message": "No time specified, defaulted to 2:00 PM",
                    "severity": "warning",
                },
                {
                    "field": "endDate",
                    "code": "duration_inferred",
                    "message": "No end time specified, assumed 2 hour duration",
                    "severity": "info",
                },
                {
                    "field": "location",
                    "code": "location_matched_fuzzy",
                    "message": "Location 'Rivoli' matched to 'The Rivoli' (confidence: 0.92)",
                    "severity": "info",
                },
            ],
            event_start_time=event4_start,
            event_end_time=event4_end,
            source_id="test-source-4",
            source_external_id="workshop-004",
        )
        created_data["entry_ids"].append(entry4_id)
        print(f"✓ Created fixture 4: Multiple warnings (entry_id={entry4_id})")

        # ------------------------------------------------------------------------
        # Fixture 5: Ambiguous timezone warning
        # ------------------------------------------------------------------------
        event5_start = future_date.replace(hour=10, minute=30, second=0, microsecond=0)

        event5 = create_test_event(
            conn,
            ulid="TESTRQ0005000000000005",
            name="Morning Yoga Class",
            description="Outdoor yoga session",
            venue_id=venue["id"],
        )
        created_data["event_ids"].append(event5["id"])

        create_test_occurrence(conn, event5["id"], event5_start, venue["id"])

        entry5_id = create_review_queue_entry(
            conn,
            event_id=event5["id"],
            original_payload={
                "@context": "https://schema.org",
                "@type": "Event",
                "name": "Morning Yoga Class",
                "startDate": "2025-04-15T10:30:00",  # No timezone
                "location": {"name": "The Rivoli"},
            },
            normalized_payload={
                "@context": "https://schema.org",
                "@type": "Event",
                "name": "Morning Yoga Class",
                "startDate": event5_start.isoformat(),
                "location": {"name": "The Rivoli"},
            },
            warnings=[
                {
                    "field": "startDate",
                    "code": "timezone_assumed",
                    "message": "No timezone specified, assumed America/Toronto based on venue location",
                    "severity": "warning",
                }
            ],
            event_start_time=event5_start,
            source_id="test-source-5",
            source_external_id="yoga-005",
        )
        created_data["entry_ids"].append(entry5_id)
        print(f"✓ Created fixture 5: Timezone assumed (entry_id={entry5_id})")

        print("=" * 60)
        print(f"✓ Created {len(created_data['entry_ids'])} review queue test fixtures")
        print("=" * 60 + "\n")

        return created_data

    except Exception as e:
        print(f"✗ Failed to setup fixtures: {e}")
        if conn:
            conn.rollback()
        raise
    finally:
        if conn:
            conn.close()


def cleanup_review_queue_fixtures(created_data=None):
    """
    Clean up test review queue fixtures.

    Deletes review queue entries, events, and places created by setup_review_queue_fixtures.
    Uses transactions to ensure atomic cleanup with rollback on error.

    Args:
        created_data: Dict with 'entry_ids', 'event_ids', 'place_ids' from setup.
                     If None, attempts to clean up all test fixtures by ULID prefix.

    Raises:
        psycopg2.Error: If cleanup fails (transaction will be rolled back)
    """
    print("\n" + "=" * 60)
    print("Cleaning up Review Queue Test Fixtures")
    print("=" * 60)

    conn = None
    try:
        conn = get_db_connection()
        cursor = conn.cursor()

        if created_data:
            # Clean up specific fixtures by ID
            if created_data.get("entry_ids"):
                cursor.execute(
                    "DELETE FROM event_review_queue WHERE id = ANY(%s)",
                    (created_data["entry_ids"],),
                )
                print(f"✓ Deleted {cursor.rowcount} review queue entries")

            if created_data.get("event_ids"):
                cursor.execute(
                    "DELETE FROM event_occurrences WHERE event_id = ANY(%s)",
                    (created_data["event_ids"],),
                )
                print(f"✓ Deleted {cursor.rowcount} event occurrences")

                cursor.execute(
                    "DELETE FROM events WHERE id = ANY(%s)",
                    (created_data["event_ids"],),
                )
                print(f"✓ Deleted {cursor.rowcount} events")

            if created_data.get("place_ids"):
                cursor.execute(
                    "DELETE FROM places WHERE id = ANY(%s)",
                    (created_data["place_ids"],),
                )
                print(f"✓ Deleted {cursor.rowcount} places")
        else:
            # Clean up all test fixtures by ULID prefix pattern
            cursor.execute(
                """
                DELETE FROM event_review_queue
                WHERE event_id IN (
                    SELECT id FROM events WHERE ulid LIKE 'TESTRQ%'
                )
                """
            )
            print(f"✓ Deleted {cursor.rowcount} review queue entries (by ULID pattern)")

            cursor.execute(
                "DELETE FROM event_occurrences WHERE event_id IN (SELECT id FROM events WHERE ulid LIKE 'TESTRQ%')"
            )
            print(f"✓ Deleted {cursor.rowcount} event occurrences (by ULID pattern)")

            cursor.execute("DELETE FROM events WHERE ulid LIKE 'TESTRQ%'")
            print(f"✓ Deleted {cursor.rowcount} events (by ULID pattern)")

            # Note: Places may be shared, so we don't delete them by pattern

        conn.commit()
        cursor.close()

        print("=" * 60)
        print("✓ Cleanup completed successfully")
        print("=" * 60 + "\n")

    except Exception as e:
        print(f"✗ Failed to cleanup fixtures: {e}")
        if conn:
            conn.rollback()
        raise
    finally:
        if conn:
            conn.close()


if __name__ == "__main__":
    """
    Run fixtures setup/cleanup directly for testing.
    
    Usage:
        python review_queue_fixture.py setup
        python review_queue_fixture.py cleanup
    """
    import sys

    if len(sys.argv) < 2:
        print("Usage: python review_queue_fixture.py [setup|cleanup]")
        sys.exit(1)

    command = sys.argv[1]

    if command == "setup":
        try:
            created_data = setup_review_queue_fixtures()
            print("\nFixture IDs (save these for cleanup):")
            print(json.dumps(created_data, indent=2))
        except Exception as e:
            print(f"\n✗ Setup failed: {e}")
            sys.exit(1)

    elif command == "cleanup":
        try:
            cleanup_review_queue_fixtures()
        except Exception as e:
            print(f"\n✗ Cleanup failed: {e}")
            sys.exit(1)

    else:
        print(f"Unknown command: {command}")
        print("Usage: python review_queue_fixture.py [setup|cleanup]")
        sys.exit(1)
