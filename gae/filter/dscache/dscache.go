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
	"bytes"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"time"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/gae/service/datastore/serialize"
)

var (
	// InstanceEnabledStatic allows you to statically (e.g. in an init() function)
	// bypass this filter by setting it to false. This takes effect when the
	// application calls IsGloballyEnabled.
	InstanceEnabledStatic = true

	// LockTimeSeconds is the number of seconds that a "lock" memcache entry will
	// have its expiration set to. It's set to just over half of the frontend
	// request handler timeout (currently 60 seconds).
	LockTimeSeconds = 31

	// CacheTimeSeconds is the default number of seconds that a cached entity will
	// be retained (memcache contention notwithstanding). A value of 0 is
	// infinite. This is #seconds instead of time.Duration, because memcache
	// truncates expiration to the second, which means a sub-second amount would
	// actually truncate to an infinite timeout.
	CacheTimeSeconds = int64((time.Hour * 24).Seconds())

	// CompressionThreshold is the number of bytes of entity value after which
	// compression kicks in.
	CompressionThreshold = 860

	// DefaultShards is the default number of key sharding to do.
	DefaultShards = 1

	// DefaultEnabled indicates whether or not caching is globally enabled or
	// disabled by default. Can still be overridden by CacheEnableMeta.
	DefaultEnabled = true
)

const (
	// MemcacheVersion will be incremented in the event that the in-memcache
	// representation of the cache data is modified.
	MemcacheVersion = "1"

	// KeyFormat is the format string used to generate memcache keys. It's
	//   gae:<version>:<shard#>:<base64_std_nopad(sha1(datastore.Key))>
	KeyFormat = "gae:" + MemcacheVersion + ":%x:%s"

	// Sha1B64Padding is the number of padding characters a base64 encoding of
	// a sha1 has.
	Sha1B64Padding = 1

	// MaxShards is the maximum number of shards a single entity can have.
	MaxShards = 256

	// MaxShardsLen is the number of characters in the key the shard field
	// occupies.
	MaxShardsLen = len("ff")

	// InternalGAEPadding is the estimated internal padding size that GAE takes
	// per memcache line.
	//   https://cloud.google.com/appengine/docs/go/memcache/#Go_Limits
	InternalGAEPadding = 96

	// ValueSizeLimit is the maximum encoded size a datastore key+entry may
	// occupy. If a datastore entity is too large, it will have an indefinite
	// lock which will cause all clients to fetch it from the datastore.
	ValueSizeLimit = (1000 * 1000) - InternalGAEPadding - MaxShardsLen

	// CacheEnableMeta is the gae metadata key name for whether or not dscache
	// is enabled for an entity type at all.
	CacheEnableMeta = "dscache.enable"

	// CacheExpirationMeta is the gae metadata key name for the default
	// expiration time (in seconds) for an entity type.
	CacheExpirationMeta = "dscache.expiration"

	// NonceBytes is the number of bytes to use in the 'lock' nonce.
	NonceBytes = 8

	// GlobalEnabledCheckInterval is how frequently IsGloballyEnabled should check
	// the globalEnabled datastore entry.
	GlobalEnabledCheckInterval = 5 * time.Minute
)

// internalValueSizeLimit is a var for testing purposes.
var internalValueSizeLimit = ValueSizeLimit

// CompressionType is the type of compression a single memcache entry has.
type CompressionType byte

// Types of compression. ZlibCompression uses "compress/zlib".
const (
	NoCompression CompressionType = iota
	ZlibCompression
)

func (c CompressionType) String() string {
	switch c {
	case NoCompression:
		return "NoCompression"
	case ZlibCompression:
		return "ZlibCompression"
	default:
		return fmt.Sprintf("UNKNOWN_CompressionType(%d)", c)
	}
}

// FlagValue is used to indicate if a memcache entry currently contains an
// item or a lock.
type FlagValue uint32

// States for a memcache entry. ItemUNKNOWN exists to distinguish the default
// zero state from a valid state, but shouldn't ever be observed in memcache. .
const (
	ItemUKNONWN FlagValue = iota
	ItemHasData
	ItemHasLock
)

// MakeMemcacheKey generates a memcache key for the given datastore Key. This
// is useful for debugging.
func MakeMemcacheKey(shard int, k *datastore.Key) string {
	return fmt.Sprintf(KeyFormat, shard, HashKey(k))
}

// HashKey generates just the hashed portion of the MemcacheKey.
func HashKey(k *datastore.Key) string {
	dgst := sha1.Sum(serialize.ToBytes(k))
	buf := bytes.Buffer{}
	enc := base64.NewEncoder(base64.StdEncoding, &buf)
	_, _ = enc.Write(dgst[:])
	enc.Close()
	return buf.String()[:buf.Len()-Sha1B64Padding]
}
