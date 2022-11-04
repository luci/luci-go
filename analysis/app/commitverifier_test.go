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
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"go.chromium.org/luci/common/clock"
	. "go.chromium.org/luci/common/testing/assertions"
	cvv0 "go.chromium.org/luci/cv/api/v0"
	cvv1 "go.chromium.org/luci/cv/api/v1"
	"go.chromium.org/luci/server/tq"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/analysis/internal/cv"
	controlpb "go.chromium.org/luci/analysis/internal/ingestion/control/proto"
	_ "go.chromium.org/luci/analysis/internal/services/resultingester" // Needed to ensure task class is registered.
	"go.chromium.org/luci/analysis/internal/tasks/taskspb"
	"go.chromium.org/luci/analysis/internal/testutil"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

// bbCreateTime is the create time assigned to buildbucket builds, for testing.
// Must be in microsecond precision as that is the precision of buildbucket.
var bbCreateTime = time.Date(2025, time.December, 1, 2, 3, 4, 5000, time.UTC)

func TestHandleCVRun(t *testing.T) {
	Convey(`Test CVRunPubSubHandler`, t, func() {
		ctx := testutil.IntegrationTestContext(t)
		ctx, skdr := tq.TestingContext(ctx, nil)

		// Setup two ingested tryjob builds. The first has
		// an invocation, the second does not.
		buildIDs := []int64{87654321, 87654322}
		buildOne := newBuildBuilder(buildIDs[0]).
			WithCreateTime(bbCreateTime).
			WithTags([]string{"user_agent:cq"}).
			WithInvocation()
		buildTwo := newBuildBuilder(buildIDs[1]).
			WithCreateTime(bbCreateTime).
			WithTags([]string{"user_agent:cq"})
		So(ingestBuild(ctx, buildOne), ShouldBeNil)
		So(ingestBuild(ctx, buildTwo), ShouldBeNil)

		// Ingest the invocation finalization.
		So(ingestFinalization(ctx, buildOne.buildID), ShouldBeNil)

		So(len(skdr.Tasks().Payloads()), ShouldEqual, 0)

		Convey(`CV run is processed`, func() {
			ctx, skdr := tq.TestingContext(ctx, nil)
			rID := "id_full_run"
			fullRunID := fullRunID("cvproject", rID)

			processCVRun := func(run *cvv0.Run) (processed bool, tasks []*taskspb.IngestTestResults) {
				existingTaskCount := len(skdr.Tasks().Payloads())

				runs := map[string]*cvv0.Run{
					fullRunID: run,
				}
				ctx = cv.UseFakeClient(ctx, runs)
				r := &http.Request{Body: makeCVRunReq(fullRunID)}
				project, processed, err := cvPubSubHandlerImpl(ctx, r)
				So(err, ShouldBeNil)
				So(project, ShouldEqual, "cvproject")

				tasks = make([]*taskspb.IngestTestResults, 0,
					len(skdr.Tasks().Payloads())-existingTaskCount)
				for _, pl := range skdr.Tasks().Payloads()[existingTaskCount:] {
					switch pl.(type) {
					case *taskspb.IngestTestResults:
						tasks = append(tasks, pl.(*taskspb.IngestTestResults))
					default:
						panic("unexpected task type")
					}
				}
				return processed, tasks
			}

			run := &cvv0.Run{
				Id:         fullRunID,
				Mode:       "FULL_RUN",
				CreateTime: timestamppb.New(clock.Now(ctx)),
				Owner:      "cl-owner@google.com",
				Tryjobs: []*cvv0.Tryjob{
					tryjob(buildIDs[0]),
					tryjob(2), // This build has not been ingested yet.
					tryjob(buildIDs[1]),
				},
				Status: cvv0.Run_SUCCEEDED,
			}
			expectedTaskTemplate := &taskspb.IngestTestResults{
				PartitionTime: run.CreateTime,
				PresubmitRun: &controlpb.PresubmitResult{
					PresubmitRunId: &pb.PresubmitRunId{
						System: "luci-cv",
						Id:     "cvproject/" + strings.Split(run.Id, "/")[3],
					},
					Status:       pb.PresubmitRunStatus_PRESUBMIT_RUN_STATUS_SUCCEEDED,
					Mode:         pb.PresubmitRunMode_FULL_RUN,
					Owner:        "user",
					CreationTime: run.CreateTime,
				},
			}
			Convey(`Baseline`, func() {
				processed, tasks := processCVRun(run)
				So(processed, ShouldBeTrue)
				So(sortTasks(tasks), ShouldResembleProto,
					sortTasks(expectedTasks(expectedTaskTemplate, buildIDs)))

				Convey(`Re-processing CV run should not result in further ingestion tasks`, func() {
					processed, tasks = processCVRun(run)
					So(processed, ShouldBeTrue)
					So(tasks, ShouldBeEmpty)
				})
			})
			Convey(`Dry run`, func() {
				run.Mode = "DRY_RUN"
				expectedTaskTemplate.PresubmitRun.Mode = pb.PresubmitRunMode_DRY_RUN

				processed, tasks := processCVRun(run)
				So(processed, ShouldBeTrue)
				So(sortTasks(tasks), ShouldResembleProto,
					sortTasks(expectedTasks(expectedTaskTemplate, buildIDs)))
			})
			Convey(`New patchset run`, func() {
				run.Mode = "NEW_PATCHSET_RUN"
				expectedTaskTemplate.PresubmitRun.Mode = pb.PresubmitRunMode_NEW_PATCHSET_RUN

				processed, tasks := processCVRun(run)
				So(processed, ShouldBeTrue)
				So(sortTasks(tasks), ShouldResembleProto,
					sortTasks(expectedTasks(expectedTaskTemplate, buildIDs)))
			})
			Convey(`CV Run owned by Automation`, func() {
				run.Owner = "chromium-autoroll@skia-public.iam.gserviceaccount.com"
				expectedTaskTemplate.PresubmitRun.Owner = "automation"

				processed, tasks := processCVRun(run)
				So(processed, ShouldBeTrue)
				So(sortTasks(tasks), ShouldResembleProto,
					sortTasks(expectedTasks(expectedTaskTemplate, buildIDs)))
			})
			Convey(`CV Run owned by Automation 2`, func() {
				run.Owner = "3su6n15k.default@developer.gserviceaccount.com"
				expectedTaskTemplate.PresubmitRun.Owner = "automation"

				processed, tasks := processCVRun(run)
				So(processed, ShouldBeTrue)
				So(sortTasks(tasks), ShouldResembleProto,
					sortTasks(expectedTasks(expectedTaskTemplate, buildIDs)))
			})
			Convey(`With non-buildbucket tryjob`, func() {
				// Should be ignored.
				run.Tryjobs = append(run.Tryjobs, &cvv0.Tryjob{
					Result: &cvv0.Tryjob_Result{},
				})

				processed, tasks := processCVRun(run)
				So(processed, ShouldBeTrue)
				So(sortTasks(tasks), ShouldResembleProto,
					sortTasks(expectedTasks(expectedTaskTemplate, buildIDs)))
			})
			Convey(`With re-used tryjob`, func() {
				// Assume that this tryjob was created by another CV run,
				// so should not be ingested with this CV run.
				run.Tryjobs[0].Reuse = true

				processed, tasks := processCVRun(run)
				So(processed, ShouldBeTrue)
				So(sortTasks(tasks), ShouldResembleProto,
					sortTasks(expectedTasks(expectedTaskTemplate, buildIDs[1:])))
			})
			Convey(`Failing Run`, func() {
				run.Status = cvv0.Run_FAILED
				expectedTaskTemplate.PresubmitRun.Status = pb.PresubmitRunStatus_PRESUBMIT_RUN_STATUS_FAILED

				processed, tasks := processCVRun(run)
				So(processed, ShouldBeTrue)
				So(sortTasks(tasks), ShouldResembleProto,
					sortTasks(expectedTasks(expectedTaskTemplate, buildIDs)))
			})
			Convey(`Cancelled Run`, func() {
				run.Status = cvv0.Run_CANCELLED
				expectedTaskTemplate.PresubmitRun.Status = pb.PresubmitRunStatus_PRESUBMIT_RUN_STATUS_CANCELED

				processed, tasks := processCVRun(run)
				So(processed, ShouldBeTrue)
				So(sortTasks(tasks), ShouldResembleProto,
					sortTasks(expectedTasks(expectedTaskTemplate, buildIDs)))
			})
		})
	})
}

