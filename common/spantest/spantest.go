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

// Package spantest implements TestMain for running tests that interact with
// Cloud Spanner.
//
// It starts and stops the Cloud Spanner emulator and provides a *spanner.Client
// connected to it. It creates emulated Spanner instances, initialized from DDL
// statements loaded from a SQL file.
//
// Skips all that if `INTEGRATION_TESTS` is not set to 1.
package spantest

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"cloud.google.com/go/spanner"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/common/spantest/emulator"
	"go.chromium.org/luci/server/span"
)

// IntegrationTestEnvVar is the name of the environment variable which controls
// whether spanner tests are executed.
// The value must be "1" for integration tests to run.
const IntegrationTestEnvVar = "INTEGRATION_TESTS"

// spannerClient is connected to a running Cloud Spanner emulator.
//
// Initialized and cleaned up in SpannerTestMain.
var spannerClient *spanner.Client

// CleanupDatabase deletes all data from all tables.
//
// TODO: Can just create a new instance per test: less code, more reliable and
// likely faster in the emulator anyway.
type CleanupDatabase func(ctx context.Context, client *spanner.Client) error

// findInitScript searches ancestor directories for the given init script.
//
// If given path "a/b/init_db.sql", it would probe "./a/b/init_db.sql", then
// "../a/b/init_db.sql", then "../../a/b/init_db.sql" and so on.
func findInitScript(rel string) (string, error) {
	rel = filepath.FromSlash(rel)
	ancestor, err := filepath.Abs(".")
	if err != nil {
		return "", err
	}
	for {
		scriptPath := filepath.Join(ancestor, rel)
		_, err := os.Stat(scriptPath)
		if os.IsNotExist(err) {
			parent := filepath.Dir(ancestor)
			if parent == ancestor {
				return "", errors.Fmt("%s not found", rel)
			}
			ancestor = parent
			continue
		}
		return scriptPath, err
	}
}

// runIntegrationTests returns true if integration tests should run.
func runIntegrationTests() bool {
	return os.Getenv(IntegrationTestEnvVar) == "1"
}

// SpannerTestContext returns a context for testing code that talks to Spanner.
//
// Skips the test if `INTEGRATION_TESTS` env var is not 1.
//
// Tests that use Spanner MUST NOT call t.Parallel().
//
// TODO: Each test can use its own emulated spanner instance, then they can run
// in parallel.
func SpannerTestContext(tb testing.TB, cleanupDatabase CleanupDatabase) context.Context {
	switch {
	case !runIntegrationTests():
		tb.Skipf("env var %s=1 is missing", IntegrationTestEnvVar)
	case spannerClient == nil:
		tb.Fatalf("spanner client is not initialized; forgot to call SpannerTestMain?")
	}

	ctx := context.Background()

	if cleanupDatabase != nil {
		err := cleanupDatabase(ctx, spannerClient)
		if err != nil {
			tb.Fatal(err)
		}
	}

	return span.UseClient(ctx, spannerClient)
}

// SpannerTestMain is a test main function for packages that have tests that
// talk to Cloud Spanner.
//
// If `INTEGRATION_TESTS` env var is set to 1, it launches a new instance of
// Cloud Spanner emulator and creates a temporary spanner database before
// running tests.
//
// The database is initialized with DDL statements from the given init script,
// discovered in one of the ancestor directories. If given an init script path
// "a/b/init_db.sql", it would probe "./a/b/init_db.sql", then
// "../a/b/init_db.sql", then "../../a/b/init_db.sql" and so on.
//
// If `INTEGRATION_TESTS` env var is not set to 1, doesn't do anything special
// and runs tests normally. All tests that need Cloud Spanner (i.e. tests that
// call SpannerTestContext) will be skipped.
//
// This function never returns. Instead it calls os.Exit with the value returned
// by m.Run().
func SpannerTestMain(m *testing.M, initScript string) {
	testing.Init()
	exitCode, err := spannerTestMain(m, initScript)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		exitCode = 1
	}
	os.Exit(exitCode)
}

func spannerTestMain(m *testing.M, initScript string) (exitCode int, err error) {
	if initScript != "" {
		initScript, err = findInitScript(initScript)
		if err != nil {
			return
		}
	}

	if !runIntegrationTests() {
		return m.Run(), nil
	}

	ctx := gologger.StdConfig.Use(context.Background())

	logging.Infof(ctx, "Launching Cloud Spanner emulator")
	emulator, err := emulator.Start(ctx)
	if err != nil {
		return 0, err
	}
	defer emulator.Stop()

	instance, err := NewEmulatedInstance(ctx, emulator)
	if err != nil {
		return 0, err
	}
	logging.Infof(ctx, "Started Cloud Spanner emulator and created temporary instance %s", instance.Name)

	db, err := NewTempDB(ctx, TempDBConfig{
		InitScriptPath:   initScript,
		EmulatedInstance: instance,
	})
	if err != nil {
		return 0, errors.Fmt("failed to create a temporary Spanner database: %w", err)
	}
	logging.Infof(ctx, "Created a temporary spanner database %s", db.Name)

	spannerClient, err = db.Client(ctx)
	if err != nil {
		return 0, err
	}

	return m.Run(), nil
}
