# SEL Core Interoperability Profile v0.1.0-DRAFT

**Status:** Proposed for Community Review  
**Version:** 0.1.0-DRAFT  
**Last Updated:** 2026-01-27  
**Conformance Level:** MUST for Federation, SHOULD for Ingestion  
**Authors:** SEL Architecture Working Group (Ryan Kelln, Gemini 3 Pro, Claude Opus 4.5, OpenAI ChatGPT 5.2)

> **Document Role:** This is the implementation profile derived from the [SEL Interoperability Profile v0.1](../togather_SEL_Interoperability_Profile_v0.1.md). It is the working reference for Togather contributors and includes implementation notes, multi-graph integration details, and updated guidance. The root-level spec is the formal community document; this profile is authoritative for the Togather implementation.

---

## Executive Summary

This document defines the **binding contract** between Shared Events Library (SEL) nodes, external agents (including LLMs), and the Artsdata knowledge graph. It is the authoritative specification for implementation and the "source of truth" for interoperability.

**Design Principles:**
- **Schema.org First:** All output uses schema.org vocabulary with explicit namespaces for extensions
- **Artsdata Compatible:** Output validates against Artsdata SHACL shapes
- **Linked Data Native:** Every entity has a dereferenceable URI; relationships use URIs not strings
- **Provenance as First-Class:** Every fact traces to a source with confidence and timestamp
- **Federation by Design:** URIs encode origin; sync protocols assume no central authority
- **License Clarity:** Only CC0-compatible data enters the public graph

---

## Table of Contents

