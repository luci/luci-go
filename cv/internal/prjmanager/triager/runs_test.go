// Copyright 2023 The LUCI Authors.
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

package triager

import (
	"testing"

	"go.chromium.org/luci/auth/identity"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/cvtesting"
	gf "go.chromium.org/luci/cv/internal/gerrit/gerritfake"
	"go.chromium.org/luci/cv/internal/gerrit/trigger"
	"go.chromium.org/luci/cv/internal/prjmanager/prjpb"
	"go.chromium.org/luci/cv/internal/run"
)

func TestFindImmediateHardDeps(t *testing.T) {
	t.Parallel()

	ftt.Run("findImmediateHardDeps", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		_ = ct.SetUp(t)

		cls := make(map[int64]*clInfo)
		nextCLID := int64(1)
		spm := &simplePMState{
			pb: &prjpb.PState{},
		}
		newCL := func() *testCLInfo {
			defer func() { nextCLID++ }()
			tci := &testCLInfo{
				pcl: &prjpb.PCL{
					Clid: nextCLID,
				},
			}
			cls[nextCLID] = (*clInfo)(tci)
			spm.pb.Pcls = append(spm.pb.Pcls, tci.pcl)
			return tci
		}
		rs := &runStage{
			pm:  pmState{spm},
			cls: cls,
		}
		CLIDs := func(pcls ...*prjpb.PCL) []int64 {
			var ret []int64
			for _, pcl := range pcls {
				ret = append(ret, pcl.GetClid())
			}
			return ret
		}

		t.Run("without deps", func(t *ftt.Test) {
			cl1 := newCL()
			cl2 := newCL()
			assert.That(t, rs.findImmediateHardDeps(cl1.pcl), should.Match(CLIDs()))
			assert.That(t, rs.findImmediateHardDeps(cl2.pcl), should.Match(CLIDs()))
		})

		t.Run("with HARD deps", func(t *ftt.Test) {
			t.Run("one immediate parent", func(t *ftt.Test) {
				cl1 := newCL()
				cl2 := newCL().Deps(cl1)
				cl3 := newCL().Deps(cl1, cl2)

				assert.That(t, rs.findImmediateHardDeps(cl1.pcl), should.Match(CLIDs()))
				assert.That(t, rs.findImmediateHardDeps(cl2.pcl), should.Match(CLIDs(cl1.pcl)))
				assert.That(t, rs.findImmediateHardDeps(cl3.pcl), should.Match(CLIDs(cl2.pcl)))
			})

			t.Run("more than one immediate parents", func(t *ftt.Test) {
				// stack 1
				s1cl1 := newCL()
				s1cl2 := newCL().Deps(s1cl1)
				assert.That(t, rs.findImmediateHardDeps(s1cl1.pcl), should.Match(CLIDs()))
				assert.That(t, rs.findImmediateHardDeps(s1cl2.pcl), should.Match(CLIDs(s1cl1.pcl)))
				// stack 2
				s2cl1 := newCL()
				s2cl2 := newCL().Deps(s2cl1)
				assert.That(t, rs.findImmediateHardDeps(s2cl1.pcl), should.Match(CLIDs()))
				assert.That(t, rs.findImmediateHardDeps(s2cl2.pcl), should.Match(CLIDs(s2cl1.pcl)))

				// cl3 belongs to both stack 1 and 2.
				// both s1cl2 and s2cl2 should be its immediate parents.
				cl3 := newCL().Deps(s1cl1, s1cl2, s2cl1, s2cl2)
				assert.That(t, rs.findImmediateHardDeps(cl3.pcl), should.Match(CLIDs(s1cl2.pcl, s2cl2.pcl)))
			})
		})

		t.Run("with SOFT deps", func(t *ftt.Test) {
			cl1 := newCL()
			cl2 := newCL().SoftDeps(cl1)
			cl3 := newCL().SoftDeps(cl1, cl2)

			assert.That(t, rs.findImmediateHardDeps(cl1.pcl), should.Match(CLIDs()))
			assert.That(t, rs.findImmediateHardDeps(cl2.pcl), should.Match(CLIDs()))
			assert.That(t, rs.findImmediateHardDeps(cl3.pcl), should.Match(CLIDs()))
		})

		t.Run("with a mix of HARD and SOFT deps", func(t *ftt.Test) {
			cl1 := newCL()
			clA := newCL()
			cl2 := newCL().Deps(cl1)
			cl3 := newCL().Deps(cl1, cl2).SoftDeps(clA)
			cl4 := newCL().Deps(cl1, cl2, cl3)

			assert.That(t, rs.findImmediateHardDeps(cl1.pcl), should.Match(CLIDs()))
			assert.That(t, rs.findImmediateHardDeps(cl2.pcl), should.Match(CLIDs(cl1.pcl)))
			assert.That(t, rs.findImmediateHardDeps(cl3.pcl), should.Match(CLIDs(cl2.pcl)))
			assert.That(t, rs.findImmediateHardDeps(cl4.pcl), should.Match(CLIDs(cl3.pcl)))
		})
	})
}

