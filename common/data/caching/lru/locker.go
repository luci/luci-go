// Copyright 2015 The LUCI Authors.
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

// Package lru provides least-recently-used (LRU) cache.
package lru

import (
	"sync"
)

// Locker is a read/write locker interface.
//
// Its RLock and RUnlock methods follow the same conventions as sync.RWMutex.
type Locker interface {
	sync.Locker

	// RLock locks the Locker for reading.
	RLock()
	// RLock unlocks the Locker for reading.
	RUnlock()
}

// nopLocker is a Locker implementation that performs no locking.
type nopLocker struct{}

func (nopLocker) Lock()    {}
func (nopLocker) Unlock()  {}
func (nopLocker) RLock()   {}
func (nopLocker) RUnlock() {}
