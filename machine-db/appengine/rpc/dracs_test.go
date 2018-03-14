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
	"github.com/VividCortex/mysqlerr"
	"github.com/go-sql-driver/mysql"

	"go.chromium.org/luci/machine-db/api/crimson/v1"
	"go.chromium.org/luci/machine-db/appengine/database"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestCreateDRAC(t *testing.T) {
	Convey("createDRAC", t, func() {
		db, m, _ := sqlmock.New()
		defer db.Close()
		c := database.With(context.Background(), db)
		insertNameStmt := `
			^INSERT INTO hostnames \(name, vlan_id\)
			VALUES \(\?, \(SELECT vlan_id FROM ips WHERE ipv4 = \? AND hostname_id IS NULL\)\)$
		`
		insertDRACStmt := `
			^INSERT INTO dracs \(hostname_id, machine_id, switch_id, switchport, mac_address\)
			VALUES \(\?, \(SELECT id FROM machines WHERE name = \?\), \(SELECT id FROM switches WHERE name = \?\), \?, \?\)$
		`
		updateIPStmt := `
			UPDATE ips
			SET hostname_id = \?
			WHERE ipv4 = \? AND hostname_id IS NULL
		`
		selectStmt := `
			^SELECT h.name, h.vlan_id, m.name, s.name, d.switchport, d.mac_address, i.ipv4
			FROM dracs d, hostnames h, machines m, switches s, ips i
			WHERE d.hostname_id = h.id AND d.machine_id = m.id AND d.switch_id = s.id AND i.hostname_id = h.id
				AND h.name IN \(\?\) AND i.ipv4 IN \(\?\)$
		`
		columns := []string{"h.name", "h.vlan_id", "m.name", "s.name", "d.switchport", "d.mac_address", "i.ipv4"}
		rows := sqlmock.NewRows(columns)

		Convey("invalid IPv4", func() {
			drac := &crimson.DRAC{
				Name:       "drac",
				Machine:    "machine",
				Switch:     "switch",
				Switchport: 1,
				MacAddress: "00:00:00:00:00:01",
				Ipv4:       "127.0.0.1/20",
			}
			res, err := createDRAC(c, drac)
			So(err, ShouldErrLike, "invalid IPv4 address")
			So(res, ShouldBeNil)
		})

		Convey("invalid MAC-48", func() {
			drac := &crimson.DRAC{
				Name:       "drac",
				Machine:    "machine",
				Switch:     "switch",
				Switchport: 1,
				MacAddress: "01:02:03:04:05:06:07:08",
				Ipv4:       "127.0.0.1",
			}
			res, err := createDRAC(c, drac)
			So(err, ShouldErrLike, "invalid MAC-48 address")
			So(res, ShouldBeNil)
		})

		Convey("begin failed", func() {
			drac := &crimson.DRAC{
				Name:       "drac",
				Machine:    "machine",
				Switch:     "switch",
				Switchport: 1,
				MacAddress: "00:00:00:00:00:01",
				Ipv4:       "127.0.0.1",
			}
			m.ExpectBegin().WillReturnError(fmt.Errorf("error"))
			res, err := createDRAC(c, drac)
			So(err, ShouldErrLike, "Internal server error")
			So(res, ShouldBeNil)
		})

		Convey("invalid IP", func() {
			drac := &crimson.DRAC{
				Name:       "drac",
				Machine:    "machine",
				Switch:     "switch",
				Switchport: 1,
				MacAddress: "00:00:00:00:00:01",
				Ipv4:       "127.0.0.1",
			}
			m.ExpectBegin()
			m.ExpectExec(insertNameStmt).WithArgs(drac.Name, 2130706433).WillReturnError(&mysql.MySQLError{Number: mysqlerr.ER_BAD_NULL_ERROR, Message: "'vlan_id'"})
			m.ExpectRollback()
			res, err := createDRAC(c, drac)
			So(err, ShouldErrLike, "ensure IPv4 address")
			So(res, ShouldBeNil)
		})

		Convey("duplicate DRAC/VLAN", func() {
			drac := &crimson.DRAC{
				Name:       "drac",
				Machine:    "machine",
				Switch:     "switch",
				Switchport: 1,
				MacAddress: "00:00:00:00:00:01",
				Ipv4:       "127.0.0.1",
			}
			m.ExpectBegin()
			m.ExpectExec(insertNameStmt).WithArgs(drac.Name, 2130706433).WillReturnError(&mysql.MySQLError{Number: mysqlerr.ER_DUP_ENTRY, Message: "'name'"})
			m.ExpectRollback()
			res, err := createDRAC(c, drac)
			So(err, ShouldErrLike, "duplicate hostname")
			So(res, ShouldBeNil)
		})

		Convey("query failed", func() {
			drac := &crimson.DRAC{
				Name:       "drac",
				Machine:    "machine",
				Switch:     "switch",
				Switchport: 1,
				MacAddress: "00:00:00:00:00:01",
				Ipv4:       "127.0.0.1",
			}
			m.ExpectBegin()
			m.ExpectExec(insertNameStmt).WithArgs(drac.Name, 2130706433).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectExec(updateIPStmt).WithArgs(1, 2130706433).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectExec(insertDRACStmt).WithArgs(1, drac.Machine).WillReturnError(fmt.Errorf("error"))
			m.ExpectRollback()
			res, err := createDRAC(c, drac)
			So(err, ShouldErrLike, "Internal server error")
			So(res, ShouldBeNil)
		})

		Convey("duplicate machine", func() {
			drac := &crimson.DRAC{
				Name:       "drac",
				Machine:    "machine",
				Switch:     "switch",
				Switchport: 1,
				MacAddress: "00:00:00:00:00:01",
				Ipv4:       "127.0.0.1",
			}
			m.ExpectBegin()
			m.ExpectExec(insertNameStmt).WithArgs(drac.Name, 2130706433).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectExec(updateIPStmt).WithArgs(1, 2130706433).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectExec(insertDRACStmt).WithArgs(1, drac.Machine, drac.Switch, 1, 1).WillReturnError(&mysql.MySQLError{Number: mysqlerr.ER_DUP_ENTRY, Message: "'machine_id'"})
			m.ExpectRollback()
			res, err := createDRAC(c, drac)
			So(err, ShouldErrLike, "duplicate DRAC for machine")
			So(res, ShouldBeNil)
		})

		Convey("duplicate MAC", func() {
			drac := &crimson.DRAC{
				Name:       "drac",
				Machine:    "machine",
				Switch:     "switch",
				Switchport: 1,
				MacAddress: "00:00:00:00:00:01",
				Ipv4:       "127.0.0.1",
			}
			m.ExpectBegin()
			m.ExpectExec(insertNameStmt).WithArgs(drac.Name, 2130706433).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectExec(updateIPStmt).WithArgs(1, 2130706433).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectExec(insertDRACStmt).WithArgs(1, drac.Machine, drac.Switch, 1, 1).WillReturnError(&mysql.MySQLError{Number: mysqlerr.ER_DUP_ENTRY, Message: "'mac_address'"})
			m.ExpectRollback()
			res, err := createDRAC(c, drac)
			So(err, ShouldErrLike, "duplicate MAC address")
			So(res, ShouldBeNil)
		})

		Convey("invalid machine", func() {
			drac := &crimson.DRAC{
				Name:       "drac",
				Machine:    "machine",
				Switch:     "switch",
				Switchport: 1,
				MacAddress: "00:00:00:00:00:01",
				Ipv4:       "127.0.0.1",
			}
			m.ExpectBegin()
			m.ExpectExec(insertNameStmt).WithArgs(drac.Name, 2130706433).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectExec(updateIPStmt).WithArgs(1, 2130706433).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectExec(insertDRACStmt).WithArgs(1, drac.Machine, drac.Switch, 1, 1).WillReturnError(&mysql.MySQLError{Number: mysqlerr.ER_BAD_NULL_ERROR, Message: "'machine_id' is null"})
			m.ExpectRollback()
			res, err := createDRAC(c, drac)
			So(err, ShouldErrLike, "unknown machine")
			So(res, ShouldBeNil)
		})

		Convey("invalid switch", func() {
			drac := &crimson.DRAC{
				Name:       "drac",
				Machine:    "machine",
				Switch:     "switch",
				Switchport: 1,
				MacAddress: "00:00:00:00:00:01",
				Ipv4:       "127.0.0.1",
			}
			m.ExpectBegin()
			m.ExpectExec(insertNameStmt).WithArgs(drac.Name, 2130706433).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectExec(updateIPStmt).WithArgs(1, 2130706433).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectExec(insertDRACStmt).WithArgs(1, drac.Machine, drac.Switch, 1, 1).WillReturnError(&mysql.MySQLError{Number: mysqlerr.ER_BAD_NULL_ERROR, Message: "'switch_id' is null"})
			m.ExpectRollback()
			res, err := createDRAC(c, drac)
			So(err, ShouldErrLike, "unknown switch")
			So(res, ShouldBeNil)
		})

		Convey("unexpected invalid", func() {
			drac := &crimson.DRAC{
				Name:       "drac",
				Machine:    "machine",
				Switch:     "switch",
				Switchport: 1,
				MacAddress: "00:00:00:00:00:01",
				Ipv4:       "127.0.0.1",
			}
			m.ExpectBegin()
			m.ExpectExec(insertNameStmt).WithArgs(drac.Name, 2130706433).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectExec(updateIPStmt).WithArgs(1, 2130706433).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectExec(insertDRACStmt).WithArgs(1, drac.Machine, drac.Switch, 1, 1).WillReturnError(&mysql.MySQLError{Number: mysqlerr.ER_BAD_NULL_ERROR, Message: "error"})
			m.ExpectRollback()
			res, err := createDRAC(c, drac)
			So(err, ShouldErrLike, "Internal server error")
			So(res, ShouldBeNil)
		})

		Convey("unexpected error", func() {
			drac := &crimson.DRAC{
				Name:       "drac",
				Machine:    "machine",
				Switch:     "switch",
				Switchport: 1,
				MacAddress: "00:00:00:00:00:01",
				Ipv4:       "127.0.0.1",
			}
			m.ExpectBegin()
			m.ExpectExec(insertNameStmt).WithArgs(drac.Name, 2130706433).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectExec(updateIPStmt).WithArgs(1, 2130706433).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectExec(insertDRACStmt).WithArgs(1, drac.Machine, drac.Switch, 1, 1).WillReturnError(&mysql.MySQLError{Number: mysqlerr.ER_NO, Message: "name vlan_id"})
			m.ExpectRollback()
			res, err := createDRAC(c, drac)
			So(err, ShouldErrLike, "Internal server error")
			So(res, ShouldBeNil)
		})

		Convey("commit failed", func() {
			drac := &crimson.DRAC{
				Name:       "drac",
				Machine:    "machine",
				Switch:     "switch",
				Switchport: 1,
				MacAddress: "00:00:00:00:00:01",
				Ipv4:       "127.0.0.1",
			}
			m.ExpectBegin()
			m.ExpectExec(insertNameStmt).WithArgs(drac.Name, 2130706433).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectExec(updateIPStmt).WithArgs(1, 2130706433).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectExec(insertDRACStmt).WithArgs(1, drac.Machine, drac.Switch, 1, 1).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectCommit().WillReturnError(fmt.Errorf("error"))
			m.ExpectRollback()
			res, err := createDRAC(c, drac)
			So(err, ShouldErrLike, "Internal server error")
			So(res, ShouldBeNil)
		})

		Convey("ok", func() {
			drac := &crimson.DRAC{
				Name:       "drac",
				Machine:    "machine",
				Switch:     "switch",
				Switchport: 1,
				MacAddress: "00:00:00:00:00:01",
				Ipv4:       "127.0.0.1",
			}
			rows.AddRow(drac.Name, 1, drac.Machine, drac.Switch, drac.Switchport, 1, 2130706433)
			m.ExpectBegin()
			m.ExpectExec(insertNameStmt).WithArgs(drac.Name, 2130706433).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectExec(updateIPStmt).WithArgs(1, 2130706433).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectExec(insertDRACStmt).WithArgs(1, drac.Machine, drac.Switch, 1, 1).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectQuery(selectStmt).WithArgs(drac.Name, 2130706433).WillReturnRows(rows)
			m.ExpectCommit()
			res, err := createDRAC(c, drac)
			So(err, ShouldBeNil)
			So(res, ShouldResemble, &crimson.DRAC{
				Name:       drac.Name,
				Machine:    drac.Machine,
				Switch:     drac.Switch,
				Switchport: drac.Switchport,
				MacAddress: drac.MacAddress,
				Ipv4:       drac.Ipv4,
				Vlan:       1,
			})
		})
	})
}

