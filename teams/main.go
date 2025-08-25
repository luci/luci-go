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
	"net/http"

	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/cron"
	_ "go.chromium.org/luci/server/encryptedcookies/session/datastore"
	"go.chromium.org/luci/server/gaeemulation"
	"go.chromium.org/luci/server/module"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/secrets"
	spanmodule "go.chromium.org/luci/server/span"
	"go.chromium.org/luci/server/tq"
	_ "go.chromium.org/luci/server/tq/txn/datastore"
	"go.chromium.org/luci/teams/internal/span"
	pb "go.chromium.org/luci/teams/proto/v1"
	"go.chromium.org/luci/teams/rpc"
)

func main() {
	// Additional modules that extend the server functionality.
	modules := []module.Module{
		cron.NewModuleFromFlags(),
		gaeemulation.NewModuleFromFlags(),
		secrets.NewModuleFromFlags(),
		tq.NewModuleFromFlags(),
		spanmodule.NewModuleFromFlags(nil),
	}

	server.Main(nil, modules, func(srv *server.Server) error {
		srv.ConfigurePRPC(func(s *prpc.Server) {
			s.AccessControl = prpc.AllowOriginAll
		})
		srv.RegisterUnaryServerInterceptors(span.SpannerDefaultsInterceptor())

		pb.RegisterTeamsServer(srv, rpc.NewTeamsServer())

		// Redirect the frontend to rpcexplorer.
		srv.Routes.GET("/", nil, func(ctx *router.Context) {
			http.Redirect(ctx.Writer, ctx.Request, "/rpcexplorer/", http.StatusFound)
		})

		// TODO: Register any cron jobs here:
		// cron.RegisterHandler("update-config", ...)
		return nil
	})
}
