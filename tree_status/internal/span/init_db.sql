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
-- This script initializes a LUCI Tree Status Spanner database.

-- Status are the individual status updates on a tree.
-- Write rate is very low (< 10 / day) so we use timestamp directly as
-- part of the primary key.  This keeps read access to the latest value
-- fast.
-- Status are "Public User Data" (go/spud) and must be deleted within
-- 175 days, so we set TTL to 140 days to allow ample time for Spanner
-- compactions.
CREATE TABLE Status (
  -- The name of the tree.
  TreeName STRING(32) NOT NULL,
  -- The unique identifier for the status. This is a randomly generated
  -- 128-bit ID, encoded as 32 lowercase hexadecimal characters.
  StatusId STRING(32) NOT NULL,
  -- The status of the tree.  One of 'open', 'closed', 'throttled' or 'maintenance'
  -- using the enum values in protos/v1/tree_status.proto
  GeneralStatus INT64 NOT NULL,
  -- The message provided with the status update.
  Message STRING(1024) NOT NULL,
  -- The username of the user who created the status update.
  -- This column will be overwritten to 'user' 30 days after CreationTime.
  -- This column should never be exported to BigQuery.
  CreateUser STRING(256) NOT NULL,
  -- The time the status update was created.
  -- Also used to control TTL of status values.
  CreateTime TIMESTAMP NOT NULL OPTIONS (allow_commit_timestamp=true),
) PRIMARY KEY (TreeName, StatusId),
ROW DELETION POLICY (OLDER_THAN(CreateTime, INTERVAL 140 DAY));
