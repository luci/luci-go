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

package ui

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"time"

	"github.com/dustin/go-humanize"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/clock"
	cfgcommonpb "go.chromium.org/luci/common/proto/config"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/xsrf"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/templates"
	bootstrap "go.chromium.org/luci/web/third_party/bootstrap/v5"

	"go.chromium.org/luci/config_service/internal/importer"
	configpb "go.chromium.org/luci/config_service/proto"
)

//go:embed pages includes
var templateFS embed.FS

//go:embed static
var staticFS embed.FS

var pageStartTimeContextKey = "start time for the frontend page"
var configsServerContextKey = "configsServer for the front page"

// InstallHandlers adds HTTP handlers that render HTML pages.
func InstallHandlers(srv *server.Server, configsSrv configpb.ConfigsServer, importer importer.Importer) {
	m := router.NewMiddlewareChain(
		func(c *router.Context, next router.Handler) {
			reqCtx := c.Request.Context()
			reqCtx = context.WithValue(reqCtx, &pageStartTimeContextKey, clock.Now(reqCtx))
			reqCtx = context.WithValue(reqCtx, &configsServerContextKey, configsSrv)
			c.Request = c.Request.WithContext(reqCtx)
			next(c)
		},
		templates.WithTemplates(prepareTemplates(&srv.Options)),
		auth.Authenticate(srv.CookieAuth),
	)
	switch sFS, err := fs.Sub(staticFS, "static"); {
	case err != nil:
		panic(fmt.Errorf("failed to return a subFS for static directory: %w", err))
	default:
		// staticFS has "static" directory at top level. We need to make the child
		// directories inside the "static" directory top level directory.
		srv.Routes.Static("/static", nil, http.FS(sFS))
	}
	srv.Routes.Static("/third_party/bootstrap", nil, http.FS(bootstrap.FS))

	srv.Routes.GET("/", m, renderErr(indexPage))
	srv.Routes.GET("/config_set/*ConfigSet", m, renderErr(configSetPage))
	srv.Routes.POST("/internal/frontend/reimport/*ConfigSet", m, importer.Reimport)
}

// prepareTemplates configures templates.Bundle used by all UI handlers.
//
// In particular it includes a set of default arguments passed to all templates.
func prepareTemplates(opts *server.Options) *templates.Bundle {
	return &templates.Bundle{
		Loader:          templates.FileSystemLoader(templateFS),
		DebugMode:       func(context.Context) bool { return !opts.Prod },
		DefaultTemplate: "base",
		FuncMap: template.FuncMap{
			"RelTime": func(ts, now time.Time) string {
				return humanize.RelTime(ts, now, "ago", "from now")
			},
			"AttemptStatus": attemptStatus,
		},
		DefaultArgs: func(ctx context.Context, e *templates.Extra) (templates.Args, error) {
			loginURL, err := auth.LoginURL(ctx, e.Request.URL.RequestURI())
			if err != nil {
				return nil, err
			}
			logoutURL, err := auth.LogoutURL(ctx, e.Request.URL.RequestURI())
			if err != nil {
				return nil, err
			}
			token, err := xsrf.Token(ctx)
			if err != nil {
				return nil, err
			}
			return templates.Args{
				"ImageVersion": opts.ImageVersion(),
				"IsAnonymous":  auth.CurrentIdentity(ctx) == identity.AnonymousIdentity,
				"User":         auth.CurrentUser(ctx),
				"LoginURL":     loginURL,
				"LogoutURL":    logoutURL,
				"XsrfToken":    token,
				"Now":          startTime(ctx),
				"HandlerDuration": func() time.Duration {
					return clock.Now(ctx).Sub(startTime(ctx))
				},
			}, nil
		},
	}
}

func startTime(c context.Context) time.Time {
	ts, ok := c.Value(&pageStartTimeContextKey).(time.Time)
	if !ok {
		panic(errors.New("impossible; pageStartTimeContextKey is not set"))
	}
	return ts
}

func configsServer(c context.Context) configpb.ConfigsServer {
	return c.Value(&configsServerContextKey).(configpb.ConfigsServer)
}

func attemptStatus(attempt *configpb.ConfigSet_Attempt) string {
	if !attempt.Success {
		return "failed"
	}
	for _, msg := range attempt.GetValidationResult().GetMessages() {
		if msg.GetSeverity() == cfgcommonpb.ValidationResult_WARNING {
			return "warning"
		}
	}
	return "success"
}
