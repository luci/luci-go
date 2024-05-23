// Copyright 2018 The LUCI Authors.
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

package promise

import (
	"context"
	"sync"
)

// Map is a map from some key to a promise that does something associated
// with this key.
//
// First call to Get initiates a new promise. All subsequent calls return exact
// same promise (even if it has finished).
type Map[K comparable, V any] struct {
	mu sync.RWMutex
	m  map[K]*Promise[V]
}

// Get either returns an existing promise for the given key or creates and
// immediately launches a new promise.
func (pm *Map[K, V]) Get(ctx context.Context, key K, gen Generator[V]) *Promise[V] {
	pm.mu.RLock()
	p := pm.m[key]
	pm.mu.RUnlock()

	if p != nil {
		return p
	}

	pm.mu.Lock()
	defer pm.mu.Unlock()

	if p = pm.m[key]; p == nil {
		p = New(ctx, gen)
		if pm.m == nil {
			pm.m = make(map[K]*Promise[V], 1)
		}
		pm.m[key] = p
	}
	return p
}
