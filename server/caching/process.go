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

var processCacheKey = "server.caching Process Cache"

// WithProcessCache installs a process-global cache into the supplied Context.
//
// It can be used as a fast layer of caching for information whose value extends
// past the lifespan of a single request.
//
// For information whose cached value is limited to a single request, use
// RequestCache.
func WithProcessCache(c context.Context, cache *lru.Cache) context.Context {
	return context.WithValue(c, &processCacheKey, cache)
}

// ProcessCache returns process-global cache that is installed into the Context.
//
// If no process-global cache is installed, this will panic.
func ProcessCache(c context.Context) *lru.Cache {
	pc, _ := c.Value(&processCacheKey).(*lru.Cache)
	if pc == nil {
		panic("server/caching: no process cache installed in Context")
	}
	return pc
}
