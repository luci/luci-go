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

package state

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock/testclock"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/service/datastore"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/configs/prjcfg"
	"go.chromium.org/luci/cv/internal/configs/prjcfg/prjcfgtest"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/gerrit/cfgmatcher"
	gf "go.chromium.org/luci/cv/internal/gerrit/gerritfake"
	"go.chromium.org/luci/cv/internal/gerrit/gobmap/gobmaptest"
	"go.chromium.org/luci/cv/internal/gerrit/poller"
	"go.chromium.org/luci/cv/internal/gerrit/trigger"
	gerritupdater "go.chromium.org/luci/cv/internal/gerrit/updater"
	"go.chromium.org/luci/cv/internal/prjmanager"
	"go.chromium.org/luci/cv/internal/prjmanager/clpurger"
	"go.chromium.org/luci/cv/internal/prjmanager/prjpb"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/tryjob"
)

type ctest struct {
	cvtesting.Test

	lProject  string
	gHost     string
	pm        *prjmanager.Notifier
	clUpdater *changelist.Updater
}

func (ct *ctest) SetUp(testingT testing.TB) context.Context {
	ctx := ct.Test.SetUp(testingT)
	ct.pm = prjmanager.NewNotifier(ct.TQDispatcher)
	ct.clUpdater = changelist.NewUpdater(ct.TQDispatcher, changelist.NewMutator(ct.TQDispatcher, ct.pm, nil, tryjob.NewNotifier(ct.TQDispatcher)))
	gerritupdater.RegisterUpdater(ct.clUpdater, ct.GFactory())
	return ctx
}

func (ct ctest) runCLUpdater(ctx context.Context, change int64) *changelist.CL {
	return ct.runCLUpdaterAs(ctx, change, ct.lProject)
}

func (ct ctest) runCLUpdaterAs(ctx context.Context, change int64, lProject string) *changelist.CL {
	assert.Loosely(ct.TB, ct.clUpdater.TestingForceUpdate(ctx, &changelist.UpdateCLTask{
		LuciProject: lProject,
		ExternalId:  string(changelist.MustGobID(ct.gHost, change)),
		Requester:   changelist.UpdateCLTask_RUN_POKE,
	}), should.BeNil)
	eid, err := changelist.GobID(ct.gHost, change)
	assert.Loosely(ct.TB, err, should.BeNil)
	cl, err := eid.Load(ctx)
	assert.Loosely(ct.TB, err, should.BeNil)
	assert.Loosely(ct.TB, cl, should.NotBeNil)
	return cl
}

func (ct ctest) submitCL(ctx context.Context, change int64) *changelist.CL {
	ct.GFake.MutateChange(ct.gHost, int(change), func(c *gf.Change) {
		gf.Status(gerritpb.ChangeStatus_MERGED)(c.Info)
		gf.Updated(ct.Clock.Now())(c.Info)
	})
	cl := ct.runCLUpdater(ctx, change)

	// If this fails, you forgot to change fake time.
	assert.Loosely(ct.TB, cl.Snapshot.GetGerrit().GetInfo().GetStatus(), should.Equal(gerritpb.ChangeStatus_MERGED))
	return cl
}

const cfgText1 = `
  config_groups {
    name: "g0"
    gerrit {
      url: "https://c-review.example.com"  # Must match gHost.
      projects {
        name: "repo/a"
        ref_regexp: "refs/heads/main"
      }
    }
  }
  config_groups {
    name: "g1"
    fallback: YES
    gerrit {
      url: "https://c-review.example.com"  # Must match gHost.
      projects {
        name: "repo/a"
        ref_regexp: "refs/heads/.+"
      }
    }
  }
`

func updateConfigToNoFallabck(ctx context.Context, ct *ctest) prjcfg.Meta {
	ct.TB.Helper()
	cfgText2 := strings.ReplaceAll(cfgText1, "fallback: YES", "fallback: NO")
	cfg2 := &cfgpb.Config{}
	assert.Loosely(ct.TB, prototext.Unmarshal([]byte(cfgText2), cfg2), should.BeNil, truth.LineContext())
	prjcfgtest.Update(ctx, ct.lProject, cfg2)
	gobmaptest.Update(ctx, ct.lProject)
	return prjcfgtest.MustExist(ctx, ct.lProject)
}

func updateConfigRenameG1toG11(ctx context.Context, ct *ctest) prjcfg.Meta {
	ct.TB.Helper()
	cfgText2 := strings.ReplaceAll(cfgText1, `"g1"`, `"g11"`)
	cfg2 := &cfgpb.Config{}
	assert.Loosely(ct.TB, prototext.Unmarshal([]byte(cfgText2), cfg2), should.BeNil, truth.LineContext())
	prjcfgtest.Update(ctx, ct.lProject, cfg2)
	gobmaptest.Update(ctx, ct.lProject)
	return prjcfgtest.MustExist(ctx, ct.lProject)
}

