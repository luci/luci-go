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

package rpcs

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/luci/deploy/api/modelpb"
	"go.chromium.org/luci/deploy/api/rpcpb"
)

func TestActuationsRPC(t *testing.T) {
	t.Parallel()

	ftt.Run("With server", t, func(t *ftt.Test) {
		now := testclock.TestRecentTimeUTC.Round(time.Millisecond)
		ctx, _ := testclock.UseTime(context.Background(), now)
		ctx = memory.Use(ctx)

		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity: "user:actuator-1@example.com",
		})

		beginReq := func(want, have int32) *rpcpb.BeginActuationRequest {
			return &rpcpb.BeginActuationRequest{
				Actuation: &modelpb.Actuation{
					Id:         "some-actuation",
					Deployment: &modelpb.Deployment{RepoRev: "mocked-deployment"},
					Actuator:   &modelpb.ActuatorInfo{Identity: "mocked-actuator"},
				},
				Assets: map[string]*rpcpb.AssetToActuate{
					"apps/app": {
						Config: &modelpb.AssetConfig{
							EnableAutomation: true,
						},
						IntendedState: intendedState(want),
						ReportedState: reportedState(have),
					},
				},
			}
		}

		endReq := func(have int32) *rpcpb.EndActuationRequest {
			return &rpcpb.EndActuationRequest{
				ActuationId: "some-actuation",
				Assets: map[string]*rpcpb.ActuatedAsset{
					"apps/app": {
						State: reportedState(have),
					},
				},
			}
		}

		srv := &Actuations{}

		t.Run("Begin + End", func(t *ftt.Test) {
			beginResp, err := srv.BeginActuation(ctx, beginReq(0, 1000))
			assert.Loosely(t, err, should.BeNil)

			assert.Loosely(t, beginResp, should.Resemble(&rpcpb.BeginActuationResponse{
				Decisions: map[string]*modelpb.ActuationDecision{
					"apps/app": {Decision: modelpb.ActuationDecision_ACTUATE_STALE},
				},
			}))

			_, err = srv.EndActuation(ctx, endReq(1000))
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run("Begin retry", func(t *ftt.Test) {
			_, err := srv.BeginActuation(ctx, beginReq(0, 1000))
			assert.Loosely(t, err, should.BeNil)
			beginResp, err := srv.BeginActuation(ctx, beginReq(0, 1000))
			assert.Loosely(t, err, should.BeNil)

			assert.Loosely(t, beginResp, should.Resemble(&rpcpb.BeginActuationResponse{
				Decisions: map[string]*modelpb.ActuationDecision{
					"apps/app": {Decision: modelpb.ActuationDecision_ACTUATE_STALE},
				},
			}))
		})

		t.Run("End retry", func(t *ftt.Test) {
			_, err := srv.BeginActuation(ctx, beginReq(0, 1000))
			assert.Loosely(t, err, should.BeNil)
			_, err = srv.EndActuation(ctx, endReq(1000))
			assert.Loosely(t, err, should.BeNil)
			_, err = srv.EndActuation(ctx, endReq(1000))
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run("Begin retry: wrong caller", func(t *ftt.Test) {
			_, err := srv.BeginActuation(ctx, beginReq(0, 1000))
			assert.Loosely(t, err, should.BeNil)

			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "user:another-actuator@example.com",
			})

			_, err = srv.BeginActuation(ctx, beginReq(0, 1000))
			assert.Loosely(t, err, grpccode.ShouldBe(codes.FailedPrecondition))
		})

		t.Run("End: missing actuation", func(t *ftt.Test) {
			_, err := srv.EndActuation(ctx, endReq(1000))
			assert.Loosely(t, err, grpccode.ShouldBe(codes.NotFound))
		})

		t.Run("End wrong caller", func(t *ftt.Test) {
			_, err := srv.BeginActuation(ctx, beginReq(0, 1000))
			assert.Loosely(t, err, should.BeNil)

			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "user:another-actuator@example.com",
			})

			_, err = srv.EndActuation(ctx, endReq(1000))
			assert.Loosely(t, err, grpccode.ShouldBe(codes.FailedPrecondition))
		})

		t.Run("End wrong asset list", func(t *ftt.Test) {
			_, err := srv.BeginActuation(ctx, beginReq(0, 1000))
			assert.Loosely(t, err, should.BeNil)

			req := endReq(1000)
			req.Assets["apps/another"] = &rpcpb.ActuatedAsset{
				State: reportedState(1000),
			}

			_, err = srv.EndActuation(ctx, req)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
		})
	})
}

