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
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/golang/protobuf/proto"

	"google.golang.org/grpc/metadata"

	"go.chromium.org/luci/appengine/gaeauth/server"
	"go.chromium.org/luci/appengine/gaemiddleware"
	"go.chromium.org/luci/appengine/gaemiddleware/standard"
	"go.chromium.org/luci/buildbucket/deprecated"
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
	"go.chromium.org/luci/milo/buildsource/buildbot/buildstore"
	"go.chromium.org/luci/milo/buildsource/buildbucket"
	"go.chromium.org/luci/milo/buildsource/swarming"
	"go.chromium.org/luci/milo/rpc"
)

// Run sets up all the routes and runs the server.
func Run(templatePath string) {
	// Register plain ol' http handlers.
	r := router.New()
	standard.InstallHandlers(r)

	baseMW := standard.Base()
	apiMW := baseMW.Extend(
		middleware.WithContextTimeout(time.Minute),
		withGitMiddleware,
		withBuildbucketClient,
	)
	htmlMW := baseMW.Extend(
		middleware.WithContextTimeout(time.Minute),
		auth.Authenticate(server.CookieAuth, &server.OAuth2Method{Scopes: []string{server.EmailScope}}),
		withAccessClientMiddleware, // This must be called after the auth.Authenticate middleware.
		withGitMiddleware,
		withBuildbucketClient,
		templates.WithTemplates(getTemplateBundle(templatePath)),
	)
	projectMW := htmlMW.Extend(projectACLMiddleware)
	backendMW := baseMW.Extend(
		middleware.WithContextTimeout(10*time.Minute),
		withBuildbucketClient)
	cronMW := backendMW.Extend(gaemiddleware.RequireCron, withBuildbucketClient)

	r.GET("/", htmlMW, frontpageHandler)
	r.GET("/p", baseMW, movedPermanently("/"))
	r.GET("/search", htmlMW, searchHandler)
	r.GET("/opensearch.xml", baseMW, searchXMLHandler)

	// Admin and cron endpoints.
	r.GET("/admin/configs", htmlMW, ConfigsHandler)

	// Cron endpoints
	r.GET("/internal/cron/stats", cronMW, cronHandler(buildbot.StatsHandler))
	r.GET("/internal/cron/update-config", cronMW, UpdateConfigHandler)
	r.GET("/internal/cron/update-pools", cronMW, cronHandler(buildbucket.UpdatePools))

	// Builds.
	r.GET("/b/:id", htmlMW, handleError(redirectLUCIBuild))
	r.GET("/p/:project/builds/b:id", baseMW, movedPermanently("/b/:id"))
	r.GET("/p/:project/builders/:bucket/:builder/:numberOrId", projectMW, handleError(handleLUCIBuild))

	// Console
	r.GET("/p/:project", projectMW, handleError(func(c *router.Context) error {
		return ConsolesHandler(c, c.Params.ByName("project"))
	}))
	r.GET("/p/:project/", baseMW, movedPermanently("/p/:project"))
	r.GET("/p/:project/g", baseMW, movedPermanently("/p/:project"))
	r.GET("/p/:project/g/:group/console", projectMW, handleError(ConsoleHandler))
	r.GET("/p/:project/g/:group", projectMW, redirect("/p/:project/g/:group/console", http.StatusFound))
	r.GET("/p/:project/g/:group/", baseMW, movedPermanently("/p/:project/g/:group"))

	// Builder list
	r.GET("/p/:project/builders", projectMW, handleError(func(c *router.Context) error {
		return BuildersRelativeHandler(c, c.Params.ByName("project"), "")
	}))
	r.GET("/p/:project/g/:group/builders", projectMW, handleError(func(c *router.Context) error {
		return BuildersRelativeHandler(c, c.Params.ByName("project"), c.Params.ByName("group"))
	}))

	// Swarming
	r.GET(swarming.URLBase+"/:id/steps/*logname", htmlMW, handleError(HandleSwarmingLog))
	r.GET(swarming.URLBase+"/:id", htmlMW, handleError(handleSwarmingBuild))
	// Backward-compatible URLs for Swarming:
	r.GET("/swarming/prod/:id/steps/*logname", htmlMW, handleError(HandleSwarmingLog))
	r.GET("/swarming/prod/:id", htmlMW, handleError(handleSwarmingBuild))

	// Buildbucket
	// If these routes change, also change links in common/model/build_summary.go:getLinkFromBuildID
	// and common/model/builder_summary.go:SelfLink.
	r.GET("/p/:project/builders/:bucket/:builder", projectMW, handleError(BuilderHandler))

	r.GET("/buildbucket/:bucket/:builder", baseMW, redirectFromProjectlessBuilder)

	// Buildbot
	// If these routes change, also change links in common/model/builder_summary.go:SelfLink.
	r.GET("/buildbot/:master/:builder/:number", htmlMW.Extend(emulationMiddleware), handleError(handleBuildbotBuild))
	r.GET("/buildbot/:master/:builder/", htmlMW.Extend(emulationMiddleware), handleError(func(c *router.Context) error {
		return BuilderHandlerLegacy(c, buildsource.BuilderID(
			fmt.Sprintf("buildbot/%s/%s", c.Params.ByName("master"), c.Params.ByName("builder"))))
	}))
	r.GET("/buildbot/:master/", baseMW, func(c *router.Context) {
		u := *c.Request.URL
		u.Path = "/search"
		u.RawQuery = fmt.Sprintf("q=%s", c.Params.ByName("master"))
		http.Redirect(c.Writer, c.Request, u.String(), http.StatusMovedPermanently)
	})

	// LogDog Milo Annotation Streams.
	// This mimics the `logdog://logdog_host/project/*path` url scheme seen on
	// swarming tasks.
	r.GET("/raw/build/:logdog_host/:project/*path", htmlMW, handleError(handleRawPresentationBuild))

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
	api.InstallHandlers(r, apiMW)

	http.DefaultServeMux.Handle("/", r)
}

