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
	"net/http"
	"sync"

	_ "github.com/go-sql-driver/mysql"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/machine-db/appengine/settings"
	"go.chromium.org/luci/server/router"

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

// dbKey is the key the context value withDatabaseConnection uses to store the db pointer.
var dbKey = "db"

// getDatabaseConnection returns a pointer to the open database connection, creating it if necessary.
func getDatabaseConnection(c context.Context) (*sql.DB, error) {
	// Update the database pointer if the database settings have changed. This operation is costly, but should be rare.
	// Done here to ensure that a connection established with an outdated connection string is closed as soon as possible.
	settings, err := settings.Get(c)
	if err != nil {
		return nil, err
	}
	connectionString := fmt.Sprintf("%s:%s@cloudsql(%s)/%s", settings.Username, settings.Password, settings.Server, settings.Database)

	// If the connection string matches what we expect, the current pointer is correct so just return it.
	dbLock.RLock()
	if connectionString == dbConnectionString {
		defer dbLock.RUnlock()
		return db, nil
	}
	dbLock.RUnlock()

	// The connection string doesn't match so the db pointer needs to be created or updated.
	dbLock.Lock()
	defer dbLock.Unlock()

	// Releasing the read lock may have allowed another concurrent request to grab the write lock first so it's
	// possible we no longer need to do anything. Check again while holding the write lock.
	if connectionString == dbConnectionString {
		return db, nil
	}

	if db != nil {
		if err := db.Close(); err != nil {
			logging.Warningf(c, "Failed to close the previous database connection: %s", err.Error())
		}
	}
	db, err = sql.Open("mysql", connectionString)
	if err != nil {
		return nil, fmt.Errorf("failed to open a new database connection: %s", err.Error())
	}

	// AppEngine limit.
	db.SetMaxOpenConns(12)
	dbConnectionString = connectionString
	return db, nil
}

// Begin begins a transaction using the database pointer embedded in the current context.
func Begin(c context.Context) (*RollbackTx, error) {
	tx, err := Get(c).BeginTx(c, nil)
	return &RollbackTx{Tx: tx}, err
}

// Get returns the database pointer embedded in the current context.
// The database pointer can be embedded in the current context using With.
func Get(c context.Context) *sql.DB {
	return c.Value(&dbKey).(*sql.DB)
}

// With installs a database pointer into the given context.
// It can be retrieved later using Get.
func With(c context.Context, database *sql.DB) context.Context {
	return context.WithValue(c, &dbKey, database)
}

// WithMiddleware is middleware which installs a database pointer into the given context.
// It can be retrieved later in the middleware chain using Get.
func WithMiddleware(c *router.Context, next router.Handler) {
	database, err := getDatabaseConnection(c.Context)
	if err != nil {
		logging.Errorf(c.Context, "Failed to retrieve a database connection: %s", err.Error())
		c.Writer.Header().Set("Content-Type", "text/plain")
		c.Writer.WriteHeader(http.StatusInternalServerError)
		return
	}
	c.Context = With(c.Context, database)
	next(c)
}
