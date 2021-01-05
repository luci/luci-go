// Copyright 2020 The LUCI Authors.
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

package impl

import (
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/server/tq/tqtesting"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/cvtesting"
	gf "go.chromium.org/luci/cv/internal/gerrit/gerritfake"
	"go.chromium.org/luci/cv/internal/gerrit/gobmap"
	"go.chromium.org/luci/cv/internal/gerrit/trigger"
	"go.chromium.org/luci/cv/internal/gerrit/updater"
	"go.chromium.org/luci/cv/internal/prjmanager"
	"go.chromium.org/luci/cv/internal/run"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestUpdateConfig(t *testing.T) {
	t.Parallel()

	Convey("updateConfig works", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()

		const lProject = "test"
		const gHost = "c-review.example.com"

		runCLUpdater := func(change int64) *changelist.CL {
			So(updater.Schedule(ctx, lProject, gHost, change, time.Time{}, 0), ShouldBeNil)
			ct.TQ.Run(ctx, tqtesting.StopAfterTask(updater.TaskClassID))
			eid, err := changelist.GobID(gHost, change)
			So(err, ShouldBeNil)
			cl, err := eid.Get(ctx)
			So(err, ShouldBeNil)
			So(cl, ShouldNotBeNil)
			return cl
		}

		const cfgText1 = `
      config_groups {
        name: "g0"
        gerrit {
          url: "https://c-review.example.com"  # Must match gHost.
          projects {
            name: "r1"
            ref_regexp:         "refs/heads/main"
          }
        }
      }
      config_groups {
        name: "g1"
				fallback: YES
        gerrit {
          url: "https://c-review.example.com"  # Must match gHost.
          projects {
            name: "r1"
            ref_regexp:         "refs/heads/.+"
          }
        }
      }`
		cfg1 := &cfgpb.Config{}
		So(prototext.Unmarshal([]byte(cfgText1), cfg1), ShouldBeNil)

		ct.Cfg.Create(ctx, lProject, cfg1)
		meta := ct.Cfg.MustExist(ctx, lProject)
		So(gobmap.Update(ctx, lProject), ShouldBeNil)

		// Add 1 CL.
		ci := gf.CI(
			101, gf.Ref("refs/heads/main"), gf.Project("r1"),
			gf.CQ(+2, ct.Clock.Now(), gf.U("user-1")), gf.Updated(ct.Clock.Now()),
		)
		ct.GFake.CreateChange(&gf.Change{Host: gHost, ACLs: gf.ACLPublic(), Info: ci})
		cl := runCLUpdater(101)

		Convey("initializes newly started project", func() {
			p := &pendingCLs{
				PendingCLs:  &prjmanager.PendingCLs{},
				luciProject: lProject,
			}
			newP, sideEffectFn, err := p.updateConfig(ctx, meta)
			So(err, ShouldBeNil)
			So(sideEffectFn, ShouldBeNil)
			So(newP, ShouldNotEqual, p)
			So(newP.PendingCLs, ShouldResembleProto, &prjmanager.PendingCLs{
				ConfigGroupNames: []string{"g0", "g1"},
				DirtyComponents:  true,
			})
			So(newP.configHash, ShouldEqual, meta.Hash())
		})

		Convey("updates existing projects watching a CLs", func() {
			ci2 := gf.CI(
				102, gf.Ref("refs/heads/other"), gf.Project("r1"),
				gf.CQ(+1, ct.Clock.Now(), gf.U("user-2")), gf.Updated(ct.Clock.Now()),
			)
			ct.GFake.CreateChange(&gf.Change{Host: gHost, ACLs: gf.ACLPublic(), Info: ci2})
			cl2 := runCLUpdater(102)

			p := &pendingCLs{
				PendingCLs: &prjmanager.PendingCLs{
					ConfigGroupNames: []string{"g0", "g1"},
					Cls: []*prjmanager.PendingCL{
						{
							Clid:             int64(cl.ID),
							Eversion:         int64(cl.EVersion),
							ConfigGroupIndex: 0, // g0.
							Trigger: &run.Trigger{
								Email:           "user-1@example.com",
								GerritAccountId: 1,
								Mode:            string(run.FullRun),
								Time:            timestamppb.New(ct.Clock.Now()),
							},
						},
						{
							Clid:             int64(cl2.ID),
							Eversion:         int64(cl2.EVersion),
							ConfigGroupIndex: 1, // g1.
							Trigger: &run.Trigger{
								Email:           "user-2@example.com",
								GerritAccountId: 2,
								Mode:            string(run.DryRun),
								Time:            timestamppb.New(ct.Clock.Now()),
							},
						},
					},
					Components: []*prjmanager.Component{
						{
							PendingIds:   []int64{int64(cl.ID)},
							DecisionTime: timestamppb.New(ct.Clock.Now().Add(5 * time.Minute)),
						},
						{
							PendingIds:   []int64{int64(cl2.ID)},
							DecisionTime: timestamppb.New(ct.Clock.Now().Add(5 * time.Minute)),
						},
					},
				},
				luciProject: lProject,
				configHash:  meta.Hash(),
			}

			cfgText2 := strings.ReplaceAll(cfgText1, "fallback: YES", "fallback: NO")
			cfg2 := &cfgpb.Config{}
			So(prototext.Unmarshal([]byte(cfgText2), cfg2), ShouldBeNil)
			ct.Cfg.Update(ctx, lProject, cfg2)
			meta2 := ct.Cfg.MustExist(ctx, lProject)

			expected := &prjmanager.PendingCLs{
				ConfigGroupNames: []string{"g0", "g1"},
				Cls: []*prjmanager.PendingCL{
					{
						// Changed because g1 is no longer fallback.
						Clid:                     1,
						Eversion:                 1,
						ConfigGroupIndex:         0,          // g0.
						ExcessConfigGroupIndexes: []int32{1}, // g1.
						Trigger: &run.Trigger{
							Email:           "user-1@example.com",
							GerritAccountId: 1,
							Mode:            string(run.FullRun),
							Time:            timestamppb.New(ct.Clock.Now()),
						},
					},
					{
						// Not changed, same as before
						Clid:             int64(cl2.ID),
						Eversion:         int64(cl2.EVersion),
						ConfigGroupIndex: 1, // g1.
						Trigger: &run.Trigger{
							Email:           "user-2@example.com",
							GerritAccountId: 2,
							Mode:            string(run.DryRun),
							Time:            timestamppb.New(ct.Clock.Now()),
						},
					},
				},
				Components:      nil,
				DirtyComponents: true,
			}

			Convey("all CLs still exist", func() {
				newP, sideEffectFn, err := p.updateConfig(ctx, meta2)
				So(err, ShouldBeNil)
				So(sideEffectFn, ShouldBeNil)
				So(newP, ShouldNotEqual, p)
				So(newP.configHash, ShouldEqual, meta2.Hash())
				So(newP.PendingCLs, ShouldResembleProto, expected)
			})

			Convey("updates existing projects with some CLs already deleted", func() {
				p.PendingCLs.Cls = append(p.PendingCLs.Cls, &prjmanager.PendingCL{
					Clid:             404, // Not in Datastore.
					Eversion:         1,
					ConfigGroupIndex: 1,
					Trigger: &run.Trigger{
						Email:           "user-3@example.com",
						GerritAccountId: 3,
						Mode:            string(run.DryRun),
						Time:            timestamppb.New(ct.Clock.Now()),
					},
				})
				p.PendingCLs.Components = append(p.PendingCLs.Components, &prjmanager.Component{
					PendingIds:   []int64{404},
					DecisionTime: timestamppb.New(ct.Clock.Now().Add(5 * time.Minute)),
				})

				newP, sideEffectFn, err := p.updateConfig(ctx, meta2)
				So(err, ShouldBeNil)
				So(sideEffectFn, ShouldBeNil)
				So(newP, ShouldNotEqual, p)
				So(newP.configHash, ShouldEqual, meta2.Hash())
				So(newP.PendingCLs, ShouldResembleProto, expected)
			})
		})

		// The rest of the test coverage of UpdateConfig is achieved by testing code
		// of makePendingCL.

		Convey("makePendingCL with full snapshot works", func() {
			p := &pendingCLs{
				PendingCLs: &prjmanager.PendingCLs{
					ConfigGroupNames: []string{"g0", "g1"},
					DirtyComponents:  true,
				},
				luciProject: lProject,
				configHash:  meta.Hash(),
			}

			Convey("happy path", func() {
				expected := &prjmanager.PendingCL{
					Clid:             int64(cl.ID),
					Eversion:         int64(cl.EVersion),
					ConfigGroupIndex: 0, // g0
					Trigger: &run.Trigger{
						Email:           "user-1@example.com",
						GerritAccountId: 1,
						Mode:            string(run.FullRun),
						Time:            timestamppb.New(ct.Clock.Now()),
					},
				}
				Convey("CL snapshotted with current config", func() {
					pcl, err := p.makePendingCL(ctx, cl)
					So(err, ShouldBeNil)
					So(pcl, ShouldResembleProto, expected)
				})
				Convey("CL snapshotted with an older config", func() {
					cl.ApplicableConfig.GetProjects()[0].ConfigGroupIds = []string{"oldhash/g0"}
					pcl, err := p.makePendingCL(ctx, cl)
					So(err, ShouldBeNil)
					So(pcl, ShouldResembleProto, expected)
				})
			})

			Convey("snapshot from diff project requires waiting", func() {
				cl.Snapshot.LuciProject = "another"
				pcl, err := p.makePendingCL(ctx, cl)
				So(err, ShouldBeNil)
				So(pcl, ShouldResembleProto, &prjmanager.PendingCL{
					Clid:             int64(cl.ID),
					Eversion:         int64(cl.EVersion),
					ConfigGroupIndex: unknownConfigGroupIndex, // -1
				})
			})

			Convey("CL from diff project are ignored", func() {
				p.luciProject = "another"
				pcl, err := p.makePendingCL(ctx, cl)
				So(err, ShouldBeNil)
				So(pcl, ShouldBeNil)
			})

			Convey("CL watched by several projects is ignored", func() {
				cl.ApplicableConfig.Projects = append(
					cl.ApplicableConfig.GetProjects(),
					&changelist.ApplicableConfig_Project{
						ConfigGroupIds: []string{"g"},
						Name:           "another",
					})
				pcl, err := p.makePendingCL(ctx, cl)
				So(err, ShouldBeNil)
				So(pcl, ShouldBeNil)
			})

			Convey("not triggered CL is ignored", func() {
				delete(cl.Snapshot.GetGerrit().GetInfo().GetLabels(), trigger.CQLabelName)
				pcl, err := p.makePendingCL(ctx, cl)
				So(err, ShouldBeNil)
				So(pcl, ShouldBeNil)
			})
		})
	})
}
