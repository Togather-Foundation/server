#!/usr/bin/env python3
"""
Update Grafana dashboard to use dual visual cues for blue/green deployment slots.
- Blue slot: Solid lines in shades of blue (dark → medium → light)
- Green slot: Dashed lines in shades of green (dark → medium → light)
"""

import json
import sys

# Color palette (from grafana-dashboard-guidelines.md)
BLUE_COLORS = {
    "dark": "#1f77b4",  # Primary metric
    "medium": "#5da5da",  # Secondary metric
    "light": "#aec7e8",  # Tertiary metric
}

GREEN_COLORS = {
    "dark": "#2ca02c",  # Primary metric
    "medium": "#6ec16e",  # Secondary metric
    "light": "#b1d8b1",  # Tertiary metric
}

# Panel-specific metric patterns (priority order: most important first)
PANEL_PATTERNS = {
    "Database Connections": [
        {"pattern": "Total", "priority": 0},
        {"pattern": "In Use", "priority": 1},
        {"pattern": "Idle", "priority": 2},
    ],
    "Request Latency (p50, p95, p99)": [
        {"pattern": "p50", "priority": 0},
        {"pattern": "p95", "priority": 1},
        {"pattern": "p99", "priority": 2},
    ],
    "Error Rate (4xx / 5xx)": [
        {"pattern": "5xx", "priority": 0},  # More critical
        {"pattern": "4xx", "priority": 1},
    ],
    "Go Memory Usage": [
        {"pattern": "Heap", "priority": 0},
        {"pattern": "System", "priority": 1},
    ],
    "Request Rate": [],  # Single metric per slot
    "Goroutines": [],  # Single metric per slot
}


def get_color_by_priority(slot, priority):
    """Return color hex code based on slot and priority level."""
    colors = BLUE_COLORS if slot == "blue" else GREEN_COLORS
    if priority == 0:
        return colors["dark"]
    elif priority == 1:
        return colors["medium"]
    else:
        return colors["light"]


def create_override(slot, pattern, priority):
    """Create a field override for a specific slot and metric pattern."""
    # Use more lenient regex to match legend text with spaces and extra words
    # e.g., "blue - Heap Alloc" matches "blue.*Heap.*"
    regex = f"{slot}.*{pattern}.*" if pattern else f".*{slot}.*"
    color = get_color_by_priority(slot, priority)

    # Base properties: color and line width
    properties = [
        {"id": "color", "value": {"fixedColor": color, "mode": "fixed"}},
        {"id": "custom.lineWidth", "value": 2},
    ]

    # Line style based on priority:
    # Priority 0 (primary): Solid
    # Priority 1 (secondary): Dashed
    # Priority 2+ (tertiary): Dotted
    if priority == 0:
        properties.append({"id": "custom.lineStyle", "value": {"fill": "solid"}})
    elif priority == 1:
        properties.append(
            {
                "id": "custom.lineStyle",
                "value": {"fill": "dash", "dash": [10, 5]},  # Dashed pattern
            }
        )
    else:  # priority >= 2
        properties.append(
            {
                "id": "custom.lineStyle",
                "value": {"fill": "dash", "dash": [2, 4]},  # Dotted pattern
            }
        )

    return {
        "matcher": {"id": "byRegexp", "options": regex},
        "properties": properties,
    }


def add_color_overrides(panel):
    """Add field overrides for blue/green coloring with line style differentiation."""
    if panel.get("type") not in ["timeseries", "graph"]:
        return panel

    panel_title = panel.get("title", "")

    # Initialize fieldConfig.overrides if it doesn't exist
    if "fieldConfig" not in panel:
        panel["fieldConfig"] = {}
    if "overrides" not in panel["fieldConfig"]:
        panel["fieldConfig"]["overrides"] = []

    # Remove existing slot-based overrides to avoid duplicates
    panel["fieldConfig"]["overrides"] = [
        o
        for o in panel["fieldConfig"]["overrides"]
        if not (
            o.get("matcher", {}).get("id") == "byRegexp"
            and any(
                slot in o.get("matcher", {}).get("options", "")
                for slot in ["blue", "green", "slot"]
            )
        )
    ]

    # Get patterns for this panel
    patterns = PANEL_PATTERNS.get(panel_title)

    if patterns is None:
        # Unknown panel: skip (don't modify)
        print(f"  ⚠ Skipping unknown panel: {panel_title}")
        return panel

    if len(patterns) == 0:
        # Single metric per slot: use simple overrides
        panel["fieldConfig"]["overrides"].extend(
            [
                create_override("blue", "", 0),
                create_override("green", "", 0),
            ]
        )
    else:
        # Multiple metrics per slot: apply pattern-based overrides
        for slot in ["blue", "green"]:
            for metric in patterns:
                override = create_override(slot, metric["pattern"], metric["priority"])
                panel["fieldConfig"]["overrides"].append(override)

    return panel


def main():
    if len(sys.argv) != 2:
        print("Usage: update-dashboard-colors.py <dashboard.json>")
        sys.exit(1)

    filepath = sys.argv[1]

    print(f"Processing dashboard: {filepath}")
    print()

    # Read dashboard
    with open(filepath, "r") as f:
        dashboard = json.load(f)

    # Update all timeseries panels
    updated_count = 0
    skipped_count = 0

    for panel in dashboard.get("panels", []):
        if panel.get("type") in ["timeseries", "graph"]:
            panel_title = panel.get("title", "Untitled")
            before_count = len(panel.get("fieldConfig", {}).get("overrides", []))

            result = add_color_overrides(panel)

            if result == panel and panel_title not in PANEL_PATTERNS:
                skipped_count += 1
                continue

            after_count = len(panel.get("fieldConfig", {}).get("overrides", []))
            overrides_added = after_count - before_count

            print(f"✓ {panel_title}: Added {overrides_added} overrides")
            updated_count += 1

    # Write back
    with open(filepath, "w") as f:
        json.dump(dashboard, f, indent=2)

    print()
    print(f"✓ Successfully updated {updated_count} timeseries panels")
    if skipped_count > 0:
        print(f"  (Skipped {skipped_count} unknown panels)")
    print()
    print("Visual scheme applied:")
    print("  • Primary metrics: Dark colors, solid lines")
    print("  • Secondary metrics: Medium colors, dashed lines")
    print("  • Tertiary metrics: Light colors, dotted lines")
    print("  • Applies consistently to both blue and green slots")


if __name__ == "__main__":
    main()
