// Copyright 2018 The LUCI Authors.
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
	"context"
	"strings"

	"github.com/golang/protobuf/ptypes/empty"

	"google.golang.org/genproto/protobuf/field_mask"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/Masterminds/squirrel"
	"github.com/VividCortex/mysqlerr"
	"github.com/go-sql-driver/mysql"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/machine-db/api/common/v1"
	"go.chromium.org/luci/machine-db/api/crimson/v1"
	"go.chromium.org/luci/machine-db/appengine/database"
)

// CreateMachine handles a request to create a new machine.
func (*Service) CreateMachine(c context.Context, req *crimson.CreateMachineRequest) (*crimson.Machine, error) {
	if err := createMachine(c, req.Machine); err != nil {
		return nil, err
	}
	return req.Machine, nil
}

// DeleteMachine handles a request to delete an existing machine.
func (*Service) DeleteMachine(c context.Context, req *crimson.DeleteMachineRequest) (*empty.Empty, error) {
	if err := deleteMachine(c, req.Name); err != nil {
		return nil, err
	}
	return &empty.Empty{}, nil
}

// ListMachines handles a request to list machines.
func (*Service) ListMachines(c context.Context, req *crimson.ListMachinesRequest) (*crimson.ListMachinesResponse, error) {
	machines, err := listMachines(c, database.Get(c), req)
	if err != nil {
		return nil, err
	}
	return &crimson.ListMachinesResponse{
		Machines: machines,
	}, nil
}

// RenameMachine handles a request to rename an existing machine.
func (*Service) RenameMachine(c context.Context, req *crimson.RenameMachineRequest) (*crimson.Machine, error) {
	return renameMachine(c, req.Name, req.NewName)
}

// UpdateMachine handles a request to update an existing machine.
func (*Service) UpdateMachine(c context.Context, req *crimson.UpdateMachineRequest) (*crimson.Machine, error) {
	return updateMachine(c, req.Machine, req.UpdateMask)
}

// createMachine creates a new machine in the database.
func createMachine(c context.Context, m *crimson.Machine) error {
	if err := validateMachineForCreation(m); err != nil {
		return err
	}

	db := database.Get(c)
	// By setting machines.platform_id and machines.rack_id NOT NULL when setting up the database, we can avoid checking if the given
	// platform and rack are valid. MySQL will turn up NULL for their column values which will be rejected as an error.
	_, err := db.ExecContext(c, `
		INSERT INTO machines (name, platform_id, rack_id, description, asset_tag, service_tag, deployment_ticket, drac_password, state)
		VALUES (?, (SELECT id FROM platforms WHERE name = ?), (SELECT id FROM racks WHERE name = ?), ?, ?, ?, ?, ?, ?)
	`, m.Name, m.Platform, m.Rack, m.Description, m.AssetTag, m.ServiceTag, m.DeploymentTicket, m.DracPassword, m.State)
	if err != nil {
		switch e, ok := err.(*mysql.MySQLError); {
		case !ok:
			// Type assertion failed.
		case e.Number == mysqlerr.ER_DUP_ENTRY:
			// e.g. "Error 1062: Duplicate entry 'machine-name' for key 'name'".
			// Name is the only required unique field (ID is required unique, but it's auto-incremented).
			return status.Errorf(codes.AlreadyExists, "duplicate machine %q", m.Name)
		case e.Number == mysqlerr.ER_BAD_NULL_ERROR && strings.Contains(e.Message, "'platform_id'"):
			// e.g. "Error 1048: Column 'platform_id' cannot be null".
			return status.Errorf(codes.NotFound, "platform %q does not exist", m.Platform)
		case e.Number == mysqlerr.ER_BAD_NULL_ERROR && strings.Contains(e.Message, "'rack_id'"):
			// e.g. "Error 1048: Column 'rack_id' cannot be null".
			return status.Errorf(codes.NotFound, "rack %q does not exist", m.Rack)
		}
		return errors.Annotate(err, "failed to create machine").Err()
	}
	return nil
}

// deleteMachine deletes an existing machine from the database.
func deleteMachine(c context.Context, name string) error {
	if name == "" {
		return status.Error(codes.InvalidArgument, "machine name is required and must be non-empty")
	}

	db := database.Get(c)
	res, err := db.ExecContext(c, `
		DELETE FROM machines WHERE name = ?
	`, name)
	if err != nil {
		switch e, ok := err.(*mysql.MySQLError); {
		case !ok:
			// Type assertion failed.
		case e.Number == mysqlerr.ER_ROW_IS_REFERENCED_2 && strings.Contains(e.Message, "`machine_id`"):
			// e.g. "Error 1452: Cannot add or update a child row: a foreign key constraint fails (FOREIGN KEY (`machine_id`) REFERENCES `machines` (`id`))".
			return status.Errorf(codes.FailedPrecondition, "delete entities referencing this machine first")
		}
		return errors.Annotate(err, "failed to delete machine").Err()
	}
	switch rows, err := res.RowsAffected(); {
	case err != nil:
		return errors.Annotate(err, "failed to fetch affected rows").Err()
	case rows == 0:
		return status.Errorf(codes.NotFound, "machine %q does not exist", name)
	}
	return nil
}

