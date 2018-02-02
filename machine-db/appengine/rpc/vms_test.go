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
		insertNameStmt := `
			^INSERT INTO hostnames \(name, vlan_id\)
			VALUES \(\?, \?\)$
		`
		insertVMStmt := `
			^INSERT INTO vms \(hostname_id, physical_host_id, os_id, description, deployment_ticket\)
			VALUES \(
				\?,
				\(SELECT p.id FROM physical_hosts p, hostnames h WHERE p.hostname_id = h.id AND h.name = \? AND h.vlan_id = \?\),
				\(SELECT id FROM oses WHERE name = \?\),
				\?,
				\?
			\)
		`
		updateIPStmt := `
			UPDATE ips
			SET hostname_id = \?
			WHERE ipv4 = \? AND vlan_id = \? AND hostname_id IS NULL
		`

		Convey("invalid IPv4", func() {
			vm := &crimson.VM{
				Name:     "vm",
				Vlan:     1,
				Host:     "host",
				HostVlan: 10,
				Os:       "os",
				Ipv4:     "127.0.0.1/20",
			}
			So(createVM(c, vm), ShouldErrLike, "invalid IPv4 address")
		})

		Convey("begin failed", func() {
			vm := &crimson.VM{
				Name:     "vm",
				Vlan:     1,
				Host:     "host",
				HostVlan: 10,
				Os:       "os",
				Ipv4:     "127.0.0.1",
			}
			m.ExpectBegin().WillReturnError(fmt.Errorf("error"))
			So(createVM(c, vm), ShouldErrLike, "Internal server error")
		})

		Convey("duplicate hostname/VLAN", func() {
			vm := &crimson.VM{
				Name:     "vm",
				Vlan:     1,
				Host:     "host",
				HostVlan: 10,
				Os:       "os",
				Ipv4:     "127.0.0.1",
			}
			m.ExpectBegin()
			m.ExpectExec(insertNameStmt).WithArgs(vm.Name, vm.Vlan).WillReturnError(&mysql.MySQLError{Number: mysqlerr.ER_DUP_ENTRY, Message: "'name'"})
			m.ExpectRollback()
			So(createVM(c, vm), ShouldErrLike, "duplicate hostname")
		})

		Convey("invalid VLAN", func() {
			vm := &crimson.VM{
				Name:     "vm",
				Vlan:     1,
				Host:     "host",
				HostVlan: 10,
				Os:       "os",
				Ipv4:     "127.0.0.1",
			}
			m.ExpectBegin()
			m.ExpectExec(insertNameStmt).WithArgs(vm.Name, vm.Vlan).WillReturnError(&mysql.MySQLError{Number: mysqlerr.ER_NO_REFERENCED_ROW_2, Message: "`vlan_id`"})
			m.ExpectRollback()
			So(createVM(c, vm), ShouldErrLike, "unknown VLAN")
		})

		Convey("duplicate or invalid IP", func() {
			vm := &crimson.VM{
				Name:     "vm",
				Vlan:     1,
				Host:     "host",
				HostVlan: 10,
				Os:       "os",
				Ipv4:     "127.0.0.1",
			}
			m.ExpectBegin()
			m.ExpectExec(insertNameStmt).WithArgs(vm.Name, vm.Vlan).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectExec(updateIPStmt).WithArgs(1, 2130706433, vm.Vlan).WillReturnResult(sqlmock.NewResult(1, 0))
			m.ExpectRollback()
			So(createVM(c, vm), ShouldErrLike, "ensure IP address")
		})

		Convey("query failed", func() {
			vm := &crimson.VM{
				Name:     "vm",
				Vlan:     1,
				Host:     "host",
				HostVlan: 10,
				Os:       "os",
				Ipv4:     "127.0.0.1",
			}
			m.ExpectBegin()
			m.ExpectExec(insertNameStmt).WithArgs(vm.Name, vm.Vlan).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectExec(updateIPStmt).WithArgs(1, 2130706433, vm.Vlan).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectExec(insertVMStmt).WithArgs(1, vm.Host, vm.HostVlan, vm.Os, vm.Description, vm.DeploymentTicket).WillReturnError(fmt.Errorf("error"))
			m.ExpectRollback()
			So(createVM(c, vm), ShouldErrLike, "Internal server error")
		})

		Convey("invalid host", func() {
			vm := &crimson.VM{
				Name:     "vm",
				Vlan:     1,
				Host:     "host",
				HostVlan: 10,
				Os:       "os",
				Ipv4:     "127.0.0.1",
			}
			m.ExpectBegin()
			m.ExpectExec(insertNameStmt).WithArgs(vm.Name, vm.Vlan).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectExec(updateIPStmt).WithArgs(1, 2130706433, vm.Vlan).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectExec(insertVMStmt).WithArgs(1, vm.Host, vm.HostVlan, vm.Os, vm.Description, vm.DeploymentTicket).WillReturnError(&mysql.MySQLError{Number: mysqlerr.ER_BAD_NULL_ERROR, Message: "'physical_host_id' is null"})
			m.ExpectRollback()
			So(createVM(c, vm), ShouldErrLike, "unknown physical host")
		})

		Convey("invalid operating system", func() {
			vm := &crimson.VM{
				Name:     "vm",
				Vlan:     1,
				Host:     "host",
				HostVlan: 10,
				Os:       "os",
				Ipv4:     "127.0.0.1",
			}
			m.ExpectBegin()
			m.ExpectExec(insertNameStmt).WithArgs(vm.Name, vm.Vlan).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectExec(updateIPStmt).WithArgs(1, 2130706433, vm.Vlan).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectExec(insertVMStmt).WithArgs(1, vm.Host, vm.HostVlan, vm.Os, vm.Description, vm.DeploymentTicket).WillReturnError(&mysql.MySQLError{Number: mysqlerr.ER_BAD_NULL_ERROR, Message: "'os_id' is null"})
			m.ExpectRollback()
			So(createVM(c, vm), ShouldErrLike, "unknown operating system")
		})

		Convey("unexpected invalid", func() {
			vm := &crimson.VM{
				Name:     "vm",
				Vlan:     1,
				Host:     "host",
				HostVlan: 10,
				Os:       "os",
				Ipv4:     "127.0.0.1",
			}
			m.ExpectBegin()
			m.ExpectExec(insertNameStmt).WithArgs(vm.Name, vm.Vlan).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectExec(updateIPStmt).WithArgs(1, 2130706433, vm.Vlan).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectExec(insertVMStmt).WithArgs(1, vm.Host, vm.HostVlan, vm.Os, vm.Description, vm.DeploymentTicket).WillReturnError(&mysql.MySQLError{Number: mysqlerr.ER_BAD_NULL_ERROR, Message: "error"})
			m.ExpectRollback()
			So(createVM(c, vm), ShouldErrLike, "Internal server error")
		})

		Convey("unexpected error", func() {
			vm := &crimson.VM{
				Name:     "vm",
				Vlan:     1,
				Host:     "host",
				HostVlan: 10,
				Os:       "os",
				Ipv4:     "127.0.0.1",
			}
			m.ExpectBegin()
			m.ExpectExec(insertNameStmt).WithArgs(vm.Name, vm.Vlan).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectExec(updateIPStmt).WithArgs(1, 2130706433, vm.Vlan).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectExec(insertVMStmt).WithArgs(1, vm.Host, vm.HostVlan, vm.Os, vm.Description, vm.DeploymentTicket).WillReturnError(&mysql.MySQLError{Number: mysqlerr.ER_NO, Message: "name vlan_id"})
			m.ExpectRollback()
			So(createVM(c, vm), ShouldErrLike, "Internal server error")
		})

		Convey("commit failed", func() {
			vm := &crimson.VM{
				Name:     "vm",
				Vlan:     1,
				Host:     "host",
				HostVlan: 10,
				Os:       "os",
				Ipv4:     "127.0.0.1",
			}
			m.ExpectBegin()
			m.ExpectExec(insertNameStmt).WithArgs(vm.Name, vm.Vlan).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectExec(updateIPStmt).WithArgs(1, 2130706433, vm.Vlan).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectExec(insertVMStmt).WithArgs(1, vm.Host, vm.HostVlan, vm.Os, vm.Description, vm.DeploymentTicket).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectCommit().WillReturnError(fmt.Errorf("error"))
			m.ExpectRollback()
			So(createVM(c, vm), ShouldErrLike, "Internal server error")
		})

		Convey("ok", func() {
			vm := &crimson.VM{
				Name:     "vm",
				Vlan:     1,
				Host:     "host",
				HostVlan: 10,
				Os:       "os",
				Ipv4:     "127.0.0.1",
			}
			m.ExpectBegin()
			m.ExpectExec(insertNameStmt).WithArgs(vm.Name, vm.Vlan).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectExec(updateIPStmt).WithArgs(1, 2130706433, vm.Vlan).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectExec(insertVMStmt).WithArgs(1, vm.Host, vm.HostVlan, vm.Os, vm.Description, vm.DeploymentTicket).WillReturnResult(sqlmock.NewResult(1, 1))
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
			^SELECT hv.name, hv.vlan_id, hp.name, hp.vlan_id, o.name, v.description, v.deployment_ticket, i.ipv4
			FROM vms v, hostnames hv, physical_hosts p, hostnames hp, oses o, ips i
			WHERE v.hostname_id = hv.id AND v.physical_host_id = p.id AND p.hostname_id = hp.id AND v.os_id = o.id AND i.hostname_id = hv.id$
		`
		columns := []string{"hv.name", "hv.vlan_id", "hp.name", "hp.vlan_id", "o.name", "vm.description", "vm.deployment_ticket", "i.ipv4"}
		rows := sqlmock.NewRows(columns)

		Convey("query failed", func() {
			names := stringset.NewFromSlice("vm")
			vlans := map[int64]struct{}{0: {}}
			m.ExpectQuery(selectStmt).WillReturnError(fmt.Errorf("error"))
			vms, err := listVMs(c, names, vlans)
			So(err, ShouldErrLike, "failed to fetch VMs")
			So(vms, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("empty", func() {
			names := stringset.NewFromSlice("vm")
			vlans := map[int64]struct{}{0: {}}
			m.ExpectQuery(selectStmt).WillReturnRows(rows)
			vms, err := listVMs(c, names, vlans)
			So(err, ShouldBeNil)
			So(vms, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("no matches", func() {
			names := stringset.NewFromSlice("vm")
			vlans := map[int64]struct{}{0: {}}
			m.ExpectQuery(selectStmt).WillReturnRows(rows)
			rows.AddRow("vm 1", 1, "host 1", 10, "os 1", "", "", 1)
			rows.AddRow("vm 2", 2, "host 2", 20, "os 2", "", "", 2)
			vms, err := listVMs(c, names, vlans)
			So(err, ShouldBeNil)
			So(vms, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("matches", func() {
			names := stringset.NewFromSlice("vm 1", "vm 2")
			vlans := map[int64]struct{}{1: {}, 2: {}}
			rows.AddRow("vm 1", 1, "host 1", 10, "os 1", "", "", 1)
			rows.AddRow("vm 2", 2, "host 2", 20, "os 2", "", "", 2)
			rows.AddRow("vm 3", 3, "host 3", 30, "os 3", "", "", 3)
			m.ExpectQuery(selectStmt).WillReturnRows(rows)
			vms, err := listVMs(c, names, vlans)
			So(err, ShouldBeNil)
			So(vms, ShouldResemble, []*crimson.VM{
				{
					Name:     "vm 1",
					Vlan:     1,
					Host:     "host 1",
					HostVlan: 10,
					Os:       "os 1",
					Ipv4:     "0.0.0.1",
				},
				{
					Name:     "vm 2",
					Vlan:     2,
					Host:     "host 2",
					HostVlan: 20,
					Os:       "os 2",
					Ipv4:     "0.0.0.2",
				},
			})
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("ok", func() {
			names := stringset.New(0)
			vlans := map[int64]struct{}{}
			rows.AddRow("vm 1", 1, "host 1", 10, "os 1", "", "", 1)
			rows.AddRow("vm 2", 2, "host 2", 20, "os 2", "", "", 2)
			m.ExpectQuery(selectStmt).WillReturnRows(rows)
			vms, err := listVMs(c, names, vlans)
			So(err, ShouldBeNil)
			So(vms, ShouldResemble, []*crimson.VM{
				{
					Name:     "vm 1",
					Vlan:     1,
					Host:     "host 1",
					HostVlan: 10,
					Os:       "os 1",
					Ipv4:     "0.0.0.1",
				},
				{
					Name:     "vm 2",
					Vlan:     2,
					Host:     "host 2",
					HostVlan: 20,
					Os:       "os 2",
					Ipv4:     "0.0.0.2",
				},
			})
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})
	})
}

func TestValidateVMForCreation(t *testing.T) {
	t.Parallel()

	Convey("vm unspecified", t, func() {
		err := validateVMForCreation(nil)
		So(err, ShouldErrLike, "VM specification is required")
	})

	Convey("hostname unspecified", t, func() {
		err := validateVMForCreation(&crimson.VM{
			Vlan:     1,
			Host:     "host",
			HostVlan: 10,
			Os:       "os",
		})
		So(err, ShouldErrLike, "hostname is required and must be non-empty")
	})

	Convey("VLAN unspecified", t, func() {
		err := validateVMForCreation(&crimson.VM{
			Name:     "vm",
			Host:     "host",
			HostVlan: 10,
			Os:       "os",
		})
		So(err, ShouldErrLike, "VLAN is required and must be positive")
	})

	Convey("VLAN negative", t, func() {
		err := validateVMForCreation(&crimson.VM{
			Name:     "vm",
			Vlan:     -1,
			Host:     "host",
			HostVlan: 10,
			Os:       "os",
		})
		So(err, ShouldErrLike, "VLAN is required and must be positive")
	})

	Convey("host unspecified", t, func() {
		err := validateVMForCreation(&crimson.VM{
			Name:     "vm",
			Vlan:     1,
			HostVlan: 10,
			Os:       "os",
		})
		So(err, ShouldErrLike, "physical hostname is required and must be non-empty")
	})

	Convey("host VLAN unspecified", t, func() {
		err := validateVMForCreation(&crimson.VM{
			Name: "vm",
			Vlan: 1,
			Host: "host",
			Os:   "os",
		})
		So(err, ShouldErrLike, "host VLAN is required and must be positive")
	})

	Convey("host VLAN negative", t, func() {
		err := validateVMForCreation(&crimson.VM{
			Name:     "vm",
			Vlan:     1,
			Host:     "host",
			HostVlan: -10,
			Os:       "os",
		})
		So(err, ShouldErrLike, "host VLAN is required and must be positive")
	})

	Convey("operating system unspecified", t, func() {
		err := validateVMForCreation(&crimson.VM{
			Name:     "vm",
			Vlan:     1,
			Host:     "host",
			HostVlan: 10,
		})
		So(err, ShouldErrLike, "operating system is required and must be non-empty")
	})

	Convey("ok", t, func() {
		err := validateVMForCreation(&crimson.VM{
			Name:     "vm",
			Vlan:     1,
			Host:     "host",
			HostVlan: 10,
			Os:       "os",
		})
		So(err, ShouldBeNil)
	})
}
