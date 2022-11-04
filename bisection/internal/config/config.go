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

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/config/server/cfgcache"
	"go.chromium.org/luci/config/validation"
)

// Cached service-level config
var cachedCfg = cfgcache.Register(&cfgcache.Entry{
	Path: serviceConfigFilename,
	Type: (*configpb.Config)(nil),
	Validator: func(ctx *validation.Context, msg proto.Message) error {
		validateConfig(ctx, msg.(*configpb.Config))
		return nil
	},
})

// Update is called from a cron periodically; it fetches the latest config
// from the LUCI Config service and caches it into the datastore
func Update(ctx context.Context) error {
	_, err := cachedCfg.Update(ctx, nil)
	return err
}

// Get returns the cached service-level config
func Get(ctx context.Context) (*configpb.Config, error) {
	cfg, err := cachedCfg.Get(ctx, nil)
	if err != nil {
		err = errors.Annotate(err, "failed to get cached config").Err()
		logging.Errorf(ctx, err.Error())
		return nil, err
	}

	return cfg.(*configpb.Config), nil
}
