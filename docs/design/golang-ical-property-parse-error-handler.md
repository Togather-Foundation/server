# golang-ical: Property Parse Error Handler — Design Notes

> Working document for bead `srv-7bi1l`.
> Tracks the rationale behind our upstream PR to `arran4/golang-ical`
> and the togather-side integration.

## Problem

Tockify ICS feeds (powering the `torevent` source, ~2,900 events) contain
`X-TKF-PROMOTION-BUTTON` properties with underscore-containing parameter
names:

```
X-TKF-PROMOTION-BUTTON;skip_details=false;button_text=Buy Tickets:https://...
```

RFC 5545 §3.1 defines IANA tokens as `1*(ALPHA / DIGIT / "-")` — no
underscore. The `arran4/golang-ical` library enforces this via
`propertyParamNameReg = [A-Za-z0-9-]{1,}` (property.go:273/277).

The regex matches `skip` (stops at `_`), then `parsePropertyParam` expects
`=` but finds `_`, producing:

```
missing property value for skip in X-TKF-PROMOTION-BUTTON
```

This error propagates: `ParseProperty` → `ParseComponent` → entire calendar
parse aborts. The `torevent` source is currently disabled.

### Why `AcceptUnknownPropertyHandler` doesn't help

The existing `WithUnknownPropertyHandler` fires **after** `ParseProperty()`
succeeds (calendar.go:785). Our error happens **inside** `ParseProperty()`
during parameter parsing — the handler never gets a chance to run.

## Approaches Considered

### Option 1: Widen the IANA token regex to allow `_` ❌

```go
// Allow underscores in property parameter names
propertyParamNameReg = regexp.MustCompile("[A-Za-z0-9_-]{1,}")
```

**Pros:**
- One-line fix.
- Would unblock torevent immediately.

**Cons:**
- Violates RFC 5545 §3.1 — the maintainer may not accept a blanket
  relaxation.
- Could mask other parse errors by accepting tokens that should fail.
- Not opt-in: affects every caller globally.
- Doesn't help with other kinds of malformed properties (missing values,
  bad quoting, etc.).

**Verdict:** Simple but likely rejected upstream. Not general enough.

### Option 2: Pre-process raw ICS bytes before parsing ❌

Strip or sanitize `X-TKF-*` lines from raw bytes using regex before passing
to the parser.

**Pros:**
- No library changes needed.
- Can deploy immediately.

**Cons:**
- Tockify **line-folds** the property. The raw bytes look like:

  ```
  X-TKF-PROMOTION-BUTTON;skip_
   details=false;button_text=Buy Tickets:https://example.com
  ```

  A regex sees `skip_` without `=` on line 1 and doesn't match. Fix
  requires unfolding the entire buffer first, then normalizing, then
  re-folding — duplicating library internals.
- Fragile: must avoid mangling `_` in property values (URLs, descriptions).
- Source-specific: doesn't help with other malformed feeds we'll encounter.
- Maintenance burden lives entirely on us.

**Verdict:** Rejected. Fragile, duplicates library logic, doesn't generalize.

### Option 3: `WithPropertyParseErrorHandler` ParseOption ✅ (chosen)

A new `ParseOption` that registers a callback invoked when `ParseProperty()`
returns an error. Follows the library's existing `WithUnknownPropertyHandler`
pattern exactly.

```go
// Handler signature:
//   rawLine: the unfolded content line that failed to parse
//   err:     the parse error from ParseProperty
//
// Returns:
//   (*BaseProperty, nil) → use the returned property as a replacement
//   (nil, nil)           → skip this property silently
//   (nil, err)           → abort parsing with the error
type PropertyParseErrorHandler func(rawLine ContentLine, err error) (*BaseProperty, error)

func WithPropertyParseErrorHandler(f PropertyParseErrorHandler) ParseOption
```

**Pros:**
- **Opt-in:** Default behavior unchanged (strict parsing, full backward
  compat). Only callers who pass the option get lenient behavior.
- **General:** Handles any property parse error, not just underscores.
  Future malformed feeds are covered.
- **Follows established patterns:** Mirrors `WithUnknownPropertyHandler` in
  API shape, storage (field on `Calendar`), and threading.
- **Caller controls policy:** Skip, recover, or abort — the library doesn't
  impose a decision.
- **Small diff:** ~80 lines of library code, no API removals, no breaking
  changes.
- **RFC-compliant by default:** The library still rejects non-compliant
  tokens unless the caller explicitly opts in to leniency.

**Cons:**
- Requires threading the handler from `Calendar` through
  `ParseComponent`/`GeneralParseComponent` (internal refactor with new
  unexported helpers).
- Public `ParseComponent` / `GeneralParseComponent` signatures unchanged but
  internally delegate to handler-aware variants — minor complexity.

**Verdict:** Best balance of generality, upstream acceptability, and
maintenance burden. This is the approach we're implementing.

## Handler Signature Sub-Options (within Option 3)

We considered three sub-options for what data the handler receives:

