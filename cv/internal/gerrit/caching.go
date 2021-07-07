// Copyright 2021 The LUCI Authors.
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

package gerrit

import (
	"context"
	"time"

	"go.chromium.org/luci/common/data/caching/lru"
)

// CachingFactory caches clients produced by another ClientFactory.
func CachingFactory(lruSize int, f ClientFactory) ClientFactory {
	cache := lru.New(lruSize)
	return func(ctx context.Context, gerritHost, luciProject string) (Client, error) {
		key := luciProject + "/" + gerritHost
		client, err := cache.GetOrCreate(ctx, key, func() (value interface{}, ttl time.Duration, err error) {
			// Default ttl of 0 means never expire. Note that specific authorization
			// token is still loaded per each request (see transport() function).
			value, err = f(ctx, gerritHost, luciProject)
			return
		})
		if err != nil {
			return nil, err
		}
		return client.(Client), nil
	}
}
