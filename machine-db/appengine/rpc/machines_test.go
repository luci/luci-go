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

func TestCreateMachine(t *testing.T) {
	Convey("createMachine", t, func() {
		db, m, _ := sqlmock.New()
		defer db.Close()
		c := database.With(context.Background(), db)
		insertStmt := `
			^INSERT INTO machines \(name, platform_id, rack_id, description, asset_tag, service_tag, deployment_ticket, state\)
			VALUES \(\?, \(SELECT id FROM platforms WHERE name = \?\), \(SELECT id FROM racks WHERE name = \?\), \?, \?, \?, \?, \?\)$
		`

		Convey("query failed", func() {
			machine := &crimson.Machine{
				Name:     "name",
				Platform: "platform",
				Rack:     "rack",
				State:    common.State_FREE,
			}
			m.ExpectExec(insertStmt).WithArgs(machine.Name, machine.Platform, machine.Rack, machine.Description, machine.AssetTag, machine.ServiceTag, machine.DeploymentTicket, machine.State).WillReturnError(fmt.Errorf("error"))
			So(createMachine(c, machine), ShouldErrLike, "failed to create machine")
		})

		Convey("duplicate machine", func() {
			machine := &crimson.Machine{
				Name:     "name",
				Platform: "platform",
				Rack:     "rack",
				State:    common.State_FREE,
			}
			m.ExpectExec(insertStmt).WithArgs(machine.Name, machine.Platform, machine.Rack, machine.Description, machine.AssetTag, machine.ServiceTag, machine.DeploymentTicket, machine.State).WillReturnError(&mysql.MySQLError{Number: mysqlerr.ER_DUP_ENTRY, Message: "error"})
			So(createMachine(c, machine), ShouldErrLike, "duplicate machine")
		})

		Convey("invalid platform", func() {
			machine := &crimson.Machine{
				Name:     "name",
				Platform: "platform",
				Rack:     "rack",
				State:    common.State_FREE,
			}
			m.ExpectExec(insertStmt).WithArgs(machine.Name, machine.Platform, machine.Rack, machine.Description, machine.AssetTag, machine.ServiceTag, machine.DeploymentTicket, machine.State).WillReturnError(&mysql.MySQLError{Number: mysqlerr.ER_BAD_NULL_ERROR, Message: "'platform_id' is null"})
			So(createMachine(c, machine), ShouldErrLike, "unknown platform")
		})

		Convey("invalid rack", func() {
			machine := &crimson.Machine{
				Name:     "name",
				Platform: "platform",
				Rack:     "rack",
				State:    common.State_FREE,
			}
			m.ExpectExec(insertStmt).WithArgs(machine.Name, machine.Platform, machine.Rack, machine.Description, machine.AssetTag, machine.ServiceTag, machine.DeploymentTicket, machine.State).WillReturnError(&mysql.MySQLError{Number: mysqlerr.ER_BAD_NULL_ERROR, Message: "'rack_id' is null"})
			So(createMachine(c, machine), ShouldErrLike, "unknown rack")
		})

		Convey("unexpected invalid", func() {
			machine := &crimson.Machine{
				Name:     "name",
				Platform: "platform",
				Rack:     "rack",
				State:    common.State_FREE,
			}
			m.ExpectExec(insertStmt).WithArgs(machine.Name, machine.Platform, machine.Rack, machine.Description, machine.AssetTag, machine.ServiceTag, machine.DeploymentTicket, machine.State).WillReturnError(&mysql.MySQLError{Number: mysqlerr.ER_BAD_NULL_ERROR, Message: "error"})
			So(createMachine(c, machine), ShouldErrLike, "failed to create machine")
		})

		Convey("unexpected error", func() {
			machine := &crimson.Machine{
				Name:     "name",
				Platform: "platform",
				Rack:     "rack",
				State:    common.State_FREE,
			}
			m.ExpectExec(insertStmt).WithArgs(machine.Name, machine.Platform, machine.Rack, machine.Description, machine.AssetTag, machine.ServiceTag, machine.DeploymentTicket, machine.State).WillReturnError(&mysql.MySQLError{Number: mysqlerr.ER_NO, Message: "name platform_id rack_id"})
			So(createMachine(c, machine), ShouldErrLike, "failed to create machine")
		})

		Convey("ok", func() {
			machine := &crimson.Machine{
				Name:     "name",
				Platform: "platform",
				Rack:     "rack",
				State:    common.State_FREE,
			}
			m.ExpectExec(insertStmt).WithArgs(machine.Name, machine.Platform, machine.Rack, machine.Description, machine.AssetTag, machine.ServiceTag, machine.DeploymentTicket, machine.State).WillReturnResult(sqlmock.NewResult(1, 1))
			So(createMachine(c, machine), ShouldBeNil)
		})
	})
}

