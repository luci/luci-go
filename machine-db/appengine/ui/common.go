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

// Package ui contains Machine Database web UI.
package ui

import (
	"net/http"
	"strings"

	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"go.chromium.org/gae/service/info"
	gaeserver "go.chromium.org/luci/appengine/gaeauth/server"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/xsrf"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/templates"

	"go.chromium.org/luci/machine-db/api/crimson/v1"
)

var serverKey = "server"

// InstallHandlers adds HTTP handlers that render HTML pages.
func InstallHandlers(r *router.Router, base router.MiddlewareChain, srv crimson.CrimsonServer, tmplPath string) {
	m := base.Extend(func(c *router.Context, next router.Handler) {
		c.Context = context.WithValue(c.Context, &serverKey, srv)
		next(c)
	})
	m = m.Extend(
		templates.WithTemplates(prepareTemplates(tmplPath)),
		auth.Authenticate(gaeserver.UsersAPIAuthMethod{}),
	)
	r.GET("/", m, indexPage)
	r.GET("/datacenters", m, datacentersPage)
	r.GET("/dracs", m, dracsPage)
	r.GET("/hosts", m, hostsPage)
	r.GET("/kvms", m, kvmsPage)
	r.GET("/machines", m, machinesPage)
	r.GET("/nics", m, nicsPage)
	r.GET("/oses", m, osesPage)
	r.GET("/platforms", m, platformsPage)
	r.GET("/racks", m, racksPage)
	r.GET("/switches", m, switchesPage)
	r.GET("/vlans", m, vlansPage)
	r.GET("/vms", m, vmsPage)
}

func indexPage(c *router.Context) {
	templates.MustRender(c.Context, c.Writer, "pages/index.html", nil)
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
				"IsAnonymous": auth.CurrentIdentity(c) == "anonymous:anonymous",
				"LoginURL":    loginURL,
				"LogoutURL":   logoutURL,
				"User":        auth.CurrentUser(c),
				"XsrfToken":   token,
			}, nil
		},
	}
}

// server returns crimson.CrimsonServer as passed to InstallHandlers.
//
// It is used to pull all data necessary to render pages.
func server(c context.Context) crimson.CrimsonServer {
	s, _ := c.Value(&serverKey).(crimson.CrimsonServer)
	if s == nil {
		panic("impossible, &serverKey is not set")
	}
	return s
}

// See https://github.com/grpc/grpc-go/blob/master/codes/codes.go
var grpcCodeToHTTP = map[codes.Code]int{
	codes.Aborted:            http.StatusConflict,
	codes.AlreadyExists:      http.StatusConflict,
	codes.FailedPrecondition: http.StatusConflict,
	codes.InvalidArgument:    http.StatusBadRequest,
	codes.NotFound:           http.StatusNotFound,
	codes.OutOfRange:         http.StatusBadRequest,
	codes.PermissionDenied:   http.StatusForbidden,
	codes.Unauthenticated:    http.StatusUnauthorized,
}

// renderErr renders a page with error message.
func renderErr(c *router.Context, err error) {
	status := grpcCodeToHTTP[grpc.Code(err)]
	if status == 0 {
		status = http.StatusInternalServerError
	}
	c.Writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	c.Writer.WriteHeader(status)
	templates.MustRender(c.Context, c.Writer, "pages/error.html", map[string]interface{}{
		"Message": err.Error(),
	})
}
