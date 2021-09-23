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
	"go.chromium.org/luci/server/tq/tqtesting"
	"google.golang.org/protobuf/types/known/timestamppb"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/configs/prjcfg/prjcfgtest"
	"go.chromium.org/luci/cv/internal/cvtesting"
	gcancel "go.chromium.org/luci/cv/internal/gerrit/cancel"
	gf "go.chromium.org/luci/cv/internal/gerrit/gerritfake"
	"go.chromium.org/luci/cv/internal/gerrit/trigger"
	"go.chromium.org/luci/cv/internal/prjmanager"
	"go.chromium.org/luci/cv/internal/prjmanager/pmtest"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/bq"
	"go.chromium.org/luci/cv/internal/run/impl/state"
	"go.chromium.org/luci/cv/internal/run/pubsub"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
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

		impl, deps := makeImpl(&ct)
		se := impl.endRun(ctx, rs, run.Status_FAILED)
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
		ct.TQ.Run(ctx, tqtesting.StopAfterTask(changelist.BatchOnCLUpdatedTaskClass))
		pmtest.AssertReceivedRunFinished(ctx, rid)
		pmtest.AssertReceivedCLsNotified(ctx, rid.LUCIProject(), []*changelist.CL{&cl})
		So(deps.clUpdater.refreshedCLs, ShouldResemble, common.MakeCLIDs(clid))

		Convey("Publish RunEnded event", func() {
			var task *pubsub.PublishRunEndedTask
			for _, t := range ct.TQ.Tasks() {
				if p, ok := t.Payload.(*pubsub.PublishRunEndedTask); ok {
					task = p
					break
				}
			}
			So(task, ShouldResembleProto, &pubsub.PublishRunEndedTask{
				PublicId:    rs.Run.ID.PublicID(),
				LuciProject: rs.Run.ID.LUCIProject(),
				Status:      rs.Run.Status,
				Eversion:    int64(rs.Run.EVersion + 1),
			})
		})
	})
}

func TestCancelTriggers(t *testing.T) {
	t.Parallel()

	Convey("CancelTriggers on a CL just deleted by its owner force-updates CL", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()

		const gHost = "x-review.example.com"
		const gRepo = "luci-go"
		const gChange = 123
		const lProject = "infra"
		const clid = 1

		prjcfgtest.Create(ctx, lProject, &cfgpb.Config{
			ConfigGroups: []*cfgpb.ConfigGroup{
				{
					Name: "main",
					Gerrit: []*cfgpb.ConfigGroup_Gerrit{{
						Url: "https://" + gHost,
						Projects: []*cfgpb.ConfigGroup_Gerrit_Project{
							{Name: gRepo, RefRegexp: []string{"refs/heads/.+"}},
						},
					}},
				},
			},
		})
		cgs, err := prjcfgtest.MustExist(ctx, lProject).GetConfigGroups(ctx)
		So(err, ShouldBeNil)
		cg := cgs[0]

		rid := common.MakeRunID("infra", ct.Clock.Now(), 1, []byte("deadbeef"))
		ci := gf.CI(gChange, gf.CQ(+2))
		cl := changelist.CL{
			ID:             clid,
			ExternalID:     changelist.MustGobID(gHost, gChange),
			IncompleteRuns: common.RunIDs{rid},
			EVersion:       3,
			UpdateTime:     ct.Clock.Now().UTC(),
			Snapshot: &changelist.Snapshot{
				Kind: &changelist.Snapshot_Gerrit{Gerrit: &changelist.Gerrit{
					Host: gHost,
					Info: ci,
				}},
				LuciProject:        lProject,
				ExternalUpdateTime: timestamppb.New(ct.Clock.Now()),
			},
		}
		rcl := run.RunCL{
			ID:         clid,
			Run:        datastore.MakeKey(ctx, run.RunKind, string(rid)),
			ExternalID: cl.ExternalID,
			Detail:     cl.Snapshot,
			Trigger:    trigger.Find(ci, cg.Content),
		}
		So(datastore.Put(ctx, &cl, &rcl), ShouldBeNil)

		// Simulate CL no longer existing in Gerrit (e.g. ct.GFake) 1 minute later,
		// just as Run Manager decides to cancel the CL triggers.
		ct.Clock.Add(time.Minute)
		impl, deps := makeImpl(&ct)
		err = impl.cancelCLTriggers(ctx, rid, []*run.RunCL{&rcl}, []changelist.ExternalID{cl.ExternalID}, "Dry Run OK", cg)
		// The cancelation errors out, but the CL refresh is scheduled.
		// TODO(crbug/1227369): fail transiently or better yet schedule another
		// retry in the future and fail with tq.Ignore.
		So(gcancel.ErrPermanentTag.In(err), ShouldBeTrue)
		So(deps.clUpdater.refreshedCLs, ShouldResemble, common.MakeCLIDs(clid))
	})
}

type dependencies struct {
	pm        *prjmanager.Notifier
	rm        *run.Notifier
	clUpdater *clUpdaterMock
}

func makeTestHandler(ct *cvtesting.Test) (Handler, dependencies) {
	// TODO(yiwzhang): add a wrapper around handler to ensure runState is not
	// accidentally modified in place.
	return makeImpl(ct)
}

// makeImpl should only be used to test common functions. For testing handler,
// please use makeTestHandler instead.
func makeImpl(ct *cvtesting.Test) (*Impl, dependencies) {
	deps := dependencies{
		pm:        prjmanager.NewNotifier(ct.TQDispatcher),
		rm:        run.NewNotifier(ct.TQDispatcher),
		clUpdater: &clUpdaterMock{},
	}
	impl := &Impl{
		PM:         deps.pm,
		RM:         deps.rm,
		CLMutator:  changelist.NewMutator(ct.TQDispatcher, deps.pm, deps.rm),
		CLUpdater:  deps.clUpdater,
		TreeClient: ct.TreeFake.Client(),
		GFactory:   ct.GFactory(),
		BQExporter: bq.NewExporter(ct.TQDispatcher, ct.BQFake),
		Publisher:  pubsub.NewPublisher(ct.TQDispatcher),
	}
	return impl, deps
}

type clUpdaterMock struct {
	m            sync.Mutex
	refreshedCLs common.CLIDs
}

func (c *clUpdaterMock) ScheduleBatch(ctx context.Context, luciProject string, cls []*changelist.CL) error {
	c.m.Lock()
	for _, cl := range cls {
		c.refreshedCLs = append(c.refreshedCLs, cl.ID)
	}
	c.m.Unlock()
	return nil
}
