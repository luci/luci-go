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

// Package module is a simple AppEngine LUCI service. It supplies basic LUCI
// service frontend and backend functionality.
//
// No RPC requests should target this service; instead, they are redirected to
// the appropriate service via "dispatch.yaml".
package module

import (
	"net/http"

	// Importing pprof implicitly installs "/debug/*" profiling handlers.
	_ "net/http/pprof"

	"go.chromium.org/luci/appengine/gaemiddleware/standard"

	"go.chromium.org/luci/server/router"
)

// Run installs and executes this site.
func init() {
	r := router.New()

	// Standard HTTP endpoints.
	standard.InstallHandlers(r)

	// Redirect "/" to "/app/".
	r.GET("/", router.MiddlewareChain{}, func(c *router.Context) {
		http.Redirect(c.Writer, c.Request, "/app/", http.StatusFound)
	})

	http.Handle("/", r)
}
