// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package middleware

import (
	"net/http"

	"github.com/julienschmidt/httprouter"
	"github.com/luci/gae/impl/memory"
	"github.com/luci/gae/impl/prod"
	"github.com/luci/luci-go/appengine/gaeauth/client"
	"github.com/luci/luci-go/appengine/gaelogger"
	"github.com/luci/luci-go/appengine/gaesettings"
	"github.com/luci/luci-go/common/logging/memlogger"
	"github.com/luci/luci-go/server/proccache"
	"github.com/luci/luci-go/server/settings"
	"golang.org/x/net/context"
)

var (
	// globalProcessCache holds state cached between requests. Used only by
	// BaseProd. Testing context doesn't have a global cache, since we
	// specifically do not want to preserve state between unit tests.
	globalProcessCache = proccache.Cache{}

	// globalSettings holds global app settings lazily updated from the datastore.
	// Used only by BaseProd.
	globalSettings = settings.New(gaesettings.Storage{})
)

// Handler is the type for all middleware handlers. Of particular note, it's the
// same as httprouder.Handle, except that it also has a context parameter.
type Handler func(context.Context, http.ResponseWriter, *http.Request, httprouter.Params)

// Base adapts a middleware-style handler to a httprouter.Handle. It passes
// a new, empty context to `h`.
func Base(h Handler) httprouter.Handle {
	return func(rw http.ResponseWriter, r *http.Request, p httprouter.Params) {
		h(context.Background(), rw, r, p)
	}
}

// BaseProd adapts a middleware-style handler to a httprouter.Handle. It passes
// a new context to `h` with the following services installed:
//   * github.com/luci/gae/impl/prod (production appengine services)
//   * github.com/luci/luci-go/appengine/gaelogger (appengine logging service)
//   * github.com/luci/luci-go/appengine/gaeauth/client (appengine urlfetch transport)
//   * github.com/luci/luci-go/server/proccache (in process memory cache)
//   * github.com/luci/luci-go/server/settings (global app settings)
func BaseProd(h Handler) httprouter.Handle {
	return func(rw http.ResponseWriter, r *http.Request, p httprouter.Params) {
		c := prod.UseRequest(r)
		c = gaelogger.Use(c)
		c = client.UseAnonymousTransport(c)
		c = proccache.Use(c, &globalProcessCache)
		c = settings.Use(c, globalSettings)
		h(c, rw, r, p)
	}
}

// BaseTest adapts a middleware-style handler to a httprouter.Handle. It passes
// a new context to `h` with the following services installed:
//   * github.com/luci/gae/impl/memory (in-memory appengine services)
//   * github.com/luci/luci-go/common/logging/memlogger (in-memory logging service)
func BaseTest(h Handler) httprouter.Handle {
	return func(rw http.ResponseWriter, r *http.Request, p httprouter.Params) {
		c := context.Background()
		c = memory.Use(c)
		c = memlogger.Use(c)
		h(c, rw, r, p)
	}
}
