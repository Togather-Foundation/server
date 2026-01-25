# Data Model: SEL Backend Server

**Date**: 2026-01-23  
**Branch**: `001-sel-backend`  
**Status**: Complete  
**Source**: [togather_schema_design.md](../../docs/togather_schema_design.md)

## Overview

The SEL data model implements a three-layer hybrid architecture:
1. **Document Truth Layer (JSONB)**: Original payloads preserved with full provenance
2. **Relational Core Layer (Postgres)**: Fast queries, time-range filtering, geospatial operations
3. **Semantic Export Layer (JSON-LD)**: Generated on-demand, cached, serves federation

This document provides a condensed reference for implementation. Full DDL is in the schema design doc.

---

## Entity Relationship Diagram

```
┌─────────────────┐     ┌──────────────────────┐     ┌─────────────────┐
│     sources     │     │       events         │     │     places      │
├─────────────────┤     ├──────────────────────┤     ├─────────────────┤
│ id              │     │ id                   │     │ id              │
│ name            │     │ ulid                 │     │ ulid            │
│ source_type     │◄────│ name                 │────►│ name            │
│ trust_level     │     │ description          │     │ address_*       │
│ license_type    │     │ lifecycle_state      │     │ geo_point       │
│ is_active       │     │ event_status         │     │ confidence      │
└─────────────────┘     │ organizer_id ────────┼──┐  └─────────────────┘
                        │ primary_venue_id ────┼──┼─►
                        │ series_id            │  │  ┌─────────────────┐
                        │ origin_node_id       │  │  │  organizations  │
                        │ confidence           │  │  ├─────────────────┤
                        │ version              │  └─►│ id              │
                        └──────────────────────┘     │ ulid            │
                                 │                   │ name            │
                                 │                   │ legal_name      │
                                 │                   │ url             │
                        ┌────────┴────────┐          └─────────────────┘
                        ▼                 ▼
            ┌────────────────────┐  ┌────────────────┐
            │ event_occurrences  │  │  event_sources │
            ├────────────────────┤  ├────────────────┤
            │ id                 │  │ id             │
            │ event_id           │  │ event_id       │
            │ start_time         │  │ source_id      │
            │ end_time           │  │ source_url     │
            │ timezone           │  │ payload (JSONB)│
            │ venue_id (override)│  │ confidence     │
            └────────────────────┘  └────────────────┘
```

---

## Core Entities

### 1. Events

Primary entity representing a cultural event. Identity is separated from temporal instances.

| Field | Type | Constraints | Schema.org Mapping |
|-------|------|-------------|-------------------|
| `id` | UUID | PK, auto-generated | — |
| `ulid` | TEXT | UNIQUE, NOT NULL | `@id` (URI component) |
| `name` | TEXT | NOT NULL, 1-500 chars | `schema:name` |
| `description` | TEXT | max 10000 chars | `schema:description` |
| `lifecycle_state` | TEXT | ENUM, default 'draft' | — |
| `event_status` | TEXT | Schema.org EventStatus URI | `schema:eventStatus` |
| `attendance_mode` | TEXT | Schema.org EventAttendanceMode URI | `schema:eventAttendanceMode` |
| `organizer_id` | UUID | FK → organizations | `schema:organizer` |
| `primary_venue_id` | UUID | FK → places | `schema:location` |
| `series_id` | UUID | FK → event_series | `schema:superEvent` |
| `image_url` | TEXT | — | `schema:image` |
| `public_url` | TEXT | — | `schema:url` |
| `virtual_url` | TEXT | — | `schema:VirtualLocation/url` |
| `keywords` | TEXT[] | — | `schema:keywords` |
| `in_language` | TEXT[] | default ['en'] | `schema:inLanguage` |
| `is_accessible_for_free` | BOOLEAN | — | `schema:isAccessibleForFree` |
| `event_domain` | TEXT | ENUM: arts/music/culture/sports/community/education/general | — |
| `origin_node_id` | UUID | FK → federation_nodes | `sel:originNode` |
| `federation_uri` | TEXT | Original URI if federated | — |
| `license_url` | TEXT | default CC0 | `schema:license` |
| `license_status` | TEXT | ENUM: cc0/cc-by/proprietary/unknown | `sel:licenseStatus` |
| `confidence` | DECIMAL(3,2) | 0-1 | `sel:confidence` |
| `quality_score` | INTEGER | 0-100 | — |
| `version` | INTEGER | default 1 | — |
| `created_at` | TIMESTAMPTZ | auto | — |
| `updated_at` | TIMESTAMPTZ | auto | — |
| `published_at` | TIMESTAMPTZ | — | — |
| `deleted_at` | TIMESTAMPTZ | — | `sel:deletedAt` |

