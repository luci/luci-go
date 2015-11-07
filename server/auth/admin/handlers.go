// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package admin

import (
	"net/http"

	"github.com/julienschmidt/httprouter"
	"golang.org/x/net/context"

	"github.com/luci/luci-go/server/auth"
	"github.com/luci/luci-go/server/auth/admin/internal/assets"
	"github.com/luci/luci-go/server/middleware"
	"github.com/luci/luci-go/server/templates"
)

// InstallHandlers installs HTTP handlers that implement admin UI.
//
// `adminAuth` is the method that will be used to authenticate the access
// (regardless of what's installed in the base context). It must be able to
// distinguish admins (aka superusers) from non-admins.
//
// auth.CurrentUser(...).Superuser is set to true for admins.
func InstallHandlers(r *httprouter.Router, base middleware.Base, adminAuth auth.Method) {
	tmpl := prepareTemplates()

	wrap := func(h middleware.Handler) httprouter.Handle {
		h = adminOnly(h)
		h = auth.Use(h, auth.Authenticator{adminAuth})
		h = templates.WithTemplates(h, tmpl)
		return base(h)
	}

	r.GET("/auth/admin/settings", wrap(settingsPage))
}

///

// prepareTemplates configures templates.Bundle.
func prepareTemplates() *templates.Bundle {
	return &templates.Bundle{
		Loader:          templates.AssetsLoader(assets.Assets()),
		DefaultTemplate: "base",
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
}

// adminOnly is middleware that ensures authenticated user is an admin.
func adminOnly(h middleware.Handler) middleware.Handler {
	return auth.Autologin(func(c context.Context, rw http.ResponseWriter, r *http.Request, p httprouter.Params) {
		if !auth.CurrentUser(c).Superuser {
			rw.WriteHeader(http.StatusForbidden)
			templates.MustRender(c, rw, "pages/access_denied.html", nil)
			return
		}
		h(c, rw, r, p)
	})
}

// settingsPage renders the settings page.
func settingsPage(c context.Context, rw http.ResponseWriter, r *http.Request, p httprouter.Params) {
	templates.MustRender(c, rw, "pages/settings.html", nil)
}
