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
	"fmt"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/gae/service/datastore"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/configs/prjcfg/prjcfgtest"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/prjmanager/pmtest"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/impl/state"

	. "github.com/smartystreets/goconvey/convey"
)

func TestCancel(t *testing.T) {
	t.Parallel()

	Convey("Cancel", t, func() {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)
		ctx, _ = pmtest.MockDispatch(ctx)

		const lProject = "chromium"
		prjcfgtest.Create(ctx, lProject, &cfgpb.Config{
			ConfigGroups: []*cfgpb.ConfigGroup{{Name: "main"}},
		})
		cgs, err := prjcfgtest.MustExist(ctx, lProject).GetConfigGroups(ctx)
		So(err, ShouldBeNil)
		cg := cgs[0]
		runID := common.MakeRunID(lProject, ct.Clock.Now(), 1, []byte("deadbeef"))
		clid := common.CLID(11)
		rs := &state.RunState{
			Run: run.Run{
				ID:            runID,
				ConfigGroupID: cg.ID,
				CreateTime:    clock.Now(ctx).UTC().Add(-2 * time.Minute),
				CLs:           common.CLIDs{clid},
			},
		}
		So(datastore.Put(ctx, &changelist.CL{
			ID: clid,
			IncompleteRuns: common.RunIDs{
				runID,
				common.MakeRunID(lProject, ct.Clock.Now().Add(1*time.Minute), 1, []byte("cafecafe")),
			},
		}), ShouldBeNil)
		h, _ := makeTestHandler(&ct)

		now := ct.Clock.Now().UTC()
		Convey("Cancel works", func() {
			rs.Status = run.Status_RUNNING
			rs.StartTime = now.Add(-1 * time.Minute)
			res, err := h.Cancel(ctx, rs, []string{"user request"})
			So(err, ShouldBeNil)
			So(res.State.Status, ShouldEqual, run.Status_CANCELLED)
			So(res.State.StartTime, ShouldResemble, now.Add(-1*time.Minute))
			So(res.State.EndTime, ShouldResemble, now)
			So(res.State.CancellationReasons, ShouldResemble, []string{"user request"})
			So(res.SideEffectFn, ShouldNotBeNil)
			So(res.PreserveEvents, ShouldBeFalse)
		})

		Convey("Filter cancellation reasons", func() {
			rs.Status = run.Status_RUNNING
			rs.StartTime = now.Add(-1 * time.Minute)
			res, err := h.Cancel(ctx, rs, []string{"user request", "", "user request"})
			So(err, ShouldBeNil)
			So(res.State.CancellationReasons, ShouldResemble, []string{"user request"})
		})

		Convey("Cancels SUBMITTING Run", func() {
			rs.Status = run.Status_SUBMITTING
			res, err := h.Cancel(ctx, rs, []string{"user request"})
			So(err, ShouldBeNil)
			So(res.State, ShouldEqual, rs)
			So(res.SideEffectFn, ShouldBeNil)
			So(res.PreserveEvents, ShouldBeTrue)
		})

		statuses := []run.Status{
			run.Status_SUCCEEDED,
			run.Status_FAILED,
			run.Status_CANCELLED,
		}
		for _, status := range statuses {
			Convey(fmt.Sprintf("Noop when Run is %s", status), func() {
				rs.Status = status
				rs.StartTime = clock.Now(ctx).UTC().Add(-1 * time.Minute)
				rs.EndTime = clock.Now(ctx).UTC().Add(-30 * time.Second)
				res, err := h.Cancel(ctx, rs, []string{"user request"})
				So(err, ShouldBeNil)
				So(res.State, ShouldEqual, rs)
				So(res.SideEffectFn, ShouldBeNil)
				So(res.PreserveEvents, ShouldBeFalse)
			})
		}
	})
}