func TestUpdateConfig(t *testing.T) {
	t.Parallel()

	ftt.Run("updateConfig works", t, func(t *ftt.Test) {
		ct := ctest{
			lProject: "test",
			gHost:    "c-review.example.com",
			Test:     cvtesting.Test{},
		}
		ctx := ct.SetUp(t)

		cfg1 := &cfgpb.Config{}
		assert.NoErr(t, prototext.Unmarshal([]byte(cfgText1), cfg1))

		prjcfgtest.Create(ctx, ct.lProject, cfg1)
		meta := prjcfgtest.MustExist(ctx, ct.lProject)
		gobmaptest.Update(ctx, ct.lProject)

		clPoller := poller.New(ct.TQDispatcher, nil, nil, nil)
		h := Handler{CLPoller: clPoller}

		t.Run("initializes newly started project", func(t *ftt.Test) {
			// Newly started project doesn't have any CLs, yet, regardless of what CL
			// snapshots are stored in Datastore.
			s0 := &State{PB: &prjpb.PState{LuciProject: ct.lProject}}
			pb0 := backupPB(s0)
			s1, sideEffect, err := h.UpdateConfig(ctx, s0)
			assert.NoErr(t, err)
			assert.That(t, s0.PB, should.Match(pb0)) // s0 must not change.
			assert.Loosely(t, sideEffect, should.HaveType[*UpdateIncompleteRunsConfig])
			assert.That(t, sideEffect.(*UpdateIncompleteRunsConfig), should.Match(&UpdateIncompleteRunsConfig{
				Hash:     meta.Hash(),
				EVersion: meta.EVersion,
				RunIDs:   nil,
			}))
			assert.That(t, s1.PB, should.Match(&prjpb.PState{
				LuciProject:         ct.lProject,
				Status:              prjpb.Status_STARTED,
				ConfigHash:          meta.Hash(),
				ConfigGroupNames:    []string{"g0", "g1"},
				Components:          nil,
				Pcls:                nil,
				RepartitionRequired: false,
			}))
			assert.That(t, s1.LogReasons, should.Match([]prjpb.LogReason{prjpb.LogReason_CONFIG_CHANGED, prjpb.LogReason_STATUS_CHANGED}))
		})

		// Add 3 CLs: 101 standalone and 202<-203 as a stack.
		triggerTS := timestamppb.New(ct.Clock.Now())
		ci101 := gf.CI(
			101, gf.PS(1), gf.Ref("refs/heads/main"), gf.Project("repo/a"),
			gf.CQ(+2, ct.Clock.Now(), gf.U("user-1")), gf.Updated(ct.Clock.Now()),
		)
		ci202 := gf.CI(
			202, gf.PS(3), gf.Ref("refs/heads/other"), gf.Project("repo/a"), gf.AllRevs(),
			gf.CQ(+1, ct.Clock.Now(), gf.U("user-2")), gf.Updated(ct.Clock.Now()),
		)
		ci203 := gf.CI(
			203, gf.PS(3), gf.Ref("refs/heads/other"), gf.Project("repo/a"), gf.AllRevs(),
			gf.CQ(+1, ct.Clock.Now(), gf.U("user-2")), gf.Updated(ct.Clock.Now()),
		)
		ct.GFake.CreateChange(&gf.Change{Host: ct.gHost, ACLs: gf.ACLPublic(), Info: ci101})
		ct.GFake.CreateChange(&gf.Change{Host: ct.gHost, ACLs: gf.ACLPublic(), Info: ci202})
		ct.GFake.CreateChange(&gf.Change{Host: ct.gHost, ACLs: gf.ACLPublic(), Info: ci203})
		ct.GFake.SetDependsOn(ct.gHost, "203_3" /* child */, "202_2" /*parent*/)
		cl101 := ct.runCLUpdater(ctx, 101)
		cl202 := ct.runCLUpdater(ctx, 202)
		cl203 := ct.runCLUpdater(ctx, 203)

		s1 := &State{
			PB: &prjpb.PState{
				LuciProject:      ct.lProject,
				Status:           prjpb.Status_STARTED,
				ConfigHash:       meta.Hash(),
				ConfigGroupNames: []string{"g0", "g1"},
				Pcls: []*prjpb.PCL{
					{
						Clid:               int64(cl101.ID),
						Eversion:           1,
						ConfigGroupIndexes: []int32{0}, // g0
						Status:             prjpb.PCL_OK,
						Triggers: &run.Triggers{CqVoteTrigger: &run.Trigger{
							Mode:            string(run.FullRun),
							Time:            triggerTS,
							Email:           gf.U("user-1").GetEmail(),
							GerritAccountId: gf.U("user-1").GetAccountId(),
						}},
					},
					{
						Clid:               int64(cl202.ID),
						Eversion:           1,
						ConfigGroupIndexes: []int32{1}, // g1
						Status:             prjpb.PCL_OK,
						Triggers: &run.Triggers{CqVoteTrigger: &run.Trigger{
							Mode:            string(run.DryRun),
							Time:            triggerTS,
							Email:           gf.U("user-2").GetEmail(),
							GerritAccountId: gf.U("user-2").GetAccountId(),
						}},
					},
					{
						Clid:               int64(cl203.ID),
						Eversion:           1,
						ConfigGroupIndexes: []int32{1}, // g1
						Status:             prjpb.PCL_OK,
						Triggers: &run.Triggers{CqVoteTrigger: &run.Trigger{
							Mode:            string(run.DryRun),
							Time:            triggerTS,
							Email:           gf.U("user-2").GetEmail(),
							GerritAccountId: gf.U("user-2").GetAccountId(),
						}},
						Deps: []*changelist.Dep{{Clid: int64(cl202.ID), Kind: changelist.DepKind_HARD}},
					},
				},
				Components: []*prjpb.Component{
					{
						Clids: []int64{int64(cl101.ID)},
						Pruns: []*prjpb.PRun{
							{
								Id:    ct.lProject + "/" + "1111-v1-beef",
								Clids: []int64{int64(cl101.ID)},
							},
						},
					},
					{
						Clids: []int64{404},
					},
				},
			},
		}
		pb1 := backupPB(s1)

		t.Run("noop update is quick", func(t *ftt.Test) {
			s2, sideEffect, err := h.UpdateConfig(ctx, s1)
			assert.NoErr(t, err)
			assert.Loosely(t, s2, should.Equal(s1)) // pointer comparison only.
			assert.Loosely(t, sideEffect, should.BeNil)
		})

		t.Run("existing project", func(t *ftt.Test) {
			t.Run("updated without touching components", func(t *ftt.Test) {
				meta2 := updateConfigToNoFallabck(ctx, &ct)
				s2, sideEffect, err := h.UpdateConfig(ctx, s1)
				assert.NoErr(t, err)
				assert.That(t, s1.PB, should.Match(pb1)) // s1 must not change.
				assert.Loosely(t, sideEffect, should.HaveType[*UpdateIncompleteRunsConfig])
				assert.That(t, sideEffect.(*UpdateIncompleteRunsConfig), should.Match(&UpdateIncompleteRunsConfig{
					Hash:     meta2.Hash(),
					EVersion: meta2.EVersion,
					RunIDs:   common.MakeRunIDs(ct.lProject + "/" + "1111-v1-beef"),
				}))
				assert.That(t, s2.PB, should.Match(&prjpb.PState{
					LuciProject:      ct.lProject,
					Status:           prjpb.Status_STARTED,
					ConfigHash:       meta2.Hash(), // changed
					ConfigGroupNames: []string{"g0", "g1"},
					Pcls: []*prjpb.PCL{
						{
							Clid:               int64(cl101.ID),
							Eversion:           1,
							ConfigGroupIndexes: []int32{0, 1}, // +g1, because g1 is no longer "fallback: YES"
							Status:             prjpb.PCL_OK,
							Triggers: &run.Triggers{CqVoteTrigger: &run.Trigger{
								Mode:            string(run.FullRun),
								Time:            triggerTS,
								Email:           gf.U("user-1").GetEmail(),
								GerritAccountId: gf.U("user-1").GetAccountId(),
							}},
						},
						pb1.Pcls[1], // #202 didn't change.
						pb1.Pcls[2], // #203 didn't change.
					},
					Components:          markForTriage(pb1.Components),
					RepartitionRequired: true,
				}))
				assert.That(t, s2.LogReasons, should.Match([]prjpb.LogReason{prjpb.LogReason_CONFIG_CHANGED}))
			})

			t.Run("If PCLs stay same, RepartitionRequired must be false", func(t *ftt.Test) {
				meta2 := updateConfigRenameG1toG11(ctx, &ct)
				s2, sideEffect, err := h.UpdateConfig(ctx, s1)
				assert.NoErr(t, err)
				assert.That(t, s1.PB, should.Match(pb1)) // s1 must not change.
				assert.Loosely(t, sideEffect, should.HaveType[*UpdateIncompleteRunsConfig])
				assert.That(t, sideEffect.(*UpdateIncompleteRunsConfig), should.Match(&UpdateIncompleteRunsConfig{
					Hash:     meta2.Hash(),
					EVersion: meta2.EVersion,
					RunIDs:   common.MakeRunIDs(ct.lProject + "/" + "1111-v1-beef"),
				}))
				assert.That(t, s2.PB, should.Match(&prjpb.PState{
					LuciProject:         ct.lProject,
					Status:              prjpb.Status_STARTED,
					ConfigHash:          meta2.Hash(),
					ConfigGroupNames:    []string{"g0", "g11"}, // g1 -> g11.
					Pcls:                pb1.GetPcls(),
					Components:          markForTriage(pb1.Components),
					RepartitionRequired: false,
				}))
			})
		})

		t.Run("disabled project updated with long ago deleted CL", func(t *ftt.Test) {
			s1.PB.Status = prjpb.Status_STOPPED
			for _, c := range s1.PB.GetComponents() {
				c.Pruns = nil // disabled projects don't have incomplete runs.
			}
			pb1 = backupPB(s1)
			changelist.Delete(ctx, cl101.ID)

			meta2 := updateConfigToNoFallabck(ctx, &ct)
			s2, sideEffect, err := h.UpdateConfig(ctx, s1)
			assert.NoErr(t, err)
			assert.That(t, s1.PB, should.Match(pb1)) // s1 must not change.
			assert.Loosely(t, sideEffect, should.HaveType[*UpdateIncompleteRunsConfig])
			assert.That(t, sideEffect.(*UpdateIncompleteRunsConfig), should.Match(&UpdateIncompleteRunsConfig{
				Hash:     meta2.Hash(),
				EVersion: meta2.EVersion,
				// No runs to notify.
			}))
			assert.That(t, s2.PB, should.Match(&prjpb.PState{
				LuciProject:      ct.lProject,
				Status:           prjpb.Status_STARTED,
				ConfigHash:       meta2.Hash(), // changed
				ConfigGroupNames: []string{"g0", "g1"},
				Pcls: []*prjpb.PCL{
					{
						Clid:     int64(cl101.ID),
						Eversion: 1,
						Status:   prjpb.PCL_DELETED,
					},
					pb1.Pcls[1], // #202 didn't change.
					pb1.Pcls[2], // #203 didn't change.
				},
				Components:          markForTriage(pb1.Components),
				RepartitionRequired: true,
			}))
			assert.That(t, s2.LogReasons, should.Match([]prjpb.LogReason{prjpb.LogReason_CONFIG_CHANGED, prjpb.LogReason_STATUS_CHANGED}))
		})

		t.Run("disabled project waits for incomplete Runs", func(t *ftt.Test) {
			prjcfgtest.Disable(ctx, ct.lProject)
			s2, sideEffect, err := h.UpdateConfig(ctx, s1)
			assert.NoErr(t, err)
			pb := backupPB(s1)
			pb.Status = prjpb.Status_STOPPING
			assert.That(t, s2.PB, should.Match(pb))
			assert.Loosely(t, sideEffect, should.HaveType[*CancelIncompleteRuns])
			assert.That(t, sideEffect.(*CancelIncompleteRuns), should.Match(&CancelIncompleteRuns{
				RunIDs: common.MakeRunIDs(ct.lProject + "/" + "1111-v1-beef"),
			}))
			assert.That(t, s2.LogReasons, should.Match([]prjpb.LogReason{prjpb.LogReason_STATUS_CHANGED}))
		})

		t.Run("disabled project stops iff there are no incomplete Runs", func(t *ftt.Test) {
			for _, c := range s1.PB.GetComponents() {
				c.Pruns = nil
			}
			prjcfgtest.Disable(ctx, ct.lProject)
			s2, sideEffect, err := h.UpdateConfig(ctx, s1)
			assert.NoErr(t, err)
			assert.Loosely(t, sideEffect, should.BeNil)
			pb := backupPB(s1)
			pb.Status = prjpb.Status_STOPPED
			assert.That(t, s2.PB, should.Match(pb))
			assert.That(t, prjpb.SortAndDedupeLogReasons(s2.LogReasons), should.Match([]prjpb.LogReason{prjpb.LogReason_STATUS_CHANGED}))
		})

		// The rest of the test coverage of UpdateConfig is achieved by testing code
		// of makePCL.

		t.Run("makePCL with full snapshot works", func(t *ftt.Test) {
			var err error
			s1.configGroups, err = meta.GetConfigGroups(ctx)
			assert.NoErr(t, err)
			s1.cfgMatcher = cfgmatcher.LoadMatcherFromConfigGroups(ctx, s1.configGroups, &meta)

			t.Run("Status == OK", func(t *ftt.Test) {
				expected := &prjpb.PCL{
					Clid:               int64(cl101.ID),
					Eversion:           cl101.EVersion,
					ConfigGroupIndexes: []int32{0}, // g0
					Triggers: &run.Triggers{CqVoteTrigger: &run.Trigger{
						Mode:            string(run.FullRun),
						Time:            triggerTS,
						Email:           gf.U("user-1").GetEmail(),
						GerritAccountId: gf.U("user-1").GetAccountId(),
					}},
				}
				t.Run("CL snapshotted with current config", func(t *ftt.Test) {
					assert.That(t, s1.makePCL(ctx, cl101), should.Match(expected))
				})
				t.Run("CL snapshotted with an older config", func(t *ftt.Test) {
					cl101.ApplicableConfig.GetProjects()[0].ConfigGroupIds = []string{"oldhash/g0"}
					assert.That(t, s1.makePCL(ctx, cl101), should.Match(expected))
				})
				t.Run("not triggered CL", func(t *ftt.Test) {
					delete(cl101.Snapshot.GetGerrit().GetInfo().GetLabels(), trigger.CQLabelName)
					expected.Triggers = nil
					assert.That(t, s1.makePCL(ctx, cl101), should.Match(expected))
				})
				t.Run("abandoned CL is not triggered even if it has CQ vote", func(t *ftt.Test) {
					cl101.Snapshot.GetGerrit().GetInfo().Status = gerritpb.ChangeStatus_ABANDONED
					expected.Triggers = nil
					assert.That(t, s1.makePCL(ctx, cl101), should.Match(expected))
				})
				t.Run("Submitted CL is also not triggered even if it has CQ vote", func(t *ftt.Test) {
					cl101.Snapshot.GetGerrit().GetInfo().Status = gerritpb.ChangeStatus_MERGED
					expected.Triggers = nil
					expected.Submitted = true
					assert.That(t, s1.makePCL(ctx, cl101), should.Match(expected))
				})
				t.Run("Submittable if the snapshot is", func(t *ftt.Test) {
					cl101.Snapshot.GetGerrit().GetInfo().Submittable = true
					expected.Submittable = true
					assert.That(t, s1.makePCL(ctx, cl101), should.Match(expected))
				})
			})

			t.Run("outdated snapshot requires waiting", func(t *ftt.Test) {
				cl101.Snapshot.Outdated = &changelist.Snapshot_Outdated{}
				assert.That(t, s1.makePCL(ctx, cl101), should.Match(&prjpb.PCL{
					Clid:     int64(cl101.ID),
					Eversion: cl101.EVersion,
					Status:   prjpb.PCL_UNKNOWN,
					Outdated: &changelist.Snapshot_Outdated{},
				}))
			})

			t.Run("snapshot from diff project requires waiting", func(t *ftt.Test) {
				cl101.Snapshot.LuciProject = "another"
				assert.That(t, s1.makePCL(ctx, cl101), should.Match(&prjpb.PCL{
					Clid:     int64(cl101.ID),
					Eversion: cl101.EVersion,
					Status:   prjpb.PCL_UNKNOWN,
				}))
			})

			t.Run("CL from diff project is unwatched", func(t *ftt.Test) {
				s1.PB.LuciProject = "another"
				assert.That(t, s1.makePCL(ctx, cl101), should.Match(&prjpb.PCL{
					Clid:     int64(cl101.ID),
					Eversion: cl101.EVersion,
					Status:   prjpb.PCL_UNWATCHED,
				}))
			})

			t.Run("CL watched by several projects is unwatched but with an error", func(t *ftt.Test) {
				cl101.ApplicableConfig.Projects = append(
					cl101.ApplicableConfig.GetProjects(),
					&changelist.ApplicableConfig_Project{
						ConfigGroupIds: []string{"g"},
						Name:           "another",
					})
				assert.That(t, s1.makePCL(ctx, cl101), should.Match(&prjpb.PCL{
					Clid:               int64(cl101.ID),
					Eversion:           cl101.EVersion,
					Status:             prjpb.PCL_OK,
					ConfigGroupIndexes: []int32{0}, // g0
					Triggers: &run.Triggers{CqVoteTrigger: &run.Trigger{
						Mode:            string(run.FullRun),
						Time:            triggerTS,
						Email:           gf.U("user-1").GetEmail(),
						GerritAccountId: gf.U("user-1").GetAccountId(),
					}},
					PurgeReasons: []*prjpb.PurgeReason{{
						ClError: &changelist.CLError{
							Kind: &changelist.CLError_WatchedByManyProjects_{
								WatchedByManyProjects: &changelist.CLError_WatchedByManyProjects{
									Projects: []string{s1.PB.GetLuciProject(), "another"},
								},
							},
						},
						ApplyTo: &prjpb.PurgeReason_AllActiveTriggers{AllActiveTriggers: true},
					}},
					Errors: []*changelist.CLError{{Kind: &changelist.CLError_WatchedByManyProjects_{
						WatchedByManyProjects: &changelist.CLError_WatchedByManyProjects{
							Projects: []string{s1.PB.GetLuciProject(), "another"},
						},
					}}},
				}))
			})

			t.Run("CL is no longer watched", func(t *ftt.Test) {
				meta2 := updateConfigToNoFallabck(ctx, &ct)
				s2, sideEffect, err := h.UpdateConfig(ctx, s1)
				assert.NoErr(t, err)
				assert.That(t, s1.PB, should.Match(pb1)) // s1 must not change.
				assert.Loosely(t, sideEffect, should.HaveType[*UpdateIncompleteRunsConfig])
				assert.That(t, sideEffect.(*UpdateIncompleteRunsConfig), should.Match(&UpdateIncompleteRunsConfig{
					Hash:     meta2.Hash(),
					RunIDs:   common.RunIDs{"test/1111-v1-beef"},
					EVersion: meta2.EVersion,
				}))
				cl101.ApplicableConfig.Projects[0].ConfigGroupIds = []string{"sha:1231/R119"}
				s2.cfgMatcher = cfgmatcher.LoadMatcherFromConfigGroups(ctx, nil /* configGroups */, &meta2)
				assert.That(t, s2.makePCL(ctx, cl101), should.Match(&prjpb.PCL{
					Clid:               int64(cl101.ID),
					Eversion:           cl101.EVersion,
					Status:             prjpb.PCL_UNWATCHED,
					ConfigGroupIndexes: []int32{},
				}))
			})

			t.Run("CL with Commit: false footer has an error", func(t *ftt.Test) {
				cl101.Snapshot.Metadata = []*changelist.StringPair{{Key: "Commit", Value: "false"}}
				assert.That(t, s1.makePCL(ctx, cl101).GetPurgeReasons(), should.Match([]*prjpb.PurgeReason{
					{
						ClError: &changelist.CLError{
							Kind: &changelist.CLError_CommitBlocked{CommitBlocked: true},
						},
						ApplyTo: &prjpb.PurgeReason_Triggers{Triggers: &run.Triggers{
							CqVoteTrigger: &run.Trigger{
								Mode:            string(run.FullRun),
								Time:            triggerTS,
								Email:           gf.U("user-1").GetEmail(),
								GerritAccountId: gf.U("user-1").GetAccountId(),
							},
						}},
					},
				}))
			})

			t.Run("'Commit: false' footer works with different capitalization", func(t *ftt.Test) {
				cl101.Snapshot.Metadata = []*changelist.StringPair{{Key: "COMMIT", Value: "FALSE"}}
				assert.That(t, s1.makePCL(ctx, cl101).GetPurgeReasons(), should.Match([]*prjpb.PurgeReason{{
					ClError: &changelist.CLError{
						Kind: &changelist.CLError_CommitBlocked{CommitBlocked: true},
					},
					ApplyTo: &prjpb.PurgeReason_Triggers{Triggers: &run.Triggers{
						CqVoteTrigger: &run.Trigger{
							Mode:            string(run.FullRun),
							Time:            triggerTS,
							Email:           gf.U("user-1").GetEmail(),
							GerritAccountId: gf.U("user-1").GetAccountId(),
						},
					}},
				}}))
			})

			t.Run("'Commit: false' has no effect for dry run CL", func(t *ftt.Test) {
				// cl202 is set up for dry run, unlike cl101.
				cl202.Snapshot.Metadata = []*changelist.StringPair{{Key: "Commit", Value: "false"}}
				assert.Loosely(t, s1.makePCL(ctx, cl202).GetPurgeReasons(), should.BeEmpty)
			})
		})
	})
}

