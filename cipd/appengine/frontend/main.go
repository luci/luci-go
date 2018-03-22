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

// Package frontend implements HTTP server that handles requests to 'default'
// module.
package frontend

import (
	"net/http"

	"go.chromium.org/luci/appengine/gaemiddleware/standard"
	"go.chromium.org/luci/grpc/discovery"
	"go.chromium.org/luci/grpc/grpcmon"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/server/router"

	api "go.chromium.org/luci/cipd/api/cipd/v1"
	"go.chromium.org/luci/cipd/appengine/impl"
)

func init() {
	r := router.New()

	// Install auth, config and tsmon handlers.
	standard.InstallHandlers(r)

	// Install all RPC servers. Catch panics, report metrics to tsmon (including
	// panics themselves, as Internal errors).
	srv := &prpc.Server{
		UnaryServerInterceptor: grpcmon.NewUnaryServerInterceptor(
			grpcutil.NewUnaryServerPanicCatcher(nil)),
	}
	api.RegisterStorageServer(srv, impl.PublicCAS)
	api.RegisterRepositoryServer(srv, impl.PublicRepo)
	discovery.Enable(srv)

	srv.InstallHandlers(r, standard.Base())
	http.DefaultServeMux.Handle("/", r)
}
