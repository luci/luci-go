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

package authdb

import (
	"time"

	"golang.org/x/net/context"

	"go.chromium.org/luci/common/data/caching/lazyslot"
	"go.chromium.org/luci/common/data/rand/mathrand"
)

// DBCacheUpdater knows how to update local in-memory copy of DB.
//
// Used by NewDBCache.
type DBCacheUpdater func(c context.Context, prev DB) (DB, error)

// NewDBCache returns a provider of DB instances that uses local memory to
// cache DB instances for 5-10 seconds. It uses supplied callback to refetch DB
// from some permanent storage when cache expires.
//
// Even though the return value is technically a function, treat it as a heavy
// stateful object, since it has the cache of DB in its closure.
func NewDBCache(updater DBCacheUpdater) func(c context.Context) (DB, error) {
	cacheSlot := lazyslot.Slot{}
	return func(c context.Context) (DB, error) {
		val, err := cacheSlot.Get(c, func(prev interface{}) (db interface{}, exp time.Duration, err error) {
			prevDB, _ := prev.(DB)
			if db, err = updater(c, prevDB); err == nil {
				exp = 5*time.Second + time.Duration(mathrand.Get(c).Intn(5000))*time.Millisecond
			}
			return
		})
		if err != nil {
			return nil, err
		}
		return val.(DB), nil
	}
}