func makeCVRunReq(runID string) io.ReadCloser {
	blob, _ := protojson.Marshal(&cvv1.PubSubRun{
		Id:       runID,
		Status:   cvv1.Run_SUCCEEDED,
		Hostname: "cvhost",
	})
	return makeReq(blob)
}

func tryjob(bID int64) *cvv0.Tryjob {
	return &cvv0.Tryjob{
		Result: &cvv0.Tryjob_Result{
			Backend: &cvv0.Tryjob_Result_Buildbucket_{
				Buildbucket: &cvv0.Tryjob_Result_Buildbucket{
					Id: int64(bID),
				},
			},
		},
		Critical: (bID % 2) == 0,
	}
}

func fullRunID(project, runID string) string {
	return fmt.Sprintf("projects/%s/runs/%s", project, runID)
}

func expectedTasks(taskTemplate *taskspb.IngestTestResults, buildIDs []int64) []*taskspb.IngestTestResults {
	res := make([]*taskspb.IngestTestResults, 0, len(buildIDs))
	for _, buildID := range buildIDs {
		t := proto.Clone(taskTemplate).(*taskspb.IngestTestResults)
		t.PresubmitRun.Critical = ((buildID % 2) == 0)
		t.Build = &controlpb.BuildResult{
			Host:         bbHost,
			Id:           buildID,
			CreationTime: timestamppb.New(bbCreateTime),
			Project:      "buildproject",
		}
		res = append(res, t)
	}
	return res
}

func sortTasks(tasks []*taskspb.IngestTestResults) []*taskspb.IngestTestResults {
	sort.Slice(tasks, func(i, j int) bool { return tasks[i].Build.Id < tasks[j].Build.Id })
	return tasks
}
