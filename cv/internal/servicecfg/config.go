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

// package servicecfg provides service-wide configs.
package servicecfg

import (
	"context"

	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/server/cfgcache"

	migrationpb "go.chromium.org/luci/cv/api/migration"
)

var cachedMigrationCfg = cfgcache.Register(&cfgcache.Entry{
	Path:      "migration-settings.cfg",
	ConfigSet: "services/commit-queue",
	Type:      (*migrationpb.Settings)(nil),
})

// ImportConfig is called from a cron to import and cache all the configs.
func ImportConfig(ctx context.Context) error {
	// TODO(tandrii): actually call this from cron handler once
	// https://crrev.com/c/2427189 lands.
	_, err := cachedMigrationCfg.Update(ctx, nil)
	return err
}

// GetMigrationConfig loads typically cached migration config.
func GetMigrationConfig(ctx context.Context) (*migrationpb.Settings, error) {
	v, err := cachedMigrationCfg.Get(ctx, &config.Meta{})
	return v.(*migrationpb.Settings), err
}

// SetTestMigrationConfig is used in tests only.
func SetTestMigrationConfig(ctx context.Context, m *migrationpb.Settings) error {
	return cachedMigrationCfg.Set(ctx, m, &config.Meta{})
}
