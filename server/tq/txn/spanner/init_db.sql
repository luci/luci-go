-- Copyright 2021 The LUCI Authors.
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
-- This script initializes Spanner tables requires by tq.
CREATE TABLE TQReminders (
    ID STRING(MAX) NOT NULL,
    FreshUntil TIMESTAMP NOT NULL,
    Payload BYTES(102400) NOT NULL,
) PRIMARY KEY (ID ASC);

CREATE TABLE TQLeases (
    SectionID STRING(MAX) NOT NULL,
    LeaseID INT64 NOT NULL,
    SerializedParts ARRAY<STRING(MAX)>,
    ExpiresAt TIMESTAMP NOT NULL,
) PRIMARY KEY (SectionID ASC, LeaseID ASC);
