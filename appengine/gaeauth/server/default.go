// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package server

import (
	"github.com/julienschmidt/httprouter"
	"golang.org/x/net/context"
	"google.golang.org/appengine"

	"github.com/luci/luci-go/server/auth"
	"github.com/luci/luci-go/server/auth/admin"
	"github.com/luci/luci-go/server/auth/openid"
	"github.com/luci/luci-go/server/auth/signing"
	"github.com/luci/luci-go/server/middleware"

	"github.com/luci/luci-go/appengine/gaeauth/server/internal/authdb"
)

// CookieAuth is default cookie-based auth method to use on GAE.
//
// On dev server it is based on dev server cookies, in prod it is based on
// OpenID. Works only if appropriate handlers have been installed into
// the router. See InstallHandlers.
var CookieAuth auth.Method

// InstallHandlers installs HTTP handlers for various routes related
// to authentication system.
//
// Must be installed in server HTTP router for authentication to work.
func InstallHandlers(r *httprouter.Router, base middleware.Base) {
	m := CookieAuth.(cookieAuthMethod)
	if oid, ok := m.Method.(*openid.AuthMethod); ok {
		oid.InstallHandlers(r, base)
	}
	admin.InstallHandlers(r, base, &UsersAPIAuthMethod{}, adminPagesConfig{})
	auth.InstallHandlers(r, base)
	authdb.InstallHandlers(r, base)
	signing.InstallHandlers(r, base)
}

// Warmup prepares local caches. It's optional.
func Warmup(c context.Context) error {
	m := CookieAuth.(cookieAuthMethod)
	if oid, ok := m.Method.(*openid.AuthMethod); ok {
		return oid.Warmup(c)
	}
	return nil
}

///

// adminPagesConfig is used by server/auth/admin to display admin UI
type adminPagesConfig struct{}

func (adminPagesConfig) GetAppServiceAccount(c context.Context) (string, error) {
	return appengine.ServiceAccount(c)
}

func (adminPagesConfig) GetReplicationState(c context.Context) (string, int64, error) {
	info, err := authdb.GetLatestSnapshotInfo(c)
	if info != nil {
		return info.AuthServiceURL, info.Rev, nil
	}
	return "", 0, err
}

func (adminPagesConfig) ConfigureAuthService(c context.Context, baseURL, authServiceURL string) error {
	return authdb.ConfigureAuthService(c, baseURL, authServiceURL)
}

///

// cookieAuthMethod implements union of openid.AuthMethod and UsersAPIAuthMethod
// methods, routing calls appropriately.
type cookieAuthMethod struct {
	auth.Method
}

func (m cookieAuthMethod) LoginURL(c context.Context, dest string) (string, error) {
	return m.Method.(auth.UsersAPI).LoginURL(c, dest)
}

func (m cookieAuthMethod) LogoutURL(c context.Context, dest string) (string, error) {
	return m.Method.(auth.UsersAPI).LogoutURL(c, dest)
}

func init() {
	if appengine.IsDevAppServer() {
		CookieAuth = cookieAuthMethod{UsersAPIAuthMethod{}}
	} else {
		CookieAuth = cookieAuthMethod{
			&openid.AuthMethod{
				SessionStore:        &SessionStore{Namespace: "openid"},
				IncompatibleCookies: []string{"SACSID", "dev_appserver_login"},
			},
		}
	}
}
