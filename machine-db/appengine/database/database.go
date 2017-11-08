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

package database

import (
	"database/sql"
	"fmt"
	"sync"

	_ "github.com/go-sql-driver/mysql"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/machine-db/api/config/v1"
	"go.chromium.org/luci/machine-db/api/crimson/v1"
	"go.chromium.org/luci/server/auth"

	"golang.org/x/net/context"
)

// The open database connection is intended to be reused and shared by multiple concurrent requests,
// therefore we keep a global singleton pointer to it, and functions in this package access it
// under lock. The pointer is lazily initialized when needed, and updated when the config changes
// by acquiring a mutually exclusive lock.

// dbLock is a read/write lock.
// A read lock should be acquired and held as long as the db pointer is being used.
// A write lock should be held to update the db pointer.
var dbLock = &sync.RWMutex{}

// dbConnectionString is the connection string used to open the database connection.
var dbConnectionString = ""

// db is a pointer to the open database connection.
var db *sql.DB

// getDatabaseConnection returns a pointer to the open database connection, creating it if necessary.
// The caller should already hold a read lock on dbLock.
func getDatabaseConnection(c context.Context) (*sql.DB, error) {
	// Update the database pointer if the database settings have changed. This operation is costly, but should be rare.
	// Done here to ensure that a connection established with an outdated connection string is closed as soon as possible.
	settings, err := GetSettings(c)
	if err != nil {
		return nil, err
	}
	connectionString := fmt.Sprintf("%s:%s@cloudsql(%s)/%s", settings.Username, settings.Password, settings.Server, settings.Database)
	if connectionString != dbConnectionString {
		logging.Infof(c, "Found new connection string")
		// Release the read lock in order to acquire the write lock. The read lock is reacquired afterwards.
		dbLock.RUnlock()
		dbLock.Lock()
		// Releasing the read lock may have allowed another concurrent request to grab the write lock first
		// so it's possible we no longer need to update anything. Check again while holding the write lock.
		if connectionString != dbConnectionString {
			logging.Infof(c, "Creating new database connection")
			if db != nil {
				if err := db.Close(); err != nil {
					logging.Errorf(c, "Failed to close the database connection: %s", err.Error())
				}
			}
			db, err = sql.Open("mysql", connectionString)
			if err != nil {
				logging.Errorf(c, "Failed to open a database connection: %s", err.Error())
				// Reacquire the caller's read lock.
				dbLock.Unlock()
				dbLock.RLock()
				return nil, err
			}
			// AppEngine limit.
			db.SetMaxOpenConns(12)
			dbConnectionString = connectionString
		}
		// Reacquire the caller's read lock.
		dbLock.Unlock()
		dbLock.RLock()
	}
	return db, nil
}

// EnsureDatacenters ensures the database contains exactly the given datacenters.
func EnsureDatacenters(c context.Context, datacenters []*config.DatacenterConfig) error {
	if len(datacenters) == 0 {
		return nil
	}

	dbLock.RLock()
	defer dbLock.RUnlock()

	database, err := getDatabaseConnection(c)
	if err != nil {
		return err
	}

	// TODO(smut): Update existing datacenters, remove datacenters no longer referenced in the config.
	// For now we just delete and re-add datacenters. Since only one cron job calls this, and only
	// every 10 minutes, we don't use a transaction because we don't expect any collisions yet.
	rows, err := database.Query("SELECT `id`, `name` from `datacenters`")
	if err != nil {
		logging.Errorf(c, "Failed to fetch datacenters: %s", err.Error())
		return err
	}
	defer rows.Close()
	statement, err := database.Prepare("DELETE FROM `datacenters` WHERE `id` = ?")
	if err != nil {
		logging.Errorf(c, "Failed to prepare statement: %s", err.Error())
		return err
	}
	defer statement.Close()
	for rows.Next() {
		var id int
		var name string
		err = rows.Scan(&id, &name)
		if err != nil {
			logging.Errorf(c, "Failed to fetch datacenter: %s", err.Error())
			return err
		}
		_, err = statement.Exec(id)
		logging.Infof(c, "Deleted datacenter: %s", name)
	}
	statement.Close()

	statement, err = database.Prepare("INSERT INTO `datacenters` (`name`, `description`) VALUES (?, ?)")
	if err != nil {
		logging.Errorf(c, "Failed to prepare statement: %s", err.Error())
		return err
	}
	defer statement.Close()
	for _, datacenter := range datacenters {
		_, err := statement.Exec(datacenter.Name, datacenter.Description)
		if err != nil {
			logging.Errorf(c, "Failed to add datacenter: %s", err.Error())
			return err
		}
		logging.Infof(c, "Added datacenter: %s", datacenter.Name)
	}
	return nil
}

// DatacentersServer handles datacenter RPCs.
type DatacentersServer struct {
}

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
	dbLock.RLock()
	defer dbLock.RUnlock()
	database, err := getDatabaseConnection(c)
	if err != nil {
		return nil, err
	}
	return database.Query("SELECT `id`, `name`, `description` from `datacenters`")
}
