// Copyright 2020 The LUCI Authors.
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

// Package main is the main entry point for the app.
package main

import (
	"net/http"

	"google.golang.org/appengine"

	"go.chromium.org/luci/appengine/gaemiddleware/standard"
	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/proto/access"
	"go.chromium.org/luci/grpc/discovery"
	"go.chromium.org/luci/grpc/grpcmon"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/server/router"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
)

func main() {
	mathrand.SeedRandomly()
	api := prpc.Server{UnaryServerInterceptor: grpcmon.UnaryServerInterceptor}
	access.RegisterAccessServer(&api, &access.UnimplementedAccessServer{})
	buildbucketpb.RegisterBuildsServer(&api, &buildbucketpb.UnimplementedBuildsServer{})
	discovery.Enable(&api)

	r := router.New()
	api.InstallHandlers(r, standard.Base())
	http.DefaultServeMux.Handle("/", r)
	standard.InstallHandlers(r)
	appengine.Main()
}
