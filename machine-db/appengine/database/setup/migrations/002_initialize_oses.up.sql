-- Copyright 2017 The LUCI Authors.
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

-- Required fields are not enforced by this schema.
-- The Machine Database will enforce any such constraints.

CREATE TABLE IF NOT EXISTS oses (
	id int NOT NULL AUTO_INCREMENT,
	-- The name of this operating system.
	name varchar(255),
	-- A description of this operating system.
	description varchar(255),
	PRIMARY KEY (id),
	UNIQUE (name)
);
