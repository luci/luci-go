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
	controlpb "go.chromium.org/luci/analysis/internal/ingestion/control/proto"
	"go.chromium.org/luci/analysis/internal/tasks/taskspb"
	"go.chromium.org/luci/analysis/internal/testutil"
	analysispb "go.chromium.org/luci/analysis/proto/v1"
	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/server/span"

	. "github.com/smartystreets/goconvey/convey"
)

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
			},
			{
				TestId:      "test_2",
				VariantHash: "hash2",
				Status:      rdbpb.TestVariantStatus_UNEXPECTED,
				Results: []*rdbpb.TestResultBundle{
					{
						Result: &rdbpb.TestResult{
							Name:   "invocations/abc/tests/xyz",
							Status: rdbpb.TestStatus_CRASH,
						},
					},
				},
			},
		}

		err := analyzeSingleBatch(ctx, tvs, payload)
		// As we haven't inserted anything in spanner yet, just check that it does
		// not error out.
		So(err, ShouldBeNil)
	})
}

func countCheckPoint(ctx context.Context) int {
	// Check that there is one checkpoint created.
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
