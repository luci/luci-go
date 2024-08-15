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

	"go.chromium.org/luci/config/server/cfgcache"
	"go.chromium.org/luci/config/validation"

	configpb "go.chromium.org/luci/source_index/proto/config"
)

// Cached service config.
var cachedCfg = cfgcache.Register(&cfgcache.Entry{
	Path: "config.cfg",
	Type: (*configpb.Config)(nil),
	Validator: func(ctx *validation.Context, msg proto.Message) error {
		validateConfig(ctx, msg.(*configpb.Config))
		return nil
	},
})

// Update fetches the config and puts it into the datastore.
func Update(ctx context.Context) error {
	_, err := cachedCfg.Update(ctx, nil)
	return err
}

// Get returns the config stored in the context.
func Get(ctx context.Context) (Config, error) {
	cfg, err := cachedCfg.Get(ctx, nil)
	if err != nil {
		return nil, err
	}
	return &config{cfg.(*configpb.Config)}, nil
}
