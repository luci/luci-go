// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package datastorecache

import (
	"time"

	"github.com/luci/luci-go/common/errors"

	"golang.org/x/net/context"
)

// ErrDeleteCacheEntry is a sentinel error value that, if returned from a
// Handler's Refresh function, indicates that the cache key that is being
// refreshed is not necessary and should be deleted.
var ErrDeleteCacheEntry = errors.New("delete this cache entry")

// Handler is a cache handler for a specific type of data. It is used at cache
// runtime to make decisions on how to populate and manage cache entries.
type Handler interface {
	// FailOpen, if true, indicates that any error preventing a cache record from
	// being retrieved or used should result in an automatic Refresh attempt.
	//
	// This increases overall reliability, since a cache failure will not
	// propagate to the cache user. However, this also has the potential to mask
	// error cases by succeeding when something is really going wrong. We try and
	// mitigate the latter case with logging and metrics.
	FailOpen() bool

	// RefreshInterval is the amount of time that can expire before data becomes
	// candidate for refresh.
	//
	// This depends on the freshness of the data, and should be chosen by the
	// implementation. The only hard requirement is that this is less than the
	// PruneInterval. If this is <= 0, cached entities will never be refreshed.
	RefreshInterval(key []byte) time.Duration

	// Refresh is a callback function to refresh a given cache entity.
	//
	// This function must be concurrency-safe.
	//
	// The entity is described by key, which is the byte key for this entity. v
	// holds the current cache value for the entry; if there is no current cached
	// value, it will be a zero-value struct.
	//
	// If the ErrDeleteCacheEntry sentinel error is returned, the entity will be
	// deleted. If an error is returned, it will be propagated verbatim to the
	// caller. Otherwise, the return value will be used to update the cache
	// entity.
	Refresh(c context.Context, key []byte, v Value) (Value, error)

	// Locker returns the Locker instance to use.
	//
	// The Locker is optional, and serves to prevent multiple independent cache
	// calls for the same data from each independently refreshing that data. If
	// Locker returns nil, no such locking will be performed.
	Locker(c context.Context) Locker
}
