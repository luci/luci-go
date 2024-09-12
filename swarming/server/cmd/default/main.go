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
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/config/server/cfgmodule"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/auth/rpcacl"
	"go.chromium.org/luci/server/cron"
	"go.chromium.org/luci/server/encryptedcookies"
	"go.chromium.org/luci/server/gaeemulation"
	"go.chromium.org/luci/server/module"
	"go.chromium.org/luci/server/pubsub"
	"go.chromium.org/luci/server/secrets"
	"go.chromium.org/luci/server/tq"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	"go.chromium.org/luci/swarming/server/botsrv"
	"go.chromium.org/luci/swarming/server/cfg"
	"go.chromium.org/luci/swarming/server/cipd"
	"go.chromium.org/luci/swarming/server/hmactoken"
	"go.chromium.org/luci/swarming/server/model"
	"go.chromium.org/luci/swarming/server/notifications"
	"go.chromium.org/luci/swarming/server/pyproxy"
	"go.chromium.org/luci/swarming/server/rbe"
	"go.chromium.org/luci/swarming/server/rpcs"
	"go.chromium.org/luci/swarming/server/testing/integrationmocks"

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
	exposeIntegrationMocks := flag.Bool(
		"expose-integration-mocks",
		false,
		"If set, expose endpoints for running integration tests. Must be used locally only.",
	)
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

		// Endpoints hit by bots.
		rbeSessions := rbe.NewSessionServer(srv.Context, sessionsConns, tokenSecret)
		botSrv := botsrv.New(srv.Context, cfg, srv.Routes, proxy, srv.Options.CloudProject, tokenSecret)
		botsrv.InstallHandler(botSrv, "/swarming/api/v1/bot/rbe/ping", pingHandler)
		botsrv.InstallHandler(botSrv, "/swarming/api/v1/bot/rbe/session/create", rbeSessions.CreateBotSession)
		botsrv.InstallHandler(botSrv, "/swarming/api/v1/bot/rbe/session/update", rbeSessions.UpdateBotSession)

		// Handlers for TQ tasks submitted by Python Swarming.
		internals, err := rbe.NewInternalsClient(srv.Context, srv.Options.CloudProject)
		if err != nil {
			return err
		}
		rbeReservations := rbe.NewReservationServer(srv.Context, reservationsConn, internals, srv.Options.ImageVersion())
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

		// Helpers for running local integration tests. They fake some of Swarming
		// Python server behavior.
		if *exposeIntegrationMocks {
			if srv.Options.Prod {
				return errors.Reason("-expose-integration-mocks should not be used with -prod").Err()
			}
			integrationmocks.RegisterIntegrationMocksServer(srv, integrationmocks.New(
				srv.Context,
				tokenSecret,
			))
		}

		// A temporary interceptor with very crude but solid ACL check for the
		// duration of the development. To avoid accidentally leaking stuff due to
		// bugs in the WIP code.
		srv.RegisterUnifiedServerInterceptors(rpcacl.Interceptor(rpcacl.Map{
			// Protect WIP or unimplemented Swarming APIs.
			"/swarming.v2.Bots/DeleteBot":             devAPIAccessGroup,
			"/swarming.v2.Bots/TerminateBot":          devAPIAccessGroup,
			"/swarming.v2.Tasks/CancelTask":           devAPIAccessGroup,
			"/swarming.v2.Tasks/NewTask":              devAPIAccessGroup,
			"/swarming.v2.Tasks/CancelTasks":          devAPIAccessGroup,
			"/buildbucket.v2.TaskBackend/RunTask":     devAPIAccessGroup,
			"/buildbucket.v2.TaskBackend/CancelTasks": devAPIAccessGroup,

			// Fully implemented APIs allowed to receive external traffic.
			"/swarming.v2.Bots/CountBots":                 rpcacl.All,
			"/swarming.v2.Bots/GetBot":                    rpcacl.All,
			"/swarming.v2.Bots/GetBotDimensions":          rpcacl.All,
			"/swarming.v2.Bots/ListBotEvents":             rpcacl.All,
			"/swarming.v2.Bots/ListBots":                  rpcacl.All,
			"/swarming.v2.Bots/ListBotTasks":              rpcacl.All,
			"/swarming.v2.Tasks/GetResult":                rpcacl.All,
			"/swarming.v2.Tasks/BatchGetResult":           rpcacl.All,
			"/swarming.v2.Tasks/GetRequest":               rpcacl.All,
			"/swarming.v2.Tasks/GetStdout":                rpcacl.All,
			"/swarming.v2.Tasks/ListTaskStates":           rpcacl.All,
			"/swarming.v2.Tasks/CountTasks":               rpcacl.All,
			"/swarming.v2.Tasks/ListTasks":                rpcacl.All,
			"/swarming.v2.Tasks/ListTaskRequests":         rpcacl.All,
			"/swarming.v2.Swarming/GetDetails":            rpcacl.All,
			"/swarming.v2.Swarming/GetPermissions":        rpcacl.All,
			"/swarming.v2.Swarming/GetToken":              rpcacl.All,
			"/buildbucket.v2.TaskBackend/FetchTasks":      rpcacl.All,
			"/buildbucket.v2.TaskBackend/ValidateConfigs": rpcacl.All,

			// An API used in local integration tests.
			"/swarming.integrationmocks.IntegrationMocks/*": devAPIAccessGroup,

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
		apipb.RegisterTasksServer(srv, &rpcs.TasksServer{TaskQuerySplitMode: model.SplitOptimally})
		apipb.RegisterSwarmingServer(srv, &rpcs.SwarmingServer{
			ServerVersion: srv.Options.ImageVersion(),
		})
		bbpb.RegisterTaskBackendServer(srv, &rpcs.TaskBackend{
			BuildbucketTarget:       fmt.Sprintf("swarming://%s", srv.Options.CloudProject),
			BuildbucketAccount:      *buildbucketServiceAccount,
			DisableBuildbucketCheck: !srv.Options.Prod,
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

////////////////////////////////////////////////////////////////////////////////

// pingRequest is a JSON structure of the ping request payload.
type pingRequest struct {
	// Dimensions is dimensions reported by the bot.
	Dimensions map[string][]string `json:"dimensions"`
	// State is the state reported by the bot.
	State map[string]any `json:"state"`
	// Version is the bot version.
	Version string `json:"version"`
	// RBEState is RBE-related state reported by the bot.
	RBEState struct {
		// Instance if the full RBE instance name to use.
		Instance string `json:"instance"`
		// PollToken is base64-encoded HMAC-tagged internalspb.PollState.
		PollToken []byte `json:"poll_token"`
	} `json:"rbe_state"`
}

func (r *pingRequest) ExtractPollToken() []byte               { return r.RBEState.PollToken }
func (r *pingRequest) ExtractSessionToken() []byte            { return nil }
func (r *pingRequest) ExtractDimensions() map[string][]string { return r.Dimensions }

func (r *pingRequest) ExtractDebugRequest() any {
	return &pingRequest{
		Dimensions: r.Dimensions,
		State:      r.State,
		Version:    r.Version,
	}
}

func pingHandler(ctx context.Context, body *pingRequest, r *botsrv.Request) (botsrv.Response, error) {
	logging.Infof(ctx, "Dimensions: %v", r.Dimensions)
	logging.Infof(ctx, "PollState: %v", r.PollState)
	logging.Infof(ctx, "Bot version: %s", body.Version)
	if body.RBEState.Instance != r.PollState.RbeInstance {
		logging.Errorf(ctx, "RBE instance mismatch: reported %q, expecting %q",
			body.RBEState.Instance, r.PollState.RbeInstance,
		)
	}
	return nil, nil
}
