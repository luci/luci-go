// Copyright 2015 The LUCI Authors.
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
//   - idx is the index of the entity, ranging from 0 through len-1.
//   - val is the data of the entity
//     * It may be nil if some of the keys to the GetMulti were bad, since all
//       keys are validated before the RPC occurs!
//   - err is an error associated with this entity (e.g. ErrNoSuchEntity).
//
// The callback is called once per element. It may be called concurrently, and
// may be called out of order. The "idx" variable describes which element is
// being processed. If any callbacks are invoked, exactly one callback will be
// invoked for each supplied element.
//
// Return nil to continue iterating, or an error to stop. If you return the
// error `Stop`, then GetMulti will stop the query and return nil.
type GetMultiCB func(idx int, val PropertyMap, err error) error

// NewKeyCB is the callback signature provided to RawInterface.PutMulti and
// RawInterface.AllocateIDs. It is invoked once for each positional key that
// was generated as the result of a call.
//
//   - idx is the index of the entity, ranging from 0 through len-1.
//   - key is the new key for the entity (if the original was incomplete)
//     * It may be nil if some of the keys/vals to the PutMulti were bad, since
//       all keys are validated before the RPC occurs!
//   - err is an error associated with putting this entity.
//
// The callback is called once per element. It may be called concurrently, and
// may be called out of order. The "idx" variable describes which element is
// being processed. If any callbacks are invoked, exactly one callback will be
// invoked for each supplied element.
//
// Return nil to continue iterating, or an error to stop. If you return the
// error `Stop`, then PutMulti will stop the query and return nil.
type NewKeyCB func(idx int, key *Key, err error) error

// DeleteMultiCB is the callback signature provided to RawInterface.DeleteMulti
//
//   - idx is the index of the entity, ranging from 0 through len-1.
//   - err is an error associated with deleting this entity.
//
// The callback is called once per element. It may be called concurrently, and
// may be called out of order. The "idx" variable describes which element is
// being processed. If any callbacks are invoked, exactly one callback will be
// invoked for each supplied element.
//
// Return nil to continue iterating, or an error to stop. If you return the
// error `Stop`, then DeleteMulti will stop the query and return nil.
type DeleteMultiCB func(idx int, err error) error

// Constraints represent implementation constraints.
//
// A zero-value Constraints is valid, and indicates that no constraints are
// present.
type Constraints struct {
	// MaxGetSize is the maximum number of entities that can be referenced in a
	// single GetMulti call. If <= 0, no constraint is applied.
	MaxGetSize int
	// MaxPutSize is the maximum number of entities that can be referenced in a
	// single PutMulti call. If <= 0, no constraint is applied.
	MaxPutSize int
	// MaxDeleteSize is the maximum number of entities that can be referenced in a
	// single DeleteMulti call. If <= 0, no constraint is applied.
	MaxDeleteSize int
}

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
	// any data. The supplied keys must be PartialValid and share the same entity
	// type.
	//
	// If there's no error, the keys in the slice will be replaced with keys
	// containing integer IDs assigned to them.
	AllocateIDs(keys []*Key, cb NewKeyCB) error

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
	// If there was a server error, it will be returned directly. Otherwise,
	// callback will execute once per key/value pair, returning either the
	// operation result or individual error for each position. If the callback
	// receives an error, it will immediately forward that error and stop
	// subsequent callbacks.
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
	// If there was a server error, it will be returned directly. Otherwise,
	// callback will execute once per key/value pair, returning either the
	// operation result or individual error for each position. If the callback
	// receives an error, it will immediately forward that error and stop
	// subsequent callbacks.
	//
	// NOTE: Implementations and filters are guaranteed that:
	//   - len(keys) > 0
	//   - len(keys) == len(vals)
	//   - all keys are Valid and in the current namespace
	//   - cb is not nil
	PutMulti(keys []*Key, vals []PropertyMap, cb NewKeyCB) error

	// DeleteMulti removes items from the datastore.
	//
	// If there was a server error, it will be returned directly. Otherwise,
	// callback will execute once per key/value pair, returning either the
	// operation result or individual error for each position. If the callback
	// receives an error, it will immediately forward that error and stop
	// subsequent callbacks.
	//
	// NOTE: Implementations and filters are guaranteed that
	//   - len(keys) > 0
	//   - all keys are Valid, !Incomplete, and in the current namespace
	//   - none keys of the keys are 'special' (use a kind prefixed with '__')
	//   - cb is not nil
	DeleteMulti(keys []*Key, cb DeleteMultiCB) error

	// WithoutTransaction returns a derived Context without a transaction applied.
	// This may be called even when outside of a transaction, in which case the
	// input Context is a valid return value.
	WithoutTransaction() context.Context

	// CurrentTransaction returns a reference to the current Transaction, or nil
	// if the Context does not have a current Transaction.
	CurrentTransaction() Transaction

	// Constraints returns this implementation's constraints.
	Constraints() Constraints

	// GetTestable returns the Testable interface for the implementation, or nil
	// if there is none.
	GetTestable() Testable
}
