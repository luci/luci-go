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

package internal

import (
	"fmt"
	"net/http"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/openid"
	"go.chromium.org/luci/server/router"
)

// CloudAuthMiddleware returns a middleware chain that authorizes requests from
// Cloud Tasks, Cloud Scheduler and Cloud Pub/Sub.
//
// Checks OpenID Connect tokens have us in the audience, and the email in them
// is in `callers` list.
//
// If `header` is set, will also accept requests that have this header,
// regardless of its value. This is used to authorize GAE tasks and crons based
// on `X-AppEngine-*` headers.
func CloudAuthMiddleware(callers []string, header string, rejected func(*router.Context)) router.MiddlewareChain {
	oidc := auth.Authenticate(&openid.GoogleIDTokenAuthMethod{
		AudienceCheck: openid.AudienceMatchesHost,
	})

	return router.NewMiddlewareChain(oidc, func(c *router.Context, next router.Handler) {
		if header != "" && c.Request.Header.Get(header) != "" {
			next(c)
			return
		}

		if ident := auth.CurrentIdentity(c.Request.Context()); ident.Kind() != identity.Anonymous {
			if checkContainsIdent(callers, ident) {
				next(c)
			} else {
				if rejected != nil {
					rejected(c)
				}
				httpReply(c, 403,
					fmt.Sprintf("Caller %q is not authorized", ident), errors.Fmt("expecting any of %q", callers),
				)
			}
			return
		}

		var err error
		if header != "" {
			err = errors.Fmt("no OIDC token and no %s header", header)
		} else {
			err = errors.New("no OIDC token")
		}
		if rejected != nil {
			rejected(c)
		}
		httpReply(c, 403, "Authentication required", err)
	})
}

// checkContainsIdent is true if `ident` email matches some of `callers`.
func checkContainsIdent(callers []string, ident identity.Identity) bool {
	if ident.Kind() != identity.User {
		return false // we want service accounts
	}
	email := ident.Email()
	for _, c := range callers {
		if email == c {
			return true
		}
	}
	return false
}

// httpReply writes and logs HTTP response.
//
// `msg` is sent to the caller as is. `err` is logged, but not sent.
func httpReply(c *router.Context, code int, msg string, err error) {
	if err != nil {
		logging.Errorf(c.Request.Context(), "%s: %s", msg, err)
	}
	http.Error(c.Writer, msg, code)
}
