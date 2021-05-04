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
	"flag"
	"fmt"

	"go.chromium.org/luci/common/errors"
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
	modules := []module.Module{
		encryptedcookies.NewModuleFromFlags(),
		gaeemulation.NewModuleFromFlags(),
		secrets.NewModuleFromFlags(),
	}

	storageFlags := bigtable.Flags{}
	storageFlags.Register(flag.CommandLine)

	server.Main(nil, modules, func(srv *server.Server) error {
		if err := storageFlags.Validate(); err != nil {
			return err
		}

		// Install the in-memory cache for configs in datastore, warm it up.
		srv.Context = config.WithStore(srv.Context, &config.Store{})
		if _, err := config.Config(srv.Context); err != nil {
			return errors.Annotate(err, "failed to fetch the initial service config").Err()
		}

		// Install core Logdog services.
		gsvc, err := flex.NewGlobalServices(srv.Context, &storageFlags)
		if err != nil {
			return err
		}
		srv.Context = flex.WithServices(srv.Context, gsvc)

		// Expose pRPC APIs.
		srv.PRPC.AccessControl = accessControl
		logspb.RegisterLogsServer(srv.PRPC, logs.New())

		// Setup HTTP endpoints. We support cookie auth for browsers and OAuth2 for
		// everything else.
		mw := router.MiddlewareChain{
			auth.Authenticate(
				srv.CookieAuth,
				&auth.GoogleOAuth2Method{
					Scopes: []string{"https://www.googleapis.com/auth/userinfo.email"},
				},
			),
		}
		srv.Routes.GET("/logs/*path", mw, logs.GetHandler)

		return nil
	})
}

func accessControl(ctx context.Context, origin string) bool {
	cfg, err := config.Config(ctx)
	if err != nil {
		panic(fmt.Sprintf("failed to get config for the access control check: %s", err))
	}

	ccfg := cfg.GetCoordinator()
	if ccfg == nil {
		return false
	}

	for _, o := range ccfg.RpcAllowOrigins {
		if o == origin {
			return true
		}
	}
	return false
}
