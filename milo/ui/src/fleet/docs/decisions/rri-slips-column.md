# RRI Slips Column Design Decisions

## Context
A P0 need for FLOPS was to be able to easily find resource requests that might be slipped (delayed). We needed to prioritize adding a separate column for "slips" in the RRI view.

## Current Architecture
We implemented quick SQL tricks to calculate the slip status on the fly. This allowed us to immediately land critical user impact without waiting for a full database migration.

## Design Tradeoffs Considered

### 1. Add the column to BigQuery and update it on BOQ side
- **Pros:** Cleaner data model if BigQuery remains the source of truth.
- **Cons:** Hides logic too deep; requires updating many layers to get a feature done.
- **Decision:** Rejected as inefficient for this specific need.

### 2. Make some SQL tricks to calculate it on the fly
- **Pros:** Fast implementation, delivers immediate value to users.
- **Cons:** Temporary solution; code will need to be deleted or refactored when we do a migration.
- **Decision:** Chosen for expediency to meet the P0 need.

### 3. Do it after migrating data from BigQuery to Postgres table
- **Pros:** Proper solution, makes managing data and mutations much easier.
- **Cons:** Delayed delivery.
- **Decision:** Left open as a future refactor to improve long-term implementation efficiency.

## Links
None.
