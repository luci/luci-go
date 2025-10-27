// Copyright 2023 The LUCI Authors.
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

package permissionscfg

import (
	"context"
	"errors"

	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/server/cfgcache"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/auth_service/api/configspb"
)

var cachedPermissionsCfg = cfgcache.Register(&cfgcache.Entry{
	Path: "permissions.cfg",
	Type: (*configspb.PermissionsConfig)(nil),
})

// Get returns the config stored in the datastore or a default empty one.
func Get(ctx context.Context) (*configspb.PermissionsConfig, *config.Meta, error) {
	meta := &config.Meta{}
	cfg, err := cachedPermissionsCfg.Fetch(ctx, meta)
	if err != nil {
		if errors.Is(err, datastore.ErrNoSuchEntity) {
			return &configspb.PermissionsConfig{}, &config.Meta{}, nil
		}
		return nil, nil, err
	}
	return cfg.(*configspb.PermissionsConfig), meta, nil
}

// SetInTest replaces the config for tests.
func SetInTest(ctx context.Context, cfg *configspb.PermissionsConfig, meta *config.Meta) error {
	return cachedPermissionsCfg.Set(ctx, cfg, meta)
}

// Update fetches the config and puts it into the datastore.
func Update(ctx context.Context) (*config.Meta, error) {
	meta := &config.Meta{}
	_, err := cachedPermissionsCfg.Update(ctx, meta)
	return meta, err
}
