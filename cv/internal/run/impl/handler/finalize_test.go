// Copyright 2021 The LUCI Authors.
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

package handler

import (
	"testing"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/gae/service/datastore"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/gerrit/updater"
	"go.chromium.org/luci/cv/internal/migration"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/eventpb"
	"go.chromium.org/luci/cv/internal/run/impl/state"
	"go.chromium.org/luci/cv/internal/run/runtest"

	. "github.com/smartystreets/goconvey/convey"
)

func TestOnFinished(t *testing.T) {
	t.Parallel()

	Convey("onFinished", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()
		h := &Impl{}
		rs := &state.RunState{
			Run: run.Run{
				ID:  common.RunID("chromium/111-1-beef"),
				CLs: []common.CLID{1},
			},
		}
		cl := &changelist.CL{
			ID:             1,
			ExternalID:     changelist.MustGobID("x-review.example.com", 1),
			IncompleteRuns: common.RunIDs{rs.Run.ID},
		}
		So(datastore.Put(ctx, cl), ShouldBeNil)

		Convey("When Run is RUNNING", func() {
			rs.Run.Status = run.Status_RUNNING

			sideEffect, newrs, err := h.OnFinished(ctx, rs)
			So(err, ShouldBeNil)
			So(newrs.Run.Status, ShouldEqual, run.Status_FINALIZING)

			So(datastore.RunInTransaction(ctx, sideEffect, nil), ShouldBeNil)

			foundRefreshTask := false
			for _, t := range ct.TQ.Tasks() {
				if proto.Equal(t.Payload, &updater.RefreshGerritCL{
					LuciProject: "chromium",
					Host:        "x-review.example.com",
					Change:      1,
					ClidHint:    1,
				}) {
					foundRefreshTask = true
					break
				}
			}
			So(foundRefreshTask, ShouldBeTrue)

			runtest.AssertInEventbox(ctx, newrs.Run.ID, &eventpb.Event{
				Event: &eventpb.Event_Finished{
					Finished: &eventpb.Finished{},
				},
				ProcessAfter: timestamppb.New(clock.Now(ctx).UTC().Add(1 * time.Minute)),
			})
		})

		Convey("When Run is FINALIZING", func() {
			rs.Run.Status = run.Status_FINALIZING
			mfr := &migration.FinishedRun{
				ID:      rs.Run.ID,
				Status:  run.Status_SUCCEEDED,
				EndTime: clock.Now(ctx).UTC().Truncate(time.Second).Add(-2 * time.Minute),
			}
			So(datastore.Put(ctx, mfr), ShouldBeNil)

			sideEffect, newrs, err := h.OnFinished(ctx, rs)
			So(err, ShouldBeNil)
			So(newrs.Run.Status, ShouldEqual, mfr.Status)
			So(newrs.Run.EndTime, ShouldEqual, mfr.EndTime)

			So(datastore.RunInTransaction(ctx, sideEffect, nil), ShouldBeNil)

			afterCL := &changelist.CL{ID: cl.ID}
			So(datastore.Get(ctx, afterCL), ShouldBeNil)
			So(afterCL.IncompleteRuns, ShouldBeEmpty)
		})
	})
}