func buildbotAPIPrelude(c context.Context, methodName string, req proto.Message) (context.Context, error) {
	deprecatable, ok := req.(interface {
		GetExcludeDeprecated() bool
	})
	if ok && !deprecatable.GetExcludeDeprecated() {
		ua := "-"
		if md, ok := metadata.FromIncomingContext(c); ok {
			if m := md["user-agent"]; len(m) > 0 {
				ua = m[0]
			}
		}
		logging.Warningf(c, "user agent %q might be using deprecated API!", ua)
	}

	noemu, ok := req.(interface {
		GetNoEmulation() bool
	})
	// Turn off emulation mode if the request sets the no emulation flag to true.
	emulation := !(ok && noemu.GetNoEmulation())

	return buildstore.WithEmulation(c, emulation), nil

}

// handleError is a wrapper for a handler so that the handler can return an error
// rather than call ErrorHandler directly.
// This should be used for handlers that render webpages.
func handleError(handler func(c *router.Context) error) func(c *router.Context) {
	return func(c *router.Context) {
		if err := handler(c); err != nil {
			ErrorHandler(c, err)
		}
	}
}

// cronHandler is a wrapper for cron handlers which do not require template rendering.
func cronHandler(handler func(c context.Context) error) func(c *router.Context) {
	return func(ctx *router.Context) {
		if err := handler(ctx.Context); err != nil {
			logging.WithError(err).Errorf(ctx.Context, "failed to run")
			ctx.Writer.WriteHeader(http.StatusInternalServerError)
			return
		}
		ctx.Writer.WriteHeader(http.StatusOK)
	}
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

func redirectFromProjectlessBuilder(c *router.Context) {
	bucket := c.Params.ByName("bucket")
	builder := c.Params.ByName("builder")

	project, _ := deprecated.BucketNameToV2(bucket)
	u := *c.Request.URL
	u.Path = fmt.Sprintf("/p/%s/builders/%s/%s", project, bucket, builder)
	http.Redirect(c.Writer, c.Request, u.String(), http.StatusMovedPermanently)
}
