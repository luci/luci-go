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

package dscache

import (
	"context"
	"time"
)

const (
	// MutationLockTimeout is expiration time of a "lock" memcache entry that
	// protects mutations (Put/Delete/Commit). It should be larger than the
	// maximum expected duration of datastore mutating operations. Must have
	// seconds precision.
	MutationLockTimeout = 120 * time.Second

	// RefreshLockTimeout is expiration time of a "lock" memcache entry that
	// protects the cache refresh process (during Get). It should be larger than
	// expected Get duration, but it's not a big deal if the lock expires sooner.
	// Must have seconds precision.
	RefreshLockTimeout = 20 * time.Second

	// CacheDuration is the default duration that a cached entity will be retained
	// (memcache contention notwithstanding). Must have seconds precision.
	CacheDuration = time.Hour * 24

	// CompressionThreshold is the number of bytes of entity value after which
	// compression kicks in.
	CompressionThreshold = 860

	// DefaultShards is the default number of key sharding to do.
	DefaultShards = 1

	// MaxShards is the maximum number of shards a single entity can have.
	MaxShards = 256

	// MemcacheVersion will be incremented in the event that the in-memcache
	// representation of the cache data is modified.
	MemcacheVersion = "1"

	// KeyFormat is the format string used to generate memcache keys. It's
	//   gae:<version>:<shard#>:<base64_std_nopad(sha1(datastore.Key))>
	KeyFormat = "gae:" + MemcacheVersion + ":%x:%s"

	// ValueSizeLimit is the maximum encoded size a datastore key+entry may
	// occupy. If a datastore entity is too large, it will have an indefinite
	// lock which will cause all clients to fetch it from the datastore.
	//
	// See https://cloud.google.com/appengine/docs/go/memcache/#Go_Limits
	// 80 is approximately the internal GAE padding. 36 is maximum length of our
	// keys.
	ValueSizeLimit = (1024 * 1024) - 80 - 36

	// CacheEnableMeta is the gae metadata key name for whether or not dscache
	// is enabled for an entity type at all.
	CacheEnableMeta = "dscache.enable"

	// CacheExpirationMeta is the gae metadata key name for the default
	// expiration time (in seconds) for an entity type.
	CacheExpirationMeta = "dscache.expiration"

	// NonceBytes is the number of bytes to use in the 'lock' nonce.
	NonceBytes = 8
)

// CacheItem represents either a cached datastore entity or a placeholder lock
// that "promises" that such entity is being fetched now (either by us or by
// someone else).
//
// CacheItem is created by TryLockAndFetch. An item that represents a lock
// can be "promoted" into either a data item or a permanent lock. Such promoted
// items are stored by CompareAndSwap.
type CacheItem interface {
	// Key is the item's key as passed to TryLockAndFetch.
	Key() string

	// Nonce returns nil for data items or a lock nonce for lock items.
	Nonce() []byte

	// Data returns nil for lock items or an item's data for data items.
	Data() []byte

	// PromoteToData converts this lock item into a data item.
	//
	// Panics if self is not a lock item.
	PromoteToData(data []byte, exp time.Duration)

	// PromoteToIndefiniteLock converts this lock into an indefinite lock.
	//
	// An indefinite lock means that the datastore item is not cacheable for some
	// reasons and 'Get' should not try to cache it. Such locks are removed by
	// PutLocks/DropLocks.
	//
	// Panics if self is not a lock item.
	PromoteToIndefiniteLock()
}

// Cache abstracts a particular memcache implementation.
//
// This interface is tightly coupled to the dscache algorithm (rather than
// trying to emulate a generic cache API) to allow the implementation to be as
// efficient as possible.
type Cache interface {
	// PutLocks is called before mutating entities during Put/Delete/Commit.
	//
	// `keys` represent CacheItem keys of all shards of all to-be-mutated
	// entities. The implementation should unconditionally write locks into all
	// these keys keys.
	//
	// Errors are treated as fatal.
	PutLocks(ctx context.Context, keys []string, timeout time.Duration) error

	// DropLocks is called after finishing Put/Delete/Commit.
	//
	// The implementation should unconditionally remove these keys, thus unlocking
	// them (if they were locked).
	//
	// Errors are logged, but ignored.
	DropLocks(ctx context.Context, keys []string) error

	// TryLockAndFetch is called before executing Get.
	//
	// Each key is either empty, or contains some random shard of a to-be-fetched
	// entity (one such key per entity). For each non-empty key, if it doesn't
	// exist yet, the implementation should try to write a lock with the nonce.
	// It then should fetch all keys (whatever they might be).
	//
	// Should always return len(keys) items, even on errors. Items matching empty
	// keys should be nil. Items that do not exist in the cache should also be
	// represented by nils.
	//
	// Errors are logged, but ignored (i.e. treated as cache misses).
	TryLockAndFetch(ctx context.Context, keys []string, nonce []byte, timeout time.Duration) ([]CacheItem, error)

	// CompareAndSwap stores promoted items (see CacheItem) in place of locks
	// they formerly represented iff the cache still has the same locks there.
	//
	// Errors are logged, but ignored.
	CompareAndSwap(ctx context.Context, items []CacheItem) error
}
