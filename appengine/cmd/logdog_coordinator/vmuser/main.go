// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"net/http"

	"github.com/julienschmidt/httprouter"
	gaeauthServer "github.com/luci/luci-go/appengine/gaeauth/server"
	"github.com/luci/luci-go/appengine/gaemiddleware"
	"github.com/luci/luci-go/appengine/logdog/coordinator/config"
	"github.com/luci/luci-go/appengine/logdog/coordinator/endpoints/admin"
	"github.com/luci/luci-go/appengine/logdog/coordinator/endpoints/logs"
	"github.com/luci/luci-go/appengine/logdog/coordinator/endpoints/services"
	adminPb "github.com/luci/luci-go/common/api/logdog_coordinator/admin/v1"
	logsPb "github.com/luci/luci-go/common/api/logdog_coordinator/logs/v1"
	servicesPb "github.com/luci/luci-go/common/api/logdog_coordinator/services/v1"
	"github.com/luci/luci-go/server/discovery"
	"github.com/luci/luci-go/server/middleware"
	"github.com/luci/luci-go/server/prpc"
	"google.golang.org/appengine"
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
func main() {
	router := httprouter.New()

	// Setup Cloud Endpoints.
	svr := prpc.Server{}
	adminPb.RegisterAdminServer(&svr, &admin.Server{})
	servicesPb.RegisterServicesServer(&svr, &services.Server{})
	logsPb.RegisterLogsServer(&svr, &logs.Server{})
	discovery.Enable(&svr)

	gaeauthServer.InstallHandlers(router, base)
	svr.InstallHandlers(router, base)

	http.Handle("/", router)
	appengine.Main()
}
