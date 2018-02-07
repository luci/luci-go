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
	"strings"

	"github.com/golang/protobuf/ptypes/empty"
	"golang.org/x/net/context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/VividCortex/mysqlerr"
	"github.com/go-sql-driver/mysql"

	"go.chromium.org/luci/common/data/stringset"
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
	machines, err := listMachines(c, stringset.NewFromSlice(req.Names...))
	if err != nil {
		return nil, internalError(c, err)
	}
	return &crimson.ListMachinesResponse{
		Machines: machines,
	}, nil
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
		INSERT INTO machines (name, platform_id, rack_id, description, asset_tag, service_tag, deployment_ticket, state)
		VALUES (?, (SELECT id FROM platforms WHERE name = ?), (SELECT id FROM racks WHERE name = ?), ?, ?, ?, ?, ?)
	`, m.Name, m.Platform, m.Rack, m.Description, m.AssetTag, m.ServiceTag, m.DeploymentTicket, m.State)
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
			return status.Errorf(codes.NotFound, "unknown platform %q", m.Platform)
		case e.Number == mysqlerr.ER_BAD_NULL_ERROR && strings.Contains(e.Message, "'rack_id'"):
			// e.g. "Error 1048: Column 'rack_id' cannot be null".
			return status.Errorf(codes.NotFound, "unknown rack %q", m.Rack)
		}
		return internalError(c, errors.Annotate(err, "failed to create machine").Err())
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
		return internalError(c, errors.Annotate(err, "failed to delete machine").Err())
	}
	switch rows, err := res.RowsAffected(); {
	case err != nil:
		return internalError(c, errors.Annotate(err, "failed to fetch rows").Err())
	case rows == 0:
		return status.Errorf(codes.NotFound, "unknown machine %q", name)
	case rows == 1:
		return nil
	default:
		// Shouldn't happen because name is unique in the database.
		return internalError(c, errors.Annotate(err, "unexpected number of affected rows %d", rows).Err())
	}
}

// listMachines returns a slice of machines in the database.
func listMachines(c context.Context, names stringset.Set) ([]*crimson.Machine, error) {
	db := database.Get(c)
	rows, err := db.QueryContext(c, `
		SELECT m.name, p.name, r.name, m.description, m.asset_tag, m.service_tag, m.deployment_ticket, m.state
		FROM machines m, platforms p, racks r
		WHERE m.platform_id = p.id
			AND m.rack_id = r.id
	`)
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
			&m.Description,
			&m.AssetTag,
			&m.ServiceTag,
			&m.DeploymentTicket,
			&m.State,
		); err != nil {
			return nil, errors.Annotate(err, "failed to fetch machine").Err()
		}
		// TODO(smut): use the database to filter rather than fetching all entries.
		if matches(m.Name, names) {
			machines = append(machines, m)
		}
	}
	return machines, nil
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
	case m.State == common.State_STATE_UNSPECIFIED:
		return status.Error(codes.InvalidArgument, "state is required")
	default:
		return nil
	}
}
