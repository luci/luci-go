// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// +build appengine

package context

import (
	"appengine"
	"appengine/datastore"

	"github.com/mjibson/goon"
)

type goonWrapper struct{ *goon.Goon }

// FromGoon creates a DangerousTransactionerContext backed by an existing
// *goon.Goon
func FromGoon(c *goon.Goon) DangerousTransactionerContext {
	return goonWrapper{c}
}

// GoonFromContext creates a DangerousTransactionerContext backed by the
// github.com/mjibson/goon library.
func GoonFromContext(c appengine.Context) DangerousTransactionerContext {
	return goonWrapper{goon.FromContext(c)}
}

// logger
func (g goonWrapper) Debugf(format string, args ...interface{}) {
	g.Goon.Context.Debugf(format, args...)
}

func (g goonWrapper) Infof(format string, args ...interface{}) {
	g.Goon.Context.Infof(format, args...)
}

func (g goonWrapper) Warningf(format string, args ...interface{}) {
	g.Goon.Context.Warningf(format, args...)
}

func (g goonWrapper) Errorf(format string, args ...interface{}) {
	g.Goon.Context.Errorf(format, args...)
}

// Kinder
func (g goonWrapper) KindNameResolver() goon.KindNameResolver {
	return g.Goon.KindNameResolver
}

func (g goonWrapper) SetKindNameResolver(knr goon.KindNameResolver) {
	g.Goon.KindNameResolver = knr
}

// NewKeyer
func (g goonWrapper) NewKey(kind, stringID string, intID int64, parent *datastore.Key) *datastore.Key {
	return datastore.NewKey(g.Goon.Context, kind, stringID, intID, parent)
}

func (g goonWrapper) NewKeyObj(obj interface{}) *datastore.Key {
	return g.Key(obj)
}

func (g goonWrapper) NewKeyObjError(obj interface{}) (*datastore.Key, error) {
	return g.KeyError(obj)
}

// Queryer
func (g goonWrapper) Run(q *datastore.Query) Iterator {
	return g.Run(q)
}

// Transactioner
func (g goonWrapper) RunInTransaction(f func(c Context) error, opts *datastore.TransactionOptions) error {
	return g.Goon.RunInTransaction(func(ig *goon.Goon) error {
		return f(Context(FromGoon(ig)))
	}, opts)
}

// Contexter
func (g goonWrapper) Context() appengine.Context {
	return g.Goon.Context
}
