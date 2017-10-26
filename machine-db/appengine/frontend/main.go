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

// Package frontend contains the Machine Database AppEngine front end.
package frontend

import (
	"net/http"

	_ "github.com/go-sql-driver/mysql"

	"go.chromium.org/luci/appengine/gaemiddleware/standard"
	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/grpc/discovery"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/server/router"
)

func init() {
	mathrand.SeedRandomly()
	InstallSettings()

	r := router.New()
	standard.InstallHandlers(r)
	r.GET("/", standard.Base(), handler)

	api := prpc.Server{}
	discovery.Enable(&api)
	api.InstallHandlers(r, standard.Base())

	http.DefaultServeMux.Handle("/", r)
}

func handler(c *router.Context) {
	c.Writer.Header().Set("Content-Type", "text/plain")
	c.Writer.WriteHeader(200)
}
