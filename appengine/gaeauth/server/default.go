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

package server

import (
	"golang.org/x/net/context"
	"google.golang.org/appengine"

	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/openid"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/warmup"

	"go.chromium.org/luci/appengine/gaeauth/server/internal/authdbimpl"
)

// CookieAuth is default cookie-based auth method to use on GAE.
//
// On dev server it is based on dev server cookies, in prod it is based on
// OpenID. Works only if appropriate handlers have been installed into
// the router. See InstallHandlers.
var CookieAuth auth.Method

// InstallHandlers installs HTTP handlers for various default routes related
// to authentication system.
//
// Must be installed in server HTTP router for authentication to work.
func InstallHandlers(r *router.Router, base router.MiddlewareChain) {
	m := CookieAuth.(cookieAuthMethod)
	if oid, ok := m.Method.(*openid.AuthMethod); ok {
		oid.InstallHandlers(r, base)
	}
	auth.InstallHandlers(r, base)
	authdbimpl.InstallHandlers(r, base)
}

func init() {
	warmup.Register("appengine/gaeauth/server", func(c context.Context) error {
		m := CookieAuth.(cookieAuthMethod)
		if oid, ok := m.Method.(*openid.AuthMethod); ok {
			return oid.Warmup(c)
		}
		return nil
	})
}

///

// cookieAuthMethod implements union of openid.AuthMethod and UsersAPIAuthMethod
// methods, routing calls appropriately.
type cookieAuthMethod struct {
	auth.Method
}

func (m cookieAuthMethod) LoginURL(c context.Context, dest string) (string, error) {
	return m.Method.(auth.UsersAPI).LoginURL(c, dest)
}

func (m cookieAuthMethod) LogoutURL(c context.Context, dest string) (string, error) {
	return m.Method.(auth.UsersAPI).LogoutURL(c, dest)
}

func init() {
	// Flip to true to enable OpenID login on devserver for debugging. Requires
	// a configuration (see /admin/portal/openid_auth page).
	const useOIDOnDevServer = false

	if appengine.IsDevAppServer() && !useOIDOnDevServer {
		CookieAuth = cookieAuthMethod{UsersAPIAuthMethod{}}
	} else {
		CookieAuth = cookieAuthMethod{
			&openid.AuthMethod{
				SessionStore:        &SessionStore{Prefix: "openid"},
				IncompatibleCookies: []string{"SACSID", "dev_appserver_login"},
				Insecure:            appengine.IsDevAppServer(), // for http:// cookie
			},
		}
	}
}
