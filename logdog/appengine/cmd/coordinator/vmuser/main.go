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

package main

import (
	"net/http"

	// Importing pprof implicitly installs "/debug/*" profiling handlers.
	_ "net/http/pprof"

	"github.com/luci/luci-go/appengine/gaemiddleware"
	"github.com/luci/luci-go/common/data/rand/mathrand"
	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/grpc/discovery"
	"github.com/luci/luci-go/grpc/prpc"
	adminPb "github.com/luci/luci-go/logdog/api/endpoints/coordinator/admin/v1"
	logsPb "github.com/luci/luci-go/logdog/api/endpoints/coordinator/logs/v1"
	registrationPb "github.com/luci/luci-go/logdog/api/endpoints/coordinator/registration/v1"
	servicesPb "github.com/luci/luci-go/logdog/api/endpoints/coordinator/services/v1"
	"github.com/luci/luci-go/logdog/appengine/coordinator"
	"github.com/luci/luci-go/logdog/appengine/coordinator/config"
	"github.com/luci/luci-go/logdog/appengine/coordinator/endpoints/admin"
	"github.com/luci/luci-go/logdog/appengine/coordinator/endpoints/logs"
	"github.com/luci/luci-go/logdog/appengine/coordinator/endpoints/registration"
	"github.com/luci/luci-go/logdog/appengine/coordinator/endpoints/services"
	"github.com/luci/luci-go/server/router"

	"golang.org/x/net/context"
	"google.golang.org/appengine"

	// Include mutations package so its Mutations will register with tumble via
	// init().
	_ "github.com/luci/luci-go/logdog/appengine/coordinator/mutations"
)

// Run installs and executes this site.
func main() {
	mathrand.SeedRandomly()

	r := router.New()

	// Setup Cloud Endpoints.
	svr := prpc.Server{
		AccessControl: accessControl,
	}
	adminPb.RegisterAdminServer(&svr, admin.New())
	servicesPb.RegisterServicesServer(&svr, services.New())
	registrationPb.RegisterRegistrationServer(&svr, registration.New())
	logsPb.RegisterLogsServer(&svr, logs.New())
	discovery.Enable(&svr)

	// Standard HTTP endpoints.
	base := gaemiddleware.BaseProd().Extend(coordinator.ProdCoordinatorService)
	gaemiddleware.InstallHandlersWithMiddleware(r, base)
	svr.InstallHandlers(r, base)

	// Redirect "/" to "/app/".
	r.GET("/", router.MiddlewareChain{}, func(c *router.Context) {
		http.Redirect(c.Writer, c.Request, "/app/", http.StatusFound)
	})

	http.Handle("/", r)
	appengine.Main()
}

func accessControl(c context.Context, origin string) bool {
	cfg, err := config.Load(c)
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to get config for access control check.")
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
