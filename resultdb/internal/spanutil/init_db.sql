-- Copyright 2019 The LUCI Authors.
--
-- Licensed under the Apache License, Version 2.0 (the "License");
-- you may not use this file except in compliance with the License.
-- You may obtain a copy of the License at
--
--      http://www.apache.org/licenses/LICENSE-2.0
--
-- Unless required by applicable law or agreed to in writing, software
-- distributed under the License is distributed on an "AS IS" BASIS,
-- WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
-- See the License for the specific language governing permissions and
-- limitations under the License.

--------------------------------------------------------------------------------
-- This script initializes a ResultDB Spanner database.

-- Stores the invocations.
-- This is the root table for much of the other data and tables, which define the
-- hierarchy (dependency graph, subsets of interest) for invocations.
CREATE TABLE Invocations (
  -- Identifies an invocation.
  -- Format: "${hex(sha256(user_provided_id)[:8])}:${user_provided_id}".
  -- SQL query construction: "CONCAT(SUBSTR(TO_HEX(SHA256(${user_provided_id})), 0, 8), ':', ${user_provided_id})"
  InvocationId STRING(MAX) NOT NULL,

  -- A random value in [0, Shards) where Shards constant is
  -- defined in code.
  -- Used in global secondary indexes, to prevent hot spots.
  -- The maximum value of ShardId in Spanner can be determined by querying the
  -- very first row in InvocationsByExpiration index.
  ShardId INT64 NOT NULL,

  -- Invocation state, see InvocationState in invocation.proto
  State INT64 NOT NULL,

  -- Security realm this invocation belongs to.
  -- Used to enforce ACLs.
  Realm STRING(64) NOT NULL,

  -- When to delete the invocation from the table.
  InvocationExpirationTime TIMESTAMP NOT NULL,

  -- When to delete expected test results from this invocation.
  -- When expected results are removed, this column is set to NULL.
  ExpectedTestResultsExpirationTime TIMESTAMP,

  -- When the invocation was created.
  CreateTime TIMESTAMP NOT NULL OPTIONS (allow_commit_timestamp=true),

  -- LUCI identity who created the invocation, typically "user:<email>".
  CreatedBy STRING(MAX),

  -- When the invocation started finalizing (invocation state was set to
  -- Finalizing).
  -- This means the invocation became immutable but directly or
  -- indirectly included invocations may still be mutable.
  FinalizeStartTime TIMESTAMP OPTIONS (allow_commit_timestamp=true),

  -- When the invocation finished finalizing (invocation state set to
  -- Finalized).
  -- This means the invocation and all its directly or indirectly
  -- included invocations became immutable.
  FinalizeTime TIMESTAMP OPTIONS (allow_commit_timestamp=true),

  -- When to force invocation finalization.
  Deadline TIMESTAMP NOT NULL,

  -- List of colon-separated key-value tags.
  -- Corresponds to Invocation.tags in invocation.proto.
  Tags ARRAY<STRING(MAX)>,

  -- Value of CreateInvocationRequest.request_id.
  -- Used to dedup invocation creation requests.
  CreateRequestId STRING(MAX),

  -- Requests to export the invocation to BigQuery, see also
  -- Invocation.bigquery_exports in invocation.proto.
  -- Each array element is a binary-encoded luci.resultdb.v1.BigQueryExport
  -- message.
  BigQueryExports ARRAY<BYTES(MAX)>,

  -- Value of Invocation.producer_resource. See its documentation.
  ProducerResource STRING(MAX),

  -- The common test id prefix for all test results directly included by the
  -- invocation.
  CommonTestIDPrefix STRING(MAX),

  -- Union of all variants of test results directly included by the invocation.
  TestResultVariantUnion ARRAY<STRING(MAX)>,

  -- DEPRECATED - DO NOT USE: Union of all variants of test results
  -- included by the invocation, directly and indirectly.
  TestResultVariantUnionRecursive ARRAY<STRING(MAX)>,

  -- The deadline, but is NULL if the invocation is not active.
  ActiveDeadline TIMESTAMP AS (IF(State = 1, Deadline, NULL)) STORED,

  -- A serialized then compressed google.protobuf.Struct that stores structured,
  -- domain-specific properties of the invocation.
  -- See spanutil.Compressed type for details of compression.
  Properties BYTES(MAX),

  -- Whether the invocation inherits its source information from the invocation that included it.
  InheritSources BOOL,

  -- A serialized luci.resultdb.v1.Sources message describing the source information for the
  -- invocation. Only this or InheritSources may be set, not both.
  Sources BYTES(MAX),

  -- Whether the invocation's source specification is immutable. This pertains to both
  -- the InheritSources field and Sources field.
  IsSourceSpecFinal BOOL,

  -- Whether this invocation is a root of the invocation graph for export purposes.
  --
  -- To help downstream systems make sense of test results, and gather overall
  -- context for a result, ResultDB data export is centered around roots. The roots
  -- typically represent a top-level buildbucket build, like a postsubmit build
  -- or presubmit tryjob. Test results are only exported if they are included from
  -- a root. They may be exported multiple times of they are included by multiple
  -- roots.
  --
  -- N.B. Roots do not affect legacy BigQuery exports configured by the
  -- BigQueryExports field.
  IsExportRoot BOOL,

  -- A user-specified baseline identifier that maps to a set of test variants.
  -- Often, this will be the source that generated the test result, such as the
  -- builder name for Chromium. For example, the baseline identifier may be
  -- try:linux-rel. The supported syntax for a baseline identifier is
  -- ^[a-z0-9\-_.]{1,100}:[a-zA-Z0-9\-_.\(\) ]{1,128}$. This syntax was selected
  -- to allow <buildbucket bucket name>:<buildbucket builder name> as a valid
  -- baseline ID.
  -- See go/src/go.chromium.org/luci/buildbucket/proto/builder_common.proto for
  -- character lengths for buildbucket bucket name and builder name.
  --
  -- Baselines are used to identify new tests; a subtraction between the set of
  -- test variants for a baseline in the Baselines table and test variants from
  -- a given invocation determines whether a test is new.
  BaselineId STRING(229),

  -- An invocation being marked submitted indicates that the test variants from
  -- this invocation are added to the set of test variants for its baseline. The
  -- set of test variants for the baseline are then used to identify new tests.
  -- If the invocation is not yet finalized at the time it is being marked
  -- submitted, it will be scheduled for handling after being finalized.
  -- Finalization does not make this field immutable - it can can be updated
  -- after the invocation has been finalized.
  Submitted BOOL,

  -- A serialized luci.resultdb.v1.Instructions describing instructions for this invocation.
  -- It may contains instructions for steps (for build-level invocation) and test results.
  -- It may contain instructions to test results directly contained in this invocation,
  -- and test results in included invocations.
  Instructions BYTES(MAX),

  -- A compressed, serialized luci.resultdb.internal.invocations.ExtendedProperties message.
  ExtendedProperties BYTES(MAX),
) PRIMARY KEY (InvocationId),
-- Add TTL of 1.5 years to Invocations table. The row deletion policy
-- configured in the parent table will also take effect on the interleaved child
-- tables (Artifacts, IncludedInvocations, TestExonerations, TestResults,
-- TestResultCounts).  Leave 30 days for Spanner to actually delete the data from
-- storage after the row is deleted.
  ROW DELETION POLICY (OLDER_THAN(CreateTime, INTERVAL 510 DAY));

