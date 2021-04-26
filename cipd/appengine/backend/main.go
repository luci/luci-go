// Copyright 2017 The LUCI Authors.
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

// Binary backend implements HTTP server that handles task queues and crons.
package main

import (
	"flag"
	"net/http"

	"go.chromium.org/luci/appengine/gaemiddleware"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/config/server/cfgmodule"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/dsmapper"
	"go.chromium.org/luci/server/gaeemulation"
	"go.chromium.org/luci/server/module"
	"go.chromium.org/luci/server/redisconn"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/secrets"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/cipd/appengine/impl"
	"go.chromium.org/luci/cipd/appengine/impl/monitoring"
	"go.chromium.org/luci/cipd/appengine/impl/settings"
)

func main() {
	modules := []module.Module{
		cfgmodule.NewModuleFromFlags(),
		gaeemulation.NewModuleFromFlags(),
		redisconn.NewModuleFromFlags(),
		secrets.NewModuleFromFlags(),
		tq.NewModuleFromFlags(),
		dsmapper.NewModuleFromFlags(),
	}

	s := &settings.Settings{}
	s.Register(flag.CommandLine)

	server.Main(nil, modules, func(srv *server.Server) error {
		if err := s.Validate(); err != nil {
			return err
		}
		impl.InitForGAE2(s)

		// Needed when using manual scaling.
		srv.Routes.GET("/_ah/start", router.MiddlewareChain{}, func(ctx *router.Context) {
			ctx.Writer.Write([]byte("OK"))
		})
		srv.Routes.GET("/_ah/stop", router.MiddlewareChain{}, func(ctx *router.Context) {
			ctx.Writer.Write([]byte("OK"))
		})

		srv.Routes.GET("/internal/cron/import-config",
			router.NewMiddlewareChain(gaemiddleware.RequireCron),
			func(ctx *router.Context) {
				if err := monitoring.ImportConfig(ctx.Context); err != nil {
					errors.Log(ctx.Context, err)
				}
				ctx.Writer.WriteHeader(http.StatusOK)
			},
		)

		return nil
	})
}
