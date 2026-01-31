# Grafana Dashboard Design Guidelines

## Color Scheme for Blue/Green Deployment Monitoring

### Core Principle
**Use both COLOR and LINE STYLE to distinguish deployment slots**

This provides two visual cues, making it easier to distinguish metrics at a glance, especially for colorblind users or when printed.

---

## Visual Scheme

### Blue Slot (Primary)
- **Line Style**: Solid (`lineStyle: "solid"`)
- **Colors**: Shades of blue from dark to light
  - **Dark Blue**: `#1f77b4` (primary metric)
  - **Medium Blue**: `#5da5da` (secondary metric)
  - **Light Blue**: `#aec7e8` (tertiary metric)

### Green Slot (Secondary)
- **Line Style**: Dashed (`lineStyle: "dash"`, `lineInterpolation: "linear"`)
- **Colors**: Shades of green from dark to light
  - **Dark Green**: `#2ca02c` (primary metric)
  - **Medium Green**: `#6ec16e` (secondary metric)
  - **Light Green**: `#b1d8b1` (tertiary metric)

---

## Application Guidelines

### Single Metric Per Slot
**Examples**: Request Rate, Goroutines

**Pattern**:
- Blue slot: Dark blue, solid line
- Green slot: Dark green, dashed line

```json
{
  "matcher": {"id": "byRegexp", "options": ".*blue.*"},
  "properties": [
    {"id": "color", "value": {"fixedColor": "#1f77b4", "mode": "fixed"}},
    {"id": "custom.lineStyle", "value": {"fill": "solid"}}
  ]
}
```

---

### Multiple Metrics Per Slot
**Examples**: Database Connections (Total/In Use/Idle), Request Latency (p50/p95/p99)

**Pattern** (Blue slot):
1. First metric (e.g., Total, p50): Dark blue (#1f77b4), solid
2. Second metric (e.g., In Use, p95): Medium blue (#5da5da), solid
3. Third metric (e.g., Idle, p99): Light blue (#aec7e8), solid

**Pattern** (Green slot):
1. First metric: Dark green (#2ca02c), dashed
2. Second metric: Medium green (#6ec16e), dashed
3. Third metric: Light green (#b1d8b1), dashed

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
      // Blue - In Use (medium blue, solid)
      {
        "matcher": {"id": "byRegexp", "options": "blue.*In Use"},
        "properties": [
          {"id": "color", "value": {"fixedColor": "#5da5da", "mode": "fixed"}},
          {"id": "custom.lineStyle", "value": {"fill": "solid"}},
          {"id": "custom.lineWidth", "value": 2}
        ]
      },
      // Blue - Idle (light blue, solid)
      {
        "matcher": {"id": "byRegexp", "options": "blue.*Idle"},
        "properties": [
          {"id": "color", "value": {"fixedColor": "#aec7e8", "mode": "fixed"}},
          {"id": "custom.lineStyle", "value": {"fill": "solid"}},
          {"id": "custom.lineWidth", "value": 2}
        ]
      },
      // Green - Total (dark green, dashed)
      {
        "matcher": {"id": "byRegexp", "options": "green.*Total"},
        "properties": [
          {"id": "color", "value": {"fixedColor": "#2ca02c", "mode": "fixed"}},
          {"id": "custom.lineStyle", "value": {"fill": "dash", "dash": [10, 5]}},
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
      // Green - Idle (light green, dashed)
      {
        "matcher": {"id": "byRegexp", "options": "green.*Idle"},
        "properties": [
          {"id": "color", "value": {"fixedColor": "#b1d8b1", "mode": "fixed"}},
          {"id": "custom.lineStyle", "value": {"fill": "dash", "dash": [10, 5]}},
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
- Green: Dark green, dashed

### Database Connections (3 metrics per slot)
- Blue Total: Dark blue, solid
- Blue In Use: Medium blue, solid  
- Blue Idle: Light blue, solid
- Green Total: Dark green, dashed
- Green In Use: Medium green, dashed
- Green Idle: Light green, dashed

### Request Latency (3 metrics per slot)
- Blue p50: Dark blue, solid (most important)
- Blue p95: Medium blue, solid
- Blue p99: Light blue, solid
- Green p50: Dark green, dashed
- Green p95: Medium green, dashed
- Green p99: Light green, dashed

### Error Rate (2 metrics per slot)
- Blue 5xx: Dark blue, solid (more critical)
- Blue 4xx: Medium blue, solid
- Green 5xx: Dark green, dashed
- Green 4xx: Medium green, dashed

### Memory Usage (2 metrics per slot)
- Blue Heap: Dark blue, solid
- Blue System: Medium blue, solid
- Green Heap: Dark green, dashed
- Green System: Medium green, dashed

### Goroutines (1 metric per slot)
- Blue: Dark blue, solid
- Green: Dark green, dashed

---

## Accessibility Considerations

### Color Blindness
The dual-coding (color + line style) ensures the dashboard works for:
- **Deuteranopia** (red-green colorblind): Can distinguish by line style
- **Protanopia** (red-green colorblind): Can distinguish by line style
- **Tritanopia** (blue-yellow colorblind): Can distinguish blue/green hues + line style

### Printing
- Black & white printing: Line styles (solid vs. dashed) remain visible
- Grayscale: Different shades of blue/green convert to different grays

---

## Best Practices

### DO:
✓ Use `{{slot}} - MetricName` legend format for clarity
✓ Apply darker shades to more important metrics
✓ Use solid lines for blue, dashed for green
✓ Keep line width at 2px for readability
✓ Test dashboard with both blue and green selected in $slot variable

### DON'T:
✗ Use more than 3 shades per slot (becomes hard to distinguish)
✗ Use the same color for different metrics in the same slot
✗ Forget to update legend format when adding new metrics
✗ Use dotted lines (dashed is more visible)

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

| Slot  | Line Style | Dark      | Medium    | Light     |
|-------|-----------|-----------|-----------|-----------|
| Blue  | Solid     | #1f77b4   | #5da5da   | #aec7e8   |
| Green | Dashed    | #2ca02c   | #6ec16e   | #b1d8b1   |

**Line Dash Pattern**: `[10, 5]` (10px dash, 5px gap)
**Line Width**: `2px`
