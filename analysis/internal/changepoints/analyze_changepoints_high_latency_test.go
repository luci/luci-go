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
	"sort"
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/analysis/internal/changepoints/bqexporter"
	"go.chromium.org/luci/analysis/internal/changepoints/inputbuffer"
	"go.chromium.org/luci/analysis/internal/changepoints/model"
	cpb "go.chromium.org/luci/analysis/internal/changepoints/proto"
	tu "go.chromium.org/luci/analysis/internal/changepoints/testutil"
	"go.chromium.org/luci/analysis/internal/changepoints/testvariantbranch"
	"go.chromium.org/luci/analysis/internal/config"
	controlpb "go.chromium.org/luci/analysis/internal/ingestion/control/proto"
	spanutil "go.chromium.org/luci/analysis/internal/span"
	"go.chromium.org/luci/analysis/internal/tasks/taskspb"
	"go.chromium.org/luci/analysis/internal/testutil"
	"go.chromium.org/luci/analysis/pbutil"
	bqpb "go.chromium.org/luci/analysis/proto/bq"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

type Invocation struct {
	Project              string
	InvocationID         string
	IngestedInvocationID string
}

func TestAnalyzeChangePoint(t *testing.T) {
	exporter, _ := fakeExporter()
	ftt.Run(`Can batch result`, t, func(t *ftt.Test) {
		ctx := newContext(t)
		payload := tu.SamplePayload()
		sourcesMap := tu.SampleSourcesWithChangelistsMap(10)

		// 900 test variants should result in 5 batches (1000 each, last one has 500).
		tvs := testVariants(4500)
		err := Analyze(ctx, tvs, payload, sourcesMap, exporter)
		assert.Loosely(t, err, should.BeNil)

		// Check that there are 5 checkpoints created.
		assert.Loosely(t, countCheckPoint(ctx, t), should.Equal(5))
	})

	ftt.Run(`Can skip batch`, t, func(t *ftt.Test) {
		ctx := newContext(t)
		payload := tu.SamplePayload()
		sourcesMap := tu.SampleSourcesWithChangelistsMap(10)
		tvs := testVariants(100)
		err := analyzeSingleBatch(ctx, tvs, payload, sourcesMap, exporter)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, countCheckPoint(ctx, t), should.Equal(1))

		// Analyze the batch again should not throw an error.
		err = analyzeSingleBatch(ctx, tvs, payload, sourcesMap, exporter)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, countCheckPoint(ctx, t), should.Equal(1))
	})

	ftt.Run(`No commit position should skip`, t, func(t *ftt.Test) {
		ctx := newContext(t)
		payload := tu.SamplePayload()
		sourcesMap := map[string]*pb.Sources{
			"sources_id": {
				GitilesCommit: &pb.GitilesCommit{
					Host:    "host",
					Project: "proj",
					Ref:     "ref",
				},
			},
		}
		tvs := testVariants(100)
		err := Analyze(ctx, tvs, payload, sourcesMap, exporter)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, countCheckPoint(ctx, t), should.BeZero)
		assert.Loosely(t, verdictCounter.Get(ctx, "chromium", "skipped_no_commit_data"), should.Equal(100))
	})

	ftt.Run(`Filter test variant`, t, func(t *ftt.Test) {
		ctx := newContext(t)
		payload := &taskspb.IngestTestVerdicts{
			Project: "chromium",
			PresubmitRun: &controlpb.PresubmitResult{
				Status: pb.PresubmitRunStatus_PRESUBMIT_RUN_STATUS_SUCCEEDED,
				Mode:   pb.PresubmitRunMode_FULL_RUN,
			},
		}

		sourcesMap := map[string]*pb.Sources{
			"sources_id": {
				GitilesCommit: &pb.GitilesCommit{
					Host:     "host",
					Project:  "proj",
					Ref:      "ref",
					Position: 10,
				},
				Changelists: []*pb.GerritChange{
					{
						Host:     "gerrithost",
						Project:  "gerritproj",
						Change:   15,
						Patchset: 16,
					},
				},
			},
			"sources_id_2": {
				GitilesCommit: &pb.GitilesCommit{
					Host:     "host_2",
					Project:  "proj_2",
					Ref:      "ref_2",
					Position: 10,
				},
				Changelists: []*pb.GerritChange{
					{
						Host:     "gerrithost",
						Project:  "gerritproj",
						Change:   15,
						Patchset: 16,
					},
				},
				IsDirty: true,
			},
		}
		tvs := []*rdbpb.TestVariant{
			{
				// All skip.
				TestId: "1",
				Results: []*rdbpb.TestResultBundle{
					{
						Result: &rdbpb.TestResult{
							Name:   "invocations/inv-1/tests/abc",
							Status: rdbpb.TestStatus_SKIP,
						},
					},
				},
				SourcesId: "sources_id",
			},
			{
				// Duplicate.
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
				SourcesId: "sources_id",
			},
			{
				// OK.
				TestId: "3",
				Results: []*rdbpb.TestResultBundle{
					{
						Result: &rdbpb.TestResult{
							Name:   "invocations/inv-3/tests/abc",
							Status: rdbpb.TestStatus_PASS,
						},
					},
				},
				SourcesId: "sources_id",
			},
			{
				// No source ID.
				TestId: "4",
				Results: []*rdbpb.TestResultBundle{
					{
						Result: &rdbpb.TestResult{
							Name:   "invocations/inv-4/tests/abc",
							Status: rdbpb.TestStatus_PASS,
						},
					},
				},
				SourcesId: "sources_id_not_exists",
			},
			{
				// Source is dirty.
				TestId: "5",
				Results: []*rdbpb.TestResultBundle{
					{
						Result: &rdbpb.TestResult{
							Name:   "invocations/inv-5/tests/abc",
							Status: rdbpb.TestStatus_PASS,
						},
					},
				},
				SourcesId: "sources_id_2",
			},
		}
		claimedInvs := map[string]bool{
			"inv-1": true,
			"inv-3": true,
			"inv-4": true,
			"inv-5": true,
		}
		tvs, err := filterTestVariantsHighLatency(ctx, tvs, payload, claimedInvs, sourcesMap)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, len(tvs), should.Equal(1))
		assert.Loosely(t, tvs[0].TestId, should.Equal("3"))
		assert.Loosely(t, verdictCounter.Get(ctx, "chromium", "skipped_no_sources"), should.Equal(1))
		assert.Loosely(t, verdictCounter.Get(ctx, "chromium", "skipped_no_commit_data"), should.Equal(1))
		assert.Loosely(t, verdictCounter.Get(ctx, "chromium", "skipped_all_skipped_or_unclaimed"), should.Equal(2))
	})

	ftt.Run(`Filter test variant with failed presubmit`, t, func(t *ftt.Test) {
		ctx := newContext(t)
		payload := &taskspb.IngestTestVerdicts{
			Project: "chromium",
			PresubmitRun: &controlpb.PresubmitResult{
				Status: pb.PresubmitRunStatus_PRESUBMIT_RUN_STATUS_FAILED,
				Mode:   pb.PresubmitRunMode_FULL_RUN,
			},
		}

		sourcesMap := tu.SampleSourcesMap(10)
		sourcesMap["sources_id"].Changelists = []*pb.GerritChange{
			{
				Host:     "host",
				Project:  "proj",
				Patchset: 1,
				Change:   12345,
			},
		}
		tvs := []*rdbpb.TestVariant{
			{
				TestId: "1",
				Results: []*rdbpb.TestResultBundle{
					{
						Result: &rdbpb.TestResult{
							Name:   "invocations/inv-1/tests/abc",
							Status: rdbpb.TestStatus_PASS,
						},
					},
				},
				SourcesId: "sources_id",
			},
		}
		claimedInvs := map[string]bool{
			"inv-1": true,
		}
		tvs, err := filterTestVariantsHighLatency(ctx, tvs, payload, claimedInvs, sourcesMap)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, len(tvs), should.BeZero)
		assert.Loosely(t, verdictCounter.Get(ctx, "chromium", "skipped_unsubmitted_code"), should.Equal(1))
	})
}

