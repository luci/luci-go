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

package join

import (
	"fmt"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/analysis/internal/ingestion/control"
	controlpb "go.chromium.org/luci/analysis/internal/ingestion/control/proto"
	"go.chromium.org/luci/analysis/internal/tasks/taskspb"
	"go.chromium.org/luci/analysis/internal/testutil"
	pb "go.chromium.org/luci/analysis/proto/v1"

	_ "go.chromium.org/luci/analysis/internal/services/verdictingester" // Needed to ensure task class is registered.
)

func TestHandleInvocationFinalization(t *testing.T) {
	ftt.Run(`Test InvocationFinalizedPubSubHandler`, t, func(t *ftt.Test) {
		ctx := testutil.IntegrationTestContext(t)
		ctx, skdr := tq.TestingContext(ctx, nil)

		assertTasksExpected := func(expectedTask *taskspb.IngestTestVerdicts) {
			// assert ingestion task has been scheduled.
			// Verify exactly one ingestion has been created.
			assert.Loosely(t, len(skdr.Tasks().Payloads()), should.Equal(1))
			resultsTask := skdr.Tasks().Payloads()[0].(*taskspb.IngestTestVerdicts)
			assert.Loosely(t, resultsTask, should.Match(expectedTask))
		}
		t.Run(`does not have buildbucket build`, func(t *ftt.Test) {
			invocationCreateTime := time.Date(2024, time.December, 11, 10, 9, 8, 7, time.UTC)
			assert.Loosely(t, ingestFinalization(ctx, "inv-123", true, invocationCreateTime), should.BeNil)

			// Check metrics were reported correctly for this sequence.
			isPresubmit := false
			hasInvocation := true
			hasBuildBucketBuild := false
			assert.Loosely(t, bbBuildInputCounter.Get(ctx, "buildproject", isPresubmit, hasInvocation), should.BeZero)
			assert.Loosely(t, bbBuildOutputCounter.Get(ctx, "buildproject", isPresubmit, hasInvocation), should.BeZero)
			assert.Loosely(t, cvBuildInputCounter.Get(ctx, "cvproject"), should.BeZero)
			assert.Loosely(t, cvBuildOutputCounter.Get(ctx, "cvproject"), should.BeZero)
			assert.Loosely(t, rdbInvocationsInputCounter.Get(ctx, "buildproject", hasBuildBucketBuild), should.Equal(1))
			assert.Loosely(t, rdbInvocationsOutputCounter.Get(ctx, "buildproject", hasBuildBucketBuild), should.Equal(1))
			// Task has been scheduled.
			expectedTask := &taskspb.IngestTestVerdicts{
				PartitionTime: timestamppb.New(invocationCreateTime),
				IngestionId:   string(control.IngestionIDFromInvocationID("inv-123")),
				Project:       "buildproject",
				Invocation: &controlpb.InvocationResult{
					ResultdbHost: rdbHost,
					InvocationId: "inv-123",
					CreationTime: timestamppb.New(invocationCreateTime),
				},
				UseNewIngestionOrder: true,
			}
			assertTasksExpected(expectedTask)
		})
		t.Run(`has buildbucket build`, func(t *ftt.Test) {
			invocationCreateTime := time.Date(2024, time.December, 11, 10, 9, 8, 7, time.UTC)

			// Buildbucket timestamps are only in microsecond precision.
			ts := time.Now().Truncate(time.Nanosecond * 1000)
			build := newBuildBuilder(6363636363).
				WithCreateTime(ts).
				WithInvocation()
			invocationID := fmt.Sprintf("build-%d", build.buildID)
			expectedTask := &taskspb.IngestTestVerdicts{
				PartitionTime: timestamppb.New(invocationCreateTime),
				Build:         build.ExpectedResult(),
				IngestionId:   string(control.IngestionIDFromInvocationID(invocationID)),
				Project:       "buildproject",
				Invocation: &controlpb.InvocationResult{
					ResultdbHost: rdbHost,
					InvocationId: invocationID,
					CreationTime: timestamppb.New(invocationCreateTime),
				},
				UseNewIngestionOrder: true,
			}
			t.Run(`Without build ingested previously`, func(t *ftt.Test) {
				assert.Loosely(t, ingestFinalization(ctx, invocationID, false, invocationCreateTime), should.BeNil)

				assert.Loosely(t, len(skdr.Tasks().Payloads()), should.BeZero)
			})
			t.Run(`With non-presubmit build ingested previously`, func(t *ftt.Test) {
				assert.Loosely(t, ingestBuild(ctx, build), should.BeNil)

				assert.Loosely(t, len(skdr.Tasks().Payloads()), should.BeZero)

				assert.Loosely(t, ingestFinalization(ctx, invocationID, false, invocationCreateTime), should.BeNil)

				assertTasksExpected(expectedTask)

				// Repeated messages should not trigger new ingestions.
				assert.Loosely(t, ingestFinalization(ctx, invocationID, false, invocationCreateTime), should.BeNil)

				assertTasksExpected(expectedTask)
			})
			t.Run(`With presubmit build ingested previously`, func(t *ftt.Test) {
				build = build.WithTags([]string{"user_agent:cq"})
				expectedTask.Build = build.ExpectedResult()
				assert.Loosely(t, ingestBuild(ctx, build), should.BeNil)

				assert.Loosely(t, len(skdr.Tasks().Payloads()), should.BeZero)

				t.Run(`With LUCI CV run ingested previously`, func(t *ftt.Test) {
					assert.Loosely(t, ingestCVRun(ctx, []int64{build.buildID}), should.BeNil)

					expectedTask.PresubmitRun = &controlpb.PresubmitResult{
						PresubmitRunId: &pb.PresubmitRunId{
							System: "luci-cv",
							Id:     "cvproject/123e4567-e89b-12d3-a456-426614174000",
						},
						Status:       pb.PresubmitRunStatus_PRESUBMIT_RUN_STATUS_FAILED,
						Owner:        "automation",
						Mode:         pb.PresubmitRunMode_FULL_RUN,
						CreationTime: timestamppb.New(cvCreateTime),
					}

					assert.Loosely(t, len(skdr.Tasks().Payloads()), should.BeZero)

					assert.Loosely(t, ingestFinalization(ctx, invocationID, false, invocationCreateTime), should.BeNil)

					assertTasksExpected(expectedTask)

					// Check metrics were reported correctly for this sequence.
					isPresubmit := true
					hasInvocation := true
					hasBuildBucketBuild := true
					assert.Loosely(t, bbBuildInputCounter.Get(ctx, "buildproject", isPresubmit, hasInvocation), should.Equal(1))
					assert.Loosely(t, bbBuildOutputCounter.Get(ctx, "buildproject", isPresubmit, hasInvocation), should.Equal(1))
					assert.Loosely(t, cvBuildInputCounter.Get(ctx, "cvproject"), should.Equal(1))
					assert.Loosely(t, cvBuildOutputCounter.Get(ctx, "cvproject"), should.Equal(1))
					assert.Loosely(t, rdbInvocationsInputCounter.Get(ctx, "buildproject", hasBuildBucketBuild), should.Equal(1))
					assert.Loosely(t, rdbInvocationsOutputCounter.Get(ctx, "buildproject", hasBuildBucketBuild), should.Equal(1))
				})
				t.Run(`Without LUCI CV run ingested previously`, func(t *ftt.Test) {
					assert.Loosely(t, ingestFinalization(ctx, invocationID, false, invocationCreateTime), should.BeNil)

					assert.Loosely(t, len(skdr.Tasks().Payloads()), should.BeZero)
				})
			})
		})
	})
}
