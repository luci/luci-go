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

func TestCreateMachine(t *testing.T) {
	Convey("createMachine", t, func() {
		db, m, _ := sqlmock.New()
		defer db.Close()
		c := database.With(context.Background(), db)
		insertStmt := `
			^INSERT INTO machines \(name, platform_id, rack_id, description, asset_tag, service_tag, deployment_ticket\)
			VALUES \(\?, \(SELECT id FROM platforms WHERE name = \?\), \(SELECT id FROM racks WHERE name = \?\), \?, \?, \?, \?\)$
		`

		Convey("begin failed", func() {
			machine := &crimson.Machine{
				Name:     "name",
				Platform: "platform",
				Rack:     "rack",
			}
			m.ExpectBegin().WillReturnError(fmt.Errorf("error"))
			So(createMachine(c, machine), ShouldErrLike, "Internal server error")
		})

		Convey("prepare failed", func() {
			machine := &crimson.Machine{
				Name:     "name",
				Platform: "platform",
				Rack:     "rack",
			}
			m.ExpectBegin()
			m.ExpectPrepare(insertStmt).WillReturnError(fmt.Errorf("error"))
			So(createMachine(c, machine), ShouldErrLike, "Internal server error")
		})

		Convey("query failed", func() {
			machine := &crimson.Machine{
				Name:     "name",
				Platform: "platform",
				Rack:     "rack",
			}
			m.ExpectBegin()
			m.ExpectPrepare(insertStmt)
			m.ExpectExec(insertStmt).WithArgs(machine.Name, machine.Platform, machine.Rack, machine.Description, machine.AssetTag, machine.ServiceTag, machine.DeploymentTicket).WillReturnError(fmt.Errorf("error"))
			So(createMachine(c, machine), ShouldErrLike, "Internal server error")
		})

		Convey("duplicate machine", func() {
			machine := &crimson.Machine{
				Name:     "name",
				Platform: "platform",
				Rack:     "rack",
			}
			m.ExpectBegin()
			m.ExpectPrepare(insertStmt)
			m.ExpectExec(insertStmt).WithArgs(machine.Name, machine.Platform, machine.Rack, machine.Description, machine.AssetTag, machine.ServiceTag, machine.DeploymentTicket).WillReturnError(&mysql.MySQLError{Number: mysqlerr.ER_DUP_ENTRY, Message: "error"})
			m.ExpectCommit()
			So(createMachine(c, machine), ShouldErrLike, "duplicate machine")
		})

		Convey("invalid platform", func() {
			machine := &crimson.Machine{
				Name:     "name",
				Platform: "platform",
				Rack:     "rack",
			}
			m.ExpectBegin()
			m.ExpectPrepare(insertStmt)
			m.ExpectExec(insertStmt).WithArgs(machine.Name, machine.Platform, machine.Rack, machine.Description, machine.AssetTag, machine.ServiceTag, machine.DeploymentTicket).WillReturnError(&mysql.MySQLError{Number: mysqlerr.ER_BAD_NULL_ERROR, Message: "'platform_id' is null"})
			m.ExpectCommit()
			So(createMachine(c, machine), ShouldErrLike, "unknown platform")
		})

		Convey("invalid rack", func() {
			machine := &crimson.Machine{
				Name:     "name",
				Platform: "platform",
				Rack:     "rack",
			}
			m.ExpectBegin()
			m.ExpectPrepare(insertStmt)
			m.ExpectExec(insertStmt).WithArgs(machine.Name, machine.Platform, machine.Rack, machine.Description, machine.AssetTag, machine.ServiceTag, machine.DeploymentTicket).WillReturnError(&mysql.MySQLError{Number: mysqlerr.ER_BAD_NULL_ERROR, Message: "'rack_id' is null"})
			m.ExpectCommit()
			So(createMachine(c, machine), ShouldErrLike, "unknown rack")
		})

		Convey("unexpected invalid", func() {
			machine := &crimson.Machine{
				Name:     "name",
				Platform: "platform",
				Rack:     "rack",
			}
			m.ExpectBegin()
			m.ExpectPrepare(insertStmt)
			m.ExpectExec(insertStmt).WithArgs(machine.Name, machine.Platform, machine.Rack, machine.Description, machine.AssetTag, machine.ServiceTag, machine.DeploymentTicket).WillReturnError(&mysql.MySQLError{Number: mysqlerr.ER_BAD_NULL_ERROR, Message: "error"})
			m.ExpectCommit()
			So(createMachine(c, machine), ShouldErrLike, "Internal server error")
		})

		Convey("unexpected error", func() {
			machine := &crimson.Machine{
				Name:     "name",
				Platform: "platform",
				Rack:     "rack",
			}
			m.ExpectBegin()
			m.ExpectPrepare(insertStmt)
			m.ExpectExec(insertStmt).WithArgs(machine.Name, machine.Platform, machine.Rack, machine.Description, machine.AssetTag, machine.ServiceTag, machine.DeploymentTicket).WillReturnError(&mysql.MySQLError{Number: mysqlerr.ER_NO, Message: "name platform_id rack_id"})
			m.ExpectCommit()
			So(createMachine(c, machine), ShouldErrLike, "Internal server error")
		})

		Convey("commit failed", func() {
			machine := &crimson.Machine{
				Name:     "name",
				Platform: "platform",
				Rack:     "rack",
			}
			m.ExpectBegin()
			m.ExpectPrepare(insertStmt)
			m.ExpectExec(insertStmt).WithArgs(machine.Name, machine.Platform, machine.Rack, machine.Description, machine.AssetTag, machine.ServiceTag, machine.DeploymentTicket).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectCommit().WillReturnError(fmt.Errorf("error"))
			So(createMachine(c, machine), ShouldErrLike, "Internal server error")
		})

		Convey("ok", func() {
			machine := &crimson.Machine{
				Name:     "name",
				Platform: "platform",
				Rack:     "rack",
			}
			m.ExpectBegin()
			m.ExpectPrepare(insertStmt)
			m.ExpectExec(insertStmt).WithArgs(machine.Name, machine.Platform, machine.Rack, machine.Description, machine.AssetTag, machine.ServiceTag, machine.DeploymentTicket).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectCommit()
			So(createMachine(c, machine), ShouldBeNil)
		})
	})
}