func TestDeleteMachine(t *testing.T) {
	Convey("deleteMachine", t, func() {
		db, m, _ := sqlmock.New()
		defer db.Close()
		c := database.With(context.Background(), db)
		deleteStmt := `
			^DELETE FROM machines WHERE name = \?$
		`

		Convey("query failed", func() {
			m.ExpectExec(deleteStmt).WithArgs("machine").WillReturnError(fmt.Errorf("error"))
			So(deleteMachine(c, "machine"), ShouldErrLike, "failed to delete machine")
		})

		Convey("referenced", func() {
			m.ExpectExec(deleteStmt).WithArgs("machine").WillReturnError(&mysql.MySQLError{Number: mysqlerr.ER_ROW_IS_REFERENCED_2, Message: "`machine_id`"})
			So(deleteMachine(c, "machine"), ShouldErrLike, "delete entities referencing this machine first")
		})

		Convey("invalid", func() {
			m.ExpectExec(deleteStmt).WithArgs("machine").WillReturnResult(sqlmock.NewResult(1, 0))
			So(deleteMachine(c, "machine"), ShouldErrLike, "unknown machine")
		})

		Convey("ok", func() {
			m.ExpectExec(deleteStmt).WithArgs("machine").WillReturnResult(sqlmock.NewResult(1, 1))
			So(deleteMachine(c, "machine"), ShouldBeNil)
		})
	})
}