func TestAnalyzeSingleBatch(t *testing.T) {
	exporter, client := fakeExporter()
	ftt.Run(`Analyze batch with empty buffer`, t, func(t *ftt.Test) {
		ctx := newContext(t)
		payload := tu.SamplePayload()
		sourcesMap := tu.SampleSourcesWithChangelistsMap(10)
		tvs := []*rdbpb.TestVariant{
			{
				TestId:      ":module!junit:package:class#test_1",
				VariantHash: "hash_1",
				Variant: &rdbpb.Variant{
					Def: map[string]string{
						"k": "v",
					},
				},
				Status: rdbpb.TestVariantStatus_EXPECTED,
				Results: []*rdbpb.TestResultBundle{
					{
						Result: &rdbpb.TestResult{
							Name:     "invocations/abc/tests/xyz",
							Status:   rdbpb.TestStatus_PASS,
							Expected: true,
						},
					},
				},
				SourcesId: "sources_id",
			},
			{
				TestId:      ":module!junit:package:class#test_2",
				VariantHash: "hash_2",
				Variant: &rdbpb.Variant{
					Def: map[string]string{
						"k2": "v2",
					},
				},
				Status: rdbpb.TestVariantStatus_UNEXPECTED,
				Results: []*rdbpb.TestResultBundle{
					{
						Result: &rdbpb.TestResult{
							Name:     "invocations/def/tests/xyz",
							Status:   rdbpb.TestStatus_PASS,
							Expected: true,
						},
					},
					{
						Result: &rdbpb.TestResult{
							Name:     "invocations/def/tests/xyz",
							Status:   rdbpb.TestStatus_FAIL,
							Expected: true,
						},
					},
					{
						Result: &rdbpb.TestResult{
							Name:     "invocations/def/tests/xyz",
							Status:   rdbpb.TestStatus_CRASH,
							Expected: true,
						},
					},
					{
						Result: &rdbpb.TestResult{
							Name:     "invocations/def/tests/xyz",
							Status:   rdbpb.TestStatus_ABORT,
							Expected: true,
						},
					},
					{
						Result: &rdbpb.TestResult{
							Name:   "invocations/def/tests/xyz",
							Status: rdbpb.TestStatus_PASS,
						},
					},
					{
						Result: &rdbpb.TestResult{
							Name:   "invocations/def/tests/xyz",
							Status: rdbpb.TestStatus_FAIL,
						},
					},
					{
						Result: &rdbpb.TestResult{
							Name:   "invocations/def/tests/xyz",
							Status: rdbpb.TestStatus_CRASH,
						},
					},
					{
						Result: &rdbpb.TestResult{
							Name:   "invocations/def/tests/xyz",
							Status: rdbpb.TestStatus_ABORT,
						},
					},
				},
				SourcesId: "sources_id",
			},
		}

		err := analyzeSingleBatch(ctx, tvs, payload, sourcesMap, exporter)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, countCheckPoint(ctx, t), should.Equal(1))

		// Check invocations.
		invs := fetchInvocations(ctx, t)
		assert.Loosely(t, invs, should.Match([]Invocation{
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
		}))

		// Check test variant branch.
		tvbs, err := FetchTestVariantBranches(ctx)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, len(tvbs), should.Equal(2))

		assert.Loosely(t, tvbs[0], should.Match(&testvariantbranch.Entry{
			Project:     "chromium",
			TestID:      ":module!junit:package:class#test_1",
			VariantHash: "hash_1",
			RefHash:     pbutil.SourceRefHash(pbutil.SourceRefFromSources(sourcesMap["sources_id"])),
			Variant: &pb.Variant{
				Def: map[string]string{
					"k": "v",
				},
			},
			SourceRef: &pb.SourceRef{
				System: &pb.SourceRef_Gitiles{
					Gitiles: &pb.GitilesRef{
						Host:    "host",
						Project: "proj",
						Ref:     "ref",
					},
				},
			},
			InputBuffer: &inputbuffer.Buffer{
				HotBuffer: inputbuffer.History{
					Runs: []inputbuffer.Run{
						{
							CommitPosition: 10,
							Hour:           payload.PartitionTime.AsTime(),
							Expected: inputbuffer.ResultCounts{
								PassCount: 1,
							},
						},
					},
				},
				ColdBuffer: inputbuffer.History{
					Runs: []inputbuffer.Run{},
				},
				HotBufferCapacity:  inputbuffer.DefaultHotBufferCapacity,
				ColdBufferCapacity: inputbuffer.DefaultColdBufferCapacity,
			},
		}))

		assert.Loosely(t, tvbs[1], should.Match(&testvariantbranch.Entry{
			Project:     "chromium",
			TestID:      ":module!junit:package:class#test_2",
			VariantHash: "hash_2",
			RefHash:     pbutil.SourceRefHash(pbutil.SourceRefFromSources(sourcesMap["sources_id"])),
			Variant: &pb.Variant{
				Def: map[string]string{
					"k2": "v2",
				},
			},
			SourceRef: &pb.SourceRef{
				System: &pb.SourceRef_Gitiles{
					Gitiles: &pb.GitilesRef{
						Host:    "host",
						Project: "proj",
						Ref:     "ref",
					},
				},
			},
			InputBuffer: &inputbuffer.Buffer{
				HotBuffer: inputbuffer.History{
					Runs: []inputbuffer.Run{
						{
							CommitPosition: 10,
							Hour:           payload.PartitionTime.AsTime(),
							Expected: inputbuffer.ResultCounts{
								PassCount:  1,
								FailCount:  1,
								CrashCount: 1,
								AbortCount: 1,
							},
							Unexpected: inputbuffer.ResultCounts{
								PassCount:  1,
								FailCount:  1,
								CrashCount: 1,
								AbortCount: 1,
							},
						},
					},
				},
				ColdBuffer: inputbuffer.History{
					Runs: []inputbuffer.Run{},
				},
				HotBufferCapacity:  inputbuffer.DefaultHotBufferCapacity,
				ColdBufferCapacity: inputbuffer.DefaultColdBufferCapacity,
			},
		}))

		assert.Loosely(t, len(client.Insertions), should.Equal(2))
		for _, insert := range client.Insertions {
			// Check the version is populated, but do not assert its value
			// as it is a commit timestamp that varies each time the test runs.
			assert.Loosely(t, insert.Version, should.NotBeNil)
			insert.Version = nil
		}

		sort.Slice(client.Insertions, func(i, j int) bool {
			return client.Insertions[i].TestId < client.Insertions[j].TestId
		})
		assert.Loosely(t, client.Insertions[0], should.Match(&bqpb.TestVariantBranchRow{
			Project: "chromium",
			TestIdStructured: &bqpb.TestIdentifier{
				ModuleName:        "module",
				ModuleScheme:      "junit",
				ModuleVariant:     `{"k":"v"}`,
				ModuleVariantHash: pbutil.VariantHash(pbutil.Variant("k", "v")),
				CoarseName:        "package",
				FineName:          "class",
				CaseName:          "test_1",
			},
			TestId:      ":module!junit:package:class#test_1",
			VariantHash: "hash_1",
			RefHash:     "6de221242e011c91",
			Variant:     `{"k":"v"}`,
			Ref: &pb.SourceRef{
				System: &pb.SourceRef_Gitiles{
					Gitiles: &pb.GitilesRef{
						Host:    "host",
						Project: "proj",
						Ref:     "ref",
					},
				},
			},
			Segments: []*bqpb.Segment{
				{
					StartPosition: 10,
					StartHour:     timestamppb.New(time.Unix(3600*10, 0)),
					EndPosition:   10,
					EndHour:       timestamppb.New(time.Unix(3600*10, 0)),
					Counts: &bqpb.Segment_Counts{
						TotalResults:          1,
						TotalRuns:             1,
						TotalVerdicts:         1,
						ExpectedPassedResults: 1,
					},
				},
			},
		}))
		assert.Loosely(t, client.Insertions[1], should.Match(&bqpb.TestVariantBranchRow{
			Project: "chromium",
			TestIdStructured: &bqpb.TestIdentifier{
				ModuleName:        "module",
				ModuleScheme:      "junit",
				ModuleVariant:     `{"k2":"v2"}`,
				ModuleVariantHash: pbutil.VariantHash(pbutil.Variant("k2", "v2")),
				CoarseName:        "package",
				FineName:          "class",
				CaseName:          "test_2",
			},
			TestId:      ":module!junit:package:class#test_2",
			VariantHash: "hash_2",
			RefHash:     "6de221242e011c91",
			Variant:     `{"k2":"v2"}`,
			Ref: &pb.SourceRef{
				System: &pb.SourceRef_Gitiles{
					Gitiles: &pb.GitilesRef{
						Host:    "host",
						Project: "proj",
						Ref:     "ref",
					},
				},
			},
			Segments: []*bqpb.Segment{
				{
					StartPosition: 10,
					StartHour:     timestamppb.New(time.Unix(3600*10, 0)),
					EndPosition:   10,
					EndHour:       timestamppb.New(time.Unix(3600*10, 0)),
					Counts: &bqpb.Segment_Counts{
						TotalVerdicts:            1,
						FlakyVerdicts:            1,
						TotalRuns:                1,
						FlakyRuns:                1,
						TotalResults:             8,
						UnexpectedResults:        4,
						ExpectedPassedResults:    1,
						ExpectedFailedResults:    1,
						ExpectedCrashedResults:   1,
						ExpectedAbortedResults:   1,
						UnexpectedPassedResults:  1,
						UnexpectedFailedResults:  1,
						UnexpectedCrashedResults: 1,
						UnexpectedAbortedResults: 1,
					},
				},
			},
		}))

		assert.Loosely(t, verdictCounter.Get(ctx, "chromium", "ingested"), should.Equal(2))
	})

	ftt.Run(`Analyze batch run analysis got change point`, t, func(t *ftt.Test) {
		ctx := newContext(t)
		exporter, client := fakeExporter()

		const ingestedVerdictPosition = 10

		// Store some existing data in spanner first.
		sourcesMap := tu.SampleSourcesWithChangelistsMap(ingestedVerdictPosition)
		positions := make([]int, 2000)
		total := make([]int, 2000)
		hasUnexpected := make([]int, 2000)
		for i := range 2000 {
			positions[i] = i + 1
			total[i] = 1
			if i >= 100 {
				hasUnexpected[i] = 1
			}
		}
		vs := inputbuffer.Verdicts(positions, total, hasUnexpected)
		ref := pbutil.SourceRefFromSources(sourcesMap["sources_id"])
		tvb := &testvariantbranch.Entry{
			IsNew:       true,
			Project:     "chromium",
			TestID:      ":module!junit:package:class#test_1",
			VariantHash: "hash_1",
			SourceRef:   ref,
			RefHash:     pbutil.SourceRefHash(ref),
			Variant: &pb.Variant{
				Def: map[string]string{
					"k": "v",
				},
			},
			InputBuffer: &inputbuffer.Buffer{
				HotBuffer: inputbuffer.History{
					Runs: []inputbuffer.Run{},
				},
				ColdBuffer: inputbuffer.History{
					Runs: vs,
				},
				IsColdBufferDirty: true,
			},
		}
		var hs inputbuffer.HistorySerializer
		mutation, err := tvb.ToMutation(&hs)
		assert.Loosely(t, err, should.BeNil)
		testutil.MustApply(ctx, t, mutation)

		// Insert a new verdict.
		payload := tu.SamplePayload()
		const ingestedVerdictHour = 55
		payload.PartitionTime = timestamppb.New(time.Unix(ingestedVerdictHour*3600, 0))

		tvs := []*rdbpb.TestVariant{
			{
				TestId:      ":module!junit:package:class#test_1",
				VariantHash: "hash_1",
				Status:      rdbpb.TestVariantStatus_EXPECTED,
				Results: []*rdbpb.TestResultBundle{
					{
						Result: &rdbpb.TestResult{
							Name:     "invocations/abc/tests/xyz",
							Status:   rdbpb.TestStatus_PASS,
							Expected: true,
						},
					},
				},
				SourcesId: "sources_id",
			},
		}

		err = analyzeSingleBatch(ctx, tvs, payload, sourcesMap, exporter)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, countCheckPoint(ctx, t), should.Equal(1))

		// Check invocations.
		invs := fetchInvocations(ctx, t)
		assert.Loosely(t, invs, should.Match([]Invocation{
			{
				Project:              "chromium",
				InvocationID:         "abc",
				IngestedInvocationID: "build-1234",
			},
		}))

		// Check test variant branch.
		tvbs, err := FetchTestVariantBranches(ctx)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, len(tvbs), should.Equal(1))
		tvb = tvbs[0]

		// Setup bucket expectations.
		// Only statistics for verdicts evicted from the output buffer should be
		// included in the statistics.
		var expectedBuckets []*cpb.Statistics_HourBucket
		for i := 1; i < 100; i++ {
			if i == ingestedVerdictPosition {
				// The verdict at this position was merged with
				// the ingested verdict, and attracted a later
				// hour.
				continue
			}
			bucket := &cpb.Statistics_HourBucket{
				Hour:                int64(i),
				TotalSourceVerdicts: 1,
			}
			expectedBuckets = append(expectedBuckets, bucket)
			if bucket.Hour == ingestedVerdictHour {
				// Add one for the verdict we just ingested.
				bucket.TotalSourceVerdicts += 1
			}
		}

		var expectedDistribution model.PositionDistribution
		for i, pr := range model.TailLikelihoods {
			if pr <= 0.995 {
				expectedDistribution[i] = 101
			} else {
				expectedDistribution[i] = 102
			}
		}

		assert.Loosely(t, tvb, should.Match(&testvariantbranch.Entry{
			Project:     "chromium",
			TestID:      ":module!junit:package:class#test_1",
			VariantHash: "hash_1",
			RefHash:     pbutil.SourceRefHash(pbutil.SourceRefFromSources(sourcesMap["sources_id"])),
			Variant: &pb.Variant{
				Def: map[string]string{
					"k": "v",
				},
			},
			SourceRef: &pb.SourceRef{
				System: &pb.SourceRef_Gitiles{
					Gitiles: &pb.GitilesRef{
						Host:    "host",
						Project: "proj",
						Ref:     "ref",
					},
				},
			},
			InputBuffer: &inputbuffer.Buffer{
				HotBuffer: inputbuffer.History{
					Runs: []inputbuffer.Run{},
				},
				ColdBuffer: inputbuffer.History{
					Runs: vs[100:],
				},
				HotBufferCapacity:  inputbuffer.DefaultHotBufferCapacity,
				ColdBufferCapacity: inputbuffer.DefaultColdBufferCapacity,
			},
			FinalizingSegment: &cpb.Segment{
				State:                        cpb.SegmentState_FINALIZING,
				HasStartChangepoint:          true,
				StartPosition:                101,
				StartHour:                    timestamppb.New(time.Unix(101*3600, 0)),
				FinalizedCounts:              &cpb.Counts{},
				StartPositionDistribution:    expectedDistribution.Serialize(),
				StartPositionLowerBound_99Th: 101,
				StartPositionUpperBound_99Th: 101,
			},
			FinalizedSegments: &cpb.Segments{
				Segments: []*cpb.Segment{
					{
						State:               cpb.SegmentState_FINALIZED,
						HasStartChangepoint: false,
						StartPosition:       1,
						StartHour:           timestamppb.New(time.Unix(3600, 0)),
						EndPosition:         100,
						EndHour:             timestamppb.New(time.Unix(100*3600, 0)),
						FinalizedCounts: &cpb.Counts{
							TotalResults:          101,
							TotalRuns:             101,
							TotalSourceVerdicts:   100,
							ExpectedPassedResults: 101,
						},
					},
				},
			},
			Statistics: &cpb.Statistics{
				HourlyBuckets: expectedBuckets,
				PartialSourceVerdict: &cpb.PartialSourceVerdict{
					CommitPosition:  100,
					LastHour:        timestamppb.New(time.Unix(100*3600, 0)),
					ExpectedResults: 1,
				},
			},
		}))
		assert.Loosely(t, len(client.Insertions), should.Equal(1))
		assert.Loosely(t, verdictCounter.Get(ctx, "chromium", "ingested"), should.Equal(1))
	})

	ftt.Run(`Analyze batch should apply retention policy`, t, func(t *ftt.Test) {
		ctx := newContext(t)
		exporter, client := fakeExporter()

		// Store some existing data in spanner first.
		sourcesMap := tu.SampleSourcesWithChangelistsMap(10)

		// Set up 110 finalized segments.
		finalizedSegments := []*cpb.Segment{}
		for i := range 110 {
			finalizedSegments = append(finalizedSegments, &cpb.Segment{
				EndHour:         timestamppb.New(time.Unix(int64(i*3600), 0)),
				FinalizedCounts: &cpb.Counts{},
			})
		}
		sourceRef := pbutil.SourceRefFromSources(sourcesMap["sources_id"])
		tvb := &testvariantbranch.Entry{
			IsNew:       true,
			Project:     "chromium",
			TestID:      ":module!junit:package:class#test_1",
			VariantHash: "hash_1",
			SourceRef:   sourceRef,
			RefHash:     pbutil.SourceRefHash(sourceRef),
			Variant: &pb.Variant{
				Def: map[string]string{
					"k": "v",
				},
			},
			InputBuffer: &inputbuffer.Buffer{
				HotBuffer: inputbuffer.History{
					Runs: []inputbuffer.Run{
						{
							CommitPosition: 1,
							Expected: inputbuffer.ResultCounts{
								PassCount: 1,
							},
						},
					},
				},
				ColdBuffer: inputbuffer.History{
					Runs: []inputbuffer.Run{},
				},
				IsColdBufferDirty: true,
			},
			FinalizedSegments: &cpb.Segments{Segments: finalizedSegments},
		}
		var hs inputbuffer.HistorySerializer
		mutation, err := tvb.ToMutation(&hs)
		assert.Loosely(t, err, should.BeNil)
		testutil.MustApply(ctx, t, mutation)

		// Insert a new verdict.
		payload := tu.SamplePayload()
		const ingestedVerdictHour = 5*365*24 + 13
		payload.PartitionTime = timestamppb.New(time.Unix(ingestedVerdictHour*3600, 0))

		tvs := []*rdbpb.TestVariant{
			{
				TestId:      ":module!junit:package:class#test_1",
				VariantHash: "hash_1",
				Status:      rdbpb.TestVariantStatus_EXPECTED,
				Results: []*rdbpb.TestResultBundle{
					{
						Result: &rdbpb.TestResult{
							Name:     "invocations/abc/tests/xyz",
							Status:   rdbpb.TestStatus_PASS,
							Expected: true,
						},
					},
				},
				SourcesId: "sources_id",
			},
		}

		err = analyzeSingleBatch(ctx, tvs, payload, sourcesMap, exporter)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, countCheckPoint(ctx, t), should.Equal(1))

		// Check invocations.
		invs := fetchInvocations(ctx, t)
		assert.Loosely(t, invs, should.Match([]Invocation{
			{
				Project:              "chromium",
				InvocationID:         "abc",
				IngestedInvocationID: "build-1234",
			},
		}))

		// Check test variant branch.
		tvbs, err := FetchTestVariantBranches(ctx)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, len(tvbs), should.Equal(1))
		tvb = tvbs[0]

		assert.Loosely(t, tvb, should.Match(&testvariantbranch.Entry{
			Project:     "chromium",
			TestID:      ":module!junit:package:class#test_1",
			VariantHash: "hash_1",
			RefHash:     pbutil.SourceRefHash(sourceRef),
			Variant: &pb.Variant{
				Def: map[string]string{
					"k": "v",
				},
			},
			SourceRef: &pb.SourceRef{
				System: &pb.SourceRef_Gitiles{
					Gitiles: &pb.GitilesRef{
						Host:    "host",
						Project: "proj",
						Ref:     "ref",
					},
				},
			},
			InputBuffer: &inputbuffer.Buffer{
				HotBuffer: inputbuffer.History{
					Runs: []inputbuffer.Run{
						{
							CommitPosition: 1,
							Expected: inputbuffer.ResultCounts{
								PassCount: 1,
							},
						},
						{
							CommitPosition: 10,
							Hour:           payload.PartitionTime.AsTime(),
							Expected: inputbuffer.ResultCounts{
								PassCount: 1,
							},
						},
					},
				},
				ColdBuffer: inputbuffer.History{
					Runs: []inputbuffer.Run{},
				},
				HotBufferCapacity:  inputbuffer.DefaultHotBufferCapacity,
				ColdBufferCapacity: inputbuffer.DefaultColdBufferCapacity,
			},

			FinalizedSegments: &cpb.Segments{
				Segments: finalizedSegments[14:],
			},
		}))
		assert.Loosely(t, len(client.Insertions), should.Equal(1))
		assert.Loosely(t, verdictCounter.Get(ctx, "chromium", "ingested"), should.Equal(1))
	})
}

