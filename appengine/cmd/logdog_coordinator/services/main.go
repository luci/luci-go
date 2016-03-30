// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package module

import (
	"net/http"

	"github.com/julienschmidt/httprouter"
	"github.com/luci/luci-go/appengine/gaemiddleware"
	"github.com/luci/luci-go/appengine/logdog/coordinator/config"
	"github.com/luci/luci-go/appengine/logdog/coordinator/endpoints/services"
	servicesPb "github.com/luci/luci-go/common/api/logdog_coordinator/services/v1"
	"github.com/luci/luci-go/server/middleware"
	"github.com/luci/luci-go/server/prpc"
	"google.golang.org/appengine"

	// Include mutations package so its Mutations will register with tumble via
	// init().
	_ "github.com/luci/luci-go/appengine/logdog/coordinator/mutations"
)

// base is the root of the middleware chain.
func base(h middleware.Handler) httprouter.Handle {
	if !appengine.IsDevAppServer() {
		h = middleware.WithPanicCatcher(h)
	}
	h = config.WithConfig(h)
	return gaemiddleware.BaseProd(h)
}

// Run installs and executes this site.
func init() {
	router := httprouter.New()

	// Setup Cloud Endpoints.
	svr := prpc.Server{}
	servicesPb.RegisterServicesServer(&svr, &services.Server{})

	// Standard HTTP endpoints.
	svr.InstallHandlers(router, base)

	http.Handle("/", router)
}
