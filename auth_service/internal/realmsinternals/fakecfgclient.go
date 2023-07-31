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

package realmsinternals

import (
	"context"
	"fmt"

	"go.chromium.org/luci/config"
)

var testRevision = "test-revision"
var testContentHash = "test-hash"

var projectsWithCfg = map[string][]string{
	RealmsDevCfgPath: {
		"test-project-a",
		"test-project-d",
		CriaDev,
	},
}

func testMeta(configSet config.Set) config.Meta {
	return config.Meta{
		ConfigSet:   configSet,
		Path:        RealmsDevCfgPath,
		ContentHash: testContentHash,
		Revision:    testRevision,
	}
}

type fakeCfgClient struct {
	config.Interface
}

func (*fakeCfgClient) GetConfig(ctx context.Context, configSet config.Set, path string, metaOnly bool) (*config.Config, error) {
	return &config.Config{Meta: testMeta(configSet)}, nil
}

func (*fakeCfgClient) GetProjectConfigs(ctx context.Context, path string, metaOnly bool) ([]config.Config, error) {
	var configsToReturn []config.Config
	if path == RealmsDevCfgPath {
		for _, project := range projectsWithCfg[RealmsDevCfgPath] {
			conf := config.Config{
				Meta: config.Meta{
					ConfigSet:   config.Set(fmt.Sprintf("projects/%s", project)),
					Path:        path,
					ContentHash: testContentHash,
				},
			}
			if project == CriaDev {
				conf.Meta.ConfigSet = CriaDev
			}
			configsToReturn = append(configsToReturn, conf)
		}
	}
	return configsToReturn, nil
}

func (*fakeCfgClient) Close() error {
	return nil
}
