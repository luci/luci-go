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

package rpc

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/filter/txndefer"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/tq"
	executorpb "go.chromium.org/turboci/proto/go/graph/executor/v1"
	orchestratorpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1"
	"go.chromium.org/turboci/proto/go/utils/ids"
	"go.chromium.org/turboci/proto/go/utils/value"

	"go.chromium.org/luci/buildbucket/appengine/internal/turboci"
	"go.chromium.org/luci/buildbucket/appengine/model"
	taskdefs "go.chromium.org/luci/buildbucket/appengine/tasks/defs"
	pb "go.chromium.org/luci/buildbucket/proto"
)

func TestCancelStage(t *testing.T) {
	t.Parallel()

	ftt.Run("CancelStage", t, func(t *ftt.Test) {
		ctx := txndefer.FilterRDS(memory.Use(context.Background()))
		ctx, sch := tq.TestingContext(ctx, nil)

		orch := &turboci.FakeOrchestratorClient{}
		ctx = turboci.WithTurboCIOrchestratorClient(ctx, orch)

		const buildID = 123
		build := &model.Build{
			ID: buildID,
			Proto: &pb.Build{
				Id: buildID,
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
				Status: pb.Status_SCHEDULED,
			},
		}
		assert.NoErr(t, datastore.Put(ctx, build))

		aID, err := ids.FromString("Lplan-id:Sstage-id:A1")
		assert.NoErr(t, err)
		attemptID := aID.GetStageAttempt()

		stage := orchestratorpb.Stage_builder{
			Identifier: attemptID.GetStage(),
			Args:       makeStageArgs(&pb.ScheduleBuildRequest{}, "project:bucket"),
			Attempts: []*orchestratorpb.Stage_Attempt{
				orchestratorpb.Stage_Attempt_builder{
					Identifier: attemptID,
					State:      orchestratorpb.StageAttemptState_STAGE_ATTEMPT_STATE_CANCELLING.Enum(),
					Details: []*orchestratorpb.ValueRef{
						value.MustInline(&pb.BuildStageDetails{
							Result: &pb.BuildStageDetails_Id{Id: buildID},
						}, "project:bucket"),
					},
				}.Build(),
			},
		}.Build()

		call := func(t *ftt.Test, req *executorpb.CancelStageRequest) error {
			info, err := extractTurboCICallInfo(req)
			assert.NoErr(t, err)
			_, err = (&TurboCIStageExecutor{}).CancelStage(context.WithValue(ctx, &turboCICallKey, info), req)
			return err
		}

		t.Run("OK", func(t *ftt.Test) {
			err = call(t, executorpb.CancelStageRequest_builder{
				Stage:             stage,
				Attempt:           attemptID,
				StageAttemptToken: proto.String("secret-token"),
			}.Build())
			assert.NoErr(t, err)

			// Cancelled the build.
			build := &model.Build{ID: buildID}
			assert.NoErr(t, datastore.Get(ctx, build))
			assert.That(t, build.Proto.CanceledBy, should.Equal("turboci"))
			assert.That(t, sch.Tasks().Payloads(), should.Match([]proto.Message{
				&taskdefs.CancelBuildTask{BuildId: buildID},
			}))

			// Switched the attempt to TEARING_DOWN.
			assert.That(t, orch.LastWriteNodesCall, should.Match(orchestratorpb.WriteNodesRequest_builder{
				CurrentAttempt: orchestratorpb.WriteNodesRequest_CurrentAttemptWrite_builder{
					StateTransition: orchestratorpb.WriteNodesRequest_CurrentAttemptWrite_StateTransition_builder{
						TearingDown: &orchestratorpb.WriteNodesRequest_CurrentAttemptWrite_StateTransition_TearingDown{},
					}.Build(),
					Details: orch.LastWriteNodesCall.GetCurrentAttempt().GetDetails(),
				}.Build(),
				Reason: orchestratorpb.WriteNodesRequest_Reason_builder{
					Message: proto.String("Buildbucket build tearing down"),
				}.Build(),
				Token: proto.String("secret-token"),
			}.Build()))
		})

		t.Run("No details", func(t *ftt.Test) {
			stage := proto.CloneOf(stage)
			stage.GetAttempts()[0].SetDetails(nil)
			err = call(t, executorpb.CancelStageRequest_builder{
				Stage:             stage,
				Attempt:           attemptID,
				StageAttemptToken: proto.String("secret-token"),
			}.Build())
			assert.That(t, appstatus.Code(err), should.Equal(codes.Internal))
		})
	})
}
