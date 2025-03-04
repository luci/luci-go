// Copyright 2025 The LUCI Authors.
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

package model

import (
	"bytes"
	"context"
	"time"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/common/tsmon/types"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/caching"
)

var taskRequestCacheOps = metric.NewCounter(
	"swarming/cache/task_request",
	"Counter of TaskRequst cache calls",
	&types.MetricMetadata{},
	field.String("outcome"), // hit, miss, error, timeout, broken
)

const (
	taskRequestCacheNamespace  = "TaskRequest"
	taskRequestCacheExpiry     = time.Hour
	taskRequestCacheRPCTimeout = 5 * time.Second
)

// FetchTaskRequest fetches TaskRequest entity from the cache or datastore.
//
// This is a best effort datastore cost optimization used by some API endpoints
// that work exclusively with one task (mostly bot APIs). It is not meant to be
// critical for performance or reliability. It often falls back to fetching from
// the datastore if the cache has problems or is slow to respond.
//
// Relies significantly on TaskRequest being immutable. Ignores datastore
// transactions (the datastore fetch, if necessary, happens
// non-transactionally).
//
// Returns datastore.ErrNoSuchEntity if there's no such task request. Any other
// error can be treated as transient.
func FetchTaskRequest(ctx context.Context, key *datastore.Key) (*TaskRequest, error) {
	if key.Kind() != "TaskRequest" {
		panic("not a TaskRequest key")
	}

	cache := caching.GlobalCache(ctx, taskRequestCacheNamespace)
	if cache == nil {
		// The cache is not configured, always fetch from the datastore.
		return fetchTaskRequest(ctx, key)
	}

	tid := RequestKeyToTaskID(key, AsRequest)

	gctx, cancel := context.WithTimeout(ctx, taskRequestCacheRPCTimeout)
	defer cancel()

	// Hit the cache first. On errors (including cache misses), hit the datastore.
	blob, err := cache.Get(gctx, tid)
	if err != nil {
		cacheMiss := false
		switch {
		case errors.Is(err, caching.ErrCacheMiss):
			taskRequestCacheOps.Add(ctx, 1, "miss")
			cacheMiss = true
		case gctx.Err() != nil:
			taskRequestCacheOps.Add(ctx, 1, "timeout")
			logging.Warningf(ctx, "Timeout fetching TaskRequest %s from cache: %s", tid, err)
		default:
			taskRequestCacheOps.Add(ctx, 1, "error")
			logging.Warningf(ctx, "Error fetching TaskRequest %s from cache: %s", tid, err)
		}
		// Hit the datastore.
		req, err := fetchTaskRequest(ctx, key)
		if err != nil {
			return nil, err
		}
		// If this was a clean cache miss, populate the cache. If it was some random
		// error, don't touch the cache (maybe it is broken and slow). Note that we
		// don't care about race conditions while populating the cache because
		// TaskRequest entities are immutable.
		if cacheMiss {
			if err := cacheTaskRequest(ctx, tid, req, cache); err != nil {
				logging.Errorf(ctx, "Failed to cache TaskRequest %s: %s", tid, err)
			}
		}
		return req, nil
	}

	// Found something in the cache.
	req, err := deserializeTaskRequest(key, blob)
	if err != nil {
		logging.Errorf(ctx, "Failed to deserialize cached TaskRequest %s: %s", tid, err)
		taskRequestCacheOps.Add(ctx, 1, "broken")
		return fetchTaskRequest(ctx, key)
	}
	taskRequestCacheOps.Add(ctx, 1, "hit")
	return req, nil
}

// cacheTaskRequest puts the given task request into the cache.
func cacheTaskRequest(ctx context.Context, tid string, req *TaskRequest, cache caching.BlobCache) error {
	blob, err := serializeTaskRequest(req)
	if err != nil {
		return errors.Annotate(err, "failed to serialize TaskRequest").Err()
	}
	ctx, cancel := context.WithTimeout(ctx, taskRequestCacheRPCTimeout)
	defer cancel()
	if err := cache.Set(ctx, tid, blob, taskRequestCacheExpiry); err != nil {
		return errors.Annotate(err, "failed to put TaskRequest into the cache").Err()
	}
	return nil
}

// fetchTaskRequest fetches TaskRequest from the datastore.
func fetchTaskRequest(ctx context.Context, key *datastore.Key) (*TaskRequest, error) {
	taskReq := &TaskRequest{Key: key}
	if err := datastore.Get(datastore.WithoutTransaction(ctx), taskReq); err != nil {
		return nil, err
	}
	return taskReq, nil
}

// serializeTaskRequest converts TaskRequest to a byte blob for the cache.
func serializeTaskRequest(req *TaskRequest) ([]byte, error) {
	pm, err := datastore.GetPLS(req).Save(false)
	if err != nil {
		return nil, err
	}
	buf := bytes.NewBuffer(make([]byte, 0, 1024))
	if err := datastore.Serialize.PropertyMap(buf, pm); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// deserializeTaskRequest is a reverse of serializeTaskRequest.
func deserializeTaskRequest(key *datastore.Key, blob []byte) (*TaskRequest, error) {
	pm, err := datastore.Deserialize.PropertyMap(bytes.NewBuffer(blob))
	if err != nil {
		return nil, err
	}
	var req TaskRequest
	if err := datastore.GetPLS(&req).Load(pm); err != nil {
		return nil, err
	}
	req.Key = key
	return &req, nil
}
