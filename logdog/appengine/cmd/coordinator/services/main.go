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

	"go.chromium.org/luci/appengine/gaemiddleware/standard"
	"go.chromium.org/luci/grpc/grpcmon"
	"go.chromium.org/luci/grpc/prpc"

	registrationPb "go.chromium.org/luci/logdog/api/endpoints/coordinator/registration/v1"
	servicesPb "go.chromium.org/luci/logdog/api/endpoints/coordinator/services/v1"
	"go.chromium.org/luci/logdog/appengine/coordinator/endpoints"
	"go.chromium.org/luci/logdog/appengine/coordinator/endpoints/registration"
	"go.chromium.org/luci/logdog/appengine/coordinator/endpoints/services"
	"go.chromium.org/luci/server/router"
)

// Run installs and executes this site.
func init() {
	ps := endpoints.ProdService{}

	r := router.New()

	// Setup Cloud Endpoints.
	svr := prpc.Server{
		UnaryServerInterceptor: grpcmon.NewUnaryServerInterceptor(nil),
	}
	servicesPb.RegisterServicesServer(&svr, services.New())
	registrationPb.RegisterRegistrationServer(&svr, registration.New())

	// Standard HTTP endpoints.
	base := standard.Base().Extend(ps.Base)
	svr.InstallHandlers(r, base)
	standard.InstallHandlers(r)

	http.Handle("/", r)
}
