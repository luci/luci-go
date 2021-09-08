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

package userhtml

import (
	"context"
	"fmt"
	"html/template"
	"net/http"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/xsrf"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/templates"

	"go.chromium.org/luci/cv/internal/changelist"
)

type startTimeContextKey int

// InstallHandlers adds HTTP handlers that render HTML pages.
func InstallHandlers(srv *server.Server) {
	m := router.NewMiddlewareChain(
		func(c *router.Context, next router.Handler) {
			c.Context = context.WithValue(c.Context, startTimeContextKey(0), clock.Now(c.Context))
			next(c)
		},
		templates.WithTemplates(prepareTemplates(&srv.Options, "templates")),
		auth.Authenticate(srv.CookieAuth),
	)

	srv.Routes.GET("/ui/recents", m, recentsPage)
	srv.Routes.GET("/ui/recents/:Project", m, recentsPage)
	srv.Routes.GET("/ui/run/:Project/:Run", m, runDetails)
}

// prepareTemplates configures templates.Bundle used by all UI handlers.
//
// In particular it includes a set of default arguments passed to all templates.
func prepareTemplates(opts *server.Options, templatesPath string) *templates.Bundle {
	versionID := "unknown"
	if idx := strings.LastIndex(opts.ContainerImageID, ":"); idx != -1 {
		versionID = opts.ContainerImageID[idx+1:]
	}
	return &templates.Bundle{
		Loader:          templates.FileSystemLoader(templatesPath),
		DebugMode:       func(context.Context) bool { return !opts.Prod },
		DefaultTemplate: "base",
		FuncMap: template.FuncMap{
			"FmTime": func(ts time.Time) string {
				return ts.UTC().Format("Mon Jan 2 15:04:05.000 MST 2006")
			},
			"Split": func(s string) []string {
				return strings.Split(s, "\n")
			},
			"SplitSlash": func(s string) []string {
				return strings.Split(s, "/")
			},
			"Title": func(s string) string {
				return strings.Title(strings.ToLower(strings.Replace(s, "_", " ", -1)))
			},
			"LinkifyExternalID": func(eid string) string {
				if eid == "" {
					// Very old RunCL entities don't have ExternalID set.
					return ""
				}
				return changelist.ExternalID(eid).MustURL()
			},
			// Shortens a cl id for display purposes.
			"DisplayExternalID": func(eid string) string {
				if eid == "" {
					// Very old RunCL entities don't have ExternalID set.
					return ""
				}
				cl := changelist.ExternalID(eid)
				host, change, err := cl.ParseGobID()
				if err != nil {
					panic(err)
				}
				host = strings.Replace(host, "-review.googlesource.com", "", 1)
				return fmt.Sprintf("%s/%d", host, change)
			},
			// Runlog specific, see run_details.go.
			"LogTypeString": logTypeString,

			"UITryjob":       makeUITryjob,
			"ByTryjobStatus": groupTryjobsByStatus,
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
				"AppVersion":  versionID,
				"IsAnonymous": auth.CurrentIdentity(ctx) == identity.AnonymousIdentity,
				"User":        auth.CurrentUser(ctx),
				"LoginURL":    loginURL,
				"LogoutURL":   logoutURL,
				"XsrfToken":   token,
				"Now":         startTime(ctx),
				"HandlerDuration": func() time.Duration {
					return clock.Now(ctx).Sub(startTime(ctx))
				},
			}, nil
		},
	}
}

func startTime(c context.Context) time.Time {
	ts, ok := c.Value(startTimeContextKey(0)).(time.Time)
	if !ok {
		panic("impossible, startTimeContextKey is not set")
	}
	return ts
}

func errPage(c *router.Context, err error) {
	logging.Errorf(c.Context, "Error: %s", err)
	code := http.StatusInternalServerError
	if status.Code(errors.Unwrap(err)) == codes.PermissionDenied {
		code = http.StatusForbidden
	}
	c.Writer.WriteHeader(code)
	templates.MustRender(c.Context, c.Writer, "pages/error.html", map[string]interface{}{
		"Error": err,
		"Code":  code,
	})
}
