#!/usr/bin/env python3
"""
Fix additional dashboard panel configurations:
1. Request Latency: Change legend display mode to show all metrics
2. Server Status & Version: Add slot-based color overrides
3. Add Active Slot indicator panel
"""

import json
import sys


def fix_request_latency_legend(panel):
    """Fix Request Latency panel to show all 6 metrics in legend."""
    if panel.get("title") != "Request Latency (p50, p95, p99)":
        return panel

    if "options" not in panel:
        panel["options"] = {}
    if "legend" not in panel["options"]:
        panel["options"]["legend"] = {}

    # Change display mode to "list" to show all series without truncation
    panel["options"]["legend"]["displayMode"] = "list"
    panel["options"]["legend"]["showLegend"] = True
    panel["options"]["legend"]["calcs"] = ["mean", "max"]
    panel["options"]["legend"]["placement"] = "bottom"

    return panel


def add_stat_panel_overrides(panel):
    """Add slot-based color overrides to stat panels (Server Status, Version)."""
    if panel.get("type") != "stat":
        return panel

    if panel.get("title") not in ["Server Status", "Version"]:
        return panel

    # Initialize fieldConfig.overrides if it doesn't exist
    if "fieldConfig" not in panel:
        panel["fieldConfig"] = {}
    if "overrides" not in panel["fieldConfig"]:
        panel["fieldConfig"]["overrides"] = []

    # Add overrides for blue and green slot text colors
    panel["fieldConfig"]["overrides"].extend(
        [
            {
                "matcher": {"id": "byRegexp", "options": ".*blue.*"},
                "properties": [
                    {"id": "color", "value": {"fixedColor": "#1f77b4", "mode": "fixed"}}
                ],
            },
            {
                "matcher": {"id": "byRegexp", "options": ".*green.*"},
                "properties": [
                    {"id": "color", "value": {"fixedColor": "#2ca02c", "mode": "fixed"}}
                ],
            },
        ]
    )

    return panel


def add_active_slot_panel(dashboard):
    """Add a new panel to show which slot is currently active."""
    # Find max panel ID
    max_id = max([p.get("id", 0) for p in dashboard["panels"]])

    active_slot_panel = {
        "id": max_id + 1,
        "title": "Active Slot",
        "type": "stat",
        "gridPos": {"h": 3, "w": 4, "x": 20, "y": 0},
        "targets": [
            {
                "expr": 'togather_app_info{slot=~"$slot", active_slot="true"}',
                "legendFormat": "{{slot}}",
                "refId": "A",
            }
        ],
        "options": {
            "colorMode": "background",
            "graphMode": "none",
            "justifyMode": "center",
            "orientation": "auto",
            "reduceOptions": {"values": False, "calcs": ["lastNotNull"], "fields": ""},
            "textMode": "name",
            "text": {"titleSize": 12, "valueSize": 18},
        },
        "fieldConfig": {
            "defaults": {
                "thresholds": {
                    "mode": "absolute",
                    "steps": [{"color": "green", "value": None}],
                },
                "mappings": [],
            },
            "overrides": [
                {
                    "matcher": {"id": "byRegexp", "options": ".*blue.*"},
                    "properties": [
                        {
                            "id": "color",
                            "value": {"fixedColor": "#1f77b4", "mode": "fixed"},
                        }
                    ],
                },
                {
                    "matcher": {"id": "byRegexp", "options": ".*green.*"},
                    "properties": [
                        {
                            "id": "color",
                            "value": {"fixedColor": "#2ca02c", "mode": "fixed"},
                        }
                    ],
                },
            ],
        },
        "datasource": {"type": "prometheus", "uid": "prometheus"},
    }

    dashboard["panels"].append(active_slot_panel)
    return dashboard


def main():
    if len(sys.argv) != 2:
        print("Usage: fix-dashboard-panels.py <dashboard.json>")
        sys.exit(1)

    filepath = sys.argv[1]

    print(f"Fixing dashboard panels: {filepath}")
    print()

    # Read dashboard
    with open(filepath, "r") as f:
        dashboard = json.load(f)

    changes = []

    # Fix Request Latency legend
    for panel in dashboard["panels"]:
        if panel.get("title") == "Request Latency (p50, p95, p99)":
            fix_request_latency_legend(panel)
            changes.append(
                "✓ Request Latency: Changed legend to list mode (shows all 6 metrics)"
            )

    # Add stat panel overrides
    for panel in dashboard["panels"]:
        if panel.get("type") == "stat" and panel.get("title") in [
            "Server Status",
            "Version",
        ]:
            add_stat_panel_overrides(panel)
            changes.append(f"✓ {panel.get('title')}: Added slot-based color overrides")

    # Add Active Slot panel
    existing_titles = [p.get("title") for p in dashboard["panels"]]
    if "Active Slot" not in existing_titles:
        add_active_slot_panel(dashboard)
        changes.append("✓ Added 'Active Slot' indicator panel")

    # Write back
    with open(filepath, "w") as f:
        json.dump(dashboard, f, indent=2)

    print("\n".join(changes))
    print()
    print(f"✓ Dashboard updated successfully")


if __name__ == "__main__":
    main()
