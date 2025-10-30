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

package testresults

import (
	"context"
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/analysis/internal/testutil"
	"go.chromium.org/luci/analysis/pbutil"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

func TestReadPassingResults(t *testing.T) {
	ftt.Run("ReadPassingResults", t, func(t *ftt.Test) {
		ctx := testutil.IntegrationTestContext(t)

		// Create test data for both source-position and chronological tests.
		err := createPassingResultsTestData(ctx)
		assert.Loosely(t, err, should.BeNil)

		// Get a read-only transaction for all sub-tests to use.
		ctx, cancel := span.ReadOnlyTransaction(ctx)
		defer cancel()

		t.Run("BySource", func(t *ftt.Test) {
			t.Run("Gitiles - Found", func(t *ftt.Test) {
				opts := ReadPassingRootInvocationsBySourceOptions{
					Project:           "testproject",
					SubRealms:         []string{"testrealm-a", "testrealm-b"},
					TestID:            "ninja://test/id",
					VariantHash:       "e5aa3a34a834a74f",
					SourceRefHash:     gitilesRefHash,
					MaxSourcePosition: 105,
					Limit:             10,
				}
				results, err := ReadPassingResultsBySource(ctx, opts)
				assert.Loosely(t, err, should.BeNil)
				// Should return inv-3 (pos 102) and inv-2 (pos 101) in that order.
				// inv-4 is filtered out because its position (106) is > MaxSourcePosition.
				assert.Loosely(t, results, should.Resemble([]string{
					"invocations/inv-3/tests/ninja:%2F%2Ftest%2Fid/results/result-0",
					"invocations/inv-2/tests/ninja:%2F%2Ftest%2Fid/results/result-0",
					"invocations/inv-1/tests/ninja:%2F%2Ftest%2Fid/results/result-0",
				}))
			})
			t.Run("Android - Found", func(t *ftt.Test) {
				opts := ReadPassingRootInvocationsBySourceOptions{
					Project:           "testproject",
					SubRealms:         []string{"testrealm-a", "testrealm-b"},
					TestID:            "ninja://test/id",
					VariantHash:       "e5aa3a34a834a74f",
					SourceRefHash:     androidRefHash,
					MaxSourcePosition: 2002,
					Limit:             10,
				}
				results, err := ReadPassingResultsBySource(ctx, opts)
				assert.Loosely(t, err, should.BeNil)
				// Should return inv-android-2 (pos 2002) and inv-android-1 (pos 2001) in order.
				assert.Loosely(t, results, should.Resemble([]string{
					"invocations/inv-android-2/tests/ninja:%2F%2Ftest%2Fid/results/result-0",
					"invocations/inv-android-1/tests/ninja:%2F%2Ftest%2Fid/results/result-0",
				}))
			})
			t.Run("Limit", func(t *ftt.Test) {
				opts := ReadPassingRootInvocationsBySourceOptions{
					Project:           "testproject",
					SubRealms:         []string{"testrealm-a", "testrealm-b"},
					TestID:            "ninja://test/id",
					VariantHash:       "e5aa3a34a834a74f",
					SourceRefHash:     gitilesRefHash,
					MaxSourcePosition: 105,
					Limit:             1,
				}
				results, err := ReadPassingResultsBySource(ctx, opts)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, results, should.Resemble([]string{"invocations/inv-3/tests/ninja:%2F%2Ftest%2Fid/results/result-0"}))
			})
			t.Run("Realm filter", func(t *ftt.Test) {
				opts := ReadPassingRootInvocationsBySourceOptions{
					Project:           "testproject",
					SubRealms:         []string{"testrealm-b"}, // only this realm
					TestID:            "ninja://test/id",
					VariantHash:       "e5aa3a34a834a74f",
					SourceRefHash:     gitilesRefHash,
					MaxSourcePosition: 105,
					Limit:             10,
				}
				results, err := ReadPassingResultsBySource(ctx, opts)
				assert.Loosely(t, err, should.BeNil)
				// Only inv-3 is in testrealm-b.
				assert.Loosely(t, results, should.Resemble([]string{"invocations/inv-3/tests/ninja:%2F%2Ftest%2Fid/results/result-0"}))
			})
			t.Run("No results", func(t *ftt.Test) {
				opts := ReadPassingRootInvocationsBySourceOptions{
					Project:           "testproject",
					SubRealms:         []string{"testrealm-a", "testrealm-b"},
					TestID:            "ninja://test/nonexistent",
					VariantHash:       "e5aa3a34a834a74f",
					SourceRefHash:     gitilesRefHash,
					MaxSourcePosition: 105,
					Limit:             10,
				}
				results, err := ReadPassingResultsBySource(ctx, opts)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, results, should.BeEmpty)
			})
		})
	})
}

