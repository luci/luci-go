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
			^INSERT INTO physical_hosts \(hostname_id, machine_id, os_id, vm_slots, description, deployment_ticket\)
			VALUES \(
				\?,
				\(SELECT id FROM machines WHERE name = \?\),
				\(SELECT id FROM oses WHERE name = \?\),
				\?,
				\?,
				\?
		\)$
		`
		updateIPStmt := `
			UPDATE ips
			SET hostname_id = \?
			WHERE ipv4 = \? AND hostname_id IS NULL
		`

		Convey("invalid IPv4", func() {
			host := &crimson.PhysicalHost{
				Name:    "host",
				Machine: "machine",
				Os:      "os",
				Ipv4:    "127.0.0.1/20",
			}
			So(createPhysicalHost(c, host), ShouldErrLike, "invalid IPv4 address")
		})

		Convey("begin failed", func() {
			host := &crimson.PhysicalHost{
				Name:    "host",
				Machine: "machine",
				Os:      "os",
				Ipv4:    "127.0.0.1",
			}
			m.ExpectBegin().WillReturnError(fmt.Errorf("error"))
			So(createPhysicalHost(c, host), ShouldErrLike, "Internal server error")
		})

		Convey("invalid IP", func() {
			host := &crimson.PhysicalHost{
				Name:    "host",
				Machine: "machine",
				Os:      "os",
				Ipv4:    "127.0.0.1",
			}
			m.ExpectBegin()
			m.ExpectExec(insertNameStmt).WithArgs(host.Name, 2130706433).WillReturnError(&mysql.MySQLError{Number: mysqlerr.ER_BAD_NULL_ERROR, Message: "'vlan_id'"})
			m.ExpectRollback()
			So(createPhysicalHost(c, host), ShouldErrLike, "ensure IPv4 address")
		})

		Convey("duplicate host/VLAN", func() {
			host := &crimson.PhysicalHost{
				Name:    "host",
				Machine: "machine",
				Os:      "os",
				Ipv4:    "127.0.0.1",
			}
			m.ExpectBegin()
			m.ExpectExec(insertNameStmt).WithArgs(host.Name, 2130706433).WillReturnError(&mysql.MySQLError{Number: mysqlerr.ER_DUP_ENTRY, Message: "'name'"})
			m.ExpectRollback()
			So(createPhysicalHost(c, host), ShouldErrLike, "duplicate hostname")
		})

		Convey("query failed", func() {
			host := &crimson.PhysicalHost{
				Name:    "host",
				Machine: "machine",
				Os:      "os",
				Ipv4:    "127.0.0.1",
			}
			m.ExpectBegin()
			m.ExpectExec(insertNameStmt).WithArgs(host.Name, 2130706433).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectExec(updateIPStmt).WithArgs(1, 2130706433).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectExec(insertHostStmt).WithArgs(1, host.Machine, host.Os, host.VmSlots, host.Description, host.DeploymentTicket).WillReturnError(fmt.Errorf("error"))
			m.ExpectRollback()
			So(createPhysicalHost(c, host), ShouldErrLike, "Internal server error")
		})

		Convey("duplicate machine", func() {
			host := &crimson.PhysicalHost{
				Name:    "host",
				Machine: "machine",
				Os:      "os",
				Ipv4:    "127.0.0.1",
			}
			m.ExpectBegin()
			m.ExpectExec(insertNameStmt).WithArgs(host.Name, 2130706433).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectExec(updateIPStmt).WithArgs(1, 2130706433).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectExec(insertHostStmt).WithArgs(1, host.Machine, host.Os, host.VmSlots, host.Description, host.DeploymentTicket).WillReturnError(&mysql.MySQLError{Number: mysqlerr.ER_DUP_ENTRY, Message: "'machine_id'"})
			m.ExpectRollback()
			So(createPhysicalHost(c, host), ShouldErrLike, "duplicate physical host for machine")
		})

		Convey("invalid machine", func() {
			host := &crimson.PhysicalHost{
				Name:    "host",
				Machine: "machine",
				Os:      "os",
				Ipv4:    "127.0.0.1",
			}
			m.ExpectBegin()
			m.ExpectExec(insertNameStmt).WithArgs(host.Name, 2130706433).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectExec(updateIPStmt).WithArgs(1, 2130706433).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectExec(insertHostStmt).WithArgs(1, host.Machine, host.Os, host.VmSlots, host.Description, host.DeploymentTicket).WillReturnError(&mysql.MySQLError{Number: mysqlerr.ER_BAD_NULL_ERROR, Message: "'machine_id' is null"})
			m.ExpectRollback()
			So(createPhysicalHost(c, host), ShouldErrLike, "unknown machine")
		})

		Convey("invalid operating system", func() {
			host := &crimson.PhysicalHost{
				Name:    "host",
				Machine: "machine",
				Os:      "os",
				Ipv4:    "127.0.0.1",
			}
			m.ExpectBegin()
			m.ExpectExec(insertNameStmt).WithArgs(host.Name, 2130706433).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectExec(updateIPStmt).WithArgs(1, 2130706433).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectExec(insertHostStmt).WithArgs(1, host.Machine, host.Os, host.VmSlots, host.Description, host.DeploymentTicket).WillReturnError(&mysql.MySQLError{Number: mysqlerr.ER_BAD_NULL_ERROR, Message: "'os_id' is null"})
			m.ExpectRollback()
			So(createPhysicalHost(c, host), ShouldErrLike, "unknown operating system")
		})

		Convey("unexpected invalid", func() {
			host := &crimson.PhysicalHost{
				Name:    "host",
				Machine: "machine",
				Os:      "os",
				Ipv4:    "127.0.0.1",
			}
			m.ExpectBegin()
			m.ExpectExec(insertNameStmt).WithArgs(host.Name, 2130706433).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectExec(updateIPStmt).WithArgs(1, 2130706433).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectExec(insertHostStmt).WithArgs(1, host.Machine, host.Os, host.VmSlots, host.Description, host.DeploymentTicket).WillReturnError(&mysql.MySQLError{Number: mysqlerr.ER_BAD_NULL_ERROR, Message: "error"})
			m.ExpectRollback()
			So(createPhysicalHost(c, host), ShouldErrLike, "Internal server error")
		})

		Convey("unexpected error", func() {
			host := &crimson.PhysicalHost{
				Name:    "host",
				Machine: "machine",
				Os:      "os",
				Ipv4:    "127.0.0.1",
			}
			m.ExpectBegin()
			m.ExpectExec(insertNameStmt).WithArgs(host.Name, 2130706433).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectExec(updateIPStmt).WithArgs(1, 2130706433).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectExec(insertHostStmt).WithArgs(1, host.Machine, host.Os, host.VmSlots, host.Description, host.DeploymentTicket).WillReturnError(&mysql.MySQLError{Number: mysqlerr.ER_NO, Message: "name vlan_id"})
			m.ExpectRollback()
			So(createPhysicalHost(c, host), ShouldErrLike, "Internal server error")
		})

		Convey("commit failed", func() {
			host := &crimson.PhysicalHost{
				Name:    "host",
				Machine: "machine",
				Os:      "os",
				Ipv4:    "127.0.0.1",
			}
			m.ExpectBegin()
			m.ExpectExec(insertNameStmt).WithArgs(host.Name, 2130706433).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectExec(updateIPStmt).WithArgs(1, 2130706433).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectExec(insertHostStmt).WithArgs(1, host.Machine, host.Os, host.VmSlots, host.Description, host.DeploymentTicket).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectCommit().WillReturnError(fmt.Errorf("error"))
			m.ExpectRollback()
			So(createPhysicalHost(c, host), ShouldErrLike, "Internal server error")
		})

		Convey("ok", func() {
			host := &crimson.PhysicalHost{
				Name:    "host",
				Machine: "machine",
				Os:      "os",
				Ipv4:    "127.0.0.1",
			}
			m.ExpectBegin()
			m.ExpectExec(insertNameStmt).WithArgs(host.Name, 2130706433).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectExec(updateIPStmt).WithArgs(1, 2130706433).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectExec(insertHostStmt).WithArgs(1, host.Machine, host.Os, host.VmSlots, host.Description, host.DeploymentTicket).WillReturnResult(sqlmock.NewResult(1, 1))
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
		columns := []string{"h.name", "v.id", "m.name", "o.name", "p.vm_slots", "p.description", "p.deployment_ticket", "i.ipv4"}
		rows := sqlmock.NewRows(columns)

		Convey("query failed", func() {
			selectStmt := `
				^SELECT h.name, v.id, m.name, o.name, p.vm_slots, p.description, p.deployment_ticket, i.ipv4
				FROM physical_hosts p, hostnames h, vlans v, machines m, oses o, ips i
				WHERE p.hostname_id = h.id AND h.vlan_id = v.id AND p.machine_id = m.id AND p.os_id = o.id AND i.hostname_id = h.id AND h.name IN \(\?\) AND v.id IN \(\?\)$
			`
			names := []string{"host"}
			vlans := []int64{0}
			m.ExpectQuery(selectStmt).WithArgs(names[0], vlans[0]).WillReturnError(fmt.Errorf("error"))
			hosts, err := listPhysicalHosts(c, db, names, vlans)
			So(err, ShouldErrLike, "failed to fetch physical hosts")
			So(hosts, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("no names", func() {
			selectStmt := `
				^SELECT h.name, v.id, m.name, o.name, p.vm_slots, p.description, p.deployment_ticket, i.ipv4
				FROM physical_hosts p, hostnames h, vlans v, machines m, oses o, ips i
				WHERE p.hostname_id = h.id AND h.vlan_id = v.id AND p.machine_id = m.id AND p.os_id = o.id AND i.hostname_id = h.id AND v.id IN \(\?\)$
			`
			names := []string{}
			vlans := []int64{0}
			m.ExpectQuery(selectStmt).WithArgs(vlans[0]).WillReturnRows(rows)
			hosts, err := listPhysicalHosts(c, db, names, vlans)
			So(err, ShouldBeNil)
			So(hosts, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("no vlans", func() {
			selectStmt := `
				^SELECT h.name, v.id, m.name, o.name, p.vm_slots, p.description, p.deployment_ticket, i.ipv4
				FROM physical_hosts p, hostnames h, vlans v, machines m, oses o, ips i
				WHERE p.hostname_id = h.id AND h.vlan_id = v.id AND p.machine_id = m.id AND p.os_id = o.id AND i.hostname_id = h.id AND h.name IN \(\?\)$
			`
			names := []string{"host"}
			vlans := []int64{}
			m.ExpectQuery(selectStmt).WithArgs(names[0]).WillReturnRows(rows)
			hosts, err := listPhysicalHosts(c, db, names, vlans)
			So(err, ShouldBeNil)
			So(hosts, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("empty", func() {
			selectStmt := `
				^SELECT h.name, v.id, m.name, o.name, p.vm_slots, p.description, p.deployment_ticket, i.ipv4
				FROM physical_hosts p, hostnames h, vlans v, machines m, oses o, ips i
				WHERE p.hostname_id = h.id AND h.vlan_id = v.id AND p.machine_id = m.id AND p.os_id = o.id AND i.hostname_id = h.id AND h.name IN \(\?\) AND v.id IN \(\?\)$
			`
			names := []string{"host"}
			vlans := []int64{0}
			m.ExpectQuery(selectStmt).WithArgs(names[0], vlans[0]).WillReturnRows(rows)
			hosts, err := listPhysicalHosts(c, db, names, vlans)
			So(err, ShouldBeNil)
			So(hosts, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("non-empty", func() {
			selectStmt := `
				^SELECT h.name, v.id, m.name, o.name, p.vm_slots, p.description, p.deployment_ticket, i.ipv4
				FROM physical_hosts p, hostnames h, vlans v, machines m, oses o, ips i
				WHERE p.hostname_id = h.id AND h.vlan_id = v.id AND p.machine_id = m.id AND p.os_id = o.id AND i.hostname_id = h.id AND h.name IN \(\?,\?\) AND v.id IN \(\?\)$
			`
			names := []string{"host 1", "host 2"}
			vlans := []int64{0}
			rows.AddRow(names[0], vlans[0], "machine 1", "os 1", 1, "", "", 1)
			rows.AddRow(names[1], vlans[0], "machine 2", "os 2", 2, "", "", 2)
			m.ExpectQuery(selectStmt).WithArgs(names[0], names[1], vlans[0]).WillReturnRows(rows)
			hosts, err := listPhysicalHosts(c, db, names, vlans)
			So(err, ShouldBeNil)
			So(hosts, ShouldResemble, []*crimson.PhysicalHost{
				{
					Name:    names[0],
					Vlan:    vlans[0],
					Machine: "machine 1",
					Os:      "os 1",
					VmSlots: 1,
					Ipv4:    "0.0.0.1",
				},
				{
					Name:    names[1],
					Vlan:    vlans[0],
					Machine: "machine 2",
					Os:      "os 2",
					VmSlots: 2,
					Ipv4:    "0.0.0.2",
				},
			})
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("ok", func() {
			selectStmt := `
				^SELECT h.name, v.id, m.name, o.name, p.vm_slots, p.description, p.deployment_ticket, i.ipv4
				FROM physical_hosts p, hostnames h, vlans v, machines m, oses o, ips i
				WHERE p.hostname_id = h.id AND h.vlan_id = v.id AND p.machine_id = m.id AND p.os_id = o.id AND i.hostname_id = h.id$
			`
			names := []string{}
			vlans := []int64{}
			rows.AddRow("host 1", 1, "machine 1", "os 1", 1, "", "", 1)
			rows.AddRow("host 2", 2, "machine 2", "os 2", 2, "", "", 2)
			m.ExpectQuery(selectStmt).WithArgs().WillReturnRows(rows)
			hosts, err := listPhysicalHosts(c, db, names, vlans)
			So(err, ShouldBeNil)
			So(hosts, ShouldResemble, []*crimson.PhysicalHost{
				{
					Name:    "host 1",
					Vlan:    1,
					Machine: "machine 1",
					Os:      "os 1",
					VmSlots: 1,
					Ipv4:    "0.0.0.1",
				},
				{
					Name:    "host 2",
					Vlan:    2,
					Machine: "machine 2",
					Os:      "os 2",
					VmSlots: 2,
					Ipv4:    "0.0.0.2",
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
			Machine: "machine",
			Os:      "os",
			Ipv4:    "127.0.0.1",
		})
		So(err, ShouldErrLike, "hostname is required and must be non-empty")
	})

	Convey("VLAN unspecified", t, func() {
		err := validatePhysicalHostForCreation(&crimson.PhysicalHost{
			Name:    "hostname",
			Vlan:    1,
			Machine: "machine",
			Os:      "os",
			Ipv4:    "127.0.0.1",
		})
		So(err, ShouldErrLike, "VLAN must not be specified, use IP address instead")
	})

	Convey("machine unspecified", t, func() {
		err := validatePhysicalHostForCreation(&crimson.PhysicalHost{
			Name: "hostname",
			Os:   "os",
			Ipv4: "127.0.0.1",
		})
		So(err, ShouldErrLike, "machine is required and must be non-empty")
	})

	Convey("operating system unspecified", t, func() {
		err := validatePhysicalHostForCreation(&crimson.PhysicalHost{
			Name:    "hostname",
			Machine: "machine",
			Ipv4:    "127.0.0.1",
		})
		So(err, ShouldErrLike, "operating system is required and must be non-empty")
	})

	Convey("IPv4 address unspecified", t, func() {
		err := validatePhysicalHostForCreation(&crimson.PhysicalHost{
			Name:    "hostname",
			Machine: "machine",
			Os:      "os",
		})
		So(err, ShouldErrLike, "IPv4 address is required and must be non-empty")
	})

	Convey("IPv4 address invalid", t, func() {
		err := validatePhysicalHostForCreation(&crimson.PhysicalHost{
			Name:    "hostname",
			Machine: "machine",
			Os:      "os",
			Ipv4:    "127.0.0.1/20",
		})
		So(err, ShouldErrLike, "invalid IPv4 address")
	})

	Convey("VM slots negative", t, func() {
		err := validatePhysicalHostForCreation(&crimson.PhysicalHost{
			Name:    "hostname",
			Machine: "machine",
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
			Os:      "os",
			Ipv4:    "127.0.0.1",
		})
		So(err, ShouldBeNil)
	})
}
