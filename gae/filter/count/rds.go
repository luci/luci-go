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
	Count            Entry
	RunInTransaction Entry
	Run              Entry
	GetAll           Entry
	Put              Entry
	Get              Entry
	Delete           Entry
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

func (r *rdsCounter) Run(q rds.Query) rds.Iterator {
	r.c.Run.up()
	return r.rds.Run(q)
}

func (r *rdsCounter) GetAll(q rds.Query, dst *[]rds.PropertyMap) ([]rds.Key, error) {
	ret, err := r.rds.GetAll(q, dst)
	return ret, r.c.GetAll.up(err)
}

func (r *rdsCounter) Count(q rds.Query) (int, error) {
	ret, err := r.rds.Count(q)
	return ret, r.c.Count.up(err)
}

func (r *rdsCounter) RunInTransaction(f func(context.Context) error, opts *rds.TransactionOptions) error {
	return r.c.RunInTransaction.up(r.rds.RunInTransaction(f, opts))
}

func (r *rdsCounter) Put(key rds.Key, src rds.PropertyLoadSaver) (rds.Key, error) {
	ret, err := r.rds.Put(key, src)
	return ret, r.c.Put.up(err)
}

func (r *rdsCounter) Get(key rds.Key, dst rds.PropertyLoadSaver) error {
	return r.c.Get.up(r.rds.Get(key, dst))
}

func (r *rdsCounter) Delete(key rds.Key) error {
	return r.c.Delete.up(r.rds.Delete(key))
}

func (r *rdsCounter) DeleteMulti(keys []rds.Key) error {
	return r.c.DeleteMulti.up(r.rds.DeleteMulti(keys))
}

func (r *rdsCounter) GetMulti(keys []rds.Key, dst []rds.PropertyLoadSaver) error {
	return r.c.GetMulti.up(r.rds.GetMulti(keys, dst))
}

func (r *rdsCounter) PutMulti(keys []rds.Key, src []rds.PropertyLoadSaver) ([]rds.Key, error) {
	ret, err := r.rds.PutMulti(keys, src)
	return ret, r.c.PutMulti.up(err)
}

// FilterRDS installs a counter RawDatastore filter in the context.
func FilterRDS(c context.Context) (context.Context, *RDSCounter) {
	state := &RDSCounter{}
	return rds.AddFilters(c, func(ic context.Context, rds rds.Interface) rds.Interface {
		return &rdsCounter{state, rds}
	}), state
}
