# Grafana Dashboard Design Guidelines

## Color Scheme for Blue/Green Deployment Monitoring

### Core Principle
**Use both COLOR and LINE STYLE to distinguish deployment slots**

This provides two visual cues, making it easier to distinguish metrics at a glance, especially for colorblind users or when printed.

---

## Active Slot Indicator

The dashboard includes an "Active Slot" panel that shows which server is currently receiving production traffic.

**Implementation:**
- Uses `ACTIVE_SLOT` environment variable set in docker-compose
- Exposed via `togather_app_info{active_slot="true"}` metric
- Panel query: `togather_app_info{slot=~"$slot", active_slot="true"}`
- Shows slot name with color-coded background (blue/green)

**Configuration:**
- Set `BLUE_ACTIVE_SLOT=true` for active blue slot
- Set `GREEN_ACTIVE_SLOT=false` for inactive green slot
- Switch values when performing blue-green deployments

---

## Visual Scheme

### Primary Metrics (Most Important)
- **Line Style**: Solid
- **Colors**: 
  - **Blue slot**: Dark blue `#1f77b4`
  - **Green slot**: Dark green `#2ca02c`

### Secondary Metrics
- **Line Style**: Dashed `[10, 5]` (10px dash, 5px gap)
- **Colors**: 
  - **Blue slot**: Medium blue `#5da5da`
  - **Green slot**: Medium green `#6ec16e`

### Tertiary Metrics  
- **Line Style**: Dotted `[2, 4]` (2px dot, 4px gap)
- **Colors**: 
  - **Blue slot**: Light blue `#aec7e8`
  - **Green slot**: Light green `#b1d8b1`

---

## Application Guidelines

### Single Metric Per Slot
**Examples**: Request Rate, Goroutines

**Pattern**:
- Blue slot: Dark blue, solid line
- Green slot: Dark green, solid line

```json
{
  "matcher": {"id": "byRegexp", "options": ".*blue.*"},
  "properties": [
    {"id": "color", "value": {"fixedColor": "#1f77b4", "mode": "fixed"}},
    {"id": "custom.lineStyle", "value": {"fill": "solid"}},
    {"id": "custom.lineWidth", "value": 2}
  ]
}
```

---

### Multiple Metrics Per Slot
**Examples**: Database Connections (Total/In Use/Idle), Request Latency (p50/p95/p99)

**Pattern**:
1. **First metric** (e.g., Total, p50): Dark color, **solid** line
2. **Second metric** (e.g., In Use, p95): Medium color, **dashed** line
3. **Third metric** (e.g., Idle, p99): Light color, **dotted** line

This applies to **both blue and green slots** - the line style differentiation is based on metric priority, not slot color.

**Legend Format**: Always use `{{slot}} - MetricName` for clarity

---

## Field Override Examples

### Example 1: Database Connections (3 metrics × 2 slots)

```json
{
  "fieldConfig": {
    "overrides": [
      // Blue - Total (dark blue, solid)
      {
        "matcher": {"id": "byRegexp", "options": "blue.*Total"},
        "properties": [
          {"id": "color", "value": {"fixedColor": "#1f77b4", "mode": "fixed"}},
          {"id": "custom.lineStyle", "value": {"fill": "solid"}},
          {"id": "custom.lineWidth", "value": 2}
        ]
      },
      // Blue - In Use (medium blue, dashed)
      {
        "matcher": {"id": "byRegexp", "options": "blue.*In Use"},
        "properties": [
          {"id": "color", "value": {"fixedColor": "#5da5da", "mode": "fixed"}},
          {"id": "custom.lineStyle", "value": {"fill": "dash", "dash": [10, 5]}},
          {"id": "custom.lineWidth", "value": 2}
        ]
      },
      // Blue - Idle (light blue, dotted)
      {
        "matcher": {"id": "byRegexp", "options": "blue.*Idle"},
        "properties": [
          {"id": "color", "value": {"fixedColor": "#aec7e8", "mode": "fixed"}},
          {"id": "custom.lineStyle", "value": {"fill": "dash", "dash": [2, 4]}},
          {"id": "custom.lineWidth", "value": 2}
        ]
      },
      // Green - Total (dark green, solid)
      {
        "matcher": {"id": "byRegexp", "options": "green.*Total"},
        "properties": [
          {"id": "color", "value": {"fixedColor": "#2ca02c", "mode": "fixed"}},
          {"id": "custom.lineStyle", "value": {"fill": "solid"}},
          {"id": "custom.lineWidth", "value": 2}
        ]
      },
      // Green - In Use (medium green, dashed)
      {
        "matcher": {"id": "byRegexp", "options": "green.*In Use"},
        "properties": [
          {"id": "color", "value": {"fixedColor": "#6ec16e", "mode": "fixed"}},
          {"id": "custom.lineStyle", "value": {"fill": "dash", "dash": [10, 5]}},
          {"id": "custom.lineWidth", "value": 2}
        ]
      },
      // Green - Idle (light green, dotted)
      {
        "matcher": {"id": "byRegexp", "options": "green.*Idle"},
        "properties": [
          {"id": "color", "value": {"fixedColor": "#b1d8b1", "mode": "fixed"}},
          {"id": "custom.lineStyle", "value": {"fill": "dash", "dash": [2, 4]}},
          {"id": "custom.lineWidth", "value": 2}
        ]
      }
    ]
  }
}
```