-- Index of invocations by expiration time.
-- Used by a cron job that periodically removes expired invocations.
CREATE INDEX InvocationsByInvocationExpiration
  ON Invocations (ShardId DESC, InvocationExpirationTime, InvocationId);

-- Index of invocations by expected test result expiration.
-- Used by a cron job that periodically removes expected test results.
CREATE NULL_FILTERED INDEX InvocationsByExpectedTestResultsExpiration
  ON Invocations (ShardId DESC, ExpectedTestResultsExpirationTime, InvocationId);

-- Index of active invocations by deadline.
-- Used to query invocations overdue to be finalized.
CREATE NULL_FILTERED INDEX InvocationsByActiveDeadline
  ON Invocations (ShardId DESC, ActiveDeadline, InvocationId);

-- Stores ids of invocations included in another invocation.
-- Interleaved in Invocations table.
CREATE TABLE IncludedInvocations (
  -- ID of the including invocation, the "source" node of the edge.
  InvocationId STRING(MAX) NOT NULL,

  -- ID of the included invocation, the "target" node of the edge.
  IncludedInvocationId STRING(MAX) NOT NULL
) PRIMARY KEY (InvocationId, IncludedInvocationId),
  INTERLEAVE IN PARENT Invocations ON DELETE CASCADE;

