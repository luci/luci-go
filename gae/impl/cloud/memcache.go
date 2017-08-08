// Copyright 2016 The LUCI Authors.
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

package cloud

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"time"

	"go.chromium.org/gae/service/info"
	mc "go.chromium.org/gae/service/memcache"

	"github.com/bradfitz/gomemcache/memcache"
	"golang.org/x/net/context"
)

const (
	// memcacheKeyPrefix is the common prefix prepended to memcached keys created
	// by this package. It is intended to ensure that keys do not conflict with
	// other users of the service.
	memcacheKeyPrefix = "go.chromium.org/gae/impl/cloud:"

	// keyHashSizeThreshold is a threshold for key hashing. If the key's length
	// exceeds this threshold, the key will be hashed and the hash used in its
	// place with the memcache service.
	//
	// This is an implementation detail, but will be visible to the user in the
	// Key field of the memcache entry on retrieval.
	keyHashSizeThreshold = 250
)

// memcacheClient is a "service/memcache" implementation built on top of a
// "memcached" client connection.
//
// Because "memcached" has no concept of a namespace, we differentiate memcache
// entries by prepending "memcacheKeyPrefix:SHA256(namespace):" to each key.
type memcacheClient struct {
	client *memcache.Client
}

func (m *memcacheClient) use(c context.Context) context.Context {
	return mc.SetRawFactory(c, func(ic context.Context) mc.RawInterface {
		return bindMemcacheClient(m, info.GetNamespace(ic))
	})
}

type memcacheItem struct {
	native *memcache.Item
}

func (it *memcacheItem) Key() string   { return it.native.Key }
func (it *memcacheItem) Value() []byte { return it.native.Value }
func (it *memcacheItem) Flags() uint32 { return it.native.Flags }
func (it *memcacheItem) Expiration() time.Duration {
	return time.Duration(it.native.Expiration) * time.Second
}

func (it *memcacheItem) SetKey(v string) mc.Item {
	it.native.Key = v
	return it
}

func (it *memcacheItem) SetValue(v []byte) mc.Item {
	it.native.Value = v
	return it
}

func (it *memcacheItem) SetFlags(v uint32) mc.Item {
	it.native.Flags = v
	return it
}

func (it *memcacheItem) SetExpiration(v time.Duration) mc.Item {
	it.native.Expiration = int32(v.Seconds())
	return it
}

func (it *memcacheItem) SetAll(other mc.Item) {
	origKey := it.native.Key

	var on memcache.Item
	if other != nil {
		on = *(other.(*memcacheItem).native)
	}
	it.native = &on
	it.native.Key = origKey
}

func hashBytes(b []byte) string {
	hash := sha256.Sum256(b)
	return hex.EncodeToString(hash[:])
}

type boundMemcacheClient struct {
	*memcacheClient
	keyPrefix string
}

func bindMemcacheClient(mc *memcacheClient, ns string) *boundMemcacheClient {
	nsPrefix := hashBytes([]byte(ns))
	return &boundMemcacheClient{
		memcacheClient: mc,
		keyPrefix:      memcacheKeyPrefix + nsPrefix + ":",
	}
}

func (*boundMemcacheClient) newMemcacheItem(nativeKey string) *memcacheItem {
	return &memcacheItem{
		native: &memcache.Item{
			Key: nativeKey,
		},
	}
}

// makeKey constructs the actual key used with the memcache service. This
// includes a service-specific prefix, the key's namespace, and the key itself.
// If the key's length exceeds the keyHashSizeThreshold, the key will be stored
// as a hash.
func (bmc *boundMemcacheClient) makeKey(base string) string {
	if len(base) > keyHashSizeThreshold {
		base = hashBytes([]byte(base))
	}
	return bmc.keyPrefix + base
}

func (bmc *boundMemcacheClient) userKey(key string) string { return key[len(bmc.keyPrefix):] }

func (bmc *boundMemcacheClient) nativeItem(itm mc.Item) *memcache.Item {
	ni := *(itm.(*memcacheItem).native)
	ni.Key = bmc.makeKey(ni.Key)
	return &ni
}

func (bmc *boundMemcacheClient) NewItem(key string) mc.Item { return bmc.newMemcacheItem(key) }

func (bmc *boundMemcacheClient) AddMulti(items []mc.Item, cb mc.RawCB) error {
	for _, itm := range items {
		err := bmc.client.Add(bmc.nativeItem(itm))
		cb(bmc.translateErr(err))
	}
	return nil
}