### Sub-option A: Partial parse state + remaining content line

```go
func(prop *BaseProperty, remainder string, err error) (*BaseProperty, error)
```

Provides the partially-parsed property (IANAToken populated, params
partially filled) plus the unparsed remainder. Enables sophisticated
recovery (e.g. re-parsing with relaxed rules).

**Rejected:** Too complex for a first upstream PR. Exposes internal parse
state that may change between versions. The partial state is unreliable
(which params parsed before failure?).

### Sub-option B: Raw content line + error ✅ (chosen)

```go
func(rawLine ContentLine, err error) (*BaseProperty, error)
```

Handler gets the complete (unfolded) content line and the error. Recovery
requires the handler to do its own parsing if needed.

**Chosen because:**
- Simplest signature — easy to understand and document.
- Sufficient for all known use cases (skip malformed properties, or
  re-parse with custom logic).
- Stable API: raw line content doesn't depend on internal parse state.
- Matches `ContentLine` type already exported by the library.
- Most likely to be accepted upstream — minimal new concepts.

### Sub-option C: Hybrid (raw line + partial state + error)

```go
func(rawLine ContentLine, partial *BaseProperty, err error) (*BaseProperty, error)
```

Best of both worlds but:
- More complex signature for a feature most callers will use as
  "skip on error".
- Partial state reliability concerns from sub-option A still apply.
- Can always be added later as a `WithPropertyParseErrorHandlerV2` if
  demand exists.

**Deferred:** Not worth the complexity for initial PR.

## Implementation Details

### Library changes (fork: `RKelln/golang-ical`)

**`calendar.go`:**
- Add `propertyParseErrorHandler` field to `Calendar` struct.
- Add `PropertyParseErrorHandler` type alias (exported, for godoc).
- Add `WithPropertyParseErrorHandler` constructor (mirrors
  `WithUnknownPropertyHandler`).
- In `ParseCalendarWithOptions`: when `ParseProperty` returns error and
  handler is set, invoke handler. On `(nil, nil)` skip; on
  `(*prop, nil)` use recovered prop; on `(_, err)` abort.
- Replace `GeneralParseComponent(cs, line)` call with internal
  `generalParseComponentWithHandler(cs, line, c.propertyParseErrorHandler)`.

**`components.go`:**
- Add `generalParseComponentWithHandler(cs, startLine, handler)` — same
  logic as `GeneralParseComponent` but threads handler into component
  construction.
- Add `parseComponentWithHandler(cs, startLine, handler)` — same logic
  as `ParseComponent` but invokes handler on `ParseProperty` errors.
- `GeneralParseComponent` and `ParseComponent` become thin wrappers
  delegating to the handler-aware variants with `nil` handler.
- Nested `BEGIN:` blocks inside components also use
  `generalParseComponentWithHandler` to propagate the handler through
  sub-components (e.g. `VALARM` inside `VEVENT`).

### Togather-side integration

**`internal/ical/parse.go`:**

```go
cal, err := ics.ParseCalendarWithOptions(bytes.NewReader(data),
    ics.WithUnknownPropertyHandler(ics.AcceptUnknownPropertyHandler),
    ics.WithPropertyParseErrorHandler(func(rawLine ics.ContentLine, err error) (*ics.BaseProperty, error) {
        // Skip malformed properties with a warning.
        // The property value is almost certainly a presentation hint
        // (X-TKF-*, etc.) that we don't need for event data.
        result.Warnings = append(result.Warnings, fmt.Sprintf(
            "skipped malformed property: %s", err))
        return nil, nil // skip
    }),
)
```

### `go.mod` change

Temporarily point to the fork until upstream merges:

```
replace github.com/arran4/golang-ical => github.com/RKelln/golang-ical v0.3.6-fork.1
```

Once upstream merges our PR and tags a release, we remove the `replace`
directive and update to the new upstream version.

## Upstream Maintainer Context

- **arran4** is active (merging PRs as of April 2026, including bot PRs).
- Issues #71, #73, #109 all relate to error handling but focus on sentinel
  errors for `errors.Is()` support — none add skip-and-continue behavior.
- The `ParseOption` functional options pattern is established and the
  maintainer seems receptive to it (`WithUnknownPropertyHandler` was their
  design).
- Our PR adds capability without changing defaults — low risk of breaking
  existing users.

## Status

- [x] Design finalized (Option 3, Sub-option B)
- [x] Fork created (`RKelln/golang-ical`)
- [x] Feature branch created (`feat/property-parse-error-handler`)
- [x] Core implementation done (calendar.go + components.go changes)
- [x] Existing library tests pass
- [x] New tests for the handler feature (6 tests)
- [x] togather go.mod pointed to fork
- [x] togather integration in `internal/ical/parse.go`
- [x] togather tests for malformed ICS handling
- [x] `make ci` passes (build, lint, tests)
- [x] Upstream PR opened to `arran4/golang-ical` (https://github.com/arran4/golang-ical/pull/135)
- [ ] Bead `srv-7bi1l` closed
