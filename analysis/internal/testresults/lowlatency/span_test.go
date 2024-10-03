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

package lowlatency

import (
	"context"
	"testing"
	"time"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/analysis/internal/testresults"
	"go.chromium.org/luci/analysis/internal/testutil"
	"go.chromium.org/luci/analysis/pbutil"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

func TestSaveTestResults(t *testing.T) {
	ftt.Run("SaveUnverified", t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)

		result := NewTestResult().Build()
		_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
			ms := result.SaveUnverified()
			span.BufferWrite(ctx, ms)
			return nil
		})
		assert.Loosely(t, err, should.BeNil)

		results, err := ReadAllForTesting(span.Single(ctx))
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, results, should.Resemble([]*TestResult{result}))
	})
}

func TestReadSourceVerdicts(t *testing.T) {
	ftt.Run("ReadSourceVerdicts", t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)

		variant := pbutil.Variant("key1", "val1", "key2", "val1")

		sourceRefHash := pbutil.SourceRefHash(&pb.SourceRef{
			System: &pb.SourceRef_Gitiles{
				Gitiles: &pb.GitilesRef{
					Host:    "mysources.googlesource.com",
					Project: "myproject/src",
					Ref:     "refs/heads/mybranch",
				},
			},
		})
		partitionTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

		// Returns soures on the queried branch at the given source position.
		onBranchAt := func(position int64, changelists ...testresults.Changelist) testresults.Sources {
			return testresults.Sources{
				RefHash:     sourceRefHash,
				Position:    position,
				Changelists: changelists,
			}
		}

		opts := ReadSourceVerdictsOptions{
			Project:             "testproject",
			TestID:              "test_id",
			VariantHash:         pbutil.VariantHash(variant),
			RefHash:             sourceRefHash,
			AllowedSubrealms:    []string{"realm1", "realm2"},
			StartSourcePosition: 2000,
			EndSourcePosition:   1000,
			StartPartitionTime:  partitionTime,
		}

		t.Run("With no source verdicts", func(t *ftt.Test) {
			svs, err := ReadSourceVerdicts(span.Single(ctx), opts)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, svs, should.HaveLength(0))
		})
		t.Run("With source verdicts", func(t *ftt.Test) {
			baseTestResult := NewTestResult().
				WithProject("testproject").
				WithTestID("test_id").
				WithVariantHash(pbutil.VariantHash(variant)).
				WithPartitionTime(partitionTime.Add(2 * time.Hour)).
				WithSubRealm("realm1").
				WithSources(onBranchAt(1111))

			type testVerdicts struct {
				baseResult TestResultBuilder
				status     pb.QuerySourceVerdictsResponse_VerdictStatus
			}

			verdicts := []testVerdicts{
				{
					// Should not be returned, source position too early.
					baseResult: baseTestResult.WithSources(onBranchAt(1000)).WithRootInvocationID("unexpected-1"),
					status:     pb.QuerySourceVerdictsResponse_EXPECTED,
				},
				{
					// Should be returned, just within source position range.
					baseResult: baseTestResult.WithSources(onBranchAt(1001)).WithRootInvocationID("expected-1"),
					status:     pb.QuerySourceVerdictsResponse_EXPECTED,
				},
				{
					// Should be returned, test multiple test verdict at one position (1/2).
					baseResult: baseTestResult.WithSources(onBranchAt(1200)).WithRootInvocationID("expected-2"),
					status:     pb.QuerySourceVerdictsResponse_UNEXPECTED,
				},
				{
					// Should be returned, test multiple test verdict at one position (2/4).
					baseResult: baseTestResult.WithSources(onBranchAt(1200)).WithRootInvocationID("expected-3"),
					status:     pb.QuerySourceVerdictsResponse_FLAKY,
				},
				{
					// Should be returned, test multiple test verdict at one position (3/4).
					baseResult: baseTestResult.WithSources(onBranchAt(1200)).WithRootInvocationID("expected-4"),
					status:     pb.QuerySourceVerdictsResponse_SKIPPED,
				},
				{
					// Should be returned,  test multiple test verdict at one position (4/4),
					// test verdict with changelists.
					baseResult: baseTestResult.WithSources(onBranchAt(1200, testresults.Changelist{
						Host:      "myproject-review.googlesource.com",
						Change:    1,
						Patchset:  2,
						OwnerKind: pb.ChangelistOwnerKind_AUTOMATION,
					})).WithRootInvocationID("expected-5"),
					status: pb.QuerySourceVerdictsResponse_EXPECTED,
				},
				{
					// Should be returned, just within source position range.
					baseResult: baseTestResult.WithSources(onBranchAt(2000)).WithRootInvocationID("expected-6"),
					status:     pb.QuerySourceVerdictsResponse_EXPECTED,
				},
				{
					// Should not be returned, source position too late.
					baseResult: baseTestResult.WithSources(onBranchAt(2001)).WithRootInvocationID("unexpected-2"),
					status:     pb.QuerySourceVerdictsResponse_EXPECTED,
				},
				{
					// Should be returned, just within partition time range.
					baseResult: baseTestResult.WithPartitionTime(partitionTime).WithRootInvocationID("expected-7"),
					status:     pb.QuerySourceVerdictsResponse_EXPECTED,
				},
				{
					// Should not be returned, just outside partition time range.
					baseResult: baseTestResult.WithPartitionTime(partitionTime.Add(-1 * time.Microsecond)).WithRootInvocationID("unexpected-3"),
					status:     pb.QuerySourceVerdictsResponse_EXPECTED,
				},
				{
					// Should not be returned, wrong project.
					baseResult: baseTestResult.WithProject("wrongproject").WithRootInvocationID("unexpected-4"),
					status:     pb.QuerySourceVerdictsResponse_EXPECTED,
				},
				{
					// Should not be returned, wrong test ID.
					baseResult: baseTestResult.WithTestID("wrong_test_id").WithRootInvocationID("unexpected-5"),
					status:     pb.QuerySourceVerdictsResponse_EXPECTED,
				},
				{
					// Should not be returned, wrong variant hash.
					baseResult: baseTestResult.WithVariantHash("0123456789abcdef").WithRootInvocationID("unexpected-6"),
					status:     pb.QuerySourceVerdictsResponse_EXPECTED,
				},
				{
					// Should not be returned, off branch.
					baseResult: baseTestResult.WithSources(testresults.Sources{
						RefHash:  []byte{1, 2, 3, 4, 5, 6, 7, 8},
						Position: 1555,
					}).WithRootInvocationID("unexpected-7"),
					status: pb.QuerySourceVerdictsResponse_EXPECTED,
				},
				{
					// Should not be returned, dirty sources.
					baseResult: baseTestResult.WithSources(testresults.Sources{
						RefHash:  sourceRefHash,
						Position: 1666,
						IsDirty:  true,
					}).WithRootInvocationID("unexpected-8"),
					status: pb.QuerySourceVerdictsResponse_EXPECTED,
				},
				{
					// Should not be returned, wrong realm.
					baseResult: baseTestResult.WithSubRealm("wrongsubrealm").WithRootInvocationID("unexpected-9"),
					status:     pb.QuerySourceVerdictsResponse_EXPECTED,
				},
			}

			var results []*TestResult
			for _, v := range verdicts {
				results = append(results, verdict(&v.baseResult, v.status)...)
			}
			err := SetForTesting(ctx, t, results)
			assert.Loosely(t, err, should.BeNil)

			svs, err := ReadSourceVerdicts(span.Single(ctx), opts)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, svs, should.Resemble([]SourceVerdict{
				{
					Position: 2000,
					Verdicts: []SourceVerdictTestVerdict{
						{
							RootInvocationID: "expected-6",
							PartitionTime:    partitionTime.Add(2 * time.Hour),
							Status:           pb.QuerySourceVerdictsResponse_EXPECTED,
						},
					},
				},
				{
					Position: 1200,
					Verdicts: []SourceVerdictTestVerdict{
						{
							RootInvocationID: "expected-2",
							PartitionTime:    partitionTime.Add(2 * time.Hour),
							Status:           pb.QuerySourceVerdictsResponse_UNEXPECTED,
						},
						{
							RootInvocationID: "expected-3",
							PartitionTime:    partitionTime.Add(2 * time.Hour),
							Status:           pb.QuerySourceVerdictsResponse_FLAKY,
						},
						{
							RootInvocationID: "expected-4",
							PartitionTime:    partitionTime.Add(2 * time.Hour),
							Status:           pb.QuerySourceVerdictsResponse_SKIPPED,
						},
						{
							RootInvocationID: "expected-5",
							PartitionTime:    partitionTime.Add(2 * time.Hour),
							Status:           pb.QuerySourceVerdictsResponse_EXPECTED,
							Changelists: []testresults.Changelist{
								{
									Host:      "myproject-review.googlesource.com",
									Change:    1,
									Patchset:  2,
									OwnerKind: pb.ChangelistOwnerKind_AUTOMATION,
								},
							},
						},
					},
				},
				{
					Position: 1111,
					Verdicts: []SourceVerdictTestVerdict{
						{
							RootInvocationID: "expected-7",
							PartitionTime:    partitionTime,
							Status:           pb.QuerySourceVerdictsResponse_EXPECTED,
						},
					},
				},
				{
					Position: 1001,
					Verdicts: []SourceVerdictTestVerdict{
						{
							RootInvocationID: "expected-1",
							PartitionTime:    partitionTime.Add(2 * time.Hour),
							Status:           pb.QuerySourceVerdictsResponse_EXPECTED,
						},
					},
				},
			}))
		})
	})
}

