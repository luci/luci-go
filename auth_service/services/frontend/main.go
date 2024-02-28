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
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/auth"
	srvauthdb "go.chromium.org/luci/server/auth/authdb"
	"go.chromium.org/luci/server/auth/openid"
	"go.chromium.org/luci/server/auth/xsrf"
	"go.chromium.org/luci/server/encryptedcookies"
	"go.chromium.org/luci/server/module"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/templates"

	"go.chromium.org/luci/auth_service/api/internalspb"
	"go.chromium.org/luci/auth_service/api/rpcpb"
	"go.chromium.org/luci/auth_service/impl"
	"go.chromium.org/luci/auth_service/impl/model"
	"go.chromium.org/luci/auth_service/impl/servers/accounts"
	"go.chromium.org/luci/auth_service/impl/servers/allowlists"
	"go.chromium.org/luci/auth_service/impl/servers/authdb"
	"go.chromium.org/luci/auth_service/impl/servers/changelogs"
	"go.chromium.org/luci/auth_service/impl/servers/groups"
	"go.chromium.org/luci/auth_service/impl/servers/imports"
	"go.chromium.org/luci/auth_service/impl/servers/internals"
	"go.chromium.org/luci/auth_service/impl/servers/oauth"
	"go.chromium.org/luci/auth_service/services/frontend/subscription"

	// Ensure registration of validation rules.
	_ "go.chromium.org/luci/auth_service/internal/configs/validation"
	// Store auth sessions in the datastore.
	_ "go.chromium.org/luci/server/encryptedcookies/session/datastore"
)