// listMachines returns a slice of machines in the database.
func listMachines(c context.Context, q database.QueryerContext, req *crimson.ListMachinesRequest) ([]*crimson.Machine, error) {
	stmt := squirrel.Select(
		"m.name",
		"p.name",
		"r.name",
		"d.name",
		"m.description",
		"m.asset_tag",
		"m.service_tag",
		"m.deployment_ticket",
		"m.drac_password",
		"m.state",
	)
	stmt = stmt.From("machines m, platforms p, racks r, datacenters d").
		Where("m.platform_id = p.id").Where("m.rack_id = r.id").Where("r.datacenter_id = d.id")
	stmt = selectInString(stmt, "m.name", req.Names)
	stmt = selectInString(stmt, "p.name", req.Platforms)
	stmt = selectInString(stmt, "r.name", req.Racks)
	stmt = selectInString(stmt, "d.name", req.Datacenters)
	stmt = selectInState(stmt, "m.state", req.States)
	query, args, err := stmt.ToSql()
	if err != nil {
		return nil, errors.Annotate(err, "failed to generate statement").Err()
	}

	rows, err := q.QueryContext(c, query, args...)
	if err != nil {
		return nil, errors.Annotate(err, "failed to fetch machines").Err()
	}
	defer rows.Close()
	var machines []*crimson.Machine
	for rows.Next() {
		m := &crimson.Machine{}
		if err = rows.Scan(
			&m.Name,
			&m.Platform,
			&m.Rack,
			&m.Datacenter,
			&m.Description,
			&m.AssetTag,
			&m.ServiceTag,
			&m.DeploymentTicket,
			&m.DracPassword,
			&m.State,
		); err != nil {
			return nil, errors.Annotate(err, "failed to fetch machine").Err()
		}
		machines = append(machines, m)
	}
	return machines, nil
}

// renameMachine renames an existing machine in the database.
func renameMachine(c context.Context, name, newName string) (*crimson.Machine, error) {
	switch {
	case name == "":
		return nil, status.Error(codes.InvalidArgument, "machine name is required and must be non-empty")
	case newName == "":
		return nil, status.Error(codes.InvalidArgument, "new name is required and must be non-empty")
	case name == newName:
		return nil, status.Error(codes.InvalidArgument, "new name must be different")
	}

	tx, err := database.Begin(c)
	if err != nil {
		return nil, errors.Annotate(err, "failed to begin transaction").Err()
	}
	defer tx.MaybeRollback(c)

	res, err := tx.ExecContext(c, `
		UPDATE machines
		SET name = ?
		WHERE name = ?
	`, newName, name)
	if err != nil {
		switch e, ok := err.(*mysql.MySQLError); {
		case !ok:
			// Type assertion failed.
		case e.Number == mysqlerr.ER_DUP_ENTRY:
			// e.g. "Error 1062: Duplicate entry 'machine-name' for key 'name'".
			return nil, status.Errorf(codes.AlreadyExists, "duplicate machine %q", newName)
		}
		return nil, errors.Annotate(err, "failed to rename machine").Err()
	}
	switch rows, err := res.RowsAffected(); {
	case err != nil:
		return nil, errors.Annotate(err, "failed to fetch affected rows").Err()
	case rows == 0:
		return nil, status.Errorf(codes.NotFound, "machine %q does not exist", name)
	}

	machines, err := listMachines(c, tx, &crimson.ListMachinesRequest{
		Names: []string{newName},
	})
	if err != nil {
		return nil, errors.Annotate(err, "failed to fetch renamed machine").Err()
	}

	if err := tx.Commit(); err != nil {
		return nil, errors.Annotate(err, "failed to commit transaction").Err()
	}
	return machines[0], nil
}

