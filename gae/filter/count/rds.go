// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package count

import (
	"golang.org/x/net/context"

	ds "github.com/luci/gae/service/datastore"
)

// DSCounter is the counter object for the datastore service.
type DSCounter struct {
	DecodeCursor     Entry
	RunInTransaction Entry
	Run              Entry
	DeleteMulti      Entry
	GetMulti         Entry
	PutMulti         Entry
}

type dsCounter struct {
	c *DSCounter

	ds ds.RawInterface
}

var _ ds.RawInterface = (*dsCounter)(nil)

func (r *dsCounter) DecodeCursor(s string) (ds.Cursor, error) {
	cursor, err := r.ds.DecodeCursor(s)
	return cursor, r.c.DecodeCursor.up(err)
}

func (r *dsCounter) Run(q *ds.FinalizedQuery, cb ds.RawRunCB) error {
	return r.c.Run.up(r.ds.Run(q, cb))
}

func (r *dsCounter) RunInTransaction(f func(context.Context) error, opts *ds.TransactionOptions) error {
	return r.c.RunInTransaction.up(r.ds.RunInTransaction(f, opts))
}

func (r *dsCounter) DeleteMulti(keys []*ds.Key, cb ds.DeleteMultiCB) error {
	return r.c.DeleteMulti.up(r.ds.DeleteMulti(keys, cb))
}

func (r *dsCounter) GetMulti(keys []*ds.Key, meta ds.MultiMetaGetter, cb ds.GetMultiCB) error {
	return r.c.GetMulti.up(r.ds.GetMulti(keys, meta, cb))
}

func (r *dsCounter) PutMulti(keys []*ds.Key, vals []ds.PropertyMap, cb ds.PutMultiCB) error {
	return r.c.PutMulti.up(r.ds.PutMulti(keys, vals, cb))
}

func (r *dsCounter) Testable() ds.Testable {
	return r.ds.Testable()
}

// FilterRDS installs a counter datastore filter in the context.
func FilterRDS(c context.Context) (context.Context, *DSCounter) {
	state := &DSCounter{}
	return ds.AddRawFilters(c, func(ic context.Context, ds ds.RawInterface) ds.RawInterface {
		return &dsCounter{state, ds}
	}), state
}
