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

CREATE TABLE IF NOT EXISTS nics (
	id int NOT NULL AUTO_INCREMENT,
	-- The name of this NIC.
	name varchar(255),
	-- The machine this NIC belongs to.
	machine_id int NOT NULL,
	-- The hostname belonging to this physical host.
	hostname_id int,
	-- The MAC address associated with this NIC.
	mac_address bigint unsigned,
	-- The switch this NIC is connected to.
	switch_id int NOT NULL,
	-- The switchport this NIC is connected to.
	switchport int NOT NULL,
	PRIMARY KEY (id),
	FOREIGN KEY (machine_id) REFERENCES machines (id) ON DELETE RESTRICT,
	FOREIGN KEY (hostname_id) REFERENCES hostnames (id) ON DELETE SET NULL,
	FOREIGN KEY (switch_id) REFERENCES switches (id) ON DELETE RESTRICT,
	-- Redundant with PRIMARY KEY (id), but needed by physical_hosts.
	UNIQUE (id, machine_id),
	UNIQUE (name, machine_id),
	UNIQUE (hostname_id),
	UNIQUE (mac_address)
);
