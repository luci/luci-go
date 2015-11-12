// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package frontend implements HTTP server that handles requests to default
// module.
package frontend

import (
	"fmt"
	"html/template"
	"net/http"

	"github.com/julienschmidt/httprouter"
	"golang.org/x/net/context"

	"github.com/luci/luci-go/appengine/gaeauth/server"
	"github.com/luci/luci-go/appengine/gaemiddleware"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/server/auth"
	"github.com/luci/luci-go/server/auth/identity"
	"github.com/luci/luci-go/server/auth/signing"
	"github.com/luci/luci-go/server/middleware"

	"github.com/luci/luci-go/appengine/cmd/helloworld/templates"
)

// base is the root of the middleware chain.
func base(h middleware.Handler) httprouter.Handle {
	methods := auth.Authenticator{
		&server.OAuth2Method{Scopes: []string{server.EmailScope}},
		server.CookieAuth,
		&server.InboundAppIDAuthMethod{},
	}
	return gaemiddleware.BaseProd(auth.Use(h, methods))
}

// authHandler returns handler that perform authentication, but does not
// enforce a login.
func authHandler(h middleware.Handler) httprouter.Handle {
	return base(auth.Authenticate(h))
}

//// Routes.

func init() {
	router := httprouter.New()
	server.InstallHandlers(router, gaemiddleware.BaseProd)
	signing.InstallHandlers(router, gaemiddleware.BaseProd)
	router.GET("/", authHandler(indexPage))
	router.GET("/_ah/warmup", authHandler(warmupHandler))
	http.DefaultServeMux.Handle("/", router)
}

//// Handlers.

func indexPage(c context.Context, w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	fail := func(msg string, err error) {
		logging.Errorf(c, "HTTP 500: %s - %s", msg, err)
		http.Error(w, fmt.Sprintf("%s - %s", msg, err), 500)
	}
	// TODO(vadimsh): Improve the way we use templates. Add caching, default
	// environment, etc.
	tmpl, err := template.New("index.html").Parse(templates.GetAssetString("index.html"))
	if err != nil {
		fail("Failed to parse template", err)
		return
	}
	loginURL, err := auth.LoginURL(c, "/")
	if err != nil {
		fail("Failed to generate login URL", err)
		return
	}
	logoutURL, err := auth.LogoutURL(c, "/")
	if err != nil {
		fail("Failed to generate logout URL", err)
		return
	}
	isAdmin, err := auth.IsMember(c, "administrators")
	if err != nil {
		fail("Failed to check group membership", err)
		return
	}
	tc := map[string]interface{}{
		"HasUser":   auth.CurrentIdentity(c).Kind() != identity.Anonymous,
		"User":      auth.CurrentUser(c),
		"LoginURL":  loginURL,
		"LogoutURL": logoutURL,
		"IsAdmin":   isAdmin,
	}
	if err := tmpl.Execute(w, tc); err != nil {
		fail("Failed to execute the template", err)
	}
}

func warmupHandler(c context.Context, w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	if _, err := fetchAppSettings(c); err != nil {
		http.Error(w, fmt.Sprintf("Failed to load app settings - %s", err), 500)
		return
	}
	if err := server.Warmup(c); err != nil {
		http.Error(w, fmt.Sprintf("Failed to warmup auth - %s", err), 500)
		return
	}
	w.Write([]byte("ok"))
}
