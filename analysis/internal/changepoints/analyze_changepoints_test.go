// Copyright 2023 The LUCI Authors.
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

package changepoints

import (
	"context"
	"fmt"
	"testing"

	"cloud.google.com/go/spanner"
	"go.chromium.org/luci/analysis/internal/changepoints/inputbuffer"
	changepointspb "go.chromium.org/luci/analysis/internal/changepoints/proto"
	controlpb "go.chromium.org/luci/analysis/internal/ingestion/control/proto"
	spanutil "go.chromium.org/luci/analysis/internal/span"
	"go.chromium.org/luci/analysis/internal/tasks/taskspb"
	"go.chromium.org/luci/analysis/internal/testutil"
	analysispb "go.chromium.org/luci/analysis/proto/v1"
	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/server/span"

	. "github.com/smartystreets/goconvey/convey"
)

type Invocation struct {
	Project              string
	InvocationID         string
	IngestedInvocationID string
}

func TestAnalyzeChangePoint(t *testing.T) {
	Convey(`Can batch result`, t, func() {
		ctx := testutil.IntegrationTestContext(t)
		payload := samplePayload(10)
		// 900 test variants should result in 5 batches (1000 each, last one has 500).
		tvs := testVariants(4500)
		err := Analyze(ctx, tvs, payload)
		So(err, ShouldBeNil)

		// Check that there are 5 checkpoints created.
		So(countCheckPoint(ctx), ShouldEqual, 5)
	})

	Convey(`Can skip batch`, t, func() {
		ctx := testutil.IntegrationTestContext(t)
		payload := samplePayload(10)
		tvs := testVariants(100)
		err := analyzeSingleBatch(ctx, tvs, payload)
		So(err, ShouldBeNil)
		So(countCheckPoint(ctx), ShouldEqual, 1)

		// Analyze the batch again should not throw an error.
		err = analyzeSingleBatch(ctx, tvs, payload)
		So(err, ShouldBeNil)
		So(countCheckPoint(ctx), ShouldEqual, 1)
	})

	Convey(`No commit position should skip`, t, func() {
		ctx := testutil.IntegrationTestContext(t)
		// Commit position = 0
		payload := samplePayload(0)
		tvs := testVariants(100)
		err := Analyze(ctx, tvs, payload)
		So(err, ShouldBeNil)
		So(countCheckPoint(ctx), ShouldEqual, 0)
	})

	Convey(`Unsubmitted code should skip`, t, func() {
		ctx := testutil.IntegrationTestContext(t)
		payload := samplePayload(10)
		payload.Build.Changelists = []*analysispb.Changelist{
			{
				Host:     "host",
				Change:   123,
				Patchset: 1,
			},
		}
		payload.PresubmitRun = &controlpb.PresubmitResult{
			Status: analysispb.PresubmitRunStatus_PRESUBMIT_RUN_STATUS_FAILED,
		}
		tvs := testVariants(100)
		err := Analyze(ctx, tvs, payload)
		So(err, ShouldBeNil)
		So(countCheckPoint(ctx), ShouldEqual, 0)
	})

	Convey(`Filter test variant`, t, func() {
		tvs := []*rdbpb.TestVariant{
			{
				TestId: "1",
				Results: []*rdbpb.TestResultBundle{
					{
						Result: &rdbpb.TestResult{
							Name:   "invocations/inv-1/tests/abc",
							Status: rdbpb.TestStatus_SKIP,
						},
					},
				},
			},
			{
				TestId: "2",
				Results: []*rdbpb.TestResultBundle{
					{
						Result: &rdbpb.TestResult{
							Name:   "invocations/inv-2/tests/abc",
							Status: rdbpb.TestStatus_PASS,
						},
					},
					{
						Result: &rdbpb.TestResult{
							Name:   "invocations/inv-2/tests/abc",
							Status: rdbpb.TestStatus_FAIL,
						},
					},
				},
			},
			{
				TestId: "3",
				Results: []*rdbpb.TestResultBundle{
					{
						Result: &rdbpb.TestResult{
							Name:   "invocations/inv-3/tests/abc",
							Status: rdbpb.TestStatus_PASS,
						},
					},
				},
			},
		}
		recycleMap := map[string]bool{
			"inv-2": true,
		}
		tvs, err := filterTestVariants(tvs, recycleMap)
		So(err, ShouldBeNil)
		So(len(tvs), ShouldEqual, 1)
		So(tvs[0].TestId, ShouldEqual, "3")
	})
}

