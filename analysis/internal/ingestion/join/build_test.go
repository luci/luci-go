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

func TestHandleBuild(t *testing.T) {
	Convey(`Test JoinBuild`, t, func() {
		ctx := testutil.IntegrationTestContext(t)
		ctx, skdr := tq.TestingContext(ctx, nil)

		buildCreateTime := time.Now()

		build := newBuildBuilder(14141414).
			WithCreateTime(buildCreateTime)

		expectedTask := &taskspb.IngestTestVerdicts{
			PartitionTime: timestamppb.New(buildCreateTime),
			Build:         build.ExpectedResult(),
			IngestionId:   fmt.Sprintf("%s/build-%d", rdbHost, build.buildID),
			Project:       "buildproject",
		}

		assertTasksExpected := func() string {
			// assert ingestion task has been scheduled.
			So(len(skdr.Tasks().Payloads()), ShouldEqual, 1)
			resultsTask := skdr.Tasks().Payloads()[0].(*taskspb.IngestTestVerdicts)
			return ShouldResembleProto(resultsTask, expectedTask)
		}

		Convey(`With vanilla build`, func() {
			Convey(`Vanilla CI Build`, func() {
				build = build.WithGardenerRotations([]string{"rotation1", "rotation2"})

				// Process build.
				So(ingestBuild(ctx, build), ShouldBeNil)

				So(assertTasksExpected(), ShouldBeEmpty)

				// Test repeated processing does not lead to further
				// ingestion tasks.
				So(ingestBuild(ctx, build), ShouldBeNil)

				So(assertTasksExpected(), ShouldBeEmpty)
			})
			Convey(`Unusual CI Build`, func() {
				// v8 project had some buildbucket-triggered builds
				// with both the user_agent:cq and user_agent:recipe tags.
				// These should be treated as CI builds.
				build = build.WithTags([]string{"user_agent:cq", "user_agent:recipe"})
				expectedTask.Build = build.ExpectedResult()

				// Process build.
				So(ingestBuild(ctx, build), ShouldBeNil)
				So(assertTasksExpected(), ShouldBeEmpty)
			})
		})
		Convey(`With build that is part of a LUCI CV run`, func() {
			build := build.WithTags([]string{"user_agent:cq"})
			expectedTask.Build = build.ExpectedResult()

			Convey(`Without LUCI CV run processed previously`, func() {
				So(ingestBuild(ctx, build), ShouldBeNil)

				So(len(skdr.Tasks().Payloads()), ShouldEqual, 0)
			})
			Convey(`With LUCI CV run processed previously`, func() {
				So(ingestCVRun(ctx, []int64{11111171, build.buildID}), ShouldBeNil)

				So(len(skdr.Tasks().Payloads()), ShouldEqual, 0)

				So(ingestBuild(ctx, build), ShouldBeNil)

				expectedTask.PresubmitRun = &controlpb.PresubmitResult{
					PresubmitRunId: &pb.PresubmitRunId{
						System: "luci-cv",
						Id:     "cvproject/123e4567-e89b-12d3-a456-426614174000",
					},
					Status:       pb.PresubmitRunStatus_PRESUBMIT_RUN_STATUS_FAILED,
					Owner:        "automation",
					Mode:         pb.PresubmitRunMode_FULL_RUN,
					CreationTime: timestamppb.New(cvCreateTime),
					Critical:     true,
				}
				So(assertTasksExpected(), ShouldBeEmpty)

				// Check metrics were reported correctly for this sequence.
				isPresubmit := true
				hasInvocation := false
				hasBuildBucketBuild := true
				So(bbBuildInputCounter.Get(ctx, "buildproject", isPresubmit, hasInvocation), ShouldEqual, 1)
				So(bbBuildOutputCounter.Get(ctx, "buildproject", isPresubmit, hasInvocation), ShouldEqual, 1)
				So(cvBuildInputCounter.Get(ctx, "cvproject"), ShouldEqual, 2)
				So(cvBuildOutputCounter.Get(ctx, "cvproject"), ShouldEqual, 1)
				So(rdbInvocationsInputCounter.Get(ctx, "buildproject", hasBuildBucketBuild), ShouldEqual, 0)
				So(rdbInvocationsOutputCounter.Get(ctx, "buildproject", hasBuildBucketBuild), ShouldEqual, 0)
			})
		})
		Convey(`With build that has a ResultDB invocation`, func() {
			build := build.WithInvocation()
			expectedTask.Build = build.ExpectedResult()

			Convey(`Without invocation finalization acknowledged previously`, func() {
				So(ingestBuild(ctx, build), ShouldBeNil)

				So(len(skdr.Tasks().Payloads()), ShouldEqual, 0)
			})
			Convey(`With invocation finalization acknowledged previously`, func() {
				invocationCreateTime := time.Date(2024, time.December, 11, 10, 9, 8, 7, time.UTC)
				So(ingestFinalization(ctx, fmt.Sprintf("build-%d", build.buildID), false, invocationCreateTime), ShouldBeNil)

				So(len(skdr.Tasks().Payloads()), ShouldEqual, 0)

				So(ingestBuild(ctx, build), ShouldBeNil)

				expectedTask.Invocation = &controlpb.InvocationResult{
					ResultdbHost: rdbHost,
					InvocationId: fmt.Sprintf("build-%d", build.buildID),
					CreationTime: timestamppb.New(invocationCreateTime),
				}
				expectedTask.PartitionTime = timestamppb.New(invocationCreateTime)
				So(assertTasksExpected(), ShouldBeEmpty)

				// Check metrics were reported correctly for this sequence.
				isPresubmit := false
				hasInvocation := true
				hasBuildBucketBuild := true
				So(bbBuildInputCounter.Get(ctx, "buildproject", isPresubmit, hasInvocation), ShouldEqual, 1)
				So(bbBuildOutputCounter.Get(ctx, "buildproject", isPresubmit, hasInvocation), ShouldEqual, 1)
				So(cvBuildInputCounter.Get(ctx, "cvproject"), ShouldEqual, 0)
				So(cvBuildOutputCounter.Get(ctx, "cvproject"), ShouldEqual, 0)
				So(rdbInvocationsInputCounter.Get(ctx, "buildproject", hasBuildBucketBuild), ShouldEqual, 1)
				So(rdbInvocationsOutputCounter.Get(ctx, "buildproject", hasBuildBucketBuild), ShouldEqual, 1)
			})
		})
	})
}
