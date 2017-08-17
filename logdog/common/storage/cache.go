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

package storage

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"time"

	"golang.org/x/net/context"
)

// CacheKey is a single cache item's key.
type CacheKey struct {
	// Schema is the item's schema value. If empty, the item is schemaless.
	Schema string

	// Type is the item's type identifier. If empty, the item has no type.
	Type string

	// Key is the item's individual key. It uniquely identifies the Item within
	// the scope of the Schema and Type.
	Key string
}

// Cache is a simple cache interface. It is capable of storage and retrieval.
type Cache interface {
	// Put caches an Items into the cache. If an item already exists in the cache,
	// it will be overridden.
	//
	// The data in val may be directly retained by the cache, and the caller
	// promises not to mutate it after being passed to Put.
	//
	// If exp, the supplied expiration Duration, is >0, the cache should only
	// store the data if it can expire it after this period of time.
	//
	// This method does not return whether or not the caching storage was
	// successful.
	Put(c context.Context, key CacheKey, val []byte, exp time.Duration)

	// Get retrieves a cached entry. If the entry was present, the returned
	// boolean will be true.
	//
	// The returned byte slice may not be modified by the caller.
	//
	// This method does not distinguish between an error and missing data. Either
	// valid data is returned, or it is not.
	Get(c context.Context, key CacheKey) ([]byte, bool)
}

// HashKey composes a hex-encoded SHA256 string from the supplied parts. The
// local schema version and KeyType are included in the hash.
func HashKey(parts ...string) string {
	hash := sha256.New()
	bio := bufio.NewWriter(hash)

	// Add a full NULL-delimited key sequence segment to our hash.
	for i, part := range parts {
		// Write the separator, except for the first key.
		if i > 0 {
			if _, err := bio.WriteRune('\x00'); err != nil {
				panic(err)
			}
		}

		if _, err := bio.WriteString(part); err != nil {
			panic(err)
		}
	}
	if err := bio.Flush(); err != nil {
		panic(err)
	}

	// Return the hex-encoded hash sum.
	return hex.EncodeToString(hash.Sum(nil))
}
