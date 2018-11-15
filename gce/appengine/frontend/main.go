// Copyright 2018 The LUCI Authors.
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

// Package frontend is the main entry point for the app.
package frontend

import (
	"net/http"

	"go.chromium.org/luci/appengine/gaemiddleware/standard"
	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/grpc/discovery"
	"go.chromium.org/luci/grpc/grpcmon"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/server/router"

	"go.chromium.org/luci/gce/api/config/v1"
	"go.chromium.org/luci/gce/appengine/backend"
	"go.chromium.org/luci/gce/appengine/rpc"
)

func init() {
	mathrand.SeedRandomly()
	api := prpc.Server{UnaryServerInterceptor: grpcmon.NewUnaryServerInterceptor(nil)}
	srv := rpc.New()
	config.RegisterConfigServer(&api, srv)
	discovery.Enable(&api)

	r := router.New()
	mw := standard.Base()
	api.InstallHandlers(r, mw)
	backend.InstallHandlers(r, mw)
	http.DefaultServeMux.Handle("/", r)
	standard.InstallHandlers(r)
}