func TestListDRACs(t *testing.T) {
	Convey("listDRACs", t, func() {
		db, m, _ := sqlmock.New()
		defer db.Close()
		c := database.With(context.Background(), db)
		columns := []string{"h.name", "h.vlan_id", "m.name", "s.name", "d.switchport", "d.mac_address", "i.ipv4"}
		rows := sqlmock.NewRows(columns)

		Convey("invalid IPv4 address", func() {
			req := &crimson.ListDRACsRequest{
				Names:    []string{"drac"},
				Machines: []string{"machine 1", "machine 2"},
				Ipv4S:    []string{"0.0.0.1", "0.0.0.2/20"},
				Vlans:    []int64{0},
			}
			dracs, err := listDRACs(c, db, req)
			So(err, ShouldErrLike, "invalid IPv4 address")
			So(dracs, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("query failed", func() {
			selectStmt := `
				^SELECT h.name, h.vlan_id, m.name, s.name, d.switchport, d.mac_address, i.ipv4
				FROM dracs d, hostnames h, machines m, switches s, ips i
				WHERE d.hostname_id = h.id AND d.machine_id = m.id AND d.switch_id = s.id AND i.hostname_id = h.id
					AND h.name IN \(\?\) AND m.name IN \(\?,\?\) AND i.ipv4 IN \(\?,\?\) AND h.vlan_id IN \(\?\)$
			`
			req := &crimson.ListDRACsRequest{
				Names:    []string{"drac"},
				Machines: []string{"machine 1", "machine 2"},
				Ipv4S:    []string{"0.0.0.1", "0.0.0.2"},
				Vlans:    []int64{0},
			}
			m.ExpectQuery(selectStmt).WithArgs(req.Names[0], req.Machines[0], req.Machines[1], 1, 2, req.Vlans[0]).WillReturnError(fmt.Errorf("error"))
			dracs, err := listDRACs(c, db, req)
			So(err, ShouldErrLike, "Internal server error")
			So(dracs, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("name", func() {
			selectStmt := `
				^SELECT h.name, h.vlan_id, m.name, s.name, d.switchport, d.mac_address, i.ipv4
				FROM dracs d, hostnames h, machines m, switches s, ips i
				WHERE d.hostname_id = h.id AND d.machine_id = m.id AND d.switch_id = s.id AND i.hostname_id = h.id AND h.name IN \(\?\)$
			`
			req := &crimson.ListDRACsRequest{
				Names: []string{"drac"},
			}
			m.ExpectQuery(selectStmt).WithArgs(req.Names[0]).WillReturnRows(rows)
			dracs, err := listDRACs(c, db, req)
			So(err, ShouldBeNil)
			So(dracs, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("machine", func() {
			selectStmt := `
				^SELECT h.name, h.vlan_id, m.name, s.name, d.switchport, d.mac_address, i.ipv4
				FROM dracs d, hostnames h, machines m, switches s, ips i
				WHERE d.hostname_id = h.id AND d.machine_id = m.id AND d.switch_id = s.id AND i.hostname_id = h.id AND m.name IN \(\?\)$
			`
			req := &crimson.ListDRACsRequest{
				Machines: []string{"machine"},
			}
			m.ExpectQuery(selectStmt).WithArgs(req.Machines[0]).WillReturnRows(rows)
			dracs, err := listDRACs(c, db, req)
			So(err, ShouldBeNil)
			So(dracs, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("IPv4", func() {
			selectStmt := `
				^SELECT h.name, h.vlan_id, m.name, s.name, d.switchport, d.mac_address, i.ipv4
				FROM dracs d, hostnames h, machines m, switches s, ips i
				WHERE d.hostname_id = h.id AND d.machine_id = m.id AND d.switch_id = s.id AND i.hostname_id = h.id AND i.ipv4 IN \(\?\)$
			`
			req := &crimson.ListDRACsRequest{
				Ipv4S: []string{"0.0.0.1"},
			}
			m.ExpectQuery(selectStmt).WithArgs(1).WillReturnRows(rows)
			dracs, err := listDRACs(c, db, req)
			So(err, ShouldBeNil)
			So(dracs, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("VLAN", func() {
			selectStmt := `
				^SELECT h.name, h.vlan_id, m.name, s.name, d.switchport, d.mac_address, i.ipv4
				FROM dracs d, hostnames h, machines m, switches s, ips i
				WHERE d.hostname_id = h.id AND d.machine_id = m.id AND d.switch_id = s.id AND i.hostname_id = h.id AND h.vlan_id IN \(\?\)$
			`
			req := &crimson.ListDRACsRequest{
				Vlans: []int64{0},
			}
			m.ExpectQuery(selectStmt).WithArgs(req.Vlans[0]).WillReturnRows(rows)
			dracs, err := listDRACs(c, db, req)
			So(err, ShouldBeNil)
			So(dracs, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("non-empty", func() {
			selectStmt := `
				^SELECT h.name, h.vlan_id, m.name, s.name, d.switchport, d.mac_address, i.ipv4
				FROM dracs d, hostnames h, machines m, switches s, ips i
				WHERE d.hostname_id = h.id AND d.machine_id = m.id AND d.switch_id = s.id AND i.hostname_id = h.id
					AND h.name IN \(\?\) AND m.name IN \(\?,\?\) AND i.ipv4 IN \(\?,\?\) AND h.vlan_id IN \(\?\)$
			`
			req := &crimson.ListDRACsRequest{
				Names:    []string{"drac"},
				Machines: []string{"machine 1", "machine 2"},
				Ipv4S:    []string{"0.0.0.1", "0.0.0.2"},
				Vlans:    []int64{0},
			}
			rows.AddRow(req.Names[0], req.Vlans[0], req.Machines[0], "switch 1", 1, 1, 1)
			rows.AddRow(req.Names[0], req.Vlans[0], req.Machines[0], "switch 2", 2, 2, 2)
			m.ExpectQuery(selectStmt).WithArgs(req.Names[0], req.Machines[0], req.Machines[1], 1, 2, req.Vlans[0]).WillReturnRows(rows)
			dracs, err := listDRACs(c, db, req)
			So(err, ShouldBeNil)
			So(dracs, ShouldResemble, []*crimson.DRAC{
				{
					Name:       req.Names[0],
					Machine:    req.Machines[0],
					Switch:     "switch 1",
					Switchport: 1,
					MacAddress: "00:00:00:00:00:01",
					Ipv4:       "0.0.0.1",
					Vlan:       req.Vlans[0],
				},
				{
					Name:       req.Names[0],
					Machine:    req.Machines[0],
					Switch:     "switch 2",
					Switchport: 2,
					MacAddress: "00:00:00:00:00:02",
					Ipv4:       "0.0.0.2",
					Vlan:       req.Vlans[0],
				},
			})
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("ok", func() {
			selectStmt := `
				^SELECT h.name, h.vlan_id, m.name, s.name, d.switchport, d.mac_address, i.ipv4
				FROM dracs d, hostnames h, machines m, switches s, ips i
				WHERE d.hostname_id = h.id AND d.machine_id = m.id AND d.switch_id = s.id AND i.hostname_id = h.id$
			`
			req := &crimson.ListDRACsRequest{}
			rows.AddRow("drac", 0, "machine 1", "switch 1", 1, 1, 1)
			rows.AddRow("drac", 0, "machine 2", "switch 2", 2, 2, 2)
			m.ExpectQuery(selectStmt).WithArgs().WillReturnRows(rows)
			dracs, err := listDRACs(c, db, req)
			So(err, ShouldBeNil)
			So(dracs, ShouldResemble, []*crimson.DRAC{
				{
					Name:       "drac",
					Machine:    "machine 1",
					Switch:     "switch 1",
					Switchport: 1,
					MacAddress: "00:00:00:00:00:01",
					Ipv4:       "0.0.0.1",
				},
				{
					Name:       "drac",
					Machine:    "machine 2",
					Switch:     "switch 2",
					Switchport: 2,
					MacAddress: "00:00:00:00:00:02",
					Ipv4:       "0.0.0.2",
				},
			})
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})
	})
}

func TestValidateDRACForCreation(t *testing.T) {
	t.Parallel()

	Convey("DRAC unspecified", t, func() {
		err := validateDRACForCreation(nil)
		So(err, ShouldErrLike, "DRAC specification is required")
	})

	Convey("hostname unspecified", t, func() {
		err := validateDRACForCreation(&crimson.DRAC{
			Machine:    "machine",
			Switch:     "switch",
			Switchport: 1,
			MacAddress: "00:00:00:00:00:01",
			Ipv4:       "127.0.0.1",
		})
		So(err, ShouldErrLike, "hostname is required and must be non-empty")
	})

	Convey("machine unspecified", t, func() {
		err := validateDRACForCreation(&crimson.DRAC{
			Name:       "drac",
			Switch:     "switch",
			Switchport: 1,
			MacAddress: "00:00:00:00:00:01",
			Ipv4:       "127.0.0.1",
		})
		So(err, ShouldErrLike, "machine is required and must be non-empty")
	})

	Convey("switch unspecified", t, func() {
		err := validateDRACForCreation(&crimson.DRAC{
			Name:       "hostname",
			Machine:    "machine",
			Switchport: 1,
			MacAddress: "00:00:00:00:00:01",
			Ipv4:       "127.0.0.1",
		})
		So(err, ShouldErrLike, "switch is required and must be non-empty")
	})

	Convey("switchport unspecified", t, func() {
		err := validateDRACForCreation(&crimson.DRAC{
			Name:       "hostname",
			Machine:    "machine",
			Switch:     "switch",
			MacAddress: "00:00:00:00:00:01",
			Ipv4:       "127.0.0.1",
		})
		So(err, ShouldErrLike, "switchport must be positive")
	})

	Convey("switchport negative", t, func() {
		err := validateDRACForCreation(&crimson.DRAC{
			Name:       "hostname",
			Machine:    "machine",
			Switch:     "switch",
			Switchport: -1,
			MacAddress: "00:00:00:00:00:01",
			Ipv4:       "127.0.0.1",
		})
		So(err, ShouldErrLike, "switchport must be positive")
	})

	Convey("MAC-48 address unspecified", t, func() {
		err := validateDRACForCreation(&crimson.DRAC{
			Name:       "hostname",
			Machine:    "machine",
			Switch:     "switch",
			Switchport: 1,
			Ipv4:       "127.0.0.1",
		})
		So(err, ShouldErrLike, "invalid MAC-48 address")
	})

	Convey("MAC-48 address invalid", t, func() {
		err := validateDRACForCreation(&crimson.DRAC{
			Name:       "hostname",
			Machine:    "machine",
			Switch:     "switch",
			Switchport: 1,
			MacAddress: "01:02:03:04:05:06:07:08",
			Ipv4:       "127.0.0.1",
		})
		So(err, ShouldErrLike, "invalid MAC-48 address")
	})

	Convey("IPv4 address unspecified", t, func() {
		err := validateDRACForCreation(&crimson.DRAC{
			Name:       "drac",
			Machine:    "machine",
			Switch:     "switch",
			Switchport: 1,
			MacAddress: "00:00:00:00:00:01",
		})
		So(err, ShouldErrLike, "invalid IPv4 address")
	})

	Convey("IPv4 address invalid", t, func() {
		err := validateDRACForCreation(&crimson.DRAC{
			Name:       "drac",
			Machine:    "machine",
			Switch:     "switch",
			Switchport: 1,
			MacAddress: "00:00:00:00:00:01",
			Ipv4:       "127.0.0.1/20",
		})
		So(err, ShouldErrLike, "invalid IPv4 address")
	})

	Convey("VLAN specified", t, func() {
		err := validateDRACForCreation(&crimson.DRAC{
			Name:       "drac",
			Machine:    "machine",
			Switch:     "switch",
			Switchport: 1,
			MacAddress: "00:00:00:00:00:01",
			Ipv4:       "127.0.0.1",
			Vlan:       1,
		})
		So(err, ShouldErrLike, "VLAN must not be specified, use IP address instead")
	})

	Convey("ok", t, func() {
		err := validateDRACForCreation(&crimson.DRAC{
			Name:       "hostname",
			Machine:    "machine",
			Switch:     "switch",
			Switchport: 1,
			MacAddress: "00:00:00:00:00:01",
			Ipv4:       "127.0.0.1",
		})
		So(err, ShouldBeNil)
	})
}
