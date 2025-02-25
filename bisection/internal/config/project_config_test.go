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

package config

import (
	"context"
	"testing"
	"time"

	"google.golang.org/protobuf/encoding/prototext"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/cfgclient"
	cfgmem "go.chromium.org/luci/config/impl/memory"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/caching"

	configpb "go.chromium.org/luci/bisection/proto/config"
)

var textPBMultiline = prototext.MarshalOptions{
	Multiline: true,
}

func TestProjectConfig(t *testing.T) {
	t.Parallel()

	ftt.Run("SetTestProjectConfig updates context config", t, func(t *ftt.Test) {
		projectA := CreatePlaceholderProjectConfig()
		configs := make(map[string]*configpb.ProjectConfig)
		configs["a"] = projectA

		ctx := memory.Use(context.Background())
		assert.Loosely(t, SetTestProjectConfig(ctx, configs), should.BeNil)

		cfg, err := Projects(ctx)

		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, len(cfg), should.Equal(1))
		assert.Loosely(t, cfg["a"], should.Match(projectA))
	})

	ftt.Run("With mocks", t, func(t *ftt.Test) {
		projectA := CreatePlaceholderProjectConfig()
		projectB := CreatePlaceholderProjectConfig()
		projectB.TestAnalysisConfig.FailureIngestionFilter = &configpb.FailureIngestionFilter{
			ExcludedBuckets: []string{"try"},
		}

		configs := map[config.Set]cfgmem.Files{
			"projects/a": {"${appid}.cfg": textPBMultiline.Format(projectA)},
			"projects/b": {"${appid}.cfg": textPBMultiline.Format(projectB)},
		}

		ctx := memory.Use(context.Background())
		ctx, tc := testclock.UseTime(ctx, testclock.TestTimeUTC)
		ctx = cfgclient.Use(ctx, cfgmem.New(configs))
		ctx = caching.WithEmptyProcessCache(ctx)

		t.Run("Update works", func(t *ftt.Test) {
			// Initial update.
			err := UpdateProjects(ctx)
			assert.Loosely(t, err, should.BeNil)
			datastore.GetTestable(ctx).CatchupIndexes()

			// Get works.
			projects, err := Projects(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(projects), should.Equal(2))
			assert.Loosely(t, projects["a"], should.Match(projectA))
			assert.Loosely(t, projects["b"], should.Match(projectB))

			// Noop update.
			err = UpdateProjects(ctx)
			assert.Loosely(t, err, should.BeNil)
			datastore.GetTestable(ctx).CatchupIndexes()

			// Real update.
			projectC := CreatePlaceholderProjectConfig()
			newProjectB := CreatePlaceholderProjectConfig()
			newProjectB.TestAnalysisConfig.FailureIngestionFilter = &configpb.FailureIngestionFilter{
				ExcludedBuckets: []string{"try2"},
			}
			delete(configs, "projects/a")
			configs["projects/b"]["${appid}.cfg"] = textPBMultiline.Format(newProjectB)
			configs["projects/c"] = cfgmem.Files{
				"${appid}.cfg": textPBMultiline.Format(projectC),
			}
			err = UpdateProjects(ctx)
			assert.Loosely(t, err, should.BeNil)
			datastore.GetTestable(ctx).CatchupIndexes()

			// Fetch returns the new value right away.
			projects, err = fetchProjects(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(projects), should.Equal(2))
			assert.Loosely(t, projects["b"], should.Match(newProjectB))
			assert.Loosely(t, projects["c"], should.Match(projectC))

			// Get still uses in-memory cached copy.
			projects, err = Projects(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(projects), should.Equal(2))
			assert.Loosely(t, projects["a"], should.Match(projectA))
			assert.Loosely(t, projects["b"], should.Match(projectB))

			// Time passes, in-memory cached copy expires.
			tc.Add(2 * time.Minute)

			// Get returns the new value now too.
			projects, err = Projects(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(projects), should.Equal(2))
			assert.Loosely(t, projects["b"], should.Match(newProjectB))
			assert.Loosely(t, projects["c"], should.Match(projectC))

			// Time passes, in-memory cached copy expires.
			tc.Add(2 * time.Minute)

			// Get returns the same value.
			projects, err = Projects(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(projects), should.Equal(2))
			assert.Loosely(t, projects["b"], should.Match(newProjectB))
			assert.Loosely(t, projects["c"], should.Match(projectC))

			// Update with milestone project should be ignored.
			milestoneProject := CreatePlaceholderProjectConfig()
			configs["projects/chromium-m110"] = cfgmem.Files{
				"${appid}.cfg": textPBMultiline.Format(milestoneProject),
			}
			err = UpdateProjects(ctx)
			assert.Loosely(t, err, should.BeNil)
			datastore.GetTestable(ctx).CatchupIndexes()

			// Fetch only returns 2 project
			projects, err = fetchProjects(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(projects), should.Equal(2))
			assert.Loosely(t, projects["b"], should.Match(newProjectB))
			assert.Loosely(t, projects["c"], should.Match(projectC))

			// Check supported projects.
			projectNames, err := SupportedProjects(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, projectNames, should.Match([]string{"b", "c"}))
		})

		t.Run("Validation works", func(t *ftt.Test) {
			configs["projects/b"]["${appid}.cfg"] = `bad data`
			err := UpdateProjects(ctx)
			datastore.GetTestable(ctx).CatchupIndexes()
			assert.Loosely(t, err, should.ErrLike("validation errors"))

			// Validation for project A passed and project is
			// available, validation for project B failed
			// as is not available.
			projects, err := Projects(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(projects), should.Equal(1))
			assert.Loosely(t, projects["a"], should.Match(projectA))
		})

		t.Run("Update retains existing config if new config is invalid", func(t *ftt.Test) {
			// Initial update.
			err := UpdateProjects(ctx)
			assert.Loosely(t, err, should.BeNil)
			datastore.GetTestable(ctx).CatchupIndexes()

			// Get works.
			projects, err := Projects(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(projects), should.Equal(2))
			assert.Loosely(t, projects["a"], should.Match(projectA))
			assert.Loosely(t, projects["b"], should.Match(projectB))

			// Attempt to update with an invalid config for project B.
			newProjectA := CreatePlaceholderProjectConfig()
			assert.Loosely(t, newProjectA.GetTestAnalysisConfig().GerritConfig.ActionsEnabled, should.BeTrue)
			newProjectA.GetTestAnalysisConfig().GerritConfig.ActionsEnabled = false
			newProjectB := CreatePlaceholderProjectConfig()
			assert.Loosely(t, newProjectB.GetTestAnalysisConfig().GerritConfig.MaxRevertibleCulpritAge, should.Equal(1))
			newProjectB.GetTestAnalysisConfig().GerritConfig.MaxRevertibleCulpritAge = 0
			configs["projects/a"]["${appid}.cfg"] = textPBMultiline.Format(newProjectA)
			configs["projects/b"]["${appid}.cfg"] = textPBMultiline.Format(newProjectB)
			err = UpdateProjects(ctx)
			assert.Loosely(t, err, should.ErrLike("validation errors"))
			datastore.GetTestable(ctx).CatchupIndexes()

			// Time passes, in-memory cached copy expires.
			tc.Add(2 * time.Minute)

			// Get returns the new configuration A and the old
			// configuration for B. This ensures an attempt to push an invalid
			// config does not result in a service outage for that project.
			projects, err = Projects(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(projects), should.Equal(2))
			assert.Loosely(t, projects["a"], should.Match(newProjectA))
			assert.Loosely(t, projects["b"], should.Match(projectB))
		})
	})
}

func TestProject(t *testing.T) {
	t.Parallel()

	ftt.Run("Project", t, func(t *ftt.Test) {
		pjChromium := CreatePlaceholderProjectConfig()
		configs := map[string]*configpb.ProjectConfig{
			"chromium": pjChromium,
		}

		ctx := memory.Use(context.Background())
		assert.Loosely(t, SetTestProjectConfig(ctx, configs), should.BeNil)

		t.Run("success", func(t *ftt.Test) {
			pj, err := Project(ctx, "chromium")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, pj, should.Match(pjChromium))
		})

		t.Run("not found", func(t *ftt.Test) {
			pj, err := Project(ctx, "random")
			assert.Loosely(t, err, should.ErrLike(ErrNotFoundProjectConfig))
			assert.Loosely(t, pj, should.BeNil)
		})
	})
}
