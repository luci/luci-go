# Device Labels Storage Architecture

## Context
Fleet Console ingests static and dynamic labels from multiple upstream sources. We needed a database storage model for device metadata.

## Architecture
We use a **hybrid storage model**: dedicated `jsonb` columns per source (`swarming_labels`, `ufs_labels`) combined with explicit typed columns for custom calculated fields.

## Tradeoffs

### 1. Single Universal `jsonb` Column
- **Pros:** No database migrations needed when adding new labels.
- **Cons:** Weak typing, mixes data sources into one blob.

### 2. Dedicated Column Per Label
- **Pros:** Strict typing and isolated columns.
- **Cons:** Migration friction for every new label and wasted storage for NULL values.

### 3. Hybrid Storage (Chosen)
- **Pros:** Isolates data sources by column, provides strict typing for custom fields, and supports partial updates when individual scrapers fail.
- **Cons:** Handlers query multiple database fields.
- **Decision:** Selected because it balances strict typing with schema flexibility.
