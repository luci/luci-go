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
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/gae/service/datastore"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/configs/prjcfg/prjcfgtest"
	"go.chromium.org/luci/cv/internal/cvtesting"
	gf "go.chromium.org/luci/cv/internal/gerrit/gerritfake"
	"go.chromium.org/luci/cv/internal/gerrit/trigger"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/eventpb"
	"go.chromium.org/luci/cv/internal/run/impl/state"

	. "github.com/smartystreets/goconvey/convey"
)

func TestOnCLsUpdated(t *testing.T) {
	Convey("OnCLsUpdated", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()

		const lProject = "chromium"
		const gHost = "x-review.example.com"

		cfg := &cfgpb.Config{
			ConfigGroups: []*cfgpb.ConfigGroup{
				{Name: "main"},
			},
		}
		prjcfgtest.Create(ctx, lProject, cfg)
		h, _, _, _ := makeTestImpl(&ct)

		// initial state
		triggerTime := clock.Now(ctx).UTC()
		rs := &state.RunState{
			Run: run.Run{
				ID:            common.MakeRunID(lProject, ct.Clock.Now(), 1, []byte("deadbeef")),
				StartTime:     triggerTime.Add(1 * time.Minute),
				Status:        run.Status_RUNNING,
				ConfigGroupID: prjcfgtest.MustExist(ctx, lProject).ConfigGroupIDs[0],
			},
		}
		updateCL := func(ci *gerritpb.ChangeInfo, ap *changelist.ApplicableConfig, acc *changelist.Access) changelist.CL {
			cl := changelist.CL{
				ID: 1,
				Snapshot: &changelist.Snapshot{
					LuciProject: lProject,
					Patchset:    ci.GetRevisions()[ci.GetCurrentRevision()].GetNumber(),
					Kind: &changelist.Snapshot_Gerrit{
						Gerrit: &changelist.Gerrit{
							Host: gHost,
							Info: ci,
						},
					},
				},
				ApplicableConfig: ap,
				Access:           acc,
			}

			So(datastore.Put(ctx, &cl), ShouldBeNil)
			return cl
		}

		aplConfigOK := &changelist.ApplicableConfig{Projects: []*changelist.ApplicableConfig_Project{
			{Name: lProject, ConfigGroupIds: prjcfgtest.MustExist(ctx, lProject).ConfigGroupNames},
		}}
		accessOK := (*changelist.Access)(nil)

		const gChange = 1
		const gPatchSet = 5

		ci := gf.CI(gChange, gf.PS(gPatchSet), gf.CQ(+2, triggerTime, gf.U("foo")))
		cl := updateCL(ci, aplConfigOK, accessOK)
		rcl := run.RunCL{
			ID:      1,
			Run:     datastore.MakeKey(ctx, run.RunKind, string(rs.Run.ID)),
			Detail:  cl.Snapshot,
			Trigger: trigger.Find(ci, cfg.GetConfigGroups()[0]),
		}
		So(rcl.Trigger, ShouldNotBeNil) // ensure trigger find is working fine.
		So(datastore.Put(ctx, &rcl), ShouldBeNil)

		ensureNoop := func() {
			res, err := h.OnCLsUpdated(ctx, rs, common.CLIDs{1})
			So(err, ShouldBeNil)
			So(res.State, ShouldEqual, rs)
			So(res.SideEffectFn, ShouldBeNil)
			So(res.PreserveEvents, ShouldBeFalse)
		}
		Convey("Noop", func() {
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
					updateCL(newCI, aplConfigOK, accessOK)
					ensureNoop()
				})

				Convey("is triggered by different user at the exact same time", func() {
					updateCL(gf.CI(gChange, gf.PS(gPatchSet), gf.CQ(+2, triggerTime, gf.U("bar"))), aplConfigOK, accessOK)
					ensureNoop()
				})
			})
		})
		Convey("Preserve events for SUBMITTING Run", func() {
			rs.Run.Status = run.Status_SUBMITTING
			res, err := h.OnCLsUpdated(ctx, rs, common.CLIDs{1})
			So(err, ShouldBeNil)
			So(res.State, ShouldEqual, rs)
			So(res.SideEffectFn, ShouldBeNil)
			So(res.PreserveEvents, ShouldBeTrue)
		})

		runAndVerifyCancelled := func() {
			res, err := h.OnCLsUpdated(ctx, rs, common.CLIDs{1})
			So(err, ShouldBeNil)
			So(res.State.Run.Status, ShouldEqual, run.Status_CANCELLED)
			So(res.SideEffectFn, ShouldNotBeNil)
			So(res.PreserveEvents, ShouldBeFalse)
		}

		Convey("Cancels Run on new Patchset", func() {
			updateCL(gf.CI(gChange, gf.PS(gPatchSet+1), gf.CQ(+2, triggerTime, gf.U("foo"))), aplConfigOK, accessOK)
			runAndVerifyCancelled()
		})
		Convey("Cancels Run on moved Ref", func() {
			updateCL(gf.CI(gChange, gf.PS(gPatchSet), gf.CQ(+2, triggerTime, gf.U("foo")), gf.Ref("refs/heads/new")), aplConfigOK, accessOK)
			runAndVerifyCancelled()
		})
		Convey("Cancels Run on removed trigger", func() {
			newCI := gf.CI(gChange, gf.PS(gPatchSet), gf.CQ(0, triggerTime.Add(1*time.Minute), gf.U("foo")))
			So(trigger.Find(newCI, cfg.GetConfigGroups()[0]), ShouldBeNil)
			updateCL(newCI, aplConfigOK, accessOK)
			runAndVerifyCancelled()
		})
		Convey("Cancels Run on changed mode", func() {
			updateCL(gf.CI(gChange, gf.PS(gPatchSet), gf.CQ(+1, triggerTime.Add(1*time.Minute), gf.U("foo"))), aplConfigOK, accessOK)
			runAndVerifyCancelled()
		})
		Convey("Cancels Run on change of triggering time", func() {
			updateCL(gf.CI(gChange, gf.PS(gPatchSet), gf.CQ(+2, triggerTime.Add(2*time.Minute), gf.U("foo"))), aplConfigOK, accessOK)
			runAndVerifyCancelled()
		})

		Convey("Change of access level to the CL", func() {
			Convey("cancel if another project started watching the same CL", func() {
				ac := proto.Clone(aplConfigOK).(*changelist.ApplicableConfig)
				ac.Projects = append(ac.Projects, &changelist.ApplicableConfig_Project{
					Name: "other-project", ConfigGroupIds: []string{"other-group"},
				})
				updateCL(ci, ac, accessOK)
				runAndVerifyCancelled()
			})
			Convey("wait if code review access was just lost, potentially due to eventual consistency", func() {
				noAccessAt := ct.Clock.Now().Add(42 * time.Second)
				acc := &changelist.Access{ByProject: map[string]*changelist.Access_Project{
					// Set NoAccessTime to the future, providing some grace period to
					// recover.
					lProject: {NoAccessTime: timestamppb.New(noAccessAt)},
				}}
				updateCL(ci, aplConfigOK, acc)
				res, err := h.OnCLsUpdated(ctx, rs, common.CLIDs{1})
				So(err, ShouldBeNil)
				So(res.State, ShouldEqual, rs)
				So(res.SideEffectFn, ShouldBeNil)
				// Event must be preserved, s.t. the same CL is re-visited later.
				So(res.PreserveEvents, ShouldBeTrue)
				// And Run Manager must have a task to re-check itself at around
				// NoAccessTime.
				So(ct.TQ.Tasks().Payloads(), ShouldHaveLength, 1)
				So(ct.TQ.Tasks().Payloads()[0].(*eventpb.ManageRunTask).GetRunId(), ShouldResemble, string(rs.Run.ID))
				So(ct.TQ.Tasks()[0].ETA, ShouldHappenOnOrBetween, noAccessAt, noAccessAt.Add(time.Second))
			})
			Convey("cancel if code review access was lost a while ago", func() {
				acc := &changelist.Access{ByProject: map[string]*changelist.Access_Project{
					lProject: {NoAccessTime: timestamppb.New(ct.Clock.Now())},
				}}
				updateCL(ci, aplConfigOK, acc)
				runAndVerifyCancelled()
			})
			Convey("wait if access level is unknown", func() {
				cl.Snapshot = nil
				cl.EVersion++
				So(datastore.Put(ctx, &cl), ShouldBeNil)
				ensureNoop()
			})
		})
	})
}
