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

CREATE TABLE IF NOT EXISTS datacenters (
	id int NOT NULL AUTO_INCREMENT,
	-- The name of this datacenter.
	name varchar(255),
	-- A description of this datacenter.
	description varchar(255),
	PRIMARY KEY (id),
	UNIQUE (name)
);

CREATE TABLE IF NOT EXISTS racks (
	id int NOT NULL AUTO_INCREMENT,
	-- The name of this rack.
	name varchar(255),
	-- A description of this rack.
	description varchar(255),
	-- The datacenter this rack belongs to.
	datacenter_id int,
	-- The state of this rack.
	state varchar(255),
	PRIMARY KEY (id),
	FOREIGN KEY (datacenter_id) REFERENCES datacenters (id) ON DELETE CASCADE,
	UNIQUE (name)
);

CREATE TABLE IF NOT EXISTS switches (
	id int NOT NULL AUTO_INCREMENT,
	-- The name of this switch.
	name varchar(255),
	-- A description of this switch.
	description varchar(255),
	-- The number of ports this switch has.
	ports int,
	-- The rack this switch belongs to.
	rack_id int,
	-- The state of this switch.
	state varchar(255),
	PRIMARY KEY (id),
	FOREIGN KEY (rack_id) REFERENCES racks (id) ON DELETE CASCADE,
	UNIQUE (name)
);
