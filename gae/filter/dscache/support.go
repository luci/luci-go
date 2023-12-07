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
	"crypto/sha1"
	"encoding/base64"
	"fmt"

	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/logging"

	ds "go.chromium.org/luci/gae/service/datastore"
)

type supportContext struct {
	ds.KeyContext

	c            context.Context
	impl         Cache
	shardsForKey []ShardFunction
}

func (s *supportContext) numShards(k *ds.Key) int {
	ret := DefaultShards
	for _, fn := range s.shardsForKey {
		if amt, ok := fn(k); ok {
			ret = amt
		}
	}
	if ret < 1 {
		return 0 // disable caching entirely
	}
	if ret > MaxShards {
		ret = MaxShards
	}
	return ret
}

func (s *supportContext) mkRandKeys(keys []*ds.Key, metas ds.MultiMetaGetter, rnd mathrand.Rand) []string {
	ret := []string(nil)
	for i, key := range keys {
		mg := metas.GetSingle(i)
		if !ds.GetMetaDefault(mg, CacheEnableMeta, true).(bool) {
			continue
		}
		shards := s.numShards(key)
		if shards == 0 {
			continue
		}
		shard := 0
		if shards > 1 {
			shard = rnd.Intn(shards)
		}
		if ret == nil {
			ret = make([]string, len(keys))
		}
		ret[i] = makeMemcacheKey(shard, key)
	}
	return ret
}

func (s *supportContext) mkAllKeys(keys []*ds.Key) []string {
	size := 0
	nums := make([]int, len(keys))
	for i, key := range keys {
		if !key.IsIncomplete() {
			shards := s.numShards(key)
			nums[i] = shards
			size += shards
		}
	}
	if size == 0 {
		return nil
	}
	ret := make([]string, 0, size)
	for i, key := range keys {
		if !key.IsIncomplete() {
			keySuffix := hashKey(key)
			for shard := 0; shard < nums[i]; shard++ {
				ret = append(ret, fmt.Sprintf(KeyFormat, shard, keySuffix))
			}
		}
	}
	return ret
}

func (s *supportContext) mutation(keys []*ds.Key, f func() error) error {
	itemKeys := s.mkAllKeys(keys)
	if len(itemKeys) == 0 {
		return f()
	}
	if err := s.impl.PutLocks(s.c, itemKeys, MutationLockTimeout); err != nil {
		// this is a hard failure. No mutation can occur if we're unable to set
		// locks out. See "DANGER ZONE" in the docs.
		logging.WithError(err).Errorf(s.c, "dscache: HARD FAILURE: supportContext.mutation(): PutLocks")
		return err
	}
	err := f()
	// Note: the mutation can *eventually* succeed even if `err` is non-nil
	// here. So on errors we pessimistically keep the locks until they expire.
	if err == nil {
		if err := s.impl.DropLocks(s.c, itemKeys); err != nil {
			logging.WithError(err).Debugf(s.c, "dscache: DropLocks")
		}
	}
	return err
}

// generateNonce creates a pseudo-random sequence of bytes for use as a nonce
// using the non-cryptographic PRNG in "math/rand".
//
// The random values here are controlled entirely by the application, will never
// be shown to, or provided by, the user, so this should be fine.
func generateNonce(rnd mathrand.Rand) []byte {
	nonce := make([]byte, NonceBytes)
	_, _ = rnd.Read(nonce) // This Read will always return len(nonce), nil.
	return nonce
}

// makeMemcacheKey generates a memcache key for the given datastore Key.
func makeMemcacheKey(shard int, k *ds.Key) string {
	return fmt.Sprintf(KeyFormat, shard, hashKey(k))
}

// hashKey generates just the hashed portion of the memcache key.
func hashKey(k *ds.Key) string {
	dgst := sha1.Sum(ds.Serialize.ToBytes(k))
	return base64.RawStdEncoding.EncodeToString(dgst[:])
}
