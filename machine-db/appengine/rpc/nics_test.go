// Copyright 2017 The LUCI Authors.
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
	"database/sql"
	"fmt"
	"testing"

	"google.golang.org/genproto/protobuf/field_mask"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/VividCortex/mysqlerr"
	"github.com/go-sql-driver/mysql"

	states "go.chromium.org/luci/machine-db/api/common/v1"
	"go.chromium.org/luci/machine-db/api/crimson/v1"
	"go.chromium.org/luci/machine-db/appengine/database"
	"go.chromium.org/luci/machine-db/common"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestCreateNIC(t *testing.T) {
	Convey("createNIC", t, func() {
		db, m, _ := sqlmock.New()
		defer db.Close()
		c := database.With(context.Background(), db)
		insertNameStmt := `
			^INSERT INTO hostnames \(name, vlan_id\)
			VALUES \(\?, \(SELECT vlan_id FROM ips WHERE ipv4 = \? AND hostname_id IS NULL\)\)$
		`
		insertNICStmt := `
			^INSERT INTO nics \(name, machine_id, mac_address, switch_id, switchport, hostname_id\)
			VALUES \(\?, \(SELECT id FROM machines WHERE name = \?\), \?, \(SELECT id FROM switches WHERE name = \?\), \?, \?\)$
		`
		updateIPStmt := `
			^UPDATE ips
			SET hostname_id = \?
			WHERE ipv4 = \? AND hostname_id IS NULL$
		`

		Convey("begin failed", func() {
			nic := &crimson.NIC{
				Name:       "eth0",
				Machine:    "machine",
				MacAddress: "ff:ff:ff:ff:ff:ff",
				Switch:     "switch",
				Switchport: 1,
			}
			m.ExpectBegin().WillReturnError(fmt.Errorf("error"))
			m.ExpectRollback()
			So(createNIC(c, nic), ShouldErrLike, "failed to begin transaction")
		})

		Convey("query failed", func() {
			nic := &crimson.NIC{
				Name:       "eth0",
				Machine:    "machine",
				MacAddress: "ff:ff:ff:ff:ff:ff",
				Switch:     "switch",
				Switchport: 1,
			}
			m.ExpectBegin()
			m.ExpectExec(insertNICStmt).WithArgs(nic.Name, nic.Machine, common.MaxMAC48, nic.Switch, nic.Switchport, sql.NullInt64{}).WillReturnError(fmt.Errorf("error"))
			m.ExpectRollback()
			So(createNIC(c, nic), ShouldErrLike, "failed to create NIC")
		})

		Convey("duplicate NIC/machine", func() {
			nic := &crimson.NIC{
				Name:       "eth0",
				Machine:    "machine",
				MacAddress: "ff:ff:ff:ff:ff:ff",
				Switch:     "switch",
				Switchport: 1,
			}
			m.ExpectBegin()
			m.ExpectExec(insertNICStmt).WithArgs(nic.Name, nic.Machine, common.MaxMAC48, nic.Switch, nic.Switchport, sql.NullInt64{}).WillReturnError(&mysql.MySQLError{Number: mysqlerr.ER_DUP_ENTRY, Message: "'name'"})
			m.ExpectRollback()
			So(createNIC(c, nic), ShouldErrLike, "duplicate NIC")
		})

		Convey("invalid machine", func() {
			nic := &crimson.NIC{
				Name:       "eth0",
				Machine:    "machine",
				MacAddress: "ff:ff:ff:ff:ff:ff",
				Switch:     "switch",
				Switchport: 1,
			}
			m.ExpectBegin()
			m.ExpectExec(insertNICStmt).WithArgs(nic.Name, nic.Machine, common.MaxMAC48, nic.Switch, nic.Switchport, sql.NullInt64{}).WillReturnError(&mysql.MySQLError{Number: mysqlerr.ER_BAD_NULL_ERROR, Message: "'machine_id' is null"})
			m.ExpectRollback()
			So(createNIC(c, nic), ShouldErrLike, "does not exist")
		})

		Convey("duplicate MAC address", func() {
			nic := &crimson.NIC{
				Name:       "eth0",
				Machine:    "machine",
				MacAddress: "ff:ff:ff:ff:ff:ff",
				Switch:     "switch",
				Switchport: 1,
			}
			m.ExpectBegin()
			m.ExpectExec(insertNICStmt).WithArgs(nic.Name, nic.Machine, common.MaxMAC48, nic.Switch, nic.Switchport, sql.NullInt64{}).WillReturnError(&mysql.MySQLError{Number: mysqlerr.ER_DUP_ENTRY, Message: "'mac_address'"})
			m.ExpectRollback()
			So(createNIC(c, nic), ShouldErrLike, "duplicate MAC address")
		})

		Convey("invalid switch", func() {
			nic := &crimson.NIC{
				Name:       "eth0",
				Machine:    "machine",
				MacAddress: "ff:ff:ff:ff:ff:ff",
				Switch:     "switch",
				Switchport: 1,
			}
			m.ExpectBegin()
			m.ExpectExec(insertNICStmt).WithArgs(nic.Name, nic.Machine, common.MaxMAC48, nic.Switch, nic.Switchport, sql.NullInt64{}).WillReturnError(&mysql.MySQLError{Number: mysqlerr.ER_BAD_NULL_ERROR, Message: "'switch_id' is null"})
			m.ExpectRollback()
			So(createNIC(c, nic), ShouldErrLike, "does not exist")
		})

		Convey("unexpected invalid", func() {
			nic := &crimson.NIC{
				Name:       "eth0",
				Machine:    "machine",
				MacAddress: "ff:ff:ff:ff:ff:ff",
				Switch:     "switch",
				Switchport: 1,
			}
			m.ExpectBegin()
			m.ExpectExec(insertNICStmt).WithArgs(nic.Name, nic.Machine, common.MaxMAC48, nic.Switch, nic.Switchport, sql.NullInt64{}).WillReturnError(&mysql.MySQLError{Number: mysqlerr.ER_BAD_NULL_ERROR, Message: "error"})
			m.ExpectRollback()
			So(createNIC(c, nic), ShouldErrLike, "failed to create NIC")
		})

		Convey("unexpected error", func() {
			nic := &crimson.NIC{
				Name:       "eth0",
				Machine:    "machine",
				MacAddress: "ff:ff:ff:ff:ff:ff",
				Switch:     "switch",
				Switchport: 1,
			}
			m.ExpectBegin()
			m.ExpectExec(insertNICStmt).WithArgs(nic.Name, nic.Machine, common.MaxMAC48, nic.Switch, nic.Switchport, sql.NullInt64{}).WillReturnError(&mysql.MySQLError{Number: mysqlerr.ER_NO, Message: "name machine_id switch_id"})
			m.ExpectRollback()
			So(createNIC(c, nic), ShouldErrLike, "failed to create NIC")
		})

		Convey("commit failed", func() {
			nic := &crimson.NIC{
				Name:       "eth0",
				Machine:    "machine",
				MacAddress: "ff:ff:ff:ff:ff:ff",
				Switch:     "switch",
				Switchport: 1,
			}
			m.ExpectBegin()
			m.ExpectExec(insertNICStmt).WithArgs(nic.Name, nic.Machine, common.MaxMAC48, nic.Switch, nic.Switchport, sql.NullInt64{}).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectCommit().WillReturnError(fmt.Errorf("error"))
			m.ExpectRollback()
			So(createNIC(c, nic), ShouldErrLike, "failed to commit transaction")
		})

		Convey("ok", func() {
			nic := &crimson.NIC{
				Name:       "eth0",
				Machine:    "machine",
				MacAddress: "ff:ff:ff:ff:ff:ff",
				Switch:     "switch",
				Switchport: 1,
				Hostname:   "hostname",
				Ipv4:       "127.0.0.1",
			}
			m.ExpectBegin()
			m.ExpectExec(insertNameStmt).WithArgs(nic.Hostname, 2130706433).WillReturnResult(sqlmock.NewResult(10, 1))
			m.ExpectExec(updateIPStmt).WithArgs(10, 2130706433).WillReturnResult(sqlmock.NewResult(100, 1))
			m.ExpectExec(insertNICStmt).WithArgs(nic.Name, nic.Machine, common.MaxMAC48, nic.Switch, nic.Switchport, sql.NullInt64{Int64: 10, Valid: true}).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectCommit()
			So(createNIC(c, nic), ShouldBeNil)
		})
	})
}

