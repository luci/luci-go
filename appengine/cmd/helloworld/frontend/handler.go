// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package frontend implements HTTP server that handles requests to default
// module.
package frontend

import (
	"net/http"
	"strings"

	"github.com/golang/protobuf/proto"
	"github.com/julienschmidt/httprouter"
	"golang.org/x/net/context"
	"google.golang.org/appengine"

	"github.com/luci/gae/service/info"
	"github.com/luci/luci-go/appengine/cmd/helloworld/proto"
	"github.com/luci/luci-go/appengine/gaeauth/server"
	"github.com/luci/luci-go/appengine/gaemiddleware"
	"github.com/luci/luci-go/server/auth"
	"github.com/luci/luci-go/server/discovery"
	"github.com/luci/luci-go/server/middleware"
	"github.com/luci/luci-go/server/prpc"
	"github.com/luci/luci-go/server/templates"
)

// templateBundle is used to render HTML templates. It provides a base args
// passed to all templates.
var templateBundle = &templates.Bundle{
	Loader:    templates.FileSystemLoader("templates"),
	DebugMode: appengine.IsDevAppServer(),
	DefaultArgs: func(c context.Context) (templates.Args, error) {
		loginURL, err := auth.LoginURL(c, "/")
		if err != nil {
			return nil, err
		}
		logoutURL, err := auth.LogoutURL(c, "/")
		if err != nil {
			return nil, err
		}
		isAdmin, err := auth.IsMember(c, "administrators")
		if err != nil {
			return nil, err
		}
		return templates.Args{
			"AppVersion":  strings.Split(info.Get(c).VersionID(), ".")[0],
			"IsAnonymous": auth.CurrentIdentity(c) == "anonymous:anonymous",
			"IsAdmin":     isAdmin,
			"User":        auth.CurrentUser(c),
			"LoginURL":    loginURL,
			"LogoutURL":   logoutURL,
		}, nil
	},
}

// pageBase is middleware for page handlers.
func pageBase(h middleware.Handler) httprouter.Handle {
	h = auth.Use(h, auth.Authenticator{server.CookieAuth})
	h = templates.WithTemplates(h, templateBundle)
	if !appengine.IsDevAppServer() {
		h = middleware.WithPanicCatcher(h)
	}
	return gaemiddleware.BaseProd(h)
}

// prpcBase is middleware for pRPC API handlers.
func prpcBase(h middleware.Handler) httprouter.Handle {
	// OAuth 2.0 with email scope is registered as a default authenticator
	// by importing "github.com/luci/luci-go/appengine/gaeauth/server".
	// No need to setup an authenticator here.
	//
	// For authorization checks, we use per-service decorators; see
	// service registration code.

	if !appengine.IsDevAppServer() {
		h = middleware.WithPanicCatcher(h)
	}
	return gaemiddleware.BaseProd(h)
}

//// Routes.

func checkAPIAccess(c context.Context, methodName string, req proto.Message) (context.Context, error) {
	// Implement authorization check here, for example:
	// hasAccess, err := auth.IsMember(c, "my-users")
	// if err != nil {
	// 	return nil, grpcutil.Errf(codes.Internal, "%s", err)
	// }
	// if !hasAccess {
	// 	return nil, grpcutil.Errf(codes.PermissionDenied, "%s is not allowed to call APIs", auth.CurrentIdentity(c))
	// }

	return c, nil
}

func init() {
	router := httprouter.New()
	server.InstallHandlers(router, pageBase)
	router.GET("/", pageBase(auth.Authenticate(indexPage)))

	var api prpc.Server
	helloworld.RegisterGreeterServer(&api, &helloworld.DecoratedGreeter{
		Service: &greeterService{},
		Prelude: checkAPIAccess,
	})
	discovery.Enable(&api)
	api.InstallHandlers(router, prpcBase)

	http.DefaultServeMux.Handle("/", router)
}

//// Handlers.

func indexPage(c context.Context, w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	templates.MustRender(c, w, "pages/index.html", nil)
}
