# Unified Device Health Metrics Design

## Context
Fleet Console provides an actionable view of device health across Android, ChromeOS, and Browser platforms by reconciling inventory intent (UFS) with operational reality (Swarming).

## UX & Gestalt Principles
1. **Proximity**: Card layouts group related metrics together (e.g., Unhealthy states in one card, Recovering states in a sub-list).
2. **Similarity**: Standard colors indicate state: Green (Healthy), Red (Unhealthy), Yellow (Other), Grey (Recovering).
3. **Mutually Exclusive Buckets**: Grouping states into Healthy, Unhealthy, and Other columns ensures card sums equal 100% of physical devices.

## Platform Implementations

### Android
- **Layout**: Uses rows for **Hosts** and **Devices** to separate infrastructure issues from device issues.
- **State Accounting**: Categorizes devices into **Online** (`fc_is_offline = false`) and **Offline** (`fc_is_offline = true`).
  - **Online**: `IDLE`, `BUSY`, and `LAMEDUCK`.
  - **Offline**: `Missing`, `Failed`, `Dying`, `Init`, `Dirty`, `Prepping`.
- **Scope Switching**: Clears `state` filters when toggling between Host and Device views, and applies `fc_is_offline` filters on card clicks.

### ChromeOS
- **Layout**: Extends grid layout to **Labstations** in the top row.
- **Recovering States**: Groups `Need repair` and `Repair failed` under "Recovering" when automation handles repair workflows.

### Browser
- **Layout**: Uses a left column for **Total Devices**, with stacked rows for **Bots** and **Devices without Bots**.
- **Missing Bots**: Classifies devices marked `SERVING` in UFS but lacking Swarming registrations as "Missing".
