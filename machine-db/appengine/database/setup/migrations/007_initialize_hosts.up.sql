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

CREATE TABLE IF NOT EXISTS hostnames (
	id int NOT NULL AUTO_INCREMENT,
	-- The hostname.
	name varchar(255),
	-- The VLAN this hostname exists on.
	vlan_id int NOT NULL,
	PRIMARY KEY (id),
	FOREIGN KEY (vlan_id) REFERENCES vlans (id) ON DELETE RESTRICT,
	UNIQUE (name)
);

CREATE TABLE IF NOT EXISTS physical_hosts (
	id int NOT NULL AUTO_INCREMENT,
	-- The hostname belonging to this physical host.
	hostname_id int NOT NULL,
	-- The machine backing this physical host.
	machine_id int NOT NULL,
	-- The operating system running on this physical host.
	os_id int NOT NULL,
	-- The number of VMs which can be deployed on this physical host.
	vm_slots int unsigned NOT NULL,
	-- The virtual datacenter VMs deployed on this physical host belong to.
	virtual_datacenter varchar(255),
	-- A description of this physical host.
	description varchar(255),
	-- The deployment ticket associated with this physical host.
	deployment_ticket varchar(255),
	PRIMARY KEY (id),
	FOREIGN KEY (hostname_id) REFERENCES hostnames (id) ON DELETE CASCADE,
	FOREIGN KEY (machine_id) REFERENCES machines (id) ON DELETE RESTRICT,
	FOREIGN KEY (os_id) REFERENCES oses (id) ON DELETE RESTRICT,
	UNIQUE (hostname_id),
	UNIQUE (machine_id)
);
