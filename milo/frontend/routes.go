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

package frontend

import (
	"context"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/julienschmidt/httprouter"
	"go.chromium.org/luci/appengine/gaeauth/server"
	"go.chromium.org/luci/appengine/gaemiddleware"
	"go.chromium.org/luci/appengine/gaemiddleware/standard"
	"go.chromium.org/luci/buildbucket/deprecated"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/grpc/discovery"
	"go.chromium.org/luci/grpc/prpc"
	milopb "go.chromium.org/luci/milo/api/service/v1"
	"go.chromium.org/luci/milo/backend"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/xsrf"
	"go.chromium.org/luci/server/middleware"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/templates"
	"go.chromium.org/luci/web/gowrappers/rpcexplorer"

	clientauth "go.chromium.org/luci/auth"

	"go.chromium.org/luci/milo/buildsource/buildbucket"
	"go.chromium.org/luci/milo/buildsource/swarming"
	"go.chromium.org/luci/milo/common"
)

// Run sets up all the routes and runs the server.
func Run(templatePath string) {
	server.SwitchToEncryptedCookies()

	// Register plain ol' http handlers.
	r := router.New()
	standard.InstallHandlers(r)

	baseMW := standard.Base()
	baseAuthMW := baseMW.Extend(
		middleware.WithContextTimeout(time.Minute),
		auth.Authenticate(server.CookieAuth, &server.OAuth2Method{Scopes: []string{server.EmailScope}}),
	)
	htmlMW := baseAuthMW.Extend(
		withAccessClientMiddleware, // This must be called after the auth.Authenticate middleware.
		withGitMiddleware,
		withBuildbucketBuildsClient,
		withBuildbucketBuildersClient,
		templates.WithTemplates(getTemplateBundle(templatePath)),
	)
	xsrfMW := htmlMW.Extend(xsrf.WithTokenCheck)
	projectMW := htmlMW.Extend(buildProjectACLMiddleware(false))
	optionalProjectMW := htmlMW.Extend(buildProjectACLMiddleware(true))
	backendMW := baseMW.Extend(
		middleware.WithContextTimeout(10*time.Minute),
		withBuildbucketBuildsClient)
	cronMW := backendMW.Extend(gaemiddleware.RequireCron, withBuildbucketBuildsClient)

	r.GET("/", htmlMW, frontpageHandler)
	r.GET("/p", baseMW, movedPermanently("/"))
	r.GET("/search", htmlMW, searchHandler)
	r.GET("/opensearch.xml", baseMW, searchXMLHandler)

	// Admin and cron endpoints.
	r.GET("/admin/configs", htmlMW, ConfigsHandler)

	// Cron endpoints
	r.GET("/internal/cron/update-config", cronMW, UpdateConfigHandler)
	r.GET("/internal/cron/update-pools", cronMW, cronHandler(buildbucket.UpdatePools))

	// Artifacts.
	r.GET("/artifact/*path", baseMW, redirect("/ui/artifact/*path", http.StatusFound))

	// Invocations.
	r.GET("/inv/*path", baseMW, redirect("/ui/inv/*path", http.StatusFound))

	// Builds.
	r.GET("/b/:id", htmlMW, handleError(redirectLUCIBuild))
	r.GET("/p/:project/builds/b:id", baseMW, movedPermanently("/b/:id"))

	buildPageMW := router.NewMiddlewareChain(func(c *router.Context, next router.Handler) {
		shouldShowNewBuildPage := getShowNewBuildPageCookie(c)
		if shouldShowNewBuildPage {
			redirect("/ui/p/:project/builders/:bucket/:builder/:numberOrId", http.StatusFound)(c)
		} else {
			next(c)
		}
	}).Extend(optionalProjectMW...)
	r.GET("/p/:project/builders/:bucket/:builder/:numberOrId", buildPageMW, handleError(handleLUCIBuild))
	// TODO(crbug/1108198): remvoe this route once we turned down the old build page.
	r.GET("/old/p/:project/builders/:bucket/:builder/:numberOrId", optionalProjectMW, handleError(handleLUCIBuild))

	// Only the new build page can take path suffix, redirect to the new build page.
	r.GET("/b/:id/*path", baseMW, redirect("/ui/b/:id/*path", http.StatusFound))
	r.GET("/p/:project/builds/b:id/*path", baseMW, redirect("/ui/b/:id/*path", http.StatusFound))
	r.GET("/p/:project/builders/:bucket/:builder/:numberOrId/*path", baseMW, redirect("/ui/p/:project/builders/:bucket/:builder/:numberOrId/*path", http.StatusFound))

	// Console
	r.GET("/p/:project", projectMW, handleError(func(c *router.Context) error {
		return ConsolesHandler(c, c.Params.ByName("project"))
	}))
	r.GET("/p/:project/", baseMW, movedPermanently("/p/:project"))
	r.GET("/p/:project/g", baseMW, movedPermanently("/p/:project"))
	r.GET("/p/:project/g/:group/console", projectMW, handleError(ConsoleHandler))
	r.GET("/p/:project/g/:group", projectMW, redirect("/p/:project/g/:group/console", http.StatusFound))
	r.GET("/p/:project/g/:group/", baseMW, movedPermanently("/p/:project/g/:group"))

	// Builder list
	r.GET("/p/:project/builders", projectMW, handleError(func(c *router.Context) error {
		return BuildersRelativeHandler(c, c.Params.ByName("project"), "")
	}))
	r.GET("/p/:project/g/:group/builders", projectMW, handleError(func(c *router.Context) error {
		return BuildersRelativeHandler(c, c.Params.ByName("project"), c.Params.ByName("group"))
	}))

	// Swarming
	r.GET(swarming.URLBase+"/:id/steps/*logname", htmlMW, handleError(HandleSwarmingLog))
	r.GET(swarming.URLBase+"/:id", htmlMW, handleError(handleSwarmingBuild))
	// Backward-compatible URLs for Swarming:
	r.GET("/swarming/prod/:id/steps/*logname", htmlMW, handleError(HandleSwarmingLog))
	r.GET("/swarming/prod/:id", htmlMW, handleError(handleSwarmingBuild))

	// Buildbucket
	// If these routes change, also change links in common/model/build_summary.go:getLinkFromBuildID
	// and common/model/builder_summary.go:SelfLink.
	r.GET("/p/:project/builders/:bucket/:builder", optionalProjectMW, handleError(BuilderHandler))

	r.GET("/buildbucket/:bucket/:builder", baseMW, redirectFromProjectlessBuilder)

	// LogDog Milo Annotation Streams.
	// This mimics the `logdog://logdog_host/project/*path` url scheme seen on
	// swarming tasks.
	r.GET("/raw/build/:logdog_host/:project/*path", htmlMW, handleError(handleRawPresentationBuild))

	// PubSub subscription endpoints.
	r.POST("/_ah/push-handlers/buildbucket", backendMW, buildbucket.PubSubHandler)

	r.POST("/actions/cancel_build", xsrfMW, handleError(cancelBuildHandler))
	r.POST("/actions/retry_build", xsrfMW, handleError(retryBuildHandler))

	r.GET("/internal_widgets/related_builds/:id", htmlMW, handleError(handleGetRelatedBuildsTable))

	// Config for ResultUI frontend.
	r.GET("/configs.js", baseMW, handleError(configsJSHandler))

	r.GET("/auth-state", baseAuthMW, handleError(getAuthState))

	apiMW := baseMW.Extend(withGitMiddleware)
	installAPIRoutes(r, apiMW)

	http.DefaultServeMux.Handle("/", r)
}

