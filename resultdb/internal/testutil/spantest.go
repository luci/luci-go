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
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/spantest"
	"go.chromium.org/luci/server/redisconn"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/spanutil"

	. "github.com/smartystreets/goconvey/convey"
)

const (
	// IntegrationTestEnvVar is the name of the environment variable which controls
	// whether spanner tests are executed.
	// The value must be "1" for integration tests to run.
	IntegrationTestEnvVar = "INTEGRATION_TESTS"

	// RedisTestEnvVar is the name of the environment variable which controls
	// whether tests will attempt to connect to *local* Redis at port 6379.
	// The value must be "1" to connect to Redis.
	//
	// Note that this mode does not support running multiple test binaries in
	// parallel, e.g. `go test ./...`.
	// This could be mitigated by using different Redis databases in different
	// test binaries, but the default limit is only 16.
	RedisTestEnvVar = "INTEGRATION_TESTS_REDIS"

	// EmulatorEnvVar is the name of the environment variable which controls
	// whether to run spanner tests using Cloud Spanner Emulator.
	// The value must be "1" to use emulator.
	EmulatorEnvVar = "SPANNER_EMULATOR"
)

// runIntegrationTests returns true if integration tests should run.
func runIntegrationTests() bool {
	return os.Getenv(IntegrationTestEnvVar) == "1"
}

func runIntegrationTestsWithEmulator() bool {
	return runIntegrationTests() && os.Getenv(EmulatorEnvVar) == "1"
}

// ConnectToRedis returns true if tests should connect to Redis.
func ConnectToRedis() bool {
	return os.Getenv(RedisTestEnvVar) == "1"
}

var spannerClient *spanner.Client

// SpannerTestContext returns a context for testing code that talks to Spanner.
// Skips the test if integration tests are not enabled.
//
// Tests that use Spanner must not call t.Parallel().
func SpannerTestContext(tb testing.TB) context.Context {
	switch {
	case !runIntegrationTests():
		tb.Skipf("env var %s=1 is missing", IntegrationTestEnvVar)
	case spannerClient == nil:
		tb.Fatalf("spanner client is not initialized; forgot to call SpannerTestMain?")
	}

	// Do not mock clock in integration tests because we cannot mock Spanner's
	// clock.
	ctx := testingContext(false)
	err := cleanupDatabase(ctx, spannerClient)
	if err != nil {
		tb.Fatal(err)
	}

	ctx = span.UseClient(ctx, spannerClient)

	if ConnectToRedis() {
		ctx = redisconn.UsePool(ctx, redisconn.NewPool("localhost:6379", 0))
		if err := cleanupRedis(ctx); err != nil {
			tb.Fatal(err)
		}
	}

	return ctx
}

