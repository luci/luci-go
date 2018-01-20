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

func TestCreateVM(t *testing.T) {
	Convey("createVM", t, func() {
		db, m, _ := sqlmock.New()
		defer db.Close()
		c := database.With(context.Background(), db)
		insertStmt := `
			^INSERT INTO vms \(name, host_id, vlan_id, os_id, description, deployment_ticket\)
			VALUES \(\?, \(SELECT id FROM hosts WHERE name = \? AND vlan_id = \?\), \(SELECT id FROM vlans WHERE id = \?\), \(SELECT id FROM oses WHERE name = \?\), \?, \?\)$
		`

		Convey("begin failed", func() {
			vm := &crimson.VM{
				Name: "vm",
				Host: "host",
				Vlan: 1,
				Os:   "os",
			}
			m.ExpectBegin().WillReturnError(fmt.Errorf("error"))
			So(createVM(c, vm), ShouldErrLike, "Internal server error")
		})

		Convey("prepare failed", func() {
			vm := &crimson.VM{
				Name: "vm",
				Host: "host",
				Vlan: 1,
				Os:   "os",
			}
			m.ExpectBegin()
			m.ExpectPrepare(insertStmt).WillReturnError(fmt.Errorf("error"))
			So(createVM(c, vm), ShouldErrLike, "Internal server error")
		})

		Convey("query failed", func() {
			vm := &crimson.VM{
				Name: "vm",
				Host: "host",
				Vlan: 1,
				Os:   "os",
			}
			m.ExpectBegin()
			m.ExpectPrepare(insertStmt)
			m.ExpectExec(insertStmt).WithArgs(vm.Name, vm.Host, vm.Vlan, vm.Vlan, vm.Os, vm.Description, vm.DeploymentTicket).WillReturnError(fmt.Errorf("error"))
			So(createVM(c, vm), ShouldErrLike, "Internal server error")
		})

		Convey("duplicate VM/host", func() {
			vm := &crimson.VM{
				Name: "vm",
				Host: "host",
				Vlan: 1,
				Os:   "os",
			}
			m.ExpectBegin()
			m.ExpectPrepare(insertStmt)
			m.ExpectExec(insertStmt).WithArgs(vm.Name, vm.Host, vm.Vlan, vm.Vlan, vm.Os, vm.Description, vm.DeploymentTicket).WillReturnError(&mysql.MySQLError{Number: mysqlerr.ER_DUP_ENTRY, Message: "'name'"})
			m.ExpectCommit()
			So(createVM(c, vm), ShouldErrLike, "duplicate VM")
		})

		Convey("duplicate VM/VLAN", func() {
			vm := &crimson.VM{
				Name: "vm",
				Host: "host",
				Vlan: 1,
				Os:   "os",
			}
			m.ExpectBegin()
			m.ExpectPrepare(insertStmt)
			m.ExpectExec(insertStmt).WithArgs(vm.Name, vm.Host, vm.Vlan, vm.Vlan, vm.Os, vm.Description, vm.DeploymentTicket).WillReturnError(&mysql.MySQLError{Number: mysqlerr.ER_DUP_ENTRY, Message: "'name_2'"})
			m.ExpectCommit()
			So(createVM(c, vm), ShouldErrLike, "duplicate VM")
		})

		Convey("invalid host", func() {
			vm := &crimson.VM{
				Name: "vm",
				Host: "host",
				Vlan: 1,
				Os:   "os",
			}
			m.ExpectBegin()
			m.ExpectPrepare(insertStmt)
			m.ExpectExec(insertStmt).WithArgs(vm.Name, vm.Host, vm.Vlan, vm.Vlan, vm.Os, vm.Description, vm.DeploymentTicket).WillReturnError(&mysql.MySQLError{Number: mysqlerr.ER_BAD_NULL_ERROR, Message: "'host_id' is null"})
			m.ExpectCommit()
			So(createVM(c, vm), ShouldErrLike, "unknown host")
		})

		Convey("invalid VLAN", func() {
			vm := &crimson.VM{
				Name: "vm",
				Host: "host",
				Vlan: 1,
				Os:   "os",
			}
			m.ExpectBegin()
			m.ExpectPrepare(insertStmt)
			m.ExpectExec(insertStmt).WithArgs(vm.Name, vm.Host, vm.Vlan, vm.Vlan, vm.Os, vm.Description, vm.DeploymentTicket).WillReturnError(&mysql.MySQLError{Number: mysqlerr.ER_BAD_NULL_ERROR, Message: "'vlan_id' is null"})
			m.ExpectCommit()
			So(createVM(c, vm), ShouldErrLike, "unknown VLAN")
		})

		Convey("invalid operating system", func() {
			vm := &crimson.VM{
				Name: "vm",
				Host: "host",
				Vlan: 1,
				Os:   "os",
			}
			m.ExpectBegin()
			m.ExpectPrepare(insertStmt)
			m.ExpectExec(insertStmt).WithArgs(vm.Name, vm.Host, vm.Vlan, vm.Vlan, vm.Os, vm.Description, vm.DeploymentTicket).WillReturnError(&mysql.MySQLError{Number: mysqlerr.ER_BAD_NULL_ERROR, Message: "'os_id' is null"})
			m.ExpectCommit()
			So(createVM(c, vm), ShouldErrLike, "unknown operating system")
		})

		Convey("unexpected invalid", func() {
			vm := &crimson.VM{
				Name: "vm",
				Host: "host",
				Vlan: 1,
				Os:   "os",
			}
			m.ExpectBegin()
			m.ExpectPrepare(insertStmt)
			m.ExpectExec(insertStmt).WithArgs(vm.Name, vm.Host, vm.Vlan, vm.Vlan, vm.Os, vm.Description, vm.DeploymentTicket).WillReturnError(&mysql.MySQLError{Number: mysqlerr.ER_BAD_NULL_ERROR, Message: "error"})
			m.ExpectCommit()
			So(createVM(c, vm), ShouldErrLike, "Internal server error")
		})

		Convey("unexpected error", func() {
			vm := &crimson.VM{
				Name: "vm",
				Host: "host",
				Vlan: 1,
				Os:   "os",
			}
			m.ExpectBegin()
			m.ExpectPrepare(insertStmt)
			m.ExpectExec(insertStmt).WithArgs(vm.Name, vm.Host, vm.Vlan, vm.Vlan, vm.Os, vm.Description, vm.DeploymentTicket).WillReturnError(&mysql.MySQLError{Number: mysqlerr.ER_NO, Message: "name host_id vlan_id"})
			m.ExpectCommit()
			So(createVM(c, vm), ShouldErrLike, "Internal server error")
		})

		Convey("commit failed", func() {
			vm := &crimson.VM{
				Name: "vm",
				Host: "host",
				Vlan: 1,
				Os:   "os",
			}
			m.ExpectBegin()
			m.ExpectPrepare(insertStmt)
			m.ExpectExec(insertStmt).WithArgs(vm.Name, vm.Host, vm.Vlan, vm.Vlan, vm.Os, vm.Description, vm.DeploymentTicket).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectCommit().WillReturnError(fmt.Errorf("error"))
			So(createVM(c, vm), ShouldErrLike, "Internal server error")
		})

		Convey("ok", func() {
			vm := &crimson.VM{
				Name: "vm",
				Host: "host",
				Vlan: 1,
				Os:   "os",
			}
			m.ExpectBegin()
			m.ExpectPrepare(insertStmt)
			m.ExpectExec(insertStmt).WithArgs(vm.Name, vm.Host, vm.Vlan, vm.Vlan, vm.Os, vm.Description, vm.DeploymentTicket).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectCommit()
			So(createVM(c, vm), ShouldBeNil)
		})
	})
}

