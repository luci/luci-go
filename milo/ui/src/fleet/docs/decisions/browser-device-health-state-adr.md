# Decision: Chrome Browser Device Health Accounting and Multi-System Reconciliation

## Context

Chrome Browser device health metrics in the Fleet Console must reconcile two distinct administrative and operational layers:

1. **Administrative Intent (`UFS`)**: Represents what physical hardware should be doing (`SERVING`, `NEEDS_REPAIR`, `MISSING`, `EXCLUDED`).
2. **Operational Reality (`Swarming`)**: Represents the active workload execution capability of the bot daemon (`alive`, `dead`, `quarantined`, `maintenance`).

This decision record documents the mathematical tensions caused by overlapping Swarming bot flags when using aggregate API counts, formalizes our current two-tier accounting model (clamped mutually exclusive top-level buckets vs. marginal drill-down items), and establishes the long-term architectural roadmap to move disjoint health classification to the backend SQL layer.

## Architectural Challenges & Overlap Tradeoffs

### 1. Mutually Exclusive UFS States vs. Multi-Attribute Swarming Flags

A core challenge in calculating browser fleet health is that UFS and Swarming model state fundamentally differently:

- **UFS (`state.pb.go`)**: Resource states are strictly **mutually exclusive**. Every physical device in the UFS inventory has exactly one `resource_state` (`SERVING`, `NEEDS_REPAIR`, `MISSING`, or non-serving/excluded states like `RESERVED`, `DECOMMISSIONED`). The sum of all UFS states equals 100% of physical devices.
- **Swarming (`swarming.pb.go`)**: Bot states are represented as **non-mutually-exclusive boolean attributes** (`alive`, `dead`, `quarantined`, `maintenance`). A single bot daemon can exhibit multiple attributes simultaneously:
  - `Alive + Quarantined`: The bot daemon is checking in (`alive = true`), but health checks or operators have quarantined it (`quarantined = true`).
  - `Alive + Maintenance`: The bot daemon is checking in (`alive = true`), but is in maintenance mode (`maintenance = true`).
  - `Dead + Quarantined`: The bot daemon has stopped checking in (`dead = true`) and is marked quarantined (`quarantined = true`).

### 2. Shortcomings of Current `CountBrowserDevices` API

The current `CountBrowserDevices` RPC (`chromebrowser.proto`) returns independent **marginal counts** for each Swarming flag (`swarmingState.alive`, `swarmingState.dead`, `swarmingState.quarantined`, `swarmingState.maintenance`).

Because multi-flag bots appear in multiple marginal counts simultaneously:

1.  **Non-Additive Totals**: Summing raw marginal counts (`sDead + sQuarantined + sMaintenance`) can exceed the total number of physical bots or double-count bots with overlapping flags.
2.  **Misleading `Alive` Counts**: A raw `sAlive` count includes bots that are `Alive + Quarantined` or `Alive + Maintenance`. While technically checking in, these bots cannot run test workloads. Treating raw `sAlive` as "Healthy" would overstate actual lab test capacity.

## Current Client-Side Architecture

To present a scannable, mathematically coherent dashboard without hiding actionable triage data, the Fleet Console UI implements a **Two-Tier Accounting Strategy** in [browser_summary_header.tsx](../../pages/device_list_page/browser/browser_summary_header.tsx):

```
+-------------------------------------------------------------------------------------------------+
|                               TOP-LEVEL HEALTH SUMMARY CARDS                                    |
|             (100% Mutually Exclusive Fleet Accounting via Overlap-Safe Clamping)                |
+---------------------------------+-------------------------------+-------------------------------+
|         HEALTHY (Serving)       |       UNHEALTHY (Broken)      |             OTHER             |
|   max(0, Alive - Quar - Maint)  |   Broken Swarming + UFS Err   |    Excluded + Missing Bots    |
+---------------------------------+-------------------------------+-------------------------------+
                                                  |
                                                  v
+-------------------------------------------------------------------------------------------------+
|                                DRILL-DOWN SUB-MENU ITEMS                                        |
|         (Exact Marginal Counts for Actionable 100% Target Navigation on Click)                  |
+-------------------------------------------------------------------------------------------------+
|   Offline / Dead: sDead   |   Quarantined: sQuarantined   |   In Maintenance: sMaintenance  |
+-------------------------------------------------------------------------------------------------+
```

### 1. Top-Level Cards: Clamped Mutually Exclusive Buckets (100% Accounting)

For the primary summary cards (**Healthy**, **Unhealthy**, **Other**), the interface enforces strict mutual exclusivity so the sum of cards equals exactly 100% of `totalDevices`.

- **Healthy (`Serving / Healthy`)**:
  A UFS `SERVING` device is only truly healthy if its bot is checking in (`Alive`) AND free of workload-blocking flags (`Quarantined` or `Maintenance`).
  `healthyCount = Math.max(0, sAlive - sQuarantined - sMaintenance)`
  _Tradeoff Rationale_: We subtract `sQuarantined` and `sMaintenance` from `sAlive` so bots that check in but cannot run tests are excluded from healthy capacity. `Math.max(0, ...)` prevents negative underflow if bots carry overlapping flags.
- **Other (`Excluded / Inactive`)**:
  Aggregates intentionally non-serving UFS devices (`RESERVED`, `DECOMMISSIONED`, etc.) plus `SERVING` devices that lack a Swarming bot registration (`sMissingBots`).
  `otherCount = ufsExcluded + nonServingOthers + sMissingBots`
- **Unhealthy (`Broken / Attention Required`)**:
  Aggregates all remaining capacity expected to serve (`UFS SERVING`) that exhibits Swarming bot failures (`sDead`, `sQuarantined`, `sMaintenance`) or UFS inventory errors (`NEEDS_REPAIR`, `MISSING`).

