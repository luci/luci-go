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

	"go.chromium.org/luci/config/server/cfgmodule"
	"golang.org/x/sync/errgroup"

	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/cron"
	"go.chromium.org/luci/server/gaeemulation"
	"go.chromium.org/luci/server/module"
	"go.chromium.org/luci/server/redisconn"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/tq"
	_ "go.chromium.org/luci/server/tq/txn/datastore"

	migrationpb "go.chromium.org/luci/cv/api/migration"
	"go.chromium.org/luci/cv/internal/admin"
	adminpb "go.chromium.org/luci/cv/internal/admin/api"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/common/bq"
	"go.chromium.org/luci/cv/internal/common/tree"
	"go.chromium.org/luci/cv/internal/configs/prjcfg"
	"go.chromium.org/luci/cv/internal/configs/srvcfg"
	"go.chromium.org/luci/cv/internal/gerrit"
	"go.chromium.org/luci/cv/internal/gerrit/updater"
	"go.chromium.org/luci/cv/internal/migration"
	"go.chromium.org/luci/cv/internal/prjmanager"
	pmimpl "go.chromium.org/luci/cv/internal/prjmanager/manager"
	"go.chromium.org/luci/cv/internal/run"
	runimpl "go.chromium.org/luci/cv/internal/run/impl"
)

func main() {
	modules := []module.Module{
		cfgmodule.NewModuleFromFlags(),
		cron.NewModuleFromFlags(),
		gaeemulation.NewModuleFromFlags(),
		redisconn.NewModuleFromFlags(),
		tq.NewModuleFromFlags(),
	}

	server.Main(nil, modules, func(srv *server.Server) error {
		if srv.Options.CloudProject == "luci-change-verifier-dev" {
			srv.Context = common.SetDev(srv.Context)
		}

		switch ctx, err := gerrit.UseProd(srv.Context); {
		case err != nil:
			return nil
		default:
			srv.Context = ctx
		}

		// Register TQ handlers.
		pmNotifier := prjmanager.NewNotifier(&tq.Default)
		runNotifier := run.NewNotifier(&tq.Default)
		clUpdater := updater.New(&tq.Default, pmNotifier, runNotifier)
		_ = pmimpl.New(pmNotifier, runNotifier, clUpdater)
		tc, err := tree.NewClient(srv.Context)
		if err != nil {
			return err
		}
		bqc, err := bq.NewProdClient(srv.Context, srv.Options.CloudProject)
		if err != nil {
			return err
		}
		_ = runimpl.New(runNotifier, pmNotifier, clUpdater, tc, bqc)

		// Register pRPC servers.
		migrationpb.RegisterMigrationServer(srv.PRPC, &migration.MigrationServer{
			RunNotifier: runNotifier,
		})
		adminpb.RegisterAdminServer(srv.PRPC, &admin.AdminServer{
			GerritUpdater: clUpdater,
			PMNotifier:    pmNotifier,
			RunNotifier:   runNotifier,
		})

		// Register cron.
		pcr := prjcfg.NewRefresher(&tq.Default, pmNotifier)
		cron.RegisterHandler("refresh-config", func(ctx context.Context) error {
			return refreshConfig(ctx, pcr)
		})

		// The service has no UI, so just redirect to the RPC Explorer.
		srv.Routes.GET("/", nil, func(c *router.Context) {
			http.Redirect(c.Writer, c.Request, "/rpcexplorer/", http.StatusFound)
		})

		return nil
	})
}

func refreshConfig(ctx context.Context, pcr *prjcfg.ProjectConfigRefresher) error {
	// The cron job interval is 1 minute.
	ctx, cancel := context.WithTimeout(ctx, 1*time.Minute)
	defer cancel()
	eg, ctx := errgroup.WithContext(ctx)
	eg.Go(func() error { return srvcfg.ImportConfig(ctx) })
	eg.Go(func() error { return pcr.SubmitRefreshTasks(ctx) })
	return eg.Wait()
}
