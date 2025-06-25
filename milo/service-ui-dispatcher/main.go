// Copyright 2025 The LUCI Authors.
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

// Package main contains setup for push-on-green. See b/367097786.
package main

import (
	"errors"
	"net/http"
	"net/http/httputil"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/router"
)

func main() {
	server.Main(nil, nil, func(srv *server.Server) error {
		srv.Routes.GET("/ui/*path", nil, dispatch)
		srv.Routes.GET("/ui_version.js", nil, dispatch)
		srv.Routes.GET("/root_sw.js", nil, dispatch)
		srv.Routes.GET("/root_sw.js.map", nil, dispatch)

		return nil
	})
}

func dispatch(c *router.Context) {
	targetURL := *c.Request.URL
	targetURL.Scheme = "https"
	targetURL.Host = c.Request.Host

	targetURL.Path = "/new-ui"
	if userInitiatedRollback(c) {
		targetURL.Path = "/old-ui"
	}

	proxy := httputil.NewSingleHostReverseProxy(&targetURL)

	proxy.ModifyResponse = func(resp *http.Response) error {
		// Add our security headers to the response.
		resp.Header.Set("Content-Security-Policy", "frame-ancestors 'self'")
		resp.Header.Set("X-Frame-Options", "SAMEORIGIN")
		return nil
	}

	proxy.ServeHTTP(c.Writer, c.Request)
}

func userInitiatedRollback(c *router.Context) bool {
	cookie, err := c.Request.Cookie("USER_INITIATED_ROLLBACK")
	if err != nil {
		if !errors.Is(err, http.ErrNoCookie) {
			logging.WithError(err).Errorf(c.Request.Context(), "failed to read USER_INITIATED_ROLLBACK cookie")
		}
		return false
	}

	return cookie.Value == "true"
}
