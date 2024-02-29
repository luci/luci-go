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
-- This script initializes a LUCI Teams Spanner database.

-- TODO: Add comment.
CREATE TABLE Teams (
  -- TODO: Replace with final schema.

  -- The unique identifier for the team. This is a randomly generated
  -- 128-bit ID, encoded as 32 lowercase hexadecimal characters.
  Id STRING(32) NOT NULL,
  -- The time the status update was created.
  -- Also used to control TTL of status values.
  CreateTime TIMESTAMP NOT NULL OPTIONS (allow_commit_timestamp=true),
) PRIMARY KEY (Id);
