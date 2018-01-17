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

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestCreatePhysicalHost(t *testing.T) {
	Convey("createPhysicalHost", t, func() {
		db, m, _ := sqlmock.New()
		defer db.Close()
		c := database.With(context.Background(), db)
		insertStmt := `
			^INSERT INTO hosts \(name, vlan_id, machine_id, os_id, vm_slots, description, deployment_ticket\)
			VALUES \(\?, \(SELECT id FROM vlans WHERE id = \?\), \(SELECT id FROM machines WHERE name = \?\), \(SELECT id FROM oses WHERE name = \?\), \?, \?, \?\)$
		`

		Convey("begin failed", func() {
			host := &crimson.PhysicalHost{
				Name:    "host",
				Vlan:    1,
				Machine: "machine",
				Os:      "os",
			}
			m.ExpectBegin().WillReturnError(fmt.Errorf("error"))
			So(createPhysicalHost(c, host), ShouldErrLike, "Internal server error")
		})

		Convey("prepare failed", func() {
			host := &crimson.PhysicalHost{
				Name:    "host",
				Vlan:    1,
				Machine: "machine",
				Os:      "os",
			}
			m.ExpectBegin()
			m.ExpectPrepare(insertStmt).WillReturnError(fmt.Errorf("error"))
			So(createPhysicalHost(c, host), ShouldErrLike, "Internal server error")
		})

		Convey("query failed", func() {
			host := &crimson.PhysicalHost{
				Name:    "host",
				Vlan:    1,
				Machine: "machine",
				Os:      "os",
			}
			m.ExpectBegin()
			m.ExpectPrepare(insertStmt)
			m.ExpectExec(insertStmt).WithArgs(host.Name, host.Vlan, host.Machine, host.Os, host.VmSlots, host.Description, host.DeploymentTicket).WillReturnError(fmt.Errorf("error"))
			So(createPhysicalHost(c, host), ShouldErrLike, "Internal server error")
		})

		Convey("duplicate host/VLAN", func() {
			host := &crimson.PhysicalHost{
				Name:    "host",
				Vlan:    1,
				Machine: "machine",
				Os:      "os",
			}
			m.ExpectBegin()
			m.ExpectPrepare(insertStmt)
			m.ExpectExec(insertStmt).WithArgs(host.Name, host.Vlan, host.Machine, host.Os, host.VmSlots, host.Description, host.DeploymentTicket).WillReturnError(&mysql.MySQLError{Number: mysqlerr.ER_DUP_ENTRY, Message: "'name'"})
			m.ExpectCommit()
			So(createPhysicalHost(c, host), ShouldErrLike, "duplicate host")
		})

		Convey("duplicate machine", func() {
			host := &crimson.PhysicalHost{
				Name:    "host",
				Vlan:    1,
				Machine: "machine",
				Os:      "os",
			}
			m.ExpectBegin()
			m.ExpectPrepare(insertStmt)
			m.ExpectExec(insertStmt).WithArgs(host.Name, host.Vlan, host.Machine, host.Os, host.VmSlots, host.Description, host.DeploymentTicket).WillReturnError(&mysql.MySQLError{Number: mysqlerr.ER_DUP_ENTRY, Message: "'machine_id'"})
			m.ExpectCommit()
			So(createPhysicalHost(c, host), ShouldErrLike, "duplicate host for machine")
		})

		Convey("invalid VLAN", func() {
			host := &crimson.PhysicalHost{
				Name:    "host",
				Vlan:    1,
				Machine: "machine",
				Os:      "os",
			}
			m.ExpectBegin()
			m.ExpectPrepare(insertStmt)
			m.ExpectExec(insertStmt).WithArgs(host.Name, host.Vlan, host.Machine, host.Os, host.VmSlots, host.Description, host.DeploymentTicket).WillReturnError(&mysql.MySQLError{Number: mysqlerr.ER_BAD_NULL_ERROR, Message: "'vlan_id' is null"})
			m.ExpectCommit()
			So(createPhysicalHost(c, host), ShouldErrLike, "unknown VLAN")
		})

		Convey("invalid machine", func() {
			host := &crimson.PhysicalHost{
				Name:    "host",
				Vlan:    1,
				Machine: "machine",
				Os:      "os",
			}
			m.ExpectBegin()
			m.ExpectPrepare(insertStmt)
			m.ExpectExec(insertStmt).WithArgs(host.Name, host.Vlan, host.Machine, host.Os, host.VmSlots, host.Description, host.DeploymentTicket).WillReturnError(&mysql.MySQLError{Number: mysqlerr.ER_BAD_NULL_ERROR, Message: "'machine_id' is null"})
			m.ExpectCommit()
			So(createPhysicalHost(c, host), ShouldErrLike, "unknown machine")
		})

		Convey("invalid operating system", func() {
			host := &crimson.PhysicalHost{
				Name:    "host",
				Vlan:    1,
				Machine: "machine",
				Os:      "os",
			}
			m.ExpectBegin()
			m.ExpectPrepare(insertStmt)
			m.ExpectExec(insertStmt).WithArgs(host.Name, host.Vlan, host.Machine, host.Os, host.VmSlots, host.Description, host.DeploymentTicket).WillReturnError(&mysql.MySQLError{Number: mysqlerr.ER_BAD_NULL_ERROR, Message: "'os_id' is null"})
			m.ExpectCommit()
			So(createPhysicalHost(c, host), ShouldErrLike, "unknown operating system")
		})

		Convey("unexpected invalid", func() {
			host := &crimson.PhysicalHost{
				Name:    "host",
				Vlan:    1,
				Machine: "machine",
				Os:      "os",
			}
			m.ExpectBegin()
			m.ExpectPrepare(insertStmt)
			m.ExpectExec(insertStmt).WithArgs(host.Name, host.Vlan, host.Machine, host.Os, host.VmSlots, host.Description, host.DeploymentTicket).WillReturnError(&mysql.MySQLError{Number: mysqlerr.ER_BAD_NULL_ERROR, Message: "error"})
			m.ExpectCommit()
			So(createPhysicalHost(c, host), ShouldErrLike, "Internal server error")
		})

		Convey("unexpected error", func() {
			host := &crimson.PhysicalHost{
				Name:    "host",
				Vlan:    1,
				Machine: "machine",
				Os:      "os",
			}
			m.ExpectBegin()
			m.ExpectPrepare(insertStmt)
			m.ExpectExec(insertStmt).WithArgs(host.Name, host.Vlan, host.Machine, host.Os, host.VmSlots, host.Description, host.DeploymentTicket).WillReturnError(&mysql.MySQLError{Number: mysqlerr.ER_NO, Message: "name vlan_id"})
			m.ExpectCommit()
			So(createPhysicalHost(c, host), ShouldErrLike, "Internal server error")
		})

		Convey("commit failed", func() {
			host := &crimson.PhysicalHost{
				Name:    "host",
				Vlan:    1,
				Machine: "machine",
				Os:      "os",
			}
			m.ExpectBegin()
			m.ExpectPrepare(insertStmt)
			m.ExpectExec(insertStmt).WithArgs(host.Name, host.Vlan, host.Machine, host.Os, host.VmSlots, host.Description, host.DeploymentTicket).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectCommit().WillReturnError(fmt.Errorf("error"))
			So(createPhysicalHost(c, host), ShouldErrLike, "Internal server error")
		})

		Convey("ok", func() {
			host := &crimson.PhysicalHost{
				Name:    "host",
				Vlan:    1,
				Machine: "machine",
				Os:      "os",
			}
			m.ExpectBegin()
			m.ExpectPrepare(insertStmt)
			m.ExpectExec(insertStmt).WithArgs(host.Name, host.Vlan, host.Machine, host.Os, host.VmSlots, host.Description, host.DeploymentTicket).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectCommit()
			So(createPhysicalHost(c, host), ShouldBeNil)
		})
	})
}

