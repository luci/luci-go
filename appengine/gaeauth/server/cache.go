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

package server

import (
	"time"

	"golang.org/x/net/context"

	"go.chromium.org/gae/service/info"
	mc "go.chromium.org/gae/service/memcache"

	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/caching"
)

// Memcache implements auth.Cache on top of GAE memcache and per-request state.
type Memcache struct {
	Namespace string
}

var _ auth.Cache = (*Memcache)(nil)

// Get returns a cached item or (nil, nil) if it's not in the cache.
//
// Assumes callers do not modify the returned array in-place. Any returned error
// is transient error.
func (m *Memcache) Get(c context.Context, key string) ([]byte, error) {
	if val := m.getLocal(c, key); val != nil {
		return val, nil
	}
	switch itm, err := mc.GetKey(m.cacheContext(c), key); {
	case err == mc.ErrCacheMiss:
		return nil, nil
	case err != nil:
		return nil, transient.Tag.Apply(err)
	default:
		m.setLocal(c, key, itm.Value(), itm.Expiration())
		return itm.Value(), nil
	}
}

// Set unconditionally overwrites an item in the cache.
//
// If 'exp' is zero, the item will have no expiration time.
//
// Any returned error is transient error.
func (m *Memcache) Set(c context.Context, key string, value []byte, exp time.Duration) error {
	m.setLocal(c, key, value, exp)
	cc := m.cacheContext(c)
	item := mc.NewItem(cc, key).SetValue(value).SetExpiration(exp)
	if err := mc.Set(cc, item); err != nil {
		return transient.Tag.Apply(err)
	}
	return nil
}

// cacheContext returns properly namespaced luci/gae context.
func (m *Memcache) cacheContext(c context.Context) context.Context {
	return info.MustNamespace(c, m.Namespace)
}

// getLocal fetches the item from the context-bound cache, checking its
// expiration. It trusts callers not to modify the returned byte array.
func (m *Memcache) getLocal(c context.Context, key string) []byte {
	e, _ := caching.RequestCache(c).Get(c, key)
	if e != nil {
		return e.([]byte)
	}
	return nil
}

// setLocal puts a copy of 'val' in the context-bound cache.
func (m *Memcache) setLocal(c context.Context, key string, val []byte, exp time.Duration) {
	caching.RequestCache(c).Put(c, key, append([]byte(nil), val...), exp)
}
