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

	"go.chromium.org/luci/machine-db/api/common/v1"
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
			VALUES \(\?, \(SELECT vlan_id FROM ips WHERE ipv4 = \? AND hostname_id IS NULL\)\)$
		`
		insertVMStmt := `
			^INSERT INTO vms \(hostname_id, physical_host_id, os_id, description, deployment_ticket, state\)
			VALUES \(
				\?,
				\(SELECT p.id FROM physical_hosts p, hostnames h WHERE p.hostname_id = h.id AND h.name = \? AND h.vlan_id = \?\),
				\(SELECT id FROM oses WHERE name = \?\),
				\?,
				\?,
				\?
			\)
		`
		updateIPStmt := `
			UPDATE ips
			SET hostname_id = \?
			WHERE ipv4 = \? AND hostname_id IS NULL
		`

		Convey("invalid IPv4", func() {
			vm := &crimson.VM{
				Name:     "vm",
				Host:     "host",
				HostVlan: 10,
				Os:       "os",
				Ipv4:     "127.0.0.1/20",
				State:    common.State_FREE,
			}
			So(createVM(c, vm), ShouldErrLike, "invalid IPv4 address")
		})

		Convey("begin failed", func() {
			vm := &crimson.VM{
				Name:     "vm",
				Host:     "host",
				HostVlan: 10,
				Os:       "os",
				Ipv4:     "127.0.0.1",
				State:    common.State_FREE,
			}
			m.ExpectBegin().WillReturnError(fmt.Errorf("error"))
			So(createVM(c, vm), ShouldErrLike, "Internal server error")
		})

		Convey("invalid IP", func() {
			vm := &crimson.VM{
				Name:     "vm",
				Host:     "host",
				HostVlan: 10,
				Os:       "os",
				Ipv4:     "127.0.0.1",
				State:    common.State_FREE,
			}
			m.ExpectBegin()
			m.ExpectExec(insertNameStmt).WithArgs(vm.Name, 2130706433).WillReturnError(&mysql.MySQLError{Number: mysqlerr.ER_BAD_NULL_ERROR, Message: "'vlan_id'"})
			m.ExpectRollback()
			So(createVM(c, vm), ShouldErrLike, "ensure IPv4 address")
		})

		Convey("duplicate hostname/VLAN", func() {
			vm := &crimson.VM{
				Name:     "vm",
				Host:     "host",
				HostVlan: 10,
				Os:       "os",
				Ipv4:     "127.0.0.1",
				State:    common.State_FREE,
			}
			m.ExpectBegin()
			m.ExpectExec(insertNameStmt).WithArgs(vm.Name, 2130706433).WillReturnError(&mysql.MySQLError{Number: mysqlerr.ER_DUP_ENTRY, Message: "'name'"})
			m.ExpectRollback()
			So(createVM(c, vm), ShouldErrLike, "duplicate hostname")
		})

		Convey("query failed", func() {
			vm := &crimson.VM{
				Name:     "vm",
				Host:     "host",
				HostVlan: 10,
				Os:       "os",
				Ipv4:     "127.0.0.1",
				State:    common.State_FREE,
			}
			m.ExpectBegin()
			m.ExpectExec(insertNameStmt).WithArgs(vm.Name, 2130706433).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectExec(updateIPStmt).WithArgs(1, 2130706433).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectExec(insertVMStmt).WithArgs(1, vm.Host, vm.HostVlan, vm.Os, vm.Description, vm.DeploymentTicket, vm.State).WillReturnError(fmt.Errorf("error"))
			m.ExpectRollback()
			So(createVM(c, vm), ShouldErrLike, "Internal server error")
		})

		Convey("invalid host", func() {
			vm := &crimson.VM{
				Name:     "vm",
				Host:     "host",
				HostVlan: 10,
				Os:       "os",
				Ipv4:     "127.0.0.1",
				State:    common.State_FREE,
			}
			m.ExpectBegin()
			m.ExpectExec(insertNameStmt).WithArgs(vm.Name, 2130706433).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectExec(updateIPStmt).WithArgs(1, 2130706433).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectExec(insertVMStmt).WithArgs(1, vm.Host, vm.HostVlan, vm.Os, vm.Description, vm.DeploymentTicket, vm.State).WillReturnError(&mysql.MySQLError{Number: mysqlerr.ER_BAD_NULL_ERROR, Message: "'physical_host_id' is null"})
			m.ExpectRollback()
			So(createVM(c, vm), ShouldErrLike, "unknown physical host")
		})

		Convey("invalid operating system", func() {
			vm := &crimson.VM{
				Name:     "vm",
				Host:     "host",
				HostVlan: 10,
				Os:       "os",
				Ipv4:     "127.0.0.1",
				State:    common.State_FREE,
			}
			m.ExpectBegin()
			m.ExpectExec(insertNameStmt).WithArgs(vm.Name, 2130706433).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectExec(updateIPStmt).WithArgs(1, 2130706433).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectExec(insertVMStmt).WithArgs(1, vm.Host, vm.HostVlan, vm.Os, vm.Description, vm.DeploymentTicket, vm.State).WillReturnError(&mysql.MySQLError{Number: mysqlerr.ER_BAD_NULL_ERROR, Message: "'os_id' is null"})
			m.ExpectRollback()
			So(createVM(c, vm), ShouldErrLike, "unknown operating system")
		})

		Convey("unexpected invalid", func() {
			vm := &crimson.VM{
				Name:     "vm",
				Host:     "host",
				HostVlan: 10,
				Os:       "os",
				Ipv4:     "127.0.0.1",
				State:    common.State_FREE,
			}
			m.ExpectBegin()
			m.ExpectExec(insertNameStmt).WithArgs(vm.Name, 2130706433).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectExec(updateIPStmt).WithArgs(1, 2130706433).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectExec(insertVMStmt).WithArgs(1, vm.Host, vm.HostVlan, vm.Os, vm.Description, vm.DeploymentTicket, vm.State).WillReturnError(&mysql.MySQLError{Number: mysqlerr.ER_BAD_NULL_ERROR, Message: "error"})
			m.ExpectRollback()
			So(createVM(c, vm), ShouldErrLike, "Internal server error")
		})

		Convey("unexpected error", func() {
			vm := &crimson.VM{
				Name:     "vm",
				Host:     "host",
				HostVlan: 10,
				Os:       "os",
				Ipv4:     "127.0.0.1",
				State:    common.State_FREE,
			}
			m.ExpectBegin()
			m.ExpectExec(insertNameStmt).WithArgs(vm.Name, 2130706433).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectExec(updateIPStmt).WithArgs(1, 2130706433).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectExec(insertVMStmt).WithArgs(1, vm.Host, vm.HostVlan, vm.Os, vm.Description, vm.DeploymentTicket, vm.State).WillReturnError(&mysql.MySQLError{Number: mysqlerr.ER_NO, Message: "name vlan_id"})
			m.ExpectRollback()
			So(createVM(c, vm), ShouldErrLike, "Internal server error")
		})

		Convey("commit failed", func() {
			vm := &crimson.VM{
				Name:     "vm",
				Host:     "host",
				HostVlan: 10,
				Os:       "os",
				Ipv4:     "127.0.0.1",
				State:    common.State_FREE,
			}
			m.ExpectBegin()
			m.ExpectExec(insertNameStmt).WithArgs(vm.Name, 2130706433).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectExec(updateIPStmt).WithArgs(1, 2130706433).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectExec(insertVMStmt).WithArgs(1, vm.Host, vm.HostVlan, vm.Os, vm.Description, vm.DeploymentTicket, vm.State).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectCommit().WillReturnError(fmt.Errorf("error"))
			m.ExpectRollback()
			So(createVM(c, vm), ShouldErrLike, "Internal server error")
		})

		Convey("ok", func() {
			vm := &crimson.VM{
				Name:     "vm",
				Host:     "host",
				HostVlan: 10,
				Os:       "os",
				Ipv4:     "127.0.0.1",
				State:    common.State_FREE,
			}
			m.ExpectBegin()
			m.ExpectExec(insertNameStmt).WithArgs(vm.Name, 2130706433).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectExec(updateIPStmt).WithArgs(1, 2130706433).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectExec(insertVMStmt).WithArgs(1, vm.Host, vm.HostVlan, vm.Os, vm.Description, vm.DeploymentTicket, vm.State).WillReturnResult(sqlmock.NewResult(1, 1))
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
		columns := []string{"hv.name", "hv.vlan_id", "hp.name", "hp.vlan_id", "o.name", "vm.description", "vm.deployment_ticket", "i.ipv4", "v.state"}
		rows := sqlmock.NewRows(columns)

		Convey("query failed", func() {
			selectStmt := `
				^SELECT hv.name, hv.vlan_id, hp.name, hp.vlan_id, o.name, v.description, v.deployment_ticket, i.ipv4, v.state
				FROM vms v, hostnames hv, physical_hosts p, hostnames hp, oses o, ips i
				WHERE v.hostname_id = hv.id AND v.physical_host_id = p.id AND p.hostname_id = hp.id AND v.os_id = o.id AND i.hostname_id = hv.id AND hv.name IN \(\?\) AND hv.vlan_id IN \(\?\)$
			`
			names := []string{"vm"}
			vlans := []int64{0}
			m.ExpectQuery(selectStmt).WithArgs(names[0], vlans[0]).WillReturnError(fmt.Errorf("error"))
			vms, err := listVMs(c, db, names, vlans)
			So(err, ShouldErrLike, "failed to fetch VMs")
			So(vms, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("no names", func() {
			selectStmt := `
				^SELECT hv.name, hv.vlan_id, hp.name, hp.vlan_id, o.name, v.description, v.deployment_ticket, i.ipv4, v.state
				FROM vms v, hostnames hv, physical_hosts p, hostnames hp, oses o, ips i
				WHERE v.hostname_id = hv.id AND v.physical_host_id = p.id AND p.hostname_id = hp.id AND v.os_id = o.id AND i.hostname_id = hv.id AND hv.vlan_id IN \(\?\)$
			`
			names := []string{}
			vlans := []int64{0}
			m.ExpectQuery(selectStmt).WithArgs(vlans[0]).WillReturnRows(rows)
			vms, err := listVMs(c, db, names, vlans)
			So(err, ShouldBeNil)
			So(vms, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("no vlans", func() {
			selectStmt := `
				^SELECT hv.name, hv.vlan_id, hp.name, hp.vlan_id, o.name, v.description, v.deployment_ticket, i.ipv4, v.state
				FROM vms v, hostnames hv, physical_hosts p, hostnames hp, oses o, ips i
				WHERE v.hostname_id = hv.id AND v.physical_host_id = p.id AND p.hostname_id = hp.id AND v.os_id = o.id AND i.hostname_id = hv.id AND hv.name IN \(\?\)$
			`
			names := []string{"vm"}
			vlans := []int64{}
			m.ExpectQuery(selectStmt).WithArgs(names[0]).WillReturnRows(rows)
			vms, err := listVMs(c, db, names, vlans)
			So(err, ShouldBeNil)
			So(vms, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("empty", func() {
			selectStmt := `
				^SELECT hv.name, hv.vlan_id, hp.name, hp.vlan_id, o.name, v.description, v.deployment_ticket, i.ipv4, v.state
				FROM vms v, hostnames hv, physical_hosts p, hostnames hp, oses o, ips i
				WHERE v.hostname_id = hv.id AND v.physical_host_id = p.id AND p.hostname_id = hp.id AND v.os_id = o.id AND i.hostname_id = hv.id AND hv.name IN \(\?\) AND hv.vlan_id IN \(\?\)$
			`
			names := []string{"vm"}
			vlans := []int64{0}
			m.ExpectQuery(selectStmt).WithArgs(names[0], vlans[0]).WillReturnRows(rows)
			vms, err := listVMs(c, db, names, vlans)
			So(err, ShouldBeNil)
			So(vms, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("non-empty", func() {
			selectStmt := `
				^SELECT hv.name, hv.vlan_id, hp.name, hp.vlan_id, o.name, v.description, v.deployment_ticket, i.ipv4, v.state
				FROM vms v, hostnames hv, physical_hosts p, hostnames hp, oses o, ips i
				WHERE v.hostname_id = hv.id AND v.physical_host_id = p.id AND p.hostname_id = hp.id AND v.os_id = o.id AND i.hostname_id = hv.id AND hv.name IN \(\?,\?\) AND hv.vlan_id IN \(\?\)$
			`
			names := []string{"vm 1", "vm 2"}
			vlans := []int64{0}
			rows.AddRow(names[0], vlans[0], "host 1", 10, "os 1", "", "", 1, 0)
			rows.AddRow(names[1], vlans[0], "host 2", 20, "os 2", "", "", 2, common.State_SERVING)
			m.ExpectQuery(selectStmt).WithArgs(names[0], names[1], vlans[0]).WillReturnRows(rows)
			vms, err := listVMs(c, db, names, vlans)
			So(err, ShouldBeNil)
			So(vms, ShouldResemble, []*crimson.VM{
				{
					Name:     names[0],
					Vlan:     vlans[0],
					Host:     "host 1",
					HostVlan: 10,
					Os:       "os 1",
					Ipv4:     "0.0.0.1",
				},
				{
					Name:     names[1],
					Vlan:     vlans[0],
					Host:     "host 2",
					HostVlan: 20,
					Os:       "os 2",
					Ipv4:     "0.0.0.2",
					State:    common.State_SERVING,
				},
			})
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("ok", func() {
			selectStmt := `
				^SELECT hv.name, hv.vlan_id, hp.name, hp.vlan_id, o.name, v.description, v.deployment_ticket, i.ipv4, v.state
				FROM vms v, hostnames hv, physical_hosts p, hostnames hp, oses o, ips i
				WHERE v.hostname_id = hv.id AND v.physical_host_id = p.id AND p.hostname_id = hp.id AND v.os_id = o.id AND i.hostname_id = hv.id$
			`
			names := []string{}
			vlans := []int64{}
			rows.AddRow("vm 1", 1, "host 1", 10, "os 1", "", "", 1, 0)
			rows.AddRow("vm 2", 2, "host 2", 20, "os 2", "", "", 2, common.State_SERVING)
			m.ExpectQuery(selectStmt).WillReturnRows(rows)
			vms, err := listVMs(c, db, names, vlans)
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
					State:    common.State_SERVING,
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
			Host:     "host",
			HostVlan: 10,
			Os:       "os",
			Ipv4:     "127.0.0.1",
			State:    common.State_FREE,
		})
		So(err, ShouldErrLike, "hostname is required and must be non-empty")
	})

	Convey("VLAN specified", t, func() {
		err := validateVMForCreation(&crimson.VM{
			Name:     "vm",
			Vlan:     1,
			Host:     "host",
			HostVlan: 10,
			Os:       "os",
			Ipv4:     "127.0.0.1",
			State:    common.State_FREE,
		})
		So(err, ShouldErrLike, "VLAN must not be specified, use IP address instead")
	})

	Convey("host unspecified", t, func() {
		err := validateVMForCreation(&crimson.VM{
			Name:     "vm",
			HostVlan: 10,
			Os:       "os",
			Ipv4:     "127.0.0.1",
			State:    common.State_FREE,
		})
		So(err, ShouldErrLike, "physical hostname is required and must be non-empty")
	})

	Convey("host VLAN unspecified", t, func() {
		err := validateVMForCreation(&crimson.VM{
			Name:  "vm",
			Host:  "host",
			Os:    "os",
			Ipv4:  "127.0.0.1",
			State: common.State_FREE,
		})
		So(err, ShouldErrLike, "host VLAN is required and must be positive")
	})

	Convey("host VLAN negative", t, func() {
		err := validateVMForCreation(&crimson.VM{
			Name:     "vm",
			Host:     "host",
			HostVlan: -10,
			Os:       "os",
			Ipv4:     "127.0.0.1",
			State:    common.State_FREE,
		})
		So(err, ShouldErrLike, "host VLAN is required and must be positive")
	})

	Convey("operating system unspecified", t, func() {
		err := validateVMForCreation(&crimson.VM{
			Name:     "vm",
			Host:     "host",
			HostVlan: 10,
			Ipv4:     "127.0.0.1",
			State:    common.State_FREE,
		})
		So(err, ShouldErrLike, "operating system is required and must be non-empty")
	})

	Convey("IPv4 address unspecified", t, func() {
		err := validateVMForCreation(&crimson.VM{
			Name:     "vm",
			Host:     "host",
			HostVlan: 10,
			Os:       "os",
			State:    common.State_FREE,
		})
		So(err, ShouldErrLike, "IPv4 address is required and must be non-empty")
	})

	Convey("IPv4 address invalid", t, func() {
		err := validateVMForCreation(&crimson.VM{
			Name:     "vm",
			Host:     "host",
			HostVlan: 10,
			Os:       "os",
			Ipv4:     "127.0.0.1/20",
			State:    common.State_FREE,
		})
		So(err, ShouldErrLike, "invalid IPv4 address")
	})

	Convey("state unspecified", t, func() {
		err := validateVMForCreation(&crimson.VM{
			Name:     "vm",
			Host:     "host",
			HostVlan: 10,
			Os:       "os",
			Ipv4:     "127.0.0.1",
		})
		So(err, ShouldErrLike, "state is required")
	})

	Convey("ok", t, func() {
		err := validateVMForCreation(&crimson.VM{
			Name:     "vm",
			Host:     "host",
			HostVlan: 10,
			Os:       "os",
			Ipv4:     "127.0.0.1",
			State:    common.State_FREE,
		})
		So(err, ShouldBeNil)
	})
}
