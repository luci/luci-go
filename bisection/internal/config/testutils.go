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

	configpb "go.chromium.org/luci/bisection/proto/config"

	"google.golang.org/protobuf/encoding/prototext"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/config"
)

const sampleConfigFileContents = `
	gerrit_config {
		actions_enabled: false

		create_revert_settings: {
			enabled: false
			daily_limit: 0
		}

		submit_revert_settings: {
			enabled: false
			daily_limit: 0
		}

		max_revertible_culprit_age: 123
	}
`

// CreateTestConfig returns a new valid configpb.Config for tests
func CreateTestConfig() (*configpb.Config, error) {
	var cfg configpb.Config
	err := prototext.Unmarshal([]byte(sampleConfigFileContents), &cfg)
	if err != nil {
		return nil, errors.Annotate(err, "unmarshaling a test config").Err()
	}

	return &cfg, nil
}

// SetTestConfig sets the cached service-level config;
// this should only be used for setting the config for tests
func SetTestConfig(ctx context.Context, cfg *configpb.Config) error {
	return cachedCfg.Set(ctx, cfg, &config.Meta{})
}
