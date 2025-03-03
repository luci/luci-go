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

	bbpb "go.chromium.org/luci/buildbucket/proto"
	bbutil "go.chromium.org/luci/buildbucket/protoutil"
	"go.chromium.org/luci/common/clock"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
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
	"go.chromium.org/luci/cv/internal/tryjob/requirement"
)

func TestUpdateConfig(t *testing.T) {
	ftt.Run("OnCLUpdated", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		const (
			lProject    = "chromium"
			gHost       = "x-review.example.com"
			gRepoFirst  = "repo/first"
			gRepoSecond = "repo/second"
			gRef        = "refs/heads/main"
		)
		runID := common.MakeRunID(lProject, ct.Clock.Now(), 1, []byte("deadbeef"))
		builder := &bbpb.BuilderID{
			Project: lProject,
			Bucket:  "bucket",
			Builder: "some-builder",
		}

		putRunCL := func(ci *gerritpb.ChangeInfo, cg *cfgpb.ConfigGroup) {
			triggers := trigger.Find(&trigger.FindInput{ChangeInfo: ci, ConfigGroup: cg})
			assert.Loosely(t, triggers.GetCqVoteTrigger(), should.NotBeNil)
			rcl := run.RunCL{
				Run:        datastore.MakeKey(ctx, common.RunKind, string(runID)),
				ID:         common.CLID(ci.GetNumber()),
				ExternalID: changelist.MustGobID(gHost, ci.GetNumber()),
				Trigger:    triggers.GetCqVoteTrigger(),
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
			assert.Loosely(t, datastore.Put(ctx, &rcl), should.BeNil)
		}

		// Seed project with one version of prior config.
		prjcfgtest.Create(ctx, lProject, &cfgpb.Config{ConfigGroups: []*cfgpb.ConfigGroup{{Name: "ev1"}}})
		metaBefore := prjcfgtest.MustExist(ctx, lProject)
		assert.Loosely(t, metaBefore.EVersion, should.Equal(1))
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
					Verifiers: &cfgpb.Verifiers{
						Tryjob: &cfgpb.Verifiers_Tryjob{
							Builders: []*cfgpb.Verifiers_Tryjob_Builder{
								{Name: bbutil.FormatBuilderID(builder)},
							},
						},
					},
					AdditionalModes: []*cfgpb.Mode{{
						Name:            "CUSTOM_RUN",
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
			// Custom+1 has no effect as AdditionalModes above is misconfigured.
			gf.Vote("Custom", +1, triggerTime, gf.U("user-1")),
		), cgMain)
		rs := &state.RunState{
			Run: run.Run{
				ID:            runID,
				CLs:           common.MakeCLIDs(1, 2),
				CreateTime:    triggerTime,
				StartTime:     triggerTime.Add(1 * time.Minute),
				Status:        run.Status_RUNNING,
				ConfigGroupID: prjcfgtest.MustExist(ctx, lProject).ConfigGroupIDs[0], // main
				Tryjobs: &run.Tryjobs{
					RequirementVersion:    1,
					RequirementComputedAt: timestamppb.New(triggerTime.Add(1 * time.Minute)),
				},
				Mode: run.DryRun,
			},
		}
		runCLs, err := run.LoadRunCLs(ctx, rs.ID, rs.CLs)
		assert.NoErr(t, err)
		initialReqmt, err := requirement.Compute(ctx, requirement.Input{
			GFactory:    ct.GFactory(),
			ConfigGroup: cgMain,
			RunOwner:    rs.Owner,
			CLs:         runCLs,
			RunOptions:  rs.Options,
			RunMode:     rs.Mode,
		})
		assert.NoErr(t, err)
		assert.Loosely(t, initialReqmt.OK(), should.BeTrue)
		rs.Tryjobs.Requirement = initialReqmt.Requirement
		// Prepare new config as a copy of existing one. Add extra ConfigGroup to it
		// to ensure its hash will always differ.
		cfgNew := proto.Clone(cfgCurrent).(*cfgpb.Config)
		cfgNew.ConfigGroups = append(cfgNew.ConfigGroups, &cfgpb.ConfigGroup{Name: "foo"})

		h, _ := makeTestHandler(&ct)

		updateConfig := func() *Result {
			prjcfgtest.Update(ctx, lProject, cfgNew)
			metaNew := prjcfgtest.MustExist(ctx, lProject)
			res, err := h.UpdateConfig(ctx, rs, metaNew.Hash())
			assert.NoErr(t, err)
			return res
		}

		t.Run("Noop", func(t *ftt.Test) {
			ensureNoop := func(res *Result) {
				assert.Loosely(t, res.State, should.Equal(rs))
				assert.Loosely(t, res.SideEffectFn, should.BeNil)
				assert.Loosely(t, res.PreserveEvents, should.BeFalse)
			}

			for _, status := range []run.Status{
				run.Status_SUCCEEDED,
				run.Status_FAILED,
				run.Status_CANCELLED,
			} {
				t.Run(fmt.Sprintf("When Run is %s", status), func(t *ftt.Test) {
					rs.Status = status
					ensureNoop(updateConfig())
				})
			}
			t.Run("When given config hash isn't new", func(t *ftt.Test) {
				t.Run("but is the same as current", func(t *ftt.Test) {
					res, err := h.UpdateConfig(ctx, rs, metaCurrent.Hash())
					assert.NoErr(t, err)
					ensureNoop(res)
				})
				t.Run("but is older than current", func(t *ftt.Test) {
					res, err := h.UpdateConfig(ctx, rs, metaBefore.Hash())
					assert.NoErr(t, err)
					ensureNoop(res)
				})
			})
		})

		t.Run("Preserve events for SUBMITTING Run", func(t *ftt.Test) {
			rs.Status = run.Status_SUBMITTING
			res := updateConfig()
			assert.Loosely(t, res.State, should.Equal(rs))
			assert.Loosely(t, res.SideEffectFn, should.BeNil)
			assert.Loosely(t, res.PreserveEvents, should.BeTrue)
		})

		t.Run("Upgrades to newer config version when", func(t *ftt.Test) {
			ensureUpdated := func(expectedGroupName string) *Result {
				res := updateConfig()
				assert.Loosely(t, res.State.ConfigGroupID.Hash(), should.NotEqual(metaCurrent.Hash()))
				assert.Loosely(t, res.State.ConfigGroupID.Name(), should.Equal(expectedGroupName))
				assert.Loosely(t, res.State.Status, should.Equal(run.Status_RUNNING))
				assert.Loosely(t, res.State.LogEntries, should.HaveLength(1))
				assert.Loosely(t, res.State.LogEntries[0].GetConfigChanged(), should.NotBeNil)
				assert.Loosely(t, res.SideEffectFn, should.BeNil)
				assert.Loosely(t, res.PreserveEvents, should.BeFalse)
				return res
			}
			t.Run("ConfigGroup is same", func(t *ftt.Test) {
				ensureUpdated("main")
			})
			t.Run("ConfigGroup renamed", func(t *ftt.Test) {
				cfgNew.ConfigGroups[0].Name = "blah"
				ensureUpdated("blah")
			})
			t.Run("ConfigGroup re-ordered and renamed", func(t *ftt.Test) {
				cfgNew.ConfigGroups[0].Name = "blah"
				cfgNew.ConfigGroups[0], cfgNew.ConfigGroups[1] = cfgNew.ConfigGroups[1], cfgNew.ConfigGroups[0]
				ensureUpdated("blah")
			})
			t.Run("Verifier config changed", func(t *ftt.Test) {
				cfgNew.ConfigGroups[0].Verifiers.TreeStatus = &cfgpb.Verifiers_TreeStatus{Url: "https://whatever.example.com"}
				res := ensureUpdated("main")
				assert.Loosely(t, res.State.Tryjobs.GetRequirementVersion(), should.Equal(rs.Tryjobs.GetRequirementVersion()))
			})
			t.Run("Watched refs changed", func(t *ftt.Test) {
				cfgNew.ConfigGroups[0].Gerrit[0].Projects[0].RefRegexpExclude = []string{"refs/heads/exclude"}
				ensureUpdated("main")
			})
			t.Run("Tryjob requirement changed", func(t *ftt.Test) {
				tryjobVerifier := cfgNew.ConfigGroups[0].Verifiers.Tryjob
				tryjobVerifier.Builders = append(tryjobVerifier.Builders,
					&cfgpb.Verifiers_Tryjob_Builder{
						Name: fmt.Sprintf("%s/another-bucket/another-builder", lProject),
					})
				res := ensureUpdated("main")
				assert.Loosely(t, proto.Equal(res.State.Tryjobs.GetRequirement(), rs.Tryjobs.GetRequirement()), should.BeFalse)
				assert.Loosely(t, res.State.Tryjobs.GetRequirementVersion(), should.Equal(rs.Tryjobs.GetRequirementVersion()+1))
				assert.That(t, res.State.Tryjobs.GetRequirementComputedAt().AsTime(), should.Match(ct.Clock.Now().UTC()))
			})
		})

		t.Run("Cancel Run when", func(t *ftt.Test) {
			ensureCancelled := func() {
				res := updateConfig()
				// Applicable ConfigGroupID should remain the same.
				assert.Loosely(t, res.State.ConfigGroupID, should.Equal(rs.ConfigGroupID))
				assert.Loosely(t, res.State.Status, should.Equal(run.Status_CANCELLED))
				assert.Loosely(t, res.SideEffectFn, should.NotBeNil)
				assert.Loosely(t, res.PreserveEvents, should.BeFalse)
			}
			t.Run("a CL is no longer watched", func(t *ftt.Test) {
				cfgNew.ConfigGroups[0].Gerrit[0].Projects[1].Name = "repo/different"
				ensureCancelled()
			})
			t.Run("a CL is watched by >1 ConfigGroup", func(t *ftt.Test) {
				cfgNew.ConfigGroups[1].Gerrit[0].Projects[0].Name = gRepoFirst
				ensureCancelled()
			})
			t.Run("CLs are watched by different ConfigGroups", func(t *ftt.Test) {
				cfgNew.ConfigGroups[0].Gerrit[0].Projects[0].Name = "repo/different"
				cfgNew.ConfigGroups[1].Gerrit[0].Projects[0].Name = gRepoFirst
				ensureCancelled()
			})
			t.Run("CLs trigger has changed", func(t *ftt.Test) {
				cfgNew.ConfigGroups[0].AdditionalModes[0].TriggeringLabel = "Custom"
				ensureCancelled()
			})
		})
	})
}
