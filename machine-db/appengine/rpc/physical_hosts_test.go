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

func TestCreatePhysicalHost(t *testing.T) {
	Convey("createPhysicalHost", t, func() {
		db, m, _ := sqlmock.New()
		defer db.Close()
		c := database.With(context.Background(), db)
		insertNameStmt := `
			^INSERT INTO hostnames \(name, vlan_id\)
			VALUES \(\?, \(SELECT vlan_id FROM ips WHERE ipv4 = \? AND hostname_id IS NULL\)\)$
		`
		insertHostStmt := `
			^INSERT INTO physical_hosts \(
				hostname_id,
				machine_id,
				nic_id,
				os_id,
				vm_slots,
				virtual_datacenter,
				description,
				deployment_ticket
			\)
			VALUES \(
				\?,
				\(SELECT id FROM machines WHERE name = \?\),
				\(SELECT n.id FROM machines m, nics n WHERE n.machine_id = m.id AND m.name = \? AND n.name = \?\),
				\(SELECT id FROM oses WHERE name = \?\),
				\?,
				\?,
				\?,
				\?
			\)$
		`
		updateIPStmt := `
			^UPDATE ips
			SET hostname_id = \?
			WHERE ipv4 = \? AND hostname_id IS NULL$
		`
		updateMachineStmt := `
			^UPDATE machines
			SET state = \?
			WHERE name = \?$
		`
		updateNICStmt := `
			^UPDATE nics
			SET hostname_id = \?
			WHERE id = \(SELECT nic_id FROM physical_hosts WHERE id = \?\)$
		`
		selectStmt := `
			^SELECT hp.name, hp.vlan_id, m.name, n.name, n.mac_address, o.name, h.vm_slots, h.virtual_datacenter, h.description, h.deployment_ticket, i.ipv4, m.state
			FROM \(physical_hosts h, hostnames hp, machines m, nics n, oses o, ips i\)
			WHERE n.hostname_id = hp.id AND h.machine_id = m.id AND h.nic_id = n.id AND h.os_id = o.id AND i.hostname_id = hp.id AND hp.name IN \(\?\)$
		`
		columns := []string{"hp.name", "hp.vlan_id", "m.name", "n.name", "n.mac_address", "o.name", "h.vm_slots", "h.virtual_datacenter", "h.description", "h.deployment_ticket", "i.ipv4", "m.state"}
		rows := sqlmock.NewRows(columns)

		Convey("invalid IPv4", func() {
			host := &crimson.PhysicalHost{
				Name:    "host",
				Machine: "machine",
				Nic:     "eth0",
				Os:      "os",
				Ipv4:    "127.0.0.1/20",
				State:   common.State_SERVING,
			}
			res, err := createPhysicalHost(c, host)
			So(err, ShouldErrLike, "invalid IPv4 address")
			So(res, ShouldBeNil)
		})

		Convey("begin failed", func() {
			host := &crimson.PhysicalHost{
				Name:    "host",
				Machine: "machine",
				Nic:     "eth0",
				Os:      "os",
				Ipv4:    "127.0.0.1",
				State:   common.State_SERVING,
			}
			m.ExpectBegin().WillReturnError(fmt.Errorf("error"))
			res, err := createPhysicalHost(c, host)
			So(err, ShouldErrLike, "failed to begin transaction")
			So(res, ShouldBeNil)
		})

		Convey("invalid IP", func() {
			host := &crimson.PhysicalHost{
				Name:    "host",
				Machine: "machine",
				Nic:     "eth0",
				Os:      "os",
				Ipv4:    "127.0.0.1",
				State:   common.State_SERVING,
			}
			m.ExpectBegin()
			m.ExpectExec(insertNameStmt).WithArgs(host.Name, 2130706433).WillReturnError(&mysql.MySQLError{Number: mysqlerr.ER_BAD_NULL_ERROR, Message: "'vlan_id'"})
			m.ExpectRollback()
			res, err := createPhysicalHost(c, host)
			So(err, ShouldErrLike, "ensure IPv4 address")
			So(res, ShouldBeNil)
		})

		Convey("duplicate host", func() {
			host := &crimson.PhysicalHost{
				Name:    "host",
				Machine: "machine",
				Nic:     "eth0",
				Os:      "os",
				Ipv4:    "127.0.0.1",
				State:   common.State_SERVING,
			}
			m.ExpectBegin()
			m.ExpectExec(insertNameStmt).WithArgs(host.Name, 2130706433).WillReturnError(&mysql.MySQLError{Number: mysqlerr.ER_DUP_ENTRY, Message: "'name'"})
			m.ExpectRollback()
			res, err := createPhysicalHost(c, host)
			So(err, ShouldErrLike, "duplicate hostname")
			So(res, ShouldBeNil)
		})

		Convey("query failed", func() {
			host := &crimson.PhysicalHost{
				Name:    "host",
				Machine: "machine",
				Nic:     "eth0",
				Os:      "os",
				Ipv4:    "127.0.0.1",
				State:   common.State_SERVING,
			}
			m.ExpectBegin()
			m.ExpectExec(insertNameStmt).WithArgs(host.Name, 2130706433).WillReturnResult(sqlmock.NewResult(10, 1))
			m.ExpectExec(updateIPStmt).WithArgs(10, 2130706433).WillReturnResult(sqlmock.NewResult(100, 1))
			m.ExpectExec(insertHostStmt).WithArgs(10, host.Machine, host.Machine, host.Nic, host.Os, host.VmSlots, host.Description, host.DeploymentTicket).WillReturnError(fmt.Errorf("error"))
			m.ExpectRollback()
			res, err := createPhysicalHost(c, host)
			So(err, ShouldErrLike, "failed to create physical host")
			So(res, ShouldBeNil)
		})

		Convey("duplicate machine", func() {
			host := &crimson.PhysicalHost{
				Name:    "host",
				Machine: "machine",
				Nic:     "eth0",
				Os:      "os",
				Ipv4:    "127.0.0.1",
				State:   common.State_SERVING,
			}
			m.ExpectBegin()
			m.ExpectExec(insertNameStmt).WithArgs(host.Name, 2130706433).WillReturnResult(sqlmock.NewResult(10, 1))
			m.ExpectExec(updateIPStmt).WithArgs(10, 2130706433).WillReturnResult(sqlmock.NewResult(100, 1))
			m.ExpectExec(insertHostStmt).WithArgs(10, host.Machine, host.Machine, host.Nic, host.Os, host.VmSlots, host.VirtualDatacenter, host.Description, host.DeploymentTicket).WillReturnError(&mysql.MySQLError{Number: mysqlerr.ER_DUP_ENTRY, Message: "'machine_id'"})
			m.ExpectRollback()
			res, err := createPhysicalHost(c, host)
			So(err, ShouldErrLike, "duplicate physical host for machine")
			So(res, ShouldBeNil)
		})

		Convey("invalid machine", func() {
			host := &crimson.PhysicalHost{
				Name:    "host",
				Machine: "machine",
				Nic:     "eth0",
				Os:      "os",
				Ipv4:    "127.0.0.1",
				State:   common.State_SERVING,
			}
			m.ExpectBegin()
			m.ExpectExec(insertNameStmt).WithArgs(host.Name, 2130706433).WillReturnResult(sqlmock.NewResult(10, 1))
			m.ExpectExec(updateIPStmt).WithArgs(10, 2130706433).WillReturnResult(sqlmock.NewResult(100, 1))
			m.ExpectExec(insertHostStmt).WithArgs(10, host.Machine, host.Machine, host.Nic, host.Os, host.VmSlots, host.VirtualDatacenter, host.Description, host.DeploymentTicket).WillReturnError(&mysql.MySQLError{Number: mysqlerr.ER_BAD_NULL_ERROR, Message: "'machine_id' is null"})
			m.ExpectRollback()
			res, err := createPhysicalHost(c, host)
			So(err, ShouldErrLike, "does not exist")
			So(res, ShouldBeNil)
		})

		Convey("invalid NIC", func() {
			host := &crimson.PhysicalHost{
				Name:    "host",
				Machine: "machine",
				Nic:     "eth0",
				Os:      "os",
				Ipv4:    "127.0.0.1",
				State:   common.State_SERVING,
			}
			m.ExpectBegin()
			m.ExpectExec(insertNameStmt).WithArgs(host.Name, 2130706433).WillReturnResult(sqlmock.NewResult(10, 1))
			m.ExpectExec(updateIPStmt).WithArgs(10, 2130706433).WillReturnResult(sqlmock.NewResult(100, 1))
			m.ExpectExec(insertHostStmt).WithArgs(10, host.Machine, host.Machine, host.Nic, host.Os, host.VmSlots, host.VirtualDatacenter, host.Description, host.DeploymentTicket).WillReturnError(&mysql.MySQLError{Number: mysqlerr.ER_BAD_NULL_ERROR, Message: "'nic_id' is null"})
			m.ExpectRollback()
			res, err := createPhysicalHost(c, host)
			So(err, ShouldErrLike, "does not exist")
			So(res, ShouldBeNil)
		})

		Convey("invalid operating system", func() {
			host := &crimson.PhysicalHost{
				Name:    "host",
				Machine: "machine",
				Nic:     "eth0",
				Os:      "os",
				Ipv4:    "127.0.0.1",
				State:   common.State_SERVING,
			}
			m.ExpectBegin()
			m.ExpectExec(insertNameStmt).WithArgs(host.Name, 2130706433).WillReturnResult(sqlmock.NewResult(10, 1))
			m.ExpectExec(updateIPStmt).WithArgs(10, 2130706433).WillReturnResult(sqlmock.NewResult(100, 1))
			m.ExpectExec(insertHostStmt).WithArgs(10, host.Machine, host.Machine, host.Nic, host.Os, host.VmSlots, host.VirtualDatacenter, host.Description, host.DeploymentTicket).WillReturnError(&mysql.MySQLError{Number: mysqlerr.ER_BAD_NULL_ERROR, Message: "'os_id' is null"})
			m.ExpectRollback()
			res, err := createPhysicalHost(c, host)
			So(err, ShouldErrLike, "does not exist")
			So(res, ShouldBeNil)
		})

		Convey("unexpected invalid", func() {
			host := &crimson.PhysicalHost{
				Name:    "host",
				Machine: "machine",
				Nic:     "eth0",
				Os:      "os",
				Ipv4:    "127.0.0.1",
				State:   common.State_SERVING,
			}
			m.ExpectBegin()
			m.ExpectExec(insertNameStmt).WithArgs(host.Name, 2130706433).WillReturnResult(sqlmock.NewResult(10, 1))
			m.ExpectExec(updateIPStmt).WithArgs(10, 2130706433).WillReturnResult(sqlmock.NewResult(100, 1))
			m.ExpectExec(insertHostStmt).WithArgs(10, host.Machine, host.Machine, host.Nic, host.Os, host.VmSlots, host.VirtualDatacenter, host.Description, host.DeploymentTicket).WillReturnError(&mysql.MySQLError{Number: mysqlerr.ER_BAD_NULL_ERROR, Message: "error"})
			m.ExpectRollback()
			res, err := createPhysicalHost(c, host)
			So(err, ShouldErrLike, "failed to create physical host")
			So(res, ShouldBeNil)
		})

		Convey("unexpected error", func() {
			host := &crimson.PhysicalHost{
				Name:    "host",
				Machine: "machine",
				Nic:     "eth0",
				Os:      "os",
				Ipv4:    "127.0.0.1",
				State:   common.State_SERVING,
			}
			m.ExpectBegin()
			m.ExpectExec(insertNameStmt).WithArgs(host.Name, 2130706433).WillReturnResult(sqlmock.NewResult(10, 1))
			m.ExpectExec(updateIPStmt).WithArgs(10, 2130706433).WillReturnResult(sqlmock.NewResult(100, 1))
			m.ExpectExec(insertHostStmt).WithArgs(10, host.Machine, host.Machine, host.Nic, host.Os, host.VmSlots, host.VirtualDatacenter, host.Description, host.DeploymentTicket).WillReturnError(&mysql.MySQLError{Number: mysqlerr.ER_NO, Message: "name vlan_id"})
			m.ExpectRollback()
			res, err := createPhysicalHost(c, host)
			So(err, ShouldErrLike, "failed to create physical host")
			So(res, ShouldBeNil)
		})

		Convey("update failed", func() {
			host := &crimson.PhysicalHost{
				Name:    "host",
				Machine: "machine",
				Nic:     "eth0",
				Os:      "os",
				Ipv4:    "127.0.0.1",
				State:   common.State_SERVING,
			}
			m.ExpectBegin()
			m.ExpectExec(insertNameStmt).WithArgs(host.Name, 2130706433).WillReturnResult(sqlmock.NewResult(10, 1))
			m.ExpectExec(updateIPStmt).WithArgs(10, 2130706433).WillReturnResult(sqlmock.NewResult(100, 1))
			m.ExpectExec(insertHostStmt).WithArgs(10, host.Machine, host.Machine, host.Nic, host.Os, host.VmSlots, host.VirtualDatacenter, host.Description, host.DeploymentTicket).WillReturnResult(sqlmock.NewResult(1000, 1))
			m.ExpectExec(updateNICStmt).WithArgs(10, 1000).WillReturnResult(sqlmock.NewResult(10000, 1))
			m.ExpectExec(updateMachineStmt).WithArgs(host.State, host.Machine).WillReturnError(fmt.Errorf("error"))
			m.ExpectRollback()
			res, err := createPhysicalHost(c, host)
			So(err, ShouldErrLike, "failed to update machine")
			So(res, ShouldBeNil)
		})

		Convey("no state", func() {
			host := &crimson.PhysicalHost{
				Name:    "host",
				Machine: "machine",
				Nic:     "eth0",
				Os:      "os",
				Ipv4:    "127.0.0.1",
			}
			rows.AddRow(host.Name, 1, host.Machine, host.Nic, 1, host.Os, host.VmSlots, host.VirtualDatacenter, host.Description, host.DeploymentTicket, 2130706433, common.State_SERVING)
			m.ExpectBegin()
			m.ExpectExec(insertNameStmt).WithArgs(host.Name, 2130706433).WillReturnResult(sqlmock.NewResult(10, 1))
			m.ExpectExec(updateIPStmt).WithArgs(10, 2130706433).WillReturnResult(sqlmock.NewResult(100, 1))
			m.ExpectExec(insertHostStmt).WithArgs(10, host.Machine, host.Machine, host.Nic, host.Os, host.VmSlots, host.VirtualDatacenter, host.Description, host.DeploymentTicket).WillReturnResult(sqlmock.NewResult(1000, 1))
			m.ExpectExec(updateNICStmt).WithArgs(10, 1000).WillReturnResult(sqlmock.NewResult(10000, 1))
			m.ExpectQuery(selectStmt).WithArgs(host.Name).WillReturnRows(rows)
			m.ExpectCommit()
			res, err := createPhysicalHost(c, host)
			So(err, ShouldBeNil)
			So(res, ShouldResemble, &crimson.PhysicalHost{
				Name:       host.Name,
				Vlan:       1,
				Machine:    host.Machine,
				Nic:        host.Nic,
				MacAddress: "00:00:00:00:00:01",
				Os:         host.Os,
				Ipv4:       host.Ipv4,
				State:      common.State_SERVING,
			})
		})

		Convey("commit failed", func() {
			host := &crimson.PhysicalHost{
				Name:    "host",
				Machine: "machine",
				Nic:     "eth0",
				Os:      "os",
				Ipv4:    "127.0.0.1",
				State:   common.State_SERVING,
			}
			rows.AddRow(host.Name, 1, host.Machine, host.Nic, 1, host.Os, host.VmSlots, host.VirtualDatacenter, host.Description, host.DeploymentTicket, 2130706433, host.State)
			m.ExpectBegin()
			m.ExpectExec(insertNameStmt).WithArgs(host.Name, 2130706433).WillReturnResult(sqlmock.NewResult(10, 1))
			m.ExpectExec(updateIPStmt).WithArgs(10, 2130706433).WillReturnResult(sqlmock.NewResult(100, 1))
			m.ExpectExec(insertHostStmt).WithArgs(10, host.Machine, host.Machine, host.Nic, host.Os, host.VmSlots, host.VirtualDatacenter, host.Description, host.DeploymentTicket).WillReturnResult(sqlmock.NewResult(1000, 1))
			m.ExpectExec(updateNICStmt).WithArgs(10, 1000).WillReturnResult(sqlmock.NewResult(10000, 1))
			m.ExpectExec(updateMachineStmt).WithArgs(host.State, host.Machine).WillReturnResult(sqlmock.NewResult(10000, 1))
			m.ExpectQuery(selectStmt).WithArgs(host.Name).WillReturnRows(rows)
			m.ExpectCommit().WillReturnError(fmt.Errorf("error"))
			m.ExpectRollback()
			res, err := createPhysicalHost(c, host)
			So(err, ShouldErrLike, "failed to commit transaction")
			So(res, ShouldBeNil)
		})

		Convey("ok", func() {
			host := &crimson.PhysicalHost{
				Name:    "host",
				Machine: "machine",
				Nic:     "eth0",
				Os:      "os",
				Ipv4:    "127.0.0.1",
				State:   common.State_SERVING,
			}
			rows.AddRow(host.Name, 1, host.Machine, host.Nic, 1, host.Os, host.VmSlots, host.VirtualDatacenter, host.Description, host.DeploymentTicket, 2130706433, host.State)
			m.ExpectBegin()
			m.ExpectExec(insertNameStmt).WithArgs(host.Name, 2130706433).WillReturnResult(sqlmock.NewResult(10, 1))
			m.ExpectExec(updateIPStmt).WithArgs(10, 2130706433).WillReturnResult(sqlmock.NewResult(100, 1))
			m.ExpectExec(insertHostStmt).WithArgs(10, host.Machine, host.Machine, host.Nic, host.Os, host.VmSlots, host.VirtualDatacenter, host.Description, host.DeploymentTicket).WillReturnResult(sqlmock.NewResult(1000, 1))
			m.ExpectExec(updateNICStmt).WithArgs(10, 1000).WillReturnResult(sqlmock.NewResult(10000, 1))
			m.ExpectExec(updateMachineStmt).WithArgs(host.State, host.Machine).WillReturnResult(sqlmock.NewResult(10000, 1))
			m.ExpectQuery(selectStmt).WithArgs(host.Name).WillReturnRows(rows)
			m.ExpectCommit()
			res, err := createPhysicalHost(c, host)
			So(err, ShouldBeNil)
			So(res, ShouldResemble, &crimson.PhysicalHost{
				Name:       host.Name,
				Vlan:       1,
				Machine:    host.Machine,
				Nic:        host.Nic,
				MacAddress: "00:00:00:00:00:01",
				Os:         host.Os,
				Ipv4:       host.Ipv4,
				State:      host.State,
			})
		})
	})
}

