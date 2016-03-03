// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package settings

import (
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"

	"google.golang.org/appengine"

	"github.com/julienschmidt/httprouter"
	"github.com/luci/gae/service/info"
	"github.com/luci/luci-go/appengine/cmd/milo/miloerror"
	"github.com/luci/luci-go/appengine/gaeauth/server"
	"github.com/luci/luci-go/appengine/gaemiddleware"
	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/server/auth"
	"github.com/luci/luci-go/server/middleware"
	"github.com/luci/luci-go/server/templates"
	"golang.org/x/net/context"
)

type themeContextKey string

type namedBundle struct {
	name   string
	bundle *templates.Bundle
}

// Theme is the base type for specifying where to find a Theme.
type Theme struct {
	IsTemplate bool
	Name       string
}

// ThemedHandler is the base type for any milo html handlers.
type ThemedHandler interface {
	// Return the template name given a theme name.
	GetTemplateName(Theme) string
	// Render the page for server side renders.
	Render(context.Context, *http.Request, httprouter.Params) (*templates.Args, error)
}

var (
	// Default is the global default theme for anonomyous users.
	Default = Theme{IsTemplate: true, Name: "buildbot"}
	// Themes is a list of all known themes.
	Themes = map[string]Theme{
		"buildbot":  Default,
		"bootstrap": {IsTemplate: true, Name: "bootstrap"},
	}
)

// GetAllThemes gets all known themes as a list of strings.
func GetAllThemes() []string {
	results := make([]string, 0, len(Themes))
	for k := range Themes {
		results = append(results, k)
	}
	return results
}

// templateBundle is used to render HTML templates. It provides a base args
// passed to all templates.
func getTemplateBundles() []namedBundle {
	result := []namedBundle{}
	for name, t := range Themes {
		if t.IsTemplate {
			templateBundle := &templates.Bundle{
				Loader:          templates.FileSystemLoader(path.Join("templates", name)),
				DefaultTemplate: name,
				DebugMode:       appengine.IsDevAppServer(),
				DefaultArgs: func(c context.Context) (templates.Args, error) {
					loginURL, err := auth.LoginURL(c, "/")
					if err != nil {
						return nil, err
					}
					logoutURL, err := auth.LogoutURL(c, "/")
					if err != nil {
						return nil, err
					}
					if err != nil {
						return nil, err
					}
					return templates.Args{
						"AppVersion":  strings.Split(info.Get(c).VersionID(), ".")[0],
						"IsAnonymous": auth.CurrentIdentity(c) == "anonymous:anonymous",
						"User":        auth.CurrentUser(c),
						"LoginURL":    loginURL,
						"LogoutURL":   logoutURL,
						"CurrentTime": clock.Now(c).String(),
					}, nil
				},
				FuncMap: funcMap,
			}
			result = append(result, namedBundle{name, templateBundle})
		}
	}
	return result
}

// useNamedBundle is like templates.Use, but with the choice of one of many bundles (themes)
func useNamedBundle(c context.Context, nb namedBundle) (context.Context, error) {
	err := nb.bundle.EnsureLoaded(c)
	return context.WithValue(c, themeContextKey(nb.name), nb.bundle), err
}

// withNamedBundle is like templates.WithTemplates, but with the choice of one of many bundles (themes)
func withNamedBundle(h middleware.Handler, nb namedBundle) middleware.Handler {
	return func(c context.Context, rw http.ResponseWriter, r *http.Request, p httprouter.Params) {
		c, err := useNamedBundle(c, nb) // calls EnsureLoaded and initializes b.err inside
		if err != nil {
			http.Error(rw, fmt.Sprintf("Can't load HTML templates.\n%s", err), http.StatusInternalServerError)
			return
		}
		h(c, rw, r, p)
	}
}

// themedMustRender renders theme and panics if it can't be rendered.  This should never fail in
// production.
func themedMustRender(c context.Context, out io.Writer, theme, name string, args templates.Args) {
	if b, _ := c.Value(themeContextKey(theme)).(*templates.Bundle); b != nil {
		blob, err := b.Render(c, name, args)
		if err != nil {
			panic(fmt.Errorf("Could not render template %s from theme %s:\n%s", name, theme, err))
		}
		_, err = out.Write(blob)
		if err != nil {
			panic(fmt.Errorf("Could not write out template %s from theme %s:\n%s", name, theme, err))
		}
		return
	}
	panic(fmt.Errorf("Error: Could not load template %s from theme %s", name, theme))
}

// Base adds the basic luci appengine middlewares.
func Base(h middleware.Handler) httprouter.Handle {
	methods := auth.Authenticator{
		&server.OAuth2Method{Scopes: []string{server.EmailScope}},
		server.CookieAuth,
		&server.InboundAppIDAuthMethod{},
	}
	for _, nb := range getTemplateBundles() {
		h = withNamedBundle(h, nb)
	}
	if !appengine.IsDevAppServer() {
		h = middleware.WithPanicCatcher(h)
	}
	return gaemiddleware.BaseProd(auth.Use(h, methods))
}

// Wrap wraps Milo "Render" functions and emits a middleware.Handler function.  Of note
// is that Render functions' interface into rendering is purely through a single
// templates.Args value which gets rendered here, while the http.ResponseWriter
// is stripped out.
func Wrap(h ThemedHandler) func(http.ResponseWriter, *http.Request, httprouter.Params) {
	hx := func(c context.Context, w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		// Figure out if we need to do the things.
		theme := GetTheme(c, r)
		template := h.GetTemplateName(theme)

		// Do the things.
		args, err := h.Render(c, r, p)

		// Throw errors.
		// TODO(hinoka): Add themes and templates for errors so they look better.
		if err != nil {
			if merr, ok := err.(*miloerror.Error); ok {
				http.Error(w, merr.Message, merr.Code)
			} else {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
			return
		}

		// Render the stuff.
		name := fmt.Sprintf("pages/%s", template)
		themedMustRender(c, w, theme.Name, name, *args)
	}
	return Base(hx)
}