func TestListMachines(t *testing.T) {
	Convey("listMachines", t, func() {
		db, m, _ := sqlmock.New()
		defer db.Close()
		c := database.With(context.Background(), db)
		selectStmt := `
			^SELECT m.name, p.name, r.name, m.description, m.asset_tag, m.service_tag, m.deployment_ticket
			FROM machines m, platforms p, racks r
			WHERE m.platform_id = p.id AND m.rack_id = r.id$
		`
		columns := []string{"m.name", "p.name", "r.name", "m.description", "m.asset_tag", "m.service_tag", "m.deployment_ticket"}
		rows := sqlmock.NewRows(columns)

		Convey("query failed", func() {
			names := stringset.NewFromSlice("machine")
			m.ExpectQuery(selectStmt).WillReturnError(fmt.Errorf("error"))
			machines, err := listMachines(c, names)
			So(err, ShouldErrLike, "failed to fetch machines")
			So(machines, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("empty", func() {
			names := stringset.NewFromSlice("machine")
			m.ExpectQuery(selectStmt).WillReturnRows(rows)
			machines, err := listMachines(c, names)
			So(err, ShouldBeNil)
			So(machines, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("no matches", func() {
			m.ExpectQuery(selectStmt).WillReturnRows(rows)
			names := stringset.NewFromSlice("machine")
			rows.AddRow("machine 1", "platform 1", "rack 1", "description 1", "", "", "")
			rows.AddRow("machine 2", "platform 2", "rack 2", "description 2", "", "", "")
			machines, err := listMachines(c, names)
			So(err, ShouldBeNil)
			So(machines, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("matches", func() {
			names := stringset.NewFromSlice("machine 2", "machine 3")
			rows.AddRow("machine 1", "platform 1", "rack 1", "description 1", "", "", "")
			rows.AddRow("machine 2", "platform 2", "rack 2", "description 2", "", "", "")
			rows.AddRow("machine 3", "platform 3", "rack 3", "description 3", "", "", "")
			m.ExpectQuery(selectStmt).WillReturnRows(rows)
			machines, err := listMachines(c, names)
			So(err, ShouldBeNil)
			So(machines, ShouldResemble, []*crimson.Machine{
				{
					Name:        "machine 2",
					Platform:    "platform 2",
					Rack:        "rack 2",
					Description: "description 2",
				},
				{
					Name:        "machine 3",
					Platform:    "platform 3",
					Rack:        "rack 3",
					Description: "description 3",
				},
			})
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("ok", func() {
			names := stringset.New(0)
			rows.AddRow("machine 1", "platform 1", "rack 1", "description 1", "", "", "")
			rows.AddRow("machine 2", "platform 2", "rack 2", "description 2", "", "", "")
			m.ExpectQuery(selectStmt).WillReturnRows(rows)
			machines, err := listMachines(c, names)
			So(err, ShouldBeNil)
			So(machines, ShouldResemble, []*crimson.Machine{
				{
					Name:        "machine 1",
					Platform:    "platform 1",
					Rack:        "rack 1",
					Description: "description 1",
				},
				{
					Name:        "machine 2",
					Platform:    "platform 2",
					Rack:        "rack 2",
					Description: "description 2",
				},
			})
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})
	})
}

func TestValidateMachineForCreation(t *testing.T) {
	t.Parallel()

	Convey("machine unspecified", t, func() {
		err := validateMachineForCreation(nil)
		So(err, ShouldErrLike, "machine specification is required")
	})

	Convey("name unspecified", t, func() {
		err := validateMachineForCreation(&crimson.Machine{
			Platform: "platform",
			Rack:     "rack",
		})
		So(err, ShouldErrLike, "machine name is required and must be non-empty")
	})

	Convey("platform unspecified", t, func() {
		err := validateMachineForCreation(&crimson.Machine{
			Name: "name",
			Rack: "rack",
		})
		So(err, ShouldErrLike, "platform is required and must be non-empty")
	})

	Convey("rack unspecified", t, func() {
		err := validateMachineForCreation(&crimson.Machine{
			Name:     "name",
			Platform: "platform",
		})
		So(err, ShouldErrLike, "rack is required and must be non-empty")
	})

	Convey("ok", t, func() {
		err := validateMachineForCreation(&crimson.Machine{
			Name:     "name",
			Platform: "platform",
			Rack:     "rack",
		})
		So(err, ShouldBeNil)
	})
}