func TestListPhysicalHosts(t *testing.T) {
	Convey("listPhysicalHosts", t, func() {
		db, m, _ := sqlmock.New()
		defer db.Close()
		c := database.With(context.Background(), db)
		selectStmt := `
			^SELECT h.name, v.id, m.name, o.name, h.vm_slots, h.description, h.deployment_ticket
			FROM hosts h, vlans v, machines m, oses o
			WHERE h.vlan_id = v.id AND h.machine_id = m.id AND h.os_id = o.id$
		`
		columns := []string{"h.name", "v.id", "m.name", "o.name", "h.vm_slots", "h.description", "h.deployment_ticket"}
		rows := sqlmock.NewRows(columns)

		Convey("query failed", func() {
			names := stringset.NewFromSlice("host")
			vlans := map[int64]struct{}{0: {}}
			m.ExpectQuery(selectStmt).WillReturnError(fmt.Errorf("error"))
			hosts, err := listPhysicalHosts(c, names, vlans)
			So(err, ShouldErrLike, "failed to fetch hosts")
			So(hosts, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("empty", func() {
			names := stringset.NewFromSlice("host")
			vlans := map[int64]struct{}{0: {}}
			m.ExpectQuery(selectStmt).WillReturnRows(rows)
			hosts, err := listPhysicalHosts(c, names, vlans)
			So(err, ShouldBeNil)
			So(hosts, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("no matches", func() {
			names := stringset.NewFromSlice("host")
			vlans := map[int64]struct{}{0: {}}
			m.ExpectQuery(selectStmt).WillReturnRows(rows)
			rows.AddRow("host 1", 1, "machine 1", "os 1", 1, "", "")
			rows.AddRow("host 2", 2, "machine 2", "os 2", 2, "", "")
			hosts, err := listPhysicalHosts(c, names, vlans)
			So(err, ShouldBeNil)
			So(hosts, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("matches", func() {
			names := stringset.NewFromSlice("host 1", "host 2")
			vlans := map[int64]struct{}{1: {}, 2: {}}
			rows.AddRow("host 1", 1, "machine 1", "os 1", 1, "", "")
			rows.AddRow("host 2", 2, "machine 2", "os 2", 2, "", "")
			rows.AddRow("host 3", 3, "machine 3", "os 3", 3, "", "")
			m.ExpectQuery(selectStmt).WillReturnRows(rows)
			hosts, err := listPhysicalHosts(c, names, vlans)
			So(err, ShouldBeNil)
			So(hosts, ShouldResemble, []*crimson.PhysicalHost{
				{
					Name:    "host 1",
					Vlan:    1,
					Machine: "machine 1",
					Os:      "os 1",
					VmSlots: 1,
				},
				{
					Name:    "host 2",
					Vlan:    2,
					Machine: "machine 2",
					Os:      "os 2",
					VmSlots: 2,
				},
			})
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("ok", func() {
			names := stringset.New(0)
			vlans := map[int64]struct{}{}
			rows.AddRow("host 1", 1, "machine 1", "os 1", 1, "", "")
			rows.AddRow("host 2", 2, "machine 2", "os 2", 2, "", "")
			m.ExpectQuery(selectStmt).WillReturnRows(rows)
			hosts, err := listPhysicalHosts(c, names, vlans)
			So(err, ShouldBeNil)
			So(hosts, ShouldResemble, []*crimson.PhysicalHost{
				{
					Name:    "host 1",
					Vlan:    1,
					Machine: "machine 1",
					Os:      "os 1",
					VmSlots: 1,
				},
				{
					Name:    "host 2",
					Vlan:    2,
					Machine: "machine 2",
					Os:      "os 2",
					VmSlots: 2,
				},
			})
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})
	})
}

func TestValidatePhysicalHostForCreation(t *testing.T) {
	t.Parallel()

	Convey("host unspecified", t, func() {
		err := validatePhysicalHostForCreation(nil)
		So(err, ShouldErrLike, "host specification is required")
	})

	Convey("hostname unspecified", t, func() {
		err := validatePhysicalHostForCreation(&crimson.PhysicalHost{
			Vlan:    1,
			Machine: "machine",
			Os:      "os",
		})
		So(err, ShouldErrLike, "hostname is required and must be non-empty")
	})

	Convey("VLAN unspecified", t, func() {
		err := validatePhysicalHostForCreation(&crimson.PhysicalHost{
			Name:    "hostname",
			Machine: "machine",
			Os:      "os",
		})
		So(err, ShouldErrLike, "VLAN is required and must be positive")
	})

	Convey("VLAN negative", t, func() {
		err := validatePhysicalHostForCreation(&crimson.PhysicalHost{
			Name:    "hostname",
			Vlan:    -1,
			Machine: "machine",
			Os:      "os",
		})
		So(err, ShouldErrLike, "VLAN is required and must be positive")
	})

	Convey("machine unspecified", t, func() {
		err := validatePhysicalHostForCreation(&crimson.PhysicalHost{
			Name: "hostname",
			Vlan: 1,
			Os:   "os",
		})
		So(err, ShouldErrLike, "machine is required and must be non-empty")
	})

	Convey("machine unspecified", t, func() {
		err := validatePhysicalHostForCreation(&crimson.PhysicalHost{
			Name: "hostname",
			Vlan: 1,
			Os:   "os",
		})
		So(err, ShouldErrLike, "machine is required and must be non-empty")
	})

	Convey("operating system unspecified", t, func() {
		err := validatePhysicalHostForCreation(&crimson.PhysicalHost{
			Name:    "hostname",
			Vlan:    1,
			Machine: "machine",
		})
		So(err, ShouldErrLike, "operating system is required and must be non-empty")
	})

	Convey("ok", t, func() {
		err := validatePhysicalHostForCreation(&crimson.PhysicalHost{
			Name:    "hostname",
			Vlan:    1,
			Machine: "machine",
			Os:      "os",
		})
		So(err, ShouldBeNil)
	})
}