func verdict(baseResult *TestResultBuilder, status pb.QuerySourceVerdictsResponse_VerdictStatus) []*TestResult {
	var results []*TestResult

	switch status {
	case pb.QuerySourceVerdictsResponse_EXPECTED:
		tr := baseResult.Build()
		tr.ResultID = "expected"
		tr.IsUnexpected = false
		tr.Status = pb.TestResultStatus_PASS
		results = append(results, tr)

		// For testing purposes, add an unexpectedly skipped result to
		// make sure it is ignored.
		tr = baseResult.Build()
		tr.ResultID = "additional-skip"
		tr.IsUnexpected = true
		tr.Status = pb.TestResultStatus_SKIP
		results = append(results, tr)
	case pb.QuerySourceVerdictsResponse_UNEXPECTED:
		tr := baseResult.Build()
		tr.ResultID = "unexpected"
		tr.IsUnexpected = true
		tr.Status = pb.TestResultStatus_FAIL
		results = append(results, tr)

		// For testing purposes, add an expectedly skipped result to
		// make sure it is ignored.
		tr = baseResult.Build()
		tr.ResultID = "additional-skip"
		tr.IsUnexpected = false
		tr.Status = pb.TestResultStatus_SKIP
		results = append(results, tr)
	case pb.QuerySourceVerdictsResponse_FLAKY:
		// Add both expected and unexpected results.
		tr := baseResult.Build()
		tr.ResultID = "flaky-result-1"
		tr.IsUnexpected = false
		tr.Status = pb.TestResultStatus_PASS
		results = append(results, tr)

		tr = baseResult.Build()
		tr.ResultID = "flaky-result-2"
		tr.IsUnexpected = true
		tr.Status = pb.TestResultStatus_FAIL
		results = append(results, tr)
	case pb.QuerySourceVerdictsResponse_SKIPPED:
		tr := baseResult.Build()
		tr.Status = pb.TestResultStatus_SKIP
		results = append(results, tr)
	}
	return results
}
