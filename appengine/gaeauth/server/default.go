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
	"context"

	"google.golang.org/appengine"

	"go.chromium.org/luci/appengine/gaeauth/server/internal/authdbimpl"
	"go.chromium.org/luci/appengine/gaemiddleware"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/deprecated"
	"go.chromium.org/luci/server/encryptedcookies"
	"go.chromium.org/luci/server/encryptedcookies/session/datastore"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/warmup"
)

var handlersInstalled = false

// CookieAuth is default cookie-based auth method to use on GAE.
//
// By default on the dev server it is based on dev server cookies (implemented
// by UsersAPIAuthMethod), in prod it is based on OpenID (implemented by
// *deprecated.CookieAuthMethod).
//
// Works only if appropriate handlers have been installed into the router. See
// InstallHandlers.
//
// It is allowed to assign to CookieAuth (e.g. to install a tweaked auth method)
// before InstallHandlers is called. In particular, use SwitchToEncryptedCookies
// to update to a better (but incompatible) method.
//
// Deprecated: this method depends on Users API not available outside of the GAE
// first-gen runtime and uses deprecated CookieAuthMethod. Use
// go.chromium.org/luci/server/encryptedcookies instead. To facilitate the
// migration you can switch to the encrypted cookies while still running on
// the GAE first-gen runtime by calling SwitchToEncryptedCookies() early during
// the server initialization.
var CookieAuth auth.Method

// DisableCookieAuth stops all cookie-based authentication.
//
// Must be called before InstallHandlers.
func DisableCookieAuth() {
	if handlersInstalled {
		panic("DisableCookieAuth must be called before InstallHandlers")
	}
	CookieAuth = nil
}

// SwitchToEncryptedCookies opts-in CookieAuth to use a better implementation.
//
// The "better implementation" is not backward compatible with the previous one,
// i.e. all existing user sessions are ignored. Calling this function is an
// acknowledgment that it is OK to relogin all users when making the switch.
//
// Must be called before InstallHandlers.
func SwitchToEncryptedCookies() {
	if handlersInstalled {
		panic("SwitchToEncryptedCookies must be called before InstallHandlers")
	}
	if method, ok := CookieAuth.(*deprecated.CookieAuthMethod); ok {
		// Upgrade! Reuse OpenID config in the settings.
		CookieAuth = &encryptedcookies.AuthMethod{
			OpenIDConfig: func(ctx context.Context) (*encryptedcookies.OpenIDConfig, error) {
				s, err := deprecated.FetchOpenIDSettings(ctx)
				if err != nil {
					return nil, err
				}
				return &encryptedcookies.OpenIDConfig{
					DiscoveryURL: s.DiscoveryURL,
					ClientID:     s.ClientID,
					ClientSecret: s.ClientSecret,
					RedirectURI:  s.RedirectURI,
				}, nil
			},
			AEADProvider:        gaemiddleware.AEADProvider,
			Sessions:            &datastore.Store{},
			Insecure:            method.Insecure,
			IncompatibleCookies: method.IncompatibleCookies,
		}
	}
}

// InstallHandlers installs HTTP handlers for various default routes related
// to authentication system.
//
// Must be installed in server HTTP router for authentication to work.
func InstallHandlers(r *router.Router, base router.MiddlewareChain) {
	if m, ok := CookieAuth.(auth.HasHandlers); ok {
		m.InstallHandlers(r, base)
	}
	auth.InstallHandlers(r, base)
	authdbimpl.InstallHandlers(r, base)
	handlersInstalled = true
}

func init() {
	warmup.Register("appengine/gaeauth/server", func(ctx context.Context) error {
		if m, ok := CookieAuth.(auth.Warmable); ok {
			return m.Warmup(ctx)
		}
		return nil
	})

	// Flip to true to enable OpenID login on devserver for debugging. Requires
	// a configuration (see /admin/portal/openid_auth page).
	const useOIDOnDevServer = false

	if appengine.IsDevAppServer() && !useOIDOnDevServer {
		CookieAuth = UsersAPIAuthMethod{}
	} else {
		CookieAuth = &deprecated.CookieAuthMethod{
			SessionStore:        &SessionStore{Prefix: "openid"},
			IncompatibleCookies: []string{"SACSID", "dev_appserver_login"},
			Insecure:            appengine.IsDevAppServer(), // for http:// cookie
		}
	}
}
