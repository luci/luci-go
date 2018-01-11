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
	"fmt"
	"strings"

	"golang.org/x/net/context"

	"github.com/VividCortex/mysqlerr"
	"github.com/go-sql-driver/mysql"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/machine-db/api/crimson/v1"
	"go.chromium.org/luci/machine-db/appengine/database"
)

// AddMachine handles a request to add a machine.
func (*Service) AddMachine(c context.Context, req *crimson.AddMachineRequest) (*crimson.AddMachineResponse, error) {
	if errs := validateAddMachineRequest(req); errs != nil {
		return &crimson.AddMachineResponse{
			Name:   req.Name,
			Errors: errs,
		}, nil
	}
	errs, err := addMachine(c, req)
	if err != nil {
		return nil, internalError(c, err)
	}
	return &crimson.AddMachineResponse{
		Name:   req.Name,
		Errors: errs,
	}, nil
}

// GetMachines handles a request to get machines.
func (*Service) GetMachines(c context.Context, req *crimson.GetMachinesRequest) (*crimson.GetMachinesResponse, error) {
	machines, err := getMachines(c, stringset.NewFromSlice(req.Names...))
	if err != nil {
		return nil, internalError(c, err)
	}
	return &crimson.GetMachinesResponse{
		Machines: machines,
	}, nil
}

// addMachine adds a machine to the database.
func addMachine(c context.Context, req *crimson.AddMachineRequest) ([]*crimson.Error, error) {
	tx, err := database.Begin(c)
	if err != nil {
		return nil, errors.Annotate(err, "failed to begin transaction").Err()
	}

	// By setting machines.platform_id and machines.rack_id NOT NULL when setting up the database, we can avoid checking if the given
	// platform and rack are valid. MySQL will turn up NULL for their column values which will be rejected as an error.
	stmt, err := tx.PrepareContext(c, `
		INSERT INTO machines (name, platform_id, rack_id, description, asset_tag, service_tag, deployment_ticket)
		VALUES (?, (SELECT id FROM platforms WHERE name = ?), (SELECT id FROM racks WHERE name = ?), ?, ?, ?, ?)
	`)
	if err != nil {
		return nil, errors.Annotate(err, "failed to prepare statement").Err()
	}
	_, err = stmt.ExecContext(c, req.Name, req.Platform, req.Rack, req.Description, req.AssetTag, req.ServiceTag, req.DeploymentTicket)
	if err != nil {
		e, ok := err.(*mysql.MySQLError)
		if !ok {
			return nil, errors.Annotate(err, "failed to add machine").Err()
		}
		switch {
		case e.Number == mysqlerr.ER_DUP_ENTRY && strings.Contains(e.Message, "name"):
			return []*crimson.Error{
				{
					Code:        crimson.ErrorCode_NOT_UNIQUE,
					Description: fmt.Sprintf("duplicate machine %q", req.Name),
				},
			}, nil
		case e.Number == mysqlerr.ER_BAD_NULL_ERROR && strings.Contains(e.Message, "platform_id"):
			return []*crimson.Error{
				{
					Code:        crimson.ErrorCode_NOT_FOUND,
					Description: fmt.Sprintf("unknown platform %q", req.Platform),
				},
			}, nil
		case e.Number == mysqlerr.ER_BAD_NULL_ERROR && strings.Contains(e.Message, "rack_id"):
			return []*crimson.Error{
				{
					Code:        crimson.ErrorCode_NOT_FOUND,
					Description: fmt.Sprintf("unknown rack %q", req.Rack),
				},
			}, nil
		}
		return nil, errors.Annotate(err, "failed to add machine").Err()
	}

	if err := tx.Commit(); err != nil {
		return nil, errors.Annotate(err, "failed to commit transaction").Err()
	}
	return nil, nil
}

// getMachines returns a slice of machines in the database.
func getMachines(c context.Context, names stringset.Set) ([]*crimson.Machine, error) {
	db := database.Get(c)
	rows, err := db.QueryContext(c, `
		SELECT m.name, p.name, r.name, m.description, m.asset_tag, m.service_tag, m.deployment_ticket
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
		if err = rows.Scan(&m.Name, &m.Platform, &m.Rack, &m.Description, &m.AssetTag, &m.ServiceTag, &m.DeploymentTicket); err != nil {
			return nil, errors.Annotate(err, "failed to fetch machine").Err()
		}
		// TODO(smut): use the database to filter rather than fetching all entries.
		if matches(m.Name, names) {
			machines = append(machines, m)
		}
	}
	return machines, nil
}

// validateAddMachineRequest validates a request to add machines, returning a slice of errors found.
func validateAddMachineRequest(req *crimson.AddMachineRequest) []*crimson.Error {
	var errs []*crimson.Error
	if req.Name == "" {
		errs = append(errs, &crimson.Error{
			Code:        crimson.ErrorCode_INVALID_ARGUMENT,
			Description: "machine name is required and must be non-empty",
		})
	}
	if req.Platform == "" {
		errs = append(errs, &crimson.Error{
			Code:        crimson.ErrorCode_INVALID_ARGUMENT,
			Description: "platform is required and must be non-empty",
		})
	}
	if req.Rack == "" {
		errs = append(errs, &crimson.Error{
			Code:        crimson.ErrorCode_INVALID_ARGUMENT,
			Description: "rack is required and must be non-empty",
		})
	}
	return errs
}
