// Copyright 2019 The LUCI Authors.
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

// Binary frontend is the main entry point for the CQ app.
package main

import (
	"net/http"

	"google.golang.org/appengine"

	"go.chromium.org/luci/appengine/gaemiddleware/standard"
	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/grpc/discovery"
	"go.chromium.org/luci/grpc/grpcmon"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/server/router"

	// Ensure registration of validation rules.
	_ "go.chromium.org/luci/cq/appengine/config"
)

func main() {
	mathrand.SeedRandomly()
	api := prpc.Server{UnaryServerInterceptor: grpcmon.NewUnaryServerInterceptor(nil)}
	discovery.Enable(&api)

	r := router.New()
	mw := standard.Base()
	api.InstallHandlers(r, mw)
	standard.InstallHandlers(r)
	http.DefaultServeMux.Handle("/", r)
	appengine.Main()
}
