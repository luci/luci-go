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

package app

import (
	"encoding/json"
	"io"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	bbv1 "go.chromium.org/luci/common/api/buildbucket/buildbucket/v1"
	. "go.chromium.org/luci/common/testing/assertions"
	"go.chromium.org/luci/server/tq"
	"google.golang.org/protobuf/types/known/timestamppb"

	controlpb "go.chromium.org/luci/analysis/internal/ingestion/control/proto"
	_ "go.chromium.org/luci/analysis/internal/services/resultingester" // Needed to ensure task class is registered.
	"go.chromium.org/luci/analysis/internal/tasks/taskspb"
	"go.chromium.org/luci/analysis/internal/testutil"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

func TestHandleBuild(t *testing.T) {
	Convey(`Test BuildbucketPubSubHandler`, t, func() {
		ctx := testutil.IntegrationTestContext(t)
		ctx, skdr := tq.TestingContext(ctx, nil)

		// Buildbucket timestamps are only in microsecond precision.
		t := time.Now().Truncate(time.Nanosecond * 1000)

		build := newBuildBuilder(14141414).
			WithCreateTime(t)

		expectedTask := &taskspb.IngestTestResults{
			PartitionTime: timestamppb.New(t),
			Build: &controlpb.BuildResult{
				Host:         bbHost,
				Id:           14141414,
				CreationTime: timestamppb.New(t),
				Project:      "buildproject",
			},
		}

		assertTasksExpected := func() {
			So(len(skdr.Tasks().Payloads()), ShouldEqual, 1)
			resultsTask := skdr.Tasks().Payloads()[0].(*taskspb.IngestTestResults)
			So(resultsTask, ShouldResembleProto, expectedTask)
		}

		Convey(`With vanilla build`, func() {
			Convey(`Vanilla CI Build`, func() {
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

				// Process build.
				So(ingestBuild(ctx, build), ShouldBeNil)
				assertTasksExpected()
			})
		})
		Convey(`With build that is part of a LUCI CV run`, func() {
			build := build.WithTags([]string{"user_agent:cq"})

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

func makeBBReq(build bbv1.LegacyApiCommonBuildMessage, hostname string) io.ReadCloser {
	bmsg := struct {
		Build    bbv1.LegacyApiCommonBuildMessage `json:"build"`
		Hostname string                           `json:"hostname"`
	}{build, hostname}
	bm, _ := json.Marshal(bmsg)
	return makeReq(bm)
}
