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

package prjcfg

import (
	"context"
	"testing"

	"google.golang.org/protobuf/encoding/prototext"

	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/cfgclient"
	cfgmemory "go.chromium.org/luci/config/impl/memory"
	gaememory "go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	pb "go.chromium.org/luci/cv/api/config/v2"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestLoadingConfigs(t *testing.T) {
	t.Parallel()
	Convey("Load project config works", t, func() {
		ctx := gaememory.Use(context.Background())
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)

		const project = "chromium"

		Convey("Not existing project", func() {
			m, err := GetLatestMeta(ctx, project)
			So(err, ShouldBeNil)
			So(m.Exists(), ShouldBeFalse)
			So(m.EVersion, ShouldEqual, 0)
			So(func() { m.Hash() }, ShouldPanic)
		})

		cfg := &pb.Config{
			ConfigGroups: []*pb.ConfigGroup{
				{
					Name: "branch_m100",
					Gerrit: []*pb.ConfigGroup_Gerrit{
						{
							Url: "https://chromium-review.googlesource.com/",
							Projects: []*pb.ConfigGroup_Gerrit_Project{
								{
									Name:      "chromium/src",
									RefRegexp: []string{"refs/heads/branch_m100"},
								},
							},
						},
					},
				},
				{
					Fallback: pb.Toggle_YES,
					Gerrit: []*pb.ConfigGroup_Gerrit{
						{
							Url: "https://chromium-review.googlesource.com/",
							Projects: []*pb.ConfigGroup_Gerrit_Project{
								{
									Name:      "chromium/src",
									RefRegexp: []string{"refs/heads/main"},
								},
							},
						},
					},
				},
			},
		}
		ctx = cfgclient.Use(ctx, cfgmemory.New(map[config.Set]cfgmemory.Files{
			config.ProjectSet(project): {ConfigFileName: prototext.Format(cfg)},
		}))
		So(UpdateProject(ctx, project, func(context.Context) error { return nil }), ShouldBeNil)

		Convey("Enabled project", func() {
			m, err := GetLatestMeta(ctx, project)
			So(err, ShouldBeNil)
			So(m.Exists(), ShouldBeTrue)
			So(m.Status, ShouldEqual, StatusEnabled)
			So(m.EVersion, ShouldEqual, 1)
			So(m.ConfigGroupNames, ShouldResemble, []string{"branch_m100", "index#1"})
			h := m.Hash()
			So(h, ShouldStartWith, "sha256:")
			So(m.ConfigGroupIDs, ShouldResemble, []ConfigGroupID{
				ConfigGroupID(h + "/branch_m100"),
				ConfigGroupID(h + "/index#1"),
			})

			m2, err := GetHashMeta(ctx, project, h)
			So(m2, ShouldResemble, m)

			cgs, err := m.GetConfigGroups(ctx)
			So(err, ShouldBeNil)
			So(len(cgs), ShouldEqual, 2)
			So(cgs[0].Content, ShouldResembleProto, cfg.ConfigGroups[0])
			So(cgs[1].Content, ShouldResembleProto, cfg.ConfigGroups[1])
		})

		cfg.ConfigGroups = append(cfg.ConfigGroups, &pb.ConfigGroup{
			Name: "branch_m200",
			Gerrit: []*pb.ConfigGroup_Gerrit{
				{
					Url: "https://chromium-review.googlesource.com/",
					Projects: []*pb.ConfigGroup_Gerrit_Project{
						{
							Name:      "chromium/src",
							RefRegexp: []string{"refs/heads/branch_m200"},
						},
					},
				},
			},
		})

		ctx = cfgclient.Use(ctx, cfgmemory.New(map[config.Set]cfgmemory.Files{
			config.ProjectSet(project): {ConfigFileName: prototext.Format(cfg)},
		}))
		So(UpdateProject(ctx, project, func(context.Context) error { return nil }), ShouldBeNil)

		Convey("Updated project", func() {
			m, err := GetLatestMeta(ctx, project)
			So(err, ShouldBeNil)
			So(m.Exists(), ShouldBeTrue)
			So(m.Status, ShouldEqual, StatusEnabled)
			So(m.EVersion, ShouldEqual, 2)
			h := m.Hash()
			So(h, ShouldStartWith, "sha256:")
			So(m.ConfigGroupIDs, ShouldResemble, []ConfigGroupID{
				ConfigGroupID(h + "/branch_m100"),
				ConfigGroupID(h + "/index#1"),
				ConfigGroupID(h + "/branch_m200"),
			})
			cgs, err := m.GetConfigGroups(ctx)
			So(err, ShouldBeNil)
			So(len(cgs), ShouldEqual, 3)

			Convey("reading ConfigGroup directly works", func() {
				cg, err := GetConfigGroup(ctx, project, m.ConfigGroupIDs[2])
				So(err, ShouldBeNil)
				So(cg.Content, ShouldResembleProto, cfg.ConfigGroups[2])
			})
		})

		So(DisableProject(ctx, project, func(context.Context) error { return nil }), ShouldBeNil)
		Convey("Disabled project", func() {
			m, err := GetLatestMeta(ctx, project)
			So(err, ShouldBeNil)
			So(m.Exists(), ShouldBeTrue)
			So(m.Status, ShouldEqual, StatusDisabled)
			So(m.EVersion, ShouldEqual, 3)
			So(len(m.ConfigGroupIDs), ShouldEqual, 3)
			cgs, err := m.GetConfigGroups(ctx)
			So(err, ShouldBeNil)
			So(len(cgs), ShouldEqual, 3)
		})

		// Re-enable the project.
		So(UpdateProject(ctx, project, func(context.Context) error { return nil }), ShouldBeNil)
		Convey("Re-enabled project", func() {
			m, err := GetLatestMeta(ctx, project)
			So(err, ShouldBeNil)
			So(m.Exists(), ShouldBeTrue)
			So(m.Status, ShouldEqual, StatusEnabled)
		})

		m, err := GetLatestMeta(ctx, project)
		So(err, ShouldBeNil)
		cgs, err := m.GetConfigGroups(ctx)
		So(err, ShouldBeNil)

		Convey("Deleted project", func() {
			So(datastore.Delete(ctx, &ProjectConfig{Project: project}, cgs), ShouldBeNil)

			m, err = GetLatestMeta(ctx, project)
			So(err, ShouldBeNil)
			So(m.Exists(), ShouldBeFalse)
		})

		Convey("reading partially deleted project", func() {
			So(datastore.Delete(ctx, cgs[1]), ShouldBeNil)
			_, err = m.GetConfigGroups(ctx)
			So(err, ShouldErrLike, "ConfigGroups for")
			So(err, ShouldErrLike, "not found")
			So(datastore.IsErrNoSuchEntity(err), ShouldBeTrue)

			// Can still read individual ConfigGroups.
			cg, err := GetConfigGroup(ctx, project, m.ConfigGroupIDs[0])
			So(err, ShouldBeNil)
			So(cg.Content, ShouldResembleProto, cfg.ConfigGroups[0])
			cg, err = GetConfigGroup(ctx, project, m.ConfigGroupIDs[2])
			So(err, ShouldBeNil)
			So(cg.Content, ShouldResembleProto, cfg.ConfigGroups[2])
			// ... except the deleted one.
			cg, err = GetConfigGroup(ctx, project, m.ConfigGroupIDs[1])
			So(datastore.IsErrNoSuchEntity(err), ShouldBeTrue)
		})
	})
}
