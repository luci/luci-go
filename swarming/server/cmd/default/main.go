// Copyright 2022 The LUCI Authors.
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
	"flag"
	"fmt"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/config/server/cfgmodule"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/auth/rpcacl"
	"go.chromium.org/luci/server/cron"
	"go.chromium.org/luci/server/encryptedcookies"
	"go.chromium.org/luci/server/gaeemulation"
	"go.chromium.org/luci/server/module"
	"go.chromium.org/luci/server/pubsub"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/secrets"
	"go.chromium.org/luci/server/tq"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	"go.chromium.org/luci/swarming/server/botapi"
	"go.chromium.org/luci/swarming/server/botsrv"
	"go.chromium.org/luci/swarming/server/cfg"
	"go.chromium.org/luci/swarming/server/cipd"
	"go.chromium.org/luci/swarming/server/hmactoken"
	"go.chromium.org/luci/swarming/server/model"
	"go.chromium.org/luci/swarming/server/notifications"
	"go.chromium.org/luci/swarming/server/pyproxy"
	"go.chromium.org/luci/swarming/server/rbe"
	"go.chromium.org/luci/swarming/server/resultdb"
	"go.chromium.org/luci/swarming/server/rpcs"
	"go.chromium.org/luci/swarming/server/tasks"

	// Store auth sessions in the datastore.
	_ "go.chromium.org/luci/server/encryptedcookies/session/datastore"
)

// Members of this group will be able to hit WIP Go Swarming API.
//
// Eventually it will go away. This is temporary during the development to
// reduce the risk of leaking stuff by exposing unfinished or buggy API routes.
const devAPIAccessGroup = "swarming-go-api-allowlist"

