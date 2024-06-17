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

	"go.chromium.org/luci/server/tq"

	controlpb "go.chromium.org/luci/analysis/internal/ingestion/control/proto"
	"go.chromium.org/luci/analysis/internal/tasks/taskspb"
	"go.chromium.org/luci/analysis/internal/testutil"
	pb "go.chromium.org/luci/analysis/proto/v1"

	_ "go.chromium.org/luci/analysis/internal/services/verdictingester" // Needed to ensure task class is registered.

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestHandleInvocationFinalization(t *testing.T) {
	Convey(`Test InvocationFinalizedPubSubHandler`, t, func() {
		ctx := testutil.IntegrationTestContext(t)
		ctx, skdr := tq.TestingContext(ctx, nil)

		assertTasksExpected := func(expectedTask *taskspb.IngestTestVerdicts) {
			// assert ingestion task has been scheduled.
			// Verify exactly one ingestion has been created.
			So(len(skdr.Tasks().Payloads()), ShouldEqual, 1)
			resultsTask := skdr.Tasks().Payloads()[0].(*taskspb.IngestTestVerdicts)
			So(resultsTask, ShouldResembleProto, expectedTask)
		}
		Convey(`does not have buildbucket build`, func() {
			invocationCreateTime := time.Date(2024, time.December, 11, 10, 9, 8, 7, time.UTC)
			So(ingestFinalization(ctx, "inv-123", true, invocationCreateTime), ShouldBeNil)

			// Check metrics were reported correctly for this sequence.
			isPresubmit := false
			hasInvocation := true
			hasBuildBucketBuild := false
			So(bbBuildInputCounter.Get(ctx, "buildproject", isPresubmit, hasInvocation), ShouldEqual, 0)
			So(bbBuildOutputCounter.Get(ctx, "buildproject", isPresubmit, hasInvocation), ShouldEqual, 0)
			So(cvBuildInputCounter.Get(ctx, "cvproject"), ShouldEqual, 0)
			So(cvBuildOutputCounter.Get(ctx, "cvproject"), ShouldEqual, 0)
			So(rdbInvocationsInputCounter.Get(ctx, "buildproject", hasBuildBucketBuild), ShouldEqual, 1)
			So(rdbInvocationsOutputCounter.Get(ctx, "buildproject", hasBuildBucketBuild), ShouldEqual, 1)
			// Task has been scheduled.
			expectedTask := &taskspb.IngestTestVerdicts{
				PartitionTime: timestamppb.New(invocationCreateTime),
				IngestionId:   fmt.Sprintf("%s/inv-123", rdbHost),
				Project:       "buildproject",
				Invocation: &controlpb.InvocationResult{
					ResultdbHost: rdbHost,
					InvocationId: "inv-123",
					CreationTime: timestamppb.New(invocationCreateTime),
				},
			}
			assertTasksExpected(expectedTask)
		})
		Convey(`has buildbucket build`, func() {
			invocationCreateTime := time.Date(2024, time.December, 11, 10, 9, 8, 7, time.UTC)

			// Buildbucket timestamps are only in microsecond precision.
			t := time.Now().Truncate(time.Nanosecond * 1000)
			build := newBuildBuilder(6363636363).
				WithCreateTime(t).
				WithInvocation()
			invocationID := fmt.Sprintf("build-%d", build.buildID)
			expectedTask := &taskspb.IngestTestVerdicts{
				PartitionTime: timestamppb.New(invocationCreateTime),
				Build:         build.ExpectedResult(),
				IngestionId:   fmt.Sprintf("%s/%s", rdbHost, invocationID),
				Project:       "buildproject",
				Invocation: &controlpb.InvocationResult{
					ResultdbHost: rdbHost,
					InvocationId: invocationID,
					CreationTime: timestamppb.New(invocationCreateTime),
				},
			}
			Convey(`Without build ingested previously`, func() {
				So(ingestFinalization(ctx, invocationID, false, invocationCreateTime), ShouldBeNil)

				So(len(skdr.Tasks().Payloads()), ShouldEqual, 0)
			})
			Convey(`With non-presubmit build ingested previously`, func() {
				So(ingestBuild(ctx, build), ShouldBeNil)

				So(len(skdr.Tasks().Payloads()), ShouldEqual, 0)

				So(ingestFinalization(ctx, invocationID, false, invocationCreateTime), ShouldBeNil)

				assertTasksExpected(expectedTask)

				// Repeated messages should not trigger new ingestions.
				So(ingestFinalization(ctx, invocationID, false, invocationCreateTime), ShouldBeNil)

				assertTasksExpected(expectedTask)
			})
			Convey(`With presubmit build ingested previously`, func() {
				build = build.WithTags([]string{"user_agent:cq"})
				expectedTask.Build = build.ExpectedResult()
				So(ingestBuild(ctx, build), ShouldBeNil)

				So(len(skdr.Tasks().Payloads()), ShouldEqual, 0)

				Convey(`With LUCI CV run ingested previously`, func() {
					So(ingestCVRun(ctx, []int64{build.buildID}), ShouldBeNil)

					expectedTask.PartitionTime = timestamppb.New(cvCreateTime)
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

					So(len(skdr.Tasks().Payloads()), ShouldEqual, 0)

					So(ingestFinalization(ctx, invocationID, false, invocationCreateTime), ShouldBeNil)

					assertTasksExpected(expectedTask)

					// Check metrics were reported correctly for this sequence.
					isPresubmit := true
					hasInvocation := true
					hasBuildBucketBuild := true
					So(bbBuildInputCounter.Get(ctx, "buildproject", isPresubmit, hasInvocation), ShouldEqual, 1)
					So(bbBuildOutputCounter.Get(ctx, "buildproject", isPresubmit, hasInvocation), ShouldEqual, 1)
					So(cvBuildInputCounter.Get(ctx, "cvproject"), ShouldEqual, 1)
					So(cvBuildOutputCounter.Get(ctx, "cvproject"), ShouldEqual, 1)
					So(rdbInvocationsInputCounter.Get(ctx, "buildproject", hasBuildBucketBuild), ShouldEqual, 1)
					So(rdbInvocationsOutputCounter.Get(ctx, "buildproject", hasBuildBucketBuild), ShouldEqual, 1)
				})
				Convey(`Without LUCI CV run ingested previously`, func() {
					So(ingestFinalization(ctx, invocationID, false, invocationCreateTime), ShouldBeNil)

					So(len(skdr.Tasks().Payloads()), ShouldEqual, 0)
				})
			})
		})
	})
}
