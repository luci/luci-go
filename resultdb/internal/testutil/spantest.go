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
	durpb "github.com/golang/protobuf/ptypes/duration"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/spantest"

	"go.chromium.org/luci/resultdb/internal/span"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"

	. "github.com/smartystreets/goconvey/convey"
)

// IntegrationTestEnvVar is the name of the environment variable which controls
// whether spanner tests are executed.
// The value must be "1" for integration tests to run.
const IntegrationTestEnvVar = "INTEGRATION_TESTS"

// RunIntegrationTests returns true if integration tests should run.
func RunIntegrationTests() bool {
	return os.Getenv(IntegrationTestEnvVar) == "1"
}

var spannerClient *spanner.Client

// SpannerTestContext returns a context for testing code that talks to Spanner.
// Skips the test if integration tests are not enabled.
//
// Tests that use Spanner must not call t.Parallel().
func SpannerTestContext(tb testing.TB) context.Context {
	switch {
	case !RunIntegrationTests():
		tb.Skipf("env var %s=1 is missing", IntegrationTestEnvVar)
	case spannerClient == nil:
		tb.Fatalf("spanner client is not initialized; forgot to call SpannerTestMain?")
	}

	ctx := TestingContext()
	err := cleanupDatabase(ctx, spannerClient)
	if err != nil {
		tb.Fatal(err)
	}

	return span.WithClient(ctx, spannerClient)
}

// findInitScript returns path //resultdb/internal/span/init_db.sql.
func findInitScript() (string, error) {
	ancestor, err := filepath.Abs(".")
	if err != nil {
		return "", err
	}

	for {
		scriptPath := filepath.Join(ancestor, "internal", "span", "init_db.sql")
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

	if RunIntegrationTests() {
		// Find init_db.sql
		initScriptPath, err := findInitScript()
		if err != nil {
			return 0, err
		}

		// Create a Spanner database.
		ctx := context.Background()
		start := time.Now()
		db, err := spantest.NewTempDB(ctx, spantest.TempDBConfig{InitScriptPath: initScriptPath})
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
		spanner.Delete("InvocationsByTag", spanner.AllKeys()),
		// All other tables are interleaved in Invocations table.
		spanner.Delete("Invocations", spanner.AllKeys()),
	})
	return err
}

// MustApply applies the mutations to the spanner client in the context.
// Asserts that application succeeds.
func MustApply(ctx context.Context, ms ...*spanner.Mutation) {
	_, err := span.Client(ctx).Apply(ctx, ms)
	So(err, ShouldBeNil)
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
func MustReadRow(ctx context.Context, table string, key spanner.Key, ptrMap map[string]interface{}) {
	err := span.ReadRow(ctx, span.Client(ctx).Single(), table, key, ptrMap)
	So(err, ShouldBeNil)
}

func fatalIf(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// InsertInvocation returns a spanner mutation that inserts an invocation.
func InsertInvocation(id span.InvocationID, state pb.Invocation_State, updateToken string, ct time.Time) *spanner.Mutation {
	future := time.Date(2050, 1, 1, 0, 0, 0, 0, time.UTC)
	values := map[string]interface{}{
		"InvocationId":                      id,
		"ShardId":                           0,
		"State":                             state,
		"Realm":                             "",
		"UpdateToken":                       updateToken,
		"InvocationExpirationTime":          future,
		"ExpectedTestResultsExpirationTime": future,
		"CreateTime":                        ct,
		"Deadline":                          ct.Add(time.Hour),
	}
	if pbutil.IsFinalized(state) {
		values["FinalizeTime"] = ct.Add(time.Hour)
	}
	return span.InsertMap("Invocations", values)
}

// InsertInvocationIncl returns mutations to insert an invocation with inclusions.
func InsertInvocationWithInclusions(id span.InvocationID, included ...span.InvocationID) []*spanner.Mutation {
	t := testclock.TestRecentTimeUTC
	ms := []*spanner.Mutation{InsertInvocation(id, pb.Invocation_COMPLETED, "", t)}
	for _, incl := range included {
		ms = append(ms, InsertInclusion(id, incl))
	}
	return ms
}

// InsertInclusion returns a spanner mutation that inserts an inclusion.
func InsertInclusion(including, included span.InvocationID) *spanner.Mutation {
	return span.InsertMap("IncludedInvocations", map[string]interface{}{
		"InvocationId":         including,
		"IncludedInvocationId": included,
	})
}

// InsertTestResults returns spanner mutations to insert test results
func InsertTestResults(trs []*pb.TestResult) []*spanner.Mutation {
	ms := make([]*spanner.Mutation, len(trs))
	for i, tr := range trs {
		invID, testPath, resultID := span.MustParseTestResultName(tr.Name)
		mutMap := map[string]interface{}{
			"InvocationId":    invID,
			"TestPath":        testPath,
			"ResultId":        resultID,
			"Variant":         trs[i].Variant,
			"CommitTimestamp": spanner.CommitTimestamp,
			"Status":          tr.Status,
			"RunDurationUsec": 1e6*i + 234567,
		}
		if !trs[i].Expected {
			mutMap["IsUnexpected"] = true
		}

		ms[i] = span.InsertMap("TestResults", mutMap)
	}
	return ms
}

// MakeTestResults creates test results.
func MakeTestResults(invID, testPath string, statuses ...pb.TestStatus) []*pb.TestResult {
	trs := make([]*pb.TestResult, len(statuses))
	for i, status := range statuses {
		resultID := fmt.Sprintf("%d", i)
		trs[i] = &pb.TestResult{
			Name:     pbutil.TestResultName(string(invID), testPath, resultID),
			TestPath: testPath,
			ResultId: resultID,
			Variant:  pbutil.Variant("k1", "v1", "k2", "v2"),
			Expected: status == pb.TestStatus_PASS,
			Status:   status,
			Duration: &durpb.Duration{Seconds: int64(i), Nanos: 234567000},
		}
	}
	return trs
}
