// Copyright 2017 The LUCI Authors.
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
	"fmt"
	"os"
	"time"

	"go.chromium.org/luci/auth/scopes"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/encryptedcookies"
	"go.chromium.org/luci/server/gaeemulation"
	"go.chromium.org/luci/server/module"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/secrets"

	logspb "go.chromium.org/luci/logdog/api/endpoints/coordinator/logs/v1"
	"go.chromium.org/luci/logdog/appengine/coordinator/flex"
	"go.chromium.org/luci/logdog/appengine/coordinator/flex/logs"
	"go.chromium.org/luci/logdog/common/storage/bigtable"
	"go.chromium.org/luci/logdog/server/config"

	// Store auth sessions in the datastore.
	_ "go.chromium.org/luci/server/encryptedcookies/session/datastore"
)

func main() {
	opts := server.Options{
		AuthServiceHost:       os.Getenv("AUTH_SERVICE_HOST"),
		TsMonAccount:          os.Getenv("TS_MON_ACCOUNT"),
		DefaultRequestTimeout: 12 * time.Hour,
	}
	modules := []module.Module{
		encryptedcookies.NewModule(&encryptedcookies.ModuleOptions{
			ClientID:     os.Getenv("OAUTH_CLIENT_ID"),
			ClientSecret: os.Getenv("OAUTH_CLIENT_SECRET"),
			RedirectURL:  os.Getenv("OAUTH_REDIRECT_URL"),
			TinkAEADKey:  os.Getenv("TINK_AEAD_KEY"),
		}),
		gaeemulation.NewModule(nil),
		secrets.NewModule(nil),
	}

	server.Main(&opts, modules, func(srv *server.Server) error {
		storageFlags := bigtable.Flags{
			Project:    os.Getenv("BIGTABLE_PROJECT"),
			Instance:   os.Getenv("BIGTABLE_INSTANCE"),
			LogTable:   os.Getenv("BIGTABLE_LOG_TABLE"),
			AppProfile: "log-reader",
		}
		if err := storageFlags.Validate(); err != nil {
			return err
		}

		// Install the in-memory cache for configs in datastore, warm it up.
		srv.Context = config.WithStore(srv.Context, &config.Store{})
		if _, err := config.Config(srv.Context); err != nil {
			return errors.Fmt("failed to fetch the initial service config: %w", err)
		}

		// Install core Logdog services.
		gsvc, err := flex.NewGlobalServices(srv.Context, &storageFlags)
		if err != nil {
			return err
		}
		srv.Context = flex.WithServices(srv.Context, gsvc)

		// Expose RPC APIs.
		srv.ConfigurePRPC(func(p *prpc.Server) { p.AccessControl = accessControl })
		logspb.RegisterLogsServer(srv, logs.New())

		// Setup HTTP endpoints. We support cookie auth for browsers and OAuth2 for
		// everything else.
		mw := router.MiddlewareChain{
			auth.Authenticate(
				srv.CookieAuth,
				&auth.GoogleOAuth2Method{
					Scopes: []string{scopes.Email},
				},
			),
		}
		srv.Routes.GET("/logs/*path", mw, logs.GetHandler)

		return nil
	})
}

func accessControl(ctx context.Context, origin string) prpc.AccessControlDecision {
	cfg, err := config.Config(ctx)
	if err != nil {
		panic(fmt.Sprintf("failed to get config for the access control check: %s", err))
	}

	ccfg := cfg.GetCoordinator()
	if ccfg == nil {
		return prpc.AccessControlDecision{}
	}

	for _, o := range ccfg.RpcAllowOrigins {
		if o == origin {
			return prpc.AccessControlDecision{
				AllowCrossOriginRequests: true,
				AllowCredentials:         true,
			}
		}
	}
	return prpc.AccessControlDecision{}
}
