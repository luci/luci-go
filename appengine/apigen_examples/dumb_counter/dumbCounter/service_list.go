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

// ListRsp is the response from the 'List' RPC. It contains a list of Counters
// including their IDs and Values.
type ListRsp struct {
	Counters []Counter
}

// List returns a list of all the counters. Note that it's very poorly
// implemented! It's completely unpaged. I don't care :).
func (Example) List(c context.Context) (rsp *ListRsp, err error) {
	ds := dstore.Get(prod.Use(c))
	rsp = &ListRsp{}
	err = ds.GetAll(dstore.NewQuery("Counter"), &rsp.Counters)
	return
}

var listMethodInfo = &endpoints.MethodInfo{
	Path: "counter",
	Desc: "Returns all of the available counters",
}
