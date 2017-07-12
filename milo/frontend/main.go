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
	"net/http"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"

	"github.com/luci/luci-go/appengine/gaemiddleware"
	"github.com/luci/luci-go/grpc/discovery"
	"github.com/luci/luci-go/grpc/grpcmon"
	"github.com/luci/luci-go/grpc/prpc"
	milo "github.com/luci/luci-go/milo/api/proto"
	"github.com/luci/luci-go/milo/buildsource/buildbot"
	"github.com/luci/luci-go/milo/buildsource/buildbucket"
	"github.com/luci/luci-go/milo/buildsource/rawpresentation"
	"github.com/luci/luci-go/milo/buildsource/swarming"
	"github.com/luci/luci-go/milo/common"
	"github.com/luci/luci-go/milo/frontend/console"
	"github.com/luci/luci-go/milo/rpc"
	"github.com/luci/luci-go/server/router"
)

func emptyPrelude(c context.Context, methodName string, req proto.Message) (context.Context, error) {
	return c, nil
}

// Run sets up all the routes and runs the server.
func Run(templatePath string) {
	// Register plain ol' http handlers.
	r := router.New()
	gaemiddleware.InstallHandlers(r)

	basemw := common.Base(templatePath)
	r.GET("/", basemw, frontpageHandler)

	// Admin and cron endpoints.
	r.GET("/admin/update", basemw.Extend(gaemiddleware.RequireCron), UpdateConfigHandler)
	r.GET("/admin/configs", basemw, ConfigsHandler)

	// Console
	r.GET("/console/:project/:name", basemw, console.ConsoleHandler)
	r.GET("/console/:project", basemw, console.Main)

	// Swarming
	r.GET(swarming.URLBase+"/:id/steps/*logname", basemw, swarming.LogHandler)
	r.GET(swarming.URLBase+"/:id", basemw, swarming.BuildHandler)
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
	r.GET("/raw/build/:logdog_host/:project/*path", basemw, rawpresentation.BuildHandler)

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
