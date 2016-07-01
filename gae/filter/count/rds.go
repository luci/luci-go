// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package count

import (
	"golang.org/x/net/context"

	ds "github.com/luci/gae/service/datastore"
)

// DSCounter is the counter object for the datastore service.
type DSCounter struct {
	AllocateIDs      Entry
	DecodeCursor     Entry
	RunInTransaction Entry
	Run              Entry
	Count            Entry
	DeleteMulti      Entry
	GetMulti         Entry
	PutMulti         Entry
}

type dsCounter struct {
	c *DSCounter

	ds ds.RawInterface
}

var _ ds.RawInterface = (*dsCounter)(nil)

func (r *dsCounter) AllocateIDs(keys []*ds.Key, cb ds.NewKeyCB) error {
	return r.c.AllocateIDs.up(r.ds.AllocateIDs(keys, cb))
}

func (r *dsCounter) DecodeCursor(s string) (ds.Cursor, error) {
	cursor, err := r.ds.DecodeCursor(s)
	return cursor, r.c.DecodeCursor.up(err)
}

func (r *dsCounter) Run(q *ds.FinalizedQuery, cb ds.RawRunCB) error {
	return r.c.Run.upFilterStop(r.ds.Run(q, cb))
}

func (r *dsCounter) Count(q *ds.FinalizedQuery) (int64, error) {
	count, err := r.ds.Count(q)
	return count, r.c.Count.up(err)
}

func (r *dsCounter) RunInTransaction(f func(context.Context) error, opts *ds.TransactionOptions) error {
	return r.c.RunInTransaction.up(r.ds.RunInTransaction(f, opts))
}

func (r *dsCounter) DeleteMulti(keys []*ds.Key, cb ds.DeleteMultiCB) error {
	return r.c.DeleteMulti.upFilterStop(r.ds.DeleteMulti(keys, cb))
}

func (r *dsCounter) GetMulti(keys []*ds.Key, meta ds.MultiMetaGetter, cb ds.GetMultiCB) error {
	return r.c.GetMulti.upFilterStop(r.ds.GetMulti(keys, meta, cb))
}

func (r *dsCounter) PutMulti(keys []*ds.Key, vals []ds.PropertyMap, cb ds.NewKeyCB) error {
	return r.c.PutMulti.upFilterStop(r.ds.PutMulti(keys, vals, cb))
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

// upFilterStop wraps up, handling the special case datastore.Stop error.
// datastore.Stop will pass through this function, but, unlike other error
// codes, will be counted as a success.
func (e *Entry) upFilterStop(err error) error {
	upErr := err
	if upErr == ds.Stop {
		upErr = nil
	}
	e.up(upErr)
	return err
}
