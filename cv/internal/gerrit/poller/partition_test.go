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

package poller

import (
	"sort"
	"testing"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/configs/prjcfg"
)

func TestPartitionConfig(t *testing.T) {
	t.Parallel()

	ftt.Run("partitionConfig works", t, func(t *ftt.Test) {

		t.Run("groups by prefix if possible", func(t *ftt.Test) {
			// makeCfgs merges several projects configs into one just to re-use
			// singleRepoConfig.
			makeCfgs := func(cfgs ...*cfgpb.Config) (ret []*prjcfg.ConfigGroup) {
				for _, cfg := range cfgs {
					for _, cg := range cfg.GetConfigGroups() {
						ret = append(ret, &prjcfg.ConfigGroup{Content: cg})
					}
				}
				return
			}
			cgs := makeCfgs(singleRepoConfig("h1", "infra/222", "infra/111"))
			assert.That(t, partitionConfig(cgs), should.Match([]*QueryState{
				{Host: "h1", OrProjects: []string{"infra/111", "infra/222"}},
			}))

			cgs = append(cgs, makeCfgs(singleRepoConfig("h1", sharedPrefixRepos("infra", 30)...))...)
			assert.That(t, partitionConfig(cgs), should.Match([]*QueryState{
				{Host: "h1", CommonProjectPrefix: "infra"},
			}))
			cgs = append(cgs, makeCfgs(singleRepoConfig("h2", "infra/499", "infra/132"))...)
			assert.That(t, partitionConfig(cgs), should.Match([]*QueryState{
				{Host: "h1", CommonProjectPrefix: "infra"},
				{Host: "h2", OrProjects: []string{"infra/132", "infra/499"}},
			}))
		})

		t.Run("evenly distributes repos among queries", func(t *ftt.Test) {
			assert.Loosely(t, minReposPerPrefixQuery, should.BeGreaterThan(5))
			repos := stringset.New(23)
			repos.AddAll(sharedPrefixRepos("a", 5))
			repos.AddAll(sharedPrefixRepos("b", 5))
			repos.AddAll(sharedPrefixRepos("c", 3))
			repos.AddAll(sharedPrefixRepos("d", 5))
			repos.AddAll(sharedPrefixRepos("e", 5))
			queries := partitionHostRepos(
				"host",
				repos.ToSlice(), // effectively shuffles repos
				7,               // at most 7 per query.
			)
			assert.Loosely(t, queries, should.HaveLength(4)) // 7*3 < 23 < 7*4

			for _, qs := range queries {
				// Ensure each has 5..6 repos instead max of 7.
				assert.Loosely(t, len(qs.GetOrProjects()), should.BeBetweenOrEqual(5, 6))
				assert.Loosely(t, sort.StringsAreSorted(qs.GetOrProjects()), should.BeTrue)
				repos.DelAll(qs.GetOrProjects())
			}

			// Ensure no overlaps or missed repos.
			assert.That(t, repos.ToSortedSlice(), should.Match([]string{}))
		})
	})
}