func TestOnCLsUpdated(t *testing.T) {
	t.Parallel()

	ftt.Run("OnCLsUpdated works", t, func(t *ftt.Test) {
		ct := ctest{
			lProject: "test",
			gHost:    "c-review.example.com",
		}
		ctx := ct.SetUp(t)

		cfg1 := &cfgpb.Config{}
		assert.NoErr(t, prototext.Unmarshal([]byte(cfgText1), cfg1))

		prjcfgtest.Create(ctx, ct.lProject, cfg1)
		meta := prjcfgtest.MustExist(ctx, ct.lProject)
		gobmaptest.Update(ctx, ct.lProject)

		// Add 3 CLs: 101 standalone and 202<-203 as a stack.
		triggerTS := timestamppb.New(ct.Clock.Now())
		ci101 := gf.CI(
			101, gf.PS(1), gf.Ref("refs/heads/main"), gf.Project("repo/a"),
			gf.CQ(+2, ct.Clock.Now(), gf.U("user-1")), gf.Updated(ct.Clock.Now()),
		)
		ci202 := gf.CI(
			202, gf.PS(3), gf.Ref("refs/heads/other"), gf.Project("repo/a"), gf.AllRevs(),
			gf.CQ(+1, ct.Clock.Now(), gf.U("user-2")), gf.Updated(ct.Clock.Now()),
		)
		ci203 := gf.CI(
			203, gf.PS(3), gf.Ref("refs/heads/other"), gf.Project("repo/a"), gf.AllRevs(),
			gf.CQ(+1, ct.Clock.Now(), gf.U("user-2")), gf.Updated(ct.Clock.Now()),
		)
		ct.GFake.CreateChange(&gf.Change{Host: ct.gHost, ACLs: gf.ACLPublic(), Info: ci101})
		ct.GFake.CreateChange(&gf.Change{Host: ct.gHost, ACLs: gf.ACLPublic(), Info: ci202})
		ct.GFake.CreateChange(&gf.Change{Host: ct.gHost, ACLs: gf.ACLPublic(), Info: ci203})
		ct.GFake.SetDependsOn(ct.gHost, "203_3" /* child */, "202_2" /*parent*/)
		cl101 := ct.runCLUpdater(ctx, 101)
		cl202 := ct.runCLUpdater(ctx, 202)
		cl203 := ct.runCLUpdater(ctx, 203)

		h := Handler{}
		s0 := &State{PB: &prjpb.PState{
			LuciProject:      ct.lProject,
			Status:           prjpb.Status_STARTED,
			ConfigHash:       meta.Hash(),
			ConfigGroupNames: []string{"g0", "g1"},
		}}
		pb0 := backupPB(s0)

		// NOTE: conversion of individual CL to PCL is in TestUpdateConfig.

		t.Run("One simple CL", func(t *ftt.Test) {
			s1, sideEffect, err := h.OnCLsUpdated(ctx, s0, map[int64]int64{
				int64(cl101.ID): cl101.EVersion,
			})
			assert.NoErr(t, err)
			assert.That(t, s0.PB, should.Match(pb0))
			assert.Loosely(t, sideEffect, should.BeNil)
			assert.That(t, s1.PB.Pcls, should.Match([]*prjpb.PCL{
				{
					Clid:               int64(cl101.ID),
					Eversion:           1,
					ConfigGroupIndexes: []int32{0}, // g0
					Status:             prjpb.PCL_OK,
					Triggers: &run.Triggers{CqVoteTrigger: &run.Trigger{
						Mode:            string(run.FullRun),
						Time:            triggerTS,
						Email:           gf.U("user-1").GetEmail(),
						GerritAccountId: gf.U("user-1").GetAccountId(),
					}},
				},
			}))
			assert.Loosely(t, s1.PB.RepartitionRequired, should.BeTrue)

			t.Run("Noop based on EVersion", func(t *ftt.Test) {
				s2, sideEffect, err := h.OnCLsUpdated(ctx, s1, map[int64]int64{
					int64(cl101.ID): 1, // already known
				})
				assert.NoErr(t, err)
				assert.Loosely(t, sideEffect, should.BeNil)
				assert.That(t, s1.PB.GetPcls(), should.Match(s2.PB.GetPcls())) // pointer comparison only.
			})

			t.Run("Marks affected components for triage", func(t *ftt.Test) {
				cl101.EVersion++
				assert.NoErr(t, datastore.Put(ctx, cl101))
				// Add 2 components, one of which references cl101.
				s1.PB.Components = []*prjpb.Component{
					{Clids: []int64{int64(cl101.ID)}},
					{Clids: []int64{int64(cl101.ID + 111111)}},
				}
				pb := backupPB(s1)
				s2, sideEffect, err := h.OnCLsUpdated(ctx, s1, map[int64]int64{
					int64(cl101.ID): cl101.EVersion,
				})
				assert.That(t, s1.PB, should.Match(pb))
				assert.NoErr(t, err)
				assert.Loosely(t, sideEffect, should.BeNil)
				// The only expected changes are:
				pb.Components[0].TriageRequired = true
				pb.Pcls[0].Eversion = cl101.EVersion
				assert.That(t, s2.PB, should.Match(pb))
			})
		})

		t.Run("One CL with a yet unknown dep", func(t *ftt.Test) {
			s1, sideEffect, err := h.OnCLsUpdated(ctx, s0, map[int64]int64{
				int64(cl203.ID): 1,
			})
			assert.NoErr(t, err)
			assert.That(t, s0.PB, should.Match(pb0))
			assert.Loosely(t, sideEffect, should.BeNil)
			assert.That(t, s1.PB, should.Match(&prjpb.PState{
				LuciProject:      ct.lProject,
				Status:           prjpb.Status_STARTED,
				ConfigHash:       meta.Hash(),
				ConfigGroupNames: []string{"g0", "g1"},
				Pcls: []*prjpb.PCL{
					{
						Clid:               int64(cl203.ID),
						Eversion:           1,
						ConfigGroupIndexes: []int32{1}, // g1
						Status:             prjpb.PCL_OK,
						Triggers: &run.Triggers{CqVoteTrigger: &run.Trigger{
							Mode:            string(run.DryRun),
							Time:            triggerTS,
							Email:           gf.U("user-2").GetEmail(),
							GerritAccountId: gf.U("user-2").GetAccountId(),
						}},
						Deps: []*changelist.Dep{{Clid: int64(cl202.ID), Kind: changelist.DepKind_HARD}},
					},
				},
				RepartitionRequired: true,
			}))
			t.Run("unknown dep becomes known and marks a component for triage", func(t *ftt.Test) {
				// Add a component which has only 203.
				s1.PB.Components = []*prjpb.Component{
					{Clids: []int64{int64(cl203.ID)}},
				}
				pb := backupPB(s1)
				s2, sideEffect, err := h.OnCLsUpdated(ctx, s1, map[int64]int64{
					int64(cl202.ID): cl202.EVersion,
				})
				assert.That(t, s1.PB, should.Match(pb))
				assert.NoErr(t, err)
				assert.Loosely(t, sideEffect, should.BeNil)
				assert.Loosely(t, s2.PB.Components[0].TriageRequired, should.BeTrue)
			})
		})

		t.Run("PCLs must remain sorted", func(t *ftt.Test) {
			pcl101 := &prjpb.PCL{
				Clid:               int64(cl101.ID),
				Eversion:           1,
				ConfigGroupIndexes: []int32{0}, // g0
				Status:             prjpb.PCL_OK,
				Triggers: &run.Triggers{CqVoteTrigger: &run.Trigger{
					Mode: string(run.FullRun),
					Time: triggerTS,
				}},
			}
			s1 := &State{PB: &prjpb.PState{
				LuciProject:      ct.lProject,
				Status:           prjpb.Status_STARTED,
				ConfigHash:       meta.Hash(),
				ConfigGroupNames: []string{"g0", "g1"},
				Pcls: sortPCLs([]*prjpb.PCL{
					pcl101,
					{
						Clid:               int64(cl203.ID),
						Eversion:           1,
						ConfigGroupIndexes: []int32{1}, // g1
						Status:             prjpb.PCL_OK,
						Triggers: &run.Triggers{CqVoteTrigger: &run.Trigger{
							Mode: string(run.DryRun),
							Time: triggerTS,
						}},
						Deps: []*changelist.Dep{{Clid: int64(cl202.ID), Kind: changelist.DepKind_HARD}},
					},
				}),
			}}
			pb1 := backupPB(s1)
			bumpEVersion(t, ctx, cl203, 3)
			s2, sideEffect, err := h.OnCLsUpdated(ctx, s1, map[int64]int64{
				404:             404,            // doesn't even exist
				int64(cl202.ID): cl202.EVersion, // new
				int64(cl101.ID): cl101.EVersion, // unchanged
				int64(cl203.ID): 3,              // updated
			})
			assert.NoErr(t, err)
			assert.That(t, s1.PB, should.Match(pb1))
			assert.Loosely(t, sideEffect, should.BeNil)
			assert.That(t, s2.PB, should.Match(&prjpb.PState{
				LuciProject:      ct.lProject,
				Status:           prjpb.Status_STARTED,
				ConfigHash:       meta.Hash(),
				ConfigGroupNames: []string{"g0", "g1"},
				Pcls: sortPCLs([]*prjpb.PCL{
					{
						Clid:     404,
						Eversion: 0,
						Status:   prjpb.PCL_DELETED,
					},
					pcl101, // 101 is unchanged
					{ // new
						Clid:               int64(cl202.ID),
						Eversion:           1,
						ConfigGroupIndexes: []int32{1}, // g1
						Status:             prjpb.PCL_OK,
						Triggers: &run.Triggers{CqVoteTrigger: &run.Trigger{
							Mode:            string(run.DryRun),
							Time:            triggerTS,
							Email:           gf.U("user-2").GetEmail(),
							GerritAccountId: gf.U("user-2").GetAccountId(),
						}},
					},
					{ // updated
						Clid:               int64(cl203.ID),
						Eversion:           3,
						ConfigGroupIndexes: []int32{1}, // g1
						Status:             prjpb.PCL_OK,
						Triggers: &run.Triggers{CqVoteTrigger: &run.Trigger{
							Mode:            string(run.DryRun),
							Time:            triggerTS,
							Email:           gf.U("user-2").GetEmail(),
							GerritAccountId: gf.U("user-2").GetAccountId(),
						}},
						Deps: []*changelist.Dep{{Clid: int64(cl202.ID), Kind: changelist.DepKind_HARD}},
					},
				}),
				RepartitionRequired: true,
			}))
		})

		t.Run("Invalid dep of some other CL must be marked as unwatched", func(t *ftt.Test) {
			// For example, if user made a typo in `CQ-Depend`, e.g.:
			//    `CQ-Depend: chromiAm:123`
			// then CL Updater will create an entity for such CL anyway,
			// but eventually fill it with DependentMeta stating that this LUCI
			// project has no access to it.
			// Note that such typos may be malicious, so PM must treat such CLs as not
			// found regardless of whether they actually exist in Gerrit.
			cl404 := ct.runCLUpdater(ctx, 404)
			assert.Loosely(t, cl404.Snapshot, should.BeNil)
			assert.Loosely(t, cl404.ApplicableConfig, should.BeNil)
			assert.Loosely(t, cl404.Access.GetByProject(), should.ContainKey(ct.lProject))
			s1, sideEffect, err := h.OnCLsUpdated(ctx, s0, map[int64]int64{
				int64(cl404.ID): 1,
			})
			assert.NoErr(t, err)
			assert.That(t, s0.PB, should.Match(pb0))
			assert.Loosely(t, sideEffect, should.BeNil)
			pb1 := proto.Clone(pb0).(*prjpb.PState)
			pb1.Pcls = append(pb0.Pcls, &prjpb.PCL{
				Clid:               int64(cl404.ID),
				Eversion:           1,
				ConfigGroupIndexes: []int32{},
				Status:             prjpb.PCL_UNWATCHED,
			})
			pb1.RepartitionRequired = true
			assert.That(t, s1.PB, should.Match(pb1))
		})

		t.Run("non-STARTED project ignores all CL events", func(t *ftt.Test) {
			s0.PB.Status = prjpb.Status_STOPPING
			s1, sideEffect, err := h.OnCLsUpdated(ctx, s0, map[int64]int64{
				int64(cl101.ID): cl101.EVersion,
			})
			assert.NoErr(t, err)
			assert.Loosely(t, sideEffect, should.BeNil)
			assert.Loosely(t, s0, should.Equal(s1)) // pointer comparison only.
		})
	})
}

