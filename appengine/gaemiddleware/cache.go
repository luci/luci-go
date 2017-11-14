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

package gaemiddleware

import (
	"time"

	"golang.org/x/net/context"

	"go.chromium.org/gae/service/info"
	"go.chromium.org/gae/service/memcache"

	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/server/caching"
)

// blobCacheProvider returns caching.BlobCache implemented on top of luci/gae.
func blobCacheProvider(namespace string) caching.BlobCache {
	return &gaeBlobCache{namespace}
}

// gaeBlobCache implements caching.BlobCache.
type gaeBlobCache struct {
	ns string
}

func (g gaeBlobCache) Get(c context.Context, key string) ([]byte, error) {
	switch itm, err := memcache.GetKey(info.MustNamespace(c, g.ns), key); {
	case err == memcache.ErrCacheMiss:
		return nil, caching.ErrCacheMiss
	case err != nil:
		return nil, transient.Tag.Apply(err)
	default:
		return itm.Value(), nil
	}
}

func (g gaeBlobCache) Set(c context.Context, key string, value []byte, exp time.Duration) error {
	c = info.MustNamespace(c, g.ns)
	item := memcache.NewItem(c, key).SetValue(value).SetExpiration(exp)
	if err := memcache.Set(c, item); err != nil {
		return transient.Tag.Apply(err)
	}
	return nil
}
