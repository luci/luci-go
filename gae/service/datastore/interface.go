// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package datastore

import (
	"golang.org/x/net/context"
)

// RunCB is the callback signature provided to Interface.Run
//
//   - obj is an object as specified by the proto argument of Run
//   - getCursor can be invoked to obtain the current cursor.
//
// Return true to continue iterating through the query results, or false to stop.
type RunCB func(obj interface{}, getCursor func() (Cursor, error)) bool

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
	// NewKey produces a new Key with the current appid and namespace.
	NewKey(kind, stringID string, intID int64, parent Key) Key

	// KeyForObj extracts a key from src.
	//
	// It is the same as KeyForObjErr, except that if KeyForObjErr would have
	// returned an error, this method panics. It's safe to use if you know that
	// src statically meets the metadata constraints described by KeyForObjErr.
	KeyForObj(src interface{}) Key

	// KeyForObjErr extracts a key from src.
	//
	// src must be one of:
	//   - *S where S is a struct
	//   - a PropertyLoadSaver
	//
	// It is expected that the struct or PropertyLoadSaver exposes the
	// following metadata (as retrieved by PropertyLoadSaver.GetMeta):
	//   - "key" (type: Key) - The full datastore key to use. Must not be nil.
	//     OR
	//   - "id" (type: int64 or string) - The id of the Key to create
	//   - "kind" (optional, type: string) - The kind of the Key to create. If
	//     blank or not present, KeyForObjErr will extract the name of the src
	//     object's type.
	//   - "parent" (optional, type: Key) - The parent key to use.
	//
	// If a required metadata item is missing or of the wrong type, then this will
	// return an error.
	KeyForObjErr(src interface{}) (Key, error)

	// DecodeKey decodes a proto-encoded key.
	//
	// The encoding is defined by the appengine SDK's implementation.  In
	// particular, it is a no-pad-base64-encoded protobuf. If there's an error
	// during the decoding process, it will be returned.
	DecodeKey(encoded string) (Key, error)

	// NewQuery creates a new Query object. No server communication occurs.
	NewQuery(kind string) Query

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
	// proto is a prototype of the objects which will be passed to the callback.
	// It will be used solely for type information, and the actual proto object
	// may be zero/nil.  It must be of the form:
	//   - S or *S where S is a struct
	//   - P or *P where *P is a concrete type implementing PropertyLoadSaver
	//   - *Key implies a keys-only query (and cb will be invoked with Key objects)
	// Run will create a new, populated instance of proto for each call of
	// cb. Run stops on the first error encountered.
	Run(q Query, proto interface{}, cb RunCB) error

	// GetAll retrieves all of the Query results into dst.
	//
	// dst must be one of:
	//   - *[]S or *[]*S where S is a struct
	//   - *[]P or *[]*P where *P is a concrete type implementing PropertyLoadSaver
	//   - *[]Key implies a keys-only query.
	GetAll(q Query, dst interface{}) error

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
	// A Key will be extracted from src via KeyForObj. If
	// KeyIncomplete(extractedKey) is true, then Put will write the resolved (i.e.
	// automatic datastore-populated) Key back to src.
	Put(src interface{}) error

	// Delete removes an item from the datastore.
	Delete(key Key) error

	// GetMulti retrieves items from the datastore.
	//
	// dst must be one of:
	//   - []S or []*S where S is a struct
	//   - []P or []*P where *P is a concrete type implementing PropertyLoadSaver
	//   - []I where I is some interface type. Each element of the slice must
	//		 be non-nil, and its underlying type must be either *S or *P.
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
	DeleteMulti(keys []Key) error

	// Raw returns the underlying RawInterface. The Interface and RawInterface may
	// be used interchangably; there's no danger of interleaving access to the
	// datastore via the two.
	Raw() RawInterface
}
