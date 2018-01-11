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

func TestAddMachine(t *testing.T) {
	Convey("addMachine", t, func() {
		db, m, _ := sqlmock.New()
		defer db.Close()
		c := database.With(context.Background(), db)
		insertStmt := `
			^INSERT INTO machines \(name, platform_id, rack_id, description, asset_tag, service_tag, deployment_ticket\)
			VALUES \(\?, \(SELECT id FROM platforms WHERE name = \?\), \(SELECT id FROM racks WHERE name = \?\), \?, \?, \?, \?\)$
		`

		Convey("begin failed", func() {
			req := &crimson.AddMachineRequest{
				Name:     "name",
				Platform: "platform",
				Rack:     "rack",
			}
			m.ExpectBegin().WillReturnError(fmt.Errorf("error"))
			errs, err := addMachine(c, req)
			So(err, ShouldErrLike, "failed to begin transaction")
			So(errs, ShouldBeNil)
		})

		Convey("prepare failed", func() {
			req := &crimson.AddMachineRequest{
				Name:     "name",
				Platform: "platform",
				Rack:     "rack",
			}
			m.ExpectBegin()
			m.ExpectPrepare(insertStmt).WillReturnError(fmt.Errorf("error"))
			errs, err := addMachine(c, req)
			So(err, ShouldErrLike, "failed to prepare statement")
			So(errs, ShouldBeNil)
		})

		Convey("query failed", func() {
			req := &crimson.AddMachineRequest{
				Name:     "name",
				Platform: "platform",
				Rack:     "rack",
			}
			m.ExpectBegin()
			m.ExpectPrepare(insertStmt)
			m.ExpectExec(insertStmt).WithArgs(req.Name, req.Platform, req.Rack, req.Description, req.AssetTag, req.ServiceTag, req.DeploymentTicket).WillReturnError(fmt.Errorf("error"))
			errs, err := addMachine(c, req)
			So(err, ShouldErrLike, "failed to add machine")
			So(errs, ShouldBeNil)
		})

		Convey("duplicate machine", func() {
			req := &crimson.AddMachineRequest{
				Name:     "name",
				Platform: "platform",
				Rack:     "rack",
			}
			m.ExpectBegin()
			m.ExpectPrepare(insertStmt)
			m.ExpectExec(insertStmt).WithArgs(req.Name, req.Platform, req.Rack, req.Description, req.AssetTag, req.ServiceTag, req.DeploymentTicket).WillReturnError(&mysql.MySQLError{Number: mysqlerr.ER_DUP_ENTRY, Message: "duplicate machine name"})
			m.ExpectCommit()
			errs, err := addMachine(c, req)
			So(err, ShouldBeNil)
			So(errs, ShouldResemble, []*crimson.Error{
				{
					Code:        crimson.ErrorCode_NOT_UNIQUE,
					Description: "duplicate machine \"name\"",
				},
			})
		})

		Convey("unexpected duplicate", func() {
			req := &crimson.AddMachineRequest{
				Name:     "name",
				Platform: "platform",
				Rack:     "rack",
			}
			m.ExpectBegin()
			m.ExpectPrepare(insertStmt)
			m.ExpectExec(insertStmt).WithArgs(req.Name, req.Platform, req.Rack, req.Description, req.AssetTag, req.ServiceTag, req.DeploymentTicket).WillReturnError(&mysql.MySQLError{Number: mysqlerr.ER_DUP_ENTRY, Message: "error"})
			m.ExpectCommit()
			errs, err := addMachine(c, req)
			So(err, ShouldErrLike, "failed to add machine")
			So(errs, ShouldBeNil)
		})

		Convey("invalid platform", func() {
			req := &crimson.AddMachineRequest{
				Name:     "name",
				Platform: "platform",
				Rack:     "rack",
			}
			m.ExpectBegin()
			m.ExpectPrepare(insertStmt)
			m.ExpectExec(insertStmt).WithArgs(req.Name, req.Platform, req.Rack, req.Description, req.AssetTag, req.ServiceTag, req.DeploymentTicket).WillReturnError(&mysql.MySQLError{Number: mysqlerr.ER_BAD_NULL_ERROR, Message: "platform_id is null"})
			m.ExpectCommit()
			errs, err := addMachine(c, req)
			So(err, ShouldBeNil)
			So(errs, ShouldResemble, []*crimson.Error{
				{
					Code:        crimson.ErrorCode_NOT_FOUND,
					Description: "unknown platform \"platform\"",
				},
			})
		})

		Convey("invalid rack", func() {
			req := &crimson.AddMachineRequest{
				Name:     "name",
				Platform: "platform",
				Rack:     "rack",
			}
			m.ExpectBegin()
			m.ExpectPrepare(insertStmt)
			m.ExpectExec(insertStmt).WithArgs(req.Name, req.Platform, req.Rack, req.Description, req.AssetTag, req.ServiceTag, req.DeploymentTicket).WillReturnError(&mysql.MySQLError{Number: mysqlerr.ER_BAD_NULL_ERROR, Message: "rack_id is null"})
			m.ExpectCommit()
			errs, err := addMachine(c, req)
			So(err, ShouldBeNil)
			So(errs, ShouldResemble, []*crimson.Error{
				{
					Code:        crimson.ErrorCode_NOT_FOUND,
					Description: "unknown rack \"rack\"",
				},
			})
		})

		Convey("unexpected invalid", func() {
			req := &crimson.AddMachineRequest{
				Name:     "name",
				Platform: "platform",
				Rack:     "rack",
			}
			m.ExpectBegin()
			m.ExpectPrepare(insertStmt)
			m.ExpectExec(insertStmt).WithArgs(req.Name, req.Platform, req.Rack, req.Description, req.AssetTag, req.ServiceTag, req.DeploymentTicket).WillReturnError(&mysql.MySQLError{Number: mysqlerr.ER_BAD_NULL_ERROR, Message: "error"})
			m.ExpectCommit()
			errs, err := addMachine(c, req)
			So(err, ShouldErrLike, "failed to add machine")
			So(errs, ShouldBeNil)
		})

		Convey("unexpected error", func() {
			req := &crimson.AddMachineRequest{
				Name:     "name",
				Platform: "platform",
				Rack:     "rack",
			}
			m.ExpectBegin()
			m.ExpectPrepare(insertStmt)
			m.ExpectExec(insertStmt).WithArgs(req.Name, req.Platform, req.Rack, req.Description, req.AssetTag, req.ServiceTag, req.DeploymentTicket).WillReturnError(&mysql.MySQLError{Number: mysqlerr.ER_NO, Message: "name platform_id rack_id"})
			m.ExpectCommit()
			errs, err := addMachine(c, req)
			So(err, ShouldErrLike, "failed to add machine")
			So(errs, ShouldBeNil)
		})

		Convey("commit failed", func() {
			req := &crimson.AddMachineRequest{
				Name:     "name",
				Platform: "platform",
				Rack:     "rack",
			}
			m.ExpectBegin()
			m.ExpectPrepare(insertStmt)
			m.ExpectExec(insertStmt).WithArgs(req.Name, req.Platform, req.Rack, req.Description, req.AssetTag, req.ServiceTag, req.DeploymentTicket).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectCommit().WillReturnError(fmt.Errorf("error"))
			errs, err := addMachine(c, req)
			So(err, ShouldErrLike, "failed to commit transaction")
			So(errs, ShouldBeNil)
		})

		Convey("ok", func() {
			req := &crimson.AddMachineRequest{
				Name:     "name",
				Platform: "platform",
				Rack:     "rack",
			}
			m.ExpectBegin()
			m.ExpectPrepare(insertStmt)
			m.ExpectExec(insertStmt).WithArgs(req.Name, req.Platform, req.Rack, req.Description, req.AssetTag, req.ServiceTag, req.DeploymentTicket).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectCommit()
			errs, err := addMachine(c, req)
			So(err, ShouldBeNil)
			So(errs, ShouldBeNil)
		})
	})
}