func (bmc *boundMemcacheClient) SetMulti(items []mc.Item, cb mc.RawCB) error {
	for _, itm := range items {
		err := bmc.client.Set(bmc.nativeItem(itm))
		cb(bmc.translateErr(err))
	}
	return nil
}

func (bmc *boundMemcacheClient) GetMulti(keys []string, cb mc.RawItemCB) error {
	nativeKeys := make([]string, len(keys))
	for i, key := range keys {
		nativeKeys[i] = bmc.makeKey(key)
	}

	itemMap, err := bmc.client.GetMulti(nativeKeys)
	if err != nil {
		return bmc.translateErr(err)
	}

	// Translate the item keys back to user keys.
	for _, v := range itemMap {
		v.Key = bmc.userKey(v.Key)
	}

	for _, k := range nativeKeys {
		if it := itemMap[k]; it != nil {
			cb(&memcacheItem{native: it}, nil)
		} else {
			cb(nil, mc.ErrCacheMiss)
		}
	}
	return nil
}

func (bmc *boundMemcacheClient) DeleteMulti(keys []string, cb mc.RawCB) error {
	for _, k := range keys {
		err := bmc.client.Delete(bmc.makeKey(k))
		cb(bmc.translateErr(err))
	}
	return nil
}

func (bmc *boundMemcacheClient) CompareAndSwapMulti(items []mc.Item, cb mc.RawCB) error {
	for _, itm := range items {
		err := bmc.client.CompareAndSwap(bmc.nativeItem(itm))
		cb(bmc.translateErr(err))
	}
	return nil
}

func (bmc *boundMemcacheClient) Increment(key string, delta int64, initialValue *uint64) (uint64, error) {
	// key is now the native key (namespaced).
	key = bmc.makeKey(key)

	op := func() (newValue uint64, err error) {
		switch {
		case delta > 0:
			newValue, err = bmc.client.Increment(key, uint64(delta))
		case delta < 0:
			newValue, err = bmc.client.Decrement(key, uint64(-delta))
		default:
			// We don't want to change the value, but we want to return ErrNotStored
			// if the value doesn't exist. Use Get.
			_, err = bmc.client.Get(key)
		}
		err = bmc.translateErr(err)
		return
	}

	if initialValue == nil {
		return op()
	}

	// The Memcache service doesn't have an "IncrementExisting" equivalent. We
	// will emulate this with other memcache operations, using Add to set the
	// initial value if appropriate.
	var (
		itm *memcacheItem
		iv  = *initialValue
	)
	for {
		// Perform compare-and-swap.
		nv, err := op()
		if err != mc.ErrCacheMiss {
			return nv, err
		}

		// The value doesn't exist. Use "Add" to set the initial value. We will
		// calculate the "initial value" as if delta were applied so we can do this
		// in one operation.
		//
		// We only need to do this once per invocation, so we will use "itm == nil"
		// as a sentinel for uninitialized.
		if itm == nil {
			// Overflow wraps around (to zero), and underflow is capped at 0.
			if delta < 0 {
				udelta := uint64(-delta)
				if udelta >= iv {
					// Would underflow, cap at 0.
					iv = 0
				} else {
					iv -= udelta
				}
			} else {
				// Apply delta. This will automatically wrap on overflow.
				iv += uint64(delta)
			}

			itm = bmc.newMemcacheItem(key)
			itm.SetValue([]byte(strconv.FormatUint(iv, 10)))
		}
		switch err := bmc.client.Add(itm.native); err {
		case nil:
			// Item was successfully set.
			return iv, nil

		case mc.ErrNotStored:
			// Something else set it in between "op" and "Add". Try "op" again.
			break

		default:
			return 0, err
		}
	}
}

func (bmc *boundMemcacheClient) Flush() error {
	// Unfortunately there's not really a good way to flush just a single
	// namespace, so Flush will flush all memcache.
	return bmc.translateErr(bmc.client.FlushAll())
}

func (bmc *boundMemcacheClient) Stats() (*mc.Statistics, error) { return nil, mc.ErrNoStats }

func (*boundMemcacheClient) translateErr(err error) error {
	switch err {
	case memcache.ErrCacheMiss:
		return mc.ErrCacheMiss
	case memcache.ErrCASConflict:
		return mc.ErrCASConflict
	case memcache.ErrNotStored:
		return mc.ErrNotStored
	case memcache.ErrServerError:
		return mc.ErrServerError
	case memcache.ErrNoStats:
		return mc.ErrNoStats
	default:
		return err
	}
}
