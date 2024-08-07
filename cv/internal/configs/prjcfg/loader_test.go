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

	"go.chromium.org/luci/gae/service/datastore"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/cvtesting"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func mockUpdateProjectConfig(ctx context.Context, project string, cgs []*cfgpb.ConfigGroup) error {
	const cfgHash = "sha256:deadbeef"
	var cfgGroups []*ConfigGroup
	var cgNames []string
	for _, cg := range cgs {
		cfgGroups = append(cfgGroups, &ConfigGroup{
			ID:      MakeConfigGroupID(cfgHash, cg.Name),
			Project: datastore.MakeKey(ctx, projectConfigKind, project),
			Content: cg,
		})
		cgNames = append(cgNames, cg.Name)
	}
	pc := &ProjectConfig{
		Project:          project,
		Enabled:          true,
		Hash:             cfgHash,
		ExternalHash:     cfgHash + "external",
		ConfigGroupNames: cgNames,
		SchemaVersion:    SchemaVersion,
	}
	cfgHashInfo := &ConfigHashInfo{
		Hash:             cfgHash,
		Project:          datastore.MakeKey(ctx, projectConfigKind, project),
		ConfigGroupNames: cgNames,
		GitRevision:      "abcdef123456",
	}
	return datastore.Put(ctx, pc, cfgHashInfo, cfgGroups)
}

func mockDisableProjectConfig(ctx context.Context, project string, cgs []*cfgpb.ConfigGroup) error {
	const cfgHash = "sha256:deadbeef"
	var cgNames []string
	for _, cg := range cgs {
		cgNames = append(cgNames, cg.Name)
	}
	pc := &ProjectConfig{
		Project:          project,
		Enabled:          false,
		Hash:             cfgHash,
		ExternalHash:     cfgHash + "external",
		ConfigGroupNames: cgNames,
		SchemaVersion:    SchemaVersion,
	}
	return datastore.Put(ctx, pc)
}

func TestLoadingConfigs(t *testing.T) {
	t.Parallel()
	Convey("Load project config works", t, func() {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		const project = "chromium"
		cfgGroups := []*cfgpb.ConfigGroup{
			{
				Name: "branch_m100",
				Gerrit: []*cfgpb.ConfigGroup_Gerrit{
					{
						Url: "https://chromium-review.googlesource.com/",
						Projects: []*cfgpb.ConfigGroup_Gerrit_Project{
							{
								Name:      "chromium/src",
								RefRegexp: []string{"refs/heads/branch_m100"},
							},
						},
					},
				},
			},
			{
				Name: "main",
				Gerrit: []*cfgpb.ConfigGroup_Gerrit{
					{
						Url: "https://chromium-review.googlesource.com/",
						Projects: []*cfgpb.ConfigGroup_Gerrit_Project{
							{
								Name:      "chromium/src",
								RefRegexp: []string{"refs/heads/main"},
							},
						},
					},
				},
			},
		}

		So(mockUpdateProjectConfig(ctx, project, cfgGroups), ShouldBeNil)

		Convey("Not existing project", func() {
			m, err := GetLatestMeta(ctx, "not_chromium")
			So(err, ShouldBeNil)
			So(m.Exists(), ShouldBeFalse)
			So(func() { m.Hash() }, ShouldPanic)
		})

		Convey("Enabled project", func() {
			m, err := GetLatestMeta(ctx, project)
			So(err, ShouldBeNil)
			So(m.Exists(), ShouldBeTrue)
			So(m.Status, ShouldEqual, StatusEnabled)
			So(m.ConfigGroupNames, ShouldResemble, []string{cfgGroups[0].Name, cfgGroups[1].Name})
			So(m.ConfigGroupIDs, ShouldResemble, []ConfigGroupID{
				ConfigGroupID(m.Hash() + "/" + cfgGroups[0].Name),
				ConfigGroupID(m.Hash() + "/" + cfgGroups[1].Name),
			})

			m2, err := GetHashMeta(ctx, project, m.Hash())
			So(err, ShouldBeNil)
			So(m2, ShouldResemble, m)

			cgs, err := m.GetConfigGroups(ctx)
			So(err, ShouldBeNil)
			So(len(cgs), ShouldEqual, 2)
			So(cgs[0].Project.StringID(), ShouldEqual, project)
			So(cgs[0].Content, ShouldResembleProto, cfgGroups[0])
			So(cgs[1].Project.StringID(), ShouldEqual, project)
			So(cgs[1].Content, ShouldResembleProto, cfgGroups[1])
		})

		cfgGroups = append(cfgGroups, &cfgpb.ConfigGroup{
			Name: "branch_m200",
			Gerrit: []*cfgpb.ConfigGroup_Gerrit{
				{
					Url: "https://chromium-review.googlesource.com/",
					Projects: []*cfgpb.ConfigGroup_Gerrit_Project{
						{
							Name:      "chromium/src",
							RefRegexp: []string{"refs/heads/branch_m200"},
						},
					},
				},
			},
		})
		So(mockUpdateProjectConfig(ctx, project, cfgGroups), ShouldBeNil)

		Convey("Updated project", func() {
			m, err := GetLatestMeta(ctx, project)
			So(err, ShouldBeNil)
			So(m.Exists(), ShouldBeTrue)
			So(m.Status, ShouldEqual, StatusEnabled)
			h := m.Hash()
			So(h, ShouldStartWith, "sha256:")
			So(m.ConfigGroupIDs, ShouldResemble, []ConfigGroupID{
				ConfigGroupID(h + "/" + cfgGroups[0].Name),
				ConfigGroupID(h + "/" + cfgGroups[1].Name),
				ConfigGroupID(h + "/" + cfgGroups[2].Name),
			})
			cgs, err := m.GetConfigGroups(ctx)
			So(err, ShouldBeNil)
			So(len(cgs), ShouldEqual, 3)

			Convey("reading ConfigGroup directly works", func() {
				cg, err := GetConfigGroup(ctx, project, m.ConfigGroupIDs[2])
				So(err, ShouldBeNil)
				So(cg.Content, ShouldResembleProto, cfgGroups[2])
			})
		})

		So(mockDisableProjectConfig(ctx, project, cfgGroups), ShouldBeNil)

		Convey("Disabled project", func() {
			m, err := GetLatestMeta(ctx, project)
			So(err, ShouldBeNil)
			So(m.Exists(), ShouldBeTrue)
			So(m.Status, ShouldEqual, StatusDisabled)
			So(len(m.ConfigGroupIDs), ShouldEqual, 3)
			cgs, err := m.GetConfigGroups(ctx)
			So(err, ShouldBeNil)
			So(len(cgs), ShouldEqual, 3)
		})

		// Re-enable the project.
		So(mockUpdateProjectConfig(ctx, project, cfgGroups), ShouldBeNil)

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
			So(cg.Content, ShouldResembleProto, cfgGroups[0])
			cg, err = GetConfigGroup(ctx, project, m.ConfigGroupIDs[2])
			So(err, ShouldBeNil)
			So(cg.Content, ShouldResembleProto, cfgGroups[2])
			// ... except the deleted one.
			_, err = GetConfigGroup(ctx, project, m.ConfigGroupIDs[1])
			So(datastore.IsErrNoSuchEntity(err), ShouldBeTrue)
		})
	})
}
