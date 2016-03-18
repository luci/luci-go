// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package frontend implements HTTP server that handles requests to default
// module.
//
// It stitches together all the code.
package frontend

import (
	"net/http"

	"github.com/golang/protobuf/proto"
	"github.com/julienschmidt/httprouter"
	"golang.org/x/net/context"
	"google.golang.org/appengine"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"github.com/luci/gae/service/info"
	"github.com/luci/luci-go/appengine/cmd/tokenserver/services/admin"
	"github.com/luci/luci-go/appengine/gaeauth/server"
	"github.com/luci/luci-go/appengine/gaeconfig"
	"github.com/luci/luci-go/appengine/gaemiddleware"
	"github.com/luci/luci-go/common/api/tokenserver/v1"
	"github.com/luci/luci-go/common/config"
	"github.com/luci/luci-go/common/config/impl/memory"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/server/auth"
	"github.com/luci/luci-go/server/discovery"
	"github.com/luci/luci-go/server/middleware"
	"github.com/luci/luci-go/server/prpc"
)

var (
	// adminServer is implementer of tokenserver.Admin RPC interface.
	adminServer = &admin.Server{
		ConfigFactory: func(c context.Context) (config.Interface, error) {
			// Use fake config data on dev server for simplicity.
			inf := info.Get(c)
			if inf.IsDevAppServer() {
				return memory.New(devServerConfigs(inf.AppID())), nil
			}
			return gaeconfig.New(c)
		},
	}

	// adminServerWithAuth wraps adminServer adding admin check.
	adminServerWithAuth = &tokenserver.DecoratedAdmin{
		Service: adminServer,
		Prelude: func(c context.Context, method string, _ proto.Message) (context.Context, error) {
			logging.Infof(c, "tokenserver.Admin: %q is calling %q", auth.CurrentIdentity(c), method)
			switch admin, err := auth.IsMember(c, "administrators"); {
			case err != nil:
				return nil, grpc.Errorf(codes.Internal, "can't check ACL - %s", err)
			case !admin:
				return nil, grpc.Errorf(codes.PermissionDenied, "not an admin")
			}
			return c, nil
		},
	}
)

func init() {
	router := httprouter.New()
	base := func(h middleware.Handler) httprouter.Handle {
		if !appengine.IsDevAppServer() {
			h = middleware.WithPanicCatcher(h)
		}
		return gaemiddleware.BaseProd(h)
	}

	// Install auth and config handlers.
	server.InstallHandlers(router, base)

	// The service has no UI, so just redirect to stock RPC explorer.
	router.GET("/", func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		http.Redirect(w, r, "/rpcexplorer/", http.StatusFound)
	})

	// Optional warmup routes.
	router.GET("/_ah/warmup", base(warmupHandler))
	router.GET("/_ah/start", base(warmupHandler))

	// Backend routes used for cron and task queues.
	router.GET("/internal/cron/read-config", base(gaemiddleware.RequireCron(readConfigCron)))

	// Install all RPC servers.
	var api prpc.Server
	tokenserver.RegisterAdminServer(&api, adminServerWithAuth)
	discovery.Enable(&api)
	api.InstallHandlers(router, base)

	// Expose all this stuff.
	http.DefaultServeMux.Handle("/", router)
}

/// Routes.

// warmupHandler warms in-memory caches.
func warmupHandler(c context.Context, w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	if err := server.Warmup(c); err != nil {
		panic(err) // let panic catcher deal with it
	}
	w.WriteHeader(http.StatusOK)
}

// readConfigCron is handler for /internal/cron/read-config GAE cron task.
func readConfigCron(c context.Context, w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	if _, err := adminServer.ImportConfig(c, nil); err != nil {
		panic(err) // let panic catcher deal with it
	}
	w.WriteHeader(http.StatusOK)
}
