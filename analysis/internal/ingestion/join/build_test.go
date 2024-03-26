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

package join

import (
	"fmt"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"google.golang.org/protobuf/types/known/timestamppb"

	resultpb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/server/tq"

	controlpb "go.chromium.org/luci/analysis/internal/ingestion/control/proto"
	"go.chromium.org/luci/analysis/internal/resultdb"
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

		createTime := time.Now()

		build := newBuildBuilder(14141414).
			WithCreateTime(createTime)

		expectedTask := &taskspb.IngestTestVerdicts{
			PartitionTime: timestamppb.New(createTime),
			Build:         build.ExpectedResult(),
		}

		assertTasksExpected := func() {
			So(len(skdr.Tasks().Payloads()), ShouldEqual, 1)
			resultsTask := skdr.Tasks().Payloads()[0].(*taskspb.IngestTestVerdicts)
			So(resultsTask, ShouldResembleProto, expectedTask)
		}

		Convey(`With vanilla build`, func() {
			Convey(`Vanilla CI Build`, func() {
				build = build.WithGardenerRotations([]string{"rotation1", "rotation2"})

				// Process build.
				So(ingestBuild(ctx, build), ShouldBeNil)

				assertTasksExpected()

				// Test repeated processing does not lead to further
				// ingestion tasks.
				So(ingestBuild(ctx, build), ShouldBeNil)

				assertTasksExpected()
			})
			Convey(`Unusual CI Build`, func() {
				// v8 project had some buildbucket-triggered builds
				// with both the user_agent:cq and user_agent:recipe tags.
				// These should be treated as CI builds.
				build = build.WithTags([]string{"user_agent:cq", "user_agent:recipe"})
				expectedTask.Build = build.ExpectedResult()

				// Process build.
				So(ingestBuild(ctx, build), ShouldBeNil)
				assertTasksExpected()
			})
		})
		Convey(`With ancestor build`, func() {
			build = build.WithAncestorIDs([]int64{01010101, 57575757})
			expectedTask.Build = build.ExpectedResult()

			ctl := gomock.NewController(t)
			defer ctl.Finish()

			mrc := resultdb.NewMockedClient(ctx, ctl)
			ctx := mrc.Ctx

			Convey(`Without invocation`, func() {
				build = build.WithContainedByAncestor(false)
				expectedTask.Build = build.ExpectedResult()

				// Process build.
				So(ingestBuild(ctx, build), ShouldBeNil)
				assertTasksExpected()
			})
			Convey(`With invocation`, func() {
				So(ingestFinalization(ctx, build.buildID), ShouldBeNil)
				So(len(skdr.Tasks().Payloads()), ShouldEqual, 0)

				Convey(`Not contained by ancestor`, func() {
					build = build.WithInvocation().WithContainedByAncestor(false)
					expectedTask.Build = build.ExpectedResult()

					req := &resultpb.GetInvocationRequest{
						Name: "invocations/build-57575757",
					}
					res := &resultpb.Invocation{
						Name:                "invocations/build-57575757",
						IncludedInvocations: []string{"invocations/build-unrelated"},
					}
					mrc.GetInvocation(req, res)

					// Process build.
					So(ingestBuild(ctx, build), ShouldBeNil)
					assertTasksExpected()
				})
				Convey(`Contained by ancestor`, func() {
					build = build.WithInvocation().WithContainedByAncestor(true)
					expectedTask.Build = build.ExpectedResult()

					req := &resultpb.GetInvocationRequest{
						Name: "invocations/build-57575757",
					}
					res := &resultpb.Invocation{
						Name: "invocations/build-57575757",
						IncludedInvocations: []string{
							"invocations/build-unrelated",
							fmt.Sprintf("invocations/build-%v", build.buildID),
						},
					}
					mrc.GetInvocation(req, res)

					// Process build.
					So(ingestBuild(ctx, build), ShouldBeNil)
					assertTasksExpected()
				})
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
					Critical:     true,
				}
				assertTasksExpected()

				// Check metrics were reported correctly for this sequence.
				isPresubmit := true
				hasInvocation := false
				So(bbBuildInputCounter.Get(ctx, "buildproject", isPresubmit, hasInvocation), ShouldEqual, 1)
				So(bbBuildOutputCounter.Get(ctx, "buildproject", isPresubmit, hasInvocation), ShouldEqual, 1)
				So(cvBuildInputCounter.Get(ctx, "cvproject"), ShouldEqual, 2)
				So(cvBuildOutputCounter.Get(ctx, "cvproject"), ShouldEqual, 1)
				So(rdbBuildInputCounter.Get(ctx, "invproject"), ShouldEqual, 0)
				So(rdbBuildOutputCounter.Get(ctx, "invproject"), ShouldEqual, 0)
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
				So(ingestFinalization(ctx, build.buildID), ShouldBeNil)

				So(len(skdr.Tasks().Payloads()), ShouldEqual, 0)

				So(ingestBuild(ctx, build), ShouldBeNil)

				assertTasksExpected()

				// Check metrics were reported correctly for this sequence.
				isPresubmit := false
				hasInvocation := true
				So(bbBuildInputCounter.Get(ctx, "buildproject", isPresubmit, hasInvocation), ShouldEqual, 1)
				So(bbBuildOutputCounter.Get(ctx, "buildproject", isPresubmit, hasInvocation), ShouldEqual, 1)
				So(cvBuildInputCounter.Get(ctx, "cvproject"), ShouldEqual, 0)
				So(cvBuildOutputCounter.Get(ctx, "cvproject"), ShouldEqual, 0)
				So(rdbBuildInputCounter.Get(ctx, "invproject"), ShouldEqual, 1)
				So(rdbBuildOutputCounter.Get(ctx, "invproject"), ShouldEqual, 1)
			})
		})
	})
}