func main() {
	modules := []module.Module{
		encryptedcookies.NewModuleFromFlags(), // for authenticating web UI calls
	}

	// Parse flags from environment variables.
	dryRunAPIChange := model.ParseDryRunEnvVar(model.DryRunAPIChangesEnvVar)
	enableGroupImports := model.ParseEnableEnvVar(model.EnableGroupImportsEnvVar)

	impl.Main(modules, func(srv *server.Server) error {
		// On GAE '/static' is served by GAE itself (see app.yaml). When running
		// locally in dev mode we need to do it ourselves.
		if !srv.Options.Prod {
			srv.Routes.Static("/static", nil, http.Dir("./static"))
		}

		// Cookie auth and pRPC have some rough edges, see prpcCookieAuth comment.
		prpcAuth := &prpcCookieAuth{cookieAuth: srv.CookieAuth}

		// Authentication methods for RPC APIs.
		srv.SetRPCAuthMethods([]auth.Method{
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
		})

		// Interceptors applying to all RPC APIs.
		srv.RegisterUnifiedServerInterceptors(
			xsrf.Interceptor(prpcAuth),
			impl.AuthorizeRPCAccess,
		)

		authdbServer := &authdb.Server{}

		// Register all RPC servers.
		internalspb.RegisterInternalsServer(srv, &internals.Server{})
		rpcpb.RegisterAccountsServer(srv, &accounts.Server{})
		rpcpb.RegisterGroupsServer(srv, groups.NewServer(dryRunAPIChange))
		rpcpb.RegisterAllowlistsServer(srv, &allowlists.Server{})
		rpcpb.RegisterAuthDBServer(srv, authdbServer)
		rpcpb.RegisterChangeLogsServer(srv, &changelogs.Server{})

		// The middleware chain applied to all plain HTTP routes.
		mw := router.MiddlewareChain{
			templates.WithTemplates(prepareTemplates(&srv.Options)),
			auth.Authenticate(srv.CookieAuth),
			requireLogin,
			authorizeUIAccess,
		}

		// The middleware chain for API like routes.
		apiMw := router.MiddlewareChain{
			auth.Authenticate(
				// The preferred authentication method.
				&openid.GoogleIDTokenAuthMethod{
					AudienceCheck: openid.AudienceMatchesHost,
					SkipNonJWT:    true, // pass OAuth2 access tokens through
				},
				// Backward compatibility for the RPC Explorer and old clients.
				&auth.GoogleOAuth2Method{
					Scopes: []string{"https://www.googleapis.com/auth/userinfo.email"},
				},
			),
			authorizeAPIAccess,
		}

		srv.Routes.GET("/", mw, func(ctx *router.Context) {
			http.Redirect(ctx.Writer, ctx.Request, "/groups", http.StatusFound)
		})
		srv.Routes.GET("/groups", mw, func(ctx *router.Context) {
			templates.MustRender(ctx.Request.Context(), ctx.Writer, "pages/groups.html", nil)
		})
		// Note that external groups have "/" in their names.
		srv.Routes.GET("/groups/*groupName", mw, func(ctx *router.Context) {
			templates.MustRender(ctx.Request.Context(), ctx.Writer, "pages/groups.html", nil)
		})
		srv.Routes.GET("/change_log", mw, func(ctx *router.Context) {
			templates.MustRender(ctx.Request.Context(), ctx.Writer, "pages/change_log.html", nil)
		})
		srv.Routes.GET("/ip_allowlists", mw, func(ctx *router.Context) {
			templates.MustRender(ctx.Request.Context(), ctx.Writer, "pages/ip_allowlists.html", nil)
		})
		srv.Routes.GET("/lookup", mw, func(ctx *router.Context) {
			templates.MustRender(ctx.Request.Context(), ctx.Writer, "pages/lookup.html", nil)
		})

		// For PubSub subscriber and AuthDB Google Storage reader authorization.
		//
		// Note: the endpoint path is unchanged as there are no API changes,
		// and it's specified in
		// https://pkg.go.dev/go.chromium.org/luci/server/auth/service#AuthService.RequestAccess
		srv.Routes.GET("/auth_service/api/v1/authdb/subscription/authorization", apiMw, adaptGrpcErr(subscription.CheckAccess))
		srv.Routes.POST("/auth_service/api/v1/authdb/subscription/authorization", apiMw, adaptGrpcErr(subscription.Authorize))
		srv.Routes.DELETE("/auth_service/api/v1/authdb/subscription/authorization", apiMw, adaptGrpcErr(subscription.Deauthorize))

		// Legacy authdbrevision serving.
		// TODO(cjacomet): Add smoke test for this endpoint
		srv.Routes.GET("/auth_service/api/v1/authdb/revisions/:revID", apiMw, adaptGrpcErr(authdbServer.HandleLegacyAuthDBServing))
		srv.Routes.GET("/auth/api/v1/server/oauth_config", nil, adaptGrpcErr(oauth.HandleLegacyOAuthEndpoint))
		if enableGroupImports {
			srv.Routes.PUT("/auth_service/api/v1/importer/ingest_tarball/:tarballName", apiMw, adaptGrpcErr(imports.HandleTarballIngestHandler))
		}

		// Endpoint to serve the V2AuthDBSnapshot for validation of Auth
		// Service v2.
		//
		// TODO: Remove this and its handler once we have fully rolled out
		// Auth Service v2 (b/321019030).
		srv.Routes.GET("/auth_service/api/v2/authdb/revisions/:revID", apiMw, adaptGrpcErr(authdbServer.HandleV2AuthDBServing))

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
func (m *prpcCookieAuth) Authenticate(ctx context.Context, req auth.RequestMetadata) (*auth.User, auth.Session, error) {
	if req.Header(xsrf.XSRFTokenMetadataKey) != "" {
		return m.cookieAuth.Authenticate(ctx, req)
	}
	return nil, nil, nil // skip this method
}

func prepareTemplates(opts *server.Options) *templates.Bundle {
	return &templates.Bundle{
		Loader:          templates.FileSystemLoader(os.DirFS("templates")),
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
			isAdmin, err := auth.IsMember(ctx, model.AdminGroup)
			if err != nil {
				return nil, err
			}
			return templates.Args{
				"AppVersion": opts.ImageVersion(),
				"User":       auth.CurrentUser(ctx),
				"IsAdmin":    isAdmin,
				"LogoutURL":  logoutURL,
				"XSRFToken":  token,
			}, nil
		},
	}
}

// requireLogin redirect anonymous users to the login page.
func requireLogin(ctx *router.Context, next router.Handler) {
	if auth.CurrentIdentity(ctx.Request.Context()) != identity.AnonymousIdentity {
		next(ctx) // already logged in
		return
	}

	loginURL, err := auth.LoginURL(ctx.Request.Context(), ctx.Request.URL.RequestURI())
	if err != nil {
		replyError(ctx, err, "Failed to generate the login URL", http.StatusInternalServerError)
		return
	}

	http.Redirect(ctx.Writer, ctx.Request, loginURL, http.StatusFound)
}

// authorizeUIAccess checks the user is allowed to access the web UI.
func authorizeUIAccess(ctx *router.Context, next router.Handler) {
	switch yes, err := auth.IsMember(ctx.Request.Context(), srvauthdb.AuthServiceAccessGroup); {
	case err != nil:
		replyError(ctx, err, "Failed to check group membership", http.StatusInternalServerError)
	case !yes:
		templates.MustRender(ctx.Request.Context(), ctx.Writer, "pages/access_denied.html", nil)
	default:
		next(ctx)
	}
}

// authorizeAPIAccess checks whether the caller is allowed to access the API.
func authorizeAPIAccess(ctx *router.Context, next router.Handler) {
	jsonErr := func(err error, code int) {
		w := ctx.Writer
		if res, err := json.Marshal(map[string]any{"text": err.Error()}); err == nil {
			http.Error(w, string(res), code)
		}
	}

	ingest := strings.Contains(ctx.Request.URL.RequestURI(), "/auth_service/api/v1/importer/ingest_tarball/")

	if auth.CurrentIdentity(ctx.Request.Context()) == identity.AnonymousIdentity {
		jsonErr(errors.New("anonymous identity"), http.StatusForbidden)
		return
	}

	if !ingest {
		switch yes, err := auth.IsMember(ctx.Request.Context(), impl.TrustedServicesGroup, impl.AdminGroup); {
		case err != nil:
			jsonErr(errors.New("failed to check group membership"), http.StatusInternalServerError)
		case !yes:
			jsonErr(fmt.Errorf("%s is not a member of %s", auth.CurrentIdentity(ctx.Request.Context()), impl.TrustedServicesGroup),
				http.StatusForbidden)
		default:
			next(ctx)
		}
	} else {
		next(ctx)
	}
}

// replyError renders an HTML page with an error message.
//
// Also logs the internal error in the server logs.
func replyError(ctx *router.Context, err error, message string, code int) {
	logging.Errorf(ctx.Request.Context(), "%s: %s", message, err)
	ctx.Writer.WriteHeader(code)
	templates.MustRender(ctx.Request.Context(), ctx.Writer, "pages/error.html", templates.Args{
		"SimpleHeader": true,
		"Message":      message,
	})
}

// adaptGrpcErr knows how to convert gRPC-style errors to ugly looking HTTP
// error pages with appropriate HTTP status codes.
//
// Recognizes either real gRPC errors (produced with status.Errorf) or
// grpc-tagged errors produced via grpcutil.
func adaptGrpcErr(h func(*router.Context) error) router.Handler {
	return func(ctx *router.Context) {
		err := grpcutil.GRPCifyAndLogErr(ctx.Request.Context(), h(ctx))
		if code := status.Code(err); code != codes.OK {
			http.Error(ctx.Writer, status.Convert(err).Message(), grpcutil.CodeStatus(code))
		}
	}
}