### 2. Drill-Down Scorecard Items: Exact Marginal Counts

Within the interactive sub-lists (`Swarming Bot Issues:` -> `Offline / Dead`, `Quarantined`, `In Maintenance`), we display the **exact marginal count** returned by the API (`sDead`, `sQuarantined`, `sMaintenance`).

- **Tradeoff Rationale**:
  If an operator clicks `"Quarantined"`, their operational intent is to triage **all** quarantined bots. If the UI enforced artificial mutual exclusivity in sub-items (e.g., hiding a quarantined bot because it is also marked `dead`), the operator would miss broken devices during triage.
- **Transparency Disclosure**:
  Scorecard info tooltips explicitly state that drill-down sub-items reflect marginal filter counts and that multi-flag bots (`Dead + Quarantined`) may appear in multiple drill-down filters.

## Planned Transition / Future Architecture

To eliminate client-side clamping approximations and support exact multi-dimensional breakdowns as the Fleet Console scales, we plan to adopt the following backend architectural decisions:

### 1. Server-Side Composite Mutually-Exclusive Bot Status

Instead of relying on independent boolean flags in aggregate responses, the backend (`CountBrowserDevices`) should compute a deterministic, mutually-exclusive composite status (`bot_health_status`) per bot row using strict triage severity precedence (`DEAD > QUARANTINED > MAINTENANCE > HEALTHY`):

```sql
CASE
  WHEN swarming_dead = true THEN 'DEAD'
  WHEN swarming_quarantined = true THEN 'QUARANTINED'
  WHEN swarming_maintenance = true THEN 'MAINTENANCE'
  WHEN swarming_alive = true THEN 'HEALTHY'
  ELSE 'MISSING'
END AS bot_health_status
```

To formalize this API contract across frontend and backend services, we define the following Protobuf enum in `chromebrowser.proto`:

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

- **Consequence**: By bucketing bots at the SQL/storage layer using strict precedence, every bot belongs to exactly one category. The sum of composite categories is mathematically guaranteed to equal 100% of bots without client-side clamping.

### 2. Migrate to Multi-Dimensional Group-By (`Faceted Aggregation`)

Migrate `CountBrowserDevices` to use the unified multi-dimensional `group_by` RPC pattern specified in [flexible_device_counting.md](../../../../../../../infra/fleetconsole/designdocs/flexible_device_counting.md):

```proto
CountBrowserDevicesRequest {
  string filter = 1;
  repeated string group_by = 2; // e.g., ["ufs_resource_state", "bot_health_status"]
}
```

#### Concrete Cell-to-Summary Card Mapping Table

When `CountBrowserDevices` returns faceted `CountRecord` cells grouped by `(ufs_resource_state, bot_health_status)`, the UI maps those cells into mutually exclusive top-level summary cards as follows:

| Top-Level Summary Card | Faceted Intersection Cell Pattern (`ufs_resource_state`, `bot_health_status`) |
| :--------------------- | :---------------------------------------------------------------------------- | -------------- | ---------------------------- | ------------- |
| **Healthy**            | `(SERVING, HEALTHY)`                                                          |
| **Unhealthy**          | `(SERVING, DEAD                                                               | QUARANTINED    | MAINTENANCE)`∪`(NEEDS_REPAIR | MISSING, \*)` |
| **Other**              | `(SERVING, MISSING)` ∪ `(RESERVED                                             | DECOMMISSIONED | ..., \*)`                    |

#### Marginal vs. Faceted Drill-Down Architectural Tradeoff

While top-level summary cards aggregate mutually exclusive faceted `CountRecord` cells (`DEAD > QUARANTINED`), drill-down sub-items must continue to use **marginal attribute filters** (`filter: swarming.quarantined = true`). Because a `Dead + Quarantined` bot is bucketed under `'DEAD'` in strict precedence, generating drill-down lists solely from `'QUARANTINED'` faceted cells would omit multi-flag bots. Preserving marginal attribute queries for drill-down items ensures operators never miss multi-flag bots during triage.

### 3. Standardize Triage Precedence Across All Platforms

Standardize the severity ordering and visual token mapping across Android, ChromeOS, and Browser frontends:

| Tier  | Triage Category                | Top-Level Card        | Gestalt Theme Token | Example Platform States                    |
| :---: | :----------------------------- | :-------------------- | :------------------ | :----------------------------------------- |
| **1** | Hardware / Infra Error         | Unhealthy             | `rose` (Red)        | UFS `NEEDS_REPAIR`, `MISSING`              |
| **2** | Offline / Daemon Dead          | Unhealthy             | `rose` (Red)        | Swarming `DEAD`, Android `OFFLINE`         |
| **3** | Quarantined / Workload Blocked | Unhealthy             | `amber/yellow`      | Swarming `QUARANTINED`                     |
| **4** | Maintenance / Recovery         | Recovering / Sub-item | `grey/cyan`         | Swarming `MAINTENANCE`, Android `PREPPING` |
| **5** | Healthy / Serving Workloads    | Healthy               | `emerald` (Green)   | UFS `SERVING` + Swarming `ALIVE`           |

## References

- [health-metrics-design.md](./health-metrics-design.md): Principles of mutually exclusive bucketing and Gestalt visual grouping.
- [swarming_state.ts](../../pages/device_list_page/browser/swarming_state.ts): Client-side Swarming state sorting and precedence implementation.
- [browser_summary_header.tsx](../../pages/device_list_page/browser/browser_summary_header.tsx): Client-side two-tier health scorecard implementation.
