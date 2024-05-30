// Copyright 2023 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package testsupport

import (
	"context"
	"fmt"

	"go.chromium.org/luci/config"
)

const (
	// The dev service associated with Auth Service aka Chrome Infra Auth,
	// to get its own configs.
	criaDev = "services/chrome-infra-auth-dev"

	// Path to use within a project or service's folder when looking
	// for realms configs.
	realmsDevCfgPath = "realms-dev.cfg"

	TestRevision    = "test-revision"
	TestContentHash = "test-hash"
)

var projectsWithCfg = map[string][]string{
	realmsDevCfgPath: {
		"test-project-a",
		"test-project-d",
		criaDev,
	},
}

func testMeta(configSet config.Set) config.Meta {
	return config.Meta{
		ConfigSet:   configSet,
		Path:        realmsDevCfgPath,
		ContentHash: TestContentHash,
		Revision:    TestRevision,
	}
}

type FakeConfigClient struct {
	config.Interface
}

func (*FakeConfigClient) GetConfig(ctx context.Context, configSet config.Set, path string, metaOnly bool) (*config.Config, error) {
	return &config.Config{Meta: testMeta(configSet)}, nil
}

func (*FakeConfigClient) GetProjectConfigs(ctx context.Context, path string, metaOnly bool) ([]config.Config, error) {
	var configsToReturn []config.Config
	if path == realmsDevCfgPath {
		for _, project := range projectsWithCfg[realmsDevCfgPath] {
			conf := config.Config{
				Meta: config.Meta{
					ConfigSet:   config.Set(fmt.Sprintf("projects/%s", project)),
					Path:        path,
					ContentHash: TestContentHash,
				},
			}
			if project == criaDev {
				conf.Meta.ConfigSet = criaDev
			}
			configsToReturn = append(configsToReturn, conf)
		}
	}
	return configsToReturn, nil
}

func (*FakeConfigClient) Close() error {
	return nil
}

func (f *FakeConfigClient) GetExpectedConfigsForTest(ctx context.Context) map[string]*config.Config {
	projectsWithRealms := projectsWithCfg[realmsDevCfgPath]
	expected := make(map[string]*config.Config, len(projectsWithRealms))
	for _, projectID := range projectsWithRealms {
		projectKey := projectID
		configSet := config.Set(fmt.Sprintf("projects/%s", projectID))
		if projectID == criaDev {
			projectKey = "@internal"
			configSet = config.Set(criaDev)
		}
		projectConfig, _ := f.GetConfig(ctx, configSet, realmsDevCfgPath, false)
		expected[projectKey] = projectConfig
	}

	return expected
}
