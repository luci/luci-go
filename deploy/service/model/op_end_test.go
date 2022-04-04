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

func TestActuationEndOp(t *testing.T) {
	t.Parallel()

	Convey("With datastore", t, func() {
		now := testclock.TestRecentTimeUTC.Round(time.Millisecond)
		ctx, _ := testclock.UseTime(context.Background(), now)
		ctx = memory.Use(ctx)

		Convey("Missing assets", func() {
			_, err := NewActuationEndOp(ctx, []string{"apps/missing"}, &Actuation{})
			So(err, ShouldErrLike, "assets entities unexpectedly missing: apps/missing")
		})

		Convey("Works", func() {
			assets := []*Asset{
				{
					ID: "apps/app1",
					Asset: &modelpb.Asset{
						Id: "apps/app1",
						LastActuation: &modelpb.Actuation{
							Id:    "actuation-id",
							State: modelpb.Actuation_EXECUTING,
						},
						LastActuateActuation: &modelpb.Actuation{
							Id:    "actuation-id",
							State: modelpb.Actuation_EXECUTING,
						},
						ReportedState: &modelpb.AssetState{
							State: &modelpb.AssetState_Appengine{
								Appengine: mockedReportedState("old reported", 0),
							},
						},
						ActuatedState: &modelpb.AssetState{
							State: &modelpb.AssetState_Appengine{
								Appengine: mockedReportedState("old actuated", 0),
							},
						},
					},
				},
				{
					ID: "apps/app2",
					Asset: &modelpb.Asset{
						Id: "apps/app2",
						LastActuation: &modelpb.Actuation{
							Id: "another-actuation",
						},
						LastActuateActuation: &modelpb.Actuation{
							Id: "another-actuation",
						},
					},
				},
			}
			So(datastore.Put(ctx, assets), ShouldBeNil)

			op, err := NewActuationEndOp(ctx, []string{"apps/app1", "apps/app2"}, &Actuation{
				ID: "actuation-id",
				Actuation: &modelpb.Actuation{
					Id:         "actuation-id",
					Deployment: mockedDeployment,
					Actuator:   mockedActuator,
					State:      modelpb.Actuation_EXECUTING,
					LogUrl:     "old-log-url",
				},
			})
			So(err, ShouldBeNil)

			Convey("Success", func() {
				op.UpdateActuationStatus(ctx, nil, "new-log-url")
				op.HandleActuatedState(ctx, "apps/app1", &rpcpb.ActuatedAsset{
					State: &modelpb.AssetState{
						State: &modelpb.AssetState_Appengine{
							Appengine: mockedReportedState("new actuated", 0),
						},
					},
				})
				op.HandleActuatedState(ctx, "apps/app2", &rpcpb.ActuatedAsset{
					State: &modelpb.AssetState{
						State: &modelpb.AssetState_Appengine{
							Appengine: mockedReportedState("new actuated", 0),
						},
					},
				})

				So(op.Apply(ctx), ShouldBeNil)

				// Updated Actuation entity.
				storedActuation := &Actuation{ID: "actuation-id"}
				So(datastore.Get(ctx, storedActuation), ShouldBeNil)
				So(storedActuation.Actuation, ShouldResembleProto, &modelpb.Actuation{
					Id:         "actuation-id",
					State:      modelpb.Actuation_SUCCEEDED,
					Deployment: mockedDeployment,
					Actuator:   mockedActuator,
					Finished:   timestamppb.New(now),
					LogUrl:     "new-log-url",
				})
				So(storedActuation.State, ShouldEqual, modelpb.Actuation_SUCCEEDED)

				// Updated the asset assigned to this actuation.
				assets, err := fetchAssets(ctx, []string{"apps/app1", "apps/app2"}, true)
				So(err, ShouldBeNil)

				So(assets["apps/app1"].Asset, ShouldResembleProto, &modelpb.Asset{
					Id:                   "apps/app1",
					LastActuation:        storedActuation.Actuation,
					LastActuateActuation: storedActuation.Actuation,
					ReportedState: &modelpb.AssetState{
						Timestamp:  timestamppb.New(now),
						Deployment: storedActuation.Actuation.Deployment,
						Actuator:   storedActuation.Actuation.Actuator,
						State: &modelpb.AssetState_Appengine{
							Appengine: mockedReportedState("new actuated", 0),
						},
					},
					ActuatedState: &modelpb.AssetState{
						Timestamp:  timestamppb.New(now),
						Deployment: storedActuation.Actuation.Deployment,
						Actuator:   storedActuation.Actuation.Actuator,
						State: &modelpb.AssetState_Appengine{
							Appengine: mockedReportedState("new actuated", 0),
						},
					},
				})

				// Wasn't touched.
				So(assets["apps/app2"].Asset, ShouldResembleProto, &modelpb.Asset{
					Id: "apps/app2",
					LastActuation: &modelpb.Actuation{
						Id: "another-actuation",
					},
					LastActuateActuation: &modelpb.Actuation{
						Id: "another-actuation",
					},
				})
			})

			Convey("Failed", func() {
				op.UpdateActuationStatus(ctx, &statuspb.Status{
					Code:    int32(codes.FailedPrecondition),
					Message: "actuation boom",
				}, "")
				op.HandleActuatedState(ctx, "apps/app1", &rpcpb.ActuatedAsset{
					State: &modelpb.AssetState{
						Status: &statuspb.Status{
							Code:    int32(codes.InvalidArgument),
							Message: "status boom",
						},
					},
				})
				op.HandleActuatedState(ctx, "apps/app2", &rpcpb.ActuatedAsset{
					State: &modelpb.AssetState{
						Status: &statuspb.Status{
							Code:    int32(codes.InvalidArgument),
							Message: "status boom",
						},
					},
				})

				So(op.Apply(ctx), ShouldBeNil)

				// Updated Actuation entity.
				storedActuation := &Actuation{ID: "actuation-id"}
				So(datastore.Get(ctx, storedActuation), ShouldBeNil)
				So(storedActuation.Actuation, ShouldResembleProto, &modelpb.Actuation{
					Id:    "actuation-id",
					State: modelpb.Actuation_FAILED,
					Status: &statuspb.Status{
						Code:    int32(codes.FailedPrecondition),
						Message: "actuation boom",
					},
					Deployment: mockedDeployment,
					Actuator:   mockedActuator,
					Finished:   timestamppb.New(now),
					LogUrl:     "old-log-url",
				})
				So(storedActuation.State, ShouldEqual, modelpb.Actuation_FAILED)

				// Updated the asset assigned to this actuation.
				assets, err := fetchAssets(ctx, []string{"apps/app1", "apps/app2"}, true)
				So(err, ShouldBeNil)

				So(assets["apps/app1"].Asset, ShouldResembleProto, &modelpb.Asset{
					Id:                   "apps/app1",
					LastActuation:        storedActuation.Actuation,
					LastActuateActuation: storedActuation.Actuation,
					ReportedState: &modelpb.AssetState{
						State: &modelpb.AssetState_Appengine{
							Appengine: mockedReportedState("old reported", 0),
						},
					},
					ActuatedState: &modelpb.AssetState{
						State: &modelpb.AssetState_Appengine{
							Appengine: mockedReportedState("old actuated", 0),
						},
					},
					PostActuationStatus: &statuspb.Status{
						Code:    int32(codes.InvalidArgument),
						Message: "status boom",
					},
				})

				// Wasn't touched.
				So(assets["apps/app2"].Asset, ShouldResembleProto, &modelpb.Asset{
					Id: "apps/app2",
					LastActuation: &modelpb.Actuation{
						Id: "another-actuation",
					},
					LastActuateActuation: &modelpb.Actuation{
						Id: "another-actuation",
					},
				})
			})
		})
	})
}
