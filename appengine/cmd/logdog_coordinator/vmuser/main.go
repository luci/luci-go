// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"net/http"

	"github.com/julienschmidt/httprouter"
	gaeauthServer "github.com/luci/luci-go/appengine/gaeauth/server"
	"github.com/luci/luci-go/appengine/gaemiddleware"
	"github.com/luci/luci-go/appengine/logdog/coordinator"
	"github.com/luci/luci-go/appengine/logdog/coordinator/config"
	"github.com/luci/luci-go/appengine/logdog/coordinator/endpoints/admin"
	"github.com/luci/luci-go/appengine/logdog/coordinator/endpoints/logs"
	"github.com/luci/luci-go/appengine/logdog/coordinator/endpoints/services"
	adminPb "github.com/luci/luci-go/common/api/logdog_coordinator/admin/v1"
	logsPb "github.com/luci/luci-go/common/api/logdog_coordinator/logs/v1"
	servicesPb "github.com/luci/luci-go/common/api/logdog_coordinator/services/v1"
	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/server/discovery"
	"github.com/luci/luci-go/server/middleware"
	"github.com/luci/luci-go/server/prpc"
	"golang.org/x/net/context"
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
	h = coordinator.WithProdServices(h)
	return gaemiddleware.BaseProd(h)
}

// Run installs and executes this site.
func main() {
	router := httprouter.New()

	// Setup Cloud Endpoints.
	svr := prpc.Server{
		AccessControl: accessControl,
	}
	adminPb.RegisterAdminServer(&svr, admin.New())
	servicesPb.RegisterServicesServer(&svr, services.New())
	logsPb.RegisterLogsServer(&svr, logs.New())
	discovery.Enable(&svr)

	// Standard HTTP endpoints.
	gaeauthServer.InstallHandlers(router, base)
	svr.InstallHandlers(router, base)

	// Redirect "/" to "/app/".
	router.GET("/", func(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		http.Redirect(w, r, "/app/", http.StatusFound)
	})

	http.Handle("/", router)
	appengine.Main()
}

func accessControl(c context.Context, origin string) bool {
	gcfg, err := config.LoadGlobalConfig(c)
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to get global config for access control check.")
		return false
	}

	cfg, err := gcfg.LoadConfig(c)
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to get application config for access control check.")
		return false
	}

	ccfg := cfg.GetCoordinator()
	if ccfg == nil {
		return false
	}

	for _, o := range ccfg.RpcAllowOrigins {
		if o == origin {
			return true
		}
	}
	return false
}
