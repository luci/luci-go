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

	"go.chromium.org/luci/auth/identity"
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
	"go.chromium.org/luci/buildbucket/appengine/rpc"
	"go.chromium.org/luci/buildbucket/appengine/tasks"
	pb "go.chromium.org/luci/buildbucket/proto"
)

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
			if k.Kind() == "Project" {
				return 1, true
			}
			return 0, true
		})

		srv.PRPC.Authenticator = &auth.Authenticator{
			Methods: []auth.Method{
				// The default method used by majority of clients.
				&auth.GoogleOAuth2Method{
					Scopes: []string{"https://www.googleapis.com/auth/userinfo.email"},
				},
				// For authenticating calls from Gerrit plugins.
				&gerritauth.Method,
			},
		}

		// Allow cross-origin calls, in particular calls using Gerrit auth headers.
		srv.PRPC.AccessControl = func(context.Context, string) prpc.AccessControlDecision {
			return prpc.AccessControlDecision{
				AllowCrossOriginRequests: true,
				AllowCredentials:         true,
				AllowHeaders:             []string{gerritauth.Method.Header},
			}
		}

		pb.RegisterBuildsServer(srv.PRPC, rpc.NewBuilds())
		pb.RegisterBuildersServer(srv.PRPC, rpc.NewBuilders())
		// TODO(crbug/1082369): Remove this workaround once field masks can be decoded.
		srv.PRPC.HackFixFieldMasksForJSON = true

		cron.RegisterHandler("delete_builds", buildcron.DeleteOldBuilds)
		cron.RegisterHandler("expire_builds", buildcron.TimeoutExpiredBuilds)
		cron.RegisterHandler("update_config", config.UpdateSettingsCfg)
		cron.RegisterHandler("update_project_config", config.UpdateProjectCfg)
		cron.RegisterHandler("reset_expired_leases", buildcron.ResetExpiredLeases)
		cron.RegisterHandler("remove_inactive_builder_stats", buildercron.RemoveInactiveBuilderStats)

		// PubSub push handler processing messages
		oidcMW := router.NewMiddlewareChain(
			auth.Authenticate(&openid.GoogleIDTokenAuthMethod{
				AudienceCheck: openid.AudienceMatchesHost,
			}),
		)
		// swarming-go-pubsub@ is a part of the PubSub Push subscription config.
		pusherID := identity.Identity(fmt.Sprintf("user:swarming-go-pubsub@%s.iam.gserviceaccount.com", srv.Options.CloudProject))
		srv.Routes.POST("/push-handlers/swarming-go/notify", oidcMW, func(ctx *router.Context) {
			if got := auth.CurrentIdentity(ctx.Context); got != pusherID {
				logging.Errorf(ctx.Context, "Expecting ID token of %q, got %q", pusherID, got)
				ctx.Writer.WriteHeader(403)
			} else {
				switch err := tasks.SubNotify(ctx.Context, ctx.Request.Body); {
				case err == nil:
					ctx.Writer.WriteHeader(200)
				case transient.Tag.In(err):
					logging.Warningf(ctx.Context, "Encounter transient error when processing pubsub msg: %s", err)
					ctx.Writer.WriteHeader(500) // PubSub will resend this msg.
				default:
					logging.Errorf(ctx.Context, "Encounter non-transient error when processing pubsub msg: %s", err)
					ctx.Writer.WriteHeader(202)
				}
			}
		})
		return nil
	})
}
