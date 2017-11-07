// Copyright 2017 The LUCI Authors.
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

package caching

import (
	"golang.org/x/net/context"

	"go.chromium.org/luci/common/data/caching/lru"
)

var requestCacheKey = "server.caching Per-Request Cache"

// WithRequestCache initializes context-bound local cache and adds it to the
// Context.
//
// The cache has unbounded size. This is fine, since the lifetime of the cache
// is still scoped to a single request.
//
// It can be used as a second fast layer of caching in front of memcache.
// It is never trimmed, only released at once upon the request completion.
func WithRequestCache(c context.Context) context.Context {
	return context.WithValue(c, &requestCacheKey, lru.New(0))
}

// RequestCache retrieves a per-request in-memory cache of the Context. If no
// request cache is installed, this will panic.
func RequestCache(c context.Context) *lru.Cache {
	rc, _ := c.Value(&requestCacheKey).(*lru.Cache)
	if rc == nil {
		panic("server/caching: no request cache installed in Context")
	}
	return rc
}
