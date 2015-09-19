// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package memcache

// Interface is the full interface to the memcache service.
//
// The *Multi methods may return a "github.com/luci/luci-go/common/errors".MultiError
// if the rpc to the server was successful, but, e.g. some of the items were
// missing. They may also return a regular error, if, for example, the rpc
// failed outright.
type Interface interface {
	// NewItem creates a new, mutable, memcache item.
	NewItem(key string) Item

	// Add puts a single item into memcache, but only if it didn't exist in
	// memcache before.
	Add(item Item) error

	// Set the item in memcache, whether or not it exists.
	Set(item Item) error

	// Get retrieves an item from memcache.
	//
	// On a cache miss ErrCacheMiss will be returned. Item will always be
	// returned, even on a miss, but it's value may be empty if it was a miss.
	Get(key string) (Item, error)

	// Delete removes an item from memcache.
	Delete(key string) error

	// CompareAndSwap accepts an item which is the result of Get() or GetMulti().
	// The Get functions add a secret field to item ('CasID'), which is used as
	// the "compare" value for the "CompareAndSwap". The actual "Value" field of
	// the object set by the Get functions is the "swap" value.
	//
	// Example:
	//   mc := memcache.Get(context)
	//   itm := mc.NewItem("aKey")
	//   mc.Get(itm) // check error
	//   itm.SetValue(append(itm.Value(), []byte("more bytes")))
	//   mc.CompareAndSwap(itm) // check error
	CompareAndSwap(item Item) error

	// Batch operations; GetMulti takes a []Item instead of []string to improve
	// ergonomics when streamlining these operations.
	AddMulti(items []Item) error
	SetMulti(items []Item) error
	GetMulti(items []Item) error
	DeleteMulti(keys []string) error
	CompareAndSwapMulti(items []Item) error

	// Increment adds delta to the uint64 contained at key. If the memcache key
	// is missing, it's populated with initialValue before applying delta (i.e.
	// the final value would be initialValue+delta).
	//
	// Underflow caps at 0, overflow wraps back to 0.
	//
	// If key contains a value which is not exactly 8 bytes, it's assumed to
	// contain non-number data and this method will return an error.
	Increment(key string, delta int64, initialValue uint64) (newValue uint64, err error)

	// IncrementExisting is like Increment, except that the valu must exist
	// already.
	IncrementExisting(key string, delta int64) (newValue uint64, err error)

	// Flush dumps the entire memcache state.
	Flush() error

	// Stats gets some best-effort statistics about the current state of memcache.
	Stats() (*Statistics, error)

	Raw() RawInterface
}
