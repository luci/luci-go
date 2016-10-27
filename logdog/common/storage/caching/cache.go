// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package caching

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"time"

	"golang.org/x/net/context"
)

// Item is a single cache item. An item is uniquely identified by its Schema and
// Key sequence.
type Item struct {
	// Schema is the item's schema value. If empty, the item is schemaless.
	Schema string

	// Type is the item's type identifier. If empty, the item has no type.
	Type string

	// Key is the item's individual key. It uniquely identifies the Item within
	// the scope of the Schema and Type.
	Key string

	// Data is the Item's data. For Put, the contents of this Data will be used to
	// populate the cache. For Get, this value will be non-nil and populated with
	// the retrieved data, or nil if the item was not present in the cache.
	Data []byte
}

// Cache is a simple cache interface. It is capable of storage and retrieval.
type Cache interface {
	// Put caches the supplied Items into the cache. If an Item already exists in
	// the cache, it will be overridden.
	//
	// If exp, the supplied expiration Duration, is >0, the cache should only
	// store the data if it can expire it after this period of time.
	//
	// This method does not return whether or not the caching storage was
	// successful.
	Put(c context.Context, exp time.Duration, items ...*Item)

	// Get retrieves a cached entry. If the entry was present, a non-nil value
	// will be returned, even if the data length is zero. If the entry was not
	// present, nil will be returned.
	//
	// This method does not distinguish between an error and missing data. Either
	// valid data is returned, or it is not.
	Get(c context.Context, items ...*Item)
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
