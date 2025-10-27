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

package allowlistcfg

import (
	"context"
	"errors"

	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/server/cfgcache"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/auth_service/api/configspb"
)

var cachedAllowlistCfg = cfgcache.Register(&cfgcache.Entry{
	Path: "ip_allowlist.cfg",
	Type: (*configspb.IPAllowlistConfig)(nil),
})

// Get returns the config stored in the datastore or a default empty one.
func Get(ctx context.Context) (*configspb.IPAllowlistConfig, *config.Meta, error) {
	meta := &config.Meta{}
	cfg, err := cachedAllowlistCfg.Fetch(ctx, meta)
	if err != nil {
		if errors.Is(err, datastore.ErrNoSuchEntity) {
			return &configspb.IPAllowlistConfig{}, &config.Meta{}, nil
		}
		return nil, nil, err
	}
	return cfg.(*configspb.IPAllowlistConfig), meta, nil
}

// SetInTest replaces the config for tests.
func SetInTest(ctx context.Context, cfg *configspb.IPAllowlistConfig, meta *config.Meta) error {
	return cachedAllowlistCfg.Set(ctx, cfg, meta)
}

// Update fetches the config and puts it into the datastore.
func Update(ctx context.Context) (*config.Meta, error) {
	meta := &config.Meta{}
	_, err := cachedAllowlistCfg.Update(ctx, meta)
	return meta, err
}
