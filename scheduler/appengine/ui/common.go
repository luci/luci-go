// Copyright 2015 The LUCI Authors.
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
	"fmt"
	"html/template"
	"os"
	"strings"
	"time"

	"go.chromium.org/luci/appengine/gaeauth/server"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/gae/service/info"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/xsrf"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/templates"

	"go.chromium.org/luci/scheduler/appengine/catalog"
	"go.chromium.org/luci/scheduler/appengine/engine"
)

// Config is global configuration of UI handlers.
type Config struct {
	Engine        engine.Engine
	Catalog       catalog.Catalog
	TemplatesPath string // path to templates directory deployed to GAE
}

// InstallHandlers adds HTTP handlers that render HTML pages.
func InstallHandlers(r *router.Router, base router.MiddlewareChain, cfg Config) {
	tmpl := prepareTemplates(cfg.TemplatesPath)

	m := base.Extend(func(c *router.Context, next router.Handler) {
		ctx := context.WithValue(c.Request.Context(), configContextKey(0), &cfg)
		c.Request = c.Request.WithContext(context.WithValue(ctx, startTimeContextKey(0), clock.Now(ctx)))
		next(c)
	})
	m = m.Extend(
		templates.WithTemplates(tmpl),
		auth.Authenticate(server.UsersAPIAuthMethod{}),
	)

	r.GET("/", m, indexPage)
	r.GET("/jobs/:ProjectID", m, projectPage)
	r.GET("/jobs/:ProjectID/:JobName", m, jobPage)
	r.GET("/jobs/:ProjectID/:JobName/:InvID", m, invocationPage)

	// All POST forms must be protected with XSRF token.
	mxsrf := m.Extend(xsrf.WithTokenCheck)
	r.POST("/actions/triggerJob/:ProjectID/:JobName", mxsrf, triggerJobAction)
	r.POST("/actions/pauseJob/:ProjectID/:JobName", mxsrf, pauseJobAction)
	r.POST("/actions/resumeJob/:ProjectID/:JobName", mxsrf, resumeJobAction)
	r.POST("/actions/abortJob/:ProjectID/:JobName", mxsrf, abortJobAction)
	r.POST("/actions/abortInvocation/:ProjectID/:JobName/:InvID", mxsrf, abortInvocationAction)
}

type configContextKey int

// config returns Config passed to InstallHandlers.
func config(c context.Context) *Config {
	cfg, _ := c.Value(configContextKey(0)).(*Config)
	if cfg == nil {
		panic("impossible, configContextKey is not set")
	}
	return cfg
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
		Loader:          templates.FileSystemLoader(os.DirFS(templatesPath)),
		DebugMode:       info.IsDevAppServer,
		DefaultTemplate: "base",
		FuncMap: template.FuncMap{
			// Count returns sequential integers in range [0, n] (inclusive).
			"Count": func(n int) []int {
				out := make([]int, n+1)
				for i := range out {
					out[i] = i
				}
				return out
			},
			// Pair combines two args into one map with keys "First" and "Second", to
			// pass pairs to templates (that in golang can accept only one argument).
			"Pair": func(a1, a2 any) map[string]any {
				return map[string]any{
					"First":  a1,
					"Second": a2,
				}
			},
			// JobCount returns "<n> job(s)".
			"JobCount": func(jobs sortedJobs) string {
				switch len(jobs) {
				case 0:
					return "NONE"
				case 1:
					return "1 JOB"
				default:
					return fmt.Sprintf("%d JOBS", len(jobs))
				}
			},
		},
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
