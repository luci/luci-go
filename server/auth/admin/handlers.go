// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package admin

import (
	"fmt"
	"html/template"
	"net/http"
	"net/url"

	"github.com/julienschmidt/httprouter"
	"golang.org/x/net/context"

	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/logging"

	"github.com/luci/luci-go/server/auth"
	"github.com/luci/luci-go/server/auth/admin/internal/assets"
	"github.com/luci/luci-go/server/auth/openid"
	"github.com/luci/luci-go/server/auth/xsrf"
	"github.com/luci/luci-go/server/middleware"
	"github.com/luci/luci-go/server/settings"
	"github.com/luci/luci-go/server/templates"
)

// Config is interface between admin pages and component that talks to
// AuthService to fetch groups (there may be multiple implementations, for GAE
// and non-GAE).
type Config interface {
	// GetAppServiceAccount returns service account representing the application.
	GetAppServiceAccount(context.Context) (string, error)

	// GetReplicationState returns current URL of main auth_service and latest
	// processed revision number (or "", 0 if not configured).
	GetReplicationState(context.Context) (authServiceURL string, rev int64, err error)

	// ConfigureAuthService changes URL of auth_service to fetch groups from. Does
	// nothing if already using given auth_service.
	//
	// `baseURL` is root URL of the current service. Will be used to construct
	// PubSub push URLs.
	//
	// If `authServiceURL` is blank, disables the fetching.
	ConfigureAuthService(c context.Context, baseURL, authServiceURL string) error
}

// InstallHandlers installs HTTP handlers that implement admin UI.
//
// `adminAuth` is the method that will be used to authenticate the access
// (regardless of what's installed in the base context). It must be able to
// distinguish admins (aka superusers) from non-admins.
//
// auth.CurrentUser(...).Superuser is set to true for admins.
func InstallHandlers(r *httprouter.Router, base middleware.Base, adminAuth auth.Method, cfg Config) {
	tmpl := prepareTemplates()

	wrap := func(h middleware.Handler) httprouter.Handle {
		h = adminOnly(h)
		h = auth.Use(h, auth.Authenticator{adminAuth})
		h = templates.WithTemplates(h, tmpl)
		h = middleware.WithContextValue(h, contextKey(0), cfg)
		return base(h)
	}

	r.GET("/auth/admin/settings", wrap(settingsPage))
	r.POST("/auth/admin/settings", wrap(xsrf.WithTokenCheck(storeSettings)))
}

///

type contextKey int

// getConfig extracts Config from the context or returns nil.
//
// It's put there by 'wrap' in InstallHandlers.
func getConfig(c context.Context) Config {
	cfg, _ := c.Value(contextKey(0)).(Config)
	return cfg
}

// prepareTemplates configures templates.Bundle.
func prepareTemplates() *templates.Bundle {
	return &templates.Bundle{
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

///

// settingsPage renders the settings page.
func settingsPage(c context.Context, rw http.ResponseWriter, r *http.Request, p httprouter.Params) {
	oidSettings := openid.Settings{}
	err := settings.GetUncached(c, openid.SettingsKey, &oidSettings)
	if err != nil && err != settings.ErrNoSettings {
		replyError(c, rw, err)
		return
	}

	cfg := getConfig(c)
	if cfg == nil {
		replyError(c, rw, errors.New("config interface is not provided"))
		return
	}

	serviceAcc, err := cfg.GetAppServiceAccount(c)
	if err != nil {
		replyError(c, rw, err)
		return
	}

	authServiceURL, authRev, err := cfg.GetReplicationState(c)
	if err != nil {
		replyError(c, rw, err)
		return
	}

	templates.MustRender(c, rw, "pages/settings.html", templates.Args{
		"AuthServiceURL":     authServiceURL,
		"AuthLatestRev":      authRev,
		"ServiceAccount":     serviceAcc,
		"OpenID":             oidSettings,
		"DefaultRedirectURI": "https://" + r.Host + "/auth/openid/callback",
		"XsrfTokenField":     xsrf.TokenField(c),
	})
}

// storeSettings is POST handler that updates the settings.
func storeSettings(c context.Context, rw http.ResponseWriter, r *http.Request, p httprouter.Params) {
	oidSettings := openid.Settings{
		DiscoveryURL: r.PostFormValue("DiscoveryURL"),
		ClientID:     r.PostFormValue("ClientID"),
		ClientSecret: r.PostFormValue("ClientSecret"),
		RedirectURI:  r.PostFormValue("RedirectURI"),
	}
	err := saveOpenIDSettings(c, &oidSettings)
	if err == nil {
		err = saveAuthServiceURL(c, "https://"+r.Host, r.PostFormValue("AuthServiceURL"))
	}
	if err != nil {
		replyError(c, rw, err)
		return
	}
	templates.MustRender(c, rw, "pages/done.html", nil)
}

///

func saveOpenIDSettings(c context.Context, s *openid.Settings) error {
	existing := openid.Settings{}
	err := settings.GetUncached(c, openid.SettingsKey, &existing)
	if err != nil && err != settings.ErrNoSettings {
		return err
	}
	if existing == *s {
		return nil
	}
	logging.Warningf(c, "OpenID settings changed from %q to %q", existing, *s)
	return settings.Set(c, openid.SettingsKey, s, auth.CurrentUser(c).Email, "via /auth/admin/settings")
}

func saveAuthServiceURL(c context.Context, baseURL, authServiceURL string) error {
	if authServiceURL != "" {
		parsed, err := url.Parse(authServiceURL)
		if err != nil {
			return fmt.Errorf("bad URL %q - %s", authServiceURL, err)
		}
		if !parsed.IsAbs() || parsed.Path != "" {
			return fmt.Errorf("bad URL %q - must be host root URL", authServiceURL)
		}
	}
	cfg := getConfig(c)
	if cfg == nil {
		return errors.New("config interface is not provided")
	}
	return cfg.ConfigureAuthService(c, baseURL, authServiceURL)
}