func TestListPhysicalHosts(t *testing.T) {
	Convey("listPhysicalHosts", t, func() {
		db, m, _ := sqlmock.New()
		defer db.Close()
		c := database.With(context.Background(), db)
		columns := []string{"hp.name", "hp.vlan_id", "m.name", "n.name", "n.mac_address", "o.name", "h.vm_slots", "h.virtual_datacenter", "h.description", "h.deployment_ticket", "i.ipv4", "m.state"}
		rows := sqlmock.NewRows(columns)

		Convey("invalid IPv4 address", func() {
			req := &crimson.ListPhysicalHostsRequest{
				Names: []string{"host"},
				Vlans: []int64{0},
				Ipv4S: []string{"0.0.0.1", "0.0.0.2/20"},
			}
			hosts, err := listPhysicalHosts(c, db, req)
			So(err, ShouldErrLike, "invalid IPv4 address")
			So(hosts, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("query failed", func() {
			selectStmt := `
				^SELECT hp.name, hp.vlan_id, m.name, n.name, n.mac_address, o.name, h.vm_slots, h.virtual_datacenter, h.description, h.deployment_ticket, i.ipv4, m.state
				FROM \(physical_hosts h, hostnames hp, machines m, nics n, oses o, ips i\)
				WHERE n.hostname_id = hp.id AND h.machine_id = m.id AND h.nic_id = n.id AND h.os_id = o.id AND i.hostname_id = hp.id
					AND hp.name IN \(\?\) AND hp.vlan_id IN \(\?\) AND i.ipv4 IN \(\?,\?\)$
			`
			req := &crimson.ListPhysicalHostsRequest{
				Names: []string{"host"},
				Vlans: []int64{0},
				Ipv4S: []string{"0.0.0.1", "0.0.0.2"},
			}
			m.ExpectQuery(selectStmt).WithArgs(req.Names[0], req.Vlans[0], 1, 2).WillReturnError(fmt.Errorf("error"))
			hosts, err := listPhysicalHosts(c, db, req)
			So(err, ShouldErrLike, "failed to fetch physical hosts")
			So(hosts, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("no names", func() {
			selectStmt := `
				^SELECT hp.name, hp.vlan_id, m.name, n.name, n.mac_address, o.name, h.vm_slots, h.virtual_datacenter, h.description, h.deployment_ticket, i.ipv4, m.state
				FROM \(physical_hosts h, hostnames hp, machines m, nics n, oses o, ips i\)
				WHERE n.hostname_id = hp.id AND h.machine_id = m.id AND h.nic_id = n.id AND h.os_id = o.id AND i.hostname_id = hp.id AND hp.vlan_id IN \(\?\)$
			`
			req := &crimson.ListPhysicalHostsRequest{
				Vlans: []int64{0},
			}
			m.ExpectQuery(selectStmt).WithArgs(req.Vlans[0]).WillReturnRows(rows)
			hosts, err := listPhysicalHosts(c, db, req)
			So(err, ShouldBeNil)
			So(hosts, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("no vlans", func() {
			selectStmt := `
				^SELECT hp.name, hp.vlan_id, m.name, n.name, n.mac_address, o.name, h.vm_slots, h.virtual_datacenter, h.description, h.deployment_ticket, i.ipv4, m.state
				FROM \(physical_hosts h, hostnames hp, machines m, nics n, oses o, ips i\)
				WHERE n.hostname_id = hp.id AND h.machine_id = m.id AND h.nic_id = n.id AND h.os_id = o.id AND i.hostname_id = hp.id AND hp.name IN \(\?\)$
			`
			req := &crimson.ListPhysicalHostsRequest{
				Names: []string{"host"},
			}
			m.ExpectQuery(selectStmt).WithArgs(req.Names[0]).WillReturnRows(rows)
			hosts, err := listPhysicalHosts(c, db, req)
			So(err, ShouldBeNil)
			So(hosts, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("datacenters", func() {
			selectStmt := `
				^SELECT hp.name, hp.vlan_id, m.name, n.name, n.mac_address, o.name, h.vm_slots, h.virtual_datacenter, h.description, h.deployment_ticket, i.ipv4, m.state
				FROM \(physical_hosts h, hostnames hp, machines m, nics n, oses o, ips i\)
				JOIN racks r ON m.rack_id = r.id
				JOIN datacenters d ON r.datacenter_id = d.id
				WHERE n.hostname_id = hp.id AND h.machine_id = m.id AND h.nic_id = n.id AND h.os_id = o.id AND i.hostname_id = hp.id AND d.name IN \(\?,\?\)$
			`
			req := &crimson.ListPhysicalHostsRequest{
				Datacenters: []string{"datacenter 1", "datacenter 2"},
			}
			m.ExpectQuery(selectStmt).WithArgs(req.Datacenters[0], req.Datacenters[1]).WillReturnRows(rows)
			hosts, err := listPhysicalHosts(c, db, req)
			So(err, ShouldBeNil)
			So(hosts, ShouldBeNil)
		})

		Convey("racks", func() {
			selectStmt := `
				^SELECT hp.name, hp.vlan_id, m.name, n.name, n.mac_address, o.name, h.vm_slots, h.virtual_datacenter, h.description, h.deployment_ticket, i.ipv4, m.state
				FROM \(physical_hosts h, hostnames hp, machines m, nics n, oses o, ips i\)
				JOIN racks r ON m.rack_id = r.id
				WHERE n.hostname_id = hp.id AND h.machine_id = m.id AND h.nic_id = n.id AND h.os_id = o.id AND i.hostname_id = hp.id AND r.name IN \(\?,\?\)$
			`
			req := &crimson.ListPhysicalHostsRequest{
				Racks: []string{"rack 1", "rack 2"},
			}
			m.ExpectQuery(selectStmt).WithArgs(req.Racks[0], req.Racks[1]).WillReturnRows(rows)
			hosts, err := listPhysicalHosts(c, db, req)
			So(err, ShouldBeNil)
			So(hosts, ShouldBeNil)
		})

		Convey("datacenters and racks", func() {
			selectStmt := `
				^SELECT hp.name, hp.vlan_id, m.name, n.name, n.mac_address, o.name, h.vm_slots, h.virtual_datacenter, h.description, h.deployment_ticket, i.ipv4, m.state
				FROM \(physical_hosts h, hostnames hp, machines m, nics n, oses o, ips i\)
				JOIN racks r ON m.rack_id = r.id
				JOIN datacenters d ON r.datacenter_id = d.id
				WHERE n.hostname_id = hp.id AND h.machine_id = m.id AND h.nic_id = n.id AND h.os_id = o.id AND i.hostname_id = hp.id
					AND d.name IN \(\?\) AND r.name IN \(\?\)$
			`
			req := &crimson.ListPhysicalHostsRequest{
				Datacenters: []string{"datacenter"},
				Racks:       []string{"rack"},
			}
			m.ExpectQuery(selectStmt).WithArgs(req.Datacenters[0], req.Racks[0]).WillReturnRows(rows)
			hosts, err := listPhysicalHosts(c, db, req)
			So(err, ShouldBeNil)
			So(hosts, ShouldBeNil)
		})

		Convey("empty", func() {
			selectStmt := `
				^SELECT hp.name, hp.vlan_id, m.name, n.name, n.mac_address, o.name, h.vm_slots, h.virtual_datacenter, h.description, h.deployment_ticket, i.ipv4, m.state
				FROM \(physical_hosts h, hostnames hp, machines m, nics n, oses o, ips i\)
				WHERE n.hostname_id = hp.id AND h.machine_id = m.id AND h.nic_id = n.id AND h.os_id = o.id AND i.hostname_id = hp.id AND hp.name IN \(\?\) AND hp.vlan_id IN \(\?\)$
			`
			req := &crimson.ListPhysicalHostsRequest{
				Names: []string{"host"},
				Vlans: []int64{0},
			}
			m.ExpectQuery(selectStmt).WithArgs(req.Names[0], req.Vlans[0]).WillReturnRows(rows)
			hosts, err := listPhysicalHosts(c, db, req)
			So(err, ShouldBeNil)
			So(hosts, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("non-empty", func() {
			selectStmt := `
				^SELECT hp.name, hp.vlan_id, m.name, n.name, n.mac_address, o.name, h.vm_slots, h.virtual_datacenter, h.description, h.deployment_ticket, i.ipv4, m.state
				FROM \(physical_hosts h, hostnames hp, machines m, nics n, oses o, ips i\)
				WHERE n.hostname_id = hp.id AND h.machine_id = m.id AND h.nic_id = n.id AND h.os_id = o.id AND i.hostname_id = hp.id AND hp.name IN \(\?,\?\) AND hp.vlan_id IN \(\?\)$
			`
			req := &crimson.ListPhysicalHostsRequest{
				Names: []string{"host 1", "host 2"},
				Vlans: []int64{0},
			}
			rows.AddRow(req.Names[0], req.Vlans[0], "machine 1", "eth0", 1, "os 1", 1, "virtual datacenter 1", "", "", 1, 0)
			rows.AddRow(req.Names[1], req.Vlans[0], "machine 2", "eth1", 2, "os 2", 2, "virtual datacenter 2", "", "", 2, common.State_SERVING)
			m.ExpectQuery(selectStmt).WithArgs(req.Names[0], req.Names[1], req.Vlans[0]).WillReturnRows(rows)
			hosts, err := listPhysicalHosts(c, db, req)
			So(err, ShouldBeNil)
			So(hosts, ShouldResemble, []*crimson.PhysicalHost{
				{
					Name:              req.Names[0],
					Vlan:              req.Vlans[0],
					Machine:           "machine 1",
					Nic:               "eth0",
					MacAddress:        "00:00:00:00:00:01",
					Os:                "os 1",
					VmSlots:           1,
					VirtualDatacenter: "virtual datacenter 1",
					Ipv4:              "0.0.0.1",
				},
				{
					Name:              req.Names[1],
					Vlan:              req.Vlans[0],
					Machine:           "machine 2",
					Nic:               "eth1",
					MacAddress:        "00:00:00:00:00:02",
					Os:                "os 2",
					VmSlots:           2,
					VirtualDatacenter: "virtual datacenter 2",
					Ipv4:              "0.0.0.2",
					State:             common.State_SERVING,
				},
			})
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("ok", func() {
			selectStmt := `
				^SELECT hp.name, hp.vlan_id, m.name, n.name, n.mac_address, o.name, h.vm_slots, h.virtual_datacenter, h.description, h.deployment_ticket, i.ipv4, m.state
				FROM \(physical_hosts h, hostnames hp, machines m, nics n, oses o, ips i\)
				WHERE n.hostname_id = hp.id AND h.machine_id = m.id AND h.nic_id = n.id AND h.os_id = o.id AND i.hostname_id = hp.id$
			`
			req := &crimson.ListPhysicalHostsRequest{}
			rows.AddRow("host 1", 1, "machine 1", "eth0", 1, "os 1", 1, "virtual datacenter 1", "", "", 1, 0)
			rows.AddRow("host 2", 2, "machine 2", "eth1", 2, "os 2", 2, "virtual datacenter 2", "", "", 2, common.State_SERVING)
			m.ExpectQuery(selectStmt).WithArgs().WillReturnRows(rows)
			hosts, err := listPhysicalHosts(c, db, req)
			So(err, ShouldBeNil)
			So(hosts, ShouldResemble, []*crimson.PhysicalHost{
				{
					Name:              "host 1",
					Vlan:              1,
					Machine:           "machine 1",
					Nic:               "eth0",
					MacAddress:        "00:00:00:00:00:01",
					Os:                "os 1",
					VmSlots:           1,
					VirtualDatacenter: "virtual datacenter 1",
					Ipv4:              "0.0.0.1",
				},
				{
					Name:              "host 2",
					Vlan:              2,
					Machine:           "machine 2",
					Nic:               "eth1",
					MacAddress:        "00:00:00:00:00:02",
					Os:                "os 2",
					VmSlots:           2,
					VirtualDatacenter: "virtual datacenter 2",
					Ipv4:              "0.0.0.2",
					State:             common.State_SERVING,
				},
			})
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})
	})
}

func TestUpdatePhysicalHost(t *testing.T) {
	Convey("updatePhysicalHost", t, func() {
		db, m, _ := sqlmock.New()
		defer db.Close()
		c := database.With(context.Background(), db)
		updateMachineStmt := `
			UPDATE machines
			SET state = \?
			WHERE id = \(SELECT machine_id FROM physical_hosts WHERE hostname_id = \(SELECT id FROM hostnames WHERE name = \?\)\)
		`
		selectStmt := `
			^SELECT hp.name, hp.vlan_id, m.name, n.name, n.mac_address, o.name, h.vm_slots, h.virtual_datacenter, h.description, h.deployment_ticket, i.ipv4, m.state
			FROM \(physical_hosts h, hostnames hp, machines m, nics n, oses o, ips i\)
			WHERE n.hostname_id = hp.id AND h.machine_id = m.id AND h.nic_id = n.id AND h.os_id = o.id AND i.hostname_id = hp.id AND hp.name IN \(\?\)$
		`
		columns := []string{"hp.name", "hp.vlan_id", "m.name", "n.name", "n.mac_address", "o.name", "h.vm_slots", "h.virtual_datacenter", "h.description", "h.deployment_ticket", "i.ipv4", "m.state"}
		rows := sqlmock.NewRows(columns)

		Convey("update machine", func() {
			updateStmt := `
				^UPDATE physical_hosts
				SET machine_id = \(SELECT id FROM machines WHERE name = \?\)
				WHERE hostname_id = \(SELECT id FROM hostnames WHERE name = \?\)$
			`
			host := &crimson.PhysicalHost{
				Name:              "hostname",
				Machine:           "machine",
				Nic:               "eth0",
				Os:                "operating system",
				VmSlots:           1,
				VirtualDatacenter: "virtual datacenter 1",
				State:             common.State_SERVING,
				Ipv4:              "0.0.0.1",
			}
			mask := &field_mask.FieldMask{
				Paths: []string{
					"machine",
				},
			}
			rows.AddRow(host.Name, host.Vlan, host.Machine, host.Nic, 1, host.Os, host.VmSlots, host.VirtualDatacenter, host.Description, host.DeploymentTicket, 1, host.State)
			m.ExpectBegin()
			m.ExpectExec(updateStmt).WithArgs(host.Machine, host.Name).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectQuery(selectStmt).WillReturnRows(rows)
			m.ExpectCommit()
			host, err := updatePhysicalHost(c, host, mask)
			So(err, ShouldBeNil)
			So(host, ShouldResemble, &crimson.PhysicalHost{
				Name:              host.Name,
				Vlan:              host.Vlan,
				Machine:           host.Machine,
				Nic:               host.Nic,
				MacAddress:        "00:00:00:00:00:01",
				Os:                host.Os,
				VmSlots:           host.VmSlots,
				VirtualDatacenter: host.VirtualDatacenter,
				State:             host.State,
				Ipv4:              host.Ipv4,
			})
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("update operating system", func() {
			updateStmt := `
				^UPDATE physical_hosts
				SET os_id = \(SELECT id FROM oses WHERE name = \?\)
				WHERE hostname_id = \(SELECT id FROM hostnames WHERE name = \?\)$
			`
			host := &crimson.PhysicalHost{
				Name:              "hostname",
				Machine:           "machine",
				Nic:               "eth0",
				Os:                "operating system",
				VmSlots:           1,
				VirtualDatacenter: "virtual datacenter 1",
				State:             common.State_SERVING,
				Ipv4:              "0.0.0.1",
			}
			mask := &field_mask.FieldMask{
				Paths: []string{
					"os",
				},
			}
			rows.AddRow(host.Name, host.Vlan, host.Machine, host.Nic, 1, host.Os, host.VmSlots, host.VirtualDatacenter, host.Description, host.DeploymentTicket, 1, host.State)
			m.ExpectBegin()
			m.ExpectExec(updateStmt).WithArgs(host.Os, host.Name).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectQuery(selectStmt).WillReturnRows(rows)
			m.ExpectCommit()
			host, err := updatePhysicalHost(c, host, mask)
			So(err, ShouldBeNil)
			So(host, ShouldResemble, &crimson.PhysicalHost{
				Name:              host.Name,
				Vlan:              host.Vlan,
				Machine:           host.Machine,
				Nic:               host.Nic,
				MacAddress:        "00:00:00:00:00:01",
				Os:                host.Os,
				VmSlots:           host.VmSlots,
				VirtualDatacenter: host.VirtualDatacenter,
				State:             host.State,
				Ipv4:              host.Ipv4,
			})
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("update VM slots", func() {
			updateStmt := `
				^UPDATE physical_hosts
				SET vm_slots = \?
				WHERE hostname_id = \(SELECT id FROM hostnames WHERE name = \?\)$
			`
			host := &crimson.PhysicalHost{
				Name:              "hostname",
				Machine:           "machine",
				Nic:               "eth0",
				Os:                "operating system",
				VmSlots:           1,
				VirtualDatacenter: "virtual datacenter 1",
				State:             common.State_SERVING,
				Ipv4:              "0.0.0.1",
			}
			mask := &field_mask.FieldMask{
				Paths: []string{
					"vm_slots",
				},
			}
			rows.AddRow(host.Name, host.Vlan, host.Machine, host.Nic, 1, host.Os, host.VmSlots, host.VirtualDatacenter, host.Description, host.DeploymentTicket, 1, host.State)
			m.ExpectBegin()
			m.ExpectExec(updateStmt).WithArgs(host.VmSlots, host.Name).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectQuery(selectStmt).WillReturnRows(rows)
			m.ExpectCommit()
			host, err := updatePhysicalHost(c, host, mask)
			So(err, ShouldBeNil)
			So(host, ShouldResemble, &crimson.PhysicalHost{
				Name:              host.Name,
				Vlan:              host.Vlan,
				Machine:           host.Machine,
				Nic:               host.Nic,
				MacAddress:        "00:00:00:00:00:01",
				Os:                host.Os,
				VmSlots:           host.VmSlots,
				VirtualDatacenter: host.VirtualDatacenter,
				State:             host.State,
				Ipv4:              host.Ipv4,
			})
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("update state", func() {
			host := &crimson.PhysicalHost{
				Name:              "hostname",
				Vlan:              1,
				Machine:           "machine",
				Nic:               "eth0",
				Os:                "operating system",
				VmSlots:           1,
				VirtualDatacenter: "virtual datacenter 1",
				State:             common.State_SERVING,
				Ipv4:              "0.0.0.1",
			}
			mask := &field_mask.FieldMask{
				Paths: []string{
					"state",
				},
			}
			rows.AddRow(host.Name, host.Vlan, host.Machine, host.Nic, 1, host.Os, host.VmSlots, host.VirtualDatacenter, host.Description, host.DeploymentTicket, 1, host.State)
			m.ExpectBegin()
			m.ExpectExec(updateMachineStmt).WithArgs(host.State, host.Name).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectQuery(selectStmt).WillReturnRows(rows)
			m.ExpectCommit()
			host, err := updatePhysicalHost(c, host, mask)
			So(err, ShouldBeNil)
			So(host, ShouldResemble, &crimson.PhysicalHost{
				Name:              host.Name,
				Vlan:              host.Vlan,
				Machine:           host.Machine,
				Nic:               host.Nic,
				MacAddress:        "00:00:00:00:00:01",
				Os:                host.Os,
				VmSlots:           host.VmSlots,
				VirtualDatacenter: host.VirtualDatacenter,
				State:             host.State,
				Ipv4:              host.Ipv4,
			})
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("ok", func() {
			updateStmt := `
				^UPDATE physical_hosts
				SET machine_id = \(SELECT id FROM machines WHERE name = \?\), os_id = \(SELECT id FROM oses WHERE name = \?\), vm_slots = \?, virtual_datacenter = \?
				WHERE hostname_id = \(SELECT id FROM hostnames WHERE name = \?\)$
			`
			host := &crimson.PhysicalHost{
				Name:              "hostname",
				Machine:           "machine",
				Os:                "operating system",
				VmSlots:           1,
				VirtualDatacenter: "virtual datacenter 1",
				State:             common.State_SERVING,
				Ipv4:              "0.0.0.1",
			}
			mask := &field_mask.FieldMask{
				Paths: []string{
					"machine",
					"os",
					"vm_slots",
					"virtual_datacenter",
					"state",
				},
			}
			rows.AddRow(host.Name, host.Vlan, host.Machine, host.Nic, 1, host.Os, host.VmSlots, host.VirtualDatacenter, host.Description, host.DeploymentTicket, 1, host.State)
			m.ExpectBegin()
			m.ExpectExec(updateStmt).WithArgs(host.Machine, host.Os, host.VmSlots, host.VirtualDatacenter, host.Name).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectExec(updateMachineStmt).WithArgs(host.State, host.Name).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectQuery(selectStmt).WillReturnRows(rows)
			m.ExpectCommit()
			host, err := updatePhysicalHost(c, host, mask)
			So(err, ShouldBeNil)
			So(host, ShouldResemble, &crimson.PhysicalHost{
				Name:              host.Name,
				Vlan:              host.Vlan,
				Machine:           host.Machine,
				Nic:               host.Nic,
				MacAddress:        "00:00:00:00:00:01",
				Os:                host.Os,
				VmSlots:           host.VmSlots,
				VirtualDatacenter: host.VirtualDatacenter,
				State:             host.State,
				Ipv4:              host.Ipv4,
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
			Machine: "machine",
			Nic:     "eth0",
			Os:      "os",
			Ipv4:    "127.0.0.1",
		})
		So(err, ShouldErrLike, "hostname is required and must be non-empty")
	})

	Convey("VLAN specified", t, func() {
		err := validatePhysicalHostForCreation(&crimson.PhysicalHost{
			Name:    "hostname",
			Vlan:    1,
			Machine: "machine",
			Nic:     "eth0",
			Os:      "os",
			Ipv4:    "127.0.0.1",
		})
		So(err, ShouldErrLike, "VLAN must not be specified, use IP address instead")
	})

	Convey("machine unspecified", t, func() {
		err := validatePhysicalHostForCreation(&crimson.PhysicalHost{
			Name: "hostname",
			Nic:  "eth0",
			Os:   "os",
			Ipv4: "127.0.0.1",
		})
		So(err, ShouldErrLike, "machine is required and must be non-empty")
	})

	Convey("NIC unspecified", t, func() {
		err := validatePhysicalHostForCreation(&crimson.PhysicalHost{
			Name:    "hostname",
			Machine: "machine",
			Os:      "os",
			Ipv4:    "127.0.0.1",
		})
		So(err, ShouldErrLike, "NIC is required and must be non-empty")
	})

	Convey("MAC address specified", t, func() {
		err := validatePhysicalHostForCreation(&crimson.PhysicalHost{
			Name:       "hostname",
			Machine:    "machine",
			Nic:        "eth0",
			MacAddress: "00:00:00:00:00:01",
			Os:         "os",
			Ipv4:       "127.0.0.1",
		})
		So(err, ShouldErrLike, "MAC address must not be specified, use NIC instead")
	})

	Convey("operating system unspecified", t, func() {
		err := validatePhysicalHostForCreation(&crimson.PhysicalHost{
			Name:    "hostname",
			Machine: "machine",
			Nic:     "eth0",
			Ipv4:    "127.0.0.1",
		})
		So(err, ShouldErrLike, "operating system is required and must be non-empty")
	})

	Convey("IPv4 address unspecified", t, func() {
		err := validatePhysicalHostForCreation(&crimson.PhysicalHost{
			Name:    "hostname",
			Machine: "machine",
			Nic:     "eth0",
			Os:      "os",
		})
		So(err, ShouldErrLike, "invalid IPv4 address")
	})

	Convey("IPv4 address invalid", t, func() {
		err := validatePhysicalHostForCreation(&crimson.PhysicalHost{
			Name:    "hostname",
			Machine: "machine",
			Nic:     "eth0",
			Os:      "os",
			Ipv4:    "127.0.0.1/20",
		})
		So(err, ShouldErrLike, "invalid IPv4 address")
	})

	Convey("VM slots negative", t, func() {
		err := validatePhysicalHostForCreation(&crimson.PhysicalHost{
			Name:    "hostname",
			Machine: "machine",
			Nic:     "eth0",
			Os:      "os",
			Ipv4:    "127.0.0.1",
			VmSlots: -1,
		})
		So(err, ShouldErrLike, "VM slots must be non-negative")
	})

	Convey("ok", t, func() {
		err := validatePhysicalHostForCreation(&crimson.PhysicalHost{
			Name:    "hostname",
			Machine: "machine",
			Nic:     "eth0",
			Os:      "os",
			Ipv4:    "127.0.0.1",
		})
		So(err, ShouldBeNil)
	})
}

func TestValidatePhysicalHostForUpdate(t *testing.T) {
	t.Parallel()

	Convey("host unspecified", t, func() {
		err := validatePhysicalHostForUpdate(nil, &field_mask.FieldMask{
			Paths: []string{
				"machine",
				"os",
				"vm_slots",
				"state",
			},
		})
		So(err, ShouldErrLike, "physical host specification is required")
	})

	Convey("hostname unspecified", t, func() {
		err := validatePhysicalHostForUpdate(&crimson.PhysicalHost{
			Machine: "machine",
			Os:      "os",
			VmSlots: 1,
			State:   common.State_SERVING,
		}, &field_mask.FieldMask{
			Paths: []string{
				"machine",
				"os",
				"vm_slots",
				"state",
			},
		})
		So(err, ShouldErrLike, "hostname is required and must be non-empty")
	})

	Convey("mask unspecified", t, func() {
		err := validatePhysicalHostForUpdate(&crimson.PhysicalHost{
			Name:    "hostname",
			Machine: "machine",
			Os:      "os",
			VmSlots: 1,
			State:   common.State_SERVING,
		}, nil)
		So(err, ShouldErrLike, "update mask is required")
	})

	Convey("no paths", t, func() {
		err := validatePhysicalHostForUpdate(&crimson.PhysicalHost{
			Name:    "hostname",
			Machine: "machine",
			Os:      "os",
			VmSlots: 1,
			State:   common.State_SERVING,
		}, &field_mask.FieldMask{})
		So(err, ShouldErrLike, "at least one update mask path is required")
	})

	Convey("unexpected hostname", t, func() {
		err := validatePhysicalHostForUpdate(&crimson.PhysicalHost{
			Name:    "hostname",
			Machine: "machine",
			Os:      "os",
			VmSlots: 1,
			State:   common.State_SERVING,
		}, &field_mask.FieldMask{
			Paths: []string{
				"name",
			},
		})
		So(err, ShouldErrLike, "hostname cannot be updated")
	})

	Convey("unexpected VLAN", t, func() {
		err := validatePhysicalHostForUpdate(&crimson.PhysicalHost{
			Name:    "hostname",
			Vlan:    1,
			Machine: "machine",
			Os:      "os",
			VmSlots: 1,
			State:   common.State_SERVING,
		}, &field_mask.FieldMask{
			Paths: []string{
				"vlan",
			},
		})
		So(err, ShouldErrLike, "VLAN cannot be updated")
	})

	Convey("unexpected NIC", t, func() {
		err := validatePhysicalHostForUpdate(&crimson.PhysicalHost{
			Name:    "hostname",
			Machine: "machine",
			Nic:     "eth0",
			Os:      "os",
			VmSlots: 1,
			State:   common.State_SERVING,
		}, &field_mask.FieldMask{
			Paths: []string{
				"nic",
			},
		})
		So(err, ShouldErrLike, "NIC cannot be updated")
	})

	Convey("unexpected MAC address", t, func() {
		err := validatePhysicalHostForUpdate(&crimson.PhysicalHost{
			Name:       "hostname",
			Machine:    "machine",
			MacAddress: "00:00:00:00:00:01",
			Os:         "os",
			VmSlots:    1,
			State:      common.State_SERVING,
		}, &field_mask.FieldMask{
			Paths: []string{
				"mac_address",
			},
		})
		So(err, ShouldErrLike, "MAC address cannot be updated")
	})

	Convey("machine unspecified", t, func() {
		err := validatePhysicalHostForUpdate(&crimson.PhysicalHost{
			Name:    "hostname",
			Os:      "os",
			VmSlots: 1,
			State:   common.State_SERVING,
		}, &field_mask.FieldMask{
			Paths: []string{
				"machine",
				"os",
				"vm_slots",
				"state",
			},
		})
		So(err, ShouldErrLike, "machine is required and must be non-empty")
	})

	Convey("operating system unspecified", t, func() {
		err := validatePhysicalHostForUpdate(&crimson.PhysicalHost{
			Name:    "hostname",
			Machine: "machine",
			VmSlots: 1,
			State:   common.State_SERVING,
		}, &field_mask.FieldMask{
			Paths: []string{
				"machine",
				"os",
				"vm_slots",
				"state",
			},
		})
		So(err, ShouldErrLike, "operating system is required and must be non-empty")
	})

	Convey("state unspecified", t, func() {
		err := validatePhysicalHostForUpdate(&crimson.PhysicalHost{
			Name:    "hostname",
			Machine: "machine",
			Os:      "os",
			VmSlots: 1,
		}, &field_mask.FieldMask{
			Paths: []string{
				"machine",
				"os",
				"vm_slots",
				"state",
			},
		})
		So(err, ShouldErrLike, "state is required")
	})

	Convey("unsupported path", t, func() {
		err := validatePhysicalHostForUpdate(&crimson.PhysicalHost{
			Name:    "hostname",
			Machine: "machine",
			Os:      "os",
			VmSlots: 1,
			State:   common.State_SERVING,
		}, &field_mask.FieldMask{
			Paths: []string{
				"unknown",
			},
		})
		So(err, ShouldErrLike, "unsupported update mask path")
	})

	Convey("duplicate path", t, func() {
		err := validatePhysicalHostForUpdate(&crimson.PhysicalHost{
			Name:    "hostname",
			Machine: "machine",
			Os:      "os",
			VmSlots: 1,
			State:   common.State_SERVING,
		}, &field_mask.FieldMask{
			Paths: []string{
				"machine",
				"os",
				"vm_slots",
				"state",
				"description",
				"deployment_ticket",
				"deployment_ticket",
			},
		})
		So(err, ShouldErrLike, "duplicate update mask path")
	})

	Convey("ok", t, func() {
		err := validatePhysicalHostForUpdate(&crimson.PhysicalHost{
			Name:    "hostname",
			Machine: "machine",
			Os:      "os",
			VmSlots: 1,
			State:   common.State_SERVING,
		}, &field_mask.FieldMask{
			Paths: []string{
				"machine",
				"os",
				"vm_slots",
				"state",
				"description",
				"deployment_ticket",
			},
		})
		So(err, ShouldBeNil)
	})
}
