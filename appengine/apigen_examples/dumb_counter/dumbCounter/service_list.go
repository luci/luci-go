// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package dumbCounter

import (
	"github.com/GoogleCloudPlatform/go-endpoints/endpoints"
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
func (e *Example) List(c context.Context) (rsp *ListRsp, err error) {
	c, err = e.Use(c, listMethodInfo)
	if err != nil {
		return
	}

	ds := dstore.Get(c)
	rsp = &ListRsp{}
	err = ds.GetAll(dstore.NewQuery("Counter"), &rsp.Counters)
	return
}

var listMethodInfo = &endpoints.MethodInfo{
	Path: "counter",
	Desc: "Returns all of the available counters",
}
