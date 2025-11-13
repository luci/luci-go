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
	"strings"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/server/span"
	"go.chromium.org/luci/server/tq"
	"go.chromium.org/luci/server/tq/tqtesting"

	"go.chromium.org/luci/analysis/internal/buildbucket"
	"go.chromium.org/luci/analysis/internal/checkpoints"
	"go.chromium.org/luci/analysis/internal/config"
	"go.chromium.org/luci/analysis/internal/gerrit"
	ctrlpb "go.chromium.org/luci/analysis/internal/ingestion/control/proto"
	"go.chromium.org/luci/analysis/internal/resultdb"
	"go.chromium.org/luci/analysis/internal/tasks/taskspb"
	"go.chromium.org/luci/analysis/internal/testutil"
	configpb "go.chromium.org/luci/analysis/proto/config"
	pb "go.chromium.org/luci/analysis/proto/v1"

	_ "go.chromium.org/luci/server/tq/txn/spanner"
)

func TestSchedule(t *testing.T) {
	ftt.Run(`TestSchedule`, t, func(t *ftt.Test) {
		ctx := testutil.IntegrationTestContext(t)
		ctx, skdr := tq.TestingContext(ctx, nil)

		task := &taskspb.IngestTestVerdicts{
			IngestionId:          "ingestion-id",
			Project:              "test-project",
			Build:                &ctrlpb.BuildResult{},
			PartitionTime:        timestamppb.New(time.Date(2025, time.January, 1, 12, 0, 0, 0, time.UTC)),
			UseNewIngestionOrder: true,
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
		ctx, skdr := tq.TestingContext(ctx, nil)
		ctx = memory.Use(ctx)
		testIngestor := &testIngester{}

		o := &orchestrator{
			sinks: []IngestionSink{
				testIngestor,
			},
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
				err := o.run(ctx, payload)
				assert.Loosely(t, err, should.ErrLike("too far in the future"))
			})
			t.Run(`too late`, func(t *ftt.Test) {
				payload.PartitionTime = timestamppb.New(clock.Now(ctx).Add(-91 * 24 * time.Hour))
				err := o.run(ctx, payload)
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

			invRes := &rdbpb.Invocation{
				Name:         testInvocation,
				Realm:        testRealm,
				IsExportRoot: true,
				FinalizeTime: timestamppb.New(time.Date(2025, 4, 8, 0, 0, 0, 0, time.UTC)),
			}
			setupGetInvocationMock := func(modifiers ...func(*rdbpb.Invocation)) {
				invReq := &rdbpb.GetInvocationRequest{
					Name: testInvocation,
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
					OrderBy:     "status_v2_effective",
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
				PageToken:            "expected_token",
				TaskIndex:            1,
				UseNewIngestionOrder: true,
			}
			expectedContinuation := proto.Clone(payload).(*taskspb.IngestTestVerdicts)
			expectedContinuation.PageToken = "continuation_token"
			expectedContinuation.TaskIndex = 2

			sourcesByID := map[string]*pb.Sources{
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
				"sources2": {
					IsDirty: true,
				},
			}
			t.Run(`First task`, func(t *ftt.Test) {
				setupGetInvocationMock()
				setupQueryTestVariantsMock()
				setupConfig(ctx, cfg)

				// Act
				err := o.run(ctx, payload)
				assert.Loosely(t, err, should.BeNil)

				// Verify
				assert.That(t, testIngestor.called, should.BeTrue)
				assert.That(t, testIngestor.gotInputs, should.Match(
					Inputs{
						Invocation:  invRes,
						Verdicts:    mockedQueryTestVariantsRsp().TestVariants,
						SourcesByID: sourcesByID,
						Payload:     payload,
						LastPage:    false,
					},
				))
				// Expect a continuation task to be created.
				verifyContinuationTask(t, skdr, expectedContinuation)
				verifyCheckpoints(ctx, t, expectedCheckpoints)
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
				err := o.run(ctx, payload)
				assert.Loosely(t, err, should.BeNil)

				// Verify
				assert.That(t, testIngestor.called, should.BeTrue)
				assert.That(t, testIngestor.gotInputs, should.Match(
					Inputs{
						Invocation:  invRes,
						Verdicts:    mockedQueryTestVariantsRsp().TestVariants,
						SourcesByID: sourcesByID,
						Payload:     payload,
						LastPage:    true,
					},
				))
				// As this is the last task, do not expect a continuation
				// task to be created.
				verifyContinuationTask(t, skdr, nil)
				verifyCheckpoints(ctx, t, expectedCheckpoints) // Expect no checkpoints to be created.
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
				err = o.run(ctx, payload)
				assert.Loosely(t, err, should.BeNil)

				// Verify
				assert.That(t, testIngestor.called, should.BeTrue)
				assert.That(t, testIngestor.gotInputs, should.Match(
					Inputs{
						Invocation:  invRes,
						Verdicts:    mockedQueryTestVariantsRsp().TestVariants,
						SourcesByID: sourcesByID,
						Payload:     payload,
						LastPage:    false,
					},
				))
				// Do not expect a continuation task to be created,
				// as it was already scheduled.
				verifyContinuationTask(t, skdr, nil)
				verifyCheckpoints(ctx, t, expectedCheckpoints)
			})

			t.Run(`Invocation is not an export root`, func(t *ftt.Test) {
				setupGetInvocationMock(func(i *rdbpb.Invocation) {
					i.IsExportRoot = false
				})
				setupConfig(ctx, cfg)

				// Act
				err := o.run(ctx, payload)
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, testIngestor.called, should.BeFalse)
			})
			t.Run(`no invocation`, func(t *ftt.Test) {
				payload.Invocation = nil
				setupConfig(ctx, cfg)

				// Act
				err := o.run(ctx, payload)
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, testIngestor.called, should.BeFalse)
			})
			t.Run(`Project not allowed`, func(t *ftt.Test) {
				cfg.Ingestion = &configpb.Ingestion{
					ProjectAllowlistEnabled: true,
					ProjectAllowlist:        []string{"other"},
				}
				setupConfig(ctx, cfg)

				// Act
				err := o.run(ctx, payload)
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, testIngestor.called, should.BeFalse)
			})
		})
	})
}

type testIngester struct {
	called    bool
	gotInputs Inputs
}

func (t *testIngester) Name() string {
	return "test-ingestor"
}

func (t *testIngester) Ingest(ctx context.Context, inputs Inputs) error {
	if t.called {
		return errors.New("already called")
	}
	t.gotInputs = inputs
	t.called = true
	return nil
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
