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

package model

import (
	"context"
	"testing"
	"time"

	statuspb "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/deploy/api/modelpb"
	"go.chromium.org/luci/deploy/api/rpcpb"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

var (
	mockedDeployment = &modelpb.Deployment{RepoRev: "mocked-deployment"}
	mockedActuator   = &modelpb.ActuatorInfo{Identity: "mocked-actuator"}
	mockedTriggers   = []*modelpb.ActuationTrigger{{}, {}}
)

func mockedIntendedState(payload string, traffic int32) *modelpb.AppengineState {
	return &modelpb.AppengineState{
		IntendedState: &modelpb.AppengineState_IntendedState{
			DeployableYamls: []*modelpb.AppengineState_IntendedState_DeployableYaml{
				{YamlPath: payload},
			},
		},
		Services: []*modelpb.AppengineState_Service{
			{
				Name:             "default",
				TrafficSplitting: modelpb.AppengineState_Service_COOKIE,
				TrafficAllocation: map[string]int32{
					"ver1": traffic,
					"ver2": 1000 - traffic,
				},
				Versions: []*modelpb.AppengineState_Service_Version{
					{
						Name:          "ver1",
						IntendedState: &modelpb.AppengineState_Service_Version_IntendedState{},
					},
					{
						Name:          "ver2",
						IntendedState: &modelpb.AppengineState_Service_Version_IntendedState{},
					},
				},
			},
		},
	}
}

func mockedReportedState(payload string, traffic int32) *modelpb.AppengineState {
	return &modelpb.AppengineState{
		CapturedState: &modelpb.AppengineState_CapturedState{
			LocationId: payload,
		},
		Services: []*modelpb.AppengineState_Service{
			{
				Name: "default",
				TrafficAllocation: map[string]int32{
					"ver1": traffic,
					"ver2": 1000 - traffic,
				},
				Versions: []*modelpb.AppengineState_Service_Version{
					{
						Name:          "ver1",
						CapturedState: &modelpb.AppengineState_Service_Version_CapturedState{},
					},
					{
						Name:          "ver2",
						CapturedState: &modelpb.AppengineState_Service_Version_CapturedState{},
					},
				},
			},
		},
	}
}

func TestActuationBeginOp(t *testing.T) {
	t.Parallel()

	Convey("With datastore", t, func() {
		now := testclock.TestRecentTimeUTC.Round(time.Millisecond)
		ctx, _ := testclock.UseTime(context.Background(), now)
		ctx = memory.Use(ctx)

		Convey("Executing", func() {
			op, err := NewActuationBeginOp(ctx, []string{"apps/app1", "apps/app2"}, &modelpb.Actuation{
				Id:         "actuation-id",
				Deployment: mockedDeployment,
				Actuator:   mockedActuator,
				Triggers:   mockedTriggers,
			})
			So(err, ShouldBeNil)

			op.MakeDecision(ctx, "apps/app1", &rpcpb.AssetToActuate{
				Config: &modelpb.AssetConfig{EnableAutomation: false},
				IntendedState: &modelpb.AssetState{
					State: &modelpb.AssetState_Appengine{
						Appengine: mockedIntendedState("app1", 0),
					},
				},
				ReportedState: &modelpb.AssetState{
					State: &modelpb.AssetState_Appengine{
						Appengine: mockedReportedState("app1", 200),
					},
				},
			})

			op.MakeDecision(ctx, "apps/app2", &rpcpb.AssetToActuate{
				Config: &modelpb.AssetConfig{EnableAutomation: true},
				IntendedState: &modelpb.AssetState{
					State: &modelpb.AssetState_Appengine{
						Appengine: mockedIntendedState("app2", 0),
					},
				},
				ReportedState: &modelpb.AssetState{
					State: &modelpb.AssetState_Appengine{
						Appengine: mockedReportedState("app2", 200),
					},
				},
			})

			decisions, err := op.Apply(ctx)
			So(err, ShouldBeNil)

			// Returned decisions are correct.
			So(decisions, ShouldHaveLength, 2)
			So(decisions["apps/app1"], ShouldResembleProto, &modelpb.ActuationDecision{
				Decision: modelpb.ActuationDecision_SKIP_DISABLED,
			})
			So(decisions["apps/app2"], ShouldResembleProto, &modelpb.ActuationDecision{
				Decision: modelpb.ActuationDecision_ACTUATE_STALE,
			})

			// Stored Actuation entity is correct.
			storedActuation := &Actuation{ID: "actuation-id"}
			So(datastore.Get(ctx, storedActuation), ShouldBeNil)
			So(storedActuation.Actuation, ShouldResembleProto, &modelpb.Actuation{
				Id:         "actuation-id",
				State:      modelpb.Actuation_EXECUTING,
				Deployment: mockedDeployment,
				Actuator:   mockedActuator,
				Triggers:   mockedTriggers,
				Created:    timestamppb.New(now),
				Expiry:     timestamppb.New(now.Add(ActuationExpiry)),
			})
			So(storedActuation.Decisions, ShouldResembleProto, &modelpb.ActuationDecisions{
				Decisions: map[string]*modelpb.ActuationDecision{
					"apps/app1": {Decision: modelpb.ActuationDecision_SKIP_DISABLED},
					"apps/app2": {Decision: modelpb.ActuationDecision_ACTUATE_STALE},
				},
			})
			So(storedActuation.State, ShouldEqual, modelpb.Actuation_EXECUTING)
			So(storedActuation.Created.Equal(now), ShouldBeTrue)
			So(storedActuation.Expiry.Equal(now.Add(ActuationExpiry)), ShouldBeTrue)

			// Stored Asset entities are correct.
			assets, err := fetchAssets(ctx, []string{"apps/app1", "apps/app2"}, true)
			So(err, ShouldBeNil)
			So(assets["apps/app1"].Asset, ShouldResembleProto, &modelpb.Asset{
				Id:            "apps/app1",
				LastActuation: storedActuation.Actuation,
				LastDecision:  decisions["apps/app1"],
				Config:        &modelpb.AssetConfig{EnableAutomation: false},
				IntendedState: &modelpb.AssetState{
					Timestamp:  timestamppb.New(now),
					Deployment: storedActuation.Actuation.Deployment,
					Actuator:   storedActuation.Actuation.Actuator,
					State: &modelpb.AssetState_Appengine{
						Appengine: mockedIntendedState("app1", 0),
					},
				},
				ReportedState: &modelpb.AssetState{
					Timestamp:  timestamppb.New(now),
					Deployment: storedActuation.Actuation.Deployment,
					Actuator:   storedActuation.Actuation.Actuator,
					State: &modelpb.AssetState_Appengine{
						Appengine: mockedReportedState("app1", 200),
					},
				},
			})

			So(assets["apps/app2"].Asset, ShouldResembleProto, &modelpb.Asset{
				Id:            "apps/app2",
				LastActuation: storedActuation.Actuation,
				LastDecision:  decisions["apps/app2"],
				Config:        &modelpb.AssetConfig{EnableAutomation: true},
				IntendedState: &modelpb.AssetState{
					Timestamp:  timestamppb.New(now),
					Deployment: storedActuation.Actuation.Deployment,
					Actuator:   storedActuation.Actuation.Actuator,
					State: &modelpb.AssetState_Appengine{
						Appengine: mockedIntendedState("app2", 0),
					},
				},
				ReportedState: &modelpb.AssetState{
					Timestamp:  timestamppb.New(now),
					Deployment: storedActuation.Actuation.Deployment,
					Actuator:   storedActuation.Actuation.Actuator,
					State: &modelpb.AssetState_Appengine{
						Appengine: mockedReportedState("app2", 200),
					},
				},
			})
		})

		Convey("Skipping disabled", func() {
			op, err := NewActuationBeginOp(ctx, []string{"apps/app1"}, &modelpb.Actuation{
				Id:         "actuation-id",
				Deployment: mockedDeployment,
				Actuator:   mockedActuator,
				Triggers:   mockedTriggers,
			})
			So(err, ShouldBeNil)

			op.MakeDecision(ctx, "apps/app1", &rpcpb.AssetToActuate{
				Config: &modelpb.AssetConfig{EnableAutomation: false},
				IntendedState: &modelpb.AssetState{
					State: &modelpb.AssetState_Appengine{
						Appengine: mockedIntendedState("app1", 0),
					},
				},
				ReportedState: &modelpb.AssetState{
					State: &modelpb.AssetState_Appengine{
						Appengine: mockedReportedState("app1", 200),
					},
				},
			})

			_, err = op.Apply(ctx)
			So(err, ShouldBeNil)

			// Stored Actuation entity is correct.
			storedActuation := &Actuation{ID: "actuation-id"}
			So(datastore.Get(ctx, storedActuation), ShouldBeNil)
			So(storedActuation.Actuation, ShouldResembleProto, &modelpb.Actuation{
				Id:         "actuation-id",
				State:      modelpb.Actuation_SUCCEEDED,
				Deployment: mockedDeployment,
				Actuator:   mockedActuator,
				Triggers:   mockedTriggers,
				Created:    timestamppb.New(now),
				Finished:   timestamppb.New(now),
			})
			So(storedActuation.Decisions, ShouldResembleProto, &modelpb.ActuationDecisions{
				Decisions: map[string]*modelpb.ActuationDecision{
					"apps/app1": {Decision: modelpb.ActuationDecision_SKIP_DISABLED},
				},
			})
			So(storedActuation.State, ShouldEqual, modelpb.Actuation_SUCCEEDED)
			So(storedActuation.Created.Equal(now), ShouldBeTrue)
			So(storedActuation.Expiry.IsZero(), ShouldBeTrue)
		})

		Convey("Skipping up-to-date", func() {
			op, err := NewActuationBeginOp(ctx, []string{"apps/app1"}, &modelpb.Actuation{
				Id:         "actuation-id",
				Deployment: mockedDeployment,
				Actuator:   mockedActuator,
				Triggers:   mockedTriggers,
			})
			So(err, ShouldBeNil)

			op.MakeDecision(ctx, "apps/app1", &rpcpb.AssetToActuate{
				Config: &modelpb.AssetConfig{EnableAutomation: true},
				IntendedState: &modelpb.AssetState{
					State: &modelpb.AssetState_Appengine{
						Appengine: mockedIntendedState("app1", 0),
					},
				},
				ReportedState: &modelpb.AssetState{
					State: &modelpb.AssetState_Appengine{
						Appengine: mockedReportedState("app1", 0),
					},
				},
			})

			_, err = op.Apply(ctx)
			So(err, ShouldBeNil)

			// Stored Actuation entity is correct.
			storedActuation := &Actuation{ID: "actuation-id"}
			So(datastore.Get(ctx, storedActuation), ShouldBeNil)
			So(storedActuation.Actuation, ShouldResembleProto, &modelpb.Actuation{
				Id:         "actuation-id",
				State:      modelpb.Actuation_SUCCEEDED,
				Deployment: mockedDeployment,
				Actuator:   mockedActuator,
				Triggers:   mockedTriggers,
				Created:    timestamppb.New(now),
				Finished:   timestamppb.New(now),
			})
			So(storedActuation.Decisions, ShouldResembleProto, &modelpb.ActuationDecisions{
				Decisions: map[string]*modelpb.ActuationDecision{
					"apps/app1": {Decision: modelpb.ActuationDecision_SKIP_UPTODATE},
				},
			})
			So(storedActuation.State, ShouldEqual, modelpb.Actuation_SUCCEEDED)
			So(storedActuation.Created.Equal(now), ShouldBeTrue)
			So(storedActuation.Expiry.IsZero(), ShouldBeTrue)
		})

		Convey("Broken", func() {
			op, err := NewActuationBeginOp(ctx, []string{"apps/app1"}, &modelpb.Actuation{
				Id:         "actuation-id",
				Deployment: mockedDeployment,
				Actuator:   mockedActuator,
				Triggers:   mockedTriggers,
			})
			So(err, ShouldBeNil)

			op.MakeDecision(ctx, "apps/app1", &rpcpb.AssetToActuate{
				Config: &modelpb.AssetConfig{EnableAutomation: true},
				IntendedState: &modelpb.AssetState{
					Status: &statuspb.Status{
						Code:    int32(codes.FailedPrecondition),
						Message: "intended broken",
					},
				},
				ReportedState: &modelpb.AssetState{
					Status: &statuspb.Status{
						Code:    int32(codes.FailedPrecondition),
						Message: "reported broken",
					},
				},
			})

			_, err = op.Apply(ctx)
			So(err, ShouldBeNil)

			// Stored Actuation entity is correct.
			storedActuation := &Actuation{ID: "actuation-id"}
			So(datastore.Get(ctx, storedActuation), ShouldBeNil)
			So(storedActuation.Actuation, ShouldResembleProto, &modelpb.Actuation{
				Id:         "actuation-id",
				State:      modelpb.Actuation_FAILED,
				Deployment: mockedDeployment,
				Actuator:   mockedActuator,
				Triggers:   mockedTriggers,
				Created:    timestamppb.New(now),
				Finished:   timestamppb.New(now),
				Status: &statuspb.Status{
					Code: int32(codes.Internal),
					Message: "asset \"apps/app1\": failed to collect intended state: " +
						"rpc error: code = FailedPrecondition desc = intended broken; " +
						"asset \"apps/app1\": failed to collect reported state: rpc error: " +
						"code = FailedPrecondition desc = reported broken",
				},
			})
			So(storedActuation.Decisions, ShouldResembleProto, &modelpb.ActuationDecisions{
				Decisions: map[string]*modelpb.ActuationDecision{
					"apps/app1": {
						Decision: modelpb.ActuationDecision_SKIP_BROKEN,
						Status: &statuspb.Status{
							Code:    int32(codes.FailedPrecondition),
							Message: "reported broken",
						},
					},
				},
			})
			So(storedActuation.State, ShouldEqual, modelpb.Actuation_FAILED)
			So(storedActuation.Created.Equal(now), ShouldBeTrue)
			So(storedActuation.Expiry.IsZero(), ShouldBeTrue)
		})
	})
}
