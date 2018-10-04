// Copyright 2018 The LUCI Authors.
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

// Package ui implements request handlers that serve user facing HTML pages.
package ui

import (
	"context"
	"net/url"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/gae/service/info"

	"go.chromium.org/luci/appengine/gaeauth/server"
	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/xsrf"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/templates"
)

// InstallHandlers adds HTTP handlers that render HTML pages.
func InstallHandlers(r *router.Router, base router.MiddlewareChain, templatesPath string) {
	m := base.Extend(func(c *router.Context, next router.Handler) {
		c.Context = context.WithValue(c.Context, startTimeContextKey(0), clock.Now(c.Context))
		next(c)
	}).Extend(
		templates.WithTemplates(prepareTemplates(templatesPath)),
		auth.Authenticate(server.UsersAPIAuthMethod{}),
	)

	r.GET("/", m, renderErr(routeToPage))
	r.GET("/p/*path", m, renderErr(routeToPage))
}

func prefixPageURL(pfx string) string {
	if pfx == "" {
		return "/"
	}
	return "/p/" + pfx
}

func packagePageURL(pkg, cursor string) string {
	p := "/p/" + pkg + "/+/"
	if cursor != "" {
		p += "?c=" + url.QueryEscape(cursor)
	}
	return p
}

func instancePageURL(pkg, ver string) string {
	return "/p/" + pkg + "/+/" + ver
}

// routeToPage routes to an appropriate page depending on the request URL.
func routeToPage(c *router.Context) error {
	path := c.Params.ByName("path")
	switch chunks := strings.SplitN(path, "/+/", 2); {
	case len(chunks) <= 1: // no '/+/' in path => prefix listing page
		return prefixListingPage(c, path)
	case len(chunks) == 2 && chunks[1] == "": // ends with '/+/' => package page
		return packagePage(c, chunks[0])
	case len(chunks) == 2: // has something after '/+/' => instance page
		return instancePage(c, chunks[0], chunks[1])
	default:
		return status.Errorf(codes.InvalidArgument, "malformed page URL")
	}
}

type startTimeContextKey int

// startTime returns timestamp when we started handling the request.
func startTime(c context.Context) time.Time {
	ts, ok := c.Value(startTimeContextKey(0)).(time.Time)
	if !ok {
		panic("impossible, startTimeContextKey is not set")
	}
	return ts
}

// prepareTemplates configures templates.Bundle used by all UI handlers.
//
// In particular it includes a set of default arguments passed to all templates.
func prepareTemplates(templatesPath string) *templates.Bundle {
	return &templates.Bundle{
		Loader:          templates.FileSystemLoader(templatesPath),
		DebugMode:       info.IsDevAppServer,
		DefaultTemplate: "base",
		DefaultArgs: func(c context.Context, e *templates.Extra) (templates.Args, error) {
			loginURL, err := auth.LoginURL(c, e.Request.URL.RequestURI())
			if err != nil {
				return nil, err
			}
			logoutURL, err := auth.LogoutURL(c, e.Request.URL.RequestURI())
			if err != nil {
				return nil, err
			}
			token, err := xsrf.Token(c)
			if err != nil {
				return nil, err
			}
			return templates.Args{
				"AppVersion":  strings.Split(info.VersionID(c), ".")[0],
				"IsAnonymous": auth.CurrentIdentity(c) == identity.AnonymousIdentity,
				"User":        auth.CurrentUser(c),
				"LoginURL":    loginURL,
				"LogoutURL":   logoutURL,
				"XsrfToken":   token,
				"HandlerDuration": func() time.Duration {
					return clock.Now(c).Sub(startTime(c))
				},
			}, nil
		},
	}
}