-- Reverse of IncludedInvocations.
-- Used to find invocations including a given one.
CREATE INDEX ReversedIncludedInvocations
  ON IncludedInvocations (IncludedInvocationId, InvocationId);

-- Tracks the export roots an invocation is directly or indirectly included by.
-- For each export root the invocation is included by, the sources it is
-- eligible to inherit (will inherit if it chooses to inherit sources by
-- setting SourceSpec.inherit = true) are tracked.
-- Export roots are considered to include themselves, and so will have
-- a row in this table where RootInvocationId equals InvocationId.
CREATE TABLE InvocationExportRoots (
  -- ID of the parent Invocations row.
  InvocationId STRING(MAX) NOT NULL,

  -- ID of the root invocation the invocation is directly or indirectly
  -- included by.
  RootInvocationId STRING(MAX) NOT NULL,

  -- Whether inherited sources for this invocation have been resolved.
  -- The value is then stored in InheritedSources.
  IsInheritedSourcesSet BOOL NOT NULL,

  -- The sources this invocation is eligible to inherit for its inclusion
  -- (directly or indirectly) from RootInvocationId.
  -- This may to a concrete luci.resultdb.v1.Sources value (if concrete
  -- sources are eligible to be inherited) or to a nil value (if empty
  -- sources are eligible to be inherited).
  -- To be able to distinguish inheriting empty/nil sources and the inherited
  -- sources not being resolved yet, see HasInheritedSourcesResolved.
  InheritedSources BYTES(MAX),

  -- The timestamp a `invocation-ready-for-export` pub/sub notification was
  -- sent for this row. Used for debugging and to avoid triggering
  -- pub/sub notifications that were already sent.
  NotifiedTime TIMESTAMP OPTIONS (allow_commit_timestamp=true),
) PRIMARY KEY (InvocationId, RootInvocationId),
  INTERLEAVE IN PARENT Invocations ON DELETE CASCADE;

-- Stores test results. Interleaved in Invocations.
CREATE TABLE TestResults (
  -- ID of the parent Invocations row.
  InvocationId STRING(MAX) NOT NULL,

  -- Unique identifier of the test,
  -- see also TestResult.test_id in test_result.proto.
  TestId STRING(MAX) NOT NULL,

  -- A suffix for PK to allow multiple test results for the same test id in
  -- a given invocation.
  -- Generated on the server.
  ResultId STRING(MAX) NOT NULL,

  -- key:value pairs in the test variant.
  -- See also TestResult.variant in test_result.proto.
  Variant ARRAY<STRING(MAX)>,

  -- A hash of the key:variant pairs in the test variant.
  -- Computed as hex(sha256(<concatenated_key_value_pairs>)[:8]),
  -- where concatenated_key_value_pairs is the result of concatenating
  -- variant pairs formatted as "<key>:<value>\n" in ascending key order.
  -- Used to filter test results by variant.
  VariantHash STRING(16) NOT NULL,

  -- Last time this row was modified.
  -- Given that we only create and delete row, for an existing row this equals
  -- row creation time.
  CommitTimestamp TIMESTAMP NOT NULL OPTIONS (allow_commit_timestamp=true),

  -- Whether the test status was unexpected
  -- MUST be either NULL or True, to keep null-filtered index below thin.
  IsUnexpected BOOL,

  -- Test status, see `status` in test_result.proto.
  Status INT64 NOT NULL,

  -- Test status V2, see `status_v2` in test_result.proto.
  StatusV2 INT64,

  -- Compressed summary of the test result for humans, in HTML.
  -- See span.Compressed type for details of compression.
  SummaryHTML BYTES(MAX),

  -- When the test execution started.
  StartTime TIMESTAMP,

  -- How long the test execution took, in microseconds.
  RunDurationUsec INT64,

  -- Tags associated with the test result, for example GTest-specific test
  -- status.
  Tags ARRAY<STRING(MAX)>,

  -- Compressed metadata for the test case.
  -- For example original test name, test location, etc.
  -- See TestResult.test_metadata for details.
  -- See span.Compressed type for details of compression.
  TestMetadata BYTES(MAX),

  -- Compressed information on how the test failed.
  -- For example error messages, stack traces, etc.
  -- See `failure_reason` in test_results.proto for details.
  -- See span.Compressed type for details of compression.
  FailureReason BYTES(MAX),

  -- A serialized then compressed google.protobuf.Struct that stores structured,
  -- domain-specific properties of the test result.
  -- See spanutil.Compressed type for details of compression.
  Properties BYTES(MAX),

  -- Reasoning behind a test skip, in machine-readable form.
  -- Used to assist downstream analyses, such as automatic bug-filing.
  -- Skip reason 0 (SKIP_REASON_UNSPECIFIED) is mapped to NULL.
  -- MUST be NULL unless status is SKIP.
  SkipReason INT64,

  -- Compressed information about why a test was skipped.
  -- See `skipped_reason` in test_results.proto for details.
  -- See span.Compressed type for details of compression.
  SkippedReason BYTES(MAX),

  -- Compressed test framework-specific data elements.
  -- See `framework_extensions` in test_results.proto for details.
  -- See spanutil.Compressed type for details of compression.
  FrameworkExtensions BYTES(MAX),
) PRIMARY KEY (InvocationId, TestId, ResultId),
  INTERLEAVE IN PARENT Invocations ON DELETE CASCADE;