func TestRunsCreatedAndFinished(t *testing.T) {
	t.Parallel()

	ftt.Run("OnRunsCreated and OnRunsFinished works", t, func(t *ftt.Test) {
		ct := ctest{
			lProject: "test",
			gHost:    "c-review.example.com",
		}
		ctx := ct.SetUp(t)

		cfg1 := &cfgpb.Config{}
		assert.NoErr(t, prototext.Unmarshal([]byte(cfgText1), cfg1))
		prjcfgtest.Create(ctx, ct.lProject, cfg1)
		meta := prjcfgtest.MustExist(ctx, ct.lProject)

		run1 := &run.Run{ID: common.RunID(ct.lProject + "/101-new"), CLs: common.CLIDs{101}}
		run789 := &run.Run{ID: common.RunID(ct.lProject + "/789-efg"), CLs: common.CLIDs{709, 707, 708}}
		run1finished := &run.Run{ID: common.RunID(ct.lProject + "/101-done"), CLs: common.CLIDs{101}, Status: run.Status_FAILED}
		assert.NoErr(t, datastore.Put(ctx, run1finished, run1, run789))
		assert.Loosely(t, run.IsEnded(run1finished.Status), should.BeTrue)

		h := Handler{}
		s1 := &State{PB: &prjpb.PState{
			LuciProject:      ct.lProject,
			Status:           prjpb.Status_STARTED,
			ConfigHash:       meta.Hash(),
			ConfigGroupNames: []string{"g0", "g1"},
			// For OnRunsFinished / OnRunsCreated PCLs don't matter, so omit them from
			// the test for brevity, even though valid State must have PCLs covering
			// all components.
			Pcls: nil,
			Components: []*prjpb.Component{
				{
					Clids: []int64{101},
					Pruns: []*prjpb.PRun{{Id: ct.lProject + "/101-aaa", Clids: []int64{101}}},
				},
				{
					Clids: []int64{202, 203, 204},
				},
			},
			CreatedPruns: []*prjpb.PRun{
				{Id: ct.lProject + "/789-efg", Clids: []int64{707, 708, 709}},
			},
		}}
		var err error
		s1.configGroups, err = meta.GetConfigGroups(ctx)
		assert.NoErr(t, err)
		pb1 := backupPB(s1)

		t.Run("Noops", func(t *ftt.Test) {
			finished := make(map[common.RunID]run.Status)
			t.Run("OnRunsFinished on not tracked Run", func(t *ftt.Test) {
				finished[run1finished.ID] = run.Status_SUCCEEDED
				s2, sideEffect, err := h.OnRunsFinished(ctx, s1, finished)
				assert.NoErr(t, err)
				assert.Loosely(t, sideEffect, should.BeNil)
				// although s2 is cloned, it must be exact same as s1.
				assert.That(t, s2.PB, should.Match(pb1))
			})
			t.Run("OnRunsCreated on already finished run", func(t *ftt.Test) {
				s2, sideEffect, err := h.OnRunsCreated(ctx, s1, common.RunIDs{run1finished.ID})
				assert.NoErr(t, err)
				assert.Loosely(t, sideEffect, should.BeNil)
				// although s2 is cloned, it must be exact same as s1.
				assert.That(t, s2.PB, should.Match(pb1))
			})
			t.Run("OnRunsCreated on already tracked Run", func(t *ftt.Test) {
				s2, sideEffect, err := h.OnRunsCreated(ctx, s1, common.MakeRunIDs(ct.lProject+"/101-aaa"))
				assert.NoErr(t, err)
				assert.Loosely(t, sideEffect, should.BeNil)
				assert.Loosely(t, s2, should.Equal(s1))
				assert.That(t, pb1, should.Match(s1.PB))
			})
			t.Run("OnRunsCreated on somehow already deleted run", func(t *ftt.Test) {
				s2, sideEffect, err := h.OnRunsCreated(ctx, s1, common.MakeRunIDs(ct.lProject+"/404-nnn"))
				assert.NoErr(t, err)
				assert.Loosely(t, sideEffect, should.BeNil)
				// although s2 is cloned, it must be exact same as s1.
				assert.That(t, s2.PB, should.Match(pb1))
			})
		})

		t.Run("OnRunsCreated", func(t *ftt.Test) {
			t.Run("when PM is started", func(t *ftt.Test) {
				runX := &run.Run{ // Run involving all of CLs and more.
					ID: common.RunID(ct.lProject + "/000-xxx"),
					// The order doesn't have to and is intentionally not sorted here.
					CLs: common.CLIDs{404, 101, 202, 204, 203},
				}
				run2 := &run.Run{ID: common.RunID(ct.lProject + "/202-bbb"), CLs: common.CLIDs{202}}
				run3 := &run.Run{ID: common.RunID(ct.lProject + "/203-ccc"), CLs: common.CLIDs{203}}
				run23 := &run.Run{ID: common.RunID(ct.lProject + "/232-bcb"), CLs: common.CLIDs{203, 202}}
				run234 := &run.Run{ID: common.RunID(ct.lProject + "/234-bcd"), CLs: common.CLIDs{203, 204, 202}}
				assert.NoErr(t, datastore.Put(ctx, run2, run3, run23, run234, runX))

				s2, sideEffect, err := h.OnRunsCreated(ctx, s1, common.RunIDs{
					run2.ID, run3.ID, run23.ID, run234.ID, runX.ID,
					// non-existing Run shouldn't derail others.
					common.RunID(ct.lProject + "/404-nnn"),
				})
				assert.NoErr(t, err)
				assert.That(t, pb1, should.Match(s1.PB))
				assert.Loosely(t, sideEffect, should.BeNil)
				assert.That(t, s2.PB, should.Match(&prjpb.PState{
					LuciProject:      ct.lProject,
					Status:           prjpb.Status_STARTED,
					ConfigHash:       meta.Hash(),
					ConfigGroupNames: []string{"g0", "g1"},
					Components: []*prjpb.Component{
						s1.PB.GetComponents()[0], // 101 is unchanged
						{
							Clids: []int64{202, 203, 204},
							Pruns: []*prjpb.PRun{
								// Runs & CLs must be sorted by their respective IDs.
								{Id: string(run2.ID), Clids: []int64{202}},
								{Id: string(run3.ID), Clids: []int64{203}},
								{Id: string(run23.ID), Clids: []int64{202, 203}},
								{Id: string(run234.ID), Clids: []int64{202, 203, 204}},
							},
							TriageRequired: true,
						},
					},
					RepartitionRequired: true,
					CreatedPruns: []*prjpb.PRun{
						{Id: string(runX.ID), Clids: []int64{101, 202, 203, 204, 404}},
						{Id: ct.lProject + "/789-efg", Clids: []int64{707, 708, 709}}, // unchanged
					},
				}))
			})
			t.Run("when PM is stopping", func(t *ftt.Test) {
				s1.PB.Status = prjpb.Status_STOPPING
				pb1 := backupPB(s1)
				t.Run("cancels incomplete Runs", func(t *ftt.Test) {
					s2, sideEffect, err := h.OnRunsCreated(ctx, s1, common.RunIDs{run1.ID, run1finished.ID})
					assert.NoErr(t, err)
					assert.That(t, pb1, should.Match(s1.PB))
					assert.Loosely(t, sideEffect, should.HaveType[*CancelIncompleteRuns])
					assert.That(t, sideEffect.(*CancelIncompleteRuns), should.Match(&CancelIncompleteRuns{
						RunIDs: common.RunIDs{run1.ID},
					}))
					assert.Loosely(t, s2, should.Equal(s1))
				})
			})
		})

		t.Run("OnRunsFinished", func(t *ftt.Test) {
			s1.PB.Status = prjpb.Status_STOPPING
			pb1 := backupPB(s1)
			finished := make(map[common.RunID]run.Status)

			t.Run("deletes from Components", func(t *ftt.Test) {
				pb1 := backupPB(s1)
				runIDs := common.MakeRunIDs(ct.lProject + "/101-aaa")
				finished[runIDs[0]] = run.Status_CANCELLED
				s2, sideEffect, err := h.OnRunsFinished(ctx, s1, finished)
				assert.NoErr(t, err)
				assert.That(t, pb1, should.Match(s1.PB))
				assert.Loosely(t, sideEffect, should.BeNil)
				assert.That(t, s2.PB, should.Match(&prjpb.PState{
					LuciProject:      ct.lProject,
					Status:           prjpb.Status_STOPPING,
					ConfigHash:       meta.Hash(),
					ConfigGroupNames: []string{"g0", "g1"},
					Components: []*prjpb.Component{
						{
							Clids:          []int64{101},
							Pruns:          nil, // removed
							TriageRequired: true,
						},
						s1.PB.GetComponents()[1], // unchanged
					},
					CreatedPruns:        s1.PB.GetCreatedPruns(), // unchanged
					RepartitionRequired: true,
				}))
			})

			t.Run("deletes from CreatedPruns", func(t *ftt.Test) {
				runIDs := common.MakeRunIDs(ct.lProject + "/789-efg")
				finished[runIDs[0]] = run.Status_CANCELLED
				s2, sideEffect, err := h.OnRunsFinished(ctx, s1, finished)
				assert.NoErr(t, err)
				assert.That(t, pb1, should.Match(s1.PB))
				assert.Loosely(t, sideEffect, should.BeNil)
				assert.That(t, s2.PB, should.Match(&prjpb.PState{
					LuciProject:      ct.lProject,
					Status:           prjpb.Status_STOPPING,
					ConfigHash:       meta.Hash(),
					ConfigGroupNames: []string{"g0", "g1"},
					Components:       s1.PB.Components, // unchanged
					CreatedPruns:     nil,              // removed
				}))
			})

			t.Run("stops PM iff all runs finished", func(t *ftt.Test) {
				runIDs := common.MakeRunIDs(
					ct.lProject+"/101-aaa",
					ct.lProject+"/789-efg",
				)
				finished[runIDs[0]] = run.Status_SUCCEEDED
				finished[runIDs[1]] = run.Status_SUCCEEDED
				s2, sideEffect, err := h.OnRunsFinished(ctx, s1, finished)
				assert.NoErr(t, err)
				assert.That(t, pb1, should.Match(s1.PB))
				assert.Loosely(t, sideEffect, should.BeNil)
				assert.That(t, s2.PB, should.Match(&prjpb.PState{
					LuciProject:      ct.lProject,
					Status:           prjpb.Status_STOPPED,
					ConfigHash:       meta.Hash(),
					ConfigGroupNames: []string{"g0", "g1"},
					Pcls:             s1.PB.GetPcls(),
					Components: []*prjpb.Component{
						{Clids: []int64{101}, TriageRequired: true},
						s1.PB.GetComponents()[1], // unchanged.
					},
					CreatedPruns:        nil, // removed
					RepartitionRequired: true,
				}))
				assert.That(t, s2.LogReasons, should.Match([]prjpb.LogReason{prjpb.LogReason_STATUS_CHANGED}))
			})

			t.Run("purges triggers of the child CLs", func(t *ftt.Test) {
				// Emulate an MCE run.
				now := testclock.TestRecentTimeUTC
				mceRun := &prjpb.PRun{
					Id:    "202-deef",
					Mode:  string(run.FullRun),
					Clids: []int64{202},
				}
				s1.PB.Components = []*prjpb.Component{
					{
						Clids: []int64{202, 203, 204},
						Pruns: []*prjpb.PRun{mceRun},
					},
				}
				s1.PB.Pcls = []*prjpb.PCL{
					{
						Clid:               int64(202),
						Eversion:           1,
						Status:             prjpb.PCL_OK,
						ConfigGroupIndexes: []int32{0},
					},
					{
						Clid:     int64(203),
						Eversion: 1,
						Status:   prjpb.PCL_OK,
						Deps: []*changelist.Dep{
							{Clid: 202, Kind: changelist.DepKind_HARD},
						},
						ConfigGroupIndexes: []int32{0},
						Triggers: &run.Triggers{CqVoteTrigger: &run.Trigger{
							Mode: string(run.FullRun),
							Time: timestamppb.New(now.Add(-10 * time.Minute)),
						}},
					},
					{
						Clid:     int64(204),
						Eversion: 1,
						Status:   prjpb.PCL_OK,
						Deps: []*changelist.Dep{
							{Clid: 202, Kind: changelist.DepKind_HARD},
							{Clid: 203, Kind: changelist.DepKind_HARD},
						},
						ConfigGroupIndexes: []int32{0},
						Triggers: &run.Triggers{CqVoteTrigger: &run.Trigger{
							Mode: string(run.FullRun),
							Time: timestamppb.New(now.Add(-10 * time.Minute)),
						}},
					},
				}
				checkPurgeTask := func(task *prjpb.PurgeCLTask, clToPurge, depRunCL int64) {
					assert.Loosely(t, task.PurgingCl.Clid, should.Equal(clToPurge))
					assert.That(t, task.PurgeReasons, should.Match([]*prjpb.PurgeReason{
						{
							ClError: &changelist.CLError{
								Kind: &changelist.CLError_DepRunFailed{
									DepRunFailed: depRunCL,
								},
							},
							ApplyTo: &prjpb.PurgeReason_Triggers{
								Triggers: &run.Triggers{
									CqVoteTrigger: &run.Trigger{
										Mode: string(run.FullRun),
										Time: s1.PB.GetPCL(clToPurge).GetTriggers().GetCqVoteTrigger().GetTime(),
									},
								},
							},
						},
					}))
				}

				t.Run("if they have CQ votes", func(t *ftt.Test) {
					finished[common.RunID(mceRun.Id)] = run.Status_FAILED
					_, sideEffect, err := h.OnRunsFinished(ctx, s1, finished)
					assert.NoErr(t, err)
					assert.Loosely(t, sideEffect, should.NotBeNil)
					tasks := sideEffect.(*TriggerPurgeCLTasks)

					// Should purge the vote on both 203 and 204.
					assert.Loosely(t, tasks.payloads, should.HaveLength(2))
					checkPurgeTask(tasks.payloads[0], 203, 202)
					checkPurgeTask(tasks.payloads[1], 204, 202)

					// Only the top CL should be configured to send an email.
					assert.That(t, tasks.payloads[0].PurgingCl.Notification, should.Match(clpurger.NoNotification))
					assert.Loosely(t, tasks.payloads[1].PurgingCl.Notification, should.BeNil)
				})
				t.Run("unless the finished Run is failed", func(t *ftt.Test) {
					finished[common.RunID(mceRun.Id)] = run.Status_SUCCEEDED
					_, sideEffect, err := h.OnRunsFinished(ctx, s1, finished)
					assert.NoErr(t, err)
					assert.Loosely(t, sideEffect, should.BeNil)
				})
				t.Run("unless they have ongoing Runs", func(t *ftt.Test) {
					finished[common.RunID(mceRun.Id)] = run.Status_FAILED
					// create a run for the middle CL, not the top CL.
					middleRun := &prjpb.PRun{
						Id:    "203-deef",
						Mode:  string(run.FullRun),
						Clids: []int64{203},
					}
					s1.PB.Components[0].Pruns = append(s1.PB.Components[0].Pruns, middleRun)
					_, sideEffect, err := h.OnRunsFinished(ctx, s1, finished)
					assert.NoErr(t, err)
					assert.Loosely(t, sideEffect, should.NotBeNil)
					tasks := sideEffect.(*TriggerPurgeCLTasks)

					// Should purge the vote on 204 only
					assert.Loosely(t, tasks.payloads, should.HaveLength(1))
					checkPurgeTask(tasks.payloads[0], 204, 202)
					assert.Loosely(t, tasks.payloads[0].PurgingCl.Notification, should.BeNil)
				})
			})
		})
	})
}