func TestDeleteNIC(t *testing.T) {
	Convey("deleteNIC", t, func() {
		db, m, _ := sqlmock.New()
		defer db.Close()
		c := database.With(context.Background(), db)
		selectHostStmt := `
			^SELECT hp.name, hp.vlan_id, m.name, n.name, n.mac_address, o.name, h.vm_slots, h.virtual_datacenter, h.description, h.deployment_ticket, i.ipv4, m.state
			FROM \(physical_hosts h, hostnames hp, machines m, nics n, oses o, ips i\)
			WHERE n.hostname_id = hp.id AND h.machine_id = m.id AND h.nic_id = n.id AND h.os_id = o.id AND i.hostname_id = hp.id AND m.name IN \(\?\)$
		`
		selectNameStmt := `
			^SELECT h.id FROM nics n, machines m, hostnames h
			WHERE n.machine_id = m.id AND n.hostname_id = h.id AND n.name = \? AND m.name = \?$
		`
		deleteNameStmt := `
			^DELETE FROM hostnames
			WHERE id = \?$
		`
		deleteNICStmt := `
			^DELETE FROM nics WHERE name = \? AND machine_id = \(SELECT id FROM machines WHERE name = \?\)$
		`
		hostColumns := []string{"hp.name", "hp.vlan_id", "m.name", "n.name", "n.mac_address", "o.name", "h.vm_slots", "h.virtual_datacenter", "h.description", "h.deployment_ticket", "i.ipv4", "m.state"}
		hostRows := sqlmock.NewRows(hostColumns)
		nameColumns := []string{"h.id"}
		nameRows := sqlmock.NewRows(nameColumns)

		Convey("begin failed", func() {
			m.ExpectBegin().WillReturnError(fmt.Errorf("error"))
			m.ExpectRollback()
			So(deleteNIC(c, "eth0", "machine"), ShouldErrLike, "failed to begin transaction")
		})

		Convey("host failed", func() {
			m.ExpectBegin()
			m.ExpectQuery(selectHostStmt).WithArgs("machine").WillReturnError(fmt.Errorf("error"))
			m.ExpectRollback()
			So(deleteNIC(c, "eth0", "machine"), ShouldErrLike, "failed to fetch associated physical host")
		})

		Convey("host exists", func() {
			hostRows.AddRow("name", 1, "machine", "eth0", 1, "os", 0, "", "", "", 2130706433, states.State_SERVING)
			m.ExpectBegin()
			m.ExpectQuery(selectHostStmt).WithArgs("machine").WillReturnRows(hostRows)
			m.ExpectRollback()
			So(deleteNIC(c, "eth0", "machine"), ShouldErrLike, "delete entities referencing this NIC first")
		})

		Convey("select name failed", func() {
			m.ExpectBegin()
			m.ExpectQuery(selectHostStmt).WithArgs("machine").WillReturnRows(hostRows)
			m.ExpectQuery(selectNameStmt).WithArgs("eth0", "machine").WillReturnError(fmt.Errorf("error"))
			m.ExpectRollback()
			So(deleteNIC(c, "eth0", "machine"), ShouldErrLike, "failed to fetch associated hostname")
		})

		Convey("delete name failed", func() {
			nameRows.AddRow("1")
			m.ExpectBegin()
			m.ExpectQuery(selectHostStmt).WithArgs("machine").WillReturnRows(hostRows)
			m.ExpectQuery(selectNameStmt).WithArgs("eth0", "machine").WillReturnRows(nameRows)
			m.ExpectExec(deleteNameStmt).WithArgs(int64(1)).WillReturnError(fmt.Errorf("error"))
			m.ExpectRollback()
			So(deleteNIC(c, "eth0", "machine"), ShouldErrLike, "failed to delete associated hostname")
		})

		Convey("delete failed", func() {
			nameRows.AddRow("1")
			m.ExpectBegin()
			m.ExpectQuery(selectHostStmt).WithArgs("machine").WillReturnRows(hostRows)
			m.ExpectQuery(selectNameStmt).WithArgs("eth0", "machine").WillReturnRows(nameRows)
			m.ExpectExec(deleteNameStmt).WithArgs(int64(1)).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectExec(deleteNICStmt).WithArgs("eth0", "machine").WillReturnError(fmt.Errorf("error"))
			m.ExpectRollback()
			So(deleteNIC(c, "eth0", "machine"), ShouldErrLike, "failed to delete NIC")
		})

		Convey("delete ok", func() {
			nameRows.AddRow("1")
			m.ExpectBegin()
			m.ExpectQuery(selectHostStmt).WithArgs("machine").WillReturnRows(hostRows)
			m.ExpectQuery(selectNameStmt).WithArgs("eth0", "machine").WillReturnRows(nameRows)
			m.ExpectExec(deleteNameStmt).WithArgs(int64(1)).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectExec(deleteNICStmt).WithArgs("eth0", "machine").WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectCommit()
			So(deleteNIC(c, "eth0", "machine"), ShouldBeNil)
		})

		Convey("no matching rows", func() {
			m.ExpectBegin()
			m.ExpectQuery(selectHostStmt).WithArgs("machine").WillReturnRows(hostRows)
			m.ExpectQuery(selectNameStmt).WithArgs("eth0", "machine").WillReturnRows(nameRows)
			m.ExpectExec(deleteNICStmt).WithArgs("eth0", "machine").WillReturnResult(sqlmock.NewResult(1, 0))
			m.ExpectRollback()
			So(deleteNIC(c, "eth0", "machine"), ShouldErrLike, "does not exist")
		})

		Convey("commit failed", func() {
			m.ExpectBegin()
			m.ExpectQuery(selectHostStmt).WithArgs("machine").WillReturnRows(hostRows)
			m.ExpectQuery(selectNameStmt).WithArgs("eth0", "machine").WillReturnRows(nameRows)
			m.ExpectExec(deleteNICStmt).WithArgs("eth0", "machine").WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectCommit().WillReturnError(fmt.Errorf("error"))
			m.ExpectRollback()
			So(deleteNIC(c, "eth0", "machine"), ShouldErrLike, "failed to commit transaction")
		})

		Convey("ok", func() {
			m.ExpectBegin()
			m.ExpectQuery(selectHostStmt).WithArgs("machine").WillReturnRows(hostRows)
			m.ExpectQuery(selectNameStmt).WithArgs("eth0", "machine").WillReturnRows(nameRows)
			m.ExpectExec(deleteNICStmt).WithArgs("eth0", "machine").WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectCommit()
			So(deleteNIC(c, "eth0", "machine"), ShouldBeNil)
		})
	})
}

