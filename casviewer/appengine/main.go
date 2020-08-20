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

package main

import (
	gaeAuthServer "go.chromium.org/luci/appengine/gaeauth/server"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/router"
)

func main() {
	server.Main(nil, nil, func(srv *server.Server) error {
		mw := router.MiddlewareChain{}
		mw.Extend(
			auth.Authenticate(
				gaeAuthServer.CookieAuth,
				&gaeAuthServer.OAuth2Method{Scopes: []string{gaeAuthServer.EmailScope}},
			),
		)

		srv.Routes.GET("/", mw, func(c *router.Context) {
			logging.Debugf(c.Context, "Hello world")
			logging.Debugf(c.Context, "%v", auth.CurrentIdentity(c.Context))
			c.Writer.Write([]byte("Hello, world. This is CAS Viewer."))
		})

		return nil
	})
}
