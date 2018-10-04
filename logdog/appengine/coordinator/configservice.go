// Copyright 2017 The LUCI Authors.
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

package coordinator

import (
	"context"
	"sync"

	"go.chromium.org/luci/logdog/api/config/svcconfig"
	"go.chromium.org/luci/logdog/appengine/coordinator/config"
	"go.chromium.org/luci/logdog/common/types"
)

// ConfigProvider is a set of support services used by Coordinator to fetch
// configurations.
//
// Each instance is valid for a single request, but can be re-used throughout
// that request. This is advised, as the Services instance may optionally cache
// values.
//
// ConfigServices methods are goroutine-safe.
type ConfigProvider interface {
	// Config returns the current instance and application configuration
	// instances.
	//
	// The production instance will cache the results for the duration of the
	// request.
	Config(context.Context) (*config.Config, error)

	// ProjectConfig returns the project configuration for the named project.
	//
	// The production instance will cache the results for the duration of the
	// request.
	//
	// Returns the same error codes as config.ProjectConfig.
	ProjectConfig(context.Context, types.ProjectName) (*svcconfig.ProjectConfig, error)
}

// LUCIConfigProvider is a ConfigProvider implementation that loads its
// configuration from the LUCI Config Service.
type LUCIConfigProvider struct {
	lock sync.Mutex

	// gcfg is the cached global configuration.
	gcfg           *config.Config
	projectConfigs map[types.ProjectName]*cachedProjectConfig
}

// Config implements ConfigProvider.
func (s *LUCIConfigProvider) Config(c context.Context) (*config.Config, error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	// Load/cache the global config.
	if s.gcfg == nil {
		var err error
		s.gcfg, err = config.Load(c)
		if err != nil {
			return nil, err
		}
	}

	return s.gcfg, nil
}

// cachedProjectConfig is a singleton instance that holds a project config
// state. It is populated when resolve is called, and is goroutine-safe for
// read-only operations.
type cachedProjectConfig struct {
	sync.Once

	project types.ProjectName
	pcfg    *svcconfig.ProjectConfig
	err     error
}

func (cp *cachedProjectConfig) resolve(c context.Context) (*svcconfig.ProjectConfig, error) {
	// Load the project config exactly once. This will be cached for the remainder
	// of this request.
	//
	// If multiple goroutines attempt to load it, exactly one will, and the rest
	// will block. All operations after this Once must be read-only.
	cp.Do(func() {
		cp.pcfg, cp.err = config.ProjectConfig(c, cp.project)
	})
	return cp.pcfg, cp.err
}

func (s *LUCIConfigProvider) getOrCreateCachedProjectConfig(project types.ProjectName) *cachedProjectConfig {
	s.lock.Lock()
	defer s.lock.Unlock()

	if s.projectConfigs == nil {
		s.projectConfigs = make(map[types.ProjectName]*cachedProjectConfig)
	}
	cp := s.projectConfigs[project]
	if cp == nil {
		cp = &cachedProjectConfig{
			project: project,
		}
		s.projectConfigs[project] = cp
	}
	return cp
}

// ProjectConfig implements ConfigProvider.
func (s *LUCIConfigProvider) ProjectConfig(c context.Context, project types.ProjectName) (*svcconfig.ProjectConfig, error) {
	return s.getOrCreateCachedProjectConfig(project).resolve(c)
}
