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
-- This script initializes a LUCI Source Index Spanner database.

-- Commits contains metadata of git commits.
CREATE TABLE Commits (
  -- The gitiles host. Must be a subdomain of `.googlesource.com`
  -- (e.g. chromium.googlesource.com).
  Host STRING(255) NOT NULL,

  -- Gitiles project, e.g. "chromium/src" part in
  -- https://chromium.googlesource.com/chromium/src/+/main
  Repository STRING(100) NOT NULL,

  -- The full hex sha1 of the commit in lowercase.
  CommitHash STRING(40) NOT NULL,

  -- The name of position defined in value of git-footer git-svn-id
  -- or Cr-Commit-Position (e.g. refs/heads/master,
  -- svn://svn.chromium.org/chrome/trunk/src)
  PositionRef STRING(255),

  -- The sequential identifier of the commit in the given branch
  -- (position_ref).
  -- This is defined when and only when PositionRef is defined.
  PositionNumber INT64,

  -- The time the commit was last written to the table.
  --
  -- This is usually the time when the commit was created. But it might be
  -- updated if a commit is written multiple times (e.g. when the first commit
  -- in a commit page is not yet ingested but the tail commits in the same page
  -- are already ingested).
  --
  -- This will be useful if we need to purge commits ingested after certain
  -- timestamp due to errors.
  LastUpdatedTime TIMESTAMP NOT NULL OPTIONS (allow_commit_timestamp=true)
) PRIMARY KEY(Host, Repository, CommitHash);

-- Index commits by commit position.
-- To support mapping commit positions to commit hashes.
CREATE NULL_FILTERED INDEX CommitsByPosition
  ON Commits (Host, Repository, PositionRef, PositionNumber DESC);

-- Stores transactional tasks reminders.
-- See https://go.chromium.org/luci/server/tq. Scanned by tq-sweeper-spanner.
CREATE TABLE TQReminders (
    ID STRING(MAX) NOT NULL,
    FreshUntil TIMESTAMP NOT NULL,
    Payload BYTES(102400) NOT NULL,
) PRIMARY KEY (ID ASC);

-- Stores transactional tasks leases.
-- See https://go.chromium.org/luci/server/tq. Scanned by tq-sweeper-spanner.
CREATE TABLE TQLeases (
    SectionID STRING(MAX) NOT NULL,
    LeaseID INT64 NOT NULL,
    SerializedParts ARRAY<STRING(MAX)>,
    ExpiresAt TIMESTAMP NOT NULL,
) PRIMARY KEY (SectionID ASC, LeaseID ASC);
