// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package dscache

import (
	"fmt"
	"time"

	ds "github.com/luci/gae/service/datastore"
	mc "github.com/luci/gae/service/memcache"

	"github.com/luci/luci-go/common/data/rand/mathrand"
	"github.com/luci/luci-go/common/errors"
	log "github.com/luci/luci-go/common/logging"

	"golang.org/x/net/context"
)

type supportContext struct {
	ds.KeyContext

	c            context.Context
	mr           mathrand.Rand
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

func (s *supportContext) mkRandKeys(keys []*ds.Key, metas ds.MultiMetaGetter) []string {
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
		if ret == nil {
			ret = make([]string, len(keys))
		}
		ret[i] = MakeMemcacheKey(s.mr.Intn(shards), key)
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
			keySuffix := HashKey(key)
			for shard := 0; shard < nums[i]; shard++ {
				ret = append(ret, fmt.Sprintf(KeyFormat, shard, keySuffix))
			}
		}
	}
	return ret
}

func (s *supportContext) mutation(keys []*ds.Key, f func() error) error {
	lockItems, lockKeys := s.mkAllLockItems(keys)
	if lockItems == nil {
		return f()
	}
	if err := mc.Set(s.c, lockItems...); err != nil {
		// this is a hard failure. No mutation can occur if we're unable to set
		// locks out. See "DANGER ZONE" in the docs.
		(log.Fields{log.ErrorKey: err}).Errorf(
			s.c, "dscache: HARD FAILURE: supportContext.mutation(): mc.SetMulti")
		return err
	}
	err := f()
	if err == nil {
		if err := errors.Filter(mc.Delete(s.c, lockKeys...), mc.ErrCacheMiss); err != nil {
			(log.Fields{log.ErrorKey: err}).Debugf(
				s.c, "dscache: mc.Delete")
		}
	}
	return err
}

func (s *supportContext) mkRandLockItems(keys []*ds.Key, metas ds.MultiMetaGetter) ([]mc.Item, []byte) {
	mcKeys := s.mkRandKeys(keys, metas)
	if len(mcKeys) == 0 {
		return nil, nil
	}
	nonce := s.generateNonce()
	ret := make([]mc.Item, len(mcKeys))
	for i, k := range mcKeys {
		if k == "" {
			continue
		}
		ret[i] = (mc.NewItem(s.c, k).
			SetFlags(uint32(ItemHasLock)).
			SetExpiration(time.Second * time.Duration(LockTimeSeconds)).
			SetValue(nonce))
	}
	return ret, nonce
}

func (s *supportContext) mkAllLockItems(keys []*ds.Key) ([]mc.Item, []string) {
	mcKeys := s.mkAllKeys(keys)
	if mcKeys == nil {
		return nil, nil
	}
	ret := make([]mc.Item, len(mcKeys))
	for i := range ret {
		ret[i] = (mc.NewItem(s.c, mcKeys[i]).
			SetFlags(uint32(ItemHasLock)).
			SetExpiration(time.Second * time.Duration(LockTimeSeconds)))
	}
	return ret, mcKeys
}

// generateNonce creates a pseudo-random sequence of bytes for use as a nonce
// usingthe non-cryptographic PRNG in "math/rand".
//
// The random values here are controlled entriely by the application, will never
// be shown to, or provided by, the user, so this should be fine.
func (s *supportContext) generateNonce() []byte {
	nonce := make([]byte, NonceBytes)
	_, _ = s.mr.Read(nonce) // This Read will always return len(nonce), nil.
	return nonce
}
