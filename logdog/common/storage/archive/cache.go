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
	"go.chromium.org/luci/common/gcloud/gs"
	"go.chromium.org/luci/logdog/common/storage/caching"

	"golang.org/x/net/context"
)

// cacheSchema represents the cache schema used by this version of the tail
// cache. If the underlying data format changes, this value must also be
// updated.
const cacheSchema = "v1"

func getCachedLogIndexData(c context.Context, cache caching.Cache, path gs.Path) []byte {
	itm := mkCachedLogIndexDataItem(path)
	cache.Get(c, itm)
	return itm.Data
}

func putCachedLogIndexData(c context.Context, cache caching.Cache, path gs.Path, indexData []byte) {
	itm := mkCachedLogIndexDataItem(path)
	itm.Data = indexData
	cache.Put(c, 0, itm)
}

func mkCachedLogIndexDataItem(path gs.Path) *caching.Item {
	return &caching.Item{
		Schema: cacheSchema,
		Type:   "archive_log_index",
		Key:    caching.HashKey(string(path)),
	}
}
