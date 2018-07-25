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

	"google.golang.org/genproto/protobuf/field_mask"

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
				\(SELECT p.id FROM physical_hosts p, hostnames h WHERE p.hostname_id = h.id AND h.name = \?\),
				\(SELECT id FROM oses WHERE name = \?\),
				\?,
				\?,
				\?
			\)
		`
		updateIPStmt := `
			^UPDATE ips
			SET hostname_id = \?
			WHERE ipv4 = \? AND hostname_id IS NULL$
		`
		selectStmt := `
			^SELECT hv.name, hv.vlan_id, hp.name, hp.vlan_id, o.name, v.description, v.deployment_ticket, i.ipv4, v.state
			FROM vms v, hostnames hv, physical_hosts p, hostnames hp, oses o, ips i
			WHERE v.hostname_id = hv.id AND v.physical_host_id = p.id AND p.hostname_id = hp.id AND v.os_id = o.id AND i.hostname_id = hv.id AND hv.name IN \(\?\)$
		`
		columns := []string{"hv.name", "hv.vlan_id", "hp.name", "hp.vlan_id", "o.name", "vm.description", "vm.deployment_ticket", "i.ipv4", "v.state"}
		rows := sqlmock.NewRows(columns)

		Convey("invalid IPv4", func() {
			vm := &crimson.VM{
				Name:  "vm",
				Host:  "host",
				Os:    "os",
				Ipv4:  "127.0.0.1/20",
				State: common.State_FREE,
			}
			res, err := createVM(c, vm)
			So(err, ShouldErrLike, "invalid IPv4 address")
			So(res, ShouldBeNil)
		})

		Convey("begin failed", func() {
			vm := &crimson.VM{
				Name:  "vm",
				Host:  "host",
				Os:    "os",
				Ipv4:  "127.0.0.1",
				State: common.State_FREE,
			}
			m.ExpectBegin().WillReturnError(fmt.Errorf("error"))
			res, err := createVM(c, vm)
			So(err, ShouldErrLike, "failed to begin transaction")
			So(res, ShouldBeNil)
		})

		Convey("invalid IP", func() {
			vm := &crimson.VM{
				Name:  "vm",
				Host:  "host",
				Os:    "os",
				Ipv4:  "127.0.0.1",
				State: common.State_FREE,
			}
			m.ExpectBegin()
			m.ExpectExec(insertNameStmt).WithArgs(vm.Name, 2130706433).WillReturnError(&mysql.MySQLError{Number: mysqlerr.ER_BAD_NULL_ERROR, Message: "'vlan_id'"})
			m.ExpectRollback()
			res, err := createVM(c, vm)
			So(err, ShouldErrLike, "ensure IPv4 address")
			So(res, ShouldBeNil)
		})

		Convey("duplicate hostname", func() {
			vm := &crimson.VM{
				Name:  "vm",
				Host:  "host",
				Os:    "os",
				Ipv4:  "127.0.0.1",
				State: common.State_FREE,
			}
			m.ExpectBegin()
			m.ExpectExec(insertNameStmt).WithArgs(vm.Name, 2130706433).WillReturnError(&mysql.MySQLError{Number: mysqlerr.ER_DUP_ENTRY, Message: "'name'"})
			m.ExpectRollback()
			res, err := createVM(c, vm)
			So(err, ShouldErrLike, "duplicate hostname")
			So(res, ShouldBeNil)
		})

		Convey("query failed", func() {
			vm := &crimson.VM{
				Name:  "vm",
				Host:  "host",
				Os:    "os",
				Ipv4:  "127.0.0.1",
				State: common.State_FREE,
			}
			m.ExpectBegin()
			m.ExpectExec(insertNameStmt).WithArgs(vm.Name, 2130706433).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectExec(updateIPStmt).WithArgs(1, 2130706433).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectExec(insertVMStmt).WithArgs(1, vm.Host, vm.Os, vm.Description, vm.DeploymentTicket, vm.State).WillReturnError(fmt.Errorf("error"))
			m.ExpectRollback()
			res, err := createVM(c, vm)
			So(err, ShouldErrLike, "failed to create VM")
			So(res, ShouldBeNil)
		})

		Convey("invalid host", func() {
			vm := &crimson.VM{
				Name:  "vm",
				Host:  "host",
				Os:    "os",
				Ipv4:  "127.0.0.1",
				State: common.State_FREE,
			}
			m.ExpectBegin()
			m.ExpectExec(insertNameStmt).WithArgs(vm.Name, 2130706433).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectExec(updateIPStmt).WithArgs(1, 2130706433).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectExec(insertVMStmt).WithArgs(1, vm.Host, vm.Os, vm.Description, vm.DeploymentTicket, vm.State).WillReturnError(&mysql.MySQLError{Number: mysqlerr.ER_BAD_NULL_ERROR, Message: "'physical_host_id' is null"})
			m.ExpectRollback()
			res, err := createVM(c, vm)
			So(err, ShouldErrLike, "does not exist")
			So(res, ShouldBeNil)
		})

		Convey("invalid operating system", func() {
			vm := &crimson.VM{
				Name:  "vm",
				Host:  "host",
				Os:    "os",
				Ipv4:  "127.0.0.1",
				State: common.State_FREE,
			}
			m.ExpectBegin()
			m.ExpectExec(insertNameStmt).WithArgs(vm.Name, 2130706433).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectExec(updateIPStmt).WithArgs(1, 2130706433).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectExec(insertVMStmt).WithArgs(1, vm.Host, vm.Os, vm.Description, vm.DeploymentTicket, vm.State).WillReturnError(&mysql.MySQLError{Number: mysqlerr.ER_BAD_NULL_ERROR, Message: "'os_id' is null"})
			m.ExpectRollback()
			res, err := createVM(c, vm)
			So(err, ShouldErrLike, "does not exist")
			So(res, ShouldBeNil)
		})

		Convey("unexpected invalid", func() {
			vm := &crimson.VM{
				Name:  "vm",
				Host:  "host",
				Os:    "os",
				Ipv4:  "127.0.0.1",
				State: common.State_FREE,
			}
			m.ExpectBegin()
			m.ExpectExec(insertNameStmt).WithArgs(vm.Name, 2130706433).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectExec(updateIPStmt).WithArgs(1, 2130706433).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectExec(insertVMStmt).WithArgs(1, vm.Host, vm.Os, vm.Description, vm.DeploymentTicket, vm.State).WillReturnError(&mysql.MySQLError{Number: mysqlerr.ER_BAD_NULL_ERROR, Message: "error"})
			m.ExpectRollback()
			res, err := createVM(c, vm)
			So(err, ShouldErrLike, "failed to create VM")
			So(res, ShouldBeNil)
		})

		Convey("unexpected error", func() {
			vm := &crimson.VM{
				Name:  "vm",
				Host:  "host",
				Os:    "os",
				Ipv4:  "127.0.0.1",
				State: common.State_FREE,
			}
			m.ExpectBegin()
			m.ExpectExec(insertNameStmt).WithArgs(vm.Name, 2130706433).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectExec(updateIPStmt).WithArgs(1, 2130706433).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectExec(insertVMStmt).WithArgs(1, vm.Host, vm.Os, vm.Description, vm.DeploymentTicket, vm.State).WillReturnError(&mysql.MySQLError{Number: mysqlerr.ER_NO, Message: "name vlan_id"})
			m.ExpectRollback()
			res, err := createVM(c, vm)
			So(err, ShouldErrLike, "failed to create VM")
			So(res, ShouldBeNil)
		})

		Convey("commit failed", func() {
			vm := &crimson.VM{
				Name:  "vm",
				Host:  "host",
				Os:    "os",
				Ipv4:  "127.0.0.1",
				State: common.State_FREE,
			}
			rows.AddRow(vm.Name, vm.Vlan, vm.Host, 10, vm.Os, vm.Description, vm.DeploymentTicket, 2130706433, vm.State)
			m.ExpectBegin()
			m.ExpectExec(insertNameStmt).WithArgs(vm.Name, 2130706433).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectExec(updateIPStmt).WithArgs(1, 2130706433).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectExec(insertVMStmt).WithArgs(1, vm.Host, vm.Os, vm.Description, vm.DeploymentTicket, vm.State).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectQuery(selectStmt).WithArgs(vm.Name).WillReturnRows(rows)
			m.ExpectCommit().WillReturnError(fmt.Errorf("error"))
			m.ExpectRollback()
			res, err := createVM(c, vm)
			So(err, ShouldErrLike, "failed to commit transaction")
			So(res, ShouldBeNil)
		})

		Convey("ok", func() {
			vm := &crimson.VM{
				Name:  "vm",
				Host:  "host",
				Os:    "os",
				Ipv4:  "127.0.0.1",
				State: common.State_FREE,
			}
			rows.AddRow(vm.Name, 1, vm.Host, 10, vm.Os, vm.Description, vm.DeploymentTicket, 2130706433, vm.State)
			m.ExpectBegin()
			m.ExpectExec(insertNameStmt).WithArgs(vm.Name, 2130706433).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectExec(updateIPStmt).WithArgs(1, 2130706433).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectExec(insertVMStmt).WithArgs(1, vm.Host, vm.Os, vm.Description, vm.DeploymentTicket, vm.State).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectQuery(selectStmt).WithArgs(vm.Name).WillReturnRows(rows)
			m.ExpectCommit()
			res, err := createVM(c, vm)
			So(err, ShouldBeNil)
			So(res, ShouldResemble, &crimson.VM{
				Name:     vm.Name,
				Vlan:     1,
				Host:     vm.Host,
				HostVlan: 10,
				Os:       vm.Os,
				Ipv4:     vm.Ipv4,
				State:    vm.State,
			})
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

		Convey("invalid IPv4 address", func() {
			req := &crimson.ListVMsRequest{
				Names: []string{"vm"},
				Vlans: []int64{0},
				Ipv4S: []string{"0.0.0.1", "0.0.0.2/20"},
			}
			vms, err := listVMs(c, db, req)
			So(err, ShouldErrLike, "invalid IPv4 address")
			So(vms, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("query failed", func() {
			selectStmt := `
				^SELECT hv.name, hv.vlan_id, hp.name, hp.vlan_id, o.name, v.description, v.deployment_ticket, i.ipv4, v.state
				FROM vms v, hostnames hv, physical_hosts p, hostnames hp, oses o, ips i
				WHERE v.hostname_id = hv.id AND v.physical_host_id = p.id AND p.hostname_id = hp.id AND v.os_id = o.id AND i.hostname_id = hv.id
					AND hv.name IN \(\?\) AND hv.vlan_id IN \(\?\) AND i.ipv4 IN \(\?,\?\)$
			`
			req := &crimson.ListVMsRequest{
				Names: []string{"vm"},
				Vlans: []int64{0},
				Ipv4S: []string{"0.0.0.1", "0.0.0.2"},
			}
			m.ExpectQuery(selectStmt).WithArgs(req.Names[0], req.Vlans[0], 1, 2).WillReturnError(fmt.Errorf("error"))
			vms, err := listVMs(c, db, req)
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
			req := &crimson.ListVMsRequest{
				Vlans: []int64{0},
			}
			m.ExpectQuery(selectStmt).WithArgs(req.Vlans[0]).WillReturnRows(rows)
			vms, err := listVMs(c, db, req)
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
			req := &crimson.ListVMsRequest{
				Names: []string{"vm"},
			}
			m.ExpectQuery(selectStmt).WithArgs(req.Names[0]).WillReturnRows(rows)
			vms, err := listVMs(c, db, req)
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
			req := &crimson.ListVMsRequest{
				Names: []string{"vm"},
				Vlans: []int64{0},
			}
			m.ExpectQuery(selectStmt).WithArgs(req.Names[0], req.Vlans[0]).WillReturnRows(rows)
			vms, err := listVMs(c, db, req)
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
			req := &crimson.ListVMsRequest{
				Names: []string{"vm 1", "vm 2"},
				Vlans: []int64{0},
			}
			rows.AddRow(req.Names[0], req.Vlans[0], "host 1", 10, "os 1", "", "", 1, 0)
			rows.AddRow(req.Names[1], req.Vlans[0], "host 2", 20, "os 2", "", "", 2, common.State_SERVING)
			m.ExpectQuery(selectStmt).WithArgs(req.Names[0], req.Names[1], req.Vlans[0]).WillReturnRows(rows)
			vms, err := listVMs(c, db, req)
			So(err, ShouldBeNil)
			So(vms, ShouldResemble, []*crimson.VM{
				{
					Name:     req.Names[0],
					Vlan:     req.Vlans[0],
					Host:     "host 1",
					HostVlan: 10,
					Os:       "os 1",
					Ipv4:     "0.0.0.1",
				},
				{
					Name:     req.Names[1],
					Vlan:     req.Vlans[0],
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
			req := &crimson.ListVMsRequest{}
			rows.AddRow("vm 1", 1, "host 1", 10, "os 1", "", "", 1, 0)
			rows.AddRow("vm 2", 2, "host 2", 20, "os 2", "", "", 2, common.State_SERVING)
			m.ExpectQuery(selectStmt).WillReturnRows(rows)
			vms, err := listVMs(c, db, req)
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

func TestUpdateVM(t *testing.T) {
	Convey("updateVM", t, func() {
		db, m, _ := sqlmock.New()
		defer db.Close()
		c := database.With(context.Background(), db)
		selectStmt := `
			^SELECT hv.name, hv.vlan_id, hp.name, hp.vlan_id, o.name, v.description, v.deployment_ticket, i.ipv4, v.state
			FROM vms v, hostnames hv, physical_hosts p, hostnames hp, oses o, ips i
			WHERE v.hostname_id = hv.id AND v.physical_host_id = p.id AND p.hostname_id = hp.id AND v.os_id = o.id AND i.hostname_id = hv.id AND hv.name IN \(\?\)$
		`
		columns := []string{"hv.name", "hv.vlan_id", "hp.name", "hp.vlan_id", "o.name", "vm.description", "vm.deployment_ticket", "i.ipv4", "v.state"}
		rows := sqlmock.NewRows(columns)

		Convey("update host", func() {
			updateStmt := `
				^UPDATE vms
				SET physical_host_id = \(SELECT id FROM physical_hosts WHERE hostname_id = \(SELECT id FROM hostnames WHERE name = \?\)\)
				WHERE hostname_id = \(SELECT id FROM hostnames WHERE name = \?\)$
			`
			vm := &crimson.VM{
				Name:  "vm",
				Host:  "host",
				Os:    "operating system",
				State: common.State_SERVING,
				Ipv4:  "0.0.0.1",
			}
			mask := &field_mask.FieldMask{
				Paths: []string{
					"host",
				},
			}
			rows.AddRow(vm.Name, 1, vm.Host, 10, vm.Os, vm.Description, vm.DeploymentTicket, 1, vm.State)
			m.ExpectBegin()
			m.ExpectExec(updateStmt).WithArgs(vm.Host, vm.Name).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectQuery(selectStmt).WillReturnRows(rows)
			m.ExpectCommit()
			vm, err := updateVM(c, vm, mask)
			So(err, ShouldBeNil)
			So(vm, ShouldResemble, &crimson.VM{
				Name:     vm.Name,
				Vlan:     1,
				Host:     vm.Host,
				HostVlan: 10,
				Os:       vm.Os,
				State:    vm.State,
				Ipv4:     vm.Ipv4,
			})
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("update operating system", func() {
			updateStmt := `
				^UPDATE vms
				SET os_id = \(SELECT id FROM oses WHERE name = \?\)
				WHERE hostname_id = \(SELECT id FROM hostnames WHERE name = \?\)$
			`
			vm := &crimson.VM{
				Name:  "vm",
				Host:  "host",
				Os:    "operating system",
				State: common.State_SERVING,
				Ipv4:  "0.0.0.1",
			}
			mask := &field_mask.FieldMask{
				Paths: []string{
					"os",
				},
			}
			rows.AddRow(vm.Name, 1, vm.Host, 10, vm.Os, vm.Description, vm.DeploymentTicket, 1, vm.State)
			m.ExpectBegin()
			m.ExpectExec(updateStmt).WithArgs(vm.Os, vm.Name).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectQuery(selectStmt).WillReturnRows(rows)
			m.ExpectCommit()
			vm, err := updateVM(c, vm, mask)
			So(err, ShouldBeNil)
			So(vm, ShouldResemble, &crimson.VM{
				Name:     vm.Name,
				Vlan:     1,
				Host:     vm.Host,
				HostVlan: 10,
				Os:       vm.Os,
				State:    vm.State,
				Ipv4:     vm.Ipv4,
			})
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("update state", func() {
			updateStmt := `
				^UPDATE vms
				SET state = \?
				WHERE hostname_id = \(SELECT id FROM hostnames WHERE name = \?\)$
			`
			vm := &crimson.VM{
				Name:  "vm",
				Host:  "host",
				Os:    "operating system",
				State: common.State_SERVING,
				Ipv4:  "0.0.0.1",
			}
			mask := &field_mask.FieldMask{
				Paths: []string{
					"state",
				},
			}
			rows.AddRow(vm.Name, 1, vm.Host, 10, vm.Os, vm.Description, vm.DeploymentTicket, 1, vm.State)
			m.ExpectBegin()
			m.ExpectExec(updateStmt).WithArgs(vm.State, vm.Name).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectQuery(selectStmt).WillReturnRows(rows)
			m.ExpectCommit()
			vm, err := updateVM(c, vm, mask)
			So(err, ShouldBeNil)
			So(vm, ShouldResemble, &crimson.VM{
				Name:     vm.Name,
				Vlan:     1,
				Host:     vm.Host,
				HostVlan: 10,
				Os:       vm.Os,
				State:    vm.State,
				Ipv4:     vm.Ipv4,
			})
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("ok", func() {
			updateStmt := `
				^UPDATE vms
				SET physical_host_id = \(SELECT id FROM physical_hosts WHERE hostname_id = \(SELECT id FROM hostnames WHERE name = \?\)\),
					os_id = \(SELECT id FROM oses WHERE name = \?\),
					state = \?
				WHERE hostname_id = \(SELECT id FROM hostnames WHERE name = \?\)$
			`
			vm := &crimson.VM{
				Name:  "vm",
				Host:  "host",
				Os:    "operating system",
				State: common.State_SERVING,
				Ipv4:  "0.0.0.1",
			}
			mask := &field_mask.FieldMask{
				Paths: []string{
					"host",
					"os",
					"state",
				},
			}
			rows.AddRow(vm.Name, 1, vm.Host, 10, vm.Os, vm.Description, vm.DeploymentTicket, 1, vm.State)
			m.ExpectBegin()
			m.ExpectExec(updateStmt).WithArgs(vm.Host, vm.Os, vm.State, vm.Name).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectQuery(selectStmt).WillReturnRows(rows)
			m.ExpectCommit()
			vm, err := updateVM(c, vm, mask)
			So(err, ShouldBeNil)
			So(vm, ShouldResemble, &crimson.VM{
				Name:     vm.Name,
				Vlan:     1,
				Host:     vm.Host,
				HostVlan: 10,
				Os:       vm.Os,
				State:    vm.State,
				Ipv4:     vm.Ipv4,
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
			Host:  "host",
			Os:    "os",
			Ipv4:  "127.0.0.1",
			State: common.State_FREE,
		})
		So(err, ShouldErrLike, "hostname is required and must be non-empty")
	})

	Convey("VLAN specified", t, func() {
		err := validateVMForCreation(&crimson.VM{
			Name:  "vm",
			Vlan:  1,
			Host:  "host",
			Os:    "os",
			Ipv4:  "127.0.0.1",
			State: common.State_FREE,
		})
		So(err, ShouldErrLike, "VLAN must not be specified, use IP address instead")
	})

	Convey("host unspecified", t, func() {
		err := validateVMForCreation(&crimson.VM{
			Name:  "vm",
			Os:    "os",
			Ipv4:  "127.0.0.1",
			State: common.State_FREE,
		})
		So(err, ShouldErrLike, "physical hostname is required and must be non-empty")
	})

	Convey("host VLAN specified", t, func() {
		err := validateVMForCreation(&crimson.VM{
			Name:     "vm",
			Host:     "host",
			HostVlan: 10,
			Os:       "os",
			Ipv4:     "127.0.0.1",
			State:    common.State_FREE,
		})
		So(err, ShouldErrLike, "host VLAN must not be specified, use physical hostname instead")
	})

	Convey("operating system unspecified", t, func() {
		err := validateVMForCreation(&crimson.VM{
			Name:  "vm",
			Host:  "host",
			Ipv4:  "127.0.0.1",
			State: common.State_FREE,
		})
		So(err, ShouldErrLike, "operating system is required and must be non-empty")
	})

	Convey("IPv4 address unspecified", t, func() {
		err := validateVMForCreation(&crimson.VM{
			Name:  "vm",
			Host:  "host",
			Os:    "os",
			State: common.State_FREE,
		})
		So(err, ShouldErrLike, "IPv4 address is required and must be non-empty")
	})

	Convey("IPv4 address invalid", t, func() {
		err := validateVMForCreation(&crimson.VM{
			Name:  "vm",
			Host:  "host",
			Os:    "os",
			Ipv4:  "127.0.0.1/20",
			State: common.State_FREE,
		})
		So(err, ShouldErrLike, "invalid IPv4 address")
	})

	Convey("state unspecified", t, func() {
		err := validateVMForCreation(&crimson.VM{
			Name: "vm",
			Host: "host",
			Os:   "os",
			Ipv4: "127.0.0.1",
		})
		So(err, ShouldErrLike, "state is required")
	})

	Convey("ok", t, func() {
		err := validateVMForCreation(&crimson.VM{
			Name:  "vm",
			Host:  "host",
			Os:    "os",
			Ipv4:  "127.0.0.1",
			State: common.State_FREE,
		})
		So(err, ShouldBeNil)
	})
}

func TestValidateVMForUpdate(t *testing.T) {
	t.Parallel()

	Convey("host unspecified", t, func() {
		err := validateVMForUpdate(nil, &field_mask.FieldMask{
			Paths: []string{
				"host",
				"os",
				"state",
			},
		})
		So(err, ShouldErrLike, "VM specification is required")
	})

	Convey("hostname unspecified", t, func() {
		err := validateVMForUpdate(&crimson.VM{
			Name:  "vm",
			Vlan:  1,
			Os:    "os",
			State: common.State_SERVING,
		}, &field_mask.FieldMask{
			Paths: []string{
				"host",
				"os",
				"state",
			},
		})
		So(err, ShouldErrLike, "hostname is required and must be non-empty")
	})

	Convey("mask unspecified", t, func() {
		err := validateVMForUpdate(&crimson.VM{
			Name:  "vm",
			Host:  "host",
			Os:    "os",
			State: common.State_SERVING,
		}, nil)
		So(err, ShouldErrLike, "update mask is required")
	})

	Convey("no paths", t, func() {
		err := validateVMForUpdate(&crimson.VM{
			Name:  "vm",
			Host:  "host",
			Os:    "os",
			State: common.State_SERVING,
		}, &field_mask.FieldMask{})
		So(err, ShouldErrLike, "at least one update mask path is required")
	})

	Convey("unexpected hostname", t, func() {
		err := validateVMForUpdate(&crimson.VM{
			Name:  "vm",
			Host:  "host",
			Os:    "os",
			State: common.State_SERVING,
		}, &field_mask.FieldMask{
			Paths: []string{
				"name",
			},
		})
		So(err, ShouldErrLike, "hostname cannot be updated")
	})

	Convey("unexpected VLAN", t, func() {
		err := validateVMForUpdate(&crimson.VM{
			Name:  "vm",
			Vlan:  1,
			Host:  "host",
			Os:    "os",
			State: common.State_SERVING,
		}, &field_mask.FieldMask{
			Paths: []string{
				"vlan",
			},
		})
		So(err, ShouldErrLike, "VLAN cannot be updated")
	})

	Convey("unexpected host VLAN", t, func() {
		err := validateVMForUpdate(&crimson.VM{
			Name:     "vm",
			Host:     "host",
			HostVlan: 10,
			Os:       "os",
			State:    common.State_SERVING,
		}, &field_mask.FieldMask{
			Paths: []string{
				"host_vlan",
			},
		})
		So(err, ShouldErrLike, "host VLAN cannot be updated")
	})

	Convey("host unspecified", t, func() {
		err := validateVMForUpdate(&crimson.VM{
			Name:  "vm",
			Os:    "os",
			State: common.State_SERVING,
		}, &field_mask.FieldMask{
			Paths: []string{
				"host",
				"os",
				"state",
			},
		})
		So(err, ShouldErrLike, "physical hostname is required and must be non-empty")
	})

	Convey("operating system unspecified", t, func() {
		err := validateVMForUpdate(&crimson.VM{
			Name:  "vm",
			Host:  "host",
			State: common.State_SERVING,
		}, &field_mask.FieldMask{
			Paths: []string{
				"host",
				"os",
				"state",
			},
		})
		So(err, ShouldErrLike, "operating system is required and must be non-empty")
	})

	Convey("state unspecified", t, func() {
		err := validateVMForUpdate(&crimson.VM{
			Name: "vm",
			Host: "host",
			Os:   "os",
		}, &field_mask.FieldMask{
			Paths: []string{
				"host",
				"os",
				"state",
			},
		})
		So(err, ShouldErrLike, "state is required")
	})

	Convey("unsupported path", t, func() {
		err := validateVMForUpdate(&crimson.VM{
			Name:  "vm",
			Host:  "host",
			Os:    "os",
			State: common.State_SERVING,
		}, &field_mask.FieldMask{
			Paths: []string{
				"unknown",
			},
		})
		So(err, ShouldErrLike, "unsupported update mask path")
	})

	Convey("duplicate path", t, func() {
		err := validateVMForUpdate(&crimson.VM{
			Name:  "vm",
			Host:  "host",
			Os:    "os",
			State: common.State_SERVING,
		}, &field_mask.FieldMask{
			Paths: []string{
				"host",
				"os",
				"state",
				"description",
				"deployment_ticket",
				"deployment_ticket",
			},
		})
		So(err, ShouldErrLike, "duplicate update mask path")
	})

	Convey("ok", t, func() {
		err := validateVMForUpdate(&crimson.VM{
			Name:  "vm",
			Host:  "host",
			Os:    "os",
			State: common.State_SERVING,
		}, &field_mask.FieldMask{
			Paths: []string{
				"host",
				"os",
				"state",
				"description",
				"deployment_ticket",
			},
		})
		So(err, ShouldBeNil)
	})
}
