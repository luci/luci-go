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

	"golang.org/x/sync/errgroup"

	"go.chromium.org/luci/config/server/cfgmodule"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/cron"
	"go.chromium.org/luci/server/encryptedcookies"
	"go.chromium.org/luci/server/gaeemulation"
	"go.chromium.org/luci/server/module"
	"go.chromium.org/luci/server/redisconn"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/secrets"
	"go.chromium.org/luci/server/tq"
	_ "go.chromium.org/luci/server/tq/txn/datastore"

	// Using datastore for user sessions.
	_ "go.chromium.org/luci/server/encryptedcookies/session/datastore"

	migrationpb "go.chromium.org/luci/cv/api/migration"
	rpcpb "go.chromium.org/luci/cv/api/rpc/v0"
	"go.chromium.org/luci/cv/internal/aggrmetrics"
	"go.chromium.org/luci/cv/internal/changelist"
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
	"go.chromium.org/luci/cv/internal/rpc/admin"
	adminpb "go.chromium.org/luci/cv/internal/rpc/admin/api"
	rpcv0 "go.chromium.org/luci/cv/internal/rpc/v0"
	"go.chromium.org/luci/cv/internal/run"
	runimpl "go.chromium.org/luci/cv/internal/run/impl"
	"go.chromium.org/luci/cv/internal/userhtml"
)

func init() {
	// TODO(crbug/1242998): delete this once it becomes default.
	datastore.EnableSafeGet()
}

func main() {
	modules := []module.Module{
		cfgmodule.NewModuleFromFlags(),
		cron.NewModuleFromFlags(),
		secrets.NewModuleFromFlags(),
		encryptedcookies.NewModuleFromFlags(),
		gaeemulation.NewModuleFromFlags(),
		redisconn.NewModuleFromFlags(),
		tq.NewModuleFromFlags(),
	}

	server.Main(nil, modules, func(srv *server.Server) error {
		if srv.Options.CloudProject == "luci-change-verifier-dev" {
			srv.Context = common.SetDev(srv.Context)
		}

		gFactory, err := gerrit.NewFactory(
			srv.Context,
			// 3 US mirrors should suffice, effectively replicating a "quorum".
			// These can be moved to to the service config if they have to be changed
			// frequently.
			"us1-mirror-", "us2-mirror-", "us3-mirror-",
		)
		if err != nil {
			return err
		}

		// Register TQ handlers.
		pmNotifier := prjmanager.NewNotifier(&tq.Default)
		runNotifier := run.NewNotifier(&tq.Default)
		clMutator := changelist.NewMutator(&tq.Default, pmNotifier, runNotifier)
		clUpdater := updater.New(&tq.Default, gFactory, clMutator)
		_ = pmimpl.New(pmNotifier, runNotifier, clMutator, gFactory, clUpdater)
		tc, err := tree.NewClient(srv.Context)
		if err != nil {
			return err
		}
		bqc, err := bq.NewProdClient(srv.Context, srv.Options.CloudProject)
		if err != nil {
			return err
		}
		_ = runimpl.New(runNotifier, pmNotifier, clMutator, clUpdater, gFactory, tc, bqc)

		// Register pRPC servers.
		migrationpb.RegisterMigrationServer(srv.PRPC, &migration.MigrationServer{
			GFactory:    gFactory,
			RunNotifier: runNotifier,
		})
		adminpb.RegisterAdminServer(srv.PRPC, &admin.AdminServer{
			TQDispatcher:  &tq.Default,
			GerritUpdater: clUpdater,
			PMNotifier:    pmNotifier,
			RunNotifier:   runNotifier,
		})
		rpcpb.RegisterRunsServer(srv.PRPC, &rpcv0.RunsServer{})

		// Register cron.
		pcr := prjcfg.NewRefresher(&tq.Default, pmNotifier)
		cron.RegisterHandler("refresh-config", func(ctx context.Context) error {
			return refreshConfig(ctx, pcr)
		})
		aggregator := aggrmetrics.New(srv.Context, &tq.Default)
		cron.RegisterHandler("aggregate-metrics", func(ctx context.Context) error {
			return aggregator.Cron(ctx)
		})

		// The service has no UI, so just redirect to the RPC Explorer.
		srv.Routes.GET("/", nil, func(c *router.Context) {
			http.Redirect(c.Writer, c.Request, "/rpcexplorer/", http.StatusFound)
		})

		userhtml.InstallHandlers(srv)

		return nil
	})
}

func refreshConfig(ctx context.Context, pcr *prjcfg.Refresher) error {
	// The cron job interval is 1 minute.
	ctx, cancel := context.WithTimeout(ctx, 1*time.Minute)
	defer cancel()
	eg, ctx := errgroup.WithContext(ctx)
	eg.Go(func() error { return srvcfg.ImportConfig(ctx) })
	eg.Go(func() error { return pcr.SubmitRefreshTasks(ctx) })
	return eg.Wait()
}
