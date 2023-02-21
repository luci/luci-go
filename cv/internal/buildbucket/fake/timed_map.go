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

package bbfake

import (
	"context"
	"sync"
	"time"

	"go.chromium.org/luci/common/clock"
)

type item struct {
	value    any
	expireAt time.Time
}

// timedMap is a map where each item has a TTL.
//
// cleanup is done lazily.
type timedMap struct {
	m  map[string]*item
	mu sync.RWMutex
}

// set adds an item to the map with ttl.
//
// zero or negative `expiration` means never expire
func (rc *timedMap) set(ctx context.Context, key string, value any, expiration time.Duration) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	i := &item{value: value}
	if expiration > 0 {
		i.expireAt = clock.Now(ctx).Add(expiration)
	}
	if rc.m == nil {
		rc.m = make(map[string]*item)
	}
	rc.m[key] = i
	rc.cleanupLocked(ctx)
}

func (rc *timedMap) get(ctx context.Context, key string) (any, bool) {
	defer func() {
		// TODO(yiwzhang): Once move to go 1.18, use Trylock for best effort clean
		// up.
		rc.mu.Lock()
		rc.cleanupLocked(ctx)
		rc.mu.Unlock()
	}()
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	switch item, ok := rc.m[key]; {
	case !ok:
		return nil, false
	case item.expired(clock.Now(ctx)):
		return nil, false
	default:
		return item.value, true
	}
}

func (rc *timedMap) cleanupLocked(ctx context.Context) {
	now := clock.Now(ctx)
	for key, item := range rc.m {
		if item.expired(now) {
			delete(rc.m, key)
		}
	}
}

func (item *item) expired(now time.Time) bool {
	return !item.expireAt.IsZero() && item.expireAt.Before(now)
}
