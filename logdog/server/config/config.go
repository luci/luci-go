// Copyright 2015 The LUCI Authors.
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
	"time"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/server/cfgclient"
	"go.chromium.org/luci/config/server/cfgclient/textproto"

	"go.chromium.org/luci/logdog/api/config/svcconfig"
)

var (
	// ErrInvalidConfig is returned when the configuration exists, but is invalid.
	ErrInvalidConfig = errors.New("invalid configuration")
)

// Config loads and returns the service configuration.
//
// The service config is minimally validated prior to being returned.
func Config(ctx context.Context) (*svcconfig.Config, error) {
	store := store(ctx)
	if store.NoCache {
		return fetchServiceConfig(ctx, store.serviceID)
	}
	cached, err := store.service.Get(ctx, func(prev interface{}) (val interface{}, exp time.Duration, err error) {
		logging.Infof(ctx, "Cache miss for services.cfg, fetching it...")
		cfg, err := fetchServiceConfig(ctx, store.serviceID)
		return cfg, 5 * time.Minute, err
	})
	if err != nil {
		return nil, err
	}
	return cached.(*svcconfig.Config), nil
}

// fetchServiceConfig fetches the service config from the storage.
func fetchServiceConfig(ctx context.Context, serviceID string) (*svcconfig.Config, error) {
	var cfg svcconfig.Config
	err := cfgclient.Get(
		ctx,
		cfgclient.AsService,
		config.ServiceSet(serviceID),
		"services.cfg",
		textproto.Message(&cfg),
		nil,
	)
	if err != nil {
		logging.Errorf(ctx, "Failed to load configuration from config service: %s", err)
		return nil, err
	}
	if err := validateServiceConfig(&cfg); err != nil {
		logging.Errorf(ctx, "Invalid Coordinator configuration: %s", err)
		return nil, ErrInvalidConfig
	}
	return &cfg, nil
}

// validateServiceConfig checks the supplied service config object to ensure
// that it meets a minimum configuration standard expected by our endpoitns and
// handlers.
func validateServiceConfig(cc *svcconfig.Config) error {
	switch {
	case cc == nil:
		return errors.New("configuration is nil")
	case cc.GetCoordinator() == nil:
		return errors.New("no Coordinator configuration")
	default:
		return nil
	}
}
