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
	"time"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"

	"go.chromium.org/luci/appengine/gaemiddleware"
	"go.chromium.org/luci/appengine/gaemiddleware/standard"
	"go.chromium.org/luci/grpc/discovery"
	"go.chromium.org/luci/grpc/grpcmon"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/server/router"

	"go.chromium.org/luci/appengine/gaeauth/server"
	"go.chromium.org/luci/common/logging"
	milo "go.chromium.org/luci/milo/api/proto"
	"go.chromium.org/luci/milo/buildsource"
	"go.chromium.org/luci/milo/buildsource/buildbot"
	"go.chromium.org/luci/milo/buildsource/buildbucket"
	"go.chromium.org/luci/milo/buildsource/rawpresentation"
	"go.chromium.org/luci/milo/buildsource/swarming"
	"go.chromium.org/luci/milo/rpc"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/middleware"
	"go.chromium.org/luci/server/templates"
)

// Run sets up all the routes and runs the server.
func Run(templatePath string) {
	// Register plain ol' http handlers.
	r := router.New()
	standard.InstallHandlers(r)

	baseMW := standard.Base().Extend(withRequestMiddleware)
	frontendMW := baseMW.Extend(middleware.WithContextTimeout(time.Minute))
	htmlMW := frontendMW.Extend(
		auth.Authenticate(server.CookieAuth),
		templates.WithTemplates(getTemplateBundle(templatePath)),
	)
	backendMW := baseMW.Extend(middleware.WithContextTimeout(10 * time.Minute))
	cronMW := backendMW.Extend(gaemiddleware.RequireCron)

	r.GET("/", htmlMW, frontpageHandler)

	// Admin and cron endpoints.
	r.GET("/admin/update", cronMW, UpdateConfigHandler)
	r.GET("/admin/configs", htmlMW, ConfigsHandler)
	r.GET("/admin/stats", cronMW, StatsHandler)

	// New Style URLs go here
	// BuildBucket build.
	r.GET("/p/:project/builds/b:id", htmlMW, func(c *router.Context) {
		BuildHandler(c, &buildbucket.BuildID{
			Project: c.Params.ByName("project"),
			ID:      c.Params.ByName("id"),
		})
	})

	// Console
	r.GET("/console/:project/:name", htmlMW, ConsoleHandler)
	r.GET("/console/:project", htmlMW, func(c *router.Context) {
		ConsolesHandler(c, c.Params.ByName("project"))
	})

	// Swarming
	r.GET(swarming.URLBase+"/:id/steps/*logname", htmlMW, func(c *router.Context) {
		LogHandler(c, &swarming.BuildID{
			TaskID: c.Params.ByName("id"),
			Host:   c.Request.FormValue("server"),
		}, c.Params.ByName("logname"))
	})
	r.GET(swarming.URLBase+"/:id", htmlMW, func(c *router.Context) {
		BuildHandler(c, &swarming.BuildID{
			TaskID: c.Params.ByName("id"),
			Host:   c.Request.FormValue("server"),
		})
	})
	// Backward-compatible URLs for Swarming:
	r.GET("/swarming/prod/:id/steps/*logname", htmlMW, func(c *router.Context) {
		LogHandler(c, &swarming.BuildID{
			TaskID: c.Params.ByName("id"),
		}, c.Params.ByName("logname"))
	})
	r.GET("/swarming/prod/:id", htmlMW, func(c *router.Context) {
		BuildHandler(c, &swarming.BuildID{TaskID: c.Params.ByName("id")})
	})

	// Buildbucket
	r.GET("/buildbucket/:bucket/:builder", htmlMW, func(c *router.Context) {
		BuilderHandler(c, buildsource.BuilderID(
			fmt.Sprintf("buildbucket/%s/%s", c.Params.ByName("bucket"), c.Params.ByName("builder"))))
	})

	// Buildbot
	r.GET("/buildbot/:master/:builder/:build", htmlMW.Extend(emulationMiddleware), func(c *router.Context) {
		BuildHandler(c, &buildbot.BuildID{
			Master:      c.Params.ByName("master"),
			BuilderName: c.Params.ByName("builder"),
			BuildNumber: c.Params.ByName("build"),
		})
	})
	r.GET("/buildbot/:master/:builder/", htmlMW.Extend(emulationMiddleware), func(c *router.Context) {
		BuilderHandler(c, buildsource.BuilderID(
			fmt.Sprintf("buildbot/%s/%s", c.Params.ByName("master"), c.Params.ByName("builder"))))
	})

	// LogDog Milo Annotation Streams.
	// This mimicks the `logdog://logdog_host/project/*path` url scheme seen on
	// swarming tasks.
	r.GET("/raw/build/:logdog_host/:project/*path", htmlMW, func(c *router.Context) {
		BuildHandler(c, rawpresentation.NewBuildID(
			c.Params.ByName("logdog_host"),
			c.Params.ByName("project"),
			c.Params.ByName("path"),
		))
	})

	// PubSub subscription endpoints.
	r.POST("/_ah/push-handlers/buildbot", backendMW, buildbot.PubSubHandler)
	r.POST("/_ah/push-handlers/buildbucket", backendMW, buildbucket.PubSubHandler)

	// pRPC style endpoints.
	api := prpc.Server{
		UnaryServerInterceptor: grpcmon.NewUnaryServerInterceptor(nil),
	}

	milo.RegisterBuildbotServer(&api, &milo.DecoratedBuildbot{
		Service: &buildbot.Service{},
		Prelude: checkUnrestrictedRequest,
	})
	milo.RegisterBuildInfoServer(&api, &rpc.BuildInfoService{})
	discovery.Enable(&api)
	api.InstallHandlers(r, frontendMW)

	http.DefaultServeMux.Handle("/", r)
}

func checkUnrestrictedRequest(c context.Context, methodName string, req proto.Message) (context.Context, error) {
	deprecatable, ok := req.(interface {
		GetExcludeDeprecated() bool
	})
	if ok && !deprecatable.GetExcludeDeprecated() {
		logging.Warningf(c, "user agent %q might be using deprecated API!", getRequest(c).UserAgent())
	}
	return c, nil
}
