-- Copyright 2024 The LUCI Authors.
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
-- This script initializes a LUCI Notify Spanner database.

-- Alerts are individual alert items about the state of the builders.
-- Currently these are mapped 1:1 to Sheriff-o-Matic alerts, but will
-- change in the future if we move away from Sheriff-o-Matic.
-- TTL is set at 30 days to keep the table size reasonably small for fast queries.
CREATE TABLE Alerts (
  -- The key of the alert.
  -- For now, this matches the key of the alert in Sheriff-o-Matic.
  -- This may be revised in the future.
  AlertKey STRING(256) NOT NULL,
  -- The bug number in Buganizer/IssueTracker.
  -- 0 if the alert is not linked to a bug.
  Bug INT64,
  -- The Gerrit CL number associated with this alert.
  -- 0 if the alert is not associated with any CL.
  GerritCL INT64,
  -- The build number to consider this alert silenced until.
  -- 0 if the alert is not silenced.
  SilenceUntil INT64,
  -- The time the alert was last modified.
  -- Used to control TTL of alert values.
  ModifyTime TIMESTAMP NOT NULL OPTIONS (allow_commit_timestamp=true),
) PRIMARY KEY (AlertKey),
ROW DELETION POLICY (OLDER_THAN(ModifyTime, INTERVAL 30 DAY));

-- AlertGroups are collections of alerts that are managed together.
CREATE TABLE AlertGroups (
  -- The rotation the alert group belongs to.
  Rotation STRING(256) NOT NULL,
  -- The ID of the alert group.
  AlertGroupId STRING(256) NOT NULL,
  -- The human-readable name of the alert group.
  DisplayName STRING(1024),
  -- A message describing the status of the alert group.
  StatusMessage STRING(MAX),
  -- The keys of the alerts in this group.
  AlertKeys ARRAY<STRING(MAX)> NOT NULL,
  -- The bugs associated with this alert group.
  Bugs ARRAY<INT64> NOT NULL,
  -- The time the alert group was last updated.
  UpdateTime TIMESTAMP NOT NULL OPTIONS (allow_commit_timestamp=true),
  -- The identity of the user who last updated the alert group.
  -- If the TTL of this table is extended past 30 days, this column must be overwritten
  -- to 'user' 30 days after CreationTime.
  -- This column should never be exported to BigQuery.
  UpdatedBy STRING(256),
) PRIMARY KEY (Rotation, AlertGroupId),
ROW DELETION POLICY (OLDER_THAN(UpdateTime, INTERVAL 30 DAY));
