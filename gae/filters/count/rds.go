// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package count

import (
	"golang.org/x/net/context"

	"infra/gae/libs/gae"
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

	rds gae.RawDatastore
}

var _ gae.RawDatastore = (*rdsCounter)(nil)

func (r *rdsCounter) NewKey(kind, stringID string, intID int64, parent gae.DSKey) gae.DSKey {
	r.c.NewKey.up()
	return r.rds.NewKey(kind, stringID, intID, parent)
}

func (r *rdsCounter) DecodeKey(encoded string) (gae.DSKey, error) {
	ret, err := r.rds.DecodeKey(encoded)
	return ret, r.c.DecodeKey.up(err)
}

func (r *rdsCounter) NewQuery(kind string) gae.DSQuery {
	r.c.NewQuery.up()
	return r.rds.NewQuery(kind)
}

func (r *rdsCounter) Run(q gae.DSQuery) gae.RDSIterator {
	r.c.Run.up()
	return r.rds.Run(q)
}

func (r *rdsCounter) GetAll(q gae.DSQuery, dst *[]gae.DSPropertyMap) ([]gae.DSKey, error) {
	ret, err := r.rds.GetAll(q, dst)
	return ret, r.c.GetAll.up(err)
}

func (r *rdsCounter) Count(q gae.DSQuery) (int, error) {
	ret, err := r.rds.Count(q)
	return ret, r.c.Count.up(err)
}

func (r *rdsCounter) RunInTransaction(f func(context.Context) error, opts *gae.DSTransactionOptions) error {
	return r.c.RunInTransaction.up(r.rds.RunInTransaction(f, opts))
}

func (r *rdsCounter) Put(key gae.DSKey, src gae.DSPropertyLoadSaver) (gae.DSKey, error) {
	ret, err := r.rds.Put(key, src)
	return ret, r.c.Put.up(err)
}

func (r *rdsCounter) Get(key gae.DSKey, dst gae.DSPropertyLoadSaver) error {
	return r.c.Get.up(r.rds.Get(key, dst))
}

func (r *rdsCounter) Delete(key gae.DSKey) error {
	return r.c.Delete.up(r.rds.Delete(key))
}

func (r *rdsCounter) DeleteMulti(keys []gae.DSKey) error {
	return r.c.DeleteMulti.up(r.rds.DeleteMulti(keys))
}

func (r *rdsCounter) GetMulti(keys []gae.DSKey, dst []gae.DSPropertyLoadSaver) error {
	return r.c.GetMulti.up(r.rds.GetMulti(keys, dst))
}

func (r *rdsCounter) PutMulti(keys []gae.DSKey, src []gae.DSPropertyLoadSaver) ([]gae.DSKey, error) {
	ret, err := r.rds.PutMulti(keys, src)
	return ret, r.c.PutMulti.up(err)
}

// FilterRDS installs a counter RawDatastore filter in the context.
func FilterRDS(c context.Context) (context.Context, *RDSCounter) {
	state := &RDSCounter{}
	return gae.AddRDSFilters(c, func(ic context.Context, rds gae.RawDatastore) gae.RawDatastore {
		return &rdsCounter{state, rds}
	}), state
}
