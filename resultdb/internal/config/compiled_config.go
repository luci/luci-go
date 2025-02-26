// Copyright 2025 The LUCI Authors.
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
	"regexp"
	"time"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/server/caching"

	configpb "go.chromium.org/luci/resultdb/proto/config"
)

// CompiledServiceConfig is a copy of service configuration with
// regular expression pre-compiled.
// This object must be treated as immutable.
type CompiledServiceConfig struct {
	// The raw service configuration.
	Config   *configpb.Config
	Revision string
	// The compiled scheme configuration, by scheme ID.
	Schemes map[string]*Scheme
}

// Scheme represents the configuration for a type of test.
// For example, JUnit or GTest.
type Scheme struct {
	// The identifier of the scheme.
	ID string
	// The human readable scheme name.
	HumanReadableName string
	// Configuration for the coarse name level.
	// If set, the scheme uses the coarse name level.
	Coarse *SchemeLevel
	// Configuration for the fine name level.
	// If set, the fine name.
	Fine *SchemeLevel
	// Configuration for the case name level.
	// Always set.
	Case *SchemeLevel
}

// SchemeLevel represents a test hierarchy level in a test scheme.
type SchemeLevel struct {
	// The human readable name of the level.
	HumanReadableName string
	// The compiled validation regular expression.
	// May be nil if no validation is to be appled.
	ValidationRegexp *regexp.Regexp
}

// NewCompiledServiceConfig compiles the given raw service config
// into a form that is faster to apply.
func NewCompiledServiceConfig(cfg *configpb.Config, revision string) (*CompiledServiceConfig, error) {
	compiledSchemes := make(map[string]*Scheme)
	compiledSchemes["legacy"] = &Scheme{
		ID:                "legacy",
		HumanReadableName: "Legacy Test Results",
		Case: &SchemeLevel{
			HumanReadableName: "Test Identifier",
		},
	}
	for _, scheme := range cfg.Schemes {
		compiledScheme := &Scheme{
			ID:                scheme.Id,
			HumanReadableName: scheme.HumanReadableName,
		}
		if scheme.Coarse != nil {
			compiledLevel, err := NewSchemeLevel(scheme.Coarse)
			if err != nil {
				return nil, err
			}
			compiledScheme.Coarse = compiledLevel
		}
		if scheme.Fine != nil {
			compiledLevel, err := NewSchemeLevel(scheme.Fine)
			if err != nil {
				return nil, err
			}
			compiledScheme.Fine = compiledLevel
		}
		compiledLevel, err := NewSchemeLevel(scheme.Case)
		if err != nil {
			return nil, err
		}
		compiledScheme.Case = compiledLevel
		compiledSchemes[scheme.Id] = compiledScheme
	}
	return &CompiledServiceConfig{
		Config:   cfg,
		Revision: revision,
		Schemes:  compiledSchemes,
	}, nil
}

func NewSchemeLevel(level *configpb.Scheme_Level) (*SchemeLevel, error) {
	result := &SchemeLevel{
		HumanReadableName: level.HumanReadableName,
	}
	if level.ValidationRegexp != "" {
		compiledRegexp, err := regexp.Compile(`^` + level.ValidationRegexp + `$`)
		if err != nil {
			// This should never happen as the configuration has been validated.
			return nil, errors.Annotate(err, "could not compile validation regexp").Err()
		}
		result.ValidationRegexp = compiledRegexp
	}
	return result, nil
}

var schemeCache = caching.RegisterCacheSlot()

// Service returns the compiled configuration for the service.
func Service(ctx context.Context) (*CompiledServiceConfig, error) {
	result, err := schemeCache.Fetch(ctx, func(prev any) (updated any, exp time.Duration, err error) {
		var meta config.Meta
		cfg, err := cachedServiceCfg.Get(ctx, &meta)
		if err != nil {
			err = errors.Annotate(err, "fetch cached service config").Err()
			logging.Errorf(ctx, "%s", err.Error())
			return nil, 0, err
		}

		if prev != nil {
			prevCfg := prev.(*CompiledServiceConfig)
			if prevCfg.Revision == meta.Revision {
				// Re-use the previously compiled config.
				// Expire within one second. This method is cheap to run as it
				// simply consults the other service config cache.
				return prevCfg, time.Second, nil
			}
		}
		cfgProto := cfg.(*configpb.Config)
		compiledCfg, err := NewCompiledServiceConfig(cfgProto, meta.Revision)
		if err != nil {
			err = errors.Annotate(err, "compile service config").Err()
			return nil, 0, err
		}

		// Expire within one second. This method is cheap to run as it
		// simply consults the other service config cache.
		return compiledCfg, time.Second, nil
	})
	if err != nil {
		return nil, err
	}
	return result.(*CompiledServiceConfig), nil
}
