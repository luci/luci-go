# Unified Device Health Metrics Design Decisions

## Context
The Fleet Console provides a unified, actionable view of device health metrics across Android, ChromeOS, and Browser platforms. By reconciling administrative intent (UFS) with operational reality (Swarming), it ensures that health metrics are exhaustive and that 100% of the fleet capacity is accounted for in the dashboard.

## UX Principles & Gestalt Theory
The design applies Gestalt principles of visual perception to make dense metrics data scannable and intuitive:
1. **Principle of Proximity**: Related metrics are grouped together in cards (e.g., all Unhealthy states in one card, all Recovering states in a sub-list). This helps users perceive them as a single functional unit and quickly locate what they need.
2. **Principle of Similarity**: Consistent color coding (emerald for healthy, rose for unhealthy, amber for other, grey for recovering) creates a visual language that allows users to assess fleet health at a glance without reading every label.
3. **Mutually Exclusive Bucketing**: States are grouped into Healthy, Unhealthy, and Other columns to ensure that the sum of parts equals the total (100% sum). This avoids confusing users with overlapping counts (e.g., when a bot is both Alive and Quarantined in Swarming).

## Platform-Specific Implementations & Plans

### Android
- **Layout**: Uses rows for **Hosts** and **Devices** to separate infrastructure issues from instance issues.
- **State Accounting**: Includes states like `Init`, `Dirty`, `Prepping`, and `Lameduck` under a **Recovering** group within the Healthy column, as they are expected to recover automatically.
- **Unclassified**: A remainder bucket to ensure 100% sum.
- **Scope-Switching Logic**: Automatically clears `state` filters when switching between Host and Device scopes (e.g., clicking "Total Hosts" clears device states) to avoid empty results from mutually exclusive states.

### ChromeOS (Plan)
- **Layout**: Plan to extend the grid layout to **Labstations** in the top row, accounting for all states (`Need Repair`, `Repair Failed`, etc.).
- **Recovering States**: States like `Need repair` and `Repair failed` can be grouped as "Recovering" or "Transient" if automation handles them.

### Browser (Plan)
- **Layout**: Plan to use a dedicated left column for **Total Devices** to ground the layout, with stacked rows for **Bots** and **Devices without Bots** on the right.
- **Missing Definition**: Addresses the "Missing Bot data" challenge by reconciling UFS and Swarming states (Devices marked `SERVING` in UFS but lacking bot data in Swarming are classified as "Missing").

## Implementation Details & Future Work
- **API Limitations**: Current API support for negative filters on counts is limited, which makes exact counts for "Missing" bots hard to fetch in a single call for Browser.
- **Parser Support**: Future work includes improving the URL parser to support complex or blank filter queries without triggering invalid state warnings.
