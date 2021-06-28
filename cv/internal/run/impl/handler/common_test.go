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
	"context"
	"sort"
	"sync"
	"testing"
	"time"

	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/prjmanager"
	"go.chromium.org/luci/cv/internal/prjmanager/pmtest"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/bq"
	"go.chromium.org/luci/cv/internal/run/impl/state"

	. "github.com/smartystreets/goconvey/convey"
)

func TestEndRun(t *testing.T) {
	t.Parallel()

	Convey("EndRun", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()

		const clid = 1
		rid := common.MakeRunID("infra", ct.Clock.Now(), 1, []byte("deadbeef"))
		rs := &state.RunState{
			Run: run.Run{
				ID:         rid,
				Status:     run.Status_RUNNING,
				CreateTime: ct.Clock.Now().Add(-2 * time.Minute),
				StartTime:  ct.Clock.Now().Add(-1 * time.Minute),
				CLs:        common.CLIDs{1},
			},
		}

		anotherRID := common.MakeRunID("infra", ct.Clock.Now(), 1, []byte("cafecafe"))
		cl := changelist.CL{
			ID:             clid,
			IncompleteRuns: common.RunIDs{rid, anotherRID},
			EVersion:       3,
			UpdateTime:     ct.Clock.Now().UTC(),
		}
		sort.Sort(cl.IncompleteRuns)
		So(datastore.Put(ctx, &cl), ShouldBeNil)

		clUpdater := &clUpdaterMock{}
		h := &Impl{
			PM:         prjmanager.NewNotifier(ct.TQDispatcher),
			CLUpdater:  clUpdater,
			BQExporter: bq.NewExporter(ct.TQDispatcher, ct.BQFake),
		}
		se := h.endRun(ctx, rs, run.Status_FAILED)
		So(rs.Run.Status, ShouldEqual, run.Status_FAILED)
		So(rs.Run.EndTime, ShouldEqual, ct.Clock.Now())
		So(datastore.RunInTransaction(ctx, se, nil), ShouldBeNil)
		cl = changelist.CL{ID: clid}
		So(datastore.Get(ctx, &cl), ShouldBeNil)
		So(cl, ShouldResemble, changelist.CL{
			ID:             clid,
			IncompleteRuns: common.RunIDs{anotherRID},
			EVersion:       4,
			UpdateTime:     ct.Clock.Now().UTC(),
		})
		pmtest.AssertReceivedRunFinished(ctx, rid)
		pmtest.AssertReceivedCLsNotified(ctx, rid.LUCIProject(), []*changelist.CL{&cl})
		So(clUpdater.refreshedCLs, ShouldResemble, common.MakeCLIDs(clid))
	})
}

type clUpdaterMock struct {
	m            sync.Mutex
	refreshedCLs common.CLIDs
}

func (c *clUpdaterMock) ScheduleBatch(ctx context.Context, luciProject string, forceNotify bool, cls []*changelist.CL) error {
	c.m.Lock()
	for _, cl := range cls {
		c.refreshedCLs = append(c.refreshedCLs, cl.ID)
	}
	c.m.Unlock()
	return nil
}