func TestListMachines(t *testing.T) {
	Convey("listMachines", t, func() {
		db, m, _ := sqlmock.New()
		defer db.Close()
		c := database.With(context.Background(), db)
		columns := []string{"m.name", "p.name", "r.name", "d.name", "m.description", "m.asset_tag", "m.service_tag", "m.deployment_ticket", "m.state"}
		rows := sqlmock.NewRows(columns)

		Convey("query failed", func() {
			selectStmt := `
				^SELECT m.name, p.name, r.name, d.name, m.description, m.asset_tag, m.service_tag, m.deployment_ticket, m.state
				FROM machines m, platforms p, racks r, datacenters d
				WHERE m.platform_id = p.id AND m.rack_id = r.id AND r.datacenter_id = d.id AND m.name IN \(\?\)$
			`
			req := &crimson.ListMachinesRequest{
				Names: []string{"machine"},
			}
			m.ExpectQuery(selectStmt).WithArgs(req.Names[0]).WillReturnError(fmt.Errorf("error"))
			machines, err := listMachines(c, db, req)
			So(err, ShouldErrLike, "failed to fetch machines")
			So(machines, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("empty request", func() {
			selectStmt := `
				^SELECT m.name, p.name, r.name, d.name, m.description, m.asset_tag, m.service_tag, m.deployment_ticket, m.state
				FROM machines m, platforms p, racks r, datacenters d
				WHERE m.platform_id = p.id AND m.rack_id = r.id AND r.datacenter_id = d.id$
			`
			req := &crimson.ListMachinesRequest{}
			m.ExpectQuery(selectStmt).WillReturnRows(rows)
			machines, err := listMachines(c, db, req)
			So(err, ShouldBeNil)
			So(machines, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("empty response", func() {
			selectStmt := `
				^SELECT m.name, p.name, r.name, d.name, m.description, m.asset_tag, m.service_tag, m.deployment_ticket, m.state
				FROM machines m, platforms p, racks r, datacenters d
				WHERE m.platform_id = p.id AND m.rack_id = r.id AND r.datacenter_id = d.id AND m.name IN \(\?\)$
			`
			req := &crimson.ListMachinesRequest{
				Names: []string{"machine"},
			}
			m.ExpectQuery(selectStmt).WithArgs(req.Names[0]).WillReturnRows(rows)
			machines, err := listMachines(c, db, req)
			So(err, ShouldBeNil)
			So(machines, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("non-empty", func() {
			selectStmt := `
				^SELECT m.name, p.name, r.name, d.name, m.description, m.asset_tag, m.service_tag, m.deployment_ticket, m.state
				FROM machines m, platforms p, racks r, datacenters d
				WHERE m.platform_id = p.id AND m.rack_id = r.id AND r.datacenter_id = d.id AND m.name IN \(\?,\?\)$
			`
			req := &crimson.ListMachinesRequest{
				Names: []string{"machine 1", "machine 2"},
			}
			rows.AddRow(req.Names[0], "platform 1", "rack 1", "datacenter 1", "description 1", "", "", "", 0)
			rows.AddRow(req.Names[1], "platform 2", "rack 2", "datacenter 2", "description 2", "", "", "", common.State_SERVING)
			m.ExpectQuery(selectStmt).WithArgs(req.Names[0], req.Names[1]).WillReturnRows(rows)
			machines, err := listMachines(c, db, req)
			So(err, ShouldBeNil)
			So(machines, ShouldResemble, []*crimson.Machine{
				{
					Name:        req.Names[0],
					Platform:    "platform 1",
					Rack:        "rack 1",
					Datacenter:  "datacenter 1",
					Description: "description 1",
				},
				{
					Name:        req.Names[1],
					Platform:    "platform 2",
					Rack:        "rack 2",
					Datacenter:  "datacenter 2",
					Description: "description 2",
					State:       common.State_SERVING,
				},
			})
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("ok", func() {
			selectStmt := `
				^SELECT m.name, p.name, r.name, d.name, m.description, m.asset_tag, m.service_tag, m.deployment_ticket, m.state
				FROM machines m, platforms p, racks r, datacenters d
				WHERE m.platform_id = p.id AND m.rack_id = r.id AND r.datacenter_id = d.id$
			`
			req := &crimson.ListMachinesRequest{}
			rows.AddRow("machine 1", "platform 1", "rack 1", "datacenter 1", "description 1", "", "", "", 0)
			rows.AddRow("machine 2", "platform 2", "rack 2", "datacenter 2", "description 2", "", "", "", common.State_SERVING)
			m.ExpectQuery(selectStmt).WillReturnRows(rows)
			machines, err := listMachines(c, db, req)
			So(err, ShouldBeNil)
			So(machines, ShouldResemble, []*crimson.Machine{
				{
					Name:        "machine 1",
					Platform:    "platform 1",
					Rack:        "rack 1",
					Datacenter:  "datacenter 1",
					Description: "description 1",
				},
				{
					Name:        "machine 2",
					Platform:    "platform 2",
					Rack:        "rack 2",
					Datacenter:  "datacenter 2",
					Description: "description 2",
					State:       common.State_SERVING,
				},
			})
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})
	})
}

func TestRenameMachine(t *testing.T) {
	Convey("renameMachine", t, func() {
		db, m, _ := sqlmock.New()
		defer db.Close()
		c := database.With(context.Background(), db)
		selectStmt := `
			^SELECT m.name, p.name, r.name, d.name, m.description, m.asset_tag, m.service_tag, m.deployment_ticket, m.state
			FROM machines m, platforms p, racks r, datacenters d
			WHERE m.platform_id = p.id AND m.rack_id = r.id AND r.datacenter_id = d.id AND m.name IN \(\?\)$
		`
		columns := []string{"m.name", "p.name", "r.name", "d.name", "m.description", "m.asset_tag", "m.service_tag", "m.deployment_ticket", "m.state"}
		rows := sqlmock.NewRows(columns)

		Convey("name unspecified", func() {
			machine, err := renameMachine(c, "", "new machine")
			So(err, ShouldErrLike, "machine name is required and must be non-empty")
			So(machine, ShouldBeNil)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("new name unspecified", func() {
			machine, err := renameMachine(c, "old machine", "")
			So(err, ShouldErrLike, "new name is required and must be non-empty")
			So(machine, ShouldBeNil)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("names equal", func() {
			machine, err := renameMachine(c, "machine", "machine")
			So(err, ShouldErrLike, "new name must be different")
			So(machine, ShouldBeNil)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("invalid machine", func() {
			updateStmt := `
				^UPDATE machines
				SET name = \?
				WHERE name = \?$
			`
			m.ExpectBegin()
			m.ExpectExec(updateStmt).WithArgs("new machine", "old machine").WillReturnResult(sqlmock.NewResult(1, 0))
			m.ExpectRollback()
			machine, err := renameMachine(c, "old machine", "new machine")
			So(err, ShouldErrLike, "unknown machine")
			So(machine, ShouldBeNil)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("duplicate machine", func() {
			updateStmt := `
				^UPDATE machines
				SET name = \?
				WHERE name = \?$
			`
			m.ExpectBegin()
			m.ExpectExec(updateStmt).WithArgs("new machine", "old machine").WillReturnError(&mysql.MySQLError{Number: mysqlerr.ER_DUP_ENTRY, Message: "error"})
			m.ExpectRollback()
			machine, err := renameMachine(c, "old machine", "new machine")
			So(err, ShouldErrLike, "duplicate machine")
			So(machine, ShouldBeNil)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("ok", func() {
			updateStmt := `
				^UPDATE machines
				SET name = \?
				WHERE name = \?$
			`
			rows.AddRow("new machine", "platform", "rack", "datacenter", "", "", "", "", common.State_SERVING)
			m.ExpectBegin()
			m.ExpectExec(updateStmt).WithArgs("new machine", "old machine").WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectQuery(selectStmt).WillReturnRows(rows)
			m.ExpectCommit()
			machine, err := renameMachine(c, "old machine", "new machine")
			So(err, ShouldBeNil)
			So(machine, ShouldResemble, &crimson.Machine{
				Name:       "new machine",
				Platform:   "platform",
				Rack:       "rack",
				Datacenter: "datacenter",
				State:      common.State_SERVING,
			})
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})
	})
}

func TestUpdateMachine(t *testing.T) {
	Convey("updateMachine", t, func() {
		db, m, _ := sqlmock.New()
		defer db.Close()
		c := database.With(context.Background(), db)
		selectStmt := `
			^SELECT m.name, p.name, r.name, d.name, m.description, m.asset_tag, m.service_tag, m.deployment_ticket, m.state
			FROM machines m, platforms p, racks r, datacenters d
			WHERE m.platform_id = p.id AND m.rack_id = r.id AND r.datacenter_id = d.id AND m.name IN \(\?\)$
		`
		columns := []string{"m.name", "p.name", "r.name", "d.name", "m.description", "m.asset_tag", "m.service_tag", "m.deployment_ticket", "m.state"}
		rows := sqlmock.NewRows(columns)

		Convey("update platform", func() {
			updateStmt := `
				^UPDATE machines
				SET platform_id = \(SELECT id FROM platforms WHERE name = \?\)
				WHERE name = \?$
			`
			machine := &crimson.Machine{
				Name:     "machine",
				Platform: "platform",
				Rack:     "rack",
				State:    common.State_SERVING,
			}
			mask := &field_mask.FieldMask{
				Paths: []string{
					"platform",
				},
			}
			rows.AddRow(machine.Name, machine.Platform, machine.Rack, machine.Datacenter, machine.Description, machine.AssetTag, machine.ServiceTag, machine.DeploymentTicket, machine.State)
			m.ExpectBegin()
			m.ExpectExec(updateStmt).WithArgs(machine.Platform, machine.Name).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectQuery(selectStmt).WillReturnRows(rows)
			m.ExpectCommit()
			machine, err := updateMachine(c, machine, mask)
			So(err, ShouldBeNil)
			So(machine, ShouldResemble, &crimson.Machine{
				Name:     machine.Name,
				Platform: machine.Platform,
				Rack:     machine.Rack,
				State:    machine.State,
			})
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("update rack", func() {
			updateStmt := `
				^UPDATE machines
				SET rack_id = \(SELECT id FROM racks WHERE name = \?\)
				WHERE name = \?$
			`
			machine := &crimson.Machine{
				Name:     "machine",
				Platform: "platform",
				Rack:     "rack",
				State:    common.State_SERVING,
			}
			mask := &field_mask.FieldMask{
				Paths: []string{
					"rack",
				},
			}
			rows.AddRow(machine.Name, machine.Platform, machine.Rack, machine.Datacenter, machine.Description, machine.AssetTag, machine.ServiceTag, machine.DeploymentTicket, machine.State)
			m.ExpectBegin()
			m.ExpectExec(updateStmt).WithArgs(machine.Rack, machine.Name).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectQuery(selectStmt).WillReturnRows(rows)
			m.ExpectCommit()
			machine, err := updateMachine(c, machine, mask)
			So(err, ShouldBeNil)
			So(machine, ShouldResemble, &crimson.Machine{
				Name:     machine.Name,
				Platform: machine.Platform,
				Rack:     machine.Rack,
				State:    machine.State,
			})
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("update state", func() {
			updateStmt := `
				^UPDATE machines
				SET state = \?
				WHERE name = \?$
			`
			machine := &crimson.Machine{
				Name:     "machine",
				Platform: "platform",
				Rack:     "rack",
				State:    common.State_SERVING,
			}
			mask := &field_mask.FieldMask{
				Paths: []string{
					"state",
				},
			}
			rows.AddRow(machine.Name, machine.Platform, machine.Rack, machine.Datacenter, machine.Description, machine.AssetTag, machine.ServiceTag, machine.DeploymentTicket, machine.State)
			m.ExpectBegin()
			m.ExpectExec(updateStmt).WithArgs(machine.State, machine.Name).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectQuery(selectStmt).WillReturnRows(rows)
			m.ExpectCommit()
			machine, err := updateMachine(c, machine, mask)
			So(err, ShouldBeNil)
			So(machine, ShouldResemble, &crimson.Machine{
				Name:     machine.Name,
				Platform: machine.Platform,
				Rack:     machine.Rack,
				State:    machine.State,
			})
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("ok", func() {
			updateStmt := `
				^UPDATE machines
				SET platform_id = \(SELECT id FROM platforms WHERE name = \?\), rack_id = \(SELECT id FROM racks WHERE name = \?\), state = \?
				WHERE name = \?$
			`
			machine := &crimson.Machine{
				Name:     "machine",
				Platform: "platform",
				Rack:     "rack",
				State:    common.State_SERVING,
			}
			mask := &field_mask.FieldMask{
				Paths: []string{
					"platform",
					"rack",
					"state",
				},
			}
			rows.AddRow(machine.Name, machine.Platform, machine.Rack, machine.Datacenter, machine.Description, machine.AssetTag, machine.ServiceTag, machine.DeploymentTicket, machine.State)
			m.ExpectBegin()
			m.ExpectExec(updateStmt).WithArgs(machine.Platform, machine.Rack, machine.State, machine.Name).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectQuery(selectStmt).WillReturnRows(rows)
			m.ExpectCommit()
			machine, err := updateMachine(c, machine, mask)
			So(err, ShouldBeNil)
			So(machine, ShouldResemble, &crimson.Machine{
				Name:     machine.Name,
				Platform: machine.Platform,
				Rack:     machine.Rack,
				State:    machine.State,
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
			State:    common.State_FREE,
		})
		So(err, ShouldErrLike, "machine name is required and must be non-empty")
	})

	Convey("platform unspecified", t, func() {
		err := validateMachineForCreation(&crimson.Machine{
			Name:  "name",
			Rack:  "rack",
			State: common.State_FREE,
		})
		So(err, ShouldErrLike, "platform is required and must be non-empty")
	})

	Convey("rack unspecified", t, func() {
		err := validateMachineForCreation(&crimson.Machine{
			Name:     "name",
			Platform: "platform",
			State:    common.State_FREE,
		})
		So(err, ShouldErrLike, "rack is required and must be non-empty")
	})

	Convey("datacenter specified", t, func() {
		err := validateMachineForCreation(&crimson.Machine{
			Name:       "name",
			Rack:       "rack",
			Datacenter: "datacenter",
			Platform:   "platform",
			State:      common.State_FREE,
		})
		So(err, ShouldErrLike, "datacenter must not be specified, use rack instead")
	})

	Convey("state unspecified", t, func() {
		err := validateMachineForCreation(&crimson.Machine{
			Name:     "name",
			Platform: "platform",
			Rack:     "rack",
		})
		So(err, ShouldErrLike, "state is required")
	})

	Convey("ok", t, func() {
		err := validateMachineForCreation(&crimson.Machine{
			Name:     "name",
			Platform: "platform",
			Rack:     "rack",
			State:    common.State_FREE,
		})
		So(err, ShouldBeNil)
	})
}

func TestValidateMachineForUpdate(t *testing.T) {
	t.Parallel()

	Convey("machine unspecified", t, func() {
		err := validateMachineForUpdate(nil, &field_mask.FieldMask{
			Paths: []string{
				"platform",
				"rack",
				"state",
				"description",
				"asset_tag",
				"service_tag",
				"deployment_ticket",
			},
		})
		So(err, ShouldErrLike, "machine specification is required")
	})

	Convey("name unspecified", t, func() {
		err := validateMachineForUpdate(&crimson.Machine{
			Platform: "platform",
			Rack:     "rack",
			State:    common.State_SERVING,
		}, &field_mask.FieldMask{
			Paths: []string{
				"platform",
				"rack",
				"state",
				"description",
				"asset_tag",
				"service_tag",
				"deployment_ticket",
			},
		})
		So(err, ShouldErrLike, "machine name is required and must be non-empty")
	})

	Convey("mask unspecified", t, func() {
		err := validateMachineForUpdate(&crimson.Machine{
			Name:     "machine",
			Platform: "platform",
			Rack:     "rack",
			State:    common.State_SERVING,
		}, nil)
		So(err, ShouldErrLike, "update mask is required")
	})

	Convey("no paths", t, func() {
		err := validateMachineForUpdate(&crimson.Machine{
			Name:     "machine",
			Platform: "platform",
			Rack:     "rack",
			State:    common.State_SERVING,
		}, &field_mask.FieldMask{})
		So(err, ShouldErrLike, "at least one update mask path is required")
	})

	Convey("unexpected name", t, func() {
		err := validateMachineForUpdate(&crimson.Machine{
			Name:     "machine",
			Platform: "platform",
			Rack:     "rack",
			State:    common.State_SERVING,
		}, &field_mask.FieldMask{
			Paths: []string{
				"name",
			},
		})
		So(err, ShouldErrLike, "machine name cannot be updated")
	})

	Convey("platform unspecified", t, func() {
		err := validateMachineForUpdate(&crimson.Machine{
			Name:  "machine",
			Rack:  "rack",
			State: common.State_SERVING,
		}, &field_mask.FieldMask{
			Paths: []string{
				"platform",
				"rack",
				"state",
				"description",
				"asset_tag",
				"service_tag",
				"deployment_ticket",
			},
		})
		So(err, ShouldErrLike, "platform is required and must be non-empty")
	})

	Convey("rack unspecified", t, func() {
		err := validateMachineForUpdate(&crimson.Machine{
			Name:     "machine",
			Platform: "platform",
		}, &field_mask.FieldMask{
			Paths: []string{
				"platform",
				"rack",
				"state",
				"description",
				"asset_tag",
				"service_tag",
				"deployment_ticket",
			},
		})
		So(err, ShouldErrLike, "rack is required and must be non-empty")
	})

	Convey("datacenter unspecified", t, func() {
		err := validateMachineForUpdate(&crimson.Machine{
			Name:       "machine",
			Rack:       "rack",
			Datacenter: "datacenter",
			Platform:   "platform",
		}, &field_mask.FieldMask{
			Paths: []string{
				"platform",
				"rack",
				"datacenter",
				"state",
				"description",
				"asset_tag",
				"service_tag",
				"deployment_ticket",
			},
		})
		So(err, ShouldErrLike, "datacenter cannot be updated, update rack instead")
	})

	Convey("state unspecified", t, func() {
		err := validateMachineForUpdate(&crimson.Machine{
			Name:     "machine",
			Platform: "platform",
			Rack:     "rack",
		}, &field_mask.FieldMask{
			Paths: []string{
				"platform",
				"rack",
				"state",
				"description",
				"asset_tag",
				"service_tag",
				"deployment_ticket",
			},
		})
		So(err, ShouldErrLike, "state is required")
	})

	Convey("unsupported path", t, func() {
		err := validateMachineForUpdate(&crimson.Machine{
			Name:     "machine",
			Platform: "platform",
			Rack:     "rack",
			State:    common.State_SERVING,
		}, &field_mask.FieldMask{
			Paths: []string{
				"unknown",
			},
		})
		So(err, ShouldErrLike, "unsupported update mask path")
	})

	Convey("duplicate path", t, func() {
		err := validateMachineForUpdate(&crimson.Machine{
			Name:     "machine",
			Platform: "platform",
			Rack:     "rack",
			State:    common.State_SERVING,
		}, &field_mask.FieldMask{
			Paths: []string{
				"platform",
				"rack",
				"state",
				"description",
				"asset_tag",
				"service_tag",
				"deployment_ticket",
				"deployment_ticket",
			},
		})
		So(err, ShouldErrLike, "duplicate update mask path")
	})

	Convey("ok", t, func() {
		err := validateMachineForUpdate(&crimson.Machine{
			Name:     "machine",
			Platform: "platform",
			Rack:     "rack",
			State:    common.State_SERVING,
		}, &field_mask.FieldMask{
			Paths: []string{
				"platform",
				"rack",
				"state",
				"description",
				"asset_tag",
				"service_tag",
				"deployment_ticket",
			},
		})
		So(err, ShouldBeNil)
	})
}
