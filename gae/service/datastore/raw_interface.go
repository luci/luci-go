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

// RunCB is the callback signature provided to Interface.Run
//
//   - key is the Key of the entity
//   - val is the data of the entity (or nil, if the query was keys-only)
//   - getCursor can be invoked to obtain the current cursor.
//
// Return true to continue iterating through the query results, or false to stop.
type RunCB func(key Key, val PropertyMap, getCursor func() (Cursor, error)) bool

// GetMultiCB is the callback signature provided to Interface.GetMulti
//
//   - val is the data of the entity
//     * It may be nil if some of the keys to the GetMulti were bad, since all
//       keys are validated before the RPC occurs!
//   - err is an error associated with this entity (e.g. ErrNoSuchEntity).
type GetMultiCB func(val PropertyMap, err error)

// PutMultiCB is the callback signature provided to Interface.PutMulti
//
//   - key is the new key for the entity (if the original was incomplete)
//     * It may be nil if some of the keys/vals to the PutMulti were bad, since
//       all keys are validated before the RPC occurs!
//   - err is an error associated with putting this entity.
type PutMultiCB func(key Key, err error)

// DeleteMultiCB is the callback signature provided to Interface.DeleteMulti
//
//   - err is an error associated with deleting this entity.
type DeleteMultiCB func(err error)

// Interface implements the datastore functionality without any of the fancy
// reflection stuff. This is so that Filters can avoid doing lots of redundant
// reflection work. See datastore.Interface for a more user-friendly interface.
type Interface interface {
	NewKey(kind, stringID string, intID int64, parent Key) Key
	DecodeKey(encoded string) (Key, error)
	NewQuery(kind string) Query

	RunInTransaction(f func(c context.Context) error, opts *TransactionOptions) error

	// Run executes the given query, and calls `cb` for each successfully item.
	Run(q Query, cb RunCB) error

	// GetMulti retrieves items from the datastore.
	//
	// Callback execues once per key, in the order of keys. Callback may not
	// execute at all if there's a server error. If callback is nil, this
	// method does nothing.
	//
	// NOTE: Implementations and filters are guaranteed that keys are all Valid
	// and Complete, and in the correct namespace.
	GetMulti(keys []Key, cb GetMultiCB) error

	// PutMulti writes items to the datastore.
	//
	// Callback execues once per item, in the order of itemss. Callback may not
	// execute at all if there's a server error.
	//
	// NOTE: Implementations and filters are guaranteed that len(keys) ==
	// len(vals), that keys are all Valid, and in the correct namespace.
	// Additionally, vals are guaranteed to be PropertyMaps already. Callback
	// may be nil.
	PutMulti(keys []Key, vals []PropertyLoadSaver, cb PutMultiCB) error

	// DeleteMulti removes items from the datastore.
	//
	// Callback execues once per key, in the order of keys. Callback may not
	// execute at all if there's a server error.
	//
	// NOTE: Implementations and filters are guaranteed that keys are all Valid
	// and Complete, and in the correct namespace, and are not 'special'.
	// Callback may be nil.
	DeleteMulti(keys []Key, cb DeleteMultiCB) error
}
