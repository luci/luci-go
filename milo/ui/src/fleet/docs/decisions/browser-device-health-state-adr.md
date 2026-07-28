# Browser Device Health Accounting

## Context
Browser device health metrics reconcile two data layers:
1. **Inventory Intent (`UFS`)**: Expected hardware role (`SERVING`, `NEEDS_REPAIR`, `MISSING`, `EXCLUDED`).
2. **Operational Reality (`Swarming`)**: Active daemon status (`alive`, `dead`, `quarantined`, `maintenance`).

This document explains our two-tier accounting model and the plan to move health classification into SQL.

## State Model Tradeoffs

### 1. Mutually Exclusive UFS vs Multi-Attribute Swarming
- **UFS**: Resource states are strictly **mutually exclusive**. Every device has exactly one `resource_state`. UFS states total 100% of physical devices.
- **Swarming**: Bot states use non-exclusive boolean flags (`alive`, `dead`, `quarantined`, `maintenance`). A single bot can show multiple flags (e.g., `Alive + Quarantined` or `Dead + Quarantined`).

### 2. Limitations of `CountBrowserDevices` API
The `CountBrowserDevices` RPC returns marginal counts for each Swarming flag. Because multi-flag bots appear in multiple marginal counts:
1. **Non-Additive Totals**: Summing marginal counts (`Dead + Quarantined + Maintenance`) can exceed the physical device count.
2. **Overstated Capacity**: Raw `Alive` counts include `Alive + Quarantined` bots that cannot run test workloads.

## Two-Tier Client Accounting

The UI uses a **Two-Tier Accounting Strategy** in `browser_summary_header.tsx`:

```
+-------------------------------------------------------------------------------+
|                      TOP-LEVEL HEALTH SUMMARY CARDS                           |
|             (100% Mutually Exclusive Accounts via Clamped Buckets)            |
+--------------------------+--------------------------+-------------------------+
|     HEALTHY (Serving)    |    UNHEALTHY (Broken)    |          OTHER          |
| max(0, Alive - Quar - M) |  Swarming + UFS Errors   | Excluded + Missing Bots |
+--------------------------+--------------------------+-------------------------+
                                        |
                                        v
+-------------------------------------------------------------------------------+
|                        DRILL-DOWN SUB-MENU ITEMS                              |
|                    (Exact Marginal Counts on Click)                           |
+-------------------------------------------------------------------------------+
|  Offline/Dead: sDead  |  Quarantined: sQuarantined  |  Maintenance: sMaint    |
+-------------------------------------------------------------------------------+
```

### 1. Top-Level Cards (100% Mutually Exclusive Accounts)
Card sums equal 100% of `totalDevices`:
- **Healthy**: `Math.max(0, sAlive - sQuarantined - sMaintenance)`. We subtract quarantined and maintenance bots from alive counts to show true test capacity.
- **Other**: Aggregates non-serving UFS devices (`RESERVED`, `DECOMMISSIONED`) and devices missing Swarming bot registrations.
- **Unhealthy**: Aggregates all serving devices with Swarming bot failures (`sDead`, `sQuarantined`, `sMaintenance`) or UFS inventory errors (`NEEDS_REPAIR`, `MISSING`).

### 2. Drill-Down Sub-Items (Exact Marginal Counts)
Sub-menu items display exact marginal counts (`sDead`, `sQuarantined`, `sMaintenance`) so operators can triage all affected devices. Scorecard tooltips clarify that multi-flag bots may appear in multiple sub-item filters.

## Planned Backend Architecture

### 1. Server-Side Composite Status
The backend (`CountBrowserDevices`) will compute a single composite status (`bot_health_status`) per bot row using strict severity precedence (`DEAD > QUARANTINED > MAINTENANCE > HEALTHY`):

```sql
CASE
  WHEN swarming_dead = true THEN 'DEAD'
  WHEN swarming_quarantined = true THEN 'QUARANTINED'
  WHEN swarming_maintenance = true THEN 'MAINTENANCE'
  WHEN swarming_alive = true THEN 'HEALTHY'
  ELSE 'MISSING'
END AS bot_health_status
```

We define the API enum in `chromebrowser.proto`:
```protobuf
enum BotHealthStatus {
  BOT_HEALTH_STATUS_UNSPECIFIED = 0;
  BOT_HEALTH_STATUS_HEALTHY = 1;
  BOT_HEALTH_STATUS_DEAD = 2;
  BOT_HEALTH_STATUS_QUARANTINED = 3;
  BOT_HEALTH_STATUS_MAINTENANCE = 4;
  BOT_HEALTH_STATUS_MISSING = 5;
}
```

### 2. Multi-Dimensional Aggregation (`group_by`)
`CountBrowserDevices` will support multi-dimensional `group_by`:
```proto
CountBrowserDevicesRequest {
  string filter = 1;
  repeated string group_by = 2;
}
```

Top-level summary cards map from faceted cells:
| Summary Card | Faceted Intersection Cell Pattern |
| :--- | :--- |
| **Healthy** | `(SERVING, HEALTHY)` |
| **Unhealthy** | `(SERVING, DEAD \| QUARANTINED \| MAINTENANCE) ∪ (NEEDS_REPAIR \| MISSING, *)` |
| **Other** | `(SERVING, MISSING) ∪ (RESERVED \| DECOMMISSIONED, *)` |

Drill-down filters continue using marginal attribute queries (`filter: swarming.quarantined = true`) so operators do not miss multi-flag bots.

### 3. Unified Triage Precedence
| Tier | Category | Summary Card | Color | Example States |
| :---: | :--- | :--- | :--- | :--- |
| **1** | Hardware Error | Unhealthy | Red | UFS `NEEDS_REPAIR`, `MISSING` |
| **2** | Offline / Dead | Unhealthy | Red | Swarming `DEAD`, Android `OFFLINE` |
| **3** | Quarantined | Unhealthy | Yellow | Swarming `QUARANTINED` |
| **4** | Maintenance | Recovering | Grey | Swarming `MAINTENANCE`, Android `PREPPING` |
| **5** | Serving Workloads | Healthy | Green | UFS `SERVING` + Swarming `ALIVE` |
