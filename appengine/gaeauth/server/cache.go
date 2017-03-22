// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package server

import (
	"time"

	"golang.org/x/net/context"

	"github.com/luci/gae/service/info"
	mc "github.com/luci/gae/service/memcache"

	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/data/caching/proccache"
	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/server/auth"
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
		return nil, errors.WrapTransient(err)
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
		return errors.WrapTransient(err)
	}
	return nil
}

// cacheContext returns properly namespaced luci/gae context.
func (m *Memcache) cacheContext(c context.Context) context.Context {
	return info.MustNamespace(c, m.Namespace)
}

type requestCacheCtxKey *Memcache

// UseRequestCache initializes context-bound local cache.
//
// It will be used as a second fast layer of caching in front of memcache. This
// cache is private to 'm' instance and accessible only from within 'm'. It is
// never trimmed, only released at once upon the request completion.
//
// This is optional.
func (m *Memcache) UseRequestCache(c context.Context) context.Context {
	return context.WithValue(c, requestCacheCtxKey(m), &proccache.Cache{})
}

// requestCache returns a context-bound cache set up in UseRequestCache or nil.
func (m *Memcache) requestCache(c context.Context) *proccache.Cache {
	pc, _ := c.Value(requestCacheCtxKey(m)).(*proccache.Cache)
	return pc
}

// getLocal fetches the item from the context-bound cache, checking its
// expiration. It trusts callers not to modify the returned byte array.
func (m *Memcache) getLocal(c context.Context, key string) []byte {
	if pc := m.requestCache(c); pc != nil {
		if e := pc.Get(key); e != nil && (e.Exp.IsZero() || clock.Now(c).Before(e.Exp)) {
			return e.Value.([]byte)
		}
	}
	return nil
}

// setLocal puts a copy of 'val' in the context-bound cache.
func (m *Memcache) setLocal(c context.Context, key string, val []byte, exp time.Duration) {
	if pc := m.requestCache(c); pc != nil {
		expTs := time.Time{}
		if exp != 0 {
			expTs = clock.Now(c).Add(exp)
		}
		pc.Put(key, append([]byte(nil), val...), expTs)
	}
}
