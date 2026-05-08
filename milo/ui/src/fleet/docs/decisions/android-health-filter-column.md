# Android Health Filter Column Design Decisions

## Context
FLOPs reported a discrepancy between the number of unhealthy devices in Fleet Console and MH. The check was based on device status only, whereas Omnilab documentation suggested checking device type as well. The structure of the check included operators like `AND`, `NOT IN`, which were impossible to replicate on the frontend side with current filters implementation.

## Current Architecture
We implemented a new column `fc_is_offline` to store calculated information about the device. This allowed the frontend to filter correctly without a complex refactor.

## Design Tradeoffs Considered

### 1. Implement a new column (`fc_is_offline`)
- **Pros:** Fast to implement, solves the immediate urgency, allows frontend to use existing filter mechanisms.
- **Cons:** Adds a specialized column that might need maintenance if the logic changes.
- **Decision:** Chosen due to the urgency of the situation.

### 2. Refactor filters mechanism
- **Pros:** Proper solution, handles complex queries on the frontend natively.
- **Cons:** Requires significant investment in refactoring and testing.
- **Decision:** Left open as a future improvement.

## Links
- Bug: `b/467077586`
