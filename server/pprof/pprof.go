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

// Package pprof is similar to net/http/pprof, except it supports auth.
//
// Use it instead of net/http/pprof in LUCI server environments.
//
// It uses temporary HMAC-based tokens (generated through admin portal) for
// authenticating requests. Requires a secret store to be installed in the
// base middleware.
package pprof

import (
	"fmt"
	"net/http"

	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/server/router"

	"go.chromium.org/luci/server/pprof/internal"
)

var pprofRoutes = map[string]http.HandlerFunc{
	"cmdline": internal.Cmdline,
	"profile": internal.Profile,
	"symbol":  internal.Symbol,
	"trace":   internal.Trace,
}

// InstallHandlers installs HTTP handlers for pprof routes.
func InstallHandlers(r *router.Router, base router.MiddlewareChain) {
	// Pprof native routing structure is not supported by julienschmidt/httprouter
	// since it mixes prefix matches and direct matches. So we'll have to do some
	// manual routing for paths under /debug/pprof :(
	r.GET("/debug/pprof/*path", base, func(c *router.Context) {
		// Validate the token generated through the portal page.
		tok := c.Request.FormValue("tok")
		if tok == "" {
			http.Error(c.Writer, "Missing 'tok' query parameter, see /admin/portal/pprof", http.StatusBadRequest)
			return
		}
		switch err := checkToken(c.Context, tok); {
		case transient.Tag.In(err):
			http.Error(c.Writer, fmt.Sprintf("Transient error: %s", err), http.StatusInternalServerError)
			return
		case err != nil:
			http.Error(c.Writer, fmt.Sprintf("Bad pprof token: %s", err), http.StatusBadRequest)
			return
		}

		// Manually route. See init() in go/src/net/http/pprof/pprof.go.
		h := pprofRoutes[c.Params.ByName("path")]
		if h == nil {
			h = internal.Index
		}
		h(c.Writer, c.Request)
	})
}
