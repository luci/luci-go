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

CREATE TABLE IF NOT EXISTS kvms (
	id int NOT NULL AUTO_INCREMENT,
	-- The hostname belonging to this KVM.
	hostname_id int NOT NULL,
	-- The type of platform this KVM is.
	platform_id int NOT NULL,
	-- The rack this KVM belongs to.
	rack_id int NOT NULL,
	-- A description of this KVM.
	description varchar(255),
	-- The MAC address associated with this KVM.
	mac_address bigint unsigned,
	-- The state this KVM is in.
	state tinyint DEFAULT 0,
	PRIMARY KEY (id),
	FOREIGN KEY (hostname_id) REFERENCES hostnames (id) ON DELETE RESTRICT,
	FOREIGN KEY (platform_id) REFERENCES platforms (id) ON DELETE RESTRICT,
	FOREIGN KEY (rack_id) REFERENCES racks (id) ON DELETE CASCADE,
	UNIQUE (hostname_id),
	UNIQUE (mac_address)
);

ALTER TABLE racks ADD COLUMN kvm_id int;
ALTER TABLE racks ADD CONSTRAINT racks_ibfk_2 FOREIGN KEY (kvm_id) REFERENCES kvms (id) ON DELETE RESTRICT;
