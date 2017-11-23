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

// DatacenterDiff encapsulates differences between a datacenter in the database and config.
type DatacenterDiff struct {
	// Config is the datacenter entry in the config.
	Config *config.DatacenterConfig
	// Database is the datacenter entry in the database.
	Database *config.DatacenterConfig
	// Id is the row ID of this datacenter.
	Id int
}

// Differences encapsulates the differences between datacenters in the database and config.
type Differences struct {
	// Extraneous is a list of datacenters present in the database but not the config.
	Extraneous []*DatacenterDiff
	// Mismatched is a list of datacenters present in the database but not matching the config.
	Mismatched []*DatacenterDiff
	// Missing is a list of datacenters present in the config but not the database.
	Missing []*DatacenterDiff
}

// getDifferences returns the state of datacenters in the database with respect to the config.
func getDifferences(c context.Context, datacenterConfigs []*config.DatacenterConfig) (*Differences, error) {
	// Convert the list of DatacenterConfigs into a map of datacenter name to DatacenterConfig.
	datacenters := make(map[string]*config.DatacenterConfig, len(datacenterConfigs))
	for _, datacenter := range datacenterConfigs {
		datacenters[datacenter.Name] = datacenter
	}

	db := database.Get(c)
	rows, err := db.QueryContext(c, "SELECT id, name, description FROM datacenters")
	if err != nil {
		return nil, errors.Annotate(err, "failed to fetch datacenters").Err()
	}
	defer rows.Close()

	differences := &Differences{}
	for rows.Next() {
		diff := &DatacenterDiff{
			Database: &config.DatacenterConfig{},
		}
		if err := rows.Scan(&diff.Id, &diff.Database.Name, &diff.Database.Description); err != nil {
			return nil, errors.Annotate(err, "failed to fetch datacenter").Err()
		}
		if datacenter, ok := datacenters[diff.Database.Name]; ok {
			// Datacenter found in the config.
			diff.Config = datacenter
			if diff.Database.Description != diff.Config.Description {
				// Datacenter doesn't match the config.
				differences.Mismatched = append(differences.Mismatched, diff)
			}
			// The config and database enforce global uniqueness of names, so we don't
			// expect to see the same named datacenter again. Remove it from the map,
			// which will leave only those datacenters which don't exist in the database
			// when the loop terminates.
			delete(datacenters, diff.Database.Name)
		} else {
			// Datacenter not found in the config.
			differences.Extraneous = append(differences.Extraneous, diff)
		}
	}

	// Datacenters remaining in the map are present in the config but not the database.
	// Iterating over the map would be fast, because it now only contains those datacenters not
	// present in the database, but non-deterministic. Instead iterate deterministically over the
	// array, checking if each datacenter is in the map.
	for _, dc := range datacenterConfigs {
		if _, ok := datacenters[dc.Name]; ok {
			differences.Missing = append(differences.Missing, &DatacenterDiff{
				Config: dc,
			})
		}
	}
	return differences, nil
}

// addDatacenters adds the given datacenters to the database.
func addDatacenters(c context.Context, datacenters []*DatacenterDiff) error {
	// Avoid using the database connection to prepare unnecessary statements.
	if len(datacenters) == 0 {
		return nil
	}

	db := database.Get(c)
	stmt, err := db.PrepareContext(c, "INSERT INTO datacenters (name, description) VALUES (?, ?)")
	if err != nil {
		return errors.Annotate(err, "failed to prepare statement").Err()
	}
	defer stmt.Close()
	for _, dc := range datacenters {
		if _, err := stmt.ExecContext(c, dc.Config.Name, dc.Config.Description); err != nil {
			return errors.Annotate(err, "failed to add datacenter: %s", dc.Config.Name).Err()
		}
		logging.Infof(c, "Added datacenter: %s", dc.Config.Name)
	}
	return nil
}

// updateDatacenters updates the given datacenters in the database.
func updateDatacenters(c context.Context, datacenters []*DatacenterDiff) error {
	// Avoid using the database connection to prepare unnecessary statements.
	if len(datacenters) == 0 {
		return nil
	}

	db := database.Get(c)
	stmt, err := db.PrepareContext(c, "UPDATE datacenters SET description = ? WHERE id = ?")
	if err != nil {
		return errors.Annotate(err, "failed to prepare statement").Err()
	}
	defer stmt.Close()
	for _, dc := range datacenters {
		if _, err := stmt.ExecContext(c, dc.Config.Description, dc.Id); err != nil {
			return errors.Annotate(err, "failed to update datacenter: %s", dc.Config.Name).Err()
		}
		logging.Infof(c, "Updated datacenter: %s", dc.Config.Name)
	}
	return nil
}

// deleteDatacenters deletes the given datacenters from the database.
func deleteDatacenters(c context.Context, datacenters []*DatacenterDiff) error {
	// Avoid using the database connection to prepare unnecessary statements.
	if len(datacenters) == 0 {
		return nil
	}

	db := database.Get(c)
	stmt, err := db.PrepareContext(c, "DELETE FROM datacenters WHERE id = ?")
	if err != nil {
		return errors.Annotate(err, "failed to prepare statement").Err()
	}
	defer stmt.Close()
	for _, dc := range datacenters {
		if _, err := stmt.ExecContext(c, dc.Id); err != nil {
			return errors.Annotate(err, "failed to delete datacenter: %s", dc.Database.Name).Err()
		}
		logging.Infof(c, "Deleted datacenter: %s", dc.Database.Name)
	}
	return nil
}

// EnsureDatacenters ensures the database contains exactly the given datacenters.
func EnsureDatacenters(c context.Context, datacenterConfigs []*config.DatacenterConfig) error {
	differences, err := getDifferences(c, datacenterConfigs)
	if err != nil {
		return errors.Annotate(err, "failed to get datacenters").Err()
	}
	if err = addDatacenters(c, differences.Missing); err != nil {
		return errors.Annotate(err, "failed to add datacenters").Err()
	}
	if err = updateDatacenters(c, differences.Mismatched); err != nil {
		return errors.Annotate(err, "failed to update datacenters").Err()
	}
	if err = deleteDatacenters(c, differences.Extraneous); err != nil {
		return errors.Annotate(err, "failed to delete datacenters").Err()
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
