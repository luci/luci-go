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

package securitycfg

import (
	"context"
	"errors"

	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/server/cfgcache"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth/service/protocol"
)

var cachedSecurityCfg = cfgcache.Register(&cfgcache.Entry{
	Path: "security.cfg",
	Type: (*protocol.SecurityConfig)(nil),
})

// Get returns the config stored in the datastore or a default empty one.
func Get(ctx context.Context) (*protocol.SecurityConfig, *config.Meta, error) {
	meta := &config.Meta{}
	cfg, err := cachedSecurityCfg.Fetch(ctx, meta)
	if err != nil {
		if errors.Is(err, datastore.ErrNoSuchEntity) {
			return &protocol.SecurityConfig{}, &config.Meta{}, nil
		}
		return nil, nil, err
	}
	return cfg.(*protocol.SecurityConfig), meta, nil
}

// SetInTest replaces the config for tests.
func SetInTest(ctx context.Context, cfg *protocol.SecurityConfig, meta *config.Meta) error {
	return cachedSecurityCfg.Set(ctx, cfg, meta)
}

// Update fetches the config and puts it into the datastore.
func Update(ctx context.Context) (*config.Meta, error) {
	meta := &config.Meta{}
	_, err := cachedSecurityCfg.Update(ctx, meta)
	return meta, err
}
