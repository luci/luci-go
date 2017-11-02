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
	"fmt"

	"go.chromium.org/luci/common/data/stringset"
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
func EnsureDatacenters(c context.Context, datacenters []*config.DatacenterConfig) error {
	db := database.Get(c)

	// TODO(smut): Update existing datacenters, remove datacenters no longer referenced in the config.
	// For now we just delete and re-add datacenters. Since only one cron job calls this, and only
	// every 10 minutes, we don't use a transaction because we don't expect any collisions yet.
	rows, err := db.QueryContext(c, "SELECT `id`, `name` from `datacenters`")
	if err != nil {
		return fmt.Errorf("failed to fetch datacenters: %s", err.Error())
	}
	defer rows.Close()
	// TODO(smut): Delete multiple at once.
	statement, err := db.PrepareContext(c, "DELETE FROM `datacenters` WHERE `id` = ?")
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %s", err.Error())
	}
	defer statement.Close()
	for rows.Next() {
		var id int
		var name string
		err = rows.Scan(&id, &name)
		if err != nil {
			return fmt.Errorf("failed to fetch datacenter: %s", err.Error())
		}
		_, err = statement.ExecContext(c, id)
		logging.Infof(c, "Deleted datacenter: %s", name)
	}

	// TODO(smut): Insert multiple at once.
	statement, err = db.PrepareContext(c, "INSERT INTO `datacenters` (`name`, `description`) VALUES (?, ?)")
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %s", err.Error())
	}
	defer statement.Close()
	for _, datacenter := range datacenters {
		_, err := statement.ExecContext(c, datacenter.Name, datacenter.Description)
		if err != nil {
			return fmt.Errorf("failed to add datacenter: %s", err.Error())
		}
		logging.Infof(c, "Added datacenter: %s", datacenter.Name)
	}
	return nil
}

// DatacentersServer handles datacenter RPCs.
type DatacentersServer struct {
}

// IsAuthorized returns whether the current user is authorized to use the DatacentersServer API.
func (d *DatacentersServer) IsAuthorized(c context.Context) (bool, error) {
	// TODO(smut): Create other groups for this.
	is, err := auth.IsMember(c, "administrators")
	if err != nil {
		logging.Errorf(c, "Failed to check group membership: %s", err.Error())
		return false, err
	}
	return is, err
}

// GetDatacenters handles a request to retrieve datacenters.
func (d *DatacentersServer) GetDatacenters(c context.Context, req *crimson.DatacentersRequest) (*crimson.DatacentersResponse, error) {
	switch authorized, err := d.IsAuthorized(c); {
	case err != nil:
		return nil, grpc.Errorf(codes.Internal, "Failed to check group membership")
	case !authorized:
		return nil, grpc.Errorf(codes.PermissionDenied, "Unauthorized user")
	}
	names := stringset.New(len(req.Names))
	for _, name := range req.Names {
		names.Add(name)
	}
	datacenters, err := getDatacenters(c, names)
	if err != nil {
		return nil, grpc.Errorf(codes.Internal, "Failed to fetch datacenters")
	}
	return &crimson.DatacentersResponse{
		Datacenters: datacenters,
	}, nil
}

// getDatacenters returns a list of datacenters in the database.
func getDatacenters(c context.Context, names stringset.Set) ([]*crimson.Datacenter, error) {
	db := database.Get(c)
	rows, err := db.QueryContext(c, "SELECT `id`, `name`, `description` from `datacenters`")
	if err != nil {
		logging.Errorf(c, "Failed to fetch datacenters: %s", err.Error())
		return nil, err
	}
	defer rows.Close()

	var datacenters []*crimson.Datacenter
	for rows.Next() {
		var id int
		var name, description string
		if err = rows.Scan(&id, &name, &description); err != nil {
			logging.Errorf(c, "Failed to fetch datacenter: %s", err.Error())
			return nil, err
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
