// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package count

import (
	"golang.org/x/net/context"

	rds "github.com/luci/gae/service/rawdatastore"
)

// RDSCounter is the counter object for the RawDatastore service.
type RDSCounter struct {
	NewKey           Entry
	DecodeKey        Entry
	NewQuery         Entry
	RunInTransaction Entry
	Run              Entry
	DeleteMulti      Entry
	GetMulti         Entry
	PutMulti         Entry
}

type rdsCounter struct {
	c *RDSCounter

	rds rds.Interface
}

var _ rds.Interface = (*rdsCounter)(nil)

func (r *rdsCounter) NewKey(kind, stringID string, intID int64, parent rds.Key) rds.Key {
	r.c.NewKey.up()
	return r.rds.NewKey(kind, stringID, intID, parent)
}

func (r *rdsCounter) DecodeKey(encoded string) (rds.Key, error) {
	ret, err := r.rds.DecodeKey(encoded)
	return ret, r.c.DecodeKey.up(err)
}

func (r *rdsCounter) NewQuery(kind string) rds.Query {
	r.c.NewQuery.up()
	return r.rds.NewQuery(kind)
}

func (r *rdsCounter) Run(q rds.Query, cb rds.RunCB) error {
	return r.c.Run.up(r.rds.Run(q, cb))
}

func (r *rdsCounter) RunInTransaction(f func(context.Context) error, opts *rds.TransactionOptions) error {
	return r.c.RunInTransaction.up(r.rds.RunInTransaction(f, opts))
}

func (r *rdsCounter) DeleteMulti(keys []rds.Key, cb rds.DeleteMultiCB) error {
	return r.c.DeleteMulti.up(r.rds.DeleteMulti(keys, cb))
}

func (r *rdsCounter) GetMulti(keys []rds.Key, cb rds.GetMultiCB) error {
	return r.c.GetMulti.up(r.rds.GetMulti(keys, cb))
}

func (r *rdsCounter) PutMulti(keys []rds.Key, vals []rds.PropertyLoadSaver, cb rds.PutMultiCB) error {
	return r.c.PutMulti.up(r.rds.PutMulti(keys, vals, cb))
}

// FilterRDS installs a counter RawDatastore filter in the context.
func FilterRDS(c context.Context) (context.Context, *RDSCounter) {
	state := &RDSCounter{}
	return rds.AddFilters(c, func(ic context.Context, rds rds.Interface) rds.Interface {
		return &rdsCounter{state, rds}
	}), state
}