-- Stores artifacts. Interleaved in Invocations.
CREATE TABLE Artifacts (
  -- Id of the parent Invocations row.
  InvocationId STRING(MAX) NOT NULL,

  -- An invocation-local ID of the Artifact parent:
  -- *   "" for invocation-level artifacts.
  -- *   "tr/{test_id}/{result_id}" for test-result-level artifacts.
  --     test_id is NOT URL-encoded because result_id cannot have a slash.
  ParentId STRING(MAX) NOT NULL,

  -- Unique identifier of the artifact within the parent.
  -- May have slashes.
  -- Example: "stdout" of a test result.
  ArtifactId STRING(MAX) NOT NULL,

  -- Media type of the artifact content.
  ContentType STRING(MAX),

  -- Content size in bytes.
  -- In the case of a GCS artifact, this field is user supplied, optional and not verified.
  Size INT64,

  -- Hash of the artifact content if it is stored in RBE-CAS.
  -- Format: "sha256:{hash}" where the hash is a lower-case hex-encoded SHA256
  -- hash of the artifact content.
  -- Example: e.g. "sha256:deadbeefdeadbeefdeadbeefdeadbeefdeadbeef"
  --
  -- The RBE-CAS instance is in the same Cloud project, named "artifacts".
  -- This field is NULL for a GCS artifact.
  RBECASHash STRING(MAX),

  -- A string of format "isolate://{isolateServerHost}/{namespace}/{hash}"
  -- if this artifact is stored in isolate.
  -- TODO(nodir): remove this when we completely switch to ResultSink.
  IsolateURL STRING(MAX),

  -- A string of format "gs://{bucket}/{path}"
  -- if this artifact is stored in Google Cloud Storage (GCS).
  GcsURI STRING(MAX),
) PRIMARY KEY (InvocationId, ParentId, ArtifactId),
  INTERLEAVE IN PARENT Invocations ON DELETE CASCADE;

-- Unexpected test results for each invocation.
-- It is significantly smaller (<2%) than TestResult table and should be used
-- for most queries.
-- It includes TestId to be able to find all unexpected test result with a
-- given test id or a test id prefix.
CREATE NULL_FILTERED INDEX UnexpectedTestResults
  ON TestResults (InvocationId, TestId, IsUnexpected) STORING (VariantHash, Variant),
  INTERLEAVE IN Invocations;


