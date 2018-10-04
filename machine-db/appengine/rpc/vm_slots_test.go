// Copyright 2018 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package rpc

import (
	"context"
	"fmt"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"go.chromium.org/luci/machine-db/api/common/v1"
	"go.chromium.org/luci/machine-db/api/crimson/v1"
	"go.chromium.org/luci/machine-db/appengine/database"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestFindVMSlots(t *testing.T) {
	Convey("findVMSlots", t, func() {
		db, m, _ := sqlmock.New()
		defer db.Close()
		c := database.With(context.Background(), db)
		columns := []string{"h.name", "h.vlan_id", "ph.vm_slots - COUNT(v.physical_host_id)", "ph.virtual_datacenter", "m.state"}
		rows := sqlmock.NewRows(columns)

		Convey("query failed", func() {
			selectStmt := `
				^SELECT h.name, h.vlan_id, ph.vm_slots - COUNT\(v.physical_host_id\), ph.virtual_datacenter, m.state
				FROM \(physical_hosts ph, hostnames h, machines m\)
				LEFT JOIN vms v on v.physical_host_id = ph.id
				WHERE ph.hostname_id = h.id AND ph.vm_slots > 0 AND ph.machine_id = m.id
				GROUP BY h.name, h.vlan_id, ph.vm_slots, ph.virtual_datacenter, m.state
				HAVING ph.vm_slots > COUNT\(v.physical_host_id\)
				LIMIT 5$
			`
			req := &crimson.FindVMSlotsRequest{
				Slots: 5,
			}
			m.ExpectQuery(selectStmt).WithArgs().WillReturnError(fmt.Errorf("error"))
			hosts, err := findVMSlots(c, db, req)
			So(err, ShouldErrLike, "failed to fetch VM slots")
			So(hosts, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("empty", func() {
			selectStmt := `
				^SELECT h.name, h.vlan_id, ph.vm_slots - COUNT\(v.physical_host_id\), ph.virtual_datacenter, m.state
				FROM \(physical_hosts ph, hostnames h, machines m\)
				LEFT JOIN vms v on v.physical_host_id = ph.id
				WHERE ph.hostname_id = h.id AND ph.vm_slots > 0 AND ph.machine_id = m.id
				GROUP BY h.name, h.vlan_id, ph.vm_slots, ph.virtual_datacenter, m.state
				HAVING ph.vm_slots > COUNT\(v.physical_host_id\)
				LIMIT 5$
			`
			req := &crimson.FindVMSlotsRequest{
				Slots: 5,
			}
			m.ExpectQuery(selectStmt).WithArgs().WillReturnRows(rows)
			hosts, err := findVMSlots(c, db, req)
			So(err, ShouldBeNil)
			So(hosts, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("extra slots", func() {
			selectStmt := `
				^SELECT h.name, h.vlan_id, ph.vm_slots - COUNT\(v.physical_host_id\), ph.virtual_datacenter, m.state
				FROM \(physical_hosts ph, hostnames h, machines m\)
				LEFT JOIN vms v on v.physical_host_id = ph.id
				WHERE ph.hostname_id = h.id AND ph.vm_slots > 0 AND ph.machine_id = m.id
				GROUP BY h.name, h.vlan_id, ph.vm_slots, ph.virtual_datacenter, m.state
				HAVING ph.vm_slots > COUNT\(v.physical_host_id\)
				LIMIT 5$
			`
			req := &crimson.FindVMSlotsRequest{
				Slots: 5,
			}
			rows.AddRow("host-1", 1, 1, "vdc-1", common.State_SERVING)
			rows.AddRow("host-2", 1, 7, "vdc-1", common.State_TEST)
			rows.AddRow("host-3", 1, 3, "vdc-2", common.State_REPAIR)
			m.ExpectQuery(selectStmt).WithArgs().WillReturnRows(rows)
			hosts, err := findVMSlots(c, db, req)
			So(err, ShouldBeNil)
			So(hosts, ShouldResemble, []*crimson.PhysicalHost{
				{
					Name:              "host-1",
					Vlan:              1,
					VmSlots:           1,
					VirtualDatacenter: "vdc-1",
					State:             common.State_SERVING,
				},
				{
					Name:              "host-2",
					Vlan:              1,
					VmSlots:           7,
					VirtualDatacenter: "vdc-1",
					State:             common.State_TEST,
				},
			})
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("manufacturer", func() {
			selectStmt := `
				^SELECT h.name, h.vlan_id, ph.vm_slots - COUNT\(v.physical_host_id\), ph.virtual_datacenter, m.state
				FROM \(physical_hosts ph, hostnames h, machines m\)
				JOIN platforms pl ON m.platform_id = pl.id
				LEFT JOIN vms v on v.physical_host_id = ph.id
				WHERE ph.hostname_id = h.id AND ph.vm_slots > 0 AND ph.machine_id = m.id AND pl.manufacturer IN \(\?\)
				GROUP BY h.name, h.vlan_id, ph.vm_slots, ph.virtual_datacenter, m.state
				HAVING ph.vm_slots > COUNT\(v.physical_host_id\)
				LIMIT 5$
			`
			req := &crimson.FindVMSlotsRequest{
				Manufacturers: []string{"manufacturer"},
				Slots:         5,
			}
			rows.AddRow("host-1", 1, 2, "vdc-1", common.State_SERVING)
			rows.AddRow("host-2", 1, 3, "vdc-2", common.State_TEST)
			m.ExpectQuery(selectStmt).WithArgs().WillReturnRows(rows)
			hosts, err := findVMSlots(c, db, req)
			So(err, ShouldBeNil)
			So(hosts, ShouldResemble, []*crimson.PhysicalHost{
				{
					Name:              "host-1",
					Vlan:              1,
					VmSlots:           2,
					VirtualDatacenter: "vdc-1",
					State:             common.State_SERVING,
				},
				{
					Name:              "host-2",
					Vlan:              1,
					VmSlots:           3,
					VirtualDatacenter: "vdc-2",
					State:             common.State_TEST,
				},
			})
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("ok", func() {
			selectStmt := `
				^SELECT h.name, h.vlan_id, ph.vm_slots - COUNT\(v.physical_host_id\), ph.virtual_datacenter, m.state
				FROM \(physical_hosts ph, hostnames h, machines m\)
				LEFT JOIN vms v on v.physical_host_id = ph.id
				WHERE ph.hostname_id = h.id AND ph.vm_slots > 0 AND ph.machine_id = m.id
				GROUP BY h.name, h.vlan_id, ph.vm_slots, ph.virtual_datacenter, m.state
				HAVING ph.vm_slots > COUNT\(v.physical_host_id\)$
			`
			req := &crimson.FindVMSlotsRequest{}
			rows.AddRow("host-1", 1, 2, "vdc-1", common.State_SERVING)
			rows.AddRow("host-2", 1, 3, "vdc-2", common.State_TEST)
			m.ExpectQuery(selectStmt).WithArgs().WillReturnRows(rows)
			hosts, err := findVMSlots(c, db, req)
			So(err, ShouldBeNil)
			So(hosts, ShouldResemble, []*crimson.PhysicalHost{
				{
					Name:              "host-1",
					Vlan:              1,
					VmSlots:           2,
					VirtualDatacenter: "vdc-1",
					State:             common.State_SERVING,
				},
				{
					Name:              "host-2",
					Vlan:              1,
					VmSlots:           3,
					VirtualDatacenter: "vdc-2",
					State:             common.State_TEST,
				},
			})
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})
	})
}