---

## Automation Script

Use `deploy/scripts/update-dashboard-colors.py` to apply these patterns automatically.

### Usage
```bash
# Apply color scheme to existing dashboard
python3 deploy/scripts/update-dashboard-colors.py deploy/config/grafana/dashboards/json/togather-overview.json

# The script will:
# - Detect single vs. multiple metric panels
# - Apply appropriate color shades
# - Add line style differentiation
# - Preserve existing configurations
```

---

## Panel-Specific Patterns

### Request Rate (1 metric per slot)
- Blue: Dark blue, solid
- Green: Dark green, solid

### Database Connections (3 metrics per slot)
- Blue Total: Dark blue, solid
- Blue In Use: Medium blue, dashed
- Blue Idle: Light blue, dotted
- Green Total: Dark green, solid
- Green In Use: Medium green, dashed
- Green Idle: Light green, dotted

### Request Latency (3 metrics per slot)
- Blue p50: Dark blue, solid (most important)
- Blue p95: Medium blue, dashed
- Blue p99: Light blue, dotted
- Green p50: Dark green, solid
- Green p95: Medium green, dashed
- Green p99: Light green, dotted

### Error Rate (2 metrics per slot)
- Blue 5xx: Dark blue, solid (more critical)
- Blue 4xx: Medium blue, dashed
- Green 5xx: Dark green, solid
- Green 4xx: Medium green, dashed

### Memory Usage (2 metrics per slot)
- Blue Heap: Dark blue, solid
- Blue System: Medium blue, dashed
- Green Heap: Dark green, solid
- Green System: Medium green, dashed

### Goroutines (1 metric per slot)
- Blue: Dark blue, solid
- Green: Dark green, solid

---

## Accessibility Considerations

### Color Blindness
The triple-coding (color + brightness + line style) ensures the dashboard works for:
- **Deuteranopia** (red-green colorblind): Can distinguish blue/green colors, brightness levels, and line styles (solid/dashed/dotted)
- **Protanopia** (red-green colorblind): Can distinguish blue/green colors, brightness levels, and line styles
- **Tritanopia** (blue-yellow colorblind): Can distinguish by line style and brightness levels

### Printing
- Black & white printing: Line styles (solid/dashed/dotted) remain clearly visible
- Grayscale: Different shades of blue/green convert to different grays, plus line style differentiation

---

## Best Practices

### DO:
✓ Use `{{slot}} - MetricName` legend format for clarity
✓ Apply darker shades to more important metrics
✓ Use solid lines for primary, dashed for secondary, dotted for tertiary
✓ Keep line width at 2px for readability
✓ Test dashboard with both blue and green selected in $slot variable

### DON'T:
✗ Use more than 3 metrics per slot (becomes hard to distinguish)
✗ Use the same color for different metrics in the same slot
✗ Forget to update legend format when adding new metrics
✗ Mix up the line style order (always solid → dashed → dotted)

---

## Testing Your Dashboard

Before committing:

1. **Load test both slots**: Generate metrics for blue and green
2. **Select "All" in $slot dropdown**: Verify all lines are distinguishable
3. **Check legend**: Ensure names are clear (e.g., "blue - p95")
4. **Test time ranges**: Verify lines don't overlap confusingly
5. **Print preview**: Check grayscale rendering
6. **Color blind simulation**: Use browser dev tools or extension

---

## Quick Reference

| Priority  | Line Style | Blue      | Green     |
|-----------|-----------|-----------|-----------|
| Primary   | Solid     | #1f77b4   | #2ca02c   |
| Secondary | Dashed    | #5da5da   | #6ec16e   |
| Tertiary  | Dotted    | #aec7e8   | #b1d8b1   |

**Dashed Line Pattern**: `[10, 5]` (10px dash, 5px gap)  
**Dotted Line Pattern**: `[2, 4]` (2px dot, 4px gap)  
**Line Width**: `2px`
