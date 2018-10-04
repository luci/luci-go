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

package model

import (
	"context"
	"fmt"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/VividCortex/mysqlerr"
	"github.com/go-sql-driver/mysql"

	"go.chromium.org/luci/machine-db/api/common/v1"
	"go.chromium.org/luci/machine-db/api/config/v1"
	"go.chromium.org/luci/machine-db/appengine/database"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestKVMs(t *testing.T) {
	Convey("fetch", t, func() {
		db, m, _ := sqlmock.New()
		defer db.Close()
		c := database.With(context.Background(), db)
		selectStmt := `
			^SELECT k.id, h.id, h.name, i.ipv4, k.description, k.mac_address, k.state, k.platform_id, k.rack_id
			FROM kvms k, hostnames h, ips i
			WHERE k.hostname_id = h.id AND i.hostname_id = h.id$`
		columns := []string{"k.id", "h.id", "h.name", "i.ipv4", "k.description", "k.mac_address", "k.state", "k.platform_id", "k.rack_id"}
		rows := sqlmock.NewRows(columns)
		table := &KVMsTable{}

		Convey("query failed", func() {
			m.ExpectQuery(selectStmt).WillReturnError(fmt.Errorf("error"))
			So(table.fetch(c), ShouldErrLike, "failed to select KVMs")
			So(table.current, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("empty", func() {
			m.ExpectQuery(selectStmt).WillReturnRows(rows)
			So(table.fetch(c), ShouldBeNil)
			So(table.current, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("ok", func() {
			rows.AddRow(1, 1, "kvm 1", 1, "description 1", 1, common.State_FREE, 1, 1)
			rows.AddRow(2, 2, "kvm 2", 2, "description 2", 2, common.State_SERVING, 2, 2)
			m.ExpectQuery(selectStmt).WillReturnRows(rows)
			So(table.fetch(c), ShouldBeNil)
			So(table.current, ShouldResemble, []*KVM{
				{
					KVM: config.KVM{
						Name:        "kvm 1",
						Description: "description 1",
						State:       common.State_FREE,
					},
					MacAddress: 1,
					IPv4:       1,
					PlatformId: 1,
					RackId:     1,
					Id:         1,
					HostnameId: 1,
				},
				{
					KVM: config.KVM{
						Name:        "kvm 2",
						Description: "description 2",
						State:       common.State_SERVING,
					},
					MacAddress: 2,
					IPv4:       2,
					PlatformId: 2,
					RackId:     2,
					Id:         2,
					HostnameId: 2,
				},
			})
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})
	})

	Convey("computeChanges", t, func() {
		c := context.Background()
		table := &KVMsTable{
			platforms: make(map[string]int64, 0),
			racks:     make(map[string]int64, 0),
		}

		Convey("empty", func() {
			table.computeChanges(c, nil)
			So(table.additions, ShouldBeEmpty)
			So(table.updates, ShouldBeEmpty)
			So(table.removals, ShouldBeEmpty)
		})

		Convey("platform id lookup failure", func() {
			dcs := []*config.Datacenter{
				{
					Name: "datacenter",
					Kvm: []*config.KVM{
						{
							Name:       "kvm",
							Ipv4:       "0.0.0.1",
							MacAddress: "00:00:00:00:00:01",
							Platform:   "platform",
							Rack:       "rack",
						},
					},
				},
			}
			table.racks[dcs[0].Kvm[0].Rack] = 1
			So(table.computeChanges(c, dcs), ShouldErrLike, "failed to determine platform ID")
			So(table.additions, ShouldBeEmpty)
			So(table.updates, ShouldBeEmpty)
			So(table.removals, ShouldBeEmpty)
		})

		Convey("rack id lookup failure", func() {
			dcs := []*config.Datacenter{
				{
					Name: "datacenter",
					Kvm: []*config.KVM{
						{
							Name:       "kvm",
							Ipv4:       "0.0.0.1",
							MacAddress: "00:00:00:00:00:01",
							Platform:   "platform",
							Rack:       "rack",
						},
					},
				},
			}
			table.platforms[dcs[0].Kvm[0].Platform] = 1
			So(table.computeChanges(c, dcs), ShouldErrLike, "failed to determine rack ID")
			So(table.additions, ShouldBeEmpty)
			So(table.updates, ShouldBeEmpty)
			So(table.removals, ShouldBeEmpty)
		})

		Convey("addition", func() {
			dcs := []*config.Datacenter{
				{
					Name: "datacenter 1",
					Kvm: []*config.KVM{
						{
							Name:        "kvm 1",
							Description: "description",
							Ipv4:        "0.0.0.1",
							MacAddress:  "00:00:00:00:00:01",
							Platform:    "platform 1",
							Rack:        "rack 1",
						},
						{
							Name:       "kvm 2",
							State:      common.State_FREE,
							Ipv4:       "0.0.0.2",
							MacAddress: "00:00:00:00:00:02",
							Platform:   "platform 2",
							Rack:       "rack 2",
						},
					},
				},
				{
					Name: "datacenter 2",
					Kvm: []*config.KVM{
						{
							Name:       "kvm 3",
							State:      common.State_SERVING,
							Ipv4:       "0.0.0.3",
							MacAddress: "00:00:00:00:00:03",
							Platform:   "platform 3",
							Rack:       "rack 3",
						},
					},
				},
			}
			table.platforms[dcs[0].Kvm[0].Platform] = 1
			table.platforms[dcs[0].Kvm[1].Platform] = 2
			table.platforms[dcs[1].Kvm[0].Platform] = 3
			table.racks[dcs[0].Kvm[0].Rack] = 1
			table.racks[dcs[0].Kvm[1].Rack] = 2
			table.racks[dcs[1].Kvm[0].Rack] = 3
			table.computeChanges(c, dcs)
			So(table.additions, ShouldResemble, []*KVM{
				{
					KVM: config.KVM{
						Name:        "kvm 1",
						Description: "description",
					},
					IPv4:       1,
					MacAddress: 1,
					PlatformId: table.platforms[dcs[0].Kvm[0].Platform],
					RackId:     table.racks[dcs[0].Kvm[0].Rack],
				},
				{
					KVM: config.KVM{
						Name:  "kvm 2",
						State: common.State_FREE,
					},
					IPv4:       2,
					MacAddress: 2,
					PlatformId: table.platforms[dcs[0].Kvm[1].Platform],
					RackId:     table.racks[dcs[0].Kvm[1].Rack],
				},
				{
					KVM: config.KVM{
						Name:  "kvm 3",
						State: common.State_SERVING,
					},
					IPv4:       3,
					MacAddress: 3,
					PlatformId: table.platforms[dcs[1].Kvm[0].Platform],
					RackId:     table.racks[dcs[1].Kvm[0].Rack],
				},
			})
			So(table.updates, ShouldBeEmpty)
			So(table.removals, ShouldBeEmpty)
		})

		Convey("update", func() {
			table.current = append(table.current, &KVM{
				KVM: config.KVM{
					Name:        "kvm 1",
					Description: "old description",
					State:       common.State_FREE,
				},
				IPv4:       1,
				MacAddress: 1,
				Id:         1,
				PlatformId: 1,
				RackId:     1,
			})
			table.current = append(table.current, &KVM{
				KVM: config.KVM{
					Name: "kvm 2",
				},
				IPv4:       2,
				MacAddress: 2,
				Id:         2,
				PlatformId: 2,
				RackId:     2,
			})
			dcs := []*config.Datacenter{
				{
					Name: "datacenter",
					Kvm: []*config.KVM{
						{
							Name:        table.current[0].Name,
							Description: "new description",
							Ipv4:        "0.0.0.1",
							MacAddress:  "00:00:00:00:00:0a",
							Platform:    "platform 1",
							Rack:        "rack 1",
						},
						{
							Name:        table.current[1].Name,
							Description: table.current[1].Description,
							State:       table.current[1].State,
							Ipv4:        "0.0.0.2",
							MacAddress:  "00:00:00:00:00:02",
							Platform:    "platform 2",
							Rack:        "rack 2",
						},
					},
				},
			}
			table.platforms[dcs[0].Kvm[0].Platform] = 1
			table.platforms[dcs[0].Kvm[1].Platform] = 2
			table.racks[dcs[0].Kvm[0].Rack] = 1
			table.racks[dcs[0].Kvm[1].Rack] = 2
			table.computeChanges(c, dcs)
			So(table.additions, ShouldBeEmpty)
			So(table.updates, ShouldResemble, []*KVM{
				{
					KVM: config.KVM{
						Name:        dcs[0].Kvm[0].Name,
						Description: dcs[0].Kvm[0].Description,
						State:       dcs[0].Kvm[0].State,
					},
					IPv4:       1,
					MacAddress: 10,
					Id:         table.current[0].Id,
					PlatformId: table.platforms[dcs[0].Kvm[0].Platform],
					RackId:     table.racks[dcs[0].Kvm[0].Rack],
				},
			})
			So(table.removals, ShouldBeEmpty)
		})

		Convey("removal", func() {
			table.current = append(table.current, &KVM{
				KVM: config.KVM{
					Name: "kvm",
				},
				Id: 1,
			})
			table.computeChanges(c, nil)
			So(table.additions, ShouldBeEmpty)
			So(table.updates, ShouldBeEmpty)
			So(table.removals, ShouldResemble, []*KVM{
				{
					KVM: config.KVM{
						Name: table.current[0].Name,
					},
					Id: table.current[0].Id,
				},
			})
		})
	})

	Convey("add", t, func() {
		db, m, _ := sqlmock.New()
		defer db.Close()
		c := database.With(context.Background(), db)
		insertNameStmt := `
			^INSERT INTO hostnames \(name, vlan_id\)
			VALUES \(\?, \(SELECT vlan_id FROM ips WHERE ipv4 = \? AND hostname_id IS NULL\)\)$
		`
		insertKVMStmt := `
			^INSERT INTO kvms \(hostname_id, description, mac_address, state, platform_id, rack_id\)
			VALUES \(\?, \?, \?, \?, \?, \?\)$
		`
		updateIPStmt := `
			^UPDATE ips
			SET hostname_id = \?
			WHERE ipv4 = \? AND hostname_id IS NULL$
		`
		table := &KVMsTable{}

		Convey("empty", func() {
			So(table.add(c), ShouldBeNil)
			So(table.additions, ShouldBeEmpty)
			So(table.current, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("prepare failed", func() {
			table.additions = append(table.additions, &KVM{
				KVM: config.KVM{
					Name: "kvm",
				},
				IPv4:       2,
				MacAddress: 3,
				PlatformId: 4,
				RackId:     5,
			})
			m.ExpectBegin()
			m.ExpectPrepare(insertKVMStmt).WillReturnError(fmt.Errorf("error"))
			m.ExpectRollback()
			So(table.add(c), ShouldErrLike, "failed to prepare statement")
			So(table.additions, ShouldHaveLength, 1)
			So(table.current, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("invalid IP", func() {
			table.additions = append(table.additions, &KVM{
				KVM: config.KVM{
					Name: "kvm",
				},
				IPv4:       2,
				MacAddress: 3,
				PlatformId: 4,
				RackId:     5,
			})
			m.ExpectBegin()
			m.ExpectPrepare(insertKVMStmt)
			m.ExpectExec(insertNameStmt).WithArgs(table.additions[0].Name, table.additions[0].IPv4).WillReturnError(&mysql.MySQLError{Number: mysqlerr.ER_BAD_NULL_ERROR, Message: "'vlan_id'"})
			m.ExpectRollback()
			So(table.add(c), ShouldErrLike, "ensure IPv4 address")
			So(table.additions, ShouldHaveLength, 1)
			So(table.current, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("duplicate host", func() {
			table.additions = append(table.additions, &KVM{
				KVM: config.KVM{
					Name: "kvm",
				},
				IPv4:       2,
				MacAddress: 3,
				PlatformId: 4,
				RackId:     5,
			})
			m.ExpectBegin()
			m.ExpectPrepare(insertKVMStmt)
			m.ExpectExec(insertNameStmt).WithArgs(table.additions[0].Name, table.additions[0].IPv4).WillReturnError(&mysql.MySQLError{Number: mysqlerr.ER_DUP_ENTRY, Message: "'name'"})
			m.ExpectRollback()
			So(table.add(c), ShouldErrLike, "duplicate hostname")
			So(table.additions, ShouldHaveLength, 1)
			So(table.current, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("exec failed", func() {
			table.additions = append(table.additions, &KVM{
				KVM: config.KVM{
					Name: "kvm",
				},
				IPv4:       2,
				MacAddress: 3,
				PlatformId: 4,
				RackId:     5,
			})
			m.ExpectBegin()
			m.ExpectPrepare(insertKVMStmt)
			m.ExpectExec(insertNameStmt).WithArgs(table.additions[0].Name, table.additions[0].IPv4).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectExec(updateIPStmt).WithArgs(1, table.additions[0].IPv4).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectExec(insertKVMStmt).WithArgs(1, table.additions[0].Description, table.additions[0].MacAddress, table.additions[0].State, table.additions[0].PlatformId, table.additions[0].RackId).WillReturnError(fmt.Errorf("error"))
			m.ExpectRollback()
			So(table.add(c), ShouldErrLike, "failed to add KVM")
			So(table.additions, ShouldHaveLength, 1)
			So(table.current, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("ok", func() {
			table.additions = append(table.additions, &KVM{
				KVM: config.KVM{
					Name: "kvm",
				},
				IPv4:       2,
				MacAddress: 3,
				PlatformId: 4,
				RackId:     5,
			})
			m.ExpectBegin()
			m.ExpectPrepare(insertKVMStmt)
			m.ExpectExec(insertNameStmt).WithArgs(table.additions[0].Name, table.additions[0].IPv4).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectExec(updateIPStmt).WithArgs(1, table.additions[0].IPv4).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectExec(insertKVMStmt).WithArgs(1, table.additions[0].Description, table.additions[0].MacAddress, table.additions[0].State, table.additions[0].PlatformId, table.additions[0].RackId).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectCommit()
			So(table.add(c), ShouldBeNil)
			So(table.additions, ShouldBeEmpty)
			So(table.current, ShouldHaveLength, 1)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})
	})

	Convey("remove", t, func() {
		db, m, _ := sqlmock.New()
		defer func() { So(m.ExpectationsWereMet(), ShouldBeNil) }()
		defer db.Close()
		c := database.With(context.Background(), db)
		deleteStmt := `^DELETE FROM hostnames WHERE id = \?$`
		table := &KVMsTable{}

		Convey("empty", func() {
			So(table.remove(c), ShouldBeNil)
			So(table.removals, ShouldBeEmpty)
			So(table.current, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("prepare failed", func() {
			table.removals = append(table.removals, &KVM{
				KVM: config.KVM{
					Name: "kvm",
				},
				Id:         1,
				HostnameId: 2,
			})
			table.current = append(table.current, &KVM{
				KVM: config.KVM{
					Name: table.removals[0].Name,
				},
				Id:         table.removals[0].Id,
				HostnameId: table.removals[0].HostnameId,
			})
			m.ExpectPrepare(deleteStmt).WillReturnError(fmt.Errorf("error"))
			So(table.remove(c), ShouldErrLike, "failed to prepare statement")
			So(table.removals, ShouldHaveLength, 1)
			So(table.current, ShouldHaveLength, 1)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("exec failed", func() {
			table.removals = append(table.removals, &KVM{
				KVM: config.KVM{
					Name: "kvm",
				},
				Id:         1,
				HostnameId: 2,
			})
			table.current = append(table.current, &KVM{
				KVM: config.KVM{
					Name: table.removals[0].Name,
				},
				Id:         table.removals[0].Id,
				HostnameId: table.removals[0].HostnameId,
			})
			m.ExpectPrepare(deleteStmt)
			m.ExpectExec(deleteStmt).WithArgs(table.removals[0].HostnameId).WillReturnError(fmt.Errorf("error"))
			So(table.remove(c), ShouldErrLike, "failed to remove KVM")
			So(table.removals, ShouldHaveLength, 1)
			So(table.current, ShouldHaveLength, 1)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("ok", func() {
			table.removals = append(table.removals, &KVM{
				KVM: config.KVM{
					Name: "kvm",
				},
				Id:         1,
				HostnameId: 2,
			})
			table.current = append(table.current, &KVM{
				KVM: config.KVM{
					Name: table.removals[0].Name,
				},
				Id:         table.removals[0].Id,
				HostnameId: table.removals[0].HostnameId,
			})
			m.ExpectPrepare(deleteStmt)
			m.ExpectExec(deleteStmt).WithArgs(table.removals[0].HostnameId).WillReturnResult(sqlmock.NewResult(1, 1))
			So(table.remove(c), ShouldBeNil)
			So(table.removals, ShouldBeEmpty)
			So(table.current, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})
	})

	Convey("update", t, func() {
		db, m, _ := sqlmock.New()
		defer db.Close()
		c := database.With(context.Background(), db)
		updateStmt := `
			^UPDATE kvms
			SET description = \?, mac_address = \?, state = \?, platform_id = \?, rack_id = \? WHERE id = \?$
		`
		table := &KVMsTable{}

		Convey("empty", func() {
			So(table.update(c), ShouldBeNil)
			So(table.updates, ShouldBeEmpty)
			So(table.current, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("prepare failed", func() {
			table.updates = append(table.updates, &KVM{
				KVM: config.KVM{
					Name:        "kvm",
					Description: "new description",
					State:       common.State_SERVING,
					MacAddress:  "00:00:00:00:00:02",
				},
				Id:         1,
				PlatformId: 2,
				RackId:     2,
			})
			table.current = append(table.current, &KVM{
				KVM: config.KVM{
					Name:        table.updates[0].Name,
					Description: "old description",
					State:       common.State_FREE,
					MacAddress:  "00:00:00:00:00:01",
				},
				Id:         table.updates[0].Id,
				PlatformId: 1,
				RackId:     1,
			})
			m.ExpectPrepare(updateStmt).WillReturnError(fmt.Errorf("error"))
			So(table.update(c), ShouldErrLike, "failed to prepare statement")
			So(table.updates, ShouldHaveLength, 1)
			So(table.current, ShouldHaveLength, 1)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("exec failed", func() {
			table.updates = append(table.updates, &KVM{
				KVM: config.KVM{
					Name:        "kvm",
					Description: "new description",
					State:       common.State_SERVING,
					MacAddress:  "00:00:00:00:00:02",
				},
				Id:         1,
				PlatformId: 2,
				RackId:     2,
			})
			table.current = append(table.current, &KVM{
				KVM: config.KVM{
					Name:        table.updates[0].Name,
					Description: "old description",
					State:       common.State_FREE,
					MacAddress:  "00:00:00:00:00:01",
				},
				Id:         table.updates[0].Id,
				PlatformId: 1,
				RackId:     1,
			})
			m.ExpectPrepare(updateStmt)
			m.ExpectExec(updateStmt).WithArgs(table.updates[0].Description, table.updates[0].MacAddress, table.updates[0].State, table.updates[0].PlatformId, table.updates[0].RackId, table.updates[0].Id).WillReturnError(fmt.Errorf("error"))
			So(table.update(c), ShouldErrLike, "failed to update KVM")
			So(table.updates, ShouldHaveLength, 1)
			So(table.current, ShouldHaveLength, 1)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("ok", func() {
			table.updates = append(table.updates, &KVM{
				KVM: config.KVM{
					Name:        "kvm",
					Description: "new description",
					State:       common.State_SERVING,
				},
				MacAddress: 2,
				Id:         1,
				PlatformId: 2,
				RackId:     2,
			})
			table.current = append(table.current, &KVM{
				KVM: config.KVM{
					Name:        table.updates[0].Name,
					Description: "old description",
					State:       common.State_FREE,
				},
				MacAddress: 2,
				Id:         table.updates[0].Id,
				PlatformId: 1,
				RackId:     1,
			})
			m.ExpectPrepare(updateStmt)
			m.ExpectExec(updateStmt).WithArgs(table.updates[0].Description, table.updates[0].MacAddress, table.updates[0].State, table.updates[0].PlatformId, table.updates[0].RackId, table.updates[0].Id).WillReturnResult(sqlmock.NewResult(1, 1))
			So(table.update(c), ShouldBeNil)
			So(table.updates, ShouldBeEmpty)
			So(table.current, ShouldHaveLength, 1)
			So(table.current[0].Description, ShouldEqual, "new description")
			So(table.current[0].State, ShouldEqual, common.State_SERVING)
			So(table.current[0].MacAddress, ShouldEqual, 2)
			So(table.current[0].PlatformId, ShouldEqual, 2)
			So(table.current[0].RackId, ShouldEqual, 2)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})
	})
}
