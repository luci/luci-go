// Copyright 2024 The LUCI Authors.
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

package spantest

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"cloud.google.com/go/spanner"
	spandb "cloud.google.com/go/spanner/admin/database/apiv1"
	dbpb "cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	"google.golang.org/api/option"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/hardcoded/chromeinfra"
)

// TempDBConfig specifies how to create a temporary database.
type TempDBConfig struct {
	// EmulatedInstance, if set, indicates to use the given emulated instance to
	// create the DB in.
	//
	// Either EmulatedInstance or CloudInstance must be set.
	EmulatedInstance *EmulatedInstance

	// CloudInstance, if set, indicates to use **real** Cloud Spanner instance.
	//
	// Format: projects/{project}/instances/{instance}.
	//
	// Either EmulatedInstance or CloudInstance must be set.
	CloudInstance string

	// InitScriptPath is a path to a DDL script to initialize the database.
	//
	// In lieu of a proper DDL parser, it is parsed using regexes.
	// Therefore the script MUST:
	//   - Use `#`` and/or `--`` for comments. No block comments.
	//   - Separate DDL statements with `;\n`.
	//
	// If empty, the database is created with no tables.
	InitScriptPath string
}

var ddlStatementSepRe = regexp.MustCompile(`;\s*\n`)
var commentRe = regexp.MustCompile(`(--|#)[^\n]*`)

// readDDLStatements read the file at cfg.InitScriptPath as a sequence of DDL
// statements. If the path is empty, returns (nil, nil).
func (cfg *TempDBConfig) readDDLStatements() ([]string, error) {
	if cfg.InitScriptPath == "" {
		return nil, nil
	}

	contents, err := os.ReadFile(cfg.InitScriptPath)
	if err != nil {
		return nil, err
	}

	statements := ddlStatementSepRe.Split(string(contents), -1)
	ret := statements[:0]
	for _, stmt := range statements {
		stmt = commentRe.ReplaceAllString(stmt, "")
		stmt = strings.TrimSpace(stmt)
		if stmt != "" {
			ret = append(ret, stmt)
		}
	}
	return ret, nil
}

// TempDB is a temporary Spanner database.
type TempDB struct {
	// Name is the full database name to pass to the spanner client.
	Name string

	opts []option.ClientOption
}

// Client returns a new spanner client connected to the database.
func (db *TempDB) Client(ctx context.Context) (*spanner.Client, error) {
	return spanner.NewClient(ctx, db.Name, db.opts...)
}

// Drop deletes the database.
func (db *TempDB) Drop(ctx context.Context) error {
	client, err := spandb.NewDatabaseAdminClient(ctx, db.opts...)
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()

	return client.DropDatabase(ctx, &dbpb.DropDatabaseRequest{
		Database: db.Name,
	})
}

// NewTempDB creates a temporary database with a random name.
//
// Creates it either in a Cloud Spanner emulator (if cfg.EmulatedInstance is
// set) or in **a real Cloud Spanner instance** (if cfg.CloudInstance is set).
//
// If this is a Cloud Spanner database, the caller is responsible for calling
// Drop on the returned TempDB to delete it after usage. Emulated databases
// are implicitly dropped when the emulator exits.
func NewTempDB(ctx context.Context, cfg TempDBConfig) (*TempDB, error) {
	var instanceName string
	var opts []option.ClientOption
	var err error

	switch {
	case cfg.EmulatedInstance != nil:
		instanceName = cfg.EmulatedInstance.Name
		opts = cfg.EmulatedInstance.Emulator.ClientOptions()
	case cfg.CloudInstance != "":
		instanceName = cfg.CloudInstance
		opts, err = prodClientOptions(ctx)
		if err != nil {
			return nil, errors.Fmt("failed to initialize production spanner client: %w", err)
		}
	default:
		return nil, errors.New("either EmulatedInstance or CloudInstance must be set")
	}

	initStatements, err := cfg.readDDLStatements()
	if err != nil {
		return nil, errors.Fmt("failed to read %q: %w", cfg.InitScriptPath, err)
	}

	client, err := spandb.NewDatabaseAdminClient(ctx, opts...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = client.Close() }()

	// Generate a random database name.
	var random uint32
	if err := binary.Read(rand.Reader, binary.LittleEndian, &random); err != nil {
		panic(err)
	}
	dbName := fmt.Sprintf("tmp%s-%d", time.Now().Format("20060102-"), random)
	dbName = sanitizeDBName(dbName)

	dbOp, err := client.CreateDatabase(ctx, &dbpb.CreateDatabaseRequest{
		Parent:          instanceName,
		CreateStatement: "CREATE DATABASE " + dbName,
		ExtraStatements: initStatements,
	})
	if err != nil {
		return nil, errors.Fmt("failed to create database: %w", err)
	}
	db, err := dbOp.Wait(ctx)
	if err != nil {
		return nil, errors.Fmt("failed to create database: %w", err)
	}

	return &TempDB{
		Name: db.Name,
		opts: opts,
	}, nil
}

func prodClientOptions(ctx context.Context) ([]option.ClientOption, error) {
	opts := chromeinfra.SetDefaultAuthOptions(auth.Options{
		Scopes: []string{
			"https://www.googleapis.com/auth/cloud-platform",
			"https://www.googleapis.com/auth/userinfo.email",
		},
	})
	ts, err := auth.NewAuthenticator(ctx, auth.SilentLogin, opts).TokenSource()
	if errors.Is(err, auth.ErrLoginRequired) {
		return nil, errors.Fmt("please login with `luci-auth login -scopes %q`: %w", strings.Join(opts.Scopes, " "), err)
	}
	return []option.ClientOption{
		option.WithTokenSource(ts),
	}, nil
}

var dbNameAlphabetInversedRe = regexp.MustCompile(`[^\w]+`)

// sanitizeDBName transforms name to a valid one.
//
// If name is already valid, returns it without changes.
func sanitizeDBName(name string) string {
	name = strings.ToLower(name)
	name = dbNameAlphabetInversedRe.ReplaceAllLiteralString(name, "_")
	const maxLen = 30
	if len(name) > maxLen {
		name = name[:maxLen]
	}
	name = strings.TrimRight(name, "_")
	return name
}
