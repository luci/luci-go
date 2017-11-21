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
	"strings"
	"time"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"

	"go.chromium.org/luci/appengine/gaeauth/server"
	"go.chromium.org/luci/appengine/gaemiddleware"
	"go.chromium.org/luci/appengine/gaemiddleware/standard"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/grpc/discovery"
	"go.chromium.org/luci/grpc/grpcmon"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/middleware"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/templates"

	milo "go.chromium.org/luci/milo/api/proto"
	"go.chromium.org/luci/milo/buildsource"
	"go.chromium.org/luci/milo/buildsource/buildbot"
	"go.chromium.org/luci/milo/buildsource/buildbucket"
	"go.chromium.org/luci/milo/buildsource/rawpresentation"
	"go.chromium.org/luci/milo/buildsource/swarming"
	"go.chromium.org/luci/milo/rpc"
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
	r.GET("/p", frontendMW, movedPermanently("/"))
	r.GET("/search", htmlMW, searchHandler)
	r.GET("/opensearch.xml", frontendMW, searchXMLHandler)

	// Admin and cron endpoints.
	r.GET("/admin/configs", htmlMW, ConfigsHandler)

	// Cron endpoints
	r.GET("/internal/cron/fix-datastore", cronMW, cronFixDatastore)
	r.GET("/internal/cron/stats", cronMW, StatsHandler)
	r.GET("/internal/cron/update-config", cronMW, UpdateConfigHandler)

	// Builds.
	r.GET("/p/:project/builds/b:id", htmlMW, func(c *router.Context) {
		BuildHandler(c, &buildbucket.BuildID{
			Project: c.Params.ByName("project"),
			Address: c.Params.ByName("id"),
		})
	})
	r.GET("/p/:project/builders/:bucket/:builder/:number", htmlMW, func(c *router.Context) {
		BuildHandler(c, &buildbucket.BuildID{
			Project: c.Params.ByName("project"),
			Address: fmt.Sprintf("%s/%s/%s",
				c.Params.ByName("bucket"),
				c.Params.ByName("builder"),
				c.Params.ByName("number")),
		})
	})

	// Console
	r.GET("/p/:project", htmlMW, func(c *router.Context) {
		ConsolesHandler(c, c.Params.ByName("project"))
	})
	r.GET("/p/:project/", frontendMW, movedPermanently("/p/:project"))
	r.GET("/p/:project/g", frontendMW, movedPermanently("/p/:project"))
	r.GET("/p/:project/g/:group/console", htmlMW, ConsoleHandler)
	r.GET("/p/:project/g/:group", htmlMW, redirect("/p/:project/g/:group/console", http.StatusFound))
	r.GET("/p/:project/g/:group/", frontendMW, movedPermanently("/p/:project/g/:group"))

	// Builder list
	r.GET("/p/:project/builders", htmlMW, func(c *router.Context) {
		BuildersRelativeHandler(c, c.Params.ByName("project"), "")
	})
	r.GET("/p/:project/g/:group/builders", htmlMW, func(c *router.Context) {
		BuildersRelativeHandler(c, c.Params.ByName("project"), c.Params.ByName("group"))
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
	// If these routes change, also change links in common/model/build_summary.go:getLinkFromBuildID
	// and common/model/builder_summary.go:SelfLink.
	r.GET("/p/:project/builders/:bucket/:builder", htmlMW, func(c *router.Context) {
		// TODO(nodir): use project parameter.
		// Besides implementation, requires deleting the redirect for
		// /buildbucket/:bucket/:builder
		// because it assumes that project is not used here and
		// simply passes project=chromium.

		BuilderHandler(c, buildsource.BuilderID(
			fmt.Sprintf("buildbucket/%s/%s", c.Params.ByName("bucket"), c.Params.ByName("builder"))))
	})
	// TODO(nodir): delete this redirect and the chromium project assumption with it
	r.GET("/buildbucket/:bucket/:builder", frontendMW, movedPermanently("/p/chromium/builders/:bucket/:builder"))

	// Buildbot
	// If these routes change, also change links in common/model/builder_summary.go:SelfLink.
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
	r.GET("/buildbot/:master/", frontendMW, func(c *router.Context) {
		u := *c.Request.URL
		u.Path = "/search"
		u.RawQuery = fmt.Sprintf("q=%s", c.Params.ByName("master"))
		http.Redirect(c.Writer, c.Request, u.String(), http.StatusMovedPermanently)
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
		Prelude: buildbotAPIPrelude,
	})
	milo.RegisterBuildInfoServer(&api, &rpc.BuildInfoService{})
	discovery.Enable(&api)
	api.InstallHandlers(r, frontendMW)

	http.DefaultServeMux.Handle("/", r)
}

func buildbotAPIPrelude(c context.Context, methodName string, req proto.Message) (context.Context, error) {
	deprecatable, ok := req.(interface {
		GetExcludeDeprecated() bool
	})
	if ok && !deprecatable.GetExcludeDeprecated() {
		logging.Warningf(c, "user agent %q might be using deprecated API!", getRequest(c).UserAgent())
	}

	return c, nil
}

// redirect returns a handler that responds with given HTTP status
// with a location specified by the pathTemplate.
func redirect(pathTemplate string, status int) router.Handler {
	if !strings.HasPrefix(pathTemplate, "/") {
		panic("pathTemplate must start with /")
	}

	return func(c *router.Context) {
		parts := strings.Split(pathTemplate, "/")
		for i, p := range parts {
			if strings.HasPrefix(p, ":") {
				parts[i] = c.Params.ByName(p[1:])
			}
		}
		u := *c.Request.URL
		u.Path = strings.Join(parts, "/")
		http.Redirect(c.Writer, c.Request, u.String(), status)
	}
}

// movedPermanently is a special instance of redirect, returning a handler
// that responds with HTTP 301 (Moved Permanently) with a location specified
// by the pathTemplate.
//
// TODO(nodir,iannucci): delete all usages.
func movedPermanently(pathTemplate string) router.Handler {
	return redirect(pathTemplate, http.StatusMovedPermanently)
}
