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

package bootstrap

import (
	"context"
	"strings"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/config"
	cfgcacheLegacy "go.chromium.org/luci/config/server/cfgcache"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/cfgcache"
	"go.chromium.org/luci/server/cfgcache/dsstore"

	api "go.chromium.org/luci/cipd/api/config/v1"
)

var cachedCfg = (&cfgcache.PassthroughServicePipeline[*api.BootstrapConfigFile]{
	ID:    "svc-bootstrap",
	Store: &dsstore.Store[*api.BootstrapConfigFile]{},
	Files: []cfgcache.File{
		&cfgcache.SingleFile[*api.BootstrapConfigFile]{Path: "bootstrap.cfg"},
	},
})

type Config struct {
	handle *cfgcache.RegisteredService[*api.BootstrapConfigFile]
}

func RegisterConfig(reg *cfgcache.Registry) *Config {
	return &Config{cachedCfg.Register(reg)}
}

// Get returns a matching bootstrap package config or nil.
func (c *Config) Get(ctx context.Context, pkg string) (*api.BootstrapConfig, error) {
	cfg, err := c.handle.GetData(ctx)
	if err != nil {
		return nil, err
	}
	for _, bc := range cfg.GetBootstrapConfig() {
		if matchesConfig(pkg, bc) {
			return bc, nil
		}
	}
	return nil, nil
}

var cachedCfgLegacy = cfgcacheLegacy.Register(&cfgcacheLegacy.Entry{
	Path: "bootstrap.cfg",
	Type: (*api.BootstrapConfigFile)(nil),
})

// ImportConfig is called from a cron to import bootstrap.cfg into datastore.
func ImportConfig(ctx context.Context) error {
	_, err := cachedCfgLegacy.Update(ctx, nil)
	if errors.Unwrap(err) == config.ErrNoConfig {
		logging.Warningf(ctx, "No bootstrap.cfg config file")
		return nil
	}
	return err
}

// BootstrapConfig returns a matching bootstrap package config or nil.
func BootstrapConfig(ctx context.Context, pkg string) (*api.BootstrapConfig, error) {
	cfg, err := cachedCfgLegacy.Get(ctx, nil)
	if err != nil {
		if errors.Contains(err, datastore.ErrNoSuchEntity) {
			return nil, nil
		}
		return nil, transient.Tag.Apply(errors.Fmt("failed to fetch the bootstrap config: %w", err))
	}
	for _, bc := range cfg.(*api.BootstrapConfigFile).BootstrapConfig {
		if matchesConfig(pkg, bc) {
			return bc, nil
		}
	}
	return nil, nil
}

// matchesConfig returns true if the given `bc` applies to the package.
func matchesConfig(pkg string, bc *api.BootstrapConfig) bool {
	return bc.Prefix == pkg || strings.HasPrefix(pkg, bc.Prefix+"/")
}
