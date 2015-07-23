// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package prod

import (
	"net/http"

	"golang.org/x/net/context"
	"google.golang.org/appengine"
)

// Use adds production implementations for all the gae services to the context,
// using the existing context obtained by appengine.NewContext.
//
// The services added are:
//   - github.com/luci/gae/service/rawdatastore
//   - github.com/luci/gae/service/taskqueue
//   - github.com/luci/gae/service/memcache
//   - github.com/luci/gae/service/info
//
// These can be retrieved with the <service>.Get functions.
//
// The implementations are all backed by the real appengine SDK functionality,
func Use(c context.Context) context.Context {
	return useRDS(useMC(useTQ(useGI(c))))
}

// Use adds production implementations for all the gae services to the context.
//
// The services added are:
//   - github.com/luci/gae/service/rawdatastore
//   - github.com/luci/gae/service/taskqueue
//   - github.com/luci/gae/service/memcache
//   - github.com/luci/gae/service/info
//
// These can be retrieved with the <service>.Get functions.
//
// The implementations are all backed by the real appengine SDK functionality,
func UseRequest(r *http.Request) context.Context {
	return useRDS(useMC(useTQ(useGI(appengine.NewContext(r)))))
}
