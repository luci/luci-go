// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package datastore

import (
	"fmt"

	"golang.org/x/net/context"
)

// Key is the equivalent of *datastore.Key from the original SDK, except that
// it can have multiple implementations. See helper.Key* methods for missing
// methods like KeyIncomplete (and some new ones like KeyValid).
type Key interface {
	Kind() string
	StringID() string
	IntID() int64
	Parent() Key
	AppID() string
	Namespace() string

	String() string
}

// KeyTok is a single token from a multi-part Key.
type KeyTok struct {
	Kind     string
	IntID    int64
	StringID string
}

// Cursor wraps datastore.Cursor.
type Cursor interface {
	fmt.Stringer
}

// Query wraps datastore.Query.
type Query interface {
	Ancestor(ancestor Key) Query
	Distinct() Query
	End(c Cursor) Query
	EventualConsistency() Query
	Filter(filterStr string, value interface{}) Query
	KeysOnly() Query
	Limit(limit int) Query
	Offset(offset int) Query
	Order(fieldName string) Query
	Project(fieldNames ...string) Query
	Start(c Cursor) Query
}

// RawRunCB is the callback signature provided to RawInterface.Run
//
//   - key is the Key of the entity
//   - val is the data of the entity (or nil, if the query was keys-only)
//   - getCursor can be invoked to obtain the current cursor.
//
// Return true to continue iterating through the query results, or false to stop.
type RawRunCB func(key Key, val PropertyMap, getCursor func() (Cursor, error)) bool

// GetMultiCB is the callback signature provided to RawInterface.GetMulti
//
//   - val is the data of the entity
//     * It may be nil if some of the keys to the GetMulti were bad, since all
//       keys are validated before the RPC occurs!
//   - err is an error associated with this entity (e.g. ErrNoSuchEntity).
type GetMultiCB func(val PropertyMap, err error)

// PutMultiCB is the callback signature provided to RawInterface.PutMulti
//
//   - key is the new key for the entity (if the original was incomplete)
//     * It may be nil if some of the keys/vals to the PutMulti were bad, since
//       all keys are validated before the RPC occurs!
//   - err is an error associated with putting this entity.
type PutMultiCB func(key Key, err error)

// DeleteMultiCB is the callback signature provided to RawInterface.DeleteMulti
//
//   - err is an error associated with deleting this entity.
type DeleteMultiCB func(err error)

type nullMetaGetterType struct{}

func (nullMetaGetterType) GetMeta(string) (interface{}, error)                   { return nil, ErrMetaFieldUnset }
func (nullMetaGetterType) GetMetaDefault(_ string, dflt interface{}) interface{} { return dflt }

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
func (m MultiMetaGetter) GetMeta(idx int, key string) (interface{}, error) {
	return m.GetSingle(idx).GetMeta(key)
}

// GetMetaDefault is like PropertyLoadSaver.GetMetaDefault, but it also takes an
// index indicating which slot you want metadata for. If idx isn't there, this
// returns dflt.
func (m MultiMetaGetter) GetMetaDefault(idx int, key string, dflt interface{}) interface{} {
	return m.GetSingle(idx).GetMetaDefault(key, dflt)
}

func (m MultiMetaGetter) GetSingle(idx int) MetaGetter {
	if idx >= len(m) || m[idx] == nil {
		return nullMetaGetter
	}
	return m[idx]
}

// RawInterface implements the datastore functionality without any of the fancy
// reflection stuff. This is so that Filters can avoid doing lots of redundant
// reflection work. See datastore.RawInterface for a more user-friendly interface.
type RawInterface interface {
	NewKey(kind, stringID string, intID int64, parent Key) Key
	DecodeKey(encoded string) (Key, error)
	NewQuery(kind string) Query

	// RunInTransaction runs f in a transaction.
	//
	// opts may be nil.
	//
	// NOTE: Implementations and filters are guaranteed that:
	//   - f is not nil
	RunInTransaction(f func(c context.Context) error, opts *TransactionOptions) error

	// Run executes the given query, and calls `cb` for each successfully item.
	//
	// NOTE: Implementations and filters are guaranteed that:
	//   - query is not nil
	//   - cb is not nil
	Run(q Query, cb RawRunCB) error

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
	GetMulti(keys []Key, meta MultiMetaGetter, cb GetMultiCB) error

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
	PutMulti(keys []Key, vals []PropertyMap, cb PutMultiCB) error

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
	DeleteMulti(keys []Key, cb DeleteMultiCB) error
}
