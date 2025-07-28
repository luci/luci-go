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
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
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

	configpb "go.chromium.org/luci/analysis/proto/config"
)

var textPBMultiline = prototext.MarshalOptions{
	Multiline: true,
}

func TestProjectConfig(t *testing.T) {
	t.Parallel()

	t.Run("SetTestProjectConfig updates context config", func(t *testing.T) {
		projectA := CreateConfigWithBothBuganizerAndMonorail(configpb.BugSystem_MONORAIL)
		projectA.LastUpdated = timestamppb.New(time.Now())
		configs := make(map[string]*configpb.ProjectConfig)
		configs["a"] = projectA

		ctx := memory.Use(context.Background())
		SetTestProjectConfig(ctx, configs)

		cfg, err := Projects(ctx)

		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, len(cfg.Keys()), should.Equal(1))
		assert.Loosely(t, cfg.Project("a"), should.Match(projectA))
	})

	doTest := func(system configpb.BugSystem) func(t *ftt.Test) {
		return func(t *ftt.Test) {
			t.Helper()

			projectA := CreateConfigWithBothBuganizerAndMonorail(system)
			projectB := CreateConfigWithBothBuganizerAndMonorail(system)
			projectB.BugManagement.Monorail.PriorityFieldId = 1

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
				creationTime := clock.Now(ctx)
				err := updateProjects(ctx)
				assert.Loosely(t, err, should.BeNil)
				datastore.GetTestable(ctx).CatchupIndexes()

				// Get works.
				projects, err := Projects(ctx)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, len(projects.Keys()), should.Equal(2))
				assert.Loosely(t, projects.Project("a"), should.Match(withLastUpdated(projectA, creationTime)))
				assert.Loosely(t, projects.Project("b"), should.Match(withLastUpdated(projectB, creationTime)))

				tc.Add(1 * time.Second)

				// Noop update.
				err = updateProjects(ctx)
				assert.Loosely(t, err, should.BeNil)
				datastore.GetTestable(ctx).CatchupIndexes()

				tc.Add(1 * time.Second)

				// Real update.
				projectC := CreateConfigWithBothBuganizerAndMonorail(system)
				newProjectB := CreateConfigWithBothBuganizerAndMonorail(system)
				newProjectB.BugManagement.Monorail.PriorityFieldId = 2
				delete(configs, "projects/a")
				configs["projects/b"]["${appid}.cfg"] = textPBMultiline.Format(newProjectB)
				configs["projects/c"] = cfgmem.Files{
					"${appid}.cfg": textPBMultiline.Format(projectC),
				}
				newProjectA := &configpb.ProjectConfig{}
				updateTime := clock.Now(ctx)
				err = updateProjects(ctx)
				assert.Loosely(t, err, should.BeNil)
				datastore.GetTestable(ctx).CatchupIndexes()

				// Fetch returns the new value right away.
				projects, err = fetchProjects(ctx)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, len(projects.Keys()), should.Equal(3))
				assert.Loosely(t, projects.Project("a"), should.Match(withLastUpdated(newProjectA, updateTime))) // Retained.
				assert.Loosely(t, projects.Project("b"), should.Match(withLastUpdated(newProjectB, updateTime)))
				assert.Loosely(t, projects.Project("c"), should.Match(withLastUpdated(projectC, updateTime)))

				// Get still uses in-memory cached copy.
				projects, err = Projects(ctx)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, len(projects.Keys()), should.Equal(2))
				assert.Loosely(t, projects.Project("a"), should.Match(withLastUpdated(projectA, creationTime))) // Retained.
				assert.Loosely(t, projects.Project("b"), should.Match(withLastUpdated(projectB, creationTime)))

				// Noop update again.
				err = updateProjects(ctx)
				assert.Loosely(t, err, should.BeNil)
				datastore.GetTestable(ctx).CatchupIndexes()

				// No change for all projects.
				projects, err = fetchProjects(ctx)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, len(projects.Keys()), should.Equal(3))
				assert.Loosely(t, projects.Project("a"), should.Match(withLastUpdated(newProjectA, updateTime))) // Retained.
				assert.Loosely(t, projects.Project("b"), should.Match(withLastUpdated(newProjectB, updateTime)))
				assert.Loosely(t, projects.Project("c"), should.Match(withLastUpdated(projectC, updateTime)))

				t.Run("Expedited cache eviction", func(t *ftt.Test) {
					projectB, err = ProjectWithMinimumVersion(ctx, "b", updateTime)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, projectB, should.Match(withLastUpdated(newProjectB, updateTime)))
				})
				t.Run("Natural cache eviction", func(t *ftt.Test) {
					// Time passes, in-memory cached copy expires.
					tc.Add(2 * time.Minute)

					// Get returns the new value now too.
					projects, err = Projects(ctx)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, len(projects.Keys()), should.Equal(3))
					assert.Loosely(t, projects.Project("a"), should.Match(withLastUpdated(newProjectA, updateTime))) // Retained.
					assert.Loosely(t, projects.Project("b"), should.Match(withLastUpdated(newProjectB, updateTime)))
					assert.Loosely(t, projects.Project("c"), should.Match(withLastUpdated(projectC, updateTime)))

					// Time passes, in-memory cached copy expires.
					tc.Add(2 * time.Minute)

					// Get returns the same value.
					projects, err = Projects(ctx)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, len(projects.Keys()), should.Equal(3))
					assert.Loosely(t, projects.Project("a"), should.Match(withLastUpdated(newProjectA, updateTime)))
					assert.Loosely(t, projects.Project("b"), should.Match(withLastUpdated(newProjectB, updateTime)))
					assert.Loosely(t, projects.Project("c"), should.Match(withLastUpdated(projectC, updateTime)))
				})

				t.Run("Add config for previously deleted project", func(t *ftt.Test) {
					tc.Add(2 * time.Second)
					configs["projects/a"] = cfgmem.Files{
						"${appid}.cfg": textPBMultiline.Format(projectA),
					}
					newUpdateTime := clock.Now(ctx)
					err = updateProjects(ctx)
					assert.Loosely(t, err, should.BeNil)
					datastore.GetTestable(ctx).CatchupIndexes()

					// Fetch returns the new value right away.
					projects, err = fetchProjects(ctx)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, len(projects.Keys()), should.Equal(3))
					assert.Loosely(t, projects.Project("a"), should.Match(withLastUpdated(projectA, newUpdateTime)))
					assert.Loosely(t, projects.Project("b"), should.Match(withLastUpdated(newProjectB, updateTime)))
					assert.Loosely(t, projects.Project("c"), should.Match(withLastUpdated(projectC, updateTime)))
				})
			})

			t.Run("Validation works", func(t *ftt.Test) {
				configs["projects/b"]["${appid}.cfg"] = `bad data`
				creationTime := clock.Now(ctx)
				err := updateProjects(ctx)
				datastore.GetTestable(ctx).CatchupIndexes()
				assert.Loosely(t, err, should.ErrLike("validation errors"))

				// Validation for project A passed and project is
				// available, validation for project B failed
				// as is not available.
				projects, err := Projects(ctx)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, len(projects.Keys()), should.Equal(1))
				assert.Loosely(t, projects.Project("a"), should.Match(withLastUpdated(projectA, creationTime)))
			})

			t.Run("Update retains existing config if new config is invalid", func(t *ftt.Test) {
				// Initial update.
				creationTime := clock.Now(ctx)
				err := updateProjects(ctx)
				assert.Loosely(t, err, should.BeNil)
				datastore.GetTestable(ctx).CatchupIndexes()

				// Get works.
				projects, err := Projects(ctx)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, len(projects.Keys()), should.Equal(2))
				assert.Loosely(t, projects.Project("a"), should.Match(withLastUpdated(projectA, creationTime)))
				assert.Loosely(t, projects.Project("b"), should.Match(withLastUpdated(projectB, creationTime)))

				tc.Add(1 * time.Second)

				// Attempt to update with an invalid config for project B.
				newProjectA := CreateConfigWithBothBuganizerAndMonorail(system)
				newProjectA.BugManagement.Monorail.Project = "new-project-a"
				newProjectB := CreateConfigWithBothBuganizerAndMonorail(system)
				newProjectB.BugManagement.Monorail.Project = ""
				configs["projects/a"]["${appid}.cfg"] = textPBMultiline.Format(newProjectA)
				configs["projects/b"]["${appid}.cfg"] = textPBMultiline.Format(newProjectB)
				updateTime := clock.Now(ctx)
				err = updateProjects(ctx)
				assert.Loosely(t, err, should.ErrLike("validation errors"))
				datastore.GetTestable(ctx).CatchupIndexes()

				// Time passes, in-memory cached copy expires.
				tc.Add(2 * time.Minute)

				// Get returns the new configuration A and the old
				// configuration for B. This ensures an attempt to push an invalid
				// config does not result in a service outage for that project.
				projects, err = Projects(ctx)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, len(projects.Keys()), should.Equal(2))
				assert.Loosely(t, projects.Project("a"), should.Match(withLastUpdated(newProjectA, updateTime)))
				assert.Loosely(t, projects.Project("b"), should.Match(withLastUpdated(projectB, creationTime)))
			})
		}
	}

	ftt.Run("monorail", t, doTest(configpb.BugSystem_MONORAIL))
	ftt.Run("buganizer", t, doTest(configpb.BugSystem_BUGANIZER))
}

