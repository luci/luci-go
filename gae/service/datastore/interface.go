// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package datastore

import (
	"golang.org/x/net/context"
)

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

	// Exists tests if the supplied objects are present in the datastore.
	//
	// ent must be one of:
	//	- *S where S is a struct
	//	- *P where *P is a concrete type implementing PropertyLoadSaver
	//	- []S or []*S where S is a struct
	//	- []P or []*P where *P is a concrete type implementing PropertyLoadSaver
	//	- []I where i is some interface type. Each element of the slice must
	//	  be non-nil, and its underlying type must be either *S or *P.
	//	- *Key, to check a specific key from the datastore.
	//	- []*Key, to check a slice of keys from the datastore.
	//
	// If an error is encountered, the returned error value will depend on the
	// input arguments. If one argument is supplied, the result will be the
	// encountered error type. If multiple arguments are supplied, the result will
	// be a MultiError whose error index corresponds to the argument in which the
	// error was encountered.
	//
	// If an ent argument is a slice, its error type will be a MultiError. Note
	// that in the scenario where multiple slices are provided, this will return a
	// MultiError containing a nested MultiError for each slice argument.
	Exists(ent ...interface{}) (*ExistsResult, error)

	// Does a GetMulti for thes keys and returns true iff they exist. Will only
	// return an error if it's not ErrNoSuchEntity. This is slightly more efficient
	// than using Get directly, because it uses the underlying RawInterface to
	// avoid some reflection and copies.
	//
	// If an error is encountered, the returned error will be a MultiError whose
	// error index corresponds to the key for which the error was encountered.
	//
	// NOTE: ExistsMulti is obsolete. The vararg-accepting Exists should be used
	// instead. This is left for backwards compatibility, but will be removed from
	// this interface at some point in the future.
	ExistsMulti(k []*Key) (BoolList, error)

	// Get retrieves objects from the datastore.
	//
	// Each element in dst must be one of:
	//	- *S where S is a struct
	//	- *P where *P is a concrete type implementing PropertyLoadSaver
	//	- []S or []*S where S is a struct
	//	- []P or []*P where *P is a concrete type implementing PropertyLoadSaver
	//	- []I where I is some interface type. Each element of the slice must
	//	  be non-nil, and its underlying type must be either *S or *P.
	//
	// If an error is encountered, the returned error value will depend on the
	// input arguments. If one argument is supplied, the result will be the
	// encountered error type. If multiple arguments are supplied, the result will
	// be a MultiError whose error index corresponds to the argument in which the
	// error was encountered.
	//
	// If a dst argument is a slice, its error type will be a MultiError. Note
	// that in the scenario where multiple slices are provided, this will return a
	// MultiError containing a nested MultiError for each slice argument.
	Get(dst ...interface{}) error

	// GetMulti retrieves items from the datastore.
	//
	// dst must be one of:
	//   - []S or []*S where S is a struct
	//   - []P or []*P where *P is a concrete type implementing PropertyLoadSaver
	//   - []I where I is some interface type. Each element of the slice must
	//     be non-nil, and its underlying type must be either *S or *P.
	//
	// NOTE: GetMulti is obsolete. The vararg-accepting Get should be used
	// instead. This is left for backwards compatibility, but will be removed from
	// this interface at some point in the future.
	GetMulti(dst interface{}) error

	// Put inserts a single object into the datastore
	//
	// src must be one of:
	//	- *S where S is a struct
	//	- *P where *P is a concrete type implementing PropertyLoadSaver
	//	- []S or []*S where S is a struct
	//	- []P or []*P where *P is a concrete type implementing PropertyLoadSaver
	//	- []I where i is some interface type. Each element of the slice must
	//	  be non-nil, and its underlying type must be either *S or *P.
	//
	// A *Key will be extracted from src via KeyForObj. If
	// extractedKey.Incomplete() is true, then Put will write the resolved (i.e.
	// automatic datastore-populated) *Key back to src.
	//
	// If an error is encountered, the returned error value will depend on the
	// input arguments. If one argument is supplied, the result will be the
	// encountered error type. If multiple arguments are supplied, the result will
	// be a MultiError whose error index corresponds to the argument in which the
	// error was encountered.
	//
	// If a src argument is a slice, its error type will be a MultiError. Note
	// that in the scenario where multiple slices are provided, this will return a
	// MultiError containing a nested MultiError for each slice argument.
	Put(src ...interface{}) error

	// PutMulti writes items to the datastore.
	//
	// src must be one of:
	//	- []S or []*S where S is a struct
	//	- []P or []*P where *P is a concrete type implementing PropertyLoadSaver
	//	- []I where i is some interface type. Each element of the slice must
	//	  be non-nil, and its underlying type must be either *S or *P.
	//
	// If items in src resolve to Incomplete keys, PutMulti will write the
	// resolved keys back to the items in src.
	//
	// NOTE: PutMulti is obsolete. The vararg-accepting Put should be used
	// instead. This is left for backwards compatibility, but will be removed from
	// this interface at some point in the future.
	PutMulti(src interface{}) error

	// Delete removes the supplied entities from the datastore.
	//
	// ent must be one of:
	//	- *S where S is a struct
	//	- *P where *P is a concrete type implementing PropertyLoadSaver
	//	- []S or []*S where S is a struct
	//	- []P or []*P where *P is a concrete type implementing PropertyLoadSaver
	//	- []I where i is some interface type. Each element of the slice must
	//	  be non-nil, and its underlying type must be either *S or *P.
	//	- *Key, to remove a specific key from the datastore.
	//	- []*Key, to remove a slice of keys from the datastore.
	//
	// If an error is encountered, the returned error value will depend on the
	// input arguments. If one argument is supplied, the result will be the
	// encountered error type. If multiple arguments are supplied, the result will
	// be a MultiError whose error index corresponds to the argument in which the
	// error was encountered.
	//
	// If an ent argument is a slice, its error type will be a MultiError. Note
	// that in the scenario where multiple slices are provided, this will return a
	// MultiError containing a nested MultiError for each slice argument.
	Delete(ent ...interface{}) error

	// DeleteMulti removes keys from the datastore.
	//
	// If an error is encountered, the returned error will be a MultiError whose
	// error index corresponds to the key for which the error was encountered.
	//
	// NOTE: DeleteMulti is obsolete. The vararg-accepting Delete should be used
	// instead. This is left for backwards compatibility, but will be removed from
	// this interface at some point in the future.
	DeleteMulti(keys []*Key) error

	// Testable returns the Testable interface for the implementation, or nil if
	// there is none.
	Testable() Testable

	// Raw returns the underlying RawInterface. The Interface and RawInterface may
	// be used interchangably; there's no danger of interleaving access to the
	// datastore via the two.
	Raw() RawInterface
}
