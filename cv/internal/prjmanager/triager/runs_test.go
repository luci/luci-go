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

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/cvtesting"
	gf "go.chromium.org/luci/cv/internal/gerrit/gerritfake"
	"go.chromium.org/luci/cv/internal/gerrit/trigger"
	"go.chromium.org/luci/cv/internal/prjmanager/prjpb"
	"go.chromium.org/luci/cv/internal/run"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestFindImmediateHardDeps(t *testing.T) {
	t.Parallel()

	Convey("findImmediateHardDeps", t, func() {
		ct := cvtesting.Test{}
		_, cancel := ct.SetUp(t)
		defer cancel()

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

		Convey("without deps", func() {
			cl1 := newCL()
			cl2 := newCL()
			So(rs.findImmediateHardDeps(cl1.pcl), ShouldEqual, CLIDs())
			So(rs.findImmediateHardDeps(cl2.pcl), ShouldEqual, CLIDs())
		})

		Convey("with HARD deps", func() {
			Convey("one immediate parent", func() {
				cl1 := newCL()
				cl2 := newCL().Deps(cl1)
				cl3 := newCL().Deps(cl1, cl2)

				So(rs.findImmediateHardDeps(cl1.pcl), ShouldEqual, CLIDs())
				So(rs.findImmediateHardDeps(cl2.pcl), ShouldEqual, CLIDs(cl1.pcl))
				So(rs.findImmediateHardDeps(cl3.pcl), ShouldEqual, CLIDs(cl2.pcl))
			})

			Convey("more than one immediate parents", func() {
				// stack 1
				s1cl1 := newCL()
				s1cl2 := newCL().Deps(s1cl1)
				So(rs.findImmediateHardDeps(s1cl1.pcl), ShouldEqual, CLIDs())
				So(rs.findImmediateHardDeps(s1cl2.pcl), ShouldEqual, CLIDs(s1cl1.pcl))
				// stack 2
				s2cl1 := newCL()
				s2cl2 := newCL().Deps(s2cl1)
				So(rs.findImmediateHardDeps(s2cl1.pcl), ShouldEqual, CLIDs())
				So(rs.findImmediateHardDeps(s2cl2.pcl), ShouldEqual, CLIDs(s2cl1.pcl))

				// cl3 belongs to both stack 1 and 2.
				// both s1cl2 and s2cl2 should be its immediate parents.
				cl3 := newCL().Deps(s1cl1, s1cl2, s2cl1, s2cl2)
				So(rs.findImmediateHardDeps(cl3.pcl), ShouldEqual, CLIDs(s1cl2.pcl, s2cl2.pcl))
			})
		})

		Convey("with SOFT deps", func() {
			cl1 := newCL()
			cl2 := newCL().SoftDeps(cl1)
			cl3 := newCL().SoftDeps(cl1, cl2)

			So(rs.findImmediateHardDeps(cl1.pcl), ShouldEqual, CLIDs())
			So(rs.findImmediateHardDeps(cl2.pcl), ShouldEqual, CLIDs())
			So(rs.findImmediateHardDeps(cl3.pcl), ShouldEqual, CLIDs())
		})

		Convey("with a mix of HARD and SOFT deps", func() {
			cl1 := newCL()
			clA := newCL()
			cl2 := newCL().Deps(cl1)
			cl3 := newCL().Deps(cl1, cl2).SoftDeps(clA)
			cl4 := newCL().Deps(cl1, cl2, cl3)

			So(rs.findImmediateHardDeps(cl1.pcl), ShouldEqual, CLIDs())
			So(rs.findImmediateHardDeps(cl2.pcl), ShouldEqual, CLIDs(cl1.pcl))
			So(rs.findImmediateHardDeps(cl3.pcl), ShouldEqual, CLIDs(cl2.pcl))
			So(rs.findImmediateHardDeps(cl4.pcl), ShouldEqual, CLIDs(cl3.pcl))
		})
	})
}

func TestQuotaPayer(t *testing.T) {
	t.Parallel()

	Convey("quotaPayer", t, func() {
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

		Convey("panics", func() {
			var expected string
			Convey("if owner is empty", func() {
				owner = ""
				expected = "empty owner"
			})
			Convey("if owner is annoymous", func() {
				owner = identity.AnonymousIdentity
				expected = "the CL owner is anonymous"
			})
			So(func() { quotaPayer(cl, owner, trigg, tr) }, ShouldPanicLike, expected)
		})

		Convey("with NewPatchsetRun", func() {
			// must be the owner always.
			tr.Mode = string(run.NewPatchsetRun)
			// the owner is the triggerer in NPR.
			trigg = owner

			setAutoSubmit(cl, true)
			So(quotaPayer(cl, owner, trigg, tr), ShouldEqual, trigg)
			setAutoSubmit(cl, false)
			So(quotaPayer(cl, owner, trigg, tr), ShouldEqual, trigg)
		})

		Convey("with Dry-Run", func() {
			Convey("by the StandardMode with AS", func() {
				tr.Mode = string(run.DryRun)
				setAutoSubmit(cl, true)
			})
			Convey("by the StandardMode without AS", func() {
				tr.Mode = string(run.DryRun)
				setAutoSubmit(cl, false)
			})
			Convey("by the CustomMode with AS", func() {
				tr.Mode = "custom-mode"
				tr.ModeDefinition = &cfgpb.Mode{
					Name:            "custom-mode",
					CqLabelValue:    trigger.CQVoteByMode(run.DryRun),
					TriggeringLabel: "custom-label",
				}
				setAutoSubmit(cl, true)
			})
			Convey("by the CustomMode without AS", func() {
				tr.Mode = "custom-mode"
				tr.ModeDefinition = &cfgpb.Mode{
					Name:            "custom-mode",
					CqLabelValue:    trigger.CQVoteByMode(run.DryRun),
					TriggeringLabel: "custom-label",
				}
				setAutoSubmit(cl, false)
			})
			// Must be the triggerer always.
			So(quotaPayer(cl, owner, trigg, tr), ShouldEqual, trigg)
		})
		Convey("with Full-Run", func() {
			var expected identity.Identity
			Convey("by the StandardMode with AS", func() {
				tr.Mode = string(run.FullRun)
				setAutoSubmit(cl, true)
				expected = owner
			})
			Convey("by the StandardMode without AS", func() {
				tr.Mode = string(run.FullRun)
				setAutoSubmit(cl, false)
				expected = trigg
			})
			Convey("by the CustomMode with AS", func() {
				tr.Mode = "custom-mode"
				tr.ModeDefinition = &cfgpb.Mode{
					Name:            "custom-mode",
					CqLabelValue:    trigger.CQVoteByMode(run.FullRun),
					TriggeringLabel: "custom-label",
				}
				setAutoSubmit(cl, true)
				expected = owner
			})
			Convey("by the CustomMode without AS", func() {
				tr.Mode = "custom-mode"
				tr.ModeDefinition = &cfgpb.Mode{
					Name:            "custom-mode",
					CqLabelValue:    trigger.CQVoteByMode(run.FullRun),
					TriggeringLabel: "custom-label",
				}
				setAutoSubmit(cl, false)
				expected = trigg
			})
			So(quotaPayer(cl, owner, trigg, tr), ShouldEqual, expected)
		})
	})
}
