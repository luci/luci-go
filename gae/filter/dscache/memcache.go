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

package dscache

import (
	"context"
	"time"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/gae/service/memcache"
)

// States for a memcache entry. itemFlagUnknown exists to distinguish the
// default zero state from a valid state, but shouldn't ever be observed in
// memcache.
const (
	itemFlagUnknown uint32 = iota
	itemFlagHasData
	itemFlagHasLock
)

type memcacheItem struct {
	item memcache.Item
}

type memcacheImpl struct{}

func (memcacheImpl) PutLocks(ctx context.Context, keys []string, timeout time.Duration) error {
	if len(keys) == 0 {
		return nil
	}
	items := make([]memcache.Item, len(keys))
	for i, key := range keys {
		items[i] = memcache.NewItem(ctx, key).
			SetFlags(itemFlagHasLock).
			SetExpiration(timeout)
	}
	return memcache.Set(ctx, items...)
}

func (memcacheImpl) DropLocks(ctx context.Context, keys []string) error {
	if len(keys) == 0 {
		return nil
	}
	return errors.Filter(memcache.Delete(ctx, keys...), memcache.ErrCacheMiss)
}

func (memcacheImpl) TryLockAndFetch(ctx context.Context, keys []string, nonce []byte, timeout time.Duration) ([]CacheItem, error) {
	if len(keys) == 0 {
		return nil, nil
	}

	mcItems := make([]memcache.Item, len(keys))
	for i, key := range keys {
		if key == "" {
			continue
		}
		mcItems[i] = memcache.NewItem(ctx, key).
			SetFlags(itemFlagHasLock).
			SetExpiration(timeout).
			SetValue(nonce)
	}

	if err := memcache.Add(ctx, mcItems...); err != nil {
		// Ignore this error. Either we couldn't add them because they exist
		// (so, not an issue), or because memcache is having sad times (in which
		// case we'll see so in the Get which immediately follows this).
	}

	// We'll return this error as is in the end, along with all items that were
	// fetched successfully.
	err := errors.Filter(memcache.Get(ctx, mcItems...), memcache.ErrCacheMiss)

	items := make([]CacheItem, len(mcItems))
	for i, mcItem := range mcItems {
		if mcItem == nil {
			continue // cache miss or a gap in "keys"
		}
		if flag := mcItem.Flags(); flag == itemFlagUnknown || flag > itemFlagHasLock {
			continue // unrecognized memcache entry, treat it as a cache miss
		}
		items[i] = memcacheItem{mcItem}
	}

	return items, err
}

func (memcacheImpl) CompareAndSwap(ctx context.Context, items []CacheItem) error {
	if len(items) == 0 {
		return nil
	}
	mcItems := make([]memcache.Item, len(items))
	for i, item := range items {
		mcItems[i] = item.(memcacheItem).item
	}
	return memcache.CompareAndSwap(ctx, mcItems...)
}

// Implement CacheItem interface for memcacheItem.

func (m memcacheItem) Key() string {
	return m.item.Key()
}

func (m memcacheItem) Nonce() []byte {
	if m.item.Flags() == itemFlagHasLock {
		return m.item.Value()
	}
	return nil
}

func (m memcacheItem) Data() []byte {
	if m.item.Flags() == itemFlagHasData {
		return m.item.Value()
	}
	return nil
}

func (m memcacheItem) Prefix() []byte {
	return nil
}

func (m memcacheItem) PromoteToData(data []byte, exp time.Duration) {
	if m.item.Flags() != itemFlagHasLock {
		panic("only locks should be promoted")
	}
	m.item.SetFlags(itemFlagHasData)
	m.item.SetExpiration(exp)
	m.item.SetValue(data)
}

func (m memcacheItem) PromoteToIndefiniteLock() {
	if m.item.Flags() != itemFlagHasLock {
		panic("only locks should be promoted")
	}
	m.item.SetFlags(itemFlagHasLock)
	m.item.SetExpiration(0)
	m.item.SetValue(nil)
}
