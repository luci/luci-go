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

package verdictingester

import (
	"context"
	"fmt"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	rdbpbutil "go.chromium.org/luci/resultdb/pbutil"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"

	"go.chromium.org/luci/analysis/internal/changepoints"
	"go.chromium.org/luci/analysis/internal/changepoints/bqexporter"
	tvbexporter "go.chromium.org/luci/analysis/internal/changepoints/bqexporter"
	"go.chromium.org/luci/analysis/internal/changepoints/inputbuffer"
	changepointspb "go.chromium.org/luci/analysis/internal/changepoints/proto"
	"go.chromium.org/luci/analysis/internal/changepoints/testvariantbranch"
	"go.chromium.org/luci/analysis/internal/checkpoints"
	"go.chromium.org/luci/analysis/internal/config"
	ctrlpb "go.chromium.org/luci/analysis/internal/ingestion/control/proto"
	"go.chromium.org/luci/analysis/internal/tasks/taskspb"
	"go.chromium.org/luci/analysis/internal/testutil"
	"go.chromium.org/luci/analysis/pbutil"
	configpb "go.chromium.org/luci/analysis/proto/config"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

func TestExportChangepointAnalysis(t *testing.T) {
	ftt.Run("TestExportChangepointAnalysis", t, func(t *ftt.Test) {
		ctx := testutil.IntegrationTestContext(t)
		ctx, _ = testclock.UseTime(ctx, testclock.TestRecentTimeLocal)
		ctx = memory.Use(ctx)
		cfg := &configpb.Config{
			TestVariantAnalysis: &configpb.TestVariantAnalysis{
				Enabled:               true,
				BigqueryExportEnabled: true,
			},
		}
		err := config.SetTestConfig(ctx, cfg)
		assert.Loosely(t, err, should.BeNil)

		partitionTime := clock.Now(ctx).Add(-1 * time.Hour)
		invocationCreationTime := partitionTime.Add(-3 * time.Hour)

		payload := &taskspb.IngestTestVerdicts{
			IngestionId: "ingestion-id",
			Project:     "project",
			Invocation: &ctrlpb.InvocationResult{
				ResultdbHost: "rdb-host",
				InvocationId: invocationID,
				CreationTime: timestamppb.New(invocationCreationTime),
			},
			PartitionTime: timestamppb.New(partitionTime),
			PresubmitRun: &ctrlpb.PresubmitResult{
				Status: pb.PresubmitRunStatus_PRESUBMIT_RUN_STATUS_SUCCEEDED,
				Mode:   pb.PresubmitRunMode_FULL_RUN,
			},
			PageToken:            "expected_token",
			TaskIndex:            1,
			UseNewIngestionOrder: true,
		}
		input := Inputs{
			Invocation: &rdbpb.Invocation{
				Name:         testInvocation,
				Realm:        testRealm,
				IsExportRoot: true,
				FinalizeTime: timestamppb.New(time.Date(2025, 4, 8, 0, 0, 0, 0, time.UTC)),
			},
			Verdicts: mockedQueryTestVariantsRsp().TestVariants,
			SourcesByID: map[string]*pb.Sources{
				"sources1": {
					BaseSources: &pb.Sources_GitilesCommit{
						GitilesCommit: &pb.GitilesCommit{
							Host:       "project.googlesource.com",
							Project:    "myproject/src",
							Ref:        "refs/heads/main",
							CommitHash: "abcdefabcd1234567890abcdefabcd1234567890",
							Position:   16801,
						},
					},
					Changelists: []*pb.GerritChange{
						{
							Host:      "project-review.googlesource.com",
							Project:   "myproject/src2",
							Change:    9991,
							Patchset:  82,
							OwnerKind: pb.ChangelistOwnerKind_HUMAN,
						},
					},
					IsDirty: false,
				},
			},
			Payload: payload,
		}
		tvBQExporterClient := bqexporter.NewFakeClient()
		ingester := ChangePointExporter{exporter: tvbexporter.NewExporter(tvBQExporterClient)}
		expectedCheckpoints := []checkpoints.Checkpoint{
			{
				Key: checkpoints.Key{
					Project:    "project",
					ResourceID: fmt.Sprintf("rdb-host/%v", invocationID),
					ProcessID:  "verdict-ingestion/analyze-changepoints",
					Uniquifier: ":module!junit:package:class#test_consistent_failure/hash",
				},
				// Creation and expiry time are not verified.
			},
		}

		// Populate some existing test variant analysis.
		// Test variant changepoint analysis uses invocation creation time
		// as partition time.
		branch := setupTestVariantAnalysis(ctx, t, invocationCreationTime)
		t.Run("without presubmit status", func(t *ftt.Test) {
			payload.PresubmitRun = nil

			err := ingester.Ingest(ctx, input)
			assert.NoErr(t, err)
			// Changepoint analysis should not be updated.
			// In this pipeline, invocations with changelists are ingested
			// only if the presubmit run was full and succeeded (leading
			// to CL submission). Invocations without builds cannot have
			// a presubmit run and therefore will not be ingested.
			tvbs, err := changepoints.FetchTestVariantBranches(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(tvbs), should.Equal(1))
			branch.IsNew = false
			assert.Loosely(t, tvbs[0], should.Match(branch))
			verifyCheckpoints(ctx, t, []checkpoints.Checkpoint{})
		})

		t.Run("with full run presubmit", func(t *ftt.Test) {
			err := ingester.Ingest(ctx, input)
			assert.NoErr(t, err)
			verifyTestVariantAnalysis(ctx, t, invocationCreationTime, tvBQExporterClient)
			verifyCheckpoints(ctx, t, expectedCheckpoints)
		})
	})
}

func verifyTestVariantAnalysis(ctx context.Context, t testing.TB, partitionTime time.Time, client *bqexporter.FakeClient) {
	t.Helper()
	tvbs, err := changepoints.FetchTestVariantBranches(ctx)
	assert.Loosely(t, err, should.BeNil, truth.LineContext())
	assert.Loosely(t, len(tvbs), should.Equal(1), truth.LineContext())
	sr := &pb.SourceRef{
		System: &pb.SourceRef_Gitiles{
			Gitiles: &pb.GitilesRef{
				Host:    "project.googlesource.com",
				Project: "myproject/src",
				Ref:     "refs/heads/main",
			},
		},
	}
	// Truncated to nearest hour.
	hour := time.Unix(partitionTime.Unix()/3600*3600, 0)

	assert.Loosely(t, tvbs[0], should.Match(&testvariantbranch.Entry{
		Project:     "project",
		TestID:      ":module!junit:package:class#test_consistent_failure",
		VariantHash: "hash",
		Variant:     &pb.Variant{},
		SourceRef:   sr,
		RefHash:     rdbpbutil.SourceRefHash(pbutil.SourceRefToResultDB(sr)),
		InputBuffer: &inputbuffer.Buffer{
			HotBufferCapacity:  100,
			ColdBufferCapacity: 2000,
			HotBuffer: inputbuffer.History{
				Runs: []inputbuffer.Run{
					{
						CommitPosition: 16801,
						Hour:           hour,
						Unexpected: inputbuffer.ResultCounts{
							FailCount: 1,
						},
					},
				},
			},
			ColdBuffer: inputbuffer.History{
				Runs: []inputbuffer.Run{},
			},
		},
		Statistics: &changepointspb.Statistics{
			HourlyBuckets: []*changepointspb.Statistics_HourBucket{
				{
					Hour:                     int64(hour.Unix()/3600 - 23),
					UnexpectedSourceVerdicts: 123,
					FlakySourceVerdicts:      456,
					TotalSourceVerdicts:      1999,
				},
			},
		},
	}), truth.LineContext())

	assert.Loosely(t, len(client.Insertions), should.Equal(1), truth.LineContext())
}

func setupTestVariantAnalysis(ctx context.Context, t testing.TB, partitionTime time.Time) *testvariantbranch.Entry {
	t.Helper()
	sr := &pb.SourceRef{
		System: &pb.SourceRef_Gitiles{
			Gitiles: &pb.GitilesRef{
				Host:    "project.googlesource.com",
				Project: "myproject/src",
				Ref:     "refs/heads/main",
			},
		},
	}
	// Truncated to nearest hour.
	hour := partitionTime.Unix() / 3600

	branch := &testvariantbranch.Entry{
		IsNew:       true,
		Project:     "project",
		TestID:      ":module!junit:package:class#test_consistent_failure",
		VariantHash: "hash",
		Variant:     &pb.Variant{},
		SourceRef:   sr,
		RefHash:     rdbpbutil.SourceRefHash(pbutil.SourceRefToResultDB(sr)),
		InputBuffer: &inputbuffer.Buffer{
			HotBufferCapacity:  100,
			ColdBufferCapacity: 2000,
			HotBuffer: inputbuffer.History{
				Runs: []inputbuffer.Run{},
			},
			ColdBuffer: inputbuffer.History{
				Runs: []inputbuffer.Run{},
			},
		},
		Statistics: &changepointspb.Statistics{
			HourlyBuckets: []*changepointspb.Statistics_HourBucket{
				{
					Hour:                     int64(hour - 23),
					UnexpectedSourceVerdicts: 123,
					FlakySourceVerdicts:      456,
					TotalSourceVerdicts:      1999,
				},
			},
		},
	}
	var hs inputbuffer.HistorySerializer
	m, err := branch.ToMutation(&hs)
	assert.Loosely(t, err, should.BeNil, truth.LineContext())
	testutil.MustApply(ctx, t, m)
	return branch
}
