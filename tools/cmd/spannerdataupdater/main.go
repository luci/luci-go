// Copyright 2022 The LUCI Authors.
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

// Provides a method for executing long-running Partitioned DML statements,
// as needed to perform data backfills after schema changes to Spanner tables.
//
// Provided because the gcloud command:
//
// gcloud spanner databases execute-sql --enable-partitioned-dml --sql="<SQL>"
//
// does not support long running statements (beyond ~40 mins or so), regardless
// of the timeout paramater specified.
//
// Example usage:
//
//	go run main.go \
//	 -sql='UPDATE TestResults SET <Something> WHERE <Something Else>' \
//	 -project=chops-weetbix-dev -instance=dev -database=chops-weetbix-dev
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"cloud.google.com/go/spanner"
	"google.golang.org/api/option"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/auth/scopes"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/hardcoded/chromeinfra"
)

var errCanceledByUser = errors.New("operation canceled by user")

type flags struct {
	sql      string
	project  string
	instance string
	database string
}

func parseFlags() (*flags, error) {
	var f flags
	flag.StringVar(&f.sql, "sql", "", "SQL Statement to run. This must satisfy the requirements of a parititoned DML statement: https://cloud.google.com/spanner/docs/dml-partitioned")
	flag.StringVar(&f.project, "project", "", "GCP Project, e.g. luci-resultdb or chops-weetbix.")
	flag.StringVar(&f.instance, "instance", "", "The Spanner instance name, e.g. dev or prod.")
	flag.StringVar(&f.database, "database", "", "The Spanner database name.")
	flag.Parse()

	switch {
	case len(flag.Args()) > 0:
		return nil, fmt.Errorf("unexpected arguments: %q", flag.Args())
	case f.sql == "":
		return nil, fmt.Errorf("-sql is required")
	case f.project == "":
		return nil, fmt.Errorf("-project is required")
	case f.instance == "":
		return nil, fmt.Errorf("-instance is required")
	case f.database == "":
		return nil, fmt.Errorf("-database is required")
	}
	return &f, nil
}

func run(ctx context.Context) error {
	flags, err := parseFlags()
	if err != nil {
		return errors.Fmt("failed to parse flags: %w", err)
	}

	// Create an Authenticator and use it for Spanner operations.
	authOpts := chromeinfra.DefaultAuthOptions()
	authOpts.Scopes = scopes.CloudScopeSet()
	authenticator := auth.NewAuthenticator(ctx, auth.InteractiveLogin, authOpts)

	authTS, err := authenticator.TokenSource()
	if err != nil {
		return errors.Fmt("could not get authentication credentials: %w", err)
	}

	database := fmt.Sprintf("projects/%s/instances/%s/databases/%s", flags.project, flags.instance, flags.database)
	c, err := spanner.NewClient(ctx, database, option.WithTokenSource(authTS))
	if err != nil {
		return errors.Fmt("could not create Spanner client: %w", err)
	}
	fmt.Println("The following partitioned DML will be applied.")
	fmt.Println(strings.Repeat("=", 80))
	fmt.Println("GCP Project:   ", flags.project)
	fmt.Println("Instance:      ", flags.instance)
	fmt.Println("Database:      ", flags.database)
	fmt.Println("SQL Statement: ", flags.sql)
	fmt.Println(strings.Repeat("=", 80))

	confirm := confirm("Continue")
	if !confirm {
		return errCanceledByUser
	}

	// Apply one-week timeout.
	ctx, cancel := context.WithTimeout(ctx, time.Hour*7*24)
	defer cancel()
	count, err := c.PartitionedUpdate(ctx, spanner.NewStatement(flags.sql))
	if err != nil {
		return errors.Fmt("applying partitioned update: %w", err)
	}
	log.Printf("%v rows updated", count)
	return nil
}

func main() {
	ctx := gologger.StdConfig.Use(context.Background())
	switch err := run(ctx); {
	case errCanceledByUser == err:
		os.Exit(1)
	case err != nil:
		log.Fatal(err)
	}
}

// confirm asks for a user confirmation for an action, with No as default.
// Only "y" or "Y" responses is treated as yes.
func confirm(action string) (response bool) {
	fmt.Printf("%s? [y/N] ", action)
	var res string
	fmt.Scanln(&res)
	return res == "y" || res == "Y"
}
