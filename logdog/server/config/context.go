// Copyright 2020 The LUCI Authors.
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

// Package config abstracts access to Logdog service and project configuration.
//
// Sync(...) assumes the context has a cfgclient implementation and a read-write
// datastore. All other methods need only read-only datastore.
package config

import (
	"context"
	"sync"
	"time"

	"go.chromium.org/luci/common/data/caching/lazyslot"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/router"

	"go.chromium.org/luci/logdog/api/config/svcconfig"
)

var (
	// ErrInvalidConfig is returned when the configuration exists, but is invalid.
	ErrInvalidConfig = errors.New("invalid configuration")
)

// Store caches configs in memory to avoid hitting cfgclient all the time.
//
// Keep at as a global variable and install into contexts via WithStore.
type Store struct {
	// NoCache disables in-process caching (useful in tests).
	NoCache bool

	service  lazyslot.Slot             // caches the main service config
	m        sync.RWMutex              // protects 'projects'
	projects map[string]*lazyslot.Slot // caches project configs
}

// projectCacheSlot returns a slot with a project config cache.
func (s *Store) projectCacheSlot(projectID string) *lazyslot.Slot {
	s.m.RLock()
	slot := s.projects[projectID]
	s.m.RUnlock()
	if slot != nil {
		return slot
	}

	s.m.Lock()
	defer s.m.Unlock()

	if slot = s.projects[projectID]; slot != nil {
		return slot
	}
	slot = &lazyslot.Slot{}

	if s.projects == nil {
		s.projects = make(map[string]*lazyslot.Slot, 1)
	}
	s.projects[projectID] = slot

	return slot
}

var storeKey = "LogDog config.Store"

// store returns the installed store or panics if it's not installed.
func store(ctx context.Context) *Store {
	s, _ := ctx.Value(&storeKey).(*Store)
	if s == nil {
		panic("config.Store is not in the context")
	}
	return s
}

// WithStore installs a store that caches configs in memory.
func WithStore(ctx context.Context, s *Store) context.Context {
	return context.WithValue(ctx, &storeKey, s)
}

// Middleware returns a middleware that installs `s` into requests' context.
func Middleware(s *Store) router.Middleware {
	return func(ctx *router.Context, next router.Handler) {
		ctx.Request = ctx.Request.WithContext(WithStore(ctx.Request.Context(), s))
		next(ctx)
	}
}

// Config loads and returns the service configuration.
func Config(ctx context.Context) (*svcconfig.Config, error) {
	store := store(ctx)
	if store.NoCache {
		return fetchServiceConfig(ctx)
	}
	cached, err := store.service.Get(ctx, func(prev any) (val any, exp time.Duration, err error) {
		logging.Infof(ctx, "Cache miss for services.cfg, fetching it from datastore...")
		cfg, err := fetchServiceConfig(ctx)
		return cfg, time.Minute, err
	})
	if err != nil {
		return nil, err
	}
	return cached.(*svcconfig.Config), nil
}

// fetchServiceConfig fetches the service config from the datastore.
func fetchServiceConfig(ctx context.Context) (*svcconfig.Config, error) {
	var cfg svcconfig.Config
	switch err := fromDatastore(ctx, serviceConfigKind, serviceConfigPath, &cfg); {
	case transient.Tag.In(err):
		return nil, err
	case err == datastore.ErrNoSuchEntity:
		return nil, config.ErrNoConfig
	case err != nil:
		logging.Errorf(ctx, "Broken service config in the datastore: %s", err)
		return nil, ErrInvalidConfig
	default:
		return &cfg, nil
	}
}

// missingProjectMarker is cached instead of *svcconfig.ProjectConfig if the
// project is missing to avoid hitting datastore all the time when accessing
// missing projects.
//
// Note: strictly speaking caching all missing projects forever in
// Store.projects introduces a DoS attack vector. But this code is scheduled for
// removal when Logdog is integrated with LUCI Realms, so it's fine to ignore
// this problem for now.
var missingProjectMarker = "missing project"

// ProjectConfig loads the project config protobuf from the config service.
//
// This function will return following errors:
//   - nil, if the project exists and the configuration successfully loaded
//   - config.ErrNoConfig if the project configuration was not present.
//   - ErrInvalidConfig if the project configuration was present, but could not
//     be loaded.
//   - Some other error if an error occurred that does not fit one of the
//     previous categories.
func ProjectConfig(ctx context.Context, projectID string) (*svcconfig.ProjectConfig, error) {
	store := store(ctx)
	if projectID == "" {
		return nil, config.ErrNoConfig
	}
	if store.NoCache {
		return fetchProjectConfig(ctx, projectID)
	}
	cached, err := store.projectCacheSlot(projectID).Get(ctx, func(prev any) (val any, exp time.Duration, err error) {
		logging.Infof(ctx, "Cache miss for %q project config, fetching it...", projectID)
		cfg, err := fetchProjectConfig(ctx, projectID)
		if err == config.ErrNoConfig {
			return &missingProjectMarker, time.Minute, nil
		}
		return cfg, time.Minute, err
	})
	if err != nil {
		return nil, err
	}
	if cached == &missingProjectMarker {
		return nil, config.ErrNoConfig
	}
	return cached.(*svcconfig.ProjectConfig), nil
}

// fetchProjectConfig fetches a project config from the datastore.
func fetchProjectConfig(ctx context.Context, projectID string) (*svcconfig.ProjectConfig, error) {
	var cfg svcconfig.ProjectConfig
	switch err := fromDatastore(ctx, projectConfigKind, projectID, &cfg); {
	case transient.Tag.In(err):
		return nil, err
	case err == datastore.ErrNoSuchEntity:
		return nil, config.ErrNoConfig
	case err != nil:
		logging.Errorf(ctx, "Broken project config for %q in the datastore: %s", projectID, err)
		return nil, ErrInvalidConfig
	default:
		return &cfg, nil
	}
}
