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
	"net/http"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	bbv1 "go.chromium.org/luci/common/api/buildbucket/buildbucket/v1"
	. "go.chromium.org/luci/common/testing/assertions"
	cvv0 "go.chromium.org/luci/cv/api/v0"
	"go.chromium.org/luci/server/tq"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/analysis/internal/cv"
	controlpb "go.chromium.org/luci/analysis/internal/ingestion/control/proto"
	_ "go.chromium.org/luci/analysis/internal/services/resultingester" // Needed to ensure task class is registered.
	"go.chromium.org/luci/analysis/internal/tasks/taskspb"
	"go.chromium.org/luci/analysis/internal/testutil"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

func TestHandleBuild(t *testing.T) {
	Convey(`With Spanner Test Database`, t, func() {
		ctx := testutil.SpannerTestContext(t)
		ctx, skdr := tq.TestingContext(ctx, nil)

		Convey(`Test BuildbucketPubSubHandler`, func() {
			Convey(`CI build is processed`, func() {
				// Buildbucket timestamps are only in microsecond precision.
				t := time.Now().Truncate(time.Nanosecond * 1000)

				buildExp := bbv1.LegacyApiCommonBuildMessage{
					Project:   "buildproject",
					Bucket:    "luci.buildproject.bucket",
					Id:        87654321,
					Status:    bbv1.StatusCompleted,
					CreatedTs: bbv1.FormatTimestamp(t),
				}

				test := func() {
					r := &http.Request{Body: makeBBReq(buildExp, "bb-hostname")}
					project, processed, err := bbPubSubHandlerImpl(ctx, r)
					So(err, ShouldBeNil)
					So(processed, ShouldBeTrue)
					So(project, ShouldEqual, "buildproject")

					So(len(skdr.Tasks().Payloads()), ShouldEqual, 1)
					resultsTask := skdr.Tasks().Payloads()[0].(*taskspb.IngestTestResults)
					So(resultsTask, ShouldResembleProto, &taskspb.IngestTestResults{
						PartitionTime: timestamppb.New(t),
						Build: &controlpb.BuildResult{
							Host:         "bb-hostname",
							Id:           87654321,
							CreationTime: timestamppb.New(t),
							Project:      "buildproject",
						},
					})
				}
				Convey(`Standard CI Build`, func() {
					test()

					// Test repeated processing does not lead to further
					// ingestion tasks.
					test()
				})
				Convey(`Unusual CI Build`, func() {
					// v8 project had some buildbucket-triggered builds
					// with both the user_agent:cq and user_agent:recipe tags.
					// These should be treated as CI builds.
					buildExp.Tags = []string{"user_agent:cq", "user_agent:recipe"}
					test()
				})
			})

			Convey(`Try build is processed`, func() {
				t := time.Date(2025, time.April, 1, 2, 3, 4, 0, time.UTC)

				buildExp := bbv1.LegacyApiCommonBuildMessage{
					Project:   "buildproject",
					Bucket:    "luci.buildproject.bucket",
					Id:        14141414,
					Status:    bbv1.StatusCompleted,
					CreatedTs: bbv1.FormatTimestamp(t),
					Tags:      []string{"user_agent:cq"},
				}

				Convey(`With presubmit run processed previously`, func() {
					partitionTime := time.Now()
					run := &cvv0.Run{
						Id:         "projects/cvproject/runs/123e4567-e89b-12d3-a456-426614174000",
						Mode:       "FULL_RUN",
						Status:     cvv0.Run_FAILED,
						Owner:      "chromium-autoroll@skia-public.iam.gserviceaccount.com",
						CreateTime: timestamppb.New(partitionTime),
						Tryjobs: []*cvv0.Tryjob{
							tryjob(2),
							tryjob(14141414),
						},
						Cls: []*cvv0.GerritChange{
							{
								Host:     "chromium-review.googlesource.com",
								Change:   12345,
								Patchset: 1,
							},
						},
					}
					runs := map[string]*cvv0.Run{
						run.Id: run,
					}
					ctx = cv.UseFakeClient(ctx, runs)

					// Process presubmit run.
					r := &http.Request{Body: makeCVChromiumRunReq(run.Id)}
					project, processed, err := cvPubSubHandlerImpl(ctx, r)
					So(err, ShouldBeNil)
					So(processed, ShouldBeTrue)
					So(project, ShouldEqual, "cvproject")

					So(len(skdr.Tasks().Payloads()), ShouldEqual, 0)

					// Process build.
					r = &http.Request{Body: makeBBReq(buildExp, bbHost)}
					project, processed, err = bbPubSubHandlerImpl(ctx, r)
					So(err, ShouldBeNil)
					So(processed, ShouldBeTrue)
					So(project, ShouldEqual, "buildproject")

					So(len(skdr.Tasks().Payloads()), ShouldEqual, 1)
					task := skdr.Tasks().Payloads()[0].(*taskspb.IngestTestResults)
					So(task, ShouldResembleProto, &taskspb.IngestTestResults{
						PartitionTime: timestamppb.New(partitionTime),
						Build: &controlpb.BuildResult{
							Host:         bbHost,
							Id:           14141414,
							CreationTime: timestamppb.New(t),
							Project:      "buildproject",
						},
						PresubmitRun: &controlpb.PresubmitResult{
							PresubmitRunId: &pb.PresubmitRunId{
								System: "luci-cv",
								Id:     "cvproject/123e4567-e89b-12d3-a456-426614174000",
							},
							Status:       pb.PresubmitRunStatus_PRESUBMIT_RUN_STATUS_FAILED,
							Owner:        "automation",
							Mode:         pb.PresubmitRunMode_FULL_RUN,
							CreationTime: timestamppb.New(partitionTime),
							Critical:     true,
						},
					})
				})
				Convey(`Without presubmit run processed previously`, func() {
					r := &http.Request{Body: makeBBReq(buildExp, bbHost)}
					project, processed, err := bbPubSubHandlerImpl(ctx, r)
					So(err, ShouldBeNil)
					So(processed, ShouldBeTrue)
					So(project, ShouldEqual, "buildproject")
					So(len(skdr.Tasks().Payloads()), ShouldEqual, 0)
				})
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
