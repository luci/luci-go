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

package flex

import (
	"time"

	"golang.org/x/net/context"
)

// gaeProcessCache is an implementation of "luci/gae"'s cloud ProcessCache on
// top of our global process cache.
type gaeProcessCache struct{}

func (gaeProcessCache) Get(c context.Context, key interface{}) interface{} {
	v, _ := ProcessCache.Get(c, key)
	return v
}

func (gaeProcessCache) Put(c context.Context, key, value interface{}, exp time.Duration) {
	// In "impl/cloud" Cache, anything <= 0 means "store, but don't expire".
	//
	// In LRU cache, an expiration of <0 means "don't store", and 0 means
	// "store, but don't expire".
	if exp < 0 {
		exp = 0
	}
	ProcessCache.Put(c, key, value, exp)
}
