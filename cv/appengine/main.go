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
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/analytics"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/cron"
	"go.chromium.org/luci/server/dsmapper"
	"go.chromium.org/luci/server/encryptedcookies"
	"go.chromium.org/luci/server/gaeemulation"
	"go.chromium.org/luci/server/gerritauth"
	"go.chromium.org/luci/server/module"
	srvquota "go.chromium.org/luci/server/quota"
	"go.chromium.org/luci/server/redisconn"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/secrets"
	"go.chromium.org/luci/server/tq"
	_ "go.chromium.org/luci/server/tq/txn/datastore"

	// Using datastore for user sessions.
	_ "go.chromium.org/luci/server/encryptedcookies/session/datastore"

	// Ensure registration of validation rules before registering cfgcache.
	_ "go.chromium.org/luci/cv/internal/configs/validation"

	apiv0pb "go.chromium.org/luci/cv/api/v0"
	"go.chromium.org/luci/cv/internal/buildbucket"
	bbfacade "go.chromium.org/luci/cv/internal/buildbucket/facade"
	bblistener "go.chromium.org/luci/cv/internal/buildbucket/listener"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/common/bq"
	"go.chromium.org/luci/cv/internal/common/tree"
	"go.chromium.org/luci/cv/internal/configs/prjcfg/refresher"
	"go.chromium.org/luci/cv/internal/configs/srvcfg"
	"go.chromium.org/luci/cv/internal/gerrit"
	gerritupdater "go.chromium.org/luci/cv/internal/gerrit/updater"
	"go.chromium.org/luci/cv/internal/prjmanager"
	pmimpl "go.chromium.org/luci/cv/internal/prjmanager/manager"
	"go.chromium.org/luci/cv/internal/quota"
	"go.chromium.org/luci/cv/internal/retention"
	"go.chromium.org/luci/cv/internal/rpc/admin"
	adminpb "go.chromium.org/luci/cv/internal/rpc/admin/api"
	rpcv0 "go.chromium.org/luci/cv/internal/rpc/v0"
	"go.chromium.org/luci/cv/internal/run"
	runimpl "go.chromium.org/luci/cv/internal/run/impl"
	"go.chromium.org/luci/cv/internal/run/rdb"
	"go.chromium.org/luci/cv/internal/tryjob"
	"go.chromium.org/luci/cv/internal/tryjob/tjcancel"
	tjupdate "go.chromium.org/luci/cv/internal/tryjob/update"
	"go.chromium.org/luci/cv/internal/userhtml"
)

func main() {
	modules := []module.Module{
		analytics.NewModuleFromFlags(),
		cfgmodule.NewModuleFromFlags(),
		cron.NewModuleFromFlags(),
		dsmapper.NewModuleFromFlags(),
		encryptedcookies.NewModuleFromFlags(),
		gaeemulation.NewModuleFromFlags(),
		gerritauth.NewModuleFromFlags(),
		redisconn.NewModuleFromFlags(),
		secrets.NewModuleFromFlags(),
		tq.NewModuleFromFlags(),
		srvquota.NewModuleFromFlags(),
	}

	server.Main(nil, modules, func(srv *server.Server) error {
		env := common.MakeEnv(srv.Options)
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
		tryjobNotifier := tryjob.NewNotifier(&tq.Default)
		clMutator := changelist.NewMutator(&tq.Default, pmNotifier, runNotifier, tryjobNotifier)
		clUpdater := changelist.NewUpdater(&tq.Default, clMutator)
		gerritupdater.RegisterUpdater(clUpdater, gFactory)

		bbFactory := buildbucket.NewClientFactory()
		bbFacade := &bbfacade.Facade{
			ClientFactory: bbFactory,
		}

		tryjobUpdater := tjupdate.NewUpdater(env, tryjobNotifier, runNotifier)
		tryjobUpdater.RegisterBackend(bbFacade)
		tryjobCancellator := tjcancel.NewCancellator(tryjobNotifier)
		tryjobCancellator.RegisterBackend(bbFacade)

		_ = pmimpl.New(pmNotifier, runNotifier, clMutator, gFactory, clUpdater)
		tc, err := tree.NewClient(srv.Context)
		if err != nil {
			return err
		}
		bqc, err := bq.NewProdClient(srv.Context, srv.Options.CloudProject)
		if err != nil {
			return err
		}
		rdbClientFactory := rdb.NewRecorderClientFactory()
		qm := quota.NewManager(gFactory)
		_ = runimpl.New(runNotifier, pmNotifier, tryjobNotifier, clMutator, clUpdater, gFactory, bbFactory, tc, bqc, rdbClientFactory, qm, env)

		// Setup pRPC authentication.
		srv.SetRPCAuthMethods([]auth.Method{
			// The default method used by majority of clients.
			&auth.GoogleOAuth2Method{
				Scopes: []string{"https://www.googleapis.com/auth/userinfo.email"},
			},
			// For authenticating calls from Gerrit plugins.
			&gerritauth.Method,
		})
		srv.ConfigurePRPC(func(p *prpc.Server) {
			p.AccessControl = func(context.Context, string) prpc.AccessControlDecision {
				return prpc.AccessControlDecision{
					AllowCrossOriginRequests: true,
					AllowCredentials:         true,
					AllowHeaders:             []string{gerritauth.Method.Header},
				}
			}
		})

		adminpb.RegisterAdminServer(srv, admin.New(
			&tq.Default, &dsmapper.Default,
			clUpdater, pmNotifier, runNotifier,
		))
		apiv0pb.RegisterRunsServer(srv, &rpcv0.RunsServer{})
		apiv0pb.RegisterGerritIntegrationServer(srv, &rpcv0.GerritIntegrationServer{})

		// Register cron.
		pcr := refresher.NewRefresher(&tq.Default, pmNotifier, qm, env)
		cron.RegisterHandler("refresh-config", func(ctx context.Context) error {
			return refreshConfig(ctx, pcr)
		})
		kickNewListenersFn := bblistener.Register(&tq.Default, srv.Options.CloudProject, tryjobNotifier, tryjobUpdater)
		cron.RegisterHandler("kick-bb-pubsub-listeners", func(ctx context.Context) error {
			return kickNewListenersFn(ctx)
		})
		retention.RegisterCrons(&tq.Default, runNotifier)

		// The service has no general-use UI, so just redirect to the RPC Explorer.
		srv.Routes.GET("/", nil, func(c *router.Context) {
			http.Redirect(c.Writer, c.Request, "/rpcexplorer/", http.StatusFound)
		})

		userhtml.InstallHandlers(srv)

		// When running locally, serve static files ourselves as well.
		if !srv.Options.Prod {
			srv.Routes.Static("/static", nil, http.Dir("./static"))
		}
		return nil
	})
}

func refreshConfig(ctx context.Context, pcr *refresher.Refresher) error {
	// The cron job interval is 1 minute.
	ctx, cancel := context.WithTimeout(ctx, 1*time.Minute)
	defer cancel()
	eg, ctx := errgroup.WithContext(ctx)
	eg.Go(func() error { return srvcfg.ImportConfig(ctx) })
	eg.Go(func() error { return pcr.SubmitRefreshTasks(ctx) })
	return eg.Wait()
}
