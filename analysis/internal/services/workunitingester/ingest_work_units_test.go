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

package workunitingester

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/server/span"
	"go.chromium.org/luci/server/tq"
	"go.chromium.org/luci/server/tq/tqtesting"

	"go.chromium.org/luci/analysis/internal/checkpoints"
	"go.chromium.org/luci/analysis/internal/config"
	"go.chromium.org/luci/analysis/internal/resultdb"
	"go.chromium.org/luci/analysis/internal/tasks/taskspb"
	"go.chromium.org/luci/analysis/internal/testutil"
	configpb "go.chromium.org/luci/analysis/proto/config"
)

// Constants for test data.
const (
	testProject          = "android"
	testResultDBHost     = "test-resultdb-host"
	testRootInvocationID = "build-123"
	testRootInvocation   = "rootInvocations/" + testRootInvocationID
	testRealm            = testProject + ":ci"
)

func TestSchedule(t *testing.T) {
	ftt.Run(`TestSchedule`, t, func(t *ftt.Test) {
		ctx := testutil.IntegrationTestContext(t)
		ctx, skdr := tq.TestingContext(ctx, nil)

		task := &taskspb.IngestWorkUnits{
			RootInvocation: testRootInvocation,
			Realm:          testRealm,
			ResultdbHost:   testResultDBHost,
			PageToken:      "initial-token",
			TaskIndex:      1,
		}
		expected := proto.Clone(task).(*taskspb.IngestWorkUnits)

		Schedule(ctx, task)

		// Verify that one task was scheduled.
		scheduledTasks := skdr.Tasks().Payloads()
		assert.Loosely(t, scheduledTasks, should.HaveLength(1))

		// Verify the scheduled task matches the expected payload.
		actualTaskPayload, ok := scheduledTasks[0].(*taskspb.IngestWorkUnits)
		assert.Loosely(t, ok, should.BeTrue)
		assert.Loosely(t, actualTaskPayload, should.Match(expected))
	})
}

