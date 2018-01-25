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

CREATE TABLE IF NOT EXISTS vms (
	id int NOT NULL AUTO_INCREMENT,
	-- The hostname belonging to this VM.
	hostname_id int NOT NULL,
	-- The physical host this VM is running on.
	physical_host_id int NOT NULL,
	-- The operating system running on this VM.
	os_id int NOT NULL,
	-- A description of this VM.
	description varchar(255),
	-- The deployment ticket associated with this VM.
	deployment_ticket varchar(255),
	PRIMARY KEY (id),
	FOREIGN KEY (hostname_id) REFERENCES hostnames (id) ON DELETE RESTRICT,
	FOREIGN KEY (physical_host_id) REFERENCES physical_hosts (id) ON DELETE RESTRICT,
	FOREIGN KEY (os_id) REFERENCES oses (id) ON DELETE RESTRICT,
	UNIQUE (hostname_id)
);
