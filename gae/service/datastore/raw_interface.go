// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package datastore

import (
	"fmt"

	"golang.org/x/net/context"
)

// Cursor wraps datastore.Cursor.
type Cursor interface {
	fmt.Stringer
}

// CursorCB is used to obtain a Cursor while Run'ing a query on either
// Interface or RawInterface.
//
// it can be invoked to obtain the current cursor.
type CursorCB func() (Cursor, error)

// RawRunCB is the callback signature provided to RawInterface.Run
//
//   - key is the Key of the entity
//   - val is the data of the entity (or nil, if the query was keys-only)
//
// Return nil to continue iterating through the query results, or an error to
// stop. If you return the error `Stop`, then Run will stop the query and
// return nil.
type RawRunCB func(key *Key, val PropertyMap, getCursor CursorCB) error

// GetMultiCB is the callback signature provided to RawInterface.GetMulti
//
//   - val is the data of the entity
//     * It may be nil if some of the keys to the GetMulti were bad, since all
//       keys are validated before the RPC occurs!
//   - err is an error associated with this entity (e.g. ErrNoSuchEntity).
type GetMultiCB func(val PropertyMap, err error) error

// PutMultiCB is the callback signature provided to RawInterface.PutMulti
//
//   - key is the new key for the entity (if the original was incomplete)
//     * It may be nil if some of the keys/vals to the PutMulti were bad, since
//       all keys are validated before the RPC occurs!
//   - err is an error associated with putting this entity.
type PutMultiCB func(key *Key, err error) error

// DeleteMultiCB is the callback signature provided to RawInterface.DeleteMulti
//
//   - err is an error associated with deleting this entity.
type DeleteMultiCB func(err error) error

type nullMetaGetterType struct{}

func (nullMetaGetterType) GetMeta(string) (interface{}, bool) { return nil, false }

var nullMetaGetter MetaGetter = nullMetaGetterType{}

// MultiMetaGetter is a carrier for metadata, used with RawInterface.GetMulti
//
// It's OK to default-construct this. GetMeta will just return
// (nil, ErrMetaFieldUnset) for every index.
type MultiMetaGetter []MetaGetter

// NewMultiMetaGetter returns a new MultiMetaGetter object. data may be nil.
func NewMultiMetaGetter(data []PropertyMap) MultiMetaGetter {
	if len(data) == 0 {
		return nil
	}
	inner := make(MultiMetaGetter, len(data))
	for i, pm := range data {
		inner[i] = pm
	}
	return inner
}

// GetMeta is like PropertyLoadSaver.GetMeta, but it also takes an index
// indicating which slot you want metadata for. If idx isn't there, this
// returns (nil, ErrMetaFieldUnset).
func (m MultiMetaGetter) GetMeta(idx int, key string) (interface{}, bool) {
	return m.GetSingle(idx).GetMeta(key)
}

// GetSingle gets a single MetaGetter at the specified index.
func (m MultiMetaGetter) GetSingle(idx int) MetaGetter {
	if idx >= len(m) || m[idx] == nil {
		return nullMetaGetter
	}
	return m[idx]
}

// RawInterface implements the datastore functionality without any of the fancy
// reflection stuff. This is so that Filters can avoid doing lots of redundant
// reflection work. See datastore.Interface for a more user-friendly interface.
type RawInterface interface {
	// AllocateIDs allows you to allocate IDs from the datastore without putting
	// any data. `incomplete` must be a PartialValid Key. If there's no error,
	// a contiguous block of IDs of n length starting at `start` will be reserved
	// indefinitely for the user application code for use in new keys. The
	// appengine automatic ID generator will never automatically assign these IDs
	// for Keys of this type.
	AllocateIDs(incomplete *Key, n int) (start int64, err error)

	// RunInTransaction runs f in a transaction.
	//
	// opts may be nil.
	//
	// NOTE: Implementations and filters are guaranteed that:
	//   - f is not nil
	RunInTransaction(f func(c context.Context) error, opts *TransactionOptions) error

	// DecodeCursor converts a string returned by a Cursor into a Cursor instance.
	// It will return an error if the supplied string is not valid, or could not
	// be decoded by the implementation.
	DecodeCursor(s string) (Cursor, error)

	// Run executes the given query, and calls `cb` for each successfully item.
	//
	// NOTE: Implementations and filters are guaranteed that:
	//   - query is not nil
	//   - cb is not nil
	Run(q *FinalizedQuery, cb RawRunCB) error

	// Count executes the given query and returns the number of entries which
	// match it.
	Count(q *FinalizedQuery) (int64, error)

	// GetMulti retrieves items from the datastore.
	//
	// Callback execues once per key, in the order of keys. Callback may not
	// execute at all if there's a server error. If callback is nil, this
	// method does nothing.
	//
	// meta is used to propagate metadata from higher levels.
	//
	// NOTE: Implementations and filters are guaranteed that:
	//   - len(keys) > 0
	//   - all keys are Valid, !Incomplete, and in the current namespace
	//   - cb is not nil
	GetMulti(keys []*Key, meta MultiMetaGetter, cb GetMultiCB) error

	// PutMulti writes items to the datastore.
	//
	// Callback execues once per key/value pair, in the passed-in order. Callback
	// may not execute at all if there was a server error.
	//
	// NOTE: Implementations and filters are guaranteed that:
	//   - len(keys) > 0
	//   - len(keys) == len(vals)
	//   - all keys are Valid and in the current namespace
	//   - cb is not nil
	PutMulti(keys []*Key, vals []PropertyMap, cb PutMultiCB) error

	// DeleteMulti removes items from the datastore.
	//
	// Callback execues once per key, in the order of keys. Callback may not
	// execute at all if there's a server error.
	//
	// NOTE: Implementations and filters are guaranteed that
	//   - len(keys) > 0
	//   - all keys are Valid, !Incomplete, and in the current namespace
	//   - none keys of the keys are 'special' (use a kind prefixed with '__')
	//   - cb is not nil
	DeleteMulti(keys []*Key, cb DeleteMultiCB) error

	// Testable returns the Testable interface for the implementation, or nil if
	// there is none.
	Testable() Testable
}