func TestListVMs(t *testing.T) {
	Convey("listVMs", t, func() {
		db, m, _ := sqlmock.New()
		defer db.Close()
		c := database.With(context.Background(), db)
		selectStmt := `
			^SELECT v.name, h.name, h.vlan_id, o.name, v.description, v.deployment_ticket
			FROM vms v, hosts h, oses o
			WHERE v.host_id = h.id AND h.os_id = o.id$
		`
		columns := []string{"v.name", "h.name", "h.vlan_id", "o.name", "h.description", "h.deployment_ticket"}
		rows := sqlmock.NewRows(columns)

		Convey("query failed", func() {
			names := stringset.NewFromSlice("vm")
			hosts := stringset.NewFromSlice("host")
			vlans := map[int64]struct{}{0: {}}
			m.ExpectQuery(selectStmt).WillReturnError(fmt.Errorf("error"))
			vms, err := listVMs(c, names, hosts, vlans)
			So(err, ShouldErrLike, "failed to fetch VMs")
			So(vms, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("empty", func() {
			names := stringset.NewFromSlice("vms")
			hosts := stringset.NewFromSlice("host")
			vlans := map[int64]struct{}{0: {}}
			m.ExpectQuery(selectStmt).WillReturnRows(rows)
			vms, err := listVMs(c, names, hosts, vlans)
			So(err, ShouldBeNil)
			So(vms, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("no matches", func() {
			names := stringset.NewFromSlice("host")
			hosts := stringset.NewFromSlice("vm")
			vlans := map[int64]struct{}{0: {}}
			m.ExpectQuery(selectStmt).WillReturnRows(rows)
			rows.AddRow("vm 1", "host 1", 1, "os 1", "", "")
			rows.AddRow("vm 2", "host 2", 2, "os 2", "", "")
			vms, err := listVMs(c, names, hosts, vlans)
			So(err, ShouldBeNil)
			So(vms, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("matches", func() {
			names := stringset.NewFromSlice("vm 1", "vm 2")
			hosts := stringset.NewFromSlice("host 1", "host 2")
			vlans := map[int64]struct{}{1: {}, 2: {}}
			rows.AddRow("vm 1", "host 1", 1, "os 1", "", "")
			rows.AddRow("vm 2", "host 2", 2, "os 2", "", "")
			rows.AddRow("vm 3", "host 3", 3, "os 3", "", "")
			m.ExpectQuery(selectStmt).WillReturnRows(rows)
			vms, err := listVMs(c, names, hosts, vlans)
			So(err, ShouldBeNil)
			So(vms, ShouldResemble, []*crimson.VM{
				{
					Name: "vm 1",
					Host: "host 1",
					Vlan: 1,
					Os:   "os 1",
				},
				{
					Name: "vm 2",
					Host: "host 2",
					Vlan: 2,
					Os:   "os 2",
				},
			})
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("ok", func() {
			names := stringset.New(0)
			hosts := stringset.New(0)
			vlans := map[int64]struct{}{}
			rows.AddRow("vm 1", "host 1", 1, "os 1", "", "")
			rows.AddRow("vm 2", "host 2", 2, "os 2", "", "")
			m.ExpectQuery(selectStmt).WillReturnRows(rows)
			vms, err := listVMs(c, names, hosts, vlans)
			So(err, ShouldBeNil)
			So(vms, ShouldResemble, []*crimson.VM{
				{
					Name: "vm 1",
					Host: "host 1",
					Vlan: 1,
					Os:   "os 1",
				},
				{
					Name: "vm 2",
					Host: "host 2",
					Vlan: 2,
					Os:   "os 2",
				},
			})
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})
	})
}

func TestValidateVMForCreation(t *testing.T) {
	t.Parallel()

	Convey("VM unspecified", t, func() {
		err := validateVMForCreation(nil)
		So(err, ShouldErrLike, "VM specification is required")
	})

	Convey("hostname unspecified", t, func() {
		err := validateVMForCreation(&crimson.VM{
			Host: "host",
			Vlan: 1,
			Os:   "os",
		})
		So(err, ShouldErrLike, "hostname is required and must be non-empty")
	})

	Convey("physical host unspecified", t, func() {
		err := validateVMForCreation(&crimson.VM{
			Name: "vm",
			Vlan: 1,
			Os:   "os",
		})
		So(err, ShouldErrLike, "physical host is required and must be non-empty")
	})

	Convey("VLAN unspecified", t, func() {
		err := validateVMForCreation(&crimson.VM{
			Name: "vm",
			Host: "host",
			Os:   "os",
		})
		So(err, ShouldErrLike, "VLAN is required and must be positive")
	})

	Convey("VLAN negative", t, func() {
		err := validateVMForCreation(&crimson.VM{
			Name: "vm",
			Host: "host",
			Vlan: -1,
			Os:   "os",
		})
		So(err, ShouldErrLike, "VLAN is required and must be positive")
	})

	Convey("operating system unspecified", t, func() {
		err := validateVMForCreation(&crimson.VM{
			Name: "vm",
			Host: "host",
			Vlan: 1,
		})
		So(err, ShouldErrLike, "operating system is required and must be non-empty")
	})

	Convey("ok", t, func() {
		err := validateVMForCreation(&crimson.VM{
			Name: "vm",
			Host: "host",
			Vlan: 1,
			Os:   "os",
		})
		So(err, ShouldBeNil)
	})
}
