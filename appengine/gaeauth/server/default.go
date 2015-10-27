// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package server

import (
	"google.golang.org/appengine"

	"github.com/luci/luci-go/server/auth"
	"github.com/luci/luci-go/server/auth/openid"
)

// NewAuthenticator returns new instance of auth.Authenticator configured to
// work on Appengine.
func NewAuthenticator() *auth.Authenticator {
	var cookieMethod auth.Method
	if appengine.IsDevAppServer() {
		// Use dev server login instead of OpenID. It simpler to use on dev server,
		// since it doesn't require additional configuration.
		cookieMethod = &CookieAuthMethod{}
	} else {
		// Use OpenID in prod.
		cookieMethod = &openid.AuthMethod{
			SessionStore:        &SessionStore{Namespace: "openid"},
			IncompatibleCookies: []string{"SACSID", "dev_appserver_login"},
		}
	}
	return auth.NewAuthenticator([]auth.Method{
		&OAuth2Method{},
		cookieMethod,
		&InboundAppIDAuthMethod{},
	})
}