// findInitScript returns path //resultdb/internal/spanutil/init_db.sql.
func findInitScript() (string, error) {
	ancestor, err := filepath.Abs(".")
	if err != nil {
		return "", err
	}

	for {
		scriptPath := filepath.Join(ancestor, "internal", "spanutil", "init_db.sql")
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

// SpannerTestMain is a test main function for packages that have tests that
// talk to spanner. It creates/destroys a temporary spanner database
// before/after running tests.
//
// This function never returns. Instead it calls os.Exit with the value returned
// by m.Run().
func SpannerTestMain(m *testing.M) {
	exitCode, err := spannerTestMain(m)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	os.Exit(exitCode)
}

func spannerTestMain(m *testing.M) (exitCode int, err error) {
	testing.Init()

	if runIntegrationTests() {
		ctx := context.Background()
		start := clock.Now(ctx)
		var instanceName string
		var emulator *spantest.Emulator

		if runIntegrationTestsWithEmulator() {
			var err error
			// Start Cloud Spanner Emulator.
			if emulator, err = spantest.StartEmulator(ctx); err != nil {
				return 0, err
			}
			defer func() {
				switch stopErr := emulator.Stop(); {
				case stopErr == nil:

				case err == nil:
					err = stopErr

				default:
					fmt.Fprintf(os.Stderr, "failed to stop the emulator: %s\n", stopErr)
				}
			}()

			// Create a Spanner instance.
			if instanceName, err = emulator.NewInstance(ctx, ""); err != nil {
				return 0, err
			}
			fmt.Printf("started cloud emulatorlator in and created a temporary Spanner instance %s in %s\n", instanceName, time.Since(start))
			start = clock.Now(ctx)
		}

		// Find init_db.sql
		initScriptPath, err := findInitScript()
		if err != nil {
			return 0, err
		}

		// Create a Spanner database.
		db, err := spantest.NewTempDB(ctx, spantest.TempDBConfig{InitScriptPath: initScriptPath, InstanceName: instanceName}, emulator)
		if err != nil {
			return 0, errors.Annotate(err, "failed to create a temporary Spanner database").Err()
		}
		fmt.Printf("created a temporary Spanner database %s in %s\n", db.Name, time.Since(start))

		defer func() {
			switch dropErr := db.Drop(ctx); {
			case dropErr == nil:

			case err == nil:
				err = dropErr

			default:
				fmt.Fprintf(os.Stderr, "failed to drop the database: %s\n", dropErr)
			}
		}()

		// Create a global Spanner client.
		spannerClient, err = db.Client(ctx)
		if err != nil {
			return 0, err
		}
	}

	return m.Run(), nil
}

// cleanupDatabase deletes all data from all tables.
func cleanupDatabase(ctx context.Context, client *spanner.Client) error {
	_, err := client.Apply(ctx, []*spanner.Mutation{
		// Tables that are not interleaved in Invocations table.
		spanner.Delete("TQReminders", spanner.AllKeys()),
		spanner.Delete("TestMetadata", spanner.AllKeys()),

		// All other tables are interleaved in Invocations table.
		spanner.Delete("Invocations", spanner.AllKeys()),

		spanner.Delete("Baselines", spanner.AllKeys()),
		spanner.Delete("BaselineTestVariants", spanner.AllKeys()),
		spanner.Delete("Checkpoints", spanner.AllKeys()),
	})
	return err
}

// cleanupRedis deletes all data from the selected Redis database.
func cleanupRedis(ctx context.Context) error {
	conn, err := redisconn.Get(ctx)
	if err != nil {
		return err
	}

	_, err = conn.Do("FLUSHDB")
	return err
}

// MustApply applies the mutations to the spanner client in the context.
// Asserts that application succeeds.
// Returns the commit timestamp.
func MustApply(ctx context.Context, ms ...*spanner.Mutation) time.Time {
	ct, err := span.Apply(ctx, ms)
	So(err, ShouldBeNil)
	return ct
}

// CombineMutations concatenates mutations
func CombineMutations(msSlice ...[]*spanner.Mutation) []*spanner.Mutation {
	totalLen := 0
	for _, ms := range msSlice {
		totalLen += len(ms)
	}
	ret := make([]*spanner.Mutation, 0, totalLen)
	for _, ms := range msSlice {
		ret = append(ret, ms...)
	}
	return ret
}

// MustReadRow is a shortcut to do a single row read in a single transaction
// using the current client, and assert success.
func MustReadRow(ctx context.Context, table string, key spanner.Key, ptrMap map[string]any) {
	err := spanutil.ReadRow(span.Single(span.WithoutTxn(ctx)), table, key, ptrMap)
	So(err, ShouldBeNil)
}

// MustNotFindRow is a shortcut to do a single row read in a single transaction
// using the current client, and assert the row was not found.
func MustNotFindRow(ctx context.Context, table string, key spanner.Key, ptrMap map[string]any) {
	err := spanutil.ReadRow(span.Single(span.WithoutTxn(ctx)), table, key, ptrMap)
	So(spanner.ErrCode(err), ShouldEqual, codes.NotFound)
}
