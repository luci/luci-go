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
	"sort"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	cvv0 "go.chromium.org/luci/cv/api/v0"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/analysis/internal/cv"
	"go.chromium.org/luci/analysis/internal/ingestion/control"
	controlpb "go.chromium.org/luci/analysis/internal/ingestion/control/proto"
	"go.chromium.org/luci/analysis/internal/tasks/taskspb"
	"go.chromium.org/luci/analysis/internal/testutil"
	pb "go.chromium.org/luci/analysis/proto/v1"

	_ "go.chromium.org/luci/analysis/internal/services/verdictingester" // Needed to ensure task class is registered.
)

// bbCreateTime is the create time assigned to buildbucket builds, for testing.
// Must be in microsecond precision as that is the precision of buildbucket.
var bbCreateTime = time.Date(2025, time.December, 1, 2, 3, 4, 5000, time.UTC)

func TestHandleCVRun(t *testing.T) {
	ftt.Run(`Test JoinCVRun`, t, func(t *ftt.Test) {
		ctx := testutil.IntegrationTestContext(t)
		ctx, skdr := tq.TestingContext(ctx, nil)

		// Setup two ingested tryjob builds. The first has
		// an invocation, the second does not.
		buildOne := newBuildBuilder(87654321).
			WithCreateTime(bbCreateTime).
			WithTags([]string{"user_agent:cq"}).
			WithInvocation()
		buildTwo := newBuildBuilder(87654322).
			WithCreateTime(bbCreateTime).
			WithTags([]string{"user_agent:cq"})
		builds := []*buildBuilder{buildOne, buildTwo}
		assert.Loosely(t, ingestBuild(ctx, buildOne), should.BeNil)
		assert.Loosely(t, ingestBuild(ctx, buildTwo), should.BeNil)

		// Ingest the invocation finalization.
		invocationCreateTime := time.Date(2024, time.December, 11, 10, 9, 8, 7, time.UTC)
		assert.Loosely(t, ingestFinalization(ctx, fmt.Sprintf("build-%d", buildOne.buildID), false, invocationCreateTime), should.BeNil)

		assert.Loosely(t, len(skdr.Tasks().Payloads()), should.BeZero)

		t.Run(`CV run is processed`, func(t *ftt.Test) {
			ctx, skdr := tq.TestingContext(ctx, nil)
			rID := "id_full_run"
			fullRunID := fullRunID("cvproject", rID)

			processCVRun := func(run *cvv0.Run) (processed bool, tasks []*taskspb.IngestTestVerdicts) {
				existingTaskCount := len(skdr.Tasks().Payloads())

				runs := map[string]*cvv0.Run{
					fullRunID: run,
				}
				ctx = cv.UseFakeClient(ctx, runs)
				r := makeCVRunPubSub(fullRunID)
				project, processed, err := JoinCVRun(ctx, r)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, project, should.Equal("cvproject"))

				tasks = make([]*taskspb.IngestTestVerdicts, 0,
					len(skdr.Tasks().Payloads())-existingTaskCount)
				for _, pl := range skdr.Tasks().Payloads()[existingTaskCount:] {
					switch pl := pl.(type) {
					case *taskspb.IngestTestVerdicts:
						tasks = append(tasks, pl)
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
				TryjobInvocations: []*cvv0.TryjobInvocation{
					tryjob(buildOne.buildID),
					tryjob(2), // This build has not been ingested yet.
					tryjob(buildTwo.buildID),
				},
				Status: cvv0.Run_SUCCEEDED,
			}
			expectedTaskTemplate := &taskspb.IngestTestVerdicts{
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
				Project: "buildproject",
			}
			t.Run(`Baseline`, func(t *ftt.Test) {
				processed, tasks := processCVRun(run)
				assert.Loosely(t, processed, should.BeTrue)
				// assert ingestion task has been scheduled.
				assert.Loosely(t, sortTasks(tasks), should.Match(
					sortTasks(expectedTasks(expectedTaskTemplate, builds))))

				t.Run(`Re-processing CV run should not result in further ingestion tasks`, func(t *ftt.Test) {
					processed, tasks = processCVRun(run)
					assert.Loosely(t, processed, should.BeTrue)
					assert.Loosely(t, tasks, should.BeEmpty)
				})
			})
			t.Run(`Dry run`, func(t *ftt.Test) {
				run.Mode = "DRY_RUN"
				expectedTaskTemplate.PresubmitRun.Mode = pb.PresubmitRunMode_DRY_RUN

				processed, tasks := processCVRun(run)
				assert.Loosely(t, processed, should.BeTrue)
				// assert ingestion task has been scheduled.
				assert.Loosely(t, sortTasks(tasks), should.Match(
					sortTasks(expectedTasks(expectedTaskTemplate, builds))))
			})
			t.Run(`New patchset run`, func(t *ftt.Test) {
				run.Mode = "NEW_PATCHSET_RUN"
				expectedTaskTemplate.PresubmitRun.Mode = pb.PresubmitRunMode_NEW_PATCHSET_RUN

				processed, tasks := processCVRun(run)
				assert.Loosely(t, processed, should.BeTrue)
				// assert ingestion task has been scheduled.
				assert.Loosely(t, sortTasks(tasks), should.Match(
					sortTasks(expectedTasks(expectedTaskTemplate, builds))))
			})
			t.Run(`CV Run owned by Automation`, func(t *ftt.Test) {
				run.Owner = "chromium-autoroll@skia-public.iam.gserviceaccount.com"
				expectedTaskTemplate.PresubmitRun.Owner = "automation"

				processed, tasks := processCVRun(run)
				assert.Loosely(t, processed, should.BeTrue)
				// assert ingestion task has been scheduled.
				assert.Loosely(t, sortTasks(tasks), should.Match(
					sortTasks(expectedTasks(expectedTaskTemplate, builds))))
			})
			t.Run(`CV Run owned by Automation 2`, func(t *ftt.Test) {
				run.Owner = "3su6n15k.default@developer.gserviceaccount.com"
				expectedTaskTemplate.PresubmitRun.Owner = "automation"

				processed, tasks := processCVRun(run)
				assert.Loosely(t, processed, should.BeTrue)
				// assert ingestion task has been scheduled.
				assert.Loosely(t, sortTasks(tasks), should.Match(
					sortTasks(expectedTasks(expectedTaskTemplate, builds))))
			})
			t.Run(`With non-buildbucket tryjob`, func(t *ftt.Test) {
				// Should be ignored.
				run.Tryjobs = append(run.Tryjobs, &cvv0.Tryjob{
					Result: &cvv0.Tryjob_Result{},
				})

				processed, tasks := processCVRun(run)
				assert.Loosely(t, processed, should.BeTrue)
				// assert ingestion task has been scheduled.
				assert.Loosely(t, sortTasks(tasks), should.Match(
					sortTasks(expectedTasks(expectedTaskTemplate, builds))))
			})
			t.Run(`With re-used tryjob`, func(t *ftt.Test) {
				// Assume that this tryjob was created by another CV run,
				// so should not be ingested with this CV run.
				run.TryjobInvocations[0].Attempts[0].Reuse = true

				processed, tasks := processCVRun(run)
				assert.Loosely(t, processed, should.BeTrue)
				// assert ingestion task has been scheduled.
				assert.Loosely(t, sortTasks(tasks), should.Match(
					sortTasks(expectedTasks(expectedTaskTemplate, builds[1:]))))
			})
			t.Run(`With retried tryjob`, func(t *ftt.Test) {
				// Despite tryjob group being marked critical,
				// build one should remain non-critical as it
				// was retried by build 3.
				run.TryjobInvocations[0].Attempts = []*cvv0.TryjobInvocation_Attempt{
					tryjob(3).Attempts[0],
					tryjob(buildOne.buildID).Attempts[0],
				}
				run.TryjobInvocations[0].Critical = true

				processed, tasks := processCVRun(run)
				assert.Loosely(t, processed, should.BeTrue)
				// assert ingestion task has been scheduled.
				assert.Loosely(t, sortTasks(tasks), should.Match(
					sortTasks(expectedTasks(expectedTaskTemplate, builds))))
			})
			t.Run(`Failing Run`, func(t *ftt.Test) {
				run.Status = cvv0.Run_FAILED
				expectedTaskTemplate.PresubmitRun.Status = pb.PresubmitRunStatus_PRESUBMIT_RUN_STATUS_FAILED

				processed, tasks := processCVRun(run)
				assert.Loosely(t, processed, should.BeTrue)
				// assert ingestion task has been scheduled.
				assert.Loosely(t, sortTasks(tasks), should.Match(
					sortTasks(expectedTasks(expectedTaskTemplate, builds))))
			})
			t.Run(`Cancelled Run`, func(t *ftt.Test) {
				run.Status = cvv0.Run_CANCELLED
				expectedTaskTemplate.PresubmitRun.Status = pb.PresubmitRunStatus_PRESUBMIT_RUN_STATUS_CANCELED

				processed, tasks := processCVRun(run)
				assert.Loosely(t, processed, should.BeTrue)
				// assert ingestion task has been scheduled.
				assert.Loosely(t, sortTasks(tasks), should.Match(
					sortTasks(expectedTasks(expectedTaskTemplate, builds))))
			})
		})
	})
}

func tryjob(bID int64) *cvv0.TryjobInvocation {
	return &cvv0.TryjobInvocation{
		Attempts: []*cvv0.TryjobInvocation_Attempt{
			{
				Result: &cvv0.TryjobResult{
					Backend: &cvv0.TryjobResult_Buildbucket_{
						Buildbucket: &cvv0.TryjobResult_Buildbucket{
							Id: int64(bID),
						},
					},
				},
			},
		},
		Critical: (bID % 2) == 0,
	}
}

func fullRunID(project, runID string) string {
	return fmt.Sprintf("projects/%s/runs/%s", project, runID)
}

func expectedTasks(taskTemplate *taskspb.IngestTestVerdicts, builds []*buildBuilder) []*taskspb.IngestTestVerdicts {
	res := make([]*taskspb.IngestTestVerdicts, 0, len(builds))
	for _, build := range builds {
		t := proto.Clone(taskTemplate).(*taskspb.IngestTestVerdicts)
		t.PresubmitRun.Critical = ((build.buildID % 2) == 0)
		t.Build = build.ExpectedResult()
		t.IngestionId = string(control.IngestionIDFromBuildID(build.buildID))
		t.PartitionTime = timestamppb.New(build.createTime)

		if build.hasInvocation {
			t.Invocation = &controlpb.InvocationResult{
				ResultdbHost: rdbHost,
				InvocationId: fmt.Sprintf("build-%d", build.buildID),
				CreationTime: timestamppb.New(time.Date(2024, time.December, 11, 10, 9, 8, 7, time.UTC)),
			}
			t.PartitionTime = t.Invocation.CreationTime
		}
		res = append(res, t)
	}
	return res
}

func sortTasks(tasks []*taskspb.IngestTestVerdicts) []*taskspb.IngestTestVerdicts {
	sort.Slice(tasks, func(i, j int) bool { return tasks[i].Build.Id < tasks[j].Build.Id })
	return tasks
}