func main() {
	modules := []module.Module{
		cfgmodule.NewModuleFromFlags(),
		cron.NewModuleFromFlags(),
		encryptedcookies.NewModuleFromFlags(),
		gaeemulation.NewModuleFromFlags(),
		pubsub.NewModuleFromFlags(),
		secrets.NewModuleFromFlags(),
		tq.NewModuleFromFlags(),
	}

	hmacSecret := flag.String(
		"shared-hmac-secret",
		"sm://shared-hmac",
		"A name of a secret with an HMAC key to use to produce various tokens.",
	)
	connPoolSize := flag.Int(
		"rbe-conn-pool",
		1,
		"RBE client connection pool size.")
	buildbucketServiceAccount := flag.String(
		"buildbucket-service-account",
		"",
		"Service account email of the Buildbucket service. Used to authorize calls to TaskBackend gRPC service.",
	)

	server.Main(nil, modules, func(srv *server.Server) error {
		tokenSecret, err := hmactoken.NewRotatingSecret(srv.Context, *hmacSecret)
		if err != nil {
			return err
		}

		// A cron job that fetches most recent configs from LUCI Config and puts
		// them into the datastore.
		var cipdClient cipd.Client
		cron.RegisterHandler("update-config", func(ctx context.Context) error {
			return cfg.UpdateConfigs(ctx, &cfg.EmbeddedBotSettings{
				ServerURL: fmt.Sprintf("https://%s.appspot.com", srv.Options.CloudProject),
			}, &cipdClient)
		})

		// A config loader for the current process which reads configs from the
		// datastore on launch (i.e. now) and then periodically refetches them in
		// background.
		cfg, err := cfg.NewProvider(srv.Context)
		if err != nil {
			return err
		}
		srv.RunInBackground("swarming.config", cfg.RefreshPeriodically)

		// A reverse proxy that sends a portion of requests to the Python server.
		// Proxied requests have "/python/..." prepended in the URL to make them
		// correctly pass dispatch.yaml rules and not get routed back to the Go
		// server.
		//
		// To simplify testing code locally without any migration configs, enable
		// the proxy only when running in prod.
		var proxy *pyproxy.Proxy
		if srv.Options.Prod {
			proxy = pyproxy.NewProxy(cfg, fmt.Sprintf("https://%s.appspot.com/python", srv.Options.CloudProject))
		}

		// Open *connPoolSize connections for SessionServer and one dedicated
		// connection for ReservationServer.
		rbeConns, err := rbe.Dial(srv.Context, *connPoolSize+1)
		if err != nil {
			return err
		}
		sessionsConns, reservationsConn := rbeConns[:*connPoolSize], rbeConns[*connPoolSize]

		// A server that can authenticate bot API calls and route them to Python.
		botSrv := botsrv.New(srv.Context, cfg, srv.Routes, proxy, knownBotProvider, srv.Options.CloudProject, tokenSecret)
		// A server that actually handles core Bot API calls.
		botAPI := botapi.NewBotAPIServer(cfg, tokenSecret, srv.Options.CloudProject, srv.Options.ImageVersion())

		// A minimal handler used by bots to test network connectivity. Install it
		// directly into the root router because we purposefully do not want to do
		// any authentication or any other non-trivial handling that botsrv does.
		srv.Routes.GET("/swarming/api/v1/bot/server_ping", nil, func(ctx *router.Context) {
			ctx.Writer.Header().Add("Content-Type", "text/plain; charset=utf-8")
			_, _ = ctx.Writer.Write([]byte("Server up"))
		})

		// Endpoints that return bot code. Used by bots and bootstrap scripts.
		botsrv.GET(botSrv, "/bot_code", botAPI.BotCode)
		botsrv.GET(botSrv, "/swarming/api/v1/bot/bot_code/:Version", botAPI.BotCode)

		// Bot API session management endpoints. They know how to deal with missing
		// session token and thus use NoSessionJSON. All other bot API endpoints
		// require a valid session and thus use JSON handler.
		botsrv.NoSessionJSON(botSrv, "/swarming/api/v1/bot/handshake", botAPI.Handshake)
		botsrv.NoSessionJSON(botSrv, "/swarming/api/v1/bot/poll", botAPI.Poll)

		// Bot API for claiming tasks and reporting events.
		botsrv.JSON(botSrv, "/swarming/api/v1/bot/claim", botAPI.Claim)
		botsrv.JSON(botSrv, "/swarming/api/v1/bot/event", botAPI.Event)

		// Bot API service account tokens minting endpoints.
		botsrv.JSON(botSrv, "/swarming/api/v1/bot/oauth_token", botAPI.OAuthToken)
		botsrv.JSON(botSrv, "/swarming/api/v1/bot/id_token", botAPI.IDToken)

		// Bot API task status update endpoints.
		botsrv.JSON(botSrv, "/swarming/api/v1/bot/task_update", botAPI.TaskUpdate)
		botsrv.JSON(botSrv, "/swarming/api/v1/bot/task_update/:TaskID", botAPI.TaskUpdate)
		botsrv.JSON(botSrv, "/swarming/api/v1/bot/task_error", botAPI.TaskError)
		botsrv.JSON(botSrv, "/swarming/api/v1/bot/task_error/:TaskID", botAPI.TaskError)

		// Bot API endpoints to control RBE session.
		rbeSessions := rbe.NewSessionServer(srv.Context, sessionsConns, tokenSecret, srv.Options.ImageVersion())
		botsrv.JSON(botSrv, "/swarming/api/v1/bot/rbe/session/create", rbeSessions.CreateBotSession)
		botsrv.JSON(botSrv, "/swarming/api/v1/bot/rbe/session/update", rbeSessions.UpdateBotSession)

		// Handlers for TQ tasks submitted by Python Swarming.
		internals, err := rbe.NewInternalsClient(srv.Context, srv.Options.CloudProject)
		if err != nil {
			return err
		}
		rbeReservations := rbe.NewReservationServer(srv.Context, reservationsConn, internals, srv.Options.CloudProject, srv.Options.ImageVersion())
		rbeReservations.RegisterTQTasks(&tq.Default)
		rbeReservations.RegisterPSHandlers(&pubsub.Default)

		// Handlers for TQ tasks for sending PubSub messages.
		pubSubNotifier, err := notifications.NewPubSubNotifier(srv.Context, srv.Options.CloudProject)
		if err != nil {
			return errors.Annotate(err, "failed to initialize the PubSubNotifier").Err()
		}
		pubSubNotifier.RegisterTQTasks(&tq.Default)
		srv.RegisterCleanup(func(context.Context) {
			pubSubNotifier.Stop()
		})

		// Handlers for TQ tasks involved in task lifecycle.
		taskLifeCycle := &tasks.LifecycleTasksViaTQ{
			Dispatcher: &tq.Default,
		}
		taskLifeCycle.RegisterTQTasks()

		// Old task deletion cron and TQ handlers.
		tasks.RegisterCleanupHandlers(&cron.Default, &tq.Default)

		// A temporary interceptor with very crude but solid ACL check for the
		// duration of the development. To avoid accidentally leaking stuff due to
		// bugs in the WIP code.
		srv.RegisterUnifiedServerInterceptors(rpcacl.Interceptor(rpcacl.Map{
			// Protect WIP or unimplemented Swarming APIs.
			"/swarming.v2.Bots/TerminateBot":      devAPIAccessGroup,
			"/buildbucket.v2.TaskBackend/RunTask": devAPIAccessGroup,

			// Fully implemented APIs allowed to receive external traffic.
			"/swarming.v2.Bots/CountBots":                 rpcacl.All,
			"/swarming.v2.Bots/DeleteBot":                 rpcacl.All,
			"/swarming.v2.Bots/GetBot":                    rpcacl.All,
			"/swarming.v2.Bots/GetBotDimensions":          rpcacl.All,
			"/swarming.v2.Bots/ListBotEvents":             rpcacl.All,
			"/swarming.v2.Bots/ListBots":                  rpcacl.All,
			"/swarming.v2.Bots/ListBotTasks":              rpcacl.All,
			"/swarming.v2.Tasks/CancelTask":               rpcacl.All,
			"/swarming.v2.Tasks/CancelTasks":              rpcacl.All,
			"/swarming.v2.Tasks/GetResult":                rpcacl.All,
			"/swarming.v2.Tasks/BatchGetResult":           rpcacl.All,
			"/swarming.v2.Tasks/GetRequest":               rpcacl.All,
			"/swarming.v2.Tasks/GetStdout":                rpcacl.All,
			"/swarming.v2.Tasks/ListTaskStates":           rpcacl.All,
			"/swarming.v2.Tasks/CountTasks":               rpcacl.All,
			"/swarming.v2.Tasks/ListTasks":                rpcacl.All,
			"/swarming.v2.Tasks/ListTaskRequests":         rpcacl.All,
			"/swarming.v2.Tasks/NewTask":                  rpcacl.All,
			"/swarming.v2.Swarming/GetDetails":            rpcacl.All,
			"/swarming.v2.Swarming/GetPermissions":        rpcacl.All,
			"/swarming.v2.Swarming/GetToken":              rpcacl.All,
			"/buildbucket.v2.TaskBackend/CancelTasks":     rpcacl.All,
			"/buildbucket.v2.TaskBackend/FetchTasks":      rpcacl.All,
			"/buildbucket.v2.TaskBackend/ValidateConfigs": rpcacl.All,

			// Leave other gRPC services open, they do they own authorization already.
			"/discovery.Discovery/*": rpcacl.All,
			"/config.Consumer/*":     rpcacl.All,
		}))

		// An interceptor that prepares per-RPC context for public gRPC servers.
		srv.RegisterUnifiedServerInterceptors(rpcs.ServerInterceptor(cfg, []string{
			"swarming.v2.Bots",
			"swarming.v2.Tasks",
			"swarming.v2.Swarming",
			"buildbucket.v2.TaskBackend",
		}))

		// Register gRPC server implementations.
		apipb.RegisterBotsServer(srv, &rpcs.BotsServer{BotQuerySplitMode: model.SplitOptimally})

		tasksServer := &rpcs.TasksServer{
			TaskQuerySplitMode:    model.SplitOptimally,
			TaskLifecycleTasks:    taskLifeCycle,
			ServerVersion:         srv.Options.ImageVersion(),
			SwarmingProject:       srv.Options.CloudProject,
			ResultDBClientFactory: resultdb.NewRecorderFactory(srv.Options.CloudProject),
		}
		apipb.RegisterTasksServer(srv, tasksServer)
		apipb.RegisterSwarmingServer(srv, &rpcs.SwarmingServer{
			ServerVersion: srv.Options.ImageVersion(),
		})
		bbpb.RegisterTaskBackendServer(srv, &rpcs.TaskBackend{
			BuildbucketTarget:       fmt.Sprintf("swarming://%s", srv.Options.CloudProject),
			BuildbucketAccount:      *buildbucketServiceAccount,
			DisableBuildbucketCheck: !srv.Options.Prod,
			TasksServer:             tasksServer,
		})

		srv.ConfigurePRPC(func(prpcSrv *prpc.Server) {
			// Allow cross-origin calls (e.g. for Milo to call ListBots).
			prpcSrv.AccessControl = prpc.AllowOriginAll
			// Enable redirection to Python if running in prod where it is set.
			if proxy != nil {
				rpcs.ConfigureMigration(prpcSrv, proxy)
			}
		})

		return nil
	})
}

// knownBotProvider returns info about a registered bot to use in Bot API.
//
// TODO: This will be very hot. May need to add a cache of some kind to avoid
// hitting the datastore all the time.
func knownBotProvider(ctx context.Context, botID string) (*botsrv.KnownBotInfo, error) {
	info := &model.BotInfo{Key: model.BotInfoKey(ctx, botID)}
	switch err := datastore.Get(ctx, info); {
	case err == nil:
		return &botsrv.KnownBotInfo{
			SessionID:     info.SessionID,
			Dimensions:    info.Dimensions,
			CurrentTaskID: info.TaskID,
		}, nil
	case errors.Is(err, datastore.ErrNoSuchEntity):
		return nil, nil
	default:
		return nil, err
	}
}
