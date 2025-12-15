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
	"math/rand"
	"testing"

	"golang.org/x/oauth2"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/auth/scopes"
	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/turboci/data"
	"go.chromium.org/luci/turboci/id"
	idspb "go.chromium.org/turboci/proto/go/graph/ids/v1"
	orchestratorpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1"

	"go.chromium.org/luci/buildbucket/appengine/internal/turboci"
	"go.chromium.org/luci/buildbucket/appengine/model"
	pb "go.chromium.org/luci/buildbucket/proto"
)

func TestLaunchTurboCIRoot(t *testing.T) {
	t.Parallel()

	ctx := memory.Use(t.Context())
	ctx = auth.WithState(ctx, &authtest.FakeState{
		Identity: "user:caller@example.com",
		UserExtra: &auth.GoogleOAuth2Info{
			Scopes: scopes.BuildbucketScopeSet(),
		},
		UserCredentialsOverride: &oauth2.Token{
			AccessToken: "some-token",
		},
	})

	builder := &pb.BuilderID{
		Project: "project",
		Bucket:  "bucket",
		Builder: "builder",
	}

	const buildID = 55555
	assert.NoErr(t, datastore.Put(ctx, &model.Build{
		ID: buildID,
		Proto: &pb.Build{
			Builder: builder,
		},
		Tags: []string{"fetched:for:real"},
	}))

	t.Run("ok", func(t *testing.T) {
		const planID = "1234"

		orch := &turboci.FakeOrchestratorClient{
			PlanID: planID,
			Token:  "creator-token",
			QueryNodesResponse: mockQueryNodesResponse(
				planID,
				rootBuildStageID,
				orchestratorpb.StageState_STAGE_STATE_ATTEMPTING,
				buildID,
				nil,
			),
		}

		req := &pb.ScheduleBuildRequest{
			Builder:   builder,
			RequestId: "some-request-id",
		}
		build := &model.Build{
			Proto: &pb.Build{
				Builder: builder,
			},
		}
		err := launchTurboCIRoot(ctx, req, build, orch)
		assert.NoErr(t, err)

		assert.That(t, orch.LastCreateCall, should.Match(orchestratorpb.CreateWorkPlanRequest_builder{
			Realm:          proto.String("project:bucket"),
			IdempotencyKey: proto.String("QHLI8/5YpFrfwDcIkQ4gFBZmltK8ttPTZS0tCdfdIrY="),
		}.Build()))

		assert.That(t, orch.LastWriteNodesCall, should.Match(orchestratorpb.WriteNodesRequest_builder{
			Token: proto.String(orch.Token),
			Stages: []*orchestratorpb.WriteNodesRequest_StageWrite{
				orchestratorpb.WriteNodesRequest_StageWrite_builder{
					Identifier: idspb.Stage_builder{
						Id:         proto.String(rootBuildStageID),
						IsWorknode: proto.Bool(false),
					}.Build(),
					Realm: proto.String("project:bucket"),
					Args: data.Value(&pb.ScheduleBuildRequest{
						Builder:   builder,
						RequestId: "some-request-id",
					}),
				}.Build(),
			},
		}.Build()))

		assert.That(t, orch.LastQueryNodesRequest, should.Match(orchestratorpb.QueryNodesRequest_builder{
			Token: proto.String(orch.Token),
			TypeInfo: orchestratorpb.QueryNodesRequest_TypeInfo_builder{
				Wanted: []string{"type.googleapis.com/buildbucket.v2.BuildStageDetails"},
			}.Build(),
			Query: []*orchestratorpb.Query{
				orchestratorpb.Query_builder{
					Select: orchestratorpb.Query_Select_builder{
						Nodes: []*idspb.Identifier{
							idspb.Identifier_builder{
								Stage: idspb.Stage_builder{
									Id:         proto.String(rootBuildStageID),
									IsWorknode: proto.Bool(false),
								}.Build(),
							}.Build(),
						},
					}.Build(),
				}.Build(),
			},
		}.Build()))

		// Updated `build` in-place.
		assert.That(t, build.ID, should.Equal(int64(buildID)))
		assert.That(t, build.Tags, should.Match([]string{"fetched:for:real"}))
	})
}

func TestPollingSchedule(t *testing.T) {
	t.Parallel()

	t.Run("Errs", func(t *testing.T) {
		ctx := mathrand.Set(t.Context(), rand.New(rand.NewSource(12345)))
		var delay []int64
		for err := range 10 {
			delay = append(delay, pollingSchedule(ctx, 0, err+1).Milliseconds())
		}
		assert.That(t, delay, should.Match([]int64{
			8,
			18,
			36,
			85,
			188,
			260,
			610,
			1528,
			2935,
			4976,
		}))
	})
}

// mockQueryNodesResponse prepares mock QueryNodesResponse.
//
// If buildID is negative, the stage will have no attempts at all.
func mockQueryNodesResponse(planID, stageID string, state orchestratorpb.StageState, buildID int64, buildErr error) *orchestratorpb.QueryNodesResponse {
	var attempts []*orchestratorpb.Stage_Attempt
	if buildID >= 0 {
		var details []*orchestratorpb.Value
		if buildID != 0 || buildErr != nil {
			details = append(details, buildStageDetails(buildID, buildErr))
		}
		attempts = append(attempts,
			// Ignored previous attempt.
			orchestratorpb.Stage_Attempt_builder{
				State: orchestratorpb.StageAttemptState_STAGE_ATTEMPT_STATE_INCOMPLETE.Enum(),
				Details: []*orchestratorpb.Value{
					buildStageDetails(0, status.Errorf(codes.Internal, "must be ignored")),
				},
			}.Build(),
			// The current attempt.
			orchestratorpb.Stage_Attempt_builder{
				State:   orchestratorpb.StageAttemptState_STAGE_ATTEMPT_STATE_PENDING.Enum(),
				Details: details,
			}.Build(),
		)
	}
	return orchestratorpb.QueryNodesResponse_builder{
		Graph: map[string]*orchestratorpb.GraphView{
			planID: orchestratorpb.GraphView_builder{
				Identifier: id.Workplan(planID),
				Stages: map[string]*orchestratorpb.StageView{
					stageID: orchestratorpb.StageView_builder{
						Stage: orchestratorpb.Stage_builder{
							Identifier: id.SetWorkplan(id.Stage(stageID), planID),
							State:      state.Enum(),
							Attempts:   attempts,
						}.Build(),
					}.Build(),
				},
			}.Build(),
		},
	}.Build()
}

func buildStageDetails(buildID int64, buildErr error) *orchestratorpb.Value {
	msg := &pb.BuildStageDetails{}
	if buildID != 0 {
		msg.Result = &pb.BuildStageDetails_Id{
			Id: buildID,
		}
	} else {
		msg.Result = &pb.BuildStageDetails_Error{
			Error: status.Convert(buildErr).Proto(),
		}
	}
	return data.Value(msg)
}
