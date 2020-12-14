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
	"sync"
	"time"

	"go.chromium.org/luci/appengine/gaemiddleware"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/config/server/cfgmodule"

	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/gaeemulation"
	"go.chromium.org/luci/server/module"
	"go.chromium.org/luci/server/redisconn"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/tq"

	migrationpb "go.chromium.org/luci/cv/api/migration"
	"go.chromium.org/luci/cv/internal/config/configcron"
	"go.chromium.org/luci/cv/internal/migration"
	"go.chromium.org/luci/cv/internal/servicecfg"
)

func main() {
	modules := []module.Module{
		cfgmodule.NewModuleFromFlags(),
		gaeemulation.NewModuleFromFlags(),
		redisconn.NewModuleFromFlags(),
		tq.NewModuleFromFlags(),
	}

	server.Main(nil, modules, func(srv *server.Server) error {
		// Register pRPC servers.
		migrationpb.RegisterMigrationServer(srv.PRPC, &migration.MigrationServer{})

		srv.Routes.GET("/internal/cron/refresh-config",
			router.NewMiddlewareChain(gaemiddleware.RequireCron),
			func(rc *router.Context) {
				// The cron job interval is 1 minute.
				ctx, cancel := context.WithTimeout(rc.Context, 1*time.Minute)
				defer cancel()
				wg := sync.WaitGroup{}
				errs := make(errors.MultiError, 2)

				wg.Add(1)
				go func() {
					defer wg.Done()
					errs[0] = servicecfg.ImportConfig(ctx)
				}()

				wg.Add(1)
				go func() {
					defer wg.Done()
					errs[1] = configcron.SubmitRefreshTasks(ctx)
				}()

				wg.Wait()
				rc.Writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
				code := 200
				for _, err := range errs {
					if err != nil {
						errors.Log(ctx, err)
						code = 500
					}
				}
				rc.Writer.WriteHeader(code)
			})

		return nil
	})
}