func TestActuationsValidation(t *testing.T) {
	t.Parallel()

	ftt.Run("validateBeginActuation", t, func(t *ftt.Test) {
		rpc := &rpcpb.BeginActuationRequest{
			Actuation: &modelpb.Actuation{
				Id:         "some-actuation",
				Deployment: &modelpb.Deployment{RepoRev: "mocked-deployment"},
				Actuator:   &modelpb.ActuatorInfo{Identity: "mocked-actuator"},
			},
			Assets: map[string]*rpcpb.AssetToActuate{
				"apps/app1": {
					Config: &modelpb.AssetConfig{
						EnableAutomation: true,
					},
					IntendedState: intendedState(0),
					ReportedState: reportedState(0),
				},
				"apps/app2": {
					Config: &modelpb.AssetConfig{
						EnableAutomation: true,
					},
					IntendedState: intendedState(0),
					ReportedState: reportedState(0),
				},
			},
		}

		t.Run("OK", func(t *ftt.Test) {
			assets, err := validateBeginActuation(rpc)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, assets, should.Resemble([]string{"apps/app1", "apps/app2"}))
		})

		t.Run("No id", func(t *ftt.Test) {
			rpc.Actuation.Id = ""
			_, err := validateBeginActuation(rpc)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
		})

		t.Run("No deployment", func(t *ftt.Test) {
			rpc.Actuation.Deployment = nil
			_, err := validateBeginActuation(rpc)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
		})

		t.Run("No actuator", func(t *ftt.Test) {
			rpc.Actuation.Actuator = nil
			_, err := validateBeginActuation(rpc)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
		})

		t.Run("No assets", func(t *ftt.Test) {
			rpc.Assets = nil
			_, err := validateBeginActuation(rpc)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
		})
	})

	ftt.Run("validateEndActuation", t, func(t *ftt.Test) {
		rpc := &rpcpb.EndActuationRequest{
			ActuationId: "some-actuation",
			Assets: map[string]*rpcpb.ActuatedAsset{
				"apps/app1": {
					State: reportedState(1000),
				},
				"apps/app2": {
					State: reportedState(1000),
				},
			},
		}

		t.Run("OK", func(t *ftt.Test) {
			assets, err := validateEndActuation(rpc)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, assets, should.Resemble([]string{"apps/app1", "apps/app2"}))
		})

		t.Run("No id", func(t *ftt.Test) {
			rpc.ActuationId = ""
			_, err := validateEndActuation(rpc)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
		})

		t.Run("No assets", func(t *ftt.Test) {
			rpc.Assets = nil
			_, err := validateEndActuation(rpc)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
		})
	})
}

func intendedState(traffic int32) *modelpb.AssetState {
	return &modelpb.AssetState{
		State: &modelpb.AssetState_Appengine{
			Appengine: &modelpb.AppengineState{
				IntendedState: &modelpb.AppengineState_IntendedState{},
				Services: []*modelpb.AppengineState_Service{
					{
						Name: "default",
						TrafficAllocation: map[string]int32{
							"version1": traffic,
							"version2": 1000 - traffic,
						},
						TrafficSplitting: modelpb.AppengineState_Service_COOKIE,
						Versions: []*modelpb.AppengineState_Service_Version{
							{
								Name:          "version1",
								IntendedState: &modelpb.AppengineState_Service_Version_IntendedState{},
							},
							{
								Name:          "version2",
								IntendedState: &modelpb.AppengineState_Service_Version_IntendedState{},
							},
						},
					},
				},
			},
		},
	}
}

func reportedState(traffic int32) *modelpb.AssetState {
	return &modelpb.AssetState{
		State: &modelpb.AssetState_Appengine{
			Appengine: &modelpb.AppengineState{
				CapturedState: &modelpb.AppengineState_CapturedState{},
				Services: []*modelpb.AppengineState_Service{
					{
						Name: "default",
						TrafficAllocation: map[string]int32{
							"version1": traffic,
							"version2": 1000 - traffic,
						},
						Versions: []*modelpb.AppengineState_Service_Version{
							{
								Name:          "version1",
								CapturedState: &modelpb.AppengineState_Service_Version_CapturedState{},
							},
							{
								Name:          "version2",
								CapturedState: &modelpb.AppengineState_Service_Version_CapturedState{},
							},
						},
					},
				},
			},
		},
	}
}
