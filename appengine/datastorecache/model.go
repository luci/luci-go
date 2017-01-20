// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package datastorecache

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"sync"
	"time"

	"github.com/luci/gae/service/datastore"
)

// entry is a single cached value.
//
// An entry is keyed on the hash of its Key via its PropertyLoadSaver.
type entry struct {
	// Key is the cache entry's key, which uniquely identifies this cache entry.
	//
	// The Key for a given cache entry must be unique within that cache's
	// Namespace and Name.
	Key []byte `gae:",noindex"`
	// CacheName is the name of the Cache that this entry belongs to. This can be
	// empty.
	CacheName string `gae:",noindex"`

	// Created is the time when this entity was initially created.
	Created time.Time `gae:",noindex"`

	// LastAccessed is the last recorded access time for this entry. This is
	// lazily updated, as its only operational requirement is to be a few times
	// smaller than this entry's Handler's PruneInterval.
	LastAccessed time.Time `gae:",noindex"`

	// LastRefreshed is the last time that this entry was refreshed.
	//
	// The maintenance task will attempt to refresh this entry if its
	// LastRefreshed time is near or outside of its Handler's RefreshInterval.
	LastRefreshed time.Time `gae:",noindex"`
	// LastRefreshDelta is a time.Duration representing the amount of time this
	// entry took to refresh.
	LastRefreshDelta int64

	// Data is the entry's cached data.
	Data []byte `gae:",noindex"`

	// The cache Value's Schema (may be empty).
	Schema string `gae:",noindex"`

	// The cache Value's description string (may be empty).
	Description string `gae:",noindex"`

	// Retained key hash. We sync here so this is goroutine-safe. This will
	// obviously not work if Key is changed in between hash calculations, but no
	// code will do this.
	initOnce    sync.Once
	calcKind    string
	calcKeyHash string
}

var _ datastore.MetaGetterSetter = (*entry)(nil)

func (e *entry) initialize() {
	e.initOnce.Do(func() {
		// Calculate our Kind.
		kindParts := []string{"DSCacheEntry", ""}[:1]

		cacheName := []byte(e.CacheName)
		if len(cacheName) > 0 {
			hash := sha256.Sum256(cacheName)
			kindParts = append(kindParts, hex.EncodeToString(hash[:]))
		}
		e.calcKind = strings.Join(kindParts, "_")

		// Calculate our key hash.
		hash := sha256.New()
		if len(cacheName) > 0 {
			_, _ = hash.Write([]byte{0x00})
			_, _ = hash.Write(cacheName)
		}
		_, _ = hash.Write(e.Key)
		e.calcKeyHash = hex.EncodeToString(hash.Sum(nil))
	})
}

// keyHash generates the hex-encoded entry hash. This incorporates the entry's
// CacheName (if present) and Kind.
func (e *entry) keyHash() string {
	e.initialize()
	return e.calcKeyHash
}

func (e *entry) kind() string {
	e.initialize()
	return e.calcKind
}

func (e *entry) toValue() Value {
	return Value{
		Schema:      e.Schema,
		Data:        e.Data,
		Description: e.Description,
	}
}

func (e *entry) loadValue(v Value) {
	e.Schema, e.Data, e.Description = v.Schema, v.Data, v.Description
}

func (e *entry) GetMeta(key string) (interface{}, bool) {
	switch key {
	case "kind":
		return e.kind(), true
	case "id":
		return e.keyHash(), true
	}
	return datastore.GetPLS(e).GetMeta(key)
}

func (e *entry) GetAllMeta() datastore.PropertyMap {
	return datastore.PropertyMap{
		"$kind": datastore.MkProperty(e.kind()),
		"$id":   datastore.MkProperty(e.keyHash()),
	}
}

func (e *entry) SetMeta(key string, val interface{}) bool {
	return datastore.GetPLS(e).SetMeta(key, val)
}

func (e *entry) lockKey() string {
	return "datastore_cache_entry_" + e.keyHash()
}

// managerShardStats are per-shard stats kept by manager runs.
type managerShardStats struct {
	// Kind specifies the datastore entity kind.
	Kind string `gae:"$kind,ManagerShardStats"`

	// Shard is the shard ID for this stats entry.
	Shard int `gae:"$id"`

	// The last time that this manager was successfully run.
	LastSuccessfulRun time.Time `gae:",noindex"`
	// The total number of cache entries identified/handled in the last round.
	LastEntryCount int `gae:",noindex"`
}
