// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package prod

import (
	"net/http"

	"golang.org/x/net/context"
	"google.golang.org/appengine"
)

type key int

var (
	prodContextKey key
	probeCacheKey  key = 1
)

// AEContext retrieves the raw "google.golang.org/appengine" compatible Context.
func AEContext(c context.Context) context.Context {
	aeCtx, _ := c.Value(prodContextKey).(context.Context)
	return aeCtx
}

// Use adds production implementations for all the gae services to the
// context.
//
// The services added are:
//   - github.com/luci/gae/service/datastore
//   - github.com/luci/gae/service/taskqueue
//   - github.com/luci/gae/service/memcache
//   - github.com/luci/gae/service/info
//   - github.com/luci/gae/service/urlfetch
//
// These can be retrieved with the <service>.Get functions.
//
// The implementations are all backed by the real appengine SDK functionality,
func Use(c context.Context, r *http.Request) context.Context {
	aeCtx := appengine.NewContext(r)
	c = context.WithValue(c, prodContextKey, aeCtx)
	return useUser(useURLFetch(useRDS(useMC(useTQ(useGI(c))))))
}
