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

package archive

import (
	"context"

	"go.chromium.org/luci/common/gcloud/gs"

	"go.chromium.org/luci/logdog/common/storage"
)

// cacheSchema represents the cache schema used by this version of the tail
// cache. If the underlying data format changes, this value must also be
// updated.
const cacheSchema = "v1"

func getCachedLogIndexData(c context.Context, cache storage.Cache, path gs.Path) ([]byte, bool) {
	return cache.Get(c, mkCachedLogIndexDataKey(path))
}

func putCachedLogIndexData(c context.Context, cache storage.Cache, path gs.Path, indexData []byte) {
	cache.Put(c, mkCachedLogIndexDataKey(path), indexData, 0)
}

func mkCachedLogIndexDataKey(path gs.Path) storage.CacheKey {
	return storage.CacheKey{
		Schema: cacheSchema,
		Type:   "archive_log_index",
		Key:    storage.HashKey(string(path)),
	}
}