func countCheckPoint(ctx context.Context, t testing.TB) int {
	t.Helper()
	st := spanner.NewStatement(`
			SELECT *
			FROM Checkpoints
		`)
	it := span.Query(span.Single(ctx), st)
	count := 0
	err := it.Do(func(r *spanner.Row) error {
		count++
		return nil
	})
	assert.Loosely(t, err, should.BeNil, truth.LineContext())
	return count
}

func fetchInvocations(ctx context.Context, t testing.TB) []Invocation {
	t.Helper()
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
	assert.Loosely(t, err, should.BeNil, truth.LineContext())
	return results
}

func testVariants(n int) []*rdbpb.TestVariant {
	tvs := make([]*rdbpb.TestVariant, n)
	for i := range n {
		testID := fmt.Sprintf("test_%d", i)
		tvs[i] = &rdbpb.TestVariant{
			TestId:      fmt.Sprintf("test_%d", i),
			VariantHash: fmt.Sprintf("hash_%d", i),
			SourcesId:   "sources_id",
			Results: []*rdbpb.TestResultBundle{
				{
					Result: &rdbpb.TestResult{
						Name: fmt.Sprintf("invocations/my-inv/tests/%s/results/1", testID),
					},
				},
			},
		}
	}
	return tvs
}

func newContext(t testing.TB) context.Context {
	t.Helper()
	ctx := memory.Use(testutil.IntegrationTestContext(t))
	assert.Loosely(t, config.SetTestConfig(ctx, tu.TestConfig()), should.BeNil, truth.LineContext())
	return ctx
}

func fakeExporter() (*bqexporter.Exporter, *bqexporter.FakeClient) {
	client := bqexporter.NewFakeClient()
	exporter := bqexporter.NewExporter(client)
	return exporter, client
}