**Lifecycle States**: draft, published, postponed, rescheduled, sold_out, cancelled, completed, deleted

**Constraint**: `primary_venue_id IS NOT NULL OR virtual_url IS NOT NULL`

---

### 2. Event Occurrences

Temporal instances of events with timezone preservation.

| Field | Type | Constraints | Schema.org Mapping |
|-------|------|-------------|-------------------|
| `id` | UUID | PK | — |
| `event_id` | UUID | FK → events, CASCADE | — |
| `start_time` | TIMESTAMPTZ | NOT NULL | `schema:startDate` |
| `end_time` | TIMESTAMPTZ | — | `schema:endDate` |
| `timezone` | TEXT | default 'America/Toronto' | — |
| `door_time` | TIMESTAMPTZ | — | `schema:doorTime` |
| `local_date` | DATE | GENERATED | — |
| `local_start_time` | TIME | GENERATED | — |
| `local_day_of_week` | INTEGER | GENERATED (ISODOW) | — |
| `venue_id` | UUID | FK → places (override) | `schema:location` |
| `virtual_url` | TEXT | override | — |
| `occurrence_index` | INTEGER | position in series | — |
| `ticket_url` | TEXT | — | `schema:offers/url` |
| `price_min` | DECIMAL(10,2) | — | `schema:offers/lowPrice` |
| `price_max` | DECIMAL(10,2) | — | `schema:offers/highPrice` |
| `price_currency` | TEXT | default 'CAD' | `schema:offers/priceCurrency` |
| `availability` | TEXT | — | `schema:offers/availability` |

**Constraint**: `end_time >= start_time`

---

### 3. Places

Venues and locations with structured addresses and geospatial support.

| Field | Type | Constraints | Schema.org Mapping |
|-------|------|-------------|-------------------|
| `id` | UUID | PK | — |
| `ulid` | TEXT | UNIQUE, NOT NULL | `@id` (URI component) |
| `name` | TEXT | NOT NULL, 1-300 chars | `schema:name` |
| `description` | TEXT | — | `schema:description` |
| `street_address` | TEXT | — | `schema:address/streetAddress` |
| `address_locality` | TEXT | city | `schema:address/addressLocality` |
| `address_region` | TEXT | province/state | `schema:address/addressRegion` |
| `postal_code` | TEXT | — | `schema:address/postalCode` |
| `address_country` | TEXT | default 'CA' | `schema:address/addressCountry` |
| `full_address` | TEXT | GENERATED | — |
| `latitude` | NUMERIC(10,7) | — | `schema:geo/latitude` |
| `longitude` | NUMERIC(11,7) | — | `schema:geo/longitude` |
| `geo_point` | GEOMETRY(Point) | GENERATED, PostGIS | — |
| `telephone` | TEXT | — | `schema:telephone` |
| `email` | TEXT | — | `schema:email` |
| `url` | TEXT | — | `schema:url` |
| `maximum_attendee_capacity` | INTEGER | — | `schema:maximumAttendeeCapacity` |
| `confidence` | DECIMAL(3,2) | 0-1 | `sel:confidence` |

---

### 4. Organizations

Event organizers, producers, and cultural organizations.

