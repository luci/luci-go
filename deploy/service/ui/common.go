// Copyright 2022 The LUCI Authors.
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

// Package ui contains implementation of Web UI handlers.
package ui

import (
	"context"
	"net/http"
	"strings"
	"unicode"
	"unicode/utf8"

	"google.golang.org/grpc/status"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/xsrf"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/templates"

	"go.chromium.org/luci/deploy/service/rpcs"
)

// UI hosts UI request handlers.
type UI struct {
	prod    bool   // true when running on GAE
	version string // e.g. "434535-abcdef"

	deployments *rpcs.Deployments
}

// RegisterRoutes installs UI HTTP routes.
func RegisterRoutes(srv *server.Server, deployments *rpcs.Deployments) {
	if !srv.Options.Prod {
		srv.Routes.Static("/static", nil, http.Dir("./static"))
	}

	version := "unknown"
	if idx := strings.LastIndex(srv.Options.ContainerImageID, ":"); idx != -1 {
		version = srv.Options.ContainerImageID[idx+1:]
	}

	ui := UI{
		prod:        srv.Options.Prod,
		version:     version,
		deployments: deployments,
	}

	mw := router.NewMiddlewareChain(
		templates.WithTemplates(ui.prepareTemplates()),
		auth.Authenticate(srv.CookieAuth),
		requireLogin,
	)

	srv.Routes.GET("/", mw, wrapErr(ui.indexPage))
}

// prepareTemplates loads HTML page templates.
func (ui *UI) prepareTemplates() *templates.Bundle {
	return &templates.Bundle{
		Loader:          templates.FileSystemLoader("templates"),
		DebugMode:       func(context.Context) bool { return !ui.prod },
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
				"AppVersion": ui.version,
				"LogoutURL":  logoutURL,
				"User":       auth.CurrentUser(ctx),
				"XsrfToken":  token,
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
		replyErr(ctx, err)
		return
	}

	http.Redirect(ctx.Writer, ctx.Request, loginURL, http.StatusFound)
}

// wrapErr is a handler wrapper that converts gRPC errors into HTML pages.
func wrapErr(h func(*router.Context) error) router.Handler {
	return func(ctx *router.Context) {
		if err := h(ctx); err != nil {
			replyErr(ctx, err)
		}
	}
}

// replyErr renders an HTML page with an error message.
func replyErr(ctx *router.Context, err error) {
	s, _ := status.FromError(err)
	message := s.Message()
	if message != "" {
		// Convert the first rune to upper case.
		r, n := utf8.DecodeRuneInString(message)
		message = string(unicode.ToUpper(r)) + message[n:]
	} else {
		message = "Unspecified error" // this should not really happen
	}

	ctx.Writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	ctx.Writer.WriteHeader(grpcutil.CodeStatus(s.Code()))
	templates.MustRender(ctx.Context, ctx.Writer, "pages/error.html", map[string]interface{}{
		"Message": message,
	})
}
