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

// Package compiledcfg contains compiled versions of the LUCI Analysis config.
// (E.g. Regular expressions are compiled for efficiency.)
package compiledcfg

import (
	"context"
	"regexp"
	"time"

	"go.chromium.org/luci/common/data/caching/lru"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server/caching"

	"go.chromium.org/luci/analysis/internal/clustering/algorithms/testname/rules"
	"go.chromium.org/luci/analysis/internal/config"
	configpb "go.chromium.org/luci/analysis/proto/config"
)

// TODO(crbug.com/1243174). Instrument the size of this cache so that we
// can monitor it.
var configCache = caching.RegisterLRUCache[string, *ProjectConfig](0)

// ProjectConfig is a compiled version of LUCI Analysis project configuration.
type ProjectConfig struct {
	// Config is the raw, uncompiled, configuration.
	Config *configpb.ProjectConfig

	// TestNameRules are the set of rules to use to cluster test results
	// by test name.
	TestNameRules []rules.Evaluator

	// ReasonMaskPatterns is the set of patterns to use to mask out parts
	// of the failure reason before clustering.
	ReasonMaskPatterns []*regexp.Regexp

	// LastUpdated is the time the configuration was last updated.
	LastUpdated time.Time
}

// NewConfig compiles the given clustering configuration into a Config
// object.
func NewConfig(config *configpb.ProjectConfig) (*ProjectConfig, error) {
	rs := config.Clustering.GetTestNameRules()
	compiledRules := make([]rules.Evaluator, len(rs))
	for i, rule := range rs {
		eval, err := rules.Compile(rule)
		if err != nil {
			return nil, errors.Fmt("compiling test name clustering rule: %w", err)
		}
		compiledRules[i] = eval
	}
	rmps := config.Clustering.GetReasonMaskPatterns()
	compiledReasonMaskPatterns := make([]*regexp.Regexp, len(rmps))
	for i, p := range rmps {
		re, err := regexp.Compile(p)
		if err != nil {
			return nil, errors.Fmt("compiling reason mask pattern: %w", err)
		}
		compiledReasonMaskPatterns[i] = re
	}

	return &ProjectConfig{
		Config:             config,
		TestNameRules:      compiledRules,
		ReasonMaskPatterns: compiledReasonMaskPatterns,
		LastUpdated:        config.LastUpdated.AsTime(),
	}, nil
}

// Project returns the clustering configuration for the given project,
// with a LastUpdated time of at least minimumVersion. If no particular
// minimum version is desired, pass time.Time{} to minimumVersion.
func Project(ctx context.Context, project string, minimumVersion time.Time) (*ProjectConfig, error) {
	cache := configCache.LRU(ctx)
	if cache == nil {
		// A fallback useful in unit tests that may not have the process cache
		// available. Production environments usually have the cache installed
		// by the framework code that initializes the root context.
		projectCfg, err := config.ProjectWithMinimumVersion(ctx, project, minimumVersion)
		if err != nil {
			return nil, err
		}
		config, err := NewConfig(projectCfg)
		if err != nil {
			return nil, err
		}
		return config, nil
	} else {
		var err error
		val, _ := cache.Mutate(ctx, project, func(it *lru.Item[*ProjectConfig]) *lru.Item[*ProjectConfig] {
			var projectCfg *configpb.ProjectConfig
			// Fetch the latest configuration for the given project, with
			// the specified minimum version.
			projectCfg, err = config.ProjectWithMinimumVersion(ctx, project, minimumVersion)
			if err != nil {
				// Delete cached value.
				return nil
			}

			if it != nil {
				cfg := it.Value
				if cfg.LastUpdated.Equal(projectCfg.LastUpdated.AsTime()) {
					// Cached value is already up to date.
					return it
				}
			}
			var config *ProjectConfig
			config, err = NewConfig(projectCfg)
			if err != nil {
				// Delete cached value.
				return nil
			}
			return &lru.Item[*ProjectConfig]{
				Value: config,
				Exp:   0, // No expiry.
			}
		})
		if err != nil {
			return nil, errors.Fmt("obtain compiled configuration: %w", err)
		}
		return val, nil
	}
}
