// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package main implements HTTP server that handles requests to backend
// module.
package main

import (
	"net/http"

	"github.com/julienschmidt/httprouter"
	"golang.org/x/net/context"
	"google.golang.org/appengine"

	"github.com/luci/luci-go/appengine/gaeauth/server"
	"github.com/luci/luci-go/appengine/gaemiddleware"
	"github.com/luci/luci-go/server/middleware"
)

// base is the root of the middleware chain.
func base(h middleware.Handler) httprouter.Handle {
	return gaemiddleware.BaseProd(h)
}

//// Routes.

func main() {
	router := httprouter.New()
	server.InstallHandlers(router, base)
	router.GET("/hi", base(sayHi))
	http.DefaultServeMux.Handle("/", router)

	appengine.Main()
}

//// Handlers.

func sayHi(c context.Context, w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	w.Write([]byte("Hi, I'm backend"))
}
