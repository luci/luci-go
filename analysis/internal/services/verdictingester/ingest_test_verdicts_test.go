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

package verdictingester

import (
	"context"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	"github.com/golang/mock/gomock"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	rdbpbutil "go.chromium.org/luci/resultdb/pbutil"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/server/span"
	"go.chromium.org/luci/server/tq"
	"go.chromium.org/luci/server/tq/tqtesting"

	"go.chromium.org/luci/analysis/internal/analysis"
	"go.chromium.org/luci/analysis/internal/analysis/clusteredfailures"
	antsExporter "go.chromium.org/luci/analysis/internal/ants/testresults/exporter"
	"go.chromium.org/luci/analysis/internal/bqutil"
	"go.chromium.org/luci/analysis/internal/buildbucket"
	"go.chromium.org/luci/analysis/internal/changepoints"
	"go.chromium.org/luci/analysis/internal/changepoints/bqexporter"
	"go.chromium.org/luci/analysis/internal/changepoints/inputbuffer"
	changepointspb "go.chromium.org/luci/analysis/internal/changepoints/proto"
	"go.chromium.org/luci/analysis/internal/changepoints/testvariantbranch"
	"go.chromium.org/luci/analysis/internal/checkpoints"
	"go.chromium.org/luci/analysis/internal/clustering/chunkstore"
	"go.chromium.org/luci/analysis/internal/clustering/ingestion"
	"go.chromium.org/luci/analysis/internal/config"
	"go.chromium.org/luci/analysis/internal/gerrit"
	ctrlpb "go.chromium.org/luci/analysis/internal/ingestion/control/proto"
	"go.chromium.org/luci/analysis/internal/resultdb"
	"go.chromium.org/luci/analysis/internal/tasks/taskspb"
	"go.chromium.org/luci/analysis/internal/testresults"
	"go.chromium.org/luci/analysis/internal/testutil"
	"go.chromium.org/luci/analysis/internal/testverdicts"
	"go.chromium.org/luci/analysis/pbutil"
	bqpb "go.chromium.org/luci/analysis/proto/bq"
	bqpblegacy "go.chromium.org/luci/analysis/proto/bq/legacy"
	configpb "go.chromium.org/luci/analysis/proto/config"
	pb "go.chromium.org/luci/analysis/proto/v1"

	_ "go.chromium.org/luci/server/tq/txn/spanner"
)

func TestSchedule(t *testing.T) {
	ftt.Run(`TestSchedule`, t, func(t *ftt.Test) {
		ctx := testutil.IntegrationTestContext(t)
		ctx, skdr := tq.TestingContext(ctx, nil)

		task := &taskspb.IngestTestVerdicts{
			IngestionId:   "ingestion-id",
			Project:       "test-project",
			Build:         &ctrlpb.BuildResult{},
			PartitionTime: timestamppb.New(time.Date(2025, time.January, 1, 12, 0, 0, 0, time.UTC)),
		}
		expected := proto.Clone(task).(*taskspb.IngestTestVerdicts)

		_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
			Schedule(ctx, task)
			return nil
		})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, skdr.Tasks().Payloads()[0], should.Match(expected))
	})
}

const invocationID = "build-87654321"
const testInvocation = "invocations/build-87654321"
const testRealm = "project:ci"
const testBuildID = int64(87654321)

