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

package resultingester

import (
	"context"
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/analysis/internal/changepoints"
	"go.chromium.org/luci/analysis/internal/changepoints/bqexporter"
	"go.chromium.org/luci/analysis/internal/changepoints/inputbuffer"
	changepointspb "go.chromium.org/luci/analysis/internal/changepoints/proto"
	"go.chromium.org/luci/analysis/internal/changepoints/testvariantbranch"
	"go.chromium.org/luci/analysis/internal/checkpoints"
	"go.chromium.org/luci/analysis/internal/config"
	"go.chromium.org/luci/analysis/internal/testutil"
	"go.chromium.org/luci/analysis/pbutil"
	bqpb "go.chromium.org/luci/analysis/proto/bq"
	configpb "go.chromium.org/luci/analysis/proto/config"
	analysispb "go.chromium.org/luci/analysis/proto/v1"

	_ "go.chromium.org/luci/server/tq/txn/spanner"
)

func TestIngestForChangepointAnalysis(t *testing.T) {
	ftt.Run("TestIngestForChangepointAnalysis", t, func(t *ftt.Test) {
		ctx := testutil.IntegrationTestContext(t)
		ctx = memory.Use(ctx) // For config cache.

		exportClient := bqexporter.NewFakeClient()
		ingester := NewIngestForChangepointAnalysis(bqexporter.NewExporter(exportClient))

		inputs := testInputs()
		// Changepoint analysis will not ingest test results with unsubmitted changes
		// in this low-latency pipeline.
		inputs.Sources.Changelists = nil
		inputs.Sources.IsDirty = false

		err := createTestVariantBranchRecords(ctx)
		assert.Loosely(t, err, should.BeNil)

		cfg := &configpb.Config{
			TestVariantAnalysis: &configpb.TestVariantAnalysis{
				Enabled:               true,
				BigqueryExportEnabled: true,
			},
		}
		err = config.SetTestConfig(ctx, cfg)
		assert.Loosely(t, err, should.BeNil)

		expectedExports := expectedExports()
		expectedCheckpoint := checkpoints.Checkpoint{
			Key: checkpoints.Key{
				Project:    "rootproject",
				ResourceID: "fake.rdb.host/test-root-invocation-name/test-invocation-name",
				ProcessID:  "result-ingestion/analyze-changepoints",
				Uniquifier: ":module!junit:package:class#test_expected/hash",
			},
			// Creation and expiry time not validated.
		}

		t.Run(`Baseline`, func(t *ftt.Test) {
			err := ingester.Ingest(ctx, inputs)
			assert.Loosely(t, err, should.BeNil)

			assert.Loosely(t, removeVersions(exportClient.Insertions), should.Match(expectedExports))
			assert.Loosely(t, verifyCheckpoints(ctx, t, expectedCheckpoint), should.BeNil)

			assert.Loosely(t, changepoints.RunCounter.Get(ctx, "rootproject", "ingested"), should.Equal(2))
			assert.Loosely(t, changepoints.RunCounter.Get(ctx, "rootproject", "skipped_only_skips"), should.Equal(1))

			t.Run(`Results are not exported again if the process is re-run`, func(t *ftt.Test) {
				exportClient.Insertions = nil

				err = ingester.Ingest(ctx, inputs)
				assert.Loosely(t, err, should.BeNil)

				// Nothing should be exported because the checkpoint already exists.
				assert.Loosely(t, exportClient.Insertions, should.BeEmpty)
				assert.Loosely(t, verifyCheckpoints(ctx, t, expectedCheckpoint), should.BeNil)
			})
		})
		t.Run(`With invocation claimed by this root invocation`, func(t *ftt.Test) {
			// E.g. this invocation was claimed for the root invocation in an
			// earlier page of test results.
			m := changepoints.ClaimInvocationMutation(inputs.Project, inputs.InvocationID, inputs.RootInvocationID)
			_, err := span.Apply(ctx, []*spanner.Mutation{m})
			assert.Loosely(t, err, should.BeNil)

			err = ingester.Ingest(ctx, inputs)
			assert.Loosely(t, err, should.BeNil)

			assert.Loosely(t, removeVersions(exportClient.Insertions), should.Match(expectedExports))
			assert.Loosely(t, verifyCheckpoints(ctx, t, expectedCheckpoint), should.BeNil)

			assert.Loosely(t, changepoints.RunCounter.Get(ctx, "rootproject", "ingested"), should.Equal(2))
			assert.Loosely(t, changepoints.RunCounter.Get(ctx, "rootproject", "skipped_only_skips"), should.Equal(1))
		})
		t.Run(`With invocation claimed by other root invocation`, func(t *ftt.Test) {
			// E.g. this invocation was first ingested under a different root
			// in this project.
			m := changepoints.ClaimInvocationMutation(inputs.Project, inputs.InvocationID, "other-root")
			_, err := span.Apply(ctx, []*spanner.Mutation{m})
			assert.Loosely(t, err, should.BeNil)

			err = ingester.Ingest(ctx, inputs)
			assert.Loosely(t, err, should.BeNil)

			assert.Loosely(t, exportClient.Insertions, should.HaveLength(0))
			assert.Loosely(t, changepoints.RunCounter.Get(ctx, "rootproject", "skipped_unclaimed"), should.Equal(1))
		})
		t.Run(`Should not ingest invocations with changelists`, func(t *ftt.Test) {
			inputs.Sources.Changelists = []*analysispb.GerritChange{
				{
					Host:     "project-review.googlesource.com",
					Project:  "gerritproject",
					Change:   9999999,
					Patchset: 11111,
				},
			}

			err := ingester.Ingest(ctx, inputs)
			assert.Loosely(t, err, should.BeNil)

			assert.Loosely(t, exportClient.Insertions, should.HaveLength(0))
			assert.Loosely(t, changepoints.RunCounter.Get(ctx, "rootproject", "skipped_excluded_from_low_latency_pipeline"), should.Equal(3))
		})
		t.Run(`Without sources`, func(t *ftt.Test) {
			inputs.Sources = nil

			err := ingester.Ingest(ctx, inputs)
			assert.Loosely(t, err, should.BeNil)

			assert.Loosely(t, exportClient.Insertions, should.HaveLength(0))
			assert.Loosely(t, changepoints.RunCounter.Get(ctx, "rootproject", "skipped_no_sources"), should.Equal(3))
		})
		t.Run(`Without gitiles commit`, func(t *ftt.Test) {
			inputs.Sources.GitilesCommit = nil

			err := ingester.Ingest(ctx, inputs)
			assert.Loosely(t, err, should.BeNil)

			assert.Loosely(t, exportClient.Insertions, should.HaveLength(0))
			assert.Loosely(t, changepoints.RunCounter.Get(ctx, "rootproject", "skipped_no_commit_data"), should.Equal(3))
		})
		t.Run(`Too far out of order source position`, func(t *ftt.Test) {
			// Out of order source positions can be accepted by changepoint analysis,
			// so long as the positions are still covered by the 2000-run input buffer
			// in which re-ordering can occur.
			inputs.Sources.GitilesCommit.Position = 1

			err := ingester.Ingest(ctx, inputs)
			assert.Loosely(t, err, should.BeNil)

			// Only "ninja://test_expected" should be ingested as it is has no
			// record in the test variant branch table yet, so it cannot be
			// out of order.
			expectedExports = expectedExports[1:]
			expectedExports[0].Segments[0].StartPosition = 1
			expectedExports[0].Segments[0].EndPosition = 1

			assert.Loosely(t, removeVersions(exportClient.Insertions), should.Match(expectedExports))
			assert.Loosely(t, changepoints.RunCounter.Get(ctx, "rootproject", "ingested"), should.Equal(1))
			assert.Loosely(t, changepoints.RunCounter.Get(ctx, "rootproject", "skipped_out_of_order"), should.Equal(1))
			assert.Loosely(t, changepoints.RunCounter.Get(ctx, "rootproject", "skipped_only_skips"), should.Equal(1))
			assert.Loosely(t, verifyCheckpoints(ctx, t, expectedCheckpoint), should.BeNil)
		})
		t.Run(`With test variant analysis disabled`, func(t *ftt.Test) {
			cfg.TestVariantAnalysis.Enabled = false
			err = config.SetTestConfig(ctx, cfg)
			assert.Loosely(t, err, should.BeNil)

			err := ingester.Ingest(ctx, inputs)
			assert.Loosely(t, err, should.BeNil)

			assert.Loosely(t, exportClient.Insertions, should.HaveLength(0))
		})
	})
}

