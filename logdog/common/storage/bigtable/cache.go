// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package bigtable

import (
	"bytes"
	"encoding/binary"
	"time"

	"github.com/luci/luci-go/common/config"
	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/logdog/common/storage/caching"
	"github.com/luci/luci-go/logdog/common/types"

	"golang.org/x/net/context"
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
func getLastTailIndex(c context.Context, cache caching.Cache, project config.ProjectName, path types.StreamPath) int64 {
	itm := mkLastTailItem(project, path)
	cache.Get(c, itm)
	if itm.Data == nil {
		return 0
	}

	v, err := binary.ReadVarint(bytes.NewReader(itm.Data))
	if err != nil {
		log.Fields{
			log.ErrorKey: err,
			"project":    project,
			"path":       path,
		}.Warningf(c, "Could not decode last tail cache.")
		return 0
	}

	return v
}

func putLastTailIndex(c context.Context, cache caching.Cache, project config.ProjectName, path types.StreamPath, v int64) {
	buf := make([]byte, binary.MaxVarintLen64)
	buf = buf[:binary.PutVarint(buf, v)]

	itm := mkLastTailItem(project, path)
	itm.Data = buf
	cache.Put(c, lastTailIndexCacheDuration, itm)
}

func mkLastTailItem(project config.ProjectName, path types.StreamPath) *caching.Item {
	return &caching.Item{
		Schema: cacheSchema,
		Type:   "bt_tail_idx",
		Key:    caching.HashKey(string(project), string(path)),
	}
}