1. [URI Scheme and Identifier Rules](#1-uri-scheme-and-identifier-rules)
2. [Canonical JSON-LD Output](#2-canonical-json-ld-output)
3. [Minimal Required Fields & SHACL Validation](#3-minimal-required-fields--shacl-validation)
6. [Provenance Model](#6-provenance-model)
7. [License Policy](#7-license-policy)
8. [Test Suite & Conformance](#8-test-suite--conformance)
9. [Appendices](#9-appendices)

---

## 1. URI Scheme and Identifier Rules

### 1.1 URI Structure

SEL uses a **Federated Identity Model**. Every entity belongs to an "Origin Node" but allows global linking.

**Pattern:**
```
https://{node-domain}/{entity-type}/{ulid}
```

**Components:**

| Component | Description | Example |
|-----------|-------------|---------|
| `node-domain` | FQDN of authoritative SEL node | `toronto.togather.foundation` |
| `entity-type` | Plural lowercase entity type | `events`, `places`, `organizations`, `persons` |
| `ulid` | Universally Unique Lexicographically Sortable Identifier | `01HYX3KQW7ERTV9XNBM2P8QJZF` |

**Examples:**
```
https://toronto.togather.foundation/events/01HYX3KQW7ERTV9XNBM2P8QJZF
https://toronto.togather.foundation/places/01HYX4ABCD1234567890VENUE
https://toronto.togather.foundation/organizations/01HYX5EFGH0987654321ORGAN
```

**Why ULID?**
- Timestamp-prefixed (sortable, enables efficient time-range queries)
- 128-bit cryptographically random suffix (globally unique)
- URL-safe Base32 encoding
- First 10 characters encode creation timestamp (debuggable)

**Requirement:** ULIDs MUST be **true ULIDs** (not UUID re-encodings).

**Implementation Default (MVP):** ULIDs are generated in the application using a ULID library with UTC timestamps and Crockford Base32 encoding (26 chars).
**Recommended Go Library:** `github.com/oklog/ulid/v2`

### 1.2 Identifier Roles

SEL distinguishes three identifier types:

| Field | Purpose | Controlled By | Example |
|-------|---------|---------------|---------|
| `@id` | Canonical URI for linked data | SEL node | `https://toronto.togather.foundation/events/01HYX...` |
| `url` | Public webpage for humans | External (venue/ticketing) | `https://masseyhall.com/events/jazz-night` |
| `sameAs` | Equivalent entities in external systems | External authorities | `http://kg.artsdata.ca/resource/K12-345` |

**Critical Rule:** `@id` ≠ `url`

The `@id` is YOUR stable semantic identifier. The `url` is where the event is promoted publicly.

### 1.3 Federation Rule (Preservation of Origin)

> **MUST:** A node MUST NOT generate an `@id` using another node's domain. When ingesting data from Montreal, the Toronto node MUST preserve the Montreal `@id`.

**Example:**
```go
func TestURIMinting(t *testing.T) {
    mtlEvent := IngestEvent{
        OriginNode: "https://montreal.togather.foundation",
        OriginalID: "01HQ...",
    }
    
    uri := Minters.CanonicalURI(mtlEvent)
    
    // Rule: Preserve original URI
    if uri != "https://montreal.togather.foundation/events/01HQ..." {
        t.Errorf("Federated event MUST preserve original URI")
    }
}
```

### 1.4 sameAs Usage

`sameAs` links ONLY to authoritative external identifiers from registered knowledge graphs.

**Supported Authorities:**

SEL supports multiple knowledge graphs to enable linking for both arts and non-arts events. The complete registry is maintained in the `knowledge_graph_authorities` database table. Core authorities include:

| Authority | URI Pattern | Domain Coverage | Example |
|-----------|-------------|-----------------|---------|
| **Artsdata** | `http://kg.artsdata.ca/resource/K{digits}-{digits}` | Arts, Culture, Music | `http://kg.artsdata.ca/resource/K11-211` |
| **Wikidata** | `http://www.wikidata.org/entity/Q{digits}` | Universal (all domains) | `http://www.wikidata.org/entity/Q636342` |
| **MusicBrainz** | `https://musicbrainz.org/{type}/{uuid}` | Music | `https://musicbrainz.org/artist/...` |
| **ISNI** | `https://isni.org/isni/{16-digits}` | Persons, Organizations | `https://isni.org/isni/0000000121032683` |
| **OpenStreetMap** | `https://www.openstreetmap.org/{type}/{id}` | Places, Venues | `https://www.openstreetmap.org/node/123456` |

**Multi-Graph Linking:**

Events MAY include `sameAs` links to **multiple authorities** simultaneously. For example, a music event might link to:
- Artsdata (Canadian arts context)
- MusicBrainz (music-specific data)
- Wikidata (universal identifier)

See [Knowledge Graph Integration Strategy](./knowledge-graphs.md) for complete multi-graph integration details.

```json
{
  "@id": "https://toronto.togather.foundation/events/01J8...",
  "sameAs": [
    "http://kg.artsdata.ca/resource/K11-456",
    "https://musicbrainz.org/event/abc-123-def",
    "http://www.wikidata.org/entity/Q12345"
  ]
}
```

**Domain-Based Reconciliation:**

SEL uses domain-aware reconciliation routing (see [Knowledge Graph Integration Strategy](./knowledge-graphs.md)):
- **Arts/Culture/Music events**: Reconcile with Artsdata first, fall back to Wikidata
- **Sports/Community/Education events**: Reconcile with Wikidata directly
- **Places**: Attempt multiple graphs (Artsdata, OpenStreetMap, Wikidata) and merge results

**Validation:**

All authority URIs MUST match the registered `base_uri_pattern` in the `knowledge_graph_authorities` table. Implementations SHOULD validate `sameAs` URIs against these patterns before accepting submissions.

**Anti-Pattern:**
- Do NOT use `sameAs` for a Ticketmaster URL or other promotional pages
- Use `schema:url` for public pages or `prov:wasDerivedFrom` for source attribution
- Do NOT fabricate or guess external identifiers — only include verified links

### 1.5 URI Dereferencing

**Dereferenceable URIs** (e.g., `https://toronto.togather.foundation/events/{id}`) MUST support content negotiation:

| Accept Header | Response |
|---------------|----------|
| `text/html` | Human-readable HTML page |
| `application/ld+json` | JSON-LD document |
| `application/json` | JSON-LD document (alias) |
| `text/turtle` | RDF Turtle serialization |

**API endpoints** (e.g., `/api/v1/events/{id}`) serve JSON-LD only (`application/json` and `application/ld+json`).

**MVP requirement:** Provide **both HTML and JSON-LD** for dereferenceable URIs (HTML for human review, JSON-LD for agents).

**HTML Rendering Minimums:**
- Render `name`, `startDate`, `location` (Place or VirtualLocation), and `organizer` when available.
- Embed canonical JSON-LD in `<script type="application/ld+json">...</script>`.
- Include the canonical `@id` and `url` (if present).

**Example:**
```bash
curl -H "Accept: application/ld+json" \
  https://toronto.togather.foundation/events/01HYX3KQW7ERTV9XNBM2P8QJZF
```

### 1.6 Tombstone URIs (Deleted Entities)

Deleted entities MUST return **HTTP 410 Gone** with a minimal JSON-LD tombstone:

```json
{
  "@context": "https://schema.org",
  "@type": "Event",
  "@id": "https://toronto.togather.foundation/events/01HYX3KQW7ERTV9XNBM2P8QJZF",
  "eventStatus": "https://schema.org/EventCancelled",
  "sel:tombstone": true,
  "sel:deletedAt": "2025-01-20T15:00:00Z",
  "sel:deletionReason": "duplicate_merged",
  "sel:supersededBy": "https://toronto.togather.foundation/events/01HYX4MERGED..."
}
```

### 1.7 Profile Discovery

SEL nodes MUST expose profile information at:

**Endpoint:** `GET /.well-known/sel-profile`

```json
{
  "profile": "https://sel.events/profiles/interop",
  "version": "0.1.0",
  "node": "https://toronto.togather.foundation",
  "updated": "2025-01-20"
}
```

**OpenAPI Specification:**

SEL nodes MUST provide their API specification at:

**Endpoint:** `GET /api/v1/openapi.json`

Returns OpenAPI 3.1.0 specification in JSON format, enabling:
- Programmatic discovery of all API endpoints
- Client library generation
- Request/response validation
- API compatibility verification for federation

---

## 2. Canonical JSON-LD Output

### 2.1 Context Definition

SEL uses a static, versioned context to ensure stability:

**Context URL:** `https://togather.foundation/contexts/sel/v0.1.jsonld`

**Local Source (repo):** `contexts/sel/v0.1.jsonld`

**Structure:**
```json
{
  "@context": [
    "https://schema.org",
    "https://toronto.togather.foundation/contexts/sel/v0.1.jsonld"
  ]
}
```

The SEL context defines `sel:*` terms for provenance and lifecycle metadata not native to schema.org.

**Key Extensions:**

| Term | Purpose | Type |
|------|---------|------|
| `sel:originNode` | Node that minted this URI | @id |
| `sel:sourceUrl` | Original source URL | @id |
| `sel:ingestedAt` | Ingestion timestamp | xsd:dateTime |
| `sel:confidence` | Confidence score (0-1) | xsd:decimal |
| `sel:tombstone` | Deletion flag | xsd:boolean |
| `sel:licenseStatus` | License classification | xsd:string |
| `sel:takedownRequested` | Takedown flag | xsd:boolean |
| `sel:takedownRequestedAt` | Takedown timestamp | xsd:dateTime |

### 2.2 Event Output

#### 2.2.1 Minimal Event (Required Fields Only)

```json
{
  "@context": [
    "https://schema.org",
    "https://togather.foundation/contexts/sel/v0.1.jsonld"
  ],
  "@id": "https://toronto.togather.foundation/events/01J8...",
  "@type": "Event",
  "name": "Jazz in the Park",
  "startDate": "2025-07-15T19:00:00-04:00",
  "location": {
    "@id": "https://toronto.togather.foundation/places/01J9...",
    "@type": "Place",
    "name": "Centennial Park"
  }
}
```

#### 2.2.2 Full Event (Recommended Fields)

```json
{
  "@context": [
    "https://schema.org",
    "https://togather.foundation/contexts/sel/v0.1.jsonld"
  ],
  "@id": "https://toronto.togather.foundation/events/01J8...",
  "@type": "Event",
  "name": "Jazz in the Park: Summer Opener",
  "description": "An evening of smooth jazz under the stars...",
  "eventStatus": "https://schema.org/EventScheduled",
  "eventAttendanceMode": "https://schema.org/OfflineEventAttendanceMode",
  
  "startDate": "2025-07-15T19:00:00-04:00",
  "endDate": "2025-07-15T22:00:00-04:00",
  "doorTime": "2025-07-15T18:00:00-04:00",
  
  "location": {
    "@id": "https://toronto.togather.foundation/places/01J9...",
    "@type": "Place",
    "name": "Centennial Park",
    "address": {
      "@type": "PostalAddress",
      "streetAddress": "256 Centennial Park Rd",
      "addressLocality": "Toronto",
      "addressRegion": "ON",
      "postalCode": "M9C 5N3",
      "addressCountry": "CA"
    },
    "geo": {
      "@type": "GeoCoordinates",
      "latitude": 43.6426,
      "longitude": -79.5652
    },
    "sameAs": [
      "http://kg.artsdata.ca/resource/K11-234"
    ]
  },

  "organizer": {
    "@id": "https://toronto.togather.foundation/organizations/01JA...",
    "@type": "Organization",
    "name": "Toronto Arts Council",
    "sameAs": ["http://kg.artsdata.ca/resource/K10-555"]
  },

  "performer": [
    {
      "@type": "Person",
      "@id": "https://toronto.togather.foundation/persons/01HYX6...",
      "name": "Sarah Chen Quartet"
    }
  ],

  "offers": {
    "@type": "Offer",
    "url": "https://ticketmaster.ca/event/...",
    "price": "0",
    "priceCurrency": "CAD",
    "availability": "https://schema.org/InStock"
  },

  "image": "https://images.toronto.togather.foundation/...",
  "url": "https://torontoartscouncil.org/events/jazz-park-2025",
  
  "inLanguage": ["en", "fr"],
  "isAccessibleForFree": true,
  
  "sel:originNode": "https://toronto.togather.foundation",
  "sel:ingestedAt": "2025-07-10T12:00:00Z",
  "sel:confidence": 0.95,
  
  "license": "https://creativecommons.org/publicdomain/zero/1.0/",
  
  "prov:wasDerivedFrom": {
    "@type": "prov:Entity",
    "schema:name": "Ticketmaster API",
    "schema:url": "https://ticketmaster.ca/..."
  }
}
```

#### 2.2.3 Recurring Event (Series with Instances)

```json
{
  "@context": [
    "https://schema.org",
    "https://togather.foundation/contexts/sel/v0.1.jsonld"
  ],
  "@type": "EventSeries",
  "@id": "https://toronto.togather.foundation/events/01HYX7SERIES...",
  
  "name": "Friday Night Jazz",
  "description": "Weekly jazz performances at the Rex Hotel",
  
  "location": {
    "@id": "https://toronto.togather.foundation/places/01HYX8REXHOTEL..."
  },
  
  "startDate": "2025-01-03T20:00:00-05:00",
  "endDate": "2025-12-26T23:00:00-05:00",
  
  "eventSchedule": {
    "@type": "Schedule",
    "repeatFrequency": "P1W",
    "byDay": "https://schema.org/Friday",
    "startTime": "20:00:00",
    "endTime": "23:00:00",
    "scheduleTimezone": "America/Toronto"
  },
  
  "subEvent": [
    {
      "@type": "Event",
      "@id": "https://toronto.togather.foundation/events/01HYX7SERIES...001",
      "startDate": "2025-01-03T20:00:00-05:00",
      "superEvent": "https://toronto.togather.foundation/events/01HYX7SERIES..."
    }
  ]
}
```

**Serialization Rules (Series ↔ Occurrences):**
- `event_series` maps to a single `EventSeries` object.
- Each `event_occurrence` maps to a child `Event` with:
  - `@id` = occurrence URI (or series URI + suffix)
  - `superEvent` = series `@id`
  - `startDate`, `endDate`, `doorTime`
  - `location` = occurrence override (`venue_id` or `virtual_url`), otherwise series default
- The `EventSeries` MAY include `subEvent` with a bounded list of upcoming occurrences (configurable window), and MUST expose `eventSchedule` for recurrence rules.

### 2.3 Place Output

```json
{
  "@context": [
    "https://schema.org",
    "https://togather.foundation/contexts/sel/v0.1.jsonld"
  ],
  "@id": "https://toronto.togather.foundation/places/01HYX4...",
  "@type": "Place",
  
  "name": "Massey Hall",
  "description": "Historic concert hall renowned for exceptional acoustics",
  
  "address": {
    "@type": "PostalAddress",
    "streetAddress": "178 Victoria Street",
    "addressLocality": "Toronto",
    "addressRegion": "ON",
    "postalCode": "M5B 1T7",
    "addressCountry": "CA"
  },
  
  "geo": {
    "@type": "GeoCoordinates",
    "latitude": 43.6544,
    "longitude": -79.3807
  },
  
  "telephone": "+1-416-872-4255",
  "url": "https://masseyhall.com",
  "maximumAttendeeCapacity": 2752,
  
  "sameAs": [
    "http://kg.artsdata.ca/resource/K11-456",
    "http://www.wikidata.org/entity/Q3297877"
  ],
  
  "sel:originNode": "https://toronto.togather.foundation",
  "sel:confidence": 0.98
}
```

### 2.4 Organization Output

```json
{
  "@context": [
    "https://schema.org",
    "https://togather.foundation/contexts/sel/v0.1.jsonld"
  ],
  "@id": "https://toronto.togather.foundation/organizations/01HYX5...",
  "@type": "Organization",
  
  "name": "Toronto Symphony Orchestra",
  "alternateName": "TSO",
  "url": "https://tso.ca",
  
  "address": {
    "@type": "PostalAddress",
    "streetAddress": "60 Simcoe Street",
    "addressLocality": "Toronto",
    "addressRegion": "ON",
    "postalCode": "M5J 2H5",
    "addressCountry": "CA"
  },
  
  "sameAs": [
    "http://kg.artsdata.ca/resource/K15-123",
    "http://www.wikidata.org/entity/Q1818498"
  ]
}
```

### 2.5 Input Acceptance (Flexible Ingestion)

SEL nodes MUST accept schema.org-compliant Event data for ingestion without requiring
node-specific field formats. Both **flat** (legacy) and **nested** (schema.org canonical)
input formats are accepted and normalized at the ingestion boundary.

**Flexibility Points:**

| Field | Accepted Formats |
|-------|-----------------|
| `location` | Place object with nested `address`/`geo`, OR flat string (venue name) |
| `image` | URL string, OR `ImageObject` with `url` key |
| `organizer` | Organization object, OR string URI |
| `keywords` | Array of strings, OR comma-separated string |
| `inLanguage` | Array of strings, OR single string |
| `offers` | Single Offer object, OR array of Offer objects |
| `@context` | Accepted and ignored gracefully (not stored) |
| `@type` | Schema.org event subtypes (e.g., `MusicEvent`, `DanceEvent`) mapped to domain categories |

**Event Subtype Mapping:**

Schema.org event subtypes are mapped to SEL domain categories:

| Schema.org Subtypes | SEL Domain |
|--------------------|------------|
| `MusicEvent` | `music` |
| `SportsEvent` | `sports` |
| `EducationEvent` | `education` |
| `SocialEvent` | `community` |
| `DanceEvent`, `TheaterEvent`, `ComedyEvent`, `LiteraryEvent`, `VisualArtsEvent`, `ScreeningEvent`, `ExhibitionEvent` | `arts` |
| `Event` (generic) | `general` |

**Example (nested schema.org input):**
```json
{
  "@context": "https://schema.org",
  "@type": "MusicEvent",
  "name": "Jazz in the Park",
  "startDate": "2025-07-15T19:00:00-04:00",
  "location": {
    "@type": "Place",
    "name": "Centennial Park",
    "address": {
      "@type": "PostalAddress",
      "streetAddress": "256 Centennial Park Rd",
      "addressLocality": "Toronto",
      "addressRegion": "ON"
    },
    "geo": {
      "@type": "GeoCoordinates",
      "latitude": 43.6426,
      "longitude": -79.5652
    }
  },
  "organizer": {
    "@type": "Organization",
    "name": "Toronto Arts Council"
  },
  "image": {
    "@type": "ImageObject",
    "url": "https://example.com/jazz.jpg"
  },
  "offers": {
    "@type": "Offer",
    "price": 0,
    "priceCurrency": "CAD"
  }
}
```

---

## 3. Minimal Required Fields & SHACL Validation

### 3.1 Required Fields by Entity Type

#### Event (Minimum Viable)

| Field | Type | Required | Validation |
|-------|------|----------|------------|
| `@id` | URI | MUST | Valid SEL URI pattern |
| `@type` | String | MUST | `Event`, `EventSeries`, or `Festival` |
| `name` | String | MUST | Non-empty, max 500 chars |
| `startDate` | DateTime | MUST | ISO 8601 with timezone |
| `location` | Place/VirtualLocation/URI | MUST | Valid Place or VirtualLocation object or SEL URI |
| `eventStatus` | URI | SHOULD | schema.org EventStatusType (defaults to EventScheduled) |

### 3.4 Deduplication Identity Rules (MVP)

An event identity is defined by **name + startDate + location** (or virtual URL), with tolerance windows.

**Rules:**
- Primary identity key: normalized `name` + normalized `startDate` (±15 minutes) + `location` (venue or virtual URL).
- If `series_id` is present, include `occurrence_index` in the identity key.
- If `eventAttendanceMode` is online, use `virtual_url` in place of venue.

**Normalization:**
- `name`: lowercased, trimmed, collapse whitespace
- `startDate`: round to nearest 5 minutes for identity comparisons
- `location`: resolve to canonical Place `@id` where available

**Virtual / Hybrid Events:**
- If `eventAttendanceMode` is `OnlineEventAttendanceMode`, `location` MUST be `VirtualLocation` with a `url`.
- If `MixedEventAttendanceMode`, `location` SHOULD include both a `Place` and a `VirtualLocation`.

#### Place (Minimum Viable)

| Field | Type | Required | Validation |
|-------|------|----------|------------|
| `@id` | URI | MUST | Valid SEL URI pattern |
| `@type` | String | MUST | `Place` |
| `name` | String | MUST | Non-empty, max 300 chars |
| `address` OR `geo` | Object | MUST (one) | Valid PostalAddress or GeoCoordinates |

#### Organization (Minimum Viable)

| Field | Type | Required | Validation |
|-------|------|----------|------------|
| `@id` | URI | MUST | Valid SEL URI pattern |
| `@type` | String | MUST | `Organization` |
| `name` | String | MUST | Non-empty, max 300 chars |

### 3.2 SHACL Shapes

#### Event Shape (Turtle)

```turtle
@prefix sh: <http://www.w3.org/ns/shacl#> .
@prefix schema: <https://schema.org/> .
@prefix xsd: <http://www.w3.org/2001/XMLSchema#> .
@prefix sel: <https://schema.togather.foundation/ns#> .

sel:EventShape
    a sh:NodeShape ;
    sh:targetClass schema:Event ;
    sh:nodeKind sh:IRI ;
    
    # Required: name
    sh:property [
        sh:path schema:name ;
        sh:minCount 1 ;
        sh:maxCount 1 ;
        sh:datatype xsd:string ;
        sh:minLength 1 ;
        sh:maxLength 500 ;
        sh:message "Event must have exactly one name (1-500 characters)" ;
    ] ;
    
    # Required: startDate
    sh:property [
        sh:path schema:startDate ;
        sh:minCount 1 ;
        sh:maxCount 1 ;
        sh:or (
            [ sh:datatype xsd:dateTime ]
            [ sh:datatype xsd:date ]
        ) ;
        sh:pattern "^\\d{4}-\\d{2}-\\d{2}T\\d{2}:\\d{2}:\\d{2}" ;
        sh:message "Event must have startDate in ISO8601 with time" ;
    ] ;
    
    # Required: location
    sh:property [
        sh:path schema:location ;
        sh:minCount 1 ;
        sh:or (
            [ sh:class schema:Place ]
            [ sh:class schema:VirtualLocation ]
            [ sh:nodeKind sh:IRI ]
        ) ;
        sh:message "Event must have a Place, VirtualLocation, or URI" ;
    ] ;
    
    # Constraint: endDate >= startDate
    sh:sparql [
        sh:message "endDate must be >= startDate" ;
        sh:select """
            SELECT $this
            WHERE {
                $this schema:startDate ?start .
                $this schema:endDate ?end .
                FILTER (?end < ?start)
            }
        """ ;
    ] ;
    
    # Optional: eventStatus (must be valid enum)
    sh:property [
        sh:path schema:eventStatus ;
        sh:maxCount 1 ;
        sh:in (
            schema:EventScheduled
            schema:EventCancelled
            schema:EventPostponed
            schema:EventRescheduled
            schema:EventMovedOnline
        ) ;
    ] .
```

#### Place Shape

```turtle
sel:PlaceShape
    a sh:NodeShape ;
    sh:targetClass schema:Place ;
    sh:nodeKind sh:IRI ;
    
    # Required: name
    sh:property [
        sh:path schema:name ;
        sh:minCount 1 ;
        sh:datatype xsd:string ;
        sh:minLength 1 ;
        sh:maxLength 300 ;
    ] ;
    
    # Required: address OR geo
    sh:or (
        [ sh:property [
            sh:path schema:address ;
            sh:minCount 1 ;
            sh:class schema:PostalAddress ;
        ] ]
        [ sh:property [
            sh:path schema:geo ;
            sh:minCount 1 ;
            sh:class schema:GeoCoordinates ;
        ] ]
    ) .
```

### 3.3 Implementation Tests

```go
func TestCanonicalJSONLD(t *testing.T) {
    event := factories.CreateEvent()
    jsonld := serializers.ToJSONLD(event)

    // Rule: @id must start with node domain
    if !strings.HasPrefix(jsonld["@id"].(string), "https://test.node") {
        t.Errorf("Invalid @id scheme")
    }

    // Rule: Must have license
    if jsonld["license"] != "https://creativecommons.org/publicdomain/zero/1.0/" {
        t.Errorf("Missing CC0 license")
    }
}

func TestEventValidation(t *testing.T) {
    // Invalid Event (No Location)
    evt := map[string]interface{}{
        "@type": "Event",
        "name": "Floating Concert",
    }
    
    report, valid := shacl.Validate(evt, "event-shape.ttl")
    
    if valid {
        t.Error("Event without location should fail SHACL validation")
    }
}
```

**Local SHACL Sources (repo):**
- `shapes/event-v0.1.ttl`
- `shapes/place-v0.1.ttl`
- `shapes/organization-v0.1.ttl`

**CI Expectation:** Validate exported JSON-LD against these shapes on every release build.

---

## 6. Provenance Model

### 6.1 Sources Registry

Every ingestion MUST come from a registered Source:

| Field | Type | Description |
|-------|------|-------------|
| `id` | String | Unique source identifier |
| `name` | String | Human-readable name |
| `type` | Enum | `scraper`, `partner`, `user`, `federation` |
| `base_url` | URL | Source base URL |
| `license` | String | CC0, CC-BY, proprietary, unknown |
| `trust_level` | Integer | 1 (Low) to 10 (High) |
| `contact` | String | Email/URL for source contact |

### 6.2 Event Source Observations

Each ingestion creates an immutable observation:

```sql
event_sources (
  event_id,
  source_id,
  source_url,
  retrieved_at,
  payload,           -- raw JSON/JSON-LD
  payload_hash
)
```

### 6.3 Field-Level Provenance

SEL SHOULD track:

```sql
field_claims (
  event_id,
  field_path,        -- JSON Pointer: /name, /location/address/postalCode
  value_hash,
  source_id,
  confidence,
  observed_at
)
```

### 6.4 Merge Policy (Normative)

**Rules:**
- SEL MUST preserve all source observations
- SEL MUST NOT silently overwrite higher-trust fields with lower-trust fields
- Conflicts between equal-trust sources SHOULD be flagged for review
- Winner selection: `trust_level DESC, confidence DESC, observed_at DESC`

### 6.5 Attribution in Export

```json
"prov:wasDerivedFrom": {
  "@type": "prov:Entity",
  "schema:name": "Ticketmaster API",
  "schema:url": "https://ticketmaster.ca/..."
}
```

---

## 7. License Policy

### 7.1 Ingestion Policy

SEL accepts data under these terms:

1. **Public Facts:** Name, Date, Location, Price (facts are not copyrightable)
2. **Licensed Content:** Descriptions/Images MUST be compatible with **CC0** or **CC-BY**

**If source has restrictive license:**
- Ingest facts
- Descriptions MAY be retained **with explicit takedown flags** and provenance
- Implementers MUST support removal on request

**License Flags (MVP):**
- `sel:licenseStatus` values: `cc0`, `cc-by`, `proprietary`, `unknown`
- `sel:takedownRequested` boolean (default false)
- `sel:takedownRequestedAt` when applicable

### 7.2 Publication Policy

All data emitted by SEL API is **CC0 1.0 Universal** (Public Domain Dedication).

**License URI:** `https://creativecommons.org/publicdomain/zero/1.0/`

This MUST appear in:
- Every exported JSON-LD document
- Dataset-level metadata

### 7.3 Attribution

While CC0 doesn't legally require attribution, SEL requests downstream users credit:

**Attribution String:** "Shared Events Library - {City Node}"

Example:
```json
"sel:provenance": {
  "sel:source": "https://toronto.togather.foundation/sources/ticketing-scraper",
  "sel:retrievedAt": "2026-07-10T10:00:00Z",
  "sel:sourceName": "Ticketmaster API",
  "sel:license": "https://creativecommons.org/publicdomain/zero/1.0/"
}
```

---

## 8. Test Suite & Conformance

### 8.1 Repository Structure

SEL implementations MUST provide:
- JSON-LD context files at `/contexts/sel/v{version}.jsonld`
- SHACL shapes at `/shapes/{entity}-v{version}.ttl`
- JSON-LD frames at `/frames/{entity}-v{version}.frame.json`

Reference the shapes in `docs/interop/schemas/` for validation requirements.

**Testing Requirements:**
1. All exported JSON-LD MUST validate against SHACL shapes
2. URI patterns MUST follow § 1.1 structure
3. Content negotiation MUST work for dereferenceable URIs
4. Profile discovery MUST respond at `/.well-known/sel-profile`

### 8.2 Conformance Levels

| Level | Requirements |
|-------|--------------|
| **Bronze** | Valid JSON-LD output, URI dereferencing |
| **Silver** | + SHACL validation, content negotiation |
| **Gold** | + Change feeds, federation sync |
| **Platinum** | + Multi-graph reconciliation, provenance tracking |

---

## 9. Appendices

### 9.1 Related Documents

- **API Contract:** See [api-contract-v1.md](./api-contract-v1.md) for HTTP API specifications
- **Federation Protocol:** See [federation-v1.md](./federation-v1.md) for sync protocols
- **Knowledge Graph Integration:** See [knowledge-graphs.md](./knowledge-graphs.md)

### 9.2 Glossary

| Term | Definition |
|------|------------|
| **SEL Node** | An instance of a Shared Events Library server |
| **Origin Node** | The SEL node that originally minted an entity's URI |
| **ULID** | Universally Unique Lexicographically Sortable Identifier |
| **Tombstone** | A placeholder response for a deleted entity |
| **Reconciliation** | The process of matching local entities to external knowledge graphs |

### 9.3 Version History

| Version | Date | Notes |
|---------|------|-------|
| 0.1.0-DRAFT | 2026-02-11 | Added § 2.5 Input Acceptance (flexible ingestion formats, event subtype mapping) |
| 0.1.0-DRAFT | 2026-01-27 | Drafted core profile |
| 0.1.0-DRAFT | 2025-01-20 | Initial draft |
