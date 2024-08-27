// Copyright 2024 The LUCI Authors.
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

	luciserver "go.chromium.org/luci/server"
	"go.chromium.org/luci/server/pubsub"
	"go.chromium.org/luci/server/router"

	"go.chromium.org/luci/milo/internal/buildsource/buildbucket"
	"go.chromium.org/luci/milo/server"

	_ "go.chromium.org/luci/server/encryptedcookies/session/datastore"
)

// main implements the entrypoint for the api service.
// This is the only service accessible from milo.api.luci.app.
func main() {
	server.Main(func(srv *luciserver.Server) error {
		service := server.CreateInternalService()
		server.RegisterPRPCHandlers(srv, service)
		server.RegisterCrons(srv, service)
		pubsub.RegisterJSONPBHandler("buildbucket", buildbucket.PubSubHandler)

		// Redirect the frontend to RPC explorer.
		srv.Routes.GET("/", nil, func(ctx *router.Context) {
			http.Redirect(ctx.Writer, ctx.Request, "/rpcexplorer/", http.StatusFound)
		})
		return nil
	})
}
