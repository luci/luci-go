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
// All methods assume the context has a server/cfgclient implementation
// available. No other assumptions are made. In particular, there's no strong
// dependency on GAE, since this package is used by the Kubernetes components as
// well.
//
// Implements in-memory caching logic itself, so server/cfgclient doesn't need
// to have this layer of the cache enabled.
package config

import (
	"context"
	"sync"

	"go.chromium.org/luci/common/data/caching/lazyslot"
	"go.chromium.org/luci/server/router"
)

// Store caches configs in memory to avoid hitting cfgclient all the time.
//
// Keep at as a global variable and install into contexts via WithStore.
type Store struct {
	// ServiceID returns LogDog's Cloud Project ID.
	ServiceID func(context.Context) string
	// NoCache disables in-process caching (useful in tests).
	NoCache bool

	once      sync.Once
	serviceID string // cached result of ServiceID(...)

	service lazyslot.Slot // caches the main service config

	m        sync.RWMutex              // protects 'projects'
	projects map[string]*lazyslot.Slot // caches project configs
}

// projectCacheSlot returns a slot with a project config cache.
func (s *Store) projectCacheSlot(projectID string) *lazyslot.Slot {
	s.m.RLock()
	slot, _ := s.projects[projectID]
	s.m.RUnlock()
	if slot != nil {
		return slot
	}

	s.m.Lock()
	defer s.m.Unlock()

	if slot, _ = s.projects[projectID]; slot != nil {
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

// WithStore installs a store that caches configs in memory.
func WithStore(ctx context.Context, s *Store) context.Context {
	return context.WithValue(ctx, &storeKey, s)
}

// Middleware returns a middleware that installs `s` into requests' context.
func Middleware(s *Store) router.Middleware {
	return func(ctx *router.Context, next router.Handler) {
		ctx.Context = WithStore(ctx.Context, s)
		next(ctx)
	}
}

// store returns the installed store or panics if it's not installed.
func store(ctx context.Context) *Store {
	s, _ := ctx.Value(&storeKey).(*Store)
	if s == nil {
		panic("config.Store is not in the context")
	}
	s.once.Do(func() {
		s.serviceID = s.ServiceID(ctx)
	})
	return s
}
