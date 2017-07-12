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

package module

import (
	"net/http"

	// Importing pprof implicitly installs "/debug/*" profiling handlers.
	_ "net/http/pprof"

	"github.com/luci/luci-go/appengine/gaemiddleware"
	"github.com/luci/luci-go/grpc/prpc"
	registrationPb "github.com/luci/luci-go/logdog/api/endpoints/coordinator/registration/v1"
	servicesPb "github.com/luci/luci-go/logdog/api/endpoints/coordinator/services/v1"
	"github.com/luci/luci-go/logdog/appengine/coordinator"
	"github.com/luci/luci-go/logdog/appengine/coordinator/endpoints/registration"
	"github.com/luci/luci-go/logdog/appengine/coordinator/endpoints/services"
	"github.com/luci/luci-go/server/router"

	// Include mutations package so its Mutations will register with tumble via
	// init().
	_ "github.com/luci/luci-go/logdog/appengine/coordinator/mutations"
)

// Run installs and executes this site.
func init() {
	r := router.New()

	// Setup Cloud Endpoints.
	svr := prpc.Server{}
	servicesPb.RegisterServicesServer(&svr, services.New())
	registrationPb.RegisterRegistrationServer(&svr, registration.New())

	// Standard HTTP endpoints.
	base := gaemiddleware.BaseProd().Extend(coordinator.ProdCoordinatorService)
	svr.InstallHandlers(r, base)

	http.Handle("/", r)
}
