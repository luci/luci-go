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
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/encryptedcookies"
	"go.chromium.org/luci/server/module"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/templates"

	"go.chromium.org/luci/auth_service/impl"

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
			return templates.Args{
				"AppVersion": versionID,
				"User":       auth.CurrentUser(ctx),
				"LogoutURL":  logoutURL,
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
	switch yes, err := auth.IsMember(ctx.Context, "auth-service-access"); {
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