func TestIngestTestVerdicts(t *testing.T) {
	ftt.Run(`TestIngestTestVerdicts`, t, func(t *ftt.Test) {
		ctx := testutil.IntegrationTestContext(t)
		ctx = caching.WithEmptyProcessCache(ctx) // For failure association rules cache.
		ctx, skdr := tq.TestingContext(ctx, nil)
		ctx, _ = testclock.UseTime(ctx, testclock.TestRecentTimeLocal)
		ctx = memory.Use(ctx)

		chunkStore := chunkstore.NewFakeClient()
		clusteredFailures := clusteredfailures.NewFakeClient()
		testVerdicts := testverdicts.NewFakeClient()
		tvBQExporterClient := bqexporter.NewFakeClient()
		analysis := analysis.NewClusteringHandler(clusteredFailures)
		antsClient := antsExporter.NewFakeClient()
		ri := &verdictIngester{
			clustering:                ingestion.New(chunkStore, analysis),
			verdictExporter:           testverdicts.NewExporter(testVerdicts),
			testVariantBranchExporter: bqexporter.NewExporter(tvBQExporterClient),
			antsTestResultExporter:    antsExporter.NewExporter(antsClient),
		}

		t.Run(`partition time`, func(t *ftt.Test) {
			payload := &taskspb.IngestTestVerdicts{
				IngestionId: "ingestion-id",
				Project:     "test-project",
				Build: &ctrlpb.BuildResult{
					Host:    "host",
					Id:      13131313,
					Project: "project",
				},
				PartitionTime: timestamppb.New(clock.Now(ctx).Add(-1 * time.Hour)),
			}
			t.Run(`too early`, func(t *ftt.Test) {
				payload.PartitionTime = timestamppb.New(clock.Now(ctx).Add(25 * time.Hour))
				err := ri.ingestTestVerdicts(ctx, payload)
				assert.Loosely(t, err, should.ErrLike("too far in the future"))
			})
			t.Run(`too late`, func(t *ftt.Test) {
				payload.PartitionTime = timestamppb.New(clock.Now(ctx).Add(-91 * 24 * time.Hour))
				err := ri.ingestTestVerdicts(ctx, payload)
				assert.Loosely(t, err, should.ErrLike("too long ago"))
			})
		})

		t.Run(`valid payload`, func(t *ftt.Test) {
			ctl := gomock.NewController(t)
			defer ctl.Finish()

			mrc := resultdb.NewMockedClient(ctx, ctl)
			mbc := buildbucket.NewMockedClient(mrc.Ctx, ctl)
			ctx = mbc.Ctx

			clsByHost := gerritChangesByHostForTesting()
			ctx = gerrit.UseFakeClient(ctx, clsByHost)

			bHost := "host"
			partitionTime := clock.Now(ctx).Add(-1 * time.Hour)

			setupGetInvocationMock := func(modifiers ...func(*rdbpb.Invocation)) {
				invReq := &rdbpb.GetInvocationRequest{
					Name: testInvocation,
				}
				invRes := &rdbpb.Invocation{
					Name:         testInvocation,
					Realm:        testRealm,
					IsExportRoot: true,
					FinalizeTime: timestamppb.New(time.Date(2025, 4, 8, 0, 0, 0, 0, time.UTC)),
				}
				for _, modifier := range modifiers {
					modifier(invRes)
				}
				mrc.GetInvocation(invReq, invRes)
			}

			setupQueryTestVariantsMock := func(modifiers ...func(*rdbpb.QueryTestVariantsResponse)) {
				tvReq := &rdbpb.QueryTestVariantsRequest{
					Invocations: []string{testInvocation},
					PageSize:    10000,
					ResultLimit: 100,
					ReadMask:    testVariantReadMask,
					PageToken:   "expected_token",
				}
				tvRsp := mockedQueryTestVariantsRsp()
				tvRsp.NextPageToken = "continuation_token"
				for _, modifier := range modifiers {
					modifier(tvRsp)
				}
				mrc.QueryTestVariants(tvReq, tvRsp)
			}

			cfg := &configpb.Config{
				TestVariantAnalysis: &configpb.TestVariantAnalysis{
					Enabled:               true,
					BigqueryExportEnabled: true,
				},
				TestVerdictExport: &configpb.TestVerdictExport{
					Enabled: true,
				},
				Clustering: &configpb.ClusteringSystem{
					QueryTestVariantAnalysisEnabled: true,
				},
			}
			setupConfig := func(ctx context.Context, cfg *configpb.Config) {
				err := config.SetTestConfig(ctx, cfg)
				assert.Loosely(t, err, should.BeNil)
			}

			invocationCreationTime := partitionTime.Add(-3 * time.Hour)

			// Populate some existing test variant analysis.
			// Test variant changepoint analysis uses invocation creation time
			// as partition time.
			branch := setupTestVariantAnalysis(ctx, t, invocationCreationTime)

			expectedCheckpoints := []checkpoints.Checkpoint{
				{
					Key: checkpoints.Key{
						Project:    "project",
						ResourceID: fmt.Sprintf("rdb-host/%v", invocationID),
						ProcessID:  "verdict-ingestion/schedule-continuation",
						Uniquifier: "1",
					},
					// Creation and expiry time are not verified.
				},
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

			payload := &taskspb.IngestTestVerdicts{
				IngestionId: "ingestion-id",
				Project:     "project",
				Invocation: &ctrlpb.InvocationResult{
					ResultdbHost: "rdb-host",
					InvocationId: invocationID,
					CreationTime: timestamppb.New(invocationCreationTime),
				},
				Build: &ctrlpb.BuildResult{
					Host:         bHost,
					Id:           testBuildID,
					CreationTime: timestamppb.New(time.Date(2020, time.April, 1, 2, 3, 4, 5, time.UTC)),
					Project:      "project",
					Bucket:       "bucket",
					Builder:      "builder",
					Status:       pb.BuildStatus_BUILD_STATUS_FAILURE,
					Changelists: []*pb.Changelist{
						{
							Host:      "anothergerrit.gerrit.instance",
							Change:    77788,
							Patchset:  19,
							OwnerKind: pb.ChangelistOwnerKind_HUMAN,
						},
						{
							Host:      "mygerrit-review.googlesource.com",
							Change:    12345,
							Patchset:  5,
							OwnerKind: pb.ChangelistOwnerKind_AUTOMATION,
						},
					},
					Commit: &bbpb.GitilesCommit{
						Host:     "myproject.googlesource.com",
						Project:  "someproject/src",
						Id:       strings.Repeat("0a", 20),
						Ref:      "refs/heads/mybranch",
						Position: 111888,
					},
					HasInvocation:     true,
					ResultdbHost:      "test.results.api.cr.dev",
					GardenerRotations: []string{"rotation1", "rotation2"},
				},
				PartitionTime: timestamppb.New(partitionTime),
				PresubmitRun: &ctrlpb.PresubmitResult{
					PresubmitRunId: &pb.PresubmitRunId{
						System: "luci-cv",
						Id:     "infra/12345",
					},
					Status:       pb.PresubmitRunStatus_PRESUBMIT_RUN_STATUS_SUCCEEDED,
					Mode:         pb.PresubmitRunMode_FULL_RUN,
					Owner:        "automation",
					CreationTime: timestamppb.New(time.Date(2021, time.April, 1, 2, 3, 4, 5, time.UTC)),
				},
				PageToken: "expected_token",
				TaskIndex: 1,
			}
			expectedContinuation := proto.Clone(payload).(*taskspb.IngestTestVerdicts)
			expectedContinuation.PageToken = "continuation_token"
			expectedContinuation.TaskIndex = 2

			t.Run(`Without build`, func(t *ftt.Test) {
				payload.Build = nil
				payload.PresubmitRun = nil

				expectedContinuation.Build = nil
				expectedContinuation.PresubmitRun = nil

				setupGetInvocationMock()
				setupQueryTestVariantsMock()
				setupConfig(ctx, cfg)

				// Act
				err := ri.ingestTestVerdicts(ctx, payload)
				assert.Loosely(t, err, should.BeNil)

				// Verify

				// Expect a continuation task to be created.
				verifyContinuationTask(t, skdr, expectedContinuation)

				// Expect only the continuation task checkpoint.
				// The test results should not be ingested into
				// changepoint analysis here as there is no
				// presubmit run that we can use to confirm the
				// changes were submitted.
				verifyCheckpoints(ctx, t, expectedCheckpoints[:1])

				verifyTestResults(ctx, t, partitionTime, false)

				// Clustering not enabled - no chunk has been written to GCS.
				assert.Loosely(t, len(chunkStore.Contents), should.BeZero)
				verifyTestVerdicts(t, testVerdicts, partitionTime, false)

				verifyAnTSExport(t, antsClient, false)

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
			})
			t.Run(`First task`, func(t *ftt.Test) {
				setupGetInvocationMock()
				setupQueryTestVariantsMock()
				setupConfig(ctx, cfg)

				// Act
				err := ri.ingestTestVerdicts(ctx, payload)
				assert.Loosely(t, err, should.BeNil)

				// Verify

				// Expect a continuation task to be created.
				verifyContinuationTask(t, skdr, expectedContinuation)
				verifyCheckpoints(ctx, t, expectedCheckpoints)
				verifyTestResults(ctx, t, partitionTime, true)
				verifyClustering(t, chunkStore, clusteredFailures)
				verifyTestVerdicts(t, testVerdicts, partitionTime, true)
				verifyTestVariantAnalysis(ctx, t, invocationCreationTime, tvBQExporterClient)
				verifyAnTSExport(t, antsClient, false)
			})
			t.Run(`Last task`, func(t *ftt.Test) {
				payload.TaskIndex = 10
				expectedCheckpoints = removeCheckpointForProcess(expectedCheckpoints, "verdict-ingestion/schedule-continuation")

				setupGetInvocationMock()
				setupQueryTestVariantsMock(func(rsp *rdbpb.QueryTestVariantsResponse) {
					rsp.NextPageToken = ""
				})
				setupConfig(ctx, cfg)

				// Act
				err := ri.ingestTestVerdicts(ctx, payload)
				assert.Loosely(t, err, should.BeNil)

				// Verify

				// As this is the last task, do not expect a continuation
				// task to be created.
				verifyContinuationTask(t, skdr, nil)
				verifyCheckpoints(ctx, t, expectedCheckpoints) // Expect no checkpoints to be created.
				verifyTestResults(ctx, t, partitionTime, true)
				verifyClustering(t, chunkStore, clusteredFailures)
				verifyTestVerdicts(t, testVerdicts, partitionTime, true)
				verifyTestVariantAnalysis(ctx, t, invocationCreationTime, tvBQExporterClient)
				verifyAnTSExport(t, antsClient, false)
			})

			t.Run(`Export to AnTS table`, func(t *ftt.Test) {
				payload.Build = nil
				payload.PresubmitRun = nil
				payload.Project = "android"

				setupGetInvocationMock()
				setupQueryTestVariantsMock()
				setupConfig(ctx, cfg)

				// Act
				err := ri.ingestTestVerdicts(ctx, payload)
				assert.Loosely(t, err, should.BeNil)

				// Verify
				verifyAnTSExport(t, antsClient, true)
			})

			t.Run(`Retry task after continuation task already created`, func(t *ftt.Test) {
				// Scenario: First task fails after it has already scheduled
				// its continuation.
				existingCheckpoint := checkpoints.Checkpoint{
					Key: checkpoints.Key{
						Project:    "project",
						ResourceID: fmt.Sprintf("rdb-host/%v", invocationID),
						ProcessID:  "verdict-ingestion/schedule-continuation",
						Uniquifier: "1",
					},
				}
				err := checkpoints.SetForTesting(ctx, t, existingCheckpoint)
				assert.Loosely(t, err, should.BeNil)

				setupGetInvocationMock()
				setupQueryTestVariantsMock()
				setupConfig(ctx, cfg)

				// Act
				err = ri.ingestTestVerdicts(ctx, payload)
				assert.Loosely(t, err, should.BeNil)

				// Verify

				// Do not expect a continuation task to be created,
				// as it was already scheduled.
				verifyContinuationTask(t, skdr, nil)
				verifyCheckpoints(ctx, t, expectedCheckpoints)
				verifyTestResults(ctx, t, partitionTime, true)
				verifyClustering(t, chunkStore, clusteredFailures)
				verifyTestVerdicts(t, testVerdicts, partitionTime, true)
				verifyTestVariantAnalysis(ctx, t, invocationCreationTime, tvBQExporterClient)
				verifyAnTSExport(t, antsClient, false)
			})
			t.Run(`No project config`, func(t *ftt.Test) {
				// If no project config exists, results should be ingested into
				// TestResults and clustered, but not used for the legacy test variant
				// analysis.
				config.SetTestProjectConfig(ctx, map[string]*configpb.ProjectConfig{})

				setupGetInvocationMock()
				setupQueryTestVariantsMock()
				setupConfig(ctx, cfg)

				// Act
				err := ri.ingestTestVerdicts(ctx, payload)
				assert.Loosely(t, err, should.BeNil)

				// Verify
				// Test results still ingested.
				verifyTestResults(ctx, t, partitionTime, true)

				// Cluster has happened.
				verifyClustering(t, chunkStore, clusteredFailures)

				// Test verdicts exported.
				verifyTestVerdicts(t, testVerdicts, partitionTime, true)
				verifyTestVariantAnalysis(ctx, t, invocationCreationTime, tvBQExporterClient)
				verifyAnTSExport(t, antsClient, false)
			})
			t.Run(`Invocation is not an export root`, func(t *ftt.Test) {
				setupGetInvocationMock(func(i *rdbpb.Invocation) {
					i.IsExportRoot = false
				})
				setupConfig(ctx, cfg)

				// Act
				err := ri.ingestTestVerdicts(ctx, payload)
				assert.Loosely(t, err, should.BeNil)

				// Verify no test results ingested into test history.
				var actualTRs []*testresults.TestResult
				err = testresults.ReadTestResults(span.Single(ctx), spanner.AllKeys(), func(tr *testresults.TestResult) error {
					actualTRs = append(actualTRs, tr)
					return nil
				})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, actualTRs, should.HaveLength(0))
			})
			t.Run(`no invocation`, func(t *ftt.Test) {
				payload.Invocation = nil
				setupConfig(ctx, cfg)

				// Act
				err := ri.ingestTestVerdicts(ctx, payload)
				assert.Loosely(t, err, should.BeNil)

				// Verify no test results ingested into test history.
				var actualTRs []*testresults.TestResult
				err = testresults.ReadTestResults(span.Single(ctx), spanner.AllKeys(), func(tr *testresults.TestResult) error {
					actualTRs = append(actualTRs, tr)
					return nil
				})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, actualTRs, should.HaveLength(0))
			})
			t.Run(`Project not allowed`, func(t *ftt.Test) {
				cfg.Ingestion = &configpb.Ingestion{
					ProjectAllowlistEnabled: true,
					ProjectAllowlist:        []string{"other"},
				}
				setupConfig(ctx, cfg)

				// Act
				err := ri.ingestTestVerdicts(ctx, payload)
				assert.Loosely(t, err, should.BeNil)

				// Verify no test results ingested into test history.
				var actualTRs []*testresults.TestResult
				err = testresults.ReadTestResults(span.Single(ctx), spanner.AllKeys(), func(tr *testresults.TestResult) error {
					actualTRs = append(actualTRs, tr)
					return nil
				})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, actualTRs, should.HaveLength(0))
			})
		})
	})
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

