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
	"net/http"

	// Importing pprof implicitly installs "/debug/*" profiling handlers.
	_ "net/http/pprof"

	logsPb "go.chromium.org/luci/logdog/api/endpoints/coordinator/logs/v1"
	"go.chromium.org/luci/logdog/appengine/coordinator/config"
	"go.chromium.org/luci/logdog/appengine/coordinator/flex"
	"go.chromium.org/luci/logdog/appengine/coordinator/flex/logs"

	flexMW "go.chromium.org/luci/appengine/gaemiddleware/flex"
	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/grpc/discovery"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/server/router"

	"golang.org/x/net/context"
)

// Run installs and executes this site.
func main() {
	mathrand.SeedRandomly()

	r := router.New()

	// Setup Cloud Endpoints.
	svr := &prpc.Server{
		AccessControl: accessControl,
	}
	logsPb.RegisterLogsServer(svr, logs.New())
	discovery.Enable(svr)

	// Setup process global Context.
	c := context.Background()
	c = gologger.StdConfig.Use(c) // Log to STDERR.
	gsvc, err := flex.NewGlobalServices(flexMW.WithGlobal(c))
	if err != nil {
		logging.WithError(err).Errorf(c, "Failed to setup Flex services.")
		panic(err)
	}
	defer gsvc.Close()

	// TODO(dnj): We currently instantiate global instances of several services,
	// with the current service configuration paramers (e.g., name of BigTable
	// table, etc.).
	//
	// We should monitor config and kill a Flex instance if it's been observed to
	// change. It would respawn, reload the new config, and then be good to go
	// until the next change.
	//
	// As things stand, this configuration basically never changes, so this is
	// not terribly important. However, it's worth noting that we should do this,
	// and that here is probably the right place to kick off such a goroutine.

	// Standard HTTP endpoints using flex LogDog services singleton.
	mw := flexMW.ReadOnlyFlex
	mw.InstallHandlers(r)

	base := mw.Base().Extend(gsvc.Base)
	svr.InstallHandlers(r, base)

	// Redirect "/" to "/app/".
	r.GET("/", router.MiddlewareChain{}, func(c *router.Context) {
		http.Redirect(c.Writer, c.Request, "/app/", http.StatusFound)
	})

	http.Handle("/", r)
	logging.Infof(c, "Listening on port 8080...")
	if err := http.ListenAndServe(":8080", r); err != nil {
		logging.WithError(err).Errorf(c, "Failed HTTP listen.")
		panic(err)
	}
}

func accessControl(c context.Context, origin string) bool {
	cfg, err := config.Load(c)
	if err != nil {
		logging.WithError(err).Errorf(c, "Failed to get config for access control check.")
		return false
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
