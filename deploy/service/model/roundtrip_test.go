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
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/deploy/api/modelpb"
	"go.chromium.org/luci/deploy/api/rpcpb"

	. "github.com/smartystreets/goconvey/convey"
)

func TestRoundTrip(t *testing.T) {
	t.Parallel()

	Convey("With datastore", t, func() {
		now := testclock.TestRecentTimeUTC.Round(time.Millisecond)
		ctx, _ := testclock.UseTime(context.Background(), now)
		ctx = memory.Use(ctx)

		store := func(a *modelpb.Asset) {
			So(datastore.Put(ctx, &Asset{
				ID:    a.Id,
				Asset: a,
			}), ShouldBeNil)
		}

		fetch := func(assetID string) *modelpb.Asset {
			ent := &Asset{ID: assetID}
			So(datastore.Get(ctx, ent), ShouldBeNil)
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

		Convey("Two assets, one up-to-date", func() {
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
			So(err, ShouldBeNil)

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
			So(err, ShouldBeNil)

			asset1 := fetch("apps/app-1")
			asset2 := fetch("apps/app-2")

			// Both assets are associated with the actuation in EXECUTING state.
			So(asset1.LastActuation.State, ShouldEqual, modelpb.Actuation_EXECUTING)
			So(asset2.LastActuation.State, ShouldEqual, modelpb.Actuation_EXECUTING)

			// LastActuateActuation changes only for the stale asset.
			So(asset1.LastActuateActuation.Id, ShouldEqual, "old-actuation")
			So(asset2.LastActuateActuation.Id, ShouldEqual, "new-actuation")
			So(asset2.LastActuateActuation.State, ShouldEqual, modelpb.Actuation_EXECUTING)

			// Finish the executing actuation.
			actuation := &Actuation{ID: "new-actuation"}
			So(datastore.Get(ctx, actuation), ShouldBeNil)
			endOp, err := NewActuationEndOp(ctx, actuation)
			So(err, ShouldBeNil)
			endOp.UpdateActuationStatus(ctx, nil, "")
			endOp.HandleActuatedState(ctx, "apps/app-2", &rpcpb.ActuatedAsset{
				State: reportedState("app-2", 500),
			})
			So(endOp.Apply(ctx), ShouldBeNil)

			asset1 = fetch("apps/app-1")
			asset2 = fetch("apps/app-2")

			// Both assets are associated with the actuation in SUCCEEDED state.
			So(asset1.LastActuation.State, ShouldEqual, modelpb.Actuation_SUCCEEDED)
			So(asset2.LastActuation.State, ShouldEqual, modelpb.Actuation_SUCCEEDED)

			// LastActuateActuation changes only for the stale asset.
			So(asset1.LastActuateActuation.Id, ShouldEqual, "old-actuation")
			So(asset2.LastActuateActuation.Id, ShouldEqual, "new-actuation")
			So(asset2.LastActuateActuation.State, ShouldEqual, modelpb.Actuation_SUCCEEDED)
		})

		Convey("Crashing actuation", func() {
			store(&modelpb.Asset{
				Id:           "apps/app",
				AppliedState: intendedState("app", 0),
			})

			beginOp, err := NewActuationBeginOp(ctx, []string{"apps/app"}, &modelpb.Actuation{
				Id: "actuation-0",
			})
			So(err, ShouldBeNil)

			// ACTUATE_STALE decision.
			beginOp.MakeDecision(ctx, "apps/app", &rpcpb.AssetToActuate{
				Config:        &modelpb.AssetConfig{EnableAutomation: true},
				IntendedState: intendedState("app", 500),
				ReportedState: reportedState("app", 0),
			})
			_, err = beginOp.Apply(ctx)
			So(err, ShouldBeNil)

			// The asset points to this actuation.
			asset := fetch("apps/app")
			So(asset.LastActuation.Id, ShouldEqual, "actuation-0")
			So(asset.LastActuateActuation.Id, ShouldEqual, "actuation-0")

			// Another actuation starts before the previous one finishes.
			beginOp, err = NewActuationBeginOp(ctx, []string{"apps/app"}, &modelpb.Actuation{
				Id: "actuation-1",
			})
			So(err, ShouldBeNil)

			// ACTUATE_STALE decision.
			beginOp.MakeDecision(ctx, "apps/app", &rpcpb.AssetToActuate{
				Config:        &modelpb.AssetConfig{EnableAutomation: true},
				IntendedState: intendedState("app", 500),
				ReportedState: reportedState("app", 0),
			})
			_, err = beginOp.Apply(ctx)
			So(err, ShouldBeNil)

			// The asset points to the new actuation.
			asset = fetch("apps/app")
			So(asset.LastActuation.Id, ShouldEqual, "actuation-1")
			So(asset.LastActuateActuation.Id, ShouldEqual, "actuation-1")

			// There's a history log that points to the crashed actuation.
			hist := history("apps/app", 1)
			So(hist.Actuation.Id, ShouldEqual, "actuation-0")
			So(hist.Actuation.State, ShouldEqual, modelpb.Actuation_EXPIRED)

			// Finish the stale actuation, it should be a noop.
			actuation := &Actuation{ID: "actuation-0"}
			So(datastore.Get(ctx, actuation), ShouldBeNil)
			endOp, err := NewActuationEndOp(ctx, actuation)
			So(err, ShouldBeNil)
			endOp.UpdateActuationStatus(ctx, nil, "")
			endOp.HandleActuatedState(ctx, "apps/app", &rpcpb.ActuatedAsset{
				State: reportedState("app", 777),
			})
			So(endOp.Apply(ctx), ShouldBeNil)

			// No new history entries.
			So(history("apps/app", 2), ShouldBeNil)

			// Finish the active actuation.
			actuation = &Actuation{ID: "actuation-1"}
			So(datastore.Get(ctx, actuation), ShouldBeNil)
			endOp, err = NewActuationEndOp(ctx, actuation)
			So(err, ShouldBeNil)
			endOp.UpdateActuationStatus(ctx, nil, "")
			endOp.HandleActuatedState(ctx, "apps/app", &rpcpb.ActuatedAsset{
				State: reportedState("app", 500),
			})
			So(endOp.Apply(ctx), ShouldBeNil)

			// Have the new history log entry.
			hist = history("apps/app", 2)
			So(hist.Actuation.Id, ShouldEqual, "actuation-1")
			So(hist.Actuation.State, ShouldEqual, modelpb.Actuation_SUCCEEDED)
		})
	})
}
