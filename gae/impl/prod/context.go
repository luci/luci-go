// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package prod

import (
	"net/http"

	"github.com/luci/gae/service/info"
	"golang.org/x/net/context"
	"google.golang.org/appengine"
)

type key int

var (
	prodContextKey      key
	prodContextNoTxnKey key = 1
	probeCacheKey       key = 2
)

// AEContext retrieves the raw "google.golang.org/appengine" compatible Context.
func AEContext(c context.Context) context.Context {
	aeCtx, _ := c.Value(prodContextKey).(context.Context)
	return aeCtx
}

// AEContextNoTxn retrieves the raw "google.golang.org/appengine" compatible
// Context that's not part of a transaction.
func AEContextNoTxn(c context.Context) context.Context {
	aeCtx, _ := c.Value(prodContextNoTxnKey).(context.Context)
	aeCtx, err := appengine.Namespace(aeCtx, info.Get(c).GetNamespace())
	if err != nil {
		panic(err)
	}
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
//   - github.com/luci/gae/service/user
//   - github.com/luci-go/common/logging
//
// These can be retrieved with the <service>.Get functions.
//
// The implementations are all backed by the real appengine SDK functionality,
func Use(c context.Context, r *http.Request) context.Context {
	aeCtx := appengine.NewContext(r)
	c = context.WithValue(c, prodContextKey, aeCtx)
	c = context.WithValue(c, prodContextNoTxnKey, aeCtx)
	return useUser(useURLFetch(useRDS(useMC(useTQ(useGI(useLogging(c)))))))
}
