// Copyright 2021 The LUCI Authors.
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

// Binary frontend implements HTTP server that handles requests to 'default'
// module.
package main

import (
	"net/http"

	"go.chromium.org/luci/auth/scopes"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/encryptedcookies"
	"go.chromium.org/luci/server/module"
	"go.chromium.org/luci/server/router"

	admingrpcpb "go.chromium.org/luci/cipd/api/admin/v1/grpcpb"
	casgrpcpb "go.chromium.org/luci/cipd/api/cipd/v1/caspb/grpcpb"
	repogrpcpb "go.chromium.org/luci/cipd/api/cipd/v1/repopb/grpcpb"
	"go.chromium.org/luci/cipd/appengine/impl"
	"go.chromium.org/luci/cipd/appengine/impl/accesslog"
	"go.chromium.org/luci/cipd/appengine/ui"

	// Using datastore for user sessions.
	_ "go.chromium.org/luci/server/encryptedcookies/session/datastore"
	// Using transactional datastore TQ tasks.
	_ "go.chromium.org/luci/server/tq/txn/datastore"
	// Initialize descriptors for the RPC Explorer.
	_ "go.chromium.org/luci/cipd/api/cipd/v1/discovery"
)

func main() {
	// Extra modules used by the frontend server only.
	extra := []module.Module{
		encryptedcookies.NewModuleFromFlags(),
	}

	impl.Main(extra, func(srv *server.Server, svc *impl.Services) error {
		// Register non-pRPC routes, such as the client bootstrap handler and routes
		// to support minimal subset of legacy API required to let old CIPD clients
		// fetch packages and self-update.
		svc.PublicRepo.InstallHandlers(srv.Routes, router.NewMiddlewareChain(
			auth.Authenticate(&auth.GoogleOAuth2Method{
				Scopes: []string{scopes.Email},
			}),
		))

		// UI pages. When running locally, serve static files ourself as well.
		ui.InstallHandlers(srv, svc, "templates")
		if !srv.Options.Prod {
			srv.Routes.Static("/static", nil, http.Dir("./static"))
		}

		// All RPC services.
		admingrpcpb.RegisterAdminServer(srv, svc.AdminAPI)
		casgrpcpb.RegisterStorageServer(srv, svc.PublicCAS)
		repogrpcpb.RegisterRepositoryServer(srv, svc.PublicRepo)

		// Log RPC requests to BigQuery.
		srv.RegisterUnaryServerInterceptors(accesslog.NewUnaryServerInterceptor(&srv.Options))

		return nil
	})
}