-- Stores test exonerations, see TestExoneration in test_result.proto
CREATE TABLE TestExonerations (
  -- ID of the parent Invocations row.
  InvocationId STRING(MAX) NOT NULL,

  -- The exoneration applies only to test results with this exact test id.
  -- This is a foreign key to TestResults.TestId column.
  TestId STRING(MAX) NOT NULL,

  -- Server-generated exoneration ID.
  -- Uniquely identifies a test exoneration within an invocation.
  --
  -- Starts with "{hex(sha256(join(sorted('{p}\n' for p in Variant))))}:".
  -- The prefix can be used to reduce scanning for test exonerations for a
  -- particular test variant.
  ExonerationId STRING(MAX) NOT NULL,

  -- The exoneration applies only to test results with this exact test variant.
  Variant ARRAY<STRING(MAX)> NOT NULL,

  -- A hash of the key:variant pairs in the test variant.
  -- Computed as hex(sha256(<concatenated_key_value_pairs>)[:8]),
  -- where concatenated_key_value_pairs is the result of concatenating
  -- variant pairs formatted as "<key>:<value>\n" in ascending key order.
  -- Used in conjunction with TestResults.VariantHash column.
  VariantHash STRING(16) NOT NULL,

  -- Compressed explanation of the exoneration for humans, in HTML.
  -- See span.Compress type for details of compression.
  ExplanationHTML BYTES(MAX),

  -- The reason the test variant was exonerated.
  -- See resultdb.v1.ExonerationReason.
  Reason INT64 NOT NULL,
) PRIMARY KEY (InvocationId, TestId, ExonerationId),
  INTERLEAVE IN PARENT Invocations ON DELETE CASCADE;

-- Stores transactional tasks reminders.
-- See https://go.chromium.org/luci/server/tq. Scanned by tq-sweeper-spanner.
CREATE TABLE TQReminders (
  ID STRING(MAX) NOT NULL,
  FreshUntil TIMESTAMP NOT NULL,
  Payload BYTES(102400) NOT NULL,
) PRIMARY KEY (ID ASC);

-- Stores test result counts for invocations. Sharded.
-- Interleaved in Invocations table.
CREATE TABLE TestResultCounts (
  -- ID of a invocation.
  InvocationId STRING(MAX) NOT NULL,

  -- ID of a shard.
  ShardId INT64 NOT NULL,

-- Counter of TesultResults that belongs to this shard of invocation directly.
  TestResultCount INT64,
) PRIMARY KEY (InvocationId, ShardId),
  INTERLEAVE IN PARENT Invocations ON DELETE CASCADE;

-- Stores per-test test metadata. See test_metadata_row.proto.
CREATE TABLE TestMetadata (
  -- The LUCI project in which the test was observed.
  Project STRING(40) NOT NULL,

  -- Unique identifier of the test,
  -- see also TestResult.test_id in test_result.proto.
  TestId STRING(MAX) NOT NULL,

  -- Hash of the reference which specifies where the code changes come from.
  -- For example, for git, using the following formula
  -- ([:8] indicates truncation to 8 bytes).
  -- SHA256("git" + "\n" +  hostname + "\n" + project + "\n"  + ref)[:8]
  RefHash BYTES(8) NOT NULL,

  -- The realm of the test result from which the variant was observed, excluding
  -- project. 62 as ResultDB allows at most 64 characters for the construction
  -- "<project>:<realm>" and project must be at least one character.
  SubRealm STRING(62) NOT NULL,

  -- The Spanner commit time the row last last updated.
  LastUpdated TIMESTAMP NOT NULL OPTIONS (allow_commit_timestamp=true),

  -- Compressed metadata for the test case.
  -- For example original test name, test location, etc.
  -- See resultdb.v1.TestMetadata for details.
  TestMetadata BYTES(MAX),

  -- Compressed source control reference.
  -- This is used to compute the RefHash.
  -- See resultdb.v1.SourceRef for details.
  SourceRef BYTES(MAX),

   -- Commit position of the test result which updated the test metadata last time.
   -- Position is always positive.
   Position INT64 NOT NULL,
) PRIMARY KEY (Project, TestId, RefHash, SubRealm),
  ROW DELETION POLICY (OLDER_THAN(LastUpdated, INTERVAL 90 DAY));

-- Stores test baselines. A baseline is a named set of test variants which is
-- believed to be part of the submitted code for a project. New tests are detected
-- by subtracting from an invocation all the test variants in its corresponding baseline.
CREATE TABLE Baselines (
  -- The LUCI project in which the test was observed.
  Project STRING(40) NOT NULL,

  -- A user-specified baseline identifier that maps to a set of test variants.
  -- Often, this will be the source that generated the test result, such as the
  -- builder name for Chromium. For example, the baseline identifier may be
  -- try:linux-rel. The supported syntax for a baseline identifier is
  -- ^[a-z0-9\-_.]{1,100}:[a-zA-Z0-9\-_.\(\) ]{1,128}$. This syntax was selected
  -- to allow <buildbucket bucket name>:<buildbucket builder name> as a valid
  -- baseline ID.
  -- See go/src/go.chromium.org/luci/buildbucket/proto/builder_common.proto for
  -- character lengths for buildbucket bucket name and builder name.
  --
  -- Baselines are used to identify new tests; a subtraction between the set of
  -- test variants for a baseline in the Baselines table and test variants from
  -- a given invocation determines whether a test is new.
  BaselineId STRING(229) NOT NULL,

  -- The time the baseline was last updated.
  LastUpdatedTime TIMESTAMP NOT NULL OPTIONS (allow_commit_timestamp=true),

  -- The time the baseline was created.
  CreationTime TIMESTAMP NOT NULL OPTIONS (allow_commit_timestamp=true),
) PRIMARY KEY (Project, BaselineId),
  ROW DELETION POLICY (OLDER_THAN(LastUpdatedTime, INTERVAL 3 DAY));