| Field | Type | Constraints | Schema.org Mapping |
|-------|------|-------------|-------------------|
| `id` | UUID | PK | — |
| `ulid` | TEXT | UNIQUE, NOT NULL | `@id` (URI component) |
| `name` | TEXT | NOT NULL, 1-300 chars | `schema:name` |
| `legal_name` | TEXT | — | `schema:legalName` |
| `alternate_name` | TEXT | — | `schema:alternateName` |
| `description` | TEXT | — | `schema:description` |
| `email` | TEXT | — | `schema:email` |
| `telephone` | TEXT | — | `schema:telephone` |
| `url` | TEXT | — | `schema:url` |
| `street_address` | TEXT | — | `schema:address/streetAddress` |
| `address_locality` | TEXT | — | `schema:address/addressLocality` |
| `address_region` | TEXT | — | `schema:address/addressRegion` |
| `postal_code` | TEXT | — | `schema:address/postalCode` |
| `address_country` | TEXT | default 'CA' | `schema:address/addressCountry` |
| `organization_type` | TEXT | — | — |
| `founding_date` | DATE | — | `schema:foundingDate` |
| `confidence` | DECIMAL(3,2) | 0-1 | `sel:confidence` |

---

## Provenance Tables

### 5. Sources

Registry of data sources with trust levels and license information.

| Field | Type | Constraints |
|-------|------|-------------|
| `id` | UUID | PK |
| `name` | TEXT | UNIQUE, NOT NULL |
| `source_type` | TEXT | ENUM: scraper/api/partner/user/federation/manual |
| `base_url` | TEXT | — |
| `trust_level` | INTEGER | 1-10, default 5 |
| `license_url` | TEXT | NOT NULL |
| `license_type` | TEXT | ENUM: CC0/CC-BY/CC-BY-SA/proprietary/unknown |
| `requires_authentication` | BOOLEAN | default false |
| `rate_limit_requests` | INTEGER | — |
| `rate_limit_window_seconds` | INTEGER | — |
| `contact_email` | TEXT | — |
| `is_active` | BOOLEAN | default true |

---

### 6. Event Sources

Row-level provenance tracking which sources contributed to each event.

| Field | Type | Constraints |
|-------|------|-------------|
| `id` | UUID | PK |
| `event_id` | UUID | FK → events |
| `source_id` | UUID | FK → sources |
| `source_url` | TEXT | NOT NULL |
| `source_event_id` | TEXT | External system's ID |
| `retrieved_at` | TIMESTAMPTZ | default now() |
| `payload` | JSONB | NOT NULL, original data |
| `payload_hash` | TEXT | NOT NULL |
| `confidence` | DECIMAL(3,2) | 0-1 |

**Unique**: `(event_id, source_id, source_url)`

---

### 7. Field Provenance

Field-level attribution for critical fields.

| Field | Type | Constraints |
|-------|------|-------------|
| `id` | UUID | PK |
| `event_id` | UUID | FK → events |
| `field_path` | TEXT | JSON Pointer (e.g., /name, /startDate) |
| `value_hash` | TEXT | NOT NULL |
| `value_preview` | TEXT | First 100 chars |
| `source_id` | UUID | FK → sources |
| `confidence` | DECIMAL(3,2) | 0-1 |
| `observed_at` | TIMESTAMPTZ | default now() |
| `is_current` | BOOLEAN | default true |

**Conflict Resolution**: `ORDER BY trust_level DESC, confidence DESC, observed_at DESC`

---

## Federation Tables

### 8. Federation Nodes

Peer SEL nodes in the federation.

| Field | Type | Constraints |
|-------|------|-------------|
| `id` | UUID | PK |
| `node_domain` | TEXT | UNIQUE, NOT NULL |
| `node_name` | TEXT | NOT NULL |
| `trust_level` | INTEGER | 1-10 |
| `is_active` | BOOLEAN | default true |
| `last_sync_at` | TIMESTAMPTZ | — |
| `last_sync_cursor` | TEXT | — |

---

### 9. Event Changes (Outbox)

Change log for federation sync and change feed API.

| Field | Type | Constraints |
|-------|------|-------------|
| `id` | UUID | PK |
| `sequence_number` | BIGSERIAL | Monotonic, per-node ordering (UNIQUE) |
| `event_id` | UUID | FK → events |
| `action` | TEXT | ENUM: create/update/delete |
| `changed_at` | TIMESTAMPTZ | default now() |
| `changed_fields` | JSONB | Array of field paths |
| `snapshot` | JSONB | Event state at change time |