func installAPIRoutes(r *router.Router, base router.MiddlewareChain) {
	server := &prpc.Server{
		// The default OAuth2Method invalidates all access tokens associated
		// with an OAuth client ID once the user signed out. This is problematic
		// because the access token could be cached and we have Milo hosted on
		// multiple domains.
		Authenticator: &auth.Authenticator{
			Methods: []auth.Method{
				&auth.GoogleOAuth2Method{
					Scopes: []string{clientauth.OAuthScopeEmail},
				},
			},
		},
	}
	milopb.RegisterMiloInternalServer(server, &backend.MiloInternalService{})

	discovery.Enable(server)
	rpcexplorer.Install(r)
	server.InstallHandlers(r, base)
}

// handleError is a wrapper for a handler so that the handler can return an error
// rather than call ErrorHandler directly.
// This should be used for handlers that render webpages.
func handleError(handler func(c *router.Context) error) func(c *router.Context) {
	return func(c *router.Context) {
		if err := handler(c); err != nil {
			ErrorHandler(c, err)
		}
	}
}

// cronHandler is a wrapper for cron handlers which do not require template rendering.
func cronHandler(handler func(c context.Context) error) func(c *router.Context) {
	return func(ctx *router.Context) {
		if err := handler(ctx.Context); err != nil {
			logging.WithError(err).Errorf(ctx.Context, "failed to run")
			ctx.Writer.WriteHeader(http.StatusInternalServerError)
			return
		}
		ctx.Writer.WriteHeader(http.StatusOK)
	}
}

