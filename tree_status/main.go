// Copyright 2023 The LUCI Authors.
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

	"go.chromium.org/luci/config/server/cfgmodule"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/cron"
	"go.chromium.org/luci/server/gaeemulation"
	"go.chromium.org/luci/server/gerritauth"
	"go.chromium.org/luci/server/module"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/secrets"
	spanmodule "go.chromium.org/luci/server/span"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/tree_status/internal/bqexporter"
	"go.chromium.org/luci/tree_status/internal/config"
	"go.chromium.org/luci/tree_status/internal/span"
	"go.chromium.org/luci/tree_status/internal/status"
	"go.chromium.org/luci/tree_status/internal/views"
	pb "go.chromium.org/luci/tree_status/proto/v1"
	"go.chromium.org/luci/tree_status/rpc"

	_ "go.chromium.org/luci/server/encryptedcookies/session/datastore"
	_ "go.chromium.org/luci/server/tq/txn/datastore"
)

func main() {
	// Additional modules that extend the server functionality.
	modules := []module.Module{
		cfgmodule.NewModuleFromFlags(),
		cron.NewModuleFromFlags(),
		gaeemulation.NewModuleFromFlags(),
		gerritauth.NewModuleFromFlags(),
		secrets.NewModuleFromFlags(),
		spanmodule.NewModuleFromFlags(nil),
		tq.NewModuleFromFlags(),
	}

	server.Main(nil, modules, func(srv *server.Server) error {
		srv.SetRPCAuthMethods([]auth.Method{
			// The default method used by majority of clients.
			&auth.GoogleOAuth2Method{
				Scopes: []string{"https://www.googleapis.com/auth/userinfo.email"},
			},
			// For authenticating calls from Gerrit plugins.
			&gerritauth.Method,
		})
		srv.ConfigurePRPC(func(s *prpc.Server) {
			s.AccessControl = func(context.Context, string) prpc.AccessControlDecision {
				return prpc.AccessControlDecision{
					AllowCrossOriginRequests: true,
					AllowCredentials:         true,
					AllowHeaders:             []string{gerritauth.Method.Header},
				}
			}
			// TODO(crbug/1082369): Remove this workaround once field masks can be decoded.
			s.EnableNonStandardFieldMasks = true
		})
		srv.RegisterUnaryServerInterceptors(span.SpannerDefaultsInterceptor())
		pb.RegisterTreeStatusServer(srv, rpc.NewTreeStatusServer())
		pb.RegisterTreesServer(srv, rpc.NewTreesServer())

		// Redirect the frontend to rpcexplorer.
		srv.Routes.GET("/", nil, func(ctx *router.Context) {
			http.Redirect(ctx.Writer, ctx.Request, "/rpcexplorer/", http.StatusFound)
		})

		cron.RegisterHandler("clear-status-users", status.ClearStatusUsers)
		cron.RegisterHandler("export-status", bqexporter.ExportStatus)
		cron.RegisterHandler("ensure-views", func(ctx context.Context) error {
			return views.CronHandler(ctx, srv.Options.CloudProject)
		})
		cron.RegisterHandler("update-config", config.Update)

		return nil
	})
}