func TestAnalyzeSingleBatch(t *testing.T) {
	Convey(`Analyze batch`, t, func() {
		ctx := testutil.IntegrationTestContext(t)
		payload := samplePayload(10)

		tvs := []*rdbpb.TestVariant{
			{
				TestId:      "test_1",
				VariantHash: "hash1",
				Status:      rdbpb.TestVariantStatus_EXPECTED,
				Results: []*rdbpb.TestResultBundle{
					{
						Result: &rdbpb.TestResult{
							Name:   "invocations/abc/tests/xyz",
							Status: rdbpb.TestStatus_PASS,
						},
					},
				},
			},
			{
				TestId:      "test_2",
				VariantHash: "hash2",
				Status:      rdbpb.TestVariantStatus_UNEXPECTED,
				Results: []*rdbpb.TestResultBundle{
					{
						Result: &rdbpb.TestResult{
							Name:   "invocations/def/tests/xyz",
							Status: rdbpb.TestStatus_CRASH,
						},
					},
				},
			},
		}

		err := analyzeSingleBatch(ctx, tvs, payload)
		So(err, ShouldBeNil)
		So(countCheckPoint(ctx), ShouldEqual, 1)

		// Check invocations
		invs := fetchInvocations(ctx)
		So(invs, ShouldResemble, []Invocation{
			{
				Project:              "chromium",
				InvocationID:         "abc",
				IngestedInvocationID: "build-1234",
			},
			{
				Project:              "chromium",
				InvocationID:         "def",
				IngestedInvocationID: "build-1234",
			},
		})
	})
}

func TestOutOfOrderVerdict(t *testing.T) {
	Convey("Out of order verdict", t, func() {
		payload := samplePayload(10)

		Convey("No test variant branch", func() {
			So(isOutOfOrderAndShouldBeDiscarded(nil, payload), ShouldBeFalse)
		})

		Convey("No finalizing or finalized segment", func() {
			tvb := &TestVariantBranch{}
			So(isOutOfOrderAndShouldBeDiscarded(tvb, payload), ShouldBeFalse)
		})

		Convey("Have finalizing segments", func() {
			tvb := finalizingTvbWithPositions([]int{1}, []int{})
			So(isOutOfOrderAndShouldBeDiscarded(tvb, payload), ShouldBeFalse)
			tvb = finalizingTvbWithPositions([]int{}, []int{1})
			So(isOutOfOrderAndShouldBeDiscarded(tvb, payload), ShouldBeFalse)
			tvb = finalizingTvbWithPositions([]int{8, 13}, []int{7, 9})
			So(isOutOfOrderAndShouldBeDiscarded(tvb, payload), ShouldBeFalse)
			tvb = finalizingTvbWithPositions([]int{11, 15}, []int{6, 8})
			So(isOutOfOrderAndShouldBeDiscarded(tvb, payload), ShouldBeFalse)
			tvb = finalizingTvbWithPositions([]int{11, 15}, []int{10, 16})
			So(isOutOfOrderAndShouldBeDiscarded(tvb, payload), ShouldBeFalse)
			tvb = finalizingTvbWithPositions([]int{11, 15}, []int{12, 16})
			So(isOutOfOrderAndShouldBeDiscarded(tvb, payload), ShouldBeTrue)
		})
	})
}

func countCheckPoint(ctx context.Context) int {
	st := spanner.NewStatement(`
			SELECT *
			FROM TestVariantBranchCheckPoint
		`)
	it := span.Query(span.Single(ctx), st)
	count := 0
	err := it.Do(func(r *spanner.Row) error {
		count++
		return nil
	})
	So(err, ShouldBeNil)
	return count
}

func fetchInvocations(ctx context.Context) []Invocation {
	st := spanner.NewStatement(`
			SELECT Project, InvocationID, IngestedInvocationID
			FROM Invocations
			ORDER BY InvocationID
		`)
	it := span.Query(span.Single(ctx), st)
	results := []Invocation{}
	err := it.Do(func(r *spanner.Row) error {
		var b spanutil.Buffer
		inv := Invocation{}
		err := b.FromSpanner(r, &inv.Project, &inv.InvocationID, &inv.IngestedInvocationID)
		if err != nil {
			return err
		}
		results = append(results, inv)
		return nil
	})
	So(err, ShouldBeNil)
	return results
}

func testVariants(n int) []*rdbpb.TestVariant {
	tvs := make([]*rdbpb.TestVariant, n)
	for i := 0; i < n; i++ {
		tvs[i] = &rdbpb.TestVariant{
			TestId:      fmt.Sprintf("test_%d", i),
			VariantHash: fmt.Sprintf("hash_%d", i),
		}
	}
	return tvs
}

func samplePayload(commitPosition int) *taskspb.IngestTestResults {
	return &taskspb.IngestTestResults{
		Build: &controlpb.BuildResult{
			Id:      1234,
			Project: "chromium",
			Commit: &buildbucketpb.GitilesCommit{
				Host:     "host",
				Project:  "proj",
				Ref:      "ref",
				Position: uint32(commitPosition),
			},
		},
	}
}

func finalizingTvbWithPositions(hotPositions []int, coldPositions []int) *TestVariantBranch {
	tvb := &TestVariantBranch{
		FinalizingSegment: &changepointspb.Segment{},
		InputBuffer:       &inputbuffer.Buffer{},
	}
	for _, pos := range hotPositions {
		tvb.InputBuffer.HotBuffer.Verdicts = append(tvb.InputBuffer.HotBuffer.Verdicts, inputbuffer.PositionVerdict{
			CommitPosition: pos,
		})
	}

	for _, pos := range coldPositions {
		tvb.InputBuffer.ColdBuffer.Verdicts = append(tvb.InputBuffer.ColdBuffer.Verdicts, inputbuffer.PositionVerdict{
			CommitPosition: pos,
		})
	}
	return tvb
}
