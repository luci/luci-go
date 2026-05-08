# Device Labels Storage Design Decisions

## Context
We fetch a bunch of static and dynamic labels from various sources that describe a particular device. We needed to decide how to persist that information in the database.

## Current Architecture
We use a **hybrid approach**: we have a separate `jsonb` column per each source (e.g., `swarming_labels`, `ufs_labels`), and add separate columns for any customized or calculated columns.

## Design Tradeoffs Considered

### 1. Dump everything into one labels `jsonb` column
- **Pros:** No DB migrations needed when adding new labels.
- **Cons:** Lack of strict typing, harder to isolate data sources physically.

### 2. Have a separate column for each label
- **Pros:** Strict typing, data isolation.
- **Cons:** Migration fatigue (every minor change requires a migration), redundant memory usage for empty fields (NULL values).

### 3. Hybrid approach
- **Pros:** Data isolation at storage level, specific columns for custom labels (benefiting from strict typing where needed), partial updates (if one scraper fails, other data is untouched).
- **Cons:** Backend needs to handle multiple fields instead of just one.
- **Decision:** Chosen as it combines the pros of both options without adding too much complication.

## Links
None.
