// Copyright 2021 The LUCI Authors.
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

// Package main is the main point of entry for the frontend module.
//
// It exposes the main API and Web UI of the service.
package main

import (
	"context"
	"net/http"
	"strings"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/openid"
	"go.chromium.org/luci/server/auth/xsrf"
	"go.chromium.org/luci/server/encryptedcookies"
	"go.chromium.org/luci/server/module"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/templates"

	"go.chromium.org/luci/auth_service/api/rpcpb"
	"go.chromium.org/luci/auth_service/impl"
	"go.chromium.org/luci/auth_service/impl/servers/accounts"
	"go.chromium.org/luci/auth_service/impl/servers/groups"

	// Store auth sessions in the datastore.
	_ "go.chromium.org/luci/server/encryptedcookies/session/datastore"
)

func main() {
	modules := []module.Module{
		encryptedcookies.NewModuleFromFlags(), // for authenticating web UI calls
	}

	impl.Main(modules, func(srv *server.Server) error {
		// On GAE '/static' is served by GAE itself (see app.yaml). When running
		// locally in dev mode we need to do it ourselves.
		if !srv.Options.Prod {
			srv.Routes.Static("/static", nil, http.Dir("./static"))
		}

		// https://crbug.com/1242998
		datastore.EnableSafeGet()

		// Cookie auth and pRPC have some rough edges, see prpcCookieAuth comment.
		prpcAuth := &prpcCookieAuth{cookieAuth: srv.CookieAuth}

		// Authentication methods for pRPC APIs.
		srv.PRPC.Authenticator = &auth.Authenticator{
			Methods: []auth.Method{
				// The preferred authentication method.
				&openid.GoogleIDTokenAuthMethod{
					AudienceCheck: openid.AudienceMatchesHost,
					SkipNonJWT:    true, // pass OAuth2 access tokens through
				},
				// Backward compatibility for the RPC Explorer and old clients.
				&auth.GoogleOAuth2Method{
					Scopes: []string{"https://www.googleapis.com/auth/userinfo.email"},
				},
				// Cookie auth is used by the Web UI. When this method is used, we also
				// check the XSRF token to be really sure it is the Web UI that called
				// the method. See xsrf.Interceptor below.
				prpcAuth,
			},
		}

		// Interceptors applying to all pRPC APIs.
		srv.PRPC.UnaryServerInterceptor = grpcutil.ChainUnaryServerInterceptors(
			xsrf.Interceptor(prpcAuth),
			impl.AuthorizeRPCAccess,
		)

		// Register all pRPC servers.
		rpcpb.RegisterAccountsServer(srv.PRPC, &accounts.Server{})
		rpcpb.RegisterGroupsServer(srv.PRPC, &groups.Server{})

		// The middleware chain applied to all plain HTTP routes.
		mw := router.MiddlewareChain{
			templates.WithTemplates(prepareTemplates(&srv.Options)),
			auth.Authenticate(srv.CookieAuth),
			requireLogin,
			authorizeUIAccess,
		}

		srv.Routes.GET("/", mw, func(ctx *router.Context) {
			templates.MustRender(ctx.Context, ctx.Writer, "pages/index.html", nil)
		})
		return nil
	})
}

// prpcCookieAuth authenticates pRPC calls using the given method, but only
// if they have `X-Xsrf-Token` header. Otherwise it ignores cookies completely.
//
// This is primarily needed to allow the RPC Explorer to keep sending cookies
// without XSRF tokens, since it is unaware of XSRF tokens (or cookies for that
// matter) and just uses XMLHttpRequest, which **always** sends cookies with
// same origin requests. There's no way to disable it. Such requests are
// rejected by xsrf.Interceptor, because they don't have XSRF tokens.
//
// The best solution would be to change the RPC Explorer to use `fetch` API with
// 'credentials: omit' policy. But this is non-trivial. So instead we just
// ignore any cookies sent by the RPC Explorer and let it authenticate calls
// using OAuth2 access tokens (as it was designed to do).
type prpcCookieAuth struct {
	cookieAuth auth.Method
}

// Authenticate is a part of auth.Method interface.
func (m *prpcCookieAuth) Authenticate(ctx context.Context, req *http.Request) (*auth.User, auth.Session, error) {
	if req.Header.Get(xsrf.XSRFTokenMetadataKey) != "" {
		return m.cookieAuth.Authenticate(ctx, req)
	}
	return nil, nil, nil // skip this method
}

func prepareTemplates(opts *server.Options) *templates.Bundle {
	versionID := "unknown"
	if idx := strings.LastIndex(opts.ContainerImageID, ":"); idx != -1 {
		versionID = opts.ContainerImageID[idx+1:]
	}
	return &templates.Bundle{
		Loader:          templates.FileSystemLoader("templates"),
		DebugMode:       func(context.Context) bool { return !opts.Prod },
		DefaultTemplate: "base",
		DefaultArgs: func(ctx context.Context, e *templates.Extra) (templates.Args, error) {
			logoutURL, err := auth.LogoutURL(ctx, e.Request.URL.RequestURI())
			if err != nil {
				return nil, err
			}
			token, err := xsrf.Token(ctx)
			if err != nil {
				return nil, err
			}
			return templates.Args{
				"AppVersion": versionID,
				"User":       auth.CurrentUser(ctx),
				"LogoutURL":  logoutURL,
				"XSRFToken":  token,
			}, nil
		},
	}
}

// requireLogin redirect anonymous users to the login page.
func requireLogin(ctx *router.Context, next router.Handler) {
	if auth.CurrentIdentity(ctx.Context) != identity.AnonymousIdentity {
		next(ctx) // already logged in
		return
	}

	loginURL, err := auth.LoginURL(ctx.Context, ctx.Request.URL.RequestURI())
	if err != nil {
		replyError(ctx, err, "Failed to generate the login URL", http.StatusInternalServerError)
		return
	}

	http.Redirect(ctx.Writer, ctx.Request, loginURL, http.StatusFound)
}

// authorizeUIAccess checks the user is allowed to access the web UI.
func authorizeUIAccess(ctx *router.Context, next router.Handler) {
	switch yes, err := auth.IsMember(ctx.Context, impl.ServiceAccessGroup); {
	case err != nil:
		replyError(ctx, err, "Failed to check group membership", http.StatusInternalServerError)
	case !yes:
		templates.MustRender(ctx.Context, ctx.Writer, "pages/access_denied.html", nil)
	default:
		next(ctx)
	}
}

// replyError renders an HTML page with an error message.
//
// Also logs the internal error in the server logs.
func replyError(ctx *router.Context, err error, message string, code int) {
	logging.Errorf(ctx.Context, "%s: %s", message, err)
	ctx.Writer.WriteHeader(code)
	templates.MustRender(ctx.Context, ctx.Writer, "pages/error.html", templates.Args{
		"SimpleHeader": true,
		"Message":      message,
	})
}
