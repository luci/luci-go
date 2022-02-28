-- Add TTL to ResultDB Spanner database based on the data retention
-- (go/resultdb#data-retention) and the proposal (go/resultdb-database-ttl).

-- Add TTL for 1.5 years to Invocations table. The row deletion policy
-- configured in the parent table will also take effect on the interleaved child
-- tables (Artifacts, IncludedInvocations, TestExonerations, TestResults,
-- TestResultCounts).
ALTER TABLE Invocations
    ADD ROW DELETION POLICY (OLDER_THAN(CreateTime, INTERVAL 540 DAY));