func TestGetMachines(t *testing.T) {
	Convey("getMachines", t, func() {
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
			machines, err := getMachines(c, names)
			So(err, ShouldErrLike, "failed to fetch machines")
			So(machines, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("empty", func() {
			names := stringset.NewFromSlice("machine")
			m.ExpectQuery(selectStmt).WillReturnRows(rows)
			machines, err := getMachines(c, names)
			So(err, ShouldBeNil)
			So(machines, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("no matches", func() {
			m.ExpectQuery(selectStmt).WillReturnRows(rows)
			names := stringset.NewFromSlice("machine")
			rows.AddRow("machine 1", "platform 1", "rack 1", "description 1", "", "", "")
			rows.AddRow("machine 2", "platform 2", "rack 2", "description 2", "", "", "")
			machines, err := getMachines(c, names)
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
			machines, err := getMachines(c, names)
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
			machines, err := getMachines(c, names)
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

func TestValidateAddMachineRequest(t *testing.T) {
	t.Parallel()

	Convey("name unspecified", t, func() {
		errs := validateAddMachineRequest(&crimson.AddMachineRequest{
			Platform: "platform",
			Rack:     "rack",
		})
		So(errs, ShouldResemble, []*crimson.Error{
			{
				Code:        crimson.ErrorCode_INVALID_ARGUMENT,
				Description: "machine name is required and must be non-empty",
			},
		})
	})

	Convey("platform unspecified", t, func() {
		errs := validateAddMachineRequest(&crimson.AddMachineRequest{
			Name: "name",
			Rack: "rack",
		})
		So(errs, ShouldResemble, []*crimson.Error{
			{
				Code:        crimson.ErrorCode_INVALID_ARGUMENT,
				Description: "platform is required and must be non-empty",
			},
		})
	})

	Convey("rack unspecified", t, func() {
		errs := validateAddMachineRequest(&crimson.AddMachineRequest{
			Name:     "name",
			Platform: "platform",
		})
		So(errs, ShouldResemble, []*crimson.Error{
			{
				Code:        crimson.ErrorCode_INVALID_ARGUMENT,
				Description: "rack is required and must be non-empty",
			},
		})
	})

	Convey("multiple", t, func() {
		errs := validateAddMachineRequest(&crimson.AddMachineRequest{})
		So(errs, ShouldResemble, []*crimson.Error{
			{
				Code:        crimson.ErrorCode_INVALID_ARGUMENT,
				Description: "machine name is required and must be non-empty",
			},
			{
				Code:        crimson.ErrorCode_INVALID_ARGUMENT,
				Description: "platform is required and must be non-empty",
			},
			{
				Code:        crimson.ErrorCode_INVALID_ARGUMENT,
				Description: "rack is required and must be non-empty",
			},
		})
	})

	Convey("ok", t, func() {
		errs := validateAddMachineRequest(&crimson.AddMachineRequest{
			Name:     "name",
			Platform: "platform",
			Rack:     "rack",
		})
		So(errs, ShouldBeNil)
	})
}