func TestOnPurgesCompleted(t *testing.T) {
	t.Parallel()

	ftt.Run("OnPurgesCompleted works", t, func(t *ftt.Test) {
		ct := ctest{
			lProject: "test",
			gHost:    "c-review.example.com",
			Test:     cvtesting.Test{},
		}
		ctx := ct.SetUp(t)

		cfg1 := &cfgpb.Config{}
		assert.NoErr(t, prototext.Unmarshal([]byte(cfgText1), cfg1))

		prjcfgtest.Create(ctx, ct.lProject, cfg1)
		meta := prjcfgtest.MustExist(ctx, ct.lProject)
		gobmaptest.Update(ctx, ct.lProject)

		h := Handler{}
		triggerTS := timestamppb.New(ct.Clock.Now())
		ci101 := gf.CI(
			101, gf.PS(1), gf.Ref("refs/heads/main"), gf.Project("repo/a"),
			gf.CQ(+2, ct.Clock.Now(), gf.U("user-1")), gf.Updated(ct.Clock.Now()),
		)
		ci202 := gf.CI(
			202, gf.PS(3), gf.Ref("refs/heads/other"), gf.Project("repo/a"), gf.AllRevs(),
			gf.CQ(+1, ct.Clock.Now(), gf.U("user-2")), gf.Updated(ct.Clock.Now()),
		)
		ci203 := gf.CI(
			203, gf.PS(3), gf.Ref("refs/heads/other"), gf.Project("repo/a"), gf.AllRevs(),
			gf.CQ(+1, ct.Clock.Now(), gf.U("user-2")), gf.Updated(ct.Clock.Now()),
		)
		ci209 := gf.CI(
			209, gf.PS(3), gf.Ref("refs/heads/other"), gf.Project("repo/a"), gf.AllRevs(),
			gf.CQ(+1, ct.Clock.Now(), gf.U("user-2")), gf.Updated(ct.Clock.Now()),
		)

		ct.GFake.CreateChange(&gf.Change{Host: ct.gHost, ACLs: gf.ACLPublic(), Info: ci101})
		ct.GFake.CreateChange(&gf.Change{Host: ct.gHost, ACLs: gf.ACLPublic(), Info: ci202})
		ct.GFake.CreateChange(&gf.Change{Host: ct.gHost, ACLs: gf.ACLPublic(), Info: ci203})
		ct.GFake.CreateChange(&gf.Change{Host: ct.gHost, ACLs: gf.ACLPublic(), Info: ci209})
		cl101 := ct.runCLUpdater(ctx, 101)
		cl202 := ct.runCLUpdater(ctx, 202)
		cl203 := ct.runCLUpdater(ctx, 203)
		cl209 := ct.runCLUpdater(ctx, 209)

		t.Run("Empty", func(t *ftt.Test) {
			s1 := &State{PB: &prjpb.PState{}}
			s2, sideEffect, evsToConsume, err := h.OnPurgesCompleted(ctx, s1, nil)
			assert.NoErr(t, err)
			assert.Loosely(t, sideEffect, should.BeNil)
			assert.Loosely(t, s1, should.Equal(s2))
			assert.Loosely(t, evsToConsume, should.HaveLength(0))
		})

		t.Run("With existing", func(t *ftt.Test) {
			now := testclock.TestRecentTimeUTC
			ctx, _ := testclock.UseTime(ctx, now)
			s1 := &State{PB: &prjpb.PState{
				LuciProject: ct.lProject,
				PurgingCls: []*prjpb.PurgingCL{
					// expires later
					{
						Clid:        int64(cl101.ID),
						OperationId: "1",
						Deadline:    timestamppb.New(now.Add(time.Minute)),
						ApplyTo:     &prjpb.PurgingCL_AllActiveTriggers{AllActiveTriggers: true},
					},
					// expires now, but due to grace period it'll stay here.
					{
						Clid:        int64(cl202.ID),
						OperationId: "2",
						Deadline:    timestamppb.New(now),
						ApplyTo:     &prjpb.PurgingCL_AllActiveTriggers{AllActiveTriggers: true},
					},
					// definitely expired.
					{
						Clid:        int64(cl203.ID),
						OperationId: "3",
						Deadline:    timestamppb.New(now.Add(-time.Hour)),
						ApplyTo:     &prjpb.PurgingCL_AllActiveTriggers{AllActiveTriggers: true},
					},
				},
				// Components require PCLs, but in this test it doesn't matter.
				Components: []*prjpb.Component{
					{Clids: []int64{int64(cl209.ID)}}, // for unconfusing indexes below.
					{Clids: []int64{int64(cl101.ID)}},
					{Clids: []int64{int64(cl202.ID)}, TriageRequired: true},
					{Clids: []int64{int64(cl203.ID)}},
				},
				// PCLs are supposed to be sorted.
				Pcls: []*prjpb.PCL{
					{
						Clid:               int64(cl101.ID),
						Eversion:           cl101.EVersion,
						Status:             prjpb.PCL_OK,
						ConfigGroupIndexes: []int32{0},
						Triggers: &run.Triggers{CqVoteTrigger: &run.Trigger{
							Mode:            string(run.FullRun),
							Time:            triggerTS,
							Email:           gf.U("user-1").GetEmail(),
							GerritAccountId: gf.U("user-1").GetAccountId(),
						}},
					},
					{
						Clid:               int64(cl202.ID),
						Eversion:           1,
						Status:             prjpb.PCL_OK,
						ConfigGroupIndexes: []int32{1},
						Triggers: &run.Triggers{CqVoteTrigger: &run.Trigger{
							Mode:            string(run.DryRun),
							Time:            triggerTS,
							Email:           gf.U("user-2").GetEmail(),
							GerritAccountId: gf.U("user-2").GetAccountId(),
						}},
					},
					{
						Clid:               int64(cl203.ID),
						Eversion:           1,
						Status:             prjpb.PCL_OK,
						ConfigGroupIndexes: []int32{1},
						Triggers: &run.Triggers{CqVoteTrigger: &run.Trigger{
							Mode:            string(run.DryRun),
							Time:            triggerTS,
							Email:           gf.U("user-2").GetEmail(),
							GerritAccountId: gf.U("user-2").GetAccountId(),
						}},
					},
					{
						Clid:               int64(cl209.ID),
						Eversion:           1,
						Status:             prjpb.PCL_OK,
						ConfigGroupIndexes: []int32{1},
						Triggers: &run.Triggers{CqVoteTrigger: &run.Trigger{
							Mode:            string(run.DryRun),
							Time:            triggerTS,
							Email:           gf.U("user-2").GetEmail(),
							GerritAccountId: gf.U("user-2").GetAccountId(),
						}},
					},
				},
				ConfigGroupNames: []string{"g0", "g1"},
				ConfigHash:       meta.Hash(),
			}}
			pb := backupPB(s1)

			t.Run("Expires and removed", func(t *ftt.Test) {
				s2, sideEffect, evsToConsume, err := h.OnPurgesCompleted(ctx, s1, []*prjpb.PurgeCompleted{{OperationId: "1", Clid: int64(cl101.ID)}})
				assert.NoErr(t, err)
				assert.Loosely(t, sideEffect, should.BeNil)
				assert.That(t, s1.PB, should.Match(pb))
				assert.That(t, evsToConsume, should.Match([]int{0}))

				pb.PurgingCls = []*prjpb.PurgingCL{
					{
						Clid: int64(cl202.ID), OperationId: "2", Deadline: timestamppb.New(now),
						ApplyTo: &prjpb.PurgingCL_AllActiveTriggers{AllActiveTriggers: true},
					},
				}
				pb.Components = []*prjpb.Component{
					pb.Components[0],
					{Clids: []int64{int64(cl101.ID)}, TriageRequired: true},
					pb.Components[2],
					{Clids: []int64{int64(cl203.ID)}, TriageRequired: true},
				}
				assert.That(t, s2.PB, should.Match(pb))
			})

			t.Run("All removed", func(t *ftt.Test) {
				s2, sideEffect, evsToConsume, err := h.OnPurgesCompleted(ctx, s1, []*prjpb.PurgeCompleted{
					{OperationId: "3", Clid: int64(cl203.ID)},
					{OperationId: "1", Clid: int64(cl101.ID)},
					{OperationId: "5", Clid: int64(cl209.ID)},
					{OperationId: "2", Clid: int64(cl202.ID)},
				})
				assert.NoErr(t, err)
				assert.Loosely(t, sideEffect, should.BeNil)
				assert.That(t, s1.PB, should.Match(pb))
				assert.That(t, evsToConsume, should.Match([]int{0, 1, 2, 3}))
				pb.PurgingCls = nil
				pb.Components = []*prjpb.Component{
					pb.Components[0],
					{Clids: []int64{int64(cl101.ID)}, TriageRequired: true},
					pb.Components[2], // it was waiting for triage already
					{Clids: []int64{int64(cl203.ID)}, TriageRequired: true},
				}
				assert.That(t, s2.PB, should.Match(pb))
			})

			t.Run("Outdated", func(t *ftt.Test) {
				cl101.Snapshot.Outdated = &changelist.Snapshot_Outdated{}
				assert.NoErr(t, datastore.Put(ctx, cl101))
				s2, sideEffect, evsToConsume, err := h.OnPurgesCompleted(ctx, s1, []*prjpb.PurgeCompleted{
					{OperationId: "1", Clid: int64(cl101.ID)},
				})
				assert.NoErr(t, err)
				assert.Loosely(t, sideEffect, should.BeNil)
				assert.Loosely(t, s2.PB.GetPurgingCL(int64(cl101.ID)), should.NotBeNil)
				assert.Loosely(t, evsToConsume, should.BeNil)
			})

			t.Run("Doesn't modify components if they are due re-repartition anyway", func(t *ftt.Test) {
				s1.PB.RepartitionRequired = true
				pb := backupPB(s1)
				s2, sideEffect, evsToConsume, err := h.OnPurgesCompleted(ctx, s1, []*prjpb.PurgeCompleted{
					{OperationId: "1", Clid: int64(cl101.ID)},
					{OperationId: "2", Clid: int64(cl202.ID)},
					{OperationId: "3", Clid: int64(cl203.ID)},
				})
				assert.NoErr(t, err)
				assert.Loosely(t, sideEffect, should.BeNil)
				assert.That(t, s1.PB, should.Match(pb))

				pb.PurgingCls = nil
				assert.That(t, s2.PB, should.Match(pb))
				assert.That(t, evsToConsume, should.Match([]int{0, 1, 2}))
			})
		})
	})
}

