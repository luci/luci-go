// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package rawdatastore

import (
	"fmt"

	"golang.org/x/net/context"
)

/// Kinds + Keys

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

// Iterator wraps datastore.Iterator.
type Iterator interface {
	Cursor() (Cursor, error)
	Next(dst PropertyLoadSaver) (Key, error)
}

// Interface implements the datastore functionality without any of the fancy
// reflection stuff. This is so that Filters can avoid doing lots of redundant
// reflection work. See helper.Datastore for a more user-friendly interface.
type Interface interface {
	NewKey(kind, stringID string, intID int64, parent Key) Key
	DecodeKey(encoded string) (Key, error)

	NewQuery(kind string) Query
	Count(q Query) (int, error)

	RunInTransaction(f func(c context.Context) error, opts *TransactionOptions) error

	Run(q Query) Iterator
	GetAll(q Query, dst *[]PropertyMap) ([]Key, error)

	Put(key Key, src PropertyLoadSaver) (Key, error)
	Get(key Key, dst PropertyLoadSaver) error
	Delete(key Key) error

	// These allow you to read and write a multiple datastore objects in
	// a non-atomic batch.
	DeleteMulti(keys []Key) error
	GetMulti(keys []Key, dst []PropertyLoadSaver) error
	PutMulti(keys []Key, src []PropertyLoadSaver) ([]Key, error)
}