func createTestVariantBranchRecords(ctx context.Context) error {
	sourceRef := &analysispb.SourceRef{
		System: &analysispb.SourceRef_Gitiles{
			Gitiles: &analysispb.GitilesRef{
				Host:    "project.googlesource.com",
				Project: "myproject/src",
				Ref:     "refs/heads/main",
			},
		},
	}
	existingRecord := &testvariantbranch.Entry{
		IsNew:       true,
		Project:     "rootproject",
		TestID:      ":module!junit:package:class#test_flaky",
		Variant:     &analysispb.Variant{},
		VariantHash: "hash",
		RefHash:     pbutil.SourceRefHash(sourceRef),
		SourceRef:   sourceRef,
		InputBuffer: &inputbuffer.Buffer{
			HotBuffer: inputbuffer.History{
				Runs: []inputbuffer.Run{
					{
						CommitPosition: 18,
						Hour:           time.Date(2020, time.October, 9, 8, 0, 0, 0, time.UTC),
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
		FinalizingSegment: &changepointspb.Segment{
			State:         changepointspb.SegmentState_FINALIZING,
			StartPosition: 10,
			StartHour:     timestamppb.New(time.Date(2010, time.October, 1, 1, 0, 0, 0, time.UTC)),
			FinalizedCounts: &changepointspb.Counts{
				TotalRuns: 10,
				FlakyRuns: 9,

				TotalResults:            8,
				UnexpectedResults:       7,
				ExpectedPassedResults:   6,
				UnexpectedFailedResults: 5,

				TotalSourceVerdicts:      4,
				FlakySourceVerdicts:      3,
				UnexpectedSourceVerdicts: 2,

				PartialSourceVerdict: &changepointspb.PartialSourceVerdict{
					CommitPosition:    11,
					LastHour:          timestamppb.New(time.Date(2010, time.October, 2, 0, 0, 0, 0, time.UTC)),
					UnexpectedResults: 1,
				},
			},
		},
		Statistics: &changepointspb.Statistics{
			HourlyBuckets: []*changepointspb.Statistics_HourBucket{},
			PartialSourceVerdict: &changepointspb.PartialSourceVerdict{
				CommitPosition:    11,
				LastHour:          timestamppb.New(time.Date(2010, time.October, 2, 0, 0, 0, 0, time.UTC)),
				UnexpectedResults: 1,
			},
		},
	}
	var hs inputbuffer.HistorySerializer
	m, err := existingRecord.ToMutation(&hs)
	if err != nil {
		return err
	}
	_, err = span.Apply(ctx, []*spanner.Mutation{m})
	return err
}

func expectedExports() []*bqpb.TestVariantBranchRow {
	sourceRef := &analysispb.SourceRef{
		System: &analysispb.SourceRef_Gitiles{
			Gitiles: &analysispb.GitilesRef{
				Host:    "project.googlesource.com",
				Project: "myproject/src",
				Ref:     "refs/heads/main",
			},
		},
	}

	return []*bqpb.TestVariantBranchRow{
		{
			Project:     "rootproject",
			TestId:      ":module!junit:package:class#test_flaky",
			Variant:     `{}`,
			VariantHash: "hash",
			RefHash:     "5d47c679cf080cb5",
			Ref:         sourceRef,
			Segments: []*bqpb.Segment{
				{
					StartPosition: 10,
					StartHour:     timestamppb.New(time.Date(2010, time.October, 1, 1, 0, 0, 0, time.UTC)),
					EndPosition:   16801,
					EndHour:       timestamppb.New(time.Date(2020, time.February, 3, 4, 0, 0, 0, time.UTC)),
					Counts: &bqpb.Segment_Counts{
						TotalRuns:               11 + 1,
						FlakyRuns:               9 + 1,
						UnexpectedUnretriedRuns: 1,

						TotalResults:            9 + 2,
						UnexpectedResults:       8 + 1,
						ExpectedPassedResults:   6 + 1,
						UnexpectedFailedResults: 6 + 1,

						TotalVerdicts:      6 + 1,
						FlakyVerdicts:      3 + 1,
						UnexpectedVerdicts: 4,
					},
				},
			},
		},
		{
			Project:     "rootproject",
			TestId:      ":module!junit:package:class#test_expected",
			Variant:     `{}`,
			VariantHash: "hash",
			RefHash:     "5d47c679cf080cb5",
			Ref:         sourceRef,
			Segments: []*bqpb.Segment{
				{
					StartPosition: 16801,
					StartHour:     timestamppb.New(time.Date(2020, time.February, 3, 4, 0, 0, 0, time.UTC)),
					EndPosition:   16801,
					EndHour:       timestamppb.New(time.Date(2020, time.February, 3, 4, 0, 0, 0, time.UTC)),
					Counts: &bqpb.Segment_Counts{
						TotalResults:          1,
						ExpectedPassedResults: 1,
						TotalRuns:             1,
						TotalVerdicts:         1,
					},
				},
			},
		},
	}
}

func removeVersions(rows []*bqpb.TestVariantBranchRow) []*bqpb.TestVariantBranchRow {
	var results []*bqpb.TestVariantBranchRow
	for _, row := range rows {
		copy := proto.Clone(row).(*bqpb.TestVariantBranchRow)
		copy.Version = nil
		results = append(results, copy)
	}
	return results
}
