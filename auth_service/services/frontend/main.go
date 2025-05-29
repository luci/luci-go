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
	"net/http"
	"os"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/grpc/prpc"
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
	"go.chromium.org/luci/auth_service/impl/servers/replicas"
	"go.chromium.org/luci/auth_service/internal/configs/srvcfg/settingscfg"
	"go.chromium.org/luci/auth_service/services/frontend/subscription"

	// Ensure registration of validation rules.
	_ "go.chromium.org/luci/auth_service/internal/configs/validation"
	// Store auth sessions in the datastore.
	_ "go.chromium.org/luci/server/encryptedcookies/session/datastore"
)

var (
	// ErrAnonymousIdentity is returned for APIs that require users to be
	// signed in.
	ErrAnonymousIdentity = errors.New("anonymous identity")
	// ErrCheckMembership is returned if there is an error looking up group
	// memberships when checking API access.
	ErrCheckMembership = errors.New("failed to check group membership")
	// ErrAccessDenied is returned if the caller does not have access to an API.
	ErrAccessDenied = errors.New("access denied")
)

func main() {
	modules := []module.Module{
		encryptedcookies.NewModuleFromFlags(), // for authenticating web UI calls
	}

	impl.Main(modules, func(srv *server.Server) error {
		// On GAE '/static' is served by GAE itself (see
		// service-defaultv2.yaml). When running locally in dev mode we need to
		// do it ourselves.
		if !srv.Options.Prod {
			srv.Routes.Static("/ui/static", nil, http.Dir("./static"))
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

		// Initialize authdb server.
		authdbServer := authdb.NewServer()
		srv.RegisterWarmup(authdbServer.WarmUp)
		srv.RunInBackground("authdb.refresh-all-permissions", authdbServer.RefreshPeriodically)

		// Initialize groups server.
		groupsServer := groups.NewServer()
		srv.RegisterWarmup(groupsServer.WarmUp)
		srv.RunInBackground("authdb.refresh-all-groups", groupsServer.RefreshPeriodically)

		// Register all RPC servers.
		internalspb.RegisterInternalsServer(srv, &internals.Server{})
		rpcpb.RegisterAccountsServer(srv, &accounts.Server{})
		rpcpb.RegisterGroupsServer(srv, groupsServer)
		rpcpb.RegisterAllowlistsServer(srv, &allowlists.Server{})
		rpcpb.RegisterAuthDBServer(srv, authdbServer)
		rpcpb.RegisterChangeLogsServer(srv, &changelogs.Server{})
		rpcpb.RegisterReplicasServer(srv, &replicas.Server{})

		// Register pPRC servers.
		srv.ConfigurePRPC(func(s *prpc.Server) {
			// Allow cross-origin calls.
			s.AccessControl = prpc.AllowOriginAll
		})

		// The middleware chain applied to all UI routes.
		uiMW := router.MiddlewareChain{
			templates.WithTemplates(prepareTemplates(&srv.Options)),
			auth.Authenticate(srv.CookieAuth),
			requireLogin,
			authorizeUIAccess,
		}

		srv.Routes.GET("/", uiMW, func(ctx *router.Context) {
			http.Redirect(ctx.Writer, ctx.Request, "/auth/groups", http.StatusFound)
		})
		srv.Routes.GET("/auth/groups", uiMW, servePage("pages/groups.html"))
		// Note that external groups have "/" in their names.
		srv.Routes.GET("/auth/groups/*groupName", uiMW, servePage("pages/groups.html"))
		srv.Routes.GET("/auth/listing", uiMW, servePage("pages/listing.html"))
		srv.Routes.GET("/auth/change_log", uiMW, servePage("pages/change_log.html"))
		srv.Routes.GET("/auth/ip_allowlists", uiMW, servePage("pages/ip_allowlists.html"))
		srv.Routes.GET("/auth/lookup", uiMW, servePage("pages/lookup.html"))
		srv.Routes.GET("/auth/services", uiMW, servePage("pages/services.html"))

		// Helper to create middleware chains requiring the caller to be
		// authenticated and a member in any of the given groups.
		makeAPIMW := func(groups ...string) router.MiddlewareChain {
			return router.MiddlewareChain{
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
				authorizeAPIAccess(groups),
			}
		}

		// Middleware chain for legacy APIs available to trusted services.
		// These are services that are eligible for PubSub subscriber
		// authorization and read access to the AuthDB in its entirety.
		trustedServicesMW := makeAPIMW(model.TrustedServicesGroup, model.AdminGroup)
		// For PubSub subscriber and AuthDB Google Storage reader authorization.
		//
		// Note: the endpoint path is unchanged as there are no API changes,
		// and it's specified in
		// https://pkg.go.dev/go.chromium.org/luci/server/auth/service#AuthService.RequestAccess
		srv.Routes.GET("/auth_service/api/v1/authdb/subscription/authorization",
			trustedServicesMW, adaptGrpcErr(subscription.CheckAccess))
		srv.Routes.POST("/auth_service/api/v1/authdb/subscription/authorization",
			trustedServicesMW, adaptGrpcErr(subscription.Authorize))
		srv.Routes.DELETE("/auth_service/api/v1/authdb/subscription/authorization",
			trustedServicesMW, adaptGrpcErr(subscription.Deauthorize))
		// Support legacy endpoint for AuthDB revisions.
		srv.Routes.GET("/auth_service/api/v1/authdb/revisions/:revID",
			trustedServicesMW, adaptGrpcErr(authdbServer.HandleLegacyAuthDBServing))

		// Middleware chain for legacy APIs available with basic service access.
		// These are callers that have read access to groups.
		authServiceAccessMW := makeAPIMW(srvauthdb.AuthServiceAccessGroup, model.AdminGroup)
		// Support legacy endpoint to get an AuthGroup.
		srv.Routes.GET("/auth/api/v1/groups/*groupName",
			authServiceAccessMW, adaptGrpcErr(groupsServer.GetLegacyAuthGroup))
		// Support legacy endpoint to get the expanded listing of an AuthGroup.
		srv.Routes.GET("/auth/api/v1/listing/groups/*groupName",
			authServiceAccessMW, adaptGrpcErr(groupsServer.GetLegacyListing))
		// Support legacy endpoint to check group membership.
		srv.Routes.GET("/auth/api/v1/memberships/check",
			authServiceAccessMW, adaptGrpcErr(authdbServer.CheckLegacyMembership))

		// Add endpoint for group imports.
		// No group membership required, but the caller must be signed in.
		requireAuthenticatedMW := makeAPIMW()
		srv.Routes.PUT("/auth_service/api/v1/importer/ingest_tarball/:tarballName",
			requireAuthenticatedMW, adaptGrpcErr(imports.HandleTarballIngestHandler))

		// Allow anonymous access to the OAuth config.
		srv.Routes.GET("/auth/api/v1/server/oauth_config", nil, adaptGrpcErr(oauth.HandleLegacyOAuthEndpoint))

		return nil
	})
}

func servePage(templatePath string) func(*router.Context) {
	return func(ctx *router.Context) {
		// Forbid loading within an iframe (see
		// https://www.owasp.org/index.php/Clickjacking_Defense_Cheat_Sheet).
		ctx.Writer.Header().Set("X-Frame-Options", "DENY")
		ctx.Writer.Header().Set("Content-Security-Policy", "frame-ancestors 'none';")

		templates.MustRender(ctx.Request.Context(), ctx.Writer, templatePath, nil)
	}
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

// getIntegratedUI gets the URL to an integrated UI for Auth Service.
func getIntegratedUI(ctx context.Context) string {
	cfg, err := settingscfg.Get(ctx)
	if err != nil {
		// Non-fatal; just log the error.
		err = errors.Fmt("error getting settings.cfg: %w", err)
		logging.Errorf(ctx, err.Error())
		return ""
	}
	return cfg.IntegratedUiUrl
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
				"AppVersion":   opts.ImageVersion(),
				"User":         auth.CurrentUser(ctx),
				"IsAdmin":      isAdmin,
				"LogoutURL":    logoutURL,
				"XSRFToken":    token,
				"IntegratedUI": getIntegratedUI(ctx),
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

// authorizeAPIAccess generates a function which checks the caller is signed in
// and is a member in any of the given groups.
func authorizeAPIAccess(groups []string) func(ctx *router.Context, next router.Handler) {
	return func(ctx *router.Context, next router.Handler) {
		jsonErr := func(err error, code int) {
			w := ctx.Writer
			if res, err := json.Marshal(map[string]any{"text": err.Error()}); err == nil {
				http.Error(w, string(res), code)
			}
		}

		// Check the caller is signed in.
		requestCtx := ctx.Request.Context()
		caller := auth.CurrentIdentity(requestCtx)
		if caller == identity.AnonymousIdentity {
			jsonErr(ErrAnonymousIdentity, http.StatusForbidden)
			return
		}

		if len(groups) == 0 {
			next(ctx)
			return
		}

		switch yes, err := auth.IsMember(requestCtx, groups...); {
		case err != nil:
			jsonErr(ErrCheckMembership, http.StatusInternalServerError)
		case !yes:
			// Internally log a few details of the unauthorized call.
			logging.Warningf(
				ctx.Request.Context(),
				"%s is not a member in any of: %s",
				caller, strings.Join(groups, ", "))
			jsonErr(ErrAccessDenied, http.StatusForbidden)
		default:
			next(ctx)
		}
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
