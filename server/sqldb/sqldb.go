// Copyright 2025 The LUCI Authors.
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

package sqldb

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"net/url"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/server/module"
	"go.chromium.org/luci/server/secrets"

	// Load the driver for the Go port of Sqlite.
	_ "modernc.org/sqlite"
)

// ModuleName can be used to refer to this module when declaring dependencies.
var ModuleName = module.RegisterName("go.chromium.org/luci/server/sqldb")

// ModuleOptions are the options for SQLDB.
type ModuleOptions struct {
	// DBPasswordSecret is a secret-manager managed secret if using the sm:// protocol
	// For local stuff use devsecret://<base64> or devsecret-text://<normal text>
	DBPasswordSecret string
	// DBConnectionURL is the connection string for the DB.
	DBConnectionURL *url.URL
}

// schemeMap maps driver names to database URI connection schemes.
//
// This is needed because the `pgx` driver requires the `postgres` scheme for
// its connection URL. It's possible that other drivers do/will require similar
// mapping.
//
// The alternative to this would be to add another flag to control the driver
// independently, but this would end up with two flags controlling the same thing,
// which would be hard to use / a configuration "gotcha".
var schemeMap = map[string]string{
	"pgx": "postgres",
	"sqlite": "sqlite",
}

// Register registers some flags.
func (o *ModuleOptions) Register(f *flag.FlagSet) {
	f.StringVar(
		&o.DBPasswordSecret,
		"sqldb-password-secret",
		o.DBPasswordSecret,
		`Either sm://<project>/<secret> or devsecret://<base64> or devseceret-text://<value>`,
	)
	f.Func(
		"sqldb-connection-url",
		`database connection string in format driver://user@hostname:port/databasename or the string "sqlite"`,
		func(connectionString string) error {
			if connectionString == "sqlite" {
				connectionString = "sqlite://"
			}
			connectionURL, err := url.Parse(connectionString)
			if err != nil {
				return err
			}
			if _, hasPassword := connectionURL.User.Password(); hasPassword {
				return errors.New("password must be set with -sqldb-password-secret")
			}
			o.DBConnectionURL = connectionURL
			return nil
		},
	)
}

// makeDBURI makes a DB URI. Wow.
//
// Do not make this function public in future refactors. We assume that the value of password was
// retrieved from a secret.
func makeDBURI(newScheme string, dbConnectionURL *url.URL, password string) (string, error) {
	if newScheme == "" {
		return "", errors.New(`newScheme (e.g. "postgres" <--- "pgx")  cannot be empty`)
	}
	if password == "" && newScheme != "sqlite" {
		return "", errors.New("password (retrieved from secret) cannot be empty")
	}
	if _, hasPassword := dbConnectionURL.User.Password(); hasPassword {
		return "", errors.New("password must be specified as a secret")
	}

	dbConnectionURL.Scheme = newScheme
	dbConnectionURL.User = url.UserPassword(dbConnectionURL.User.Username(), password)
	return dbConnectionURL.String(), nil
}

// NewModule makes a new module.
func NewModule(opts *ModuleOptions) module.Module {
	if opts == nil {
		opts = &ModuleOptions{}
	}
	return &sqlDBModule{opts: opts}
}

type sqlDBModule struct {
	opts *ModuleOptions
}

// Name returns the unique name of this module.
func (*sqlDBModule) Name() module.Name {
	return ModuleName
}

// Dependencies returns our dependencies. We always depend on secretmanager because the prod
// path for the module uses it to store the password.
func (*sqlDBModule) Dependencies() []module.Dependency {
	return []module.Dependency{
		module.RequiredDependency(secrets.ModuleName),
	}
}

// Initialize validates our options, creates a new database connection, and puts it in the context for others to use.
func (m *sqlDBModule) Initialize(ctx context.Context, _ module.Host, _ module.HostOptions) (context.Context, error) {
	if m.opts.DBConnectionURL == nil {
		return nil, errors.New(`must specify connection string via -sqldb-connection-url`)
	}
	if _, hasPassword := m.opts.DBConnectionURL.User.Password(); hasPassword {
		return nil, errors.New("password must be specified as a secret, not in the URL")
	}

	driver := m.opts.DBConnectionURL.Scheme
	newScheme, ok := schemeMap[driver]
	if !ok {
		return nil, fmt.Errorf("scheme %q not recognized", newScheme)
	}

	if driver == "sqlite" {
		// Just hardcode memory for now, future versions can do something smarter like sqlite:///path/to/something.db
		dbConn, err := sql.Open(driver, ":memory:")
		if err != nil {
			return nil, errors.Fmt("initializing sqldb module: connecting to database: %w", err)
		}
		return UseDB(ctx, dbConn), nil
	}

	secretStore := secrets.CurrentStore(ctx)
	passwordSecret, err := secretStore.StoredSecret(ctx, m.opts.DBPasswordSecret)
	if err != nil {
		return nil, errors.Fmt("initializing sqldb module: getting secret: %w", err)
	}

	dbURI, err := makeDBURI(newScheme, m.opts.DBConnectionURL, string(passwordSecret.Active))
	if err != nil {
		return nil, errors.Fmt("initializing sqldb module: constructing URI: %w", err)
	}

	dbConn, err := sql.Open(driver, dbURI)
	if err != nil {
		return nil, errors.Fmt("initializing sqldb module: connecting to database: %w", err)
	}

	return UseDB(ctx, dbConn), nil
}

// NewModuleFromFlags is a variant of NewModule that initializes options through
// command line flags.
//
// Calling this function registers flags in flag.CommandLine. They are usually
// parsed in server.Main(...).
func NewModuleFromFlags() module.Module {
	opts := &ModuleOptions{}
	opts.Register(flag.CommandLine)
	return NewModule(opts)
}
