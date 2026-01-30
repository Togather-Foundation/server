#!/usr/bin/env python3
"""
Update Grafana dashboard to use blue lines for blue slot and green lines for green slot.
"""

import json
import sys


def add_color_overrides(panel):
    """Add field overrides for blue/green coloring based on slot label."""
    if panel.get("type") not in ["timeseries", "graph"]:
        return panel

    # Initialize fieldConfig.overrides if it doesn't exist
    if "fieldConfig" not in panel:
        panel["fieldConfig"] = {}
    if "overrides" not in panel["fieldConfig"]:
        panel["fieldConfig"]["overrides"] = []

    # Remove existing slot-based color overrides to avoid duplicates
    panel["fieldConfig"]["overrides"] = [
        o
        for o in panel["fieldConfig"]["overrides"]
        if not (
            o.get("matcher", {}).get("id") == "byRegexp"
            and "slot" in o.get("matcher", {}).get("options", "")
        )
    ]

    # Add blue override for blue slot
    panel["fieldConfig"]["overrides"].append(
        {
            "matcher": {"id": "byRegexp", "options": ".*blue.*"},
            "properties": [
                {"id": "color", "value": {"fixedColor": "blue", "mode": "fixed"}}
            ],
        }
    )

    # Add green override for green slot
    panel["fieldConfig"]["overrides"].append(
        {
            "matcher": {"id": "byRegexp", "options": ".*green.*"},
            "properties": [
                {"id": "color", "value": {"fixedColor": "green", "mode": "fixed"}}
            ],
        }
    )

    return panel


def main():
    if len(sys.argv) != 2:
        print("Usage: update-dashboard-colors.py <dashboard.json>")
        sys.exit(1)

    filepath = sys.argv[1]

    # Read dashboard
    with open(filepath, "r") as f:
        dashboard = json.load(f)

    # Update all timeseries panels
    updated_count = 0
    for panel in dashboard.get("panels", []):
        if panel.get("type") in ["timeseries", "graph"]:
            add_color_overrides(panel)
            updated_count += 1

    # Write back
    with open(filepath, "w") as f:
        json.dump(dashboard, f, indent=2)

    print(f"✓ Updated {updated_count} panels with blue/green color overrides")


if __name__ == "__main__":
    main()
