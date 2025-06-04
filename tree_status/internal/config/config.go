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

// Package config implements service-level config for LUCI Tree Status.
package config

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/config/server/cfgcache"
	"go.chromium.org/luci/config/validation"

	configpb "go.chromium.org/luci/tree_status/proto/config"
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

// ErrNotFoundTreeConfig will be returned if configuration for a given project does not exist.
var ErrNotFoundTreeConfig = fmt.Errorf("tree config not found")

// Update fetches the config and puts it into the datastore.
func Update(ctx context.Context) error {
	_, err := cachedCfg.Update(ctx, nil)
	return err
}

// Get returns the config stored in the context.
func Get(ctx context.Context) (*configpb.Config, error) {
	cfg, err := cachedCfg.Get(ctx, nil)
	if err != nil {
		return nil, err
	}
	return cfg.(*configpb.Config), nil
}

// GetTreeConfig returns config for a tree name.
// If the config for the tree name is not found, it will return an ErrNotFoundTreeConfig error.
func GetTreeConfig(ctx context.Context, treeName string) (*configpb.Tree, error) {
	cfg, err := Get(ctx)
	if err != nil {
		return nil, errors.Fmt("getting config: %w", err)
	}
	for _, treeConfg := range cfg.Trees {
		if treeConfg.Name == treeName {
			return treeConfg, nil
		}
	}
	return nil, ErrNotFoundTreeConfig
}
