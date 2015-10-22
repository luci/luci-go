// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package dumbCounter

import (
	"github.com/GoogleCloudPlatform/go-endpoints/endpoints"
	"github.com/luci/gae/impl/prod"
	dstore "github.com/luci/gae/service/datastore"
	"golang.org/x/net/context"
)

// AddReq describes the input parameters to the 'Add' RPC. Name is required,
// which makes it show up in the REST path, and Delta will be encoded in the
// request body as JSON.
type AddReq struct {
	Name string `endpoints:"required"`

	Delta int64 `json:",string"`
}

// AddRsp describes the return value from the 'Add' RPC. Prev is the previous
// value, and Cur is the post-increment value.
type AddRsp struct {
	Prev int64 `json:",string"`
	Cur  int64 `json:",string"`
}

// Add adds a value to the current counter, and returns the old+new values. It
// may cause a counter to come into existance.
func (Example) Add(c context.Context, r *AddReq) (rsp *AddRsp, err error) {
	rsp = &AddRsp{}

	c = prod.Use(c)
	err = dstore.Get(c).RunInTransaction(func(c context.Context) error {
		ds := dstore.Get(c)
		ctr := &Counter{Name: r.Name}
		if err := ds.Get(ctr); err != nil && err != dstore.ErrNoSuchEntity {
			return err
		}
		rsp.Prev = ctr.Val
		ctr.Val += r.Delta
		rsp.Cur = ctr.Val
		return ds.Put(ctr)
	}, nil)
	return
}

var addMethodInfo = &endpoints.MethodInfo{
	Path: "counter/{Name}",
	Desc: "Add an an amount to a particular counter",
}