-- Stores test baselines: the set of test variants which are expected to be run on a target (such as builder).
CREATE TABLE BaselineTestVariants (
  -- The LUCI project in which the test was observed.
  Project STRING(40) NOT NULL,

  -- A user-specified baseline identifier that maps to a set of test variants.
  -- Often, this will be the source that generated the test result, such as the
  -- builder name for Chromium. For example, the baseline identifier may be
  -- try:linux-rel. The supported syntax for a baseline identifier is
  -- ^[a-z0-9\-_.]{1,100}:[a-zA-Z0-9\-_.\(\) ]{1,128}$. This syntax was selected
  -- to allow <buildbucket bucket name>:<buildbucket builder name> as a valid
  -- baseline ID.
  -- See go/src/go.chromium.org/luci/buildbucket/proto/builder_common.proto for
  -- character lengths for buildbucket bucket name and builder name.
  --
  -- Baselines are used to identify new tests; subtracting the test variants in
  -- a baseline from the test variants in an invocation determines which test
  -- variants are new.
  BaselineId String(229) NOT NULL,

  -- Unique identifier of the test,
  -- see also TestResult.test_id in test_result.proto.
  TestId STRING(MAX) NOT NULL,

  -- A hash of the key:variant pairs in the test variant.
  -- Computed as hex(sha256(<concatenated_key_value_pairs>)[:8]),
  -- where concatenated_key_value_pairs is the result of concatenating
  -- variant pairs formatted as "<key>:<value>\n" in ascending key order.
  -- Used to filter test results by variant.
  VariantHash STRING(16) NOT NULL,

  -- When the test history was introduced for the (project, subrealm, source).
  -- Used to remove tests that have not been run longer than 72 hours.
  LastUpdated TIMESTAMP NOT NULL OPTIONS (allow_commit_timestamp=true),
) PRIMARY KEY (Project, BaselineId, TestId, VariantHash),
  ROW DELETION POLICY (OLDER_THAN(LastUpdated, INTERVAL 3 DAY));

-- Checkpoints is used by processes to ensure they only perform some task
-- once, or as close to once as possible.
-- It is useful when the task being performed is not inherently idempontent,
-- such as exporting rows to BigQuery.
CREATE TABLE Checkpoints (
  -- LUCI Project for which the process is occuring.
  -- Used to enforce hard data separation between the data of each project.
  Project STRING(40) NOT NULL,
  -- The identifier of the resource to which the checkpointed process relates.
  -- For example, the ResultDB invocation being ingested.
  ResourceId STRING(MAX) NOT NULL,
  -- The name of process for which checkpointing is occuring. For example,
  -- "result-ingestion/schedule-continuation".
  -- Used to namespace checkpoints between processes.
  -- Valid pattern: ^[a-z0-9\-/]{1,64}$.
  ProcessId STRING(MAX) NOT NULL,
  -- A uniqifier for the checkpoint.
  -- This could be the page number processed, or the starting test identifier
  -- and variant hash of a batch that was processed.
  -- For processes with only one checkpoint, this may be left empty ("").
  Uniquifier STRING(MAX) NOT NULL,
  -- Time that this record was inserted in the table.
  CreationTime TIMESTAMP NOT NULL OPTIONS (allow_commit_timestamp=true),
  -- Time that this record expires from the table.
  ExpiryTime TIMESTAMP NOT NULL,
) PRIMARY KEY(Project, ResourceId, ProcessId, Uniquifier),
  ROW DELETION POLICY (OLDER_THAN(ExpiryTime, INTERVAL 0 DAY));
