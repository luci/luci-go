// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package helloworld

import (
	"net/http"

	"github.com/julienschmidt/httprouter"
	"google.golang.org/appengine"

	"github.com/luci/luci-go/appengine/gaeauth/server"
	"github.com/luci/luci-go/appengine/gaemiddleware"
	"github.com/luci/luci-go/server/auth"
	"github.com/luci/luci-go/server/middleware"
)

// base is the root of the middleware chain.
func base(h middleware.Handler) httprouter.Handle {
	methods := auth.Authenticator{
		&server.OAuth2Method{Scopes: []string{server.EmailScope}},
		server.CookieAuth,
		&server.InboundAppIDAuthMethod{},
	}
	h = auth.Use(h, methods)
	if !appengine.IsDevAppServer() {
		h = middleware.WithPanicCatcher(h)
	}
	return gaemiddleware.BaseProd(h)
}

//// Routes.

func init() {
	router := httprouter.New()
	server.InstallHandlers(router, base)

	InstallAPIRoutes(router, base)

	http.DefaultServeMux.Handle("/", router)
}
