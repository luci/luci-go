// Copyright 2015 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package frontend

import (
	"fmt"
	"net/http"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"

	"go.chromium.org/luci/appengine/gaemiddleware"
	"go.chromium.org/luci/grpc/discovery"
	"go.chromium.org/luci/grpc/grpcmon"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/server/router"

	milo "go.chromium.org/luci/milo/api/proto"
	"go.chromium.org/luci/milo/buildsource"
	"go.chromium.org/luci/milo/buildsource/buildbot"
	"go.chromium.org/luci/milo/buildsource/buildbucket"
	"go.chromium.org/luci/milo/buildsource/rawpresentation"
	"go.chromium.org/luci/milo/buildsource/swarming"
	"go.chromium.org/luci/milo/rpc"
)

func emptyPrelude(c context.Context, methodName string, req proto.Message) (context.Context, error) {
	return c, nil
}

// Run sets up all the routes and runs the server.
func Run(templatePath string) {
	// Register plain ol' http handlers.
	r := router.New()
	gaemiddleware.InstallHandlers(r)

	basemw := base(templatePath)
	r.GET("/", basemw, frontpageHandler)

	// Admin and cron endpoints.
	r.GET("/admin/update", basemw.Extend(gaemiddleware.RequireCron), UpdateConfigHandler)
	r.GET("/admin/configs", basemw, ConfigsHandler)

	// Console
	r.GET("/console/:project/:name", basemw, ConsoleHandler)
	r.GET("/console/:project", basemw, ConsoleMainHandler)

	// Swarming
	r.GET(swarming.URLBase+"/:id/steps/*logname", basemw, func(c *router.Context) {
		LogHandler(c, &swarming.BuildID{
			TaskID: c.Params.ByName("id"),
			Host:   c.Request.FormValue("server"),
		}, c.Params.ByName("logname"))
	})
	r.GET(swarming.URLBase+"/:id", basemw, func(c *router.Context) {
		BuildHandler(c, &swarming.BuildID{
			TaskID: c.Params.ByName("id"),
			Host:   c.Request.FormValue("server"),
		})
	})
	// Backward-compatible URLs for Swarming:
	r.GET("/swarming/prod/:id/steps/*logname", basemw, func(c *router.Context) {
		LogHandler(c, &swarming.BuildID{
			TaskID: c.Params.ByName("id"),
		}, c.Params.ByName("logname"))
	})
	r.GET("/swarming/prod/:id", basemw, func(c *router.Context) {
		BuildHandler(c, &swarming.BuildID{TaskID: c.Params.ByName("id")})
	})

	// Buildbucket
	r.GET("/buildbucket/:bucket/:builder", basemw, func(c *router.Context) {
		BuilderHandler(c, buildsource.BuilderID(
			fmt.Sprintf("buildbucket/%s/%s", c.Params.ByName("bucket"), c.Params.ByName("builder"))))
	})

	// Buildbot
	r.GET("/buildbot/:master/:builder/:build", basemw, func(c *router.Context) {
		BuildHandler(c, &buildbot.BuildID{
			Master:      c.Params.ByName("master"),
			BuilderName: c.Params.ByName("builder"),
			BuildNumber: c.Params.ByName("build"),
		})
	})
	r.GET("/buildbot/:master/:builder/", basemw, func(c *router.Context) {
		BuilderHandler(c, buildsource.BuilderID(
			fmt.Sprintf("buildbot/%s/%s", c.Params.ByName("master"), c.Params.ByName("builder"))))
	})

	// LogDog Milo Annotation Streams.
	// This mimicks the `logdog://logdog_host/project/*path` url scheme seen on
	// swarming tasks.
	r.GET("/raw/build/:logdog_host/:project/*path", basemw, func(c *router.Context) {
		BuildHandler(c, rawpresentation.NewBuildID(
			c.Params.ByName("logdog_host"),
			c.Params.ByName("project"),
			c.Params.ByName("path"),
		))
	})

	// PubSub subscription endpoints.
	r.POST("/_ah/push-handlers/buildbot", basemw, buildbot.PubSubHandler)
	r.POST("/_ah/push-handlers/buildbucket", basemw, buildbucket.PubSubHandler)

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