func TestOnTriggeringCLDepsCompleted(t *testing.T) {
	t.Parallel()

	ftt.Run("OnTriggeringCLDepsCompleted", t, func(t *ftt.Test) {
		ct := ctest{
			lProject: "test",
			gHost:    "c-review.example.com",
			Test:     cvtesting.Test{},
		}
		ctx := ct.SetUp(t)

		cfg1 := &cfgpb.Config{}
		assert.NoErr(t, prototext.Unmarshal([]byte(cfgText1), cfg1))

		prjcfgtest.Create(ctx, ct.lProject, cfg1)
		meta := prjcfgtest.MustExist(ctx, ct.lProject)
		gobmaptest.Update(ctx, ct.lProject)

		clPoller := poller.New(ct.TQDispatcher, nil, nil, nil)
		h := Handler{CLPoller: clPoller}

		// mock CLs
		now := ct.Clock.Now()
		ci101 := gf.CI(
			101, gf.PS(1), gf.Ref("refs/heads/main"), gf.Project("repo/a"),
			gf.CQ(+2, now, gf.U("user-1")), gf.Updated(now),
		)
		ci102 := gf.CI(
			102, gf.PS(3), gf.Ref("refs/heads/main"), gf.Project("repo/a"), gf.AllRevs(),
			gf.CQ(+1, now, gf.U("user-1")), gf.Updated(now),
		)
		ci103 := gf.CI(
			103, gf.PS(3), gf.Ref("refs/heads/main"), gf.Project("repo/a"), gf.AllRevs(),
			gf.CQ(+1, now, gf.U("user-1")), gf.Updated(now),
		)
		ct.GFake.CreateChange(&gf.Change{Host: ct.gHost, ACLs: gf.ACLPublic(), Info: ci101})
		ct.GFake.CreateChange(&gf.Change{Host: ct.gHost, ACLs: gf.ACLPublic(), Info: ci102})
		ct.GFake.CreateChange(&gf.Change{Host: ct.gHost, ACLs: gf.ACLPublic(), Info: ci103})
		ct.GFake.SetDependsOn(ct.gHost, "103_3", "102_3", "101_1")
		cl101 := ct.runCLUpdater(ctx, 101)
		cl102 := ct.runCLUpdater(ctx, 102)
		cl103 := ct.runCLUpdater(ctx, 103)

		s1 := &State{PB: &prjpb.PState{
			LuciProject: ct.lProject,
			Pcls: []*prjpb.PCL{
				{
					Clid:     int64(cl101.ID),
					Eversion: 1,
					Status:   prjpb.PCL_OK,
					Triggers: &run.Triggers{CqVoteTrigger: &run.Trigger{
						Mode: string(run.FullRun),
						Time: timestamppb.New(now.Add(-10 * time.Minute)),
					}},
					ConfigGroupIndexes: []int32{0},
				},
				{
					Clid:     int64(cl102.ID),
					Eversion: 1,
					Status:   prjpb.PCL_OK,
					Triggers: &run.Triggers{CqVoteTrigger: &run.Trigger{
						Mode: string(run.FullRun),
						Time: timestamppb.New(now.Add(-10 * time.Minute)),
					}},
					ConfigGroupIndexes: []int32{0},
				},
				{
					Clid:               int64(cl103.ID),
					Eversion:           1,
					Status:             prjpb.PCL_OK,
					ConfigGroupIndexes: []int32{0},
				},
			},
			// Components require PCLs, but in this test it doesn't matter.
			Components: []*prjpb.Component{
				{Clids: []int64{int64(cl101.ID)}}, // for unconfusing indexes below.
				{Clids: []int64{int64(cl102.ID)}},
				{Clids: []int64{int64(cl103.ID)}},
			},
			ConfigGroupNames: []string{"g0"},
			ConfigHash:       meta.Hash(),
		}}
		addTriggeringCLDeps := func(s *State, deadline time.Time, origin *changelist.CL, deps ...*changelist.CL) *prjpb.TriggeringCLDeps {
			var clids []int64
			for _, dep := range deps {
				clids = append(clids, int64(dep.ID))
			}
			op := &prjpb.TriggeringCLDeps{
				OriginClid:  int64(origin.ID),
				DepClids:    clids,
				OperationId: fmt.Sprintf("op-%d", origin.ID),
				Deadline:    timestamppb.New(deadline),
				Trigger:     &run.Trigger{Mode: string(run.FullRun)},
			}
			s.PB.TriggeringClDeps, _ = s.PB.COWTriggeringCLDeps(nil, []*prjpb.TriggeringCLDeps{op})
			return op
		}
		TriggeringCLDeps := func(s *State, cl *changelist.CL) *prjpb.TriggeringCLDeps {
			return s.PB.GetTriggeringCLDeps(int64(cl.ID))
		}

		t.Run("effectively noop if empty", func(t *ftt.Test) {
			s2, se, evIndexes, err := h.OnTriggeringCLDepsCompleted(ctx, s1, nil)
			assert.NoErr(t, err)
			assert.Loosely(t, se, should.BeNil)
			assert.Loosely(t, evIndexes, should.BeNil)
			// OnTriggeringCLDepsCompleted() always makes a shallow clone for
			// PCL evaluations. There shouldn't be any changes other than that.
			s2.alreadyCloned = true
			assert.That(t, s1, should.Match(s2))
		})
		t.Run("removes an expired op", func(t *ftt.Test) {
			addTriggeringCLDeps(s1, now.Add(-time.Hour), cl103, cl101, cl102)
			s2, se, evIndexes, err := h.OnTriggeringCLDepsCompleted(ctx, s1, nil)
			assert.NoErr(t, err)
			assert.Loosely(t, TriggeringCLDeps(s2, cl103), should.BeNil)
			assert.Loosely(t, se, should.BeNil)
			assert.Loosely(t, evIndexes, should.BeNil)
		})
		t.Run("with succeeeded ops", func(t *ftt.Test) {
			op := addTriggeringCLDeps(s1, now.Add(time.Minute), cl103, cl101, cl102)
			events := []*prjpb.TriggeringCLDepsCompleted{
				{
					OperationId: op.GetOperationId(),
					Origin:      int64(cl103.ID),
					Succeeded:   []int64{int64(cl101.ID), int64(cl102.ID)},
				},
			}
			t.Run("removes the op", func(t *ftt.Test) {
				s2, se, evIndexes, err := h.OnTriggeringCLDepsCompleted(ctx, s1, events)
				assert.NoErr(t, err)
				assert.Loosely(t, TriggeringCLDeps(s2, cl103), should.BeNil)
				assert.Loosely(t, se, should.BeNil)
				assert.That(t, evIndexes, should.Match([]int{0}))
			})
			t.Run("keeps the op, if any dep PCL is outdated", func(t *ftt.Test) {
				cl102.Snapshot.Outdated = &changelist.Snapshot_Outdated{}
				assert.NoErr(t, datastore.Put(ctx, cl102 /* dep */))
				s2, se, evIndexes, err := h.OnTriggeringCLDepsCompleted(ctx, s1, events)
				assert.NoErr(t, err)
				assert.Loosely(t, TriggeringCLDeps(s2, cl103 /* origin */), should.NotBeNil)
				assert.Loosely(t, se, should.BeNil)
				assert.Loosely(t, evIndexes, should.BeNil)
			})
		})
		t.Run("enqueues PurgeCLTasks for the origin and dep CLs, if an Op has fails", func(t *ftt.Test) {
			op := addTriggeringCLDeps(s1, now.Add(time.Minute), cl103, cl101, cl102)
			events := []*prjpb.TriggeringCLDepsCompleted{
				{
					OperationId: op.GetOperationId(),
					Origin:      int64(cl103.ID),
					Succeeded:   []int64{int64(cl101.ID)},
					Failed: []*changelist.CLError_TriggerDeps{{
						PermissionDenied: []*changelist.CLError_TriggerDeps_PermissionDenied{{
							Clid:  int64(cl102.ID),
							Email: "foo@example.org",
						}},
					}},
				},
			}
			s2, se, evIndexes, err := h.OnTriggeringCLDepsCompleted(ctx, s1, events)
			assert.NoErr(t, err)
			assert.That(t, evIndexes, should.Match([]int{0}))

			// remove the TriggeringCLDeps, but schedule PurgingCL(s).
			assert.Loosely(t, TriggeringCLDeps(s2, cl103), should.BeNil)
			assert.Loosely(t, s2.PB.GetPurgingCL(int64(cl101.ID)), should.NotBeNil)
			assert.Loosely(t, s2.PB.GetPurgingCL(int64(cl102.ID)), should.BeNil)

			// verify the PurginCL payload.
			tasks := se.(*TriggerPurgeCLTasks)
			dl := timestamppb.New(now.Add(maxPurgingCLDuration))
			opID := dl.AsTime().Unix()
			assert.Loosely(t, tasks.payloads, should.HaveLength(2))
			tr := &run.Triggers{
				CqVoteTrigger: &run.Trigger{
					Mode: string(run.FullRun),
				},
			}
			oriPT, depPT := tasks.payloads[0], tasks.payloads[1]
			if oriPT.GetPurgingCl().GetClid() != int64(cl103.ID) {
				oriPT, depPT = depPT, oriPT
			}
			expectedPurgeReasons := []*prjpb.PurgeReason{
				{
					ClError: &changelist.CLError{
						Kind: &changelist.CLError_TriggerDeps_{
							TriggerDeps: &changelist.CLError_TriggerDeps{
								PermissionDenied: []*changelist.CLError_TriggerDeps_PermissionDenied{{
									Clid:  int64(cl102.ID),
									Email: "foo@example.org",
								}},
							},
						},
					},
					ApplyTo: &prjpb.PurgeReason_Triggers{Triggers: tr},
				},
			}
			assert.That(t, oriPT.GetPurgeReasons(), should.Match(expectedPurgeReasons))
			assert.That(t, oriPT.GetPurgingCl(), should.Match(&prjpb.PurgingCL{
				Clid:     int64(cl103.ID),
				Deadline: dl,
				// Must be nil for the default notifications.
				Notification: nil,
				OperationId:  fmt.Sprintf("%d-%d", opID, cl103.ID),
				ApplyTo:      &prjpb.PurgingCL_Triggers{Triggers: tr},
			}))
			assert.That(t, depPT.GetPurgeReasons(), should.Match(expectedPurgeReasons))
			assert.That(t, depPT.GetPurgingCl(), should.Match(&prjpb.PurgingCL{
				Clid:         int64(cl101.ID),
				Deadline:     dl,
				Notification: clpurger.NoNotification,
				OperationId:  fmt.Sprintf("%d-%d", opID, cl101.ID),
				ApplyTo:      &prjpb.PurgingCL_Triggers{Triggers: tr},
			}))
		})
	})
}

