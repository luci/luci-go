-- Clean old data before setting TTL (go/resultdb-database-ttl). This gives us
-- more control over the resource usage, instead of leaving it to the TTL
-- background process. Row deletion policies run at a low priority, ideal for
-- incremental clean-up.

-- Delete old data over 1.5 years in Invocations table. The interleaved child
-- rows in child tables (Artifacts, IncludedInvocations, TestExonerations,
-- TestResultCounts) will be deleted atomically.
--
-- Execute via Partitioned DML for better performance:
-- gcloud spanner databases execute-sql luci-resultdb \
--     --project=luci-resultdb --instance=prod --enable-partitioned-dml \
--     --sql='DELETE FROM Invocations WHERE TIMESTAMP_ADD(CreateTime, INTERVAL 547 DAY) < CURRENT_TIMESTAMP()'
DELETE
FROM Invocations
WHERE TIMESTAMP_ADD(CreateTime, INTERVAL 540 DAY) < CURRENT_TIMESTAMP();
