# Android Health Filter Column

## Context
FLOPS identified a mismatch in unhealthy device counts between Fleet Console and Mobile Harness. Checking health required combining device status and device type using complex operators (`AND`, `NOT IN`), which frontend URL filters could not evaluate natively.

## Architecture
We added a calculated column (`fc_is_offline`) in the database. This allows the UI to filter health states directly using standard equality filters.

## Tradeoffs

### 1. Database Calculated Column (`fc_is_offline`)
- **Pros:** Fast implementation, resolves urgent count discrepancies, uses existing frontend filter hooks.
- **Cons:** Requires schema maintenance if offline logic changes.
- **Decision:** Chosen to address the immediate P0 discrepancy.

### 2. Frontend Filter Engine Refactor
- **Pros:** Handles complex boolean queries natively in the UI.
- **Cons:** High development cost and complex refactoring.
- **Decision:** Deferred as a future enhancement.

## Links
- Bug: `b/467077586`
