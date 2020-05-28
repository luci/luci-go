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
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	durpb "github.com/golang/protobuf/ptypes/duration"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/spantest"

	"go.chromium.org/luci/resultdb/internal/span"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
	typepb "go.chromium.org/luci/resultdb/proto/type"

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

	// Do not mock clock in integration tests because we cannot mock Spanner's
	// clock.
	ctx := testingContext(false)
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
		spanner.Delete("InvocationTasks", spanner.AllKeys()),
		// All other tables are interleaved in Invocations table.
		spanner.Delete("Invocations", spanner.AllKeys()),
	})
	return err
}

// MustApply applies the mutations to the spanner client in the context.
// Asserts that application succeeds.
// Returns the commit timestamp.
func MustApply(ctx context.Context, ms ...*spanner.Mutation) time.Time {
	ct, err := span.Client(ctx).Apply(ctx, ms)
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
func MustReadRow(ctx context.Context, table string, key spanner.Key, ptrMap map[string]interface{}) {
	err := span.ReadRow(ctx, span.Client(ctx).Single(), table, key, ptrMap)
	So(err, ShouldBeNil)
}

// MustNotFindRow is a shortcut to do a single row read in a single transaction
// using the current client, and assert the row was not found.
func MustNotFindRow(ctx context.Context, table string, key spanner.Key, ptrMap map[string]interface{}) {
	err := span.ReadRow(ctx, span.Client(ctx).Single(), table, key, ptrMap)
	So(spanner.ErrCode(err), ShouldEqual, codes.NotFound)
}

func fatalIf(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func updateDict(dest, source map[string]interface{}) {
	for k, v := range source {
		dest[k] = v
	}
}

// InsertInvocation returns a spanner mutation that inserts an invocation.
func InsertInvocation(id span.InvocationID, state pb.Invocation_State, extraValues map[string]interface{}) *spanner.Mutation {
	future := time.Date(2050, 1, 1, 0, 0, 0, 0, time.UTC)
	values := map[string]interface{}{
		"InvocationId":                      id,
		"ShardId":                           0,
		"State":                             state,
		"Realm":                             "",
		"InvocationExpirationTime":          future,
		"ExpectedTestResultsExpirationTime": future,
		"CreateTime":                        spanner.CommitTimestamp,
		"Deadline":                          future,
		"TestResultCount":                   0,
	}

	if state == pb.Invocation_FINALIZED {
		values["FinalizeTime"] = spanner.CommitTimestamp
	}
	updateDict(values, extraValues)
	return span.InsertMap("Invocations", values)
}

// InsertFinalizedInvocationWithInclusions returns mutations to insert a finalized invocation with inclusions.
func InsertFinalizedInvocationWithInclusions(id span.InvocationID, included ...span.InvocationID) []*spanner.Mutation {
	return InsertInvocationWithInclusions(id, pb.Invocation_FINALIZED, included...)
}

// InsertInvocationWithInclusions returns mutations to insert an invocation with inclusions.
func InsertInvocationWithInclusions(id span.InvocationID, state pb.Invocation_State, included ...span.InvocationID) []*spanner.Mutation {
	ms := []*spanner.Mutation{InsertInvocation(id, state, nil)}
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
func InsertTestResults(invID, testID string, v *typepb.Variant, statuses ...pb.TestStatus) []*spanner.Mutation {
	return InsertTestResultMessages(MakeTestResults(invID, testID, v, statuses...))
}

// InsertTestResultMessages returns spanner mutations to insert test results
func InsertTestResultMessages(trs []*pb.TestResult) []*spanner.Mutation {
	ms := make([]*spanner.Mutation, len(trs))
	for i, tr := range trs {
		invID, testID, resultID := span.MustParseTestResultName(tr.Name)
		mutMap := map[string]interface{}{
			"InvocationId":    invID,
			"TestId":          testID,
			"ResultId":        resultID,
			"Variant":         trs[i].Variant,
			"VariantHash":     pbutil.VariantHash(trs[i].Variant),
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

// InsertTestExonerations returns spanner mutations to insert test exonerations.
func InsertTestExonerations(invID span.InvocationID, testID string, variant *typepb.Variant, count int) []*spanner.Mutation {
	ms := make([]*spanner.Mutation, count)
	for i := 0; i < count; i++ {
		ms[i] = span.InsertMap("TestExonerations", map[string]interface{}{
			"InvocationId":    invID,
			"TestId":          testID,
			"ExonerationId":   strconv.Itoa(i),
			"Variant":         variant,
			"VariantHash":     pbutil.VariantHash(variant),
			"ExplanationHTML": span.Compressed(fmt.Sprintf("explanation %d", i)),
		})
	}
	return ms
}

// InsertInvocationArtifact returns a spanner mutation to insert an invocation
// artifact.
func InsertInvocationArtifact(invID span.InvocationID, artifactID string, extraValues map[string]interface{}) *spanner.Mutation {
	values := map[string]interface{}{
		"InvocationId": invID,
		"ParentID":     "",
		"ArtifactId":   artifactID,
	}
	updateDict(values, extraValues)
	return span.InsertMap("Artifacts", values)
}

// InsertTestResultArtifact returns a spanner mutation to insert a test result
// artifact.
func InsertTestResultArtifact(invID span.InvocationID, testID, resultID, artifactID string, extraValues map[string]interface{}) *spanner.Mutation {
	values := map[string]interface{}{
		"InvocationId": invID,
		"ParentID":     span.ArtifactParentID(testID, resultID),
		"ArtifactId":   artifactID,
	}
	updateDict(values, extraValues)
	return span.InsertMap("Artifacts", values)
}

// MakeTestResults creates test results.
func MakeTestResults(invID, testID string, v *typepb.Variant, statuses ...pb.TestStatus) []*pb.TestResult {
	trs := make([]*pb.TestResult, len(statuses))
	for i, status := range statuses {
		resultID := fmt.Sprintf("%d", i)
		trs[i] = &pb.TestResult{
			Name:     pbutil.TestResultName(invID, testID, resultID),
			TestId:   testID,
			ResultId: resultID,
			Variant:  v,
			Expected: status == pb.TestStatus_PASS,
			Status:   status,
			Duration: &durpb.Duration{Seconds: int64(i), Nanos: 234567000},
		}
	}
	return trs
}

// LogQueryResults executes the statement and logs results.
// Useful to debug failing tests.
func LogQueryResults(ctx context.Context, st spanner.Statement) {
	var values []interface{}
	buf := &bytes.Buffer{}
	err := span.Client(ctx).Single().Query(ctx, st).Do(func(row *spanner.Row) error {
		if len(values) < row.Size() {
			values = make([]interface{}, row.Size())
			for i := range values {
				values[i] = &spanner.GenericColumnValue{}
			}
		}
		if err := row.Columns(values...); err != nil {
			return err
		}

		buf.Reset()
		for i, v := range values {
			fmt.Fprintf(buf, "\n  %s = %s", row.ColumnName(i), v.(*spanner.GenericColumnValue).Value)
		}

		logging.Infof(ctx, "row: %s", buf)
		return nil
	})
	So(err, ShouldBeNil)
}
