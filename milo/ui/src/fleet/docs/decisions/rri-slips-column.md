# RRI Slips Column Architecture

## Context
FLOPS requested a dedicated column in the Resource Request Insights (RRI) view to filter delayed ("slipped") resource requests.

## Architecture
We implemented on-the-fly SQL calculations to compute slippage without waiting for a full database migration.

## Tradeoffs

### 1. Update BigQuery Source Schema
- **Pros:** Keeps data logic inside BigQuery.
- **Cons:** Slow delivery across multiple service layers.

### 2. On-the-Fly SQL Calculation (Chosen)
- **Pros:** Fast implementation, delivers immediate user value.
- **Cons:** Temporary logic to replace during database migrations.
- **Decision:** Selected for immediate P0 delivery.

### 3. Migration to Postgres Table
- **Pros:** Simplifies queries and data mutations.
- **Cons:** Delayed release.
- **Decision:** Planned for future database migration work.
