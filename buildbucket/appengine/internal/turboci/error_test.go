// Copyright 2026 The LUCI Authors.
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

package turboci

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/filter/txndefer"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"
	orchestratorpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1"

	"go.chromium.org/luci/buildbucket/appengine/model"
	taskdefs "go.chromium.org/luci/buildbucket/appengine/tasks/defs"
	pb "go.chromium.org/luci/buildbucket/proto"
)

func TestHandleStageAttemptStatusConflict(t *testing.T) {
	t.Parallel()

	ftt.Run("HandleStageAttemptStatusConflict", t, func(t *ftt.Test) {
		ctx := txndefer.FilterRDS(memory.Use(context.Background()))
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)
		ctx, _ = testclock.UseTime(ctx, testclock.TestRecentTimeUTC)

		b := &pb.Build{
			Id: 1,
			Builder: &pb.BuilderID{
				Project: "project",
				Bucket:  "bucket",
				Builder: "builder",
			},
		}
		bld := &model.Build{
			ID:    1,
			Proto: b,
		}
		assert.NoErr(t, datastore.Put(ctx, bld))

		t.Run("no StageAttemptCurrentState in error", func(t *ftt.Test) {
			err := status.Error(codes.FailedPrecondition, "some error")
			assert.ErrIsLike(t, HandleStageAttemptStatusConflict(ctx, b, err), err)
		})

		t.Run("unsupported state", func(t *ftt.Test) {
			turboCIErr := ErrorWithStageAttemptCurrentState(orchestratorpb.StageAttemptState_STAGE_ATTEMPT_STATE_PENDING.Enum(), t)
			err := HandleStageAttemptStatusConflict(ctx, b, turboCIErr)
			assert.ErrIsLike(t, err, "status STAGE_ATTEMPT_STATE_PENDING is not supported")
			assert.Loosely(t, status.Code(err), should.Equal(codes.InvalidArgument))
		})

		t.Run("running state", func(t *ftt.Test) {
			turboCIErr := ErrorWithStageAttemptCurrentState(orchestratorpb.StageAttemptState_STAGE_ATTEMPT_STATE_RUNNING.Enum(), t)
			assert.NoErr(t, HandleStageAttemptStatusConflict(ctx, b, turboCIErr))
		})

		t.Run("tearing down state", func(t *ftt.Test) {
			turboCIErr := ErrorWithStageAttemptCurrentState(orchestratorpb.StageAttemptState_STAGE_ATTEMPT_STATE_RUNNING.Enum(), t)
			assert.NoErr(t, HandleStageAttemptStatusConflict(ctx, b, turboCIErr))
		})

		t.Run("cancelling state", func(t *ftt.Test) {
			turboCIErr := ErrorWithStageAttemptCurrentState(orchestratorpb.StageAttemptState_STAGE_ATTEMPT_STATE_CANCELLING.Enum(), t)

			t.Run("build not found", func(t *ftt.Test) {
				assert.ErrIsLike(t, HandleStageAttemptStatusConflict(ctx, &pb.Build{Id: 2}, turboCIErr), "build not found")
			})

			t.Run("build already cancelled", func(t *ftt.Test) {
				bld.Proto.Status = pb.Status_CANCELED
				assert.NoErr(t, datastore.Put(ctx, bld))
				assert.NoErr(t, HandleStageAttemptStatusConflict(ctx, b, turboCIErr))
			})

			t.Run("build already ended", func(t *ftt.Test) {
				bld.Proto.Status = pb.Status_SUCCESS
				assert.NoErr(t, datastore.Put(ctx, bld))
				assert.NoErr(t, HandleStageAttemptStatusConflict(ctx, b, turboCIErr))
			})

			t.Run("build already being cancelled", func(t *ftt.Test) {
				bld.Proto.Status = pb.Status_STARTED
				bld.Proto.CancelTime = timestamppb.New(testclock.TestRecentTimeUTC)
				assert.NoErr(t, datastore.Put(ctx, bld))
				assert.NoErr(t, HandleStageAttemptStatusConflict(ctx, b, turboCIErr))
			})

			t.Run("cancel build", func(t *ftt.Test) {
				ctx, sch := tq.TestingContext(ctx, nil)
				bld.Proto.Status = pb.Status_STARTED
				bld.Proto.CancelTime = nil
				assert.NoErr(t, datastore.Put(ctx, bld))

				err := HandleStageAttemptStatusConflict(ctx, b, turboCIErr)
				assert.Loosely(t, err, should.BeNil)

				tasks := sch.Tasks()
				assert.Loosely(t, len(tasks), should.Equal(1))
				assert.Loosely(t, tasks[0].Payload, should.Resemble(&taskdefs.CancelBuildTask{BuildId: 1}))

				updatedBld := &model.Build{ID: 1}
				assert.Loosely(t, datastore.Get(ctx, updatedBld), should.BeNil)
				assert.Loosely(t, updatedBld.Proto.CancelTime, should.NotBeNil)
				assert.Loosely(t, updatedBld.Proto.CancellationMarkdown, should.Equal("Cancelled by TurboCI"))
			})
		})

		t.Run("ended state", func(t *ftt.Test) {
			t.Run("incomplete", func(t *ftt.Test) {
				turboCIErr := ErrorWithStageAttemptCurrentState(orchestratorpb.StageAttemptState_STAGE_ATTEMPT_STATE_INCOMPLETE.Enum(), t)

				t.Run("build not found", func(t *ftt.Test) {
					assert.ErrIsLike(t, HandleStageAttemptStatusConflict(ctx, &pb.Build{Id: 2}, turboCIErr), "build not found")
				})

				t.Run("build already ended with matching status", func(t *ftt.Test) {
					bld.Proto.Status = pb.Status_CANCELED
					assert.NoErr(t, datastore.Put(ctx, bld))
					assert.NoErr(t, HandleStageAttemptStatusConflict(ctx, b, turboCIErr))
				})

				t.Run("build already ended with mismatching status", func(t *ftt.Test) {
					bld.Proto.Status = pb.Status_SUCCESS
					assert.NoErr(t, datastore.Put(ctx, bld))
					assert.NoErr(t, HandleStageAttemptStatusConflict(ctx, b, turboCIErr))
				})

				t.Run("cancel build", func(t *ftt.Test) {
					ctx, sch := tq.TestingContext(ctx, nil)
					bld.Proto.Status = pb.Status_STARTED
					assert.NoErr(t, datastore.Put(ctx, bld))

					err := HandleStageAttemptStatusConflict(ctx, b, turboCIErr)
					assert.Loosely(t, err, should.BeNil)

					tasks := sch.Tasks()
					assert.Loosely(t, len(tasks), should.Equal(1))
					assert.Loosely(t, tasks[0].Payload, should.Resemble(&taskdefs.CancelBuildTask{BuildId: 1}))

					updatedBld := &model.Build{ID: 1}
					assert.Loosely(t, datastore.Get(ctx, updatedBld), should.BeNil)
					assert.Loosely(t, updatedBld.Proto.CancelTime, should.NotBeNil)
					assert.Loosely(t, updatedBld.Proto.CancellationMarkdown, should.Equal("Cancelled because TurboCI stage attempt ends with state STAGE_ATTEMPT_STATE_INCOMPLETE"))
				})
			})

			t.Run("complete", func(t *ftt.Test) {
				turboCIErr := ErrorWithStageAttemptCurrentState(orchestratorpb.StageAttemptState_STAGE_ATTEMPT_STATE_COMPLETE.Enum(), t)

				t.Run("build already ended with matching status", func(t *ftt.Test) {
					bld.Proto.Status = pb.Status_SUCCESS
					assert.NoErr(t, datastore.Put(ctx, bld))
					assert.NoErr(t, HandleStageAttemptStatusConflict(ctx, b, turboCIErr))
				})

				t.Run("build already ended with mismatching status", func(t *ftt.Test) {
					bld.Proto.Status = pb.Status_CANCELED
					assert.NoErr(t, datastore.Put(ctx, bld))
					assert.NoErr(t, HandleStageAttemptStatusConflict(ctx, b, turboCIErr))
				})

				t.Run("cancel build", func(t *ftt.Test) {
					ctx, sch := tq.TestingContext(ctx, nil)
					bld.Proto.Status = pb.Status_STARTED
					assert.NoErr(t, datastore.Put(ctx, bld))

					err := HandleStageAttemptStatusConflict(ctx, b, turboCIErr)
					assert.Loosely(t, err, should.BeNil)

					tasks := sch.Tasks()
					assert.Loosely(t, len(tasks), should.Equal(1))
					assert.Loosely(t, tasks[0].Payload, should.Resemble(&taskdefs.CancelBuildTask{BuildId: 1}))

					updatedBld := &model.Build{ID: 1}
					assert.Loosely(t, datastore.Get(ctx, updatedBld), should.BeNil)
					assert.Loosely(t, updatedBld.Proto.CancelTime, should.NotBeNil)
					assert.Loosely(t, updatedBld.Proto.CancellationMarkdown, should.Equal("Cancelled because TurboCI stage attempt ends with state STAGE_ATTEMPT_STATE_COMPLETE"))
				})
			})
		})
	})
}
