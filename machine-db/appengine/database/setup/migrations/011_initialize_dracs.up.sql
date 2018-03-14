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

CREATE TABLE IF NOT EXISTS dracs (
	id int NOT NULL AUTO_INCREMENT,
	-- The hostname belonging to this DRAC.
	hostname_id int NOT NULL,
	-- The machine this DRAC belongs to.
	machine_id int NOT NULL,
	-- The MAC address associated with this DRAC.
	mac_address bigint unsigned,
	-- The switch this DRAC is connected to.
	switch_id int NOT NULL,
	-- The switchport this DRAC is connected to.
	switchport int NOT NULL,
	PRIMARY KEY (id),
	FOREIGN KEY (hostname_id) REFERENCES hostnames (id) ON DELETE CASCADE,
	FOREIGN KEY (machine_id) REFERENCES machines (id) ON DELETE RESTRICT,
	FOREIGN KEY (switch_id) REFERENCES switches (id) ON DELETE RESTRICT,
	UNIQUE (hostname_id),
	UNIQUE (machine_id),
	UNIQUE (mac_address)
);
