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

// Package main is the main entry point for the app.
package main

import (
	"context"
	"fmt"
	"io"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/config/server/cfgmodule"
	"go.chromium.org/luci/gae/filter/dscache"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/openid"
	"go.chromium.org/luci/server/bqlog"
	"go.chromium.org/luci/server/cron"
	"go.chromium.org/luci/server/gaeemulation"
	"go.chromium.org/luci/server/gerritauth"
	"go.chromium.org/luci/server/module"
	"go.chromium.org/luci/server/redisconn"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/secrets"
	"go.chromium.org/luci/server/tq"

	// Enable datastore transactional tasks support.
	_ "go.chromium.org/luci/server/tq/txn/datastore"

	"go.chromium.org/luci/buildbucket/appengine/internal/buildcron"
	"go.chromium.org/luci/buildbucket/appengine/internal/buildercron"
	"go.chromium.org/luci/buildbucket/appengine/internal/clients"
	"go.chromium.org/luci/buildbucket/appengine/internal/config"
	"go.chromium.org/luci/buildbucket/appengine/internal/metrics"
	"go.chromium.org/luci/buildbucket/appengine/internal/redirect"
	"go.chromium.org/luci/buildbucket/appengine/rpc"
	"go.chromium.org/luci/buildbucket/appengine/tasks"
	pb "go.chromium.org/luci/buildbucket/proto"
)

var cacheEnabled = stringset.NewFromSlice("Project", "BuildStatus")

func handlePubSubMessage(ctx *router.Context, identity identity.Identity, handler func(context.Context, io.Reader) error) {
	if got := auth.CurrentIdentity(ctx.Request.Context()); got != identity {
		logging.Errorf(ctx.Request.Context(), "Expecting ID token of %q, got %q", identity, got)
		ctx.Writer.WriteHeader(403)
	} else {
		switch err := handler(ctx.Request.Context(), ctx.Request.Body); {
		case err == nil:
			ctx.Writer.WriteHeader(200)
		case transient.Tag.In(err):
			logging.Warningf(ctx.Request.Context(), "Encounter transient error when processing pubsub msg: %s", err)
			ctx.Writer.WriteHeader(500) // PubSub will resend this msg.
		default:
			logging.Errorf(ctx.Request.Context(), "Encounter non-transient error when processing pubsub msg: %s", err)
			ctx.Writer.WriteHeader(202)
		}
	}
}

func main() {
	mods := []module.Module{
		bqlog.NewModuleFromFlags(),
		cfgmodule.NewModuleFromFlags(),
		cron.NewModuleFromFlags(),
		gaeemulation.NewModuleFromFlags(),
		gerritauth.NewModuleFromFlags(),
		tq.NewModuleFromFlags(),
		redisconn.NewModuleFromFlags(),
		secrets.NewModuleFromFlags(),
	}

	server.Main(nil, mods, func(srv *server.Server) error {
		o := srv.Options
		srv.Context = metrics.WithServiceInfo(srv.Context, o.TsMonServiceName, o.TsMonJobName, o.Hostname)

		// Install a global bigquery client.
		bqClient, err := clients.NewBqClient(srv.Context, o.CloudProject)
		if err != nil {
			return errors.Annotate(err, "failed to initiate the global Bigquery client").Err()
		}
		srv.Context = clients.WithBqClient(srv.Context, bqClient)

		// Enable dscache on Project entities only. Other datastore entities aren't
		// ready.
		srv.Context = dscache.AddShardFunctions(srv.Context, func(k *datastore.Key) (shards int, ok bool) {
			if cacheEnabled.Has(k.Kind()) {
				return 1, true
			}
			return 0, true
		})

		srv.SetRPCAuthMethods([]auth.Method{
			// OpenID Connect tokens are the prefered auth method.
			//
			// However, this method must be first because GoogleOAuth2Method doesn't
			// know how to ignore a JWT in the Authorization header.
			//
			// This method does not interfere with gerritauth, however, because
			// gerritauth looks at a separate header (usually "X-Gerrit-Auth").
			&openid.GoogleIDTokenAuthMethod{
				AudienceCheck: openid.AudienceMatchesHost,

				// This is true to also allow GoogleOAuth2Method - if GoogleOAuth2Method
				// is removed then this should be removed as well.
				SkipNonJWT: true,
			},

			// This method is ~deprecated, but is still used by the majority of
			// clients as of this CL.
			//
			// This is because the OAuth2/OpenID split complicates clients because
			// they need to know when to use OAuth2 vs OpenID. See additional details:
			// https://pkg.go.dev/go.chromium.org/luci/server/auth#GoogleOAuth2Method
			&auth.GoogleOAuth2Method{
				Scopes: []string{"https://www.googleapis.com/auth/userinfo.email"},
			},

			// For authenticating calls from Gerrit plugins.
			&gerritauth.Method,
		})

		srv.ConfigurePRPC(func(p *prpc.Server) {
			// Allow cross-origin calls, in particular calls using Gerrit auth
			// headers.
			p.AccessControl = func(context.Context, string) prpc.AccessControlDecision {
				return prpc.AccessControlDecision{
					AllowCrossOriginRequests: true,
					AllowCredentials:         true,
					AllowHeaders:             []string{gerritauth.Method.Header},
				}
			}
			// TODO(crbug/1082369): Remove this workaround once field masks can be
			// decoded.
			p.HackFixFieldMasksForJSON = true
		})

		pb.RegisterBuildsServer(srv, rpc.NewBuilds())
		pb.RegisterBuildersServer(srv, rpc.NewBuilders())

		cron.RegisterHandler("delete_builds", buildcron.DeleteOldBuilds)
		cron.RegisterHandler("expire_builds", buildcron.TimeoutExpiredBuilds)
		cron.RegisterHandler("sync_backend_tasks", buildcron.TriggerSyncBackendTasks)
		cron.RegisterHandler("update_config", config.UpdateSettingsCfg)
		cron.RegisterHandler("update_project_config", config.UpdateProjectCfg)
		cron.RegisterHandler("reset_expired_leases", buildcron.ResetExpiredLeases)
		cron.RegisterHandler("remove_inactive_builder_stats", buildercron.RemoveInactiveBuilderStats)
		redirect.InstallHandlers(srv.Routes)

		// PubSub push handler processing messages
		oidcMW := router.NewMiddlewareChain(
			auth.Authenticate(&openid.GoogleIDTokenAuthMethod{
				AudienceCheck: openid.AudienceMatchesHost,
			}),
		)
		// swarming-go-pubsub@ is a part of the PubSub Push subscription config.
		swarmingPusherID := identity.Identity(fmt.Sprintf("user:swarming-go-pubsub@%s.iam.gserviceaccount.com", srv.Options.CloudProject))
		srv.Routes.POST("/push-handlers/swarming-go/notify", oidcMW, func(ctx *router.Context) {
			handlePubSubMessage(ctx, swarmingPusherID, tasks.SubNotify)
		})

		// task-backend-update-task-push@ is a part of the PubSub Push subscription config.
		taskBackendPusherID := identity.Identity(fmt.Sprintf("user:task-backend-update-task-push@%s.iam.gserviceaccount.com", srv.Options.CloudProject))
		srv.Routes.POST("/internal/pubsub/backend/update-build-task", oidcMW, func(ctx *router.Context) {
			handlePubSubMessage(ctx, taskBackendPusherID, tasks.UpdateBuildTask)
		})

		return nil
	})
}