// backupPB returns a deep copy of State.PB for future assertion that State
// wasn't modified.
func backupPB(s *State) *prjpb.PState {
	ret := &prjpb.PState{}
	proto.Merge(ret, s.PB)
	return ret
}

func bumpEVersion(t testing.TB, ctx context.Context, cl *changelist.CL, desired int64) {
	t.Helper()
	if cl.EVersion >= desired {
		panic(fmt.Errorf("can't go %d to %d", cl.EVersion, desired))
	}
	cl.EVersion = desired
	assert.Loosely(t, datastore.Put(ctx, cl), should.BeNil, truth.LineContext())
}

func defaultPCL(cl *changelist.CL) *prjpb.PCL {
	p := &prjpb.PCL{
		Clid:               int64(cl.ID),
		Eversion:           cl.EVersion,
		ConfigGroupIndexes: []int32{0},
		Status:             prjpb.PCL_OK,
		Deps:               cl.Snapshot.GetDeps(),
	}
	ci := cl.Snapshot.GetGerrit().GetInfo()
	if ci != nil {
		p.Triggers = trigger.Find(&trigger.FindInput{ChangeInfo: ci, ConfigGroup: &cfgpb.ConfigGroup{}})
	}
	return p
}

func i64s(vs ...any) []int64 {
	res := make([]int64, len(vs))
	for i, v := range vs {
		switch x := v.(type) {
		case int64:
			res[i] = x
		case common.CLID:
			res[i] = int64(x)
		case int:
			res[i] = int64(x)
		default:
			panic(fmt.Errorf("unknown type: %T %v", v, v))
		}
	}
	return res
}

func i64sorted(vs ...any) []int64 {
	res := i64s(vs...)
	sort.Slice(res, func(i, j int) bool { return res[i] < res[j] })
	return res
}

func sortPCLs(vs []*prjpb.PCL) []*prjpb.PCL {
	sort.Slice(vs, func(i, j int) bool { return vs[i].GetClid() < vs[j].GetClid() })
	return vs
}

func mkClidsSet(cls map[int]*changelist.CL, ids ...int) common.CLIDsSet {
	res := make(common.CLIDsSet, len(ids))
	for _, id := range ids {
		res[cls[id].ID] = struct{}{}
	}
	return res
}

func sortByFirstCL(cs []*prjpb.Component) []*prjpb.Component {
	sort.Slice(cs, func(i, j int) bool { return cs[i].GetClids()[0] < cs[j].GetClids()[0] })
	return cs
}
