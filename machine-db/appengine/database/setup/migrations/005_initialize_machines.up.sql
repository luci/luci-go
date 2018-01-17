-- Copyright 2018 The LUCI Authors.
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

CREATE TABLE IF NOT EXISTS machines (
	id int NOT NULL AUTO_INCREMENT,
	-- The name of this machine.
	name varchar(255),
	-- The type of platform this machine is.
	platform_id int NOT NULL,
	-- The rack this machine belongs to.
	rack_id int NOT NULL,
	-- A description of this machine.
	description varchar(255),
	-- The asset tag associated with this machine.
	asset_tag varchar(255),
	-- The service tag associated with this machine.
	service_tag varchar(255),
	-- The deployment ticket associated with this machine.
	deployment_ticket varchar(255),
	PRIMARY KEY (id),
	FOREIGN KEY (platform_id) REFERENCES platforms (id) ON DELETE RESTRICT,
	FOREIGN KEY (rack_id) REFERENCES racks (id) ON DELETE RESTRICT,
	UNIQUE (name)
);
