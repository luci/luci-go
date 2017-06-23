// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package frontend

import (
	"net/http"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"

	"github.com/luci/luci-go/appengine/gaemiddleware"
	"github.com/luci/luci-go/grpc/discovery"
	"github.com/luci/luci-go/grpc/grpcmon"
	"github.com/luci/luci-go/grpc/prpc"
	milo "github.com/luci/luci-go/milo/api/proto"
	"github.com/luci/luci-go/milo/build_source/buildbot"
	"github.com/luci/luci-go/milo/build_source/buildbucket"
	"github.com/luci/luci-go/milo/build_source/raw_presentation"
	"github.com/luci/luci-go/milo/build_source/swarming"
	"github.com/luci/luci-go/milo/common"
	"github.com/luci/luci-go/milo/frontend/console"
	"github.com/luci/luci-go/milo/rpc"
	"github.com/luci/luci-go/server/router"
)

func emptyPrelude(c context.Context, methodName string, req proto.Message) (context.Context, error) {
	return c, nil
}

// Where it all begins!!!
func Run() {
	// Register plain ol' http handlers.
	r := router.New()
	gaemiddleware.InstallHandlers(r)

	basemw := common.Base("templates")
	r.GET("/", basemw, frontpageHandler)

	// Admin and cron endpoints.
	r.GET("/admin/update", basemw.Extend(gaemiddleware.RequireCron), UpdateConfigHandler)
	r.GET("/admin/configs", basemw, ConfigsHandler)

	// Console
	r.GET("/console/:project/:name", basemw, console.ConsoleHandler)
	r.GET("/console/:project", basemw, console.Main)

	// Swarming
	r.GET("/swarming/task/:id/steps/*logname", basemw, swarming.LogHandler)
	r.GET("/swarming/task/:id", basemw, swarming.BuildHandler)
	// Backward-compatible URLs for Swarming:
	r.GET("/swarming/prod/:id/steps/*logname", basemw, swarming.LogHandler)
	r.GET("/swarming/prod/:id", basemw, swarming.BuildHandler)

	// Buildbucket
	r.GET("/buildbucket/:bucket/:builder", basemw, buildbucket.BuilderHandler)

	// Buildbot
	r.GET("/buildbot/:master/:builder/:build", basemw, buildbot.BuildHandler)
	r.GET("/buildbot/:master/:builder/", basemw, buildbot.BuilderHandler)

	// LogDog Milo Annotation Streams.
	// This mimicks the `logdog://logdog_host/project/*path` url scheme seen on
	// swarming tasks.
	r.GET("/raw/build/:logdog_host/:project/*path", basemw, raw_presentation.BuildHandler)

	// PubSub subscription endpoints.
	r.POST("/_ah/push-handlers/buildbot", basemw, buildbot.PubSubHandler)

	// pRPC style endpoints.
	api := prpc.Server{
		UnaryServerInterceptor: grpcmon.NewUnaryServerInterceptor(nil),
	}
	milo.RegisterBuildbotServer(&api, &milo.DecoratedBuildbot{
		Service: &buildbot.Service{},
		Prelude: emptyPrelude,
	})
	milo.RegisterBuildInfoServer(&api, &milo.DecoratedBuildInfo{
		Service: &rpc.BuildInfoService{},
		Prelude: emptyPrelude,
	})
	discovery.Enable(&api)
	api.InstallHandlers(r, gaemiddleware.BaseProd())

	http.DefaultServeMux.Handle("/", r)
}
