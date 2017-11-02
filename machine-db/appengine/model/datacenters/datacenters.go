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
	"database/sql"
	"fmt"

	_ "github.com/go-sql-driver/mysql"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/machine-db/api/config/v1"
	"go.chromium.org/luci/machine-db/api/crimson/v1"
	"go.chromium.org/luci/machine-db/appengine/database"
	"go.chromium.org/luci/server/auth"

	"golang.org/x/net/context"
)

// EnsureDatacenters ensures the database contains exactly the given datacenters.
func EnsureDatacenters(c context.Context, datacenters []*config.DatacenterConfig) error {
	db := database.GetDatabase(c)

	// TODO(smut): Update existing datacenters, remove datacenters no longer referenced in the config.
	// For now we just delete and re-add datacenters. Since only one cron job calls this, and only
	// every 10 minutes, we don't use a transaction because we don't expect any collisions yet.
	rows, err := db.Query("SELECT `id`, `name` from `datacenters`")
	if err != nil {
		return fmt.Errorf("failed to fetch datacenters: %s", err.Error())
	}
	defer rows.Close()
	// TODO(smut): Delete multiple at once.
	statement, err := db.Prepare("DELETE FROM `datacenters` WHERE `id` = ?")
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
		_, err = statement.Exec(id)
		logging.Infof(c, "Deleted datacenter: %s", name)
	}

	// TODO(smut): Insert multiple at once.
	statement, err = db.Prepare("INSERT INTO `datacenters` (`name`, `description`) VALUES (?, ?)")
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %s", err.Error())
	}
	defer statement.Close()
	for _, datacenter := range datacenters {
		_, err := statement.Exec(datacenter.Name, datacenter.Description)
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

// GetDatacenters handles a request to retrieve datacenters.
func (d *DatacentersServer) GetDatacenters(c context.Context, req *crimson.DatacentersRequest) (*crimson.DatacentersResponse, error) {
	var datacenters []*crimson.Datacenter

	// TODO(smut): Create other groups for this.
	is, err := auth.IsMember(c, "administrators")
	if err != nil {
		logging.Errorf(c, "Failed to check group membership: %s", err.Error())
		return nil, fmt.Errorf("Failed to check group membership")
	}
	if !is {
		return &crimson.DatacentersResponse{}, nil
	}

	// Convert the requested list of names into a map for O(1) access.
	names := make(map[string]struct{})
	for _, name := range req.Names {
		names[name] = struct{}{}
	}

	rows, err := getDatacenters(c)
	if err != nil {
		logging.Errorf(c, "Failed to fetch datacenters: %s", err.Error())
		return nil, fmt.Errorf("Failed to fetch datacenters")
	}
	defer rows.Close()

	for rows.Next() {
		var id int
		var name, description string
		err = rows.Scan(&id, &name, &description)
		if err != nil {
			logging.Errorf(c, "Failed to fetch datacenter: %s", err.Error())
			return nil, fmt.Errorf("Failed to fetch datacenter")
		}
		_, ok := names[name]
		if ok || len(names) == 0 {
			datacenters = append(datacenters, &crimson.Datacenter{
				Name:        name,
				Description: description,
			})
		}
	}

	return &crimson.DatacentersResponse{
		Datacenters: datacenters,
	}, nil
}

// getDatacenters returns a list of datacenter rows in the database.
func getDatacenters(c context.Context) (*sql.Rows, error) {
	db := database.GetDatabase(c)
	return db.Query("SELECT `id`, `name`, `description` from `datacenters`")
}
