// Copyright 2021 The LUCI Authors.
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

	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/server/cfgcache"

	pb "go.chromium.org/luci/buildbucket/proto"
)

// Cached settings config.
var cachedSettingsCfg = cfgcache.Register(&cfgcache.Entry{
	Path: "settings.cfg",
	Type: (*pb.SettingsCfg)(nil),
})

// UpdateSettingsCfg is called from a cron periodically to import settings.cfg into datastore.
func UpdateSettingsCfg(ctx context.Context) error {
	_, err := cachedSettingsCfg.Update(ctx, nil)
	return err
}

// GetSettingsCfg fetches the settings.cfg from luci-config.
func GetSettingsCfg(ctx context.Context) (*pb.SettingsCfg, error) {
	cfg, err := cachedSettingsCfg.Get(ctx, nil)
	if err != nil {
		return nil, err
	}
	return cfg.(*pb.SettingsCfg), nil
}

// SetTestSettingsCfg is used in tests only.
func SetTestSettingsCfg(ctx context.Context, cfg *pb.SettingsCfg) error {
	return cachedSettingsCfg.Set(ctx, cfg, &config.Meta{Path: "settings.cfg"})
}
