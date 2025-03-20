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

package internal

import (
	"sync"
)

// SyncWriteMap is a map that can be written to concurrently.
//
// Reads are unsynchronized and should happen only after the map is fully
// constructed (with the necessary barrier provided externally).
//
// A zero value is ready for use.
type SyncWriteMap[K comparable, V any] struct {
	mu sync.Mutex
	m  map[K]V
}

// Put adds an item into the map.
func (s *SyncWriteMap[K, V]) Put(k K, v V) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.m == nil {
		s.m = make(map[K]V, 1)
	}
	s.m[k] = v
}

// Get reads an item from the map.
func (s *SyncWriteMap[K, V]) Get(k K) V {
	return s.m[k]
}
