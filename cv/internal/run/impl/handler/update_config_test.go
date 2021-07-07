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

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/configs/prjcfg/prjcfgtest"
	"go.chromium.org/luci/cv/internal/cvtesting"
	gf "go.chromium.org/luci/cv/internal/gerrit/gerritfake"
	"go.chromium.org/luci/cv/internal/gerrit/trigger"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/impl/state"

	. "github.com/smartystreets/goconvey/convey"
)

func TestUpdateConfig(t *testing.T) {
	Convey("OnCLUpdated", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()

		const (
			lProject    = "chromium"
			gHost       = "x-review.example.com"
			gRepoFirst  = "repo/first"
			gRepoSecond = "repo/second"
			gRef        = "refs/heads/main"
		)
		runID := common.MakeRunID(lProject, ct.Clock.Now(), 1, []byte("deadbeef"))
		// TODO(tandrii): remove this once Run finalization fully conducted by CV.
		ct.EnableCVRunManagement(ctx, lProject)

		putRunCL := func(ci *gerritpb.ChangeInfo, cg *cfgpb.ConfigGroup) {
			tr := trigger.Find(ci, cg)
			So(tr, ShouldNotBeNil)
			rcl := run.RunCL{
				Run:        datastore.MakeKey(ctx, run.RunKind, string(runID)),
				ID:         common.CLID(ci.GetNumber()),
				ExternalID: changelist.MustGobID(gHost, ci.GetNumber()),
				Trigger:    tr,
				Detail: &changelist.Snapshot{
					Patchset: ci.GetRevisions()[ci.GetCurrentRevision()].GetNumber(),
					Kind: &changelist.Snapshot_Gerrit{
						Gerrit: &changelist.Gerrit{
							Info: ci,
							Host: gHost,
						},
					},
				},
			}
			So(datastore.Put(ctx, &rcl), ShouldBeNil)
		}

		// Seed project with one version of prior config.
		prjcfgtest.Create(ctx, lProject, &cfgpb.Config{ConfigGroups: []*cfgpb.ConfigGroup{{Name: "ev1"}}})
		metaBefore := prjcfgtest.MustExist(ctx, lProject)
		So(metaBefore.EVersion, ShouldEqual, 1)
		// Set up initial Run state.
		cfgCurrent := &cfgpb.Config{
			ConfigGroups: []*cfgpb.ConfigGroup{
				{
					Name: "main",
					Gerrit: []*cfgpb.ConfigGroup_Gerrit{{
						Url: "https://" + gHost,
						Projects: []*cfgpb.ConfigGroup_Gerrit_Project{
							{Name: gRepoFirst, RefRegexp: []string{"refs/heads/.+"}},
							{Name: gRepoSecond, RefRegexp: []string{"refs/heads/.+"}},
						},
					}},
					AdditionalModes: []*cfgpb.Mode{{
						Name:            "QUICK_DRY_RUN",
						CqLabelValue:    1,
						TriggeringValue: 1,
						TriggeringLabel: "Will-Be-Changed-In-Tests-Below",
					}},
				},
				{
					Name: "special",
					Gerrit: []*cfgpb.ConfigGroup_Gerrit{{
						Url: "https://" + gHost,
						Projects: []*cfgpb.ConfigGroup_Gerrit_Project{{
							Name:      "repo/will-be-replaced-in-tests-below",
							RefRegexp: []string{"refs/heads/.+"},
						}},
					}},
				},
			},
		}
		prjcfgtest.Update(ctx, lProject, cfgCurrent)
		metaCurrent := prjcfgtest.MustExist(ctx, lProject)
		triggerTime := clock.Now(ctx).UTC()
		cgMain := cfgCurrent.GetConfigGroups()[0]
		putRunCL(gf.CI(1, gf.Project(gRepoFirst), gf.CQ(+1, triggerTime, gf.U("user-1"))), cgMain)
		putRunCL(gf.CI(
			2, gf.Project(gRepoSecond),
			gf.CQ(+1, triggerTime, gf.U("user-1")),
			// Quick+1 has no effect as AdditionalModes above is misconfigured.
			gf.Vote("Quick", +1, triggerTime, gf.U("user-1")),
		), cgMain)
		rs := &state.RunState{
			Run: run.Run{
				ID:            runID,
				CLs:           common.MakeCLIDs(1, 2),
				CreateTime:    triggerTime,
				StartTime:     triggerTime.Add(1 * time.Minute),
				Status:        run.Status_RUNNING,
				ConfigGroupID: prjcfgtest.MustExist(ctx, lProject).ConfigGroupIDs[0], // main
			},
		}
		// Prepare new config as a copy of existing one. Add extra ConfigGroup to it
		// to ensure its hash will always differ.
		cfgNew := proto.Clone(cfgCurrent).(*cfgpb.Config)
		cfgNew.ConfigGroups = append(cfgNew.ConfigGroups, &cfgpb.ConfigGroup{Name: "foo"})

		h := &Impl{
			GFactory:  ct.GFake.Factory(),
			CLUpdater: &clUpdaterMock{},
		}

		updateConfig := func() *Result {
			prjcfgtest.Update(ctx, lProject, cfgNew)
			metaNew := prjcfgtest.MustExist(ctx, lProject)
			res, err := h.UpdateConfig(ctx, rs, metaNew.Hash())
			So(err, ShouldBeNil)
			return res
		}

		Convey("Noop", func() {
			ensureNoop := func(res *Result) {
				So(res.State, ShouldEqual, rs)
				So(res.SideEffectFn, ShouldBeNil)
				So(res.PreserveEvents, ShouldBeFalse)
			}

			for _, status := range []run.Status{
				run.Status_SUCCEEDED,
				run.Status_FAILED,
				run.Status_CANCELLED,
			} {
				Convey(fmt.Sprintf("When Run is %s", status), func() {
					rs.Run.Status = status
					ensureNoop(updateConfig())
				})
			}
			Convey("When given config hash isn't new", func() {
				Convey("but is the same as current", func() {
					res, err := h.UpdateConfig(ctx, rs, metaCurrent.Hash())
					So(err, ShouldBeNil)
					ensureNoop(res)
				})
				Convey("but is older than current", func() {
					res, err := h.UpdateConfig(ctx, rs, metaBefore.Hash())
					So(err, ShouldBeNil)
					ensureNoop(res)
				})
			})
		})

		Convey("Preserve events for SUBMITTING Run", func() {
			rs.Run.Status = run.Status_SUBMITTING
			res := updateConfig()
			So(res.State, ShouldEqual, rs)
			So(res.SideEffectFn, ShouldBeNil)
			So(res.PreserveEvents, ShouldBeTrue)
		})

		Convey("Upgrades to newer config version when", func() {
			ensureUpdated := func(expectedGroupName string) {
				res := updateConfig()
				So(res.State.Run.ConfigGroupID.Hash(), ShouldNotEqual, metaCurrent.Hash())
				So(res.State.Run.ConfigGroupID.Name(), ShouldEqual, expectedGroupName)
				So(res.State.Run.Status, ShouldEqual, run.Status_RUNNING)
				So(res.SideEffectFn, ShouldBeNil)
				So(res.PreserveEvents, ShouldBeFalse)
			}
			Convey("ConfigGroup is same", func() {
				ensureUpdated("main")
			})
			Convey("ConfigGroup renamed", func() {
				cfgNew.ConfigGroups[0].Name = "blah"
				ensureUpdated("blah")
			})
			Convey("ConfigGroup re-ordered and renamed", func() {
				cfgNew.ConfigGroups[0].Name = "blah"
				cfgNew.ConfigGroups[0], cfgNew.ConfigGroups[1] = cfgNew.ConfigGroups[1], cfgNew.ConfigGroups[0]
				ensureUpdated("blah")
			})
			Convey("Verifier config changed", func() {
				cfgNew.ConfigGroups[0].Verifiers = &cfgpb.Verifiers{
					TreeStatus: &cfgpb.Verifiers_TreeStatus{Url: "https://whatever.example.com"},
				}
				ensureUpdated("main")
			})
			Convey("Watched refs changed", func() {
				cfgNew.ConfigGroups[0].Gerrit[0].Projects[0].RefRegexpExclude = []string{"refs/heads/exclude"}
				ensureUpdated("main")
			})
		})

		Convey("Cancel Run when", func() {
			ensureCancelled := func() {
				res := updateConfig()
				// Applicable ConfigGroupID should remain the same.
				So(res.State.Run.ConfigGroupID, ShouldEqual, rs.Run.ConfigGroupID)
				So(res.State.Run.Status, ShouldEqual, run.Status_CANCELLED)
				So(res.SideEffectFn, ShouldNotBeNil)
				So(res.PreserveEvents, ShouldBeFalse)
			}
			Convey("a CL is no longer watched", func() {
				cfgNew.ConfigGroups[0].Gerrit[0].Projects[1].Name = "repo/different"
				ensureCancelled()
			})
			Convey("a CL is watched by >1 ConfigGroup", func() {
				cfgNew.ConfigGroups[1].Gerrit[0].Projects[0].Name = gRepoFirst
				ensureCancelled()
			})
			Convey("CLs are watched by different ConfigGroups", func() {
				cfgNew.ConfigGroups[0].Gerrit[0].Projects[0].Name = "repo/different"
				cfgNew.ConfigGroups[1].Gerrit[0].Projects[0].Name = gRepoFirst
				ensureCancelled()
			})
			Convey("CLs trigger has changed", func() {
				cfgNew.ConfigGroups[0].AdditionalModes[0].TriggeringLabel = "Quick"
				ensureCancelled()
			})
		})
	})
}