func TestQuotaPayer(t *testing.T) {
	t.Parallel()

	ftt.Run("quotaPayer", t, func(t *ftt.Test) {
		cl := &changelist.CL{
			ID: 1234,
			Snapshot: &changelist.Snapshot{
				Kind: &changelist.Snapshot_Gerrit{
					Gerrit: &changelist.Gerrit{
						Info: &gerritpb.ChangeInfo{
							Labels: make(map[string]*gerritpb.LabelInfo),
						},
					},
				},
			},
		}
		owner := identity.Identity("user:user-1@example.com")
		trigg := identity.Identity("user:user-2@example.com")
		tr := &run.Trigger{}

		setAutoSubmit := func(cl *changelist.CL, val bool) {
			labels := cl.Snapshot.GetGerrit().GetInfo().Labels
			if !val {
				delete(labels, trigger.AutoSubmitLabelName)
			} else {
				labels[trigger.AutoSubmitLabelName] = &gerritpb.LabelInfo{
					All: []*gerritpb.ApprovalInfo{{
						User:  gf.U(owner.Value()),
						Value: 1,
					}},
				}
			}
		}

		t.Run("panics", func(t *ftt.Test) {
			t.Run("if owner is empty", func(t *ftt.Test) {
				assert.Loosely(t, func() {
					quotaPayer(cl, "", trigg, tr)
				}, should.PanicLike("empty owner"))
			})
			t.Run("if owner is annoymous", func(t *ftt.Test) {
				assert.Loosely(t, func() {
					quotaPayer(cl, identity.AnonymousIdentity, trigg, tr)
				}, should.PanicLike("the CL owner is anonymous"))
			})
		})

		t.Run("with NewPatchsetRun", func(t *ftt.Test) {
			// must be the owner always.
			tr.Mode = string(run.NewPatchsetRun)
			// the owner is the triggerer in NPR.
			trigg = owner

			setAutoSubmit(cl, true)
			assert.Loosely(t, quotaPayer(cl, owner, trigg, tr), should.Equal(trigg))
			setAutoSubmit(cl, false)
			assert.Loosely(t, quotaPayer(cl, owner, trigg, tr), should.Equal(trigg))
		})

		t.Run("with Dry-Run", func(t *ftt.Test) {
			t.Run("by the StandardMode with AS", func(t *ftt.Test) {
				tr.Mode = string(run.DryRun)
				setAutoSubmit(cl, true)
			})
			t.Run("by the StandardMode without AS", func(t *ftt.Test) {
				tr.Mode = string(run.DryRun)
				setAutoSubmit(cl, false)
			})
			t.Run("by the CustomMode with AS", func(t *ftt.Test) {
				tr.Mode = "custom-mode"
				tr.ModeDefinition = &cfgpb.Mode{
					Name:            "custom-mode",
					CqLabelValue:    trigger.CQVoteByMode(run.DryRun),
					TriggeringLabel: "custom-label",
				}
				setAutoSubmit(cl, true)
			})
			t.Run("by the CustomMode without AS", func(t *ftt.Test) {
				tr.Mode = "custom-mode"
				tr.ModeDefinition = &cfgpb.Mode{
					Name:            "custom-mode",
					CqLabelValue:    trigger.CQVoteByMode(run.DryRun),
					TriggeringLabel: "custom-label",
				}
				setAutoSubmit(cl, false)
			})
			// Must be the triggerer always.
			assert.Loosely(t, quotaPayer(cl, owner, trigg, tr), should.Equal(trigg))
		})
		t.Run("with Full-Run", func(t *ftt.Test) {
			var expected identity.Identity
			check := func(t testing.TB) {
				t.Helper()
				assert.Loosely(t, quotaPayer(cl, owner, trigg, tr), should.Equal(expected))
			}

			t.Run("by the StandardMode with AS", func(t *ftt.Test) {
				tr.Mode = string(run.FullRun)
				setAutoSubmit(cl, true)
				expected = owner
				check(t)
			})
			t.Run("by the StandardMode without AS", func(t *ftt.Test) {
				tr.Mode = string(run.FullRun)
				setAutoSubmit(cl, false)
				expected = trigg
				check(t)
			})
			t.Run("by the CustomMode with AS", func(t *ftt.Test) {
				tr.Mode = "custom-mode"
				tr.ModeDefinition = &cfgpb.Mode{
					Name:            "custom-mode",
					CqLabelValue:    trigger.CQVoteByMode(run.FullRun),
					TriggeringLabel: "custom-label",
				}
				setAutoSubmit(cl, true)
				expected = owner
				check(t)
			})
			t.Run("by the CustomMode without AS", func(t *ftt.Test) {
				tr.Mode = "custom-mode"
				tr.ModeDefinition = &cfgpb.Mode{
					Name:            "custom-mode",
					CqLabelValue:    trigger.CQVoteByMode(run.FullRun),
					TriggeringLabel: "custom-label",
				}
				setAutoSubmit(cl, false)
				expected = trigg
				check(t)
			})
		})
	})
}
