// Copyright 2020 The LUCI Authors.
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

package main

import (
	"context"
	"net/http"
	"time"

	"go.chromium.org/luci/appengine/gaemiddleware"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/config/server/cfgmodule"
	"golang.org/x/sync/errgroup"

	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/gaeemulation"
	"go.chromium.org/luci/server/module"
	"go.chromium.org/luci/server/redisconn"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/tq"

	diagnosticpb "go.chromium.org/luci/cv/api/diagnostic"
	migrationpb "go.chromium.org/luci/cv/api/migration"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/config/configcron"
	"go.chromium.org/luci/cv/internal/diagnostic"
	"go.chromium.org/luci/cv/internal/gerrit"
	"go.chromium.org/luci/cv/internal/migration"
	"go.chromium.org/luci/cv/internal/servicecfg"

	// import all modules with server/tq handler additions in init() calls,
	// which are otherwise not imported directly or transitively via imports
	// above.
	_ "go.chromium.org/luci/cv/internal/prjmanager/impl"
	_ "go.chromium.org/luci/cv/internal/run/impl"
)

func main() {
	modules := []module.Module{
		cfgmodule.NewModuleFromFlags(),
		gaeemulation.NewModuleFromFlags(),
		redisconn.NewModuleFromFlags(),
		tq.NewModuleFromFlags(),
	}

	server.Main(nil, modules, func(srv *server.Server) error {
		if srv.Options.CloudProject == "luci-change-verifier-dev" {
			srv.Context = common.SetDev(srv.Context)
		}
		srv.Context = gerrit.UseProd(srv.Context)

		// Register pRPC servers.
		migrationpb.RegisterMigrationServer(srv.PRPC, &migration.MigrationServer{})
		diagnosticpb.RegisterDiagnosticServer(srv.PRPC, &diagnostic.DiagnosticServer{})

		srv.Routes.GET(
			"/internal/cron/refresh-config",
			router.NewMiddlewareChain(gaemiddleware.RequireCron),
			refreshConfig,
		)

		// The service has no UI, so just redirect to the RPC Explorer.
		srv.Routes.GET("/", router.MiddlewareChain{}, func(c *router.Context) {
			http.Redirect(c.Writer, c.Request, "/rpcexplorer/", http.StatusFound)
		})

		return nil
	})
}

func refreshConfig(rc *router.Context) {
	// The cron job interval is 1 minute.
	ctx, cancel := context.WithTimeout(rc.Context, 1*time.Minute)
	defer cancel()
	eg, ctx := errgroup.WithContext(ctx)
	eg.Go(func() error { return servicecfg.ImportConfig(ctx) })
	eg.Go(func() error { return configcron.SubmitRefreshTasks(ctx) })
	code := 200
	if err := eg.Wait(); err != nil {
		errors.Log(ctx, err)
		code = 500
	}
	rc.Writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
	rc.Writer.WriteHeader(code)
}
