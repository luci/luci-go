// Copyright 2022 The LUCI Authors.
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

package listener

import (
	"context"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/configs/prjcfg/prjcfgtest"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/gerrit/gobmap"
	listenerpb "go.chromium.org/luci/cv/settings/listener"
)

func createTestLUCIProject(t testing.TB, ctx context.Context, prj, url, repo string) {
	t.Helper()

	prjcfgtest.Create(ctx, prj, &cfgpb.Config{
		ConfigGroups: []*cfgpb.ConfigGroup{
			{
				Name: "main",
				Gerrit: []*cfgpb.ConfigGroup_Gerrit{
					{
						Url: url,
						Projects: []*cfgpb.ConfigGroup_Gerrit_Project{
							{
								Name:      repo,
								RefRegexp: []string{"refs/heads/main"},
							},
						},
					},
				},
			},
		},
	})
	meta := prjcfgtest.MustExist(ctx, prj)
	cgs, err := meta.GetConfigGroups(ctx)
	assert.Loosely(t, err, should.BeNil, truth.LineContext())
	assert.Loosely(t, gobmap.Update(ctx, &meta, cgs), should.BeNil, truth.LineContext())
}

func TestProjectFinder(t *testing.T) {
	t.Parallel()

	ftt.Run("ProjectFinder", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)
		finder := &projectFinder{
			isListenerEnabled: func(string) bool { return true },
		}
		check := func(host, repo string) []string {
			prjs, err := finder.lookup(ctx, host, repo)
			assert.NoErr(t, err)
			return prjs
		}
		createTestLUCIProject(t, ctx, "chromium", "https://cr-review.gs.com/", "cr/src")
		createTestLUCIProject(t, ctx, "chromium-m123", "https://cr-review.gs.com", "cr/src")

		lCfg := &listenerpb.Settings{
			GerritSubscriptions: []*listenerpb.Settings_GerritSubscription{
				{Host: "cr-review.gs.com"},
			},
		}

		t.Run("with no disabled projects", func(t *ftt.Test) {
			assert.Loosely(t, finder.reload(lCfg), should.BeNil)
			assert.Loosely(t, check("cr-review.gs.com", "cr/src"), should.Match([]string{"chromium", "chromium-m123"}))
		})

		t.Run("with a disabled project", func(t *ftt.Test) {
			lCfg.DisabledProjectRegexps = []string{"chromium"}
			assert.Loosely(t, finder.reload(lCfg), should.BeNil)
			assert.Loosely(t, check("cr-review.gs.com", "cr/src"), should.Match([]string{"chromium-m123"}))
		})
	})
}