// updateMachine updates an existing machine in the database.
func updateMachine(c context.Context, m *crimson.Machine, mask *field_mask.FieldMask) (*crimson.Machine, error) {
	if err := validateMachineForUpdate(m, mask); err != nil {
		return nil, err
	}
	stmt := squirrel.Update("machines")
	for _, path := range mask.Paths {
		switch path {
		case "platform":
			stmt = stmt.Set("platform_id", squirrel.Expr("(SELECT id FROM platforms WHERE name = ?)", m.Platform))
		case "rack":
			stmt = stmt.Set("rack_id", squirrel.Expr("(SELECT id FROM racks WHERE name = ?)", m.Rack))
		case "state":
			stmt = stmt.Set("state", m.State)
		case "description":
			stmt = stmt.Set("description", m.Description)
		case "asset_tag":
			stmt = stmt.Set("asset_tag", m.AssetTag)
		case "service_tag":
			stmt = stmt.Set("service_tag", m.ServiceTag)
		case "deployment_ticket":
			stmt = stmt.Set("deployment_ticket", m.DeploymentTicket)
		case "drac_password":
			stmt = stmt.Set("drac_password", m.DracPassword)
		}
	}
	stmt = stmt.Where("name = ?", m.Name)
	query, args, err := stmt.ToSql()
	if err != nil {
		return nil, errors.Annotate(err, "failed to generate statement").Err()
	}

	tx, err := database.Begin(c)
	if err != nil {
		return nil, errors.Annotate(err, "failed to begin transaction").Err()
	}
	defer tx.MaybeRollback(c)

	_, err = tx.ExecContext(c, query, args...)
	if err != nil {
		switch e, ok := err.(*mysql.MySQLError); {
		case !ok:
			// Type assertion failed.
		case e.Number == mysqlerr.ER_BAD_NULL_ERROR && strings.Contains(e.Message, "'platform_id'"):
			// e.g. "Error 1048: Column 'platform_id' cannot be null".
			return nil, status.Errorf(codes.NotFound, "platform %q does not exist", m.Platform)
		case e.Number == mysqlerr.ER_BAD_NULL_ERROR && strings.Contains(e.Message, "'rack_id'"):
			// e.g. "Error 1048: Column 'rack_id' cannot be null".
			return nil, status.Errorf(codes.NotFound, "rack %q does not exist", m.Rack)
		}
		return nil, errors.Annotate(err, "failed to update machine").Err()
	}
	// The number of rows affected cannot distinguish between zero because the machine didn't exist
	// and zero because the row already matched, so skip looking at the number of rows affected.

	machines, err := listMachines(c, tx, &crimson.ListMachinesRequest{
		Names: []string{m.Name},
	})
	switch {
	case err != nil:
		return nil, errors.Annotate(err, "failed to fetch updated machine").Err()
	case len(machines) == 0:
		return nil, status.Errorf(codes.NotFound, "machine %q does not exist", m.Name)
	}

	if err := tx.Commit(); err != nil {
		return nil, errors.Annotate(err, "failed to commit transaction").Err()
	}
	return machines[0], nil
}

// validateMachineForCreation validates a machine for creation.
func validateMachineForCreation(m *crimson.Machine) error {
	switch {
	case m == nil:
		return status.Error(codes.InvalidArgument, "machine specification is required")
	case m.Name == "":
		return status.Error(codes.InvalidArgument, "machine name is required and must be non-empty")
	case m.Platform == "":
		return status.Error(codes.InvalidArgument, "platform is required and must be non-empty")
	case m.Rack == "":
		return status.Error(codes.InvalidArgument, "rack is required and must be non-empty")
	case m.Datacenter != "":
		return status.Error(codes.InvalidArgument, "datacenter must not be specified, use rack instead")
	case m.State == common.State_STATE_UNSPECIFIED:
		return status.Error(codes.InvalidArgument, "state is required")
	default:
		return nil
	}
}

// validateMachineForUpdate validates a machine for update.
func validateMachineForUpdate(m *crimson.Machine, mask *field_mask.FieldMask) error {
	switch err := validateUpdateMask(mask); {
	case m == nil:
		return status.Error(codes.InvalidArgument, "machine specification is required")
	case m.Name == "":
		return status.Error(codes.InvalidArgument, "machine name is required and must be non-empty")
	case err != nil:
		return err
	}
	for _, path := range mask.Paths {
		switch path {
		case "name":
			return status.Error(codes.InvalidArgument, "machine name cannot be updated, delete and create a new machine instead")
		case "platform":
			if m.Platform == "" {
				return status.Error(codes.InvalidArgument, "platform is required and must be non-empty")
			}
		case "rack":
			if m.Rack == "" {
				return status.Error(codes.InvalidArgument, "rack is required and must be non-empty")
			}
		case "datacenter":
			return status.Error(codes.InvalidArgument, "datacenter cannot be updated, update rack instead")
		case "state":
			if m.State == common.State_STATE_UNSPECIFIED {
				return status.Error(codes.InvalidArgument, "state is required")
			}
		case "description":
			// Empty description is allowed, nothing to validate.
		case "asset_tag":
			// Empty asset tag is allowed, nothing to validate.
		case "service_tag":
			// Empty service tag is allowed, nothing to validate.
		case "deployment_ticket":
			// Empty deployment ticket is allowed, nothing to validate.
		case "drac_password":
			// Empty DRAC password is allowed, nothing to validate.
		default:
			return status.Errorf(codes.InvalidArgument, "unsupported update mask path %q", path)
		}
	}
	return nil
}
