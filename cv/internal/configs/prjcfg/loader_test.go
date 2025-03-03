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

	"github.com/google/go-cmp/cmp/cmpopts"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/registry"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/service/datastore"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/cvtesting"
)

func init() {
	registry.RegisterCmpOption(cmpopts.IgnoreUnexported(Meta{}))
}

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
	ftt.Run("Load project config works", t, func(t *ftt.Test) {
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

		assert.Loosely(t, mockUpdateProjectConfig(ctx, project, cfgGroups), should.BeNil)

		t.Run("Not existing project", func(t *ftt.Test) {
			m, err := GetLatestMeta(ctx, "not_chromium")
			assert.NoErr(t, err)
			assert.Loosely(t, m.Exists(), should.BeFalse)
			assert.Loosely(t, func() { m.Hash() }, should.Panic)
		})

		t.Run("Enabled project", func(t *ftt.Test) {
			m, err := GetLatestMeta(ctx, project)
			assert.NoErr(t, err)
			assert.Loosely(t, m.Exists(), should.BeTrue)
			assert.Loosely(t, m.Status, should.Equal(StatusEnabled))
			assert.That(t, m.ConfigGroupNames, should.Match([]string{cfgGroups[0].Name, cfgGroups[1].Name}))
			assert.That(t, m.ConfigGroupIDs, should.Match([]ConfigGroupID{
				ConfigGroupID(m.Hash() + "/" + cfgGroups[0].Name),
				ConfigGroupID(m.Hash() + "/" + cfgGroups[1].Name),
			}))

			m2, err := GetHashMeta(ctx, project, m.Hash())
			assert.NoErr(t, err)
			assert.That(t, m2, should.Match(m))

			cgs, err := m.GetConfigGroups(ctx)
			assert.NoErr(t, err)
			assert.Loosely(t, len(cgs), should.Equal(2))
			assert.Loosely(t, cgs[0].Project.StringID(), should.Equal(project))
			assert.That(t, cgs[0].Content, should.Match(cfgGroups[0]))
			assert.Loosely(t, cgs[1].Project.StringID(), should.Equal(project))
			assert.That(t, cgs[1].Content, should.Match(cfgGroups[1]))
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
		assert.Loosely(t, mockUpdateProjectConfig(ctx, project, cfgGroups), should.BeNil)

		t.Run("Updated project", func(t *ftt.Test) {
			m, err := GetLatestMeta(ctx, project)
			assert.NoErr(t, err)
			assert.Loosely(t, m.Exists(), should.BeTrue)
			assert.Loosely(t, m.Status, should.Equal(StatusEnabled))
			h := m.Hash()
			assert.Loosely(t, h, should.HavePrefix("sha256:"))
			assert.That(t, m.ConfigGroupIDs, should.Match([]ConfigGroupID{
				ConfigGroupID(h + "/" + cfgGroups[0].Name),
				ConfigGroupID(h + "/" + cfgGroups[1].Name),
				ConfigGroupID(h + "/" + cfgGroups[2].Name),
			}))
			cgs, err := m.GetConfigGroups(ctx)
			assert.NoErr(t, err)
			assert.Loosely(t, len(cgs), should.Equal(3))

			t.Run("reading ConfigGroup directly works", func(t *ftt.Test) {
				cg, err := GetConfigGroup(ctx, project, m.ConfigGroupIDs[2])
				assert.NoErr(t, err)
				assert.That(t, cg.Content, should.Match(cfgGroups[2]))
			})
		})

		assert.Loosely(t, mockDisableProjectConfig(ctx, project, cfgGroups), should.BeNil)

		t.Run("Disabled project", func(t *ftt.Test) {
			m, err := GetLatestMeta(ctx, project)
			assert.NoErr(t, err)
			assert.Loosely(t, m.Exists(), should.BeTrue)
			assert.Loosely(t, m.Status, should.Equal(StatusDisabled))
			assert.Loosely(t, len(m.ConfigGroupIDs), should.Equal(3))
			cgs, err := m.GetConfigGroups(ctx)
			assert.NoErr(t, err)
			assert.Loosely(t, len(cgs), should.Equal(3))
		})

		// Re-enable the project.
		assert.Loosely(t, mockUpdateProjectConfig(ctx, project, cfgGroups), should.BeNil)

		t.Run("Re-enabled project", func(t *ftt.Test) {
			m, err := GetLatestMeta(ctx, project)
			assert.NoErr(t, err)
			assert.Loosely(t, m.Exists(), should.BeTrue)
			assert.Loosely(t, m.Status, should.Equal(StatusEnabled))
		})

		m, err := GetLatestMeta(ctx, project)
		assert.NoErr(t, err)
		cgs, err := m.GetConfigGroups(ctx)
		assert.NoErr(t, err)

		t.Run("Deleted project", func(t *ftt.Test) {
			assert.Loosely(t, datastore.Delete(ctx, &ProjectConfig{Project: project}, cgs), should.BeNil)

			m, err = GetLatestMeta(ctx, project)
			assert.NoErr(t, err)
			assert.Loosely(t, m.Exists(), should.BeFalse)
		})

		t.Run("reading partially deleted project", func(t *ftt.Test) {
			assert.Loosely(t, datastore.Delete(ctx, cgs[1]), should.BeNil)
			_, err = m.GetConfigGroups(ctx)
			assert.Loosely(t, err, should.ErrLike("ConfigGroups for"))
			assert.Loosely(t, err, should.ErrLike("not found"))
			assert.Loosely(t, datastore.IsErrNoSuchEntity(err), should.BeTrue)

			// Can still read individual ConfigGroups.
			cg, err := GetConfigGroup(ctx, project, m.ConfigGroupIDs[0])
			assert.NoErr(t, err)
			assert.That(t, cg.Content, should.Match(cfgGroups[0]))
			cg, err = GetConfigGroup(ctx, project, m.ConfigGroupIDs[2])
			assert.NoErr(t, err)
			assert.That(t, cg.Content, should.Match(cfgGroups[2]))
			// ... except the deleted one.
			_, err = GetConfigGroup(ctx, project, m.ConfigGroupIDs[1])
			assert.Loosely(t, datastore.IsErrNoSuchEntity(err), should.BeTrue)
		})
	})
}