func TestListNICs(t *testing.T) {
	Convey("listNICs", t, func() {
		db, m, _ := sqlmock.New()
		defer db.Close()
		c := database.With(context.Background(), db)
		columns := []string{"n.name", "m.name", "n.mac_address", "s.name", "s.switchport"}
		rows := sqlmock.NewRows(columns)

		Convey("invalid MAC-48 address", func() {
			req := &crimson.ListNICsRequest{
				Names:        []string{"eth0"},
				Machines:     []string{"machine"},
				MacAddresses: []string{"01:02:03:04:05:06", "01:02:03:04:05:06:07:08"},
			}
			nics, err := listNICs(c, db, req)
			So(err, ShouldErrLike, "invalid MAC-48 address")
			So(nics, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("query failed", func() {
			selectStmt := `
				^SELECT n.name, m.name, n.mac_address, s.name, n.switchport
				FROM nics n, machines m, switches s
				WHERE n.machine_id = m.id AND n.switch_id = s.id AND n.name IN \(\?\) AND m.name IN \(\?\)$
			`
			req := &crimson.ListNICsRequest{
				Names:    []string{"eth0"},
				Machines: []string{"machine"},
			}
			m.ExpectQuery(selectStmt).WithArgs(req.Names[0], req.Machines[0]).WillReturnError(fmt.Errorf("error"))
			nics, err := listNICs(c, db, req)
			So(err, ShouldErrLike, "failed to fetch NICs")
			So(nics, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("no names", func() {
			selectStmt := `
				^SELECT n.name, m.name, n.mac_address, s.name, n.switchport
				FROM nics n, machines m, switches s
				WHERE n.machine_id = m.id AND n.switch_id = s.id AND m.name IN \(\?\)$
			`
			req := &crimson.ListNICsRequest{
				Machines: []string{"machine"},
			}
			m.ExpectQuery(selectStmt).WithArgs(req.Machines[0]).WillReturnRows(rows)
			nics, err := listNICs(c, db, req)
			So(err, ShouldBeNil)
			So(nics, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("no machines", func() {
			selectStmt := `
				^SELECT n.name, m.name, n.mac_address, s.name, n.switchport
				FROM nics n, machines m, switches s
				WHERE n.machine_id = m.id AND n.switch_id = s.id AND n.name IN \(\?\)$
			`
			req := &crimson.ListNICsRequest{
				Names: []string{"eth0"},
			}
			m.ExpectQuery(selectStmt).WithArgs(req.Names[0]).WillReturnRows(rows)
			nics, err := listNICs(c, db, req)
			So(err, ShouldBeNil)
			So(nics, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("empty", func() {
			selectStmt := `
				^SELECT n.name, m.name, n.mac_address, s.name, n.switchport
				FROM nics n, machines m, switches s
				WHERE n.machine_id = m.id AND n.switch_id = s.id AND n.name IN \(\?\) AND m.name IN \(\?\)$
			`
			req := &crimson.ListNICsRequest{
				Names:    []string{"eth0"},
				Machines: []string{"machine"},
			}
			m.ExpectQuery(selectStmt).WithArgs(req.Names[0], req.Machines[0]).WillReturnRows(rows)
			nics, err := listNICs(c, db, req)
			So(err, ShouldBeNil)
			So(nics, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("non-empty", func() {
			selectStmt := `
				^SELECT n.name, m.name, n.mac_address, s.name, n.switchport
				FROM nics n, machines m, switches s
				WHERE n.machine_id = m.id AND n.switch_id = s.id AND n.name IN \(\?,\?\) AND m.name IN \(\?\)$
			`
			req := &crimson.ListNICsRequest{
				Names:    []string{"eth0", "eth1"},
				Machines: []string{"machine"},
			}
			rows.AddRow(req.Names[0], req.Machines[0], 1, "switch", 1)
			rows.AddRow(req.Names[1], req.Machines[0], common.MaxMAC48, "switch", 2)
			m.ExpectQuery(selectStmt).WithArgs(req.Names[0], req.Names[1], req.Machines[0]).WillReturnRows(rows)
			nics, err := listNICs(c, db, req)
			So(err, ShouldBeNil)
			So(nics, ShouldResemble, []*crimson.NIC{
				{
					Name:       req.Names[0],
					Machine:    req.Machines[0],
					MacAddress: "00:00:00:00:00:01",
					Switch:     "switch",
					Switchport: 1,
				},
				{
					Name:       req.Names[1],
					Machine:    req.Machines[0],
					MacAddress: "ff:ff:ff:ff:ff:ff",
					Switch:     "switch",
					Switchport: 2,
				},
			})
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("ok", func() {
			selectStmt := `
				^SELECT n.name, m.name, n.mac_address, s.name, n.switchport
				FROM nics n, machines m, switches s
				WHERE n.machine_id = m.id AND n.switch_id = s.id$
			`
			req := &crimson.ListNICsRequest{}
			rows.AddRow("eth0", "machine", 0, "switch", 0)
			rows.AddRow("eth1", "machine", 1, "switch", 1)
			m.ExpectQuery(selectStmt).WillReturnRows(rows)
			nics, err := listNICs(c, db, req)
			So(err, ShouldBeNil)
			So(nics, ShouldResemble, []*crimson.NIC{
				{
					Name:       "eth0",
					Machine:    "machine",
					MacAddress: "00:00:00:00:00:00",
					Switch:     "switch",
					Switchport: 0,
				},
				{
					Name:       "eth1",
					Machine:    "machine",
					MacAddress: "00:00:00:00:00:01",
					Switch:     "switch",
					Switchport: 1,
				},
			})
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})
	})
}

func TestUpdateNIC(t *testing.T) {
	Convey("updateNIC", t, func() {
		db, m, _ := sqlmock.New()
		defer db.Close()
		c := database.With(context.Background(), db)
		selectStmt := `
			^SELECT n.name, m.name, n.mac_address, s.name, n.switchport
			FROM nics n, machines m, switches s
			WHERE n.machine_id = m.id AND n.switch_id = s.id AND n.name IN \(\?\) AND m.name IN \(\?\)$
		`
		columns := []string{"n.name", "m.machine", "n.mac_address", "s.name", "s.switchport"}
		rows := sqlmock.NewRows(columns)

		Convey("update MAC address", func() {
			updateStmt := `
				^UPDATE nics
				SET mac_address = \?
				WHERE name = \? AND machine_id = \(SELECT id FROM machines WHERE name = \?\)$
			`
			nic := &crimson.NIC{
				Name:       "eth0",
				Machine:    "machine",
				MacAddress: "ff:ff:ff:ff:ff:ff",
				Switch:     "switch",
				Switchport: 1,
			}
			mask := &field_mask.FieldMask{
				Paths: []string{
					"mac_address",
				},
			}
			rows.AddRow(nic.Name, nic.Machine, common.MaxMAC48, nic.Switch, nic.Switchport)
			m.ExpectBegin()
			m.ExpectExec(updateStmt).WithArgs(common.MaxMAC48, nic.Name, nic.Machine).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectQuery(selectStmt).WillReturnRows(rows)
			m.ExpectCommit()
			nic, err := updateNIC(c, nic, mask)
			So(err, ShouldBeNil)
			So(nic, ShouldResemble, &crimson.NIC{
				Name:       nic.Name,
				Machine:    nic.Machine,
				MacAddress: nic.MacAddress,
				Switch:     nic.Switch,
				Switchport: nic.Switchport,
			})
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("update switch", func() {
			updateStmt := `
				^UPDATE nics
				SET switch_id = \(SELECT id FROM switches WHERE name = \?\)
				WHERE name = \? AND machine_id = \(SELECT id FROM machines WHERE name = \?\)$
			`
			nic := &crimson.NIC{
				Name:       "eth0",
				Machine:    "machine",
				MacAddress: "ff:ff:ff:ff:ff:ff",
				Switch:     "switch",
				Switchport: 1,
			}
			mask := &field_mask.FieldMask{
				Paths: []string{
					"switch",
				},
			}
			rows.AddRow(nic.Name, nic.Machine, common.MaxMAC48, nic.Switch, nic.Switchport)
			m.ExpectBegin()
			m.ExpectExec(updateStmt).WithArgs(nic.Switch, nic.Name, nic.Machine).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectQuery(selectStmt).WillReturnRows(rows)
			m.ExpectCommit()
			nic, err := updateNIC(c, nic, mask)
			So(err, ShouldBeNil)
			So(nic, ShouldResemble, &crimson.NIC{
				Name:       nic.Name,
				Machine:    nic.Machine,
				MacAddress: nic.MacAddress,
				Switch:     nic.Switch,
				Switchport: nic.Switchport,
			})
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("update switchport", func() {
			updateStmt := `
				^UPDATE nics
				SET switchport = \?
				WHERE name = \? AND machine_id = \(SELECT id FROM machines WHERE name = \?\)$
			`
			nic := &crimson.NIC{
				Name:       "eth0",
				Machine:    "machine",
				MacAddress: "ff:ff:ff:ff:ff:ff",
				Switch:     "switch",
				Switchport: 1,
			}
			mask := &field_mask.FieldMask{
				Paths: []string{
					"switchport",
				},
			}
			rows.AddRow(nic.Name, nic.Machine, common.MaxMAC48, nic.Switch, nic.Switchport)
			m.ExpectBegin()
			m.ExpectExec(updateStmt).WithArgs(nic.Switchport, nic.Name, nic.Machine).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectQuery(selectStmt).WillReturnRows(rows)
			m.ExpectCommit()
			nic, err := updateNIC(c, nic, mask)
			So(err, ShouldBeNil)
			So(nic, ShouldResemble, &crimson.NIC{
				Name:       nic.Name,
				Machine:    nic.Machine,
				MacAddress: nic.MacAddress,
				Switch:     nic.Switch,
				Switchport: nic.Switchport,
			})
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("ok", func() {
			updateStmt := `
				^UPDATE nics
				SET mac_address = \?, switch_id = \(SELECT id FROM switches WHERE name = \?\), switchport = \?
				WHERE name = \? AND machine_id = \(SELECT id FROM machines WHERE name = \?\)$
			`
			nic := &crimson.NIC{
				Name:       "eth0",
				Machine:    "machine",
				MacAddress: "ff:ff:ff:ff:ff:ff",
				Switch:     "switch",
				Switchport: 1,
			}
			mask := &field_mask.FieldMask{
				Paths: []string{
					"mac_address",
					"switch",
					"switchport",
				},
			}
			rows.AddRow(nic.Name, nic.Machine, common.MaxMAC48, nic.Switch, nic.Switchport)
			m.ExpectBegin()
			m.ExpectExec(updateStmt).WithArgs(common.MaxMAC48, nic.Switch, nic.Switchport, nic.Name, nic.Machine).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectQuery(selectStmt).WillReturnRows(rows)
			m.ExpectCommit()
			nic, err := updateNIC(c, nic, mask)
			So(err, ShouldBeNil)
			So(nic, ShouldResemble, &crimson.NIC{
				Name:       nic.Name,
				Machine:    nic.Machine,
				MacAddress: nic.MacAddress,
				Switch:     nic.Switch,
				Switchport: nic.Switchport,
			})
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})
	})
}

func TestValidateNICForCreation(t *testing.T) {
	t.Parallel()

	Convey("NIC unspecified", t, func() {
		err := validateNICForCreation(nil)
		So(err, ShouldErrLike, "NIC specification is required")
	})

	Convey("name unspecified", t, func() {
		err := validateNICForCreation(&crimson.NIC{
			Machine:    "machine",
			MacAddress: "ff:ff:ff:ff:ff:ff",
			Switch:     "switch",
			Switchport: 1,
		})
		So(err, ShouldErrLike, "NIC name is required and must be non-empty")
	})

	Convey("machine unspecified", t, func() {
		err := validateNICForCreation(&crimson.NIC{
			Name:       "eth0",
			MacAddress: "ff:ff:ff:ff:ff:ff",
			Switch:     "switch",
			Switchport: 1,
		})
		So(err, ShouldErrLike, "machine is required and must be non-empty")
	})

	Convey("MAC address unspecified", t, func() {
		err := validateNICForCreation(&crimson.NIC{
			Name:       "eth0",
			Machine:    "machine",
			Switch:     "switch",
			Switchport: 1,
		})
		So(err, ShouldErrLike, "invalid MAC-48 address")
	})

	Convey("MAC address invalid", t, func() {
		err := validateNICForCreation(&crimson.NIC{
			Name:       "eth0",
			Machine:    "machine",
			MacAddress: "01:02:03:04:05:06:07:08",
			Switch:     "switch",
			Switchport: 1,
		})
		So(err, ShouldErrLike, "invalid MAC-48 address")
	})

	Convey("switch unspecified", t, func() {
		err := validateNICForCreation(&crimson.NIC{
			Name:       "eth0",
			MacAddress: "ff:ff:ff:ff:ff:ff",
			Machine:    "machine",
			Switchport: 1,
		})
		So(err, ShouldErrLike, "switch is required and must be non-empty")
	})

	Convey("switchport unspecified", t, func() {
		err := validateNICForCreation(&crimson.NIC{
			Name:       "eth0",
			Machine:    "machine",
			MacAddress: "ff:ff:ff:ff:ff:ff",
			Switch:     "switch",
		})
		So(err, ShouldErrLike, "switchport must be positive")
	})

	Convey("switchport negative", t, func() {
		err := validateNICForCreation(&crimson.NIC{
			Name:       "eth0",
			Machine:    "machine",
			MacAddress: "ff:ff:ff:ff:ff:ff",
			Switch:     "switch",
			Switchport: -1,
		})
		So(err, ShouldErrLike, "switchport must be positive")
	})

	Convey("ok", t, func() {
		err := validateNICForCreation(&crimson.NIC{
			Name:       "eth0",
			Machine:    "machine",
			MacAddress: "ff:ff:ff:ff:ff:ff",
			Switch:     "switch",
			Switchport: 1,
		})
		So(err, ShouldBeNil)
	})
}

func TestValidateNICForUpdate(t *testing.T) {
	t.Parallel()

	Convey("NIC unspecified", t, func() {
		err := validateNICForUpdate(nil, &field_mask.FieldMask{
			Paths: []string{
				"mac_address",
				"switch",
				"switchport",
			},
		})
		So(err, ShouldErrLike, "NIC specification is required")
	})

	Convey("name unspecified", t, func() {
		err := validateNICForUpdate(&crimson.NIC{
			Machine:    "machine",
			MacAddress: "ff:ff:ff:ff:ff:ff",
			Switch:     "switch",
			Switchport: 1,
		}, &field_mask.FieldMask{
			Paths: []string{
				"mac_address",
				"switch",
				"switchport",
			},
		})
		So(err, ShouldErrLike, "NIC name is required and must be non-empty")
	})

	Convey("machine unspecified", t, func() {
		err := validateNICForUpdate(&crimson.NIC{
			Name:       "eth0",
			MacAddress: "ff:ff:ff:ff:ff:ff",
			Switch:     "switch",
			Switchport: 1,
		}, &field_mask.FieldMask{
			Paths: []string{
				"mac_address",
				"switch",
				"switchport",
			},
		})
		So(err, ShouldErrLike, "machine is required and must be non-empty")
	})

	Convey("mask unspecified", t, func() {
		err := validateNICForUpdate(&crimson.NIC{
			Name:       "eth0",
			Machine:    "machine",
			MacAddress: "ff:ff:ff:ff:ff:ff",
			Switch:     "switch",
			Switchport: 1,
		}, nil)
		So(err, ShouldErrLike, "update mask is required")
	})

	Convey("no paths", t, func() {
		err := validateNICForUpdate(&crimson.NIC{
			Name:       "eth0",
			Machine:    "machine",
			MacAddress: "ff:ff:ff:ff:ff:ff",
			Switch:     "switch",
			Switchport: 1,
		}, &field_mask.FieldMask{})
		So(err, ShouldErrLike, "at least one update mask path is required")
	})

	Convey("unexpected name", t, func() {
		err := validateNICForUpdate(&crimson.NIC{
			Name:       "eth0",
			Machine:    "machine",
			MacAddress: "ff:ff:ff:ff:ff:ff",
			Switch:     "switch",
			Switchport: 1,
		}, &field_mask.FieldMask{
			Paths: []string{
				"name",
			},
		})
		So(err, ShouldErrLike, "NIC name cannot be updated")
	})

	Convey("unexpected machine", t, func() {
		err := validateNICForUpdate(&crimson.NIC{
			Name:       "eth0",
			Machine:    "machine",
			MacAddress: "ff:ff:ff:ff:ff:ff",
			Switch:     "switch",
			Switchport: 1,
		}, &field_mask.FieldMask{
			Paths: []string{
				"machine",
			},
		})
		So(err, ShouldErrLike, "machine cannot be updated")
	})

	Convey("MAC address unspecified", t, func() {
		err := validateNICForUpdate(&crimson.NIC{
			Name:       "eth0",
			Machine:    "machine",
			Switch:     "switch",
			Switchport: 1,
		}, &field_mask.FieldMask{
			Paths: []string{
				"mac_address",
				"switch",
				"switchport",
			},
		})
		So(err, ShouldErrLike, "MAC address is required and must be non-empty")
	})

	Convey("MAC address invalid", t, func() {
		err := validateNICForUpdate(&crimson.NIC{
			Name:       "eth0",
			Machine:    "machine",
			MacAddress: "01:02:03:04:05:06:07:08",
			Switch:     "switch",
			Switchport: 1,
		}, &field_mask.FieldMask{
			Paths: []string{
				"mac_address",
				"switch",
				"switchport",
			},
		})
		So(err, ShouldErrLike, "invalid MAC-48 address")
	})

	Convey("switch unspecified", t, func() {
		err := validateNICForUpdate(&crimson.NIC{
			Name:       "eth0",
			Machine:    "machine",
			MacAddress: "ff:ff:ff:ff:ff:ff",
			Switchport: 1,
		}, &field_mask.FieldMask{
			Paths: []string{
				"mac_address",
				"switch",
				"switchport",
			},
		})
		So(err, ShouldErrLike, "switch is required and must be non-empty")
	})

	Convey("switchport unspecified", t, func() {
		err := validateNICForUpdate(&crimson.NIC{
			Name:       "eth0",
			Machine:    "machine",
			MacAddress: "ff:ff:ff:ff:ff:ff",
			Switch:     "switch",
		}, &field_mask.FieldMask{
			Paths: []string{
				"mac_address",
				"switch",
				"switchport",
			},
		})
		So(err, ShouldErrLike, "switchport must be positive")
	})

	Convey("switchport negative", t, func() {
		err := validateNICForUpdate(&crimson.NIC{
			Name:       "eth0",
			Machine:    "machine",
			MacAddress: "ff:ff:ff:ff:ff:ff",
			Switch:     "switch",
			Switchport: -1,
		}, &field_mask.FieldMask{
			Paths: []string{
				"mac_address",
				"switch",
				"switchport",
			},
		})
		So(err, ShouldErrLike, "switchport must be positive")
	})

	Convey("unsupported path", t, func() {
		err := validateNICForUpdate(&crimson.NIC{
			Name:       "eth0",
			Machine:    "machine",
			MacAddress: "ff:ff:ff:ff:ff:ff",
			Switch:     "switch",
			Switchport: 1,
		}, &field_mask.FieldMask{
			Paths: []string{
				"unknown",
			},
		})
		So(err, ShouldErrLike, "unsupported update mask path")
	})

	Convey("duplicate path", t, func() {
		err := validateNICForUpdate(&crimson.NIC{
			Name:       "eth0",
			Machine:    "machine",
			MacAddress: "ff:ff:ff:ff:ff:ff",
			Switch:     "switch",
			Switchport: 1,
		}, &field_mask.FieldMask{
			Paths: []string{
				"mac_address",
				"switch",
				"switchport",
				"mac_address",
			},
		})
		So(err, ShouldErrLike, "duplicate update mask path")
	})

	Convey("ok", t, func() {
		err := validateNICForUpdate(&crimson.NIC{
			Name:       "eth0",
			Machine:    "machine",
			MacAddress: "ff:ff:ff:ff:ff:ff",
			Switch:     "switch",
			Switchport: 1,
		}, &field_mask.FieldMask{
			Paths: []string{
				"mac_address",
				"switch",
				"switchport",
			},
		})
		So(err, ShouldBeNil)
	})
}
