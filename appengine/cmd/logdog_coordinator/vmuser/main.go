// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"fmt"
	"net/http"

	"github.com/GoogleCloudPlatform/go-endpoints/endpoints"
	"github.com/julienschmidt/httprouter"
	"github.com/luci/luci-go/appengine/ephelper"
	"github.com/luci/luci-go/appengine/ephelper/epfrontend"
	gaeauthServer "github.com/luci/luci-go/appengine/gaeauth/server"
	"github.com/luci/luci-go/appengine/gaemiddleware"
	"github.com/luci/luci-go/appengine/logdog/coordinator/config"
	"github.com/luci/luci-go/appengine/logdog/coordinator/endpoints/admin"
	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/server/auth"
	"github.com/luci/luci-go/server/middleware"
	"golang.org/x/net/context"
	"google.golang.org/appengine"
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
	return gaemiddleware.BaseProd(h)
}

func configureEndpoints(h *ephelper.Helper, s *endpoints.Server, sb *ephelper.ServiceBase) error {
	if err := h.Register(s, &admin.Admin{ServiceBase: *sb}, &admin.Info, admin.MethodInfoMap); err != nil {
		return fmt.Errorf("failed to register 'admin' endpoint: %v", err)
	}
	return nil
}

func installEndpointServices(c context.Context) (context.Context, error) {
	c = gaemiddleware.WithProd(c, endpoints.HTTPRequest(c))

	// Set up LUCI config. We'll fork a Context with an e-mail OAuth2 scope
	// transport to use for "luci-config" service communication.
	//
	// First, load our global settings.
	c, err := config.Install(c)
	if err != nil {
		log.Fields{
			log.ErrorKey: err,
		}.Errorf(c, "Could not install application configuration.")
		return nil, endpoints.NewInternalServerError("application not configured")
	}

	return c, nil
}

// Run installs and executes this site.
func main() {
	router := httprouter.New()

	// Setup Cloud Endpoints.
	ep := endpoints.NewServer("")
	epfe := epfrontend.New("/api/", ep)
	h := ephelper.Helper{
		Frontend: epfe,
	}
	sb := ephelper.ServiceBase{
		InstallServices: installEndpointServices,
	}
	configureEndpoints(&h, ep, &sb)

	// Standard HTTP endpoints.
	gaeauthServer.InstallHandlers(router, base)

	ep.HandleHTTP(nil)
	epfe.HandleHTTP(nil)
	http.Handle("/", router)
	appengine.Main()
}
