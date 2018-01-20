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

CREATE TABLE IF NOT EXISTS hosts (
	id int NOT NULL AUTO_INCREMENT,
	-- The name of this host.
	name varchar(255),
	-- The VLAN this host belongs to.
	vlan_id int NOT NULL,
	-- The machine backing this host.
	machine_id int NOT NULL,
	-- The operating system running on this host.
	os_id int NOT NULL,
	-- The number of VMs which can be deployed on this host.
	vm_slots int unsigned NOT NULL,
	-- A description of this host.
	description varchar(255),
	-- The deployment ticket associated with this host.
	deployment_ticket varchar(255),
	PRIMARY KEY (id),
	FOREIGN KEY (vlan_id) REFERENCES vlans (id) ON DELETE RESTRICT,
	FOREIGN KEY (machine_id) REFERENCES machines (id) ON DELETE RESTRICT,
	FOREIGN KEY (os_id) REFERENCES oses (id) ON DELETE RESTRICT,
	UNIQUE (name, vlan_id),
	UNIQUE (machine_id)
);
