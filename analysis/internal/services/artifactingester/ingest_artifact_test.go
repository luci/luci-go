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

package artifactingester

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	resultpb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/server/span"
	"go.chromium.org/luci/server/tq"
	"go.chromium.org/luci/server/tq/tqtesting"

	antsexporter "go.chromium.org/luci/analysis/internal/ants/artifacts/exporter"
	"go.chromium.org/luci/analysis/internal/checkpoints"
	"go.chromium.org/luci/analysis/internal/config"
	"go.chromium.org/luci/analysis/internal/resultdb"
	"go.chromium.org/luci/analysis/internal/tasks/taskspb"
	"go.chromium.org/luci/analysis/internal/testutil"
	bqpb "go.chromium.org/luci/analysis/proto/bq/legacy"
	configpb "go.chromium.org/luci/analysis/proto/config"
)

// Constants for test data.
const (
	testProject            = "android"
	testResultDBHost       = "test-resultdb-host"
	testRootInvocationID   = "test-invocation-123"
	testRootInvocationName = "rootInvocations/" + testRootInvocationID
	testRealm              = testProject + ":ci"
)

func TestSchedule(t *testing.T) {
	ftt.Run(`TestSchedule`, t, func(t *ftt.Test) {
		ctx := testutil.IntegrationTestContext(t)
		ctx, skdr := tq.TestingContext(ctx, nil)

		task := &taskspb.IngestArtifacts{
			RootInvocationNotification: &resultpb.RootInvocationFinalizedNotification{
				ResultdbHost: testResultDBHost,
				RootInvocation: &resultpb.RootInvocation{
					Name:             testRootInvocationName,
					Realm:            testRealm,
					FinalizeTime:     timestamppb.New(time.Date(2025, 4, 8, 0, 0, 0, 0, time.UTC)),
					RootInvocationId: testRootInvocationID,
				},
			},
			PageToken: "initial-token",
			TaskIndex: 1,
		}
		expected := proto.Clone(task).(*taskspb.IngestArtifacts)

		Schedule(ctx, task)

		// Verify that one task was scheduled.
		scheduledTasks := skdr.Tasks().Payloads()
		assert.Loosely(t, scheduledTasks, should.HaveLength(1))

		// Verify the scheduled task matches the expected payload.
		// Note: The `Title` field is generated dynamically, so we only compare Payload.
		actualTaskPayload, ok := scheduledTasks[0].(*taskspb.IngestArtifacts)
		assert.Loosely(t, ok, should.BeTrue)
		assert.Loosely(t, actualTaskPayload, should.Match(expected))
	})
}