func verifyTestResults(ctx context.Context, t testing.TB, expectedPartitionTime time.Time, withBuildSource bool) {
	trBuilder := testresults.NewTestResult().
		WithProject("project").
		WithPartitionTime(expectedPartitionTime.In(time.UTC)).
		WithIngestedInvocationID("build-87654321").
		WithSubRealm("ci").
		WithSources(testresults.Sources{})

	if withBuildSource {
		trBuilder = trBuilder.
			WithSources(testresults.Sources{
				Changelists: []testresults.Changelist{
					{
						Host:      "anothergerrit.gerrit.instance",
						Change:    77788,
						Patchset:  19,
						OwnerKind: pb.ChangelistOwnerKind_HUMAN,
					},
					{
						Host:      "mygerrit-review.googlesource.com",
						Change:    12345,
						Patchset:  5,
						OwnerKind: pb.ChangelistOwnerKind_AUTOMATION,
					},
				},
			})
	}

	rdbSources := testresults.Sources{
		RefHash: pbutil.SourceRefHash(&pb.SourceRef{
			System: &pb.SourceRef_Gitiles{
				Gitiles: &pb.GitilesRef{
					Host:    "project.googlesource.com",
					Project: "myproject/src",
					Ref:     "refs/heads/main",
				},
			},
		}),
		Position: 16801,
		Changelists: []testresults.Changelist{
			{
				Host:      "project-review.googlesource.com",
				Change:    9991,
				Patchset:  82,
				OwnerKind: pb.ChangelistOwnerKind_HUMAN,
			},
		},
	}

	expectedTRs := []*testresults.TestResult{
		trBuilder.WithTestID(":module!junit:package:class#test_consistent_failure").
			WithVariantHash("hash").
			WithRunIndex(0).
			WithResultIndex(0).
			WithIsUnexpected(true).
			WithStatus(pb.TestResultStatus_FAIL).
			WithRunDuration(3*time.Second+1*time.Microsecond).
			WithExonerationReasons(pb.ExonerationReason_OCCURS_ON_OTHER_CLS, pb.ExonerationReason_NOT_CRITICAL, pb.ExonerationReason_OCCURS_ON_MAINLINE).
			WithIsFromBisection(false).
			WithSources(rdbSources).
			Build(),
		trBuilder.WithTestID(":module!junit:package:class#test_expected").
			WithVariantHash("hash").
			WithRunIndex(0).
			WithResultIndex(0).
			WithIsUnexpected(false).
			WithStatus(pb.TestResultStatus_PASS).
			WithRunDuration(5 * time.Second).
			WithoutExoneration().
			WithIsFromBisection(false).
			Build(),
		trBuilder.WithTestID(":module!junit:package:class#test_filtering_event").
			WithVariantHash("hash").
			WithRunIndex(0).
			WithResultIndex(0).
			WithIsUnexpected(false).
			WithStatus(pb.TestResultStatus_SKIP).
			WithoutRunDuration().
			WithoutExoneration().
			WithIsFromBisection(false).
			Build(),
		trBuilder.WithTestID(":module!junit:package:class#test_from_luci_bisection").
			WithVariantHash("hash").
			WithRunIndex(0).
			WithResultIndex(0).
			WithIsUnexpected(true).
			WithStatus(pb.TestResultStatus_PASS).
			WithoutRunDuration().
			WithoutExoneration().
			WithIsFromBisection(true).
			Build(),
		trBuilder.WithTestID(":module!junit:package:class#test_has_unexpected").
			WithVariantHash("hash").
			WithRunIndex(0).
			WithResultIndex(0).
			WithIsUnexpected(true).
			WithStatus(pb.TestResultStatus_FAIL).
			WithoutRunDuration().
			WithoutExoneration().
			WithIsFromBisection(false).
			Build(),
		trBuilder.WithTestID(":module!junit:package:class#test_has_unexpected").
			WithVariantHash("hash").
			WithRunIndex(1).
			WithResultIndex(0).
			WithIsUnexpected(false).
			WithStatus(pb.TestResultStatus_PASS).
			WithoutRunDuration().
			WithoutExoneration().
			WithIsFromBisection(false).
			Build(),
		trBuilder.WithTestID(":module!junit:package:class#test_known_flake").
			WithVariantHash("hash_2").
			WithRunIndex(0).
			WithResultIndex(0).
			WithIsUnexpected(true).
			WithStatus(pb.TestResultStatus_FAIL).
			WithRunDuration(2 * time.Second).
			WithoutExoneration().
			WithIsFromBisection(false).
			Build(),
		trBuilder.WithTestID(":module!junit:package:class#test_new_failure").
			WithVariantHash("hash_1").
			WithRunIndex(0).
			WithResultIndex(0).
			WithIsUnexpected(true).
			WithStatus(pb.TestResultStatus_FAIL).
			WithRunDuration(1 * time.Second).
			WithoutExoneration().
			WithIsFromBisection(false).
			Build(),
		trBuilder.WithTestID(":module!junit:package:class#test_new_flake").
			WithVariantHash("hash").
			WithRunIndex(0).
			WithResultIndex(0).
			WithIsUnexpected(true).
			WithStatus(pb.TestResultStatus_FAIL).
			WithRunDuration(10 * time.Second).
			WithoutExoneration().
			WithIsFromBisection(false).
			Build(),
		trBuilder.WithTestID(":module!junit:package:class#test_new_flake").
			WithVariantHash("hash").
			WithRunIndex(0).
			WithResultIndex(1).
			WithIsUnexpected(true).
			WithStatus(pb.TestResultStatus_FAIL).
			WithRunDuration(11 * time.Second).
			WithoutExoneration().
			WithIsFromBisection(false).
			Build(),
		trBuilder.WithTestID(":module!junit:package:class#test_new_flake").
			WithVariantHash("hash").
			WithRunIndex(1).
			WithResultIndex(0).
			WithIsUnexpected(false).
			WithStatus(pb.TestResultStatus_PASS).
			WithRunDuration(12 * time.Second).
			WithoutExoneration().
			WithIsFromBisection(false).
			Build(),
		trBuilder.WithTestID(":module!junit:package:class#test_no_new_results").
			WithVariantHash("hash").
			WithRunIndex(0).
			WithResultIndex(0).
			WithIsUnexpected(true).
			WithStatus(pb.TestResultStatus_FAIL).
			WithRunDuration(4 * time.Second).
			WithoutExoneration().
			WithIsFromBisection(false).
			Build(),
		trBuilder.WithTestID(":module!junit:package:class#test_skip").
			WithVariantHash("hash").
			WithRunIndex(0).
			WithResultIndex(0).
			WithIsUnexpected(true).
			WithStatus(pb.TestResultStatus_SKIP).
			WithoutRunDuration().
			WithoutExoneration().
			WithIsFromBisection(false).
			Build(),
		trBuilder.WithTestID(":module!junit:package:class#test_unexpected_pass").
			WithVariantHash("hash").
			WithRunIndex(0).
			WithResultIndex(0).
			WithIsUnexpected(true).
			WithStatus(pb.TestResultStatus_PASS).
			WithoutRunDuration().
			WithoutExoneration().
			WithIsFromBisection(false).
			Build(),
	}

	// Validate TestResults table is populated.
	var actualTRs []*testresults.TestResult
	err := testresults.ReadTestResults(span.Single(ctx), spanner.AllKeys(), func(tr *testresults.TestResult) error {
		actualTRs = append(actualTRs, tr)
		return nil
	})
	assert.Loosely(t, err, should.BeNil, truth.LineContext())
	assert.Loosely(t, actualTRs, should.Match(expectedTRs), truth.LineContext())

	// Validate TestVariantRealms table is populated.
	tvrs := make([]*testresults.TestVariantRealm, 0)
	err = testresults.ReadTestVariantRealms(span.Single(ctx), spanner.AllKeys(), func(tvr *testresults.TestVariantRealm) error {
		tvrs = append(tvrs, tvr)
		return nil
	})
	assert.Loosely(t, err, should.BeNil, truth.LineContext())

	expectedRealms := []*testresults.TestVariantRealm{
		{
			Project:     "project",
			TestID:      ":module!junit:package:class#test_consistent_failure",
			VariantHash: "hash",
			SubRealm:    "ci",
			Variant:     &pb.Variant{},
		},
		{
			Project:     "project",
			TestID:      ":module!junit:package:class#test_expected",
			VariantHash: "hash",
			SubRealm:    "ci",
			Variant:     &pb.Variant{},
		},
		{
			Project:     "project",
			TestID:      ":module!junit:package:class#test_filtering_event",
			VariantHash: "hash",
			SubRealm:    "ci",
			Variant:     &pb.Variant{},
		},
		{
			Project:     "project",
			TestID:      ":module!junit:package:class#test_from_luci_bisection",
			VariantHash: "hash",
			SubRealm:    "ci",
			Variant:     &pb.Variant{},
		},
		{
			Project:     "project",
			TestID:      ":module!junit:package:class#test_has_unexpected",
			VariantHash: "hash",
			SubRealm:    "ci",
			Variant:     &pb.Variant{},
		},
		{
			Project:     "project",
			TestID:      ":module!junit:package:class#test_known_flake",
			VariantHash: "hash_2",
			SubRealm:    "ci",
			Variant:     pbutil.VariantFromResultDB(rdbpbutil.Variant("k1", "v2")),
		},
		{
			Project:     "project",
			TestID:      ":module!junit:package:class#test_new_failure",
			VariantHash: "hash_1",
			SubRealm:    "ci",
			Variant:     pbutil.VariantFromResultDB(rdbpbutil.Variant("k1", "v1")),
		},
		{
			Project:     "project",
			TestID:      ":module!junit:package:class#test_new_flake",
			VariantHash: "hash",
			SubRealm:    "ci",
			Variant:     &pb.Variant{},
		},
		{
			Project:     "project",
			TestID:      ":module!junit:package:class#test_no_new_results",
			VariantHash: "hash",
			SubRealm:    "ci",
			Variant:     &pb.Variant{},
		},
		{
			Project:     "project",
			TestID:      ":module!junit:package:class#test_skip",
			VariantHash: "hash",
			SubRealm:    "ci",
			Variant:     &pb.Variant{},
		},
		{
			Project:     "project",
			TestID:      ":module!junit:package:class#test_unexpected_pass",
			VariantHash: "hash",
			SubRealm:    "ci",
			Variant:     &pb.Variant{},
		},
	}

	assert.Loosely(t, tvrs, should.HaveLength(len(expectedRealms)), truth.LineContext())
	for i, tvr := range tvrs {
		expectedTVR := expectedRealms[i]
		assert.Loosely(t, tvr.LastIngestionTime, should.NotBeZero, truth.LineContext())
		expectedTVR.LastIngestionTime = tvr.LastIngestionTime
		assert.Loosely(t, tvr, should.Match(expectedTVR), truth.LineContext())
	}

	// Validate TestRealms table is populated.
	testRealms := make([]*testresults.TestRealm, 0)
	err = testresults.ReadTestRealms(span.Single(ctx), spanner.AllKeys(), func(tvr *testresults.TestRealm) error {
		testRealms = append(testRealms, tvr)
		return nil
	})
	assert.Loosely(t, err, should.BeNil, truth.LineContext())

	// The order of test realms doesn't matter. Sort it to make comparing it
	// against the expected test realm list easier.
	sort.Slice(testRealms, func(i, j int) bool {
		item1 := testRealms[i]
		item2 := testRealms[j]
		if item1.Project != item2.Project {
			return item1.Project <= item2.Project
		}
		if item1.SubRealm != item2.SubRealm {
			return item1.SubRealm <= item2.SubRealm
		}
		if item1.TestID != item2.TestID {
			return item1.TestID <= item2.TestID
		}
		return false
	})

	expectedTestRealms := []*testresults.TestRealm{
		{
			Project:  "project",
			TestID:   ":module!junit:package:class#test_consistent_failure",
			SubRealm: "ci",
		},
		{
			Project:  "project",
			TestID:   ":module!junit:package:class#test_expected",
			SubRealm: "ci",
		},
		{
			Project:  "project",
			TestID:   ":module!junit:package:class#test_filtering_event",
			SubRealm: "ci",
		},
		{
			Project:  "project",
			TestID:   ":module!junit:package:class#test_from_luci_bisection",
			SubRealm: "ci",
		},
		{
			Project:  "project",
			TestID:   ":module!junit:package:class#test_has_unexpected",
			SubRealm: "ci",
		},
		{
			Project:  "project",
			TestID:   ":module!junit:package:class#test_known_flake",
			SubRealm: "ci",
		},
		{
			Project:  "project",
			TestID:   ":module!junit:package:class#test_new_failure",
			SubRealm: "ci",
		},
		{
			Project:  "project",
			TestID:   ":module!junit:package:class#test_new_flake",
			SubRealm: "ci",
		},
		{
			Project:  "project",
			TestID:   ":module!junit:package:class#test_no_new_results",
			SubRealm: "ci",
		},
		{
			Project:  "project",
			TestID:   ":module!junit:package:class#test_skip",
			SubRealm: "ci",
		},
		{
			Project:  "project",
			TestID:   ":module!junit:package:class#test_unexpected_pass",
			SubRealm: "ci",
		},
	}

	assert.Loosely(t, testRealms, should.HaveLength(len(expectedTestRealms)), truth.LineContext())
	for i, tr := range testRealms {
		expectedTR := expectedTestRealms[i]
		assert.Loosely(t, tr.LastIngestionTime, should.NotBeZero, truth.LineContext())
		expectedTR.LastIngestionTime = tr.LastIngestionTime
		assert.Loosely(t, tr, should.Match(expectedTR), truth.LineContext())
	}
}

