// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package admin implements HTTP routes for settings UI.
package admin

import (
	"html/template"
	"net"
	"net/http"

	"github.com/julienschmidt/httprouter"
	"golang.org/x/net/context"

	"github.com/luci/luci-go/common/errors"

	"github.com/luci/luci-go/server/auth"
	"github.com/luci/luci-go/server/auth/identity"
	"github.com/luci/luci-go/server/auth/xsrf"
	"github.com/luci/luci-go/server/middleware"
	"github.com/luci/luci-go/server/templates"

	"github.com/luci/luci-go/server/settings/admin/internal/assets"
)

// InstallHandlers installs HTTP handlers that implement admin UI.
//
// `adminAuth` is the method that will be used to authenticate the access
// (regardless of what's installed in the base context). It must be able to
// distinguish admins (aka superusers) from non-admins. It is needed because
// settings UI must be usable even before auth system is configured.
func InstallHandlers(r *httprouter.Router, base middleware.Base, adminAuth auth.Method) {
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
		auth.ErroringDB{
			Error: errors.New("admin: unexpected call to auth.DB on admin page"),
		},
	}

	wrap := func(h middleware.Handler) httprouter.Handle {
		h = adminOnly(h)
		h = auth.WithDB(h, func(c context.Context) (auth.DB, error) {
			return adminDB, nil
		})
		h = auth.Use(h, auth.Authenticator{adminAuth})
		h = templates.WithTemplates(h, tmpl)
		return base(h)
	}

	r.GET("/admin/settings", wrap(indexPage))
	r.GET("/admin/settings/:SettingsKey", wrap(settingsPageGET))
	r.POST("/admin/settings/:SettingsKey", wrap(xsrf.WithTokenCheck(settingsPagePOST)))
}

// replyError sends HTML error page with status 500 on transient errors or 400
// on fatal ones.
func replyError(c context.Context, rw http.ResponseWriter, err error) {
	if errors.IsTransient(err) {
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
	auth.ErroringDB
}

func (adminBypassDB) GetWhitelistForIdentity(c context.Context, ident identity.Identity) (string, error) {
	return "", nil
}

func (adminBypassDB) IsInWhitelist(c context.Context, ip net.IP, whitelist string) (bool, error) {
	return false, nil
}

// adminOnly is middleware that ensures authenticated user is local site admin
// aka superuser. On GAE it grants access only to users that have Editor or
// Owner roles in the Cloud Project.
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
