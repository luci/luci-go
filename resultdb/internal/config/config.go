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

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/server/cfgcache"
	"go.chromium.org/luci/config/validation"
	"go.chromium.org/luci/gae/service/datastore"

	configpb "go.chromium.org/luci/resultdb/proto/config"
)

const serviceConfigFilename = "config.cfg"

// Cached service-level config
var cachedServiceCfg = cfgcache.Register(&cfgcache.Entry{
	Path: serviceConfigFilename,
	Type: (*configpb.Config)(nil),
	Validator: func(ctx *validation.Context, msg proto.Message) error {
		validateServiceConfig(ctx, msg.(*configpb.Config))
		return nil
	},
})

// UpdateConfig is called from a cron periodically; it fetches the latest
// service-wide config and project config from the LUCI Config service
// and caches them into the datastore
func UpdateConfig(ctx context.Context) error {
	var errs []error
	err := UpdateProjects(ctx)
	if err != nil {
		errs = append(errs, errors.Fmt("update project configs: %w", err))
	}
	err = UpdateServiceConfig(ctx)
	if err != nil {
		errs = append(errs, errors.Fmt("update service configs: %w", err))
	}
	if len(errs) > 0 {
		return errors.NewMultiError(errs...)
	}
	return nil
}

// UpdateServiceConfig fetches the latest service config and caches it in datastore.
func UpdateServiceConfig(ctx context.Context) error {
	_, err := cachedServiceCfg.Update(ctx, nil)
	return err
}

// GetServiceConfig returns the cached service-level config
func GetServiceConfig(ctx context.Context) (*configpb.Config, error) {
	cfg, err := cachedServiceCfg.Get(ctx, nil)
	if err != nil {
		err = errors.Fmt("failed to get cached config: %w", err)
		logging.Errorf(ctx, "%s", err.Error())
		return nil, err
	}

	return cfg.(*configpb.Config), nil
}

// SetServiceConfigForTesting installs the service configuration into the context ctx.
// This is only used for the purpose of testing.
func SetServiceConfigForTesting(ctx context.Context, cfg *configpb.Config) error {
	return SetServiceConfigWithMetaForTesting(ctx, cfg, &config.Meta{})
}

// SetServiceConfigWithMetaForTesting installs the service configuration into the
// context ctx, with the given metadata.
// This is only used for the purpose of testing.
func SetServiceConfigWithMetaForTesting(ctx context.Context, cfg *configpb.Config, meta *config.Meta) error {
	testable := datastore.GetTestable(ctx)
	if testable == nil {
		return errors.New("SetServiceConfig should only be used with testable datastore implementations")
	}
	err := cachedServiceCfg.Set(ctx, cfg, meta)
	if err != nil {
		return err
	}
	testable.CatchupIndexes()
	return nil
}
