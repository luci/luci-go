// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package dscache

import (
	"fmt"
	"math/rand"
	"time"

	ds "github.com/luci/gae/service/datastore"
	"github.com/luci/gae/service/memcache"
	"github.com/luci/luci-go/common/errors"
	log "github.com/luci/luci-go/common/logging"
	"golang.org/x/net/context"
)

type supportContext struct {
	aid string
	ns  string

	c            context.Context
	mc           memcache.Interface
	mr           *rand.Rand
	shardsForKey func(*ds.Key) int
}

func (s *supportContext) numShards(k *ds.Key) int {
	ret := DefaultShards
	if s.shardsForKey != nil {
		ret = s.shardsForKey(k)
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
		if !key.Incomplete() {
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
		if !key.Incomplete() {
			keySuffix := HashKey(key)
			for shard := 0; shard < nums[i]; shard++ {
				ret = append(ret, fmt.Sprintf(KeyFormat, shard, keySuffix))
			}
		}
	}
	return ret
}

// crappyNonce creates a really crappy nonce using math/rand. This is generally
// unacceptable for cryptographic purposes, but since mathrand is the only
// mocked randomness source, we use that.
//
// The random values here are controlled entriely by the application, will never
// be shown to, or provided by, the user, so this should be fine.
//
// Do not use this function for anything other than mkRandLockItems or your hair
// will fall out. You've been warned.
func (s *supportContext) crappyNonce() []byte {
	ret := make([]byte, NonceUint32s*4)
	for w := uint(0); w < NonceUint32s; w++ {
		word := s.mr.Uint32()
		for i := uint(0); i < 4; i++ {
			ret[(w*4)+i] = byte(word >> (8 * i))
		}
	}
	return ret
}

func (s *supportContext) mutation(keys []*ds.Key, f func() error) error {
	lockItems, lockKeys := s.mkAllLockItems(keys)
	if lockItems == nil {
		return f()
	}
	if err := s.mc.SetMulti(lockItems); err != nil {
		// this is a hard failure. No mutation can occur if we're unable to set
		// locks out. See "DANGER ZONE" in the docs.
		(log.Fields{log.ErrorKey: err}).Errorf(
			s.c, "dscache: HARD FAILURE: supportContext.mutation(): mc.SetMulti")
		return err
	}
	err := f()
	if err == nil {
		if err := errors.Filter(s.mc.DeleteMulti(lockKeys), memcache.ErrCacheMiss); err != nil {
			(log.Fields{log.ErrorKey: err}).Debugf(
				s.c, "dscache: mc.DeleteMulti")
		}
	}
	return err
}

func (s *supportContext) mkRandLockItems(keys []*ds.Key, metas ds.MultiMetaGetter) ([]memcache.Item, []byte) {
	mcKeys := s.mkRandKeys(keys, metas)
	if len(mcKeys) == 0 {
		return nil, nil
	}
	nonce := s.crappyNonce()
	ret := make([]memcache.Item, len(mcKeys))
	for i, k := range mcKeys {
		if k == "" {
			continue
		}
		ret[i] = (s.mc.NewItem(k).
			SetFlags(uint32(ItemHasLock)).
			SetExpiration(time.Second * time.Duration(LockTimeSeconds)).
			SetValue(nonce))
	}
	return ret, nonce
}

func (s *supportContext) mkAllLockItems(keys []*ds.Key) ([]memcache.Item, []string) {
	mcKeys := s.mkAllKeys(keys)
	if mcKeys == nil {
		return nil, nil
	}
	ret := make([]memcache.Item, len(mcKeys))
	for i := range ret {
		ret[i] = (s.mc.NewItem(mcKeys[i]).
			SetFlags(uint32(ItemHasLock)).
			SetExpiration(time.Second * time.Duration(LockTimeSeconds)))
	}
	return ret, mcKeys
}
