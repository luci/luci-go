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

// Package config implements app-level configs for LUCI Analysis.
package config

import (
	"context"

	configpb "go.chromium.org/luci/analysis/proto/config"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/server/cfgcache"
	"go.chromium.org/luci/config/validation"
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

// Update fetches the latest config and puts it into the datastore.
func Update(ctx context.Context) error {
	var errs []error
	if _, err := cachedCfg.Update(ctx, nil); err != nil {
		errs = append(errs, err)
	}
	if err := updateProjects(ctx); err != nil {
		errs = append(errs, err)
	}
	if len(errs) > 0 {
		return errors.NewMultiError(errs...)
	}
	return nil
}

// Get returns the service-level config.
func Get(ctx context.Context) (*configpb.Config, error) {
	cfg, err := cachedCfg.Get(ctx, nil)
	if err != nil {
		return nil, err
	}
	return cfg.(*configpb.Config), err
}

// SetTestConfig set test configs in the cachedCfg.
func SetTestConfig(ctx context.Context, cfg *configpb.Config) error {
	return cachedCfg.Set(ctx, cfg, &config.Meta{})
}

func WithBothBugSystems(f func(system configpb.BugSystem, name string)) func() {
	return func() {
		f(configpb.BugSystem_MONORAIL, "monorail")
		f(configpb.BugSystem_BUGANIZER, "buganizer")
	}
}
