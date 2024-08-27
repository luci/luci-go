// Copyright 2024 The LUCI Authors.
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

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/gae/service/datastore"

	configpb "go.chromium.org/luci/tree_status/proto/config"
)

func TestConfig() *configpb.Config {
	return &configpb.Config{
		Trees: []*configpb.Tree{
			{
				Name:           "chromium",
				Projects:       []string{"chromium"},
				UseDefaultAcls: true,
			},
			{
				Name:           "v8",
				Projects:       []string{"v8", "v8-internal"},
				UseDefaultAcls: true,
			},
		},
	}
}

// SetConfig installs the config into the context ctx.
// This is only used for the purpose of testing.
func SetConfig(ctx context.Context, cfg *configpb.Config) error {
	testable := datastore.GetTestable(ctx)
	if testable == nil {
		return errors.New("SetConfig should only be used with testable datastore implementations")
	}
	err := cachedCfg.Set(ctx, cfg, &config.Meta{})
	if err != nil {
		return err
	}
	testable.CatchupIndexes()
	return nil
}
