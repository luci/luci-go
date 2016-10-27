// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package archive

import (
	"github.com/luci/luci-go/common/gcloud/gs"
	"github.com/luci/luci-go/logdog/common/storage/caching"

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
