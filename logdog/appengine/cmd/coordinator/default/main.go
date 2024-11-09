// Copyright 2015 The LUCI Authors.
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

// Binary default is a simple AppEngine LUCI service. It supplies basic LUCI
// service frontend and backend functionality.
//
// No RPC requests should target this service; instead, they are redirected to
// the appropriate service via "dispatch.yaml".
package main

import (
	"net/http"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/config/server/cfgmodule"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/cron"
	"go.chromium.org/luci/server/gaeemulation"
	"go.chromium.org/luci/server/module"
	"go.chromium.org/luci/server/router"

	logsPb "go.chromium.org/luci/logdog/api/endpoints/coordinator/logs/v1"
	registrationPb "go.chromium.org/luci/logdog/api/endpoints/coordinator/registration/v1"
	servicesPb "go.chromium.org/luci/logdog/api/endpoints/coordinator/services/v1"
	"go.chromium.org/luci/logdog/server/config"
)

// main is the entrypoint for the `default` service.
func main() {
	modules := []module.Module{
		cfgmodule.NewModuleFromFlags(),
		cron.NewModuleFromFlags(),
		gaeemulation.NewModuleFromFlags(), // For datastore support.
	}

	server.Main(nil, modules, func(srv *server.Server) error {
		// Install the in-memory cache for configs in datastore, warm it up.
		srv.Context = config.WithStore(srv.Context, &config.Store{})
		if _, err := config.Config(srv.Context); err != nil {
			return errors.Annotate(err, "failed to fetch the initial service config").Err()
		}

		// Register dummy pRPC services (for RPC explorer discovery only).
		//
		// Note that most of these services have dedicated services to
		// handle them, and any RPCs sent to this module will automatically
		// be routed to them via "dispatch.yaml".
		logsPb.RegisterLogsServer(srv, dummyLogsService)
		registrationPb.RegisterRegistrationServer(srv, dummyRegistrationService)
		servicesPb.RegisterServicesServer(srv, dummyServicesService)

		// Register cron job handlers.
		cron.RegisterHandler("sync-configs", config.Sync)

		// Register UI routes.
		srv.Routes.GET("/", nil, func(c *router.Context) {
			http.Redirect(c.Writer, c.Request, "/rpcexplorer/", http.StatusFound)
		})
		srv.Routes.GET("/v/", nil, func(c *router.Context) {
			path := "/logs/" + c.Request.URL.Query().Get("s")
			http.Redirect(c.Writer, c.Request, path, http.StatusFound)
		})
		return nil
	})
}
