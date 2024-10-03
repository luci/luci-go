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

package testutil

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"cloud.google.com/go/spanner"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/spantest"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/server/span"
)

// cleanupDatabase deletes all data from all tables.
func cleanupDatabase(ctx context.Context, client *spanner.Client) error {
	_, err := client.Apply(ctx, []*spanner.Mutation{
		// No need to explicitly delete interleaved tables.
		spanner.Delete("Checkpoints", spanner.AllKeys()),
		spanner.Delete("ClusteringState", spanner.AllKeys()),
		spanner.Delete("FailureAssociationRules", spanner.AllKeys()),
		spanner.Delete("GerritChangelists", spanner.AllKeys()),
		spanner.Delete("IngestionJoins", spanner.AllKeys()),
		spanner.Delete("ReclusteringRuns", spanner.AllKeys()),
		spanner.Delete("ReclusteringShards", spanner.AllKeys()),
		spanner.Delete("TestResults", spanner.AllKeys()),
		spanner.Delete("TestResultsBySourcePosition", spanner.AllKeys()),
		spanner.Delete("TestVariantRealms", spanner.AllKeys()),
		spanner.Delete("TestRealms", spanner.AllKeys()),
		spanner.Delete("TestVariantBranch", spanner.AllKeys()),
		spanner.Delete("Invocations", spanner.AllKeys()),
	})
	return err
}

// IntegrationTestContext returns a context for testing code that talks to Spanner
// and uses tsmon.
// Skips the test if integration tests are not enabled.
//
// Tests that use Spanner must not call t.Parallel().
func IntegrationTestContext(tb testing.TB) context.Context {
	// tsmon metrics are used fairly extensively throughout LUCI Analysis,
	// especially in contexts that also use Spanner.
	ctx := SpannerTestContext(tb)
	ctx, _ = tsmon.WithDummyInMemory(ctx)
	return ctx
}

// SpannerTestContext returns a context for testing code that talks to Spanner.
// Skips the test if integration tests are not enabled.
//
// Tests that use Spanner must not call t.Parallel().
func SpannerTestContext(tb testing.TB) context.Context {
	return spantest.SpannerTestContext(tb, cleanupDatabase)
}

// findInitScript returns path //analysis/internal/span/init_db.sql.
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
	spantest.SpannerTestMain(m, findInitScript)
}

// MustApply applies the mutations to the spanner client in the context.
// Asserts that application succeeds.
// Returns the commit timestamp.
func MustApply(ctx context.Context, t testing.TB, ms ...*spanner.Mutation) time.Time {
	t.Helper()
	ct, err := span.Apply(ctx, ms)
	assert.Loosely(t, err, should.BeNil, truth.LineContext())
	return ct
}
