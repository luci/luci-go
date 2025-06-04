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
	"go.chromium.org/luci/common/testing/truth"
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

func TestHandleBuild(t *testing.T) {
	ftt.Run(`Test JoinBuild`, t, func(t *ftt.Test) {
		ctx := testutil.IntegrationTestContext(t)
		ctx, skdr := tq.TestingContext(ctx, nil)

		buildCreateTime := time.Now()

		build := newBuildBuilder(14141414).
			WithCreateTime(buildCreateTime)

		expectedTask := &taskspb.IngestTestVerdicts{
			PartitionTime:        timestamppb.New(buildCreateTime),
			Build:                build.ExpectedResult(),
			IngestionId:          string(control.IngestionIDFromBuildID(build.buildID)),
			Project:              "buildproject",
			UseNewIngestionOrder: true,
		}

		assertTasksExpected := func(t testing.TB) {
			t.Helper()
			// assert ingestion task has been scheduled.
			assert.Loosely(t, len(skdr.Tasks().Payloads()), should.Equal(1), truth.LineContext())
			resultsTask := skdr.Tasks().Payloads()[0].(*taskspb.IngestTestVerdicts)
			assert.That(t, resultsTask, should.Match(expectedTask), truth.LineContext())
		}

		t.Run(`With vanilla build`, func(t *ftt.Test) {
			t.Run(`Vanilla CI Build`, func(t *ftt.Test) {
				build = build.WithGardenerRotations([]string{"rotation1", "rotation2"})

				// Process build.
				assert.Loosely(t, ingestBuild(ctx, build), should.BeNil)

				assertTasksExpected(t)

				// Test repeated processing does not lead to further
				// ingestion tasks.
				assert.Loosely(t, ingestBuild(ctx, build), should.BeNil)

				assertTasksExpected(t)
			})
			t.Run(`Unusual CI Build`, func(t *ftt.Test) {
				// v8 project had some buildbucket-triggered builds
				// with both the user_agent:cq and user_agent:recipe tags.
				// These should be treated as CI builds.
				build = build.WithTags([]string{"user_agent:cq", "user_agent:recipe"})
				expectedTask.Build = build.ExpectedResult()

				// Process build.
				assert.Loosely(t, ingestBuild(ctx, build), should.BeNil)
				assertTasksExpected(t)
			})
		})
		t.Run(`With build that is part of a LUCI CV run`, func(t *ftt.Test) {
			build := build.WithTags([]string{"user_agent:cq"})
			expectedTask.Build = build.ExpectedResult()

			t.Run(`Without LUCI CV run processed previously`, func(t *ftt.Test) {
				assert.Loosely(t, ingestBuild(ctx, build), should.BeNil)

				assert.Loosely(t, len(skdr.Tasks().Payloads()), should.BeZero)
			})
			t.Run(`With LUCI CV run processed previously`, func(t *ftt.Test) {
				assert.Loosely(t, ingestCVRun(ctx, []int64{11111171, build.buildID}), should.BeNil)

				assert.Loosely(t, len(skdr.Tasks().Payloads()), should.BeZero)

				assert.Loosely(t, ingestBuild(ctx, build), should.BeNil)

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
				assertTasksExpected(t)

				// Check metrics were reported correctly for this sequence.
				isPresubmit := true
				hasInvocation := false
				hasBuildBucketBuild := true
				assert.Loosely(t, bbBuildInputCounter.Get(ctx, "buildproject", isPresubmit, hasInvocation), should.Equal(1))
				assert.Loosely(t, bbBuildOutputCounter.Get(ctx, "buildproject", isPresubmit, hasInvocation), should.Equal(1))
				assert.Loosely(t, cvBuildInputCounter.Get(ctx, "cvproject"), should.Equal(2))
				assert.Loosely(t, cvBuildOutputCounter.Get(ctx, "cvproject"), should.Equal(1))
				assert.Loosely(t, rdbInvocationsInputCounter.Get(ctx, "buildproject", hasBuildBucketBuild), should.BeZero)
				assert.Loosely(t, rdbInvocationsOutputCounter.Get(ctx, "buildproject", hasBuildBucketBuild), should.BeZero)
			})
		})
		t.Run(`With build that has a ResultDB invocation`, func(t *ftt.Test) {
			build := build.WithInvocation()
			expectedTask.Build = build.ExpectedResult()

			t.Run(`Without invocation finalization acknowledged previously`, func(t *ftt.Test) {
				assert.Loosely(t, ingestBuild(ctx, build), should.BeNil)

				assert.Loosely(t, len(skdr.Tasks().Payloads()), should.BeZero)
			})
			t.Run(`With invocation finalization acknowledged previously`, func(t *ftt.Test) {
				invocationCreateTime := time.Date(2024, time.December, 11, 10, 9, 8, 7, time.UTC)
				assert.Loosely(t, ingestFinalization(ctx, fmt.Sprintf("build-%d", build.buildID), false, invocationCreateTime), should.BeNil)

				assert.Loosely(t, len(skdr.Tasks().Payloads()), should.BeZero)

				assert.Loosely(t, ingestBuild(ctx, build), should.BeNil)

				expectedTask.Invocation = &controlpb.InvocationResult{
					ResultdbHost: rdbHost,
					InvocationId: fmt.Sprintf("build-%d", build.buildID),
					CreationTime: timestamppb.New(invocationCreateTime),
				}
				expectedTask.PartitionTime = timestamppb.New(invocationCreateTime)
				assertTasksExpected(t)

				// Check metrics were reported correctly for this sequence.
				isPresubmit := false
				hasInvocation := true
				hasBuildBucketBuild := true
				assert.Loosely(t, bbBuildInputCounter.Get(ctx, "buildproject", isPresubmit, hasInvocation), should.Equal(1))
				assert.Loosely(t, bbBuildOutputCounter.Get(ctx, "buildproject", isPresubmit, hasInvocation), should.Equal(1))
				assert.Loosely(t, cvBuildInputCounter.Get(ctx, "cvproject"), should.BeZero)
				assert.Loosely(t, cvBuildOutputCounter.Get(ctx, "cvproject"), should.BeZero)
				assert.Loosely(t, rdbInvocationsInputCounter.Get(ctx, "buildproject", hasBuildBucketBuild), should.Equal(1))
				assert.Loosely(t, rdbInvocationsOutputCounter.Get(ctx, "buildproject", hasBuildBucketBuild), should.Equal(1))
			})
		})
	})
}
