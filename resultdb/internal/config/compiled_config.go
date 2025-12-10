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

	"go.chromium.org/luci/resultdb/internal/androidbuild"
	"go.chromium.org/luci/resultdb/internal/producersystems"
	"go.chromium.org/luci/resultdb/internal/schemes"
	"go.chromium.org/luci/resultdb/pbutil"
	configpb "go.chromium.org/luci/resultdb/proto/config"
)

// CompiledServiceConfig is a copy of service configuration with
// regular expression pre-compiled.
// This object is shared between multiple request handlers and
// must be treated as immutable, do not append to any of the
// collections returned here.
type CompiledServiceConfig struct {
	// The raw service configuration.
	Config   *configpb.Config
	Revision string
	// The compiled scheme configuration, by scheme ID.
	Schemes map[string]*schemes.Scheme
	// The compiled producer system configuration, by producer system name.
	ProducerSystems map[string]*producersystems.ProducerSystem
	// The compiled android build system configuration.
	AndroidBuild *androidbuild.Config
}

// NewCompiledServiceConfig compiles the given raw service config
// into a form that is faster to apply.
func NewCompiledServiceConfig(cfg *configpb.Config, revision string) (*CompiledServiceConfig, error) {
	compiledSchemes := make(map[string]*schemes.Scheme)

	compiledSchemes[pbutil.LegacySchemeID] = schemes.LegacyScheme
	for _, scheme := range cfg.Schemes {
		compiledScheme, err := compileScheme(scheme)
		if err != nil {
			return nil, err
		}
		compiledSchemes[scheme.Id] = compiledScheme
	}

	compiledProducerSystems := make(map[string]*producersystems.ProducerSystem, len(cfg.ProducerSystems))
	for _, ps := range cfg.ProducerSystems {
		compiledPS, err := producersystems.NewProducerSystem(ps)
		if err != nil {
			return nil, err
		}
		compiledProducerSystems[ps.System] = compiledPS
	}

	androidBuild, err := androidbuild.NewConfig(cfg.AndroidBuild)
	if err != nil {
		return nil, err
	}

	return &CompiledServiceConfig{
		Config:          cfg,
		Revision:        revision,
		Schemes:         compiledSchemes,
		ProducerSystems: compiledProducerSystems,
		AndroidBuild:    androidBuild,
	}, nil
}

func compileScheme(scheme *configpb.Scheme) (*schemes.Scheme, error) {
	compiledScheme := &schemes.Scheme{
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
	return compiledScheme, nil
}

func NewSchemeLevel(level *configpb.Scheme_Level) (*schemes.SchemeLevel, error) {
	result := &schemes.SchemeLevel{
		HumanReadableName: level.HumanReadableName,
	}
	if level.ValidationRegexp != "" {
		compiledRegexp, err := regexp.Compile(level.ValidationRegexp)
		if err != nil {
			// This should never happen as the configuration has been validated.
			return nil, errors.Fmt("could not compile validation regexp: %w", err)
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
			err = errors.Fmt("fetch cached service config: %w", err)
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
			err = errors.Fmt("compile service config: %w", err)
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
