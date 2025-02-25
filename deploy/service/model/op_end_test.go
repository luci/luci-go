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
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/deploy/api/modelpb"
	"go.chromium.org/luci/deploy/api/rpcpb"
)

func TestActuationEndOp(t *testing.T) {
	t.Parallel()

	ftt.Run("With datastore", t, func(t *ftt.Test) {
		now := testclock.TestRecentTimeUTC.Round(time.Millisecond)
		ctx, _ := testclock.UseTime(context.Background(), now)
		ctx = memory.Use(ctx)

		t.Run("Missing assets", func(t *ftt.Test) {
			_, err := NewActuationEndOp(ctx, &Actuation{
				Decisions: &modelpb.ActuationDecisions{
					Decisions: map[string]*modelpb.ActuationDecision{
						"apps/missing": {Decision: modelpb.ActuationDecision_ACTUATE_STALE},
					},
				},
			})
			assert.Loosely(t, err, should.ErrLike("assets entities unexpectedly missing: apps/missing"))
		})

		t.Run("Works", func(t *ftt.Test) {
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
						IntendedState: &modelpb.AssetState{
							State: &modelpb.AssetState_Appengine{
								Appengine: mockedReportedState("intended", 0),
							},
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
					LastHistoryID: 123,
					HistoryEntry: &modelpb.AssetHistory{
						AssetId:   "apps/app1",
						HistoryId: 124, // i.e. being recorded now
						Actuation: &modelpb.Actuation{
							Id: "phony-to-be-overridden",
						},
						PriorConsecutiveFailures: 111,
					},
					ConsecutiveFailures: 111,
				},
				{
					ID: "apps/app2",
					Asset: &modelpb.Asset{
						Id: "apps/app2",
						IntendedState: &modelpb.AssetState{
							State: &modelpb.AssetState_Appengine{
								Appengine: mockedReportedState("intended", 0),
							},
						},
						LastActuation: &modelpb.Actuation{
							Id: "another-actuation",
						},
						LastActuateActuation: &modelpb.Actuation{
							Id: "another-actuation",
						},
					},
					LastHistoryID: 123,
					HistoryEntry: &modelpb.AssetHistory{
						AssetId:   "apps/app2",
						HistoryId: 123,
						Actuation: &modelpb.Actuation{
							Id: "phony-to-be-untouched",
						},
					},
					ConsecutiveFailures: 222,
				},
			}
			assert.Loosely(t, datastore.Put(ctx, assets), should.BeNil)

			op, err := NewActuationEndOp(ctx, &Actuation{
				ID: "actuation-id",
				Actuation: &modelpb.Actuation{
					Id:         "actuation-id",
					Deployment: mockedDeployment,
					Actuator:   mockedActuator,
					State:      modelpb.Actuation_EXECUTING,
					LogUrl:     "old-log-url",
				},
				Decisions: &modelpb.ActuationDecisions{
					Decisions: map[string]*modelpb.ActuationDecision{
						"apps/app1": {Decision: modelpb.ActuationDecision_ACTUATE_STALE},
						"apps/app2": {Decision: modelpb.ActuationDecision_ACTUATE_STALE},
					},
				},
			})
			assert.Loosely(t, err, should.BeNil)

			t.Run("Success", func(t *ftt.Test) {
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

				assert.Loosely(t, op.Apply(ctx), should.BeNil)

				// Updated Actuation entity.
				storedActuation := &Actuation{ID: "actuation-id"}
				assert.Loosely(t, datastore.Get(ctx, storedActuation), should.BeNil)
				assert.Loosely(t, storedActuation.Actuation, should.Match(&modelpb.Actuation{
					Id:         "actuation-id",
					State:      modelpb.Actuation_SUCCEEDED,
					Deployment: mockedDeployment,
					Actuator:   mockedActuator,
					Finished:   timestamppb.New(now),
					LogUrl:     "new-log-url",
				}))
				assert.Loosely(t, storedActuation.State, should.Equal(modelpb.Actuation_SUCCEEDED))

				// Updated the asset assigned to this actuation.
				assets, err := fetchAssets(ctx, []string{"apps/app1", "apps/app2"}, true)
				assert.Loosely(t, err, should.BeNil)

				assert.Loosely(t, assets["apps/app1"].Asset, should.Match(&modelpb.Asset{
					Id:                   "apps/app1",
					LastActuation:        storedActuation.Actuation,
					LastActuateActuation: storedActuation.Actuation,
					IntendedState: &modelpb.AssetState{
						State: &modelpb.AssetState_Appengine{
							Appengine: mockedReportedState("intended", 0),
						},
					},
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
					AppliedState: &modelpb.AssetState{
						State: &modelpb.AssetState_Appengine{
							Appengine: mockedReportedState("intended", 0),
						},
					},
				}))
				assert.Loosely(t, assets["apps/app1"].LastHistoryID, should.Equal(124))
				assert.Loosely(t, assets["apps/app1"].HistoryEntry, should.Match(&modelpb.AssetHistory{
					AssetId:   "apps/app1",
					HistoryId: 124,
					Actuation: storedActuation.Actuation,
					PostActuationState: &modelpb.AssetState{
						Timestamp:  timestamppb.New(now),
						Deployment: storedActuation.Actuation.Deployment,
						Actuator:   storedActuation.Actuation.Actuator,
						State: &modelpb.AssetState_Appengine{
							Appengine: mockedReportedState("new actuated", 0),
						},
					},
					PriorConsecutiveFailures: 111,
				}))
				assert.Loosely(t, assets["apps/app1"].ConsecutiveFailures, should.BeZero)

				// Created the history entity.
				rec := AssetHistory{ID: 124, Parent: datastore.KeyForObj(ctx, assets["apps/app1"])}
				assert.Loosely(t, datastore.Get(ctx, &rec), should.BeNil)
				assert.Loosely(t, rec.Entry, should.Match(assets["apps/app1"].HistoryEntry))

				// Wasn't touched.
				assert.Loosely(t, assets["apps/app2"].Asset, should.Match(&modelpb.Asset{
					Id: "apps/app2",
					IntendedState: &modelpb.AssetState{
						State: &modelpb.AssetState_Appengine{
							Appengine: mockedReportedState("intended", 0),
						},
					},
					LastActuation: &modelpb.Actuation{
						Id: "another-actuation",
					},
					LastActuateActuation: &modelpb.Actuation{
						Id: "another-actuation",
					},
				}))
				assert.Loosely(t, assets["apps/app2"].LastHistoryID, should.Equal(123))
				assert.Loosely(t, assets["apps/app2"].HistoryEntry, should.Match(&modelpb.AssetHistory{
					AssetId:   "apps/app2",
					HistoryId: 123,
					Actuation: &modelpb.Actuation{
						Id: "phony-to-be-untouched",
					},
				}))
				assert.Loosely(t, assets["apps/app2"].ConsecutiveFailures, should.Equal(222))
			})

			t.Run("Failed", func(t *ftt.Test) {
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

				assert.Loosely(t, op.Apply(ctx), should.BeNil)

				// Updated Actuation entity.
				storedActuation := &Actuation{ID: "actuation-id"}
				assert.Loosely(t, datastore.Get(ctx, storedActuation), should.BeNil)
				assert.Loosely(t, storedActuation.Actuation, should.Match(&modelpb.Actuation{
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
				}))
				assert.Loosely(t, storedActuation.State, should.Equal(modelpb.Actuation_FAILED))

				// Updated the asset assigned to this actuation.
				assets, err := fetchAssets(ctx, []string{"apps/app1", "apps/app2"}, true)
				assert.Loosely(t, err, should.BeNil)

				assert.Loosely(t, assets["apps/app1"].Asset, should.Match(&modelpb.Asset{
					Id:                   "apps/app1",
					LastActuation:        storedActuation.Actuation,
					LastActuateActuation: storedActuation.Actuation,
					IntendedState: &modelpb.AssetState{
						State: &modelpb.AssetState_Appengine{
							Appengine: mockedReportedState("intended", 0),
						},
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
					PostActuationStatus: &statuspb.Status{
						Code:    int32(codes.InvalidArgument),
						Message: "status boom",
					},
				}))

				// Stored the historical record with correct PriorConsecutiveFailures
				// counter.
				assert.Loosely(t, assets["apps/app1"].ConsecutiveFailures, should.Equal(112))
				assert.Loosely(t, assets["apps/app1"].LastHistoryID, should.Equal(124))
				assert.Loosely(t, assets["apps/app1"].HistoryEntry.PriorConsecutiveFailures, should.Equal(111))
				rec := AssetHistory{ID: 124, Parent: datastore.KeyForObj(ctx, assets["apps/app1"])}
				assert.Loosely(t, datastore.Get(ctx, &rec), should.BeNil)
				assert.Loosely(t, rec.Entry, should.Match(assets["apps/app1"].HistoryEntry))

				// Wasn't touched.
				assert.Loosely(t, assets["apps/app2"].Asset, should.Match(&modelpb.Asset{
					Id: "apps/app2",
					IntendedState: &modelpb.AssetState{
						State: &modelpb.AssetState_Appengine{
							Appengine: mockedReportedState("intended", 0),
						},
					},
					LastActuation: &modelpb.Actuation{
						Id: "another-actuation",
					},
					LastActuateActuation: &modelpb.Actuation{
						Id: "another-actuation",
					},
				}))
				assert.Loosely(t, assets["apps/app2"].ConsecutiveFailures, should.Equal(222))
			})
		})
	})
}
