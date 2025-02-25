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
	"google.golang.org/protobuf/types/known/durationpb"
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

var (
	mockedDeployment = &modelpb.Deployment{
		RepoRev: "mocked-deployment",
		Config: &modelpb.DeploymentConfig{
			ActuationTimeout: durationpb.New(3 * time.Minute),
		},
	}
	mockedActuator = &modelpb.ActuatorInfo{Identity: "mocked-actuator"}
	mockedTriggers = []*modelpb.ActuationTrigger{{}, {}}
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

	ftt.Run("With datastore", t, func(t *ftt.Test) {
		now := testclock.TestRecentTimeUTC.Round(time.Millisecond)
		ctx, _ := testclock.UseTime(context.Background(), now)
		ctx = memory.Use(ctx)

		assert.Loosely(t, datastore.Put(ctx, &Asset{
			ID:                  "apps/app1",
			Asset:               &modelpb.Asset{Id: "apps/app1"},
			ConsecutiveFailures: 111,
		}, &Asset{
			ID:                  "apps/app2",
			Asset:               &modelpb.Asset{Id: "apps/app2"},
			ConsecutiveFailures: 222,
		}), should.BeNil)

		t.Run("Executing", func(t *ftt.Test) {
			op, err := NewActuationBeginOp(ctx, []string{"apps/app1", "apps/app2"}, &modelpb.Actuation{
				Id:         "actuation-id",
				Deployment: mockedDeployment,
				Actuator:   mockedActuator,
				Triggers:   mockedTriggers,
			})
			assert.Loosely(t, err, should.BeNil)

			app1Call := &rpcpb.AssetToActuate{
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
			}
			op.MakeDecision(ctx, "apps/app1", app1Call)

			app2Call := &rpcpb.AssetToActuate{
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
			}
			op.MakeDecision(ctx, "apps/app2", app2Call)

			decisions, err := op.Apply(ctx)
			assert.Loosely(t, err, should.BeNil)

			// Returned decisions are correct.
			assert.Loosely(t, decisions, should.HaveLength(2))
			assert.Loosely(t, decisions["apps/app1"], should.Match(&modelpb.ActuationDecision{
				Decision: modelpb.ActuationDecision_SKIP_DISABLED,
			}))
			assert.Loosely(t, decisions["apps/app2"], should.Match(&modelpb.ActuationDecision{
				Decision: modelpb.ActuationDecision_ACTUATE_STALE,
			}))

			// Stored Actuation entity is correct.
			storedActuation := &Actuation{ID: "actuation-id"}
			assert.Loosely(t, datastore.Get(ctx, storedActuation), should.BeNil)
			assert.Loosely(t, storedActuation.Actuation, should.Match(&modelpb.Actuation{
				Id:         "actuation-id",
				State:      modelpb.Actuation_EXECUTING,
				Deployment: mockedDeployment,
				Actuator:   mockedActuator,
				Triggers:   mockedTriggers,
				Created:    timestamppb.New(now),
				Expiry:     timestamppb.New(now.Add(3 * time.Minute)),
			}))
			assert.Loosely(t, storedActuation.Decisions, should.Match(&modelpb.ActuationDecisions{
				Decisions: map[string]*modelpb.ActuationDecision{
					"apps/app1": {Decision: modelpb.ActuationDecision_SKIP_DISABLED},
					"apps/app2": {Decision: modelpb.ActuationDecision_ACTUATE_STALE},
				},
			}))
			assert.Loosely(t, storedActuation.State, should.Equal(modelpb.Actuation_EXECUTING))
			assert.Loosely(t, storedActuation.Created.Equal(now), should.BeTrue)
			assert.Loosely(t, storedActuation.Expiry.Equal(now.Add(3*time.Minute)), should.BeTrue)

			// Stored Asset entities are correct.
			assets, err := fetchAssets(ctx, []string{"apps/app1", "apps/app2"}, true)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, assets["apps/app1"].Asset, should.Match(&modelpb.Asset{
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
			}))
			assert.Loosely(t, assets["apps/app1"].ConsecutiveFailures, should.BeZero) // was reset

			assert.Loosely(t, assets["apps/app2"].Asset, should.Match(&modelpb.Asset{
				Id:                   "apps/app2",
				LastActuation:        storedActuation.Actuation,
				LastDecision:         decisions["apps/app2"],
				LastActuateActuation: storedActuation.Actuation,
				LastActuateDecision:  decisions["apps/app2"],
				Config:               &modelpb.AssetConfig{EnableAutomation: true},
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
			}))
			assert.Loosely(t, assets["apps/app2"].ConsecutiveFailures, should.Equal(222)) // unchanged

			// Made correct history records.
			assert.Loosely(t, assets["apps/app1"].LastHistoryID, should.Equal(1))
			assert.Loosely(t, assets["apps/app1"].HistoryEntry, should.Match(&modelpb.AssetHistory{
				AssetId:                  "apps/app1",
				HistoryId:                1,
				Decision:                 decisions["apps/app1"],
				Actuation:                storedActuation.Actuation,
				Config:                   app1Call.Config,
				IntendedState:            app1Call.IntendedState,
				ReportedState:            app1Call.ReportedState,
				PriorConsecutiveFailures: 111,
			}))
			rec := AssetHistory{ID: 1, Parent: datastore.KeyForObj(ctx, assets["apps/app1"])}
			assert.Loosely(t, datastore.Get(ctx, &rec), should.BeNil)
			assert.Loosely(t, rec.Entry, should.Match(assets["apps/app1"].HistoryEntry))

			assert.Loosely(t, assets["apps/app2"].LastHistoryID, should.BeZero)
			assert.Loosely(t, assets["apps/app2"].HistoryEntry, should.Match(&modelpb.AssetHistory{
				AssetId:                  "apps/app2",
				HistoryId:                1,
				Decision:                 decisions["apps/app2"],
				Actuation:                storedActuation.Actuation,
				Config:                   app2Call.Config,
				IntendedState:            app2Call.IntendedState,
				ReportedState:            app2Call.ReportedState,
				PriorConsecutiveFailures: 222,
			}))
			rec = AssetHistory{ID: 1, Parent: datastore.KeyForObj(ctx, assets["apps/app2"])}
			assert.Loosely(t, datastore.Get(ctx, &rec), should.Equal(datastore.ErrNoSuchEntity))
		})

		t.Run("Skipping disabled", func(t *ftt.Test) {
			op, err := NewActuationBeginOp(ctx, []string{"apps/app1"}, &modelpb.Actuation{
				Id:         "actuation-id",
				Deployment: mockedDeployment,
				Actuator:   mockedActuator,
				Triggers:   mockedTriggers,
			})
			assert.Loosely(t, err, should.BeNil)

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
			assert.Loosely(t, err, should.BeNil)

			// Stored Actuation entity is correct.
			storedActuation := &Actuation{ID: "actuation-id"}
			assert.Loosely(t, datastore.Get(ctx, storedActuation), should.BeNil)
			assert.Loosely(t, storedActuation.Actuation, should.Match(&modelpb.Actuation{
				Id:         "actuation-id",
				State:      modelpb.Actuation_SUCCEEDED,
				Deployment: mockedDeployment,
				Actuator:   mockedActuator,
				Triggers:   mockedTriggers,
				Created:    timestamppb.New(now),
				Finished:   timestamppb.New(now),
			}))
			assert.Loosely(t, storedActuation.Decisions, should.Match(&modelpb.ActuationDecisions{
				Decisions: map[string]*modelpb.ActuationDecision{
					"apps/app1": {Decision: modelpb.ActuationDecision_SKIP_DISABLED},
				},
			}))
			assert.Loosely(t, storedActuation.State, should.Equal(modelpb.Actuation_SUCCEEDED))
			assert.Loosely(t, storedActuation.Created.Equal(now), should.BeTrue)
			assert.Loosely(t, storedActuation.Expiry.IsZero(), should.BeTrue)

			// Reset ConsecutiveFailures counter.
			assets, _ := fetchAssets(ctx, []string{"apps/app1"}, true)
			assert.Loosely(t, assets["apps/app1"].ConsecutiveFailures, should.BeZero)
		})

		t.Run("Skipping up-to-date", func(t *ftt.Test) {
			datastore.Put(ctx, &Asset{
				ID: "apps/app1",
				Asset: &modelpb.Asset{
					Id: "apps/app1",
					AppliedState: &modelpb.AssetState{
						State: &modelpb.AssetState_Appengine{
							Appengine: mockedIntendedState("app1", 0),
						},
					},
				},
			})

			op, err := NewActuationBeginOp(ctx, []string{"apps/app1"}, &modelpb.Actuation{
				Id:         "actuation-id",
				Deployment: mockedDeployment,
				Actuator:   mockedActuator,
				Triggers:   mockedTriggers,
			})
			assert.Loosely(t, err, should.BeNil)

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
			assert.Loosely(t, err, should.BeNil)

			// Stored Actuation entity is correct.
			storedActuation := &Actuation{ID: "actuation-id"}
			assert.Loosely(t, datastore.Get(ctx, storedActuation), should.BeNil)
			assert.Loosely(t, storedActuation.Actuation, should.Match(&modelpb.Actuation{
				Id:         "actuation-id",
				State:      modelpb.Actuation_SUCCEEDED,
				Deployment: mockedDeployment,
				Actuator:   mockedActuator,
				Triggers:   mockedTriggers,
				Created:    timestamppb.New(now),
				Finished:   timestamppb.New(now),
			}))
			assert.Loosely(t, storedActuation.Decisions, should.Match(&modelpb.ActuationDecisions{
				Decisions: map[string]*modelpb.ActuationDecision{
					"apps/app1": {Decision: modelpb.ActuationDecision_SKIP_UPTODATE},
				},
			}))
			assert.Loosely(t, storedActuation.State, should.Equal(modelpb.Actuation_SUCCEEDED))
			assert.Loosely(t, storedActuation.Created.Equal(now), should.BeTrue)
			assert.Loosely(t, storedActuation.Expiry.IsZero(), should.BeTrue)

			// Stored Asset entity is correct.
			assets, err := fetchAssets(ctx, []string{"apps/app1"}, true)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, assets["apps/app1"].Asset, should.Match(&modelpb.Asset{
				Id:            "apps/app1",
				LastActuation: storedActuation.Actuation,
				LastDecision:  storedActuation.Decisions.Decisions["apps/app1"],
				Config:        &modelpb.AssetConfig{EnableAutomation: true},
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
						Appengine: mockedReportedState("app1", 0),
					},
				},
				AppliedState: &modelpb.AssetState{
					Timestamp:  timestamppb.New(now),
					Deployment: storedActuation.Actuation.Deployment,
					Actuator:   storedActuation.Actuation.Actuator,
					State: &modelpb.AssetState_Appengine{
						Appengine: mockedIntendedState("app1", 0),
					},
				},
			}))
			assert.Loosely(t, assets["apps/app1"].ConsecutiveFailures, should.BeZero)
		})

		t.Run("Broken", func(t *ftt.Test) {
			op, err := NewActuationBeginOp(ctx, []string{"apps/app1"}, &modelpb.Actuation{
				Id:         "actuation-id",
				Deployment: mockedDeployment,
				Actuator:   mockedActuator,
				Triggers:   mockedTriggers,
			})
			assert.Loosely(t, err, should.BeNil)

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
			assert.Loosely(t, err, should.BeNil)

			// Stored Actuation entity is correct.
			storedActuation := &Actuation{ID: "actuation-id"}
			assert.Loosely(t, datastore.Get(ctx, storedActuation), should.BeNil)
			assert.Loosely(t, storedActuation.Actuation, should.Match(&modelpb.Actuation{
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
			}))
			assert.Loosely(t, storedActuation.Decisions, should.Match(&modelpb.ActuationDecisions{
				Decisions: map[string]*modelpb.ActuationDecision{
					"apps/app1": {
						Decision: modelpb.ActuationDecision_SKIP_BROKEN,
						Status: &statuspb.Status{
							Code:    int32(codes.FailedPrecondition),
							Message: "reported broken",
						},
					},
				},
			}))
			assert.Loosely(t, storedActuation.State, should.Equal(modelpb.Actuation_FAILED))
			assert.Loosely(t, storedActuation.Created.Equal(now), should.BeTrue)
			assert.Loosely(t, storedActuation.Expiry.IsZero(), should.BeTrue)

			// Incremented ConsecutiveFailures counter.
			assets, _ := fetchAssets(ctx, []string{"apps/app1"}, true)
			assert.Loosely(t, assets["apps/app1"].ConsecutiveFailures, should.Equal(112))

			// Stored the historical record with correct ConsecutiveFailures counter.
			assert.Loosely(t, assets["apps/app1"].LastHistoryID, should.Equal(1))
			assert.Loosely(t, assets["apps/app1"].HistoryEntry.PriorConsecutiveFailures, should.Equal(111))
			rec := AssetHistory{ID: 1, Parent: datastore.KeyForObj(ctx, assets["apps/app1"])}
			assert.Loosely(t, datastore.Get(ctx, &rec), should.BeNil)
			assert.Loosely(t, rec.Entry, should.Match(assets["apps/app1"].HistoryEntry))
		})
	})
}
