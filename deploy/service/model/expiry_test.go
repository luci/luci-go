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

	"google.golang.org/protobuf/types/known/durationpb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/deploy/api/modelpb"
	"go.chromium.org/luci/deploy/api/rpcpb"
)

func TestExpireActuations(t *testing.T) {
	t.Parallel()

	ftt.Run("With datastore", t, func(t *ftt.Test) {
		now := testclock.TestRecentTimeUTC.Round(time.Millisecond)
		ctx, tc := testclock.UseTime(context.Background(), now)
		ctx = memory.Use(ctx)

		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)

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

		actuation := func(actuationID string) *modelpb.Actuation {
			ent := &Actuation{ID: actuationID}
			assert.Loosely(t, datastore.Get(ctx, ent), should.BeNil)
			return ent.Actuation
		}

		assetEntity := func(assetID string) *Asset {
			ent := &Asset{ID: assetID}
			assert.Loosely(t, datastore.Get(ctx, ent), should.BeNil)
			return ent
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

		t.Run("Expiry works", func(t *ftt.Test) {
			assert.Loosely(t, datastore.Put(ctx, &Asset{
				ID:                  "apps/app-1",
				Asset:               &modelpb.Asset{Id: "apps/app-1"},
				ConsecutiveFailures: 111,
			}), should.BeNil)

			// Start the new actuation.
			op, err := NewActuationBeginOp(ctx, []string{"apps/app-1"}, &modelpb.Actuation{
				Id: "new-actuation",
				Deployment: &modelpb.Deployment{
					Config: &modelpb.DeploymentConfig{
						ActuationTimeout: durationpb.New(5 * time.Minute),
					},
				},
			})
			assert.Loosely(t, err, should.BeNil)
			op.MakeDecision(ctx, "apps/app-1", &rpcpb.AssetToActuate{
				Config:        &modelpb.AssetConfig{EnableAutomation: true},
				IntendedState: intendedState("app-1", 0),
				ReportedState: reportedState("app-1", 0),
			})
			_, err = op.Apply(ctx)
			assert.Loosely(t, err, should.BeNil)

			// It is executing now.
			assert.Loosely(t, actuation("new-actuation").State, should.Equal(modelpb.Actuation_EXECUTING))

			// Run the expiration cron a bit later (but before the expiry).
			tc.Add(4 * time.Minute)
			assert.Loosely(t, ExpireActuations(ctx), should.BeNil)

			// Still executing.
			assert.Loosely(t, actuation("new-actuation").State, should.Equal(modelpb.Actuation_EXECUTING))

			// Run the expiration cron after the expiry.
			tc.Add(2 * time.Minute)
			assert.Loosely(t, ExpireActuations(ctx), should.BeNil)

			// The actuation has expired.
			assert.Loosely(t, actuation("new-actuation").State, should.Equal(modelpb.Actuation_EXPIRED))

			// There's a history record for the asset being actuated.
			assert.Loosely(t, history("apps/app-1", 1).Actuation.State, should.Equal(modelpb.Actuation_EXPIRED))

			// Failure counter incremented.
			assert.Loosely(t, assetEntity("apps/app-1").ConsecutiveFailures, should.Equal(112))
		})
	})
}