// redirect returns a handler that responds with given HTTP status
// with a location specified by the pathTemplate.
func redirect(pathTemplate string, status int) router.Handler {
	if !strings.HasPrefix(pathTemplate, "/") {
		panic("pathTemplate must start with /")
	}

	interpolator := createInterpolator(pathTemplate)
	return func(c *router.Context) {
		path := interpolator(c.Params)
		http.Redirect(c.Writer, c.Request, path, status)
	}
}

// createInterpolator returns a function that can replace the variables in the
// pathTemplate with the provided params.
func createInterpolator(pathTemplate string) func(params httprouter.Params) string {
	templateParts := strings.Split(pathTemplate, "/")

	return func(params httprouter.Params) string {
		components := make([]string, 0, len(templateParts))

		for _, p := range templateParts {
			if strings.HasPrefix(p, ":") {
				components = append(components, params.ByName(p[1:]))
			} else if strings.HasPrefix(p, "*_") {
				// httprouter uses the decoded URL path to perform routing
				// (which defeats the whole purpose of encoding), so we have to
				// use '*' to capture a path component containing %2F.
				// "*_" is a special syntax to signal that although we are
				// capturing all characters till the end of the path, the
				// captured value should be treated as a single path component,
				// therefore '/' should also be encoded.
				//
				// Caveat: because '*' is used, this hack only works for the
				// last path component.
				//
				// https://github.com/julienschmidt/httprouter/issues/284
				component := params.ByName(p[1:])
				component = strings.TrimPrefix(component, "/")
				components = append(components, component)
			} else if strings.HasPrefix(p, "*") {
				path := params.ByName(p[1:])
				path = strings.TrimPrefix(path, "/")

				// Split the path into components before passing them to
				// url.PathEscape. Otherwise url.PathEscape will encode "/" into
				// "%2F" because it escapes all non-safe characters in a path
				// component (it should be renamed to url.PathComponentEscape).
				components = append(components, strings.Split(path, "/")...)
			} else {
				components = append(components, p)
			}
		}

		// Escape the path components ourselves.
		// url.URL.String() should not be used because it escapes everything
		// automatically except '/' making it impossible to have %2F (encoded
		// '/') in a path component ('%2F' will be double encoded to '%252F'
		// while '/' won't be encoded at all).
		for i, p := range components {
			components[i] = url.PathEscape(p)
		}
		return strings.Join(components, "/")
	}
}

// movedPermanently is a special instance of redirect, returning a handler
// that responds with HTTP 301 (Moved Permanently) with a location specified
// by the pathTemplate.
//
// TODO(nodir,iannucci): delete all usages.
func movedPermanently(pathTemplate string) router.Handler {
	return redirect(pathTemplate, http.StatusMovedPermanently)
}

func redirectFromProjectlessBuilder(c *router.Context) {
	bucket := c.Params.ByName("bucket")
	builder := c.Params.ByName("builder")

	project, _ := deprecated.BucketNameToV2(bucket)
	u := *c.Request.URL
	u.Path = fmt.Sprintf("/p/%s/builders/%s/%s", project, bucket, builder)
	http.Redirect(c.Writer, c.Request, u.String(), http.StatusMovedPermanently)
}

// configsJSHandler serves /configs.js used by ResultUI frontend code.
func configsJSHandler(c *router.Context) error {
	template, err := template.ParseFiles("templates/configs.template.js")
	if err != nil {
		logging.Errorf(c.Context, "Failed to load configs.template.js: %s", err)
		return err
	}

	settings := common.GetSettings(c.Context)

	header := c.Writer.Header()
	header.Set("content-type", "application/javascript")

	// The configs file rarely changes, and may block other scripts from running.
	// Set max-age to one hour, stale-while-revalidate to 7 days to improve
	// performance.
	header.Set("cache-control", "max-age=3600,stale-while-revalidate=604800")
	err = template.Execute(c.Writer, map[string]interface{}{
		"ResultDB": map[string]string{
			"Host": settings.GetResultdb().GetHost(),
		},
		"Buildbucket": map[string]string{
			"Host": settings.GetBuildbucket().GetHost(),
		},
	})

	if err != nil {
		logging.Errorf(c.Context, "Failed to execute configs.template.js: %s", err)
		return err
	}

	return nil
}
