// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package datastore

import (
	"golang.org/x/net/context"
)

// BoolList is a convenience wrapper for []bool that provides summary methods
// for working with the list in aggregate.
type BoolList []bool

// All returns true iff all of the booleans in this list are true.
func (bl BoolList) All() bool {
	for _, b := range bl {
		if !b {
			return false
		}
	}
	return true
}

// Any returns true iff any of the booleans in this list are true.
func (bl BoolList) Any() bool {
	for _, b := range bl {
		if b {
			return true
		}
	}
	return false
}

// Interface is the 'user-friendly' interface to access the current filtered
// datastore service implementation.
//
// Note that in exchange for userfriendliness, this interface ends up doing
// a lot of reflection.
//
// Methods taking 'interface{}' objects describe what a valid type for that
// interface are in the comments.
//
// Struct objects passed in will be converted to PropertyLoadSaver interfaces
// using this package's GetPLS function.
type Interface interface {
	// AllocateIDs allows you to allocate IDs from the datastore without putting
	// any data. `incomplete` must be a PartialValid Key. If there's no error,
	// a contiguous block of IDs of n length starting at `start` will be reserved
	// indefinitely for the user application code for use in new keys. The
	// appengine automatic ID generator will never automatically assign these IDs
	// for Keys of this type.
	AllocateIDs(incomplete *Key, n int) (start int64, err error)

	// KeyForObj extracts a key from src.
	//
	// It is the same as KeyForObjErr, except that if KeyForObjErr would have
	// returned an error, this method panics. It's safe to use if you know that
	// src statically meets the metadata constraints described by KeyForObjErr.
	KeyForObj(src interface{}) *Key

	// MakeKey is a convenience method for manufacturing a *Key. It should only be
	// used when elems... is known statically (e.g. in the code) to be correct.
	//
	// elems is pairs of (string, string|int|int32|int64) pairs, which correspond
	// to Kind/id pairs. Example:
	//   dstore.MakeKey("Parent", 1, "Child", "id")
	//
	// Would create the key:
	//   <current appID>:<current Namespace>:/Parent,1/Child,id
	//
	// If elems is not parsable (e.g. wrong length, wrong types, etc.) this method
	// will panic.
	MakeKey(elems ...interface{}) *Key

	// NewKey constructs a new key in the current appID/Namespace, using the
	// specified parameters.
	NewKey(kind, stringID string, intID int64, parent *Key) *Key

	// NewKeyToks constructs a new key in the current appID/Namespace, using the
	// specified key tokens.
	NewKeyToks([]KeyTok) *Key

	// KeyForObjErr extracts a key from src.
	//
	// src must be one of:
	//   - *S where S is a struct
	//   - a PropertyLoadSaver
	//
	// It is expected that the struct exposes the following metadata (as retrieved
	// by MetaGetter.GetMeta):
	//   - "key" (type: Key) - The full datastore key to use. Must not be nil.
	//     OR
	//   - "id" (type: int64 or string) - The id of the Key to create
	//   - "kind" (optional, type: string) - The kind of the Key to create. If
	//     blank or not present, KeyForObjErr will extract the name of the src
	//     object's type.
	//   - "parent" (optional, type: Key) - The parent key to use.
	//
	// By default, the metadata will be extracted from the struct and its tagged
	// properties. However, if the struct implements MetaGetterSetter it is
	// wholly responsible for exporting the required fields. A struct that
	// implements GetMeta to make some minor tweaks can evoke the defualt behavior
	// by using GetPLS(s).GetMeta.
	//
	// If a required metadata item is missing or of the wrong type, then this will
	// return an error.
	KeyForObjErr(src interface{}) (*Key, error)

	// RunInTransaction runs f inside of a transaction. See the appengine SDK's
	// documentation for full details on the behavior of transactions in the
	// datastore.
	//
	// Note that the behavior of transactions may change depending on what filters
	// have been installed. It's possible that we'll end up implementing things
	// like nested/buffered transactions as filters.
	RunInTransaction(f func(c context.Context) error, opts *TransactionOptions) error

	// Run executes the given query, and calls `cb` for each successfully
	// retrieved item.
	//
	// cb is a callback function whose signature is
	//   func(obj TYPE[, getCursor CursorCB]) [error]
	//
	// Where TYPE is one of:
	//   - S or *S where S is a struct
	//   - P or *P where *P is a concrete type implementing PropertyLoadSaver
	//   - *Key (implies a keys-only query)
	//
	// If the error is omitted from the signature, this will run until the query
	// returns all its results, or has an error/times out.
	//
	// If error is in the signature, the query will continue as long as the
	// callback returns nil. If it returns `Stop`, the query will stop and Run
	// will return nil. Otherwise, the query will stop and Run will return the
	// user's error.
	//
	// Run may also stop on the first datastore error encountered, which can occur
	// due to flakiness, timeout, etc. If it encounters such an error, it will
	// be returned.
	Run(q *Query, cb interface{}) error

	// Count executes the given query and returns the number of entries which
	// match it.
	Count(q *Query) (int64, error)

	// DecodeCursor converts a string returned by a Cursor into a Cursor instance.
	// It will return an error if the supplied string is not valid, or could not
	// be decoded by the implementation.
	DecodeCursor(string) (Cursor, error)

	// GetAll retrieves all of the Query results into dst.
	//
	// dst must be one of:
	//   - *[]S or *[]*S where S is a struct
	//   - *[]P or *[]*P where *P is a concrete type implementing
	//     PropertyLoadSaver
	//   - *[]*Key implies a keys-only query.
	GetAll(q *Query, dst interface{}) error

	// Does a Get for this key and returns true iff it exists. Will only return
	// an error if it's not ErrNoSuchEntity. This is slightly more efficient
	// than using Get directly, because it uses the underlying RawInterface to
	// avoid some reflection and copies.
	Exists(k *Key) (bool, error)

	// Does a GetMulti for thes keys and returns true iff they exist. Will only
	// return an error if it's not ErrNoSuchEntity. This is slightly more efficient
	// than using Get directly, because it uses the underlying RawInterface to
	// avoid some reflection and copies.
	ExistsMulti(k []*Key) (BoolList, error)

	// Get retrieves a single object from the datastore
	//
	// dst must be one of:
	//   - *S where S is a struct
	//   - *P where *P is a concrete type implementing PropertyLoadSaver
	Get(dst interface{}) error

	// Put inserts a single object into the datastore
	//
	// src must be one of:
	//   - *S where S is a struct
	//   - *P where *P is a concrete type implementing PropertyLoadSaver
	//
	// A *Key will be extracted from src via KeyForObj. If
	// extractedKey.Incomplete() is true, then Put will write the resolved (i.e.
	// automatic datastore-populated) *Key back to src.
	Put(src interface{}) error

	// Delete removes an item from the datastore.
	Delete(key *Key) error

	// GetMulti retrieves items from the datastore.
	//
	// dst must be one of:
	//   - []S or []*S where S is a struct
	//   - []P or []*P where *P is a concrete type implementing PropertyLoadSaver
	//   - []I where I is some interface type. Each element of the slice must
	//     be non-nil, and its underlying type must be either *S or *P.
	GetMulti(dst interface{}) error

	// PutMulti writes items to the datastore.
	//
	// src must be one of:
	//   - []S or []*S where S is a struct
	//   - []P or []*P where *P is a concrete type implementing PropertyLoadSaver
	//   - []I where i is some interface type. Each elemet of the slice must
	//     be non-nil, and its underlying type must be either *S or *P.
	//
	// If items in src resolve to Incomplete keys, PutMulti will write the
	// resolved keys back to the items in src.
	PutMulti(src interface{}) error

	// DeleteMulti removes items from the datastore.
	DeleteMulti(keys []*Key) error

	// Testable returns the Testable interface for the implementation, or nil if
	// there is none.
	Testable() Testable

	// Raw returns the underlying RawInterface. The Interface and RawInterface may
	// be used interchangably; there's no danger of interleaving access to the
	// datastore via the two.
	Raw() RawInterface
}
