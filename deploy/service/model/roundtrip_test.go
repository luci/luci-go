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

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/deploy/api/modelpb"
	"go.chromium.org/luci/deploy/api/rpcpb"
)

func TestRoundTrip(t *testing.T) {
	t.Parallel()

	ftt.Run("With datastore", t, func(t *ftt.Test) {
		now := testclock.TestRecentTimeUTC.Round(time.Millisecond)
		ctx, _ := testclock.UseTime(context.Background(), now)
		ctx = memory.Use(ctx)

		store := func(a *modelpb.Asset) {
			assert.Loosely(t, datastore.Put(ctx, &Asset{
				ID:    a.Id,
				Asset: a,
			}), should.BeNil)
		}

		fetch := func(assetID string) *modelpb.Asset {
			ent := &Asset{ID: assetID}
			assert.Loosely(t, datastore.Get(ctx, ent), should.BeNil)
			return ent.Asset
		}

		history := func(assetID string, historyID int64) *modelpb.AssetHistory {
			ent := &AssetHistory{
				ID:     historyID,
				Parent: datastore.NewKey(ctx, "Asset", assetID, 0, nil),
			}
			if datastore.Get(ctx, ent) == datastore.ErrNoSuchEntity {
				return nil
			}
			return ent.Entry
		}

		intendedState := func(payload string, traffic int32) *modelpb.AssetState {
			return &modelpb.AssetState{
				State: &modelpb.AssetState_Appengine{
					Appengine: mockedIntendedState(payload, traffic),
				},
			}
		}

		reportedState := func(payload string, traffic int32) *modelpb.AssetState {
			return &modelpb.AssetState{
				State: &modelpb.AssetState_Appengine{
					Appengine: mockedReportedState(payload, traffic),
				},
			}
		}

		t.Run("Two assets, one up-to-date", func(t *ftt.Test) {
			store(&modelpb.Asset{
				Id:           "apps/app-1",
				AppliedState: intendedState("app-1", 0),
				LastActuateActuation: &modelpb.Actuation{
					Id: "old-actuation",
				},
			})
			store(&modelpb.Asset{
				Id:           "apps/app-2",
				AppliedState: intendedState("app-2", 0),
				LastActuateActuation: &modelpb.Actuation{
					Id: "old-actuation",
				},
			})

			beginOp, err := NewActuationBeginOp(ctx, []string{"apps/app-1", "apps/app-2"}, &modelpb.Actuation{
				Id: "new-actuation",
			})
			assert.Loosely(t, err, should.BeNil)

			// SKIP_UPTODATE decision.
			beginOp.MakeDecision(ctx, "apps/app-1", &rpcpb.AssetToActuate{
				Config:        &modelpb.AssetConfig{EnableAutomation: true},
				IntendedState: intendedState("app-1", 0),
				ReportedState: reportedState("app-1", 0),
			})
			// ACTUATE_STALE decision.
			beginOp.MakeDecision(ctx, "apps/app-2", &rpcpb.AssetToActuate{
				Config:        &modelpb.AssetConfig{EnableAutomation: true},
				IntendedState: intendedState("app-2", 500),
				ReportedState: reportedState("app-2", 0),
			})

			_, err = beginOp.Apply(ctx)
			assert.Loosely(t, err, should.BeNil)

			asset1 := fetch("apps/app-1")
			asset2 := fetch("apps/app-2")

			// Both assets are associated with the actuation in EXECUTING state.
			assert.Loosely(t, asset1.LastActuation.State, should.Equal(modelpb.Actuation_EXECUTING))
			assert.Loosely(t, asset2.LastActuation.State, should.Equal(modelpb.Actuation_EXECUTING))

			// LastActuateActuation changes only for the stale asset.
			assert.Loosely(t, asset1.LastActuateActuation.Id, should.Equal("old-actuation"))
			assert.Loosely(t, asset2.LastActuateActuation.Id, should.Equal("new-actuation"))
			assert.Loosely(t, asset2.LastActuateActuation.State, should.Equal(modelpb.Actuation_EXECUTING))

			// Finish the executing actuation.
			actuation := &Actuation{ID: "new-actuation"}
			assert.Loosely(t, datastore.Get(ctx, actuation), should.BeNil)
			endOp, err := NewActuationEndOp(ctx, actuation)
			assert.Loosely(t, err, should.BeNil)
			endOp.UpdateActuationStatus(ctx, nil, "")
			endOp.HandleActuatedState(ctx, "apps/app-2", &rpcpb.ActuatedAsset{
				State: reportedState("app-2", 500),
			})
			assert.Loosely(t, endOp.Apply(ctx), should.BeNil)

			asset1 = fetch("apps/app-1")
			asset2 = fetch("apps/app-2")

			// Both assets are associated with the actuation in SUCCEEDED state.
			assert.Loosely(t, asset1.LastActuation.State, should.Equal(modelpb.Actuation_SUCCEEDED))
			assert.Loosely(t, asset2.LastActuation.State, should.Equal(modelpb.Actuation_SUCCEEDED))

			// LastActuateActuation changes only for the stale asset.
			assert.Loosely(t, asset1.LastActuateActuation.Id, should.Equal("old-actuation"))
			assert.Loosely(t, asset2.LastActuateActuation.Id, should.Equal("new-actuation"))
			assert.Loosely(t, asset2.LastActuateActuation.State, should.Equal(modelpb.Actuation_SUCCEEDED))
		})

		t.Run("Crashing actuation", func(t *ftt.Test) {
			store(&modelpb.Asset{
				Id:           "apps/app",
				AppliedState: intendedState("app", 0),
			})

			beginOp, err := NewActuationBeginOp(ctx, []string{"apps/app"}, &modelpb.Actuation{
				Id: "actuation-0",
			})
			assert.Loosely(t, err, should.BeNil)

			// ACTUATE_STALE decision.
			beginOp.MakeDecision(ctx, "apps/app", &rpcpb.AssetToActuate{
				Config:        &modelpb.AssetConfig{EnableAutomation: true},
				IntendedState: intendedState("app", 500),
				ReportedState: reportedState("app", 0),
			})
			_, err = beginOp.Apply(ctx)
			assert.Loosely(t, err, should.BeNil)

			// The asset points to this actuation.
			asset := fetch("apps/app")
			assert.Loosely(t, asset.LastActuation.Id, should.Equal("actuation-0"))
			assert.Loosely(t, asset.LastActuateActuation.Id, should.Equal("actuation-0"))

			// Another actuation starts before the previous one finishes.
			beginOp, err = NewActuationBeginOp(ctx, []string{"apps/app"}, &modelpb.Actuation{
				Id: "actuation-1",
			})
			assert.Loosely(t, err, should.BeNil)

			// ACTUATE_STALE decision.
			beginOp.MakeDecision(ctx, "apps/app", &rpcpb.AssetToActuate{
				Config:        &modelpb.AssetConfig{EnableAutomation: true},
				IntendedState: intendedState("app", 500),
				ReportedState: reportedState("app", 0),
			})
			_, err = beginOp.Apply(ctx)
			assert.Loosely(t, err, should.BeNil)

			// The asset points to the new actuation.
			asset = fetch("apps/app")
			assert.Loosely(t, asset.LastActuation.Id, should.Equal("actuation-1"))
			assert.Loosely(t, asset.LastActuateActuation.Id, should.Equal("actuation-1"))

			// There's a history log that points to the crashed actuation.
			hist := history("apps/app", 1)
			assert.Loosely(t, hist.Actuation.Id, should.Equal("actuation-0"))
			assert.Loosely(t, hist.Actuation.State, should.Equal(modelpb.Actuation_EXPIRED))

			// Finish the stale actuation, it should be a noop.
			actuation := &Actuation{ID: "actuation-0"}
			assert.Loosely(t, datastore.Get(ctx, actuation), should.BeNil)
			endOp, err := NewActuationEndOp(ctx, actuation)
			assert.Loosely(t, err, should.BeNil)
			endOp.UpdateActuationStatus(ctx, nil, "")
			endOp.HandleActuatedState(ctx, "apps/app", &rpcpb.ActuatedAsset{
				State: reportedState("app", 777),
			})
			assert.Loosely(t, endOp.Apply(ctx), should.BeNil)

			// No new history entries.
			assert.Loosely(t, history("apps/app", 2), should.BeNil)

			// Finish the active actuation.
			actuation = &Actuation{ID: "actuation-1"}
			assert.Loosely(t, datastore.Get(ctx, actuation), should.BeNil)
			endOp, err = NewActuationEndOp(ctx, actuation)
			assert.Loosely(t, err, should.BeNil)
			endOp.UpdateActuationStatus(ctx, nil, "")
			endOp.HandleActuatedState(ctx, "apps/app", &rpcpb.ActuatedAsset{
				State: reportedState("app", 500),
			})
			assert.Loosely(t, endOp.Apply(ctx), should.BeNil)

			// Have the new history log entry.
			hist = history("apps/app", 2)
			assert.Loosely(t, hist.Actuation.Id, should.Equal("actuation-1"))
			assert.Loosely(t, hist.Actuation.State, should.Equal(modelpb.Actuation_SUCCEEDED))
		})
	})
}