func TestArtifactIngesterRun(t *testing.T) {
	ftt.Run(`TestArtifactIngesterRun`, t, func(t *ftt.Test) {
		ctx := testutil.IntegrationTestContext(t)
		ctx, skdr := tq.TestingContext(ctx, nil)
		ctx = memory.Use(ctx) // Use in-memory datastore for checkpoints

		// Mock the AnTS exporter.
		antsClient := antsexporter.NewFakeClient()
		ingester := &artifactIngester{
			antsExporter: antsexporter.NewExporter(antsClient),
		}

		invocationFinalizedTime := timestamppb.New(time.Date(2025, 4, 8, 0, 0, 0, 0, time.UTC))

		// Base payload.
		basePayload := &taskspb.IngestArtifacts{
			RootInvocationNotification: &resultpb.RootInvocationFinalizedNotification{
				ResultdbHost: testResultDBHost,
				RootInvocation: &resultpb.RootInvocation{
					Name:             testRootInvocationName,
					Realm:            testRealm,
					FinalizeTime:     invocationFinalizedTime,
					RootInvocationId: testRootInvocationID,
				},
			},
			PageToken: "initial-page-token",
			TaskIndex: 1,
		}

		t.Run(`Valid payload`, func(t *ftt.Test) {
			ctl := gomock.NewController(t)
			defer ctl.Finish()
			// Mock the ResultDB client.
			mrc := resultdb.NewMockedClient(ctx, ctl)
			ctx = mrc.Ctx

			req := &resultpb.QueryArtifactsRequest{
				Parent:    testRootInvocationName,
				PageSize:  1000,
				PageToken: "initial-page-token",
				ReadMask: &fieldmaskpb.FieldMask{
					Paths: artifactFields,
				},
			}

			mockArtifacts := []*resultpb.Artifact{
				{
					Name:        fmt.Sprintf("rootInvocations/%s/workUnits/wu-1/artifacts/screenshot.png", testRootInvocationID),
					ArtifactId:  "screenshot.png",
					ContentType: "image/png",
					SizeBytes:   102400,
				},
				{
					Name:        fmt.Sprintf("rootInvocations/%s/workUnits/wu-2/tests/my_test_case/results/result789/artifacts/log.txt", testRootInvocationID),
					TestId:      "my_test_case",
					ResultId:    "result789",
					ArtifactId:  "log.txt",
					ContentType: "text/plain",
					SizeBytes:   5120,
				},
			}
			expectedAntsArtifactRows := []*bqpb.AntsArtifactRow{
				{
					InvocationId:   testRootInvocationID,
					WorkUnitId:     "wu-1",
					TestResultId:   "",
					Name:           "screenshot.png",
					Size:           102400,
					ContentType:    "image/png",
					ArtifactType:   "",
					CompletionTime: invocationFinalizedTime,
				},
				{
					InvocationId:   testRootInvocationID,
					WorkUnitId:     "wu-2",
					TestResultId:   "result789",
					Name:           "log.txt",
					Size:           5120,
					ContentType:    "text/plain",
					ArtifactType:   "",
					CompletionTime: invocationFinalizedTime,
				},
			}

			setupQueryArtifactsMock := func(req *resultpb.QueryArtifactsRequest, modifiers ...func(*resultpb.QueryArtifactsResponse)) {
				arRes := &resultpb.QueryArtifactsResponse{
					Artifacts:     mockArtifacts,
					NextPageToken: "continuation-token",
				}
				for _, modifier := range modifiers {
					modifier(arRes)
				}
				mrc.QueryArtifacts(req, arRes)
			}

			// Set up service config.
			cfg := &configpb.Config{}
			err := config.SetTestConfig(ctx, cfg)
			assert.Loosely(t, err, should.BeNil)

			t.Run(`with next page`, func(t *ftt.Test) {
				setupQueryArtifactsMock(req)

				err := ingester.run(ctx, basePayload)
				assert.Loosely(t, err, should.BeNil)

				// A continuation task should be scheduled.
				expectedContinuationPayload := proto.Clone(basePayload).(*taskspb.IngestArtifacts)
				expectedContinuationPayload.PageToken = "continuation-token"
				expectedContinuationPayload.TaskIndex = basePayload.TaskIndex + 1 // TaskIndex increments
				verifyScheduledArtifactTask(t, skdr, expectedContinuationPayload)

				// A checkpoint should be created.
				expectedCheckpoints := []checkpoints.Checkpoint{
					{
						Key: checkpoints.Key{
							Project:    testProject,
							ResourceID: fmt.Sprintf("rootInvocation/%s/%s", testResultDBHost, "test-invocation-123"),
							ProcessID:  "artifact-ingestion/schedule-continuation",
							Uniquifier: fmt.Sprintf("%v", basePayload.TaskIndex), // Uniquifier is current TaskIndex
						},
					},
				}
				verifyCheckpoints(ctx, t, expectedCheckpoints)
				// Check exported artifacts.
				assert.Loosely(t, antsClient.Insertions, should.Match(expectedAntsArtifactRows))
			})

			t.Run(`last page`, func(t *ftt.Test) {
				setupQueryArtifactsMock(req, func(res *resultpb.QueryArtifactsResponse) {
					res.NextPageToken = ""
				})

				err := ingester.run(ctx, basePayload)
				assert.Loosely(t, err, should.BeNil)
				// No continuation task should be scheduled.
				assert.Loosely(t, skdr.Tasks().Payloads(), should.BeEmpty)
				// No checkpoints should be created.
				verifyCheckpoints(ctx, t, []checkpoints.Checkpoint{})
				// Check exported artifacts.
				assert.Loosely(t, antsClient.Insertions, should.Match(expectedAntsArtifactRows))
			})

			t.Run(`Retry Task After Continuation Task Already Created`, func(t *ftt.Test) {
				setupQueryArtifactsMock(req, func(res *resultpb.QueryArtifactsResponse) {
					res.NextPageToken = "next-page-token"
				})

				// Pre-populate a checkpoint, simulating a previous (failed) attempt
				// that successfully scheduled the continuation task.
				existingCheckpoint := checkpoints.Checkpoint{
					Key: checkpoints.Key{
						Project:    testProject,
						ResourceID: fmt.Sprintf("rootInvocation/%s/%s", testResultDBHost, testRootInvocationID),
						ProcessID:  "artifact-ingestion/schedule-continuation",
						Uniquifier: fmt.Sprintf("%v", basePayload.TaskIndex),
					},
					ExpiryTime: time.Now().Add(checkpointTTL), // Needs a valid expiry
				}
				err := checkpoints.SetForTesting(ctx, t, existingCheckpoint)
				assert.Loosely(t, err, should.BeNil)

				err = ingester.run(ctx, basePayload)
				assert.Loosely(t, err, should.BeNil)
				// No new continuation task should be scheduled because the checkpoint exists.
				assert.Loosely(t, skdr.Tasks().Payloads(), should.BeEmpty)
				// The checkpoint should still exist.
				verifyCheckpoints(ctx, t, []checkpoints.Checkpoint{existingCheckpoint})
				// Check exported artifacts.
				assert.Loosely(t, antsClient.Insertions, should.Match(expectedAntsArtifactRows))
			})
			t.Run("legacy invocation payload", func(t *ftt.Test) {
				basePayload.Notification = &resultpb.InvocationFinalizedNotification{
					ResultdbHost: testResultDBHost,
					Invocation:   "invocations/test-invocation",
					Realm:        testRealm,
				}
				basePayload.RootInvocationNotification = nil
				// Set up invocation mock.
				invReq := &resultpb.GetInvocationRequest{
					Name: "invocations/test-invocation",
				}
				invRes := &resultpb.Invocation{
					Name:         "invocations/test-invocation",
					Realm:        testRealm,
					IsExportRoot: true,
					FinalizeTime: invocationFinalizedTime,
				}
				mrc.GetInvocation(invReq, invRes)
				req.Parent = ""
				req.Invocations = []string{"invocations/test-invocation"}
				setupQueryArtifactsMock(req)

				err := ingester.run(ctx, basePayload)
				assert.Loosely(t, err, should.BeNil)

				// A continuation task should be scheduled.
				expectedContinuationPayload := proto.Clone(basePayload).(*taskspb.IngestArtifacts)
				expectedContinuationPayload.PageToken = "continuation-token"
				expectedContinuationPayload.TaskIndex = basePayload.TaskIndex + 1 // TaskIndex increments
				verifyScheduledArtifactTask(t, skdr, expectedContinuationPayload)

				// A checkpoint should be created.
				expectedCheckpoints := []checkpoints.Checkpoint{
					{
						Key: checkpoints.Key{
							Project:    testProject,
							ResourceID: fmt.Sprintf("%s/%s", testResultDBHost, "test-invocation"),
							ProcessID:  "artifact-ingestion/schedule-continuation",
							Uniquifier: fmt.Sprintf("%v", basePayload.TaskIndex), // Uniquifier is current TaskIndex
						},
					},
				}
				verifyCheckpoints(ctx, t, expectedCheckpoints)

				// Check exported artifacts.
				expectedAntsArtifactRows[0].WorkUnitId = ""
				expectedAntsArtifactRows[0].InvocationId = "test-invocation"
				expectedAntsArtifactRows[1].WorkUnitId = ""
				expectedAntsArtifactRows[1].InvocationId = "test-invocation"
				assert.Loosely(t, antsClient.Insertions, should.Match(expectedAntsArtifactRows))
			})
			t.Run(`Project not allowlisted for ingestion`, func(t *ftt.Test) {
				cfg.Ingestion = &configpb.Ingestion{
					ProjectAllowlistEnabled: true,
					ProjectAllowlist:        []string{"other"},
				}
				err := config.SetTestConfig(ctx, cfg)
				assert.Loosely(t, err, should.BeNil)

				err = ingester.run(ctx, basePayload)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, antsClient.Insertions, should.BeEmpty)
			})
		})

		t.Run(`Invalid payload`, func(t *ftt.Test) {
			t.Run(`TaskIndex Zero`, func(t *ftt.Test) {
				payload := proto.Clone(basePayload).(*taskspb.IngestArtifacts)
				payload.TaskIndex = 0

				err := ingester.run(ctx, payload)
				assert.Loosely(t, err, should.ErrLike("validate payload: task index must be positive"))
			})

			// Other invalid payload cases are not tested because validatePayload is intended
			// only as a sanity check for logic errors, not as a comprehensive validation
			// of trusted input.
		})
	})
}

// verifyScheduledArtifactTask is a helper to check if the correct IngestArtifacts task was scheduled.
func verifyScheduledArtifactTask(t testing.TB, skdr *tqtesting.Scheduler, expectedPayload *taskspb.IngestArtifacts) {
	t.Helper()
	count := 0
	for _, pl := range skdr.Tasks().Payloads() {
		if actualPayload, ok := pl.(*taskspb.IngestArtifacts); ok {
			assert.Loosely(t, actualPayload, should.Match(expectedPayload), truth.LineContext())
			count++
		}
	}
	assert.Loosely(t, count, should.Equal(1), truth.LineContext())
}

// verifyCheckpoints is a helper to verify checkpoints.
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
