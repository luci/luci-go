// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gaemiddleware

import (
	"net/http"

	"github.com/julienschmidt/httprouter"
	"github.com/luci/gae/impl/prod"
	"github.com/luci/luci-go/appengine/gaeauth/client"
	"github.com/luci/luci-go/appengine/gaeauth/server/gaesigner"
	"github.com/luci/luci-go/appengine/gaelogger"
	"github.com/luci/luci-go/appengine/gaesecrets"
	"github.com/luci/luci-go/appengine/gaesettings"
	"github.com/luci/luci-go/server/middleware"
	"github.com/luci/luci-go/server/proccache"
	"github.com/luci/luci-go/server/settings"
	"golang.org/x/net/context"
	"google.golang.org/appengine"
)

var (
	// globalProcessCache holds state cached between requests.
	globalProcessCache = &proccache.Cache{}

	// globalSettings holds global app settings lazily updated from the datastore.
	globalSettings = settings.New(gaesettings.Storage{})
)

// WithProd installs the set of standard production AppEngine services:
//   * github.com/luci/gae/impl/prod (production appengine services)
//   * github.com/luci/luci-go/appengine/gaelogger (appengine logging service)
//   * github.com/luci/luci-go/appengine/gaeauth/client (appengine urlfetch transport)
//   * github.com/luci/luci-go/server/proccache (in process memory cache)
//   * github.com/luci/luci-go/server/settings (global app settings)
//   * github.com/luci/luci-go/appengine/gaesecrets (access to secret keys in datastore)
//   * github.com/luci/luci-go/appengine/gaeauth/server/gaesigner (RSA signer)
func WithProd(c context.Context, req *http.Request) context.Context {
	c = prod.Use(appengine.WithContext(c, req))
	c = gaelogger.Use(c)
	c = client.UseAnonymousTransport(c)
	c = proccache.Use(c, globalProcessCache)
	c = settings.Use(c, globalSettings)
	c = gaesecrets.Use(c, nil)
	c = gaesigner.Use(c)
	return c
}

// BaseProd adapts a middleware-style handler to a httprouter.Handle. It
// installs services using InstallProd, then passes the context.
func BaseProd(h middleware.Handler) httprouter.Handle {
	return func(rw http.ResponseWriter, r *http.Request, p httprouter.Params) {
		c := WithProd(context.Background(), r)
		h(c, rw, r, p)
	}
}
