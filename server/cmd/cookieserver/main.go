// Copyright 2023 The LUCI Authors.
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

// Executable cookieserver is a LUCI Server that hosts encryptedcookies module
// using Cloud Datastore as the session storage backend.
//
// Its purpose is to allow using encrypted cookies as an authentication method
// in Web UIs that
//   - Are SPAs or similar (i.e. make all backend calls from JavaScript).
//   - Use `Authorization` header with OAuth or ID tokens when making calls.
//   - Their main backend is not in Go or can't host encryptedcookies natively.
//
// # Setup
//
// To use the cookieserver one will need to run it as a "microservice" and route
// HTTP requests targeting `https://<domain>/auth/openid/*` to it, where
// `<domain>` is the same domain that serves the JavaScript code that needs
// authentication tokens.
//
// The following command line flags are required when launching the server in
// production:
//
//	cookieserver \
//		-encrypted-cookies-client-id <OAuth Client ID> \
//		-encrypted-cookies-client-secret sm://<GSM secrets with OAuth secret> \
//		-encrypted-cookies-redirect-url https://<domain>/auth/openid/callback \
//		-encrypted-cookies-tink-aead-key sm://<GSM secret with encryption key>
//
// See https://go.chromium.org/luci/server/encryptedcookies for instructions
// how to setup necessary secrets and the OAuth client.
//
// # Usage
//
// To identify a signed in user and get their fresh OAuth or OpenID tokens on
// the frontend make a GET fetch(...) request to `/auth/openid/state` endpoint,
// sending cookies with it, e.g. by setting `credentials: "same-origin"`. This
// endpoint checks the validity of the session cookie, fetches the encrypted
// OAuth refresh token from the session store and uses it to generate fresh
// OAuth and ID tokens. This process also verifies user's Google account still
// exists and the user didn't revoke access to it.
//
// If the endpoint replies with
//
//	{
//	 "identity": "anonymous:anonymous"
//	}
//
// then there's no signed in user. To start the sign in flow redirect the
// browser to e.g.
//
//	/auth/openid/login?r=/come/back
//
// This will launch a series of redirects which ends with setting the session
// cookie and redirecting to `/come/back`.
//
// If the session cookie is already set and valid, `/auth/openid/state` replies
// with e.g.
//
//	{
//	  "identity": "user:someone@example.com",
//	  "email": "someone@example.com",
//	  "picture": "https://<profile-picture-url>",
//	  "accessToken": "ya29.a....",
//	  "accessTokenExpiry": 1689903385,
//	  "accessTokenExpiresIn": 3213,
//	  "idToken": "eyJhbGc....",
//	  "idTokenExpiry": 1689903385,
//	  "idTokenExpiresIn": 3213
//	}
//
// Fields `email` and `picture` can be used in the UI to show info about the
// user. Fields `accessToken` and `idToken` can be used to make authenticated
// fetch calls to the backend.
//
// For the best performance cache the access and ID tokens based on `ExpiresIn`,
// in the JavaScript state. In the example above these tokens are valid for
// 3213 seconds.
//
// If something is misconfigured or there are internal transient errors, the
// endpoint may return an HTTP 500 status with plain text error message.
//
// To log the user out and unset the cookie, redirect the browser to e.g.
//
//	/auth/openid/logout?r=/come/back
//
// This will eventually redirect back `/come/back` with the cookie unset.
//
// # Running locally
//
// When running the server locally (without `-prod` flag) and when not passing
// any `-encrypted-cookies-...` flags, the server fakes out OpenID flows and
// the session store. It uses developer's own credentials (ones that are used to
// run the server itself) to serve `/auth/openid/state`. This means there's no
// configuration required whatsoever to run this code locally, but the backend
// will see tokens with the default LUCI OAuth client ID as the audience (see
// https://go.chromium.org/luci/hardcoded/chromeinfra).
package main

import (
	"flag"

	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/encryptedcookies"
	"go.chromium.org/luci/server/gaeemulation"
	"go.chromium.org/luci/server/module"
	"go.chromium.org/luci/server/secrets"

	// Use datastore as a backend for auth sessions.
	_ "go.chromium.org/luci/server/encryptedcookies/session/datastore"
)

func main() {
	// Prefill some flags with better defaults for this case.
	cookiesOpts := &encryptedcookies.ModuleOptions{
		// Make "/auth/openid/state" available.
		ExposeStateEndpoint: true,
		// Limit the cookie to "/auth/openid/" HTTP path, make it `SameSite strict`.
		LimitCookieExposure: true,
	}
	cookiesOpts.Register(flag.CommandLine)

	// `encryptedcookies` module and its dependencies.
	modules := []module.Module{
		encryptedcookies.NewModule(cookiesOpts),
		gaeemulation.NewModuleFromFlags(),
		secrets.NewModuleFromFlags(),
	}

	server.Main(nil, modules, func(srv *server.Server) error {
		// All useful routes are already installed by `encryptedcookies` module.
		return nil
	})
}
