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

package bigtable

import (
	"bytes"
	"context"
	"encoding/binary"
	"time"

	log "go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/logdog/common/storage"
	"go.chromium.org/luci/logdog/common/types"
)

// cacheSchema represents the cache schema used by this version of the tail
// cache. If the underlying data format changes, this value must also be
// updated.
const cacheSchema = "v1"

// lastTailIndexCacheDuration is the amount of time that the last tail index
// should be cached.
const lastTailIndexCacheDuration = 1 * time.Hour

// getLastTailIndex will return the cached last tail index of a given stream.
//
// If there was an error, or if the item was not cached, 0 (first index) will be
// returned.
func getLastTailIndex(c context.Context, cache storage.Cache, project string, path types.StreamPath) int64 {
	data, ok := cache.Get(c, mkLastTailKey(project, path))
	if !ok {
		return 0
	}

	v, err := binary.ReadVarint(bytes.NewReader(data))
	if err != nil {
		log.Fields{
			log.ErrorKey: err,
			"project":    project,
			"path":       path,
		}.Warningf(c, "Could not decode last tail cache.")
		return 0
	}

	log.Fields{
		"index": v,
	}.Infof(c, "Using cached tail index.")
	return v
}

func putLastTailIndex(c context.Context, cache storage.Cache, project string, path types.StreamPath, v int64) {
	buf := make([]byte, binary.MaxVarintLen64)
	buf = buf[:binary.PutVarint(buf, v)]

	cache.Put(c, mkLastTailKey(project, path), buf, lastTailIndexCacheDuration)
}

func mkLastTailKey(project string, path types.StreamPath) storage.CacheKey {
	return storage.CacheKey{
		Schema: cacheSchema,
		Type:   "bt_tail_idx",
		Key:    storage.HashKey(project, string(path)),
	}
}
