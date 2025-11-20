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
	"testing"

	"golang.org/x/oauth2"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"

	idspb "go.chromium.org/turboci/proto/go/graph/ids/v1"
	orchestratorpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1"

	"go.chromium.org/luci/auth/scopes"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/luci/buildbucket/appengine/model"
	pb "go.chromium.org/luci/buildbucket/proto"
)

func TestLaunchTurboCIRoot(t *testing.T) {
	t.Parallel()

	ctx := auth.WithState(t.Context(), &authtest.FakeState{
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

	orch := &fakeOrchestratorClient{
		planID: "1234",
	}

	t.Run("ok", func(t *testing.T) {
		req := &pb.ScheduleBuildRequest{
			Builder:   builder,
			RequestId: "some-request-id",
		}
		build := &model.Build{
			Proto: &pb.Build{
				Builder: builder,
			},
		}
		// Ignore the error for now, it is Unimplemented.
		_ = launchTurboCIRoot(ctx, req, build, orch)
		assert.That(t, orch.lastCreateCall, should.Match(orchestratorpb.CreateWorkPlanRequest_builder{
			Realm:          proto.String("project:bucket"),
			IdempotencyKey: proto.String("QHLI8/5YpFrfwDcIkQ4gFBZmltK8ttPTZS0tCdfdIrY="),
		}.Build()))
	})
}

type fakeOrchestratorClient struct {
	lastCreateCall *orchestratorpb.CreateWorkPlanRequest
	planID         string
}

func (o *fakeOrchestratorClient) CreateWorkPlan(ctx context.Context, in *orchestratorpb.CreateWorkPlanRequest, opts ...grpc.CallOption) (*orchestratorpb.CreateWorkPlanResponse, error) {
	o.lastCreateCall = proto.Clone(in).(*orchestratorpb.CreateWorkPlanRequest)
	return orchestratorpb.CreateWorkPlanResponse_builder{
		Identifier: idspb.WorkPlan_builder{
			Id: proto.String(o.planID),
		}.Build(),
	}.Build(), nil
}

func (o *fakeOrchestratorClient) WriteNodes(ctx context.Context, in *orchestratorpb.WriteNodesRequest, opts ...grpc.CallOption) (*orchestratorpb.WriteNodesResponse, error) {
	panic("should not be called yet")
}

func (o *fakeOrchestratorClient) QueryNodes(ctx context.Context, in *orchestratorpb.QueryNodesRequest, opts ...grpc.CallOption) (*orchestratorpb.QueryNodesResponse, error) {
	panic("should not be called yet")
}
