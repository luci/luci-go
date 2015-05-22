// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// +build appengine

package context

import (
	"appengine"
	"appengine/datastore"

	"github.com/luci/luci-go/common/logging"
	"github.com/mjibson/goon"
)

/// Kinds + Keys

type Kinder interface {
	Kind(src interface{}) string
}

type KindSetter interface {
	KindNameResolver() goon.KindNameResolver
	SetKindNameResolver(goon.KindNameResolver)
}

type NewKeyer interface {
	NewKey(kind, stringID string, intID int64, parent *datastore.Key) *datastore.Key
	NewKeyObj(src interface{}) *datastore.Key
	NewKeyObjError(src interface{}) (*datastore.Key, error)
}

type KindKeyer interface {
	Kinder
	NewKeyer
}

/// Read + Write

type SingleReadWriter interface {
	Put(src interface{}) (*datastore.Key, error)
	Get(dst interface{}) error
	Delete(key *datastore.Key) error
}

type MultiReadWriter interface {
	SingleReadWriter
	DeleteMulti(keys []*datastore.Key) error
	GetMulti(dst interface{}) error
	PutMulti(src interface{}) ([]*datastore.Key, error)
}

/// Queries

type Queryer interface {
	Run(q *datastore.Query) Iterator
	GetAll(q *datastore.Query, dst interface{}) ([]*datastore.Key, error)
	Count(q *datastore.Query) (int, error)
}

type Iterator interface {
	Cursor() (datastore.Cursor, error)
	Next(dst interface{}) (*datastore.Key, error)
}

/// Transactions

type Transactioner interface {
	RunInTransaction(f func(c Context) error, opts *datastore.TransactionOptions) error
}

/// Abstraction breaking!

type DangerousContexter interface {
	Context() appengine.Context
}

type SingleContext interface {
	logging.Logger
	Kinder
	NewKeyer
	SingleReadWriter
}

type Context interface {
	logging.Logger
	Kinder
	NewKeyer
	MultiReadWriter
	Queryer
	KindSetter
}

type TransactionerContext interface {
	Context
	Transactioner
}

type DangerousContext interface {
	DangerousContexter
	Context
}

type DangerousTransactionerContext interface {
	DangerousContexter
	TransactionerContext
}
