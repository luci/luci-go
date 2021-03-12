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

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/clock"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/cvtesting"
	gf "go.chromium.org/luci/cv/internal/gerrit/gerritfake"
	"go.chromium.org/luci/cv/internal/gerrit/trigger"
	"go.chromium.org/luci/cv/internal/prjmanager/pmtest"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/impl/state"

	. "github.com/smartystreets/goconvey/convey"
)

func TestOnCLUpdated(t *testing.T) {
	Convey("OnCLUpdated", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()
		h := &Impl{}

		// initial state
		triggerTime := clock.Now(ctx).UTC()
		rs := &state.RunState{
			Run: run.Run{
				ID:        common.RunID("chromium/111-2-deadbeef"),
				StartTime: triggerTime.Add(1 * time.Minute),
				Status:    run.Status_RUNNING,
			},
		}
		UpdateCL := func(ci *gerritpb.ChangeInfo) changelist.CL {
			cl := changelist.CL{
				ID: 1,
				Snapshot: &changelist.Snapshot{
					Patchset: ci.GetRevisions()[ci.GetCurrentRevision()].GetNumber(),
					Kind: &changelist.Snapshot_Gerrit{
						Gerrit: &changelist.Gerrit{
							Info: ci,
						},
					},
				},
			}

			So(datastore.Put(ctx, &cl), ShouldBeNil)
			return cl
		}

		ci := gf.CI(1, gf.PS(5), gf.CQ(2, triggerTime, gf.U("foo")))
		cl := UpdateCL(ci)
		rcl := run.RunCL{
			ID:      1,
			Run:     datastore.MakeKey(ctx, run.RunKind, string(rs.Run.ID)),
			Detail:  cl.Snapshot,
			Trigger: trigger.Find(ci),
		}
		So(rcl.Trigger, ShouldNotBeNil) // ensure trigger find is working fine.
		So(datastore.Put(ctx, &rcl), ShouldBeNil)

		Convey("Noop", func() {
			ensureNoop := func() {
				res, err := h.OnCLUpdated(ctx, rs, common.CLIDs{1})
				So(err, ShouldBeNil)
				So(res.State, ShouldEqual, rs)
				So(res.SideEffectFn, ShouldBeNil)
				So(res.PreserveEvents, ShouldBeFalse)
			}
			statuses := []run.Status{
				run.Status_SUCCEEDED,
				run.Status_FAILED,
				run.Status_CANCELLED,
			}
			for _, status := range statuses {
				Convey(fmt.Sprintf("When Run is %s", status), func() {
					rs.Run.Status = status
					ensureNoop()
				})
			}

			Convey("When new CL Version", func() {
				Convey("is a message update", func() {
					newCI := proto.Clone(ci).(*gerritpb.ChangeInfo)
					gf.Messages(&gerritpb.ChangeMessageInfo{
						Message: "This is a message",
					})(newCI)
					UpdateCL(newCI)
					ensureNoop()
				})

				Convey("regress to DryRun", func() {
					newCI := gf.CI(1, gf.PS(5), gf.CQ(1, triggerTime, gf.U("foo")))
					UpdateCL(newCI)
					ensureNoop()
				})

				Convey("switch FullRun trigger to different user", func() {
					newCI := gf.CI(2, gf.PS(5), gf.CQ(1, triggerTime, gf.U("bar")))
					UpdateCL(newCI)
					ensureNoop()
				})
			})
		})
		Convey("Preserve events for SUBMITTING Run", func() {
			rs.Run.Status = run.Status_SUBMITTING
			res, err := h.OnCLUpdated(ctx, rs, common.CLIDs{1})
			So(err, ShouldBeNil)
			So(res.State, ShouldEqual, rs)
			So(res.SideEffectFn, ShouldBeNil)
			So(res.PreserveEvents, ShouldBeTrue)
		})

		Convey("Cancels Run on new Patchset", func() {
			newCI := proto.Clone(ci).(*gerritpb.ChangeInfo)
			gf.PS(6)(newCI)
			UpdateCL(newCI)
			res, err := h.OnCLUpdated(ctx, rs, common.CLIDs{1})
			So(err, ShouldBeNil)
			So(res.State.Run.Status, ShouldEqual, run.Status_CANCELLED)
			So(res.SideEffectFn, ShouldNotBeNil)
			So(res.SideEffectFn(ctx), ShouldBeNil)
			So(res.PreserveEvents, ShouldBeFalse)
			pmtest.AssertReceivedRunFinished(ctx, rs.Run.ID)
		})
		Convey("Cancels Run on removed trigger", func() {
			newCI := gf.CI(2, gf.PS(5), gf.CQ(0))
			UpdateCL(newCI)

			res, err := h.OnCLUpdated(ctx, rs, common.CLIDs{1})
			So(err, ShouldBeNil)
			So(res.State.Run.Status, ShouldEqual, run.Status_CANCELLED)
			So(res.SideEffectFn, ShouldNotBeNil)
			So(res.SideEffectFn(ctx), ShouldBeNil)
			So(res.PreserveEvents, ShouldBeFalse)
			pmtest.AssertReceivedRunFinished(ctx, rs.Run.ID)
		})
	})
}
