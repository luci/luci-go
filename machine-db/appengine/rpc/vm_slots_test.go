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
	"fmt"
	"testing"

	"golang.org/x/net/context"

	"github.com/DATA-DOG/go-sqlmock"

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
		selectStmt := `
			^SELECT h.name, h.vlan_id, p.vm_slots - COUNT\(v.physical_host_id\)
			FROM \(physical_hosts p, hostnames h\)
			LEFT JOIN vms v on v.physical_host_id = p.id
			WHERE p.hostname_id = h.id AND p.vm_slots > 0
			GROUP BY h.name, h.vlan_id, p.vm_slots
			HAVING p.vm_slots > COUNT\(v.physical_host_id\)
			LIMIT \?$
		`
		columns := []string{"h.name", "h.vlan_id", "p.vm_slots - COUNT(v.physical_host_id)"}
		rows := sqlmock.NewRows(columns)

		Convey("unspecified slots", func() {
			req := &crimson.FindVMSlotsRequest{}
			hosts, err := findVMSlots(c, db, req)
			So(err, ShouldErrLike, "slots is required and must be positive")
			So(hosts, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("negative slots", func() {
			req := &crimson.FindVMSlotsRequest{
				Slots: -1,
			}
			hosts, err := findVMSlots(c, db, req)
			So(err, ShouldErrLike, "slots is required and must be positive")
			So(hosts, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("excessive slots", func() {
			req := &crimson.FindVMSlotsRequest{
				Slots: maxVMSlots + 1,
			}
			hosts, err := findVMSlots(c, db, req)
			So(err, ShouldErrLike, "slots must not exceed")
			So(hosts, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("query failed", func() {
			req := &crimson.FindVMSlotsRequest{
				Slots: 5,
			}
			m.ExpectQuery(selectStmt).WithArgs(req.Slots).WillReturnError(fmt.Errorf("error"))
			hosts, err := findVMSlots(c, db, req)
			So(err, ShouldErrLike, "Internal server error")
			So(hosts, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("empty", func() {
			req := &crimson.FindVMSlotsRequest{
				Slots: 5,
			}
			m.ExpectQuery(selectStmt).WithArgs(req.Slots).WillReturnRows(rows)
			hosts, err := findVMSlots(c, db, req)
			So(err, ShouldBeNil)
			So(hosts, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("extra slots", func() {
			req := &crimson.FindVMSlotsRequest{
				Slots: 5,
			}
			rows.AddRow("host-1", 1, 1)
			rows.AddRow("host-2", 1, 7)
			rows.AddRow("host-3", 1, 3)
			m.ExpectQuery(selectStmt).WithArgs(req.Slots).WillReturnRows(rows)
			hosts, err := findVMSlots(c, db, req)
			So(err, ShouldBeNil)
			So(hosts, ShouldResemble, []*crimson.PhysicalHost{
				{
					Name:    "host-1",
					Vlan:    1,
					VmSlots: 1,
				},
				{
					Name:    "host-2",
					Vlan:    1,
					VmSlots: 7,
				},
			})
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("ok", func() {
			req := &crimson.FindVMSlotsRequest{
				Slots: 5,
			}
			rows.AddRow("host-1", 1, 2)
			rows.AddRow("host-2", 1, 3)
			m.ExpectQuery(selectStmt).WithArgs(req.Slots).WillReturnRows(rows)
			hosts, err := findVMSlots(c, db, req)
			So(err, ShouldBeNil)
			So(hosts, ShouldResemble, []*crimson.PhysicalHost{
				{
					Name:    "host-1",
					Vlan:    1,
					VmSlots: 2,
				},
				{
					Name:    "host-2",
					Vlan:    1,
					VmSlots: 3,
				},
			})
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})
	})
}
