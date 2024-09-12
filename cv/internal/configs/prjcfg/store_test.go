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
	"strings"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	gaememory "go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
)

func TestComputeHash(t *testing.T) {
	t.Parallel()
	testCfg := &cfgpb.Config{
		CqStatusHost: "chromium-cq-status.appspot.com",
		ConfigGroups: []*cfgpb.ConfigGroup{
			{
				Name: "group_foo",
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
		},
	}

	ftt.Run("Compute Hash", t, func(t *ftt.Test) {
		tokens := strings.Split(ComputeHash(testCfg), ":")
		assert.Loosely(t, tokens, should.HaveLength(2))
		assert.Loosely(t, tokens[0], should.Equal("sha256"))
		assert.Loosely(t, tokens[1], should.HaveLength(16))
	})
}

func TestGetAllProjectIDs(t *testing.T) {
	t.Parallel()
	ftt.Run("Get Project IDs", t, func(t *ftt.Test) {
		ctx := gaememory.Use(context.Background())
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)
		enabledPC := ProjectConfig{
			Project: "enabledProject",
			Enabled: true,
		}
		disabledPC := ProjectConfig{
			Project: "disabledProject",
			Enabled: false,
		}
		err := datastore.Put(ctx, &enabledPC, &disabledPC)
		assert.Loosely(t, err, should.BeNil)

		t.Run("All", func(t *ftt.Test) {
			ret, err := GetAllProjectIDs(ctx, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ret, should.Resemble([]string{"disabledProject", "enabledProject"}))
		})

		t.Run("Enabled", func(t *ftt.Test) {
			ret, err := GetAllProjectIDs(ctx, true)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ret, should.Resemble([]string{"enabledProject"}))
		})
	})
}

func TestMakeConfigGroupID(t *testing.T) {
	t.Parallel()
	ftt.Run("Make ConfigGroupID", t, func(t *ftt.Test) {
		id := MakeConfigGroupID("sha256:deadbeefdeadbeef", "foo")
		assert.Loosely(t, id, should.Equal(ConfigGroupID("sha256:deadbeefdeadbeef/foo")))
	})
}

func TestConfigGroupProjectString(t *testing.T) {
	t.Parallel()

	ftt.Run("ConfigGroup.ProjectString works", t, func(t *ftt.Test) {
		ctx := gaememory.Use(context.Background())
		c := ConfigGroup{
			Project: ProjectConfigKey(ctx, "chromium"),
		}
		assert.Loosely(t, c.ProjectString(), should.Equal("chromium"))
	})
}

func TestGetAllGerritHosts(t *testing.T) {
	t.Parallel()

	ftt.Run("GetAllGerritHosts", t, func(t *ftt.Test) {
		ctx := gaememory.Use(context.Background())
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)

		pc1 := &ProjectConfig{
			Project: "enabledProject",
			Hash:    "sha256:deadbeef",
			Enabled: true,
		}
		pc2 := &ProjectConfig{
			Project: "disabledProject",
			Hash:    "sha256:dddeadbbbbef",
			Enabled: true,
		}
		assert.Loosely(t, datastore.Put(ctx, pc1, pc2), should.BeNil)

		addCG := func(pc *ProjectConfig, cgName string, hosts ...string) error {
			cpb := &cfgpb.ConfigGroup{Name: cgName}
			for _, host := range hosts {
				cpb.Gerrit = append(cpb.Gerrit, &cfgpb.ConfigGroup_Gerrit{
					Url: "https://" + host,
					Projects: []*cfgpb.ConfigGroup_Gerrit_Project{
						{Name: "cr/src1"},
						{Name: "cr/src2"},
					},
				})
			}
			cg := &ConfigGroup{
				Project: ProjectConfigKey(ctx, pc.Project),
				ID:      MakeConfigGroupID(pc.Hash, cgName),
				Content: cpb,
			}
			pc.ConfigGroupNames = append(pc.ConfigGroupNames, cgName)
			return datastore.Put(ctx, pc, cg)
		}

		t.Run("works", func(t *ftt.Test) {
			assert.Loosely(t, addCG(pc1, "main", "example.com", "example.org"), should.BeNil)
			assert.Loosely(t, addCG(pc2, "main", "example.edu", "example.net"), should.BeNil)

			hosts, err := GetAllGerritHosts(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, hosts[pc1.Project].ToSortedSlice(), should.Resemble(
				[]string{"example.com", "example.org"}))
			assert.Loosely(t, hosts[pc2.Project].ToSortedSlice(), should.Resemble(
				[]string{"example.edu", "example.net"}))
		})

		t.Run("doesn't include disabled projects", func(t *ftt.Test) {
			pc2.Enabled = false
			assert.Loosely(t, datastore.Put(ctx, pc2), should.BeNil)
			hosts, err := GetAllGerritHosts(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, hosts, should.NotContainKey(pc2.Project))
		})
	})
}
