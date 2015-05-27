// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wrapper

import (
	"fmt"

	"golang.org/x/net/context"

	"appengine/datastore"

	"github.com/mjibson/goon"
)

/// Kinds + Keys

// DSKinder simply allows you to resolve the datastore 'kind' of an object.
// See goon.Goon.Kind().
type DSKinder interface {
	Kind(src interface{}) string
}

// DSKindSetter allows you to manipulate the current Kind resolution function.
// See goon.Goon.KindNameResolver.
type DSKindSetter interface {
	KindNameResolver() goon.KindNameResolver
	SetKindNameResolver(goon.KindNameResolver)
}

// DSNewKeyer allows you to generate a new *datastore.Key in a couple ways.
// See goon.Goon.Key* for the NewKeyObj* methods, as well as datastore.NewKey.
type DSNewKeyer interface {
	NewKey(kind, stringID string, intID int64, parent *datastore.Key) *datastore.Key
	NewKeyObj(src interface{}) *datastore.Key
	NewKeyObjError(src interface{}) (*datastore.Key, error)
}

/// Read + Write

// DSSingleReadWriter allows you to read and write a single datastore object.
// See goon.Goon for more detail on the same-named functions.
type DSSingleReadWriter interface {
	Put(src interface{}) (*datastore.Key, error)
	Get(dst interface{}) error
	Delete(key *datastore.Key) error
}

// DSMultiReadWriter allows you to read and write a multiple datastore objects
// in a non-atomic batch. See goon.Goon for more detail on the same-named.
// functions. Also implies DSSingleReadWriter.
type DSMultiReadWriter interface {
	DSSingleReadWriter
	DeleteMulti(keys []*datastore.Key) error
	GetMulti(dst interface{}) error
	PutMulti(src interface{}) ([]*datastore.Key, error)
}

/// Queries

// DSCursor wraps datastore.Cursor.
type DSCursor interface {
	fmt.Stringer
}

// DSIterator wraps datastore.Iterator.
type DSIterator interface {
	Cursor() (DSCursor, error)
	Next(dst interface{}) (*datastore.Key, error)
}

// DSQuery wraps datastore.Query.
type DSQuery interface {
	Ancestor(ancestor *datastore.Key) DSQuery
	Distinct() DSQuery
	End(c DSCursor) DSQuery
	EventualConsistency() DSQuery
	Filter(filterStr string, value interface{}) DSQuery
	KeysOnly() DSQuery
	Limit(limit int) DSQuery
	Offset(offset int) DSQuery
	Order(fieldName string) DSQuery
	Project(fieldNames ...string) DSQuery
	Start(c DSCursor) DSQuery
}

// DSQueryer implements all query-related functionality of goon.Goon/datastore.
type DSQueryer interface {
	NewQuery(kind string) DSQuery
	Run(q DSQuery) DSIterator
	GetAll(q DSQuery, dst interface{}) ([]*datastore.Key, error)
	Count(q DSQuery) (int, error)
}

/// Transactions

// DSTransactioner implements the function for entering a transaction with
// datastore. The function receives a context without the ability to enter a
// transaction again (since recursive transactions are not implemented in
// datastore).
type DSTransactioner interface {
	RunInTransaction(f func(c context.Context) error, opts *datastore.TransactionOptions) error
}

// Datastore implements the full datastore functionality.
type Datastore interface {
	DSKinder
	DSNewKeyer
	DSMultiReadWriter
	DSQueryer
	DSKindSetter
	DSTransactioner
}

// DSFactory is the function signature for factory methods compatible with
// SetDSFactory.
type DSFactory func(context.Context) Datastore

// GetDS gets the Datastore implementation from context.
func GetDS(c context.Context) Datastore {
	if f, ok := c.Value(datastoreKey).(DSFactory); ok && f != nil {
		return f(c)
	}
	return nil
}

// SetDSFactory sets the function to produce Datastore instances, as returned by
// the GetDS method.
func SetDSFactory(c context.Context, dsf DSFactory) context.Context {
	return context.WithValue(c, datastoreKey, dsf)
}

// SetDS sets the current Datastore object in the context. Useful for testing
// with a quick mock. This is just a shorthand SetDSFactory invocation to set
// a factory which always returns the same object.
func SetDS(c context.Context, ds Datastore) context.Context {
	return SetDSFactory(c, func(context.Context) Datastore { return ds })
}
