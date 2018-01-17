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
	"fmt"
	"testing"

	"golang.org/x/net/context"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/VividCortex/mysqlerr"
	"github.com/go-sql-driver/mysql"

	"go.chromium.org/luci/common/data/stringset"

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
		insertStmt := `
			^INSERT INTO nics \(name, machine_id, mac_address, switch_id, switchport\)
			VALUES \(\?, \(SELECT id FROM machines WHERE name = \?\), \?, \(SELECT id FROM switches WHERE name = \?\), \?\)$
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
			So(createNIC(c, nic), ShouldErrLike, "Internal server error")
		})

		Convey("prepare failed", func() {
			nic := &crimson.NIC{
				Name:       "eth0",
				Machine:    "machine",
				MacAddress: "ff:ff:ff:ff:ff:ff",
				Switch:     "switch",
				Switchport: 1,
			}
			m.ExpectBegin()
			m.ExpectPrepare(insertStmt).WillReturnError(fmt.Errorf("error"))
			So(createNIC(c, nic), ShouldErrLike, "Internal server error")
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
			m.ExpectPrepare(insertStmt)
			m.ExpectExec(insertStmt).WithArgs(nic.Name, nic.Machine, common.MaxMAC48, nic.Switch, nic.Switchport).WillReturnError(fmt.Errorf("error"))
			So(createNIC(c, nic), ShouldErrLike, "Internal server error")
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
			m.ExpectPrepare(insertStmt)
			m.ExpectExec(insertStmt).WithArgs(nic.Name, nic.Machine, common.MaxMAC48, nic.Switch, nic.Switchport).WillReturnError(&mysql.MySQLError{Number: mysqlerr.ER_DUP_ENTRY, Message: "'name'"})
			m.ExpectCommit()
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
			m.ExpectPrepare(insertStmt)
			m.ExpectExec(insertStmt).WithArgs(nic.Name, nic.Machine, common.MaxMAC48, nic.Switch, nic.Switchport).WillReturnError(&mysql.MySQLError{Number: mysqlerr.ER_BAD_NULL_ERROR, Message: "'machine_id' is null"})
			m.ExpectCommit()
			So(createNIC(c, nic), ShouldErrLike, "unknown machine")
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
			m.ExpectPrepare(insertStmt)
			m.ExpectExec(insertStmt).WithArgs(nic.Name, nic.Machine, common.MaxMAC48, nic.Switch, nic.Switchport).WillReturnError(&mysql.MySQLError{Number: mysqlerr.ER_DUP_ENTRY, Message: "'mac_address'"})
			m.ExpectCommit()
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
			m.ExpectPrepare(insertStmt)
			m.ExpectExec(insertStmt).WithArgs(nic.Name, nic.Machine, common.MaxMAC48, nic.Switch, nic.Switchport).WillReturnError(&mysql.MySQLError{Number: mysqlerr.ER_BAD_NULL_ERROR, Message: "'switch_id' is null"})
			m.ExpectCommit()
			So(createNIC(c, nic), ShouldErrLike, "unknown switch")
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
			m.ExpectPrepare(insertStmt)
			m.ExpectExec(insertStmt).WithArgs(nic.Name, nic.Machine, common.MaxMAC48, nic.Switch, nic.Switchport).WillReturnError(&mysql.MySQLError{Number: mysqlerr.ER_BAD_NULL_ERROR, Message: "error"})
			m.ExpectCommit()
			So(createNIC(c, nic), ShouldErrLike, "Internal server error")
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
			m.ExpectPrepare(insertStmt)
			m.ExpectExec(insertStmt).WithArgs(nic.Name, nic.Machine, common.MaxMAC48, nic.Switch, nic.Switchport).WillReturnError(&mysql.MySQLError{Number: mysqlerr.ER_NO, Message: "name machine_id switch_id"})
			m.ExpectCommit()
			So(createNIC(c, nic), ShouldErrLike, "Internal server error")
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
			m.ExpectPrepare(insertStmt)
			m.ExpectExec(insertStmt).WithArgs(nic.Name, nic.Machine, common.MaxMAC48, nic.Switch, nic.Switchport).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectCommit().WillReturnError(fmt.Errorf("error"))
			So(createNIC(c, nic), ShouldErrLike, "Internal server error")
		})

		Convey("ok", func() {
			nic := &crimson.NIC{
				Name:       "eth0",
				Machine:    "machine",
				MacAddress: "ff:ff:ff:ff:ff:ff",
				Switch:     "switch",
				Switchport: 1,
			}
			m.ExpectBegin()
			m.ExpectPrepare(insertStmt)
			m.ExpectExec(insertStmt).WithArgs(nic.Name, nic.Machine, common.MaxMAC48, nic.Switch, nic.Switchport).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectCommit()
			So(createNIC(c, nic), ShouldBeNil)
		})
	})
}

func TestListNICs(t *testing.T) {
	Convey("listNICs", t, func() {
		db, m, _ := sqlmock.New()
		defer db.Close()
		c := database.With(context.Background(), db)
		selectStmt := `
			^SELECT n.name, m.name, n.mac_address, s.name, n.switchport
			FROM nics n, machines m, switches s
			WHERE n.machine_id = m.id AND n.switch_id = s.id$
		`
		columns := []string{"n.name", "m.name", "n.mac_address", "s.name", "s.switchport"}
		rows := sqlmock.NewRows(columns)

		Convey("query failed", func() {
			names := stringset.NewFromSlice("eth0")
			machines := stringset.NewFromSlice("machine")
			m.ExpectQuery(selectStmt).WillReturnError(fmt.Errorf("error"))
			nics, err := listNICs(c, names, machines)
			So(err, ShouldErrLike, "failed to fetch NICs")
			So(nics, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("empty", func() {
			names := stringset.NewFromSlice("eth0")
			machines := stringset.NewFromSlice("machine")
			m.ExpectQuery(selectStmt).WillReturnRows(rows)
			nics, err := listNICs(c, names, machines)
			So(err, ShouldBeNil)
			So(nics, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("no matches", func() {
			names := stringset.NewFromSlice("eth0")
			machines := stringset.NewFromSlice("machines")
			m.ExpectQuery(selectStmt).WillReturnRows(rows)
			rows.AddRow("eth0", "machine", 0, "switch", 0)
			rows.AddRow("eth1", "machine", 1, "switch", 1)
			nics, err := listNICs(c, names, machines)
			So(err, ShouldBeNil)
			So(nics, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("matches", func() {
			names := stringset.NewFromSlice("eth1", "eth2")
			machines := stringset.NewFromSlice("machine 1")
			rows.AddRow("eth0", "machine 0", 0, "switch", 0)
			rows.AddRow("eth1", "machine 1", 1, "switch", 1)
			rows.AddRow("eth2", "machine 1", common.MaxMAC48, "switch", 2)
			m.ExpectQuery(selectStmt).WillReturnRows(rows)
			nics, err := listNICs(c, names, machines)
			So(err, ShouldBeNil)
			So(nics, ShouldResemble, []*crimson.NIC{
				{
					Name:       "eth1",
					Machine:    "machine 1",
					MacAddress: "00:00:00:00:00:01",
					Switch:     "switch",
					Switchport: 1,
				},
				{
					Name:       "eth2",
					Machine:    "machine 1",
					MacAddress: "ff:ff:ff:ff:ff:ff",
					Switch:     "switch",
					Switchport: 2,
				},
			})
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("ok", func() {
			names := stringset.New(0)
			machines := stringset.New(0)
			rows.AddRow("eth0", "machine", 0, "switch", 0)
			rows.AddRow("eth1", "machine", 1, "switch", 1)
			m.ExpectQuery(selectStmt).WillReturnRows(rows)
			nics, err := listNICs(c, names, machines)
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
		})
		So(err, ShouldErrLike, "NIC name is required and must be non-empty")
	})

	Convey("machine unspecified", t, func() {
		err := validateNICForCreation(&crimson.NIC{
			Name:       "eth0",
			MacAddress: "ff:ff:ff:ff:ff:ff",
			Switch:     "switch",
		})
		So(err, ShouldErrLike, "machine is required and must be non-empty")
	})

	Convey("MAC address unspecified", t, func() {
		err := validateNICForCreation(&crimson.NIC{
			Name:    "eth0",
			Machine: "machine",
			Switch:  "switch",
		})
		So(err, ShouldErrLike, "MAC address is required and must be non-empty")
	})
	Convey("switch unspecified", t, func() {
		err := validateNICForCreation(&crimson.NIC{
			Name:       "eth0",
			MacAddress: "ff:ff:ff:ff:ff:ff",
			Machine:    "machine",
		})
		So(err, ShouldErrLike, "switch is required and must be non-empty")
	})

	Convey("ok", t, func() {
		err := validateNICForCreation(&crimson.NIC{
			Name:       "eth0",
			Machine:    "machine",
			MacAddress: "ff:ff:ff:ff:ff:ff",
			Switch:     "switch",
		})
		So(err, ShouldBeNil)
	})
}
