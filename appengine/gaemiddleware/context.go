// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gaemiddleware

import (
	"net/http"

	"github.com/julienschmidt/httprouter"
	"github.com/luci/gae/impl/prod"
	"github.com/luci/luci-go/appengine/gaeauth/client"
	"github.com/luci/luci-go/appengine/gaelogger"
	"github.com/luci/luci-go/appengine/gaesecrets"
	"github.com/luci/luci-go/appengine/gaesettings"
	"github.com/luci/luci-go/server/middleware"
	"github.com/luci/luci-go/server/proccache"
	"github.com/luci/luci-go/server/settings"
)

var (
	// globalProcessCache holds state cached between requests.
	globalProcessCache = &proccache.Cache{}

	// globalSettings holds global app settings lazily updated from the datastore.
	globalSettings = settings.New(gaesettings.Storage{})
)

// BaseProd adapts a middleware-style handler to a httprouter.Handle. It passes
// a new context to `h` with the following services installed:
//   * github.com/luci/gae/impl/prod (production appengine services)
//   * github.com/luci/luci-go/appengine/gaelogger (appengine logging service)
//   * github.com/luci/luci-go/appengine/gaeauth/client (appengine urlfetch transport)
//   * github.com/luci/luci-go/server/proccache (in process memory cache)
//   * github.com/luci/luci-go/server/settings (global app settings)
//   * github.com/luci/luci-go/appengine/gaesecrets (access to secret keys in datastore)
func BaseProd(h middleware.Handler) httprouter.Handle {
	return func(rw http.ResponseWriter, r *http.Request, p httprouter.Params) {
		c := prod.UseRequest(r)
		c = gaelogger.Use(c)
		c = client.UseAnonymousTransport(c)
		c = proccache.Use(c, globalProcessCache)
		c = settings.Use(c, globalSettings)
		c = gaesecrets.Use(c, nil)
		h(c, rw, r, p)
	}
}
