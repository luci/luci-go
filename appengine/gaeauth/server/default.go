// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package server

import (
	"runtime"
	"strings"

	"golang.org/x/net/context"
	"google.golang.org/appengine"

	gae_info "github.com/luci/gae/service/info"
	"github.com/luci/luci-go/server/auth"
	"github.com/luci/luci-go/server/auth/info"
	"github.com/luci/luci-go/server/auth/openid"
	"github.com/luci/luci-go/server/auth/signing"
	"github.com/luci/luci-go/server/router"

	"github.com/luci/luci-go/appengine/gaeauth/server/internal/authdb"
)

// CookieAuth is default cookie-based auth method to use on GAE.
//
// On dev server it is based on dev server cookies, in prod it is based on
// OpenID. Works only if appropriate handlers have been installed into
// the router. See InstallHandlers.
var CookieAuth auth.Method

// InstallHandlers installs HTTP handlers for various default routes related
// to authentication system.
//
// Must be installed in server HTTP router for authentication to work.
func InstallHandlers(r *router.Router, base router.MiddlewareChain) {
	m := CookieAuth.(cookieAuthMethod)
	if oid, ok := m.Method.(*openid.AuthMethod); ok {
		oid.InstallHandlers(r, base)
	}
	auth.InstallHandlers(r, base)
	authdb.InstallHandlers(r, base)
	info.InstallHandlers(r, base, getServiceInfo)
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

func getServiceInfo(c context.Context) (info.ServiceInfo, error) {
	i := gae_info.Get(c)

	account, err := i.ServiceAccount()
	if err != nil {
		return info.ServiceInfo{}, err
	}
	return info.ServiceInfo{
		AppID:              i.AppID(),
		AppRuntime:         "go",
		AppRuntimeVersion:  runtime.Version(),
		AppVersion:         strings.Split(i.VersionID(), ".")[0],
		ServiceAccountName: account,
	}, nil
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
				SessionStore:        &SessionStore{Prefix: "openid"},
				IncompatibleCookies: []string{"SACSID", "dev_appserver_login"},
			},
		}
	}
}
