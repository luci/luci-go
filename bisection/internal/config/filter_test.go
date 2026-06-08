// Copyright 2026 The LUCI Authors.
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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"

	configpb "go.chromium.org/luci/bisection/proto/config"
)

func TestFilter(t *testing.T) {
	t.Parallel()

	ftt.Run("Test filter helper functions", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())

		t.Run("GetExcludedBuilderGroupsForCompile", func(t *ftt.Test) {
			t.Run("No config", func(t *ftt.Test) {
				res, err := GetExcludedBuilderGroupsForCompile(ctx, "chromium")
				assert.Loosely(t, err, should.ErrLike(ErrNotFoundProjectConfig))
				assert.Loosely(t, res, should.BeNil)
			})

			t.Run("With config", func(t *ftt.Test) {
				projectCfg := CreatePlaceholderProjectConfig()
				projectCfg.CompileAnalysisConfig.FailureIngestionFilter = &configpb.FailureIngestionFilter{
					ExcludedBuilderGroups: []string{"group1", "group2"},
				}
				configs := map[string]*configpb.ProjectConfig{
					"chromium": projectCfg,
				}
				assert.Loosely(t, SetTestProjectConfig(ctx, configs), should.BeNil)

				res, err := GetExcludedBuilderGroupsForCompile(ctx, "chromium")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res, should.Match([]string{"group1", "group2"}))
			})
		})

		t.Run("GetExcludedBuilderGroupsForTest", func(t *ftt.Test) {
			projectCfg := CreatePlaceholderProjectConfig()
			projectCfg.TestAnalysisConfig.FailureIngestionFilter = &configpb.FailureIngestionFilter{
				ExcludedBuilderGroups: []string{"group3"},
			}
			configs := map[string]*configpb.ProjectConfig{
				"chromium": projectCfg,
			}
			assert.Loosely(t, SetTestProjectConfig(ctx, configs), should.BeNil)

			res, err := GetExcludedBuilderGroupsForTest(ctx, "chromium")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Match([]string{"group3"}))
		})

		t.Run("GetExcludedBuildersForTest", func(t *ftt.Test) {
			projectCfg := CreatePlaceholderProjectConfig()
			projectCfg.TestAnalysisConfig.FailureIngestionFilter = &configpb.FailureIngestionFilter{
				ExcludedBuilders: []string{"builder1", "builder2"},
			}
			configs := map[string]*configpb.ProjectConfig{
				"chromium": projectCfg,
			}
			assert.Loosely(t, SetTestProjectConfig(ctx, configs), should.BeNil)

			res, err := GetExcludedBuildersForTest(ctx, "chromium")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Match([]string{"builder1", "builder2"}))
		})

		t.Run("GetAllowedBuildersForTest", func(t *ftt.Test) {
			projectCfg := CreatePlaceholderProjectConfig()
			projectCfg.TestAnalysisConfig.FailureIngestionFilter = &configpb.FailureIngestionFilter{
				AllowedBuilders: []string{"builder3"},
			}
			configs := map[string]*configpb.ProjectConfig{
				"chromium": projectCfg,
			}
			assert.Loosely(t, SetTestProjectConfig(ctx, configs), should.BeNil)

			res, err := GetAllowedBuildersForTest(ctx, "chromium")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Match([]string{"builder3"}))
		})
	})
}