**Indexes**:
- `idx_event_changes_sequence` on `(sequence_number)`
- `idx_event_changes_event` on `(event_id, changed_at DESC)`

---

## Authentication Tables

### 10. Users

Admin user accounts.

| Field | Type | Constraints |
|-------|------|-------------|
| `id` | UUID | PK |
| `email` | TEXT | UNIQUE, NOT NULL |
| `password_hash` | TEXT | NOT NULL |
| `role` | TEXT | ENUM: admin/editor/viewer |
| `is_active` | BOOLEAN | default true |
| `created_at` | TIMESTAMPTZ | default now() |
| `last_login_at` | TIMESTAMPTZ | — |

---

### 11. API Keys

Agent authentication credentials.

| Field | Type | Constraints |
|-------|------|-------------|
| `id` | UUID | PK |
| `prefix` | TEXT | First 8 chars for lookup |
| `key_hash` | TEXT | SHA-256 hash |
| `name` | TEXT | Descriptive name |
| `source_id` | UUID | FK → sources |
| `role` | TEXT | ENUM: agent |
| `rate_limit_tier` | TEXT | — |
| `is_active` | BOOLEAN | default true |
| `created_at` | TIMESTAMPTZ | default now() |
| `last_used_at` | TIMESTAMPTZ | — |
| `expires_at` | TIMESTAMPTZ | — |

---

## State Transitions

### Event Lifecycle

```
                    ┌──────────────┐
                    │    draft     │
                    └──────┬───────┘
                           │ publish
                           ▼
    ┌──────────────────────────────────────────┐
    │               published                   │
    └───┬────────────────┬────────────────┬────┘
        │                │                │
        │ postpone       │ reschedule     │ cancel
        ▼                ▼                ▼
  ┌──────────┐    ┌────────────┐    ┌───────────┐
  │postponed │    │rescheduled │    │ cancelled │
  └────┬─────┘    └─────┬──────┘    └───────────┘
       │                │
       │ reschedule     │ (new occurrence)
       └───────┬────────┘
               ▼
         ┌──────────┐
         │published │ (updated)
         └────┬─────┘
              │ complete (after end_time)
              ▼
         ┌──────────┐
         │completed │
         └──────────┘

Any state → deleted (admin action, returns 410 Gone)
```

---

## Indexes Summary

### Events
- `idx_events_lifecycle` on `(lifecycle_state, updated_at)`
- `idx_events_dedup` on `dedup_hash` WHERE not deleted
- `idx_events_search_vector` GIN full-text on name + description
- `idx_events_name_trgm` GIN trigram for fuzzy matching

### Event Occurrences
- `idx_occurrences_time_range` on `(start_time, end_time)`
- `idx_occurrences_local_date` on `(local_date, local_start_time)`
- `idx_occurrences_day_of_week` on `(local_day_of_week, local_start_time)`

### Places
- `idx_places_geo` GIST on `geo_point`
- `idx_places_locality` on `address_locality`
- `idx_places_name_trgm` GIN trigram

### Event Changes
- `idx_event_changes_sequence` on `(sequence_number)`
- `idx_event_changes_event` on `(event_id, changed_at DESC)`

---

## Validation Rules

### Required Fields (FR-002)
- `name`: non-empty, 1-500 characters
- `startDate`: valid ISO8601 datetime
- At least one of: `location` (Place) or `virtualLocation` (URL)

### Format Validation
- ULIDs: 26-character Crockford Base32
- URIs: valid URL format for `url`, `image_url`, `virtual_url`
- Dates: ISO8601 with timezone
- Coordinates: latitude -90 to 90, longitude -180 to 180

### Business Rules
- `end_time >= start_time`
- `price_max >= price_min`
- Non-CC0 sources rejected at ingestion (FR-015)
- `lifecycle_state` transitions must follow valid paths

---

## Migration Phases

1. **Phase 1**: Core tables (events, places, organizations, occurrences)
2. **Phase 2**: Provenance system (sources, event_sources, field_provenance)
3. **Phase 3**: Federation (federation_nodes, event_changes)
4. **Phase 4**: Authentication (users, api_keys)
5. **Phase 5**: Operational (idempotency_keys, history tables)