func verifyClustering(t testing.TB, chunkStore *chunkstore.FakeClient, clusteredFailures *clusteredfailures.FakeClient) {
	t.Helper()
	// Confirm chunks have been written to GCS.
	assert.Loosely(t, len(chunkStore.Contents), should.Equal(1), truth.LineContext())

	// Confirm clustering has occurred, with each test result in at
	// least one cluster.
	actualClusteredFailures := make(map[string]int)
	for _, f := range clusteredFailures.Insertions {
		assert.Loosely(t, f.Project, should.Equal("project"), truth.LineContext())
		actualClusteredFailures[f.TestId] += 1
	}
	expectedClusteredFailures := map[string]int{
		":module!junit:package:class#test_new_failure":        1,
		":module!junit:package:class#test_known_flake":        1,
		":module!junit:package:class#test_consistent_failure": 2, // One failure is in two clusters due it having a failure reason.
		":module!junit:package:class#test_no_new_results":     1,
		":module!junit:package:class#test_new_flake":          2,
		":module!junit:package:class#test_has_unexpected":     1,
	}
	assert.Loosely(t, actualClusteredFailures, should.Match(expectedClusteredFailures), truth.LineContext())

	for _, cf := range clusteredFailures.Insertions {
		assert.Loosely(t, cf.BuildGardenerRotations, should.Match([]string{"rotation1", "rotation2"}), truth.LineContext())

		// Verify test variant branch stats were correctly populated.
		if cf.TestId == ":module!junit:package:class#test_consistent_failure" {
			assert.Loosely(t, cf.TestVariantBranch, should.Match(&bqpb.ClusteredFailureRow_TestVariantBranch{
				UnexpectedVerdicts_24H: 124,
				FlakyVerdicts_24H:      456,
				TotalVerdicts_24H:      2000,
			}), truth.LineContext())
		} else {
			assert.Loosely(t, cf.TestVariantBranch, should.BeNil, truth.LineContext())
		}
	}
}