var (
	gitilesRef = &pb.SourceRef{
		System: &pb.SourceRef_Gitiles{
			Gitiles: &pb.GitilesRef{Host: "my-g-host", Project: "my-g-proj", Ref: "refs/heads/main"},
		},
	}
	gitilesRefHash = pbutil.SourceRefHash(gitilesRef)

	androidRef = &pb.SourceRef{
		System: &pb.SourceRef_AndroidBuild{
			AndroidBuild: &pb.AndroidBuildBranch{DataRealm: "prod", Branch: "git_main"},
		},
	}
	androidRefHash = pbutil.SourceRefHash(androidRef)
)

func createPassingResultsTestData(ctx context.Context) error {
	// Common values.
	project := "testproject"
	testID := "ninja://test/id"
	variantHash := "e5aa3a34a834a74f"

	// Create rows for TestResultsBySourcePosition.
	var sourcePosMutations []*spanner.Mutation
	// Gitiles passes
	sourcePosMutations = append(sourcePosMutations, sourcePosTestResult(project, "testrealm-a", testID, variantHash, gitilesRefHash, 101, "inv-2", time.Unix(200, 0), true))
	sourcePosMutations = append(sourcePosMutations, sourcePosTestResult(project, "testrealm-b", testID, variantHash, gitilesRefHash, 102, "inv-3", time.Unix(300, 0), true))
	sourcePosMutations = append(sourcePosMutations, sourcePosTestResult(project, "testrealm-a", testID, variantHash, gitilesRefHash, 106, "inv-4", time.Unix(400, 0), true))
	// A fail to ensure we filter by status.
	sourcePosMutations = append(sourcePosMutations, sourcePosTestResult(project, "testrealm-a", testID, variantHash, gitilesRefHash, 103, "inv-fail", time.Unix(350, 0), false))
	// A pass in an unauthorized realm.
	sourcePosMutations = append(sourcePosMutations, sourcePosTestResult(project, "unauthorized-realm", testID, variantHash, gitilesRefHash, 104, "inv-unauth", time.Unix(360, 0), true))
	// An old pass that should be filtered by time.
	sourcePosMutations = append(sourcePosMutations, sourcePosTestResult(project, "testrealm-a", testID, variantHash, gitilesRefHash, 100, "inv-1", time.Unix(1, 0), true))

	// Android passes
	sourcePosMutations = append(sourcePosMutations, sourcePosTestResult(project, "testrealm-a", testID, variantHash, androidRefHash, 2001, "inv-android-1", time.Unix(500, 0), true))
	sourcePosMutations = append(sourcePosMutations, sourcePosTestResult(project, "testrealm-a", testID, variantHash, androidRefHash, 2002, "inv-android-2", time.Unix(600, 0), true))

	_, err := span.Apply(ctx, sourcePosMutations)
	return err
}

func sourcePosTestResult(project, realm, testID, variantHash string, sourceRefHash []byte, pos int64, invID string, partitionTime time.Time, passed bool) *spanner.Mutation {
	status := rdbpb.TestStatus_FAIL
	if passed {
		status = rdbpb.TestStatus_PASS
	}
	return spanner.Insert("TestResultsBySourcePosition",
		[]string{"Project", "SubRealm", "TestId", "VariantHash", "SourceRefHash", "SourcePosition", "RootInvocationId", "InvocationId", "ResultId", "PartitionTime", "Status", "IsUnexpected", "ChangelistHosts", "ChangelistChanges", "ChangelistPatchsets", "ChangelistOwnerKinds", "HasDirtySources"},
		[]interface{}{project, realm, testID, variantHash, sourceRefHash, pos, invID, invID, "result-0", partitionTime, int64(status), false, []string{}, []int64{}, []int64{}, []string{}, false})
}
