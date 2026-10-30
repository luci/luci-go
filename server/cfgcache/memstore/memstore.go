// Copyright 2026 The LUCI Authors.
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

package memstore

import (
	"context"
	"sync"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/config"

	"go.chromium.org/luci/server/cfgcache"
	"go.chromium.org/luci/server/module"
)

// Store implements a [cfgcache.Store] which supports in-memory subscription
// backed by an in-memory map.
//
// This is intended for writing tests.
type Store[StoreMsg proto.Message] struct {
	mu sync.Mutex

	// pipelineID -> config.Set -> Meta + Value
	data map[string]map[config.Set]*cfgcache.StoredConfig[StoreMsg]
}

var _ cfgcache.Store[proto.Message] = (*Store[proto.Message])(nil)

// ModuleDependencies implements cfgcache.Store.
func (*Store[StoreMsg]) ModuleDependencies() []module.Dependency { return nil }

// LoadMeta implements [cfgcache.Store].
func (s *Store[StoreMsg]) LoadMeta(ctx context.Context, pipelineID string) (map[config.Set]cfgcache.Meta, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	pipeline := s.data[pipelineID]
	if len(pipeline) == 0 {
		return nil, nil
	}

	ret := make(map[config.Set]cfgcache.Meta, len(s.data[pipelineID]))
	for cs, sc := range pipeline {
		ret[cs] = sc.Meta
	}

	return ret, nil
}

// Put implements [cfgcache.Store].
func (s *Store[StoreMsg]) Put(ctx context.Context, pipelineID string, cs config.Set, m cfgcache.Meta, out StoreMsg) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	pipeline := s.data[pipelineID]
	if pipeline == nil {
		if s.data == nil {
			s.data = map[string]map[config.Set]*cfgcache.StoredConfig[StoreMsg]{}
		}

		pipeline = map[config.Set]*cfgcache.StoredConfig[StoreMsg]{}
		s.data[pipelineID] = pipeline
	}

	pipeline[cs] = &cfgcache.StoredConfig[StoreMsg]{ConfigSet: cs, Meta: m, Config: out}

	return nil
}

// Delete implements [cfgcache.Store].
func (s *Store[StoreMsg]) Delete(ctx context.Context, pipelineID string, sets []config.Set) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	pipeline := s.data[pipelineID]
	if pipeline == nil {
		return nil
	}

	for _, cs := range sets {
		delete(pipeline, cs)
	}

	return nil
}
