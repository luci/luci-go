// Copyright 2019 The LUCI Authors.
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

package testutil

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"cloud.google.com/go/spanner"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/spantest"
	"go.chromium.org/luci/resultdb/internal/storage"

	. "github.com/smartystreets/goconvey/convey"
)

var spannerClient *spanner.Client

// PrepareSpannerClient returns a spanner client for testing.
// It cleans up all tables before returning.
//
// Requires the test binary to use SpannerTestMain.
// If -short flag was passed to the test execution, then skips this test.
func PrepareSpannerClient(ctx context.Context, t *testing.T) *spanner.Client {
	if spannerClient == nil {
		t.Skipf("spanner client is not initialized")

	}
	err := cleanupDatabase(ctx, spannerClient)
	So(err, ShouldBeNil)
	return spannerClient
}

// PrepareContext returns a context for testing.
// It includes a spanner client.
func PrepareContext(t *testing.T) context.Context {
	ctx := context.Background()
	return storage.WithClient(ctx, PrepareSpannerClient(ctx, t))
}

// findInitScript returns path //resultdb/internal/spanner/init_db.sql.
func findInitScript() (string, error) {
	ancestor, err := filepath.Abs(".")
	if err != nil {
		return "", err
	}

	for {
		scriptPath := filepath.Join(ancestor, "internal", "spanner", "init_db.sql")
		_, err := os.Stat(scriptPath)
		if os.IsNotExist(err) {
			parent := filepath.Dir(ancestor)
			if parent == ancestor {
				return "", errors.Reason("init_db.sql not found").Err()
			}
			ancestor = parent
			continue
		}

		return scriptPath, err
	}
}

func fatalIf(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// SpannerTestMain is a test main function for packages that have tests that
// talk to spanner. It creates/destroys a temporary spanner database
// before/after running tests.
//
// This function never returns. Instead it calls os.Exit with the value returned
// by m.Run().
func SpannerTestMain(m *testing.M) {
	ctx := context.Background()
	testing.Init()
	flag.Parse()

	if testing.Short() {
		os.Exit(m.Run())
	}

	// Find init_db.sql
	initScriptPath, err := findInitScript()
	fatalIf(err)

	// Create a Spanner database.
	start := time.Now()
	db, err := spantest.NewTempDB(ctx, spantest.TempDBConfig{
		InitScriptPath: initScriptPath,
	})
	fatalIf(errors.Annotate(err, "failed to create a temporary Spanner database").Err())
	fmt.Printf("created a temporary Spanner database %s in %s\n", db.Name, time.Since(start))
	defer db.Drop(ctx) // this does not run unless m.Run() panics.

	// Create a Spanner client.
	spannerClient, err = db.Client(ctx)
	fatalIf(err)

	// Run tests, drop the db and exit.
	exitCode := m.Run()
	fatalIf(db.Drop(ctx))
	os.Exit(exitCode)
}

// cleanupDatabase deletes all data from all tables.
func cleanupDatabase(ctx context.Context, client *spanner.Client) error {
	_, err := client.Apply(ctx, []*spanner.Mutation{
		// All other tables are interleaved in Invocations table.
		spanner.Delete("Invocations", spanner.AllKeys()),
	})
	return err
}

// MustApply applies the mutations to the spanner client in the context.
// Asserts that application succeeds.
func MustApply(ctx context.Context, ms ...*spanner.Mutation) {
	_, err := storage.Client(ctx).Apply(ctx, ms)
	So(err, ShouldBeNil)
}
