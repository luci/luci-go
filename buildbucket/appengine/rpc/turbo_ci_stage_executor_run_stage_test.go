// Copyright 2025 The LUCI Authors.
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
	"crypto/sha256"
	"fmt"
	"testing"

	"golang.org/x/oauth2"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/auth/scopes"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/filter/txndefer"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/tq"
	"go.chromium.org/luci/turboci/id"
	stagepb "go.chromium.org/turboci/proto/go/data/stage/v1"
	executorpb "go.chromium.org/turboci/proto/go/graph/executor/v1"
	idspb "go.chromium.org/turboci/proto/go/graph/ids/v1"
	orchestratorpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1"

	"go.chromium.org/luci/buildbucket/appengine/internal/config"
	"go.chromium.org/luci/buildbucket/appengine/internal/metrics"
	"go.chromium.org/luci/buildbucket/appengine/internal/turboci"
	"go.chromium.org/luci/buildbucket/appengine/model"
	"go.chromium.org/luci/buildbucket/appengine/rpc/testutil"
	taskdefs "go.chromium.org/luci/buildbucket/appengine/tasks/defs"
	"go.chromium.org/luci/buildbucket/bbperms"
	pb "go.chromium.org/luci/buildbucket/proto"
)

func TestRunStage(t *testing.T) {
	t.Parallel()

	ftt.Run("RunStage", t, func(t *ftt.Test) {
		ctx := txndefer.FilterRDS(memory.Use(context.Background()))
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)
		ctx, sch := tq.TestingContext(ctx, nil)
		ctx = metrics.WithServiceInfo(ctx, "svc", "job", "ins")
		assert.NoErr(t, config.SetTestSettingsCfg(ctx, &pb.SettingsCfg{
			Swarming: &pb.SwarmingSettings{
				MiloHostname: "milo.com",
			},
		}))

		const testUserID = "user:test@example.com"
		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity: identity.Identity(testUserID),
			UserExtra: &auth.GoogleOAuth2Info{
				Scopes: scopes.BuildbucketScopeSet(),
			},
			UserCredentialsOverride: &oauth2.Token{
				AccessToken: "some-token",
			},
			FakeDB: authtest.NewFakeDB(
				authtest.MockPermission(testUserID, "project:bucket", bbperms.BuildsAdd),
				authtest.MockPermission(testUserID, "project:bucket", bbperms.BuildsGet),
			),
		})

		setup := func() (context.Context, *executorpb.RunStageRequest, *idspb.StageAttempt, *turboci.FakeOrchestratorClient, *TurboCIStageExecutor) {
			schReq := &pb.ScheduleBuildRequest{
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
			}
			args, err := anypb.New(schReq)
			assert.NoErr(t, err)
			ctx = context.WithValue(ctx, &turboCICallKey, &TurboCICallInfo{ScheduleBuild: schReq})

			aID, err := id.FromString("Lplan-id:Sstage-id:A1")
			assert.NoErr(t, err)
			attemptID := aID.GetStageAttempt()

			stage := orchestratorpb.Stage_builder{
				Identifier: attemptID.GetStage(),
				Args:       orchestratorpb.Value_builder{Value: args}.Build(),
				Attempts: []*orchestratorpb.Stage_Attempt{
					orchestratorpb.Stage_Attempt_builder{
						Identifier: attemptID,
						State:      orchestratorpb.StageAttemptState_STAGE_ATTEMPT_STATE_PENDING.Enum(),
					}.Build(),
				},
			}.Build()

			req := executorpb.RunStageRequest_builder{
				Stage:             stage,
				Attempt:           attemptID,
				StageAttemptToken: proto.String("secret-token"),
			}.Build()

			mockOrch := &turboci.FakeOrchestratorClient{}
			ctx = turboci.WithTurboCIOrchestratorClient(ctx, mockOrch)
			se := &TurboCIStageExecutor{
				TurboCIHost: "turbocihost",
			}
			return ctx, req, attemptID, mockOrch, se
		}
		t.Run("OK", func(t *ftt.Test) {
			ctx, req, attemptID, mockOrch, se := setup()
			attemptIDStr := id.ToString(attemptID)
			testutil.PutBucket(ctx, "project", "bucket", &pb.Bucket{Swarming: &pb.Swarming{}})
			testutil.PutBuilder(ctx, "project", "bucket", "builder", "")

			resp, err := se.RunStage(ctx, req)
			assert.NoErr(t, err)
			assert.That(t, resp, should.Match(&executorpb.RunStageResponse{}))

			reqID := &model.RequestID{
				ID: fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%s:%s", auth.CurrentIdentity(ctx), sha256hex(attemptIDStr))))),
			}
			assert.NoErr(t, datastore.Get(ctx, reqID))

			bld := &model.Build{
				ID: reqID.BuildID,
			}
			infra := &model.BuildInfra{
				Build: datastore.KeyForObj(ctx, bld),
			}
			assert.NoErr(t, datastore.Get(ctx, bld, infra))
			assert.That(t, bld.StageAttemptID, should.Equal(attemptIDStr))
			assert.That(t, bld.StageAttemptToken, should.Equal("secret-token"))
			assert.That(t, infra.Proto.Turboci, should.Match(&pb.BuildInfra_TurboCI{
				Hostname: "turbocihost",
			}))

			assert.That(t, mockOrch.LastWriteNodesCall.GetToken(), should.Equal("secret-token"))
			reasons := mockOrch.LastWriteNodesCall.GetReasons()
			assert.That(t, len(reasons), should.Equal(1))
			assert.That(t, reasons[0].GetMessage(), should.Equal(`Buildbucket build scheduled`))
			assert.That(t, mockOrch.LastWriteNodesCall.GetCurrentAttempt().GetStateTransition().HasScheduled(), should.BeTrue)
			assert.That(t, len(mockOrch.LastWriteNodesCall.GetCurrentAttempt().GetDetails()), should.Equal(2))
			bldDetails := &pb.BuildStageDetails{}
			assert.NoErr(t, mockOrch.LastWriteNodesCall.GetCurrentAttempt().GetDetails()[0].GetValue().UnmarshalTo(bldDetails))
			assert.That(t, bldDetails.GetId(), should.Equal(bld.ID))
			commonDetails := &stagepb.CommonStageAttemptDetails{}
			assert.NoErr(t, mockOrch.LastWriteNodesCall.GetCurrentAttempt().GetDetails()[1].GetValue().UnmarshalTo(commonDetails))
			assert.That(t, commonDetails.GetViewUrls()["Buildbucket"].GetUrl(), should.Equal(fmt.Sprintf("https://app.appspot.com/build/%d", bld.ID)))
		})

		t.Run("validation failures", func(t *ftt.Test) {
			t.Run("attempt not found", func(t *ftt.Test) {
				ctx, req, _, _, se := setup()
				missingAttemptID, err := id.FromString("Lplan-id:Sstage-id:A3")
				assert.NoErr(t, err)
				req.SetAttempt(missingAttemptID.GetStageAttempt())

				_, err = se.RunStage(ctx, req)
				assert.That(t, err, should.ErrLike(`attempt Lplan-id:Sstage-id:A3 not found`))
			})

			t.Run("attempt not pending", func(t *ftt.Test) {
				ctx, req, _, _, se := setup()
				req.GetStage().GetAttempts()[0].SetState(orchestratorpb.StageAttemptState_STAGE_ATTEMPT_STATE_RUNNING)
				_, err := se.RunStage(ctx, req)
				assert.That(t, err, should.ErrLike("is in state STAGE_ATTEMPT_STATE_RUNNING, expecting PENDING"))
			})

			t.Run("missing token", func(t *ftt.Test) {
				ctx, req, _, _, se := setup()
				req.ClearStageAttemptToken()
				_, err := se.RunStage(ctx, req)
				assert.That(t, err, should.ErrLike("missing stage_attempt_token"))
			})
		})

		t.Run("get parent failure", func(t *ftt.Test) {
			ctx, req, _, mockOrch, se := setup()
			pStageAttemptIDStr := "Lplan-id:Sstage-id:A0"
			pStageAttemptID, err := id.FromString(pStageAttemptIDStr)
			assert.NoErr(t, err)
			req.GetStage().SetCreatedBy(orchestratorpb.Actor_builder{
				StageAttempt: pStageAttemptID.GetStageAttempt(),
			}.Build())

			_, err = se.RunStage(ctx, req)
			assert.NoErr(t, err)

			// Check that failCurrentStage was called.
			assert.That(t, mockOrch.LastWriteNodesCall.GetToken(), should.Equal("secret-token"))
			assert.That(t, mockOrch.LastWriteNodesCall.GetCurrentAttempt().GetStateTransition().HasIncomplete(), should.BeTrue)
			progress := mockOrch.LastWriteNodesCall.GetCurrentAttempt().GetProgress()
			assert.That(t, len(progress), should.Equal(1))
			assert.That(t, progress[0].GetMessage(), should.Equal(`rpc error: code = FailedPrecondition desc = expect 1 build by stage_attempt_id "Lplan-id:Sstage-id:A0", but got 0`))
		})

		t.Run("get parent transient failure", func(t *ftt.Test) {
			ctx, req, _, mockOrch, se := setup()
			pStageAttemptIDStr := "Lplan-id:Sstage-id:A0"
			pStageAttemptID, err := id.FromString(pStageAttemptIDStr)
			assert.NoErr(t, err)
			req.GetStage().SetCreatedBy(orchestratorpb.Actor_builder{
				StageAttempt: pStageAttemptID.GetStageAttempt(),
			}.Build())

			// Inject a datastore filter to simulate a transient error.
			ctx = datastore.AddRawFilters(ctx, func(ctx context.Context, rds datastore.RawInterface) datastore.RawInterface {
				return transientErrFilter{rds}
			})

			_, err = se.RunStage(ctx, req)
			assert.That(t, err, should.ErrLike("failed to query builds by stage_attempt_id"))

			// Check that failCurrentStage was not called.
			assert.Loosely(t, mockOrch.LastWriteNodesCall, should.BeNil)
		})

		t.Run("scheduleBuilds failure", func(t *ftt.Test) {
			ctx, req, _, mockOrch, se := setup()
			// No bucket or builder in datastore, so scheduleBuilds will fail.
			_, err := se.RunStage(ctx, req)
			assert.NoErr(t, err)

			// Check that failCurrentStage was called.
			assert.That(t, mockOrch.LastWriteNodesCall.GetToken(), should.Equal("secret-token"))
			reasons := mockOrch.LastWriteNodesCall.GetReasons()
			assert.That(t, len(reasons), should.Equal(1))
			assert.That(t, reasons[0].GetMessage(), should.Equal(`Set the stage attempt Lplan-id:Sstage-id:A1 to INCOMPLETE due to an error: rpc error: code = NotFound desc = requested resource not found or "user:test@example.com" does not have permission to view it`))
			assert.That(t, mockOrch.LastWriteNodesCall.GetCurrentAttempt().GetStateTransition().HasIncomplete(), should.BeTrue)
			progress := mockOrch.LastWriteNodesCall.GetCurrentAttempt().GetProgress()
			assert.That(t, len(progress), should.Equal(1))
			assert.That(t, progress[0].GetMessage(), should.Equal(`rpc error: code = NotFound desc = requested resource not found or "user:test@example.com" does not have permission to view it`))
		})

		t.Run("updateStageAttemptToScheduled failure", func(t *ftt.Test) {
			ctx, req, _, mockOrch, se := setup()
			testutil.PutBucket(ctx, "project", "bucket", &pb.Bucket{Swarming: &pb.Swarming{}})
			testutil.PutBuilder(ctx, "project", "bucket", "builder", "")

			mockOrch.Err = status.Error(codes.Internal, "update failed")

			_, err := se.RunStage(ctx, req)
			assert.That(t, err, should.ErrLike("update failed"))
		})

		t.Run("attempt is alread running", func(t *ftt.Test) {
			ctx, req, _, mockOrch, se := setup()
			testutil.PutBucket(ctx, "project", "bucket", &pb.Bucket{Swarming: &pb.Swarming{}})
			testutil.PutBuilder(ctx, "project", "bucket", "builder", "")

			mockOrch.Err = turboci.ErrorWithStageAttemptCurrentState(orchestratorpb.StageAttemptState_STAGE_ATTEMPT_STATE_RUNNING.Enum(), t)

			_, err := se.RunStage(ctx, req)
			assert.NoErr(t, err)
		})

		t.Run("attempt is cancelling", func(t *ftt.Test) {
			ctx, req, _, mockOrch, se := setup()
			testutil.PutBucket(ctx, "project", "bucket", &pb.Bucket{Swarming: &pb.Swarming{}})
			testutil.PutBuilder(ctx, "project", "bucket", "builder", "")

			mockOrch.Err = turboci.ErrorWithStageAttemptCurrentState(orchestratorpb.StageAttemptState_STAGE_ATTEMPT_STATE_CANCELLING.Enum(), t)

			_, err := se.RunStage(ctx, req)
			assert.NoErr(t, err)

			tasks := sch.Tasks()
			// CreateBackendBuildTask, NotifyPubSubGoProxy for schedule build
			// CancelBuildTask for handling status mismatch from TurboCI
			assert.That(t, len(tasks), should.Equal(3))
			_, ok := tasks.Payloads()[2].(*taskdefs.CancelBuildTask)
			assert.Loosely(t, ok, should.BeTrue)
		})

		t.Run("attempt is incomplete", func(t *ftt.Test) {
			ctx, req, _, mockOrch, se := setup()
			testutil.PutBucket(ctx, "project", "bucket", &pb.Bucket{Swarming: &pb.Swarming{}})
			testutil.PutBuilder(ctx, "project", "bucket", "builder", "")

			mockOrch.Err = turboci.ErrorWithStageAttemptCurrentState(orchestratorpb.StageAttemptState_STAGE_ATTEMPT_STATE_INCOMPLETE.Enum(), t)

			_, err := se.RunStage(ctx, req)
			assert.NoErr(t, err)

			tasks := sch.Tasks()
			// CreateBackendBuildTask, NotifyPubSubGoProxy for schedule build
			// CancelBuildTask for handling status mismatch from TurboCI
			assert.That(t, len(tasks), should.Equal(3))
			_, ok := tasks.Payloads()[2].(*taskdefs.CancelBuildTask)
			assert.Loosely(t, ok, should.BeTrue)
		})

		t.Run("failCurrentStage failure", func(t *ftt.Test) {
			ctx, req, _, mockOrch, se := setup()
			mockOrch.Err = status.Error(codes.Internal, "fail stage failed")

			_, err := se.RunStage(ctx, req)
			assert.That(t, err, should.ErrLike("fail stage failed"))
		})

		t.Run("dedup request id - OK", func(t *ftt.Test) {
			ctx, req, attemptID, mockOrch, se := setup()
			attemptIDStr := id.ToString(attemptID)
			testutil.PutBucket(ctx, "project", "bucket", &pb.Bucket{Swarming: &pb.Swarming{}})
			testutil.PutBuilder(ctx, "project", "bucket", "builder", "")

			// First RunStage.
			mockOrch.Err = status.Error(codes.Internal, "update failed")
			se.RunStage(ctx, req)

			reqID := &model.RequestID{
				ID: fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%s:%s", auth.CurrentIdentity(ctx), sha256hex(attemptIDStr))))),
			}
			assert.NoErr(t, datastore.Get(ctx, reqID))

			bld := &model.Build{
				ID: reqID.BuildID,
			}
			assert.NoErr(t, datastore.Get(ctx, bld))
			assert.That(t, bld.StageAttemptID, should.Equal(attemptIDStr))
			assert.That(t, bld.StageAttemptToken, should.Equal("secret-token"))

			// Second RunStage.
			mockOrch.Err = nil
			_, err := se.RunStage(ctx, req)
			assert.NoErr(t, err)

			assert.That(t, mockOrch.LastWriteNodesCall.GetToken(), should.Equal("secret-token"))
			assert.That(t, mockOrch.LastWriteNodesCall.GetCurrentAttempt().GetStateTransition().HasScheduled(), should.BeTrue)
			bldDetails := &pb.BuildStageDetails{}
			assert.NoErr(t, mockOrch.LastWriteNodesCall.GetCurrentAttempt().GetDetails()[0].GetValue().UnmarshalTo(bldDetails))
			assert.That(t, bldDetails.GetId(), should.Equal(bld.ID))
		})

		t.Run("dedup request id - the build has started", func(t *ftt.Test) {
			ctx, req, attemptID, mockOrch, se := setup()
			attemptIDStr := id.ToString(attemptID)
			testutil.PutBucket(ctx, "project", "bucket", &pb.Bucket{Swarming: &pb.Swarming{}})
			testutil.PutBuilder(ctx, "project", "bucket", "builder", "")

			// First RunStage.
			se.RunStage(ctx, req)

			reqID := &model.RequestID{
				ID: fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%s:%s", auth.CurrentIdentity(ctx), sha256hex(attemptIDStr))))),
			}
			assert.NoErr(t, datastore.Get(ctx, reqID))

			bld := &model.Build{
				ID: reqID.BuildID,
			}
			assert.NoErr(t, datastore.Get(ctx, bld))
			assert.That(t, bld.StageAttemptID, should.Equal(attemptIDStr))
			assert.That(t, bld.StageAttemptToken, should.Equal("secret-token"))

			// The build has started.
			bld.Proto.Status = pb.Status_STARTED
			assert.NoErr(t, datastore.Put(ctx, bld))

			// reset mockOrch
			mockOrch.LastWriteNodesCall = nil

			// Second RunStage.
			_, err := se.RunStage(ctx, req)
			assert.NoErr(t, err)

			// No update to TurboCI.
			assert.Loosely(t, mockOrch.LastWriteNodesCall, should.BeNil)
		})
	})
}

type transientErrFilter struct{ datastore.RawInterface }

func (f transientErrFilter) Run(q *datastore.FinalizedQuery, cb datastore.RawRunCB) error {
	return appstatus.Error(codes.Internal, "transient internal error")
}
