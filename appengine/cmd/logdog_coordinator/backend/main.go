// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package module

import (
	"net/http"

	"github.com/julienschmidt/httprouter"
	gaeauthServer "github.com/luci/luci-go/appengine/gaeauth/server"
	"github.com/luci/luci-go/appengine/gaemiddleware"
	"github.com/luci/luci-go/appengine/logdog/coordinator/backend"
	"github.com/luci/luci-go/appengine/logdog/coordinator/config"
	"github.com/luci/luci-go/appengine/tumble"
	"github.com/luci/luci-go/server/auth"
	"github.com/luci/luci-go/server/middleware"
	"google.golang.org/appengine"

	// Include mutations package so its Mutations will register with tumble via
	// init().
	_ "github.com/luci/luci-go/appengine/logdog/coordinator/mutations"
)

func authenticator(scopes ...string) auth.Authenticator {
	return auth.Authenticator{
		&gaeauthServer.OAuth2Method{Scopes: scopes},
		gaeauthServer.CookieAuth,
		&gaeauthServer.InboundAppIDAuthMethod{},
	}
}

// base is the root of the middleware chain.
func base(h middleware.Handler) httprouter.Handle {
	a := authenticator(gaeauthServer.EmailScope)
	h = auth.Use(h, a)
	if !appengine.IsDevAppServer() {
		h = middleware.WithPanicCatcher(h)
	}
	h = config.WithConfig(h)
	return gaemiddleware.BaseProd(h)
}

func init() {
	b := backend.Backend{}
	tmb := tumble.Service{}

	router := httprouter.New()
	b.InstallHandlers(router, base)
	tmb.InstallHandlers(router)

	http.Handle("/", router)
}
