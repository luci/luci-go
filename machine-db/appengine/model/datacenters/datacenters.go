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

package datacenters

import (
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/machine-db/api/config/v1"
	"go.chromium.org/luci/machine-db/api/crimson/v1"
	"go.chromium.org/luci/machine-db/appengine/database"
	"go.chromium.org/luci/server/auth"

	"golang.org/x/net/context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
)

// EnsureDatacenters ensures the database contains exactly the given datacenters.
func EnsureDatacenters(c context.Context, datacenterConfigs []*config.DatacenterConfig) error {
	db := database.Get(c)

	// Convert the list of DatacenterConfigs into a map of datacenter name to DatacenterConfig.
	datacenters := make(map[string]*config.DatacenterConfig, len(datacenterConfigs))
	for _, datacenter := range datacenterConfigs {
		datacenters[datacenter.Name] = datacenter
	}

	updateStmt, err := db.PrepareContext(c, "UPDATE datacenters SET description = ? WHERE id = ?")
	if err != nil {
		return errors.Annotate(err, "failed to prepare statement").Err()
	}
	defer updateStmt.Close()

	deleteStmt, err := db.PrepareContext(c, "DELETE FROM datacenters WHERE id = ?")
	if err != nil {
		return errors.Annotate(err, "failed to prepare statement").Err()
	}
	defer deleteStmt.Close()

	insertStmt, err := db.PrepareContext(c, "INSERT INTO datacenters (name, description) VALUES (?, ?)")
	if err != nil {
		return errors.Annotate(err, "failed to prepare statement").Err()
	}
	defer insertStmt.Close()

	rows, err := db.QueryContext(c, "SELECT id, name, description FROM datacenters")
	if err != nil {
		return errors.Annotate(err, "failed to fetch datacenters").Err()
	}
	defer rows.Close()

	// Update existing datacenters, delete datacenters no longer referenced in the config.
	// TODO(smut): Collect the names of datacenters to update/delete and batch the operations.
	for rows.Next() {
		var id int
		var name, description string
		err := rows.Scan(&id, &name, &description)
		if err != nil {
			return errors.Annotate(err, "failed to fetch datacenter").Err()
		}
		if datacenter, ok := datacenters[name]; ok {
			// Datacenter found in the config, update if necessary.
			if description != datacenter.Description {
				if _, err := updateStmt.ExecContext(c, datacenter.Description, id); err != nil {
					// This function is called from cron, so it's okay to return early
					// in the event of an error. Eventually the database will be consistent
					// with the config.
					return errors.Annotate(err, "failed to update datacenter: %s", name).Err()
				}
				logging.Infof(c, "Updated datacenter: %s", name)
			}
			// The config enforces global uniqueness of names, and since the config is the only
			// way to insert datacenters, we don't expect to see the same named datacenter again.
			// Remove it from the map, which will leave only those datacenters which don't exist
			// in the database when the loop terminates.
			delete(datacenters, name)
		} else {
			// Datacenter not found in the config, delete it from the database.
			if _, err := deleteStmt.ExecContext(c, id); err != nil {
				return errors.Annotate(err, "failed to delete datacenter: %s", name).Err()
			}
			logging.Infof(c, "Deleted datacenter: %s", name)
		}
	}

	// Add new datacenters.
	// Iterating over datacenters would be faster because it now only contains those datacenters not
	// present in the database, but non-deterministic because it's a map. Instead iterate over
	// datacenterConfigs deterministically and check if each datacenter is in the datacenters map.
	// If so, it needs to be added to the database.
	// TODO(smut): Batch this.
	for _, datacenter := range datacenterConfigs {
		if _, ok := datacenters[datacenter.Name]; ok {
			if _, err := insertStmt.ExecContext(c, datacenter.Name, datacenter.Description); err != nil {
				return errors.Annotate(err, "failed to add datacenter: %s", datacenter.Name).Err()
			}
			logging.Infof(c, "Added datacenter: %s", datacenter.Name)
		}
	}
	return nil
}

// Logs and returns an internal gRPC error.
func internalRPCError(c context.Context, err error) error {
	errors.Log(c, err)
	return grpc.Errorf(codes.Internal, "Internal server error")
}

// DatacentersServer handles datacenter RPCs.
type DatacentersServer struct {
}

// IsAuthorized returns whether the current user is authorized to use the DatacentersServer API.
func (d *DatacentersServer) IsAuthorized(c context.Context) (bool, error) {
	// TODO(smut): Create other groups for this.
	is, err := auth.IsMember(c, "administrators")
	if err != nil {
		return false, errors.Annotate(err, "failed to check group membership").Err()
	}
	return is, err
}

// GetDatacenters handles a request to retrieve datacenters.
func (d *DatacentersServer) GetDatacenters(c context.Context, req *crimson.DatacentersRequest) (*crimson.DatacentersResponse, error) {
	switch authorized, err := d.IsAuthorized(c); {
	case err != nil:
		return nil, internalRPCError(c, err)
	case !authorized:
		return nil, grpc.Errorf(codes.PermissionDenied, "Unauthorized user")
	}
	names := stringset.New(len(req.Names))
	for _, name := range req.Names {
		names.Add(name)
	}
	datacenters, err := getDatacenters(c, names)
	if err != nil {
		return nil, internalRPCError(c, err)
	}
	return &crimson.DatacentersResponse{
		Datacenters: datacenters,
	}, nil
}

// getDatacenters returns a list of datacenters in the database.
func getDatacenters(c context.Context, names stringset.Set) ([]*crimson.Datacenter, error) {
	db := database.Get(c)
	rows, err := db.QueryContext(c, "SELECT id, name, description from datacenters")
	if err != nil {
		return nil, errors.Annotate(err, "failed to fetch datacenters").Err()
	}
	defer rows.Close()

	var datacenters []*crimson.Datacenter
	for rows.Next() {
		var id int
		var name, description string
		if err = rows.Scan(&id, &name, &description); err != nil {
			return nil, errors.Annotate(err, "failed to fetch datacenter").Err()
		}
		if names.Has(name) || names.Len() == 0 {
			datacenters = append(datacenters, &crimson.Datacenter{
				Name:        name,
				Description: description,
			})
		}
	}
	return datacenters, nil
}