func TestWorkUnitIngesterRun(t *testing.T) {
	ftt.Run(`TestWorkUnitIngesterRun`, t, func(t *ftt.Test) {
		ctx := testutil.IntegrationTestContext(t)
		ctx, skdr := tq.TestingContext(ctx, nil)
		ctx = memory.Use(ctx) // Use in-memory datastore for checkpoints

		ingester := &workUnitIngester{}
		basePayload := &taskspb.IngestWorkUnits{
			RootInvocation: testRootInvocation,
			Realm:          testRealm,
			ResultdbHost:   testResultDBHost,
			PageToken:      "initial-page-token",
			TaskIndex:      1,
		}

		t.Run(`Valid payload`, func(t *ftt.Test) {
			ctl := gomock.NewController(t)
			defer ctl.Finish()
			// Mock the ResultDB client.
			mrc := resultdb.NewMockedClient(ctx, ctl)
			ctx = mrc.Ctx

			finalizeTime := timestamppb.New(time.Date(2025, 4, 8, 0, 0, 0, 0, time.UTC))
			setupGetRootInvocationMock := func(modifiers ...func(*rdbpb.RootInvocation)) {
				invReq := &rdbpb.GetRootInvocationRequest{
					Name: testRootInvocation,
				}
				invRes := &rdbpb.RootInvocation{
					Name:             testRootInvocation,
					Realm:            testRealm,
					RootInvocationId: testRootInvocationID,
					FinalizeTime:     finalizeTime,
					PrimaryBuild: &rdbpb.BuildDescriptor{
						Definition: &rdbpb.BuildDescriptor_AndroidBuild{
							AndroidBuild: &rdbpb.AndroidBuildDescriptor{
								BuildId:     "123",
								Branch:      "main",
								BuildTarget: "test-target",
							},
						},
					},
				}
				for _, modifier := range modifiers {
					modifier(invRes)
				}
				mrc.GetRootInvocation(invReq, invRes)
			}
			setupQueryWorkUnitsMock := func(modifiers ...func(*rdbpb.QueryWorkUnitsResponse)) {
				wuReq := &rdbpb.QueryWorkUnitsRequest{
					Parent:    testRootInvocation,
					PageSize:  1000,
					PageToken: "initial-page-token",
					View:      rdbpb.WorkUnitView_WORK_UNIT_VIEW_BASIC,
				}
				wuRes := &rdbpb.QueryWorkUnitsResponse{
					WorkUnits:     []*rdbpb.WorkUnit{},
					NextPageToken: "continuation-token",
				}
				for _, modifier := range modifiers {
					modifier(wuRes)
				}
				mrc.QueryWorkUnits(wuReq, wuRes)
			}

			// Set up service config.
			cfg := &configpb.Config{
				Ingestion: &configpb.Ingestion{
					ProjectAllowlistEnabled: true,
					ProjectAllowlist:        []string{testProject},
				},
			}
			err := config.SetTestConfig(ctx, cfg)
			assert.Loosely(t, err, should.BeNil)

			t.Run(`with next page`, func(t *ftt.Test) {
				setupGetRootInvocationMock()
				setupQueryWorkUnitsMock()

				err := ingester.run(ctx, basePayload)
				assert.Loosely(t, err, should.BeNil)

				// A continuation task should be scheduled.
				expectedContinuationPayload := proto.Clone(basePayload).(*taskspb.IngestWorkUnits)
				expectedContinuationPayload.PageToken = "continuation-token"
				expectedContinuationPayload.TaskIndex = basePayload.TaskIndex + 1 // TaskIndex increments
				verifyScheduledWorkUnitTask(t, skdr, expectedContinuationPayload)

				// A checkpoint should be created.
				expectedCheckpoints := []checkpoints.Checkpoint{
					{
						Key: checkpoints.Key{
							Project:    testProject,
							ResourceID: fmt.Sprintf("%s/%s", testResultDBHost, testRootInvocationID),
							ProcessID:  "work-unit-ingestion/schedule-continuation",
							Uniquifier: fmt.Sprintf("%v", basePayload.TaskIndex), // Uniquifier is current TaskIndex
						},
					},
				}
				verifyCheckpoints(ctx, t, expectedCheckpoints)
			})

			t.Run(`last page`, func(t *ftt.Test) {
				setupGetRootInvocationMock()
				setupQueryWorkUnitsMock(func(res *rdbpb.QueryWorkUnitsResponse) {
					res.NextPageToken = ""
				})

				err := ingester.run(ctx, basePayload)
				assert.Loosely(t, err, should.BeNil)
				// No continuation task should be scheduled.
				assert.Loosely(t, skdr.Tasks().Payloads(), should.BeEmpty)
				// No checkpoints should be created.
				verifyCheckpoints(ctx, t, []checkpoints.Checkpoint{})
			})

			t.Run(`Retry Task After Continuation Task Already Created`, func(t *ftt.Test) {
				setupGetRootInvocationMock()
				setupQueryWorkUnitsMock()

				// Pre-populate a checkpoint, simulating a previous (failed) attempt
				// that successfully scheduled the continuation task.
				existingCheckpoint := checkpoints.Checkpoint{
					Key: checkpoints.Key{
						Project:    testProject,
						ResourceID: fmt.Sprintf("%s/%s", testResultDBHost, testRootInvocationID),
						ProcessID:  "work-unit-ingestion/schedule-continuation",
						Uniquifier: fmt.Sprintf("%v", basePayload.TaskIndex),
					},
					ExpiryTime: time.Now().Add(checkpointTTL),
				}
				err := checkpoints.SetForTesting(ctx, t, existingCheckpoint)
				assert.Loosely(t, err, should.BeNil)

				err = ingester.run(ctx, basePayload)
				assert.Loosely(t, err, should.BeNil)
				// No new continuation task should be scheduled because the checkpoint exists.
				assert.Loosely(t, skdr.Tasks().Payloads(), should.BeEmpty)
				// The checkpoint should still exist.
				verifyCheckpoints(ctx, t, []checkpoints.Checkpoint{existingCheckpoint})
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
			})
		})

		t.Run(`Invalid payload`, func(t *ftt.Test) {
			t.Run(`ResultDB host missing`, func(t *ftt.Test) {
				payload := proto.Clone(basePayload).(*taskspb.IngestWorkUnits)
				payload.ResultdbHost = ""

				err := ingester.run(ctx, payload)
				assert.Loosely(t, err, should.ErrLike("validate payload: resultdb host is required"))
			})

			t.Run(`TaskIndex is zero`, func(t *ftt.Test) {
				payload := proto.Clone(basePayload).(*taskspb.IngestWorkUnits)
				payload.TaskIndex = 0

				err := ingester.run(ctx, payload)
				assert.Loosely(t, err, should.ErrLike("validate payload: task index must be positive"))
			})

			t.Run(`RootInvocation missing`, func(t *ftt.Test) {
				payload := proto.Clone(basePayload).(*taskspb.IngestWorkUnits)
				payload.RootInvocation = ""

				err := ingester.run(ctx, payload)
				assert.Loosely(t, err, should.ErrLike("validate payload: root invocation name"))
			})

			t.Run(`Realm missing`, func(t *ftt.Test) {
				payload := proto.Clone(basePayload).(*taskspb.IngestWorkUnits)
				payload.Realm = ""

				err := ingester.run(ctx, payload)
				assert.Loosely(t, err, should.ErrLike("validate payload: realm"))
			})
		})
	})
}

// verifyScheduledWorkUnitTask is a helper to check if the correct IngestWorkUnits task was scheduled.
func verifyScheduledWorkUnitTask(t testing.TB, skdr *tqtesting.Scheduler, expectedPayload *taskspb.IngestWorkUnits) {
	t.Helper()
	count := 0
	for _, pl := range skdr.Tasks().Payloads() {
		if actualPayload, ok := pl.(*taskspb.IngestWorkUnits); ok {
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