// withLastUpdated returns a copy of the given ProjectConfig with the
// specified LastUpdated time set.
func withLastUpdated(cfg *configpb.ProjectConfig, lastUpdated time.Time) *configpb.ProjectConfig {
	result := proto.Clone(cfg).(*configpb.ProjectConfig)
	result.LastUpdated = timestamppb.New(lastUpdated)
	return result
}

func TestProject(t *testing.T) {
	t.Parallel()

	doTest := func(system configpb.BugSystem) func(t *ftt.Test) {
		return func(t *ftt.Test) {
			pjChromium := CreateConfigWithBothBuganizerAndMonorail(system)
			configs := map[string]*configpb.ProjectConfig{
				"chromium": pjChromium,
			}

			ctx := memory.Use(context.Background())
			SetTestProjectConfig(ctx, configs)

			t.Run("success", func(t *ftt.Test) {
				pj, err := Project(ctx, "chromium")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, pj, should.Match(pjChromium))
			})

			t.Run("not found", func(t *ftt.Test) {
				pj, err := Project(ctx, "random")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, pj, should.Match(&configpb.ProjectConfig{LastUpdated: timestamppb.New(StartingEpoch)}))
			})
		}
	}

	ftt.Run("monorail", t, doTest(configpb.BugSystem_MONORAIL))
	ftt.Run("buganizer", t, doTest(configpb.BugSystem_BUGANIZER))
}