func verifyTestVerdicts(t testing.TB, client *testverdicts.FakeClient, expectedPartitionTime time.Time, expectBuild bool) {
	t.Helper()
	actualRows := client.Insertions

	invocation := &bqpb.TestVerdictRow_InvocationRecord{
		Id:         "build-87654321",
		Realm:      "project:ci",
		Properties: "{}",
	}

	// Proto marshalling may not be the same on all platforms,
	// so find what we should expect on this platform.
	tmdProperties, err := bqutil.MarshalStructPB(&structpb.Struct{
		Fields: map[string]*structpb.Value{
			"string":  structpb.NewStringValue("value"),
			"number":  structpb.NewNumberValue(123),
			"boolean": structpb.NewBoolValue(true),
		},
	})
	assert.NoErr(t, err)

	testMetadata := &bqpb.TestMetadata{
		Name: "updated_name",
		Location: &rdbpb.TestLocation{
			Repo:     "repo",
			FileName: "file_name",
			Line:     456,
		},
		BugComponent: &rdbpb.BugComponent{
			System: &rdbpb.BugComponent_IssueTracker{
				IssueTracker: &rdbpb.IssueTrackerComponent{
					ComponentId: 12345,
				},
			},
		},
		PropertiesSchema: "myproject.MyMessage",
		Properties:       tmdProperties,
		PreviousTestId:   "another_previous_test_id",
	}

	var buildbucketBuild *bqpb.TestVerdictRow_BuildbucketBuild
	var cvRun *bqpb.TestVerdictRow_ChangeVerifierRun
	if expectBuild {
		buildbucketBuild = &bqpb.TestVerdictRow_BuildbucketBuild{
			Id: testBuildID,
			Builder: &bqpb.TestVerdictRow_BuildbucketBuild_Builder{
				Project: "project",
				Bucket:  "bucket",
				Builder: "builder",
			},
			Status:            "FAILURE",
			GardenerRotations: []string{"rotation1", "rotation2"},
		}
		cvRun = &bqpb.TestVerdictRow_ChangeVerifierRun{
			Id:              "infra/12345",
			Mode:            pb.PresubmitRunMode_FULL_RUN,
			Status:          "SUCCEEDED",
			IsBuildCritical: false,
		}
	}

	// Different platforms may use different spacing when serializing
	// JSONPB. Expect the spacing scheme used by this platform.
	expectedProperties, err := bqutil.MarshalStructPB(testProperties)
	assert.Loosely(t, err, should.BeNil, truth.LineContext())

	sr := &pb.SourceRef{
		System: &pb.SourceRef_Gitiles{
			Gitiles: &pb.GitilesRef{
				Host:    "project.googlesource.com",
				Project: "myproject/src",
				Ref:     "refs/heads/main",
			},
		},
	}
	expectedRows := []*bqpb.TestVerdictRow{
		{
			Project: "project",
			TestIdStructured: &bqpb.TestIdentifier{
				ModuleName:        "module",
				ModuleScheme:      "junit",
				ModuleVariant:     "{}",
				ModuleVariantHash: pbutil.VariantHash(pbutil.Variant()), // Hash of empty test variant.
				CoarseName:        "package",
				FineName:          "class",
				CaseName:          "test_consistent_failure",
			},
			TestId:        ":module!junit:package:class#test_consistent_failure",
			Variant:       "{}",
			VariantHash:   "hash",
			Invocation:    invocation,
			PartitionTime: timestamppb.New(expectedPartitionTime),
			Status:        pb.TestVerdictStatus_EXONERATED,
			Results: []*bqpb.TestVerdictRow_TestResult{
				{
					Parent: &bqpb.TestVerdictRow_ParentInvocationRecord{
						Id: "build-1234",
					},
					Name:        "invocations/build-1234/tests/:module%21junit:package:class%23test_consistent_failure/results/one",
					ResultId:    "one",
					Expected:    false,
					Status:      pb.TestResultStatus_FAIL,
					SummaryHtml: "SummaryHTML",
					StartTime:   timestamppb.New(time.Date(2010, time.March, 1, 0, 0, 0, 0, time.UTC)),
					Duration:    3.000001,
					FailureReason: &rdbpb.FailureReason{
						PrimaryErrorMessage: "abc.def(123): unexpected nil-deference",
					},
					Properties: expectedProperties,
				},
			},
			Exonerations: []*bqpb.TestVerdictRow_Exoneration{
				{
					ExplanationHtml: "LUCI Analysis reported this test as flaky.",
					Reason:          pb.ExonerationReason_OCCURS_ON_OTHER_CLS,
				},
				{
					ExplanationHtml: "Test is marked informational.",
					Reason:          pb.ExonerationReason_NOT_CRITICAL,
				},
				{
					Reason: pb.ExonerationReason_OCCURS_ON_MAINLINE,
				},
			},
			Counts: &bqpb.TestVerdictRow_Counts{
				Total: 1, TotalNonSkipped: 1, Unexpected: 1, UnexpectedNonSkipped: 1, UnexpectedNonSkippedNonPassed: 1,
			},
			BuildbucketBuild:  buildbucketBuild,
			ChangeVerifierRun: cvRun,
			Sources: &pb.Sources{
				GitilesCommit: &pb.GitilesCommit{
					Host:       "project.googlesource.com",
					Project:    "myproject/src",
					Ref:        "refs/heads/main",
					CommitHash: "abcdefabcd1234567890abcdefabcd1234567890",
					Position:   16801,
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
			SourceRef:     sr,
			SourceRefHash: hex.EncodeToString(pbutil.SourceRefHash(sr)),
			InsertTime:    timestamppb.New(testclock.TestRecentTimeLocal),
		},
		{
			Project: "project",
			TestIdStructured: &bqpb.TestIdentifier{
				ModuleName:        "module",
				ModuleScheme:      "junit",
				ModuleVariant:     "{}",
				ModuleVariantHash: pbutil.VariantHash(pbutil.Variant()), // Hash of empty test variant.
				CoarseName:        "package",
				FineName:          "class",
				CaseName:          "test_expected",
			},
			TestId:        ":module!junit:package:class#test_expected",
			Variant:       "{}",
			VariantHash:   "hash",
			Invocation:    invocation,
			PartitionTime: timestamppb.New(expectedPartitionTime),
			Status:        pb.TestVerdictStatus_EXPECTED,
			Results: []*bqpb.TestVerdictRow_TestResult{
				{
					Parent: &bqpb.TestVerdictRow_ParentInvocationRecord{
						Id: "build-1234",
					},
					Name:       "invocations/build-1234/tests/:module%21junit:package:class%23test_expected/results/one",
					ResultId:   "one",
					StartTime:  timestamppb.New(time.Date(2010, time.May, 1, 0, 0, 0, 0, time.UTC)),
					Status:     pb.TestResultStatus_PASS,
					Expected:   true,
					Duration:   5.0,
					Properties: "{}",
				},
			},
			Counts: &bqpb.TestVerdictRow_Counts{
				Total: 1, TotalNonSkipped: 1, Unexpected: 0, UnexpectedNonSkipped: 0, UnexpectedNonSkippedNonPassed: 0,
			},
			BuildbucketBuild:  buildbucketBuild,
			ChangeVerifierRun: cvRun,
			InsertTime:        timestamppb.New(testclock.TestRecentTimeLocal),
		},
		{
			Project: "project",
			TestIdStructured: &bqpb.TestIdentifier{
				ModuleName:        "module",
				ModuleScheme:      "junit",
				ModuleVariant:     "{}",
				ModuleVariantHash: pbutil.VariantHash(pbutil.Variant()), // Hash of empty test variant.
				CoarseName:        "package",
				FineName:          "class",
				CaseName:          "test_filtering_event",
			},
			TestId:        ":module!junit:package:class#test_filtering_event",
			Variant:       "{}",
			VariantHash:   "hash",
			Invocation:    invocation,
			PartitionTime: timestamppb.New(expectedPartitionTime),
			Status:        pb.TestVerdictStatus_EXPECTED,
			Results: []*bqpb.TestVerdictRow_TestResult{
				{
					Parent: &bqpb.TestVerdictRow_ParentInvocationRecord{
						Id: "build-1234",
					},
					Name:       "invocations/build-1234/tests/:module%21junit:package:class%23test_filtering_event/results/one",
					ResultId:   "one",
					StartTime:  timestamppb.New(time.Date(2010, time.February, 2, 0, 0, 0, 0, time.UTC)),
					Status:     pb.TestResultStatus_SKIP,
					SkipReason: rdbpb.SkipReason_AUTOMATICALLY_DISABLED_FOR_FLAKINESS.String(),
					Expected:   true,
					Properties: "{}",
				},
			},
			Counts: &bqpb.TestVerdictRow_Counts{
				Total: 1, TotalNonSkipped: 0, Unexpected: 0, UnexpectedNonSkipped: 0, UnexpectedNonSkippedNonPassed: 0,
			},
			BuildbucketBuild:  buildbucketBuild,
			ChangeVerifierRun: cvRun,
			InsertTime:        timestamppb.New(testclock.TestRecentTimeLocal),
		},
		{
			Project: "project",
			TestIdStructured: &bqpb.TestIdentifier{
				ModuleName:        "module",
				ModuleScheme:      "junit",
				ModuleVariant:     "{}",
				ModuleVariantHash: pbutil.VariantHash(pbutil.Variant()), // Hash of empty test variant.
				CoarseName:        "package",
				FineName:          "class",
				CaseName:          "test_from_luci_bisection",
			},
			TestId:        ":module!junit:package:class#test_from_luci_bisection",
			Variant:       "{}",
			VariantHash:   "hash",
			Invocation:    invocation,
			PartitionTime: timestamppb.New(expectedPartitionTime),
			Status:        pb.TestVerdictStatus_UNEXPECTED,
			Results: []*bqpb.TestVerdictRow_TestResult{
				{
					Parent: &bqpb.TestVerdictRow_ParentInvocationRecord{
						Id: "build-1234",
					},
					Name:       "invocations/build-1234/tests/:module%21junit:package:class%23test_from_luci_bisection/results/one",
					ResultId:   "one",
					Status:     pb.TestResultStatus_PASS,
					Expected:   false,
					Properties: "{}",
					Tags:       []*pb.StringPair{{Key: "is_luci_bisection", Value: "true"}},
				},
			},
			Counts: &bqpb.TestVerdictRow_Counts{
				Total: 1, TotalNonSkipped: 1, Unexpected: 1, UnexpectedNonSkipped: 1, UnexpectedNonSkippedNonPassed: 0,
			},
			BuildbucketBuild:  buildbucketBuild,
			ChangeVerifierRun: cvRun,
			InsertTime:        timestamppb.New(testclock.TestRecentTimeLocal),
		},
		{
			Project: "project",
			TestIdStructured: &bqpb.TestIdentifier{
				ModuleName:        "module",
				ModuleScheme:      "junit",
				ModuleVariant:     "{}",
				ModuleVariantHash: pbutil.VariantHash(pbutil.Variant()), // Hash of empty test variant.
				CoarseName:        "package",
				FineName:          "class",
				CaseName:          "test_has_unexpected",
			},
			TestId:        ":module!junit:package:class#test_has_unexpected",
			VariantHash:   "hash",
			Variant:       "{}",
			Invocation:    invocation,
			PartitionTime: timestamppb.New(expectedPartitionTime),
			Status:        pb.TestVerdictStatus_FLAKY,
			Results: []*bqpb.TestVerdictRow_TestResult{
				{
					Parent: &bqpb.TestVerdictRow_ParentInvocationRecord{
						Id: "invocation-0b",
					},
					Name:       "invocations/invocation-0b/tests/:module%21junit:package:class%23test_has_unexpected/results/one",
					ResultId:   "one",
					StartTime:  timestamppb.New(time.Date(2010, time.February, 1, 0, 0, 10, 0, time.UTC)),
					Status:     pb.TestResultStatus_FAIL,
					Expected:   false,
					Properties: "{}",
				},
				{
					Parent: &bqpb.TestVerdictRow_ParentInvocationRecord{
						Id: "invocation-0a",
					},
					Name:       "invocations/invocation-0a/tests/:module%21junit:package:class%23test_has_unexpected/results/two",
					ResultId:   "two",
					StartTime:  timestamppb.New(time.Date(2010, time.February, 1, 0, 0, 20, 0, time.UTC)),
					Status:     pb.TestResultStatus_PASS,
					Expected:   true,
					Properties: "{}",
				},
			},
			Counts: &bqpb.TestVerdictRow_Counts{
				Total: 2, TotalNonSkipped: 2, Unexpected: 1, UnexpectedNonSkipped: 1, UnexpectedNonSkippedNonPassed: 1,
			},
			BuildbucketBuild:  buildbucketBuild,
			ChangeVerifierRun: cvRun,
			InsertTime:        timestamppb.New(testclock.TestRecentTimeLocal),
		},
		{
			Project: "project",
			TestIdStructured: &bqpb.TestIdentifier{
				ModuleName:        "module",
				ModuleScheme:      "junit",
				ModuleVariant:     `{"k1":"v2"}`,
				ModuleVariantHash: pbutil.VariantHash(pbutil.Variant("k1", "v2")),
				CoarseName:        "package",
				FineName:          "class",
				CaseName:          "test_known_flake",
			},
			TestId:        ":module!junit:package:class#test_known_flake",
			Variant:       `{"k1":"v2"}`,
			VariantHash:   "hash_2",
			Invocation:    invocation,
			PartitionTime: timestamppb.New(expectedPartitionTime),
			Status:        pb.TestVerdictStatus_UNEXPECTED,
			TestMetadata:  testMetadata,
			Results: []*bqpb.TestVerdictRow_TestResult{
				{
					Parent: &bqpb.TestVerdictRow_ParentInvocationRecord{
						Id: "build-1234",
					},
					Name:       "invocations/build-1234/tests/:module%21junit:package:class%23test_known_flake/results/one",
					ResultId:   "one",
					StartTime:  timestamppb.New(time.Date(2010, time.February, 1, 0, 0, 0, 0, time.UTC)),
					Status:     pb.TestResultStatus_FAIL,
					Expected:   false,
					Duration:   2.0,
					Tags:       pbutil.StringPairs("os", "Mac", "monorail_component", "Monorail>Component"),
					Properties: "{}",
				},
			},
			Counts: &bqpb.TestVerdictRow_Counts{
				Total: 1, TotalNonSkipped: 1, Unexpected: 1, UnexpectedNonSkipped: 1, UnexpectedNonSkippedNonPassed: 1,
			},
			BuildbucketBuild:  buildbucketBuild,
			ChangeVerifierRun: cvRun,
			InsertTime:        timestamppb.New(testclock.TestRecentTimeLocal),
		},
		{
			Project: "project",
			TestIdStructured: &bqpb.TestIdentifier{
				ModuleName:        "module",
				ModuleScheme:      "junit",
				ModuleVariant:     `{"k1":"v1"}`,
				ModuleVariantHash: pbutil.VariantHash(pbutil.Variant("k1", "v1")),
				CoarseName:        "package",
				FineName:          "class",
				CaseName:          "test_new_failure",
			},
			TestId:        ":module!junit:package:class#test_new_failure",
			Variant:       `{"k1":"v1"}`,
			VariantHash:   "hash_1",
			Invocation:    invocation,
			PartitionTime: timestamppb.New(expectedPartitionTime),
			Status:        pb.TestVerdictStatus_UNEXPECTED,
			TestMetadata:  testMetadata,
			Results: []*bqpb.TestVerdictRow_TestResult{
				{
					Parent: &bqpb.TestVerdictRow_ParentInvocationRecord{
						Id: "build-1234",
					},
					Name:       "invocations/build-1234/tests/:module%21junit:package:class%23test_new_failure/results/one",
					ResultId:   "one",
					StartTime:  timestamppb.New(time.Date(2010, time.January, 1, 0, 0, 0, 0, time.UTC)),
					Status:     pb.TestResultStatus_FAIL,
					Expected:   false,
					Duration:   1.0,
					Tags:       pbutil.StringPairs("random_tag", "random_tag_value", "public_buganizer_component", "951951951"),
					Properties: "{}",
				},
			},
			Counts: &bqpb.TestVerdictRow_Counts{
				Total: 1, TotalNonSkipped: 1, Unexpected: 1, UnexpectedNonSkipped: 1, UnexpectedNonSkippedNonPassed: 1,
			},
			BuildbucketBuild:  buildbucketBuild,
			ChangeVerifierRun: cvRun,
			InsertTime:        timestamppb.New(testclock.TestRecentTimeLocal),
		},
		{
			Project: "project",
			TestIdStructured: &bqpb.TestIdentifier{
				ModuleName:        "module",
				ModuleScheme:      "junit",
				ModuleVariant:     `{}`,
				ModuleVariantHash: pbutil.VariantHash(pbutil.Variant()),
				CoarseName:        "package",
				FineName:          "class",
				CaseName:          "test_new_flake",
			},
			TestId:        ":module!junit:package:class#test_new_flake",
			Variant:       "{}",
			VariantHash:   "hash",
			Invocation:    invocation,
			PartitionTime: timestamppb.New(expectedPartitionTime),
			Status:        pb.TestVerdictStatus_FLAKY,
			Results: []*bqpb.TestVerdictRow_TestResult{
				{
					Parent: &bqpb.TestVerdictRow_ParentInvocationRecord{
						Id: "invocation-1234",
					},
					Name:       "invocations/invocation-1234/tests/:module%21junit:package:class%23test_new_flake/results/two",
					ResultId:   "two",
					StartTime:  timestamppb.New(time.Date(2010, time.January, 1, 0, 0, 20, 0, time.UTC)),
					Status:     pb.TestResultStatus_FAIL,
					Expected:   false,
					Duration:   11.0,
					Properties: "{}",
				},
				{
					Parent: &bqpb.TestVerdictRow_ParentInvocationRecord{
						Id: "invocation-1234",
					},
					Name:       "invocations/invocation-1234/tests/:module%21junit:package:class%23test_new_flake/results/one",
					ResultId:   "one",
					StartTime:  timestamppb.New(time.Date(2010, time.January, 1, 0, 0, 10, 0, time.UTC)),
					Status:     pb.TestResultStatus_FAIL,
					Expected:   false,
					Duration:   10.0,
					Properties: "{}",
				},
				{
					Parent: &bqpb.TestVerdictRow_ParentInvocationRecord{
						Id: "invocation-4567",
					},
					Name:       "invocations/invocation-4567/tests/:module%21junit:package:class%23test_new_flake/results/three",
					ResultId:   "three",
					StartTime:  timestamppb.New(time.Date(2010, time.January, 1, 0, 0, 15, 0, time.UTC)),
					Status:     pb.TestResultStatus_PASS,
					Expected:   true,
					Duration:   12.0,
					Properties: "{}",
				},
			},
			Counts: &bqpb.TestVerdictRow_Counts{
				Total: 3, TotalNonSkipped: 3, Unexpected: 2, UnexpectedNonSkipped: 2, UnexpectedNonSkippedNonPassed: 2,
			},
			BuildbucketBuild:  buildbucketBuild,
			ChangeVerifierRun: cvRun,
			InsertTime:        timestamppb.New(testclock.TestRecentTimeLocal),
		},
		{
			Project: "project",
			TestIdStructured: &bqpb.TestIdentifier{
				ModuleName:        "module",
				ModuleScheme:      "junit",
				ModuleVariant:     `{}`,
				ModuleVariantHash: pbutil.VariantHash(pbutil.Variant()),
				CoarseName:        "package",
				FineName:          "class",
				CaseName:          "test_no_new_results",
			},
			TestId:        ":module!junit:package:class#test_no_new_results",
			Variant:       "{}",
			VariantHash:   "hash",
			Invocation:    invocation,
			PartitionTime: timestamppb.New(expectedPartitionTime),
			Status:        pb.TestVerdictStatus_UNEXPECTED,
			Results: []*bqpb.TestVerdictRow_TestResult{
				{
					Parent: &bqpb.TestVerdictRow_ParentInvocationRecord{
						Id: "build-1234",
					},
					Name:       "invocations/build-1234/tests/:module%21junit:package:class%23test_no_new_results/results/one",
					ResultId:   "one",
					StartTime:  timestamppb.New(time.Date(2010, time.April, 1, 0, 0, 0, 0, time.UTC)),
					Status:     pb.TestResultStatus_FAIL,
					Expected:   false,
					Duration:   4.0,
					Properties: "{}",
				},
			},
			Counts: &bqpb.TestVerdictRow_Counts{
				Total: 1, TotalNonSkipped: 1, Unexpected: 1, UnexpectedNonSkipped: 1, UnexpectedNonSkippedNonPassed: 1,
			},
			BuildbucketBuild:  buildbucketBuild,
			ChangeVerifierRun: cvRun,
			InsertTime:        timestamppb.New(testclock.TestRecentTimeLocal),
		},
		{
			Project: "project",
			TestIdStructured: &bqpb.TestIdentifier{
				ModuleName:        "module",
				ModuleScheme:      "junit",
				ModuleVariant:     `{}`,
				ModuleVariantHash: pbutil.VariantHash(pbutil.Variant()),
				CoarseName:        "package",
				FineName:          "class",
				CaseName:          "test_skip",
			},
			TestId:        ":module!junit:package:class#test_skip",
			Variant:       "{}",
			VariantHash:   "hash",
			Invocation:    invocation,
			PartitionTime: timestamppb.New(expectedPartitionTime),
			Status:        pb.TestVerdictStatus_UNEXPECTEDLY_SKIPPED,
			Results: []*bqpb.TestVerdictRow_TestResult{
				{
					Parent: &bqpb.TestVerdictRow_ParentInvocationRecord{
						Id: "build-1234",
					},
					Name:       "invocations/build-1234/tests/:module%21junit:package:class%23test_skip/results/one",
					ResultId:   "one",
					StartTime:  timestamppb.New(time.Date(2010, time.February, 2, 0, 0, 0, 0, time.UTC)),
					Status:     pb.TestResultStatus_SKIP,
					Expected:   false,
					Properties: "{}",
				},
			},
			Counts: &bqpb.TestVerdictRow_Counts{
				Total: 1, TotalNonSkipped: 0, Unexpected: 1, UnexpectedNonSkipped: 0, UnexpectedNonSkippedNonPassed: 0,
			},
			BuildbucketBuild:  buildbucketBuild,
			ChangeVerifierRun: cvRun,
			InsertTime:        timestamppb.New(testclock.TestRecentTimeLocal),
		},
		{
			Project: "project",
			TestIdStructured: &bqpb.TestIdentifier{
				ModuleName:        "module",
				ModuleScheme:      "junit",
				ModuleVariant:     `{}`,
				ModuleVariantHash: pbutil.VariantHash(pbutil.Variant()),
				CoarseName:        "package",
				FineName:          "class",
				CaseName:          "test_unexpected_pass",
			},
			TestId:        ":module!junit:package:class#test_unexpected_pass",
			Variant:       "{}",
			VariantHash:   "hash",
			Invocation:    invocation,
			PartitionTime: timestamppb.New(expectedPartitionTime),
			Status:        pb.TestVerdictStatus_UNEXPECTED,
			Results: []*bqpb.TestVerdictRow_TestResult{
				{
					Parent: &bqpb.TestVerdictRow_ParentInvocationRecord{
						Id: "build-1234",
					},
					Name:       "invocations/build-1234/tests/:module%21junit:package:class%23test_unexpected_pass/results/one",
					ResultId:   "one",
					Status:     pb.TestResultStatus_PASS,
					Expected:   false,
					Properties: "{}",
				},
			},
			Counts: &bqpb.TestVerdictRow_Counts{
				Total: 1, TotalNonSkipped: 1, Unexpected: 1, UnexpectedNonSkipped: 1, UnexpectedNonSkippedNonPassed: 0,
			},
			BuildbucketBuild:  buildbucketBuild,
			ChangeVerifierRun: cvRun,
			InsertTime:        timestamppb.New(testclock.TestRecentTimeLocal),
		},
	}
	assert.Loosely(t, actualRows, should.HaveLength(len(expectedRows)), truth.LineContext())
	for i, row := range actualRows {
		assert.Loosely(t, row, should.Match(expectedRows[i]), truth.LineContext())
	}
}

func verifyContinuationTask(t testing.TB, skdr *tqtesting.Scheduler, expectedContinuation *taskspb.IngestTestVerdicts) {
	t.Helper()
	count := 0
	for _, pl := range skdr.Tasks().Payloads() {
		if pl, ok := pl.(*taskspb.IngestTestVerdicts); ok {
			assert.Loosely(t, pl, should.Match(expectedContinuation), truth.LineContext())
			count++
		}
	}
	if expectedContinuation != nil {
		assert.Loosely(t, count, should.Equal(1), truth.LineContext())
	} else {
		assert.Loosely(t, count, should.BeZero, truth.LineContext())
	}
}

func verifyCheckpoints(ctx context.Context, t testing.TB, expected []checkpoints.Checkpoint) {
	t.Helper()
	result, err := checkpoints.ReadAllForTesting(span.Single(ctx))
	assert.Loosely(t, err, should.BeNil, truth.LineContext())

	var wantKeys []checkpoints.Key
	var gotKeys []checkpoints.Key
	for _, c := range expected {
		wantKeys = append(wantKeys, c.Key)
	}
	for _, c := range result {
		gotKeys = append(gotKeys, c.Key)
	}
	checkpoints.SortKeys(gotKeys)
	checkpoints.SortKeys(wantKeys)

	assert.Loosely(t, gotKeys, should.Match(wantKeys), truth.LineContext())
}

func removeCheckpointForProcess(cs []checkpoints.Checkpoint, processID string) []checkpoints.Checkpoint {
	var result []checkpoints.Checkpoint
	for _, c := range cs {
		if c.ProcessID != processID {
			result = append(result, c)
		}
	}
	return result
}

func verifyAnTSExport(t testing.TB, client *antsExporter.FakeClient, shouldExport bool) {
	t.Helper()
	actualRows := client.Insertions
	var expectedRows []*bqpblegacy.AntsTestResultRow
	if shouldExport {
		expectedRows = []*bqpblegacy.AntsTestResultRow{
			{
				TestResultId: "one",
				InvocationId: invocationID,
				TestIdentifier: &bqpblegacy.AntsTestResultRow_TestIdentifier{
					Module:      "module",
					TestClass:   "package.class",
					ClassName:   "class",
					PackageName: "package",
					Method:      "test_consistent_failure",
				},
				TestIdentifierHash: "439715a66ee549703a54f6ba7fc6883bb29bc6485ea04ddeace09a3675e8c188",
				TestStatus:         bqpblegacy.AntsTestResultRow_FAIL,
				DebugInfo: &bqpblegacy.AntsTestResultRow_DebugInfo{
					ErrorMessage: "abc.def(123): unexpected nil-deference",
				},
				Timing: &bqpblegacy.AntsTestResultRow_Timing{
					CreationTimestamp: 1267401600, // 2010-03-01 00:00:00 UTC
					CompleteTimestamp: 1267401603, // Start + 3s
					CreationMonth:     "2010-03",  // From StartTime
				},
				TestId:         ":module!junit:package:class#test_consistent_failure",
				CompletionTime: timestamppb.New(time.Date(2025, 4, 8, 0, 0, 0, 0, time.UTC)),
				InsertTime:     timestamppb.New(testclock.TestRecentTimeLocal),
			},
			{
				TestResultId: "one",
				InvocationId: invocationID,
				TestIdentifier: &bqpblegacy.AntsTestResultRow_TestIdentifier{
					Module:      "module",
					TestClass:   "package.class",
					ClassName:   "class",
					PackageName: "package",
					Method:      "test_expected",
				},
				TestIdentifierHash: "c9b736b4fa95a9e2726cc1d54407a4b3cfef716b41932bf0b8764aa060d4cc30",
				TestStatus:         bqpblegacy.AntsTestResultRow_PASS,
				Timing: &bqpblegacy.AntsTestResultRow_Timing{
					CreationTimestamp: 1272672000, // 2010-05-01 00:00:00 UTC
					CompleteTimestamp: 1272672005, // Start + 5s
					CreationMonth:     "2010-05",
				},
				TestId:         ":module!junit:package:class#test_expected",
				CompletionTime: timestamppb.New(time.Date(2025, 4, 8, 0, 0, 0, 0, time.UTC)),
				InsertTime:     timestamppb.New(testclock.TestRecentTimeLocal),
			},
			{
				TestResultId: "one",
				InvocationId: invocationID,
				TestIdentifier: &bqpblegacy.AntsTestResultRow_TestIdentifier{
					Module:      "module",
					TestClass:   "package.class",
					ClassName:   "class",
					PackageName: "package",
					Method:      "test_filtering_event",
				},
				TestIdentifierHash: "e0c64cde6e07ab7e6056eab6904ac2dd7572955c2fa46f905df6240077821f1a",
				TestStatus:         bqpblegacy.AntsTestResultRow_TEST_SKIPPED,
				Timing: &bqpblegacy.AntsTestResultRow_Timing{
					CreationTimestamp: 1265068800, // 2010-02-02 00:00:00 UTC
					CompleteTimestamp: 1265068800, // Start + 0s
					CreationMonth:     "2010-02",
				},
				TestId:         ":module!junit:package:class#test_filtering_event",
				CompletionTime: timestamppb.New(time.Date(2025, 4, 8, 0, 0, 0, 0, time.UTC)),
				InsertTime:     timestamppb.New(testclock.TestRecentTimeLocal),
			},
			{
				TestResultId: "one",
				InvocationId: invocationID,
				TestIdentifier: &bqpblegacy.AntsTestResultRow_TestIdentifier{
					Module:      "module",
					TestClass:   "package.class",
					ClassName:   "class",
					PackageName: "package",
					Method:      "test_from_luci_bisection",
				},
				TestIdentifierHash: "7fbf857cd5260ec91627cd5b8558f57ed2079db286f75a94a73e279b777b372a",
				TestStatus:         bqpblegacy.AntsTestResultRow_PASS,
				Timing: &bqpblegacy.AntsTestResultRow_Timing{
					CreationTimestamp: 0,         // StartTime is nil in input
					CompleteTimestamp: 0,         // Start + 0s
					CreationMonth:     "1970-01", // Default from zero time
				},
				Properties: []*bqpblegacy.AntsTestResultRow_StringPair{
					{Name: "is_luci_bisection", Value: "true"},
				},
				TestId:         ":module!junit:package:class#test_from_luci_bisection",
				CompletionTime: timestamppb.New(time.Date(2025, 4, 8, 0, 0, 0, 0, time.UTC)),
				InsertTime:     timestamppb.New(testclock.TestRecentTimeLocal),
			},
			{
				TestResultId: "one",
				InvocationId: invocationID,
				TestIdentifier: &bqpblegacy.AntsTestResultRow_TestIdentifier{
					Module:      "module",
					TestClass:   "package.class",
					ClassName:   "class",
					PackageName: "package",
					Method:      "test_has_unexpected",
				},
				TestIdentifierHash: "02a1ce2dd211366074c1456e549acfe211c957090f2f55071c09c550c0a1e9d0",
				TestStatus:         bqpblegacy.AntsTestResultRow_FAIL,
				Timing: &bqpblegacy.AntsTestResultRow_Timing{
					CreationTimestamp: 1264982410, // 2010-02-01 00:00:10 UTC
					CompleteTimestamp: 1264982410, // Start + 0s
					CreationMonth:     "2010-02",
				},
				TestId:         ":module!junit:package:class#test_has_unexpected",
				CompletionTime: timestamppb.New(time.Date(2025, 4, 8, 0, 0, 0, 0, time.UTC)),
				InsertTime:     timestamppb.New(testclock.TestRecentTimeLocal),
			},
			{
				TestResultId: "two",
				InvocationId: invocationID,
				TestIdentifier: &bqpblegacy.AntsTestResultRow_TestIdentifier{
					Module:      "module",
					TestClass:   "package.class",
					ClassName:   "class",
					PackageName: "package",
					Method:      "test_has_unexpected",
				},
				TestIdentifierHash: "02a1ce2dd211366074c1456e549acfe211c957090f2f55071c09c550c0a1e9d0",
				TestStatus:         bqpblegacy.AntsTestResultRow_PASS,
				Timing: &bqpblegacy.AntsTestResultRow_Timing{
					CreationTimestamp: 1264982420, // 2010-02-01 00:00:20 UTC
					CompleteTimestamp: 1264982420, // Start + 0s
					CreationMonth:     "2010-02",
				},
				TestId:         ":module!junit:package:class#test_has_unexpected",
				CompletionTime: timestamppb.New(time.Date(2025, 4, 8, 0, 0, 0, 0, time.UTC)),
				InsertTime:     timestamppb.New(testclock.TestRecentTimeLocal),
			},
			{
				TestResultId: "one",
				InvocationId: invocationID,
				TestIdentifier: &bqpblegacy.AntsTestResultRow_TestIdentifier{
					Module:      "module",
					TestClass:   "package.class",
					ClassName:   "class",
					PackageName: "package",
					Method:      "test_known_flake",
				},
				TestIdentifierHash: "f244f7948ccfb0ff95f037103d6f1d935145a7a92277e3e0e12a77f4ad344e87",
				TestStatus:         bqpblegacy.AntsTestResultRow_FAIL,
				Timing: &bqpblegacy.AntsTestResultRow_Timing{
					CreationTimestamp: 1264982400, // 2010-02-01 00:00:00 UTC
					CompleteTimestamp: 1264982402, // Start + 2s
					CreationMonth:     "2010-02",
				},
				Properties: []*bqpblegacy.AntsTestResultRow_StringPair{
					{Name: "os", Value: "Mac"},
					{Name: "monorail_component", Value: "Monorail>Component"},
				},
				TestId:         ":module!junit:package:class#test_known_flake",
				CompletionTime: timestamppb.New(time.Date(2025, 4, 8, 0, 0, 0, 0, time.UTC)),
				InsertTime:     timestamppb.New(testclock.TestRecentTimeLocal),
			},
			{
				TestResultId: "one",
				InvocationId: invocationID,
				TestIdentifier: &bqpblegacy.AntsTestResultRow_TestIdentifier{
					Module:      "module",
					TestClass:   "package.class",
					ClassName:   "class",
					PackageName: "package",
					Method:      "test_new_failure",
				},
				TestIdentifierHash: "e121e340a729e3bc31f66b747e96577028c0c87a4c8c36f950a6e12b5d57bfe3",
				TestStatus:         bqpblegacy.AntsTestResultRow_FAIL,
				Timing: &bqpblegacy.AntsTestResultRow_Timing{
					CreationTimestamp: 1262304000, // 2010-01-01 00:00:00 UTC
					CompleteTimestamp: 1262304001, // Start + 1s
					CreationMonth:     "2010-01",
				},
				Properties: []*bqpblegacy.AntsTestResultRow_StringPair{
					{Name: "random_tag", Value: "random_tag_value"},
					{Name: "public_buganizer_component", Value: "951951951"},
				},
				TestId:         ":module!junit:package:class#test_new_failure",
				CompletionTime: timestamppb.New(time.Date(2025, 4, 8, 0, 0, 0, 0, time.UTC)),
				InsertTime:     timestamppb.New(testclock.TestRecentTimeLocal),
			},
			{
				TestResultId: "two",
				InvocationId: invocationID,
				TestIdentifier: &bqpblegacy.AntsTestResultRow_TestIdentifier{
					Module:      "module",
					TestClass:   "package.class",
					ClassName:   "class",
					PackageName: "package",
					Method:      "test_new_flake",
				},
				TestIdentifierHash: "c6b22f8616e1403c797c923d867e0ab9a970915218a2f04a58f058809aa1a180",
				TestStatus:         bqpblegacy.AntsTestResultRow_FAIL,
				Timing: &bqpblegacy.AntsTestResultRow_Timing{
					CreationTimestamp: 1262304020, // 2010-01-01 00:00:20 UTC
					CompleteTimestamp: 1262304031, // Start + 11s
					CreationMonth:     "2010-01",
				},
				TestId:         ":module!junit:package:class#test_new_flake",
				CompletionTime: timestamppb.New(time.Date(2025, 4, 8, 0, 0, 0, 0, time.UTC)),
				InsertTime:     timestamppb.New(testclock.TestRecentTimeLocal),
			},
			{
				TestResultId: "one",
				InvocationId: invocationID,
				TestIdentifier: &bqpblegacy.AntsTestResultRow_TestIdentifier{
					Module:      "module",
					TestClass:   "package.class",
					ClassName:   "class",
					PackageName: "package",
					Method:      "test_new_flake",
				},
				TestIdentifierHash: "c6b22f8616e1403c797c923d867e0ab9a970915218a2f04a58f058809aa1a180",
				TestStatus:         bqpblegacy.AntsTestResultRow_FAIL,
				Timing: &bqpblegacy.AntsTestResultRow_Timing{
					CreationTimestamp: 1262304010, // 2010-01-01 00:00:10 UTC
					CompleteTimestamp: 1262304020, // Start + 10s
					CreationMonth:     "2010-01",
				},
				TestId:         ":module!junit:package:class#test_new_flake",
				CompletionTime: timestamppb.New(time.Date(2025, 4, 8, 0, 0, 0, 0, time.UTC)),
				InsertTime:     timestamppb.New(testclock.TestRecentTimeLocal),
			},
			{
				TestResultId: "three",
				InvocationId: invocationID,
				TestIdentifier: &bqpblegacy.AntsTestResultRow_TestIdentifier{
					Module:      "module",
					TestClass:   "package.class",
					ClassName:   "class",
					PackageName: "package",
					Method:      "test_new_flake",
				},
				TestIdentifierHash: "c6b22f8616e1403c797c923d867e0ab9a970915218a2f04a58f058809aa1a180",
				TestStatus:         bqpblegacy.AntsTestResultRow_PASS,
				Timing: &bqpblegacy.AntsTestResultRow_Timing{
					CreationTimestamp: 1262304015, // 2010-01-01 00:00:15 UTC
					CompleteTimestamp: 1262304027, // Start + 12s
					CreationMonth:     "2010-01",
				},
				TestId:         ":module!junit:package:class#test_new_flake",
				CompletionTime: timestamppb.New(time.Date(2025, 4, 8, 0, 0, 0, 0, time.UTC)),
				InsertTime:     timestamppb.New(testclock.TestRecentTimeLocal),
			},
			{
				TestResultId: "one",
				InvocationId: invocationID,
				TestIdentifier: &bqpblegacy.AntsTestResultRow_TestIdentifier{
					Module:      "module",
					TestClass:   "package.class",
					ClassName:   "class",
					PackageName: "package",
					Method:      "test_no_new_results",
				},
				TestIdentifierHash: "c7129eb417ecbf4dff711bda0ef118759b474c8acac81dfc221fdcc19aabdebd",
				TestStatus:         bqpblegacy.AntsTestResultRow_FAIL,
				Timing: &bqpblegacy.AntsTestResultRow_Timing{
					CreationTimestamp: 1270080000, // 2010-04-01 00:00:00 UTC
					CompleteTimestamp: 1270080004, // Start + 4s
					CreationMonth:     "2010-04",
				},
				TestId:         ":module!junit:package:class#test_no_new_results",
				CompletionTime: timestamppb.New(time.Date(2025, 4, 8, 0, 0, 0, 0, time.UTC)),
				InsertTime:     timestamppb.New(testclock.TestRecentTimeLocal),
			},
			{
				TestResultId: "one",
				InvocationId: invocationID,
				TestIdentifier: &bqpblegacy.AntsTestResultRow_TestIdentifier{
					Module:      "module",
					TestClass:   "package.class",
					ClassName:   "class",
					PackageName: "package",
					Method:      "test_skip",
				},
				TestIdentifierHash: "bf352f379fffe06da2f8c24ae6ddc8bac9d2d8ac18238ab25ee7771d59ff230e",
				TestStatus:         bqpblegacy.AntsTestResultRow_TEST_SKIPPED,
				Timing: &bqpblegacy.AntsTestResultRow_Timing{
					CreationTimestamp: 1265068800, // 2010-02-02 00:00:00 UTC
					CompleteTimestamp: 1265068800, // Start + 0s
					CreationMonth:     "2010-02",
				},
				TestId:         ":module!junit:package:class#test_skip",
				CompletionTime: timestamppb.New(time.Date(2025, 4, 8, 0, 0, 0, 0, time.UTC)),
				InsertTime:     timestamppb.New(testclock.TestRecentTimeLocal),
			},
			{
				TestResultId: "one",
				InvocationId: invocationID,
				TestIdentifier: &bqpblegacy.AntsTestResultRow_TestIdentifier{
					Module:      "module",
					TestClass:   "package.class",
					ClassName:   "class",
					PackageName: "package",
					Method:      "test_unexpected_pass",
				},
				TestIdentifierHash: "336d86b97fda98f485e7a94fa47e4ef03672bf3dcec192371c9eb2b4ee313ac4",
				TestStatus:         bqpblegacy.AntsTestResultRow_PASS,
				Timing: &bqpblegacy.AntsTestResultRow_Timing{
					CreationTimestamp: 0,         // StartTime is nil in input
					CompleteTimestamp: 0,         // Start + 0s
					CreationMonth:     "1970-01", // Default from zero time
				},
				TestId:         ":module!junit:package:class#test_unexpected_pass",
				CompletionTime: timestamppb.New(time.Date(2025, 4, 8, 0, 0, 0, 0, time.UTC)),
				InsertTime:     timestamppb.New(testclock.TestRecentTimeLocal),
			},
		}
		assert.Loosely(t, actualRows, should.HaveLength(len(expectedRows)), truth.LineContext())
		for i, row := range actualRows {
			assert.Loosely(t, row, should.Match(expectedRows[i]), truth.LineContext())
		}
	}
}
