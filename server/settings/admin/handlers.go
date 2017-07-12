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

// Package admin implements HTTP routes for settings UI.
package admin

import (
	"html/template"
	"net"
	"net/http"

	"golang.org/x/net/context"

	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/retry/transient"

	"github.com/luci/luci-go/server/auth"
	"github.com/luci/luci-go/server/auth/authdb"
	"github.com/luci/luci-go/server/auth/identity"
	"github.com/luci/luci-go/server/auth/xsrf"
	"github.com/luci/luci-go/server/router"
	"github.com/luci/luci-go/server/templates"

	"github.com/luci/luci-go/server/settings/admin/internal/assets"
)

// InstallHandlers installs HTTP handlers that implement admin UI.
//
// `adminAuth` is the method that will be used to authenticate the access
// (regardless of what's installed in the base context). It must be able to
// distinguish admins (aka superusers) from non-admins. It is needed because
// settings UI must be usable even before auth system is configured.
func InstallHandlers(r *router.Router, base router.MiddlewareChain, adminAuth auth.Method) {
	tmpl := &templates.Bundle{
		Loader:          templates.AssetsLoader(assets.Assets()),
		DefaultTemplate: "base",
		FuncMap: template.FuncMap{
			"includeCSS": func(name string) template.CSS {
				return template.CSS(assets.GetAsset(name))
			},
		},
		DefaultArgs: func(c context.Context) (templates.Args, error) {
			logoutURL, err := auth.LogoutURL(c, "/")
			if err != nil {
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

	rr := r.Subrouter("/admin/settings")
	rr.Use(base.Extend(
		templates.WithTemplates(tmpl),
		adminDB.install,
		auth.Authenticate(adminAuth),
		adminAutologin,
	))

	rr.GET("", router.MiddlewareChain{}, indexPage)
	rr.GET("/:SettingsKey", router.MiddlewareChain{}, settingsPageGET)
	rr.POST("/:SettingsKey", router.NewMiddlewareChain(xsrf.WithTokenCheck), settingsPagePOST)
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

// adminBypassDB skips IP whitelist checks (assuming no IPs are whitelisted) and
// errors on all other checks.
//
// It is needed to make admin pages accessible even when AuthDB is not
// configured.
type adminBypassDB struct {
	authdb.ErroringDB
}

func (adminBypassDB) GetWhitelistForIdentity(c context.Context, ident identity.Identity) (string, error) {
	return "", nil
}

func (adminBypassDB) IsInWhitelist(c context.Context, ip net.IP, whitelist string) (bool, error) {
	return false, nil
}

func (d adminBypassDB) install(c *router.Context, next router.Handler) {
	c.Context = auth.ModifyConfig(c.Context, func(cfg auth.Config) auth.Config {
		cfg.DBProvider = func(context.Context) (authdb.DB, error) {
			return d, nil
		}
		return cfg
	})
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
	u := auth.CurrentUser(c.Context)

	// Redirect anonymous users to a login page that redirects back to the current
	// page.
	if u.Identity == identity.AnonymousIdentity {
		// Make the current URL relative to the host.
		destURL := *c.Request.URL
		destURL.Host = ""
		destURL.Scheme = ""
		url, err := auth.LoginURL(c.Context, destURL.String())
		if err != nil {
			logging.WithError(err).Errorf(c.Context, "Error when generating login URL")
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

	// Non anonymous users must be admins to proceed.
	if !u.Superuser {
		c.Writer.WriteHeader(http.StatusForbidden)
		templates.MustRender(c.Context, c.Writer, "pages/access_denied.html", nil)
		return
	}

	next(c)
}
