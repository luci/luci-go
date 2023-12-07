// Copyright 2016 The LUCI Authors.
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

// Package portal implements HTTP routes for portal pages.
//
// These pages can be registered at init()-time, and will be routed to
// /admin/portal.
//
// Typically they read/write `settings` as defined by
// `go.chromium.org/luci/server/settings`, but they can also be used to provide
// information to administrators or to provide admin-only actions (such as
// clearing queues or providing admin tokens).
package portal

import (
	"context"
	"html/template"
	"net"
	"net/http"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"

	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authdb"
	"go.chromium.org/luci/server/auth/xsrf"
	"go.chromium.org/luci/server/portal/internal/assets"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/templates"
)

// AssumeTrustedPort can be passed as auth.Method to InstallHandlers to indicate
// that portal endpoints are being exposed on an internal port accessible only
// to cluster administrators and no additional auth checks are required (or they
// are not possible).
var AssumeTrustedPort auth.Method = trustedPortAuth{}

// InstallHandlers installs HTTP handlers that implement admin UI.
//
// `adminAuth` is the method that will be used to authenticate the access
// (regardless of what's installed in the base context). It must be able to
// distinguish admins (aka superusers) from non-admins. It is needed because
// settings UI must be usable even before auth system is configured.
//
// `adminAuth` can be a special value portal.AssumeTrustedPort which completely
// disables all authentication and authorization checks (by delegating them to
// the network layer).
func InstallHandlers(r *router.Router, base router.MiddlewareChain, adminAuth auth.Method) {
	tmpl := &templates.Bundle{
		Loader:          templates.AssetsLoader(assets.Assets()),
		DefaultTemplate: "base",
		FuncMap: template.FuncMap{
			"includeCSS": func(name string) template.CSS { return template.CSS(assets.GetAsset(name)) },
			"includeJS":  func(name string) template.JS { return template.JS(assets.GetAsset(name)) },
		},
		DefaultArgs: func(c context.Context, e *templates.Extra) (templates.Args, error) {
			logoutURL, err := auth.LogoutURL(c, "/")
			if err != nil && err != auth.ErrNoUsersAPI {
				return nil, err
			}
			return templates.Args{
				"Email":     auth.CurrentUser(c).Email,
				"LogoutURL": logoutURL,
			}, nil
		},
	}

	adminDB := adminBypassDB{
		authdb.ErroringDB{
			Error: errors.New("admin: unexpected call to authdb.DB on admin page"),
		},
	}

	rr := r.Subrouter("/admin/portal")
	rr.Use(base.Extend(
		templates.WithTemplates(tmpl),
		adminDB.install,
		auth.Authenticate(adminAuth),
		adminAutologin,
	))

	rr.GET("", nil, indexPage)
	rr.GET("/:PageKey", nil, portalPageGET)
	rr.POST("/:PageKey", router.NewMiddlewareChain(xsrf.WithTokenCheck), portalPagePOST)
	rr.GET("/:PageKey/:ActionID", nil, portalActionGETPOST)
	rr.POST("/:PageKey/:ActionID", router.NewMiddlewareChain(xsrf.WithTokenCheck), portalActionGETPOST)
}

// replyError sends HTML error page with status 500 on transient errors or 400
// on fatal ones.
func replyError(c context.Context, rw http.ResponseWriter, err error) {
	if transient.Tag.In(err) {
		rw.WriteHeader(http.StatusInternalServerError)
	} else {
		rw.WriteHeader(http.StatusBadRequest)
	}
	templates.MustRender(c, rw, "pages/error.html", templates.Args{
		"Error": err.Error(),
	})
}

////////////////////////////////////////////////////////////////////////////////
// Auth related helpers.

// trustedPortAuth is auth.Method that assumes all request are coming from a
// super-admin through a trusted internal port.
type trustedPortAuth struct{}

// Authenticate is part of auth.Method interface.
//
// It returns User with Anonymous identity that has Superuser bit set: we
// know *some* admin is accessing endpoints (thus we set Superuser bit), but
// don't know who they are exactly (thus setting Anonymous identity).
func (trustedPortAuth) Authenticate(context.Context, auth.RequestMetadata) (*auth.User, auth.Session, error) {
	return &auth.User{
		Identity:  identity.AnonymousIdentity,
		Superuser: true,
	}, nil, nil
}

// adminBypassDB skips IP allowlist checks (assuming no IPs are allowlisted) and
// errors on all other checks.
//
// It is needed to make admin pages accessible even when AuthDB is not
// configured.
type adminBypassDB struct {
	authdb.ErroringDB
}

func (adminBypassDB) GetAllowlistForIdentity(c context.Context, ident identity.Identity) (string, error) {
	return "", nil
}

func (adminBypassDB) IsAllowedIP(c context.Context, ip net.IP, allowlist string) (bool, error) {
	return false, nil
}

func (d adminBypassDB) install(c *router.Context, next router.Handler) {
	c.Request = c.Request.WithContext(auth.ModifyConfig(c.Request.Context(), func(cfg auth.Config) auth.Config {
		cfg.DBProvider = func(context.Context) (authdb.DB, error) {
			return d, nil
		}
		return cfg
	}))
	next(c)
}

// adminAutologin is middleware that ensures authenticated user is local site
// admin (aka superuser).
//
// On GAE it grants access only to users that have Editor or Owner roles in
// the Cloud Project.
//
// It redirect anonymous users to login page, and displays "Access denied" page
// to authenticated non-admin users.
func adminAutologin(c *router.Context, next router.Handler) {
	u := auth.CurrentUser(c.Request.Context())

	// Redirect anonymous users to a login page that redirects back to the current
	// page. Don't do it if this anonymous user is also marked as Superuser, which
	// happens when using AssumeTrustedPort auth method.
	if u.Identity == identity.AnonymousIdentity && !u.Superuser {
		// Make the current URL relative to the host.
		destURL := *c.Request.URL
		destURL.Host = ""
		destURL.Scheme = ""
		url, err := auth.LoginURL(c.Request.Context(), destURL.String())
		if err != nil {
			logging.WithError(err).Errorf(c.Request.Context(), "Error when generating login URL")
			if transient.Tag.In(err) {
				http.Error(c.Writer, "Transient error when generating login URL, see logs", 500)
			} else {
				http.Error(c.Writer, "Can't generate login URL, see logs", 401)
			}
			return
		}
		http.Redirect(c.Writer, c.Request, url, 302)
		return
	}

	// Only superusers can proceed.
	if !u.Superuser {
		c.Writer.WriteHeader(http.StatusForbidden)
		templates.MustRender(c.Request.Context(), c.Writer, "pages/access_denied.html", nil)
		return
	}

	next(c)
}
